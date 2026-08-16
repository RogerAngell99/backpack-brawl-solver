package solver

import (
	"fmt"
	"math/bits"
	"sort"
	"strings"
	"sync"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

const (
	repairMinNodeBudget        = int64(20000)
	repairBudgetPercent        = int64(40)
	repairMaxNeighborhoodSize  = 8
	repairMaxNeighborhoods     = 96
	repairTargetTasksPerWorker = 8
	repairMaxSplitDepth        = 2
	repairMaxTasks             = 8192
	repairMinParallelBudget    = int64(20000)
)

type repairNeighborhood struct {
	Operator    string
	InstanceIDs []string
	Priority    int
	Key         string
}

type repairSearchTask struct {
	Index         int
	Occupied      uint64
	Placements    []model.Placement
	CoverageState coverageSearchState
	BoundState    exactBoundState
	NodeBudget    int64
	HasNodeBudget bool
}

type repairTaskRunResult struct {
	Index  int
	Result repairResult
}

type repairJob struct {
	Index int
	Task  repairSearchTask
}

func repairNodeBudget(maxNodes int64, seedNodes int64) int64 {
	if maxNodes <= 0 {
		return 0
	}
	remaining := maxNodes - seedNodes
	if remaining <= 0 {
		return 0
	}
	budget := maxNodes * repairBudgetPercent / 100
	if budget > remaining {
		budget = remaining
	}
	if budget < repairMinNodeBudget {
		return 0
	}
	return budget
}

func repairSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	coverage *coverageContext,
	gridMask uint64,
	nodeBudget int64,
	initialSolutions []model.Solution,
	progress *progressTracker,
) repairResult {
	if nodeBudget <= 0 || len(initialSolutions) == 0 {
		return repairResult{}
	}

	results := mergeSolutions(nil, initialSolutions, config.TopN)
	if len(results) == 0 {
		return repairResult{}
	}

	instanceByID := map[string]model.InventoryInstance{}
	for _, instance := range instances {
		instanceByID[instance.InstanceID] = instance
	}

	var nodes int64
	var progressBatch int64
	canceled := false
	reportNodes := func(delta int64) {
		nodes += delta
		if config.Context != nil && nodes%progressNodeInterval == 0 && config.Context.Err() != nil {
			canceled = true
		}
		if progress == nil || delta <= 0 {
			return
		}
		progressBatch += delta
		if progressBatch >= progressNodeInterval {
			progress.addNodes(ProgressPhaseRepair, progressBatch, false)
			progressBatch = 0
		}
	}
	flushProgress := func() {
		if progress != nil && progressBatch > 0 {
			progress.addNodes(ProgressPhaseRepair, progressBatch, false)
			progressBatch = 0
		}
	}

	seenNeighborhoods := map[string]bool{}
	var iterations int
	var improvements int
	var candidateCount int
	var coverageBoundChecks int64
	var coveragePrunedNodes int64
	var exactBoundChecks int64
	var exactBoundPrunedNodes int64
	var parallelTasks int
	var parallelWorkersUsed int

	for nodes < nodeBudget && !canceled {
		beforeBest := results[0]
		neighborhoods := buildRepairNeighborhoods(catalog, instances, optionsByInstance, results, config, coverage, seenNeighborhoods)
		if len(neighborhoods) == 0 {
			break
		}

		roundImproved := false
		for _, neighborhood := range neighborhoods {
			if canceled || nodes >= nodeBudget {
				break
			}
			seenNeighborhoods[neighborhood.Key] = true
			iterations++
			partial := runRepairNeighborhood(
				catalog,
				instances,
				instanceByID,
				optionsByInstance,
				results,
				neighborhood,
				config,
				coverage,
				gridMask,
				nodeBudget-nodes,
				reportNodes,
			)
			candidateCount += partial.CandidateCount
			coverageBoundChecks += partial.CoverageBoundChecks
			coveragePrunedNodes += partial.CoveragePrunedNodes
			exactBoundChecks += partial.ExactBoundChecks
			exactBoundPrunedNodes += partial.ExactBoundPrunedNodes
			parallelTasks += partial.ParallelTasks
			if partial.ParallelWorkersUsed > parallelWorkersUsed {
				parallelWorkersUsed = partial.ParallelWorkersUsed
			}
			if len(partial.Solutions) == 0 {
				continue
			}
			nextResults := mergeSolutions(results, partial.Solutions, config.TopN)
			if len(nextResults) > 0 && SolutionLess(nextResults[0], results[0]) {
				improvements++
				roundImproved = true
			}
			results = nextResults
		}

		if !roundImproved || !SolutionLess(results[0], beforeBest) {
			break
		}
	}

	flushProgress()
	return repairResult{
		Solutions:             results,
		NodesExplored:         nodes,
		Iterations:            iterations,
		Improvements:          improvements,
		CandidateCount:        candidateCount,
		BestSummary:           repairBestSummary(results),
		CoverageBoundChecks:   coverageBoundChecks,
		CoveragePrunedNodes:   coveragePrunedNodes,
		ExactBoundChecks:      exactBoundChecks,
		ExactBoundPrunedNodes: exactBoundPrunedNodes,
		ParallelTasks:         parallelTasks,
		ParallelWorkersUsed:   parallelWorkersUsed,
	}
}

