package solver

import (
	"context"
	"fmt"
	"math/bits"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

type Config struct {
	TopN           int
	AllowSkips     bool
	MaxNodes       int64
	MaxRefineMoves int64 // Zero derives a bounded limit from MaxNodes.
	Workers        int
	// InitialPlacements is reserved for EvaluateKnownLayout. Automatic search
	// never accepts a manual layout as an incumbent.
	InitialPlacements []model.Placement
	// DiagnosticReference is validated and evaluated solely to annotate
	// diagnostic samples. It is never an incumbent or search input.
	DiagnosticReference   []model.Placement
	PrioritySemantics     model.PrioritySemantics
	Priorities            []string
	CoverageGroups        []model.CoverageGroup
	StopOnCoverageCeiling bool
	StopOnPriorityCeiling bool
	RepairSearch          bool
	PlateauVariant        string
	// EnableConstellationSeedV1 enables the benchmark-only, oracle-blind V3
	// macro-seed experiment. It is intentionally not a scenario setting.
	EnableConstellationSeedV1 bool
	// ConstellationSeedVariant selects an opt-in constellation experiment. Empty
	// keeps the legacy boolean alias semantics and never enables a seed alone.
	ConstellationSeedVariant string
	// ConstellationFeasibilityProbe diagnoses rooted-packing completion using
	// only unused root quota. It requires Diagnostics and never supplies search
	// candidates.
	ConstellationFeasibilityProbe bool
	// ConstellationCompletionOptimizationProbe exactly optimizes completions of
	// completed MRV constellation roots using only unused root quota. It requires Diagnostics
	// and never supplies search candidates.
	ConstellationCompletionOptimizationProbe bool
	// ConstellationCandidatePoolFeasibilitySweep exactly classifies the bounded
	// V4 candidate pool after normal rooted packing. It requires Diagnostics and
	// never supplies search candidates.
	ConstellationCandidatePoolFeasibilitySweep bool
	// ConstellationCandidateCompletionOptimizationProbe exactly optimizes one
	// V4 candidate selected by its stable exact-anchor hash. It requires
	// Diagnostics and never supplies search candidates.
	ConstellationCandidateCompletionOptimizationProbe       bool
	ConstellationCandidateCompletionOptimizationCandidateID string
	ConstellationCandidateCompletionOptimizationStage       string
	// Zero retains the normal residual-quota behavior. A positive value creates
	// an additive, diagnostic-only ledger lane for the targeted exact probe.
	ConstellationCandidateCompletionOptimizationNodeBudget                        int64
	ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey           string
	ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint string
	ConstellationForcedCandidateRootedPackingProbe                                bool
	ConstellationForcedCandidateRootedPackingCandidateID                          string
	ConstellationForcedCandidateRootedPackingSlot                                 int
	ConstellationForcedCandidateRootedPackingStage                                string
	ConstellationForcedCandidateRootedPackingBeamWidth                            int
	ConstellationForcedCandidateRootedPackingRanking                              string
	ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey               string
	ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint     string
	// ConstellationParentFrontierHedgeProbe replays the V5-selected parent and
	// frontier in one sealed diagnostic family quota.
	ConstellationParentFrontierHedgeProbe      bool
	ConstellationParentFrontierHedgeProbeStage string
	DisableExactBounds                         bool
	DisableOutgoingBounds                      bool
	// Diagnostics records an unthrottled incumbent trace without affecting
	// search ordering, progress reporting, or pruning.
	Diagnostics bool
	// OperationProfiling enables benchmark-only deterministic search-operation
	// counters. It never participates in search ordering, pruning, ranking, or
	// budgets, and requires a binary built with -tags searchprofile.
	OperationProfiling bool
	ProgressReporter   ProgressReporter
	Context            context.Context

	trace                                    *diagnosticTrace
	tracePhase                               string
	priorityBounds                           *priorityBoundContext
	plateauArchive                           *plateauArchive
	policy                                   *ResolvedSearchPolicy
	ledger                                   *nodeLedger
	stageID                                  string
	stageIncumbents                          []model.Solution
	executionFingerprint                     string
	taskAllocation                           model.TaskAllocationStats
	repairPriorityTarget                     []int
	constellationRootOrigins                 map[string]string
	constellationCandidateCompletionSnapshot *constellationCandidateCompletionSnapshot
	constellationRootPackingCollector        *constellationRootPackingCollector
	forcedRootPackingReplay                  bool
	diagnosticNodeBudget                     int64
}

type searchTask struct {
	Index         int
	Occupied      uint64
	Placements    []model.Placement
	NodeBudget    int64
	HasNodeBudget bool
}

type searchResult struct {
	Solutions                   []model.Solution
	NodesExplored               int64
	HitNodeBudget               bool
	CoverageBoundChecks         int64
	CoveragePrunedNodes         int64
	ExactBoundChecks            int64
	ExactBoundPrunedNodes       int64
	OutgoingBoundChecks         int64
	OutgoingBoundPrunedNodes    int64
	SymmetryPrunedBranches      int64
	FirstCompleteNode           int64
	ParallelTasks               int
	ParallelWorkersUsed         int
	StoppedAfterCoverageCeiling bool
	StoppedAfterPriorityCeiling bool
	TasksExecuted               int
	TasksPrunedBeforeExecution  int
	TaskAllocation              model.TaskAllocationStats
	BoundOperationProfile       *model.BoundAttributionOperationProfile
}

type coverageSeedResult struct {
	Solutions                   []model.Solution
	NodesExplored               int64
	CandidateCount              int
	BestSummary                 string
	StoppedAfterCoverageCeiling bool
	SymmetryPrunedBranches      int64
	StatesDeduplicated          int64
	HardPrunedNodes             int64
	FirstCompleteNode           int64
	PackingSeedOperationProfile *model.PackingSeedFeasibilityOperationProfile
	BoundOperationProfile       *model.BoundAttributionOperationProfile
	PackingDiagnostics          model.PackingSeedDiagnostics
}

type repairResult struct {
	Solutions                    []model.Solution
	NodesExplored                int64
	Iterations                   int
	Improvements                 int
	CandidateCount               int
	BestSummary                  string
	CoverageBoundChecks          int64
	CoveragePrunedNodes          int64
	ExactBoundChecks             int64
	ExactBoundPrunedNodes        int64
	OutgoingBoundChecks          int64
	OutgoingBoundPrunedNodes     int64
	SymmetryPrunedBranches       int64
	ParallelTasks                int
	ParallelWorkersUsed          int
	PriorityPreservingCandidates int64
	PriorityBoundPruned          int64
	CompletedBelowPriority       int64
	CompareScoreImprovements     int64
	NeighborhoodsGenerated       int
	NeighborhoodsAttempted       int
	TerminationReason            string
	BoundOperationProfile        *model.BoundAttributionOperationProfile
}

type taskRunResult struct {
	Index  int
	Result searchResult
}

type searchJob struct {
	Index int
	Task  searchTask
}

func ParseInventorySpec(spec string) ([]string, error) {
	var itemIDs []string
	for _, rawPart := range strings.Split(spec, ",") {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}
		if strings.Contains(part, ":") {
			pieces := strings.SplitN(part, ":", 2)
			itemID := strings.TrimSpace(pieces[0])
			count, err := strconv.Atoi(strings.TrimSpace(pieces[1]))
			if err != nil {
				return nil, fmt.Errorf("invalid count in %q: %w", part, err)
			}
			if count < 0 {
				return nil, fmt.Errorf("item count must be non-negative: %s", part)
			}
			for i := 0; i < count; i++ {
				itemIDs = append(itemIDs, itemID)
			}
		} else {
			itemIDs = append(itemIDs, part)
		}
	}
	if len(itemIDs) == 0 {
		return nil, fmt.Errorf("inventory cannot be empty")
	}
	return itemIDs, nil
}

func ExpandInventory(itemIDs []string) []model.InventoryInstance {
	instances := make([]model.InventoryInstance, 0, len(itemIDs))
	for idx, itemID := range itemIDs {
		instances = append(instances, model.InventoryInstance{
			InstanceID:    fmt.Sprintf("%s#%d", itemID, idx),
			ItemID:        itemID,
			OriginalIndex: idx,
		})
	}
	return instances
}

func PlacementOptions(catalog model.Catalog, instance model.InventoryInstance, gridMask uint64) ([]model.Placement, error) {
	item, ok := catalog.Items[instance.ItemID]
	if !ok {
		return nil, fmt.Errorf("unknown item %q", instance.ItemID)
	}

	variants, err := geometry.VariantsForItem(item)
	if err != nil {
		return nil, err
	}

	var options []model.Placement
	for _, variant := range variants {
		maxRow := 0
		maxCol := 0
		for _, cell := range variant.Cells {
			if cell.Row > maxRow {
				maxRow = cell.Row
			}
			if cell.Col > maxCol {
				maxCol = cell.Col
			}
		}
		for originRow := 0; originRow < geometry.GridRows-maxRow; originRow++ {
			for originCol := 0; originCol < geometry.GridCols-maxCol; originCol++ {
				origin := model.Coord{Row: originRow, Col: originCol}
				cells := geometry.TranslateCells(variant.Cells, origin)
				valid := true
				for _, cell := range cells {
					if !geometry.InBounds(cell) {
						valid = false
						break
					}
				}
				if !valid {
					continue
				}
				mask := geometry.MaskFromCells(cells)
				if mask&^gridMask != 0 {
					continue
				}

				starPositions := make([]model.StarPosition, 0, len(variant.Stars))
				for _, star := range variant.Stars {
					starPositions = append(starPositions, model.StarPosition{
						Star: star,
						Position: model.Coord{
							Row: origin.Row + star.Offset.Row,
							Col: origin.Col + star.Offset.Col,
						},
					})
				}

				options = append(options, model.Placement{
					InstanceID:    instance.InstanceID,
					ItemID:        instance.ItemID,
					OriginalIndex: instance.OriginalIndex,
					Rotation:      variant.Rotation,
					Origin:        origin,
					Cells:         cells,
					StarPositions: starPositions,
					Mask:          mask,
					AdjacentMask:  geometry.AdjacentMaskFromCells(cells),
				})
			}
		}
	}

	sort.Slice(options, func(i, j int) bool {
		return placementKey(options[i]) < placementKey(options[j])
	})
	return options, nil
}

func placementOptionsForInstance(template []model.Placement, instance model.InventoryInstance) []model.Placement {
	options := make([]model.Placement, len(template))
	copy(options, template)
	for idx := range options {
		options[idx].InstanceID = instance.InstanceID
		options[idx].ItemID = instance.ItemID
		options[idx].OriginalIndex = instance.OriginalIndex
	}
	return options
}

