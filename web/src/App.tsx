import { useDeferredValue, useEffect, useMemo, useRef, useState, type CSSProperties, type DragEvent } from "react";
import type {
  Catalog,
  CatalogItem,
  CoverageGroup,
  CoordTuple,
  HeroFilter,
  ItemVisualMetadataMap,
  Placement,
  Recipe,
  RuntimeItemMetadata,
  RuntimeMetadata,
  RemoteSolveMetadata,
  Scenario,
  Solution,
  SolveProgress,
  SolverBackend,
  Star,
} from "./types";
import { evaluateLayoutWithWasm, RemoteSolveError, solveWithOci, solveWithRemote, solveWithWorker } from "./wasm";

const ROWS = 6;
const COLS = 9;

type Grid = boolean[][];
type HeroViewMode = "all" | "hero" | "shared";
type OutgoingPrioritySemantics = "outgoing-v2" | "outgoing-per-instance-v3";

interface DraftPlacementSpec {
  instanceID: string;
  itemID: string;
  rotation: number;
  origin: CoordTuple;
}

interface ManualEditorPlacement extends DraftPlacementSpec {
  key: string;
}

// Transcribed from the supplied screenshot. This remains the manual-editor
// source; its literal solver evaluation is frozen separately as an oracle.
const PRINT_RECONSTRUCTION: DraftPlacementSpec[] = [
  { instanceID: "banana#0", itemID: "banana", rotation: 0, origin: [2, 2] },
  { instanceID: "cactrio#1", itemID: "cactrio", rotation: 0, origin: [0, 0] },
  { instanceID: "champion_s_ripper#2", itemID: "champion_s_ripper", rotation: 0, origin: [1, 6] },
  { instanceID: "cleansing_crown#3", itemID: "cleansing_crown", rotation: 90, origin: [1, 0] },
  { instanceID: "death_essence#4", itemID: "death_essence", rotation: 0, origin: [3, 7] },
  { instanceID: "discordant_harp#5", itemID: "discordant_harp", rotation: 270, origin: [0, 4] },
  { instanceID: "donut#6", itemID: "donut", rotation: 0, origin: [4, 3] },
  { instanceID: "ginseng_root#7", itemID: "ginseng_root", rotation: 90, origin: [4, 0] },
  { instanceID: "ginseng_root#8", itemID: "ginseng_root", rotation: 90, origin: [3, 5] },
  { instanceID: "green_snapper#9", itemID: "green_snapper", rotation: 0, origin: [5, 5] },
  { instanceID: "hooded_cowl#10", itemID: "hooded_cowl", rotation: 0, origin: [0, 7] },
  { instanceID: "longing_begonia#11", itemID: "longing_begonia", rotation: 90, origin: [0, 2] },
  { instanceID: "pitahaya#12", itemID: "pitahaya", rotation: 90, origin: [3, 0] },
  { instanceID: "pitahaya#13", itemID: "pitahaya", rotation: 0, origin: [4, 4] },
  { instanceID: "spice#14", itemID: "spice", rotation: 270, origin: [3, 4] },
  { instanceID: "spice#15", itemID: "spice", rotation: 0, origin: [5, 1] },
  { instanceID: "spice#16", itemID: "spice", rotation: 0, origin: [5, 2] },
  { instanceID: "spicy_sausage#17", itemID: "spicy_sausage", rotation: 0, origin: [2, 1] },
  { instanceID: "spiked_sickle#18", itemID: "spiked_sickle", rotation: 180, origin: [1, 4] },
  { instanceID: "spirit_biscuit#19", itemID: "spirit_biscuit", rotation: 0, origin: [5, 3] },
  { instanceID: "steadfast_boots#20", itemID: "steadfast_boots", rotation: 180, origin: [2, 7] },
  { instanceID: "tender_sausage#21", itemID: "tender_sausage", rotation: 0, origin: [4, 1] },
  { instanceID: "thornwall#22", itemID: "thornwall", rotation: 0, origin: [4, 7] },
  { instanceID: "twinmaw#23", itemID: "twinmaw", rotation: 90, origin: [1, 1] },
];

const PRINT_RECONSTRUCTION_ITEMS = PRINT_RECONSTRUCTION.reduce<Record<string, number>>((items, placement) => {
  items[placement.itemID] = (items[placement.itemID] || 0) + 1;
  return items;
}, {});

const defaultGrid = (): Grid => Array.from({ length: ROWS }, () => Array.from({ length: COLS }, () => true));
const emptyGrid = (): Grid => Array.from({ length: ROWS }, () => Array.from({ length: COLS }, () => false));
const defaultSolverBackend = (): SolverBackend =>
  (import.meta as unknown as { env?: { PROD?: boolean } }).env?.PROD ? "remote" : "local";
const OCI_ENDPOINT_STORAGE_KEY = "backpack_solver_oci_endpoint";
const OCI_TOKEN_STORAGE_KEY = "backpack_solver_oci_token";

type PriorityKind = "star_source" | "craft";

interface PriorityOption {
  key: string;
  kind: PriorityKind;
  itemID: string;
  title: string;
  subtitle: string;
  imagePath: string;
}

type GlobalPriorityEntry =
  | { key: string; kind: "coverage_group"; groupIndex: number; group: CoverageGroup }
  | { key: string; kind: PriorityKind; option: PriorityOption };

interface WebScenarioInput {
  grid: Grid;
  quantities: Record<string, number>;
  top: number;
  maxNodes: number;
  noSkips: boolean;
  stopOnCoverageCeiling: boolean;
  stopOnPriorityCeiling: boolean;
  repairSearch: boolean;
  prioritySemantics: OutgoingPrioritySemantics;
  priorityOrder: string[];
  priorityOptionsByKey: Map<string, PriorityOption>;
  coverageGroups: CoverageGroup[];
  starSourceIDs: string[];
  disabledPrioritySet: Set<string>;
  heroFilter: HeroFilter;
}

interface DebugPriorityState {
  priority_order: string[];
  disabled_priority_keys: string[];
  coverage_groups_configured: CoverageGroup[];
  loose_star_priorities: Array<{ key: string; item_id: string; title: string; enabled: boolean }>;
  craft_priorities: Array<{ key: string; item_id: string; title: string; enabled: boolean }>;
}

interface SelectedItemPlacementSummary {
  item_id: string;
  item_name: string;
  selected: number;
  placed: number;
  not_placed: number;
}

