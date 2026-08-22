package solver

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestEquivalentCopiesHaveOneCanonicalSearchRepresentation(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	items := []string{"source", "source", "food", "food"}
	instances := ExpandInventory(items)
	grid := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, cat, instances, grid)
	canonical := []model.Placement{
		testPlacement(t, options["source#0"], model.Coord{Col: 0}, 0),
		testPlacement(t, options["source#1"], model.Coord{Col: 2}, 0),
		testPlacement(t, options["food#2"], model.Coord{Col: 1}, 0),
		testPlacement(t, options["food#3"], model.Coord{Col: 3}, 0),
	}
	permuted := []model.Placement{
		testPlacement(t, options["source#0"], model.Coord{Col: 2}, 0),
		testPlacement(t, options["source#1"], model.Coord{Col: 0}, 0),
		canonical[2], canonical[3],
	}
	config := Config{PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:source"}}
	if got, want := evaluateScoreForConfig(cat, canonical, config), evaluateScoreForConfig(cat, permuted, config); !reflect.DeepEqual(got, want) {
		t.Fatalf("equivalent copy scores differ: canonical=%+v permuted=%+v", got, want)
	}
	if got, want := physicalCellsKey(canonical), physicalCellsKey(permuted); got != want {
		t.Fatalf("equivalent copies changed physical cells: %q vs %q", got, want)
	}
	if got, want := canonicalLayoutHash(canonical), canonicalLayoutHash(permuted); got != want {
		t.Fatalf("equivalent copies have different canonical hashes: %q vs %q", got, want)
	}
	if placementRespectsCanonicalCopyOrder(permuted[0], []model.Placement{permuted[1]}) {
		t.Fatal("permuted copy labels passed canonical order")
	}
	tasks := buildTasks(instances, options, false, len(instances))
	seenPhysical := map[string]bool{}
	for _, task := range tasks {
		key := physicalLayoutClassKey(task.Placements)
		if seenPhysical[key] {
			t.Fatalf("search emitted duplicate equivalent layout %q", key)
		}
		seenPhysical[key] = true
	}
}

func TestPackingSeedHardPrunesDeadHighScoreState(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"bar":    {ID: "bar", Shape: []model.Coord{{}, {Col: 1}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "food", "bar"})
	grid := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, cat, instances, grid)
	result := packingSeedSearch(cat, instances, instances, options, Config{
		TopN:              1,
		AllowSkips:        false,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source"},
	}, grid, 10_000, nil)
	if result.HardPrunedNodes == 0 {
		t.Fatal("packing seed kept every branch; expected dead-state hard prune")
	}
	if len(result.Solutions) == 0 || len(result.Solutions[0].Placements) != len(instances) {
		t.Fatalf("packing seed did not retain a complete alternative: %+v", result)
	}
	if result.PackingDiagnostics.MaxDepth != len(instances) || len(result.PackingDiagnostics.LayerWidths) != len(instances) {
		t.Fatalf("packing diagnostics=%+v want depth and width for all layers", result.PackingDiagnostics)
	}
}

func TestPerInstanceCeilingCanBeReachedByEveryCopy(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {
			ID:        "source",
			Shape:     []model.Coord{{}},
			Stars:     []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}, {Offset: model.Coord{Col: 2}, TargetTypes: []string{"Food"}}},
			Rotations: []int{0},
		},
		"food": {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}, {Row: 1}, {Row: 2}}, Rotations: []int{0}},
	}}
	items := []string{"source", "source", "source", "food", "food"}
	instances := ExpandInventory(items)
	options := testOptionsByInstance(t, cat, instances)
	placements := []model.Placement{
		testPlacement(t, options["source#0"], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, options["source#1"], model.Coord{Row: 1, Col: 0}, 0),
		testPlacement(t, options["source#2"], model.Coord{Row: 2, Col: 0}, 0),
		testPlacement(t, options["food#3"], model.Coord{Row: 0, Col: 1}, 0),
		testPlacement(t, options["food#4"], model.Coord{Row: 0, Col: 2}, 0),
	}
	config := Config{PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:source"}}
	if got := evaluateScoreForConfig(cat, placements, config).PriorityCounts; !reflect.DeepEqual(got, []int{6}) {
		t.Fatalf("V3 score=%v want [6]", got)
	}
	ceiling := newPriorityBoundContext(cat, instances, config.Priorities, config.PrioritySemantics)
	if ceiling == nil || !ceiling.reached(model.Score{PriorityCounts: []int{6}}) {
		t.Fatalf("priority ceiling=%+v does not accept all copies at cap", ceiling)
	}
}

func TestLexicographicPrioritiesOutrankLaterScore(t *testing.T) {
	firstPriority := model.Score{PriorityCounts: []int{1, 0}, StarCount: 1}
	laterPriority := model.Score{PriorityCounts: []int{0, 100}, StarCount: 100}
	if compareScores(firstPriority, laterPriority) <= 0 {
		t.Fatalf("lexicographic ordering lost first priority: %+v vs %+v", firstPriority, laterPriority)
	}
}

