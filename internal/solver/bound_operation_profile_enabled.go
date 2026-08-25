//go:build searchprofile

package solver

import (
	"math/bits"

	"backpack-brawl-solver/internal/model"
)

// boundOperationCounters is task/session-local. All aggregation happens only
// after the owning search operation returns.
type boundOperationCounters struct {
	profile model.BoundAttributionOperationProfile
}

func newBoundOperationCounters(config Config) *boundOperationCounters {
	if !config.OperationProfiling {
		return nil
	}
	return &boundOperationCounters{profile: model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}}
}

func (c *boundOperationCounters) snapshot() *model.BoundAttributionOperationProfile {
	if c == nil {
		return nil
	}
	copy := c.profile
	return &copy
}

func (c *boundOperationCounters) snapshotSearch(checks int64, pruned int64) *model.BoundAttributionOperationProfile {
	profile := c.snapshot()
	if profile == nil {
		return nil
	}
	profile.Outgoing.Search.Checks = checks
	profile.Outgoing.Search.PrunedNodes = pruned
	return profile
}

func (c *boundOperationCounters) snapshotRepair(checks int64, pruned int64) *model.BoundAttributionOperationProfile {
	profile := c.snapshot()
	if profile == nil {
		return nil
	}
	profile.Outgoing.Repair.Checks = checks
	profile.Outgoing.Repair.PrunedNodes = pruned
	return profile
}

func (c *boundOperationCounters) prioritySite(site boundPriorityAttributionSite) *model.PriorityUpperBoundSiteProfile {
	if c == nil {
		return nil
	}
	switch site {
	case boundPriorityConstellationFilter:
		return &c.profile.PriorityUpper.ConstellationFilter
	case boundPriorityRepairDFS:
		return &c.profile.PriorityUpper.RepairDFS
	case boundPriorityPlateauPrefilter:
		return &c.profile.PriorityUpper.PlateauPrefilter
	case boundPriorityPlateauDFS:
		return &c.profile.PriorityUpper.PlateauDFS
	default:
		panic("unknown priority bound attribution site")
	}
}

func (c *boundOperationCounters) outgoingSite(site boundOutgoingAttributionSite) *model.OutgoingBoundSiteProfile {
	if c == nil {
		return nil
	}
	switch site {
	case boundOutgoingSearch:
		return &c.profile.Outgoing.Search
	case boundOutgoingRepair:
		return &c.profile.Outgoing.Repair
	default:
		panic("unknown outgoing bound attribution site")
	}
}

func (c *boundOperationCounters) constellationFilterInvocation(states int) {
	if c == nil {
		return
	}
	c.profile.PriorityUpper.ConstellationFilterInvocations++
	c.profile.PriorityUpper.ConstellationStatesInput += int64(states)
}

func (c *boundOperationCounters) constellationFilterResult(feasible bool) {
	if c == nil {
		return
	}
	if feasible {
		c.profile.PriorityUpper.ConstellationStatesRetained++
	} else {
		c.profile.PriorityUpper.ConstellationStatesRejected++
	}
}

func recordPriorityUpperBoundResult(profile *model.PriorityUpperBoundSiteProfile, feasible bool) {
	if feasible {
		profile.FeasibleResults++
	} else {
		profile.RejectedResults++
	}
}

