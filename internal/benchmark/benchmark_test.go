package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/solver"
)

func TestParseBudgets(t *testing.T) {
	budgets, err := ParseBudgets("10, 50,100")
	if err != nil {
		t.Fatalf("ParseBudgets returned error: %v", err)
	}
	want := []int64{10, 50, 100}
	if len(budgets) != len(want) {
		t.Fatalf("len=%d want %d", len(budgets), len(want))
	}
	for idx := range want {
		if budgets[idx] != want[idx] {
			t.Fatalf("budgets[%d]=%d want %d", idx, budgets[idx], want[idx])
		}
	}
}

func TestRunScenariosRespectsBudgetsAndRepeats(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "grid": ["111111111", "111111111", "111111111", "111111111", "111111111", "111111111"],
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)

	report, err := RunScenarios(RunConfig{
		CatalogPath: filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir: dir,
		Budgets:     []int64{100, 200},
		Repeat:      2,
		Workers:     1,
		Top:         1,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if len(report.Runs) != 4 {
		t.Fatalf("len(report.Runs)=%d want 4", len(report.Runs))
	}
	if report.RepairSearchMode != RepairSearchModeScenario {
		t.Fatalf("repair_search_mode=%q want %q", report.RepairSearchMode, RepairSearchModeScenario)
	}
	if report.PlateauVariant != solver.DefaultPlateauVariant {
		t.Fatalf("plateau_variant=%q want %q", report.PlateauVariant, solver.DefaultPlateauVariant)
	}
	if report.ConstellationSeedV1 {
		t.Fatal("constellation_seed_v1=true want false by default")
	}
	for _, run := range report.Runs {
		if run.Scenario != "tiny" {
			t.Fatalf("scenario=%q want tiny", run.Scenario)
		}
		if run.RepairSearch {
			t.Fatalf("repair search should follow scenario false by default")
		}
		if run.PlateauVariant != solver.DefaultPlateauVariant {
			t.Fatalf("run plateau_variant=%q want %q", run.PlateauVariant, solver.DefaultPlateauVariant)
		}
		if run.ConstellationSeedV1 {
			t.Fatal("run constellation_seed_v1=true want false by default")
		}
		if run.Error != "" {
			t.Fatalf("unexpected run error: %s", run.Error)
		}
		if len(run.Solution) == 0 {
			t.Fatalf("solution JSON should be present")
		}
	}
}

func TestRunScenariosRetainsConstellationSeedV1(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)

	report, err := RunScenarios(RunConfig{
		CatalogPath:                   filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:                   dir,
		Budgets:                       []int64{100},
		Repeat:                        1,
		Workers:                       1,
		Top:                           1,
		Diagnostic:                    true,
		ConstellationSeedV1:           true,
		ConstellationFeasibilityProbe: true,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if !report.ConstellationSeedV1 || !report.Runs[0].ConstellationSeedV1 || !report.ConstellationFeasibilityProbe || !report.Runs[0].ConstellationFeasibilityProbe || !report.Runs[0].SolverSettings.ConstellationFeasibilityProbe {
		t.Fatalf("constellation report/run/settings=%+v/%+v/%+v", report, report.Runs[0], report.Runs[0].SolverSettings)
	}
}

func TestRunScenariosRequiresDiagnosticsForConstellationFeasibilityProbe(t *testing.T) {
	_, err := RunScenarios(RunConfig{
		CatalogPath:                   filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:                   t.TempDir(),
		Budgets:                       []int64{100},
		Repeat:                        1,
		Workers:                       1,
		Top:                           1,
		ConstellationFeasibilityProbe: true,
	})
	if err == nil || !strings.Contains(err.Error(), "requires diagnostic") {
		t.Fatalf("probe without diagnostics err=%v", err)
	}
}

func TestRunScenariosRetainsConstellationCompletionOptimizationProbe(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)

	report, err := RunScenarios(RunConfig{
		CatalogPath:                              filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:                              dir,
		Budgets:                                  []int64{100},
		Repeat:                                   1,
		Workers:                                  1,
		Top:                                      1,
		Diagnostic:                               true,
		ConstellationSeedVariant:                 solver.ConstellationSeedVariantV3,
		ConstellationCompletionOptimizationProbe: true,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if !report.ConstellationCompletionOptimizationProbe || !report.Runs[0].ConstellationCompletionOptimizationProbe || !report.Runs[0].SolverSettings.ConstellationCompletionOptimizationProbe {
		t.Fatalf("completion optimization report/run/settings=%+v/%+v/%+v", report, report.Runs[0], report.Runs[0].SolverSettings)
	}
}

func TestRunScenariosValidatesConstellationCompletionOptimizationProbe(t *testing.T) {
	base := RunConfig{
		CatalogPath:                              filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:                              t.TempDir(),
		Budgets:                                  []int64{100},
		Repeat:                                   1,
		Workers:                                  1,
		Top:                                      1,
		ConstellationCompletionOptimizationProbe: true,
	}
	_, err := RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "requires diagnostic") {
		t.Fatalf("probe without diagnostics err=%v", err)
	}
	base.Diagnostic = true
	base.ConstellationSeedVariant = solver.ConstellationSeedVariantV2
	_, err = RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "requires constellation seed variant") {
		t.Fatalf("probe without V3 err=%v", err)
	}
	base.ConstellationSeedVariant = solver.ConstellationSeedVariantV3
	base.ConstellationFeasibilityProbe = true
	_, err = RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "cannot run together") {
		t.Fatalf("combined probes err=%v", err)
	}
}

