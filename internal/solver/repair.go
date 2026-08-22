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
	repairEliteCandidateLimit  = 4
	repairTargetTasksPerWorker = 8
	repairMaxSplitDepth        = 2
	repairMaxTasks             = 8192
	repairMinParallelBudget    = int64(10000)
)

type repairNeighborhood struct {
	Operator      string
	InstanceIDs   []string
	MandatorySize int
	OptionalSize  int
	Priority      int
	Key           string
	BaseLayoutKey string
}

type repairSearchTask struct {
	Index         int
	Occupied      uint64
	Placements    []model.Placement
	CoverageState coverageSearchState
	BoundState    exactBoundState
	NodeBudget    int64
	HasNodeBudget bool
	PartialState  partialRepairState
}

type repairTaskRunResult struct {
	Index  int
	Result repairResult
}

type repairJob struct {
	Index int
	Task  repairSearchTask
}

func repairNodeBudget(policy *ResolvedSearchPolicy, maxNodes int64, seedNodes int64) int64 {
	if maxNodes <= 0 {
		return 0
	}
	remaining := maxNodes - seedNodes
	if remaining <= 0 {
		return 0
	}
	resolved := resolveSearchPolicy(Config{}, maxNodes)
	if policy != nil {
		resolved = *policy
	}
	budget := maxNodes * resolved.RepairBudgetPercent / 100
	if budget > remaining {
		budget = remaining
	}
	if budget < resolved.RepairMinNodeBudget {
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
	outgoingBounds *outgoingBoundContext,
	gridMask uint64,
	nodeBudget int64,
	initialSolutions []model.Solution,
	progress *progressTracker,
) repairResult {
	if nodeBudget <= 0 || len(initialSolutions) == 0 {
		reason := "no_budget"
		if len(initialSolutions) == 0 {
			reason = "no_seed_solution"
		}
		return repairResult{TerminationReason: reason}
	}

	results := mergeSolutions(nil, initialSolutions, config.TopN)
	if len(results) == 0 {
		return repairResult{TerminationReason: "no_seed_solution"}
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
	var outgoingBoundChecks int64
	var outgoingBoundPrunedNodes int64
	var symmetryPruned int64
	var parallelTasks int
	var parallelWorkersUsed int
	var neighborhoodsGenerated int
	var neighborhoodsAttempted int
	terminationReason := "budget_exhausted"

	for nodes < nodeBudget && !canceled {
		beforeBest := results[0]
		neighborhoods := buildRepairNeighborhoods(catalog, instances, optionsByInstance, results, config, coverage, seenNeighborhoods)
		neighborhoodsGenerated += len(neighborhoods)
		if len(neighborhoods) == 0 {
			if iterations == 0 {
				terminationReason = "no_eligible_neighborhoods"
			} else {
				terminationReason = "no_remaining_neighborhoods"
			}
			break
		}
		neighborhoodBudgets := allocateRepairNeighborhoodBudgets(neighborhoods, nodeBudget-nodes)

		roundImproved := false
		for _, neighborhood := range neighborhoods {
			if canceled || nodes >= nodeBudget {
				break
			}
			seenNeighborhoods[neighborhood.Key] = true
			iterations++
			neighborhoodsAttempted++
			quota := neighborhoodBudgets[neighborhood.Key]
			if quota <= 0 {
				continue
			}
			partial := runRepairNeighborhood(
				catalog,
				instances,
				instanceByID,
				optionsByInstance,
				results,
				neighborhood,
				config,
				coverage,
				outgoingBounds,
				gridMask,
				quota,
				reportNodes,
			)
			candidateCount += partial.CandidateCount
			coverageBoundChecks += partial.CoverageBoundChecks
			coveragePrunedNodes += partial.CoveragePrunedNodes
			exactBoundChecks += partial.ExactBoundChecks
			exactBoundPrunedNodes += partial.ExactBoundPrunedNodes
			outgoingBoundChecks += partial.OutgoingBoundChecks
			outgoingBoundPrunedNodes += partial.OutgoingBoundPrunedNodes
			symmetryPruned += partial.SymmetryPrunedBranches
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
			terminationReason = "no_improvement"
			break
		}
	}
	if canceled {
		terminationReason = "canceled"
	}

	flushProgress()
	return repairResult{
		Solutions:                results,
		NodesExplored:            nodes,
		Iterations:               iterations,
		Improvements:             improvements,
		CandidateCount:           candidateCount,
		BestSummary:              repairBestSummary(results),
		CoverageBoundChecks:      coverageBoundChecks,
		CoveragePrunedNodes:      coveragePrunedNodes,
		ExactBoundChecks:         exactBoundChecks,
		ExactBoundPrunedNodes:    exactBoundPrunedNodes,
		OutgoingBoundChecks:      outgoingBoundChecks,
		OutgoingBoundPrunedNodes: outgoingBoundPrunedNodes,
		SymmetryPrunedBranches:   symmetryPruned,
		ParallelTasks:            parallelTasks,
		ParallelWorkersUsed:      parallelWorkersUsed,
		NeighborhoodsGenerated:   neighborhoodsGenerated,
		NeighborhoodsAttempted:   neighborhoodsAttempted,
		TerminationReason:        terminationReason,
	}
}

// allocateRepairNeighborhoodBudgets reserves equal phase budget for each
// elite base layout, then splits that reservation across its neighborhoods.
// A high-priority neighborhood therefore cannot consume the entire LNS phase.
func allocateRepairNeighborhoodBudgets(neighborhoods []repairNeighborhood, total int64) map[string]int64 {
	budgets := make(map[string]int64, len(neighborhoods))
	if total <= 0 || len(neighborhoods) == 0 {
		return budgets
	}
	baseKeys := make([]string, 0, repairEliteCandidateLimit)
	counts := map[string]int64{}
	for _, neighborhood := range neighborhoods {
		if _, exists := counts[neighborhood.BaseLayoutKey]; !exists {
			baseKeys = append(baseKeys, neighborhood.BaseLayoutKey)
		}
		counts[neighborhood.BaseLayoutKey]++
	}
	baseBudget := total / int64(len(baseKeys))
	baseRemainder := total % int64(len(baseKeys))
	allocatedByBase := map[string]int64{}
	for index, key := range baseKeys {
		allocatedByBase[key] = baseBudget
		if int64(index) < baseRemainder {
			allocatedByBase[key]++
		}
	}
	seenByBase := map[string]int64{}
	for _, neighborhood := range neighborhoods {
		key := neighborhood.BaseLayoutKey
		share := allocatedByBase[key] / counts[key]
		if seenByBase[key] < allocatedByBase[key]%counts[key] {
			share++
		}
		seenByBase[key]++
		budgets[neighborhood.Key] = share
	}
	return budgets
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
	policy := policyForConfig(config)
	if len(solutions) > policy.RepairEliteCandidates {
		solutions = solutions[:policy.RepairEliteCandidates]
	}
	for solutionIndex, solution := range solutions {
		placementByID := placementByInstanceID(solution.Placements)
		skipped := skippedInstances(instances, placementByID)
		addCoverageGapNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, solutionIndex, policy.RepairMaxNeighborhoodSize)
		addSourceRetargetNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, solutionIndex, policy.RepairMaxNeighborhoodSize)
		addLooseStarNeighborhoods(&neighborhoods, catalog, instances, optionsByInstance, solution, placementByID, skipped, config, solutionIndex)
		addCraftGapNeighborhoods(&neighborhoods, catalog, instances, solution, placementByID, skipped, config.Priorities, solutionIndex, policy.RepairMaxNeighborhoodSize)
		addPackingPressureNeighborhoods(&neighborhoods, catalog, instances, optionsByInstance, solution, placementByID, skipped, config.Priorities, coverage, solutionIndex, policy.RepairMaxNeighborhoodSize)
	}

	sort.Slice(neighborhoods, func(i, j int) bool {
		if neighborhoods[i].Priority != neighborhoods[j].Priority {
			return neighborhoods[i].Priority > neighborhoods[j].Priority
		}
		return neighborhoods[i].Key < neighborhoods[j].Key
	})

	filtered := make([]repairNeighborhood, 0, minInt(len(neighborhoods), policy.RepairMaxNeighborhoods))
	localSeen := map[string]bool{}
	for _, neighborhood := range neighborhoods {
		if seen[neighborhood.Key] || localSeen[neighborhood.Key] {
			continue
		}
		localSeen[neighborhood.Key] = true
		filtered = append(filtered, neighborhood)
		if len(filtered) >= policy.RepairMaxNeighborhoods {
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
	maxSize int,
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
			appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, solution.LayoutKey, "coverage-gap", priority, mandatory, optional, maxSize)
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
	maxSize int,
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
			appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, solution.LayoutKey, "source-retarget", priority, mandatory, optional, maxSize)
		}
	}
}

func addLooseStarNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	config Config,
	solutionIndex int,
) {
	if config.PrioritySemantics == model.PrioritySemanticsOutgoingPerInstanceV3 {
		addPerInstanceLooseStarNeighborhoods(neighborhoods, catalog, instances, optionsByInstance, solution, placementByID, skipped, config, solutionIndex)
		return
	}
	seenSources := map[string]bool{}
	for _, priority := range config.Priorities {
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
		appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, solution.LayoutKey, "loose-star-gap", 70000+len(mandatory), mandatory, optional, policyForConfig(config).RepairMaxNeighborhoodSize)
	}
}

// addPerInstanceLooseStarNeighborhoods targets the coupled arrangement needed
// when several copies of a source need their own targets. It intentionally
// keeps all source copies and expands the candidate target set gradually,
// allowing repair to cross multi-item valleys that one-item refinement cannot.
func addPerInstanceLooseStarNeighborhoods(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	solution model.Solution,
	placementByID map[string]model.Placement,
	skipped []model.InventoryInstance,
	config Config,
	solutionIndex int,
) {
	seenSources := map[string]bool{}
	for priorityIndex, priority := range config.Priorities {
		kind, sourceItemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" || seenSources[sourceItemID] {
			continue
		}
		seenSources[sourceItemID] = true

		sourceIDs := append(matchingPlacedInstances(solution.Placements, map[string]struct{}{sourceItemID: {}}), matchingSkippedInstances(skipped, map[string]struct{}{sourceItemID: {}})...)
		maxSize := policyForConfig(config).RepairMaxNeighborhoodSize
		if len(sourceIDs) == 0 || len(sourceIDs) >= maxSize {
			continue
		}
		linksBySource := looseLinksBySource(solution, sourceItemID)
		maxLinks := len(catalog.Items[sourceItemID].Stars)
		deficit := 0
		for _, sourceID := range sourceIDs {
			if linksBySource[sourceID] < maxLinks {
				deficit += maxLinks - linksBySource[sourceID]
			}
		}
		if deficit == 0 {
			continue
		}

		targets := rankedPerInstanceTargets(catalog, instances, optionsByInstance, sourceItemID, sourceIDs, linksBySource, config.Priorities, priorityIndex)
		if len(targets) == 0 {
			continue
		}
		maxTargets := minInt(maxSize-len(sourceIDs)-1, len(targets))
		if maxTargets <= 0 {
			maxTargets = minInt(maxSize-len(sourceIDs), len(targets))
		}
		for _, subset := range perInstanceTargetSubsets(targets, sourceIDs, linksBySource, maxLinks, maxTargets) {
			mandatory := append([]string(nil), sourceIDs...)
			mandatory = append(mandatory, subset.ids...)
			optional := append(perInstanceBlockers(solution.Placements, optionsByInstance, mandatory), nearbyInstances(solution.Placements, placementByID, mandatory)...)
			priority := 90000 - priorityIndex*100 + deficit*100 + subset.score
			appendRepairNeighborhood(
				neighborhoods,
				catalog,
				instances,
				solutionIndex,
				solution.LayoutKey,
				"loose-per-instance-gap",
				priority,
				mandatory,
				optional,
				maxSize,
			)
		}
	}
}