func buildRepairNeighborhoods(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solutions []model.Solution,
	config Config,
	coverage *coverageContext,
	seen map[string]bool,
) []repairNeighborhood {
	var neighborhoods []repairNeighborhood
	for solutionIndex, solution := range solutions {
		placementByID := placementByInstanceID(solution.Placements)
		skipped := skippedInstances(instances, placementByID)
		addCoverageGapNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, solutionIndex)
		addSourceRetargetNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, solutionIndex)
		addLooseStarNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, config.Priorities, solutionIndex)
		addCraftGapNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, config.Priorities, solutionIndex)
		addPackingPressureNeighborhoods(&neighborhoods, catalog, instances, optionsByInstance, solution, placementByID, skipped, config.Priorities, coverage, solutionIndex)
	}

	sort.Slice(neighborhoods, func(i, j int) bool {
		if neighborhoods[i].Priority != neighborhoods[j].Priority {
			return neighborhoods[i].Priority > neighborhoods[j].Priority
		}
		return neighborhoods[i].Key < neighborhoods[j].Key
	})

	filtered := make([]repairNeighborhood, 0, minInt(len(neighborhoods), repairMaxNeighborhoods))
	localSeen := map[string]bool{}
	for _, neighborhood := range neighborhoods {
		if seen[neighborhood.Key] || localSeen[neighborhood.Key] {
			continue
		}
		localSeen[neighborhood.Key] = true
		filtered = append(filtered, neighborhood)
		if len(filtered) >= repairMaxNeighborhoods {
			break
		}
	}
	return filtered
}

func addCoverageGapNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	solutionIndex int,
) {
	groups := solution.Evaluation.StarCoverageGroups
	if len(groups) == 0 && solution.Evaluation.StarCoverage != nil {
		groups = []model.StarCoverageBreakdown{*solution.Evaluation.StarCoverage}
	}
	for groupIndex, group := range groups {
		sourceSet := stringSet(group.Sources)
		for _, target := range group.Targets {
			totalSources := len(group.Sources)
			if totalSources == 0 || target.CoveredCount >= totalSources {
				continue
			}
			mandatory := []string{target.TargetInstance}
			mandatory = append(mandatory, matchingPlacedInstances(solution.Placements, sourceSet)...)
			mandatory = append(mandatory, matchingSkippedInstances(skipped, sourceSet)...)
			optional := nearbyInstances(solution.Placements, placementByID, mandatory)
			priority := 100000 + (totalSources-target.CoveredCount)*2000 + totalSources*100 + groupIndex
			appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, "coverage-gap", priority, mandatory, optional)
		}
	}
}

func addSourceRetargetNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	solutionIndex int,
) {
	groups := solution.Evaluation.StarCoverageGroups
	if len(groups) == 0 && solution.Evaluation.StarCoverage != nil {
		groups = []model.StarCoverageBreakdown{*solution.Evaluation.StarCoverage}
	}
	for groupIndex, group := range groups {
		if len(group.Sources) == 0 {
			continue
		}
		sourceSet := stringSet(group.Sources)
		targets := underCoveredTargets(group)
		if len(targets) == 0 {
			continue
		}
		for _, sourceID := range append(matchingPlacedInstances(solution.Placements, sourceSet), matchingSkippedInstances(skipped, sourceSet)...) {
			mandatory := []string{sourceID}
			for _, target := range targets {
				mandatory = append(mandatory, target.TargetInstance)
				if len(mandatory) >= 4 {
					break
				}
			}
			optional := nearbyInstances(solution.Placements, placementByID, mandatory)
			priority := 85000 + groupIndex*100 + len(targets)
			appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, "source-retarget", priority, mandatory, optional)
		}
	}
}

func addLooseStarNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	priorities []string,
	solutionIndex int,
) {
	seenSources := map[string]bool{}
	for _, priority := range priorities {
		kind, sourceItemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" || seenSources[sourceItemID] {
			continue
		}
		seenSources[sourceItemID] = true
		coveredTargets := looseCoveredTargets(solution, sourceItemID)
		sourceSet := map[string]struct{}{sourceItemID: struct{}{}}
		mandatory := append([]string(nil), matchingPlacedInstances(solution.Placements, sourceSet)...)
		mandatory = append(mandatory, matchingSkippedInstances(skipped, sourceSet)...)
		for _, placement := range solution.Placements {
			if coveredTargets[placement.InstanceID] {
				continue
			}
			if looseSourceCanTarget(catalog, sourceItemID, placement.ItemID) {
				mandatory = append(mandatory, placement.InstanceID)
				if len(mandatory) >= 4 {
					break
				}
			}
		}
		if len(mandatory) == 0 {
			continue
		}
		optional := nearbyInstances(solution.Placements, placementByID, mandatory)
		appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, "loose-star-gap", 70000+len(mandatory), mandatory, optional)
	}
}

func addCraftGapNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	priorities []string,
	solutionIndex int,
) {
	activeCrafts := map[string]bool{}
	for _, craft := range solution.Evaluation.Crafts {
		activeCrafts[craft.RecipeResult] = true
	}
	for _, priority := range priorities {
		kind, result, ok := parsePriorityForSolver(priority)
		if !ok || kind != "craft" || activeCrafts[result] {
			continue
		}
		for _, recipe := range catalog.Recipes {
			if recipe.Result != result {
				continue
			}
			mandatory := instancesForRecipe(instances, placementByID, skipped, recipe)
			optional := nearbyInstances(solution.Placements, placementByID, mandatory)
			appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, "craft-gap", 60000+len(mandatory), mandatory, optional)
		}
	}
}

func addPackingPressureNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	priorities []string,
	coverage *coverageContext,
	solutionIndex int,
) {
	if len(skipped) == 0 {
		return
	}
	for _, instance := range skipped {
		priority := instancePriority(catalog, instance, priorities, coverage)
		if priority <= len(catalog.Items[instance.ItemID].Shape) {
			continue
		}
		mandatory := []string{instance.InstanceID}
		optional := blockingInstancesForOptions(solution.Placements, optionsByInstance[instance.InstanceID])
		appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, "packing-pressure", 50000+priority, mandatory, optional)
	}
	_ = placementByID
}

