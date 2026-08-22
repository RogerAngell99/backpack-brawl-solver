package solver

import "sync"

// nodeLedger is the single authority for charged work. Stage-local limits are
// checked while holding the same lock as the global limit, so parallel tasks
// cannot overspend either budget.
type nodeLedger struct {
	mu                     sync.Mutex
	maxNodes               int64
	charged                int64
	diagnosticMax          int64
	diagnosticCharged      int64
	stageLimits            map[string]int64
	stageCharged           map[string]int64
	diagnosticStageCharged map[string]int64
}

type ledgerSnapshot struct {
	StageCharged               int64
	ExecutionCharged           int64
	DiagnosticStageCharged     int64
	DiagnosticExecutionCharged int64
}

const (
	ledgerStopSourceGlobal     = "global_ledger"
	ledgerStopSourceStage      = "stage_ledger"
	ledgerStopSourceDiagnostic = "diagnostic_ledger"
)

func newNodeLedger(maxNodes int64, stages []SearchStage, diagnosticMax ...int64) *nodeLedger {
	configuredDiagnosticMax := int64(0)
	if len(diagnosticMax) > 0 {
		configuredDiagnosticMax = diagnosticMax[0]
	}
	ledger := &nodeLedger{
		maxNodes:               maxNodes,
		diagnosticMax:          configuredDiagnosticMax,
		stageLimits:            make(map[string]int64, len(stages)),
		stageCharged:           make(map[string]int64, len(stages)),
		diagnosticStageCharged: make(map[string]int64, len(stages)),
	}
	for _, stage := range stages {
		ledger.stageLimits[stage.ID] = stage.NodeLimit
	}
	return ledger
}

func (ledger *nodeLedger) chargeDiagnosticWithReason(stageID string) (bool, string) {
	if ledger == nil {
		return true, ""
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.diagnosticMax <= 0 || ledger.diagnosticCharged >= ledger.diagnosticMax {
		return false, ledgerStopSourceDiagnostic
	}
	ledger.diagnosticCharged++
	ledger.diagnosticStageCharged[stageID]++
	return true, ""
}

func (ledger *nodeLedger) configureDiagnosticMax(maxNodes int64) bool {
	if ledger == nil || maxNodes < 0 {
		return false
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.diagnosticCharged != 0 || ledger.diagnosticMax != 0 {
		return false
	}
	ledger.diagnosticMax = maxNodes
	return true
}

func (ledger *nodeLedger) diagnosticMaxValue() int64 {
	if ledger == nil {
		return 0
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.diagnosticMax
}

func (ledger *nodeLedger) charge(stageID string) bool {
	ok, _ := ledger.chargeWithReason(stageID)
	return ok
}

func (ledger *nodeLedger) chargeWithReason(stageID string) (bool, string) {
	if ledger == nil {
		return true, ""
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	if ledger.maxNodes > 0 && ledger.charged >= ledger.maxNodes {
		return false, ledgerStopSourceGlobal
	}
	if limit := ledger.stageLimits[stageID]; limit > 0 && ledger.stageCharged[stageID] >= limit {
		return false, ledgerStopSourceStage
	}
	ledger.charged++
	ledger.stageCharged[stageID]++
	return true, ""
}

func (ledger *nodeLedger) total() int64 {
	if ledger == nil {
		return 0
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.charged
}

func (ledger *nodeLedger) diagnosticTotal() int64 {
	if ledger == nil {
		return 0
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.diagnosticCharged
}

func (ledger *nodeLedger) stageTotal(stageID string) int64 {
	if ledger == nil {
		return 0
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledger.stageCharged[stageID]
}

func (ledger *nodeLedger) snapshot(stageID string) ledgerSnapshot {
	if ledger == nil {
		return ledgerSnapshot{}
	}
	ledger.mu.Lock()
	defer ledger.mu.Unlock()
	return ledgerSnapshot{
		StageCharged:               ledger.stageCharged[stageID],
		ExecutionCharged:           ledger.charged,
		DiagnosticStageCharged:     ledger.diagnosticStageCharged[stageID],
		DiagnosticExecutionCharged: ledger.diagnosticCharged,
	}
}

func chargeDiagnosticNodeWithReason(config Config) (bool, string) {
	if config.ledger == nil {
		return false, ledgerStopSourceDiagnostic
	}
	return config.ledger.chargeDiagnosticWithReason(config.stageID)
}

func chargeNode(config Config, phase string) bool {
	ok, _ := chargeNodeWithReason(config, phase)
	return ok
}

func chargeNodeWithReason(config Config, phase string) (bool, string) {
	if config.ledger != nil {
		ok, reason := config.ledger.chargeWithReason(config.stageID)
		if !ok {
			return false, reason
		}
	}
	config.trace.addCharged(phase, 1)
	return true, ""
}
