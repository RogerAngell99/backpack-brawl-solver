package render

import (
	"encoding/json"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestSolutionsJSONSerializesExecutionDiagnostics(t *testing.T) {
	solution := model.Solution{Search: model.SearchStats{
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
			IncumbentTrace: []model.IncumbentEvent{{
				StageBudgetConsumed:     20,
				ExecutionBudgetConsumed: 20,
			}},
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
	}}

	search := renderedSearch(t, solution)
	assertJSONSearchFields(t, search, "execution_fingerprint", "unused_global_nodes")
	assertJSONSearchMissingFields(t, search, "config_fingerprint", "phase_work", "stages", "task_allocation")

	solution.Search.DiagnosticsEnabled = true
	search = renderedSearch(t, solution)
	assertJSONSearchFields(t, search, "config_fingerprint", "execution_fingerprint", "phase_work", "stages", "task_allocation")
	assertPhaseWorkJSONFields(t, search["phase_work"])
	var stages []map[string]json.RawMessage
	if err := json.Unmarshal(search["stages"], &stages); err != nil {
		t.Fatalf("unmarshal stages: %v", err)
	}
	if len(stages) != 1 {
		t.Fatalf("stages=%d want 1", len(stages))
	}
	for _, field := range []string{"stage_budget_consumed", "execution_budget_consumed", "incumbent_trace"} {
		if _, ok := stages[0][field]; !ok {
			t.Errorf("stage missing JSON field %q", field)
		}
	}
	if phaseWork, ok := stages[0]["phase_work"]; !ok {
		t.Error("stage missing JSON field \"phase_work\"")
	} else {
		assertPhaseWorkJSONFields(t, phaseWork)
	}
}

func TestSolutionsJSONSerializesConstellationSeed(t *testing.T) {
	solution := model.Solution{Search: model.SearchStats{
		ConstellationSeedNodes:       12,
		ConstellationSeedCandidates:  3,
		ConstellationSeedDiagnostics: model.ConstellationSeedDiagnostics{},
	}}

	search := renderedSearch(t, solution)
	assertJSONSearchFields(t, search, "constellation_seed_nodes", "constellation_seed_candidates", "constellation_seed_diagnostics")
	var nodes int64
	if err := json.Unmarshal(search["constellation_seed_nodes"], &nodes); err != nil {
		t.Fatalf("unmarshal constellation seed nodes: %v", err)
	}
	if nodes != 12 {
		t.Fatalf("constellation_seed_nodes=%d want 12", nodes)
	}

	search = renderedSearch(t, model.Solution{})
	assertJSONSearchMissingFields(t, search, "constellation_seed_nodes", "constellation_seed_candidates", "constellation_seed_diagnostics")
}

func renderedSearch(t *testing.T, solution model.Solution) map[string]json.RawMessage {
	t.Helper()
	content, err := SolutionsJSON([]model.Solution{solution})
	if err != nil {
		t.Fatalf("render solution JSON: %v", err)
	}
	var payload []struct {
		Search map[string]json.RawMessage `json:"search"`
	}
	if err := json.Unmarshal(content, &payload); err != nil {
		t.Fatalf("unmarshal rendered solution: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("rendered solutions=%d want 1", len(payload))
	}
	return payload[0].Search
}

func assertJSONSearchFields(t *testing.T, search map[string]json.RawMessage, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := search[field]; !ok {
			t.Errorf("missing JSON field %q", field)
		}
	}
}

func assertJSONSearchMissingFields(t *testing.T, search map[string]json.RawMessage, fields ...string) {
	t.Helper()
	for _, field := range fields {
		if _, ok := search[field]; ok {
			t.Errorf("unexpected JSON field %q", field)
		}
	}
}

func assertPhaseWorkJSONFields(t *testing.T, content json.RawMessage) {
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