func initialSolutionsForConfig(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
) ([]model.Solution, error) {
	if len(config.InitialPlacements) == 0 {
		return nil, nil
	}

	instanceByID := make(map[string]model.InventoryInstance, len(instances))
	for _, instance := range instances {
		instanceByID[instance.InstanceID] = instance
	}
	placements := make([]model.Placement, 0, len(config.InitialPlacements))
	seen := make(map[string]struct{}, len(config.InitialPlacements))
	var occupied uint64
	for _, requested := range config.InitialPlacements {
		instance, ok := instanceByID[requested.InstanceID]
		if !ok {
			return nil, fmt.Errorf("initial placement references unknown instance %q", requested.InstanceID)
		}
		if _, exists := seen[instance.InstanceID]; exists {
			return nil, fmt.Errorf("initial placement repeats instance %q", instance.InstanceID)
		}
		if requested.ItemID != "" && requested.ItemID != instance.ItemID {
			return nil, fmt.Errorf("initial placement %q has item %q, want %q", instance.InstanceID, requested.ItemID, instance.ItemID)
		}

		var matched *model.Placement
		rotationIsEquivalent := false
		item := catalog.Items[instance.ItemID]
		for optionIndex := range optionsByInstance[instance.InstanceID] {
			option := &optionsByInstance[instance.InstanceID][optionIndex]
			if option.Origin != requested.Origin {
				continue
			}
			if option.Rotation == requested.Rotation {
				matched = option
				break
			}
			if rotationsRepresentSameVariant(item, requested.Rotation, option.Rotation) {
				matched = option
				rotationIsEquivalent = true
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("initial placement %q at rotation %d origin %s is invalid", instance.InstanceID, requested.Rotation, requested.Origin)
		}
		if occupied&matched.Mask != 0 {
			return nil, fmt.Errorf("initial placement %q overlaps another placement", instance.InstanceID)
		}
		selected := *matched
		if rotationIsEquivalent {
			// The solver stores one canonical rotation for symmetric variants.
			// Retain the user's equivalent orientation for a manual layout preview.
			selected.Rotation = requested.Rotation
		}
		seen[instance.InstanceID] = struct{}{}
		occupied |= selected.Mask
		placements = append(placements, selected)
	}
	if !config.AllowSkips && len(placements) != len(instances) {
		return nil, fmt.Errorf("initial layout places %d of %d instances while skips are disabled", len(placements), len(instances))
	}

	sortPlacementsByOriginal(placements)
	evaluation := evaluateLayoutForConfig(catalog, placements, config)
	return []model.Solution{{
		Placements:          placements,
		Evaluation:          evaluation,
		LayoutKey:           layoutKey(placements, instances),
		CanonicalLayoutHash: canonicalLayoutHash(placements),
	}}, nil
}

func rotationsRepresentSameVariant(item model.Item, leftRotation int, rightRotation int) bool {
	left, leftErr := geometry.NormalizeVariant(item.Shape, item.Stars, leftRotation)
	right, rightErr := geometry.NormalizeVariant(item.Shape, item.Stars, rightRotation)
	if leftErr != nil || rightErr != nil || len(left.Cells) != len(right.Cells) || len(left.Stars) != len(right.Stars) {
		return false
	}
	for index := range left.Cells {
		if left.Cells[index] != right.Cells[index] {
			return false
		}
	}
	for index := range left.Stars {
		if left.Stars[index].Offset != right.Stars[index].Offset {
			return false
		}
	}
	return true
}

func SolveLayout(catalog model.Catalog, itemIDs []string, gridMask uint64, config Config) ([]model.Solution, error) {
	if len(config.InitialPlacements) > 0 {
		return nil, fmt.Errorf("initial placements are supported only by EvaluateKnownLayout")
	}
	if config.TopN <= 0 {
		config.TopN = 3
	}
	if config.Workers <= 0 {
		config.Workers = 1
	}
	if config.Context == nil {
		config.Context = context.Background()
	}
	if config.Diagnostics && config.Workers != 1 {
		return nil, fmt.Errorf("diagnostics require exactly one worker")
	}
	if config.OperationProfiling && config.Workers != 1 {
		return nil, fmt.Errorf("operation profiling requires exactly one worker")
	}
	if config.OperationProfiling && !OperationProfilingAvailable() {
		return nil, fmt.Errorf("operation profiling requires a binary built with -tags searchprofile")
	}
	if config.ConstellationFeasibilityProbe && !config.Diagnostics {
		return nil, fmt.Errorf("constellation feasibility probe requires diagnostics")
	}
	if config.ConstellationCompletionOptimizationProbe && !config.Diagnostics {
		return nil, fmt.Errorf("constellation completion optimization probe requires diagnostics")
	}
	if config.ConstellationCompletionOptimizationProbe {
		if err := ValidateConstellationCompletionOptimizationProbeVariant(config.ConstellationSeedVariant); err != nil {
			return nil, err
		}
	}
	if config.ConstellationFeasibilityProbe && config.ConstellationCompletionOptimizationProbe {
		return nil, fmt.Errorf("constellation feasibility and completion optimization probes cannot run together")
	}
	if config.ConstellationCandidatePoolFeasibilitySweep && !config.Diagnostics {
		return nil, fmt.Errorf("constellation candidate pool feasibility sweep requires diagnostics")
	}
	if config.ConstellationCandidatePoolFeasibilitySweep && config.ConstellationSeedVariant != ConstellationSeedVariantV4 {
		return nil, fmt.Errorf("constellation candidate pool feasibility sweep requires constellation seed variant %q", ConstellationSeedVariantV4)
	}
	if config.ConstellationCandidatePoolFeasibilitySweep && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe) {
		return nil, fmt.Errorf("constellation candidate pool feasibility sweep cannot run with another constellation probe")
	}
	if config.ConstellationCandidateCompletionOptimizationProbe && !config.Diagnostics {
		return nil, fmt.Errorf("constellation candidate completion optimization probe requires diagnostics")
	}
	if config.ConstellationCandidateCompletionOptimizationProbe {
		if err := ValidateConstellationCandidateCompletionOptimizationTarget(config.ConstellationSeedVariant, config.ConstellationCandidateCompletionOptimizationCandidateID, config.ConstellationCandidateCompletionOptimizationStage); err != nil {
			return nil, err
		}
		if config.ConstellationCandidateCompletionOptimizationNodeBudget < 0 {
			return nil, fmt.Errorf("constellation candidate completion optimization node budget must be non-negative")
		}
		if (config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey == "") != (config.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint == "") {
			return nil, fmt.Errorf("candidate completion optimization witness layout key and semantic fingerprint must be supplied together")
		}
		if config.ConstellationCandidateCompletionOptimizationNodeBudget > 0 {
			if config.MaxNodes == 20_000_000 && (config.ConstellationCandidateCompletionOptimizationStage == "" || config.ConstellationCandidateCompletionOptimizationStage == "single") {
				return nil, fmt.Errorf("dedicated candidate completion optimization budget requires an explicit 20M stage")
			}
			if config.MaxNodes != 20_000_000 && config.ConstellationCandidateCompletionOptimizationStage != "" && config.ConstellationCandidateCompletionOptimizationStage != "single" {
				return nil, fmt.Errorf("dedicated candidate completion optimization budget requires stage %q for a single-stage run", "single")
			}
		}
	}
	if config.ConstellationCandidateCompletionOptimizationProbe && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe || config.ConstellationCandidatePoolFeasibilitySweep) {
		return nil, fmt.Errorf("constellation candidate completion optimization probe cannot run with another constellation probe")
	}
	if config.ConstellationForcedCandidateRootedPackingProbe {
		if err := ValidateConstellationForcedCandidateRootedPackingTarget(config.ConstellationSeedVariant, config.ConstellationForcedCandidateRootedPackingCandidateID, config.ConstellationForcedCandidateRootedPackingSlot, config.ConstellationForcedCandidateRootedPackingStage); err != nil {
			return nil, err
		}
		if (config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey == "") != (config.ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint == "") {
			return nil, fmt.Errorf("forced rooted packing shadow witness layout key and semantic fingerprint must be supplied together")
		}
		if config.ConstellationForcedCandidateRootedPackingBeamWidth < 0 {
			return nil, fmt.Errorf("constellation forced candidate rooted packing beam width must be non-negative")
		}
		if err := ValidateConstellationForcedCandidateRootedPackingRanking(config.ConstellationForcedCandidateRootedPackingRanking); err != nil {
			return nil, err
		}
	}
	if config.ConstellationForcedCandidateRootedPackingProbe && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe || config.ConstellationCandidatePoolFeasibilitySweep || config.ConstellationCandidateCompletionOptimizationProbe) {
		return nil, fmt.Errorf("constellation forced candidate rooted packing probe cannot run with another constellation probe")
	}
	if config.ConstellationParentFrontierHedgeProbe {
		if !config.Diagnostics {
			return nil, fmt.Errorf("constellation parent-frontier hedge probe requires diagnostics")
		}
		if err := ValidateConstellationParentFrontierHedgeProbeTarget(config.ConstellationSeedVariant, config.ConstellationParentFrontierHedgeProbeStage); err != nil {
			return nil, err
		}
	}
	if config.ConstellationParentFrontierHedgeProbe && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe || config.ConstellationCandidatePoolFeasibilitySweep || config.ConstellationCandidateCompletionOptimizationProbe || config.ConstellationForcedCandidateRootedPackingProbe) {
		return nil, fmt.Errorf("constellation parent-frontier hedge probe cannot run with another constellation probe")
	}

	stages := configuredSearchStages(config)
	diagnosticBudget := int64(0)
	if config.ConstellationCandidateCompletionOptimizationProbe {
		diagnosticBudget = config.ConstellationCandidateCompletionOptimizationNodeBudget
	}
	ledger := newNodeLedger(config.MaxNodes, stages, diagnosticBudget)
	executionFingerprint := resolvedExecutionFingerprint(config, stages)
	var carried []model.Solution
	instances := ExpandInventory(itemIDs)
	archive := newPlateauArchiveWithCapacity(newPriorityBoundContext(catalog, instances, config.Priorities, config.PrioritySemantics), stages[0].Policy.PlateauArchiveCapacity)
	var results []model.Solution
	stageSearches := make([]model.SearchStats, 0, len(stages))
	stageStats := make([]model.SearchStageStats, 0, len(stages))
	for index, stage := range stages {
		stageConfig := config
		stageConfig.MaxNodes = stage.NodeLimit
		stageConfig.MaxRefineMoves = stage.Policy.RefineMoveLimit
		stageConfig.policy = &stage.Policy
		stageConfig.ledger = ledger
		stageConfig.stageID = stage.ID
		stageConfig.executionFingerprint = executionFingerprint
		stageConfig.diagnosticNodeBudget = diagnosticBudgetForStage(config, stage.ID)
		stageConfig.plateauArchive = archive
		stageConfig.stageIncumbents = cloneStageSolutions(carried)
		if index > 0 {
			// Only automatic candidates can cross a scheduler boundary.
			stageConfig.InitialPlacements = nil
		}
		inputScore := bestScore(carried)
		stageResults, err := solveLayoutStage(catalog, itemIDs, gridMask, stageConfig)
		if err != nil {
			return nil, err
		}
		var stageSearch model.SearchStats
		hasStageSearch := len(stageResults) > 0
		if hasStageSearch {
			stageSearch = stageResults[0].Search
		}
		// A stage may add candidates, but it cannot discard an automatic
		// incumbent carried by its predecessor.
		stageResults = mergeSolutions(carried, stageResults, config.TopN)
		if len(stageResults) == 0 {
			continue
		}
		search := stageResults[0].Search
		if hasStageSearch {
			// The merge above may retain an equal carried incumbent first. Stage
			// telemetry must still describe work performed by this stage.
			search = stageSearch
		}
		carried = cloneStageSolutions(stageResults)
		archive = stageConfig.plateauArchive
		stageSearches = append(stageSearches, search)
		budget := ledger.snapshot(stage.ID)
		stageStat := model.SearchStageStats{
			ID:                                stage.ID,
			NodeLimit:                         stage.NodeLimit,
			StagePolicyFingerprint:            resolvedPolicyFingerprint(stage.Policy),
			StageInputScore:                   cloneScore(inputScore),
			StageOutputScore:                  cloneScore(bestScore(stageResults)),
			NodesReserved:                     stage.NodeLimit,
			NodesCharged:                      budget.StageCharged,
			StageBudgetConsumed:               budget.StageCharged,
			ExecutionBudgetConsumed:           budget.ExecutionCharged,
			DiagnosticNodesReserved:           diagnosticReservationForSearch(search),
			DiagnosticNodesCharged:            budget.DiagnosticStageCharged,
			DiagnosticExecutionBudgetConsumed: budget.DiagnosticExecutionCharged,
			ExecutionTotalBudgetConsumed:      budget.ExecutionCharged + budget.DiagnosticExecutionCharged,
			FinalCarriedScore:                 cloneScore(bestScore(carried)),
			TaskAllocation:                    search.TaskAllocation,
			PhaseWork:                         clonePhaseWork(search.PhaseWork),
		}
		if config.Diagnostics {
			stageStat.IncumbentTrace = append([]model.IncumbentEvent(nil), search.IncumbentTrace...)
			stageStat.PlateauArchive = search.PlateauArchive
		}
		stageStats = append(stageStats, stageStat)
		results = stageResults
	}
	if len(results) == 0 {
		return nil, nil
	}
	search := aggregateStageSearches(stageSearches, ledger.total())
	search.NormalBudgetConfigured = config.MaxNodes
	search.NormalBudgetConsumed = ledger.total()
	search.DiagnosticBudgetConfigured = ledger.diagnosticMaxValue()
	search.DiagnosticBudgetConsumed = ledger.diagnosticTotal()
	search.ExecutionBudgetConfigured = config.MaxNodes + search.DiagnosticBudgetConfigured
	search.ExecutionBudgetConsumed = search.NormalBudgetConsumed + search.DiagnosticBudgetConsumed
	if config.MaxNodes > search.NodesExplored {
		search.UnusedGlobalNodes = config.MaxNodes - search.NodesExplored
	}
	search.Stages = stageStats
	search.ExecutionFingerprint = executionFingerprint
	// Kept as an alias for callers compiled against the former field.
	search.ConfigFingerprint = executionFingerprint
	for index := range results {
		results[index].Search = search
	}
	return results, nil
}

func diagnosticBudgetForStage(config Config, stageID string) int64 {
	if config.ConstellationCandidateCompletionOptimizationNodeBudget <= 0 {
		return 0
	}
	target := config.ConstellationCandidateCompletionOptimizationStage
	if target == "" || target == stageID {
		return config.ConstellationCandidateCompletionOptimizationNodeBudget
	}
	return 0
}

func diagnosticReservationForSearch(search model.SearchStats) int64 {
	var reserved int64
	for _, phase := range search.PhaseWork {
		if phase.BudgetScope == "diagnostic" {
			reserved += phase.NodesReserved
		}
	}
	return reserved
}

func candidateOptimizationTermination(optimization model.ConstellationCandidateCompletionOptimization) string {
	if len(optimization.Attempts) == 0 {
		return "target_not_found"
	}
	attempt := optimization.Attempts[0]
	if attempt.SelectionStatus != "accepted" {
		return attempt.SelectionStatus
	}
	return attempt.TerminationReason
}

func forcedRootedPackingTermination(probe model.ConstellationForcedCandidateRootedPacking) string {
	if len(probe.Attempts) == 0 {
		return "target_not_found"
	}
	attempt := probe.Attempts[0]
	if attempt.SelectionStatus != "accepted" {
		return attempt.SelectionStatus
	}
	return attempt.TerminationReason
}

func parentFrontierHedgeTermination(probe model.ConstellationParentFrontierHedge) string {
	if len(probe.Attempts) == 0 {
		return "target_not_found"
	}
	attempt := probe.Attempts[0]
	if attempt.SelectionStatus != "accepted" {
		return attempt.SelectionStatus
	}
	if attempt.Frontier.SkippedReason != "" {
		return attempt.Frontier.SkippedReason
	}
	return attempt.Frontier.TerminationReason
}

func cloneStageSolutions(solutions []model.Solution) []model.Solution {
	if len(solutions) == 0 {
		return nil
	}
	cloned := make([]model.Solution, len(solutions))
	for index, solution := range solutions {
		cloned[index] = clonePlateauSolution(solution)
	}
	return cloned
}

func aggregateStageSearches(stages []model.SearchStats, charged int64) model.SearchStats {
	if len(stages) == 0 {
		return model.SearchStats{NodesExplored: charged}
	}
	aggregated := stages[len(stages)-1]
	aggregated.NodesExplored = charged
	aggregated.GlobalBudgetConsumed = charged
	aggregated.PhaseWork = aggregatePhaseWork(stages)
	if len(stages) == 1 {
		return aggregated
	}
	aggregated.SetupMS = 0
	aggregated.CoverageBoundChecks = 0
	aggregated.CoveragePrunedNodes = 0
	aggregated.ExactBoundChecks = 0
	aggregated.ExactBoundPrunedNodes = 0
	aggregated.OutgoingBoundChecks = 0
	aggregated.OutgoingBoundPrunedNodes = 0
	aggregated.CoverageSeedNodes = 0
	aggregated.CoverageSeedCandidates = 0
	aggregated.StarSeedNodes = 0
	aggregated.StarSeedCandidates = 0
	aggregated.ConstellationSeedNodes = 0
	aggregated.ConstellationSeedCandidates = 0
	aggregated.ConstellationSeedDiagnostics = model.ConstellationSeedDiagnostics{}
	aggregated.PackingSeedNodes = 0
	aggregated.PackingSeedCandidates = 0
	aggregated.PackingSeedHardPruned = 0
	aggregated.PackingSeedStatesDeduplicated = 0
	aggregated.PackingSeedOperationProfile = nil
	aggregated.BoundOperationProfile = nil
	aggregated.SymmetryPrunedBranches = 0
	aggregated.ParallelTasks = 0
	aggregated.RepairNodes = 0
	aggregated.RepairIterations = 0
	aggregated.RepairImprovements = 0
	aggregated.RepairCandidates = 0
	aggregated.RepairParallelTasks = 0
	aggregated.PlateauLNSNodes = 0
	aggregated.PlateauRefineNodes = 0
	aggregated.PlateauArchive.ReferenceEvaluations = 0
	aggregated.PlateauArchive.MinimumReferenceDistance = nil
	for _, stage := range stages {
		aggregated.SetupMS += stage.SetupMS
		aggregated.CoverageBoundChecks += stage.CoverageBoundChecks
		aggregated.CoveragePrunedNodes += stage.CoveragePrunedNodes
		aggregated.ExactBoundChecks += stage.ExactBoundChecks
		aggregated.ExactBoundPrunedNodes += stage.ExactBoundPrunedNodes
		aggregated.OutgoingBoundChecks += stage.OutgoingBoundChecks
		aggregated.OutgoingBoundPrunedNodes += stage.OutgoingBoundPrunedNodes
		aggregated.CoverageSeedNodes += stage.CoverageSeedNodes
		aggregated.CoverageSeedCandidates += stage.CoverageSeedCandidates
		aggregated.StarSeedNodes += stage.StarSeedNodes
		aggregated.StarSeedCandidates += stage.StarSeedCandidates
		aggregated.ConstellationSeedNodes += stage.ConstellationSeedNodes
		aggregated.ConstellationSeedCandidates += stage.ConstellationSeedCandidates
		aggregated.ConstellationSeedDiagnostics = mergeConstellationSeedDiagnostics(aggregated.ConstellationSeedDiagnostics, stage.ConstellationSeedDiagnostics)
		aggregated.PackingSeedNodes += stage.PackingSeedNodes
		aggregated.PackingSeedCandidates += stage.PackingSeedCandidates
		aggregated.PackingSeedHardPruned += stage.PackingSeedHardPruned
		aggregated.PackingSeedStatesDeduplicated += stage.PackingSeedStatesDeduplicated
		aggregated.PackingSeedOperationProfile = mergePackingSeedFeasibilityOperationProfiles(aggregated.PackingSeedOperationProfile, stage.PackingSeedOperationProfile)
		aggregated.BoundOperationProfile = mergeBoundAttributionOperationProfiles(aggregated.BoundOperationProfile, stage.BoundOperationProfile)
		aggregated.SymmetryPrunedBranches += stage.SymmetryPrunedBranches
		aggregated.ParallelTasks += stage.ParallelTasks
		aggregated.RepairNodes += stage.RepairNodes
		aggregated.RepairIterations += stage.RepairIterations
		aggregated.RepairImprovements += stage.RepairImprovements
		aggregated.RepairCandidates += stage.RepairCandidates
		aggregated.RepairParallelTasks += stage.RepairParallelTasks
		aggregated.PlateauLNSNodes += stage.PlateauLNSNodes
		aggregated.PlateauRefineNodes += stage.PlateauRefineNodes
		aggregated.PlateauArchive.ReferenceEvaluations += stage.PlateauArchive.ReferenceEvaluations
		if minimum := stage.PlateauArchive.MinimumReferenceDistance; minimum != nil && (aggregated.PlateauArchive.MinimumReferenceDistance == nil || referenceDistanceLess(*minimum, *aggregated.PlateauArchive.MinimumReferenceDistance)) {
			aggregated.PlateauArchive.MinimumReferenceDistance = cloneReferenceDistance(minimum)
		}
		if stage.ParallelWorkersUsed > aggregated.ParallelWorkersUsed {
			aggregated.ParallelWorkersUsed = stage.ParallelWorkersUsed
		}
		if stage.RepairParallelWorkersUsed > aggregated.RepairParallelWorkersUsed {
			aggregated.RepairParallelWorkersUsed = stage.RepairParallelWorkersUsed
		}
	}
	return aggregated
}

