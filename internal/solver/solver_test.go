package solver

import (
	"context"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

func loadTestCatalog(t testing.TB) model.Catalog {
	t.Helper()
	// These placement regressions intentionally use the curated fixture; production uses the runtime projection.
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog-curated.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return loaded
}

func TestInventoryCountSyntax(t *testing.T) {
	got, err := ParseInventorySpec("kiwi_dewdrop:2,cactus")
	if err != nil {
		t.Fatalf("ParseInventorySpec returned error: %v", err)
	}
	want := []string{"kiwi_dewdrop", "kiwi_dewdrop", "cactus"}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d", len(got), len(want))
	}
	for idx := range got {
		if got[idx] != want[idx] {
			t.Fatalf("got[%d]=%q want %q", idx, got[idx], want[idx])
		}
	}
}

func TestSolveLayoutAcceptsGridCapacityAndRejectsLargerInventory(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"one": {
			ID:        "one",
			Shape:     []model.Coord{{Row: 0, Col: 0}},
			Rotations: []int{0},
		},
	}}
	items := make([]string, geometry.GridCells)
	for idx := range items {
		items[idx] = "one"
	}
	if _, err := SolveLayout(cat, items, 0, Config{AllowSkips: false, MaxNodes: 1}); err != nil {
		t.Fatalf("SolveLayout at grid capacity returned error: %v", err)
	}
	items = append(items, "one")
	if _, err := SolveLayout(cat, items, 0, Config{AllowSkips: false, MaxNodes: 1}); err == nil {
		t.Fatal("SolveLayout accepted inventory larger than the grid capacity")
	}
}

func TestRecipeFilteringAndScoreOnlyCandidatesMatchExhaustiveEvaluation(t *testing.T) {
	cat := exhaustiveScoringCatalog()
	itemIDs := []string{"source", "target", "anchor", "ingredient"}
	instances := ExpandInventory(itemIDs)
	gridMask := geometry.MaskFromCells([]model.Coord{
		{Row: 0, Col: 0},
		{Row: 0, Col: 1},
		{Row: 0, Col: 2},
		{Row: 0, Col: 3},
	})
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, gridMask)
		if err != nil {
			t.Fatalf("PlacementOptions(%s) returned error: %v", instance.InstanceID, err)
		}
		optionsByInstance[instance.InstanceID] = options
	}
	filteredCatalog := filterInventoryImpossibleRecipes(cat, itemIDs)
	if got := recipeResults(filteredCatalog.Recipes); !reflect.DeepEqual(got, []string{"crafted"}) {
		t.Fatalf("filtered recipe results=%v want [crafted]", got)
	}

	config := Config{
		TopN:       3,
		Priorities: []string{"coverage_group:0", "craft:crafted", "star_source:source"},
		CoverageGroups: []model.CoverageGroup{{
			Name:    "Target coverage",
			Sources: []string{"source"},
			Targets: []string{"target"},
		}},
	}
	var fullResults []model.Solution
	var scoreOnlyResults []model.Solution
	var layouts int
	var visit func(int, uint64, []model.Placement)
	visit = func(index int, occupied uint64, placements []model.Placement) {
		if index == len(instances) {
			layouts++
			fullEvaluation := scoring.EvaluateLayoutWithCoverageGroups(cat, placements, config.Priorities, config.CoverageGroups)
			filteredEvaluation := scoring.EvaluateLayoutWithCoverageGroups(filteredCatalog, placements, config.Priorities, config.CoverageGroups)
			if !reflect.DeepEqual(filteredEvaluation, fullEvaluation) {
				t.Fatalf("filtered evaluation differs for %s:\nfiltered=%+v\nfull=%+v", layoutKey(placements, instances), filteredEvaluation, fullEvaluation)
			}
			scoreOnly := scoring.EvaluateScoreOnlyWithCoverageGroups(cat, placements, config.Priorities, config.CoverageGroups)
			if !reflect.DeepEqual(scoreOnly, fullEvaluation.Score) {
				t.Fatalf("score-only differs for %s: got=%+v want=%+v", layoutKey(placements, instances), scoreOnly, fullEvaluation.Score)
			}
			fullResults = insertCandidate(fullResults, placements, instances, fullEvaluation, config.TopN)
			scoreOnlyResults = insertCandidateWithScoreOnlyFilter(cat, scoreOnlyResults, placements, instances, config)
			return
		}
		instance := instances[index]
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied != 0 {
				continue
			}
			next, _ := insertPlacementSorted(append([]model.Placement(nil), placements...), option)
			visit(index+1, occupied|option.Mask, next)
		}
		visit(index+1, occupied, placements)
	}
	visit(0, 0, nil)
	if layouts != 209 {
		t.Fatalf("exhaustive layout count=%d want 209", layouts)
	}
	if !reflect.DeepEqual(scoreOnlyResults, fullResults) {
		t.Fatalf("score-only candidates differ from full candidates:\nscore-only=%+v\nfull=%+v", scoreOnlyResults, fullResults)
	}
}

func TestCoveragePlacementPrioritiesMatchPairwiseReference(t *testing.T) {
	cat := coverageSeedGenericCatalog()
	instances := ExpandInventory([]string{"source_a", "source_b", "weapon_a", "weapon_b"})
	optionsByInstance := testOptionsByInstance(t, cat, instances)
	coverage := newCoverageContext(cat, instances, optionsByInstance, []string{"star_source:source_a", "star_source:source_b"})
	if coverage == nil {
		t.Fatal("expected coverage context")
	}
	for _, instance := range instances {
		for _, option := range optionsByInstance[instance.InstanceID] {
			got := coverage.priorityForPlacement(option)
			want := coverage.computePlacementPriority(cat, option, instances, optionsByInstance)
			if got != want {
				t.Fatalf("priority for %s at %s: got %d want %d", option.InstanceID, placementKey(option), got, want)
			}
		}
	}
}

