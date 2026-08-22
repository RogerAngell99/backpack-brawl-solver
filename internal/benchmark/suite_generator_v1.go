package benchmark

import (
	"fmt"
	"math/rand"
	"sort"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

// search-suite-generator-v1 is frozen.
//
// Changes that alter generated scenario semantics require a new generator
// version. Do not "fix" v1 by changing its output; introduce v2 instead.
func materializeGeneratedSearchSuiteCaseV1(catalog model.Catalog, entry GeneratedSearchSuiteCase) (scenario.Scenario, error) {
	if entry.Seed == nil {
		return scenario.Scenario{}, fmt.Errorf("generated case %q has no public seed", entry.ID)
	}
	if entry.Family != GeneratedFamilySparse && entry.Family != GeneratedFamilyDuplicated && entry.Family != GeneratedFamilyLoose {
		return scenario.Scenario{}, fmt.Errorf("generated case %q has unsupported family %q", entry.ID, entry.Family)
	}
	random := rand.New(rand.NewSource(*entry.Seed))
	sources := sortedStarSourcesV1(catalog)
	if len(sources) < 2 {
		return scenario.Scenario{}, fmt.Errorf("catalog has fewer than two star sources")
	}
	random.Shuffle(len(sources), func(left, right int) { sources[left], sources[right] = sources[right], sources[left] })
	var sourceIDs []string
	var targetIDs []string
	for left := 0; left < len(sources)-1 && len(sourceIDs) == 0; left++ {
		for right := left + 1; right < len(sources); right++ {
			targets := compatibleTargetIDsV1(catalog, []string{sources[left], sources[right]})
			if len(targets) < 2 {
				continue
			}
			sourceIDs = []string{sources[left], sources[right]}
			targetIDs = targets
			break
		}
	}
	if len(sourceIDs) != 2 {
		return scenario.Scenario{}, fmt.Errorf("catalog has no compatible two-source family")
	}
	random.Shuffle(len(targetIDs), func(left, right int) { targetIDs[left], targetIDs[right] = targetIDs[right], targetIDs[left] })
	targetCount := 3
	copyCount := 1
	desiredItemCount := 14
	switch entry.Family {
	case GeneratedFamilyDuplicated:
		targetCount = 5
		copyCount = 2
		desiredItemCount = 18
	case GeneratedFamilyLoose:
		targetCount = 8
		desiredItemCount = 18
	}
	if targetCount > len(targetIDs) {
		targetCount = len(targetIDs)
	}
	items := map[string]int{sourceIDs[0]: copyCount, sourceIDs[1]: copyCount}
	for _, targetID := range targetIDs[:targetCount] {
		items[targetID]++
	}
	fillGeneratedSuiteInventoryV1(catalog, random, items, sourceIDs, targetIDs[:targetCount], desiredItemCount)
	top := 1
	workers := 1
	noSkips := true
	repair := true
	generated := scenario.Scenario{
		Name:              entry.ID,
		Grid:              []string{"111111111", "111111111", "111111111", "111111111", "111111111", "111111111"},
		Items:             items,
		Top:               &top,
		Workers:           &workers,
		NoSkips:           &noSkips,
		RepairSearch:      &repair,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:" + sourceIDs[0], "star_source:" + sourceIDs[1]},
	}
	return generated, generated.Validate()
}

func fillGeneratedSuiteInventoryV1(catalog model.Catalog, random *rand.Rand, items map[string]int, sourceIDs []string, targetIDs []string, desiredItemCount int) {
	if desiredItemCount <= generatedSuiteItemCountV1(items) {
		return
	}
	excluded := make(map[string]struct{}, len(sourceIDs)+len(targetIDs))
	for _, itemID := range sourceIDs {
		excluded[itemID] = struct{}{}
	}
	for _, itemID := range targetIDs {
		excluded[itemID] = struct{}{}
	}
	fillers := make([]string, 0)
	for itemID, item := range catalog.Items {
		if _, excluded := excluded[itemID]; excluded || len(item.Shape) == 0 || len(item.Shape) > 3 {
			continue
		}
		fillers = append(fillers, itemID)
	}
	sort.Strings(fillers)
	random.Shuffle(len(fillers), func(left, right int) { fillers[left], fillers[right] = fillers[right], fillers[left] })
	for _, itemID := range fillers {
		if generatedSuiteItemCountV1(items) >= desiredItemCount || generatedSuiteAreaV1(catalog, items)+len(catalog.Items[itemID].Shape) > 42 {
			break
		}
		items[itemID]++
	}
}

func generatedSuiteItemCountV1(items map[string]int) int {
	total := 0
	for _, count := range items {
		total += count
	}
	return total
}

func generatedSuiteAreaV1(catalog model.Catalog, items map[string]int) int {
	total := 0
	for itemID, count := range items {
		total += len(catalog.Items[itemID].Shape) * count
	}
	return total
}

func sortedStarSourcesV1(catalog model.Catalog) []string {
	result := make([]string, 0)
	for itemID, item := range catalog.Items {
		if len(item.Stars) > 0 && len(item.Shape) <= 4 {
			result = append(result, itemID)
		}
	}
	sort.Strings(result)
	return result
}

func compatibleTargetIDsV1(catalog model.Catalog, sourceIDs []string) []string {
	sourceSet := make(map[string]struct{}, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		sourceSet[sourceID] = struct{}{}
	}
	result := make([]string, 0)
	for itemID, item := range catalog.Items {
		if _, source := sourceSet[itemID]; source || len(item.Shape) > 4 {
			continue
		}
		if targetCompatibleWithSourcesV1(item, sourceIDs, catalog) {
			result = append(result, itemID)
		}
	}
	sort.Strings(result)
	return result
}

func targetCompatibleWithSourcesV1(target model.Item, sourceIDs []string, catalog model.Catalog) bool {
	for _, sourceID := range sourceIDs {
		source := catalog.Items[sourceID]
		for _, star := range source.Stars {
			for _, itemID := range star.TargetItems {
				if itemID == target.ID {
					return true
				}
			}
			for _, targetType := range star.TargetTypes {
				for _, itemType := range target.Types {
					if targetType == itemType {
						return true
					}
				}
			}
		}
	}
	return false
}
