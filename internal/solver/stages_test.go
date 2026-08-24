package solver

import (
	"reflect"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestConfiguredTwentyMillionRunPreservesFiveMillionPrefix(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "110000000/000000000/000000000/000000000/000000000/000000000")
	config := Config{
		TopN:              1,
		AllowSkips:        false,
		Workers:           1,
		Diagnostics:       true,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source"},
	}
	fiveConfig := config
	fiveConfig.MaxNodes = 5_000_000
	five, err := SolveLayout(cat, []string{"source", "food"}, grid, fiveConfig)
	if err != nil {
		t.Fatalf("SolveLayout(5M): %v", err)
	}
	twentyConfig := config
	twentyConfig.MaxNodes = 20_000_000
	twenty, err := SolveLayout(cat, []string{"source", "food"}, grid, twentyConfig)
	if err != nil {
		t.Fatalf("SolveLayout(20M): %v", err)
	}
	if len(five) != 1 || len(twenty) != 1 || len(five[0].Search.Stages) != 1 || len(twenty[0].Search.Stages) != 2 {
		t.Fatalf("unexpected stage results: five=%d/%d twenty=%d/%d", len(five), len(five[0].Search.Stages), len(twenty), len(twenty[0].Search.Stages))
	}
	prefix := twenty[0].Search.Stages[0]
	isolated := five[0].Search.Stages[0]
	if prefix.StagePolicyFingerprint != isolated.StagePolicyFingerprint || !reflect.DeepEqual(prefix.StageOutputScore, isolated.StageOutputScore) || prefix.NodesCharged != isolated.NodesCharged {
		t.Fatalf("20M prefix differs from isolated 5M: prefix=%+v isolated=%+v", prefix, isolated)
	}
	if !sameIncumbentDecisionTrace(prefix.IncumbentTrace, isolated.IncumbentTrace) || !reflect.DeepEqual(prefix.PlateauArchive, isolated.PlateauArchive) {
		t.Fatalf("20M prefix changed the 5M diagnostic trace or archive: prefix=%+v isolated=%+v", prefix.PlateauArchive, isolated.PlateauArchive)
	}
	if twenty[0].Search.ExecutionFingerprint == five[0].Search.ExecutionFingerprint {
		t.Fatal("execution fingerprint omitted the total budget or stage schedule")
	}
	if !reflect.DeepEqual(twenty[0].Search.Stages[1].StageInputScore, prefix.FinalCarriedScore) {
		t.Fatalf("second stage input=%+v want carried prefix=%+v", twenty[0].Search.Stages[1].StageInputScore, prefix.FinalCarriedScore)
	}
	if compareScores(twenty[0].Evaluation.Score, five[0].Evaluation.Score) < 0 {
		t.Fatalf("20M regressed from 5M: twenty=%+v five=%+v", twenty[0].Evaluation.Score, five[0].Evaluation.Score)
	}
	var charged int64
	var executionCharged int64
	for _, stage := range twenty[0].Search.Stages {
		charged += stage.NodesCharged
		if stage.StageBudgetConsumed != stage.NodesCharged {
			t.Fatalf("stage %s consumed=%d want charged=%d", stage.ID, stage.StageBudgetConsumed, stage.NodesCharged)
		}
		executionCharged += stage.StageBudgetConsumed
		if stage.ExecutionBudgetConsumed != executionCharged {
			t.Fatalf("stage %s execution consumed=%d want %d", stage.ID, stage.ExecutionBudgetConsumed, executionCharged)
		}
	}
	if charged != twenty[0].Search.NodesExplored || charged > twentyConfig.MaxNodes {
		t.Fatalf("stage charges=%d total=%d budget=%d", charged, twenty[0].Search.NodesExplored, twentyConfig.MaxNodes)
	}
	for _, event := range prefix.IncumbentTrace {
		if event.StageBudgetConsumed != event.ExecutionBudgetConsumed || event.StageBudgetConsumed != event.GlobalBudgetConsumed {
			t.Fatalf("prefix event has inconsistent scopes: %+v", event)
		}
	}
}