func looseLinksBySource(solution model.Solution, sourceItemID string) map[string]int {
	placementByID := placementByInstanceID(solution.Placements)
	targetsBySource := map[string]map[string]struct{}{}
	for _, star := range solution.Evaluation.Stars {
		source, ok := placementByID[star.SourceInstance]
		if !ok || source.ItemID != sourceItemID {
			continue
		}
		if targetsBySource[star.SourceInstance] == nil {
			targetsBySource[star.SourceInstance] = map[string]struct{}{}
		}
		targetsBySource[star.SourceInstance][star.TargetInstance] = struct{}{}
	}
	counts := map[string]int{}
	for sourceID, targets := range targetsBySource {
		counts[sourceID] = len(targets)
	}
	return counts
}

type perInstanceTargetRank struct {
	id            string
	supportMask   uint64
	compatibility int
	laterValue    int
}

func rankedPerInstanceTargets(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	sourceItemID string,
	sourceIDs []string,
	linksBySource map[string]int,
	priorities []string,
	priorityIndex int,
) []perInstanceTargetRank {
	sourceSet := stringSet(sourceIDs)
	ranked := make([]perInstanceTargetRank, 0, len(instances))
	for _, target := range instances {
		if _, source := sourceSet[target.InstanceID]; source || !looseSourceCanTarget(catalog, sourceItemID, target.ItemID) {
			continue
		}
		rank := perInstanceTargetRank{id: target.InstanceID}
		for sourceIndex, sourceID := range sourceIDs {
			if linksBySource[sourceID] >= len(catalog.Items[sourceItemID].Stars) {
				continue
			}
			compatibility := 1
			if optionsByInstance != nil {
				compatibility = directedOptionCompatibility(catalog, optionsByInstance[sourceID], optionsByInstance[target.InstanceID])
			}
			if compatibility == 0 {
				continue
			}
			rank.supportMask |= uint64(1) << uint(sourceIndex)
			rank.compatibility += compatibility
		}
		for laterIndex := priorityIndex + 1; laterIndex < len(priorities); laterIndex++ {
			kind, itemID, ok := parsePriorityForSolver(priorities[laterIndex])
			if ok && kind == "star_source" && itemID == target.ItemID {
				rank.laterValue += len(priorities) - laterIndex
			}
		}
		if rank.supportMask != 0 {
			ranked = append(ranked, rank)
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		leftScore := bits.OnesCount64(ranked[i].supportMask)*10_000 + ranked[i].compatibility*10 + ranked[i].laterValue
		rightScore := bits.OnesCount64(ranked[j].supportMask)*10_000 + ranked[j].compatibility*10 + ranked[j].laterValue
		if leftScore != rightScore {
			return leftScore > rightScore
		}
		return ranked[i].id < ranked[j].id
	})
	return ranked
}