func TestRunScenariosRetainsConstellationCandidatePoolFeasibilitySweep(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)
	report, err := RunScenarios(RunConfig{
		CatalogPath:              filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:              dir,
		Budgets:                  []int64{100},
		Repeat:                   1,
		Workers:                  1,
		Top:                      1,
		Diagnostic:               true,
		ConstellationSeedVariant: solver.ConstellationSeedVariantV4,
		ConstellationCandidatePoolFeasibilitySweep: true,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if !report.ConstellationCandidatePoolFeasibilitySweep || !report.Runs[0].ConstellationCandidatePoolFeasibilitySweep || !report.Runs[0].SolverSettings.ConstellationCandidatePoolFeasibilitySweep {
		t.Fatalf("candidate sweep report/run/settings=%+v/%+v/%+v", report, report.Runs[0], report.Runs[0].SolverSettings)
	}
}

func TestRunScenariosValidatesConstellationCandidatePoolFeasibilitySweep(t *testing.T) {
	base := RunConfig{
		CatalogPath: filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir: t.TempDir(),
		Budgets:     []int64{100},
		Repeat:      1,
		Workers:     1,
		Top:         1,
		ConstellationCandidatePoolFeasibilitySweep: true,
	}
	_, err := RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "requires diagnostic") {
		t.Fatalf("sweep without diagnostics err=%v", err)
	}
	base.Diagnostic = true
	base.ConstellationSeedVariant = solver.ConstellationSeedVariantV3
	_, err = RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "requires constellation seed variant") {
		t.Fatalf("sweep without V4 err=%v", err)
	}
	base.ConstellationSeedVariant = solver.ConstellationSeedVariantV4
	base.ConstellationFeasibilityProbe = true
	_, err = RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "cannot run with another constellation probe") {
		t.Fatalf("sweep with probe err=%v", err)
	}
}