export default function App() {
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [runtimeMetadata, setRuntimeMetadata] = useState<RuntimeMetadata | null>(null);
  const [itemVisualMetadata, setItemVisualMetadata] = useState<ItemVisualMetadataMap>({});
  const [grid, setGrid] = useState<Grid>(defaultGrid);
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  const [query, setQuery] = useState("");
  const deferredQuery = useDeferredValue(query);
  const [heroViewMode, setHeroViewMode] = useState<HeroViewMode>("all");
  const [selectedHeroID, setSelectedHeroID] = useState("");
  const [excludedHeroID, setExcludedHeroID] = useState("");
  const [heroExcludeMode, setHeroExcludeMode] = useState<"strict" | "exclusive_only">("strict");
  const [top, setTop] = useState(3);
  const [maxNodes, setMaxNodes] = useState(0);
  const [noSkips, setNoSkips] = useState(false);
  const [stopOnCoverageCeiling, setStopOnCoverageCeiling] = useState(false);
  const [stopOnPriorityCeiling, setStopOnPriorityCeiling] = useState(false);
  const [repairSearch, setRepairSearch] = useState(true);
  const [prioritySemantics, setPrioritySemantics] = useState<OutgoingPrioritySemantics>("outgoing-v2");
  const [solutions, setSolutions] = useState<Solution[]>([]);
  const [partialResultStatus, setPartialResultStatus] = useState<"running" | "canceled" | null>(null);
  const [selectedSolution, setSelectedSolution] = useState(0);
  const [isSolving, setIsSolving] = useState(false);
  const [solveAbortController, setSolveAbortController] = useState<AbortController | null>(null);
  const [solveProgress, setSolveProgress] = useState<SolveProgress | null>(null);
  const [solveDurationMs, setSolveDurationMs] = useState<number | null>(null);
  const [solverBackend, setSolverBackend] = useState<SolverBackend>(defaultSolverBackend);
  const [ociEndpoint, setOciEndpoint] = useState(() => localStorage.getItem(OCI_ENDPOINT_STORAGE_KEY) || "");
  const [ociToken, setOciToken] = useState(() => localStorage.getItem(OCI_TOKEN_STORAGE_KEY) || "");
  const [lastSolveBackend, setLastSolveBackend] = useState<SolverBackend | null>(null);
  const [remoteSolveMetadata, setRemoteSolveMetadata] = useState<RemoteSolveMetadata | null>(null);
  const [remoteFallbackNotice, setRemoteFallbackNotice] = useState<string | null>(null);
  const [hoveredItemID, setHoveredItemID] = useState<string | null>(null);
  const [pinnedItemID, setPinnedItemID] = useState<string | null>(null);
  const [priorityOrder, setPriorityOrder] = useState<string[]>([]);
  const [coverageGroups, setCoverageGroups] = useState<CoverageGroup[]>([]);
  const [disabledPriorityKeys, setDisabledPriorityKeys] = useState<string[]>([]);
  const [draggingPriorityKey, setDraggingPriorityKey] = useState<string | null>(null);
  const [draggingStarSourceID, setDraggingStarSourceID] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [debugCopyStatus, setDebugCopyStatus] = useState<string | null>(null);
  const [debugFallbackText, setDebugFallbackText] = useState<string | null>(null);
	const [printReconstructionPreview, setPrintReconstructionPreview] = useState(false);
	const [manualEditorPlacements, setManualEditorPlacements] = useState<ManualEditorPlacement[]>([]);
	const [manualLayoutStatus, setManualLayoutStatus] = useState<string | null>(null);
	const [manualLayoutEvaluating, setManualLayoutEvaluating] = useState(false);
  const lastPartialSolutionsRef = useRef<{ value: Solution[] | null; signature: string | null }>({ value: null, signature: null });
  const initializedStarPriorityKeysRef = useRef(new Set<string>());

  useEffect(() => {
    void loadInitialState();
  }, []);

  useEffect(() => {
    localStorage.setItem(OCI_ENDPOINT_STORAGE_KEY, ociEndpoint.trim());
  }, [ociEndpoint]);

  useEffect(() => {
    localStorage.setItem(OCI_TOKEN_STORAGE_KEY, ociToken);
  }, [ociToken]);

  async function loadInitialState() {
    try {
      const [catalogResponse, scenarioResponse, metadataResponse, visualMetadataResponse] = await Promise.all([
        fetch("/data/catalog.json"),
        fetch("/scenarios/spinegrowth-basic.json"),
        fetch("/data/item-metadata.json"),
        fetch("/data/item-visual-metadata.json"),
      ]);
      const loadedCatalog = (await catalogResponse.json()) as Catalog;
      const loadedScenario = (await scenarioResponse.json()) as Scenario;
      const loadedMetadata = (await metadataResponse.json()) as RuntimeMetadata;
      const loadedVisualMetadata = (await visualMetadataResponse.json()) as ItemVisualMetadataMap;
      setCatalog(loadedCatalog);
      setRuntimeMetadata(loadedMetadata);
      setItemVisualMetadata(loadedVisualMetadata);
      if (loadedScenario.grid) {
        setGrid(gridFromRows(loadedScenario.grid));
      }
      setQuantities({});
      setTop(loadedScenario.top ?? 3);
      setMaxNodes(loadedScenario.max_nodes ?? 0);
      setNoSkips(loadedScenario.no_skips ?? false);
      setStopOnCoverageCeiling(loadedScenario.stop_on_coverage_ceiling ?? false);
      setStopOnPriorityCeiling(loadedScenario.stop_on_priority_ceiling ?? false);
      setRepairSearch(loadedScenario.repair_search ?? true);
      setPrioritySemantics(
        loadedScenario.priority_semantics === "outgoing-per-instance-v3" ? "outgoing-per-instance-v3" : "outgoing-v2",
      );
      const loadedHeroFilter = loadedScenario.hero_filter;
      if (loadedHeroFilter?.mode === "shared") {
        setHeroViewMode("shared");
      } else if (loadedHeroFilter?.include_heroes?.[0]) {
        setHeroViewMode("hero");
        setSelectedHeroID(loadedHeroFilter.include_heroes[0]);
      }
      setExcludedHeroID(loadedHeroFilter?.exclude_heroes?.[0] || "");
      setHeroExcludeMode(loadedHeroFilter?.exclude_mode || "strict");
      setPriorityOrder(loadedScenario.priorities || []);
      loadedScenario.priorities
        ?.filter((key) => key.startsWith("star_source:"))
        .forEach((key) => initializedStarPriorityKeysRef.current.add(key));
      setCoverageGroups(loadedScenario.coverage_groups || []);
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : "Failed to load catalog");
    }
  }

  const heroFilter = useMemo<HeroFilter>(() => {
    const filter: HeroFilter = { unknown_policy: "exclude" };
    if (heroViewMode === "shared") {
      filter.mode = "shared";
    } else if (heroViewMode === "hero" && selectedHeroID) {
      filter.include_heroes = [selectedHeroID];
      filter.mode = "any";
    }
    if (excludedHeroID) {
      filter.exclude_heroes = [excludedHeroID];
      filter.exclude_mode = heroExcludeMode;
    }
    return filter;
  }, [heroViewMode, selectedHeroID, excludedHeroID, heroExcludeMode]);

  const availableItemIDs = useMemo(() => {
    if (!catalog) return new Set<string>();
    return new Set(catalog.items.filter((item) => heroScopeMatches(item.hero_scope, heroFilter)).map((item) => item.id));
  }, [catalog, heroFilter]);
  const playableHeroes = useMemo(() => (catalog?.heroes || []).filter((hero) => !hero.npc), [catalog]);

  const items = useMemo(() => {
    if (!catalog) {
      return [];
    }
    const normalizedQuery = deferredQuery.trim().toLowerCase();
    return [...catalog.items]
      .filter((item) => {
        if (!normalizedQuery) {
          return heroScopeMatches(item.hero_scope, heroFilter);
        }
        return (
          heroScopeMatches(item.hero_scope, heroFilter) &&
          (item.name.toLowerCase().includes(normalizedQuery) || item.id.includes(normalizedQuery))
        );
      })
      .sort((left, right) => {
        const leftSelected = (quantities[left.id] || 0) > 0;
        const rightSelected = (quantities[right.id] || 0) > 0;
        if (leftSelected !== rightSelected) {
          return leftSelected ? -1 : 1;
        }
        return left.name.localeCompare(right.name) || left.id.localeCompare(right.id);
      });
  }, [catalog, deferredQuery, quantities, heroFilter]);

  useEffect(() => {
    setQuantities((current) => {
      const next = Object.fromEntries(Object.entries(current).filter(([itemID]) => availableItemIDs.has(itemID)));
      return Object.keys(next).length === Object.keys(current).length ? current : next;
    });
  }, [availableItemIDs]);

  const selectedItemsCount = Object.values(quantities).reduce((sum, count) => sum + count, 0);
  const currentSolution = solutions[selectedSolution];
  const priorityOptions = useMemo(() => (catalog ? buildPriorityOptions(catalog, quantities) : []), [catalog, quantities]);
  const priorityOptionsByKey = useMemo(() => new Map(priorityOptions.map((option) => [option.key, option])), [priorityOptions]);
  const coverageGroupPriorityKeys = useMemo(() => coverageGroups.map((_, index) => coverageGroupPriorityKey(index)), [coverageGroups]);
  const orderedPriorityOptions = useMemo(
    () => priorityOrder.map((key) => priorityOptionsByKey.get(key)).filter(isPriorityOption),
    [priorityOrder, priorityOptionsByKey],
  );
  const craftPriorityOptions = useMemo(
    () => orderedPriorityOptions.filter((option) => option.kind === "craft"),
    [orderedPriorityOptions],
  );
  const starSourceOptions = useMemo(
    () => priorityOptions.filter((option) => option.kind === "star_source"),
    [priorityOptions],
  );
  const starSourceIDs = useMemo(() => starSourceOptions.map((option) => option.itemID), [starSourceOptions]);
  const groupedStarSourceIDs = useMemo(() => new Set(coverageGroups.flatMap((group) => group.sources)), [coverageGroups]);
  const looseStarPriorityOptions = useMemo(
    () => orderedPriorityOptions.filter((option) => option.kind === "star_source" && !groupedStarSourceIDs.has(option.itemID)),
    [orderedPriorityOptions, groupedStarSourceIDs],
  );
  const globalPriorityEntries = useMemo(
    () => buildGlobalPriorityEntries(priorityOrder, priorityOptionsByKey, coverageGroups, groupedStarSourceIDs),
    [priorityOrder, priorityOptionsByKey, coverageGroups, groupedStarSourceIDs],
  );
  const disabledPrioritySet = useMemo(() => new Set(disabledPriorityKeys), [disabledPriorityKeys]);
  const currentScenario = useMemo(
    () =>
      buildWebScenario({
        grid,
        quantities,
        top,
        maxNodes,
        noSkips,
        stopOnCoverageCeiling,
        stopOnPriorityCeiling,
        repairSearch,
        prioritySemantics,
        priorityOrder,
        priorityOptionsByKey,
        coverageGroups,
        starSourceIDs,
        disabledPrioritySet,
        heroFilter,
      }),
    [
      grid,
      quantities,
      top,
      maxNodes,
      noSkips,
      stopOnCoverageCeiling,
      stopOnPriorityCeiling,
      repairSearch,
      prioritySemantics,
      priorityOrder,
      priorityOptionsByKey,
      coverageGroups,
      starSourceIDs,
      disabledPrioritySet,
      heroFilter,
    ],
  );
  const recipesByResult = useMemo(() => {
    const map = new Map<string, Recipe[]>();
    catalog?.recipes.filter((recipe) => heroScopeMatches(recipe.hero_scope, heroFilter)).forEach((recipe) => {
      map.set(recipe.result, [...(map.get(recipe.result) || []), recipe]);
    });
    return map;
  }, [catalog, heroFilter]);
  const activeItemID = pinnedItemID || hoveredItemID;
  const activeItem = useMemo(() => {
    if (!catalog || !activeItemID) {
      return null;
    }
    return catalog.items.find((item) => item.id === activeItemID) || null;
  }, [catalog, activeItemID]);
  const activeItemMetadata = useMemo<RuntimeItemMetadata | null>(() => {
    if (!runtimeMetadata || !activeItemID) {
      return null;
    }
    return runtimeMetadata.items.find((item) => item.catalog_id === activeItemID) || null;
  }, [runtimeMetadata, activeItemID]);
  const activeRecipes = activeItem ? recipesByResult.get(activeItem.id) || [] : [];

  useEffect(() => {
    const optionKeys = [
      ...coverageGroupPriorityKeys,
      ...priorityOptions.filter((option) => option.kind === "star_source").map((option) => option.key),
      ...priorityOptions.filter((option) => option.kind === "craft").map((option) => option.key),
    ];
    setPriorityOrder((current) => reconcilePriorityOrder(current, optionKeys, coverageGroupPriorityKeys));
    setDisabledPriorityKeys((current) => {
      const next = new Set(current.filter((key) => optionKeys.includes(key)));
      priorityOptions
        .filter((option) => option.kind === "star_source")
        .forEach((option) => {
          if (!initializedStarPriorityKeysRef.current.has(option.key)) {
            initializedStarPriorityKeysRef.current.add(option.key);
            next.add(option.key);
          }
        });
      return [...next];
    });
  }, [priorityOptions, coverageGroupPriorityKeys]);

  useEffect(() => {
    setCoverageGroups((current) => {
      const next = reconcileCoverageGroups(current, starSourceIDs);
      return coverageGroupsEqual(current, next) ? current : next;
    });
  }, [starSourceIDs]);

  function showItemInfo(itemID: string) {
    if (!pinnedItemID) {
      setHoveredItemID(itemID);
    }
  }

  function hideItemInfo() {
    if (!pinnedItemID) {
      setHoveredItemID(null);
    }
  }

  function togglePinnedItem(itemID: string) {
    if (pinnedItemID === itemID) {
      setPinnedItemID(null);
      setHoveredItemID(itemID);
      return;
    }
    setPinnedItemID(itemID);
    setHoveredItemID(itemID);
  }

  async function runSolve() {
    if (!catalog) {
      return;
    }
    const abortController = new AbortController();
    void requestSolveNotificationPermission();
    setSolveAbortController(abortController);
    setIsSolving(true);
    setSolveProgress(initialSolveProgress(currentScenario));
    setError(null);
	setPrintReconstructionPreview(false);
    setSolutions([]);
    setPartialResultStatus(null);
    setSolveDurationMs(null);
    setLastSolveBackend(solverBackend);
    setRemoteSolveMetadata(null);
    setRemoteFallbackNotice(null);
    setSelectedSolution(0);
    setDebugCopyStatus(null);
    setDebugFallbackText(null);
    lastPartialSolutionsRef.current = { value: null, signature: null };
    let remoteProgressTimer: number | undefined;
    let sawPartialResult = false;
    const applyPartialSolutions = (partialSolutions: Solution[]) => {
      if (!Array.isArray(partialSolutions) || partialSolutions.length === 0) {
        return;
      }
      const previous = lastPartialSolutionsRef.current;
      if (partialSolutions === previous.value) {
        return;
      }
      const signature = JSON.stringify(partialSolutions);
      if (signature === previous.signature) {
        previous.value = partialSolutions;
        return;
      }
      lastPartialSolutionsRef.current = { value: partialSolutions, signature };
      const hadPartialResult = sawPartialResult;
      sawPartialResult = true;
      setSolutions(partialSolutions);
      setSelectedSolution(0);
      if (!hadPartialResult) {
        setPartialResultStatus("running");
      }
    };
    const handleProgress = (progress: SolveProgress) => {
      const { partial_solutions: partialSolutions, ...progressState } = progress;
      setSolveProgress(progressState);
      if (Array.isArray(partialSolutions) && partialSolutions.length > 0) {
        applyPartialSolutions(partialSolutions);
      }
    };
    try {
      await waitForPaint();
      const startedAt = performance.now();
      let nextSolutions: Solution[];
      let usedBackend: SolverBackend = solverBackend;
      if (solverBackend === "remote") {
        setSolveProgress(remoteSolveProgress(startedAt));
        remoteProgressTimer = window.setInterval(() => {
          setSolveProgress(remoteSolveProgress(startedAt));
        }, 250);
        try {
          const remoteResult = await solveWithRemote(catalog, currentScenario, abortController.signal);
          nextSolutions = remoteResult.solutions;
          setRemoteSolveMetadata(remoteResult.metadata);
        } catch (remoteError) {
          if (isAbortError(remoteError)) {
            throw remoteError;
          }
          if (!(remoteError instanceof RemoteSolveError) || !remoteError.fallbackAllowed) {
            throw remoteError;
          }
          if (remoteProgressTimer !== undefined) {
            window.clearInterval(remoteProgressTimer);
            remoteProgressTimer = undefined;
          }
          const message = remoteError.message || "Remote solver unavailable";
          setRemoteFallbackNotice(`${message}. Falling back to Local WASM.`);
          setSolveProgress(initialSolveProgress(currentScenario));
          setLastSolveBackend("local");
          nextSolutions = await solveWithWorker(catalog, currentScenario, handleProgress, abortController.signal);
          usedBackend = "local";
        }
      } else if (solverBackend === "oci") {
        setSolveProgress(ociSolveProgress(startedAt));
        const ociResult = await solveWithOci(
          catalog,
          currentScenario,
          ociEndpoint,
          ociToken,
          handleProgress,
          applyPartialSolutions,
          abortController.signal,
        );
        nextSolutions = ociResult.solutions;
        setRemoteSolveMetadata(ociResult.metadata);
      } else {
        nextSolutions = await solveWithWorker(catalog, currentScenario, handleProgress, abortController.signal);
      }
      const duration = performance.now() - startedAt;
      setSolveDurationMs(duration);
      setLastSolveBackend(usedBackend);
      setSolutions(nextSolutions);
      setPartialResultStatus(null);
      if (nextSolutions.length === 0) {
        setError("No solutions found.");
      }
      notifySolveFinished(nextSolutions.length, duration);
    } catch (solveError) {
      if (isAbortError(solveError)) {
        if (sawPartialResult) {
          setPartialResultStatus("canceled");
          setError("Solve canceled. Showing the best partial result found so far; it is not proven optimal.");
        } else {
          setError("Solve canceled.");
        }
        return;
      }
      const message = solveError instanceof Error ? solveError.message : "Solver failed";
      setError(message);
      notifySolveFailed(message);
    } finally {
      setIsSolving(false);
      if (remoteProgressTimer !== undefined) {
        window.clearInterval(remoteProgressTimer);
      }
      setSolveProgress(null);
      setSolveAbortController((current) => (current === abortController ? null : current));
    }
  }

  function cancelSolve() {
    solveAbortController?.abort();
  }

  function previewPrintReconstruction() {
    if (!catalog) {
      return;
    }
    try {
      const draft = buildPrintReconstructionSolution(catalog);
      const starKeys = buildPriorityOptions(catalog, PRINT_RECONSTRUCTION_ITEMS)
        .filter((option) => option.kind === "star_source")
        .map((option) => option.key);
      const enabled = new Set(["star_source:spirit_biscuit", "star_source:spice"]);
      starKeys.forEach((key) => initializedStarPriorityKeysRef.current.add(key));
      setGrid(defaultGrid());
      setQuantities(PRINT_RECONSTRUCTION_ITEMS);
      setTop(1);
      setNoSkips(true);
      setPrioritySemantics("outgoing-per-instance-v3");
      setPriorityOrder(["star_source:spirit_biscuit", "star_source:spice"]);
      setCoverageGroups([]);
      setDisabledPriorityKeys(starKeys.filter((key) => !enabled.has(key)));
		setManualEditorPlacements(PRINT_RECONSTRUCTION.map((placement, index) => ({ ...placement, key: `print-${index}` })));
		setManualLayoutStatus("Draft loaded. Click Evaluate manual layout to calculate its real stars.");
      setSolutions([draft]);
      setSelectedSolution(0);
      setPrintReconstructionPreview(true);
      setError(null);
    } catch (previewError) {
      setError(previewError instanceof Error ? previewError.message : "Could not build print reconstruction");
    }
  }

  async function evaluateManualLayout() {
    if (!catalog || manualLayoutEvaluating) {
      return;
    }
	try {
		if (manualEditorPlacements.length === 0) {
			throw new Error("Place at least one item before evaluating the manual layout.");
		}
		const placements = manualEditorPlacements.map((placement) => ({
			...(placement.instanceID ? { instance_id: placement.instanceID } : {}),
			item_id: placement.itemID,
			rotation: placement.rotation,
			origin: placement.origin,
		}));
		const items = placements.reduce<Record<string, number>>((counts, placement) => {
			counts[placement.item_id] = (counts[placement.item_id] || 0) + 1;
			return counts;
		}, {});
		const scenario: Scenario = {
			...currentScenario,
			grid: rowsFromGrid(grid),
			items,
			no_skips: true,
		};
		setManualLayoutEvaluating(true);
		setManualLayoutStatus("Evaluating layout locally...");
		const solution = await evaluateLayoutWithWasm(catalog, { scenario, placements });
		setQuantities(items);
		setSolutions([solution]);
		setSelectedSolution(0);
		setPrintReconstructionPreview(false);
		setManualLayoutStatus(`Evaluated: ${solution.score.stars} stars, priority ${solution.score.priority_counts?.join("/") || "none"}.`);
		setError(null);
	} catch (layoutError) {
		setManualLayoutStatus(null);
		setError(layoutError instanceof Error ? layoutError.message : "Manual layout evaluation failed");
	} finally {
		setManualLayoutEvaluating(false);
	}
  }

  async function copyDebugLog() {
    if (!catalog || !currentSolution) {
      return;
    }
    const debugLog = buildDebugLog({
      catalog,
      scenario: currentScenario,
      solution: currentSolution,
      selectedSolutionIndex: selectedSolution,
      solveDurationMs,
      solveBackend: lastSolveBackend,
      remoteSolveMetadata,
      ociEndpoint,
      partialResultStatus,
      uiPriorityState: {
        priority_order: currentScenario.priorities || [],
        disabled_priority_keys: disabledPriorityKeys,
        coverage_groups_configured: coverageGroups,
        loose_star_priorities: looseStarPriorityOptions.map((option) => ({
          key: option.key,
          item_id: option.itemID,
          title: option.title,
          enabled: !disabledPrioritySet.has(option.key),
        })),
        craft_priorities: craftPriorityOptions.map((option) => ({
          key: option.key,
          item_id: option.itemID,
          title: option.title,
          enabled: !disabledPrioritySet.has(option.key),
        })),
      },
    });
    try {
      await navigator.clipboard.writeText(debugLog);
      setDebugFallbackText(null);
      setDebugCopyStatus("Debug copied.");
      window.setTimeout(() => {
        setDebugCopyStatus((current) => (current === "Debug copied." ? null : current));
      }, 2500);
    } catch {
      setDebugFallbackText(debugLog);
      setDebugCopyStatus("Clipboard blocked. Select the debug text below.");
    }
  }

  function updateQuantity(itemID: string, delta: number) {
    setQuantities((current) => {
      const next = Math.max(0, (current[itemID] || 0) + delta);
      const updated = { ...current };
      if (next === 0) {
        delete updated[itemID];
      } else {
        updated[itemID] = next;
      }
		setManualEditorPlacements((placements) => {
			const currentPlaced = placements.filter((placement) => placement.itemID === itemID);
			if (currentPlaced.length <= next) {
				return placements;
			}
			const removeKeys = new Set(currentPlaced.slice(next).map((placement) => placement.key));
			return placements.filter((placement) => !removeKeys.has(placement.key));
		});
      return updated;
    });
  }

  function moveGlobalPriority(key: string, delta: number) {
    setPriorityOrder((current) => moveKeyByDelta(current, key, delta));
  }

  function moveCraftPriority(key: string, delta: number) {
    moveGlobalPriority(key, delta);
  }

  function moveLooseStarPriority(key: string, delta: number) {
    moveGlobalPriority(key, delta);
  }

  function togglePriorityEnabled(key: string) {
    setDisabledPriorityKeys((current) => {
      if (current.includes(key)) {
        return current.filter((currentKey) => currentKey !== key);
      }
      return [...current, key];
    });
  }

  function handlePriorityDrop(targetKey: string, event: DragEvent<HTMLElement>) {
    event.preventDefault();
    if (!draggingPriorityKey) {
      return;
    }
    const targetIndex = priorityOrder.indexOf(targetKey);
    if (targetIndex < 0) {
      setDraggingPriorityKey(null);
      return;
    }
    const bounds = event.currentTarget.getBoundingClientRect();
    const afterTarget = event.clientY > bounds.top + bounds.height / 2;
    setPriorityOrder((current) => moveKeyToIndex(current, draggingPriorityKey, targetIndex + (afterTarget ? 1 : 0)));
    setDraggingPriorityKey(null);
  }

  function addCoverageGroup() {
    setCoverageGroups((current) => [...current, { name: `Group ${current.length + 1}`, sources: [] }]);
  }

  function moveCoverageGroup(index: number, delta: number) {
    moveGlobalPriority(coverageGroupPriorityKey(index), delta);
  }

  function renameCoverageGroup(index: number, name: string) {
    setCoverageGroups((current) =>
      current.map((group, groupIndex) => (groupIndex === index ? { ...group, name } : group)),
    );
  }

  function moveStarSourceToGroup(sourceID: string, groupIndex: number) {
    setCoverageGroups((current) => moveSourceToGroup(current, sourceID, groupIndex));
	// Dropping a source into an incoming group is an explicit activation choice.
	setDisabledPriorityKeys((current) => current.filter((key) => key !== `star_source:${sourceID}`));
    setDraggingStarSourceID(null);
  }

  function removeStarSourceFromGroups(sourceID: string) {
    setCoverageGroups((current) => removeSourceFromGroups(current, sourceID));
  }

  function toggleCoverageGroupTarget(groupIndex: number, sourceID: string) {
    setCoverageGroups((current) => toggleGroupTarget(current, groupIndex, sourceID));
  }

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <h1>Backpack Brawl Solver</h1>
            <p>{catalog ? `${items.length}/${catalog.items.length} items loaded` : "Loading catalog"}</p>
        </div>
        <div className="topbar-actions">
          <label className="backend-select">
            Backend
            <select
              value={solverBackend}
              disabled={isSolving}
              onChange={(event) => setSolverBackend(event.target.value as SolverBackend)}
            >
              <option value="remote">Vercel remote</option>
              <option value="oci">OCI VM</option>
              <option value="local">Local WASM</option>
            </select>
          </label>
          {solverBackend === "oci" && (
            <div className="oci-settings">
              <label className="backend-select">
                OCI endpoint
                <input
                  value={ociEndpoint}
                  disabled={isSolving}
                  onChange={(event) => setOciEndpoint(event.target.value)}
                  placeholder="https://solver.example"
                />
              </label>
              <label className="backend-select">
                Token
                <input
                  type="password"
                  value={ociToken}
                  disabled={isSolving}
                  onChange={(event) => setOciToken(event.target.value)}
                  placeholder="X-Solver-Token"
                />
              </label>
            </div>
          )}
          <button
            className={isSolving ? "primary-action cancel-action" : "primary-action"}
            disabled={
              !isSolving &&
              (!catalog || selectedItemsCount === 0 || (solverBackend === "oci" && (ociEndpoint.trim() === "" || ociToken.trim() === "")))
            }
            onClick={isSolving ? cancelSolve : runSolve}
          >
            {isSolving ? "Cancel" : "Solve"}
          </button>
        </div>
      </header>

      <section className="workspace">
        <aside className="panel inventory-panel">
          <div className="panel-header">
            <h2>Items</h2>
            <span>{selectedItemsCount}</span>
          </div>
          <input
            className="search-input"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="Search"
          />
          <div className="hero-filter-controls">
            <label>
              Availability
              <select value={heroViewMode} onChange={(event) => setHeroViewMode(event.target.value as HeroViewMode)}>
                <option value="all">All items</option>
                <option value="hero">Specific hero</option>
                <option value="shared">Shared only</option>
              </select>
            </label>
            {heroViewMode === "hero" && (
              <label>
                Hero
                <select value={selectedHeroID} onChange={(event) => setSelectedHeroID(event.target.value)}>
                  <option value="">Choose a hero</option>
                  {playableHeroes.map((hero) => (
                    <option key={hero.id} value={hero.id}>
                      {hero.english_name || hero.name}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label>
              Exclude hero
              <select value={excludedHeroID} onChange={(event) => setExcludedHeroID(event.target.value)}>
                <option value="">None</option>
                {playableHeroes.map((hero) => (
                  <option key={hero.id} value={hero.id}>
                    {hero.english_name || hero.name}
                  </option>
                ))}
              </select>
            </label>
            {excludedHeroID && (
              <label>
                Exclusion
                <select value={heroExcludeMode} onChange={(event) => setHeroExcludeMode(event.target.value as "strict" | "exclusive_only")}>
                  <option value="strict">Strict</option>
                  <option value="exclusive_only">Exclusive items only</option>
                </select>
              </label>
            )}
          </div>
          <div className="item-list">
            {items.map((item) => (
              <ItemRow
                key={item.id}
                item={item}
                quantity={quantities[item.id] || 0}
                onDecrease={() => updateQuantity(item.id, -1)}
                onIncrease={() => updateQuantity(item.id, 1)}
                onInspect={() => showItemInfo(item.id)}
                onInspectEnd={hideItemInfo}
                onTogglePin={() => togglePinnedItem(item.id)}
                isPinned={pinnedItemID === item.id}
              />
            ))}
          </div>
        </aside>

        <section className="panel editor-panel">
          <div className="panel-header">
            <h2>Backpack</h2>
            <div className="panel-header-actions">
			  <button className="secondary-action" disabled={!catalog || isSolving} onClick={previewPrintReconstruction}>
				Preview print
			  </button>
              <button className="secondary-action" onClick={() => setGrid(defaultGrid())}>
                Full
              </button>
              <button className="secondary-action" onClick={() => setGrid(emptyGrid())}>
                Empty
              </button>
            </div>
          </div>
          <EditableGrid grid={grid} onToggle={(row, col) => setGrid(toggleGridCell(grid, row, col))} />
		  <ManualLayoutPanel
			catalog={catalog}
			visualMetadata={itemVisualMetadata}
			quantities={quantities}
			placements={manualEditorPlacements}
			onPlacementsChange={(placements) => {
			  setManualEditorPlacements(placements);
			  setManualLayoutStatus("Layout changed. Evaluate it to calculate the exact stars.");
			  setSolutions([]);
			  setPrintReconstructionPreview(false);
			}}
			onEvaluate={evaluateManualLayout}
			onCopyDetails={copyDebugLog}
			evaluating={manualLayoutEvaluating}
			canCopy={Boolean(currentSolution)}
			status={manualLayoutStatus || debugCopyStatus}
		  />
          <PriorityPanel
            craftOptions={craftPriorityOptions}
            starOptions={starSourceOptions}
            looseStarOptions={looseStarPriorityOptions}
            prioritySemantics={prioritySemantics}
            globalEntries={globalPriorityEntries}
            coverageGroups={coverageGroups}
            catalog={catalog}
            quantities={quantities}
            disabledKeys={disabledPrioritySet}
            draggingKey={draggingPriorityKey}
            draggingStarSourceID={draggingStarSourceID}
            onMoveCraft={moveCraftPriority}
            onMoveLooseStar={moveLooseStarPriority}
            onPrioritySemanticsChange={setPrioritySemantics}
            onAddGroup={addCoverageGroup}
            onMoveGroup={moveCoverageGroup}
            onRenameGroup={renameCoverageGroup}
            onRemoveFromGroups={removeStarSourceFromGroups}
            onToggleGroupTarget={toggleCoverageGroupTarget}
            onToggleEnabled={togglePriorityEnabled}
            onDragStart={setDraggingPriorityKey}
            onDragStarSourceStart={setDraggingStarSourceID}
            onDragEnd={() => setDraggingPriorityKey(null)}
            onDragStarSourceEnd={() => setDraggingStarSourceID(null)}
            onDrop={handlePriorityDrop}
            onDropStarSource={moveStarSourceToGroup}
          />
          <div className="solver-controls">
            <label>
              Top
              <input min={1} type="number" value={top} onChange={(event) => setTop(toPositiveInt(event.target.value, 1))} />
            </label>
            <label>
              Max nodes
              <input min={0} type="number" value={maxNodes} onChange={(event) => setMaxNodes(toNonNegativeInt(event.target.value))} />
            </label>
            <label className="checkbox-control">
              <input type="checkbox" checked={noSkips} onChange={(event) => setNoSkips(event.target.checked)} />
              No skips
            </label>
            <label className="checkbox-control">
              <input
                type="checkbox"
                checked={stopOnCoverageCeiling}
                onChange={(event) => setStopOnCoverageCeiling(event.target.checked)}
              />
              Stop on coverage ceiling
            </label>
			<label className="checkbox-control">
			  <input
				type="checkbox"
				checked={stopOnPriorityCeiling}
				disabled={prioritySemantics !== "outgoing-per-instance-v3"}
				onChange={(event) => setStopOnPriorityCeiling(event.target.checked)}
			  />
			  Stop on priority ceiling
			</label>
            <label className="checkbox-control">
              <input
                type="checkbox"
                checked={maxNodes > 0 && repairSearch}
                disabled={maxNodes === 0}
                onChange={(event) => setRepairSearch(event.target.checked)}
              />
              Repair search
            </label>
          </div>
        </section>

        <section className="panel results-panel">
          <div className="panel-header">
            <h2>Results</h2>
            <div className="panel-header-actions">
              <button className="secondary-action compact-action" disabled={!currentSolution} onClick={copyDebugLog}>
                Copy debug
              </button>
              <span>{solutions.length}</span>
            </div>
          </div>
          {debugCopyStatus && <div className="status-ok">{debugCopyStatus}</div>}
          {debugFallbackText && (
            <textarea
              className="debug-fallback"
              readOnly
              value={debugFallbackText}
              aria-label="Debug log fallback"
              onFocus={(event) => event.currentTarget.select()}
            />
          )}
          {isSolving && solveProgress && <SolveProgressView progress={solveProgress} />}
          {partialResultStatus && (
            <div className="status-warning">
              Partial result / not proven{partialResultStatus === "canceled" ? " / canceled" : ""}. The final exhaustive result may still improve it.
            </div>
          )}
          {remoteFallbackNotice && <div className="status-warning">{remoteFallbackNotice}</div>}
          {solveDurationMs !== null && <div className="solve-meta">Solved in {formatDurationMs(solveDurationMs)}</div>}
          {lastSolveBackend && (
            <div className="solve-meta">
              Backend: {formatSolverBackend(lastSolveBackend)}
              {formatRemoteMetadata(currentSolution?.search, remoteSolveMetadata)}
            </div>
          )}
          {currentSolution?.search?.max_nodes_capped && currentSolution.search.max_nodes_applied && (
            <div className="status-warning">
              Remote max_nodes applied: {currentSolution.search.max_nodes_applied.toLocaleString()}
            </div>
          )}
          {currentSolution?.search && (
            <div className="search-meta">
              {currentSolution.search.nodes_explored.toLocaleString()} nodes explored
              {formatNodesPerSecond(currentSolution.search.nodes_per_second, currentSolution.search.nodes_explored, solveDurationMs)}
              {currentSolution.search.refined ? " - refined" : ""}
              {currentSolution.search.limited ? " - limited search" : " - exhaustive search"}
              {formatCoveragePruning(currentSolution.search.coverage_bound_checks, currentSolution.search.coverage_pruned_nodes)}
              {formatExactBoundPruning(currentSolution.search.exact_bound_checks, currentSolution.search.exact_bound_pruned_nodes)}
              {formatCoverageSeed(currentSolution.search.coverage_seed_nodes, currentSolution.search.coverage_seed_candidates)}
              {formatRefineSearch(currentSolution.search)}
              {formatRepairSearch(currentSolution.search)}
            </div>
          )}
          {currentSolution?.search?.coverage_ceiling && currentSolution.search.coverage_ceiling.length > 0 && (
            <div className="search-meta">
              Coverage ceiling: {formatCoverageBuckets(currentSolution.search.coverage_ceiling)}
              {typeof currentSolution.search.coverage_target_count === "number"
                ? ` across ${currentSolution.search.coverage_target_count} target(s)`
                : ""}
              {currentSolution.search.coverage_seed_best ? ` - seed best ${currentSolution.search.coverage_seed_best}` : ""}
            </div>
          )}
          {currentSolution?.search?.limited && (
            <div className="status-warning">Best found with the current node limit; not guaranteed optimal.</div>
          )}
          {currentSolution?.search?.coverage_ceiling_reached && <div className="status-ok">Coverage ceiling reached.</div>}
          {currentSolution?.search?.stopped_after_coverage_ceiling && (
            <div className="status-warning">Stopped after reaching the coverage ceiling; later tie-breakers may not be fully optimized.</div>
          )}
          {error && <div className="status-error">{error}</div>}
          {solutions.length > 0 && (
            <>
              <div className="solution-tabs">
                {solutions.map((solution, index) => (
                  <button
                    key={index}
                    className={index === selectedSolution ? "active" : ""}
                    onClick={() => setSelectedSolution(index)}
                  >
                    #{index + 1} C{solution.score.crafts} S{solution.score.stars}
                  </button>
                ))}
              </div>
              <SolutionView
                catalog={catalog}
                solution={currentSolution}
                scenario={currentScenario}
                grid={grid}
                visualMetadata={itemVisualMetadata}
				draft={printReconstructionPreview}
                onInspectItem={showItemInfo}
                onInspectEnd={hideItemInfo}
                onTogglePinItem={togglePinnedItem}
                pinnedItemID={pinnedItemID}
              />
            </>
          )}
        </section>
      </section>
      {activeItem && (
        <ItemInfoCard
          item={activeItem}
          metadata={activeItemMetadata}
          recipes={activeRecipes}
          pinned={pinnedItemID === activeItem.id}
          onClose={() => {
            setPinnedItemID(null);
            setHoveredItemID(null);
          }}
        />
      )}
    </main>
  );
}

function buildWebScenario({
  grid,
  quantities,
  top,
  maxNodes,
  noSkips,
  stopOnCoverageCeiling,
  stopOnPriorityCeiling,
  repairSearch,
  prioritySemantics,
  priorityOrder,
  priorityOptionsByKey,
  coverageGroups,
  starSourceIDs,
  disabledPrioritySet,
  heroFilter,
}: WebScenarioInput): Scenario {
  const selectedItemIDs = Object.keys(cleanQuantities(quantities));
  const { groups: cleanGroups, indexByOriginal } = cleanCoverageGroupsWithIndex(
    coverageGroups,
    starSourceIDs,
    selectedItemIDs,
    disabledPrioritySet,
  );
  return {
    name: "web-scenario",
    grid: rowsFromGrid(grid),
    items: cleanQuantities(quantities),
    top,
    workers: 1,
    max_nodes: maxNodes,
    no_skips: noSkips,
    stop_on_coverage_ceiling: stopOnCoverageCeiling,
    stop_on_priority_ceiling: prioritySemantics === "outgoing-per-instance-v3" && stopOnPriorityCeiling,
    repair_search: maxNodes > 0 && repairSearch,
    priority_semantics: prioritySemantics,
    priorities: buildScenarioPriorityOrder(priorityOrder, priorityOptionsByKey, indexByOriginal, cleanGroups, disabledPrioritySet),
    coverage_groups: cleanGroups,
    hero_filter: heroFilter,
  };
}

function buildDebugLog({
  catalog,
  scenario,
  solution,
  selectedSolutionIndex,
  solveDurationMs,
  solveBackend,
  remoteSolveMetadata,
  ociEndpoint,
  partialResultStatus,
  uiPriorityState,
}: {
  catalog: Catalog;
  scenario: Scenario;
  solution: Solution;
  selectedSolutionIndex: number;
  solveDurationMs: number | null;
  solveBackend: SolverBackend | null;
  remoteSolveMetadata: RemoteSolveMetadata | null;
  ociEndpoint: string;
  partialResultStatus: "running" | "canceled" | null;
  uiPriorityState: DebugPriorityState;
}): string {
  const itemByID = new Map(catalog.items.map((item) => [item.id, item]));
  const selectedItems = selectedItemSummaries(itemByID, scenario.items, solution.placements);
  const selectedItemsNotPlaced = selectedItems.filter((entry) => entry.not_placed > 0);
  const coverageBreakdowns =
    solution.coverage_groups && solution.coverage_groups.length > 0 ? solution.coverage_groups : solution.coverage ? [solution.coverage] : [];
  const placementsWithNames = solution.placements.map((placement) => ({
    ...placement,
    item_name: itemName(itemByID, placement.item_id),
  }));
  const debugJSON = {
    generated_at: new Date().toISOString(),
    scenario,
    solve_backend: solveBackend,
    partial_result_status: partialResultStatus,
    remote_solve: remoteSolveMetadata,
    oci_endpoint: solveBackend === "oci" ? ociEndpoint : undefined,
    ui_priority_state: uiPriorityState,
    selected_solution_index: selectedSolutionIndex,
    selected_solution_label: `#${selectedSolutionIndex + 1}`,
    selected_items: selectedItems,
    selected_items_not_placed: selectedItemsNotPlaced,
    solution: {
      ...solution,
      placements: placementsWithNames,
    },
  };

  const lines: string[] = [];
  lines.push("# Backpack Brawl Solver Debug");
  lines.push(`Generated: ${debugJSON.generated_at}`);
  lines.push(`Selected solution: #${selectedSolutionIndex + 1}`);
  if (partialResultStatus) {
    lines.push(`Result status: partial / not proven${partialResultStatus === "canceled" ? " / canceled" : ""}`);
  }
  if (solveDurationMs !== null) {
    lines.push(`Solved in: ${formatDurationMs(solveDurationMs)}`);
  }
  if (solveBackend) {
    lines.push(`Backend: ${formatSolverBackend(solveBackend)}${formatRemoteMetadata(solution.search, remoteSolveMetadata)}`);
  }
  lines.push(
    `Search: ${solution.search.nodes_explored.toLocaleString()} nodes, ${solution.search.limited ? "limited" : "exhaustive"}${
      solution.search.refined ? ", refined" : ""
    }`,
  );
  if (solution.search.parallel_tasks && solution.search.parallel_tasks > 0) {
    lines.push(
      `Parallel search: ${solution.search.parallel_tasks.toLocaleString()} tasks, ${
        solution.search.parallel_workers_used || 0
      } workers`,
    );
  }
  if (solution.search.repair_nodes && solution.search.repair_nodes > 0) {
    lines.push(
      `Repair: ${solution.search.repair_nodes.toLocaleString()} nodes, ${solution.search.repair_iterations || 0} iterations, ${
        solution.search.repair_candidates || 0
      } candidates, ${solution.search.repair_improvements || 0} improvements${
        solution.search.repair_best ? `, best ${solution.search.repair_best}` : ""
      }`,
    );
    if (solution.search.repair_parallel_tasks && solution.search.repair_parallel_tasks > 0) {
      lines.push(
        `Repair parallel: ${solution.search.repair_parallel_tasks.toLocaleString()} tasks, ${
          solution.search.repair_parallel_workers_used || 0
        } workers`,
      );
    }
  }
  if (solution.search.exact_bound_checks && solution.search.exact_bound_checks > 0) {
    lines.push(
      `Exact bounds: checks=${solution.search.exact_bound_checks.toLocaleString()}, pruned=${(
        solution.search.exact_bound_pruned_nodes || 0
      ).toLocaleString()}`,
    );
  }
	if (solution.search.priority_ceiling && solution.search.priority_ceiling.length > 0) {
		lines.push(
		  `Priority ceiling: ${solution.search.priority_ceiling.join("/")}${
				solution.search.priority_ceiling_reached ? " (reached)" : ""
		  }${solution.search.stopped_after_priority_ceiling ? ", stopped" : ""}`,
		);
	}
	const phasePriorityLines: Array<[string, number[] | undefined]> = [
		["Initial", solution.search.initial_best_priority_counts],
		["Seed", solution.search.seed_best_priority_counts],
		["Search", solution.search.search_best_priority_counts],
		["Post-repair", solution.search.post_repair_best_priority_counts],
		["Refine", solution.search.refine_best_priority_counts],
	];
	phasePriorityLines.forEach(([phase, counts]) => {
		if (counts && counts.length > 0) {
			lines.push(`${phase} best priority: ${counts.join("/")}`);
		}
	});
  if (solution.search.refine_moves_checked && solution.search.refine_moves_checked > 0) {
    lines.push(
      `Refine: moves=${solution.search.refine_moves_checked.toLocaleString()}, improvements=${
        solution.search.refine_improvements || 0
      }${solution.search.refine_best_delta ? `, ${solution.search.refine_best_delta}` : ""}`,
    );
  }
  if (solution.search.completion_moves_checked && solution.search.completion_moves_checked > 0) {
    lines.push(
      `Completion: moves=${solution.search.completion_moves_checked.toLocaleString()}, improvements=${
        solution.search.completion_improvements || 0
      }`,
    );
  }
  lines.push(
    `Score: crafts=${solution.score.crafts}, stars=${solution.score.stars}, items=${solution.score.items}, targets=${
      solution.score.star_target_breadth || 0
    }, reciprocal=${solution.score.star_reciprocal_pairs || 0}, source_diversity=${
      solution.score.star_source_definition_diversity || 0
    }`,
  );
  if (solution.score.priority_counts && solution.score.priority_counts.length > 0) {
    lines.push(`Priority counts: ${solution.score.priority_counts.join("/")}`);
  }
  lines.push("");
  lines.push("## Scenario");
  lines.push(`Grid: ${safeArray(scenario.grid).join("/")}`);
  lines.push(`Top: ${scenario.top}, max_nodes: ${scenario.max_nodes}, no_skips: ${Boolean(scenario.no_skips)}`);
  lines.push(`Stop on coverage ceiling: ${Boolean(scenario.stop_on_coverage_ceiling)}`);
  lines.push(`Stop on priority ceiling: ${Boolean(scenario.stop_on_priority_ceiling)}`);
  lines.push(`Repair search: ${Boolean(scenario.repair_search)}`);
  lines.push(`Priority semantics: ${scenario.priority_semantics || "legacy-incoming-v1"}`);
  lines.push("Selected items:");
  selectedItems.forEach((entry) => lines.push(`- ${entry.item_name} (${entry.item_id}) x${entry.selected}`));
  if (selectedItemsNotPlaced.length > 0) {
    lines.push("Selected items not placed:");
    selectedItemsNotPlaced.forEach((entry) =>
      lines.push(`- ${entry.item_name} (${entry.item_id}): selected ${entry.selected}, placed ${entry.placed}, not placed ${entry.not_placed}`),
    );
  } else {
    lines.push("Selected items not placed: none");
  }
  lines.push("");
  lines.push("## Priorities");
  lines.push("Incoming coverage groups sent:");
  safeArray(scenario.coverage_groups).forEach((group, index) => {
    const targetText = safeArray(group.targets).length > 0 ? ` | targets: ${safeArray(group.targets).join(", ")}` : "";
    lines.push(`- ${group.name || `Group ${index + 1}`}: ${safeArray(group.sources).join(", ")}${targetText}`);
  });
  if (!scenario.coverage_groups || scenario.coverage_groups.length === 0) {
    lines.push("- none");
  }
  lines.push(`Priorities sent: ${safeArray(scenario.priorities).join(", ") || "none"}`);
  lines.push(`Disabled priority keys: ${uiPriorityState.disabled_priority_keys.join(", ") || "none"}`);
  lines.push("");
  lines.push("## Incoming Convergence");
  if (coverageBreakdowns.length === 0) {
    lines.push("none");
  } else {
    coverageBreakdowns.forEach((coverage, index) => {
      lines.push(`${coverage.name || `Group ${index + 1}`}: ${coverage.summary}`);
      lines.push(`Sources: ${safeArray(coverage.sources).join(", ")}`);
      if (safeArray(coverage.target_item_ids).length > 0) {
        lines.push(`Targets: ${safeArray(coverage.target_item_ids).join(", ")}`);
      }
      coverage.targets.forEach((target) => {
        const coveredSources = safeArray(target.covered_sources);
        lines.push(
          `- ${target.target_instance}: ${target.covered_count}/${safeArray(coverage.sources).length} ${
            coveredSources.length > 0 ? `(${coveredSources.join(", ")})` : "(none)"
          }`,
        );
      });
    });
  }
  lines.push("");
  const perInstanceOutgoing = scenario.priority_semantics === "outgoing-per-instance-v3";
  lines.push(perInstanceOutgoing ? "## Outgoing Targets Per Copy" : "## Outgoing Distinct Targets");
  if (!solution.loose_star_priorities || solution.loose_star_priorities.length === 0) {
    lines.push("none");
  } else {
    solution.loose_star_priorities.forEach((priority) => {
      const count = perInstanceOutgoing ? priority.link_count ?? priority.target_count : priority.target_count;
      const perCopy = perInstanceOutgoing && priority.instance_target_counts?.length
        ? ` (${priority.instance_target_counts.map((instance) => `${instance.source_instance}=${instance.target_count}`).join(", ")})`
        : "";
      lines.push(`- ${itemName(itemByID, priority.source_item_id)} (${priority.source_item_id}): ${count}${perCopy}`);
    });
  }
  lines.push("");
  lines.push("## Crafts");
  if (solution.crafts.length === 0) {
    lines.push("none");
  } else {
    solution.crafts.forEach((craft) => lines.push(`- ${craft.result}: anchor=${craft.anchor_instance}, ingredients=${craft.ingredient_instances.join(", ")}`));
  }
  lines.push("");
  lines.push("## Stars");
  if (solution.stars.length === 0) {
    lines.push("none");
  } else {
    solution.stars.forEach((star) => lines.push(`- ${star.source_instance} -> ${star.target_instance} at ${formatCoord(star.star_position)}`));
  }
  lines.push("");
  lines.push("## Placements");
  placementsWithNames.forEach((placement) =>
    lines.push(
      `- ${placement.instance_id} ${placement.item_name}: rotation=${placement.rotation}, origin=${formatCoord(placement.origin)}, cells=${formatCoordList(
        placement.cells,
      )}, star_positions=${formatCoordList(placement.star_positions)}`,
    ),
  );
  lines.push("");
  lines.push("## JSON");
  lines.push("```json");
  lines.push(JSON.stringify(debugJSON, null, 2));
  lines.push("```");
  return lines.join("\n");
}

function heroScopeMatches(scope: CatalogItem["hero_scope"], filter: HeroFilter): boolean {
  const hasFilter = filter.mode === "shared" || (filter.include_heroes?.length || 0) > 0 || (filter.exclude_heroes?.length || 0) > 0;
  if (!hasFilter) return true;
  if (!scope || scope.status !== "confirmed" || scope.kind === "unknown") {
    return filter.unknown_policy === "include";
  }
  const available = new Set(scope.available_to || []);
  if (filter.mode === "shared" && scope.kind !== "shared") return false;
  if (filter.include_heroes?.length) {
    const matches = filter.mode === "all"
      ? filter.include_heroes.every((heroID) => available.has(heroID))
      : filter.include_heroes.some((heroID) => available.has(heroID));
    if (!matches) return false;
  }
  if (filter.exclude_heroes?.length) {
    if (filter.exclude_mode === "exclusive_only") {
      const onlyExcluded = [...available].every((heroID) => filter.exclude_heroes?.includes(heroID));
      if (onlyExcluded) return false;
    } else if (filter.exclude_heroes.some((heroID) => available.has(heroID))) {
      return false;
    }
  }
  return true;
}

function ItemRow({
  item,
  quantity,
  onDecrease,
  onIncrease,
  onInspect,
  onInspectEnd,
  onTogglePin,
  isPinned,
}: {
  item: CatalogItem;
  quantity: number;
  onDecrease: () => void;
  onIncrease: () => void;
  onInspect: () => void;
  onInspectEnd: () => void;
  onTogglePin: () => void;
  isPinned: boolean;
}) {
  return (
    <div
      className={[quantity > 0 ? "item-row selected" : "item-row", isPinned ? "pinned" : ""].filter(Boolean).join(" ")}
      tabIndex={0}
      onMouseEnter={onInspect}
      onMouseLeave={onInspectEnd}
      onFocus={onInspect}
      onBlur={onInspectEnd}
      onClick={onTogglePin}
      aria-label={`Review ${item.name}`}
    >
      {item.image_path ? (
        <img src={assetPath(item.image_path)} alt={item.name} loading="lazy" decoding="async" />
      ) : (
        <span className="item-row-placeholder" aria-hidden="true">?</span>
      )}
      <div className="item-row-main">
        <strong>{item.name}</strong>
        <span>{item.types.join(", ") || "No type"}</span>
        {item.hero_scope && <small>{formatHeroScope(item.hero_scope.kind)}</small>}
      </div>
      <div className="quantity-control">
        <button
          onClick={(event) => {
            event.stopPropagation();
            onDecrease();
          }}
          disabled={quantity === 0}
        >
          -
        </button>
        <span>{quantity}</span>
        <button
          onClick={(event) => {
            event.stopPropagation();
            onIncrease();
          }}
        >
          +
        </button>
      </div>
    </div>
  );
}

function formatHeroScope(kind: NonNullable<CatalogItem["hero_scope"]>["kind"]): string {
  if (kind === "shared") return "Shared";
  if (kind === "hero_specific") return "Hero-specific";
  if (kind === "multi_hero") return "Multi-hero";
  return "Unknown scope";
}

function EditableGrid({ grid, onToggle }: { grid: Grid; onToggle: (row: number, col: number) => void }) {
  return (
    <div className="editable-grid" style={{ gridTemplateColumns: `repeat(${COLS}, 1fr)` }}>
      {grid.map((row, rowIndex) =>
        row.map((available, colIndex) => (
          <button
            key={`${rowIndex}-${colIndex}`}
            className={available ? "grid-cell available" : "grid-cell blocked"}
            onClick={() => onToggle(rowIndex, colIndex)}
            aria-label={`cell ${rowIndex + 1}, ${colIndex + 1}`}
          >
            {available ? "1" : "0"}
          </button>
        )),
      )}
    </div>
  );
}

function ManualLayoutPanel({
	catalog,
	visualMetadata,
	quantities,
	placements,
	onPlacementsChange,
	onEvaluate,
	onCopyDetails,
	evaluating,
	canCopy,
	status,
}: {
	catalog: Catalog | null;
	visualMetadata: ItemVisualMetadataMap;
	quantities: Record<string, number>;
	placements: ManualEditorPlacement[];
	onPlacementsChange: (placements: ManualEditorPlacement[]) => void;
	onEvaluate: () => void;
	onCopyDetails: () => void;
	evaluating: boolean;
	canCopy: boolean;
	status: string | null;
}) {
  const [selectedItemID, setSelectedItemID] = useState("");
  const [selectedPlacementKey, setSelectedPlacementKey] = useState<string | null>(null);
  const [rotation, setRotation] = useState(0);
	const [placementHint, setPlacementHint] = useState("");
	const itemByID = useMemo(() => new Map(catalog?.items.map((item) => [item.id, item]) || []), [catalog]);
	const selectedItems = useMemo(
		() => [...itemByID.values()].filter((item) => (quantities[item.id] || 0) > 0).sort((left, right) => left.name.localeCompare(right.name)),
		[itemByID, quantities],
	);
	const displayedPlacements = useMemo(() => {
		if (!catalog) return [];
		return placements.map((placement) => manualPlacementDisplay(catalog, placement));
	}, [catalog, placements]);
	const selectedPlacement = selectedPlacementKey ? placements.find((placement) => placement.key === selectedPlacementKey) : undefined;

	useEffect(() => {
		if (selectedPlacementKey && !placements.some((placement) => placement.key === selectedPlacementKey)) {
			setSelectedPlacementKey(null);
		}
	}, [placements, selectedPlacementKey]);

	const occupied = useMemo(() => {
		const byCell = new Map<string, ManualEditorPlacement>();
		displayedPlacements.forEach((placement, index) => {
			placement.cells.forEach((cell) => byCell.set(coordKey(cell), placements[index]));
		});
		return byCell;
	}, [displayedPlacements, placements]);

	const applyRotation = (nextRotation: number) => {
		if (!selectedPlacement) {
			setRotation(nextRotation);
			setPlacementHint(`Next item will use R${nextRotation}.`);
			return;
		}
		const candidate = { ...selectedPlacement, rotation: nextRotation };
		if (!manualPlacementFits(catalog, candidate, placements.filter((placement) => placement.key !== candidate.key))) {
			setPlacementHint(`R${nextRotation} does not fit for the selected item.`);
			return;
		}
		onPlacementsChange(placements.map((placement) => (placement.key === candidate.key ? candidate : placement)));
		setRotation(nextRotation);
		setPlacementHint(`Selected item rotated to R${nextRotation}.`);
	};

	const updateSelectedRotation = (delta: number) => {
		applyRotation(normalizeRotation(rotation + delta));
	};

	const removeSelected = () => {
		if (!selectedPlacementKey) return;
		onPlacementsChange(placements.filter((placement) => placement.key !== selectedPlacementKey));
		setSelectedPlacementKey(null);
		setPlacementHint("Selected item removed.");
	};

	const clearSelection = () => {
		setSelectedPlacementKey(null);
		setSelectedItemID("");
		setPlacementHint("");
	};

	const moveSelectedBy = (rowDelta: number, colDelta: number, direction: string) => {
		if (!selectedPlacement) return;
		const origin: CoordTuple = [selectedPlacement.origin[0] + rowDelta, selectedPlacement.origin[1] + colDelta];
		const candidate = { ...selectedPlacement, origin };
		if (manualPlacementFits(catalog, candidate, placements.filter((placement) => placement.key !== candidate.key))) {
			onPlacementsChange(placements.map((placement) => (placement.key === candidate.key ? candidate : placement)));
			setPlacementHint(`Moved selected item ${direction} to r${origin[0]} c${origin[1]}.`);
			return;
		}
		setPlacementHint(`Cannot move ${selectedPlacement.itemID} ${direction}: its occupied cells would overlap another item or leave the grid.`);
	};

	const selectPlacement = (key: string) => {
		const placement = placements.find((entry) => entry.key === key);
		if (!placement) return;
		setSelectedPlacementKey(key);
		setSelectedItemID("");
		setRotation(placement.rotation);
		setPlacementHint(`Selected ${placement.itemID} at R${placement.rotation}.`);
	};

	const handleCell = (origin: CoordTuple) => {
		if (!catalog) return;
		if (selectedPlacement) {
			const candidate = { ...selectedPlacement, origin };
			if (manualPlacementFits(catalog, candidate, placements.filter((placement) => placement.key !== candidate.key))) {
				onPlacementsChange(placements.map((placement) => (placement.key === candidate.key ? candidate : placement)));
				setPlacementHint(`Moved selected item to r${origin[0]} c${origin[1]}.`);
			} else {
				setPlacementHint(`The selected item does not fit at r${origin[0]} c${origin[1]} with R${rotation}.`);
			}
			return;
		}
		if (!selectedItemID) return;
		const available = (quantities[selectedItemID] || 0) - placements.filter((placement) => placement.itemID === selectedItemID).length;
		if (available <= 0) return;
		const candidate: ManualEditorPlacement = {
			key: `${selectedItemID}-${crypto.randomUUID()}`,
			instanceID: "",
			itemID: selectedItemID,
			rotation,
			origin,
		};
		if (manualPlacementFits(catalog, candidate, placements)) {
			onPlacementsChange([...placements, candidate]);
			setSelectedPlacementKey(candidate.key);
			setSelectedItemID("");
			setPlacementHint(`Placed ${candidate.itemID} at r${origin[0]} c${origin[1]} with R${rotation}.`);
			return;
		}
		const suggestedRotation = [0, 90, 180, 270].find((candidateRotation) =>
			manualPlacementFits(catalog, { ...candidate, rotation: candidateRotation }, placements),
		);
		if (suggestedRotation !== undefined) {
			setRotation(suggestedRotation);
			setPlacementHint(`R${rotation} does not fit at r${origin[0]} c${origin[1]}. Switched to R${suggestedRotation}; click the same origin again to place it.`);
		} else {
			setPlacementHint(`No rotation fits ${candidate.itemID} at r${origin[0]} c${origin[1]}.`);
		}
	};

	return (
	  <details className="manual-layout-panel">
		<summary>Manual layout comparison</summary>
		<p>
		  Select an item, choose a rotation, then click a free cell to place it. Click a placed item to select it; rotate, move, or remove it. Evaluate uses the local WASM evaluator and shows exact stars below.
		</p>
		<div className="manual-layout-tools">
		  <label>
			Item
			<select
			  aria-label="Manual layout item"
			  value={selectedItemID}
			  onChange={(event) => {
				setSelectedItemID(event.target.value);
				setSelectedPlacementKey(null);
				setPlacementHint(event.target.value ? `Choose a rotation, then click its origin cell.` : "");
			  }}
			>
			  <option value="">Choose an available item</option>
			  {selectedItems.map((item) => {
				const remaining = (quantities[item.id] || 0) - placements.filter((placement) => placement.itemID === item.id).length;
				return <option key={item.id} value={item.id} disabled={remaining <= 0}>{item.name} ({Math.max(0, remaining)} left)</option>;
			  })}
			</select>
		  </label>
		  <label>
			Rotation
			<select aria-label="Manual layout rotation" value={rotation} onChange={(event) => applyRotation(Number(event.target.value))}>
			  <option value={0}>R0</option>
			  <option value={90}>R90</option>
			  <option value={180}>R180</option>
			  <option value={270}>R270</option>
			</select>
		  </label>
		  <span className="manual-layout-selection">{selectedPlacement ? `Selected ${selectedPlacement.itemID}` : selectedItemID ? `Placing ${selectedItemID}` : "Select an item or placed tile"}</span>
		</div>
		<div className="manual-layout-actions">
		  <button className="secondary-action" onClick={() => updateSelectedRotation(-90)} disabled={!selectedPlacement && !selectedItemID}>
			Rotate left
		  </button>
		  <button className="secondary-action" onClick={() => updateSelectedRotation(90)} disabled={!selectedPlacement && !selectedItemID}>
			Rotate right
		  </button>
		  <button className="secondary-action" onClick={removeSelected} disabled={!selectedPlacement}>
			Remove selected
		  </button>
		  <button className="secondary-action" onClick={() => moveSelectedBy(0, -1, "left")} disabled={!selectedPlacement}>
			Move left
		  </button>
		  <button className="secondary-action" onClick={() => moveSelectedBy(0, 1, "right")} disabled={!selectedPlacement}>
			Move right
		  </button>
		  <button className="secondary-action" onClick={() => moveSelectedBy(-1, 0, "up")} disabled={!selectedPlacement}>
			Move up
		  </button>
		  <button className="secondary-action" onClick={() => moveSelectedBy(1, 0, "down")} disabled={!selectedPlacement}>
			Move down
		  </button>
		  <button className="secondary-action" onClick={clearSelection} disabled={!selectedPlacement && !selectedItemID}>
			Clear selection
		  </button>
		</div>
		<div
		  className={selectedItemID || selectedPlacement ? "manual-layout-grid placing" : "manual-layout-grid"}
		  style={{ gridTemplateColumns: `repeat(${COLS}, minmax(0, 1fr))`, gridTemplateRows: `repeat(${ROWS}, minmax(0, 1fr))` }}
		>
		  {Array.from({ length: ROWS }, (_, row) =>
			Array.from({ length: COLS }, (_, col) => {
			  const owner = occupied.get(coordKey([row, col]));
			  return (
				<button
				  key={`${row}-${col}`}
				  className={owner ? "manual-layout-cell occupied" : "manual-layout-cell"}
				  style={{ gridRow: `${row + 1}`, gridColumn: `${col + 1}` }}
				  onClick={() => handleCell([row, col])}
				  aria-label={`Manual layout cell r${row} c${col}${owner ? ` occupied by ${owner.itemID}` : ""}`}
				/>
			  );
			}),
		  )}
		  {displayedPlacements.map((placement, index) => (
			<PlacedItem
			  key={placements[index].key}
			  placement={placement}
			  item={itemByID.get(placement.item_id)}
			  itemColor={ITEM_BORDER_COLORS[index % ITEM_BORDER_COLORS.length]}
			  visualMetadata={visualMetadata}
			  label={labelFor(index)}
			  onInspect={() => undefined}
			  onInspectEnd={() => undefined}
			  onTogglePin={() => selectPlacement(placements[index].key)}
			  isPinned={selectedPlacementKey === placements[index].key}
			/>
		  ))}
		</div>
		<div className="manual-layout-actions">
		  <button className="secondary-action" onClick={onEvaluate} disabled={evaluating || placements.length === 0}>
			{evaluating ? "Evaluating..." : "Evaluate manual layout"}
		  </button>
		  <button className="secondary-action" onClick={onCopyDetails} disabled={!canCopy || evaluating}>
			Copy evaluated details
		  </button>
		</div>
		{status && <p className="manual-layout-status">{status}</p>}
		{placementHint && <p className="manual-layout-hint">{placementHint}</p>}
	  </details>
	);
}

function PriorityPanel({
  craftOptions,
  starOptions,
  looseStarOptions,
  prioritySemantics,
  globalEntries,
  coverageGroups,
  catalog,
  quantities,
  disabledKeys,
  draggingKey,
  draggingStarSourceID,
  onMoveCraft,
  onMoveLooseStar,
  onPrioritySemanticsChange,
  onAddGroup,
  onMoveGroup,
  onRenameGroup,
  onRemoveFromGroups,
  onToggleGroupTarget,
  onToggleEnabled,
  onDragStart,
  onDragStarSourceStart,
  onDragEnd,
  onDragStarSourceEnd,
  onDrop,
  onDropStarSource,
}: {
  craftOptions: PriorityOption[];
  starOptions: PriorityOption[];
  looseStarOptions: PriorityOption[];
  prioritySemantics: OutgoingPrioritySemantics;
  globalEntries: GlobalPriorityEntry[];
  coverageGroups: CoverageGroup[];
  catalog: Catalog | null;
  quantities: Record<string, number>;
  disabledKeys: Set<string>;
  draggingKey: string | null;
  draggingStarSourceID: string | null;
  onMoveCraft: (key: string, delta: number) => void;
  onMoveLooseStar: (key: string, delta: number) => void;
  onPrioritySemanticsChange: (semantics: OutgoingPrioritySemantics) => void;
  onAddGroup: () => void;
  onMoveGroup: (index: number, delta: number) => void;
  onRenameGroup: (index: number, name: string) => void;
  onRemoveFromGroups: (sourceID: string) => void;
  onToggleGroupTarget: (groupIndex: number, sourceID: string) => void;
  onToggleEnabled: (key: string) => void;
  onDragStart: (key: string) => void;
  onDragStarSourceStart: (sourceID: string) => void;
  onDragEnd: () => void;
  onDragStarSourceEnd: () => void;
  onDrop: (targetKey: string, event: DragEvent<HTMLElement>) => void;
  onDropStarSource: (sourceID: string, groupIndex: number) => void;
}) {
  const starOptionsByID = new Map(starOptions.map((option) => [option.itemID, option]));
  const availableStarIDs = new Set(starOptions.map((option) => option.itemID));
  const enabledCraftCount = craftOptions.filter((option) => !disabledKeys.has(option.key)).length;
  const enabledGroupedStarCount = coverageGroups.reduce(
    (sum, group) => sum + group.sources.filter((sourceID) => availableStarIDs.has(sourceID) && !disabledKeys.has(`star_source:${sourceID}`)).length,
    0,
  );
  const enabledLooseStarCount = looseStarOptions.filter((option) => !disabledKeys.has(option.key)).length;
  const totalCount = craftOptions.length + starOptions.length;
  const enabledCount = enabledCraftCount + enabledGroupedStarCount + enabledLooseStarCount;
  return (
    <section className="priority-panel">
      <div className="priority-header">
        <h3>Priorities</h3>
        <span>
          {enabledCount}/{totalCount}
        </span>
      </div>
      <p className="muted priority-semantics-hint">
        By default, star sources are disabled so the solver maximizes total valid links. Coverage groups prioritize incoming convergence.
      </p>
      <label className="priority-semantics-select">
        Outgoing source objective
        <select value={prioritySemantics} onChange={(event) => onPrioritySemanticsChange(event.target.value as OutgoingPrioritySemantics)}>
          <option value="outgoing-v2">Distinct targets between copies</option>
          <option value="outgoing-per-instance-v3">Targets per copy</option>
        </select>
      </label>
      {totalCount === 0 ? (
        <p className="muted priority-empty">none</p>
      ) : (
        <div className="priority-list">
          {globalEntries.length > 0 && <div className="priority-subheader">Priority order</div>}
          {globalEntries.map((entry, index) => {
            if (entry.kind === "coverage_group") {
              const group = entry.group;
              const groupIndex = entry.groupIndex;
            const groupSources = group.sources.map((sourceID) => starOptionsByID.get(sourceID)).filter(isPriorityOption);
            const activeGroup = {
              ...group,
              sources: group.sources.filter((sourceID) => !disabledKeys.has(`star_source:${sourceID}`)),
            };
            const targetCount = catalog ? coverageGroupTargetCount(activeGroup, catalog, quantities) : 0;
            const targetOptions = catalog ? coverageGroupTargetOptions(activeGroup, catalog, quantities) : [];
            return (
              <section
                key={entry.key}
                className="coverage-group"
                draggable
                onDragStart={(event) => {
                  event.dataTransfer.effectAllowed = "move";
                  onDragStart(entry.key);
                }}
                onDragOver={(event) => {
                  event.preventDefault();
                  event.dataTransfer.dropEffect = "move";
                }}
                onDrop={(event) => {
                  event.preventDefault();
                  if (draggingStarSourceID) {
                    onDropStarSource(draggingStarSourceID, groupIndex);
                  } else {
                    onDrop(entry.key, event);
                  }
                }}
                onDragEnd={onDragEnd}
              >
                <div className="coverage-group-header">
                  <div>
                    <input
                      className="coverage-group-name"
                      value={group.name}
                      onChange={(event) => onRenameGroup(groupIndex, event.target.value)}
                      aria-label={`Coverage group ${groupIndex + 1} name`}
                    />
                    <span>
                      Incoming convergence: {activeGroup.sources.length} active source(s), {targetCount} target item(s)
                    </span>
                  </div>
                  <div className="priority-actions">
                    <button onClick={() => onMoveGroup(groupIndex, -1)} disabled={index === 0} aria-label={`Move ${group.name} up`}>
                      Up
                    </button>
                    <button
                      onClick={() => onMoveGroup(groupIndex, 1)}
                      disabled={index === globalEntries.length - 1}
                      aria-label={`Move ${group.name} down`}
                    >
                      Down
                    </button>
                  </div>
                </div>
                {groupSources.length === 0 ? (
                  <p className="muted coverage-group-empty">drop star sources here for incoming convergence</p>
                ) : (
                  groupSources.map((option) => {
                    const disabled = disabledKeys.has(option.key);
                    const orbitTarget = safeArray(group.targets).includes(option.itemID);
                    return (
                      <article
                        key={option.key}
                        className={[
                          "priority-row star-row",
                          "group-star-row",
                          draggingStarSourceID === option.itemID ? "dragging" : "",
                          disabled ? "disabled" : "",
                        ]
                          .filter(Boolean)
                          .join(" ")}
                        draggable
                        onDragStart={(event) => {
                          event.dataTransfer.effectAllowed = "move";
                          onDragStarSourceStart(option.itemID);
                        }}
                        onDragEnd={onDragStarSourceEnd}
                      >
                        <label className="priority-toggle">
                          <input
                            type="checkbox"
                            checked={!disabled}
                            onChange={() => onToggleEnabled(option.key)}
                            aria-label={`${disabled ? "Enable" : "Disable"} ${option.title}`}
                          />
                        </label>
                        {option.imagePath && <img src={assetPath(option.imagePath)} alt="" loading="lazy" decoding="async" />}
                        <div className="priority-main">
                          <strong>{option.title}</strong>
                          <span>{option.subtitle}</span>
                        </div>
                        <span className="priority-kind star">Incoming</span>
                        <label className="orbit-toggle">
                          <input
                            type="checkbox"
                            checked={orbitTarget}
                            onChange={() => onToggleGroupTarget(groupIndex, option.itemID)}
                            aria-label={`${orbitTarget ? "Remove" : "Set"} ${option.title} as an incoming convergence target`}
                          />
                          Target
                        </label>
                        <button className="priority-mini-action" onClick={() => onRemoveFromGroups(option.itemID)}>
                          Outgoing
                        </button>
                      </article>
                    );
                  })
                )}
                {groupSources.length > 0 && activeGroup.sources.length === 0 && (
                  <p className="muted coverage-group-empty">enable at least one source to choose incoming targets</p>
                )}
                {targetOptions.length > 0 && (
                  <div className="orbit-target-panel">
                    <div className="orbit-target-title">Incoming convergence targets</div>
                    <div className="orbit-target-options">
                      {targetOptions.map((item) => {
                        const checked = safeArray(group.targets).includes(item.id);
                        return (
                          <label key={item.id} className={checked ? "orbit-target-chip selected" : "orbit-target-chip"}>
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => onToggleGroupTarget(groupIndex, item.id)}
                              aria-label={`${checked ? "Remove" : "Set"} ${item.name} as an incoming convergence target`}
                            />
                            {item.image_path && <img src={assetPath(item.image_path)} alt="" loading="lazy" decoding="async" />}
                            <span>{item.name}</span>
                          </label>
                        );
                      })}
                    </div>
                    <p className="orbit-target-hint">none selected = all eligible incoming targets</p>
                  </div>
                )}
              </section>
            );
            }
            const option = entry.option;
            const disabled = disabledKeys.has(option.key);
            if (option.kind === "star_source") {
              return (
                <article
                  key={option.key}
                  className={["priority-row star-row", draggingStarSourceID === option.itemID ? "dragging" : "", disabled ? "disabled" : ""]
                    .filter(Boolean)
                    .join(" ")}
                  draggable
                  onDragStart={(event) => {
                    event.dataTransfer.effectAllowed = "move";
                    onDragStart(option.key);
                    onDragStarSourceStart(option.itemID);
                  }}
                  onDragOver={(event) => {
                    event.preventDefault();
                    event.dataTransfer.dropEffect = "move";
                  }}
                  onDrop={(event) => onDrop(option.key, event)}
                  onDragEnd={() => {
                    onDragEnd();
                    onDragStarSourceEnd();
                  }}
                >
                  <label className="priority-toggle">
                    <input
                      type="checkbox"
                      checked={!disabled}
                      onChange={() => onToggleEnabled(option.key)}
                      aria-label={`${disabled ? "Enable" : "Disable"} ${option.title}`}
                    />
                  </label>
                  {option.imagePath && <img src={assetPath(option.imagePath)} alt="" loading="lazy" decoding="async" />}
                  <div className="priority-main">
                    <strong>{option.title}</strong>
                    <span>{option.subtitle}</span>
                  </div>
                  <span className="priority-kind star">Outgoing</span>
                  <div className="priority-actions">
                    <button onClick={() => onMoveLooseStar(option.key, -1)} disabled={index === 0} aria-label={`Move ${option.title} up`}>
                      Up
                    </button>
                    <button
                      onClick={() => onMoveLooseStar(option.key, 1)}
                      disabled={index === globalEntries.length - 1}
                      aria-label={`Move ${option.title} down`}
                    >
                      Down
                    </button>
                  </div>
                </article>
              );
            }
            return (
              <article
                key={option.key}
                className={["priority-row", draggingKey === option.key ? "dragging" : "", disabled ? "disabled" : ""].filter(Boolean).join(" ")}
                draggable
                onDragStart={(event) => {
                  event.dataTransfer.effectAllowed = "move";
                  onDragStart(option.key);
                }}
                onDragOver={(event) => {
                  event.preventDefault();
                  event.dataTransfer.dropEffect = "move";
                }}
                onDrop={(event) => onDrop(option.key, event)}
                onDragEnd={onDragEnd}
              >
                <label className="priority-toggle">
                  <input
                    type="checkbox"
                    checked={!disabled}
                    onChange={() => onToggleEnabled(option.key)}
                    aria-label={`${disabled ? "Enable" : "Disable"} ${option.title}`}
                  />
                </label>
                {option.imagePath && <img src={assetPath(option.imagePath)} alt="" loading="lazy" decoding="async" />}
                <div className="priority-main">
                  <strong>{option.title}</strong>
                  <span>{option.subtitle}</span>
                </div>
                <span className="priority-kind craft">Craft</span>
                <div className="priority-actions">
                  <button onClick={() => onMoveCraft(option.key, -1)} disabled={index === 0} aria-label={`Move ${option.title} up`}>
                    Up
                  </button>
                  <button
                    onClick={() => onMoveCraft(option.key, 1)}
                    disabled={index === globalEntries.length - 1}
                    aria-label={`Move ${option.title} down`}
                  >
                    Down
                  </button>
                </div>
              </article>
            );
          })}
          {starOptions.length > 0 && (
            <button className="coverage-add" onClick={onAddGroup}>
              Add incoming group
            </button>
          )}
        </div>
      )}
    </section>
  );
}

function SolveProgressView({ progress }: { progress: SolveProgress }) {
  const hasPercent = typeof progress.percent === "number";
  const percent = hasPercent ? Math.max(0, Math.min(100, progress.percent || 0)) : 0;
  const phaseText = solvePhaseText(progress.phase);
  const etaText = typeof progress.eta_ms === "number" && progress.eta_ms > 0 ? formatDurationMs(progress.eta_ms) : null;
  const nodesPerSecond =
    typeof progress.nodes_per_second === "number" && progress.nodes_per_second > 0
      ? `${Math.round(progress.nodes_per_second).toLocaleString()} nodes/sec`
      : null;

  return (
    <div className="solve-progress" role="status" aria-live="polite">
      <div className="solve-progress-header">
        <strong>{phaseText}</strong>
        <span>{hasPercent ? `${Math.floor(percent)}%` : "running"}</span>
      </div>
      <div
        className={hasPercent ? "progress-bar" : "progress-bar indeterminate"}
        aria-label="Solve progress"
        aria-valuemin={hasPercent ? 0 : undefined}
        aria-valuemax={hasPercent ? 100 : undefined}
        aria-valuenow={hasPercent ? Math.floor(percent) : undefined}
      >
        <div className="progress-bar-fill" style={hasPercent ? { width: `${percent}%` } : undefined} />
      </div>
      <div className="solve-progress-meta">
        <span>{progress.nodes_explored.toLocaleString()} nodes</span>
        {nodesPerSecond && <span>{nodesPerSecond}</span>}
        <span>elapsed {formatDurationMs(progress.elapsed_ms)}</span>
        <span>{etaText ? `ETA ${etaText}` : "ETA unavailable"}</span>
      </div>
    </div>
  );
}

function SolutionView({
  catalog,
  solution,
  scenario,
  grid,
  visualMetadata,
	draft,
  onInspectItem,
  onInspectEnd,
  onTogglePinItem,
  pinnedItemID,
}: {
  catalog: Catalog | null;
  solution: Solution;
  scenario: Scenario;
  grid: Grid;
  visualMetadata: ItemVisualMetadataMap;
	draft: boolean;
  onInspectItem: (itemID: string) => void;
  onInspectEnd: () => void;
  onTogglePinItem: (itemID: string) => void;
  pinnedItemID: string | null;
}) {
  const itemsByID = useMemo(() => {
    const map = new Map<string, CatalogItem>();
    catalog?.items.forEach((item) => map.set(item.id, item));
    return map;
  }, [catalog]);
  const coverageBreakdowns =
    solution.coverage_groups && solution.coverage_groups.length > 0
      ? solution.coverage_groups
      : solution.coverage
        ? [solution.coverage]
        : [];
  const looseStarPriorities = safeArray(solution.loose_star_priorities);
  const perInstanceOutgoing = scenario.priority_semantics === "outgoing-per-instance-v3";
  const itemColors = useMemo(() => buildItemColorMap(solution.placements), [solution.placements]);
  const selectedItemsNotPlaced = useMemo(
    () => selectedItemSummaries(itemsByID, scenario.items, solution.placements).filter((entry) => entry.not_placed > 0),
    [itemsByID, scenario.items, solution.placements],
  );
  const [hoveredPlacementID, setHoveredPlacementID] = useState<string | null>(null);
  const [pinnedPlacementID, setPinnedPlacementID] = useState<string | null>(null);
  const activePlacementID = pinnedPlacementID || hoveredPlacementID;
  const activePlacement = activePlacementID ? solution.placements.find((placement) => placement.instance_id === activePlacementID) : undefined;
  const activeStarPositions = useMemo(() => {
    const positions = new Set<string>();
    if (!activePlacementID) {
      return positions;
    }
    solution.stars.forEach((star) => {
      if (star.source_instance === activePlacementID) {
        positions.add(coordKey(star.star_position));
      }
    });
    return positions;
  }, [activePlacementID, solution.stars]);

  useEffect(() => {
    if (!pinnedItemID) {
      setPinnedPlacementID(null);
    }
  }, [pinnedItemID]);

  useEffect(() => {
    setHoveredPlacementID(null);
    setPinnedPlacementID(null);
  }, [solution]);

  return (
    <div className="solution-detail">
	  {draft && (
		<div className="status-warning draft-preview-notice">
		  <strong>Draft print reconstruction.</strong> Validate the item cells and rotation badges against the screenshot. Stars and score are intentionally not evaluated yet.
		</div>
	  )}
      <div className="score-strip">
        <span>Crafts {solution.score.crafts}</span>
        <span>Stars {solution.score.stars}</span>
        <span>Items {solution.score.items}</span>
        {typeof solution.score.star_target_breadth === "number" && <span>Targets {solution.score.star_target_breadth}</span>}
        {typeof solution.score.star_reciprocal_pairs === "number" && <span>Reciprocal {solution.score.star_reciprocal_pairs}</span>}
        {solution.score.priority_counts && solution.score.priority_counts.length > 0 && (
          <span>Priority {solution.score.priority_counts.join("/")}</span>
        )}
        {coverageBreakdowns.length > 0 && (
          <span>Incoming convergence {coverageBreakdowns.map((coverage) => coverage.summary).join(" | ")}</span>
        )}
        {looseStarPriorities.length > 0 && (
          <span>
            {perInstanceOutgoing ? "Outgoing links per copy" : "Outgoing targets"}{" "}
            {looseStarPriorities
              .map((priority) => `${priority.source_item_id}=${perInstanceOutgoing ? priority.link_count ?? priority.target_count : priority.target_count}`)
              .join(" | ")}
          </span>
        )}
      </div>
      {selectedItemsNotPlaced.length > 0 && (
        <div className="status-warning">Not placed: {formatNotPlacedItems(selectedItemsNotPlaced)}</div>
      )}
      <div
        className="layout-grid"
        style={{
          gridTemplateColumns: `repeat(${COLS}, minmax(0, 1fr))`,
          gridTemplateRows: `repeat(${ROWS}, minmax(0, 1fr))`,
        }}
      >
        {grid.map((row, rowIndex) =>
          row.map((available, colIndex) => (
            <div
              key={`${rowIndex}-${colIndex}`}
              className={available ? "layout-cell" : "layout-cell blocked"}
              style={{
                gridRow: `${rowIndex + 1}`,
                gridColumn: `${colIndex + 1}`,
              }}
            />
          )),
        )}
        {solution.placements.map((placement, index) => (
          <PlacedItem
            key={placement.instance_id}
            placement={placement}
            item={itemsByID.get(placement.item_id)}
            itemColor={itemColors.get(placement.item_id) || DEFAULT_ITEM_BORDER_COLOR}
            visualMetadata={visualMetadata}
            label={labelFor(index)}
            onInspect={() => {
              setHoveredPlacementID(placement.instance_id);
              onInspectItem(placement.item_id);
            }}
            onInspectEnd={() => {
              setHoveredPlacementID(null);
              onInspectEnd();
            }}
            onTogglePin={() => {
              setPinnedPlacementID((current) => (current === placement.instance_id ? null : placement.instance_id));
              onTogglePinItem(placement.item_id);
            }}
            isPinned={
              pinnedPlacementID === placement.instance_id ||
              hoveredPlacementID === placement.instance_id ||
              (!pinnedPlacementID && pinnedItemID === placement.item_id)
            }
          />
        ))}
        {activePlacement &&
          safeArray(activePlacement.star_positions)
            .filter(inBoundsCoord)
            .map((position, index) => {
              const key = coordKey(position);
              const active = activeStarPositions.has(key);
              return (
                <div
                  key={`${activePlacement.instance_id}-star-${key}-${index}`}
                  className={active ? "result-star-marker active" : "result-star-marker"}
                  style={{
                    gridRow: `${position[0] + 1}`,
                    gridColumn: `${position[1] + 1}`,
                  }}
                  aria-hidden="true"
                >
                  *
                </div>
              );
            })}
      </div>
      <section className="activation-list">
        <h3>Crafts</h3>
        {solution.crafts.length === 0 ? (
          <p>none</p>
        ) : (
          solution.crafts.map((craft) => (
            <p key={`${craft.result}-${craft.anchor_instance}`}>
              {craft.result}: {craft.ingredient_instances.join(", ")}
            </p>
          ))
        )}
      </section>
      <section className="activation-list">
        <h3>Stars</h3>
        {solution.stars.length === 0 ? (
          <p>none</p>
        ) : (
          solution.stars.map((star) => (
            <p key={`${star.source_instance}-${star.target_instance}-${star.star_position.join("-")}`}>
              {star.source_instance} {"->"} {star.target_instance}
            </p>
          ))
        )}
      </section>
      {coverageBreakdowns.length > 0 && (
        <section className="activation-list">
          <h3>Incoming convergence</h3>
          {coverageBreakdowns.map((coverage, coverageIndex) => (
            <div className="coverage-result-group" key={`${coverage.name || "coverage"}-${coverageIndex}`}>
              <p>
                <strong>{coverage.name || `Group ${coverageIndex + 1}`}:</strong> {coverage.summary}
              </p>
              <p>Sources: {safeArray(coverage.sources).join(", ")}</p>
              {safeArray(coverage.target_item_ids).length > 0 && <p>Targets: {safeArray(coverage.target_item_ids).join(", ")}</p>}
              {coverage.targets.map((target) => (
                <CoverageTargetLine
                  key={`${coverage.name || coverageIndex}-${target.target_instance}`}
                  target={target}
                  sourceCount={safeArray(coverage.sources).length}
                />
              ))}
            </div>
          ))}
        </section>
      )}
      {looseStarPriorities.length > 0 && (
        <section className="activation-list">
          <h3>{perInstanceOutgoing ? "Outgoing targets per copy" : "Outgoing distinct targets"}</h3>
          {looseStarPriorities.map((priority) => {
            const item = itemsByID.get(priority.source_item_id);
            return (
              <p key={priority.source_item_id}>
                {item?.name || priority.source_item_id}: {perInstanceOutgoing ? priority.link_count ?? priority.target_count : priority.target_count}
                {perInstanceOutgoing && priority.instance_target_counts?.length
                  ? ` (${priority.instance_target_counts.map((instance) => `${instance.source_instance}=${instance.target_count}`).join(", ")})`
                  : ""}
              </p>
            );
          })}
        </section>
      )}
	  {draft && (
		<section className="activation-list draft-placement-list">
		  <h3>Draft coordinates</h3>
		  {solution.placements.map((placement) => (
			<p key={placement.instance_id}>
			  {placement.instance_id}: r{placement.origin[0]} c{placement.origin[1]}, R{placement.rotation}
			</p>
		  ))}
		</section>
	  )}
    </div>
  );
}

function CoverageTargetLine({
  target,
  sourceCount,
}: {
  target: NonNullable<Solution["coverage"]>["targets"][number];
  sourceCount: number;
}) {
  const coveredSources = safeArray(target.covered_sources);
  return (
    <p>
      {target.target_instance}: {target.covered_count}/{sourceCount}
      {coveredSources.length > 0 ? ` (${coveredSources.join(", ")})` : " (none)"}
    </p>
  );
}

function PlacedItem({
  placement,
  item,
  itemColor,
  visualMetadata,
  label,
  onInspect,
  onInspectEnd,
  onTogglePin,
  isPinned,
}: {
  placement: Placement;
  item?: CatalogItem;
  itemColor: string;
  visualMetadata: ItemVisualMetadataMap;
  label: string;
  onInspect: () => void;
  onInspectEnd: () => void;
  onTogglePin: () => void;
  isPinned: boolean;
}) {
  const rows = placement.cells.map(([row]) => row);
  const cols = placement.cells.map(([, col]) => col);
  const minRow = Math.min(...rows);
  const maxRow = Math.max(...rows);
  const minCol = Math.min(...cols);
  const maxCol = Math.max(...cols);
  const rowSpan = maxRow - minRow + 1;
  const colSpan = maxCol - minCol + 1;
  const occupiedCells = placement.cells.map(([row, col]) => [row - minRow, col - minCol] as CoordTuple);
  const firstCell = occupiedCells.reduce((first, cell) => (cell[0] < first[0] || (cell[0] === first[0] && cell[1] < first[1]) ? cell : first), occupiedCells[0]);
  const itemVisualMetadata = item ? visualMetadata[item.id] : undefined;
  const visualRotation = item ? visualRotationFor(placement.rotation, itemVisualMetadata?.base_rotation) : normalizeRotation(placement.rotation);
  const rotatedImage = visualRotation === 90 || visualRotation === 270;
  const imageMaxWidth = rotatedImage ? `${(82 * rowSpan) / colSpan}%` : "82%";
  const imageMaxHeight = rotatedImage ? `${(82 * colSpan) / rowSpan}%` : "82%";
  const pivot = itemVisualMetadata?.pivot || [0.5, 0.5];
  const offset = itemVisualMetadata?.offset || [0, 0];
  const scale = itemVisualMetadata?.scale || 1;
  const scaleX = (itemVisualMetadata?.mirror_x ? -1 : 1) * scale;
  const scaleY = (itemVisualMetadata?.mirror_y ? -1 : 1) * scale;

  return (
    <>
      <div
        className={isPinned ? "placed-item pinned" : "placed-item"}
        style={{
          gridRow: `${minRow + 1} / span ${rowSpan}`,
          gridColumn: `${minCol + 1} / span ${colSpan}`,
          gridTemplateRows: `repeat(${rowSpan}, minmax(0, 1fr))`,
          gridTemplateColumns: `repeat(${colSpan}, minmax(0, 1fr))`,
        }}
        title={placement.instance_id}
        aria-label={`Review ${placement.instance_id}`}
        data-item-id={placement.item_id}
        data-rotation={placement.rotation}
        data-visual-rotation={visualRotation}
      >
        {item?.image_path && (
          <div
            className="placed-item-image"
            style={{
              gridRow: `1 / span ${rowSpan}`,
              gridColumn: `1 / span ${colSpan}`,
            }}
          >
            <img
              src={assetPath(item.image_path)}
              alt={item.name}
              loading="lazy"
              decoding="async"
              style={{
                maxWidth: imageMaxWidth,
                maxHeight: imageMaxHeight,
                transform: `translate(${offset[1]}%, ${offset[0]}%) rotate(${visualRotation}deg) scale(${scaleX}, ${scaleY})`,
                transformOrigin: `${pivot[1] * 100}% ${pivot[0] * 100}%`,
              }}
            />
          </div>
        )}
        <span
          className="placed-label"
          style={{
            gridRow: `${firstCell[0] + 1}`,
            gridColumn: `${firstCell[1] + 1}`,
          }}
        >
          {label}
        </span>
        <span className="rotation-badge">R{placement.rotation}</span>
      </div>
      {occupiedCells.map(([row, col]) => {
        const isFirstCell = row === firstCell[0] && col === firstCell[1];
        return (
        <div
          key={`${row}-${col}`}
          className={isPinned ? "placed-cell active" : "placed-cell"}
          style={{
            gridRow: `${minRow + row + 1}`,
            gridColumn: `${minCol + col + 1}`,
            "--item-color": itemColor,
          } as CSSProperties}
          data-placement-id={placement.instance_id}
          data-item-id={placement.item_id}
          title={placement.instance_id}
          tabIndex={isFirstCell ? 0 : -1}
          onMouseEnter={onInspect}
          onMouseLeave={onInspectEnd}
          onFocus={onInspect}
          onBlur={onInspectEnd}
          onPointerDown={(event) => {
            event.stopPropagation();
            event.preventDefault();
            onTogglePin();
          }}
          onClick={(event) => {
            event.stopPropagation();
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter" || event.key === " ") {
              event.stopPropagation();
              event.preventDefault();
              onTogglePin();
            }
          }}
          aria-label={isFirstCell ? `Review ${placement.instance_id}` : undefined}
        />
        );
      })}
    </>
  );
}

function ItemInfoCard({
  item,
  metadata,
  recipes,
  pinned,
  onClose,
}: {
  item: CatalogItem;
  metadata: RuntimeItemMetadata | null;
  recipes: Recipe[];
  pinned: boolean;
  onClose: () => void;
}) {
  return (
    <aside className={pinned ? "item-info-card pinned" : "item-info-card hover-preview"} aria-live="polite">
      <div className="item-info-header">
        {item.image_path ? (
          <img src={assetPath(item.image_path)} alt={item.name} loading="lazy" decoding="async" />
        ) : (
          <div className="muted">no image</div>
        )}
        <div>
          <h2>{item.name}</h2>
          <p>{item.id}</p>
          <div className="item-info-tags">
            {item.types.length === 0 ? <span>no type</span> : item.types.map((type) => <span key={type}>{type}</span>)}
            <span>{item.needs_review ? "needs review" : "reviewed"}</span>
            {pinned && <span>pinned</span>}
          </div>
        </div>
        <button className="item-info-close" onClick={onClose} aria-label="Close item review">
          x
        </button>
      </div>

      <section>
        <h3>Shape</h3>
        <ItemShapeGrid item={item} />
      </section>

      {safeArray(item.counts_as).length > 0 && (
        <section>
          <h3>Counts as</h3>
          <p className="ability-text">{safeArray(item.counts_as).map(countsAsText).join(", ")}</p>
        </section>
      )}

      <section>
        <h3>Stars</h3>
        {item.stars.length === 0 ? (
          <p className="muted">none</p>
        ) : (
          <ul className="logic-list">
            {item.stars.map((star, index) => (
              <li key={`${star.offset.join("-")}-${index}`}>
                <strong>Star {coordText(star.offset)}:</strong> {starTargetText(star)}
                {star.effect_text && <span>{star.effect_text}</span>}
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h3>Recipe</h3>
        {recipes.length === 0 ? (
          <p className="muted">none</p>
        ) : (
          <ul className="logic-list">
            {recipes.map((recipe, index) => (
              <li key={`${recipe.result}-${index}`}>
                <strong>{recipe.result}</strong> = {recipe.ingredients.join(" + ")} <span>anchor: {recipe.anchor}</span>
              </li>
            ))}
          </ul>
        )}
      </section>

      <section>
        <h3>Ability</h3>
        <p className="ability-text">{item.ability_text || "none"}</p>
      </section>

      {metadata && (
        <section>
          <h3>Runtime data</h3>
          <p className="ability-text">
            Rarity {metadata.rarity ?? "unknown"}, layer {metadata.layer ?? "unknown"}, max level {metadata.levels.max_level ?? "unknown"}
          </p>
          {metadata.stats.length > 0 && (
            <ul className="logic-list">
              {metadata.stats.map((stat) => (
                <li key={stat.type}>
                  <strong>{stat.type}:</strong> {stat.value}
                </li>
              ))}
            </ul>
          )}
          <p className="muted">
            {metadata.levels.effects.length} level effects captured
            {metadata.levels.effects.some((effect) => !effect.stat_target) ? ", some targets unresolved" : ""}
          </p>
        </section>
      )}

      {item.source_url ? (
        <a className="source-link" href={item.source_url} target="_blank" rel="noreferrer">
          Source
        </a>
      ) : (
        <p className="muted">Source: client runtime capture</p>
      )}
    </aside>
  );
}

function ItemShapeGrid({ item }: { item: CatalogItem }) {
  const bounds = reviewBounds(item);
  const rows = bounds.maxRow - bounds.minRow + 1;
  const cols = bounds.maxCol - bounds.minCol + 1;
  const itemCells = new Set(item.shape.map(coordKey));
  const starCells = new Set(item.stars.map((star) => coordKey(star.offset)));

  return (
    <div
      className="item-shape-grid"
      style={{
        gridTemplateColumns: `repeat(${cols}, 24px)`,
        gridTemplateRows: `repeat(${rows}, 24px)`,
      }}
    >
      {Array.from({ length: rows }).map((_, rowIndex) =>
        Array.from({ length: cols }).map((__, colIndex) => {
          const coord: [number, number] = [bounds.minRow + rowIndex, bounds.minCol + colIndex];
          const key = coordKey(coord);
          const isItem = itemCells.has(key);
          const isStar = starCells.has(key);
          return (
            <span key={key} className={isItem ? "shape-cell item" : isStar ? "shape-cell star" : "shape-cell empty"}>
              {isItem ? "I" : isStar ? "*" : "."}
            </span>
          );
        }),
      )}
    </div>
  );
}

function buildPriorityOptions(catalog: Catalog, quantities: Record<string, number>): PriorityOption[] {
  const selectedItemIDs = Object.keys(cleanQuantities(quantities)).sort();
  const itemByID = new Map(catalog.items.map((item) => [item.id, item]));
  const options: PriorityOption[] = [];

  catalog.recipes
    .filter((recipe) => recipeCanUseInventory(recipe, quantities))
    .sort((left, right) => {
      const leftName = itemByID.get(left.result)?.name || left.result;
      const rightName = itemByID.get(right.result)?.name || right.result;
      return leftName.localeCompare(rightName) || left.result.localeCompare(right.result);
    })
    .forEach((recipe) => {
      const resultItem = itemByID.get(recipe.result);
      options.push({
        key: `craft:${recipe.result}`,
        kind: "craft",
        itemID: recipe.result,
        title: `Craft: ${resultItem?.name || recipe.result}`,
        subtitle: `${recipe.ingredients.join(" + ")} - anchor: ${recipe.anchor}`,
        imagePath: resultItem?.image_path || "",
      });
    });

  selectedItemIDs
    .map((itemID) => itemByID.get(itemID))
    .filter(isCatalogItem)
    .filter((item) => item.stars.length > 0)
    .filter((item) => starSourceCanTargetInventory(item, quantities))
    .sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id))
    .forEach((item) => {
      options.push({
        key: `star_source:${item.id}`,
        kind: "star_source",
        itemID: item.id,
        title: `Stars: ${item.name}`,
        subtitle: starOptionSummary(item),
        imagePath: item.image_path,
      });
    });

  return options;
}

function recipeCanUseInventory(recipe: Recipe, quantities: Record<string, number>): boolean {
  const required: Record<string, number> = {};
  recipe.ingredients.forEach((ingredient) => {
    required[ingredient] = (required[ingredient] || 0) + 1;
  });
  return Object.entries(required).every(([itemID, count]) => (quantities[itemID] || 0) >= count);
}

function starSourceCanTargetInventory(source: CatalogItem, quantities: Record<string, number>): boolean {
  return Object.entries(quantities).some(([itemID, count]) => (itemID !== source.id || count > 1) && count > 0);
}

function itemMatchesStarFilter(source: CatalogItem, target: CatalogItem, star: Star): boolean {
  if (star.exclude_source_item && source.id === target.id) {
    return false;
  }
  if (star.target_types.length === 0 && star.target_items.length === 0) {
    return true;
  }
  if (star.target_items.includes(target.id)) {
    return true;
  }
  if (safeArray(target.counts_as).some((alias) => star.target_items.includes(alias.item_id))) {
    return true;
  }
  return star.target_types.some((targetType) => target.types.includes(targetType));
}

function starOptionSummary(item: CatalogItem): string {
  const targetSummaries = Array.from(new Set(item.stars.map(starTargetText))).sort();
  const effectSummaries = Array.from(new Set(item.stars.map((star) => star.effect_text).filter(Boolean))).sort();
  const target = targetSummaries.length > 0 ? targetSummaries.join(" OR ") : "any item";
  const effect = effectSummaries.length > 0 ? ` - ${effectSummaries[0]}` : "";
  return `${item.stars.length} stars - ${target}${effect}`;
}

function coverageGroupPriorityKey(index: number): string {
  return `coverage_group:${index}`;
}

function parseCoverageGroupPriorityKey(key: string): number | null {
  if (!key.startsWith("coverage_group:")) {
    return null;
  }
  const parsed = Number.parseInt(key.slice("coverage_group:".length), 10);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null;
}

function reconcilePriorityOrder(current: string[], available: string[], coverageGroupKeys: string[]): string[] {
  const availableSet = new Set(available);
  const hasCoverageGroupOrder = current.some((key) => key.startsWith("coverage_group:"));
  const base = !hasCoverageGroupOrder && coverageGroupKeys.length > 0 ? [...coverageGroupKeys, ...current] : current;
  const next = base.filter((key) => availableSet.has(key));
  const nextSet = new Set(next);
  available.forEach((key) => {
    if (!nextSet.has(key)) {
      next.push(key);
    }
  });
  return next;
}

function buildGlobalPriorityEntries(
  priorityOrder: string[],
  priorityOptionsByKey: Map<string, PriorityOption>,
  coverageGroups: CoverageGroup[],
  groupedStarSourceIDs: Set<string>,
): GlobalPriorityEntry[] {
  return priorityOrder
    .map((key): GlobalPriorityEntry | null => {
      const groupIndex = parseCoverageGroupPriorityKey(key);
      if (groupIndex !== null) {
        const group = coverageGroups[groupIndex];
        return group ? { key, kind: "coverage_group", groupIndex, group } : null;
      }
      const option = priorityOptionsByKey.get(key);
      if (!option) {
        return null;
      }
      if (option.kind === "star_source" && groupedStarSourceIDs.has(option.itemID)) {
        return null;
      }
      return { key, kind: option.kind, option };
    })
    .filter(isGlobalPriorityEntry);
}

function reconcileCoverageGroups(current: CoverageGroup[], availableSourceIDs: string[]): CoverageGroup[] {
  const available = new Set(availableSourceIDs);
  return current.map((group, index) => {
    const sources = uniqueStrings(group.sources.filter((sourceID) => available.has(sourceID)));
    return {
      name: group.name?.trim() || `Group ${index + 1}`,
      sources,
      targets: uniqueStrings(safeArray(group.targets)),
    };
  });
}

function coverageGroupsEqual(left: CoverageGroup[], right: CoverageGroup[]): boolean {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((leftGroup, index) => {
    const rightGroup = right[index];
    return (
      leftGroup.name === rightGroup.name &&
      stringArraysEqual(leftGroup.sources, rightGroup.sources) &&
      stringArraysEqual(safeArray(leftGroup.targets), safeArray(rightGroup.targets))
    );
  });
}

function cleanCoverageGroupsWithIndex(
  groups: CoverageGroup[],
  availableSourceIDs: string[],
  selectedItemIDs: string[],
  disabledKeys: Set<string>,
): { groups: CoverageGroup[]; indexByOriginal: Map<number, number> } {
  const available = new Set(availableSourceIDs);
  const selected = new Set(selectedItemIDs);
  const indexByOriginal = new Map<number, number>();
  const cleaned: CoverageGroup[] = [];
  groups.forEach((group, index) => {
    const configuredSources = uniqueStrings(group.sources.filter((sourceID) => available.has(sourceID)));
    const next = {
      name: group.name?.trim() || `Group ${index + 1}`,
      sources: configuredSources.filter((sourceID) => !disabledKeys.has(`star_source:${sourceID}`)),
      targets: uniqueStrings(safeArray(group.targets).filter((targetID) => selected.has(targetID))),
    };
    if (next.sources.length > 0) {
      indexByOriginal.set(index, cleaned.length);
      cleaned.push(next);
    }
  });
  return { groups: cleaned, indexByOriginal };
}

function buildScenarioPriorityOrder(
  priorityOrder: string[],
  priorityOptionsByKey: Map<string, PriorityOption>,
  groupIndexByOriginal: Map<number, number>,
  cleanGroups: CoverageGroup[],
  disabledKeys: Set<string>,
): string[] {
  const groupedSources = new Set(cleanGroups.flatMap((group) => group.sources));
  const priorities: string[] = [];
  const emittedGroups = new Set<number>();
  priorityOrder.forEach((key) => {
    const groupIndex = parseCoverageGroupPriorityKey(key);
    if (groupIndex !== null) {
      const cleanIndex = groupIndexByOriginal.get(groupIndex);
      if (typeof cleanIndex === "number") {
        priorities.push(coverageGroupPriorityKey(cleanIndex));
        emittedGroups.add(cleanIndex);
      }
      return;
    }
    const option = priorityOptionsByKey.get(key);
    if (!option || disabledKeys.has(key)) {
      return;
    }
    if (option.kind === "star_source" && groupedSources.has(option.itemID)) {
      return;
    }
    priorities.push(key);
  });
  cleanGroups.forEach((_, index) => {
    if (!emittedGroups.has(index)) {
      priorities.push(coverageGroupPriorityKey(index));
    }
  });
  return priorities;
}

function moveSourceToGroup(groups: CoverageGroup[], sourceID: string, groupIndex: number): CoverageGroup[] {
  if (groupIndex < 0 || groupIndex >= groups.length) {
    return groups;
  }
  const next = groups.map((group) => ({
    ...group,
    sources: group.sources.filter((currentSourceID) => currentSourceID !== sourceID),
    targets: safeArray(group.targets).filter((currentTargetID) => currentTargetID !== sourceID),
  }));
  next[groupIndex] = {
    ...next[groupIndex],
    sources: [...next[groupIndex].sources, sourceID],
  };
  return next;
}

function removeSourceFromGroups(groups: CoverageGroup[], sourceID: string): CoverageGroup[] {
  return groups.map((group) => ({
    ...group,
    sources: group.sources.filter((currentSourceID) => currentSourceID !== sourceID),
    targets: safeArray(group.targets).filter((currentTargetID) => currentTargetID !== sourceID),
  }));
}

function toggleGroupTarget(groups: CoverageGroup[], groupIndex: number, sourceID: string): CoverageGroup[] {
  return groups.map((group, index) => {
    if (index !== groupIndex) {
      return group;
    }
    const targets = safeArray(group.targets);
    return {
      ...group,
      targets: targets.includes(sourceID) ? targets.filter((targetID) => targetID !== sourceID) : [...targets, sourceID],
    };
  });
}

function coverageGroupTargetCount(group: CoverageGroup, catalog: Catalog, quantities: Record<string, number>): number {
  const explicitTargets = uniqueStrings(safeArray(group.targets).filter((targetID) => (quantities[targetID] || 0) > 0));
  if (explicitTargets.length > 0) {
    return explicitTargets.length;
  }
  const itemByID = new Map(catalog.items.map((item) => [item.id, item]));
  const targetIDs = new Set<string>();
  group.sources.forEach((sourceID) => {
    const source = itemByID.get(sourceID);
    if (!source) {
      return;
    }
    catalog.items.forEach((target) => {
      const selectedCount = quantities[target.id] || 0;
      const availableTargets = target.id === source.id ? selectedCount - 1 : selectedCount;
      if (availableTargets <= 0) {
        return;
      }
      if (source.stars.some((star) => itemMatchesStarFilter(source, target, star))) {
        targetIDs.add(target.id);
      }
    });
  });
  return targetIDs.size;
}

function coverageGroupTargetOptions(group: CoverageGroup, catalog: Catalog, quantities: Record<string, number>): CatalogItem[] {
  const itemByID = new Map(catalog.items.map((item) => [item.id, item]));
  const sources = group.sources.map((sourceID) => itemByID.get(sourceID)).filter(isCatalogItem);
  if (sources.length === 0) {
    return [];
  }
  return catalog.items
    .filter((target) => (quantities[target.id] || 0) > 0)
    .filter((target) =>
      sources.some((source) => {
        const selectedCount = quantities[target.id] || 0;
        const availableTargets = target.id === source.id ? selectedCount - 1 : selectedCount;
        if (availableTargets <= 0) {
          return false;
        }
        return source.stars.some((star) => itemMatchesStarFilter(source, target, star));
      }),
    )
    .sort((left, right) => left.name.localeCompare(right.name) || left.id.localeCompare(right.id));
}

function moveKeyByDelta(current: string[], key: string, delta: number): string[] {
  const fromIndex = current.indexOf(key);
  if (fromIndex < 0) {
    return current;
  }
  const toIndex = Math.max(0, Math.min(current.length - 1, fromIndex + delta));
  if (toIndex === fromIndex) {
    return current;
  }
  const next = [...current];
  [next[fromIndex], next[toIndex]] = [next[toIndex], next[fromIndex]];
  return next;
}

function moveKeyToIndex(current: string[], key: string, targetIndex: number): string[] {
  const fromIndex = current.indexOf(key);
  if (fromIndex < 0) {
    return current;
  }
  const next = current.filter((currentKey) => currentKey !== key);
  let insertionIndex = targetIndex;
  if (fromIndex < targetIndex) {
    insertionIndex--;
  }
  insertionIndex = Math.max(0, Math.min(next.length, insertionIndex));
  next.splice(insertionIndex, 0, key);
  return next;
}

function isPriorityOption(option: PriorityOption | undefined): option is PriorityOption {
  return Boolean(option);
}

function isGlobalPriorityEntry(entry: GlobalPriorityEntry | null): entry is GlobalPriorityEntry {
  return Boolean(entry);
}

function isCatalogItem(item: CatalogItem | undefined): item is CatalogItem {
  return Boolean(item);
}

function safeArray<T>(values: T[] | null | undefined): T[] {
  return Array.isArray(values) ? values : [];
}

function itemName(itemByID: Map<string, CatalogItem>, itemID: string): string {
  return itemByID.get(itemID)?.name || itemID;
}

function formatCoord(coord: [number, number]): string {
  return `(${coord[0]}, ${coord[1]})`;
}

function formatCoordList(coords: [number, number][]): string {
  return `[${safeArray(coords).map(formatCoord).join(", ")}]`;
}

function gridFromRows(rows: string[]): Grid {
  return Array.from({ length: ROWS }, (_, row) =>
    Array.from({ length: COLS }, (_, col) => rows[row]?.[col] !== "0"),
  );
}

function rowsFromGrid(grid: Grid): string[] {
  return grid.map((row) => row.map((available) => (available ? "1" : "0")).join(""));
}

function toggleGridCell(grid: Grid, row: number, col: number): Grid {
  return grid.map((currentRow, rowIndex) =>
    currentRow.map((available, colIndex) => (rowIndex === row && colIndex === col ? !available : available)),
  );
}

function cleanQuantities(quantities: Record<string, number>): Record<string, number> {
  return Object.fromEntries(Object.entries(quantities).filter(([, count]) => count > 0));
}

function selectedItemSummaries(
  itemByID: Map<string, CatalogItem>,
  selectedCounts: Record<string, number>,
  placements: Placement[],
): SelectedItemPlacementSummary[] {
  const placedCounts = placements.reduce<Record<string, number>>((counts, placement) => {
    counts[placement.item_id] = (counts[placement.item_id] || 0) + 1;
    return counts;
  }, {});
  return Object.entries(selectedCounts)
    .filter(([, count]) => count > 0)
    .map(([itemID, selected]) => {
      const placed = placedCounts[itemID] || 0;
      return {
        item_id: itemID,
        item_name: itemName(itemByID, itemID),
        selected,
        placed,
        not_placed: Math.max(0, selected - placed),
      };
    })
    .sort((left, right) => left.item_name.localeCompare(right.item_name) || left.item_id.localeCompare(right.item_id));
}

function formatNotPlacedItems(items: SelectedItemPlacementSummary[]): string {
  return items.map((entry) => `${entry.item_name} x${entry.not_placed}`).join(", ");
}

function uniqueStrings(values: string[]): string[] {
  const result: string[] = [];
  values.forEach((value) => {
    if (value && !result.includes(value)) {
      result.push(value);
    }
  });
  return result;
}

function stringArraysEqual(left: string[], right: string[]): boolean {
  if (left.length !== right.length) {
    return false;
  }
  return left.every((value, index) => value === right[index]);
}

function assetPath(path: string): string {
  return `/${path.replace(/\\/g, "/")}`;
}

const ITEM_BORDER_COLORS = [
  "#9f3d45",
  "#176b87",
  "#7a4c9c",
  "#8a6116",
  "#2c7a54",
  "#b45f06",
  "#3f5c9a",
  "#8d4774",
  "#4b5f2a",
  "#9b4f2f",
  "#246b67",
  "#6b4b3e",
];
const DEFAULT_ITEM_BORDER_COLOR = ITEM_BORDER_COLORS[0];

function buildItemColorMap(placements: Placement[]): Map<string, string> {
  const itemIDs = [...new Set(placements.map((placement) => placement.item_id))].sort();
  return new Map(itemIDs.map((itemID, index) => [itemID, ITEM_BORDER_COLORS[index % ITEM_BORDER_COLORS.length]]));
}

function buildPrintReconstructionSolution(catalog: Catalog): Solution {
  const itemByID = new Map(catalog.items.map((item) => [item.id, item]));
  const occupied = new Set<string>();
  const instanceIDs = new Set<string>();
  const placements = PRINT_RECONSTRUCTION.map((spec) => {
    if (instanceIDs.has(spec.instanceID)) {
      throw new Error(`Draft repeats ${spec.instanceID}`);
    }
    instanceIDs.add(spec.instanceID);
    const item = itemByID.get(spec.itemID);
    if (!item) {
      throw new Error(`Draft references unknown item ${spec.itemID}`);
    }
    const variant = draftVariant(item, spec.rotation);
    const cells = variant.cells.map(([row, col]) => [spec.origin[0] + row, spec.origin[1] + col] as CoordTuple);
    cells.forEach((cell) => {
      if (!inBoundsCoord(cell)) {
        throw new Error(`Draft places ${spec.instanceID} out of bounds at r${cell[0]} c${cell[1]}`);
      }
      const key = coordKey(cell);
      if (occupied.has(key)) {
        throw new Error(`Draft overlaps at r${cell[0]} c${cell[1]}`);
      }
      occupied.add(key);
    });
    return {
      instance_id: spec.instanceID,
      item_id: spec.itemID,
      rotation: spec.rotation,
      origin: spec.origin,
      cells,
      star_positions: variant.stars.map(([row, col]) => [spec.origin[0] + row, spec.origin[1] + col] as CoordTuple),
    };
  });
  if (occupied.size !== ROWS * COLS) {
    throw new Error(`Draft covers ${occupied.size}/${ROWS * COLS} cells; it is not a complete reconstruction`);
  }
  return {
    layout_key: "draft-print-reconstruction",
    score: { crafts: 0, stars: 0, items: placements.length },
    search: { nodes_explored: 0, limited: false, refined: false },
    placements,
    crafts: [],
    stars: [],
  };
}

function manualPlacementDisplay(catalog: Catalog, placement: ManualEditorPlacement): Placement {
	const item = catalog.items.find((entry) => entry.id === placement.itemID);
	if (!item) {
		throw new Error(`Manual layout references unknown item ${placement.itemID}`);
	}
	const variant = draftVariant(item, placement.rotation);
	return {
		instance_id: placement.instanceID || placement.key,
		item_id: placement.itemID,
		rotation: placement.rotation,
		origin: placement.origin,
		cells: variant.cells.map(([row, col]) => [placement.origin[0] + row, placement.origin[1] + col] as CoordTuple),
		star_positions: variant.stars.map(([row, col]) => [placement.origin[0] + row, placement.origin[1] + col] as CoordTuple),
	};
}

function manualPlacementFits(catalog: Catalog | null, candidate: ManualEditorPlacement, others: ManualEditorPlacement[]): boolean {
	if (!catalog) {
		return false;
	}
	try {
		const occupied = new Set<string>();
		for (const placement of others) {
			for (const cell of manualPlacementDisplay(catalog, placement).cells) {
				occupied.add(coordKey(cell));
			}
		}
		for (const cell of manualPlacementDisplay(catalog, candidate).cells) {
			if (!inBoundsCoord(cell) || occupied.has(coordKey(cell))) {
				return false;
			}
		}
		return true;
	} catch {
		return false;
	}
}

function draftVariant(item: CatalogItem, rotation: number): { cells: CoordTuple[]; stars: CoordTuple[] } {
  const normalizedRotation = normalizeRotation(rotation);
  const shape = item.shape.map((coord) => rotateDraftCoord(coord, normalizedRotation));
  const minRow = Math.min(...shape.map(([row]) => row));
  const minCol = Math.min(...shape.map(([, col]) => col));
  const normalize = ([row, col]: CoordTuple): CoordTuple => [row - minRow, col - minCol];
  return {
    cells: shape.map(normalize).sort(compareCoords),
    stars: item.stars.map((star) => normalize(rotateDraftCoord(star.offset, normalizedRotation))),
  };
}

function rotateDraftCoord([row, col]: CoordTuple, rotation: number): CoordTuple {
  if (rotation === 90) return [col, -row];
  if (rotation === 180) return [-row, -col];
  if (rotation === 270) return [-col, row];
  return [row, col];
}

function compareCoords(left: CoordTuple, right: CoordTuple): number {
  return left[0] - right[0] || left[1] - right[1];
}

function normalizeRotation(rotation: number | undefined): number {
  const normalized = ((Math.round(rotation || 0) % 360) + 360) % 360;
  return normalized === 90 || normalized === 180 || normalized === 270 ? normalized : 0;
}

function visualRotationFor(placementRotation: number, baseRotation?: number): number {
  return normalizeRotation(normalizeRotation(placementRotation) - normalizeRotation(baseRotation));
}

function toPositiveInt(value: string, fallback: number): number {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed < 1) {
    return fallback;
  }
  return parsed;
}

function toNonNegativeInt(value: string): number {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed < 0) {
    return 0;
  }
  return parsed;
}

function isAbortError(error: unknown): boolean {
  return typeof error === "object" && error !== null && "name" in error && (error as { name?: string }).name === "AbortError";
}

function initialSolveProgress(scenario: Scenario): SolveProgress {
  const nodesTotal = scenario.max_nodes && scenario.max_nodes > 0 ? scenario.max_nodes : undefined;
  return {
    phase: "loading",
    nodes_explored: 0,
    nodes_total: nodesTotal,
    percent: nodesTotal ? 0 : undefined,
    elapsed_ms: 0,
  };
}

function remoteSolveProgress(startedAt: number): SolveProgress {
  return {
    phase: "remote",
    nodes_explored: 0,
    elapsed_ms: performance.now() - startedAt,
  };
}

function ociSolveProgress(startedAt: number): SolveProgress {
  return {
    phase: "remote",
    nodes_explored: 0,
    elapsed_ms: performance.now() - startedAt,
  };
}

function requestSolveNotificationPermission(): void {
  if (!("Notification" in window) || Notification.permission !== "default") {
    return;
  }
  void Notification.requestPermission().catch(() => undefined);
}

function notifySolveFinished(solutionCount: number, durationMs: number): void {
  showBrowserNotification("Backpack solve finished", `${solutionCount} solution(s) found in ${formatDurationMs(durationMs)}.`);
}

function notifySolveFailed(message: string): void {
  showBrowserNotification("Backpack solve failed", message.length > 140 ? `${message.slice(0, 137)}...` : message);
}

function showBrowserNotification(title: string, body: string): void {
  if (!("Notification" in window) || Notification.permission !== "granted") {
    return;
  }
  try {
    new Notification(title, { body });
  } catch {
    // Browser notification support depends on browser policy and context.
  }
}

function labelFor(index: number): string {
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
  return alphabet[index] || "?";
}

function formatDurationMs(durationMs: number): string {
  if (durationMs < 1000) {
    return `${Math.max(0, Math.round(durationMs))} ms`;
  }
  return `${(durationMs / 1000).toFixed(2)} s`;
}

function formatSolverBackend(backend: SolverBackend): string {
  if (backend === "remote") {
    return "Vercel remote";
  }
  if (backend === "oci") {
    return "OCI VM";
  }
  return "Local WASM";
}

function formatRemoteMetadata(search: Solution["search"] | undefined, metadata: RemoteSolveMetadata | null): string {
  const serverElapsed = search?.server_elapsed_ms ?? metadata?.server_elapsed_ms;
  const workers = search?.remote_workers ?? metadata?.workers;
  const maxNodesApplied = search?.max_nodes_applied ?? metadata?.max_nodes_applied;
  const parts: string[] = [];
  if (typeof serverElapsed === "number" && serverElapsed > 0) {
    parts.push(`server ${formatDurationMs(serverElapsed)}`);
  }
  if (typeof workers === "number" && workers > 0) {
    parts.push(`${workers} worker(s)`);
  }
  if (typeof maxNodesApplied === "number" && maxNodesApplied > 0) {
    parts.push(`max nodes ${maxNodesApplied.toLocaleString()}`);
  }
  return parts.length > 0 ? ` - ${parts.join(" - ")}` : "";
}

function solvePhaseText(phase: SolveProgress["phase"]): string {
  switch (phase) {
    case "loading":
      return "Loading solver";
    case "remote":
      return "Remote solving";
    case "seed":
      return "Finding coverage seed";
    case "repair":
      return "Repairing";
    case "search":
      return "Searching";
    case "refine":
      return "Refining";
    case "done":
      return "Done";
    default:
      return "Solving";
  }
}

function formatNodesPerSecond(value: number | undefined, nodesExplored: number, durationMs: number | null): string {
  const nodesPerSecond =
    typeof value === "number" && value > 0
      ? value
      : durationMs && durationMs > 0
        ? nodesExplored / (durationMs / 1000)
        : 0;
  if (!Number.isFinite(nodesPerSecond) || nodesPerSecond <= 0) {
    return "";
  }
  return ` - ${Math.round(nodesPerSecond).toLocaleString()} nodes/sec`;
}

function formatCoverageBuckets(buckets: { covered_sources: number; target_count: number }[]): string {
  return buckets.map((bucket) => `${bucket.covered_sources}/${buckets[0]?.covered_sources || bucket.covered_sources}=${bucket.target_count}`).join(", ");
}

function formatCoveragePruning(checks: number | undefined, pruned: number | undefined): string {
  if (!checks || checks <= 0) {
    return "";
  }
  return ` - coverage pruned ${(pruned || 0).toLocaleString()}/${checks.toLocaleString()}`;
}

function formatExactBoundPruning(checks: number | undefined, pruned: number | undefined): string {
  if (!checks || checks <= 0) {
    return "";
  }
  return ` - exact bound pruned ${(pruned || 0).toLocaleString()}/${checks.toLocaleString()}`;
}

function formatCoverageSeed(nodes: number | undefined, candidates: number | undefined): string {
  if (!nodes || nodes <= 0) {
    return "";
  }
  return ` - seed ${nodes.toLocaleString()} nodes/${(candidates || 0).toLocaleString()} candidates`;
}

function formatRefineSearch(search: Solution["search"]): string {
  if (!search.refine_moves_checked || search.refine_moves_checked <= 0) {
    return "";
  }
  const parts = [`refine ${search.refine_moves_checked.toLocaleString()} moves`];
  if (search.refine_improvements) {
    parts.push(`${search.refine_improvements.toLocaleString()} improvements`);
  }
  if (search.refine_best_delta) {
    parts.push(search.refine_best_delta);
  }
  return ` - ${parts.join("/")}`;
}

function formatRepairSearch(search: Solution["search"]): string {
  if (!search.repair_nodes || search.repair_nodes <= 0) {
    return "";
  }
  const parts = [`repair ${search.repair_nodes.toLocaleString()} nodes`];
  if (search.repair_iterations) {
    parts.push(`${search.repair_iterations.toLocaleString()} iterations`);
  }
  if (search.repair_improvements) {
    parts.push(`${search.repair_improvements.toLocaleString()} improvements`);
  }
  if (search.repair_candidates) {
    parts.push(`${search.repair_candidates.toLocaleString()} candidates`);
  }
  if (search.repair_parallel_tasks) {
    parts.push(`${search.repair_parallel_tasks.toLocaleString()} tasks`);
  }
  if (search.repair_parallel_workers_used) {
    parts.push(`${search.repair_parallel_workers_used.toLocaleString()} workers`);
  }
  return ` - ${parts.join("/")}`;
}

function waitForPaint(): Promise<void> {
  return new Promise((resolve) => {
    requestAnimationFrame(() => {
      requestAnimationFrame(() => resolve());
    });
  });
}

function reviewBounds(item: CatalogItem): { minRow: number; minCol: number; maxRow: number; maxCol: number } {
  const coords = [...item.shape, ...item.stars.map((star) => star.offset)];
  if (coords.length === 0) {
    return { minRow: 0, minCol: 0, maxRow: 0, maxCol: 0 };
  }
  let minRow = coords[0][0];
  let minCol = coords[0][1];
  let maxRow = coords[0][0];
  let maxCol = coords[0][1];
  coords.forEach(([row, col]) => {
    minRow = Math.min(minRow, row);
    minCol = Math.min(minCol, col);
    maxRow = Math.max(maxRow, row);
    maxCol = Math.max(maxCol, col);
  });
  return { minRow, minCol, maxRow, maxCol };
}

function coordKey([row, col]: [number, number]): string {
  return `${row},${col}`;
}

function inBoundsCoord([row, col]: [number, number]): boolean {
  return row >= 0 && row < ROWS && col >= 0 && col < COLS;
}

function coordText([row, col]: [number, number]): string {
  return `(${row}, ${col})`;
}

function starTargetText(star: Star): string {
  if (star.rule_status === "unknown") {
    return "rule unresolved";
  }
  const suffix = star.exclude_source_item ? ", excluding same item" : "";
  if (star.target_types.length === 0 && star.target_items.length === 0) {
    return `any item${suffix}`;
  }
  const parts: string[] = [];
  if (star.target_types.length > 0) {
    parts.push(`types: ${star.target_types.join(", ")}`);
  }
  if (star.target_items.length > 0) {
    parts.push(`items: ${star.target_items.join(", ")}`);
  }
  return `${parts.join(" OR ")}${suffix}`;
}

function countsAsText(alias: { item_id: string; count: number }): string {
  return `${alias.item_id} x${alias.count}`;
}