func runRepairNeighborhood(
	catalog model.Catalog,
	original []model.InventoryInstance,
	instanceByID map[string]model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	incumbents []model.Solution,
	neighborhood repairNeighborhood,
	config Config,
	coverage *coverageContext,
	gridMask uint64,
	nodeBudget int64,
	reportNodes func(int64),
) repairResult {
	if nodeBudget <= 0 || len(neighborhood.InstanceIDs) == 0 || len(incumbents) == 0 {
		return repairResult{}
	}
	base := incumbents[0]
	removeSet := stringSet(neighborhood.InstanceIDs)
	fixedPlacements := make([]model.Placement, 0, len(base.Placements))
	fixedOccupied := uint64(0)
	for _, placement := range base.Placements {
		if _, remove := removeSet[placement.InstanceID]; remove {
			continue
		}
		fixedPlacements = append(fixedPlacements, placement)
		fixedOccupied |= placement.Mask
	}

	repairInstances := make([]model.InventoryInstance, 0, len(neighborhood.InstanceIDs))
	for _, instanceID := range neighborhood.InstanceIDs {
		instance, ok := instanceByID[instanceID]
		if !ok {
			continue
		}
		repairInstances = append(repairInstances, instance)
	}
	if len(repairInstances) == 0 {
		return repairResult{}
	}
	sort.Slice(repairInstances, func(i, j int) bool {
		leftPriority := instancePriority(catalog, repairInstances[i], config.Priorities, coverage)
		rightPriority := instancePriority(catalog, repairInstances[j], config.Priorities, coverage)
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		leftOptions := len(optionsByInstance[repairInstances[i].InstanceID])
		rightOptions := len(optionsByInstance[repairInstances[j].InstanceID])
		if leftOptions != rightOptions {
			return leftOptions < rightOptions
		}
		return repairInstances[i].OriginalIndex < repairInstances[j].OriginalIndex
	})

	if coverage != nil {
		coverage.prepareOrder(repairInstances)
	}
	state := coverageSearchState{}
	if coverage != nil {
		state = coverage.initialState(catalog, repairInstances, 0, fixedPlacements)
	}

	currentPlacements := append([]model.Placement(nil), fixedPlacements...)
	sortPlacementsByOriginal(currentPlacements)
	exactBounds := newExactBoundContext(catalog, original, repairInstances, optionsByInstance, config)
	boundState := exactBoundState{}
	if exactBounds != nil {
		boundState = exactBounds.initialState(catalog, repairInstances, 0, fixedPlacements)
	}

	splitDepth := repairSplitDepth(repairInstances, optionsByInstance, config, nodeBudget)
	tasks := buildRepairSearchTasks(catalog, repairInstances, optionsByInstance, config.AllowSkips, coverage, exactBounds, fixedOccupied, currentPlacements, state, boundState, splitDepth)
	assignRepairBudgets(tasks, nodeBudget)
	result := runRepairTasks(catalog, original, repairInstances, optionsByInstance, tasks, incumbents, config, gridMask, coverage, exactBounds)
	if reportNodes != nil {
		reportNodes(result.NodesExplored)
	}
	return result
}

func repairSplitDepth(
	repairInstances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	nodeBudget int64,
) int {
	if config.Workers <= 1 || nodeBudget < repairMinParallelBudget || len(repairInstances) == 0 {
		return 0
	}
	maxDepth := repairMaxSplitDepth
	if len(repairInstances) < maxDepth {
		maxDepth = len(repairInstances)
	}
	targetTasks := config.Workers * repairTargetTasksPerWorker
	maxTasksByBudget := int(nodeBudget / repairMinParallelBudget)
	if maxTasksByBudget < 2 {
		return 0
	}
	if targetTasks > maxTasksByBudget {
		targetTasks = maxTasksByBudget
	}
	if targetTasks <= 1 {
		return 0
	}
	bestDepth := 0
	for depth := 1; depth <= maxDepth; depth++ {
		taskCount := estimateTaskCount(repairInstances, optionsByInstance, config.AllowSkips, depth)
		if taskCount == 0 || taskCount > repairMaxTasks {
			break
		}
		if int64(taskCount)*repairMinParallelBudget > nodeBudget {
			break
		}
		bestDepth = depth
		if taskCount >= targetTasks {
			break
		}
	}
	return bestDepth
}