func TestRefinementRespectsMoveLimitAndContext(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"one": {
			ID:        "one",
			Shape:     []model.Coord{{Row: 0, Col: 0}},
			Rotations: []int{0},
		},
	}}
	instances := ExpandInventory([]string{"one"})
	options, err := PlacementOptions(cat, instances[0], geometry.FullGridMask())
	if err != nil {
		t.Fatalf("PlacementOptions returned error: %v", err)
	}
	optionsByInstance := map[string][]model.Placement{instances[0].InstanceID: options}
	solution := buildSolution(cat, []model.Placement{options[0]}, instances, nil)

	_, _, stats, err := refineSolution(cat, instances, optionsByInstance, solution, Config{MaxRefineMoves: 1})
	if err != nil {
		t.Fatalf("refineSolution with move limit returned error: %v", err)
	}
	if stats.MovesChecked != 1 || !stats.MoveLimitReached {
		t.Fatalf("refine stats=%+v want one checked move and a reached limit", stats)
	}
	other := buildSolution(cat, []model.Placement{options[1]}, instances, nil)
	refined, searchStats, err := refineSolutions(cat, instances, optionsByInstance, []model.Solution{solution, other}, model.SearchStats{}, Config{MaxRefineMoves: 1})
	if err != nil {
		t.Fatalf("refineSolutions with move limit returned error: %v", err)
	}
	if len(refined) != 2 || refined[1].LayoutKey != other.LayoutKey {
		t.Fatalf("refineSolutions dropped unrefined candidates: %+v", refined)
	}
	if searchStats.RefineMovesChecked != 1 {
		t.Fatalf("refine solutions checked %d moves want 1", searchStats.RefineMovesChecked)
	}

	canceledContext := &cancelAfterChecksContext{Context: context.Background(), cancelAt: 4}
	_, _, stats, err = refineSolution(cat, instances, optionsByInstance, solution, Config{
		Context:        canceledContext,
		MaxRefineMoves: 10,
	})
	if err != context.Canceled {
		t.Fatalf("refineSolution cancellation error=%v want %v", err, context.Canceled)
	}
	if stats.MovesChecked != 1 {
		t.Fatalf("refine checked %d moves before cancellation, want 1", stats.MovesChecked)
	}
}

func TestDefaultRefineMoveLimitUsesSearchBudget(t *testing.T) {
	tests := []struct {
		maxNodes int64
		want     int64
	}{
		{maxNodes: 0, want: 25000},
		{maxNodes: 4000, want: 1000},
		{maxNodes: 20000, want: 5000},
		{maxNodes: 200000, want: 25000},
	}
	for _, tt := range tests {
		if got := defaultRefineMoveLimit(tt.maxNodes); got != tt.want {
			t.Fatalf("defaultRefineMoveLimit(%d)=%d want %d", tt.maxNodes, got, tt.want)
		}
	}
}

type cancelAfterChecksContext struct {
	context.Context
	checks   int
	cancelAt int
}

func (ctx *cancelAfterChecksContext) Err() error {
	ctx.checks++
	if ctx.checks >= ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func exhaustiveScoringCatalog() model.Catalog {
	return model.Catalog{
		Items: map[string]model.Item{
			"source": {
				ID:        "source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"target"}}},
				Rotations: []int{0},
			},
			"target": {
				ID:        "target",
				Types:     []string{"target"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"anchor": {
				ID:        "anchor",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"ingredient": {
				ID:        "ingredient",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
		Recipes: []model.Recipe{
			{Result: "missing-first", Anchor: "source", Ingredients: []string{"source", "missing"}},
			{Result: "crafted", Anchor: "anchor", Ingredients: []string{"anchor", "ingredient"}},
			{Result: "too-many", Anchor: "ingredient", Ingredients: []string{"ingredient", "ingredient"}},
		},
	}
}

func recipeResults(recipes []model.Recipe) []string {
	results := make([]string, 0, len(recipes))
	for _, recipe := range recipes {
		results = append(results, recipe.Result)
	}
	return results
}

func TestPlacementOptionsRespectUnavailableCells(t *testing.T) {
	cat := model.Catalog{
		Items: map[string]model.Item{
			"one": {
				ID:        "one",
				Name:      "One",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}
	instance := ExpandInventory([]string{"one"})[0]
	grid, err := geometry.ParseGridText("100000000/000000000/000000000/000000000/000000000/000000000")
	if err != nil {
		t.Fatalf("ParseGridText returned error: %v", err)
	}

	options, err := PlacementOptions(cat, instance, grid)
	if err != nil {
		t.Fatalf("PlacementOptions returned error: %v", err)
	}
	if len(options) != 1 {
		t.Fatalf("len(options)=%d want 1", len(options))
	}
	if options[0].Origin != (model.Coord{Row: 0, Col: 0}) {
		t.Fatalf("origin=%v want (0, 0)", options[0].Origin)
	}
}

func TestSolverReturnsDeterministicOrder(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"scalemail", "thornwall", "armor_kit"}
	first, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 20000, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout first returned error: %v", err)
	}
	second, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 20000, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout second returned error: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("solution counts differ: %d vs %d", len(first), len(second))
	}
	for idx := range first {
		if first[idx].LayoutKey != second[idx].LayoutKey {
			t.Fatalf("layout key mismatch at %d: %q vs %q", idx, first[idx].LayoutKey, second[idx].LayoutKey)
		}
	}
	if first[0].Evaluation.Score.CraftCount != 1 {
		t.Fatalf("craft count=%d want 1", first[0].Evaluation.Score.CraftCount)
	}
}

func TestProgressReporterDoesNotChangeBestResult(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"kiwi_dewdrop", "kiwi_dewdrop", "cactus"}
	config := Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 1}
	baseline, err := SolveLayout(cat, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("SolveLayout baseline returned error: %v", err)
	}

	var snapshots []ProgressSnapshot
	config.ProgressReporter = func(snapshot ProgressSnapshot) {
		snapshots = append(snapshots, snapshot)
	}
	withProgress, err := SolveLayout(cat, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("SolveLayout with progress returned error: %v", err)
	}

	if baseline[0].LayoutKey != withProgress[0].LayoutKey {
		t.Fatalf("layout key differs with progress: %q vs %q", baseline[0].LayoutKey, withProgress[0].LayoutKey)
	}
	if len(snapshots) == 0 {
		t.Fatal("expected progress snapshots")
	}
	last := snapshots[len(snapshots)-1]
	if last.Phase != ProgressPhaseDone {
		t.Fatalf("last phase=%q want %q", last.Phase, ProgressPhaseDone)
	}
	if last.NodesTotal != config.MaxNodes || last.Percent != 100 {
		t.Fatalf("final progress total=%d percent=%f, want total=%d percent=100", last.NodesTotal, last.Percent, config.MaxNodes)
	}
	var previousNodes int64
	for _, snapshot := range snapshots {
		if snapshot.NodesExplored < previousNodes {
			t.Fatalf("progress nodes went backwards: %d after %d", snapshot.NodesExplored, previousNodes)
		}
		previousNodes = snapshot.NodesExplored
	}
}

func TestExhaustiveProgressHasNoPercentTotal(t *testing.T) {
	cat := loadTestCatalog(t)
	var snapshots []ProgressSnapshot
	_, err := SolveLayout(cat, []string{"cactus"}, geometry.FullGridMask(), Config{
		TopN:             1,
		AllowSkips:       true,
		MaxNodes:         0,
		Workers:          1,
		ProgressReporter: func(snapshot ProgressSnapshot) { snapshots = append(snapshots, snapshot) },
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(snapshots) == 0 {
		t.Fatal("expected progress snapshots")
	}
	for _, snapshot := range snapshots {
		if snapshot.NodesTotal != 0 || snapshot.Percent != 0 {
			t.Fatalf("exhaustive snapshot has total/percent: %+v", snapshot)
		}
	}
}

func TestProgressReporterEmitsPartialSolutions(t *testing.T) {
	cat := loadTestCatalog(t)
	var partials []ProgressSnapshot
	solutions, err := SolveLayout(cat, []string{"kiwi_dewdrop", "kiwi_dewdrop"}, geometry.FullGridMask(), Config{
		TopN:       1,
		AllowSkips: true,
		MaxNodes:   0,
		Workers:    1,
		ProgressReporter: func(snapshot ProgressSnapshot) {
			if len(snapshot.PartialSolutions) > 0 {
				partials = append(partials, snapshot)
			}
		},
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected final solution")
	}
	if len(partials) == 0 {
		t.Fatal("expected at least one partial solution snapshot")
	}
	bestPartial := partials[len(partials)-1].PartialSolutions[0]
	if compareScores(bestPartial.Evaluation.Score, solutions[0].Evaluation.Score) > 0 {
		t.Fatalf("partial score unexpectedly better than final score: partial=%+v final=%+v", bestPartial.Evaluation.Score, solutions[0].Evaluation.Score)
	}
	if bestPartial.Search.NodesExplored == 0 {
		t.Fatalf("partial search stats were not stamped: %+v", bestPartial.Search)
	}
}

func TestSolverCanOptimizeStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	solutions, err := SolveLayout(cat, []string{"kiwi_dewdrop", "kiwi_dewdrop"}, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 0, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if solutions[0].Evaluation.Score.StarCount != 2 {
		t.Fatalf("star count=%d want 2", solutions[0].Evaluation.Score.StarCount)
	}
	if solutions[0].Evaluation.Score.ItemCount != 2 {
		t.Fatalf("item count=%d want 2", solutions[0].Evaluation.Score.ItemCount)
	}
}

func TestWorkersReturnSameBestResult(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"kiwi_dewdrop", "kiwi_dewdrop", "cactus"}
	oneWorker, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout one worker returned error: %v", err)
	}
	manyWorkers, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 4})
	if err != nil {
		t.Fatalf("SolveLayout many workers returned error: %v", err)
	}
	if oneWorker[0].LayoutKey != manyWorkers[0].LayoutKey {
		t.Fatalf("layout key differs: %q vs %q", oneWorker[0].LayoutKey, manyWorkers[0].LayoutKey)
	}
}

func TestLimitedSearchRefinesScalemailStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{
		"armor_kit",
		"kiwi_dewdrop",
		"leather_armor",
		"poison_potion",
		"scalemail",
		"spiked_shield",
	}
	solutions, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	best := solutions[0]
	if best.Evaluation.Score.StarCount < 6 {
		t.Fatalf("star count=%d want at least 6\nlayout=%s", best.Evaluation.Score.StarCount, best.LayoutKey)
	}
	if !best.Search.Limited {
		t.Fatalf("search limited=%v want true", best.Search.Limited)
	}
	if best.Search.NodesExplored == 0 {
		t.Fatal("expected nodes explored metadata")
	}
}

func TestRefinementDoesNotWorsenSolution(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{
		"armor_kit",
		"kiwi_dewdrop",
		"leather_armor",
		"poison_potion",
		"scalemail",
		"spiked_shield",
	}
	instances := ExpandInventory(items)
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}
	solutions, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	refined, _, _, err := refineSolution(cat, instances, optionsByInstance, solutions[0], Config{})
	if err != nil {
		t.Fatalf("refineSolution returned error: %v", err)
	}
	if SolutionLess(solutions[0], refined) {
		t.Fatalf("refinement worsened solution: before=%+v after=%+v", solutions[0].Evaluation.Score, refined.Evaluation.Score)
	}
}