func clonePhaseWork(work []model.SearchPhaseWork) []model.SearchPhaseWork {
	if len(work) == 0 {
		return nil
	}
	cloned := make([]model.SearchPhaseWork, len(work))
	for index, phase := range work {
		cloned[index] = phase
		if phase.BestScore != nil {
			score := cloneScore(*phase.BestScore)
			cloned[index].BestScore = &score
		}
	}
	return cloned
}

func aggregatePhaseWork(stages []model.SearchStats) []model.SearchPhaseWork {
	if len(stages) == 0 {
		return nil
	}
	byPhase := make(map[string]model.SearchPhaseWork)
	for _, stage := range stages {
		for _, phase := range stage.PhaseWork {
			combined, exists := byPhase[phase.Phase]
			if !exists {
				combined.Phase = phase.Phase
			}
			combined.ChargedNodes += phase.ChargedNodes
			if phase.BudgetScope != "" {
				combined.BudgetScope = phase.BudgetScope
			}
			combined.NodesConsumed += phase.NodesConsumed
			combined.UnchargedMoves += phase.UnchargedMoves
			combined.Candidates += phase.Candidates
			combined.NodesReserved += phase.NodesReserved
			combined.NodesReturned += phase.NodesReturned
			combined.Eligible = combined.Eligible || phase.Eligible
			combined.Invoked = combined.Invoked || phase.Invoked
			if phase.BestScore != nil && (combined.BestScore == nil || compareScores(*phase.BestScore, *combined.BestScore) > 0) {
				score := cloneScore(*phase.BestScore)
				combined.BestScore = &score
			}
			if !phase.Eligible && phase.SkipReason != "" {
				combined.SkipReason = phase.SkipReason
			}
			if phase.TerminationReason != "" {
				combined.TerminationReason = phase.TerminationReason
			}
			if phase.ReturnTarget != "" {
				combined.ReturnTarget = phase.ReturnTarget
			}
			byPhase[phase.Phase] = combined
		}
	}
	result := make([]model.SearchPhaseWork, 0, len(byPhase))
	for _, phase := range tracePhases {
		if combined, exists := byPhase[phase]; exists {
			combined.ChargedNodes = combined.NodesConsumed
			result = append(result, combined)
		}
	}
	return result
}