type perInstanceTargetSubset struct {
	ids   []string
	score int
}

func perInstanceTargetSubsets(
	targets []perInstanceTargetRank,
	sourceIDs []string,
	linksBySource map[string]int,
	maxLinks int,
	maxTargets int,
) []perInstanceTargetSubset {
	if maxTargets <= 0 {
		return nil
	}
	if len(targets) > 8 {
		targets = targets[:8]
	}
	if maxTargets > len(targets) {
		maxTargets = len(targets)
	}
	var candidates []perInstanceTargetSubset
	var choose func(start int, remaining int, selected []perInstanceTargetRank)
	choose = func(start int, remaining int, selected []perInstanceTargetRank) {
		if remaining == 0 {
			var supported uint64
			compatibility := 0
			laterValue := 0
			ids := make([]string, 0, len(selected))
			for _, target := range selected {
				supported |= target.supportMask
				compatibility += target.compatibility
				laterValue += target.laterValue
				ids = append(ids, target.id)
			}
			closable := 0
			for sourceIndex, sourceID := range sourceIDs {
				if supported&(uint64(1)<<uint(sourceIndex)) == 0 {
					continue
				}
				missing := maxLinks - linksBySource[sourceID]
				if missing > len(selected) {
					missing = len(selected)
				}
				closable += missing
			}
			candidates = append(candidates, perInstanceTargetSubset{
				ids:   ids,
				score: bits.OnesCount64(supported)*10_000 + closable*1_000 + compatibility*10 + laterValue,
			})
			return
		}
		for index := start; index <= len(targets)-remaining; index++ {
			choose(index+1, remaining-1, append(selected, targets[index]))
		}
	}
	for size := 1; size <= maxTargets; size++ {
		choose(0, size, nil)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return strings.Join(candidates[i].ids, ",") < strings.Join(candidates[j].ids, ",")
	})
	if len(candidates) > 16 {
		candidates = candidates[:16]
	}
	return candidates
}

