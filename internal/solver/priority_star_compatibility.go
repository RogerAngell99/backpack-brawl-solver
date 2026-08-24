package solver

import (
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

// priorityStarCompatibility is an immutable, execution-local projection of
// static source-item/star-slot compatibility onto inventory original indexes.
// A nil relation or missing entry deliberately falls back to the legacy
// predicate in the priority upper bound.
type priorityStarCompatibility struct {
	slotsBySourceOriginal [64][]uint64
	sourceOriginals       uint64
	targetOriginals       uint64
}

func newPriorityStarCompatibility(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	sourceItemIDs []string,
) *priorityStarCompatibility {
	if len(instances) == 0 || len(instances) > 64 {
		return nil
	}

	var originalIndexSeen [64]bool
	for _, instance := range instances {
		if instance.OriginalIndex < 0 || instance.OriginalIndex >= 64 || originalIndexSeen[instance.OriginalIndex] {
			return nil
		}
		originalIndexSeen[instance.OriginalIndex] = true
	}

	slotsByItem := make(map[string][]uint64, len(sourceItemIDs))
	for _, sourceItemID := range sourceItemIDs {
		if _, alreadyBuilt := slotsByItem[sourceItemID]; alreadyBuilt {
			continue
		}
		sourceItem, exists := catalog.Items[sourceItemID]
		if !exists {
			return nil
		}
		slots := make([]uint64, len(sourceItem.Stars))
		for starIndex := range sourceItem.Stars {
			for _, target := range instances {
				if scoring.StarMatchesCatalogItems(catalog, sourceItemID, target.ItemID, &sourceItem.Stars[starIndex]) {
					slots[starIndex] |= uint64(1) << uint(target.OriginalIndex)
				}
			}
		}
		slotsByItem[sourceItemID] = slots
	}

	compatibility := &priorityStarCompatibility{}
	for _, target := range instances {
		compatibility.targetOriginals |= uint64(1) << uint(target.OriginalIndex)
	}
	for _, source := range instances {
		if slots, exists := slotsByItem[source.ItemID]; exists {
			compatibility.slotsBySourceOriginal[source.OriginalIndex] = slots
			compatibility.sourceOriginals |= uint64(1) << uint(source.OriginalIndex)
		}
	}
	return compatibility
}

func (ctx *priorityStarCompatibility) match(sourceOriginal int, starIndex int, targetOriginal int) (match bool, cached bool) {
	if ctx == nil || sourceOriginal < 0 || sourceOriginal >= len(ctx.slotsBySourceOriginal) || targetOriginal < 0 || targetOriginal >= 64 {
		return false, false
	}
	sourceBit := uint64(1) << uint(sourceOriginal)
	targetBit := uint64(1) << uint(targetOriginal)
	if ctx.sourceOriginals&sourceBit == 0 || ctx.targetOriginals&targetBit == 0 {
		return false, false
	}
	slots := ctx.slotsBySourceOriginal[sourceOriginal]
	if starIndex < 0 || starIndex >= len(slots) {
		return false, false
	}
	return slots[starIndex]&targetBit != 0, true
}