func TestRunScenariosRetainsConstellationCandidateCompletionOptimization(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)
	candidateID := strings.Repeat("0", 64)
	report, err := RunScenarios(RunConfig{
		CatalogPath:              filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:              dir,
		Budgets:                  []int64{100},
		Repeat:                   1,
		Workers:                  1,
		Top:                      1,
		Diagnostic:               true,
		ConstellationSeedVariant: solver.ConstellationSeedVariantV4,
		ConstellationCandidateCompletionOptimizationProbe:       true,
		ConstellationCandidateCompletionOptimizationCandidateID: candidateID,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if !report.ConstellationCandidateCompletionOptimizationProbe || report.ConstellationCandidateCompletionOptimizationCandidateID != candidateID || !report.Runs[0].ConstellationCandidateCompletionOptimizationProbe || report.Runs[0].ConstellationCandidateCompletionOptimizationCandidateID != candidateID || !report.Runs[0].SolverSettings.ConstellationCandidateCompletionOptimizationProbe {
		t.Fatalf("candidate optimization report/run/settings=%+v/%+v/%+v", report, report.Runs[0], report.Runs[0].SolverSettings)
	}
}

func TestRunScenariosValidatesConstellationCandidateCompletionOptimization(t *testing.T) {
	base := RunConfig{
		CatalogPath: filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir: t.TempDir(),
		Budgets:     []int64{100},
		Repeat:      1,
		Workers:     1,
		Top:         1,
		ConstellationCandidateCompletionOptimizationProbe:       true,
		ConstellationCandidateCompletionOptimizationCandidateID: strings.Repeat("0", 64),
	}
	_, err := RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "requires diagnostic") {
		t.Fatalf("candidate optimization without diagnostics err=%v", err)
	}
	base.Diagnostic = true
	base.ConstellationSeedVariant = solver.ConstellationSeedVariantV3
	_, err = RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "requires constellation seed variant") {
		t.Fatalf("candidate optimization without V4 err=%v", err)
	}
	base.ConstellationSeedVariant = solver.ConstellationSeedVariantV4
	base.ConstellationCandidatePoolFeasibilitySweep = true
	_, err = RunScenarios(base)
	if err == nil || !strings.Contains(err.Error(), "cannot run with another constellation probe") {
		t.Fatalf("candidate optimization with sweep err=%v", err)
	}
}

