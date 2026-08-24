package solver

import (
	"testing"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

func TestPartialRepairBoundsAreOptimisticForAllCompletions(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {
			ID:        "source",
			Shape:     []model.Coord{{}},
			Stars:     []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}, {Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}, {Offset: model.Coord{Col: 2}, TargetTypes: []string{"Food"}}},
			Rotations: []int{0},
		},
		"food": {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111110000/000000000/000000000/000000000/000000000/000000000")
	instances := ExpandInventory([]string{"source", "source", "food", "food"})
	optionsByInstance := make(map[string][]model.Placement, len(instances))
	for _, instance := range instances {
		options, err := PlacementOptions(catalog, instance, grid)
		if err != nil {
			t.Fatalf("PlacementOptions(%s): %v", instance.InstanceID, err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	fixedSource := partialRepairBoundPlacementAt(t, optionsByInstance["source#0"], 0)
	currentFood := partialRepairBoundPlacementAt(t, optionsByInstance["food#2"], 1)
	state := partialRepairState{
		FixedPlacements:   []model.Placement{fixedSource},
		CurrentPlacements: []model.Placement{currentFood},
		RemovedInstances:  []model.InventoryInstance{instances[1], instances[3]},
		FreeCells:         grid &^ (fixedSource.Mask | currentFood.Mask),
	}
	priorities := []string{"star_source:source"}
	priorityUpper := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, nil)
	starUpper := partialRelaxedStarUpperBound(catalog, state, optionsByInstance)
	if got := len(partialRepairFixedStars(catalog, state)); got != 1 {
		t.Fatalf("fixed stars=%d want 1", got)
	}
	if got, want := partialRepairFixedStarHeadroom(catalog, state, optionsByInstance), starUpper-1; got != want {
		t.Fatalf("fixed star headroom=%d want %d", got, want)
	}
	if !partialRepairTargetVectorFeasible(priorityUpper, priorityUpper) {
		t.Fatal("priority upper vector rejected itself")
	}
	if partialRepairTargetVectorFeasible(priorityUpper, []int{priorityUpper[0] + 1}) {
		t.Fatal("priority upper vector accepted a strictly higher target")
	}

	removed := state.unplacedRemoved(state.anchoredPlacements())
	removedOptions := state.filteredRemovedOptions(removed, optionsByInstance)
	occupied := partialRepairOccupied(state.anchoredPlacements())
	completions := 0
	var visit func(int, uint64, []model.Placement)
	visit = func(index int, used uint64, placements []model.Placement) {
		if index == len(removed) {
			completions++
			complete := append(state.anchoredPlacements(), placements...)
			score := scoring.EvaluateScoreOnlyWithCoverageGroupsAndSemantics(
				catalog,
				complete,
				priorities,
				nil,
				model.PrioritySemanticsOutgoingPerInstanceV3,
			)
			if !partialRepairTargetVectorFeasible(priorityUpper, score.PriorityCounts) {
				t.Fatalf("priority bound=%v is below completion=%v", priorityUpper, score.PriorityCounts)
			}
			if starUpper < score.StarCount {
				t.Fatalf("star bound=%d is below completion=%d", starUpper, score.StarCount)
			}
			return
		}
		for _, option := range removedOptions[removed[index].InstanceID] {
			if option.Mask&used != 0 {
				continue
			}
			visit(index+1, used|option.Mask, append(placements, option))
		}
	}
	visit(0, occupied, nil)
	if completions == 0 {
		t.Fatal("expected at least one completion")
	}
}

func partialRepairBoundPlacementAt(t *testing.T, options []model.Placement, col int) model.Placement {
	t.Helper()
	for _, option := range options {
		if option.Origin.Row == 0 && option.Origin.Col == col && option.Rotation == 0 {
			return option
		}
	}
	t.Fatalf("missing placement at column %d", col)
	return model.Placement{}
}