func TestDiagnosticTraceRecordsActualDeterministicWork(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	config := Config{TopN: 1, AllowSkips: false, MaxNodes: 2_000, Workers: 1, Diagnostics: true}
	first, err := SolveLayout(cat, []string{"source", "food"}, grid, config)
	if err != nil {
		t.Fatalf("first diagnostic solve: %v", err)
	}
	second, err := SolveLayout(cat, []string{"source", "food"}, grid, config)
	if err != nil {
		t.Fatalf("second diagnostic solve: %v", err)
	}
	if len(first) == 0 || len(second) == 0 {
		t.Fatal("diagnostic solve returned no solutions")
	}

	firstStats := first[0].Search
	secondStats := second[0].Search
	if !firstStats.DiagnosticsEnabled || len(firstStats.IncumbentTrace) == 0 {
		t.Fatalf("missing diagnostic trace: %+v", firstStats)
	}
	if firstStats.GlobalBudgetConsumed > config.MaxNodes {
		t.Fatalf("charged work=%d exceeds MaxNodes=%d", firstStats.GlobalBudgetConsumed, config.MaxNodes)
	}
	if firstStats.FirstCompletePhase != tracePhaseDFS || firstStats.FirstCompleteNodes == 0 {
		t.Fatalf("first complete=%s/%d want an actual DFS candidate", firstStats.FirstCompletePhase, firstStats.FirstCompleteNodes)
	}
	if firstStats.FirstFullyPackedPhase != tracePhaseDFS {
		t.Fatalf("first fully packed phase=%q want %q", firstStats.FirstFullyPackedPhase, tracePhaseDFS)
	}
	if first[0].CanonicalLayoutHash == "" {
		t.Fatal("final solution has no canonical layout hash")
	}
	var charged int64
	for _, phase := range firstStats.PhaseWork {
		charged += phase.ChargedNodes
	}
	if charged != firstStats.GlobalBudgetConsumed {
		t.Fatalf("phase charged work=%d want global=%d", charged, firstStats.GlobalBudgetConsumed)
	}
	if len(firstStats.IncumbentTrace) != len(secondStats.IncumbentTrace) {
		t.Fatalf("trace lengths differ: %d vs %d", len(firstStats.IncumbentTrace), len(secondStats.IncumbentTrace))
	}
	for index := range firstStats.IncumbentTrace {
		left := firstStats.IncumbentTrace[index]
		right := secondStats.IncumbentTrace[index]
		if left.Phase != right.Phase || left.GlobalBudgetConsumed != right.GlobalBudgetConsumed || left.UnchargedWork != right.UnchargedWork || left.LayoutKey != right.LayoutKey || left.CanonicalLayoutHash != right.CanonicalLayoutHash || !reflect.DeepEqual(left.Score, right.Score) || !reflect.DeepEqual(left.Reasons, right.Reasons) {
			t.Fatalf("trace event %d is not deterministic: left=%+v right=%+v", index, left, right)
		}
		if left.StarUpperBounds.Structural < left.Score.StarCount || left.StarUpperBounds.Compatible < left.Score.StarCount || left.StarUpperBounds.Available < left.Score.StarCount || left.StarUpperBounds.GeometricRelaxed < left.Score.StarCount {
			t.Fatalf("trace event %d has non-optimistic star bound: event=%+v", index, left)
		}
	}
}

func TestStarUpperBoundsCoverEveryMicroScenarioCompletion(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "food", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, cat, instances, grid)
	upperBounds := newStarUpperBoundContext(cat, instances, options)
	if upperBounds == nil {
		t.Fatal("missing diagnostic star upper bounds")
	}

	var visit func(index int, occupied uint64, placements []model.Placement) (int, bool)
	visit = func(index int, occupied uint64, placements []model.Placement) (int, bool) {
		if index == len(instances) {
			return evaluateScoreForConfig(cat, placements, Config{}).StarCount, true
		}
		best := 0
		found := false
		instance := instances[index]
		for _, option := range options[instance.InstanceID] {
			if option.Mask&occupied != 0 || !placementRespectsCanonicalCopyOrder(option, placements) {
				continue
			}
			next, _ := insertPlacementSorted(append([]model.Placement(nil), placements...), option)
			if stars, complete := visit(index+1, occupied|option.Mask, next); complete && (!found || stars > best) {
				best = stars
				found = true
			}
		}
		if found {
			bounds := upperBounds.forPlacements(placements, instances)
			if bounds.Structural < best || bounds.Compatible < best || bounds.Available < best || bounds.GeometricRelaxed < best {
				t.Fatalf("partial placements=%v have pessimistic bounds=%+v for max stars=%d", placements, bounds, best)
			}
		}
		return best, found
	}
	if _, complete := visit(0, 0, nil); !complete {
		t.Fatal("micro scenario unexpectedly has no complete layouts")
	}
}

func physicalCellsKey(placements []model.Placement) string {
	cells := make([]string, 0)
	for _, placement := range placements {
		for _, cell := range placement.Cells {
			cells = append(cells, string(rune('A'+cell.Row))+string(rune('A'+cell.Col)))
		}
	}
	sort.Strings(cells)
	return strings.Join(cells, ",")
}

func physicalLayoutClassKey(placements []model.Placement) string {
	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		parts = append(parts, placement.ItemID+":"+placementKey(placement))
	}
	sort.Strings(parts)
	return strings.Join(parts, ";")
}

func testOptionsForGrid(t testing.TB, cat model.Catalog, instances []model.InventoryInstance, grid uint64) map[string][]model.Placement {
	t.Helper()
	options := make(map[string][]model.Placement, len(instances))
	for _, instance := range instances {
		placements, err := PlacementOptions(cat, instance, grid)
		if err != nil {
			t.Fatalf("placement options for %s: %v", instance.InstanceID, err)
		}
		options[instance.InstanceID] = placements
	}
	return options
}