func TestSolveLayoutAggregatesReferenceDiagnosticsAcrossStages(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "110000000/000000000/000000000/000000000/000000000/000000000")
	instances := ExpandInventory([]string{"source", "food"})
	options := testOptionsForGrid(t, cat, instances, grid)
	solutions, err := SolveLayout(cat, []string{"source", "food"}, grid, Config{
		TopN:        1,
		AllowSkips:  false,
		MaxNodes:    20_000_000,
		Workers:     1,
		Diagnostics: true,
		DiagnosticReference: []model.Placement{
			testPlacement(t, options["source#0"], model.Coord{}, 0),
			testPlacement(t, options["food#1"], model.Coord{Col: 1}, 0),
		},
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source"},
	})
	if err != nil {
		t.Fatalf("multi-stage solve: %v", err)
	}
	if len(solutions) != 1 || len(solutions[0].Search.Stages) != 2 {
		t.Fatalf("multi-stage results=%d stages=%d", len(solutions), len(solutions[0].Search.Stages))
	}
	var evaluations int64
	for _, stage := range solutions[0].Search.Stages {
		if stage.PlateauArchive.ReferenceEvaluations == 0 || stage.PlateauArchive.MinimumReferenceDistance == nil {
			t.Fatalf("stage %s reference diagnostics=%+v", stage.ID, stage.PlateauArchive)
		}
		evaluations += stage.PlateauArchive.ReferenceEvaluations
	}
	aggregated := solutions[0].Search.PlateauArchive
	if aggregated.ReferenceEvaluations != evaluations || aggregated.MinimumReferenceDistance == nil {
		t.Fatalf("aggregate reference diagnostics=%+v want evaluations=%d", aggregated, evaluations)
	}
}

func TestAggregateStageSearchesSelectsDeterministicReferenceMinimum(t *testing.T) {
	first := &model.ReferenceDistance{
		Delta:               model.ReferenceDelta{StructuralDistance: 3, CanonicalLinksLost: 2},
		LayoutKey:           "first",
		CanonicalLayoutHash: "first",
	}
	second := &model.ReferenceDistance{
		Delta:               model.ReferenceDelta{StructuralDistance: 3, CanonicalLinksLost: 1},
		LayoutKey:           "second",
		CanonicalLayoutHash: "second",
	}
	aggregated := aggregateStageSearches([]model.SearchStats{
		{PlateauArchive: model.PlateauArchiveStats{ReferenceEvaluations: 4, MinimumReferenceDistance: first}},
		{PlateauArchive: model.PlateauArchiveStats{ReferenceEvaluations: 7, MinimumReferenceDistance: second}},
	}, 11)
	if aggregated.PlateauArchive.ReferenceEvaluations != 11 {
		t.Fatalf("reference evaluations=%d want 11", aggregated.PlateauArchive.ReferenceEvaluations)
	}
	if !reflect.DeepEqual(aggregated.PlateauArchive.MinimumReferenceDistance, second) {
		t.Fatalf("reference minimum=%+v want %+v", aggregated.PlateauArchive.MinimumReferenceDistance, second)
	}
}

func TestAggregateStageSearchesMergesPackingSeedFeasibilityOperationProfiles(t *testing.T) {
	first := &model.PackingSeedFeasibilityOperationProfile{
		Version:                 PackingSeedFeasibilityProfileVersion,
		SearchCalls:             1,
		CandidateExpansions:     10,
		FeasibilityOptionChecks: 100,
		CandidateCanonical: model.PackingSeedCanonicalCopyOrderOperationProfile{
			PlacementKeyBytes: 200,
		},
	}
	second := &model.PackingSeedFeasibilityOperationProfile{
		Version:                 PackingSeedFeasibilityProfileVersion,
		SearchCalls:             1,
		CandidateExpansions:     20,
		FeasibilityOptionChecks: 300,
		CandidateCanonical: model.PackingSeedCanonicalCopyOrderOperationProfile{
			PlacementKeyBytes: 500,
		},
	}
	aggregated := aggregateStageSearches([]model.SearchStats{
		{PackingSeedOperationProfile: first},
		{PackingSeedOperationProfile: second},
	}, 30)
	profile := aggregated.PackingSeedOperationProfile
	if profile == nil || profile.SearchCalls != 2 || profile.CandidateExpansions != 30 || profile.FeasibilityOptionChecks != 400 || profile.CandidateCanonical.PlacementKeyBytes != 700 {
		t.Fatalf("aggregated packing-seed feasibility profile=%+v", profile)
	}
}