func solveLayoutStage(catalog model.Catalog, itemIDs []string, gridMask uint64, config Config) ([]model.Solution, error) {
	if len(itemIDs) > geometry.GridCells {
		return nil, fmt.Errorf("inventory has %d items; maximum is %d for the %dx%d grid", len(itemIDs), geometry.GridCells, geometry.GridRows, geometry.GridCols)
	}
	if config.TopN <= 0 {
		config.TopN = 3
	}
	if config.policy == nil {
		policy := resolveSearchPolicy(config, config.MaxNodes)
		config.policy = &policy
	}
	if config.MaxRefineMoves <= 0 {
		config.MaxRefineMoves = config.policy.RefineMoveLimit
	}
	if config.Workers <= 0 {
		config.Workers = 1
	}
	if config.Diagnostics && config.Workers != 1 {
		return nil, fmt.Errorf("diagnostics require exactly one worker")
	}
	if config.OperationProfiling && config.Workers != 1 {
		return nil, fmt.Errorf("operation profiling requires exactly one worker")
	}
	if config.OperationProfiling && !OperationProfilingAvailable() {
		return nil, fmt.Errorf("operation profiling requires a binary built with -tags searchprofile")
	}
	if config.Context == nil {
		config.Context = context.Background()
	}

	instances := ExpandInventory(itemIDs)
	var missing []string
	for _, instance := range instances {
		if _, ok := catalog.Items[instance.ItemID]; !ok {
			missing = append(missing, instance.ItemID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("inventory references unknown item(s): %s", strings.Join(uniqueStrings(missing), ", "))
	}
	catalog = filterInventoryImpossibleRecipes(catalog, itemIDs)

	solveStarted := time.Now()
	setupStarted := solveStarted
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(catalog, instance, gridMask)
		if err != nil {
			return nil, err
		}
		optionsByInstance[instance.InstanceID] = options
	}
	knownSolutions := cloneStageSolutions(config.stageIncumbents)
	if len(knownSolutions) == 0 {
		var err error
		knownSolutions, err = initialSolutionsForConfig(catalog, instances, optionsByInstance, config)
		if err != nil {
			return nil, err
		}
	}
	initialBestPriorityCounts := priorityCountsForSolutions(knownSolutions)
	coverage := newCoverageContextForConfig(catalog, instances, optionsByInstance, config)
	priorityBounds := newPriorityBoundContext(catalog, instances, config.Priorities, config.PrioritySemantics)
	config.priorityBounds = priorityBounds
	if config.plateauArchive == nil {
		config.plateauArchive = newPlateauArchiveWithCapacity(priorityBounds, config.policy.PlateauArchiveCapacity)
	}
	if config.constellationRootOrigins == nil {
		config.constellationRootOrigins = make(map[string]string)
	}
	starBounds := newStarUpperBoundContext(catalog, instances, optionsByInstance)
	var diagnosticReference *model.Solution
	if config.Diagnostics && len(config.DiagnosticReference) > 0 {
		referenceConfig := config
		referenceConfig.InitialPlacements = config.DiagnosticReference
		referenceConfig.DiagnosticReference = nil
		references, referenceErr := initialSolutionsForConfig(catalog, instances, optionsByInstance, referenceConfig)
		if referenceErr != nil {
			return nil, fmt.Errorf("diagnostic reference: %w", referenceErr)
		}
		if len(references) == 1 {
			reference := references[0]
			diagnosticReference = &reference
		}
	}
	if config.Diagnostics {
		config.trace = newDiagnosticTrace(solveStarted, config.MaxNodes, config.ledger, config.stageID, priorityBounds, starBounds, diagnosticReference, config.policy.ConstellationSeedVersion != "")
		initializeDiagnosticPhasePlans(config)
	}
	var starPotential *starPotentialContext
	if config.MaxNodes > 0 && config.PrioritySemantics.IsOutgoing() {
		starPotential = newStarPotentialContext(catalog, instances, optionsByInstance, config.Priorities, config.PrioritySemantics)
	}
	sortPlacementOptionsForCoverage(optionsByInstance, coverage, starPotential)

	requestedTopN := config.TopN
	searchConfig := config
	searchConfig.TopN = config.policy.CandidateLimit
	searchConfig.StopOnCoverageCeiling = config.policy.StopOnCoverageCeiling

	limitedMode := config.MaxNodes > 0
	ordered := append([]model.InventoryInstance(nil), instances...)
	sort.Slice(ordered, func(i, j int) bool {
		if limitedMode {
			leftPriority := instancePriority(catalog, ordered[i], config.Priorities, coverage, starPotential)
			rightPriority := instancePriority(catalog, ordered[j], config.Priorities, coverage, starPotential)
			if leftPriority != rightPriority {
				return leftPriority > rightPriority
			}
		}
		left := len(optionsByInstance[ordered[i].InstanceID])
		right := len(optionsByInstance[ordered[j].InstanceID])
		if left != right {
			return left < right
		}
		return ordered[i].OriginalIndex < ordered[j].OriginalIndex
	})
	if coverage != nil {
		coverage.prepareOrder(ordered)
	}
	exactBounds := newExactBoundContext(catalog, instances, ordered, optionsByInstance, searchConfig)
	outgoingBounds := newOutgoingBoundContext(catalog, instances, optionsByInstance, searchConfig, starPotential)
	setupMS := time.Since(setupStarted).Milliseconds()

	progress := newProgressTracker(config.ProgressReporter, config.MaxNodes)
	seedBudget := config.policy.CoverageSeedNodeBudget
	genericStarSeed := coverage == nil && limitedMode && starPotential != nil && config.PrioritySemantics.IsOutgoing()
	if genericStarSeed && config.PrioritySemantics == model.PrioritySemanticsOutgoingPerInstanceV3 {
		seedBudget = config.policy.StarSeedNodeBudget
	}
	if progress != nil && seedBudget > 0 && ((coverage != nil && coverage.enabled) || genericStarSeed) {
		progress.emitPhase(ProgressPhaseSeed)
	}
	coverageEligible := coverage != nil && coverage.enabled && coverage.pruningEnabled && coverage.targetCount() > 0 && seedBudget > 0
	config.trace.planPhase(tracePhaseCoverageSeed, coverageEligible, phaseSkipReason(coverageEligible, "coverage_disabled"), seedBudget)
	if coverageEligible {
		config.trace.invokePhase(tracePhaseCoverageSeed)
	}
	seed := coverageSeedSearch(catalog, instances, optionsByInstance, withTracePhase(searchConfig, tracePhaseCoverageSeed), coverage, gridMask, seedBudget, progress)
	config.trace.finishPhase(tracePhaseCoverageSeed, phaseTermination(coverageEligible, "completed"), phaseUnused(seedBudget, seed.NodesExplored), "dfs")
	var packingSeed coverageSeedResult
	var objectiveStarSeed coverageSeedResult
	var constellationSeed coverageSeedResult
	var constellationDiagnostics model.ConstellationSeedDiagnostics
	var constellationCandidateSnapshot *constellationCandidateCompletionSnapshot
	if (config.ConstellationCandidateCompletionOptimizationProbe && config.diagnosticNodeBudget > 0) || config.ConstellationForcedCandidateRootedPackingProbe || config.ConstellationParentFrontierHedgeProbe {
		constellationCandidateSnapshot = &constellationCandidateCompletionSnapshot{}
		searchConfig.constellationCandidateCompletionSnapshot = constellationCandidateSnapshot
	}
	if genericStarSeed {
		if config.PrioritySemantics == model.PrioritySemanticsOutgoingPerInstanceV3 && seedBudget >= 4 {
			packingBudget := config.policy.PackingSeedNodeBudget
			packingOrder := packingSeedOrder(instances, optionsByInstance)
			config.trace.planPhase(tracePhasePackingSeed, packingBudget > 0, phaseSkipReason(packingBudget > 0, "no_budget"), packingBudget)
			if packingBudget > 0 {
				config.trace.invokePhase(tracePhasePackingSeed)
			}
			packingSeed = packingSeedSearch(catalog, instances, packingOrder, optionsByInstance, withTracePhase(searchConfig, tracePhasePackingSeed), gridMask, packingBudget, progress)
			config.trace.finishPhase(tracePhasePackingSeed, phaseTermination(packingBudget > 0, "completed"), phaseUnused(packingBudget, packingSeed.NodesExplored), "dfs")
			if config.policy.ConstellationSeedNodeBudget > 0 {
				config.trace.planPhase(tracePhaseConstellationSeed, true, "", config.policy.ConstellationSeedNodeBudget)
				config.trace.invokePhase(tracePhaseConstellationSeed)
				constellationSeed, constellationDiagnostics = constellationSeedSearch(catalog, instances, optionsByInstance, withTracePhase(searchConfig, tracePhaseConstellationSeed), gridMask, config.policy.ConstellationSeedNodeBudget, starPotential, progress)
				termination := "completed"
				if constellationDiagnostics.SkeletonsDistinct == 0 {
					termination = "no_priority_constellation"
				}
				config.trace.finishPhase(tracePhaseConstellationSeed, termination, phaseUnused(config.policy.ConstellationSeedNodeBudget, constellationSeed.NodesExplored), "star_seed")
			}
			starBudget := seedBudget - packingBudget - constellationSeed.NodesExplored
			config.trace.planPhase(tracePhaseStarSeed, starBudget > 0, phaseSkipReason(starBudget > 0, "no_budget"), starBudget)
			if starBudget > 0 {
				config.trace.invokePhase(tracePhaseStarSeed)
			}
			objectiveStarSeed = starSeedSearch(catalog, instances, ordered, optionsByInstance, withTracePhase(searchConfig, tracePhaseStarSeed), gridMask, starBudget, starPotential, progress)
			config.trace.finishPhase(tracePhaseStarSeed, phaseTermination(starBudget > 0, "completed"), phaseUnused(starBudget, objectiveStarSeed.NodesExplored), "dfs")
			seed = mergeSeedResults(packingSeed, objectiveStarSeed, searchConfig.TopN)
			seed = mergeSeedResults(seed, constellationSeed, searchConfig.TopN)
		} else {
			config.trace.planPhase(tracePhaseStarSeed, seedBudget > 0, phaseSkipReason(seedBudget > 0, "no_budget"), seedBudget)
			if seedBudget > 0 {
				config.trace.invokePhase(tracePhaseStarSeed)
			}
			objectiveStarSeed = starSeedSearch(catalog, instances, ordered, optionsByInstance, withTracePhase(searchConfig, tracePhaseStarSeed), gridMask, seedBudget, starPotential, progress)
			config.trace.finishPhase(tracePhaseStarSeed, phaseTermination(seedBudget > 0, "completed"), phaseUnused(seedBudget, objectiveStarSeed.NodesExplored), "dfs")
			seed = objectiveStarSeed
		}
	}
	seed.Solutions = mergeSolutions(knownSolutions, seed.Solutions, searchConfig.TopN)
	seedBestPriorityCounts := priorityCountsForSolutions(seed.Solutions)
	seedBestScore := bestScore(seed.Solutions)
	if config.StopOnPriorityCeiling && priorityBounds.reached(bestScore(seed.Solutions)) {
		markDownstreamPhasesStopped(config, "stopped_after_priority_ceiling")
		return finishPriorityCeilingResult(seed.Solutions, requestedTopN, limitedMode, setupMS, seed, repairResult{}, searchResult{}, priorityBounds, config)
	}
	if err := config.Context.Err(); err != nil {
		return nil, err
	}
	if progress != nil && len(seed.Solutions) > 0 {
		progress.reportIncumbent(ProgressPhaseSeed, seed.Solutions, true)
	}
	if seed.StoppedAfterCoverageCeiling {
		if progress != nil {
			progress.finish()
		}
		markDownstreamPhasesStopped(config, "stopped_after_coverage_ceiling")
		searchStats := model.SearchStats{
			NodesExplored:               seed.NodesExplored,
			SetupMS:                     setupMS,
			Limited:                     limitedMode,
			CoverageSeedNodes:           seed.NodesExplored,
			CoverageSeedCandidates:      seed.CandidateCount,
			CoverageSeedBest:            seed.BestSummary,
			StoppedAfterCoverageCeiling: true,
		}
		if genericStarSeed {
			searchStats.CoverageSeedNodes = 0
			searchStats.CoverageSeedCandidates = 0
			searchStats.CoverageSeedBest = ""
			searchStats.StarSeedNodes = objectiveStarSeed.NodesExplored
			searchStats.StarSeedCandidates = objectiveStarSeed.CandidateCount
			searchStats.PackingSeedNodes = packingSeed.NodesExplored
			searchStats.PackingSeedCandidates = packingSeed.CandidateCount
			searchStats.PackingSeedHardPruned = packingSeed.HardPrunedNodes
			searchStats.PackingSeedStatesDeduplicated = packingSeed.StatesDeduplicated
			applyConstellationSeedStats(&searchStats, constellationSeed, constellationDiagnostics)
			searchStats.SymmetryPrunedBranches = seed.SymmetryPrunedBranches
		}
		if coverage != nil {
			searchStats.CoverageSources = append([]string(nil), coverage.sourceItemIDs...)
			searchStats.CoverageTargetCount = coverage.targetCount()
			searchStats.CoverageCeiling = append([]model.StarCoverageBucket(nil), coverage.coverageCeiling...)
		}
		applyPackingSeedStats(&searchStats, seed)
		config.trace.apply(&searchStats)
		applyPlateauSearchStats(&searchStats, config)
		results := append([]model.Solution(nil), seed.Solutions...)
		for idx := range results {
			results[idx].Search = searchStats
			results[idx].Search.CoverageCeilingReached = coverage != nil && coverage.ceilingReached(results[idx].Evaluation.Score)
		}
		if len(results) > requestedTopN {
			results = results[:requestedTopN]
		}
		return results, nil
	}

	repairBudget := int64(0)
	postRepairBudget := int64(0)
	preRepairBudget := int64(0)
	var repair repairResult
	initialSolutions := seed.Solutions
	repairEligible := false
	repairSkipReason := "disabled"
	if config.RepairSearch && limitedMode && len(seed.Solutions) > 0 {
		repairBudget = repairNodeBudget(config.policy, config.MaxNodes, seed.NodesExplored)
		if repairBudget > 0 {
			preRepairBudget = repairBudget / 2
			postRepairBudget = repairBudget - preRepairBudget
			repairEligible = preRepairBudget > 0
			repairSkipReason = ""
		}
	} else if !config.RepairSearch || !limitedMode {
		repairSkipReason = "disabled"
	} else if len(seed.Solutions) == 0 {
		repairSkipReason = "no_seed_solution"
	} else {
		repairSkipReason = "no_budget"
	}
	config.trace.planPhase(tracePhasePreRepair, repairEligible, repairSkipReason, preRepairBudget)
	if repairEligible {
		config.trace.invokePhase(tracePhasePreRepair)
		if progress != nil {
			progress.emitPhase(ProgressPhaseRepair)
		}
		repair = repairSearch(catalog, instances, optionsByInstance, withTracePhase(searchConfig, tracePhasePreRepair), coverage, outgoingBounds, gridMask, preRepairBudget, seed.Solutions, progress)
		config.trace.finishPhase(tracePhasePreRepair, repair.TerminationReason, phaseUnused(preRepairBudget, repair.NodesExplored), "dfs")
		if err := config.Context.Err(); err != nil {
			return nil, err
		}
		initialSolutions = mergeSolutions(seed.Solutions, repair.Solutions, searchConfig.TopN)
		if progress != nil && len(initialSolutions) > 0 {
			progress.reportIncumbent(ProgressPhaseRepair, initialSolutions, true)
		}
		if coverage != nil {
			coverage.prepareOrder(ordered)
		}
		if searchConfig.StopOnCoverageCeiling && coverage != nil && len(initialSolutions) > 0 && coverage.ceilingReached(initialSolutions[0].Evaluation.Score) {
			if progress != nil {
				progress.finish()
			}
			config.trace.planPhase(tracePhaseDFS, false, "stopped_after_coverage_ceiling", 0)
			config.trace.planPhase(tracePhasePostRepair, false, "stopped_after_coverage_ceiling", postRepairBudget)
			config.trace.planPhase(tracePhasePlateauLNS, false, "stopped_after_coverage_ceiling", 0)
			config.trace.planPhase(tracePhasePlateauRefine, false, "stopped_after_coverage_ceiling", 0)
			searchStats := model.SearchStats{
				NodesExplored:               seed.NodesExplored + repair.NodesExplored,
				SetupMS:                     setupMS,
				Limited:                     limitedMode,
				CoverageSeedNodes:           seed.NodesExplored,
				CoverageSeedCandidates:      seed.CandidateCount,
				CoverageSeedBest:            seed.BestSummary,
				StoppedAfterCoverageCeiling: true,
			}
			// This projection is also responsible for preserving the authoritative
			// outgoing counters that predate R1I on the coverage-ceiling return.
			applyRepairResultStats(&searchStats, repair)
			if genericStarSeed {
				searchStats.CoverageSeedNodes = 0
				searchStats.CoverageSeedCandidates = 0
				searchStats.CoverageSeedBest = ""
				searchStats.StarSeedNodes = objectiveStarSeed.NodesExplored
				searchStats.StarSeedCandidates = objectiveStarSeed.CandidateCount
				searchStats.PackingSeedNodes = packingSeed.NodesExplored
				searchStats.PackingSeedCandidates = packingSeed.CandidateCount
				searchStats.PackingSeedHardPruned = packingSeed.HardPrunedNodes
				searchStats.PackingSeedStatesDeduplicated = packingSeed.StatesDeduplicated
				applyConstellationSeedStats(&searchStats, constellationSeed, constellationDiagnostics)
				searchStats.SymmetryPrunedBranches = seed.SymmetryPrunedBranches + repair.SymmetryPrunedBranches
			}
			if coverage != nil {
				searchStats.CoverageSources = append([]string(nil), coverage.sourceItemIDs...)
				searchStats.CoverageTargetCount = coverage.targetCount()
				searchStats.CoverageCeiling = append([]model.StarCoverageBucket(nil), coverage.coverageCeiling...)
			}
			applyPackingSeedStats(&searchStats, seed)
			config.trace.apply(&searchStats)
			applyPlateauSearchStats(&searchStats, config)
			results := append([]model.Solution(nil), initialSolutions...)
			for idx := range results {
				results[idx].Search = searchStats
				results[idx].Search.CoverageCeilingReached = coverage != nil && coverage.ceilingReached(results[idx].Evaluation.Score)
			}
			if len(results) > requestedTopN {
				results = results[:requestedTopN]
			}
			return results, nil
		}
	}

	backtrackConfig := searchConfig
	if backtrackConfig.MaxNodes > 0 {
		backtrackConfig.MaxNodes -= seed.NodesExplored + repair.NodesExplored + postRepairBudget
		if backtrackConfig.MaxNodes < 0 {
			backtrackConfig.MaxNodes = 0
		}
	}
	var search searchResult
	if config.MaxNodes == 0 || backtrackConfig.MaxNodes > 0 {
		config.trace.planPhase(tracePhaseDFS, true, "", backtrackConfig.MaxNodes)
		config.trace.invokePhase(tracePhaseDFS)
		if progress != nil {
			progress.emitPhase(ProgressPhaseSearch)
		}
		splitDepth := initialSplitDepth(ordered, optionsByInstance, backtrackConfig)
		tasks, splitSymmetryPruned := buildTasksWithStats(ordered, optionsByInstance, config.AllowSkips, splitDepth)
		config.taskAllocation = taskAllocationStats(tasks, splitDepth)
		assignBudgetsWithPolicy(tasks, backtrackConfig.MaxNodes, starPotential, config.PrioritySemantics, config.policy)
		config.taskAllocation = taskAllocationStats(tasks, splitDepth)
		search = runTasks(catalog, instances, ordered, optionsByInstance, tasks, withTracePhase(backtrackConfig, tracePhaseDFS), gridMask, coverage, exactBounds, outgoingBounds, priorityBounds, initialSolutions, progress)
		config.taskAllocation.TasksExecuted = search.TasksExecuted
		config.taskAllocation.TasksPrunedBeforeExecution = search.TasksPrunedBeforeExecution
		search.TaskAllocation = config.taskAllocation
		search.SymmetryPrunedBranches += splitSymmetryPruned
		termination := "completed"
		if search.HitNodeBudget {
			termination = "budget_exhausted"
		} else if search.StoppedAfterPriorityCeiling {
			termination = "stopped_after_priority_ceiling"
		} else if search.StoppedAfterCoverageCeiling {
			termination = "stopped_after_coverage_ceiling"
		}
		config.trace.finishPhase(tracePhaseDFS, termination, phaseUnused(backtrackConfig.MaxNodes, search.NodesExplored), "stage_headroom")
		if err := config.Context.Err(); err != nil {
			return nil, err
		}
	} else {
		config.trace.planPhase(tracePhaseDFS, false, "no_budget", 0)
	}
	results := mergeSolutions(initialSolutions, search.Solutions, searchConfig.TopN)
	searchBestPriorityCounts := priorityCountsForSolutions(results)
	searchBestScore := bestScore(results)
	if config.StopOnPriorityCeiling && priorityBounds.reached(bestScore(results)) {
		markDownstreamPhasesStopped(config, "stopped_after_priority_ceiling")
		return finishPriorityCeilingResult(results, requestedTopN, limitedMode, setupMS, seed, repair, search, priorityBounds, config)
	}
	postRepairEligible := postRepairBudget > 0 && len(results) > 0
	postRepairSkipReason := phaseSkipReason(postRepairEligible, "no_results")
	if postRepairBudget <= 0 {
		postRepairSkipReason = "no_budget"
	}
	config.trace.planPhase(tracePhasePostRepair, postRepairEligible, postRepairSkipReason, postRepairBudget)
	if postRepairBudget > 0 && len(results) > 0 {
		config.trace.invokePhase(tracePhasePostRepair)
		if progress != nil {
			progress.emitPhase(ProgressPhaseRepair)
		}
		postRepair := repairSearch(catalog, instances, optionsByInstance, withTracePhase(searchConfig, tracePhasePostRepair), coverage, outgoingBounds, gridMask, postRepairBudget, results, progress)
		config.trace.finishPhase(tracePhasePostRepair, postRepair.TerminationReason, phaseUnused(postRepairBudget, postRepair.NodesExplored), "stage_headroom")
		if err := config.Context.Err(); err != nil {
			return nil, err
		}
		repair = mergeRepairPhases(repair, postRepair, searchConfig.TopN)
		results = mergeSolutions(results, postRepair.Solutions, searchConfig.TopN)
	}
	postRepairBestPriorityCounts := priorityCountsForSolutions(results)
	postRepairBestScore := bestScore(results)
	var plateauLNS repairResult
	if config.MaxNodes > 0 && len(results) > 0 && priorityBounds.reached(results[0].Evaluation.Score) {
		usedNodes := seed.NodesExplored + search.NodesExplored + repair.NodesExplored
		remainingNodes := config.MaxNodes - usedNodes
		lnsBudget := config.MaxNodes * config.policy.PlateauLNSBudgetPercent / 100
		if lnsBudget > remainingNodes {
			lnsBudget = remainingNodes
		}
		if lnsBudget > 0 {
			config.trace.planPhase(tracePhasePlateauLNS, true, "", lnsBudget)
			config.trace.invokePhase(tracePhasePlateauLNS)
			plateauLNS = plateauTieBreakLNS(catalog, instances, optionsByInstance, results[0], searchConfig, coverage, gridMask, lnsBudget)
			termination := "completed"
			if plateauLNS.NodesExplored == 0 {
				termination = "no_eligible_neighborhoods"
			} else if plateauLNS.Improvements > 0 {
				termination = "improved_level"
			}
			config.trace.finishPhase(tracePhasePlateauLNS, termination, phaseUnused(lnsBudget, plateauLNS.NodesExplored), "stage_headroom")
			if len(plateauLNS.Solutions) > 0 && compareScores(plateauLNS.Solutions[0].Evaluation.Score, results[0].Evaluation.Score) > 0 {
				results = mergeSolutions(results, plateauLNS.Solutions, searchConfig.TopN)
			}
		} else {
			config.trace.planPhase(tracePhasePlateauLNS, false, "no_budget", 0)
		}
	} else {
		config.trace.planPhase(tracePhasePlateauLNS, false, "priority_not_reached", 0)
	}
	var plateauRefine plateauRefineStats
	if config.MaxNodes > 0 && len(results) > 0 && priorityBounds.reached(results[0].Evaluation.Score) {
		usedNodes := seed.NodesExplored + search.NodesExplored + repair.NodesExplored + plateauLNS.NodesExplored
		remainingNodes := config.MaxNodes - usedNodes
		refineBudget := config.MaxNodes * config.policy.PlateauRefineBudgetPercent / 100
		if refineBudget > remainingNodes {
			refineBudget = remainingNodes
		}
		if refineBudget > 0 {
			config.trace.planPhase(tracePhasePlateauRefine, true, "", refineBudget)
			config.trace.invokePhase(tracePhasePlateauRefine)
			candidate, stats, err := refinePlateau(catalog, instances, optionsByInstance, results[0], searchConfig, starBounds, refineBudget)
			if err != nil {
				return nil, err
			}
			plateauRefine = stats
			termination := "completed"
			if plateauRefine.NodesExplored == 0 {
				termination = "queue_exhausted"
			} else if plateauRefine.Improved {
				termination = "improved_level"
			}
			config.trace.finishPhase(tracePhasePlateauRefine, termination, phaseUnused(refineBudget, plateauRefine.NodesExplored), "stage_headroom")
			if compareScores(candidate.Evaluation.Score, results[0].Evaluation.Score) > 0 {
				results = mergeSolutions(results, []model.Solution{candidate}, searchConfig.TopN)
			}
		} else {
			config.trace.planPhase(tracePhasePlateauRefine, false, "no_budget", 0)
		}
	} else {
		config.trace.planPhase(tracePhasePlateauRefine, false, "priority_not_reached", 0)
	}
	searchStats := model.SearchStats{
		NodesExplored:                seed.NodesExplored + search.NodesExplored + plateauLNS.NodesExplored + plateauRefine.NodesExplored,
		SetupMS:                      setupMS,
		Limited:                      limitedMode,
		CoverageBoundChecks:          repair.CoverageBoundChecks + search.CoverageBoundChecks,
		CoveragePrunedNodes:          repair.CoveragePrunedNodes + search.CoveragePrunedNodes,
		ExactBoundChecks:             repair.ExactBoundChecks + search.ExactBoundChecks,
		ExactBoundPrunedNodes:        repair.ExactBoundPrunedNodes + search.ExactBoundPrunedNodes,
		OutgoingBoundChecks:          repair.OutgoingBoundChecks + search.OutgoingBoundChecks,
		OutgoingBoundPrunedNodes:     repair.OutgoingBoundPrunedNodes + search.OutgoingBoundPrunedNodes,
		CoverageSeedNodes:            seed.NodesExplored,
		CoverageSeedCandidates:       seed.CandidateCount,
		CoverageSeedBest:             seed.BestSummary,
		InitialBestPriorityCounts:    initialBestPriorityCounts,
		SeedBestPriorityCounts:       seedBestPriorityCounts,
		SearchBestPriorityCounts:     searchBestPriorityCounts,
		PostRepairBestPriorityCounts: postRepairBestPriorityCounts,
		ParallelTasks:                search.ParallelTasks,
		ParallelWorkersUsed:          search.ParallelWorkersUsed,
		RepairNodes:                  repair.NodesExplored,
		RepairIterations:             repair.Iterations,
		RepairImprovements:           repair.Improvements,
		RepairCandidates:             repair.CandidateCount,
		RepairBest:                   repair.BestSummary,
		RepairParallelTasks:          repair.ParallelTasks,
		RepairParallelWorkersUsed:    repair.ParallelWorkersUsed,
		PlateauLNSNodes:              plateauLNS.NodesExplored,
		PlateauRefineNodes:           plateauRefine.NodesExplored,
		PlateauRefineWalkLength:      plateauRefine.MaxDepth,
		PlateauRefineMaxValley:       plateauRefine.MaxValley,
		PlateauRefineImproved:        plateauRefine.Improved,
		StoppedAfterCoverageCeiling:  seed.StoppedAfterCoverageCeiling || search.StoppedAfterCoverageCeiling,
		StoppedAfterPriorityCeiling:  search.StoppedAfterPriorityCeiling,
		SeedBestScore:                seedBestScore,
		SearchBestScore:              searchBestScore,
		PostRepairBestScore:          postRepairBestScore,
		TaskAllocation:               search.TaskAllocation,
		BoundOperationProfile: mergeBoundAttributionOperationProfiles(
			mergeBoundAttributionOperationProfiles(
				mergeBoundAttributionOperationProfiles(nil, repair.BoundOperationProfile),
				search.BoundOperationProfile,
			),
			plateauLNS.BoundOperationProfile,
		),
	}
	if genericStarSeed {
		searchStats.CoverageSeedNodes = 0
		searchStats.CoverageSeedCandidates = 0
		searchStats.CoverageSeedBest = ""
		searchStats.StarSeedNodes = objectiveStarSeed.NodesExplored
		searchStats.StarSeedCandidates = objectiveStarSeed.CandidateCount
		searchStats.PackingSeedNodes = packingSeed.NodesExplored
		searchStats.PackingSeedCandidates = packingSeed.CandidateCount
		searchStats.PackingSeedHardPruned = packingSeed.HardPrunedNodes
		searchStats.PackingSeedStatesDeduplicated = packingSeed.StatesDeduplicated
		applyConstellationSeedStats(&searchStats, constellationSeed, constellationDiagnostics)
	}
	applyPackingSeedStats(&searchStats, seed)
	searchStats.SymmetryPrunedBranches = seed.SymmetryPrunedBranches + repair.SymmetryPrunedBranches + search.SymmetryPrunedBranches
	searchStats.NodesExplored += repair.NodesExplored
	if coverage != nil {
		searchStats.CoverageSources = append([]string(nil), coverage.sourceItemIDs...)
		searchStats.CoverageTargetCount = coverage.targetCount()
		searchStats.CoverageCeiling = append([]model.StarCoverageBucket(nil), coverage.coverageCeiling...)
	}
	for idx := range results {
		results[idx].Search = searchStats
	}
	var completionErr error
	completionEligible := config.AllowSkips && len(results) > 0
	completionSkipReason := phaseSkipReason(completionEligible, "skips_disabled")
	if !config.AllowSkips {
		completionSkipReason = "skips_disabled"
	} else if len(results) == 0 {
		completionSkipReason = "no_results"
	}
	config.trace.planPhase(tracePhaseCompletion, completionEligible, completionSkipReason, 0)
	if completionEligible {
		config.trace.invokePhase(tracePhaseCompletion)
	}
	results, searchStats, completionErr = completeSkippedSolutions(catalog, instances, optionsByInstance, results, searchStats, withTracePhase(config, tracePhaseCompletion))
	if completionEligible {
		config.trace.finishPhase(tracePhaseCompletion, "completed", 0, "")
	}
	if completionErr != nil {
		return nil, completionErr
	}
	sort.Slice(results, func(i, j int) bool {
		return SolutionLess(results[i], results[j])
	})
	if progress != nil {
		progress.emitPhase(ProgressPhaseRefine)
	}
	refineEligible := len(results) > 0
	config.trace.planPhase(tracePhaseRefine, refineEligible, phaseSkipReason(refineEligible, "no_results"), 0)
	if refineEligible {
		config.trace.invokePhase(tracePhaseRefine)
	}
	if err := config.Context.Err(); err != nil {
		return nil, err
	}
	var beforeRefineBest *model.Score
	if len(results) > 0 {
		score := results[0].Evaluation.Score
		beforeRefineBest = &score
	}
	var refineErr error
	results, searchStats, refineErr = refineSolutions(catalog, instances, optionsByInstance, results, searchStats, withTracePhase(config, tracePhaseRefine))
	if refineEligible {
		termination := "completed"
		if searchStats.RefineMovesChecked >= config.MaxRefineMoves && config.MaxRefineMoves > 0 {
			termination = "move_limit"
		}
		config.trace.finishPhase(tracePhaseRefine, termination, 0, "")
	}
	if refineErr != nil {
		return nil, refineErr
	}
	sort.Slice(results, func(i, j int) bool {
		return SolutionLess(results[i], results[j])
	})
	results = uniqueSolutions(results)
	searchStats.RefineBestPriorityCounts = priorityCountsForSolutions(results)
	searchStats.RefineBestScore = bestScore(results)
	if beforeRefineBest != nil && len(results) > 0 && compareScores(results[0].Evaluation.Score, *beforeRefineBest) > 0 {
		searchStats.RefineBestDelta = scoreDeltaSummary(*beforeRefineBest, results[0].Evaluation.Score)
	}
	if len(results) > requestedTopN {
		results = results[:requestedTopN]
	}
	coverageCeilingReached := false
	if coverage != nil && len(results) > 0 {
		coverageCeilingReached = coverage.ceilingReached(results[0].Evaluation.Score)
	}
	searchStats.CoverageCeilingReached = coverageCeilingReached
	if priorityBounds != nil {
		searchStats.PriorityCeiling = append([]int(nil), priorityBounds.ceiling...)
		searchStats.PriorityCeilingReached = priorityBounds.reached(bestScore(results))
	}
	config.trace.apply(&searchStats)
	applyPlateauSearchStats(&searchStats, config)
	if constellationCandidateSnapshot != nil && config.ConstellationCandidateCompletionOptimizationProbe && config.diagnosticNodeBudget > 0 {
		diagnosticOptimization := constellationCandidateCompletionOptimization(catalog, instances, optionsByInstance, constellationCandidateSnapshot.candidates, constellationCandidateSnapshot.selectedRoots, config, gridMask, config.diagnosticNodeBudget, func() (bool, string) {
			return chargeDiagnosticNodeWithReason(config)
		})
		var diagnosticNodes int64
		for _, attempt := range diagnosticOptimization.Attempts {
			diagnosticNodes += attempt.NodesConsumed
		}
		constellationDiagnostics.CandidateCompletionOptimization = &diagnosticOptimization
		constellationDiagnostics.CandidateCompletionOptimizationNodes = diagnosticNodes
		searchStats.ConstellationSeedDiagnostics = constellationDiagnostics
		searchStats.DiagnosticBudgetConfigured = config.diagnosticNodeBudget
		searchStats.DiagnosticBudgetConsumed = diagnosticNodes
		searchStats.ExecutionBudgetConfigured = config.MaxNodes + config.diagnosticNodeBudget
		searchStats.ExecutionBudgetConsumed = searchStats.GlobalBudgetConsumed + diagnosticNodes
		searchStats.PhaseWork = append(searchStats.PhaseWork, model.SearchPhaseWork{
			Phase:             tracePhaseConstellationCandidateOptimization,
			BudgetScope:       "diagnostic",
			ChargedNodes:      diagnosticNodes,
			Eligible:          true,
			Invoked:           true,
			TerminationReason: candidateOptimizationTermination(diagnosticOptimization),
			NodesReserved:     config.diagnosticNodeBudget,
			NodesConsumed:     diagnosticNodes,
			NodesReturned:     config.diagnosticNodeBudget - diagnosticNodes,
		})
	}
	if constellationCandidateSnapshot != nil && config.ConstellationForcedCandidateRootedPackingProbe {
		forced := constellationForcedCandidateRootedPacking(catalog, instances, optionsByInstance, constellationCandidateSnapshot.candidates, constellationCandidateSnapshot.selectedRoots, config, gridMask)
		var diagnosticNodes int64
		for _, attempt := range forced.Attempts {
			diagnosticNodes += attempt.NodesConsumed
		}
		constellationDiagnostics.ForcedCandidateRootedPacking = &forced
		constellationDiagnostics.ForcedCandidateRootedPackingNodes = diagnosticNodes
		searchStats.ConstellationSeedDiagnostics = constellationDiagnostics
		searchStats.DiagnosticBudgetConfigured = config.ledger.diagnosticMaxValue()
		searchStats.DiagnosticBudgetConsumed = diagnosticNodes
		searchStats.ExecutionBudgetConfigured = config.MaxNodes + searchStats.DiagnosticBudgetConfigured
		searchStats.ExecutionBudgetConsumed = searchStats.GlobalBudgetConsumed + diagnosticNodes
		searchStats.PhaseWork = append(searchStats.PhaseWork, model.SearchPhaseWork{
			Phase:             tracePhaseConstellationForcedRootedPacking,
			BudgetScope:       "diagnostic",
			ChargedNodes:      diagnosticNodes,
			Eligible:          true,
			Invoked:           true,
			TerminationReason: forcedRootedPackingTermination(forced),
			NodesReserved:     searchStats.DiagnosticBudgetConfigured,
			NodesConsumed:     diagnosticNodes,
			NodesReturned:     searchStats.DiagnosticBudgetConfigured - diagnosticNodes,
		})
	}
	if constellationCandidateSnapshot != nil && config.ConstellationParentFrontierHedgeProbe {
		hedge := constellationParentFrontierHedge(catalog, instances, optionsByInstance, constellationCandidateSnapshot, config, gridMask)
		var diagnosticNodes int64
		for _, attempt := range hedge.Attempts {
			diagnosticNodes += attempt.FamilyConsumed
		}
		constellationDiagnostics.ParentFrontierHedge = &hedge
		constellationDiagnostics.ParentFrontierHedgeNodes = diagnosticNodes
		searchStats.ConstellationSeedDiagnostics = constellationDiagnostics
		searchStats.DiagnosticBudgetConfigured = config.ledger.diagnosticMaxValue()
		searchStats.DiagnosticBudgetConsumed = diagnosticNodes
		searchStats.ExecutionBudgetConfigured = config.MaxNodes + searchStats.DiagnosticBudgetConfigured
		searchStats.ExecutionBudgetConsumed = searchStats.GlobalBudgetConsumed + diagnosticNodes
		searchStats.PhaseWork = append(searchStats.PhaseWork, model.SearchPhaseWork{
			Phase:             tracePhaseConstellationParentFrontierHedge,
			BudgetScope:       "diagnostic",
			ChargedNodes:      diagnosticNodes,
			Eligible:          true,
			Invoked:           true,
			TerminationReason: parentFrontierHedgeTermination(hedge),
			NodesReserved:     searchStats.DiagnosticBudgetConfigured,
			NodesConsumed:     diagnosticNodes,
			NodesReturned:     searchStats.DiagnosticBudgetConfigured - diagnosticNodes,
		})
	}
	for idx := range results {
		results[idx].Search = searchStats
	}
	if progress != nil {
		progress.finish()
	}
	return results, nil
}

// EvaluateKnownLayout validates and scores a complete supplied layout without
// running seed, repair, or backtracking search.
func EvaluateKnownLayout(catalog model.Catalog, itemIDs []string, gridMask uint64, config Config) (model.Solution, error) {
	if len(itemIDs) == 0 {
		return model.Solution{}, fmt.Errorf("inventory cannot be empty")
	}
	if len(itemIDs) > geometry.GridCells {
		return model.Solution{}, fmt.Errorf("inventory has %d items; maximum is %d for the %dx%d grid", len(itemIDs), geometry.GridCells, geometry.GridRows, geometry.GridCols)
	}
	instances := ExpandInventory(itemIDs)
	for _, instance := range instances {
		if _, ok := catalog.Items[instance.ItemID]; !ok {
			return model.Solution{}, fmt.Errorf("inventory references unknown item %q", instance.ItemID)
		}
	}
	catalog = filterInventoryImpossibleRecipes(catalog, itemIDs)
	optionsByInstance := make(map[string][]model.Placement, len(instances))
	for _, instance := range instances {
		options, err := PlacementOptions(catalog, instance, gridMask)
		if err != nil {
			return model.Solution{}, err
		}
		optionsByInstance[instance.InstanceID] = options
	}
	config.AllowSkips = false
	solutions, err := initialSolutionsForConfig(catalog, instances, optionsByInstance, config)
	if err != nil {
		return model.Solution{}, err
	}
	if len(solutions) != 1 {
		return model.Solution{}, fmt.Errorf("known layout did not produce a solution")
	}
	return solutions[0], nil
}

func bestScore(solutions []model.Solution) model.Score {
	if len(solutions) == 0 {
		return model.Score{}
	}
	return solutions[0].Evaluation.Score
}

func applyPackingSeedStats(stats *model.SearchStats, seed coverageSeedResult) {
	if stats == nil {
		return
	}
	if seed.PackingDiagnostics.MaxDepth > 0 {
		stats.PackingSeedDiagnostics = seed.PackingDiagnostics
	}
	stats.PackingSeedOperationProfile = seed.PackingSeedOperationProfile
	stats.BoundOperationProfile = mergeBoundAttributionOperationProfiles(stats.BoundOperationProfile, seed.BoundOperationProfile)
}

func initializeDiagnosticPhasePlans(config Config) {
	if config.trace == nil {
		return
	}
	config.trace.planPhase(tracePhaseCoverageSeed, false, "disabled", 0)
	config.trace.planPhase(tracePhasePackingSeed, false, "not_selected", 0)
	config.trace.planPhase(tracePhaseStarSeed, false, "not_selected", 0)
	config.trace.planPhase(tracePhaseConstellationSeed, false, "disabled", 0)
	config.trace.planPhase(tracePhasePreRepair, false, "disabled", 0)
	config.trace.planPhase(tracePhaseDFS, false, "not_selected", 0)
	config.trace.planPhase(tracePhasePostRepair, false, "not_selected", 0)
	config.trace.planPhase(tracePhaseCompletion, false, "skips_disabled", 0)
	config.trace.planPhase(tracePhaseRefine, false, "no_results", 0)
	config.trace.planPhase(tracePhasePlateauLNS, false, "priority_not_reached", 0)
	config.trace.planPhase(tracePhasePlateauRefine, false, "priority_not_reached", 0)
}

func phaseUnused(nodesReserved int64, nodesConsumed int64) int64 {
	if nodesReserved <= nodesConsumed {
		return 0
	}
	return nodesReserved - nodesConsumed
}

func phaseSkipReason(eligible bool, reason string) string {
	if eligible {
		return ""
	}
	return reason
}

func phaseTermination(eligible bool, reason string) string {
	if !eligible {
		return ""
	}
	return reason
}

func markDownstreamPhasesStopped(config Config, reason string) {
	if config.trace == nil {
		return
	}
	for _, phase := range []string{tracePhasePreRepair, tracePhaseDFS, tracePhasePostRepair, tracePhasePlateauLNS, tracePhasePlateauRefine, tracePhaseCompletion, tracePhaseRefine} {
		config.trace.planPhase(phase, false, reason, 0)
	}
}

func applyConstellationSeedStats(stats *model.SearchStats, seed coverageSeedResult, diagnostics model.ConstellationSeedDiagnostics) {
	if stats == nil || diagnostics.Version == "" {
		return
	}
	stats.ConstellationSeedNodes = seed.NodesExplored
	stats.ConstellationSeedCandidates = seed.CandidateCount
	stats.ConstellationSeedDiagnostics = diagnostics
}

func mergeConstellationSeedDiagnostics(left model.ConstellationSeedDiagnostics, right model.ConstellationSeedDiagnostics) model.ConstellationSeedDiagnostics {
	if left.Version == "" {
		return right
	}
	if right.Version == "" {
		return left
	}
	left.ConstructionNodes += right.ConstructionNodes
	left.PackingNodes += right.PackingNodes
	left.StatesGenerated += right.StatesGenerated
	left.StatesDeduplicated += right.StatesDeduplicated
	left.SourceStatesRetained += right.SourceStatesRetained
	left.TargetInstancesConsidered += right.TargetInstancesConsidered
	left.TargetStatesRetained += right.TargetStatesRetained
	left.SkeletonsReached += right.SkeletonsReached
	left.SkeletonsDistinct += right.SkeletonsDistinct
	left.RootsCompleted += right.RootsCompleted
	left.PriorityConstellations += right.PriorityConstellations
	left.PrioritySourceGeometryCount += right.PrioritySourceGeometryCount
	left.CandidatePrioritySourceGeometryCount += right.CandidatePrioritySourceGeometryCount
	left.CandidatePrioritySourceGeometryOrbitCount += right.CandidatePrioritySourceGeometryOrbitCount
	left.SelectedPrioritySourceGeometryCount += right.SelectedPrioritySourceGeometryCount
	left.SelectedPrioritySourceGeometryOrbitCount += right.SelectedPrioritySourceGeometryOrbitCount
	left.CandidateRootFreeMaskCount += right.CandidateRootFreeMaskCount
	left.CandidateRootFreeMaskOrbitCount += right.CandidateRootFreeMaskOrbitCount
	left.PoolSweepNodes += right.PoolSweepNodes
	left.CandidateCompletionOptimizationNodes += right.CandidateCompletionOptimizationNodes
	left.ForcedCandidateRootedPackingNodes += right.ForcedCandidateRootedPackingNodes
	left.ParentFrontierHedgeNodes += right.ParentFrontierHedgeNodes
	left.PriorityTargetAssignmentCount += right.PriorityTargetAssignmentCount
	left.RootFreeMaskCount += right.RootFreeMaskCount
	left.CandidatePoolFeasibilitySweep = mergeConstellationCandidatePoolFeasibilitySweep(left.CandidatePoolFeasibilitySweep, right.CandidatePoolFeasibilitySweep)
	left.CandidateCompletionOptimization = mergeConstellationCandidateCompletionOptimization(left.CandidateCompletionOptimization, right.CandidateCompletionOptimization)
	left.ForcedCandidateRootedPacking = mergeConstellationForcedCandidateRootedPacking(left.ForcedCandidateRootedPacking, right.ForcedCandidateRootedPacking)
	left.ParentFrontierHedge = mergeConstellationParentFrontierHedge(left.ParentFrontierHedge, right.ParentFrontierHedge)
	left.Skeletons = append(left.Skeletons, right.Skeletons...)
	left.Roots = append(left.Roots, right.Roots...)
	left.RootPackingOperationProfile = aggregateRootPackingOperationProfiles(left.Roots)
	return left
}

func mergeConstellationForcedCandidateRootedPacking(left *model.ConstellationForcedCandidateRootedPacking, right *model.ConstellationForcedCandidateRootedPacking) *model.ConstellationForcedCandidateRootedPacking {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	merged := *left
	if merged.RequestedCandidateID != right.RequestedCandidateID {
		merged.RequestedCandidateID = "multiple"
	}
	if merged.RequestedStageID != right.RequestedStageID {
		merged.RequestedStageID = "multiple"
	}
	if merged.RequestedSlot != right.RequestedSlot {
		merged.RequestedSlot = 0
	}
	merged.Attempts = append(append([]model.ConstellationForcedCandidateRootedPackingAttempt(nil), left.Attempts...), right.Attempts...)
	return &merged
}

func mergeConstellationParentFrontierHedge(left *model.ConstellationParentFrontierHedge, right *model.ConstellationParentFrontierHedge) *model.ConstellationParentFrontierHedge {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	merged := *left
	if merged.RequestedStageID != right.RequestedStageID {
		merged.RequestedStageID = "multiple"
	}
	if merged.HighParentConsumptionProbeThresholdBps != right.HighParentConsumptionProbeThresholdBps {
		merged.HighParentConsumptionProbeThresholdBps = 0
	}
	merged.Attempts = append(append([]model.ConstellationParentFrontierHedgeAttempt(nil), left.Attempts...), right.Attempts...)
	return &merged
}

func mergeConstellationCandidateCompletionOptimization(left *model.ConstellationCandidateCompletionOptimization, right *model.ConstellationCandidateCompletionOptimization) *model.ConstellationCandidateCompletionOptimization {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	merged := *left
	if merged.RequestedCandidateID != right.RequestedCandidateID {
		merged.RequestedCandidateID = "multiple"
	}
	if merged.RequestedStageID != right.RequestedStageID {
		merged.RequestedStageID = "multiple"
	}
	merged.Attempts = append(append([]model.ConstellationCandidateCompletionAttempt(nil), left.Attempts...), right.Attempts...)
	return &merged
}

func mergeConstellationCandidatePoolFeasibilitySweep(left *model.ConstellationCandidatePoolFeasibilitySweep, right *model.ConstellationCandidatePoolFeasibilitySweep) *model.ConstellationCandidatePoolFeasibilitySweep {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	merged := *left
	merged.StageID = "multiple"
	merged.CandidateCount += right.CandidateCount
	merged.SelectedRootCount += right.SelectedRootCount
	merged.NodesAvailable += right.NodesAvailable
	merged.NodesConsumed += right.NodesConsumed
	merged.NodesReturned += right.NodesReturned
	merged.FeasibleCount += right.FeasibleCount
	merged.InfeasibleProvenCount += right.InfeasibleProvenCount
	merged.UnknownBudgetCount += right.UnknownBudgetCount
	merged.Candidates = append(append([]model.ConstellationCandidateFeasibilityRecord(nil), left.Candidates...), right.Candidates...)
	merged.Orbits = append(append([]model.ConstellationCandidateFeasibilityOrbit(nil), left.Orbits...), right.Orbits...)
	return &merged
}

func priorityCountsForSolutions(solutions []model.Solution) []int {
	if len(solutions) == 0 {
		return nil
	}
	return append([]int(nil), solutions[0].Evaluation.Score.PriorityCounts...)
}

func applyRepairResultStats(stats *model.SearchStats, repair repairResult) {
	if stats == nil {
		return
	}
	stats.CoverageBoundChecks += repair.CoverageBoundChecks
	stats.CoveragePrunedNodes += repair.CoveragePrunedNodes
	stats.ExactBoundChecks += repair.ExactBoundChecks
	stats.ExactBoundPrunedNodes += repair.ExactBoundPrunedNodes
	stats.OutgoingBoundChecks += repair.OutgoingBoundChecks
	stats.OutgoingBoundPrunedNodes += repair.OutgoingBoundPrunedNodes
	stats.RepairNodes += repair.NodesExplored
	stats.RepairIterations += repair.Iterations
	stats.RepairImprovements += repair.Improvements
	stats.RepairCandidates += repair.CandidateCount
	stats.RepairBest = repair.BestSummary
	stats.RepairParallelTasks += repair.ParallelTasks
	stats.RepairParallelWorkersUsed = repair.ParallelWorkersUsed
	stats.BoundOperationProfile = mergeBoundAttributionOperationProfiles(stats.BoundOperationProfile, repair.BoundOperationProfile)
}

func applySearchResultStats(stats *model.SearchStats, search searchResult) {
	if stats == nil {
		return
	}
	stats.CoverageBoundChecks += search.CoverageBoundChecks
	stats.CoveragePrunedNodes += search.CoveragePrunedNodes
	stats.ExactBoundChecks += search.ExactBoundChecks
	stats.ExactBoundPrunedNodes += search.ExactBoundPrunedNodes
	stats.OutgoingBoundChecks += search.OutgoingBoundChecks
	stats.OutgoingBoundPrunedNodes += search.OutgoingBoundPrunedNodes
	stats.ParallelTasks += search.ParallelTasks
	stats.ParallelWorkersUsed = search.ParallelWorkersUsed
	stats.StoppedAfterCoverageCeiling = stats.StoppedAfterCoverageCeiling || search.StoppedAfterCoverageCeiling
	stats.StoppedAfterPriorityCeiling = stats.StoppedAfterPriorityCeiling || search.StoppedAfterPriorityCeiling
	stats.TaskAllocation = search.TaskAllocation
	stats.BoundOperationProfile = mergeBoundAttributionOperationProfiles(stats.BoundOperationProfile, search.BoundOperationProfile)
}

func finishPriorityCeilingResult(
	solutions []model.Solution,
	requestedTopN int,
	limitedMode bool,
	setupMS int64,
	seed coverageSeedResult,
	repair repairResult,
	search searchResult,
	priorityBounds *priorityBoundContext,
	config Config,
) ([]model.Solution, error) {
	results := append([]model.Solution(nil), solutions...)
	if len(results) > requestedTopN {
		results = results[:requestedTopN]
	}
	stats := model.SearchStats{
		NodesExplored:               seed.NodesExplored + repair.NodesExplored + search.NodesExplored,
		SetupMS:                     setupMS,
		Limited:                     limitedMode,
		CoverageSeedNodes:           seed.NodesExplored,
		CoverageSeedCandidates:      seed.CandidateCount,
		CoverageSeedBest:            seed.BestSummary,
		SymmetryPrunedBranches:      seed.SymmetryPrunedBranches + repair.SymmetryPrunedBranches + search.SymmetryPrunedBranches,
		StoppedAfterPriorityCeiling: priorityBounds != nil,
	}
	if priorityBounds != nil {
		stats.PriorityCeiling = append([]int(nil), priorityBounds.ceiling...)
		stats.PriorityCeilingReached = priorityBounds.reached(bestScore(results))
	}
	applyPackingSeedStats(&stats, seed)
	applyRepairResultStats(&stats, repair)
	applySearchResultStats(&stats, search)
	config.trace.apply(&stats)
	applyPlateauSearchStats(&stats, config)
	for index := range results {
		results[index].Search = stats
	}
	return results, nil
}

func buildTasks(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	allowSkips bool,
	splitDepth int,
) []searchTask {
	tasks, _ := buildTasksWithStats(ordered, optionsByInstance, allowSkips, splitDepth)
	return tasks
}

func buildTasksWithStats(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	allowSkips bool,
	splitDepth int,
) ([]searchTask, int64) {
	if len(ordered) < splitDepth {
		splitDepth = len(ordered)
	}

	var tasks []searchTask
	var symmetryPruned int64
	var visit func(index int, occupied uint64, placements []model.Placement)
	visit = func(index int, occupied uint64, placements []model.Placement) {
		if index == len(ordered) || index >= splitDepth {
			tasks = append(tasks, searchTask{
				Index:      index,
				Occupied:   occupied,
				Placements: append([]model.Placement(nil), placements...),
			})
			return
		}

		instance := ordered[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			if !placementRespectsCanonicalCopyOrder(option, placements) {
				symmetryPruned++
				continue
			}
			var insertedAt int
			placements, insertedAt = insertPlacementSorted(placements, option)
			visit(index+1, occupied|option.Mask, placements)
			placements = removePlacementAt(placements, insertedAt)
		}

		if allowSkips {
			visit(index+1, occupied, placements)
		}
	}
	visit(0, 0, nil)
	return tasks, symmetryPruned
}

func packingSeedOrder(instances []model.InventoryInstance, optionsByInstance map[string][]model.Placement) []model.InventoryInstance {
	ordered := append([]model.InventoryInstance(nil), instances...)
	sort.Slice(ordered, func(i, j int) bool {
		leftOptions := len(optionsByInstance[ordered[i].InstanceID])
		rightOptions := len(optionsByInstance[ordered[j].InstanceID])
		if leftOptions != rightOptions {
			return leftOptions < rightOptions
		}
		leftArea := 0
		for _, option := range optionsByInstance[ordered[i].InstanceID] {
			leftArea = len(option.Cells)
			break
		}
		rightArea := 0
		for _, option := range optionsByInstance[ordered[j].InstanceID] {
			rightArea = len(option.Cells)
			break
		}
		if leftArea != rightArea {
			return leftArea > rightArea
		}
		return ordered[i].OriginalIndex < ordered[j].OriginalIndex
	})
	return ordered
}

func mergeSeedResults(left coverageSeedResult, right coverageSeedResult, topN int) coverageSeedResult {
	merged := coverageSeedResult{
		Solutions:              mergeSolutions(left.Solutions, right.Solutions, topN),
		NodesExplored:          left.NodesExplored + right.NodesExplored,
		CandidateCount:         left.CandidateCount + right.CandidateCount,
		SymmetryPrunedBranches: left.SymmetryPrunedBranches + right.SymmetryPrunedBranches,
		StatesDeduplicated:     left.StatesDeduplicated + right.StatesDeduplicated,
		HardPrunedNodes:        left.HardPrunedNodes + right.HardPrunedNodes,
	}
	merged.PackingSeedOperationProfile = mergePackingSeedFeasibilityOperationProfiles(nil, left.PackingSeedOperationProfile)
	merged.PackingSeedOperationProfile = mergePackingSeedFeasibilityOperationProfiles(merged.PackingSeedOperationProfile, right.PackingSeedOperationProfile)
	merged.BoundOperationProfile = mergeBoundAttributionOperationProfiles(nil, left.BoundOperationProfile)
	merged.BoundOperationProfile = mergeBoundAttributionOperationProfiles(merged.BoundOperationProfile, right.BoundOperationProfile)
	if left.PackingDiagnostics.MaxDepth > 0 {
		merged.PackingDiagnostics = left.PackingDiagnostics
	} else {
		merged.PackingDiagnostics = right.PackingDiagnostics
	}
	return merged
}

func initialSplitDepth(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
) int {
	policy := policyForConfig(config)
	maxDepth := policy.MaxInitialSplitDepth
	if len(ordered) < maxDepth {
		maxDepth = len(ordered)
	}
	if config.MaxNodes == 0 {
		return maxDepth
	}
	maxTasks := config.Workers * policy.MaxTasksPerWorker
	if maxTasks < 1 {
		maxTasks = 1
	}
	if policy.MinAllocatedNodesPerTask > 0 {
		byBudget := int(config.MaxNodes / policy.MinAllocatedNodesPerTask)
		if byBudget < maxTasks {
			maxTasks = byBudget
		}
	}
	if maxTasks < 1 {
		return 0
	}
	bestDepth := 0
	for depth := 1; depth <= maxDepth; depth++ {
		taskCount := estimateTaskCount(ordered, optionsByInstance, config.AllowSkips, depth)
		if taskCount == 0 || taskCount > maxTasks {
			break
		}
		bestDepth = depth
	}
	return bestDepth
}

func estimateTaskCount(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	allowSkips bool,
	depth int,
) int {
	if depth <= 0 || len(ordered) == 0 {
		return 1
	}
	if depth > len(ordered) {
		depth = len(ordered)
	}
	count := 1
	for idx := 0; idx < depth; idx++ {
		branchCount := len(optionsByInstance[ordered[idx].InstanceID])
		if allowSkips {
			branchCount++
		}
		if branchCount == 0 {
			return 0
		}
		count *= branchCount
	}
	return count
}

func assignBudgets(tasks []searchTask, maxNodes int64, potential *starPotentialContext, semantics model.PrioritySemantics) {
	assignBudgetsWithPolicy(tasks, maxNodes, potential, semantics, nil)
}

func assignBudgetsWithPolicy(tasks []searchTask, maxNodes int64, potential *starPotentialContext, semantics model.PrioritySemantics, policy *ResolvedSearchPolicy) {
	if maxNodes == 0 || len(tasks) == 0 {
		return
	}
	resolved := resolveSearchPolicy(Config{}, maxNodes)
	if policy != nil {
		resolved = *policy
	}
	if semantics != model.PrioritySemanticsOutgoingPerInstanceV3 || potential == nil {
		assignEqualBudgets(tasks, maxNodes)
		return
	}

	// Keep every root alive, then invest the remaining budget in roots whose
	// fixed prefix exposes more compatible links for active V3 sources.
	minimum := resolved.MinAllocatedNodesPerTask
	if minimum <= 0 || minimum*int64(len(tasks)) > maxNodes {
		minimum = maxNodes / int64(len(tasks))
	}
	remaining := maxNodes - minimum*int64(len(tasks))
	weights := make([]int64, len(tasks))
	var totalWeight int64
	for index := range tasks {
		weight := int64(1)
		for _, placement := range tasks[index].Placements {
			weight += int64(potential.priorityPlacementPotential[coveragePlacementKey(placement)])
		}
		weights[index] = weight
		totalWeight += weight
		tasks[index].HasNodeBudget = true
		tasks[index].NodeBudget = minimum
	}
	if remaining <= 0 || totalWeight == 0 {
		return
	}
	assigned := int64(0)
	for index := range tasks {
		extra := remaining * weights[index] / totalWeight
		tasks[index].NodeBudget += extra
		assigned += extra
	}
	for index := 0; assigned < remaining; index = (index + 1) % len(tasks) {
		tasks[index].NodeBudget++
		assigned++
	}
}

func assignEqualBudgets(tasks []searchTask, maxNodes int64) {
	base := maxNodes / int64(len(tasks))
	remainder := maxNodes % int64(len(tasks))
	for idx := range tasks {
		tasks[idx].HasNodeBudget = true
		tasks[idx].NodeBudget = base
		if int64(idx) < remainder {
			tasks[idx].NodeBudget++
		}
	}
}

func taskAllocationStats(tasks []searchTask, splitDepth int) model.TaskAllocationStats {
	stats := model.TaskAllocationStats{SplitDepth: splitDepth, TasksGenerated: len(tasks)}
	if len(tasks) == 0 {
		return stats
	}
	budgets := make([]int64, len(tasks))
	for index, task := range tasks {
		budgets[index] = task.NodeBudget
	}
	sort.Slice(budgets, func(i, j int) bool { return budgets[i] < budgets[j] })
	stats.AllocatedNodesMin = budgets[0]
	stats.AllocatedNodesMax = budgets[len(budgets)-1]
	stats.AllocatedNodesP50 = budgets[(len(budgets)-1)/2]
	p90 := (len(budgets)*90 + 99) / 100
	if p90 < 1 {
		p90 = 1
	}
	stats.AllocatedNodesP90 = budgets[p90-1]
	return stats
}

func runTasks(
	catalog model.Catalog,
	original []model.InventoryInstance,
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	tasks []searchTask,
	config Config,
	gridMask uint64,
	coverage *coverageContext,
	exactBounds *exactBoundContext,
	outgoingBounds *outgoingBoundContext,
	priorityBounds *priorityBoundContext,
	initialSolutions []model.Solution,
	progress *progressTracker,
) searchResult {
	var stopFlag atomic.Bool
	if config.Context != nil && config.Context.Err() != nil {
		stopFlag.Store(true)
	}
	if config.Workers <= 1 || len(tasks) <= 1 {
		var combined searchResult
		for _, task := range tasks {
			if stopFlag.Load() {
				break
			}
			combined.TasksExecuted++
			partial := runTask(catalog, original, ordered, optionsByInstance, task, config, gridMask, coverage, exactBounds, outgoingBounds, priorityBounds, initialSolutions, &stopFlag, progress)
			combined.Solutions = mergeSolutions(combined.Solutions, partial.Solutions, config.TopN)
			combined.NodesExplored += partial.NodesExplored
			combined.HitNodeBudget = combined.HitNodeBudget || partial.HitNodeBudget
			combined.CoverageBoundChecks += partial.CoverageBoundChecks
			combined.CoveragePrunedNodes += partial.CoveragePrunedNodes
			combined.ExactBoundChecks += partial.ExactBoundChecks
			combined.ExactBoundPrunedNodes += partial.ExactBoundPrunedNodes
			combined.OutgoingBoundChecks += partial.OutgoingBoundChecks
			combined.OutgoingBoundPrunedNodes += partial.OutgoingBoundPrunedNodes
			combined.BoundOperationProfile = mergeBoundAttributionOperationProfiles(combined.BoundOperationProfile, partial.BoundOperationProfile)
			combined.SymmetryPrunedBranches += partial.SymmetryPrunedBranches
			combined.StoppedAfterCoverageCeiling = combined.StoppedAfterCoverageCeiling || partial.StoppedAfterCoverageCeiling
			combined.StoppedAfterPriorityCeiling = combined.StoppedAfterPriorityCeiling || partial.StoppedAfterPriorityCeiling
		}
		combined.TasksPrunedBeforeExecution = len(tasks) - combined.TasksExecuted
		combined.ParallelTasks = len(tasks)
		if len(tasks) > 0 {
			combined.ParallelWorkersUsed = 1
		}
		return combined
	}

	jobs := make(chan searchJob)
	results := make(chan taskRunResult, len(tasks))
	var waitGroup sync.WaitGroup
	workerCount := config.Workers
	if workerCount > len(tasks) {
		workerCount = len(tasks)
	}
	for i := 0; i < workerCount; i++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for job := range jobs {
				results <- taskRunResult{
					Index:  job.Index,
					Result: runTask(catalog, original, ordered, optionsByInstance, job.Task, config, gridMask, coverage, exactBounds, outgoingBounds, priorityBounds, initialSolutions, &stopFlag, progress),
				}
			}
		}()
	}

	dispatched := 0
	for taskIndex, task := range tasks {
		if stopFlag.Load() {
			break
		}
		if config.Context != nil && config.Context.Err() != nil {
			stopFlag.Store(true)
			break
		}
		jobs <- searchJob{Index: taskIndex, Task: task}
		dispatched++
	}
	close(jobs)
	waitGroup.Wait()
	close(results)

	partials := make([]searchResult, len(tasks))
	received := make([]bool, len(tasks))
	for partial := range results {
		if partial.Index >= 0 && partial.Index < len(partials) {
			partials[partial.Index] = partial.Result
			received[partial.Index] = true
		}
	}

	var combined searchResult
	for idx, partial := range partials {
		if !received[idx] {
			continue
		}
		combined.Solutions = mergeSolutions(combined.Solutions, partial.Solutions, config.TopN)
		combined.NodesExplored += partial.NodesExplored
		combined.HitNodeBudget = combined.HitNodeBudget || partial.HitNodeBudget
		combined.CoverageBoundChecks += partial.CoverageBoundChecks
		combined.CoveragePrunedNodes += partial.CoveragePrunedNodes
		combined.ExactBoundChecks += partial.ExactBoundChecks
		combined.ExactBoundPrunedNodes += partial.ExactBoundPrunedNodes
		combined.OutgoingBoundChecks += partial.OutgoingBoundChecks
		combined.OutgoingBoundPrunedNodes += partial.OutgoingBoundPrunedNodes
		combined.BoundOperationProfile = mergeBoundAttributionOperationProfiles(combined.BoundOperationProfile, partial.BoundOperationProfile)
		combined.SymmetryPrunedBranches += partial.SymmetryPrunedBranches
		combined.StoppedAfterCoverageCeiling = combined.StoppedAfterCoverageCeiling || partial.StoppedAfterCoverageCeiling
		combined.StoppedAfterPriorityCeiling = combined.StoppedAfterPriorityCeiling || partial.StoppedAfterPriorityCeiling
	}
	combined.ParallelTasks = len(tasks)
	combined.ParallelWorkersUsed = workerCount
	combined.TasksExecuted = dispatched
	combined.TasksPrunedBeforeExecution = len(tasks) - dispatched
	return combined
}