func TestRefinementMovesItemOntoFreeStarCell(t *testing.T) {
	cat := loadTestCatalog(t)
	gridMask, err := geometry.ParseGridText("111111100/111111100/111111111/111111111/110000000/110000000")
	if err != nil {
		t.Fatalf("ParseGridText returned error: %v", err)
	}
	items := []string{
		"cactrio",
		"champion_s_ripper",
		"gloves_of_power",
		"ironwill_banner",
		"magic_essence",
		"poison_dagger",
		"power_stone",
		"prickly_potion",
		"stunbreaker_shield",
		"succulent",
		"tenacity_potion",
		"tusk",
		"twinmaw",
		"wooden_buckler",
	}
	instances := ExpandInventory(items)
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, gridMask)
		if err != nil {
			t.Fatalf("PlacementOptions(%s) returned error: %v", instance.InstanceID, err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	placements := []model.Placement{
		mustPlacementOption(t, optionsByInstance, "cactrio#0", model.Coord{Row: 1, Col: 4}, 0),
		mustPlacementOption(t, optionsByInstance, "champion_s_ripper#1", model.Coord{Row: 0, Col: 3}, 0),
		mustPlacementOption(t, optionsByInstance, "gloves_of_power#2", model.Coord{Row: 0, Col: 4}, 0),
		mustPlacementOption(t, optionsByInstance, "ironwill_banner#3", model.Coord{Row: 1, Col: 2}, 0),
		mustPlacementOption(t, optionsByInstance, "magic_essence#4", model.Coord{Row: 1, Col: 1}, 0),
		mustPlacementOption(t, optionsByInstance, "poison_dagger#5", model.Coord{Row: 4, Col: 0}, 0),
		mustPlacementOption(t, optionsByInstance, "power_stone#6", model.Coord{Row: 2, Col: 4}, 0),
		mustPlacementOption(t, optionsByInstance, "prickly_potion#7", model.Coord{Row: 4, Col: 1}, 0),
		mustPlacementOption(t, optionsByInstance, "stunbreaker_shield#8", model.Coord{Row: 2, Col: 7}, 0),
		mustPlacementOption(t, optionsByInstance, "succulent#9", model.Coord{Row: 0, Col: 1}, 0),
		mustPlacementOption(t, optionsByInstance, "tenacity_potion#10", model.Coord{Row: 3, Col: 4}, 90),
		mustPlacementOption(t, optionsByInstance, "tusk#11", model.Coord{Row: 0, Col: 0}, 0),
		mustPlacementOption(t, optionsByInstance, "twinmaw#12", model.Coord{Row: 0, Col: 6}, 0),
		mustPlacementOption(t, optionsByInstance, "wooden_buckler#13", model.Coord{Row: 2, Col: 0}, 0),
	}
	priorities := []string{
		"craft:spiked_shield",
		"coverage_group:0",
		"coverage_group:1",
		"star_source:ironwill_banner",
		"star_source:magic_essence",
	}
	coverageGroups := []model.CoverageGroup{
		{
			Name:    "Group 1",
			Sources: []string{"power_stone"},
			Targets: []string{"poison_dagger", "champion_s_ripper", "twinmaw", "power_stone"},
		},
		{
			Name:    "Group 2",
			Sources: []string{"gloves_of_power"},
			Targets: []string{"champion_s_ripper", "twinmaw", "gloves_of_power"},
		},
	}
	evaluation := scoring.EvaluateLayoutWithCoverageGroups(cat, placements, priorities, coverageGroups)
	if !reflect.DeepEqual(evaluation.Score.PriorityCounts, []int{1, 2, 2, 1, 3}) {
		t.Fatalf("initial priority counts=%v want [1 2 2 1 3]", evaluation.Score.PriorityCounts)
	}
	solution := model.Solution{
		Placements: placements,
		Evaluation: evaluation,
		LayoutKey:  layoutKey(placements, instances),
	}

	refined, changed, stats, err := refineSolution(cat, instances, optionsByInstance, solution, Config{
		Priorities:     priorities,
		CoverageGroups: coverageGroups,
	})
	if err != nil {
		t.Fatalf("refineSolution returned error: %v", err)
	}
	if !changed {
		t.Fatal("expected refinement to improve the layout")
	}
	if stats.MovesChecked == 0 || stats.Improvements == 0 {
		t.Fatalf("refine stats=%+v want checked moves and improvements", stats)
	}
	wantScore := model.Score{
		CraftCount:     1,
		StarCount:      9,
		ItemCount:      14,
		PriorityCounts: []int{1, 2, 2, 1, 4},
	}
	if !reflect.DeepEqual(refined.Evaluation.Score, wantScore) {
		t.Fatalf("refined score=%+v want %+v", refined.Evaluation.Score, wantScore)
	}
	succulent, ok := placementByInstance(refined.Placements, "succulent#9")
	if !ok {
		t.Fatal("succulent placement missing after refinement")
	}
	if succulent.Origin != (model.Coord{Row: 0, Col: 2}) {
		t.Fatalf("succulent origin=%v want (0, 2)", succulent.Origin)
	}
}

func mustPlacementOption(
	t testing.TB,
	optionsByInstance map[string][]model.Placement,
	instanceID string,
	origin model.Coord,
	rotation int,
) model.Placement {
	t.Helper()
	for _, option := range optionsByInstance[instanceID] {
		if option.Origin == origin && option.Rotation == rotation {
			return option
		}
	}
	t.Fatalf("placement option not found for %s at %v rotation=%d", instanceID, origin, rotation)
	return model.Placement{}
}

func placementByInstance(placements []model.Placement, instanceID string) (model.Placement, bool) {
	for _, placement := range placements {
		if placement.InstanceID == instanceID {
			return placement, true
		}
	}
	return model.Placement{}, false
}

func TestPriorityScoreBeatsLowerPriorityCounts(t *testing.T) {
	highPriority := model.Solution{
		Evaluation: model.Evaluation{
			Score: model.Score{
				CraftCount:     0,
				StarCount:      1,
				ItemCount:      1,
				PriorityCounts: []int{1, 0},
			},
		},
		LayoutKey: "z",
	}
	manyLowPriority := model.Solution{
		Evaluation: model.Evaluation{
			Score: model.Score{
				CraftCount:     99,
				StarCount:      99,
				ItemCount:      99,
				PriorityCounts: []int{0, 99},
			},
		},
		LayoutKey: "a",
	}

	if !SolutionLess(highPriority, manyLowPriority) {
		t.Fatalf("high priority solution should win: high=%+v low=%+v", highPriority.Evaluation.Score, manyLowPriority.Evaluation.Score)
	}
}

func TestSolutionLessFallsBackWithoutPriorities(t *testing.T) {
	craftSolution := model.Solution{
		Evaluation: model.Evaluation{
			Score: model.Score{CraftCount: 1, StarCount: 0, ItemCount: 1},
		},
		LayoutKey: "z",
	}
	starSolution := model.Solution{
		Evaluation: model.Evaluation{
			Score: model.Score{CraftCount: 0, StarCount: 99, ItemCount: 99},
		},
		LayoutKey: "a",
	}

	if !SolutionLess(craftSolution, starSolution) {
		t.Fatalf("legacy craft score should win when priorities are absent")
	}
}

func TestScoreOnlyCandidateCanEnterFiltersBeforeFullEvaluation(t *testing.T) {
	original := []model.InventoryInstance{{InstanceID: "one#0", ItemID: "one", OriginalIndex: 0}}
	latePlacement := model.Placement{InstanceID: "one#0", ItemID: "one", OriginalIndex: 0, Origin: model.Coord{Row: 1, Col: 0}}
	earlyPlacement := model.Placement{InstanceID: "one#0", ItemID: "one", OriginalIndex: 0, Origin: model.Coord{Row: 0, Col: 0}}
	worst := model.Solution{
		Evaluation: model.Evaluation{Score: model.Score{StarCount: 1, ItemCount: 1}},
		LayoutKey:  layoutKey([]model.Placement{latePlacement}, original),
	}
	results := []model.Solution{worst}

	if scoreOnlyCandidateCanEnter(results, []model.Placement{latePlacement}, original, model.Score{StarCount: 0, ItemCount: 1}, 1) {
		t.Fatal("worse score should be filtered before full evaluation")
	}
	if scoreOnlyCandidateCanEnter(results, []model.Placement{latePlacement}, original, model.Score{StarCount: 1, ItemCount: 1}, 1) {
		t.Fatal("equal score with non-better layout key should be filtered before full evaluation")
	}
	if !scoreOnlyCandidateCanEnter(results, []model.Placement{earlyPlacement}, original, model.Score{StarCount: 1, ItemCount: 1}, 1) {
		t.Fatal("equal score with better layout key should enter")
	}
	if !scoreOnlyCandidateCanEnter(results, []model.Placement{latePlacement}, original, model.Score{CraftCount: 1, StarCount: 1, ItemCount: 1}, 1) {
		t.Fatal("better score should enter")
	}
}

func TestLimitedWorkersReturnSameBestResult(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{
		"armor_kit",
		"kiwi_dewdrop",
		"leather_armor",
		"poison_potion",
		"scalemail",
		"spiked_shield",
	}
	oneWorker, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout one worker returned error: %v", err)
	}
	manyWorkers, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 1, AllowSkips: true, MaxNodes: 20000, Workers: 4})
	if err != nil {
		t.Fatalf("SolveLayout many workers returned error: %v", err)
	}
	if oneWorker[0].LayoutKey != manyWorkers[0].LayoutKey {
		t.Fatalf("layout key differs: %q vs %q", oneWorker[0].LayoutKey, manyWorkers[0].LayoutKey)
	}
}