func TestAggregateStageSearchesMergesBoundAttributionOperationProfiles(t *testing.T) {
	first := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	first.PriorityUpper.ConstellationFilter.Calls = 3
	first.Outgoing.Search.Checks = 5
	second := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	second.PriorityUpper.ConstellationFilter.Calls = 7
	second.Outgoing.Repair.Checks = 11
	aggregated := aggregateStageSearches([]model.SearchStats{
		{BoundOperationProfile: first},
		{BoundOperationProfile: second},
	}, 30)
	profile := aggregated.BoundOperationProfile
	if profile == nil || profile.PriorityUpper.ConstellationFilter.Calls != 10 || profile.Outgoing.Search.Checks != 5 || profile.Outgoing.Repair.Checks != 11 {
		t.Fatalf("aggregated bound attribution profile=%+v", profile)
	}
}

func TestConfiguredTwentyMillionRunPreservesEnabledConstellationPrefix(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	config := Config{TopN: 1, AllowSkips: false, Workers: 1, Diagnostics: true, EnableConstellationSeedV1: true, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	fiveConfig := config
	fiveConfig.MaxNodes = 5_000_000
	five, err := SolveLayout(cat, []string{"left", "right", "food"}, grid, fiveConfig)
	if err != nil {
		t.Fatalf("5M solve: %v", err)
	}
	twentyConfig := config
	twentyConfig.MaxNodes = 20_000_000
	twenty, err := SolveLayout(cat, []string{"left", "right", "food"}, grid, twentyConfig)
	if err != nil {
		t.Fatalf("20M solve: %v", err)
	}
	if len(five) != 1 || len(twenty) != 1 {
		t.Fatalf("results five=%d twenty=%d", len(five), len(twenty))
	}
	isolated := five[0].Search.Stages[0]
	prefix := twenty[0].Search.Stages[0]
	if isolated.StagePolicyFingerprint != prefix.StagePolicyFingerprint || !reflect.DeepEqual(isolated.StageOutputScore, prefix.StageOutputScore) || isolated.NodesCharged != prefix.NodesCharged || !sameIncumbentDecisionTrace(isolated.IncumbentTrace, prefix.IncumbentTrace) || !reflect.DeepEqual(isolated.PlateauArchive, prefix.PlateauArchive) {
		t.Fatalf("enabled constellation prefix differs: isolated=%+v prefix=%+v", isolated, prefix)
	}
	if five[0].Search.ConstellationSeedNodes == 0 || twenty[0].Search.ConstellationSeedNodes < five[0].Search.ConstellationSeedNodes {
		t.Fatalf("constellation stats five=%d twenty=%d", five[0].Search.ConstellationSeedNodes, twenty[0].Search.ConstellationSeedNodes)
	}
}

func TestConfiguredTwentyMillionRunPreservesV4ConstellationPrefix(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	config := Config{TopN: 1, AllowSkips: false, Workers: 1, Diagnostics: true, ConstellationSeedVariant: ConstellationSeedVariantV4, ConstellationCandidatePoolFeasibilitySweep: true, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	fiveConfig := config
	fiveConfig.MaxNodes = 5_000_000
	five, err := SolveLayout(cat, []string{"left", "right", "food"}, grid, fiveConfig)
	if err != nil {
		t.Fatalf("5M solve: %v", err)
	}
	twentyConfig := config
	twentyConfig.MaxNodes = 20_000_000
	twenty, err := SolveLayout(cat, []string{"left", "right", "food"}, grid, twentyConfig)
	if err != nil {
		t.Fatalf("20M solve: %v", err)
	}
	if len(five) != 1 || len(twenty) != 1 {
		t.Fatalf("results five=%d twenty=%d", len(five), len(twenty))
	}
	isolation := five[0].Search.Stages[0]
	prefix := twenty[0].Search.Stages[0]
	if isolation.StagePolicyFingerprint != prefix.StagePolicyFingerprint || !reflect.DeepEqual(isolation.StageOutputScore, prefix.StageOutputScore) || isolation.NodesCharged != prefix.NodesCharged || !sameIncumbentDecisionTrace(isolation.IncumbentTrace, prefix.IncumbentTrace) || !reflect.DeepEqual(isolation.PlateauArchive, prefix.PlateauArchive) {
		t.Fatalf("V4 constellation prefix differs: isolated=%+v prefix=%+v", isolation, prefix)
	}
	if five[0].Search.ConstellationSeedDiagnostics.Version != ConstellationSeedVariantV4 || twenty[0].Search.Stages[0].StagePolicyFingerprint == "" {
		t.Fatalf("V4 diagnostics/stage=%+v/%+v", five[0].Search.ConstellationSeedDiagnostics, prefix)
	}
	isolationSweep := five[0].Search.ConstellationSeedDiagnostics.CandidatePoolFeasibilitySweep
	aggregatedSweep := twenty[0].Search.ConstellationSeedDiagnostics.CandidatePoolFeasibilitySweep
	if isolationSweep == nil || aggregatedSweep == nil {
		t.Fatalf("missing V4 sweep isolation=%+v aggregate=%+v", isolationSweep, aggregatedSweep)
	}
	var prefixRecords []model.ConstellationCandidateFeasibilityRecord
	var remainderRecords []model.ConstellationCandidateFeasibilityRecord
	for _, record := range aggregatedSweep.Candidates {
		switch record.StageID {
		case "prefix-5m":
			record.StageID = "single"
			prefixRecords = append(prefixRecords, record)
		case "remainder-15m":
			remainderRecords = append(remainderRecords, record)
		default:
			t.Fatalf("unexpected sweep record stage=%q", record.StageID)
		}
	}
	if !reflect.DeepEqual(prefixRecords, isolationSweep.Candidates) || len(remainderRecords) == 0 {
		t.Fatalf("V4 sweep reused or changed stages: prefix=%+v isolated=%+v remainder=%+v", prefixRecords, isolationSweep.Candidates, remainderRecords)
	}
}

func TestAggregateStagePhaseWorkPreservesAccounting(t *testing.T) {
	best := model.Score{StarCount: 2}
	stages := []model.SearchStats{
		{PhaseWork: []model.SearchPhaseWork{
			{Phase: tracePhasePackingSeed, Eligible: true, Invoked: true, ChargedNodes: 10, NodesConsumed: 10, NodesReserved: 20, NodesReturned: 10, ReturnTarget: "dfs", BestScore: &best},
			{Phase: tracePhasePreRepair, Eligible: true, Invoked: true, NodesReserved: 5, NodesReturned: 5, TerminationReason: "no_eligible_neighborhoods", ReturnTarget: "dfs"},
		}},
		{PhaseWork: []model.SearchPhaseWork{
			{Phase: tracePhasePackingSeed, Eligible: true, Invoked: true, ChargedNodes: 7, NodesConsumed: 7, NodesReserved: 10, NodesReturned: 3, ReturnTarget: "dfs"},
			{Phase: tracePhasePreRepair, Eligible: false, SkipReason: "disabled"},
		}},
	}
	aggregated := aggregatePhaseWork(stages)
	if len(aggregated) != 2 {
		t.Fatalf("phase count=%d want 2", len(aggregated))
	}
	packing := aggregated[0]
	if packing.Phase != tracePhasePackingSeed || packing.ChargedNodes != 17 || packing.NodesConsumed != 17 || packing.NodesReserved != 30 || packing.NodesReturned != 13 || !packing.Eligible || !packing.Invoked || packing.BestScore == nil || packing.BestScore.StarCount != 2 {
		t.Fatalf("aggregated packing=%+v", packing)
	}
	repair := aggregated[1]
	if repair.Phase != tracePhasePreRepair || !repair.Eligible || !repair.Invoked || repair.NodesReserved != 5 || repair.NodesReturned != 5 || repair.TerminationReason != "no_eligible_neighborhoods" {
		t.Fatalf("aggregated repair=%+v", repair)
	}
}

func sameIncumbentDecisionTrace(left []model.IncumbentEvent, right []model.IncumbentEvent) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftEvent := left[index]
		rightEvent := right[index]
		// Wall-clock elapsed time is not a search decision and naturally varies.
		leftEvent.ElapsedMS = 0
		rightEvent.ElapsedMS = 0
		if !reflect.DeepEqual(leftEvent, rightEvent) {
			return false
		}
	}
	return true
}
