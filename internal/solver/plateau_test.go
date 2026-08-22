package solver

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"backpack-brawl-solver/internal/model"
)

func TestPlateauArchiveIsIndependentFromDiagnostics(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "110000000/000000000/000000000/000000000/000000000/000000000")
	config := Config{
		TopN:              1,
		AllowSkips:        false,
		MaxNodes:          5_000,
		Workers:           1,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source"},
	}
	normal, err := SolveLayout(cat, []string{"source", "food"}, grid, config)
	if err != nil {
		t.Fatalf("normal solve: %v", err)
	}
	config.Diagnostics = true
	diagnostic, err := SolveLayout(cat, []string{"source", "food"}, grid, config)
	if err != nil {
		t.Fatalf("diagnostic solve: %v", err)
	}
	if len(normal) != 1 || len(diagnostic) != 1 {
		t.Fatalf("solution counts normal=%d diagnostic=%d", len(normal), len(diagnostic))
	}
	if normal[0].LayoutKey != diagnostic[0].LayoutKey || !reflect.DeepEqual(normal[0].Evaluation.Score, diagnostic[0].Evaluation.Score) {
		t.Fatalf("diagnostics changed result: normal=%+v diagnostic=%+v", normal[0], diagnostic[0])
	}
	if normal[0].Search.NodesExplored != diagnostic[0].Search.NodesExplored {
		t.Fatalf("diagnostics changed charged budget: normal=%d diagnostic=%d", normal[0].Search.NodesExplored, diagnostic[0].Search.NodesExplored)
	}
	if normal[0].Search.PlateauArchive.Size == 0 || len(normal[0].Search.PlateauArchive.Samples) != 0 {
		t.Fatalf("normal archive=%+v want algorithmic archive without samples", normal[0].Search.PlateauArchive)
	}
	if len(diagnostic[0].Search.PlateauArchive.Samples) == 0 {
		t.Fatalf("diagnostic archive=%+v want samples", diagnostic[0].Search.PlateauArchive)
	}
	if diagnostic[0].Search.ConfigFingerprint == "" {
		t.Fatal("missing configuration fingerprint")
	}
}

func TestDiagnosticReferenceNeverChangesSearchResult(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "110000000/000000000/000000000/000000000/000000000/000000000")
	baseConfig := Config{TopN: 1, AllowSkips: false, MaxNodes: 5_000, Workers: 1, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:source"}}
	baseline, err := SolveLayout(cat, []string{"source", "food"}, grid, baseConfig)
	if err != nil {
		t.Fatalf("baseline solve: %v", err)
	}
	withReference := baseConfig
	withReference.Diagnostics = true
	withReference.DiagnosticReference = append([]model.Placement(nil), baseline[0].Placements...)
	diagnostic, err := SolveLayout(cat, []string{"source", "food"}, grid, withReference)
	if err != nil {
		t.Fatalf("diagnostic solve: %v", err)
	}
	if diagnostic[0].LayoutKey != baseline[0].LayoutKey || !reflect.DeepEqual(diagnostic[0].Evaluation.Score, baseline[0].Evaluation.Score) {
		t.Fatalf("reference changed search result: baseline=%+v diagnostic=%+v", baseline[0], diagnostic[0])
	}
	if diagnostic[0].Search.NodesExplored != baseline[0].Search.NodesExplored {
		t.Fatalf("reference changed node count: baseline=%d diagnostic=%d", baseline[0].Search.NodesExplored, diagnostic[0].Search.NodesExplored)
	}
	samples := diagnostic[0].Search.PlateauArchive.Samples
	if len(samples) == 0 || samples[0].DeltaToReference == "" {
		t.Fatalf("reference diagnostics missing delta: %+v", diagnostic[0].Search.PlateauArchive)
	}
	if diagnostic[0].Search.PlateauArchive.ReferenceEvaluations == 0 || diagnostic[0].Search.PlateauArchive.MinimumReferenceDistance == nil {
		t.Fatalf("reference minimum missing: %+v", diagnostic[0].Search.PlateauArchive)
	}
}