// partialRepairV3PriorityUpperBoundProfiled mirrors
// partialRepairV3PriorityUpperBound without reordering or hoisting work.
func partialRepairV3PriorityUpperBoundProfiled(
	catalog model.Catalog,
	state partialRepairState,
	optionsByInstance map[string][]model.Placement,
	priorities []string,
	compatibility *priorityStarCompatibility,
	profile *model.PriorityUpperBoundSiteProfile,
) []int {
	profile.Calls++
	for _, priority := range priorities {
		kind, sourceItemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" {
			profile.InvalidPriorityReturns++
			return nil
		}
		if _, exists := catalog.Items[sourceItemID]; !exists {
			profile.InvalidPriorityReturns++
			return nil
		}
		profile.PriorityEntriesValidated++
	}

	profile.FixedPlacementInputs += int64(len(state.FixedPlacements))
	profile.CurrentPlacementInputs += int64(len(state.CurrentPlacements))
	profile.RemovedInstanceInputs += int64(len(state.RemovedInstances))
	anchored := state.anchoredPlacements()
	profile.AnchoredPlacements += int64(len(anchored))
	removed := state.unplacedRemoved(anchored)
	profile.RemovedInstances += int64(len(removed))
	removedOptions := filteredRemovedOptionsProfiled(state, removed, optionsByInstance, profile)
	upperBySourceItem := map[string]int{}
	upper := make([]int, len(priorities))
	for priorityIndex, priority := range priorities {
		_, sourceItemID, _ := parsePriorityForSolver(priority)
		count, exists := upperBySourceItem[sourceItemID]
		if !exists {
			profile.UniquePrioritySourceItems++
			count = partialRepairStarUpperForItemProfiled(catalog, sourceItemID, anchored, removed, removedOptions, compatibility, profile)
			upperBySourceItem[sourceItemID] = count
		}
		upper[priorityIndex] = count
	}
	return upper
}

func filteredRemovedOptionsProfiled(
	state partialRepairState,
	removed []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	profile *model.PriorityUpperBoundSiteProfile,
) map[string][]model.Placement {
	fixedOccupied := partialRepairOccupied(state.anchoredPlacements())
	filtered := make(map[string][]model.Placement, len(removed))
	for _, instance := range removed {
		options := optionsByInstance[instance.InstanceID]
		for _, option := range options {
			profile.RemovedOptionCandidates++
			if option.Mask&fixedOccupied != 0 {
				profile.RemovedOptionRejectedFixedOverlap++
				continue
			}
			if option.Mask&^state.FreeCells != 0 {
				profile.RemovedOptionRejectedOutsideFree++
				continue
			}
			profile.RemovedOptionsRetained++
			filtered[instance.InstanceID] = append(filtered[instance.InstanceID], option)
		}
	}
	return filtered
}

func partialRepairStarUpperForItemProfiled(
	catalog model.Catalog,
	sourceItemID string,
	anchored []model.Placement,
	removed []model.InventoryInstance,
	removedOptions map[string][]model.Placement,
	compatibility *priorityStarCompatibility,
	profile *model.PriorityUpperBoundSiteProfile,
) int {
	upper := 0
	for sourceIndex := range anchored {
		if anchored[sourceIndex].ItemID != sourceItemID {
			continue
		}
		profile.AnchoredSourceInstances++
		upper += partialRepairSourceStarUpperProfiled(catalog, &anchored[sourceIndex], model.InventoryInstance{}, anchored, removed, removedOptions, compatibility, profile)
	}
	for sourceIndex := range removed {
		if removed[sourceIndex].ItemID != sourceItemID {
			continue
		}
		profile.RemovedSourceInstances++
		upper += partialRepairSourceStarUpperProfiled(catalog, nil, removed[sourceIndex], anchored, removed, removedOptions, compatibility, profile)
	}
	return upper
}

