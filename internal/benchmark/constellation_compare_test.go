package benchmark

import (
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/solver"
)

func TestCompareConstellationExperimentUsesCanonicalScoreAndSeparatesHashCost(t *testing.T) {
	baselineScore := model.Score{PriorityCounts: []int{6, 12}, StarCount: 58}
	currentScore := model.Score{PriorityCounts: []int{5, 99}, StarCount: 99}
	baseline := constellationExperimentReport("v4", 1, constellationExperimentRun(baselineScore, "baseline", 100, 40, "execution-v4", "policy-v4", v4Diagnostics("strict", baselineScore)))
	current := constellationExperimentReport("v5", 1, constellationExperimentRun(currentScore, "different-layout", 125, 44, "execution-v5", "policy-v5", v5Diagnostics("strict", "relaxed", currentScore)))
	comparison, err := CompareConstellationExperimentReports(baseline, current)
	if err != nil {
		t.Fatalf("compare reports: %v", err)
	}
	row := comparison.Rows[0]
	if row.ScoreStatus != "LOSS" || row.ScoreCmp != -1 || row.SameHash || row.NormalNodeDelta != 25 || row.NormalNodeRatio != 1.25 || row.FirstCompletionNodeDelta == nil || *row.FirstCompletionNodeDelta != 4 || row.V5BehaviorClass != v5BehaviorFrontierSelected || row.RegressionAttribution != regressionFrontier {
		t.Fatalf("row=%+v", row)
	}
	if len(row.RootPairs) != 1 || row.RootPairs[0].Kind != "frontier_replacement" || row.RootPairs[0].ScoreCmp >= 0 {
		t.Fatalf("root pairs=%+v", row.RootPairs)
	}
}

func TestCompareConstellationExperimentTreatsDifferentHashAtEqualScoreAsTie(t *testing.T) {
	score := model.Score{PriorityCounts: []int{6, 12}, StarCount: 58}
	baseline := constellationExperimentReport("v4", 1, constellationExperimentRun(score, "baseline", 100, 40, "execution-v4", "policy-v4", v4Diagnostics("strict", score)))
	current := constellationExperimentReport("v5", 1, constellationExperimentRun(score, "different", 100, 40, "execution-v5", "policy-v5", v5Diagnostics("strict", "relaxed", score)))
	comparison, err := CompareConstellationExperimentReports(baseline, current)
	if err != nil {
		t.Fatalf("compare reports: %v", err)
	}
	row := comparison.Rows[0]
	if row.ScoreStatus != "TIE" || row.ScoreCmp != 0 || row.SameHash {
		t.Fatalf("row=%+v", row)
	}
}

func TestCompareConstellationExperimentAttributesSameRootRegression(t *testing.T) {
	score := model.Score{PriorityCounts: []int{6, 12}, StarCount: 58}
	baselineDiagnostics := &model.ConstellationSeedDiagnostics{
		Skeletons: []model.ConstellationSkeletonDiagnostic{{ID: "strict-skeleton", ExactKey: "strict"}, {ID: "same-skeleton", ExactKey: "same"}},
		Roots: []model.ConstellationRootDiagnostic{
			{ID: "root-1", SkeletonID: "strict-skeleton", Completed: true, BestScore: &score},
			{ID: "root-2", SkeletonID: "same-skeleton", Completed: true, BestScore: &score},
		},
	}
	lower := model.Score{PriorityCounts: []int{6, 12}, StarCount: 57}
	currentDiagnostics := &model.ConstellationSeedDiagnostics{
		Version:                          "v5",
		RelaxationFrontierExists:         true,
		RelaxationFrontierSelected:       true,
		RelaxationFrontierRootID:         "root-1",
		RelaxationFrontierParentExactKey: "strict",
		Skeletons:                        []model.ConstellationSkeletonDiagnostic{{ID: "frontier-skeleton", ExactKey: "frontier"}, {ID: "same-skeleton", ExactKey: "same"}},
		Roots: []model.ConstellationRootDiagnostic{
			{ID: "root-1", SkeletonID: "frontier-skeleton", Completed: true, BestScore: &score},
			{ID: "root-2", SkeletonID: "same-skeleton", Completed: true, BestScore: &lower},
		},
	}
	baseline := constellationExperimentReport("v4", 1, constellationExperimentRun(score, "baseline", 100, 40, "execution-v4", "policy-v4", baselineDiagnostics))
	current := constellationExperimentReport("v5", 1, constellationExperimentRun(lower, "current", 100, 40, "execution-v5", "policy-v5", currentDiagnostics))
	comparison, err := CompareConstellationExperimentReports(baseline, current)
	if err != nil {
		t.Fatalf("compare reports: %v", err)
	}
	if got := comparison.Rows[0].RegressionAttribution; got != regressionPacking {
		t.Fatalf("attribution=%q rows=%+v", got, comparison.Rows[0].RootPairs)
	}
}

