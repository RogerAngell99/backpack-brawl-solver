package solver

import (
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

// priorityBoundContext holds an optimistic structural ceiling for the V3
// priority vector. Reaching this ceiling proves no later search can improve a
// priority, but it deliberately says nothing about star-count tie breakers.
type priorityBoundContext struct {
	ceiling           []int
	starCompatibility *priorityStarCompatibility
}

func newPriorityBoundContext(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	priorities []string,
	semantics model.PrioritySemantics,
) *priorityBoundContext {
	if semantics != model.PrioritySemanticsOutgoingPerInstanceV3 || len(priorities) == 0 {
		return nil
	}
	ceiling := make([]int, 0, len(priorities))
	sourceItemIDs := make([]string, 0, len(priorities))
	for _, priority := range priorities {
		kind, sourceItemID, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" {
			// Craft ceilings depend on recipes and packing, so do not claim a
			// complete priority ceiling when one is present.
			return nil
		}
		source, exists := catalog.Items[sourceItemID]
		if !exists {
			return nil
		}
		maxForSource := 0
		for _, sourceInstance := range instances {
			if sourceInstance.ItemID != sourceItemID {
				continue
			}
			compatibleTargets := 0
			for _, targetInstance := range instances {
				if targetInstance.InstanceID == sourceInstance.InstanceID {
					continue
				}
				for starIndex := range source.Stars {
					if scoring.StarMatchesCatalogItems(catalog, sourceItemID, targetInstance.ItemID, &source.Stars[starIndex]) {
						compatibleTargets++
						break
					}
				}
			}
			if compatibleTargets < len(source.Stars) {
				maxForSource += compatibleTargets
			} else {
				maxForSource += len(source.Stars)
			}
		}
		ceiling = append(ceiling, maxForSource)
		sourceItemIDs = append(sourceItemIDs, sourceItemID)
	}
	return &priorityBoundContext{
		ceiling:           ceiling,
		starCompatibility: newPriorityStarCompatibility(catalog, instances, sourceItemIDs),
	}
}

func (ctx *priorityBoundContext) staticStarCompatibility() *priorityStarCompatibility {
	if ctx == nil {
		return nil
	}
	return ctx.starCompatibility
}

func (ctx *priorityBoundContext) reached(score model.Score) bool {
	if ctx == nil || len(ctx.ceiling) == 0 || len(score.PriorityCounts) < len(ctx.ceiling) {
		return false
	}
	for index, ceiling := range ctx.ceiling {
		if score.PriorityCounts[index] < ceiling {
			return false
		}
	}
	return true
}