func partialRepairSourceStarUpperProfiled(
	catalog model.Catalog,
	fixedSource *model.Placement,
	removedSource model.InventoryInstance,
	anchored []model.Placement,
	removed []model.InventoryInstance,
	removedOptions map[string][]model.Placement,
	compatibility *priorityStarCompatibility,
	profile *model.PriorityUpperBoundSiteProfile,
) int {
	sourceItemID := removedSource.ItemID
	if fixedSource != nil {
		sourceItemID = fixedSource.ItemID
	}
	item, exists := catalog.Items[sourceItemID]
	if !exists || len(item.Stars) == 0 {
		return 0
	}

	profile.StarSlots += int64(len(item.Stars))
	targetCount := len(anchored) + len(removed)
	slots := make([][]int, len(item.Stars))
	for starIndex := range slots {
		for targetIndex := range anchored {
			profile.FixedTargetChecks++
			if partialRepairSlotCanHitFixedTargetProfiled(catalog, fixedSource, removedSource, anchored[targetIndex], starIndex, removedOptions, compatibility, profile) {
				slots[starIndex] = append(slots[starIndex], targetIndex)
			}
		}
		for targetIndex := range removed {
			profile.RemovedTargetChecks++
			if partialRepairSlotCanHitRemovedTargetProfiled(catalog, fixedSource, removedSource, removed[targetIndex], starIndex, removedOptions, compatibility, profile) {
				slots[starIndex] = append(slots[starIndex], len(anchored)+targetIndex)
			}
		}
	}
	profile.MatchingCalls++
	return partialRepairMaximumSlotMatching(slots, targetCount)
}

func partialRepairSlotCanHitFixedTargetProfiled(
	catalog model.Catalog,
	fixedSource *model.Placement,
	removedSource model.InventoryInstance,
	fixedTarget model.Placement,
	starIndex int,
	removedOptions map[string][]model.Placement,
	compatibility *priorityStarCompatibility,
	profile *model.PriorityUpperBoundSiteProfile,
) bool {
	if fixedSource != nil {
		if fixedSource.InstanceID == fixedTarget.InstanceID {
			profile.SelfTargetSkips++
			return false
		}
		profile.FixedFixedGeometryChecks++
		staticCompatible, cached := compatibility.match(fixedSource.OriginalIndex, starIndex, fixedTarget.OriginalIndex)
		return priorityGeometryCandidateHitsProfiled(catalog, *fixedSource, fixedTarget, starIndex, staticCompatible, cached, profile)
	}
	if removedSource.InstanceID == fixedTarget.InstanceID {
		profile.SelfTargetSkips++
		return false
	}
	staticCompatible, cached := compatibility.match(removedSource.OriginalIndex, starIndex, fixedTarget.OriginalIndex)
	for _, sourceOption := range removedOptions[removedSource.InstanceID] {
		profile.RemovedSourceOptionChecksFixedTarget++
		if priorityGeometryCandidateHitsProfiled(catalog, sourceOption, fixedTarget, starIndex, staticCompatible, cached, profile) {
			return true
		}
	}
	return false
}

func partialRepairSlotCanHitRemovedTargetProfiled(
	catalog model.Catalog,
	fixedSource *model.Placement,
	removedSource model.InventoryInstance,
	removedTarget model.InventoryInstance,
	starIndex int,
	removedOptions map[string][]model.Placement,
	compatibility *priorityStarCompatibility,
	profile *model.PriorityUpperBoundSiteProfile,
) bool {
	if fixedSource != nil {
		if fixedSource.InstanceID == removedTarget.InstanceID {
			profile.SelfTargetSkips++
			return false
		}
		staticCompatible, cached := compatibility.match(fixedSource.OriginalIndex, starIndex, removedTarget.OriginalIndex)
		for _, targetOption := range removedOptions[removedTarget.InstanceID] {
			profile.FixedSourceTargetOptionChecks++
			if priorityGeometryCandidateHitsProfiled(catalog, *fixedSource, targetOption, starIndex, staticCompatible, cached, profile) {
				return true
			}
		}
		return false
	}
	if removedSource.InstanceID == removedTarget.InstanceID {
		profile.SelfTargetSkips++
		return false
	}
	staticCompatible, cached := compatibility.match(removedSource.OriginalIndex, starIndex, removedTarget.OriginalIndex)
	for _, sourceOption := range removedOptions[removedSource.InstanceID] {
		for _, targetOption := range removedOptions[removedTarget.InstanceID] {
			profile.RemovedSourceTargetOptionPairs++
			if priorityGeometryCandidateHitsProfiled(catalog, sourceOption, targetOption, starIndex, staticCompatible, cached, profile) {
				return true
			}
		}
	}
	return false
}