func runTask(
	catalog model.Catalog,
	original []model.InventoryInstance,
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	task searchTask,
	config Config,
	gridMask uint64,
	coverage *coverageContext,
	exactBounds *exactBoundContext,
	outgoingBounds *outgoingBoundContext,
	priorityBounds *priorityBoundContext,
	initialSolutions []model.Solution,
	stopFlag *atomic.Bool,
	progress *progressTracker,
) searchResult {
	operationCounters := newBoundOperationCounters(config)
	results := mergeSolutions(nil, initialSolutions, config.TopN)
	var nodes int64
	var hitNodeBudget bool
	var coverageBoundChecks int64
	var coveragePrunedNodes int64
	var exactBoundChecks int64
	var exactBoundPrunedNodes int64
	var outgoingBoundChecks int64
	var outgoingBoundPrunedNodes int64
	var symmetryPruned int64
	var stoppedAfterCoverageCeiling bool
	var stoppedAfterPriorityCeiling bool
	var progressBatch int64
	reportNode := func() bool {
		if !chargeNode(config, config.tracePhase) {
			if stopFlag != nil {
				stopFlag.Store(true)
			}
			return false
		}
		nodes++
		if config.Context != nil && nodes%progressNodeInterval == 0 && config.Context.Err() != nil && stopFlag != nil {
			stopFlag.Store(true)
		}
		if progress == nil {
			return true
		}
		progressBatch++
		if progressBatch >= progressNodeInterval {
			progress.addNodes(ProgressPhaseSearch, progressBatch, false)
			progressBatch = 0
		}
		return true
	}
	flushProgress := func() {
		if progress != nil && progressBatch > 0 {
			progress.addNodes(ProgressPhaseSearch, progressBatch, false)
			progressBatch = 0
		}
	}
	remainingCells := remainingCellCounts(catalog, ordered)
	coverageState := coverageSearchState{}
	if coverage != nil {
		coverageState = coverage.initialState(catalog, ordered, task.Index, task.Placements)
	}
	initialBoundState := exactBoundState{}
	if exactBounds != nil {
		initialBoundState = exactBounds.initialState(catalog, ordered, task.Index, task.Placements)
	}

	var visit func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState)
	visit = func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState) {
		if stopFlag != nil && stopFlag.Load() {
			return
		}
		if config.Context != nil && nodes%progressNodeInterval == 0 && config.Context.Err() != nil {
			if stopFlag != nil {
				stopFlag.Store(true)
			}
			return
		}
		if task.HasNodeBudget && nodes >= task.NodeBudget {
			hitNodeBudget = true
			return
		}
		if !reportNode() {
			return
		}
		if stopFlag != nil && stopFlag.Load() {
			return
		}
		if !config.AllowSkips && remainingCells[index] > bits.OnesCount64(gridMask&^occupied) {
			return
		}
		if coverage != nil && coverage.pruningEnabled {
			coverageBoundChecks++
			if coverage.shouldPrune(state, index, results, config.TopN) {
				coveragePrunedNodes++
				return
			}
		}
		if exactBounds != nil && exactBounds.shouldCheck(boundState) && len(results) >= config.TopN && config.TopN > 0 {
			exactBoundChecks++
			if exactBounds.shouldPrune(boundState, index, results, config.TopN) {
				exactBoundPrunedNodes++
				return
			}
		}
		if outgoingBounds != nil && len(results) >= config.TopN && config.TopN > 0 {
			outgoingBoundChecks++
			pruned := false
			if searchOperationProfilingAvailable && config.OperationProfiling {
				pruned = outgoingBounds.shouldPruneProfiled(placements, results, config.TopN, operationCounters.outgoingSite(boundOutgoingSearch))
			} else {
				pruned = outgoingBounds.shouldPrune(placements, results, config.TopN)
			}
			if pruned {
				outgoingBoundPrunedNodes++
				return
			}
		}

		if index == len(ordered) {
			previousBest, hadPreviousBest := bestSolution(results)
			results = insertCandidateWithScoreOnlyFilter(catalog, results, placements, original, config)
			if progress != nil && bestSolutionImproved(previousBest, hadPreviousBest, results) {
				flushProgress()
				progress.reportIncumbent(ProgressPhaseSearch, results, false)
			}
			if config.StopOnCoverageCeiling && coverage != nil && len(results) > 0 && coverage.ceilingReached(results[0].Evaluation.Score) {
				stoppedAfterCoverageCeiling = true
				if stopFlag != nil {
					stopFlag.Store(true)
				}
			}
			if config.StopOnPriorityCeiling && len(results) > 0 && priorityBounds.reached(results[0].Evaluation.Score) {
				stoppedAfterPriorityCeiling = true
				if stopFlag != nil {
					stopFlag.Store(true)
				}
			}
			return
		}

		instance := ordered[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			if !placementRespectsCanonicalCopyOrder(option, placements) {
				symmetryPruned++
				continue
			}
			var insertedAt int
			nextState := state
			if coverage != nil {
				nextState = coverage.withPlacement(catalog, state, option, placements)
			}
			nextBoundState := boundState
			if exactBounds != nil {
				nextBoundState = exactBounds.withPlacement(catalog, boundState, option, placements)
			}
			placements, insertedAt = insertPlacementSorted(placements, option)
			visit(index+1, occupied|option.Mask, placements, nextState, nextBoundState)
			placements = removePlacementAt(placements, insertedAt)
		}

		if config.AllowSkips {
			nextState := state
			if coverage != nil {
				nextState = coverage.withSkip(state, instance)
			}
			nextBoundState := boundState
			if exactBounds != nil {
				nextBoundState = exactBounds.withSkip(boundState, instance)
			}
			visit(index+1, occupied, placements, nextState, nextBoundState)
		}
	}

	visit(task.Index, task.Occupied, append([]model.Placement(nil), task.Placements...), coverageState, initialBoundState)
	flushProgress()
	return searchResult{
		Solutions:                   results,
		NodesExplored:               nodes,
		HitNodeBudget:               hitNodeBudget,
		CoverageBoundChecks:         coverageBoundChecks,
		CoveragePrunedNodes:         coveragePrunedNodes,
		ExactBoundChecks:            exactBoundChecks,
		ExactBoundPrunedNodes:       exactBoundPrunedNodes,
		OutgoingBoundChecks:         outgoingBoundChecks,
		OutgoingBoundPrunedNodes:    outgoingBoundPrunedNodes,
		SymmetryPrunedBranches:      symmetryPruned,
		StoppedAfterCoverageCeiling: stoppedAfterCoverageCeiling,
		StoppedAfterPriorityCeiling: stoppedAfterPriorityCeiling,
		BoundOperationProfile:       operationCounters.snapshotSearch(outgoingBoundChecks, outgoingBoundPrunedNodes),
	}
}