func buildRepairSearchTasks(
	catalog model.Catalog,
	repairInstances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	allowSkips bool,
	coverage *coverageContext,
	exactBounds *exactBoundContext,
	occupied uint64,
	placements []model.Placement,
	state coverageSearchState,
	boundState exactBoundState,
	splitDepth int,
) []repairSearchTask {
	if len(repairInstances) < splitDepth {
		splitDepth = len(repairInstances)
	}
	var tasks []repairSearchTask
	var visit func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState)
	visit = func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState) {
		if index == len(repairInstances) || index >= splitDepth {
			tasks = append(tasks, repairSearchTask{
				Index:         index,
				Occupied:      occupied,
				Placements:    append([]model.Placement(nil), placements...),
				CoverageState: state,
				BoundState:    boundState,
			})
			return
		}
		instance := repairInstances[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			nextState := state
			if coverage != nil {
				nextState = coverage.withPlacement(catalog, state, option, placements)
			}
			nextBoundState := boundState
			if exactBounds != nil {
				nextBoundState = exactBounds.withPlacement(catalog, boundState, option, placements)
			}
			var insertedAt int
			placements, insertedAt = insertPlacementSorted(placements, option)
			visit(index+1, occupied|option.Mask, placements, nextState, nextBoundState)
			placements = removePlacementAt(placements, insertedAt)
		}
		if allowSkips {
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
	visit(0, occupied, append([]model.Placement(nil), placements...), state, boundState)
	return tasks
}

func assignRepairBudgets(tasks []repairSearchTask, nodeBudget int64) {
	if nodeBudget <= 0 || len(tasks) == 0 {
		return
	}
	base := nodeBudget / int64(len(tasks))
	remainder := nodeBudget % int64(len(tasks))
	for idx := range tasks {
		tasks[idx].HasNodeBudget = true
		tasks[idx].NodeBudget = base
		if int64(idx) < remainder {
			tasks[idx].NodeBudget++
		}
	}
}

func runRepairTasks(
	catalog model.Catalog,
	original []model.InventoryInstance,
	repairInstances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	tasks []repairSearchTask,
	incumbents []model.Solution,
	config Config,
	gridMask uint64,
	coverage *coverageContext,
	exactBounds *exactBoundContext,
) repairResult {
	if len(tasks) == 0 {
		return repairResult{}
	}
	if config.Workers <= 1 || len(tasks) <= 1 {
		var combined repairResult
		for _, task := range tasks {
			partial := runRepairTask(catalog, original, repairInstances, optionsByInstance, task, incumbents, config, gridMask, coverage, exactBounds)
			combined = mergeRepairTaskResult(combined, partial, config.TopN)
		}
		combined.ParallelTasks = len(tasks)
		combined.ParallelWorkersUsed = 1
		return combined
	}

	workerCount := config.Workers
	if workerCount > len(tasks) {
		workerCount = len(tasks)
	}
	jobs := make(chan repairJob)
	results := make(chan repairTaskRunResult, len(tasks))
	var waitGroup sync.WaitGroup
	for idx := 0; idx < workerCount; idx++ {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			for job := range jobs {
				results <- repairTaskRunResult{
					Index:  job.Index,
					Result: runRepairTask(catalog, original, repairInstances, optionsByInstance, job.Task, incumbents, config, gridMask, coverage, exactBounds),
				}
			}
		}()
	}
	for idx, task := range tasks {
		jobs <- repairJob{Index: idx, Task: task}
	}
	close(jobs)
	waitGroup.Wait()
	close(results)

	partials := make([]repairResult, len(tasks))
	received := make([]bool, len(tasks))
	for partial := range results {
		if partial.Index >= 0 && partial.Index < len(partials) {
			partials[partial.Index] = partial.Result
			received[partial.Index] = true
		}
	}
	var combined repairResult
	for idx, partial := range partials {
		if !received[idx] {
			continue
		}
		combined = mergeRepairTaskResult(combined, partial, config.TopN)
	}
	combined.ParallelTasks = len(tasks)
	combined.ParallelWorkersUsed = workerCount
	return combined
}