func priorityGeometryCandidateHitsProfiled(
	catalog model.Catalog,
	source model.Placement,
	target model.Placement,
	starIndex int,
	staticCompatible bool,
	compatibilityCached bool,
	profile *model.PriorityUpperBoundSiteProfile,
) bool {
	profile.GeometryCandidateChecks++
	if source.Mask&target.Mask != 0 {
		profile.GeometryOverlapRejects++
		return false
	}
	profile.StarPositionHitCalls++
	var hit bool
	if compatibilityCached {
		hit = source.InstanceID != target.InstanceID && staticCompatible && starPositionGeometryHitsTarget(source, target, starIndex)
	} else {
		hit = starPositionHitsTarget(catalog, source, target, starIndex)
	}
	if !hit {
		return false
	}
	profile.StarPositionHitTrue++
	profile.SlotTargetHits++
	return true
}

func (ctx *outgoingBoundContext) shouldPruneProfiled(
	placements []model.Placement,
	results []model.Solution,
	topN int,
	profile *model.OutgoingBoundSiteProfile,
) bool {
	if ctx == nil || len(results) < topN || topN <= 0 {
		return false
	}
	upper := ctx.upperPriorityCountsProfiled(placements, profile)
	return comparePriorityCounts(upper, results[len(results)-1].Evaluation.Score.PriorityCounts) < 0
}

func (ctx *outgoingBoundContext) upperPriorityCountsProfiled(placements []model.Placement, profile *model.OutgoingBoundSiteProfile) []int {
	index, ok := ctx.buildOutgoingPlacementIndex(placements)
	if !ok {
		return ctx.upperPriorityCountsLegacyProfiled(placements, profile)
	}
	return ctx.upperPriorityCountsIndexedProfiled(placements, index, profile)
}

// upperPriorityCountsLegacyProfiled mirrors upperPriorityCountsLegacy without
// reordering its map, mask, priority, source, target, potential, popcount, or
// clamp work.
func (ctx *outgoingBoundContext) upperPriorityCountsLegacyProfiled(placements []model.Placement, profile *model.OutgoingBoundSiteProfile) []int {
	profile.PlacedMapBuilds++
	profile.PlacedMapInsertions += int64(len(placements))
	placedByID := placementByInstanceID(placements)
	placedMask := uint64(0)
	for _, instance := range ctx.instances {
		profile.PlacedMaskInstanceChecks++
		if _, placed := placedByID[instance.InstanceID]; placed {
			placedMask |= uint64(1) << uint(instance.OriginalIndex)
		}
	}
	upper := make([]int, len(ctx.priorityItems))
	for priorityIndex, sourceItemID := range ctx.priorityItems {
		profile.PriorityIterations++
		for _, sourceInstance := range ctx.instances {
			profile.SourceInstanceIterations++
			if sourceInstance.ItemID != sourceItemID {
				continue
			}
			profile.PrioritySourceMatches++
			starCount := len(ctx.catalog.Items[sourceItemID].Stars)
			if starCount == 0 {
				profile.ZeroStarSourceSkips++
				continue
			}
			var targets uint64
			if sourcePlacement, placed := placedByID[sourceInstance.InstanceID]; placed {
				profile.PlacedSourceIterations++
				for _, targetInstance := range ctx.instances {
					profile.PlacedSourceTargetIterations++
					if targetInstance.InstanceID == sourceInstance.InstanceID {
						profile.SelfTargetSkips++
						continue
					}
					profile.TargetPlacementLookups++
					targetPlacement, targetPlaced := placedByID[targetInstance.InstanceID]
					if !targetPlaced {
						profile.UnplacedTargets++
						continue
					}
					profile.PlacedTargetsFound++
					profile.SourceHitsTargetCalls++
					if sourceHitsTargetWithCatalog(ctx.catalog, sourcePlacement, targetPlacement) {
						profile.SourceHitsTargetTrue++
						targets |= uint64(1) << uint(targetInstance.OriginalIndex)
					}
				}
				profile.CoveragePlacementKeyCalls++
				key := coveragePlacementKey(sourcePlacement)
				profile.PlacedPotentialLookups++
				targets |= ctx.potential.outgoingTargets[key] &^ placedMask
			} else {
				profile.FreeSourceIterations++
				profile.FreePotentialLookups++
				targets = ctx.potential.instanceOutgoingTargets[sourceInstance.InstanceID]
			}
			targets &^= uint64(1) << uint(sourceInstance.OriginalIndex)
			profile.PopcountCalls++
			count := bits.OnesCount64(targets)
			if count > starCount {
				profile.StarCountClamps++
				count = starCount
			}
			upper[priorityIndex] += count
		}
	}
	return upper
}