func directedOptionCompatibility(catalog model.Catalog, sources []model.Placement, targets []model.Placement) int {
	compatible := 0
	for _, source := range sources {
		for _, target := range targets {
			if source.Mask&target.Mask == 0 && sourceHitsTargetWithCatalog(catalog, source, target) {
				compatible++
			}
		}
	}
	return compatible
}

func perInstanceBlockers(placements []model.Placement, optionsByInstance map[string][]model.Placement, mandatory []string) []string {
	mandatorySet := stringSet(mandatory)
	var blockers []string
	for _, instanceID := range mandatory {
		blockers = append(blockers, blockingInstancesForOptions(placements, optionsByInstance[instanceID])...)
	}
	blockers = uniqueInstanceIDs(blockers)
	filtered := blockers[:0]
	for _, blocker := range blockers {
		if _, mandatory := mandatorySet[blocker]; !mandatory {
			filtered = append(filtered, blocker)
		}
	}
	return filtered
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
	maxSize int,
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
			appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, solution.LayoutKey, "craft-gap", 60000+len(mandatory), mandatory, optional, maxSize)
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
	maxSize int,
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
		appendRepairNeighborhood(neighborhoods, catalog, instances, solutionIndex, solution.LayoutKey, "packing-pressure", 50000+priority, mandatory, optional, maxSize)
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
	outgoingBounds *outgoingBoundContext,
	gridMask uint64,
	nodeBudget int64,
	reportNodes func(int64),
) repairResult {
	if nodeBudget <= 0 || len(neighborhood.InstanceIDs) == 0 || len(incumbents) == 0 {
		return repairResult{}
	}
	base, found := solutionForRepairNeighborhood(incumbents, neighborhood)
	if !found {
		return repairResult{}
	}
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

	partialState := partialRepairState{
		FixedPlacements:  append([]model.Placement(nil), fixedPlacements...),
		RemovedInstances: append([]model.InventoryInstance(nil), repairInstances...),
		FreeCells:        gridMask &^ fixedOccupied,
	}
	splitDepth := repairSplitDepth(repairInstances, optionsByInstance, config, nodeBudget)
	tasks, splitSymmetryPruned := buildRepairSearchTasks(catalog, repairInstances, optionsByInstance, config.AllowSkips, coverage, exactBounds, fixedOccupied, currentPlacements, state, boundState, partialState, splitDepth)
	assignRepairBudgets(tasks, nodeBudget)
	result := runRepairTasks(catalog, original, repairInstances, optionsByInstance, tasks, incumbents, config, gridMask, coverage, exactBounds, outgoingBounds)
	result.SymmetryPrunedBranches += splitSymmetryPruned
	if reportNodes != nil {
		reportNodes(result.NodesExplored)
	}
	return result
}

func solutionForRepairNeighborhood(incumbents []model.Solution, neighborhood repairNeighborhood) (model.Solution, bool) {
	for _, solution := range incumbents {
		if solution.LayoutKey == neighborhood.BaseLayoutKey {
			return solution, true
		}
	}
	return model.Solution{}, false
}

