package solver

import (
	"testing"
	"time"

	"backpack-brawl-solver/internal/model"
)

func TestDiagnosticTraceRecordsStageAndExecutionBudgetScopes(t *testing.T) {
	stages := []SearchStage{{ID: "prefix", NodeLimit: 10}, {ID: "remainder", NodeLimit: 10}}
	ledger := newNodeLedger(20, stages)
	for index := 0; index < 7; index++ {
		if !ledger.charge("prefix") {
			t.Fatal("prefix charge rejected")
		}
	}
	trace := newDiagnosticTrace(time.Now(), 10, ledger, "remainder", nil, nil, nil)
	if !ledger.charge("remainder") {
		t.Fatal("remainder charge rejected")
	}
	trace.addCharged(tracePhaseDFS, 1)
	trace.observeCandidate(tracePhaseDFS, nil, nil, model.Score{}, true, nil)
	var stats model.SearchStats
	trace.apply(&stats)
	if len(stats.IncumbentTrace) != 1 {
		t.Fatalf("events=%d want 1", len(stats.IncumbentTrace))
	}
	event := stats.IncumbentTrace[0]
	if event.GlobalBudgetConsumed != 1 || event.StageBudgetConsumed != 1 || event.ExecutionBudgetConsumed != 8 {
		t.Fatalf("budget scopes=%+v want legacy/stage=1 execution=8", event)
	}
}

func TestDiagnosticTraceSingleStageBudgetScopesMatch(t *testing.T) {
	ledger := newNodeLedger(10, []SearchStage{{ID: "single", NodeLimit: 10}})
	trace := newDiagnosticTrace(time.Now(), 10, ledger, "single", nil, nil, nil)
	if !ledger.charge("single") {
		t.Fatal("single-stage charge rejected")
	}
	trace.addCharged(tracePhaseDFS, 1)
	trace.observeCandidate(tracePhaseDFS, nil, nil, model.Score{}, true, nil)
	var stats model.SearchStats
	trace.apply(&stats)
	if len(stats.IncumbentTrace) != 1 {
		t.Fatalf("events=%d want 1", len(stats.IncumbentTrace))
	}
	event := stats.IncumbentTrace[0]
	if event.GlobalBudgetConsumed != event.StageBudgetConsumed || event.StageBudgetConsumed != event.ExecutionBudgetConsumed {
		t.Fatalf("single-stage scopes diverged: %+v", event)
	}
}

func TestNodeLedgerSeparatesDiagnosticBudgetLane(t *testing.T) {
	ledger := newNodeLedger(2, []SearchStage{{ID: "single", NodeLimit: 2}}, 3)
	for index := 0; index < 2; index++ {
		if ok, reason := ledger.chargeWithReason("single"); !ok || reason != "" {
			t.Fatalf("normal charge %d rejected: %q", index, reason)
		}
	}
	if ok, reason := ledger.chargeWithReason("single"); ok || reason != ledgerStopSourceGlobal {
		t.Fatalf("normal lane did not stop at cap: ok=%t reason=%q", ok, reason)
	}
	for index := 0; index < 3; index++ {
		if ok, reason := ledger.chargeDiagnosticWithReason("single"); !ok || reason != "" {
			t.Fatalf("diagnostic charge %d rejected: %q", index, reason)
		}
	}
	if ok, reason := ledger.chargeDiagnosticWithReason("single"); ok || reason != ledgerStopSourceDiagnostic {
		t.Fatalf("diagnostic lane did not stop at cap: ok=%t reason=%q", ok, reason)
	}
	snapshot := ledger.snapshot("single")
	if snapshot.StageCharged != 2 || snapshot.ExecutionCharged != 2 || snapshot.DiagnosticStageCharged != 3 || snapshot.DiagnosticExecutionCharged != 3 {
		t.Fatalf("ledger snapshot=%+v", snapshot)
	}
}

func TestDiagnosticPhasePlanDistinguishesSkippedFromEligibleZeroWork(t *testing.T) {
	ledger := newNodeLedger(100, []SearchStage{{ID: "single", NodeLimit: 100}})
	trace := newDiagnosticTrace(time.Now(), 100, ledger, "single", nil, nil, nil)
	trace.planPhase(tracePhaseConstellationSeed, false, "disabled", 0)
	trace.planPhase(tracePhasePreRepair, true, "", 40)
	trace.invokePhase(tracePhasePreRepair)
	trace.finishPhase(tracePhasePreRepair, "no_eligible_neighborhoods", 40, "dfs")
	if !ledger.charge("single") {
		t.Fatal("charge rejected")
	}
	trace.addCharged(tracePhaseDFS, 1)
	var stats model.SearchStats
	trace.apply(&stats)
	byPhase := map[string]model.SearchPhaseWork{}
	for _, phase := range stats.PhaseWork {
		byPhase[phase.Phase] = phase
	}
	constellation := byPhase[tracePhaseConstellationSeed]
	if constellation.Eligible || constellation.Invoked || constellation.SkipReason != "disabled" {
		t.Fatalf("disabled phase=%+v", constellation)
	}
	repair := byPhase[tracePhasePreRepair]
	if !repair.Eligible || !repair.Invoked || repair.SkipReason != "" || repair.TerminationReason != "no_eligible_neighborhoods" || repair.NodesReserved != 40 || repair.NodesConsumed != 0 || repair.NodesReturned != 40 || repair.ReturnTarget != "dfs" {
		t.Fatalf("eligible zero-work repair=%+v", repair)
	}
	if stats.UnusedGlobalNodes != 99 {
		t.Fatalf("unused=%d want 99", stats.UnusedGlobalNodes)
	}
}
