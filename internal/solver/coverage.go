package solver

import (
	"math/bits"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

type coverageContext struct {
	enabled                  bool
	sourceItemIDs            []string
	sourceIndexByItemID      map[string]int
	sourceMaskByOriginal     [64]uint64
	targetIndexByOriginal    [64]int
	targetPossibleSourceMask [64]uint64
	coverageCeiling          []model.StarCoverageBucket
	ceilingCounts            []int
	coveragePriorityStart    int
	pruningEnabled           bool
	placementPriority        map[string]int
	remainingSourceMask      []uint64
}

type coverageSearchState struct {
	targetCoverage    [64]uint64
	targetPlacedMask  uint64
	targetDecidedMask uint64
}

func newCoverageContext(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	priorities []string,
) *coverageContext {
	sources := priorityStarSourcesForSolver(priorities)
	return newCoverageContextFromSources(catalog, instances, optionsByInstance, sources, nil, coveragePriorityStart(priorities))
}

func newCoverageContextForConfig(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
) *coverageContext {
	if len(config.CoverageGroups) > 0 {
		if globalPriorityOrderEnabledForSolver(config.Priorities) {
			groupIndex, ok := firstPriorityCoverageGroupIndex(config.Priorities)
			if !ok || groupIndex < 0 || groupIndex >= len(config.CoverageGroups) {
				return nil
			}
			return newCoverageContextFromSources(catalog, instances, optionsByInstance, config.CoverageGroups[groupIndex].Sources, config.CoverageGroups[groupIndex].Targets, 0)
		}
		if config.PrioritySemantics.IsOutgoing() {
			return nil
		}
		return newCoverageContextFromSources(catalog, instances, optionsByInstance, config.CoverageGroups[0].Sources, config.CoverageGroups[0].Targets, 0)
	}
	if config.PrioritySemantics.IsOutgoing() {
		return nil
	}
	return newCoverageContext(catalog, instances, optionsByInstance, config.Priorities)
}

func newCoverageContextFromSources(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	sources []string,
	targets []string,
	priorityStart int,
) *coverageContext {
	sources = uniqueCoverageSourcesForSolver(sources)
	if len(sources) == 0 {
		return nil
	}
	targetFilter := uniqueCoverageSourcesForSolver(targets)
	targetFilterSet := map[string]struct{}{}
	for _, target := range targetFilter {
		targetFilterSet[target] = struct{}{}
	}

	ctx := &coverageContext{
		enabled:               true,
		sourceItemIDs:         sources,
		sourceIndexByItemID:   map[string]int{},
		coveragePriorityStart: priorityStart,
		placementPriority:     map[string]int{},
	}
	for idx := range ctx.targetIndexByOriginal {
		ctx.targetIndexByOriginal[idx] = -1
	}
	for idx, itemID := range sources {
		ctx.sourceIndexByItemID[itemID] = idx
	}

	var availableSourceMask uint64
	for _, instance := range instances {
		sourceMask := ctx.sourceMaskForItem(instance.ItemID)
		if sourceMask == 0 {
			continue
		}
		ctx.sourceMaskByOriginal[instance.OriginalIndex] = sourceMask
		availableSourceMask |= sourceMask
	}

	targetCount := 0
	for _, instance := range instances {
		if len(targetFilterSet) > 0 {
			if _, ok := targetFilterSet[instance.ItemID]; !ok {
				continue
			}
		}
		possibleMask := ctx.possibleSourceMaskForTarget(catalog, instance.ItemID) & availableSourceMask
		if possibleMask == 0 && len(targetFilterSet) == 0 {
			continue
		}
		ctx.targetIndexByOriginal[instance.OriginalIndex] = targetCount
		ctx.targetPossibleSourceMask[targetCount] = possibleMask
		targetCount++
	}

	ctx.ceilingCounts = coverageCountsFromTargetMasks(ctx.targetPossibleSourceMask[:targetCount], len(ctx.sourceItemIDs))
	ctx.coverageCeiling = coverageBucketsFromCounts(ctx.ceilingCounts, len(ctx.sourceItemIDs))
	ctx.pruningEnabled = ctx.coveragePriorityStart == 0
	ctx.precomputePlacementPriorities(catalog, instances, optionsByInstance)
	return ctx
}

func (ctx *coverageContext) targetCount() int {
	if ctx == nil {
		return 0
	}
	count := 0
	for _, targetIndex := range ctx.targetIndexByOriginal {
		if targetIndex >= count {
			count = targetIndex + 1
		}
	}
	return count
}

func (ctx *coverageContext) prepareOrder(ordered []model.InventoryInstance) {
	if ctx == nil || !ctx.enabled {
		return
	}
	ctx.remainingSourceMask = make([]uint64, len(ordered)+1)
	var remaining uint64
	counts := make([]int, len(ctx.sourceItemIDs))
	for _, instance := range ordered {
		sourceMask := ctx.sourceMaskByOriginal[instance.OriginalIndex]
		for sourceIndex := 0; sourceIndex < len(ctx.sourceItemIDs); sourceIndex++ {
			if sourceMask&(uint64(1)<<uint(sourceIndex)) != 0 {
				counts[sourceIndex]++
				remaining |= uint64(1) << uint(sourceIndex)
			}
		}
	}
	ctx.remainingSourceMask[0] = remaining
	for idx, instance := range ordered {
		sourceMask := ctx.sourceMaskByOriginal[instance.OriginalIndex]
		for sourceIndex := 0; sourceIndex < len(ctx.sourceItemIDs); sourceIndex++ {
			bit := uint64(1) << uint(sourceIndex)
			if sourceMask&bit == 0 {
				continue
			}
			counts[sourceIndex]--
			if counts[sourceIndex] == 0 {
				remaining &^= bit
			}
		}
		ctx.remainingSourceMask[idx+1] = remaining
	}
}

func (ctx *coverageContext) initialState(catalog model.Catalog, ordered []model.InventoryInstance, index int, placements []model.Placement) coverageSearchState {
	var state coverageSearchState
	if ctx == nil || !ctx.enabled {
		return state
	}
	for orderedIndex := 0; orderedIndex < index && orderedIndex < len(ordered); orderedIndex++ {
		targetIndex := ctx.targetIndexByOriginal[ordered[orderedIndex].OriginalIndex]
		if targetIndex >= 0 {
			state.targetDecidedMask |= uint64(1) << uint(targetIndex)
		}
	}
	for _, placement := range placements {
		targetIndex := ctx.targetIndexByOriginal[placement.OriginalIndex]
		if targetIndex >= 0 {
			state.targetDecidedMask |= uint64(1) << uint(targetIndex)
			state.targetPlacedMask |= uint64(1) << uint(targetIndex)
		}
	}
	for _, source := range placements {
		sourceMask := ctx.sourceMaskByOriginal[source.OriginalIndex]
		if sourceMask == 0 {
			continue
		}
		for _, target := range placements {
			targetIndex := ctx.targetIndexByOriginal[target.OriginalIndex]
			if targetIndex < 0 || source.InstanceID == target.InstanceID {
				continue
			}
			if sourceHitsTargetWithCatalog(catalog, source, target) {
				state.targetCoverage[targetIndex] |= sourceMask
			}
		}
	}
	return state
}

func (ctx *coverageContext) withPlacement(catalog model.Catalog, state coverageSearchState, placement model.Placement, existing []model.Placement) coverageSearchState {
	if ctx == nil || !ctx.enabled {
		return state
	}
	targetIndex := ctx.targetIndexByOriginal[placement.OriginalIndex]
	if targetIndex >= 0 {
		state.targetDecidedMask |= uint64(1) << uint(targetIndex)
		state.targetPlacedMask |= uint64(1) << uint(targetIndex)
		for _, source := range existing {
			sourceMask := ctx.sourceMaskByOriginal[source.OriginalIndex]
			if sourceMask != 0 && sourceHitsTargetWithCatalog(catalog, source, placement) {
				state.targetCoverage[targetIndex] |= sourceMask
			}
		}
	}

	sourceMask := ctx.sourceMaskByOriginal[placement.OriginalIndex]
	if sourceMask != 0 {
		for _, target := range existing {
			targetIndex := ctx.targetIndexByOriginal[target.OriginalIndex]
			if targetIndex >= 0 && sourceHitsTargetWithCatalog(catalog, placement, target) {
				state.targetCoverage[targetIndex] |= sourceMask
			}
		}
	}
	return state
}

func (ctx *coverageContext) withSkip(state coverageSearchState, instance model.InventoryInstance) coverageSearchState {
	if ctx == nil || !ctx.enabled {
		return state
	}
	targetIndex := ctx.targetIndexByOriginal[instance.OriginalIndex]
	if targetIndex >= 0 {
		state.targetDecidedMask |= uint64(1) << uint(targetIndex)
	}
	return state
}

func (ctx *coverageContext) upperBoundCounts(state coverageSearchState, index int) []int {
	if ctx == nil || !ctx.enabled {
		return nil
	}
	remainingSources := uint64(0)
	if index >= 0 && index < len(ctx.remainingSourceMask) {
		remainingSources = ctx.remainingSourceMask[index]
	}
	targetCount := ctx.targetCount()
	targetMasks := make([]uint64, 0, targetCount)
	for targetIndex := 0; targetIndex < targetCount; targetIndex++ {
		targetBit := uint64(1) << uint(targetIndex)
		placed := state.targetPlacedMask&targetBit != 0
		decided := state.targetDecidedMask&targetBit != 0
		if decided && !placed {
			continue
		}
		if placed {
			targetMasks = append(targetMasks, state.targetCoverage[targetIndex]|(ctx.targetPossibleSourceMask[targetIndex]&remainingSources))
		} else {
			targetMasks = append(targetMasks, ctx.targetPossibleSourceMask[targetIndex])
		}
	}
	return coverageCountsFromTargetMasks(targetMasks, len(ctx.sourceItemIDs))
}

func (ctx *coverageContext) shouldPrune(state coverageSearchState, index int, results []model.Solution, topN int) bool {
	if ctx == nil || !ctx.enabled || !ctx.pruningEnabled || len(results) < topN || topN <= 0 {
		return false
	}
	boundCounts := ctx.upperBoundCounts(state, index)
	worst := results[len(results)-1]
	if comparePriorityCounts(boundCounts, worst.Evaluation.Score.PriorityCounts[:minInt(len(worst.Evaluation.Score.PriorityCounts), len(boundCounts))]) < 0 {
		return true
	}
	return false
}

func (ctx *coverageContext) ceilingReached(score model.Score) bool {
	if ctx == nil || !ctx.enabled || len(ctx.ceilingCounts) == 0 || len(score.PriorityCounts) == 0 {
		return false
	}
	if ctx.coveragePriorityStart != 0 {
		return false
	}
	if ctx.ceilingCounts[0] <= 0 {
		return false
	}
	return score.PriorityCounts[0] >= ctx.ceilingCounts[0]
}

func (ctx *coverageContext) sourceMaskForItem(itemID string) uint64 {
	if ctx == nil {
		return 0
	}
	sourceIndex, ok := ctx.sourceIndexByItemID[itemID]
	if !ok {
		return 0
	}
	return uint64(1) << uint(sourceIndex)
}

func (ctx *coverageContext) possibleSourceMaskForTarget(catalog model.Catalog, itemID string) uint64 {
	_, ok := catalog.Items[itemID]
	if !ok {
		return 0
	}
	var mask uint64
	for sourceIndex, sourceID := range ctx.sourceItemIDs {
		source, ok := catalog.Items[sourceID]
		if !ok {
			continue
		}
		for starIndex := range source.Stars {
			if scoring.StarMatchesCatalogItems(catalog, sourceID, itemID, &source.Stars[starIndex]) {
				mask |= uint64(1) << uint(sourceIndex)
				break
			}
		}
	}
	return mask
}

func (ctx *coverageContext) precomputePlacementPriorities(catalog model.Catalog, instances []model.InventoryInstance, optionsByInstance map[string][]model.Placement) {
	if ctx == nil || !ctx.enabled {
		return
	}
	for _, instance := range instances {
		for _, option := range optionsByInstance[instance.InstanceID] {
			ctx.placementPriority[coveragePlacementKey(option)] = ctx.basePlacementPriority(option)
		}
	}

	// A successful source-to-target pairing improves both placement priorities.
	// Calculate it once rather than rediscovering it while scoring each endpoint.
	for _, sourceInstance := range instances {
		if ctx.sourceMaskByOriginal[sourceInstance.OriginalIndex] == 0 {
			continue
		}
		for _, targetInstance := range instances {
			if sourceInstance.InstanceID == targetInstance.InstanceID || ctx.targetIndexByOriginal[targetInstance.OriginalIndex] < 0 {
				continue
			}
			for _, sourceOption := range optionsByInstance[sourceInstance.InstanceID] {
				for _, targetOption := range optionsByInstance[targetInstance.InstanceID] {
					if sourceOption.Mask&targetOption.Mask != 0 || !sourceHitsTargetWithCatalog(catalog, sourceOption, targetOption) {
						continue
					}
					ctx.placementPriority[coveragePlacementKey(sourceOption)]++
					ctx.placementPriority[coveragePlacementKey(targetOption)]++
				}
			}
		}
	}
}

func (ctx *coverageContext) basePlacementPriority(option model.Placement) int {
	priority := 0
	sourceMask := ctx.sourceMaskByOriginal[option.OriginalIndex]
	targetIndex := ctx.targetIndexByOriginal[option.OriginalIndex]
	if sourceMask != 0 {
		priority += 100000
	}
	if targetIndex >= 0 {
		priority += 50000 + bits.OnesCount64(ctx.targetPossibleSourceMask[targetIndex])*1000
	}
	if sourceMask == 0 && targetIndex < 0 {
		return priority
	}
	return priority
}

func (ctx *coverageContext) computePlacementPriority(catalog model.Catalog, option model.Placement, instances []model.InventoryInstance, optionsByInstance map[string][]model.Placement) int {
	priority := ctx.basePlacementPriority(option)
	sourceMask := ctx.sourceMaskByOriginal[option.OriginalIndex]
	targetIndex := ctx.targetIndexByOriginal[option.OriginalIndex]
	if sourceMask == 0 && targetIndex < 0 {
		return priority
	}
	hits := 0
	for _, otherInstance := range instances {
		if otherInstance.InstanceID == option.InstanceID {
			continue
		}
		for _, otherOption := range optionsByInstance[otherInstance.InstanceID] {
			if option.Mask&otherOption.Mask != 0 {
				continue
			}
			if sourceMask != 0 && ctx.targetIndexByOriginal[otherOption.OriginalIndex] >= 0 && sourceHitsTargetWithCatalog(catalog, option, otherOption) {
				hits++
			}
			if targetIndex >= 0 && ctx.sourceMaskByOriginal[otherOption.OriginalIndex] != 0 && sourceHitsTargetWithCatalog(catalog, otherOption, option) {
				hits++
			}
		}
	}
	return priority + hits
}

func (ctx *coverageContext) priorityForPlacement(placement model.Placement) int {
	if ctx == nil || !ctx.enabled {
		return 0
	}
	return ctx.placementPriority[coveragePlacementKey(placement)]
}

func priorityStarSourcesForSolver(priorities []string) []string {
	var sources []string
	for _, priority := range priorities {
		kind, value, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" || containsString(sources, value) {
			continue
		}
		sources = append(sources, value)
	}
	return sources
}

