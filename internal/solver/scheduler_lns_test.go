package solver

import (
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestTaskPolicyReservesMinimumBeforeV3Weighting(t *testing.T) {
	high := model.Placement{ItemID: "spice", Origin: model.Coord{Row: 0, Col: 0}}
	low := model.Placement{ItemID: "spice", Origin: model.Coord{Row: 0, Col: 1}}
	tasks := []searchTask{{Placements: []model.Placement{high}}, {Placements: []model.Placement{low}}, {}}
	potential := &starPotentialContext{priorityPlacementPotential: map[string]int{
		coveragePlacementKey(high): 9,
		coveragePlacementKey(low):  0,
	}}
	policy := &ResolvedSearchPolicy{MinAllocatedNodesPerTask: 200}
	assignBudgetsWithPolicy(tasks, 1_000, potential, model.PrioritySemanticsOutgoingPerInstanceV3, policy)
	var total int64
	for index, task := range tasks {
		if task.NodeBudget < 200 {
			t.Fatalf("task %d budget=%d want at least 200", index, task.NodeBudget)
		}
		total += task.NodeBudget
	}
	if total != 1_000 || tasks[0].NodeBudget <= tasks[1].NodeBudget {
		t.Fatalf("weighted allocation=%d/%d/%d total=%d", tasks[0].NodeBudget, tasks[1].NodeBudget, tasks[2].NodeBudget, total)
	}
}

func TestTaskPolicyPreventsSingleWorkerTaskExplosion(t *testing.T) {
	ordered := []model.InventoryInstance{{InstanceID: "item#0", ItemID: "item"}}
	options := make([]model.Placement, 10)
	for index := range options {
		options[index] = model.Placement{InstanceID: "item#0", Mask: uint64(1) << uint(index)}
	}
	config := Config{
		AllowSkips: false,
		MaxNodes:   10_000,
		Workers:    1,
		policy: &ResolvedSearchPolicy{
			MinAllocatedNodesPerTask: 128,
			MaxTasksPerWorker:        2,
			MaxInitialSplitDepth:     2,
		},
	}
	if depth := initialSplitDepth(ordered, map[string][]model.Placement{"item#0": options}, config); depth != 0 {
		t.Fatalf("single-worker split depth=%d want 0 when first split exceeds task cap", depth)
	}
}

func TestPlateauRepairFindsPriorityPreservingRetarget(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	instances := ExpandInventory([]string{"source", "food", "food"})
	options := testOptionsForGrid(t, catalog, instances, grid)
	basePlacements := []model.Placement{
		testPlacement(t, options["source#0"], model.Coord{Col: 0}, 0),
		testPlacement(t, options["food#1"], model.Coord{Col: 1}, 0),
		testPlacement(t, options["food#2"], model.Coord{Col: 3}, 0),
	}
	config := Config{
		TopN:                 8,
		AllowSkips:           false,
		Workers:              1,
		PrioritySemantics:    model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:           []string{"star_source:source"},
		repairPriorityTarget: []int{1},
	}
	base := model.Solution{
		Placements:          basePlacements,
		Evaluation:          evaluateLayoutForConfig(catalog, basePlacements, config),
		LayoutKey:           layoutKey(basePlacements, instances),
		CanonicalLayoutHash: canonicalLayoutHash(basePlacements),
	}
	byID := map[string]model.InventoryInstance{}
	for _, instance := range instances {
		byID[instance.InstanceID] = instance
	}
	result := runRepairNeighborhood(
		catalog,
		instances,
		byID,
		options,
		[]model.Solution{base},
		repairNeighborhood{Operator: "test", InstanceIDs: []string{"source#0", "food#1"}, BaseLayoutKey: base.LayoutKey},
		config,
		nil,
		nil,
		grid,
		20_000,
		nil,
	)
	if result.PriorityPreservingCandidates == 0 || result.PriorityBoundPruned < 0 {
		t.Fatalf("priority repair counters=%+v", result)
	}
	foundRetarget := false
	for _, candidate := range result.Solutions {
		if comparePriorityCounts(candidate.Evaluation.Score.PriorityCounts, []int{1}) < 0 {
			continue
		}
		for _, star := range candidate.Evaluation.Stars {
			if star.SourceInstance == "source#0" && star.TargetInstance == "food#2" {
				foundRetarget = true
			}
		}
	}
	if !foundRetarget {
		t.Fatalf("repair did not retain a priority-preserving retarget: %+v", result.Solutions)
	}
}