func bestSolution(solutions []model.Solution) (model.Solution, bool) {
	if len(solutions) == 0 {
		return model.Solution{}, false
	}
	return solutions[0], true
}

func bestSolutionImproved(previous model.Solution, hadPrevious bool, current []model.Solution) bool {
	if len(current) == 0 {
		return false
	}
	if !hadPrevious {
		return true
	}
	return SolutionLess(current[0], previous)
}

func sortPlacementOptionsForCoverage(optionsByInstance map[string][]model.Placement, coverage *coverageContext, potential ...*starPotentialContext) {
	var starPotential *starPotentialContext
	if len(potential) > 0 {
		starPotential = potential[0]
	}
	for instanceID := range optionsByInstance {
		options := optionsByInstance[instanceID]
		sort.Slice(options, func(i, j int) bool {
			leftCoverage := 0
			rightCoverage := 0
			if coverage != nil && coverage.enabled {
				leftCoverage = coverage.priorityForPlacement(options[i])
				rightCoverage = coverage.priorityForPlacement(options[j])
			}
			if leftCoverage != rightCoverage {
				return leftCoverage > rightCoverage
			}
			leftPriority := starPotential.priorityForPlacement(options[i])
			rightPriority := starPotential.priorityForPlacement(options[j])
			if leftPriority != rightPriority {
				return leftPriority > rightPriority
			}
			return placementKey(options[i]) < placementKey(options[j])
		})
	}
}

