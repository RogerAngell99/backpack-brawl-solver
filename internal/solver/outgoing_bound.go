package solver

import (
	"math/bits"

	"backpack-brawl-solver/internal/model"
)

// outgoingBoundContext is a deliberately relaxed V3 bound. It may count
// mutually incompatible future placements, but it never omits a target that a
// source could still reach. Equality is never pruned because score tie-breakers
// are intentionally outside this bound.
type outgoingBoundContext struct {
	catalog       model.Catalog
	instances     []model.InventoryInstance
	priorityItems []string
	potential     *starPotentialContext
	indexDomain   *outgoingPlacementIndexDomain
}

type outgoingPlacementIndexDomain struct {
	instanceIDByOriginal [64]string
	inventoryMask        uint64
}

type outgoingPlacementIndex struct {
	positionPlusOne [64]uint8
	presentMask     uint64
}

func newOutgoingPlacementIndexDomain(instances []model.InventoryInstance) *outgoingPlacementIndexDomain {
	if len(instances) > 64 {
		return nil
	}
	domain := &outgoingPlacementIndexDomain{}
	instanceIDs := make(map[string]struct{}, len(instances))
	for _, instance := range instances {
		if instance.OriginalIndex < 0 || instance.OriginalIndex >= len(domain.instanceIDByOriginal) || instance.InstanceID == "" {
			return nil
		}
		bit := uint64(1) << uint(instance.OriginalIndex)
		if domain.inventoryMask&bit != 0 {
			return nil
		}
		if _, exists := instanceIDs[instance.InstanceID]; exists {
			return nil
		}
		instanceIDs[instance.InstanceID] = struct{}{}
		domain.instanceIDByOriginal[instance.OriginalIndex] = instance.InstanceID
		domain.inventoryMask |= bit
	}
	return domain
}

func (ctx *outgoingBoundContext) buildOutgoingPlacementIndex(placements []model.Placement) (outgoingPlacementIndex, bool) {
	var index outgoingPlacementIndex
	if ctx == nil || ctx.indexDomain == nil || len(placements) > len(index.positionPlusOne) {
		return index, false
	}
	for position, placement := range placements {
		if placement.OriginalIndex < 0 || placement.OriginalIndex >= len(index.positionPlusOne) {
			return outgoingPlacementIndex{}, false
		}
		bit := uint64(1) << uint(placement.OriginalIndex)
		if ctx.indexDomain.inventoryMask&bit == 0 || index.presentMask&bit != 0 || ctx.indexDomain.instanceIDByOriginal[placement.OriginalIndex] != placement.InstanceID {
			return outgoingPlacementIndex{}, false
		}
		index.positionPlusOne[placement.OriginalIndex] = uint8(position + 1)
		index.presentMask |= bit
	}
	return index, true
}

func newOutgoingBoundContext(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	potential *starPotentialContext,
) *outgoingBoundContext {
	if config.DisableOutgoingBounds || config.PrioritySemantics != model.PrioritySemanticsOutgoingPerInstanceV3 || len(config.Priorities) == 0 {
		return nil
	}
	if potential == nil {
		potential = newStarPotentialContext(catalog, instances, optionsByInstance, config.Priorities, config.PrioritySemantics)
	}
	if potential == nil {
		return nil
	}
	priorityItems := make([]string, 0, len(config.Priorities))
	for _, priority := range config.Priorities {
		kind, itemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" {
			return nil
		}
		if _, exists := catalog.Items[itemID]; !exists {
			return nil
		}
		priorityItems = append(priorityItems, itemID)
	}
	return &outgoingBoundContext{
		catalog:       catalog,
		instances:     instances,
		priorityItems: priorityItems,
		potential:     potential,
		indexDomain:   newOutgoingPlacementIndexDomain(instances),
	}
}

func (ctx *outgoingBoundContext) shouldPrune(placements []model.Placement, results []model.Solution, topN int) bool {
	if ctx == nil || len(results) < topN || topN <= 0 {
		return false
	}
	upper := ctx.upperPriorityCounts(placements)
	return comparePriorityCounts(upper, results[len(results)-1].Evaluation.Score.PriorityCounts) < 0
}