func runRepairTask(
	catalog model.Catalog,
	original []model.InventoryInstance,
	repairInstances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	task repairSearchTask,
	incumbents []model.Solution,
	config Config,
	gridMask uint64,
	coverage *coverageContext,
	exactBounds *exactBoundContext,
) repairResult {
	results := mergeSolutions(nil, incumbents, config.TopN)
	remainingCells := remainingCellCounts(catalog, repairInstances)
	var nodes int64
	var hitBudget bool
	var canceled bool
	var candidateCount int
	var coverageBoundChecks int64
	var coveragePrunedNodes int64
	var exactBoundChecks int64
	var exactBoundPrunedNodes int64

	var visit func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState)
	visit = func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState) {
		if canceled {
			return
		}
		if task.HasNodeBudget && nodes >= task.NodeBudget {
			hitBudget = true
			return
		}
		nodes++
		if config.Context != nil && nodes%progressNodeInterval == 0 && config.Context.Err() != nil {
			canceled = true
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
		if index == len(repairInstances) {
			candidateCount++
			results = insertCandidateWithScoreOnlyFilter(catalog, results, placements, original, config)
			return
		}

		instance := repairInstances[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			nextState := state
			if coverage != nil {
				nextState = coverage.withPlacement(catalog, state, option, placements)
			}
			nextBoundState := boundState
			if exactBounds != nil {
				nextBoundState = exactBounds.withPlacement(catalog, boundState, option, placements)
			}
			var insertedAt int
			placements, insertedAt = insertPlacementSorted(placements, option)
			visit(index+1, occupied|option.Mask, placements, nextState, nextBoundState)
			placements = removePlacementAt(placements, insertedAt)
			if hitBudget {
				break
			}
		}
		if config.AllowSkips && !hitBudget {
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

	visit(task.Index, task.Occupied, append([]model.Placement(nil), task.Placements...), task.CoverageState, task.BoundState)
	return repairResult{
		Solutions:             results,
		NodesExplored:         nodes,
		CandidateCount:        candidateCount,
		CoverageBoundChecks:   coverageBoundChecks,
		CoveragePrunedNodes:   coveragePrunedNodes,
		ExactBoundChecks:      exactBoundChecks,
		ExactBoundPrunedNodes: exactBoundPrunedNodes,
	}
}

func mergeRepairTaskResult(left repairResult, right repairResult, topN int) repairResult {
	left.Solutions = mergeSolutions(left.Solutions, right.Solutions, topN)
	left.NodesExplored += right.NodesExplored
	left.CandidateCount += right.CandidateCount
	left.CoverageBoundChecks += right.CoverageBoundChecks
	left.CoveragePrunedNodes += right.CoveragePrunedNodes
	left.ExactBoundChecks += right.ExactBoundChecks
	left.ExactBoundPrunedNodes += right.ExactBoundPrunedNodes
	return left
}

func appendRepairNeighborhood(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	solutionIndex int,
	operator string,
	priority int,
	mandatory []string,
	optional []string,
) {
	totalInstances := len(instances)
	minSize := minInt(3, totalInstances)
	ids := uniqueInstanceIDs(append(append([]string(nil), mandatory...), optional...))
	if len(ids) < minSize {
		return
	}
	if len(ids) > repairMaxNeighborhoodSize {
		ids = ids[:repairMaxNeighborhoodSize]
	}
	sort.SliceStable(ids, func(i, j int) bool {
		leftMandatory := containsString(mandatory, ids[i])
		rightMandatory := containsString(mandatory, ids[j])
		if leftMandatory != rightMandatory {
			return leftMandatory
		}
		return ids[i] < ids[j]
	})
	if len(ids) > repairMaxNeighborhoodSize {
		ids = ids[:repairMaxNeighborhoodSize]
	}
	key := repairNeighborhoodKey(solutionIndex, operator, ids)
	*neighborhoods = append(*neighborhoods, repairNeighborhood{
		Operator:    operator,
		InstanceIDs: ids,
		Priority:    priority + neighborhoodShapeWeight(catalog, instances, ids),
		Key:         key,
	})
}

func neighborhoodShapeWeight(catalog model.Catalog, instances []model.InventoryInstance, ids []string) int {
	instanceByID := map[string]model.InventoryInstance{}
	for _, instance := range instances {
		instanceByID[instance.InstanceID] = instance
	}
	weight := 0
	for _, id := range ids {
		instance, ok := instanceByID[id]
		if !ok {
			continue
		}
		weight += len(catalog.Items[instance.ItemID].Shape)
	}
	return weight
}

func repairNeighborhoodKey(solutionIndex int, operator string, ids []string) string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return fmt.Sprintf("%03d|%s|%s", solutionIndex, operator, strings.Join(sorted, ","))
}

func placementByInstanceID(placements []model.Placement) map[string]model.Placement {
	out := map[string]model.Placement{}
	for _, placement := range placements {
		out[placement.InstanceID] = placement
	}
	return out
}

func skippedInstances(instances []model.InventoryInstance, placementByID map[string]model.Placement) []model.InventoryInstance {
	var skipped []model.InventoryInstance
	for _, instance := range instances {
		if _, ok := placementByID[instance.InstanceID]; !ok {
			skipped = append(skipped, instance)
		}
	}
	return skipped
}

func matchingPlacedInstances(placements []model.Placement, itemSet map[string]struct{}) []string {
	var ids []string
	for _, placement := range placements {
		if _, ok := itemSet[placement.ItemID]; ok {
			ids = append(ids, placement.InstanceID)
		}
	}
	sort.Strings(ids)
	return ids
}

func matchingSkippedInstances(skipped []model.InventoryInstance, itemSet map[string]struct{}) []string {
	var ids []string
	for _, instance := range skipped {
		if _, ok := itemSet[instance.ItemID]; ok {
			ids = append(ids, instance.InstanceID)
		}
	}
	sort.Strings(ids)
	return ids
}

func underCoveredTargets(group model.StarCoverageBreakdown) []model.StarCoverageTarget {
	totalSources := len(group.Sources)
	var targets []model.StarCoverageTarget
	for _, target := range group.Targets {
		if target.CoveredCount < totalSources {
			targets = append(targets, target)
		}
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].CoveredCount != targets[j].CoveredCount {
			return targets[i].CoveredCount < targets[j].CoveredCount
		}
		return targets[i].TargetInstance < targets[j].TargetInstance
	})
	return targets
}