func TestExhaustiveWorkersReturnSameBestResult(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"scalemail", "thornwall", "armor_kit"}
	oneWorker, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 0, Workers: 1})
	if err != nil {
		t.Fatalf("SolveLayout one worker returned error: %v", err)
	}
	manyWorkers, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 0, Workers: 16})
	if err != nil {
		t.Fatalf("SolveLayout many workers returned error: %v", err)
	}
	if oneWorker[0].LayoutKey != manyWorkers[0].LayoutKey {
		t.Fatalf("layout key differs: %q vs %q", oneWorker[0].LayoutKey, manyWorkers[0].LayoutKey)
	}
	if !reflect.DeepEqual(oneWorker[0].Evaluation.Score, manyWorkers[0].Evaluation.Score) {
		t.Fatalf("score differs: %+v vs %+v", oneWorker[0].Evaluation.Score, manyWorkers[0].Evaluation.Score)
	}
}

func TestSearchSplitBuildsUniqueParallelTasks(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{
		"apple",
		"banana",
		"bolstered_helmet",
		"excalibur",
		"fine_sword",
		"gloves_of_power",
		"knight_s_sigil",
		"piercing_lance",
		"pitahaya",
		"power_stone",
		"scalemail",
		"spice",
		"stunbreaker_shield",
	}
	ordered := ExpandInventory(items)
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range ordered {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}
	config := Config{AllowSkips: true, MaxNodes: 1_000_000, Workers: 16}
	depth := initialSplitDepth(ordered, optionsByInstance, config)
	if depth <= 0 {
		t.Fatalf("split depth=%d want > 0", depth)
	}
	tasks := buildTasks(ordered, optionsByInstance, config.AllowSkips, depth)
	if len(tasks) <= config.Workers {
		t.Fatalf("task count=%d want more than workers=%d", len(tasks), config.Workers)
	}
	seen := map[string]bool{}
	for _, task := range tasks {
		key := layoutKey(task.Placements, ordered) + "|" + strconv.Itoa(task.Index) + "|" + strconv.FormatUint(task.Occupied, 16)
		if seen[key] {
			t.Fatalf("duplicate task key %s", key)
		}
		seen[key] = true
	}
}

