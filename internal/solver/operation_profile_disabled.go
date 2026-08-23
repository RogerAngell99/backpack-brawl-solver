//go:build !searchprofile

package solver

import "backpack-brawl-solver/internal/model"

const searchOperationProfilingAvailable = false

// The normal build intentionally reduces every hook to an inlineable no-op so
// CPU and allocation profiles measure the production search path.
type rootPackingOperationCounters struct{}

func newRootPackingOperationCounters(Config) *rootPackingOperationCounters { return nil }
func (*rootPackingOperationCounters) clone() *rootPackingOperationCounters { return nil }
func (*rootPackingOperationCounters) snapshot() *model.ConstellationRootPackingOperationProfile {
	return nil
}
func (*rootPackingOperationCounters) sessionStarted()          {}
func (*rootPackingOperationCounters) runCall()                 {}
func (*rootPackingOperationCounters) pauseReturn()             {}
func (*rootPackingOperationCounters) depthStarted()            {}
func (*rootPackingOperationCounters) statePrepared()           {}
func (*rootPackingOperationCounters) areaPrune()               {}
func (*rootPackingOperationCounters) mrvSelection()            {}
func (*rootPackingOperationCounters) mrvInstance()             {}
func (*rootPackingOperationCounters) mrvOption()               {}
func (*rootPackingOperationCounters) mrvLegalPlacement()       {}
func (*rootPackingOperationCounters) ledgerChargeAttempt()     {}
func (*rootPackingOperationCounters) ledgerChargeDenied()      {}
func (*rootPackingOperationCounters) candidateExpansion()      {}
func (*rootPackingOperationCounters) completeCandidate()       {}
func (*rootPackingOperationCounters) placementCopy(int)        {}
func (*rootPackingOperationCounters) feasibilityCall()         {}
func (*rootPackingOperationCounters) feasibilityInstance()     {}
func (*rootPackingOperationCounters) feasibilityOption()       {}
func (*rootPackingOperationCounters) fragmentationEvaluation() {}
func (*rootPackingOperationCounters) partialScoreEvaluation()  {}
func (*rootPackingOperationCounters) stateKey(int)             {}
func (*rootPackingOperationCounters) dedupLookup()             {}
func (*rootPackingOperationCounters) dedupHit()                {}
func (*rootPackingOperationCounters) dedupReplacement()        {}
func (*rootPackingOperationCounters) depthFinish(int)          {}
func (*rootPackingOperationCounters) statesSorted(int)         {}

type packingSeedFeasibilityOperationCounters struct{}

func newPackingSeedFeasibilityOperationCounters(Config) *packingSeedFeasibilityOperationCounters {
	return nil
}
func (*packingSeedFeasibilityOperationCounters) snapshot() *model.PackingSeedFeasibilityOperationProfile {
	return nil
}
func (*packingSeedFeasibilityOperationCounters) searchCall()             {}
func (*packingSeedFeasibilityOperationCounters) stateVisited()           {}
func (*packingSeedFeasibilityOperationCounters) candidateOption()        {}
func (*packingSeedFeasibilityOperationCounters) candidateOverlapReject() {}
func (*packingSeedFeasibilityOperationCounters) candidateChargeAttempt() {}
func (*packingSeedFeasibilityOperationCounters) candidateChargeDenied()  {}
func (*packingSeedFeasibilityOperationCounters) candidateExpansion()     {}
func (*packingSeedFeasibilityOperationCounters) candidateCanonical(placement model.Placement, existing []model.Placement) bool {
	return placementRespectsCanonicalCopyOrder(placement, existing)
}
func packingSeedFeasibilityProfiled(remaining []model.InventoryInstance, optionsByInstance map[string][]model.Placement, occupied uint64, placements []model.Placement, _ *packingSeedFeasibilityOperationCounters) (int, int, bool) {
	return packingFeasibility(remaining, optionsByInstance, occupied, placements)
}
func placementRespectsCanonicalCopyOrderProfiled(placement model.Placement, existing []model.Placement, _ *model.PackingSeedCanonicalCopyOrderOperationProfile) bool {
	return placementRespectsCanonicalCopyOrder(placement, existing)
}