func TestReferenceDeltaSelfIsZero(t *testing.T) {
	reference := model.Solution{
		Placements: []model.Placement{{InstanceID: "source#0", ItemID: "source", Origin: model.Coord{Row: 1, Col: 2}, Rotation: 90}},
		Evaluation: model.Evaluation{Stars: []model.StarActivation{{SourceInstance: "source#0", TargetInstance: "source#0", StarPosition: model.Coord{Row: 1, Col: 3}}}},
	}
	sample := plateauSample(reference)
	delta := applyReferenceDelta(&sample, reference)
	if delta != (model.ReferenceDelta{}) {
		t.Fatalf("self delta=%+v want zero", delta)
	}
	if sample.ReferenceDelta == nil || *sample.ReferenceDelta != delta {
		t.Fatalf("sample reference delta=%+v want %+v", sample.ReferenceDelta, delta)
	}
}

func TestReferenceDeltaCountsMovesAndRotationOnlyAtSharedOrigins(t *testing.T) {
	reference := model.Solution{Placements: []model.Placement{{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{Col: 1}, Rotation: 0}}}
	moved := plateauSample(model.Solution{Placements: []model.Placement{{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{Col: 2}, Rotation: 90}}})
	movedDelta := calculateReferenceDelta(moved, reference)
	if movedDelta.MovedItems != 1 || movedDelta.RotationChanges != 0 || movedDelta.StructuralDistance != 1 {
		t.Fatalf("move delta=%+v want one move and no rotation change", movedDelta)
	}

	rotated := plateauSample(model.Solution{Placements: []model.Placement{{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{Col: 1}, Rotation: 90}}})
	rotatedDelta := calculateReferenceDelta(rotated, reference)
	if rotatedDelta.MovedItems != 0 || rotatedDelta.RotationChanges != 1 || rotatedDelta.StructuralDistance != 1 {
		t.Fatalf("rotation delta=%+v want one rotation and no move", rotatedDelta)
	}
}

func TestReferenceDeltaSeparatesLiteralAndCanonicalCopyLinks(t *testing.T) {
	reference := model.Solution{
		Placements: []model.Placement{
			{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0, Origin: model.Coord{Col: 0}, Cells: []model.Coord{{Col: 0}}},
			{InstanceID: "source#1", ItemID: "source", OriginalIndex: 1, Origin: model.Coord{Col: 2}, Cells: []model.Coord{{Col: 2}}},
			{InstanceID: "food#2", ItemID: "food", OriginalIndex: 2, Origin: model.Coord{Col: 1}, Cells: []model.Coord{{Col: 1}}},
			{InstanceID: "food#3", ItemID: "food", OriginalIndex: 3, Origin: model.Coord{Col: 3}, Cells: []model.Coord{{Col: 3}}},
		},
		Evaluation: model.Evaluation{Stars: []model.StarActivation{
			{SourceInstance: "source#0", TargetInstance: "food#2"},
			{SourceInstance: "source#1", TargetInstance: "food#3"},
		}},
	}
	candidate := model.Solution{
		Placements: []model.Placement{
			{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0, Origin: model.Coord{Col: 2}, Cells: []model.Coord{{Col: 2}}},
			{InstanceID: "source#1", ItemID: "source", OriginalIndex: 1, Origin: model.Coord{Col: 0}, Cells: []model.Coord{{Col: 0}}},
			{InstanceID: "food#2", ItemID: "food", OriginalIndex: 2, Origin: model.Coord{Col: 1}, Cells: []model.Coord{{Col: 1}}},
			{InstanceID: "food#3", ItemID: "food", OriginalIndex: 3, Origin: model.Coord{Col: 3}, Cells: []model.Coord{{Col: 3}}},
		},
		Evaluation: model.Evaluation{Stars: []model.StarActivation{
			{SourceInstance: "source#1", TargetInstance: "food#2"},
			{SourceInstance: "source#0", TargetInstance: "food#3"},
		}},
	}
	delta := calculateReferenceDelta(plateauSample(candidate), reference)
	if delta.MovedItems != 0 || delta.ExactLiteralLinksLost != 2 || delta.ExactLiteralLinksGained != 2 || delta.CanonicalLinksLost != 0 || delta.CanonicalLinksGained != 0 || delta.StructuralDistance != 0 {
		t.Fatalf("copy-normalized delta=%+v", delta)
	}
}