// upperPriorityCountsIndexedProfiled preserves the public counters as logical
// work-site counts. PlacedMapBuilds, PlacedMapInsertions, mask checks, and
// target lookups retain their historical meaning even though the indexed path
// performs no physical string-map work.
func (ctx *outgoingBoundContext) upperPriorityCountsIndexedProfiled(placements []model.Placement, index outgoingPlacementIndex, profile *model.OutgoingBoundSiteProfile) []int {
	profile.PlacedMapBuilds++
	profile.PlacedMapInsertions += int64(len(placements))
	profile.PlacedMaskInstanceChecks += int64(len(ctx.instances))
	placedMask := index.presentMask
	upper := make([]int, len(ctx.priorityItems))
	for priorityIndex, sourceItemID := range ctx.priorityItems {
		profile.PriorityIterations++
		for _, sourceInstance := range ctx.instances {
			profile.SourceInstanceIterations++
			if sourceInstance.ItemID != sourceItemID {
				continue
			}
			profile.PrioritySourceMatches++
			starCount := len(ctx.catalog.Items[sourceItemID].Stars)
			if starCount == 0 {
				profile.ZeroStarSourceSkips++
				continue
			}
			var targets uint64
			if sourcePosition := index.positionPlusOne[sourceInstance.OriginalIndex]; sourcePosition != 0 {
				profile.PlacedSourceIterations++
				sourcePlacement := placements[int(sourcePosition)-1]
				for _, targetInstance := range ctx.instances {
					profile.PlacedSourceTargetIterations++
					if targetInstance.InstanceID == sourceInstance.InstanceID {
						profile.SelfTargetSkips++
						continue
					}
					profile.TargetPlacementLookups++
					targetPosition := index.positionPlusOne[targetInstance.OriginalIndex]
					if targetPosition == 0 {
						profile.UnplacedTargets++
						continue
					}
					profile.PlacedTargetsFound++
					profile.SourceHitsTargetCalls++
					targetPlacement := placements[int(targetPosition)-1]
					if sourceHitsTargetWithCatalog(ctx.catalog, sourcePlacement, targetPlacement) {
						profile.SourceHitsTargetTrue++
						targets |= uint64(1) << uint(targetInstance.OriginalIndex)
					}
				}
				profile.CoveragePlacementKeyCalls++
				key := coveragePlacementKey(sourcePlacement)
				profile.PlacedPotentialLookups++
				targets |= ctx.potential.outgoingTargets[key] &^ placedMask
			} else {
				profile.FreeSourceIterations++
				profile.FreePotentialLookups++
				targets = ctx.potential.instanceOutgoingTargets[sourceInstance.InstanceID]
			}
			targets &^= uint64(1) << uint(sourceInstance.OriginalIndex)
			profile.PopcountCalls++
			count := bits.OnesCount64(targets)
			if count > starCount {
				profile.StarCountClamps++
				count = starCount
			}
			upper[priorityIndex] += count
		}
	}
	return upper
}