func TestCompareConstellationExperimentValidatesWorkerAndSemanticPolicies(t *testing.T) {
	score := model.Score{PriorityCounts: []int{1}, StarCount: 1}
	v4 := constellationExperimentReport("v4", 1, constellationExperimentRun(score, "hash", 10, 4, "execution-v4", "policy-v4", nil))
	v5DifferentWorkers := constellationExperimentReport("v5", 2, constellationExperimentRun(score, "hash", 10, 4, "execution-v5", "policy-v5", nil))
	if _, err := CompareConstellationExperimentReports(v4, v5DifferentWorkers); err == nil || !strings.Contains(err.Error(), "equal worker") {
		t.Fatalf("V4/V5 worker validation err=%v", err)
	}

	v5Serial := constellationExperimentReport("v5", 1, constellationExperimentRun(score, "hash", 10, 4, "execution-serial", "semantic-policy", nil))
	v5Parallel := constellationExperimentReport("v5", 4, constellationExperimentRun(score, "hash", 10, 4, "execution-parallel", "semantic-policy", nil))
	comparison, err := CompareConstellationExperimentReports(v5Serial, v5Parallel)
	if err != nil {
		t.Fatalf("parallel compare: %v", err)
	}
	if comparison.Mode != constellationComparisonModeParallel || comparison.Rows[0].SemanticPolicyFingerprint == "" || !comparison.Rows[0].ExecutionFingerprintChanged {
		t.Fatalf("parallel comparison=%+v", comparison)
	}

	v5DifferentPolicy := constellationExperimentReport("v5", 4, constellationExperimentRun(score, "hash", 10, 4, "execution-parallel", "different-policy", nil))
	if _, err := CompareConstellationExperimentReports(v5Serial, v5DifferentPolicy); err == nil || !strings.Contains(err.Error(), "semantic policy") {
		t.Fatalf("parallel semantic validation err=%v", err)
	}

	v5Serial.Runs[0].Search.Stages = nil
	v5Parallel.Runs[0].Search.Stages = nil
	comparison, err = CompareConstellationExperimentReports(v5Serial, v5Parallel)
	if err != nil || comparison.Rows[0].SemanticPolicyFingerprint == "" {
		t.Fatalf("settings fallback comparison=%+v err=%v", comparison, err)
	}
}

func constellationExperimentReport(variant string, workers int, run Run) Report {
	run.ConstellationSeedVariant = variant
	return Report{
		CatalogSHA256:            "catalog",
		Budgets:                  []int64{1_000_000},
		Repeat:                   1,
		Workers:                  workers,
		Top:                      1,
		RepairSearchMode:         RepairSearchModeScenario,
		PlateauVariant:           "legacy-large-off",
		Diagnostic:               true,
		ConstellationSeedVariant: variant,
		Runs:                     []Run{run},
	}
}

func constellationExperimentRun(score model.Score, hash string, nodes int64, firstComplete int64, executionFingerprint string, policyFingerprint string, diagnostics *model.ConstellationSeedDiagnostics) Run {
	return Run{
		Scenario:            "scenario",
		ScenarioPath:        "scenario.json",
		Budget:              1_000_000,
		Repeat:              1,
		RepairSearch:        true,
		PlateauVariant:      "legacy-large-off",
		PrioritySemantics:   model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:          []string{"star_source:left", "star_source:right"},
		Score:               ScoreSummary{PriorityCounts: append([]int(nil), score.PriorityCounts...), Crafts: score.CraftCount, Stars: score.StarCount, Items: score.ItemCount, StarTargetBreadth: score.StarTargetBreadth, StarReciprocalPairs: score.StarReciprocalPairs, StarSourceDefinitionDiversity: score.StarSourceDefinitionDiversity},
		CanonicalLayoutHash: hash,
		SolverSettings:      solverSettingsForExperiment(),
		Search: SearchSummary{
			NodesExplored:                nodes,
			NormalBudgetConsumed:         nodes,
			FirstCompleteNodes:           firstComplete,
			ExecutionFingerprint:         executionFingerprint,
			ConstellationSeedDiagnostics: diagnostics,
			Stages:                       []model.SearchStageStats{{ID: "single", StagePolicyFingerprint: policyFingerprint}},
		},
	}
}

func solverSettingsForExperiment() solver.BenchmarkSettings {
	return solver.BenchmarkSettings{ConstellationSeedPackingBeamWidth: 128, ConstellationSeedPackingStrategy: "state_mrv", ConstellationSeedMaxSkeletons: 4, ConstellationSeedShareBps: 1_500}
}

func v4Diagnostics(exactKey string, score model.Score) *model.ConstellationSeedDiagnostics {
	return &model.ConstellationSeedDiagnostics{
		Version: "v4",
		Skeletons: []model.ConstellationSkeletonDiagnostic{
			{ID: "strict-skeleton", ExactKey: exactKey},
		},
		Roots: []model.ConstellationRootDiagnostic{
			{ID: "root-1", SkeletonID: "strict-skeleton", Completed: true, BestScore: &score},
		},
	}
}

func v5Diagnostics(parentExact string, frontierExact string, frontierScore model.Score) *model.ConstellationSeedDiagnostics {
	return &model.ConstellationSeedDiagnostics{
		Version:                          "v5",
		RelaxationFrontierExists:         true,
		RelaxationFrontierSelected:       true,
		RelaxationFrontierRootID:         "root-1",
		RelaxationFrontierParentExactKey: parentExact,
		ConstellationRootWinnerID:        "root-1",
		ConstellationRootWinnerScore:     &frontierScore,
		ConstellationRootWinnerHash:      "frontier-hash",
		Skeletons: []model.ConstellationSkeletonDiagnostic{
			{ID: "frontier-skeleton", ExactKey: frontierExact},
		},
		Roots: []model.ConstellationRootDiagnostic{
			{ID: "root-1", SkeletonID: "frontier-skeleton", Completed: true, BestScore: &frontierScore},
		},
	}
}
