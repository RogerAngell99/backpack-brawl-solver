package solver

import (
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

// partialRepairState describes one repair-search prefix. FixedPlacements are
// outside the repair neighborhood, while CurrentPlacements are choices already
// made for removed instances. FreeCells must exclude unavailable board cells;
// removed options are also checked against all anchored placements.
type PartialRepairState struct {
	FixedPlacements   []model.Placement
	CurrentPlacements []model.Placement
	RemovedInstances  []model.InventoryInstance
	FreeCells         uint64
}

// partialRepairState keeps existing repair plumbing concise while the exported
// name documents the state carried across partial repair prefixes.
type partialRepairState = PartialRepairState

// partialRepairV3PriorityUpperBound returns a relaxed upper vector for an
// outgoing-per-instance V3 priority list. A nil result means the list includes
// a non-star priority, for which this bound deliberately makes no claim.
func partialRepairV3PriorityUpperBound(
	catalog model.Catalog,
	state partialRepairState,
	optionsByInstance map[string][]model.Placement,
	priorities []string,
) []int {
	for _, priority := range priorities {
		kind, sourceItemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" {
			return nil
		}
		if _, exists := catalog.Items[sourceItemID]; !exists {
			return nil
		}
	}

	anchored := state.anchoredPlacements()
	removed := state.unplacedRemoved(anchored)
	removedOptions := state.filteredRemovedOptions(removed, optionsByInstance)
	upperBySourceItem := map[string]int{}
	upper := make([]int, len(priorities))
	for priorityIndex, priority := range priorities {
		_, sourceItemID, _ := parsePriorityForSolver(priority)
		count, exists := upperBySourceItem[sourceItemID]
		if !exists {
			count = partialRepairStarUpperForItem(catalog, sourceItemID, anchored, removed, removedOptions)
			upperBySourceItem[sourceItemID] = count
		}
		upper[priorityIndex] = count
	}
	return upper
}

// partialRepairTargetVectorFeasible is conservative for partial search. An
// unavailable upper bound is never grounds for rejection; otherwise only a
// lexicographically lower upper vector is infeasible.
func partialRepairTargetVectorFeasible(upper []int, target []int) bool {
	return upper == nil || comparePriorityCounts(upper, target) >= 0
}

// partialRelaxedStarUpperBound is diagnostic-only. Matching is independent for
// every source and intentionally ignores conflicts between different links, so
// callers must not use it to prune a repair branch.
func partialRelaxedStarUpperBound(
	catalog model.Catalog,
	state partialRepairState,
	optionsByInstance map[string][]model.Placement,
) int {
	anchored := state.anchoredPlacements()
	removed := state.unplacedRemoved(anchored)
	removedOptions := state.filteredRemovedOptions(removed, optionsByInstance)
	upper := 0
	for sourceIndex := range anchored {
		upper += partialRepairSourceStarUpper(catalog, &anchored[sourceIndex], model.InventoryInstance{}, anchored, removed, removedOptions)
	}
	for sourceIndex := range removed {
		upper += partialRepairSourceStarUpper(catalog, nil, removed[sourceIndex], anchored, removed, removedOptions)
	}
	return upper
}

// partialRepairFixedStars returns activations that cannot change below this
// prefix because both endpoints are already anchored.
func partialRepairFixedStars(catalog model.Catalog, state partialRepairState) []model.StarActivation {
	return scoring.EvaluateStars(catalog, state.anchoredPlacements())
}

// partialRepairFixedStarHeadroom reports how many additional stars the relaxed
// diagnostic bound permits beyond the already anchored activations.
func partialRepairFixedStarHeadroom(
	catalog model.Catalog,
	state partialRepairState,
	optionsByInstance map[string][]model.Placement,
) int {
	headroom := partialRelaxedStarUpperBound(catalog, state, optionsByInstance) - len(partialRepairFixedStars(catalog, state))
	if headroom < 0 {
		return 0
	}
	return headroom
}

func (state partialRepairState) anchoredPlacements() []model.Placement {
	anchored := make([]model.Placement, 0, len(state.FixedPlacements)+len(state.CurrentPlacements))
	seen := make(map[string]struct{}, len(state.FixedPlacements)+len(state.CurrentPlacements))
	for _, placement := range state.FixedPlacements {
		if placement.InstanceID == "" {
			continue
		}
		seen[placement.InstanceID] = struct{}{}
		anchored = append(anchored, placement)
	}
	for _, placement := range state.CurrentPlacements {
		if placement.InstanceID == "" {
			continue
		}
		if _, exists := seen[placement.InstanceID]; exists {
			continue
		}
		seen[placement.InstanceID] = struct{}{}
		anchored = append(anchored, placement)
	}
	return anchored
}

func (state partialRepairState) unplacedRemoved(anchored []model.Placement) []model.InventoryInstance {
	anchoredIDs := make(map[string]struct{}, len(anchored))
	for _, placement := range anchored {
		anchoredIDs[placement.InstanceID] = struct{}{}
	}
	removed := make([]model.InventoryInstance, 0, len(state.RemovedInstances))
	seen := make(map[string]struct{}, len(state.RemovedInstances))
	for _, instance := range state.RemovedInstances {
		if instance.InstanceID == "" {
			continue
		}
		if _, anchored := anchoredIDs[instance.InstanceID]; anchored {
			continue
		}
		if _, exists := seen[instance.InstanceID]; exists {
			continue
		}
		seen[instance.InstanceID] = struct{}{}
		removed = append(removed, instance)
	}
	return removed
}