func TestNoSkipsStillPlacesAllItemsAfterPruning(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"rune_of_r_lyeh", "venomous_pincer", "shaman_s_talisman", "royal_seax", "ragnarok", "doombringer"}
	solutions, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{
		TopN:       1,
		AllowSkips: false,
		MaxNodes:   200000,
		Workers:    1,
		Priorities: []string{
			"star_source:rune_of_r_lyeh",
			"star_source:venomous_pincer",
			"star_source:shaman_s_talisman",
		},
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if solutions[0].Evaluation.Score.ItemCount != len(items) {
		t.Fatalf("item count=%d want %d", solutions[0].Evaluation.Score.ItemCount, len(items))
	}
	if len(solutions[0].Evaluation.Score.PriorityCounts) != 3 {
		t.Fatalf("priority counts=%v want 3 coverage buckets", solutions[0].Evaluation.Score.PriorityCounts)
	}
}

func TestCoverageContextDetectsSourcesTargetsAndCeiling(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"rune_of_r_lyeh", "venomous_pincer", "shaman_s_talisman", "royal_seax", "ragnarok", "doombringer"}
	instances := ExpandInventory(items)
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	coverage := newCoverageContext(cat, instances, optionsByInstance, []string{
		"star_source:rune_of_r_lyeh",
		"star_source:venomous_pincer",
		"star_source:shaman_s_talisman",
	})
	if coverage == nil {
		t.Fatal("coverage context is nil")
	}
	if len(coverage.sourceItemIDs) != 3 {
		t.Fatalf("source count=%d want 3", len(coverage.sourceItemIDs))
	}
	if coverage.targetCount() != 3 {
		t.Fatalf("target count=%d want 3", coverage.targetCount())
	}
	if len(coverage.ceilingCounts) != 3 || coverage.ceilingCounts[0] != 3 {
		t.Fatalf("ceiling counts=%v want first bucket 3", coverage.ceilingCounts)
	}
}

func TestCoverageOrderingPrioritizesSourcesAndTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"rune_of_r_lyeh", "royal_seax", "cactus"}
	instances := ExpandInventory(items)
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	coverage := newCoverageContext(cat, instances, optionsByInstance, []string{"star_source:rune_of_r_lyeh"})
	sourcePriority := instancePriority(cat, instances[0], []string{"star_source:rune_of_r_lyeh"}, coverage)
	targetPriority := instancePriority(cat, instances[1], []string{"star_source:rune_of_r_lyeh"}, coverage)
	neutralPriority := instancePriority(cat, instances[2], []string{"star_source:rune_of_r_lyeh"}, coverage)

	if sourcePriority <= neutralPriority {
		t.Fatalf("source priority=%d want greater than neutral=%d", sourcePriority, neutralPriority)
	}
	if targetPriority <= neutralPriority {
		t.Fatalf("target priority=%d want greater than neutral=%d", targetPriority, neutralPriority)
	}
}

