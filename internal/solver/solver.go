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

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

type Config struct {
	TopN                  int
	AllowSkips            bool
	MaxNodes              int64
	MaxRefineMoves        int64 // Zero derives a bounded limit from MaxNodes.
	Workers               int
	Priorities            []string
	CoverageGroups        []model.CoverageGroup
	StopOnCoverageCeiling bool
	RepairSearch          bool
	DisableExactBounds    bool
	ProgressReporter      ProgressReporter
	Context               context.Context
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
	ParallelTasks               int
	ParallelWorkersUsed         int
	StoppedAfterCoverageCeiling bool
}

type coverageSeedResult struct {
	Solutions                   []model.Solution
	NodesExplored               int64
	CandidateCount              int
	BestSummary                 string
	StoppedAfterCoverageCeiling bool
}

type repairResult struct {
	Solutions             []model.Solution
	NodesExplored         int64
	Iterations            int
	Improvements          int
	CandidateCount        int
	BestSummary           string
	CoverageBoundChecks   int64
	CoveragePrunedNodes   int64
	ExactBoundChecks      int64
	ExactBoundPrunedNodes int64
	ParallelTasks         int
	ParallelWorkersUsed   int
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
		leftPriority := staticPlacementPriority(options[i])
		rightPriority := staticPlacementPriority(options[j])
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
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

func SolveLayout(catalog model.Catalog, itemIDs []string, gridMask uint64, config Config) ([]model.Solution, error) {
	if len(itemIDs) > geometry.GridCells {
		return nil, fmt.Errorf("inventory has %d items; maximum is %d for the %dx%d grid", len(itemIDs), geometry.GridCells, geometry.GridRows, geometry.GridCols)
	}
	if config.TopN <= 0 {
		config.TopN = 3
	}
	if config.MaxRefineMoves <= 0 {
		config.MaxRefineMoves = defaultRefineMoveLimit(config.MaxNodes)
	}
	if config.Workers <= 0 {
		config.Workers = 1
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

	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(catalog, instance, gridMask)
		if err != nil {
			return nil, err
		}
		optionsByInstance[instance.InstanceID] = options
	}
	coverage := newCoverageContextForConfig(catalog, instances, optionsByInstance, config)
	sortPlacementOptionsForCoverage(optionsByInstance, coverage)

	requestedTopN := config.TopN
	searchConfig := config
	searchConfig.TopN = candidateLimit(requestedTopN)
	searchConfig.StopOnCoverageCeiling = config.StopOnCoverageCeiling && requestedTopN == 1

	limitedMode := config.MaxNodes > 0
	ordered := append([]model.InventoryInstance(nil), instances...)
	sort.Slice(ordered, func(i, j int) bool {
		if limitedMode {
			leftPriority := instancePriority(catalog, ordered[i], config.Priorities, coverage)
			rightPriority := instancePriority(catalog, ordered[j], config.Priorities, coverage)
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

	progress := newProgressTracker(config.ProgressReporter, config.MaxNodes)
	seedBudget := seedNodeBudget(config.MaxNodes)
	if progress != nil && coverage != nil && coverage.enabled && seedBudget > 0 {
		progress.emitPhase(ProgressPhaseSeed)
	}
	seed := coverageSeedSearch(catalog, instances, optionsByInstance, searchConfig, coverage, gridMask, seedBudget, progress)
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
		searchStats := model.SearchStats{
			NodesExplored:               seed.NodesExplored,
			Limited:                     limitedMode,
			CoverageSeedNodes:           seed.NodesExplored,
			CoverageSeedCandidates:      seed.CandidateCount,
			CoverageSeedBest:            seed.BestSummary,
			StoppedAfterCoverageCeiling: true,
		}
		if coverage != nil {
			searchStats.CoverageSources = append([]string(nil), coverage.sourceItemIDs...)
			searchStats.CoverageTargetCount = coverage.targetCount()
			searchStats.CoverageCeiling = append([]model.StarCoverageBucket(nil), coverage.coverageCeiling...)
		}
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
	var repair repairResult
	initialSolutions := seed.Solutions
	if config.RepairSearch && limitedMode && len(seed.Solutions) > 0 {
		repairBudget = repairNodeBudget(config.MaxNodes, seed.NodesExplored)
		if repairBudget > 0 {
			if progress != nil {
				progress.emitPhase(ProgressPhaseRepair)
			}
			repair = repairSearch(catalog, instances, optionsByInstance, searchConfig, coverage, gridMask, repairBudget, seed.Solutions, progress)
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
				searchStats := model.SearchStats{
					NodesExplored:               seed.NodesExplored + repair.NodesExplored,
					Limited:                     limitedMode,
					CoverageSeedNodes:           seed.NodesExplored,
					CoverageSeedCandidates:      seed.CandidateCount,
					CoverageSeedBest:            seed.BestSummary,
					CoverageBoundChecks:         repair.CoverageBoundChecks,
					CoveragePrunedNodes:         repair.CoveragePrunedNodes,
					ExactBoundChecks:            repair.ExactBoundChecks,
					ExactBoundPrunedNodes:       repair.ExactBoundPrunedNodes,
					RepairNodes:                 repair.NodesExplored,
					RepairIterations:            repair.Iterations,
					RepairImprovements:          repair.Improvements,
					RepairCandidates:            repair.CandidateCount,
					RepairBest:                  repair.BestSummary,
					RepairParallelTasks:         repair.ParallelTasks,
					RepairParallelWorkersUsed:   repair.ParallelWorkersUsed,
					StoppedAfterCoverageCeiling: true,
				}
				if coverage != nil {
					searchStats.CoverageSources = append([]string(nil), coverage.sourceItemIDs...)
					searchStats.CoverageTargetCount = coverage.targetCount()
					searchStats.CoverageCeiling = append([]model.StarCoverageBucket(nil), coverage.coverageCeiling...)
				}
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
	}

	backtrackConfig := searchConfig
	if backtrackConfig.MaxNodes > 0 {
		backtrackConfig.MaxNodes -= seed.NodesExplored + repair.NodesExplored
		if backtrackConfig.MaxNodes < 0 {
			backtrackConfig.MaxNodes = 0
		}
	}
	var search searchResult
	if config.MaxNodes == 0 || backtrackConfig.MaxNodes > 0 {
		if progress != nil {
			progress.emitPhase(ProgressPhaseSearch)
		}
		tasks := buildTasks(ordered, optionsByInstance, config.AllowSkips, initialSplitDepth(ordered, optionsByInstance, backtrackConfig))
		assignBudgets(tasks, backtrackConfig.MaxNodes)
		search = runTasks(catalog, instances, ordered, optionsByInstance, tasks, backtrackConfig, gridMask, coverage, exactBounds, initialSolutions, progress)
		if err := config.Context.Err(); err != nil {
			return nil, err
		}
	}
	results := mergeSolutions(initialSolutions, search.Solutions, searchConfig.TopN)
	searchStats := model.SearchStats{
		NodesExplored:               seed.NodesExplored + search.NodesExplored,
		Limited:                     limitedMode,
		CoverageBoundChecks:         repair.CoverageBoundChecks + search.CoverageBoundChecks,
		CoveragePrunedNodes:         repair.CoveragePrunedNodes + search.CoveragePrunedNodes,
		ExactBoundChecks:            repair.ExactBoundChecks + search.ExactBoundChecks,
		ExactBoundPrunedNodes:       repair.ExactBoundPrunedNodes + search.ExactBoundPrunedNodes,
		CoverageSeedNodes:           seed.NodesExplored,
		CoverageSeedCandidates:      seed.CandidateCount,
		CoverageSeedBest:            seed.BestSummary,
		ParallelTasks:               search.ParallelTasks,
		ParallelWorkersUsed:         search.ParallelWorkersUsed,
		RepairNodes:                 repair.NodesExplored,
		RepairIterations:            repair.Iterations,
		RepairImprovements:          repair.Improvements,
		RepairCandidates:            repair.CandidateCount,
		RepairBest:                  repair.BestSummary,
		RepairParallelTasks:         repair.ParallelTasks,
		RepairParallelWorkersUsed:   repair.ParallelWorkersUsed,
		StoppedAfterCoverageCeiling: seed.StoppedAfterCoverageCeiling || search.StoppedAfterCoverageCeiling,
	}
	searchStats.NodesExplored += repair.NodesExplored
	if coverage != nil {
		searchStats.CoverageSources = append([]string(nil), coverage.sourceItemIDs...)
		searchStats.CoverageTargetCount = coverage.targetCount()
		searchStats.CoverageCeiling = append([]model.StarCoverageBucket(nil), coverage.coverageCeiling...)
	}
	for idx := range results {
		results[idx].Search = searchStats
	}
	if progress != nil {
		progress.emitPhase(ProgressPhaseRefine)
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
	results, searchStats, refineErr = refineSolutions(catalog, instances, optionsByInstance, results, searchStats, config)
	if refineErr != nil {
		return nil, refineErr
	}
	sort.Slice(results, func(i, j int) bool {
		return SolutionLess(results[i], results[j])
	})
	results = uniqueSolutions(results)
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
	for idx := range results {
		results[idx].Search = searchStats
	}
	if progress != nil {
		progress.finish()
	}
	return results, nil
}

func buildTasks(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	allowSkips bool,
	splitDepth int,
) []searchTask {
	if len(ordered) < splitDepth {
		splitDepth = len(ordered)
	}

	var tasks []searchTask
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
	return tasks
}

func initialSplitDepth(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
) int {
	// Keep the DFS split identical to the pre-parallel baseline. The adaptive
	// split changed limited-search budget distribution in benchmarks; repair
	// parallelism is the only active parallel change in this V1.
	return legacyInitialSplitDepth(ordered, optionsByInstance, config)
}

func legacyInitialSplitDepth(
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
) int {
	maxDepth := 2
	if len(ordered) < maxDepth {
		maxDepth = len(ordered)
	}
	if config.MaxNodes == 0 {
		return maxDepth
	}

	const minimumTaskBudget = int64(128)
	for depth := maxDepth; depth > 0; depth-- {
		taskCount := estimateTaskCount(ordered, optionsByInstance, config.AllowSkips, depth)
		if taskCount == 0 || config.MaxNodes/int64(taskCount) >= minimumTaskBudget {
			return depth
		}
	}
	return 0
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

func assignBudgets(tasks []searchTask, maxNodes int64) {
	if maxNodes == 0 || len(tasks) == 0 {
		return
	}
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
			partial := runTask(catalog, original, ordered, optionsByInstance, task, config, gridMask, coverage, exactBounds, initialSolutions, &stopFlag, progress)
			combined.Solutions = mergeSolutions(combined.Solutions, partial.Solutions, config.TopN)
			combined.NodesExplored += partial.NodesExplored
			combined.HitNodeBudget = combined.HitNodeBudget || partial.HitNodeBudget
			combined.CoverageBoundChecks += partial.CoverageBoundChecks
			combined.CoveragePrunedNodes += partial.CoveragePrunedNodes
			combined.ExactBoundChecks += partial.ExactBoundChecks
			combined.ExactBoundPrunedNodes += partial.ExactBoundPrunedNodes
			combined.StoppedAfterCoverageCeiling = combined.StoppedAfterCoverageCeiling || partial.StoppedAfterCoverageCeiling
		}
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
					Result: runTask(catalog, original, ordered, optionsByInstance, job.Task, config, gridMask, coverage, exactBounds, initialSolutions, &stopFlag, progress),
				}
			}
		}()
	}

	for taskIndex, task := range tasks {
		if stopFlag.Load() {
			break
		}
		if config.Context != nil && config.Context.Err() != nil {
			stopFlag.Store(true)
			break
		}
		jobs <- searchJob{Index: taskIndex, Task: task}
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
		combined.StoppedAfterCoverageCeiling = combined.StoppedAfterCoverageCeiling || partial.StoppedAfterCoverageCeiling
	}
	combined.ParallelTasks = len(tasks)
	combined.ParallelWorkersUsed = workerCount
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
	initialSolutions []model.Solution,
	stopFlag *atomic.Bool,
	progress *progressTracker,
) searchResult {
	results := mergeSolutions(nil, initialSolutions, config.TopN)
	var nodes int64
	var hitNodeBudget bool
	var coverageBoundChecks int64
	var coveragePrunedNodes int64
	var exactBoundChecks int64
	var exactBoundPrunedNodes int64
	var stoppedAfterCoverageCeiling bool
	var progressBatch int64
	reportNode := func() {
		nodes++
		if config.Context != nil && nodes%progressNodeInterval == 0 && config.Context.Err() != nil && stopFlag != nil {
			stopFlag.Store(true)
		}
		if progress == nil {
			return
		}
		progressBatch++
		if progressBatch >= progressNodeInterval {
			progress.addNodes(ProgressPhaseSearch, progressBatch, false)
			progressBatch = 0
		}
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
		reportNode()
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
			return
		}

		instance := ordered[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
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
		StoppedAfterCoverageCeiling: stoppedAfterCoverageCeiling,
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

func inboundStarCount(placement model.Placement) int {
	count := 0
	for _, starPosition := range placement.StarPositions {
		if geometry.InBounds(starPosition.Position) {
			count++
		}
	}
	return count
}

func staticPlacementPriority(placement model.Placement) int {
	return inboundStarCount(placement)
}

func sortPlacementOptionsForCoverage(optionsByInstance map[string][]model.Placement, coverage *coverageContext) {
	if coverage == nil || !coverage.enabled {
		return
	}
	for instanceID := range optionsByInstance {
		options := optionsByInstance[instanceID]
		sort.Slice(options, func(i, j int) bool {
			leftCoverage := coverage.priorityForPlacement(options[i])
			rightCoverage := coverage.priorityForPlacement(options[j])
			if leftCoverage != rightCoverage {
				return leftCoverage > rightCoverage
			}
			leftPriority := staticPlacementPriority(options[i])
			rightPriority := staticPlacementPriority(options[j])
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
		Placements: placed,
		Evaluation: scoring.EvaluateLayoutWithCoverageGroups(catalog, placed, priorities, nil),
		LayoutKey:  layoutKey(placed, original),
	}
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
	MovesChecked     int64
	Improvements     int
	MoveLimitReached bool
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
					return current, changed, stats, nil
				}
				if option.Mask&fixedMask != 0 {
					continue
				}
				stats.MovesChecked++
				candidatePlacements[placementIndex] = option
				score := scoring.EvaluateScoreOnlyWithCoverageGroups(catalog, candidatePlacements, config.Priorities, config.CoverageGroups)
				if config.Context != nil {
					if err := config.Context.Err(); err != nil {
						return current, changed, stats, err
					}
				}
				if !scoreOnlyImprovesSolution(candidatePlacements, original, score, best) {
					continue
				}
				evaluation := scoring.EvaluateLayoutWithCoverageGroups(catalog, candidatePlacements, config.Priorities, config.CoverageGroups)
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
		Placements: append([]model.Placement(nil), placements...),
		Evaluation: evaluation,
		LayoutKey:  candidateLayoutKey,
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
			Placements: append([]model.Placement(nil), placements...),
			Evaluation: evaluation,
			LayoutKey:  candidateLayoutKey,
		}
		return bubbleSolutionBackward(results, len(results)-1)
	}

	solution := model.Solution{
		Placements: append([]model.Placement(nil), placements...),
		Evaluation: evaluation,
		LayoutKey:  layoutKey(placements, original),
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
		evaluation := scoring.EvaluateLayoutWithCoverageGroups(catalog, placements, config.Priorities, config.CoverageGroups)
		return insertCandidate(results, placements, original, evaluation, config.TopN)
	}
	score := scoring.EvaluateScoreOnlyWithCoverageGroups(catalog, placements, config.Priorities, config.CoverageGroups)
	if !scoreOnlyCandidateCanEnter(results, placements, original, score, config.TopN) {
		return results
	}
	evaluation := scoring.EvaluateLayoutWithCoverageGroups(catalog, placements, config.Priorities, config.CoverageGroups)
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

func instancePriority(catalog model.Catalog, instance model.InventoryInstance, priorities []string, coverage *coverageContext) int {
	item := catalog.Items[instance.ItemID]
	priority := len(item.Shape) + len(item.Stars)*6
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
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for idx := 0; idx < maxLen; idx++ {
		leftCount := 0
		if idx < len(left) {
			leftCount = left[idx]
		}
		rightCount := 0
		if idx < len(right) {
			rightCount = right[idx]
		}
		if leftCount != rightCount {
			return leftCount - rightCount
		}
	}
	return 0
}

func compareScores(left model.Score, right model.Score) int {
	if compare := comparePriorityCounts(left.PriorityCounts, right.PriorityCounts); compare != 0 {
		return compare
	}
	if left.CraftCount != right.CraftCount {
		return left.CraftCount - right.CraftCount
	}
	if left.StarCount != right.StarCount {
		return left.StarCount - right.StarCount
	}
	if left.ItemCount != right.ItemCount {
		return left.ItemCount - right.ItemCount
	}
	return 0
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
	fmt.Fprintf(&builder, "%03d|%03d|%03d|", placement.Rotation, placement.Origin.Row, placement.Origin.Col)
	for _, cell := range placement.Cells {
		fmt.Fprintf(&builder, "%03d,%03d;", cell.Row, cell.Col)
	}
	return builder.String()
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