func nearbyInstances(placements []model.Placement, placementByID map[string]model.Placement, baseIDs []string) []string {
	var baseCells []model.Coord
	baseSet := stringSet(baseIDs)
	for _, id := range baseIDs {
		placement, ok := placementByID[id]
		if !ok {
			continue
		}
		baseCells = append(baseCells, placement.Cells...)
		for _, star := range placement.StarPositions {
			if geometry.InBounds(star.Position) {
				baseCells = append(baseCells, star.Position)
			}
		}
	}
	if len(baseCells) == 0 {
		return nil
	}
	type ranked struct {
		id       string
		distance int
	}
	var candidates []ranked
	for _, placement := range placements {
		if _, skip := baseSet[placement.InstanceID]; skip {
			continue
		}
		candidates = append(candidates, ranked{
			id:       placement.InstanceID,
			distance: placementDistance(placement, baseCells),
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].id < candidates[j].id
	})
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.id)
	}
	return ids
}

func placementDistance(placement model.Placement, cells []model.Coord) int {
	best := 999
	for _, cell := range placement.Cells {
		for _, other := range cells {
			distance := absInt(cell.Row-other.Row) + absInt(cell.Col-other.Col)
			if distance < best {
				best = distance
			}
		}
	}
	return best
}

func blockingInstancesForOptions(placements []model.Placement, options []model.Placement) []string {
	type ranked struct {
		id    string
		count int
	}
	counts := map[string]int{}
	limit := minInt(len(options), 12)
	for optionIndex := 0; optionIndex < limit; optionIndex++ {
		option := options[optionIndex]
		for _, placement := range placements {
			if option.Mask&placement.Mask != 0 {
				counts[placement.InstanceID]++
			}
		}
	}
	candidates := make([]ranked, 0, len(counts))
	for id, count := range counts {
		candidates = append(candidates, ranked{id: id, count: count})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].count != candidates[j].count {
			return candidates[i].count > candidates[j].count
		}
		return candidates[i].id < candidates[j].id
	})
	ids := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.id)
	}
	return ids
}

