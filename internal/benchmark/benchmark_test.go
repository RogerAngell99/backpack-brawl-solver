package benchmark

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	for _, run := range report.Runs {
		if run.Scenario != "tiny" {
			t.Fatalf("scenario=%q want tiny", run.Scenario)
		}
		if run.RepairSearch {
			t.Fatalf("repair search should follow scenario false by default")
		}
		if run.Error != "" {
			t.Fatalf("unexpected run error: %s", run.Error)
		}
		if len(run.Solution) == 0 {
			t.Fatalf("solution JSON should be present")
		}
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

func writeScenario(t *testing.T, dir string, name string, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
}