func TestStopOnCoverageCeilingStopsAndReports(t *testing.T) {
	cat := coverageCeilingTestCatalog()

	solutions, err := SolveLayout(cat, []string{"left_source", "weapon", "right_source"}, geometry.FullGridMask(), Config{
		TopN:                  1,
		AllowSkips:            false,
		MaxNodes:              0,
		Workers:               1,
		Priorities:            []string{"star_source:left_source", "star_source:right_source"},
		StopOnCoverageCeiling: true,
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if !solutions[0].Search.CoverageCeilingReached {
		t.Fatalf("coverage ceiling reached=%v want true", solutions[0].Search.CoverageCeilingReached)
	}
	if !solutions[0].Search.StoppedAfterCoverageCeiling {
		t.Fatalf("stopped after coverage ceiling=%v want true", solutions[0].Search.StoppedAfterCoverageCeiling)
	}
	if got := solutions[0].Evaluation.Score.PriorityCounts; len(got) != 2 || got[0] != 1 || got[1] != 0 {
		t.Fatalf("priority counts=%v want [1 0]", got)
	}
}

func TestStopOnCoverageCeilingRequiresTopOne(t *testing.T) {
	cat := coverageCeilingTestCatalog()

	solutions, err := SolveLayout(cat, []string{"left_source", "weapon", "right_source"}, geometry.FullGridMask(), Config{
		TopN:                  2,
		AllowSkips:            false,
		MaxNodes:              0,
		Workers:               1,
		Priorities:            []string{"star_source:left_source", "star_source:right_source"},
		StopOnCoverageCeiling: true,
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if !solutions[0].Search.CoverageCeilingReached {
		t.Fatalf("coverage ceiling reached=%v want true", solutions[0].Search.CoverageCeilingReached)
	}
	if solutions[0].Search.StoppedAfterCoverageCeiling {
		t.Fatalf("stopped after coverage ceiling=%v want false when top > 1", solutions[0].Search.StoppedAfterCoverageCeiling)
	}
}

func TestCoverageSeedSearchFindsCompleteGenericCoverage(t *testing.T) {
	cat := coverageSeedGenericCatalog()
	items := []string{"source_a", "source_b", "weapon_a", "weapon_b"}

	solutions, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{
		TopN:                  1,
		AllowSkips:            false,
		MaxNodes:              200000,
		Workers:               1,
		Priorities:            []string{"star_source:source_a", "star_source:source_b"},
		StopOnCoverageCeiling: true,
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected at least one solution")
	}
	if solutions[0].Search.CoverageSeedNodes == 0 {
		t.Fatal("expected coverage seed nodes")
	}
	if !solutions[0].Search.StoppedAfterCoverageCeiling {
		t.Fatal("expected seed search to stop after coverage ceiling")
	}
	if solutions[0].Evaluation.Score.ItemCount != len(items) {
		t.Fatalf("item count=%d want %d", solutions[0].Evaluation.Score.ItemCount, len(items))
	}
	if got := solutions[0].Evaluation.Score.PriorityCounts; len(got) != 2 || got[0] != 2 || got[1] != 0 {
		t.Fatalf("priority counts=%v want [2 0]", got)
	}
}

func TestCoverageContextRespectsTargetItemFilter(t *testing.T) {
	cat := model.Catalog{
		Items: map[string]model.Item{
			"source": {
				ID:        "source",
				Name:      "Source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetItems: []string{"specific_weapon"}}},
				Rotations: []int{0},
			},
			"specific_weapon": {
				ID:        "specific_weapon",
				Name:      "Specific Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"wrong_weapon": {
				ID:        "wrong_weapon",
				Name:      "Wrong Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}
	instances := ExpandInventory([]string{"source", "specific_weapon", "wrong_weapon"})
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	coverage := newCoverageContext(cat, instances, optionsByInstance, []string{"star_source:source"})
	if coverage == nil {
		t.Fatal("coverage context is nil")
	}
	if coverage.targetCount() != 1 {
		t.Fatalf("target count=%d want 1", coverage.targetCount())
	}
	if coverage.targetIndexByOriginal[1] < 0 {
		t.Fatal("specific weapon should be a coverage target")
	}
	if coverage.targetIndexByOriginal[2] >= 0 {
		t.Fatal("wrong weapon should not be a coverage target")
	}
}

func TestCoverageContextRespectsExplicitGroupTargets(t *testing.T) {
	cat := model.Catalog{
		Items: map[string]model.Item{
			"source": {
				ID:        "source",
				Name:      "Source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"Weapon"}}},
				Rotations: []int{0},
			},
			"target_weapon": {
				ID:        "target_weapon",
				Name:      "Target Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"other_weapon": {
				ID:        "other_weapon",
				Name:      "Other Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}
	instances := ExpandInventory([]string{"source", "target_weapon", "other_weapon"})
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	coverage := newCoverageContextFromSources(cat, instances, optionsByInstance, []string{"source"}, []string{"target_weapon"}, 0)
	if coverage == nil {
		t.Fatal("coverage context is nil")
	}
	if coverage.targetIndexByOriginal[1] < 0 {
		t.Fatal("target_weapon should be a coverage target")
	}
	if coverage.targetIndexByOriginal[2] >= 0 {
		t.Fatal("other_weapon should not be a coverage target when explicit targets are set")
	}
	if coverage.targetCount() != 1 {
		t.Fatalf("target count=%d want 1", coverage.targetCount())
	}
}

func TestCoverageContextRespectsSourceItemExclusion(t *testing.T) {
	cat := loadTestCatalog(t)
	instances := ExpandInventory([]string{"apple", "apple", "banana"})
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions returned error: %v", err)
		}
		optionsByInstance[instance.InstanceID] = options
	}

	coverage := newCoverageContext(cat, instances, optionsByInstance, []string{"star_source:apple"})
	if coverage == nil {
		t.Fatal("coverage context is nil")
	}
	if coverage.targetIndexByOriginal[1] >= 0 {
		t.Fatal("second apple should not be a coverage target for apple stars")
	}
	if coverage.targetIndexByOriginal[2] < 0 {
		t.Fatal("banana should be a coverage target for apple stars")
	}
	if coverage.targetCount() != 1 {
		t.Fatalf("target count=%d want 1", coverage.targetCount())
	}
}

func TestCoverageSeedDedupesDuplicateSourceItemID(t *testing.T) {
	cat := model.Catalog{
		Items: map[string]model.Item{
			"source": {
				ID:        "source",
				Name:      "Source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"Weapon"}}},
				Rotations: []int{0},
			},
			"weapon": {
				ID:        "weapon",
				Name:      "Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}

	solutions, err := SolveLayout(cat, []string{"source", "source", "weapon"}, geometry.FullGridMask(), Config{
		TopN:       1,
		AllowSkips: false,
		MaxNodes:   200000,
		Workers:    1,
		Priorities: []string{"star_source:source"},
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if got := solutions[0].Evaluation.Score.PriorityCounts; len(got) != 1 || got[0] != 1 {
		t.Fatalf("priority counts=%v want [1]", got)
	}
}

func TestCoverageSeedImprovesRealWeaponCoverage(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"rune_of_r_lyeh", "venomous_pincer", "shaman_s_talisman", "royal_seax", "ragnarok", "doombringer"}

	solutions, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{
		TopN:                  1,
		AllowSkips:            false,
		MaxNodes:              1000000,
		Workers:               1,
		Priorities:            []string{"star_source:rune_of_r_lyeh", "star_source:venomous_pincer", "star_source:shaman_s_talisman"},
		StopOnCoverageCeiling: true,
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if got := solutions[0].Evaluation.Score.PriorityCounts; len(got) != 3 || got[0] != 3 || got[1] != 0 || got[2] != 0 {
		t.Fatalf("priority counts=%v want [3 0 0]", got)
	}
	if !solutions[0].Search.CoverageCeilingReached {
		t.Fatal("expected coverage ceiling reached")
	}
}

func TestRepairSearchImprovesCoverageIncumbent(t *testing.T) {
	cat := repairTestCatalog()
	items := []string{"source", "weapon", "blocker"}
	instances := ExpandInventory(items)
	optionsByInstance := testOptionsByInstance(t, cat, instances)
	priorities := []string{"star_source:source"}
	coverage := newCoverageContext(cat, instances, optionsByInstance, priorities)
	sortPlacementOptionsForCoverage(optionsByInstance, coverage)

	initialPlacements := []model.Placement{
		testPlacement(t, optionsByInstance[instances[0].InstanceID], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, optionsByInstance[instances[1].InstanceID], model.Coord{Row: 2, Col: 2}, 0),
		testPlacement(t, optionsByInstance[instances[2].InstanceID], model.Coord{Row: 0, Col: 1}, 0),
	}
	initial := buildSolution(cat, initialPlacements, instances, priorities)
	if got := initial.Evaluation.Score.PriorityCounts; len(got) != 1 || got[0] != 0 {
		t.Fatalf("initial priority counts=%v want [0]", got)
	}

	result := repairSearch(cat, instances, optionsByInstance, Config{
		TopN:         1,
		AllowSkips:   false,
		MaxNodes:     50000,
		Workers:      1,
		Priorities:   priorities,
		RepairSearch: true,
	}, coverage, geometry.FullGridMask(), 50000, []model.Solution{initial}, nil)

	if result.NodesExplored == 0 || result.Iterations == 0 || result.CandidateCount == 0 {
		t.Fatalf("repair did not run: %+v", result)
	}
	if result.Improvements == 0 {
		t.Fatalf("repair improvements=%d want > 0", result.Improvements)
	}
	if len(result.Solutions) == 0 {
		t.Fatal("repair returned no solutions")
	}
	if !SolutionLess(result.Solutions[0], initial) {
		t.Fatalf("repair best did not improve initial: initial=%v best=%v", initial.Evaluation.Score, result.Solutions[0].Evaluation.Score)
	}
	if got := result.Solutions[0].Evaluation.Score.PriorityCounts; len(got) != 1 || got[0] != 1 {
		t.Fatalf("repair priority counts=%v want [1]", got)
	}
}

func TestParallelRepairSearchDoesNotWorsenIncumbent(t *testing.T) {
	cat := repairTestCatalog()
	items := []string{"source", "weapon", "blocker"}
	instances := ExpandInventory(items)
	optionsByInstance := testOptionsByInstance(t, cat, instances)
	priorities := []string{"star_source:source"}
	coverage := newCoverageContext(cat, instances, optionsByInstance, priorities)
	sortPlacementOptionsForCoverage(optionsByInstance, coverage)

	initialPlacements := []model.Placement{
		testPlacement(t, optionsByInstance[instances[0].InstanceID], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, optionsByInstance[instances[1].InstanceID], model.Coord{Row: 2, Col: 2}, 0),
		testPlacement(t, optionsByInstance[instances[2].InstanceID], model.Coord{Row: 0, Col: 1}, 0),
	}
	initial := buildSolution(cat, initialPlacements, instances, priorities)

	serial := repairSearch(cat, instances, optionsByInstance, Config{
		TopN:         1,
		AllowSkips:   false,
		MaxNodes:     50000,
		Workers:      1,
		Priorities:   priorities,
		RepairSearch: true,
	}, coverage, geometry.FullGridMask(), 50000, []model.Solution{initial}, nil)
	parallel := repairSearch(cat, instances, optionsByInstance, Config{
		TopN:         1,
		AllowSkips:   false,
		MaxNodes:     2_000_000,
		Workers:      4,
		Priorities:   priorities,
		RepairSearch: true,
	}, coverage, geometry.FullGridMask(), 2_000_000, []model.Solution{initial}, nil)

	if len(parallel.Solutions) == 0 {
		t.Fatal("parallel repair returned no solutions")
	}
	if SolutionLess(initial, parallel.Solutions[0]) {
		t.Fatalf("parallel repair worsened incumbent: initial=%v best=%v", initial.Evaluation.Score, parallel.Solutions[0].Evaluation.Score)
	}
	if len(serial.Solutions) > 0 && SolutionLess(serial.Solutions[0], parallel.Solutions[0]) {
		t.Fatalf("parallel repair worse than serial: serial=%v parallel=%v", serial.Solutions[0].Evaluation.Score, parallel.Solutions[0].Evaluation.Score)
	}
	if parallel.ParallelTasks <= 1 || parallel.ParallelWorkersUsed <= 1 {
		t.Fatalf("parallel repair stats=%+v want multiple tasks/workers", parallel)
	}
}

func TestRepairSearchDoesNotRunForExhaustiveSolve(t *testing.T) {
	cat := repairTestCatalog()
	solutions, err := SolveLayout(cat, []string{"source", "weapon", "blocker"}, geometry.FullGridMask(), Config{
		TopN:         1,
		AllowSkips:   false,
		MaxNodes:     0,
		Workers:      1,
		Priorities:   []string{"star_source:source"},
		RepairSearch: true,
	})
	if err != nil {
		t.Fatalf("SolveLayout returned error: %v", err)
	}
	if len(solutions) == 0 {
		t.Fatal("expected solution")
	}
	if solutions[0].Search.RepairNodes != 0 || solutions[0].Search.RepairIterations != 0 {
		t.Fatalf("repair stats=%+v want zero for exhaustive solve", solutions[0].Search)
	}
}

func TestRepairSearchEnabledDoesNotWorsenLimitedSolve(t *testing.T) {
	cat := loadTestCatalog(t)
	items := []string{"rune_of_r_lyeh", "venomous_pincer", "shaman_s_talisman", "royal_seax", "ragnarok", "doombringer"}
	config := Config{
		TopN:       3,
		AllowSkips: false,
		MaxNodes:   200000,
		Workers:    1,
		Priorities: []string{
			"star_source:rune_of_r_lyeh",
			"star_source:venomous_pincer",
			"star_source:shaman_s_talisman",
		},
	}

	withoutRepair, err := SolveLayout(cat, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("SolveLayout without repair returned error: %v", err)
	}
	defaultConfig, err := SolveLayout(cat, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("SolveLayout default returned error: %v", err)
	}
	if withoutRepair[0].LayoutKey != defaultConfig[0].LayoutKey {
		t.Fatalf("default repair setting changed direct solver result: %q vs %q", withoutRepair[0].LayoutKey, defaultConfig[0].LayoutKey)
	}

	config.RepairSearch = true
	withRepair, err := SolveLayout(cat, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("SolveLayout with repair returned error: %v", err)
	}
	if SolutionLess(withoutRepair[0], withRepair[0]) {
		t.Fatalf("repair worsened best solution: without=%v with=%v", withoutRepair[0].Evaluation.Score, withRepair[0].Evaluation.Score)
	}
}

func coverageCeilingTestCatalog() model.Catalog {
	return model.Catalog{
		Items: map[string]model.Item{
			"left_source": {
				ID:        "left_source",
				Name:      "Left Source",
				Types:     []string{"Source"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"Weapon"}}},
				Rotations: []int{0},
			},
			"right_source": {
				ID:        "right_source",
				Name:      "Right Source",
				Types:     []string{"Source"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: -1}, TargetTypes: []string{"Weapon"}}},
				Rotations: []int{0},
			},
			"weapon": {
				ID:        "weapon",
				Name:      "Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}
}

func repairTestCatalog() model.Catalog {
	return model.Catalog{
		Items: map[string]model.Item{
			"source": {
				ID:        "source",
				Name:      "Source",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"Weapon"}}},
				Rotations: []int{0},
			},
			"weapon": {
				ID:        "weapon",
				Name:      "Weapon",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"blocker": {
				ID:        "blocker",
				Name:      "Blocker",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}
}

func testOptionsByInstance(t *testing.T, cat model.Catalog, instances []model.InventoryInstance) map[string][]model.Placement {
	t.Helper()
	optionsByInstance := map[string][]model.Placement{}
	for _, instance := range instances {
		options, err := PlacementOptions(cat, instance, geometry.FullGridMask())
		if err != nil {
			t.Fatalf("PlacementOptions(%s): %v", instance.InstanceID, err)
		}
		optionsByInstance[instance.InstanceID] = options
	}
	return optionsByInstance
}

func testPlacement(t *testing.T, options []model.Placement, origin model.Coord, rotation int) model.Placement {
	t.Helper()
	for _, option := range options {
		if option.Origin == origin && option.Rotation == rotation {
			return option
		}
	}
	t.Fatalf("placement at %v rotation=%d not found", origin, rotation)
	return model.Placement{}
}

func coverageSeedGenericCatalog() model.Catalog {
	sourceAStars := []model.Star{
		{Offset: model.Coord{Row: -1, Col: -1}, TargetTypes: []string{"Weapon"}},
		{Offset: model.Coord{Row: -1, Col: 0}, TargetTypes: []string{"Weapon"}},
		{Offset: model.Coord{Row: -1, Col: 1}, TargetTypes: []string{"Weapon"}},
	}
	sourceBStars := []model.Star{
		{Offset: model.Coord{Row: -1, Col: -3}, TargetTypes: []string{"Weapon"}},
		{Offset: model.Coord{Row: -1, Col: -2}, TargetTypes: []string{"Weapon"}},
		{Offset: model.Coord{Row: -1, Col: -1}, TargetTypes: []string{"Weapon"}},
	}
	return model.Catalog{
		Items: map[string]model.Item{
			"source_a": {
				ID:        "source_a",
				Name:      "Source A",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     sourceAStars,
				Rotations: []int{0},
			},
			"source_b": {
				ID:        "source_b",
				Name:      "Source B",
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Stars:     sourceBStars,
				Rotations: []int{0},
			},
			"weapon_a": {
				ID:        "weapon_a",
				Name:      "Weapon A",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
			"weapon_b": {
				ID:        "weapon_b",
				Name:      "Weapon B",
				Types:     []string{"Weapon"},
				Shape:     []model.Coord{{Row: 0, Col: 0}},
				Rotations: []int{0},
			},
		},
	}
}

func BenchmarkSmallSolve(b *testing.B) {
	cat := loadTestCatalog(b)
	items := []string{"scalemail", "thornwall", "armor_kit"}
	for i := 0; i < b.N; i++ {
		if _, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 0, Workers: 1}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSmallSolveWithoutExactBounds(b *testing.B) {
	cat := loadTestCatalog(b)
	items := []string{"scalemail", "thornwall", "armor_kit"}
	for i := 0; i < b.N; i++ {
		if _, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 0, Workers: 1, DisableExactBounds: true}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMediumSolve(b *testing.B) {
	cat := loadTestCatalog(b)
	items := []string{"kiwi_dewdrop", "kiwi_dewdrop", "cactus", "cactus", "spinegrowth_breastplate"}
	for i := 0; i < b.N; i++ {
		if _, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{TopN: 3, AllowSkips: true, MaxNodes: 200000, Workers: 1}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMediumSolveWithProgress(b *testing.B) {
	cat := loadTestCatalog(b)
	items := []string{"kiwi_dewdrop", "kiwi_dewdrop", "cactus", "cactus", "spinegrowth_breastplate"}
	for i := 0; i < b.N; i++ {
		if _, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{
			TopN:             3,
			AllowSkips:       true,
			MaxNodes:         200000,
			Workers:          1,
			ProgressReporter: func(ProgressSnapshot) {},
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCoverageWeaponSolve(b *testing.B) {
	cat := loadTestCatalog(b)
	items := []string{
		"rune_of_r_lyeh",
		"venomous_pincer",
		"shaman_s_talisman",
		"royal_seax",
		"ragnarok",
		"doombringer",
	}
	priorities := []string{
		"star_source:rune_of_r_lyeh",
		"star_source:venomous_pincer",
		"star_source:shaman_s_talisman",
	}
	for i := 0; i < b.N; i++ {
		if _, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{
			TopN:       3,
			AllowSkips: false,
			MaxNodes:   200000,
			Workers:    1,
			Priorities: priorities,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCoverageRepairSolve(b *testing.B) {
	cat := loadTestCatalog(b)
	items := []string{
		"rune_of_r_lyeh",
		"venomous_pincer",
		"shaman_s_talisman",
		"royal_seax",
		"ragnarok",
		"doombringer",
	}
	priorities := []string{
		"star_source:rune_of_r_lyeh",
		"star_source:venomous_pincer",
		"star_source:shaman_s_talisman",
	}
	for i := 0; i < b.N; i++ {
		if _, err := SolveLayout(cat, items, geometry.FullGridMask(), Config{
			TopN:         3,
			AllowSkips:   false,
			MaxNodes:     200000,
			Workers:      1,
			Priorities:   priorities,
			RepairSearch: true,
		}); err != nil {
			b.Fatal(err)
		}
	}
}
