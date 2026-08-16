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
import { RemoteSolveError, solveWithOci, solveWithRemote, solveWithWorker } from "./wasm";

const ROWS = 6;
const COLS = 9;

type Grid = boolean[][];
type HeroViewMode = "all" | "hero" | "shared";

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
  repairSearch: boolean;
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
  const [repairSearch, setRepairSearch] = useState(true);
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
  const lastPartialSolutionsRef = useRef<{ value: Solution[] | null; signature: string | null }>({ value: null, signature: null });

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
      setRepairSearch(loadedScenario.repair_search ?? true);
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
        repairSearch,
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
      repairSearch,
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
    setDisabledPriorityKeys((current) => current.filter((key) => optionKeys.includes(key)));
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
              <button className="secondary-action" onClick={() => setGrid(defaultGrid())}>
                Full
              </button>
              <button className="secondary-action" onClick={() => setGrid(emptyGrid())}>
                Empty
              </button>
            </div>
          </div>
          <EditableGrid grid={grid} onToggle={(row, col) => setGrid(toggleGridCell(grid, row, col))} />
          <PriorityPanel
            craftOptions={craftPriorityOptions}
            starOptions={starSourceOptions}
            looseStarOptions={looseStarPriorityOptions}
            globalEntries={globalPriorityEntries}
            coverageGroups={coverageGroups}
            catalog={catalog}
            quantities={quantities}
            disabledKeys={disabledPrioritySet}
            draggingKey={draggingPriorityKey}
            draggingStarSourceID={draggingStarSourceID}
            onMoveCraft={moveCraftPriority}
            onMoveLooseStar={moveLooseStarPriority}
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
  repairSearch,
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
    repair_search: maxNodes > 0 && repairSearch,
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
  if (solution.search.refine_moves_checked && solution.search.refine_moves_checked > 0) {
    lines.push(
      `Refine: moves=${solution.search.refine_moves_checked.toLocaleString()}, improvements=${
        solution.search.refine_improvements || 0
      }${solution.search.refine_best_delta ? `, ${solution.search.refine_best_delta}` : ""}`,
    );
  }
  lines.push(`Score: crafts=${solution.score.crafts}, stars=${solution.score.stars}, items=${solution.score.items}`);
  if (solution.score.priority_counts && solution.score.priority_counts.length > 0) {
    lines.push(`Priority counts: ${solution.score.priority_counts.join("/")}`);
  }
  lines.push("");
  lines.push("## Scenario");
  lines.push(`Grid: ${safeArray(scenario.grid).join("/")}`);
  lines.push(`Top: ${scenario.top}, max_nodes: ${scenario.max_nodes}, no_skips: ${Boolean(scenario.no_skips)}`);
  lines.push(`Stop on coverage ceiling: ${Boolean(scenario.stop_on_coverage_ceiling)}`);
  lines.push(`Repair search: ${Boolean(scenario.repair_search)}`);
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
  lines.push("Coverage groups sent:");
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
  lines.push("## Coverage");
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
  lines.push("## Loose Stars");
  if (!solution.loose_star_priorities || solution.loose_star_priorities.length === 0) {
    lines.push("none");
  } else {
    solution.loose_star_priorities.forEach((priority) =>
      lines.push(`- ${itemName(itemByID, priority.source_item_id)} (${priority.source_item_id}): ${priority.target_count}`),
    );
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

function PriorityPanel({
  craftOptions,
  starOptions,
  looseStarOptions,
  globalEntries,
  coverageGroups,
  catalog,
  quantities,
  disabledKeys,
  draggingKey,
  draggingStarSourceID,
  onMoveCraft,
  onMoveLooseStar,
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
  globalEntries: GlobalPriorityEntry[];
  coverageGroups: CoverageGroup[];
  catalog: Catalog | null;
  quantities: Record<string, number>;
  disabledKeys: Set<string>;
  draggingKey: string | null;
  draggingStarSourceID: string | null;
  onMoveCraft: (key: string, delta: number) => void;
  onMoveLooseStar: (key: string, delta: number) => void;
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
                      {groupSources.length} source(s), {targetCount} target item(s)
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
                  <p className="muted coverage-group-empty">drop star sources here</p>
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
                        <span className="priority-kind star">Stars</span>
                        <label className="orbit-toggle">
                          <input
                            type="checkbox"
                            checked={orbitTarget}
                            onChange={() => onToggleGroupTarget(groupIndex, option.itemID)}
                            aria-label={`${orbitTarget ? "Remove" : "Set"} ${option.title} as orbit target`}
                          />
                          Orbit
                        </label>
                        <button className="priority-mini-action" onClick={() => onRemoveFromGroups(option.itemID)}>
                          Loose
                        </button>
                      </article>
                    );
                  })
                )}
                {targetOptions.length > 0 && (
                  <div className="orbit-target-panel">
                    <div className="orbit-target-title">Orbit targets</div>
                    <div className="orbit-target-options">
                      {targetOptions.map((item) => {
                        const checked = safeArray(group.targets).includes(item.id);
                        return (
                          <label key={item.id} className={checked ? "orbit-target-chip selected" : "orbit-target-chip"}>
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => onToggleGroupTarget(groupIndex, item.id)}
                              aria-label={`${checked ? "Remove" : "Set"} ${item.name} as orbit target`}
                            />
                            {item.image_path && <img src={assetPath(item.image_path)} alt="" loading="lazy" decoding="async" />}
                            <span>{item.name}</span>
                          </label>
                        );
                      })}
                    </div>
                    <p className="orbit-target-hint">none selected = automatic targets</p>
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
                  <span className="priority-kind star">Stars</span>
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
              Add group
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
      <div className="score-strip">
        <span>Crafts {solution.score.crafts}</span>
        <span>Stars {solution.score.stars}</span>
        <span>Items {solution.score.items}</span>
        {solution.score.priority_counts && solution.score.priority_counts.length > 0 && (
          <span>Priority {solution.score.priority_counts.join("/")}</span>
        )}
        {coverageBreakdowns.length > 0 && <span>Coverage {coverageBreakdowns.map((coverage) => coverage.summary).join(" | ")}</span>}
        {looseStarPriorities.length > 0 && (
          <span>Loose stars {looseStarPriorities.map((priority) => `${priority.source_item_id}=${priority.target_count}`).join(" | ")}</span>
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
          <h3>Coverage</h3>
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
          <h3>Loose stars</h3>
          {looseStarPriorities.map((priority) => {
            const item = itemsByID.get(priority.source_item_id);
            return (
              <p key={priority.source_item_id}>
                {item?.name || priority.source_item_id}: {priority.target_count}
              </p>
            );
          })}
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
    .filter((item) => starSourceCanTargetInventory(item, catalog, quantities))
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

function starSourceCanTargetInventory(source: CatalogItem, catalog: Catalog, quantities: Record<string, number>): boolean {
  return source.stars.some((star) =>
    catalog.items.some((target) => {
      if (star.exclude_source_item && target.id === source.id) {
        return false;
      }
      const selectedCount = quantities[target.id] || 0;
      const availableTargets = target.id === source.id ? selectedCount - 1 : selectedCount;
      return availableTargets > 0 && itemMatchesStarFilter(source, target, star);
    }),
  );
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

function coverageGroupsFromPriorities(priorities: string[]): CoverageGroup[] {
  const sources = priorities
    .map((priority) => priority.trim())
    .filter((priority) => priority.startsWith("star_source:"))
    .map((priority) => priority.slice("star_source:".length))
    .filter(Boolean);
  return sources.length > 0 ? [{ name: "Group 1", sources: uniqueStrings(sources), targets: [] }] : [];
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
  priorityOrder.forEach((key) => {
    const groupIndex = parseCoverageGroupPriorityKey(key);
    if (groupIndex !== null) {
      const cleanIndex = groupIndexByOriginal.get(groupIndex);
      if (typeof cleanIndex === "number") {
        priorities.push(coverageGroupPriorityKey(cleanIndex));
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