func (ctx *outgoingBoundContext) upperPriorityCounts(placements []model.Placement) []int {
	index, ok := ctx.buildOutgoingPlacementIndex(placements)
	if !ok {
		return ctx.upperPriorityCountsLegacy(placements)
	}
	return ctx.upperPriorityCountsIndexed(placements, index)
}

func (ctx *outgoingBoundContext) upperPriorityCountsLegacy(placements []model.Placement) []int {
	placedByID := placementByInstanceID(placements)
	placedMask := uint64(0)
	for _, instance := range ctx.instances {
		if _, placed := placedByID[instance.InstanceID]; placed {
			placedMask |= uint64(1) << uint(instance.OriginalIndex)
		}
	}
	upper := make([]int, len(ctx.priorityItems))
	for priorityIndex, sourceItemID := range ctx.priorityItems {
		for _, sourceInstance := range ctx.instances {
			if sourceInstance.ItemID != sourceItemID {
				continue
			}
			starCount := len(ctx.catalog.Items[sourceItemID].Stars)
			if starCount == 0 {
				continue
			}
			var targets uint64
			if sourcePlacement, placed := placedByID[sourceInstance.InstanceID]; placed {
				for _, targetInstance := range ctx.instances {
					if targetInstance.InstanceID == sourceInstance.InstanceID {
						continue
					}
					targetPlacement, targetPlaced := placedByID[targetInstance.InstanceID]
					if targetPlaced && sourceHitsTargetWithCatalog(ctx.catalog, sourcePlacement, targetPlacement) {
						targets |= uint64(1) << uint(targetInstance.OriginalIndex)
					}
				}
				// Only targets that are still movable get geometric potential.
				targets |= ctx.potential.outgoingTargets[coveragePlacementKey(sourcePlacement)] &^ placedMask
			} else {
				// A free source may choose any option from the precomputed geometric
				// relaxation. Fixed-item conflicts are intentionally ignored here.
				targets = ctx.potential.instanceOutgoingTargets[sourceInstance.InstanceID]
			}
			targets &^= uint64(1) << uint(sourceInstance.OriginalIndex)
			count := bits.OnesCount64(targets)
			if count > starCount {
				count = starCount
			}
			upper[priorityIndex] += count
		}
	}
	return upper
}

func (ctx *outgoingBoundContext) upperPriorityCountsIndexed(placements []model.Placement, index outgoingPlacementIndex) []int {
	placedMask := index.presentMask
	upper := make([]int, len(ctx.priorityItems))
	for priorityIndex, sourceItemID := range ctx.priorityItems {
		for _, sourceInstance := range ctx.instances {
			if sourceInstance.ItemID != sourceItemID {
				continue
			}
			starCount := len(ctx.catalog.Items[sourceItemID].Stars)
			if starCount == 0 {
				continue
			}
			var targets uint64
			if sourcePosition := index.positionPlusOne[sourceInstance.OriginalIndex]; sourcePosition != 0 {
				sourcePlacement := placements[int(sourcePosition)-1]
				for _, targetInstance := range ctx.instances {
					if targetInstance.InstanceID == sourceInstance.InstanceID {
						continue
					}
					targetPosition := index.positionPlusOne[targetInstance.OriginalIndex]
					if targetPosition != 0 {
						targetPlacement := placements[int(targetPosition)-1]
						if sourceHitsTargetWithCatalog(ctx.catalog, sourcePlacement, targetPlacement) {
							targets |= uint64(1) << uint(targetInstance.OriginalIndex)
						}
					}
				}
				// Only targets that are still movable get geometric potential.
				targets |= ctx.potential.outgoingTargets[coveragePlacementKey(sourcePlacement)] &^ placedMask
			} else {
				// A free source may choose any option from the precomputed geometric
				// relaxation. Fixed-item conflicts are intentionally ignored here.
				targets = ctx.potential.instanceOutgoingTargets[sourceInstance.InstanceID]
			}
			targets &^= uint64(1) << uint(sourceInstance.OriginalIndex)
			count := bits.OnesCount64(targets)
			if count > starCount {
				count = starCount
			}
			upper[priorityIndex] += count
		}
	}
	return upper
}