func TestReferenceMinimumSurvivesDiagnosticSampleTruncation(t *testing.T) {
	reference := model.Solution{Placements: []model.Placement{{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{}}}}
	trace := newDiagnosticTrace(time.Now(), 0, nil, "", nil, nil, &reference)
	for index := 0; index < plateauDiagnosticSampleLimit; index++ {
		trace.observePrioritySample(model.PlateauSample{
			Score:               model.Score{StarCount: 1},
			Placements:          []model.Placement{{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{Col: index + 1}}},
			LayoutKey:           fmt.Sprintf("far-%03d", index),
			CanonicalLayoutHash: fmt.Sprintf("far-%03d", index),
		})
	}
	trace.observePrioritySample(model.PlateauSample{
		Placements:          append([]model.Placement(nil), reference.Placements...),
		LayoutKey:           "closest",
		CanonicalLayoutHash: "closest",
	})

	var stats model.SearchStats
	trace.apply(&stats)
	archive := stats.PlateauArchive
	if len(archive.Samples) != plateauDiagnosticSampleLimit {
		t.Fatalf("retained samples=%d want %d", len(archive.Samples), plateauDiagnosticSampleLimit)
	}
	for _, sample := range archive.Samples {
		if sample.LayoutKey == "closest" {
			t.Fatal("closest sample unexpectedly survived score-based display retention")
		}
	}
	if archive.ReferenceEvaluations != plateauDiagnosticSampleLimit+1 || archive.MinimumReferenceDistance == nil {
		t.Fatalf("reference diagnostics=%+v", archive)
	}
	if archive.MinimumReferenceDistance.LayoutKey != "closest" || archive.MinimumReferenceDistance.Delta.StructuralDistance != 0 {
		t.Fatalf("minimum reference distance=%+v want closest zero", archive.MinimumReferenceDistance)
	}
}

func TestCanonicalLinkSignatureIgnoresEquivalentCopyLabels(t *testing.T) {
	left := []model.Placement{
		{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0, Cells: []model.Coord{{Col: 0}}},
		{InstanceID: "source#1", ItemID: "source", OriginalIndex: 1, Cells: []model.Coord{{Col: 2}}},
		{InstanceID: "food#2", ItemID: "food", OriginalIndex: 2, Cells: []model.Coord{{Col: 1}}},
		{InstanceID: "food#3", ItemID: "food", OriginalIndex: 3, Cells: []model.Coord{{Col: 3}}},
	}
	right := []model.Placement{
		{InstanceID: "source#0", ItemID: "source", OriginalIndex: 0, Cells: []model.Coord{{Col: 2}}},
		{InstanceID: "source#1", ItemID: "source", OriginalIndex: 1, Cells: []model.Coord{{Col: 0}}},
		{InstanceID: "food#2", ItemID: "food", OriginalIndex: 2, Cells: []model.Coord{{Col: 3}}},
		{InstanceID: "food#3", ItemID: "food", OriginalIndex: 3, Cells: []model.Coord{{Col: 1}}},
	}
	leftStars := []model.StarActivation{{SourceInstance: "source#0", TargetInstance: "food#2"}, {SourceInstance: "source#1", TargetInstance: "food#3"}}
	rightStars := []model.StarActivation{{SourceInstance: "source#1", TargetInstance: "food#3"}, {SourceInstance: "source#0", TargetInstance: "food#2"}}
	if got, want := canonicalLinkSignature(left, leftStars), canonicalLinkSignature(right, rightStars); got != want {
		t.Fatalf("canonical link signatures differ: %q vs %q", got, want)
	}
}

func TestStarUpperBoundDoesNotShareTargetCapacityAcrossSources(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "source", "food"})
	options := testOptionsByInstance(t, cat, instances)
	bound := newStarUpperBoundContext(cat, instances, options)
	if bound == nil {
		t.Fatal("missing star upper bound")
	}
	if got := bound.forPlacements(nil, nil).GeometricRelaxed; got != 2 {
		t.Fatalf("geometric relaxed bound=%d want 2; target capacity leaked across sources", got)
	}
}

