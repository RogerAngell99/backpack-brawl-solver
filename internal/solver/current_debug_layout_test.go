package solver

import (
	"path/filepath"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
)

func TestCurrentDebugSkippedItemsFitUnusedSpace(t *testing.T) {
	cat, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	instances := ExpandInventory([]string{
		"apple", "cactus", "cactus", "cactus", "champion_s_ripper", "defender_s_shield", "errant_lance",
		"fly_agaric", "ginseng_root", "gloves_of_power", "heater_shield", "magic_essence", "pitahaya",
		"shield_spikes", "steadfast_boots", "vampire_bat", "venomous_pincer",
	})
	optionsByInstance := testOptionsByInstance(t, cat, instances)
	placements := []model.Placement{
		testPlacement(t, optionsByInstance["apple#0"], model.Coord{Row: 1, Col: 2}, 0),
		testPlacement(t, optionsByInstance["cactus#1"], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, optionsByInstance["cactus#2"], model.Coord{Row: 2, Col: 1}, 0),
		testPlacement(t, optionsByInstance["cactus#3"], model.Coord{Row: 0, Col: 5}, 0),
		testPlacement(t, optionsByInstance["errant_lance#6"], model.Coord{Row: 1, Col: 0}, 0),
		testPlacement(t, optionsByInstance["fly_agaric#7"], model.Coord{Row: 2, Col: 4}, 90),
		testPlacement(t, optionsByInstance["ginseng_root#8"], model.Coord{Row: 0, Col: 2}, 0),
		testPlacement(t, optionsByInstance["magic_essence#11"], model.Coord{Row: 1, Col: 4}, 0),
		testPlacement(t, optionsByInstance["pitahaya#12"], model.Coord{Row: 1, Col: 3}, 0),
		testPlacement(t, optionsByInstance["venomous_pincer#16"], model.Coord{Row: 1, Col: 1}, 0),
	}
	occupied := uint64(0)
	for _, placement := range placements {
		occupied |= placement.Mask
	}
	skipped := []string{
		"champion_s_ripper#4", "defender_s_shield#5", "gloves_of_power#9", "heater_shield#10",
		"shield_spikes#13", "steadfast_boots#14", "vampire_bat#15",
	}
	for _, instanceID := range skipped {
		found := false
		for _, option := range optionsByInstance[instanceID] {
			if option.Mask&occupied == 0 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s has no non-overlapping option", instanceID)
		}
	}
	var pack func(index int, mask uint64) bool
	pack = func(index int, mask uint64) bool {
		if index == len(skipped) {
			return true
		}
		for _, option := range optionsByInstance[skipped[index]] {
			if option.Mask&mask == 0 && pack(index+1, mask|option.Mask) {
				return true
			}
		}
		return false
	}
	if !pack(0, occupied) {
		t.Fatal("all skipped items cannot fit together despite individually fitting")
	}
	config := Config{
		AllowSkips:        true,
		MaxRefineMoves:    25000,
		PrioritySemantics: model.PrioritySemanticsOutgoingV2,
		Priorities: []string{
			"star_source:errant_lance", "star_source:venomous_pincer", "star_source:gloves_of_power",
			"star_source:magic_essence", "star_source:steadfast_boots", "star_source:pitahaya",
			"star_source:fly_agaric", "star_source:shield_spikes", "star_source:ginseng_root", "star_source:apple",
			"craft:cactrio",
		},
	}
	solution := model.Solution{
		Placements: placements,
		Evaluation: evaluateLayoutForConfig(cat, placements, config),
		LayoutKey:  layoutKey(placements, instances),
	}
	completed, stats, err := completeSkippedSolution(cat, instances, optionsByInstance, solution, config, completionMoveLimit(config.MaxRefineMoves))
	if err != nil {
		t.Fatalf("complete skipped solution: %v", err)
	}
	if stats.Improvements == 0 || len(completed.Placements) != len(instances) {
		t.Fatalf("completion stats=%+v placements=%d want %d", stats, len(completed.Placements), len(instances))
	}
}