func uniqueCoverageSourcesForSolver(values []string) []string {
	var sources []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || containsString(sources, value) {
			continue
		}
		sources = append(sources, value)
	}
	return sources
}

func coveragePriorityStart(priorities []string) int {
	position := 0
	for _, priority := range priorities {
		kind, _, ok := parsePriorityForSolver(priority)
		if !ok {
			position++
			continue
		}
		if kind == "star_source" {
			return position
		}
		position++
	}
	return 0
}

func globalPriorityOrderEnabledForSolver(priorities []string) bool {
	for _, priority := range priorities {
		kind, _, ok := parsePriorityForSolver(priority)
		if ok && kind == "coverage_group" {
			return true
		}
	}
	return false
}

func firstPriorityCoverageGroupIndex(priorities []string) (int, bool) {
	if len(priorities) == 0 {
		return 0, false
	}
	kind, value, ok := parsePriorityForSolver(priorities[0])
	if !ok || kind != "coverage_group" {
		return 0, false
	}
	return parseCoverageGroupIndexForSolver(value)
}

func parseCoverageGroupIndexForSolver(value string) (int, bool) {
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return index, true
}

func parsePriorityForSolver(priority string) (string, string, bool) {
	priority = strings.TrimSpace(priority)
	kind, value, ok := strings.Cut(priority, ":")
	if !ok {
		return "", "", false
	}
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind == "" || value == "" {
		return "", "", false
	}
	return kind, value, true
}