func TestPlateauRefineCrossesTieBreakStarValley(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"priority": {ID: "priority", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"blocker":  {ID: "blocker", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Row: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"alternate": {ID: "alternate", Shape: []model.Coord{{}}, Stars: []model.Star{
			{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}},
			{Offset: model.Coord{Col: 2}, TargetTypes: []string{"Food"}},
		}, Rotations: []int{0}},
		"food": {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	items := []string{"priority", "food", "blocker", "food", "food", "food", "alternate"}
	instances := ExpandInventory(items)
	grid := mustParseGridForTest(t, "111110000/111110000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, cat, instances, grid)
	placements := []model.Placement{
		testPlacement(t, options["priority#0"], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, options["food#1"], model.Coord{Row: 0, Col: 1}, 0),
		testPlacement(t, options["blocker#2"], model.Coord{Row: 0, Col: 2}, 0),
		testPlacement(t, options["food#3"], model.Coord{Row: 1, Col: 2}, 0),
		testPlacement(t, options["food#4"], model.Coord{Row: 0, Col: 3}, 0),
		testPlacement(t, options["food#5"], model.Coord{Row: 0, Col: 4}, 0),
		testPlacement(t, options["alternate#6"], model.Coord{Row: 1, Col: 3}, 0),
	}
	sortPlacementsByOriginal(placements)
	config := Config{PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:priority"}}
	incumbent := model.Solution{
		Placements:          placements,
		Evaluation:          evaluateLayoutForConfig(cat, placements, config),
		LayoutKey:           layoutKey(placements, instances),
		CanonicalLayoutHash: canonicalLayoutHash(placements),
	}
	if incumbent.Evaluation.Score.StarCount != 2 || !reflect.DeepEqual(incumbent.Evaluation.Score.PriorityCounts, []int{1}) {
		t.Fatalf("unexpected incumbent score=%+v", incumbent.Evaluation.Score)
	}
	config.priorityBounds = newPriorityBoundContext(cat, instances, config.Priorities, config.PrioritySemantics)
	config.plateauArchive = newPlateauArchive(config.priorityBounds)
	config.plateauArchive.observe(incumbent, "test")
	best, stats, err := refinePlateau(cat, instances, options, incumbent, config, newStarUpperBoundContext(cat, instances, options), 20_000)
	if err != nil {
		t.Fatalf("refine plateau: %v", err)
	}
	if best.Evaluation.Score.StarCount <= incumbent.Evaluation.Score.StarCount || !reflect.DeepEqual(best.Evaluation.Score.PriorityCounts, []int{1}) {
		t.Fatalf("plateau refine score=%+v want priority preserved and more stars", best.Evaluation.Score)
	}
	if stats.MaxValley < 1 || !stats.Improved {
		t.Fatalf("plateau refine did not report valley/improvement: %+v", stats)
	}
}

func TestPlateauIndirectBlockerIsIncludedAtDepthTwo(t *testing.T) {
	placements := []model.Placement{
		{InstanceID: "direct", Mask: 1 << 0},
		{InstanceID: "indirect", Mask: 1 << 1},
	}
	options := map[string][]model.Placement{
		"direct": []model.Placement{{InstanceID: "direct", Mask: 1 << 1}},
	}
	got := plateauIndirectBlockers(placements, options, []string{"direct"}, []string{"source"})
	if !reflect.DeepEqual(got, []string{"indirect"}) {
		t.Fatalf("indirect blockers=%v want [indirect]", got)
	}
}

func TestPlateauClosureStatsSeparateAttemptsFromEnqueuedWork(t *testing.T) {
	archive := newPlateauArchive(&priorityBoundContext{})
	archive.recordClosure(8, 9, 3, true)
	archive.recordClosure(8, 5, 7, false)
	archive.recordUniqueClosure(8)
	archive.recordPriorityFeasibleClosures(8, 1)
	archive.recordEnqueuedClosure(8)
	stats := archive.stats()
	if len(stats.ClosureStats) != 1 {
		t.Fatalf("closure stats=%+v", stats.ClosureStats)
	}
	closure := stats.ClosureStats[0]
	if closure.Attempts != 2 || closure.UniqueClosures != 1 || closure.PriorityFeasible != 1 || closure.Enqueued != 1 || closure.ClosureTooLarge != 1 {
		t.Fatalf("closure counters=%+v", closure)
	}
	if closure.MandatorySizeMin != 5 || closure.MandatorySizeMax != 9 || closure.OptionalSizeMin != 3 || closure.OptionalSizeMax != 7 {
		t.Fatalf("closure ranges=%+v", closure)
	}
	if !reflect.DeepEqual(closure.MandatorySizeHistogram, []model.ClosureSizeBucket{{Size: 5, Count: 1}, {Size: 9, Count: 1}}) {
		t.Fatalf("mandatory histogram=%+v", closure.MandatorySizeHistogram)
	}
}

func TestFiveMillionPlateauLevelsPreserveLegacyPolicy(t *testing.T) {
	policy := resolveSearchPolicy(Config{}, 5_000_000)
	want := []PlateauLevelPolicy{
		{MaxNeighborhoodSize: 8, QuotaBps: 2500},
		{MaxNeighborhoodSize: 9, QuotaBps: 2500},
		{MaxNeighborhoodSize: 10, QuotaBps: 2500},
		{MaxNeighborhoodSize: 12, QuotaBps: 2500},
	}
	if !reflect.DeepEqual(policy.PlateauLevels, want) {
		t.Fatalf("5M plateau levels=%+v want %+v", policy.PlateauLevels, want)
	}
	changed := policy
	changed.PlateauLevels = append([]PlateauLevelPolicy(nil), policy.PlateauLevels...)
	changed.PlateauLevels[0].QuotaBps--
	if resolvedPolicyFingerprint(policy) == resolvedPolicyFingerprint(changed) {
		t.Fatal("policy fingerprint omitted plateau levels")
	}
	changed = policy
	changed.PlateauVariant = PlateauVariantLarge16
	if resolvedPolicyFingerprint(policy) == resolvedPolicyFingerprint(changed) {
		t.Fatal("policy fingerprint omitted plateau variant")
	}
}

func TestPlateauVariantsResolveNamedPolicies(t *testing.T) {
	legacy := []PlateauLevelPolicy{
		{MaxNeighborhoodSize: 8, QuotaBps: 2500},
		{MaxNeighborhoodSize: 9, QuotaBps: 2500},
		{MaxNeighborhoodSize: 10, QuotaBps: 2500},
		{MaxNeighborhoodSize: 12, QuotaBps: 2500},
	}
	tests := []struct {
		variant string
		levels  []PlateauLevelPolicy
	}{
		{PlateauVariantLegacyLargeOff, legacy},
		{PlateauVariantLarge16, append(append([]PlateauLevelPolicy(nil), []PlateauLevelPolicy{
			{MaxNeighborhoodSize: 8, QuotaBps: 2490},
			{MaxNeighborhoodSize: 9, QuotaBps: 2490},
			{MaxNeighborhoodSize: 10, QuotaBps: 2490},
			{MaxNeighborhoodSize: 12, QuotaBps: 2490},
		}...), PlateauLevelPolicy{MaxNeighborhoodSize: 16, MinMandatorySize: 13, MaxMandatorySize: 16, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000})},
		{PlateauVariantLarge1618, append(append([]PlateauLevelPolicy(nil), []PlateauLevelPolicy{
			{MaxNeighborhoodSize: 8, QuotaBps: 2480},
			{MaxNeighborhoodSize: 9, QuotaBps: 2480},
			{MaxNeighborhoodSize: 10, QuotaBps: 2480},
			{MaxNeighborhoodSize: 12, QuotaBps: 2480},
		}...), []PlateauLevelPolicy{
			{MaxNeighborhoodSize: 16, MinMandatorySize: 13, MaxMandatorySize: 16, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
			{MaxNeighborhoodSize: 18, MinMandatorySize: 17, MaxMandatorySize: 18, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
		}...)},
		{PlateauVariantLarge161820, append(append([]PlateauLevelPolicy(nil), []PlateauLevelPolicy{
			{MaxNeighborhoodSize: 8, QuotaBps: 2470},
			{MaxNeighborhoodSize: 9, QuotaBps: 2470},
			{MaxNeighborhoodSize: 10, QuotaBps: 2470},
			{MaxNeighborhoodSize: 12, QuotaBps: 2470},
		}...), []PlateauLevelPolicy{
			{MaxNeighborhoodSize: 16, MinMandatorySize: 13, MaxMandatorySize: 16, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
			{MaxNeighborhoodSize: 18, MinMandatorySize: 17, MaxMandatorySize: 18, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
			{MaxNeighborhoodSize: 20, MinMandatorySize: 19, MaxMandatorySize: 20, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
		}...)},
	}

	var legacyFiveMillionFingerprint string
	for _, test := range tests {
		t.Run(test.variant, func(t *testing.T) {
			policy := resolveSearchPolicy(Config{PlateauVariant: test.variant}, 15_000_000)
			if policy.PlateauVariant != test.variant || !reflect.DeepEqual(policy.PlateauLevels, test.levels) {
				t.Fatalf("15M policy=%+v want variant %q levels %+v", policy, test.variant, test.levels)
			}
			fiveMillion := resolveSearchPolicy(Config{PlateauVariant: test.variant}, 5_000_000)
			if fiveMillion.PlateauVariant != test.variant || !reflect.DeepEqual(fiveMillion.PlateauLevels, legacy) {
				t.Fatalf("5M policy=%+v want selected variant with legacy levels", fiveMillion)
			}
			fingerprint := resolvedPolicyFingerprint(fiveMillion)
			if test.variant == PlateauVariantLegacyLargeOff {
				legacyFiveMillionFingerprint = fingerprint
			} else if fingerprint == legacyFiveMillionFingerprint {
				t.Fatalf("5M fingerprint omitted selected variant %q", test.variant)
			}
		})
	}
}

func TestBenchmarkSettingsReportActualPlateauStages(t *testing.T) {
	settings := SettingsForBenchmark(20_000_000, PlateauVariantLarge16)
	if len(settings.StageSettings) != 2 {
		t.Fatalf("stage settings=%+v want two stages", settings.StageSettings)
	}
	prefix := settings.StageSettings[0]
	remainder := settings.StageSettings[1]
	if prefix.ID != "prefix-5m" || prefix.NodeLimit != 5_000_000 || prefix.PlateauVariant != PlateauVariantLegacyLargeOff {
		t.Fatalf("prefix stage=%+v", prefix)
	}
	if remainder.ID != "remainder-15m" || remainder.NodeLimit != 15_000_000 || remainder.PlateauVariant != PlateauVariantLarge16 {
		t.Fatalf("remainder stage=%+v", remainder)
	}
	if len(prefix.PlateauLevels) != 4 || len(remainder.PlateauLevels) != 5 || settings.PlateauLevels[len(settings.PlateauLevels)-1].MaxNeighborhoodSize != 16 {
		t.Fatalf("stage levels prefix=%+v remainder=%+v requested=%+v", prefix.PlateauLevels, remainder.PlateauLevels, settings.PlateauLevels)
	}
}

func TestFifteenMillionPlateauLevelsIncludeDeepBands(t *testing.T) {
	policy := resolveSearchPolicy(Config{PlateauVariant: PlateauVariantLarge161820}, 15_000_000)
	want := []PlateauLevelPolicy{
		{MaxNeighborhoodSize: 8, QuotaBps: 2470},
		{MaxNeighborhoodSize: 9, QuotaBps: 2470},
		{MaxNeighborhoodSize: 10, QuotaBps: 2470},
		{MaxNeighborhoodSize: 12, QuotaBps: 2470},
		{MaxNeighborhoodSize: 16, MinMandatorySize: 13, MaxMandatorySize: 16, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
		{MaxNeighborhoodSize: 18, MinMandatorySize: 17, MaxMandatorySize: 18, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
		{MaxNeighborhoodSize: 20, MinMandatorySize: 19, MaxMandatorySize: 20, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
	}
	if !reflect.DeepEqual(policy.PlateauLevels, want) {
		t.Fatalf("15M plateau levels=%+v want %+v", policy.PlateauLevels, want)
	}
	var quotaBps int64
	for _, level := range policy.PlateauLevels {
		quotaBps += level.QuotaBps
	}
	if quotaBps != 10_000 {
		t.Fatalf("plateau quota bps=%d want 10000", quotaBps)
	}
	if settings := SettingsForBenchmark(15_000_000, PlateauVariantLarge161820); !reflect.DeepEqual(settings.PlateauLevels, policy.PlateauLevels) {
		t.Fatalf("benchmark plateau levels=%+v want %+v", settings.PlateauLevels, policy.PlateauLevels)
	}
}

func TestDeepPlateauMandatoryBandsAndClosureStats(t *testing.T) {
	levels := resolveSearchPolicy(Config{PlateauVariant: PlateauVariantLarge161820}, 15_000_000).PlateauLevels[4:]
	wants := map[int]int{12: 0, 13: 16, 16: 16, 17: 18, 18: 18, 19: 20, 20: 20}
	for mandatory, wantSize := range wants {
		gotSize := 0
		for _, level := range levels {
			if level.matchesMandatorySize(mandatory) {
				gotSize = level.MaxNeighborhoodSize
			}
		}
		if gotSize != wantSize {
			t.Fatalf("mandatory size %d matched level %d want %d", mandatory, gotSize, wantSize)
		}
	}

	archive := newPlateauArchive(&priorityBoundContext{})
	level := levels[0]
	archive.recordLevelClosure(level, 12)
	archive.recordLevelClosure(level, 13)
	archive.recordLevelClosure(level, 17)
	stats := archive.stats().LevelStats
	if len(stats) != 1 || stats[0].RejectedBelowBand != 1 || stats[0].CandidatesMatchingBand != 1 || stats[0].RejectedAboveBand != 1 {
		t.Fatalf("deep level closure stats=%+v", stats)
	}
}

func TestDeepPlateauSelectionHonorsDeterministicCaps(t *testing.T) {
	neighborhoods := []repairNeighborhood{
		{Key: "a", BaseLayoutKey: "base-a"},
		{Key: "b", BaseLayoutKey: "base-a"},
		{Key: "c", BaseLayoutKey: "base-b"},
		{Key: "d", BaseLayoutKey: "base-c"},
	}
	level := PlateauLevelPolicy{MaxSelected: 2, MaxSelectedPerBase: 1}
	selected, perBaseDrops, selectedCapDrops := selectPlateauLevelNeighborhoods(neighborhoods, level)
	got := make([]string, len(selected))
	for index, neighborhood := range selected {
		got[index] = neighborhood.Key
	}
	if !reflect.DeepEqual(got, []string{"a", "c"}) || perBaseDrops != 1 || selectedCapDrops != 1 {
		t.Fatalf("selected=%v per-base/cap drops=%d/%d", got, perBaseDrops, selectedCapDrops)
	}
}

func TestDeepPlateauSelectionUsesMandatorySizeTieBreak(t *testing.T) {
	level := PlateauLevelPolicy{MinMandatorySize: 13, MaxSelected: 1}
	neighborhoods := []repairNeighborhood{
		{Key: "larger", BaseLayoutKey: "base-a", Priority: 100, MandatorySize: 16},
		{Key: "smaller", BaseLayoutKey: "base-b", Priority: 100, MandatorySize: 13},
	}
	selected, _, capDrops := selectPlateauLevelNeighborhoods(neighborhoods, level)
	if len(selected) != 1 || selected[0].Key != "smaller" || capDrops != 1 {
		t.Fatalf("selected=%+v capDrops=%d", selected, capDrops)
	}
}

func TestPlateauQuotaUsesRemainderAndMinimumGate(t *testing.T) {
	levels := []PlateauLevelPolicy{
		{QuotaBps: 2500},
		{QuotaBps: 2500},
		{QuotaBps: 2500},
		{QuotaBps: 2500, MinNodeBudget: 3_000},
	}
	remaining := int64(10_003)
	var allocated int64
	for index := range levels {
		quota := plateauLevelQuota(10_003, remaining, 0, index, levels)
		allocated += quota
		remaining -= quota
		if index < len(levels)-1 && quota != 2_500 {
			t.Fatalf("level %d quota=%d want 2500", index, quota)
		}
		if index == len(levels)-1 {
			if quota != 2_503 || plateauLevelCanRun(quota, levels[index]) {
				t.Fatalf("last quota=%d should carry remainder and fail minimum gate", quota)
			}
		}
	}
	if allocated != 10_003 || remaining != 0 {
		t.Fatalf("allocated=%d remaining=%d", allocated, remaining)
	}
}

func TestDeepPlateauLastLevelDoesNotAbsorbLegacyRemainder(t *testing.T) {
	levels := []PlateauLevelPolicy{
		{MaxNeighborhoodSize: 8, QuotaBps: 2490},
		{MaxNeighborhoodSize: 16, MinMandatorySize: 13, QuotaBps: 40, MinNodeBudget: 5_000},
	}
	if got := plateauLevelQuota(1_500_000, 1_126_500, 0, 1, levels); got != 6_000 {
		t.Fatalf("deep last-level quota=%d want 6000", got)
	}
}