func looseCoveredTargets(solution model.Solution, sourceItemID string) map[string]bool {
	placementByID := placementByInstanceID(solution.Placements)
	targets := map[string]bool{}
	for _, star := range solution.Evaluation.Stars {
		source, ok := placementByID[star.SourceInstance]
		if !ok || source.ItemID != sourceItemID {
			continue
		}
		targets[star.TargetInstance] = true
	}
	return targets
}

func looseSourceCanTarget(catalog model.Catalog, sourceItemID string, targetItemID string) bool {
	source, ok := catalog.Items[sourceItemID]
	if !ok {
		return false
	}
	_, ok = catalog.Items[targetItemID]
	if !ok {
		return false
	}
	for starIndex := range source.Stars {
		if scoring.StarMatchesCatalogItems(catalog, sourceItemID, targetItemID, &source.Stars[starIndex]) {
			return true
		}
	}
	return false
}

func instancesForRecipe(
	instances []model.InventoryInstance,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	recipe model.Recipe,
) []string {
	available := append([]model.InventoryInstance(nil), instances...)
	sort.Slice(available, func(i, j int) bool {
		leftPlaced := instancePlaced(available[i], placementByID)
		rightPlaced := instancePlaced(available[j], placementByID)
		if leftPlaced != rightPlaced {
			return leftPlaced
		}
		return available[i].OriginalIndex < available[j].OriginalIndex
	})
	var ids []string
	if id := firstInstanceForItem(available, recipe.Anchor, nil); id != "" {
		ids = append(ids, id)
	}
	used := stringSet(ids)
	for _, ingredient := range recipe.Ingredients {
		if id := firstInstanceForItem(available, ingredient, used); id != "" {
			ids = append(ids, id)
			used[id] = struct{}{}
		}
	}
	_ = skipped
	return ids
}

func instancePlaced(instance model.InventoryInstance, placementByID map[string]model.Placement) bool {
	_, ok := placementByID[instance.InstanceID]
	return ok
}

func firstInstanceForItem(instances []model.InventoryInstance, itemID string, used map[string]struct{}) string {
	for _, instance := range instances {
		if instance.ItemID != itemID {
			continue
		}
		if _, ok := used[instance.InstanceID]; ok {
			continue
		}
		return instance.InstanceID
	}
	return ""
}

func uniqueInstanceIDs(ids []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func repairBestSummary(results []model.Solution) string {
	if coverage := seedBestSummary(results); coverage != "" {
		return coverage
	}
	if len(results) == 0 {
		return ""
	}
	score := results[0].Evaluation.Score
	if len(score.PriorityCounts) > 0 {
		parts := make([]string, 0, len(score.PriorityCounts))
		for _, count := range score.PriorityCounts {
			parts = append(parts, fmt.Sprintf("%d", count))
		}
		return "priority=" + strings.Join(parts, "/")
	}
	return fmt.Sprintf("crafts=%d, stars=%d, items=%d", score.CraftCount, score.StarCount, score.ItemCount)
}

func absInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