func coverageCountsFromTargetMasks(targetMasks []uint64, sourceCount int) []int {
	counts := make([]int, sourceCount)
	for _, mask := range targetMasks {
		covered := bits.OnesCount64(mask)
		if covered <= 0 || covered > sourceCount {
			continue
		}
		counts[sourceCount-covered]++
	}
	return counts
}

func coverageBucketsFromCounts(counts []int, sourceCount int) []model.StarCoverageBucket {
	buckets := make([]model.StarCoverageBucket, 0, sourceCount)
	for idx := 0; idx < sourceCount; idx++ {
		buckets = append(buckets, model.StarCoverageBucket{
			CoveredSources: sourceCount - idx,
			TargetCount:    counts[idx],
		})
	}
	return buckets
}

func sourceHitsTargetWithCatalog(catalog model.Catalog, source model.Placement, target model.Placement) bool {
	if source.InstanceID == target.InstanceID {
		return false
	}
	for starPositionIndex := range source.StarPositions {
		starPosition := &source.StarPositions[starPositionIndex]
		if !geometry.InBounds(starPosition.Position) {
			continue
		}
		cellBit := uint64(1) << uint(geometry.CellIndex(starPosition.Position))
		if target.Mask&cellBit == 0 {
			continue
		}
		if scoring.StarMatchesCatalogItems(catalog, source.ItemID, target.ItemID, &starPosition.Star) {
			return true
		}
	}
	return false
}

func coveragePlacementKey(placement model.Placement) string {
	return placement.InstanceID + "\x00" + placementKey(placement)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
