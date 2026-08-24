package solver

import (
	"math/bits"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

// starUpperBoundContext provides progressively tighter optimistic star bounds
// for diagnostics. No bound in this file participates in pruning.
type starUpperBoundContext struct {
	catalog           model.Catalog
	instances         []model.InventoryInstance
	optionsByInstance map[string][]model.Placement
	compatibleTarget  [64]uint64
	geometricSlots    [64][]uint64
	root              model.StarUpperBounds
}

func newStarUpperBoundContext(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
) *starUpperBoundContext {
	if len(instances) == 0 || len(instances) > 64 {
		return nil
	}
	context := &starUpperBoundContext{
		catalog:           catalog,
		instances:         append([]model.InventoryInstance(nil), instances...),
		optionsByInstance: optionsByInstance,
	}
	for sourceIndex, source := range instances {
		item := catalog.Items[source.ItemID]
		context.geometricSlots[sourceIndex] = make([]uint64, len(item.Stars))
		for targetIndex, target := range instances {
			if sourceIndex == targetIndex || !looseSourceCanTarget(catalog, source.ItemID, target.ItemID) {
				continue
			}
			context.compatibleTarget[sourceIndex] |= uint64(1) << uint(targetIndex)
			for starIndex := range item.Stars {
				if placementOptionsCanHitStar(catalog, optionsByInstance[source.InstanceID], optionsByInstance[target.InstanceID], starIndex) {
					context.geometricSlots[sourceIndex][starIndex] |= uint64(1) << uint(targetIndex)
				}
			}
		}
		availableTargets := len(instances) - 1
		context.root.Structural += minInt(len(item.Stars), availableTargets)
		context.root.Compatible += minInt(len(item.Stars), bits.OnesCount64(context.compatibleTarget[sourceIndex]))
		context.root.GeometricRelaxed += maximumSlotMatching(context.geometricSlots[sourceIndex])
	}
	context.root.Available = context.root.Compatible
	return context
}

func placementOptionsCanHit(catalog model.Catalog, sources []model.Placement, targets []model.Placement) bool {
	for _, source := range sources {
		for _, target := range targets {
			if source.Mask&target.Mask != 0 {
				continue
			}
			if sourceHitsTargetWithCatalog(catalog, source, target) {
				return true
			}
		}
	}
	return false
}

func placementOptionsCanHitStar(catalog model.Catalog, sources []model.Placement, targets []model.Placement, starIndex int) bool {
	for _, source := range sources {
		for _, target := range targets {
			if source.Mask&target.Mask != 0 {
				continue
			}
			if starPositionHitsTarget(catalog, source, target, starIndex) {
				return true
			}
		}
	}
	return false
}

func (context *starUpperBoundContext) forPlacements(placements []model.Placement, instances []model.InventoryInstance) model.StarUpperBounds {
	if context == nil {
		return model.StarUpperBounds{}
	}
	if len(placements) == 0 || len(instances) == 0 {
		return context.root
	}
	placedByID := make(map[string]model.Placement, len(placements))
	for _, placement := range placements {
		placedByID[placement.InstanceID] = placement
	}
	activeBySource := map[string]int{}
	for _, activation := range scoring.EvaluateStars(context.catalog, placements) {
		activeBySource[activation.SourceInstance]++
	}
	bound := context.root
	bound.Available = 0
	bound.GeometricRelaxed = 0
	for sourceIndex, source := range context.instances {
		item := context.catalog.Items[source.ItemID]
		if _, placed := placedByID[source.InstanceID]; !placed {
			// An unplaced source may still target already fixed items. Ignoring
			// occupancy conflicts keeps this availability level admissible.
			bound.Available += minInt(len(item.Stars), bits.OnesCount64(context.compatibleTarget[sourceIndex]))
			bound.GeometricRelaxed += context.matchingForSource(sourceIndex, nil, placedByID)
			continue
		}

		availableTargets := 0
		for targetIndex, target := range context.instances {
			if target.InstanceID == source.InstanceID {
				continue
			}
			if _, targetPlaced := placedByID[target.InstanceID]; targetPlaced {
				continue
			}
			if context.compatibleTarget[sourceIndex]&(uint64(1)<<uint(targetIndex)) != 0 {
				availableTargets++
			}
		}
		active := activeBySource[source.InstanceID]
		bound.Available += minInt(len(item.Stars), active+availableTargets)
		placedSource := placedByID[source.InstanceID]
		bound.GeometricRelaxed += context.matchingForSource(sourceIndex, &placedSource, placedByID)
	}
	return bound
}

func (context *starUpperBoundContext) matchingForSource(sourceIndex int, sourcePlacement *model.Placement, placedByID map[string]model.Placement) int {
	source := context.instances[sourceIndex]
	item := context.catalog.Items[source.ItemID]
	slots := make([]uint64, len(item.Stars))
	for starIndex := range slots {
		for targetIndex, target := range context.instances {
			if target.InstanceID == source.InstanceID {
				continue
			}
			targetPlacement, targetPlaced := placedByID[target.InstanceID]
			if sourcePlacement != nil {
				if targetPlaced {
					if starPositionHitsTarget(context.catalog, *sourcePlacement, targetPlacement, starIndex) {
						slots[starIndex] |= uint64(1) << uint(targetIndex)
					}
					continue
				}
				for _, option := range contextOptionsForTarget(context, target.InstanceID) {
					if starPositionHitsTarget(context.catalog, *sourcePlacement, option, starIndex) {
						slots[starIndex] |= uint64(1) << uint(targetIndex)
						break
					}
				}
				continue
			}
			if targetPlaced {
				for _, option := range contextOptionsForTarget(context, source.InstanceID) {
					if starPositionHitsTarget(context.catalog, option, targetPlacement, starIndex) {
						slots[starIndex] |= uint64(1) << uint(targetIndex)
						break
					}
				}
				continue
			}
			slots[starIndex] |= context.geometricSlots[sourceIndex][starIndex] & (uint64(1) << uint(targetIndex))
		}
	}
	return maximumSlotMatching(slots)
}

// options are retained only for diagnostic matching. They never participate in
// pruning, and a matching intentionally ignores conflicts between sources.
func contextOptionsForTarget(context *starUpperBoundContext, instanceID string) []model.Placement {
	// The geometric slot relation already includes every unplaced pair. Fixed
	// source/target checks need concrete options, which are cached in the
	// placement lookup populated by the constructor below.
	return context.optionsByInstance[instanceID]
}

func starPositionHitsTarget(catalog model.Catalog, source model.Placement, target model.Placement, starIndex int) bool {
	if starIndex < 0 || starIndex >= len(source.StarPositions) || source.InstanceID == target.InstanceID {
		return false
	}
	star := source.StarPositions[starIndex]
	if !geometry.InBounds(star.Position) || !scoring.StarMatchesCatalogItems(catalog, source.ItemID, target.ItemID, &star.Star) {
		return false
	}
	return target.Mask&(uint64(1)<<uint(geometry.CellIndex(star.Position))) != 0
}

func starPositionGeometryHitsTarget(source model.Placement, target model.Placement, starIndex int) bool {
	if starIndex < 0 || starIndex >= len(source.StarPositions) {
		return false
	}
	star := source.StarPositions[starIndex]
	if !geometry.InBounds(star.Position) {
		return false
	}
	return target.Mask&(uint64(1)<<uint(geometry.CellIndex(star.Position))) != 0
}

func maximumSlotMatching(slotTargets []uint64) int {
	matchedSlots := [64]int{}
	for index := range matchedSlots {
		matchedSlots[index] = -1
	}
	var tryMatch func(int, *[64]bool) bool
	tryMatch = func(slot int, seen *[64]bool) bool {
		for targets := slotTargets[slot]; targets != 0; {
			target := bits.TrailingZeros64(targets)
			targets &^= uint64(1) << uint(target)
			if seen[target] {
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
		var seen [64]bool
		if tryMatch(slot, &seen) {
			matched++
		}
	}
	return matched
}