func recipeContainsItem(recipe model.Recipe, itemID string) bool {
	for _, ingredient := range recipe.Ingredients {
		if ingredient == itemID {
			return true
		}
	}
	return false
}

func filterInventoryImpossibleRecipes(catalog model.Catalog, itemIDs []string) model.Catalog {
	inventoryCounts := make(map[string]int, len(itemIDs))
	for _, itemID := range itemIDs {
		inventoryCounts[itemID]++
	}
	filtered := make([]model.Recipe, 0, len(catalog.Recipes))
	for _, recipe := range catalog.Recipes {
		if recipePossibleWithInventory(recipe, inventoryCounts) {
			filtered = append(filtered, recipe)
		}
	}
	if len(filtered) == len(catalog.Recipes) {
		return catalog
	}
	catalog.Recipes = filtered
	return catalog
}

func recipePossibleWithInventory(recipe model.Recipe, inventoryCounts map[string]int) bool {
	requirements := recipe.CompiledRequirements
	if !requirements.Ready {
		requirements = model.BuildRecipeRequirements(recipe.Anchor, recipe.Ingredients)
	}
	needed := map[string]int{recipe.Anchor: 1}
	for idx := 0; idx < requirements.Len; idx++ {
		needed[requirements.Items[idx]] += requirements.Counts[idx]
	}
	for itemID, count := range needed {
		if inventoryCounts[itemID] < count {
			return false
		}
	}
	return true
}

func buildSolution(catalog model.Catalog, placements []model.Placement, original []model.InventoryInstance, priorities []string) model.Solution {
	placed := append([]model.Placement(nil), placements...)
	sortPlacementsByOriginal(placed)
	return model.Solution{
		Placements:          placed,
		Evaluation:          scoring.EvaluateLayoutWithCoverageGroups(catalog, placed, priorities, nil),
		LayoutKey:           layoutKey(placed, original),
		CanonicalLayoutHash: canonicalLayoutHash(placed),
	}
}

func evaluateLayoutForConfig(catalog model.Catalog, placements []model.Placement, config Config) model.Evaluation {
	return scoring.EvaluateLayoutWithCoverageGroupsAndSemantics(catalog, placements, config.Priorities, config.CoverageGroups, config.PrioritySemantics)
}

func evaluateScoreForConfig(catalog model.Catalog, placements []model.Placement, config Config) model.Score {
	return scoring.EvaluateScoreOnlyWithCoverageGroupsAndSemantics(catalog, placements, config.Priorities, config.CoverageGroups, config.PrioritySemantics)
}

func refineSolutions(
	catalog model.Catalog,
	original []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solutions []model.Solution,
	searchStats model.SearchStats,
	config Config,
) ([]model.Solution, model.SearchStats, error) {
	if config.MaxRefineMoves <= 0 {
		config.MaxRefineMoves = defaultRefineMoveLimit(config.MaxNodes)
	}
	refined := make([]model.Solution, 0, len(solutions))
	remainingMoves := config.MaxRefineMoves
	for solutionIndex, solution := range solutions {
		if remainingMoves <= 0 {
			refined = append(refined, solutions[solutionIndex:]...)
			break
		}
		refineConfig := config
		refineConfig.MaxRefineMoves = remainingMoves
		next, changed, stats, err := refineSolution(catalog, original, optionsByInstance, solution, refineConfig)
		if err != nil {
			return nil, searchStats, err
		}
		searchStats.RefineMovesChecked += stats.MovesChecked
		searchStats.RefineImprovements += stats.Improvements
		searchStats.SymmetryPrunedBranches += stats.SymmetryPrunedBranches
		searchStats.Refined = searchStats.Refined || solution.Search.Refined || changed
		next.Search = searchStats
		refined = append(refined, next)
		remainingMoves -= stats.MovesChecked
		if stats.MoveLimitReached {
			refined = append(refined, solutions[solutionIndex+1:]...)
			break
		}
	}
	return refined, searchStats, nil
}

