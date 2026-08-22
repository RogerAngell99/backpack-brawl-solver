package solver

import (
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestOutgoingBoundPreservesExhaustiveV3Result(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	config := Config{
		TopN:              1,
		AllowSkips:        false,
		MaxNodes:          0,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source"},
	}
	withoutBound := config
	withoutBound.DisableOutgoingBounds = true
	without, err := SolveLayout(cat, []string{"source", "source", "food", "food"}, grid, withoutBound)
	if err != nil {
		t.Fatalf("solve without outgoing bound: %v", err)
	}
	with, err := SolveLayout(cat, []string{"source", "source", "food", "food"}, grid, config)
	if err != nil {
		t.Fatalf("solve with outgoing bound: %v", err)
	}
	assertSameTopSolutions(t, without, with)
	instances := ExpandInventory([]string{"source", "source", "food", "food"})
	options := testOptionsForGrid(t, cat, instances, grid)
	bound := newOutgoingBoundContext(cat, instances, options, config, nil)
	deadPrefix := []model.Placement{
		testPlacement(t, options["source#0"], model.Coord{Col: 2}, 0),
		testPlacement(t, options["source#1"], model.Coord{Col: 3}, 0),
		testPlacement(t, options["food#2"], model.Coord{Col: 0}, 0),
		testPlacement(t, options["food#3"], model.Coord{Col: 1}, 0),
	}
	if !bound.shouldPrune(deadPrefix, []model.Solution{{Evaluation: model.Evaluation{Score: model.Score{PriorityCounts: []int{1}}}}}, 1) {
		t.Fatal("outgoing bound did not prune a strictly worse complete prefix")
	}
}

func TestOutgoingBoundNeverPrunesPriorityTieForLateTieBreakers(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "food", "food"})
	options := testOptionsByInstance(t, cat, instances)
	config := Config{
		TopN:              1,
		AllowSkips:        true,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source"},
	}
	bound := newOutgoingBoundContext(cat, instances, options, config, nil)
	upper := bound.upperPriorityCounts(nil)
	incumbent := model.Solution{Evaluation: model.Evaluation{Score: model.Score{
		PriorityCounts:                upper,
		StarCount:                     1,
		StarTargetBreadth:             0,
		StarReciprocalPairs:           0,
		StarSourceDefinitionDiversity: 0,
	}}}
	if bound.shouldPrune(nil, []model.Solution{incumbent}, 1) {
		t.Fatal("outgoing UB pruned an equal priority bound while later score fields may improve")
	}
}