func (state partialRepairState) filteredRemovedOptions(
	removed []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
) map[string][]model.Placement {
	fixedOccupied := partialRepairOccupied(state.anchoredPlacements())
	filtered := make(map[string][]model.Placement, len(removed))
	for _, instance := range removed {
		options := optionsByInstance[instance.InstanceID]
		for _, option := range options {
			if option.Mask&fixedOccupied != 0 || option.Mask&^state.FreeCells != 0 {
				continue
			}
			filtered[instance.InstanceID] = append(filtered[instance.InstanceID], option)
		}
	}
	return filtered
}

func partialRepairOccupied(placements []model.Placement) uint64 {
	occupied := uint64(0)
	for _, placement := range placements {
		occupied |= placement.Mask
	}
	return occupied
}

func partialRepairStarUpperForItem(
	catalog model.Catalog,
	sourceItemID string,
	anchored []model.Placement,
	removed []model.InventoryInstance,
	removedOptions map[string][]model.Placement,
) int {
	upper := 0
	for sourceIndex := range anchored {
		if anchored[sourceIndex].ItemID != sourceItemID {
			continue
		}
		upper += partialRepairSourceStarUpper(catalog, &anchored[sourceIndex], model.InventoryInstance{}, anchored, removed, removedOptions)
	}
	for sourceIndex := range removed {
		if removed[sourceIndex].ItemID != sourceItemID {
			continue
		}
		upper += partialRepairSourceStarUpper(catalog, nil, removed[sourceIndex], anchored, removed, removedOptions)
	}
	return upper
}

func partialRepairSourceStarUpper(
	catalog model.Catalog,
	fixedSource *model.Placement,
	removedSource model.InventoryInstance,
	anchored []model.Placement,
	removed []model.InventoryInstance,
	removedOptions map[string][]model.Placement,
) int {
	sourceItemID := removedSource.ItemID
	if fixedSource != nil {
		sourceItemID = fixedSource.ItemID
	}
	item, exists := catalog.Items[sourceItemID]
	if !exists || len(item.Stars) == 0 {
		return 0
	}

	targetCount := len(anchored) + len(removed)
	slots := make([][]int, len(item.Stars))
	for starIndex := range slots {
		for targetIndex := range anchored {
			if partialRepairSlotCanHitFixedTarget(catalog, fixedSource, removedSource, anchored[targetIndex], starIndex, removedOptions) {
				slots[starIndex] = append(slots[starIndex], targetIndex)
			}
		}
		for targetIndex := range removed {
			if partialRepairSlotCanHitRemovedTarget(catalog, fixedSource, removedSource, removed[targetIndex], starIndex, removedOptions) {
				slots[starIndex] = append(slots[starIndex], len(anchored)+targetIndex)
			}
		}
	}
	return partialRepairMaximumSlotMatching(slots, targetCount)
}

func partialRepairSlotCanHitFixedTarget(
	catalog model.Catalog,
	fixedSource *model.Placement,
	removedSource model.InventoryInstance,
	fixedTarget model.Placement,
	starIndex int,
	removedOptions map[string][]model.Placement,
) bool {
	if fixedSource != nil {
		if fixedSource.InstanceID == fixedTarget.InstanceID || fixedSource.Mask&fixedTarget.Mask != 0 {
			return false
		}
		return starPositionHitsTarget(catalog, *fixedSource, fixedTarget, starIndex)
	}
	if removedSource.InstanceID == fixedTarget.InstanceID {
		return false
	}
	for _, sourceOption := range removedOptions[removedSource.InstanceID] {
		if sourceOption.Mask&fixedTarget.Mask == 0 && starPositionHitsTarget(catalog, sourceOption, fixedTarget, starIndex) {
			return true
		}
	}
	return false
}

func partialRepairSlotCanHitRemovedTarget(
	catalog model.Catalog,
	fixedSource *model.Placement,
	removedSource model.InventoryInstance,
	removedTarget model.InventoryInstance,
	starIndex int,
	removedOptions map[string][]model.Placement,
) bool {
	if fixedSource != nil {
		if fixedSource.InstanceID == removedTarget.InstanceID {
			return false
		}
		for _, targetOption := range removedOptions[removedTarget.InstanceID] {
			if fixedSource.Mask&targetOption.Mask == 0 && starPositionHitsTarget(catalog, *fixedSource, targetOption, starIndex) {
				return true
			}
		}
		return false
	}
	if removedSource.InstanceID == removedTarget.InstanceID {
		return false
	}
	for _, sourceOption := range removedOptions[removedSource.InstanceID] {
		for _, targetOption := range removedOptions[removedTarget.InstanceID] {
			if sourceOption.Mask&targetOption.Mask == 0 && starPositionHitsTarget(catalog, sourceOption, targetOption, starIndex) {
				return true
			}
		}
	}
	return false
}

func partialRepairMaximumSlotMatching(slotTargets [][]int, targetCount int) int {
	matchedSlots := make([]int, targetCount)
	for targetIndex := range matchedSlots {
		matchedSlots[targetIndex] = -1
	}
	var tryMatch func(int, []bool) bool
	tryMatch = func(slot int, seen []bool) bool {
		for _, target := range slotTargets[slot] {
			if target < 0 || target >= len(matchedSlots) || seen[target] {
				continue
			}
			seen[target] = true
			if matchedSlots[target] < 0 || tryMatch(matchedSlots[target], seen) {
				matchedSlots[target] = slot
				return true
			}
		}
		return false
	}

	matched := 0
	for slot := range slotTargets {
		if tryMatch(slot, make([]bool, targetCount)) {
			matched++
		}
	}
	return matched
}