type refineStats struct {
	MovesChecked           int64
	Improvements           int
	MoveLimitReached       bool
	SymmetryPrunedBranches int64
	FirstCompleteNode      int64
}

func refineSolution(
	catalog model.Catalog,
	original []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solution model.Solution,
	config Config,
) (model.Solution, bool, refineStats, error) {
	const maxRounds = 4
	current := solution
	changed := false
	var stats refineStats
	maxMoves := config.MaxRefineMoves
	if maxMoves <= 0 {
		maxMoves = defaultRefineMoveLimit(config.MaxNodes)
	}
	for round := 0; round < maxRounds; round++ {
		if config.Context != nil {
			if err := config.Context.Err(); err != nil {
				return current, changed, stats, err
			}
		}
		best := current
		improved := false
		for placementIndex, placement := range current.Placements {
			if config.Context != nil {
				if err := config.Context.Err(); err != nil {
					return current, changed, stats, err
				}
			}
			fixedMask := occupiedExcept(current.Placements, placementIndex)
			candidatePlacements := append([]model.Placement(nil), current.Placements...)
			for _, option := range optionsByInstance[placement.InstanceID] {
				if config.Context != nil {
					if err := config.Context.Err(); err != nil {
						return current, changed, stats, err
					}
				}
				if stats.MovesChecked >= maxMoves {
					stats.MoveLimitReached = true
					if improved {
						current = best
						changed = true
					}
					return current, changed, stats, nil
				}
				if option.Mask&fixedMask != 0 {
					continue
				}
				stats.MovesChecked++
				config.trace.addUncharged(config.tracePhase, 1)
				candidatePlacements[placementIndex] = option
				if !placementRespectsCanonicalCopyOrder(option, candidatePlacements) {
					stats.SymmetryPrunedBranches++
					continue
				}
				score := evaluateScoreForConfig(catalog, candidatePlacements, config)
				observeSearchCandidate(catalog, candidatePlacements, original, score, true, config)
				if config.Context != nil {
					if err := config.Context.Err(); err != nil {
						return current, changed, stats, err
					}
				}
				if !scoreOnlyImprovesSolution(candidatePlacements, original, score, best) {
					continue
				}
				evaluation := evaluateLayoutForConfig(catalog, candidatePlacements, config)
				if config.Context != nil {
					if err := config.Context.Err(); err != nil {
						return current, changed, stats, err
					}
				}
				candidate, ok := improvedCandidate(candidatePlacements, original, evaluation, best)
				if ok {
					candidate.Search = current.Search
					best = candidate
					improved = true
					stats.Improvements++
				}
			}
		}
		if !improved {
			break
		}
		current = best
		changed = true
	}
	return current, changed, stats, nil
}

func occupiedExcept(placements []model.Placement, skippedIndex int) uint64 {
	var occupied uint64
	for idx, placement := range placements {
		if idx != skippedIndex {
			occupied |= placement.Mask
		}
	}
	return occupied
}

func improvedCandidate(
	placements []model.Placement,
	original []model.InventoryInstance,
	evaluation model.Evaluation,
	best model.Solution,
) (model.Solution, bool) {
	scoreCompare := compareScores(evaluation.Score, best.Evaluation.Score)
	if scoreCompare < 0 {
		return model.Solution{}, false
	}
	candidateLayoutKey := layoutKey(placements, original)
	if scoreCompare == 0 && candidateLayoutKey >= best.LayoutKey {
		return model.Solution{}, false
	}
	return model.Solution{
		Placements:          append([]model.Placement(nil), placements...),
		Evaluation:          evaluation,
		LayoutKey:           candidateLayoutKey,
		CanonicalLayoutHash: canonicalLayoutHash(placements),
	}, true
}

func insertCandidate(
	results []model.Solution,
	placements []model.Placement,
	original []model.InventoryInstance,
	evaluation model.Evaluation,
	topN int,
) []model.Solution {
	if topN <= 0 {
		return results
	}
	if len(results) == topN {
		worst := results[len(results)-1]
		scoreCompare := compareScores(evaluation.Score, worst.Evaluation.Score)
		if scoreCompare < 0 {
			return results
		}
		candidateLayoutKey := layoutKey(placements, original)
		if scoreCompare == 0 && candidateLayoutKey >= worst.LayoutKey {
			return results
		}
		results[len(results)-1] = model.Solution{
			Placements:          append([]model.Placement(nil), placements...),
			Evaluation:          evaluation,
			LayoutKey:           candidateLayoutKey,
			CanonicalLayoutHash: canonicalLayoutHash(placements),
		}
		return bubbleSolutionBackward(results, len(results)-1)
	}

	solution := model.Solution{
		Placements:          append([]model.Placement(nil), placements...),
		Evaluation:          evaluation,
		LayoutKey:           layoutKey(placements, original),
		CanonicalLayoutHash: canonicalLayoutHash(placements),
	}
	results = append(results, solution)
	return bubbleSolutionBackward(results, len(results)-1)
}

func insertCandidateWithScoreOnlyFilter(
	catalog model.Catalog,
	results []model.Solution,
	placements []model.Placement,
	original []model.InventoryInstance,
	config Config,
) []model.Solution {
	if config.TopN <= 0 {
		return results
	}
	if len(results) < config.TopN {
		evaluation := evaluateLayoutForConfig(catalog, placements, config)
		observeSearchCandidate(catalog, placements, original, evaluation.Score, true, config)
		return insertCandidate(results, placements, original, evaluation, config.TopN)
	}
	score := evaluateScoreForConfig(catalog, placements, config)
	observeSearchCandidate(catalog, placements, original, score, true, config)
	if !scoreOnlyCandidateCanEnter(results, placements, original, score, config.TopN) {
		return results
	}
	evaluation := evaluateLayoutForConfig(catalog, placements, config)
	return insertCandidate(results, placements, original, evaluation, config.TopN)
}

func scoreOnlyCandidateCanEnter(
	results []model.Solution,
	placements []model.Placement,
	original []model.InventoryInstance,
	score model.Score,
	topN int,
) bool {
	if topN <= 0 {
		return false
	}
	if len(results) < topN {
		return true
	}
	worst := results[len(results)-1]
	scoreCompare := compareScores(score, worst.Evaluation.Score)
	if scoreCompare < 0 {
		return false
	}
	if scoreCompare > 0 {
		return true
	}
	return layoutKey(placements, original) < worst.LayoutKey
}

func scoreOnlyImprovesSolution(
	placements []model.Placement,
	original []model.InventoryInstance,
	score model.Score,
	best model.Solution,
) bool {
	scoreCompare := compareScores(score, best.Evaluation.Score)
	if scoreCompare < 0 {
		return false
	}
	if scoreCompare > 0 {
		return true
	}
	return layoutKey(placements, original) < best.LayoutKey
}

func mergeSolutions(left []model.Solution, right []model.Solution, topN int) []model.Solution {
	merged := append(append([]model.Solution(nil), left...), right...)
	sortSolutions(merged)
	if len(merged) > topN {
		merged = merged[:topN]
	}
	return merged
}

func uniqueSolutions(solutions []model.Solution) []model.Solution {
	seen := map[string]bool{}
	unique := make([]model.Solution, 0, len(solutions))
	for _, solution := range solutions {
		if seen[solution.LayoutKey] {
			continue
		}
		seen[solution.LayoutKey] = true
		unique = append(unique, solution)
	}
	return unique
}

func remainingCellCounts(catalog model.Catalog, ordered []model.InventoryInstance) []int {
	remaining := make([]int, len(ordered)+1)
	for idx := len(ordered) - 1; idx >= 0; idx-- {
		remaining[idx] = remaining[idx+1] + len(catalog.Items[ordered[idx].ItemID].Shape)
	}
	return remaining
}

func insertPlacementSorted(placements []model.Placement, placement model.Placement) ([]model.Placement, int) {
	insertAt := sort.Search(len(placements), func(idx int) bool {
		return placements[idx].OriginalIndex > placement.OriginalIndex
	})
	placements = append(placements, model.Placement{})
	copy(placements[insertAt+1:], placements[insertAt:])
	placements[insertAt] = placement
	return placements, insertAt
}

// placementRespectsCanonicalCopyOrder removes only label permutations of
// identical inventory copies. The physical layout remains representable by
// assigning its placements to copies in original-index order.
func placementRespectsCanonicalCopyOrder(placement model.Placement, existing []model.Placement) bool {
	var key string
	keyReady := false
	for _, other := range existing {
		if other.ItemID != placement.ItemID || other.InstanceID == placement.InstanceID {
			continue
		}
		if !keyReady {
			key = placementKey(placement)
			keyReady = true
		}
		otherKey := placementKey(other)
		if other.OriginalIndex < placement.OriginalIndex && otherKey > key {
			return false
		}
		if other.OriginalIndex > placement.OriginalIndex && otherKey < key {
			return false
		}
	}
	return true
}

func removePlacementAt(placements []model.Placement, index int) []model.Placement {
	copy(placements[index:], placements[index+1:])
	placements[len(placements)-1] = model.Placement{}
	return placements[:len(placements)-1]
}

func sortPlacementsByOriginal(placements []model.Placement) {
	for idx := 1; idx < len(placements); idx++ {
		current := placements[idx]
		insertAt := idx
		for insertAt > 0 && placements[insertAt-1].OriginalIndex > current.OriginalIndex {
			placements[insertAt] = placements[insertAt-1]
			insertAt--
		}
		placements[insertAt] = current
	}
}

func sortSolutions(solutions []model.Solution) {
	for idx := 1; idx < len(solutions); idx++ {
		bubbleSolutionBackward(solutions, idx)
	}
}

func bubbleSolutionBackward(solutions []model.Solution, index int) []model.Solution {
	for index > 0 && SolutionLess(solutions[index], solutions[index-1]) {
		solutions[index], solutions[index-1] = solutions[index-1], solutions[index]
		index--
	}
	return solutions
}

func candidateLimit(topN int) int {
	limit := topN * 8
	if limit < 16 {
		return 16
	}
	return limit
}

func defaultRefineMoveLimit(maxNodes int64) int64 {
	const (
		minimumRefineMoves = int64(1000)
		maximumRefineMoves = int64(25000)
	)
	if maxNodes <= 0 {
		return maximumRefineMoves
	}
	limit := maxNodes / 4
	if limit < minimumRefineMoves {
		return minimumRefineMoves
	}
	if limit > maximumRefineMoves {
		return maximumRefineMoves
	}
	return limit
}

func instancePriority(catalog model.Catalog, instance model.InventoryInstance, priorities []string, coverage *coverageContext, potential ...*starPotentialContext) int {
	item := catalog.Items[instance.ItemID]
	priority := len(item.Shape)
	if len(potential) > 0 && potential[0] != nil {
		priority += potential[0].priorityForInstance(instance) * 6
	} else {
		priority += len(item.Stars) * 6
	}
	if coverage != nil && coverage.enabled {
		targetIndex := coverage.targetIndexByOriginal[instance.OriginalIndex]
		if targetIndex >= 0 {
			priority += 256 + bits.OnesCount64(coverage.targetPossibleSourceMask[targetIndex])*16
		}
		if coverage.sourceMaskByOriginal[instance.OriginalIndex] != 0 {
			priority += 128
		}
	}
	if hasPriority(priorities, "star_source", instance.ItemID) {
		priority += 32
	}
	for _, recipe := range catalog.Recipes {
		if hasPriority(priorities, "craft", recipe.Result) {
			if recipe.Anchor == instance.ItemID {
				priority += 32
			}
			if recipeContainsItem(recipe, instance.ItemID) {
				priority += 16
			}
		}
		if recipe.Anchor == instance.ItemID {
			priority += 8
		}
		if recipeContainsItem(recipe, instance.ItemID) {
			priority += 2
		}
	}
	return priority
}

func SolutionLess(left model.Solution, right model.Solution) bool {
	if compare := compareScores(left.Evaluation.Score, right.Evaluation.Score); compare != 0 {
		return compare > 0
	}
	return left.LayoutKey < right.LayoutKey
}

func hasPriority(priorities []string, expectedKind string, expectedValue string) bool {
	for _, priority := range priorities {
		priority = strings.TrimSpace(priority)
		kind, value, ok := strings.Cut(priority, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if kind == expectedKind && value == expectedValue {
			return true
		}
	}
	return false
}

func comparePriorityCounts(left []int, right []int) int {
	return model.ComparePriorityCounts(left, right)
}

func compareScores(left model.Score, right model.Score) int {
	return model.CompareScores(left, right)
}

func scoreDeltaSummary(before model.Score, after model.Score) string {
	parts := []string{
		"priority " + scoreCountsText(before.PriorityCounts) + " -> " + scoreCountsText(after.PriorityCounts),
	}
	if before.CraftCount != after.CraftCount {
		parts = append(parts, fmt.Sprintf("crafts %d -> %d", before.CraftCount, after.CraftCount))
	}
	if before.StarCount != after.StarCount {
		parts = append(parts, fmt.Sprintf("stars %d -> %d", before.StarCount, after.StarCount))
	}
	if before.ItemCount != after.ItemCount {
		parts = append(parts, fmt.Sprintf("items %d -> %d", before.ItemCount, after.ItemCount))
	}
	return strings.Join(parts, ", ")
}

func scoreCountsText(counts []int) string {
	if len(counts) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(counts))
	for _, count := range counts {
		parts = append(parts, strconv.Itoa(count))
	}
	return strings.Join(parts, "/")
}

func layoutKey(placements []model.Placement, instances []model.InventoryInstance) string {
	var builder strings.Builder
	builder.Grow(len(instances) * 24)
	placementIndex := 0
	for _, instance := range instances {
		if placementIndex >= len(placements) || placements[placementIndex].InstanceID != instance.InstanceID {
			writePaddedInt(&builder, instance.OriginalIndex)
			builder.WriteByte('|')
			builder.WriteString(instance.ItemID)
			builder.WriteString("|999|999|999;")
			continue
		}
		placement := placements[placementIndex]
		writePaddedInt(&builder, instance.OriginalIndex)
		builder.WriteByte('|')
		builder.WriteString(instance.ItemID)
		builder.WriteByte('|')
		writePaddedInt(&builder, placement.Rotation)
		builder.WriteByte('|')
		writePaddedInt(&builder, placement.Origin.Row)
		builder.WriteByte('|')
		writePaddedInt(&builder, placement.Origin.Col)
		builder.WriteByte(';')
		placementIndex++
	}
	return builder.String()
}

func writePaddedInt(builder *strings.Builder, value int) {
	if value < 0 {
		builder.WriteString(strconv.Itoa(value))
		return
	}
	if value < 10 {
		builder.WriteString("00")
	} else if value < 100 {
		builder.WriteByte('0')
	}
	builder.WriteString(strconv.Itoa(value))
}

func placementKey(placement model.Placement) string {
	var builder strings.Builder
	builder.Grow(12 + len(placement.Cells)*8)
	writePlacementKeyInt(&builder, placement.Rotation)
	builder.WriteByte('|')
	writePlacementKeyInt(&builder, placement.Origin.Row)
	builder.WriteByte('|')
	writePlacementKeyInt(&builder, placement.Origin.Col)
	builder.WriteByte('|')
	for _, cell := range placement.Cells {
		writePlacementKeyInt(&builder, cell.Row)
		builder.WriteByte(',')
		writePlacementKeyInt(&builder, cell.Col)
		builder.WriteByte(';')
	}
	return builder.String()
}

func writePlacementKeyInt(builder *strings.Builder, value int) {
	if value < 0 || value > 999 {
		fmt.Fprintf(builder, "%03d", value)
		return
	}
	builder.WriteByte(byte('0' + value/100))
	builder.WriteByte(byte('0' + value/10%10))
	builder.WriteByte(byte('0' + value%10))
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	last := ""
	for idx, value := range values {
		if idx == 0 || value != last {
			result = append(result, value)
			last = value
		}
	}
	return result
}