func repairSplitDepth(
	repairInstances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	nodeBudget int64,
) int {
	policy := policyForConfig(config)
	if config.Workers <= 1 || nodeBudget < policy.RepairMinParallelBudget || len(repairInstances) == 0 {
		return 0
	}
	maxDepth := policy.RepairMaxSplitDepth
	if len(repairInstances) < maxDepth {
		maxDepth = len(repairInstances)
	}
	targetTasks := config.Workers * policy.RepairTargetTasksPerWorker
	maxTasksByBudget := int(nodeBudget / policy.RepairMinParallelBudget)
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
		if taskCount == 0 || taskCount > policy.RepairMaxTasks {
			break
		}
		if int64(taskCount)*policy.RepairMinParallelBudget > nodeBudget {
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
	partialState partialRepairState,
	splitDepth int,
) ([]repairSearchTask, int64) {
	if len(repairInstances) < splitDepth {
		splitDepth = len(repairInstances)
	}
	var tasks []repairSearchTask
	var symmetryPruned int64
	var visit func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState, partialState partialRepairState)
	visit = func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState, partialState partialRepairState) {
		if index == len(repairInstances) || index >= splitDepth {
			tasks = append(tasks, repairSearchTask{
				Index:         index,
				Occupied:      occupied,
				Placements:    append([]model.Placement(nil), placements...),
				CoverageState: state,
				BoundState:    boundState,
				PartialState:  partialState,
			})
			return
		}
		instance := repairInstances[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			if !placementRespectsCanonicalCopyOrder(option, placements) {
				symmetryPruned++
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
			nextPartialState := partialState
			nextPartialState.CurrentPlacements = append(append([]model.Placement(nil), partialState.CurrentPlacements...), option)
			nextPartialState.FreeCells &^= option.Mask
			visit(index+1, occupied|option.Mask, placements, nextState, nextBoundState, nextPartialState)
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
			visit(index+1, occupied, placements, nextState, nextBoundState, partialState)
		}
	}
	visit(0, occupied, append([]model.Placement(nil), placements...), state, boundState, partialState)
	return tasks, symmetryPruned
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
	outgoingBounds *outgoingBoundContext,
) repairResult {
	if len(tasks) == 0 {
		return repairResult{}
	}
	if config.Workers <= 1 || len(tasks) <= 1 {
		var combined repairResult
		for _, task := range tasks {
			partial := runRepairTask(catalog, original, repairInstances, optionsByInstance, task, incumbents, config, gridMask, coverage, exactBounds, outgoingBounds)
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
					Result: runRepairTask(catalog, original, repairInstances, optionsByInstance, job.Task, incumbents, config, gridMask, coverage, exactBounds, outgoingBounds),
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
	outgoingBounds *outgoingBoundContext,
) repairResult {
	results := mergeSolutions(nil, incumbents, config.TopN)
	remainingCells := remainingCellCounts(catalog, repairInstances)
	var nodes int64
	var hitBudget bool
	var canceled bool
	var candidateCount int
	var priorityPreservingCandidates int64
	var priorityBoundPruned int64
	var completedBelowPriority int64
	var compareScoreImprovements int64
	var coverageBoundChecks int64
	var coveragePrunedNodes int64
	var exactBoundChecks int64
	var exactBoundPrunedNodes int64
	var outgoingBoundChecks int64
	var outgoingBoundPrunedNodes int64
	var symmetryPruned int64

	var visit func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState, partialState partialRepairState)
	visit = func(index int, occupied uint64, placements []model.Placement, state coverageSearchState, boundState exactBoundState, partialState partialRepairState) {
		if canceled {
			return
		}
		if task.HasNodeBudget && nodes >= task.NodeBudget {
			hitBudget = true
			return
		}
		if !chargeNode(config, config.tracePhase) {
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
		if len(config.repairPriorityTarget) > 0 {
			upper := partialRepairV3PriorityUpperBound(catalog, partialState, optionsByInstance, config.Priorities)
			if !partialRepairTargetVectorFeasible(upper, config.repairPriorityTarget) {
				priorityBoundPruned++
				return
			}
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
			if outgoingBounds.shouldPrune(placements, results, config.TopN) {
				outgoingBoundPrunedNodes++
				return
			}
		}
		if index == len(repairInstances) {
			candidateCount++
			score := evaluateScoreForConfig(catalog, placements, config)
			if len(config.repairPriorityTarget) > 0 && comparePriorityCounts(score.PriorityCounts, config.repairPriorityTarget) < 0 {
				completedBelowPriority++
				return
			}
			if len(config.repairPriorityTarget) > 0 || (config.priorityBounds != nil && config.priorityBounds.reached(score)) {
				priorityPreservingCandidates++
			}
			before := bestScore(results)
			results = insertCandidateWithScoreOnlyFilter(catalog, results, placements, original, config)
			if compareScores(bestScore(results), before) > 0 {
				compareScoreImprovements++
			}
			return
		}

		instance := repairInstances[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			if !placementRespectsCanonicalCopyOrder(option, placements) {
				symmetryPruned++
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
			nextPartialState := partialState
			nextPartialState.CurrentPlacements = append(append([]model.Placement(nil), partialState.CurrentPlacements...), option)
			nextPartialState.FreeCells &^= option.Mask
			visit(index+1, occupied|option.Mask, placements, nextState, nextBoundState, nextPartialState)
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
			visit(index+1, occupied, placements, nextState, nextBoundState, partialState)
		}
	}

	visit(task.Index, task.Occupied, append([]model.Placement(nil), task.Placements...), task.CoverageState, task.BoundState, task.PartialState)
	return repairResult{
		Solutions:                    results,
		NodesExplored:                nodes,
		CandidateCount:               candidateCount,
		PriorityPreservingCandidates: priorityPreservingCandidates,
		PriorityBoundPruned:          priorityBoundPruned,
		CompletedBelowPriority:       completedBelowPriority,
		CompareScoreImprovements:     compareScoreImprovements,
		CoverageBoundChecks:          coverageBoundChecks,
		CoveragePrunedNodes:          coveragePrunedNodes,
		ExactBoundChecks:             exactBoundChecks,
		ExactBoundPrunedNodes:        exactBoundPrunedNodes,
		OutgoingBoundChecks:          outgoingBoundChecks,
		OutgoingBoundPrunedNodes:     outgoingBoundPrunedNodes,
		SymmetryPrunedBranches:       symmetryPruned,
	}
}

func mergeRepairTaskResult(left repairResult, right repairResult, topN int) repairResult {
	left.Solutions = mergeSolutions(left.Solutions, right.Solutions, topN)
	left.NodesExplored += right.NodesExplored
	left.CandidateCount += right.CandidateCount
	left.PriorityPreservingCandidates += right.PriorityPreservingCandidates
	left.PriorityBoundPruned += right.PriorityBoundPruned
	left.CompletedBelowPriority += right.CompletedBelowPriority
	left.CompareScoreImprovements += right.CompareScoreImprovements
	left.NeighborhoodsGenerated += right.NeighborhoodsGenerated
	left.NeighborhoodsAttempted += right.NeighborhoodsAttempted
	if right.TerminationReason != "" {
		left.TerminationReason = right.TerminationReason
	}
	left.CoverageBoundChecks += right.CoverageBoundChecks
	left.CoveragePrunedNodes += right.CoveragePrunedNodes
	left.ExactBoundChecks += right.ExactBoundChecks
	left.ExactBoundPrunedNodes += right.ExactBoundPrunedNodes
	left.OutgoingBoundChecks += right.OutgoingBoundChecks
	left.OutgoingBoundPrunedNodes += right.OutgoingBoundPrunedNodes
	left.SymmetryPrunedBranches += right.SymmetryPrunedBranches
	return left
}

func mergeRepairPhases(left repairResult, right repairResult, topN int) repairResult {
	merged := mergeRepairTaskResult(left, right, topN)
	merged.Iterations = left.Iterations + right.Iterations
	merged.Improvements = left.Improvements + right.Improvements
	if right.ParallelWorkersUsed > merged.ParallelWorkersUsed {
		merged.ParallelWorkersUsed = right.ParallelWorkersUsed
	}
	merged.ParallelTasks = left.ParallelTasks + right.ParallelTasks
	merged.BestSummary = repairBestSummary(merged.Solutions)
	return merged
}

func appendRepairNeighborhood(
	neighborhoods *[]repairNeighborhood,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	solutionIndex int,
	baseLayoutKey string,
	operator string,
	priority int,
	mandatory []string,
	optional []string,
	maxSize int,
) {
	totalInstances := len(instances)
	minSize := minInt(3, totalInstances)
	mandatory = uniqueInstanceIDs(mandatory)
	if len(mandatory) > maxSize {
		return
	}
	ids := append([]string(nil), mandatory...)
	for _, optionalID := range uniqueInstanceIDs(optional) {
		if len(ids) >= maxSize {
			break
		}
		ids = uniqueInstanceIDs(append(ids, optionalID))
	}
	if len(ids) < minSize {
		return
	}
	sort.SliceStable(ids, func(i, j int) bool {
		leftMandatory := containsString(mandatory, ids[i])
		rightMandatory := containsString(mandatory, ids[j])
		if leftMandatory != rightMandatory {
			return leftMandatory
		}
		return ids[i] < ids[j]
	})
	key := repairNeighborhoodKey(solutionIndex, operator, ids)
	*neighborhoods = append(*neighborhoods, repairNeighborhood{
		Operator:      operator,
		InstanceIDs:   ids,
		Priority:      priority + neighborhoodShapeWeight(catalog, instances, ids),
		Key:           key,
		BaseLayoutKey: baseLayoutKey,
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