func TestRunScenariosRetainsPlateauVariant(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)

	report, err := RunScenarios(RunConfig{
		CatalogPath:    filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:    dir,
		Budgets:        []int64{100},
		Repeat:         1,
		Workers:        1,
		Top:            1,
		PlateauVariant: solver.PlateauVariantLarge16,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if report.PlateauVariant != solver.PlateauVariantLarge16 || report.Runs[0].PlateauVariant != solver.PlateauVariantLarge16 {
		t.Fatalf("report plateau variants=%q/%q", report.PlateauVariant, report.Runs[0].PlateauVariant)
	}
}

func TestRunScenariosRepairSearchModeOverridesScenario(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "grid": ["111111111", "111111111", "111111111", "111111111", "111111111", "111111111"],
  "items": {"kiwi_dewdrop": 2},
  "repair_search": false
}`)

	reportOn, err := RunScenarios(RunConfig{
		CatalogPath:      filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:      dir,
		Budgets:          []int64{100},
		Repeat:           1,
		Workers:          1,
		Top:              1,
		RepairSearchMode: RepairSearchModeOn,
	})
	if err != nil {
		t.Fatalf("RunScenarios on returned error: %v", err)
	}
	if !reportOn.Runs[0].RepairSearch {
		t.Fatalf("repair_search=false want true with mode on")
	}

	reportOff, err := RunScenarios(RunConfig{
		CatalogPath:      filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:      dir,
		Budgets:          []int64{100},
		Repeat:           1,
		Workers:          1,
		Top:              1,
		RepairSearchMode: RepairSearchModeOff,
	})
	if err != nil {
		t.Fatalf("RunScenarios off returned error: %v", err)
	}
	if reportOff.Runs[0].RepairSearch {
		t.Fatalf("repair_search=true want false with mode off")
	}
}

func TestRunScenariosRepairSearchModeOnDoesNotRunForExhaustiveBudget(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "tiny.json", `{
  "name": "tiny",
  "grid": ["111111111", "111111111", "111111111", "111111111", "111111111", "111111111"],
  "items": {"kiwi_dewdrop": 2},
  "repair_search": true
}`)

	report, err := RunScenarios(RunConfig{
		CatalogPath:      filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir:      dir,
		Budgets:          []int64{0},
		Repeat:           1,
		Workers:          1,
		Top:              1,
		RepairSearchMode: RepairSearchModeOn,
	})
	if err != nil {
		t.Fatalf("RunScenarios returned error: %v", err)
	}
	if report.Runs[0].RepairSearch {
		t.Fatalf("repair_search=true want false for exhaustive budget")
	}
}

func TestValidateRepairSearchModeRejectsInvalidValue(t *testing.T) {
	if err := ValidateRepairSearchMode("maybe"); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestRunScenariosRejectsInvalidPlateauVariant(t *testing.T) {
	_, err := RunScenarios(RunConfig{PlateauVariant: "not-a-variant"})
	if err == nil || !strings.Contains(err.Error(), "plateau variant") {
		t.Fatalf("RunScenarios error=%v want plateau variant validation", err)
	}
}

func TestRunScenariosMissingItemReturnsClearError(t *testing.T) {
	dir := t.TempDir()
	writeScenario(t, dir, "missing.json", `{
  "name": "missing",
  "items": {"not_a_real_item": 1}
}`)

	_, err := RunScenarios(RunConfig{
		CatalogPath: filepath.Join("..", "..", "data", "catalog.json"),
		ScenarioDir: dir,
		Budgets:     []int64{10},
		Repeat:      1,
		Workers:     1,
		Top:         1,
	})
	if err == nil {
		t.Fatal("expected missing item error")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "not_a_real_item") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDiagnosticReferenceIsLimitedToOutgoingPerInstanceFood(t *testing.T) {
	reference := diagnosticReferenceForScenario(outgoingPerInstanceFoodScenarioName, true)
	if len(reference) != 24 {
		t.Fatalf("diagnostic reference placements=%d want 24", len(reference))
	}
	if reference := diagnosticReferenceForScenario(outgoingPerInstanceFoodScenarioName, false); reference != nil {
		t.Fatal("normal outgoing-per-instance-food benchmark should not receive a reference")
	}
	if reference := diagnosticReferenceForScenario("other", true); reference != nil {
		t.Fatal("unrelated diagnostic benchmark should not receive a reference")
	}
}

func TestCompareReportsClassifiesSelfAsTie(t *testing.T) {
	report := Report{Runs: []Run{
		{
			Scenario:  "tiny",
			Budget:    100,
			Repeat:    1,
			Score:     ScoreSummary{PriorityCounts: []int{1}, Crafts: 0, Stars: 2, Items: 2},
			LayoutKey: "a",
		},
	}}

	comparison, err := CompareReports(report, report)
	if err != nil {
		t.Fatalf("CompareReports returned error: %v", err)
	}
	if comparison.Wins != 0 || comparison.Losses != 0 || comparison.Ties != 1 {
		t.Fatalf("unexpected comparison counts: %+v", comparison)
	}
	if comparison.ScoreLosses != 0 {
		t.Fatalf("score losses=%d want 0", comparison.ScoreLosses)
	}
}

func TestCompareReportsDetectsPriorityRegression(t *testing.T) {
	baseline := Report{Runs: []Run{
		{
			Scenario:  "tiny",
			Budget:    100,
			Repeat:    1,
			Score:     ScoreSummary{PriorityCounts: []int{2}, Stars: 2, Items: 2},
			LayoutKey: "a",
		},
	}}
	current := Report{Runs: []Run{
		{
			Scenario:  "tiny",
			Budget:    100,
			Repeat:    1,
			Score:     ScoreSummary{PriorityCounts: []int{1}, Stars: 9, Items: 2},
			LayoutKey: "a",
		},
	}}

	comparison, err := CompareReports(baseline, current)
	if err != nil {
		t.Fatalf("CompareReports returned error: %v", err)
	}
	if comparison.Losses != 1 || comparison.ScoreLosses != 1 {
		t.Fatalf("unexpected comparison counts: %+v", comparison)
	}
}

func TestCompareReportsUsesExecutionFingerprintWhenAvailable(t *testing.T) {
	tests := []struct {
		name             string
		baselineSearch   SearchSummary
		currentSearch    SearchSummary
		wantErrorContain string
	}{
		{
			name: "matching execution fingerprints supersede legacy configuration",
			baselineSearch: SearchSummary{
				ConfigFingerprint:    "legacy-baseline",
				ExecutionFingerprint: "execution",
			},
			currentSearch: SearchSummary{
				ConfigFingerprint:    "legacy-current",
				ExecutionFingerprint: "execution",
			},
		},
		{
			name: "different execution fingerprints fail",
			baselineSearch: SearchSummary{
				ConfigFingerprint:    "legacy",
				ExecutionFingerprint: "execution-baseline",
			},
			currentSearch: SearchSummary{
				ConfigFingerprint:    "legacy",
				ExecutionFingerprint: "execution-current",
			},
			wantErrorContain: "execution fingerprints",
		},
		{
			name: "missing execution fingerprint falls back to legacy configuration",
			baselineSearch: SearchSummary{
				ConfigFingerprint:    "legacy-baseline",
				ExecutionFingerprint: "execution",
			},
			currentSearch: SearchSummary{
				ConfigFingerprint: "legacy-current",
			},
			wantErrorContain: "configuration fingerprints",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			baseline := Report{Runs: []Run{{Scenario: "tiny", Budget: 100, Repeat: 1, Search: test.baselineSearch}}}
			current := Report{Runs: []Run{{Scenario: "tiny", Budget: 100, Repeat: 1, Search: test.currentSearch}}}

			_, err := CompareReports(baseline, current)
			if test.wantErrorContain == "" {
				if err != nil {
					t.Fatalf("CompareReports returned error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErrorContain) {
				t.Fatalf("CompareReports error=%v want containing %q", err, test.wantErrorContain)
			}
		})
	}
}

func TestSearchSummarySerializesExecutionDiagnostics(t *testing.T) {
	search := model.SearchStats{
		ConfigFingerprint:    "legacy",
		ExecutionFingerprint: "execution",
		UnusedGlobalNodes:    25,
		PhaseWork: []model.SearchPhaseWork{{
			Phase:         "search",
			ChargedNodes:  75,
			Eligible:      true,
			Invoked:       true,
			NodesReserved: 100,
			NodesConsumed: 75,
		}},
		Stages: []model.SearchStageStats{{
			ID:                      "search",
			NodeLimit:               100,
			StageBudgetConsumed:     75,
			ExecutionBudgetConsumed: 75,
			PhaseWork: []model.SearchPhaseWork{{
				Phase:         "search",
				ChargedNodes:  75,
				Eligible:      true,
				Invoked:       true,
				NodesReserved: 100,
				NodesConsumed: 75,
			}},
		}},
		TaskAllocation: model.TaskAllocationStats{TasksGenerated: 4},
	}

	summary := searchSummary(search)
	if summary.ExecutionFingerprint != "execution" {
		t.Fatalf("execution_fingerprint=%q want execution", summary.ExecutionFingerprint)
	}
	if summary.Stages != nil || summary.TaskAllocation != nil {
		t.Fatal("non-diagnostic summary included diagnostic execution details")
	}
	assertSearchSummaryJSONFields(t, summary, "execution_fingerprint", "unused_global_nodes")
	assertSearchSummaryJSONMissingFields(t, summary, "phase_work", "stages", "task_allocation")

	search.DiagnosticsEnabled = true
	summary = searchSummary(search)
	if len(summary.Stages) != 1 || summary.Stages[0].ID != "search" {
		t.Fatalf("stages=%+v want search stage", summary.Stages)
	}
	if summary.TaskAllocation == nil || summary.TaskAllocation.TasksGenerated != 4 {
		t.Fatalf("task_allocation=%+v want generated=4", summary.TaskAllocation)
	}
	assertSearchSummaryJSONFields(t, summary, "execution_fingerprint", "phase_work", "stages", "task_allocation")
	payload := searchSummaryJSON(t, summary)
	assertSearchPhaseWorkJSONFields(t, payload["phase_work"])
	var stages []map[string]json.RawMessage
	if err := json.Unmarshal(payload["stages"], &stages); err != nil {
		t.Fatalf("unmarshal stages: %v", err)
	}
	if len(stages) != 1 {
		t.Fatalf("stages=%d want 1", len(stages))
	}
	for _, field := range []string{"stage_budget_consumed", "execution_budget_consumed"} {
		if _, ok := stages[0][field]; !ok {
			t.Errorf("stage missing JSON field %q", field)
		}
	}
	if phaseWork, ok := stages[0]["phase_work"]; !ok {
		t.Error("stage missing JSON field \"phase_work\"")
	} else {
		assertSearchPhaseWorkJSONFields(t, phaseWork)
	}
}

func TestSearchSummarySerializesConstellationSeed(t *testing.T) {
	search := model.SearchStats{
		ConstellationSeedNodes:       12,
		ConstellationSeedCandidates:  3,
		ConstellationSeedDiagnostics: model.ConstellationSeedDiagnostics{},
	}

	summary := searchSummary(search)
	if summary.ConstellationSeedNodes != 12 || summary.ConstellationSeedCandidates != 3 {
		t.Fatalf("constellation seed=%d/%d want 12/3", summary.ConstellationSeedNodes, summary.ConstellationSeedCandidates)
	}
	if summary.ConstellationSeedDiagnostics == nil {
		t.Fatal("constellation seed diagnostics missing")
	}
	assertSearchSummaryJSONFields(t, summary, "constellation_seed_nodes", "constellation_seed_candidates", "constellation_seed_diagnostics")

	summary = searchSummary(model.SearchStats{})
	assertSearchSummaryJSONMissingFields(t, summary, "constellation_seed_nodes", "constellation_seed_candidates", "constellation_seed_diagnostics")
}

func TestSearchSummarySerializesBoundOperationProfile(t *testing.T) {
	profile := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	profile.Outgoing.Search.Checks = 7
	summary := searchSummary(model.SearchStats{BoundOperationProfile: profile})
	if summary.BoundOperationProfile == nil || summary.BoundOperationProfile.Version != model.BoundAttributionProfileVersion || summary.BoundOperationProfile.Outgoing.Search.Checks != 7 {
		t.Fatalf("bound operation profile=%+v", summary.BoundOperationProfile)
	}
	assertSearchSummaryJSONFields(t, summary, "bound_operation_profile")
	assertSearchSummaryJSONMissingFields(t, searchSummary(model.SearchStats{}), "bound_operation_profile")
}

func assertSearchSummaryJSONFields(t *testing.T, summary SearchSummary, fields ...string) {
	t.Helper()
	payload := searchSummaryJSON(t, summary)
	for _, field := range fields {
		if _, ok := payload[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}
}

func assertSearchSummaryJSONMissingFields(t *testing.T, summary SearchSummary, fields ...string) {
	t.Helper()
	payload := searchSummaryJSON(t, summary)
	for _, field := range fields {
		if _, ok := payload[field]; ok {
			t.Errorf("unexpected JSON field %q", field)
		}
	}
}

func searchSummaryJSON(t *testing.T, summary SearchSummary) map[string]json.RawMessage {
	t.Helper()
	content, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal search summary: %v", err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal search summary: %v", err)
	}
	return payload
}

func assertJSONFieldEquals(t *testing.T, payload map[string]json.RawMessage, field string, want string) {
	t.Helper()
	if got, ok := payload[field]; !ok || string(got) != want {
		t.Errorf("%s=%s want %s", field, got, want)
	}
}

func assertSearchPhaseWorkJSONFields(t *testing.T, content json.RawMessage) {
	t.Helper()
	var phaseWork []map[string]json.RawMessage
	if err := json.Unmarshal(content, &phaseWork); err != nil {
		t.Fatalf("unmarshal phase work: %v", err)
	}
	if len(phaseWork) != 1 {
		t.Fatalf("phase work=%d want 1", len(phaseWork))
	}
	for _, field := range []string{"eligible", "invoked", "nodes_reserved", "nodes_consumed", "charged_nodes"} {
		if _, ok := phaseWork[0][field]; !ok {
			t.Errorf("phase work missing JSON field %q", field)
		}
	}
}

func writeScenario(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
}
