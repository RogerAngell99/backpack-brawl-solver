package solver

import (
	"sort"
	"testing"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

func TestExactBoundScoreIsOptimistic(t *testing.T) {
	cat := exactBoundFixtureCatalog()
	items := []string{
		"source",
		"any_source",
		"crystal",
		"weapon",
		"armor",
		"starlight_potion",
		"anchor",
		"ingredient",
	}
	config := Config{
		TopN:       1,
		AllowSkips: true,
		MaxNodes:   0,
		Workers:    1,
		CoverageGroups: []model.CoverageGroup{
			{Name: "Weapons", Sources: []string{"source", "any_source"}, Targets: []string{"weapon", "armor"}},
			{Name: "Bloom", Sources: []string{"crystal"}, Targets: []string{"starlight_potion"}},
		},
		Priorities: []string{"star_source:crystal", "craft:crafted"},
	}
	ctx, ordered, optionsByInstance := exactBoundContextForTest(t, cat, items, config)
	placements := []model.Placement{
		testPlacement(t, optionsByInstance["source#0"], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, optionsByInstance["any_source#1"], model.Coord{Row: 1, Col: 0}, 0),
		testPlacement(t, optionsByInstance["crystal#2"], model.Coord{Row: 3, Col: 0}, 0),
		testPlacement(t, optionsByInstance["weapon#3"], model.Coord{Row: 0, Col: 1}, 0),
		testPlacement(t, optionsByInstance["armor#4"], model.Coord{Row: 2, Col: 0}, 0),
		testPlacement(t, optionsByInstance["starlight_potion#5"], model.Coord{Row: 3, Col: 1}, 0),
		testPlacement(t, optionsByInstance["anchor#6"], model.Coord{Row: 4, Col: 0}, 0),
		testPlacement(t, optionsByInstance["ingredient#7"], model.Coord{Row: 4, Col: 1}, 0),
	}
	evaluation := scoring.EvaluateLayoutWithCoverageGroups(cat, placements, config.Priorities, config.CoverageGroups)
	bound, ok := ctx.boundScore(ctx.initialState(cat, ordered, 0, nil), 0)
	if !ok {
		t.Fatal("expected complete exact bound")
	}
	if compareScores(bound, evaluation.Score) < 0 {
		t.Fatalf("bound score is pessimistic: bound=%+v actual=%+v", bound, evaluation.Score)
	}
}

func TestExactBoundGlobalPriorityOrderIsOptimistic(t *testing.T) {
	cat := exactBoundFixtureCatalog()
	items := []string{
		"source",
		"any_source",
		"crystal",
		"weapon",
		"armor",
		"starlight_potion",
		"anchor",
		"ingredient",
	}
	config := Config{
		TopN:       1,
		AllowSkips: true,
		MaxNodes:   0,
		Workers:    1,
		CoverageGroups: []model.CoverageGroup{
			{Name: "Weapons", Sources: []string{"source", "any_source"}, Targets: []string{"weapon", "armor"}},
			{Name: "Bloom", Sources: []string{"crystal"}, Targets: []string{"starlight_potion"}},
		},
		Priorities: []string{"coverage_group:1", "craft:crafted", "coverage_group:0", "star_source:crystal"},
	}
	ctx, ordered, optionsByInstance := exactBoundContextForTest(t, cat, items, config)
	placements := []model.Placement{
		testPlacement(t, optionsByInstance["source#0"], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, optionsByInstance["any_source#1"], model.Coord{Row: 1, Col: 0}, 0),
		testPlacement(t, optionsByInstance["crystal#2"], model.Coord{Row: 3, Col: 0}, 0),
		testPlacement(t, optionsByInstance["weapon#3"], model.Coord{Row: 0, Col: 1}, 0),
		testPlacement(t, optionsByInstance["armor#4"], model.Coord{Row: 2, Col: 0}, 0),
		testPlacement(t, optionsByInstance["starlight_potion#5"], model.Coord{Row: 3, Col: 1}, 0),
		testPlacement(t, optionsByInstance["anchor#6"], model.Coord{Row: 4, Col: 0}, 0),
		testPlacement(t, optionsByInstance["ingredient#7"], model.Coord{Row: 4, Col: 1}, 0),
	}
	evaluation := scoring.EvaluateLayoutWithCoverageGroups(cat, placements, config.Priorities, config.CoverageGroups)
	bound, ok := ctx.boundScore(ctx.initialState(cat, ordered, 0, nil), 0)
	if !ok {
		t.Fatal("expected complete exact bound")
	}
	if compareScores(bound, evaluation.Score) < 0 {
		t.Fatalf("bound score is pessimistic: bound=%+v actual=%+v", bound, evaluation.Score)
	}
	if got, want := len(bound.PriorityCounts), len(evaluation.Score.PriorityCounts); got != want {
		t.Fatalf("bound priority count len=%d want %d: bound=%v actual=%v", got, want, bound.PriorityCounts, evaluation.Score.PriorityCounts)
	}
}

func TestExactBoundDoesNotPruneScoreTie(t *testing.T) {
	cat := exactBoundSimpleCatalog()
	items := []string{"a", "b"}
	config := Config{TopN: 1, AllowSkips: true, MaxNodes: 0, Workers: 1, Priorities: []string{"craft:missing"}}
	ctx, _, _ := exactBoundContextForTest(t, cat, items, config)
	state := exactBoundState{}
	bound, ok := ctx.boundScore(state, 0)
	if !ok {
		t.Fatal("expected complete exact bound")
	}
	results := []model.Solution{{
		Evaluation: model.Evaluation{Score: bound},
		LayoutKey:  "999|tie",
	}}
	if ctx.shouldPrune(state, 0, results, 1) {
		t.Fatal("exact bound pruned a score tie")
	}
}

func TestExactBoundIncompleteContextDoesNotPrune(t *testing.T) {
	var ctx *exactBoundContext
	results := []model.Solution{{Evaluation: model.Evaluation{Score: model.Score{ItemCount: 99}}}}
	if ctx.shouldPrune(exactBoundState{}, 0, results, 1) {
		t.Fatal("nil exact bound context pruned")
	}
}

func TestExactBoundsPreserveExhaustiveBestAndPrune(t *testing.T) {
	cat := exactBoundSimpleCatalog()
	items := []string{"a", "b", "c", "d"}
	config := Config{TopN: 1, AllowSkips: true, MaxNodes: 0, Workers: 1, Priorities: []string{"craft:missing"}}
	gridMask := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	withoutBounds, err := SolveLayout(cat, items, gridMask, withExactBoundsDisabled(config))
	if err != nil {
		t.Fatalf("SolveLayout without bounds: %v", err)
	}
	withBounds, err := SolveLayout(cat, items, gridMask, config)
	if err != nil {
		t.Fatalf("SolveLayout with bounds: %v", err)
	}
	assertSameTopSolutions(t, withoutBounds, withBounds)
	if withBounds[0].Search.ExactBoundPrunedNodes == 0 {
		t.Fatal("expected exact bounds to prune at least one exhaustive branch")
	}
}

func TestExactBoundsPreserveExhaustiveBestWithNoSkips(t *testing.T) {
	cat := exactBoundFixtureCatalog()
	items := []string{"source", "weapon", "any_source", "armor", "anchor", "ingredient"}
	gridMask := mustParseGridForTest(t, "111000000/111000000/000000000/000000000/000000000/000000000")
	config := Config{
		TopN:       3,
		AllowSkips: false,
		MaxNodes:   0,
		Workers:    1,
		Priorities: []string{"star_source:source", "craft:crafted"},
	}
	withoutBounds, err := SolveLayout(cat, items, gridMask, withExactBoundsDisabled(config))
	if err != nil {
		t.Fatalf("SolveLayout without bounds: %v", err)
	}
	withBounds, err := SolveLayout(cat, items, gridMask, config)
	if err != nil {
		t.Fatalf("SolveLayout with bounds: %v", err)
	}
	assertSameTopSolutions(t, withoutBounds, withBounds)
}

func mustParseGridForTest(t *testing.T, value string) uint64 {
	t.Helper()
	gridMask, err := geometry.ParseGridText(value)
	if err != nil {
		t.Fatalf("ParseGridText(%q): %v", value, err)
	}
	return gridMask
}

func withExactBoundsDisabled(config Config) Config {
	config.DisableExactBounds = true
	return config
}

func assertSameTopSolutions(t *testing.T, left []model.Solution, right []model.Solution) {
	t.Helper()
	if len(left) != len(right) {
		t.Fatalf("solution counts differ: %d vs %d", len(left), len(right))
	}
	for idx := range left {
		if compareScores(left[idx].Evaluation.Score, right[idx].Evaluation.Score) != 0 {
			t.Fatalf("score mismatch at %d: %+v vs %+v", idx, left[idx].Evaluation.Score, right[idx].Evaluation.Score)
		}
		if left[idx].LayoutKey != right[idx].LayoutKey {
			t.Fatalf("layout key mismatch at %d: %q vs %q", idx, left[idx].LayoutKey, right[idx].LayoutKey)
		}
	}
}

func exactBoundContextForTest(t *testing.T, cat model.Catalog, items []string, config Config) (*exactBoundContext, []model.InventoryInstance, map[string][]model.Placement) {
	t.Helper()
	instances := ExpandInventory(items)
	optionsByInstance := testOptionsByInstance(t, cat, instances)
	coverage := newCoverageContextForConfig(cat, instances, optionsByInstance, config)
	sortPlacementOptionsForCoverage(optionsByInstance, coverage)
	ordered := append([]model.InventoryInstance(nil), instances...)
	limitedMode := config.MaxNodes > 0
	sort.Slice(ordered, func(i, j int) bool {
		if limitedMode {
			leftPriority := instancePriority(cat, ordered[i], config.Priorities, coverage)
			rightPriority := instancePriority(cat, ordered[j], config.Priorities, coverage)
			if leftPriority != rightPriority {
				return leftPriority > rightPriority
			}
		}
		left := len(optionsByInstance[ordered[i].InstanceID])
		right := len(optionsByInstance[ordered[j].InstanceID])
		if left != right {
			return left < right
		}
		return ordered[i].OriginalIndex < ordered[j].OriginalIndex
	})
	if coverage != nil {
		coverage.prepareOrder(ordered)
	}
	ctx := newExactBoundContext(cat, instances, ordered, optionsByInstance, config)
	if ctx == nil {
		t.Fatal("expected exact bound context")
	}
	return ctx, ordered, optionsByInstance
}

func exactBoundSimpleCatalog() model.Catalog {
	return model.Catalog{
		Items: map[string]model.Item{
			"a": {ID: "a", Name: "A", Shape: []model.Coord{{Row: 0, Col: 0}}, Rotations: []int{0}},
			"b": {ID: "b", Name: "B", Shape: []model.Coord{{Row: 0, Col: 0}}, Rotations: []int{0}},
			"c": {ID: "c", Name: "C", Shape: []model.Coord{{Row: 0, Col: 0}}, Rotations: []int{0}},
			"d": {ID: "d", Name: "D", Shape: []model.Coord{{Row: 0, Col: 0}}, Rotations: []int{0}},
		},
	}
}

func exactBoundFixtureCatalog() model.Catalog {
	return model.Catalog{
		Items: map[string]model.Item{
			"source": {
				ID:        "source",
				Name:      "Source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"Weapon"}}},
				Rotations: []int{0},
			},
			"any_source": {
				ID:        "any_source",
				Name:      "Any Source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 1, Col: 0}}},
				Rotations: []int{0},
			},
			"crystal": {
				ID:        "crystal",
				Name:      "Crystal",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetItems: []string{"starbloom"}}},
				Rotations: []int{0},
			},
			"weapon": {
				ID:        "weapon",
				Name:      "Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"armor": {
				ID:        "armor",
				Name:      "Armor",
				Types:     []string{"Armor"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"starlight_potion": {
				ID:        "starlight_potion",
				Name:      "Starlight Potion",
				CountsAs:  []model.ItemAlias{{ItemID: "starbloom", Count: 1}},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"anchor": {
				ID:        "anchor",
				Name:      "Anchor",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"ingredient": {
				ID:        "ingredient",
				Name:      "Ingredient",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
		Recipes: []model.Recipe{
			{Result: "crafted", Anchor: "anchor", Ingredients: []string{"ingredient"}},
		},
	}
}
