//go:build searchprofile

package solver

import "backpack-brawl-solver/internal/model"

const searchOperationProfilingAvailable = true

// rootPackingOperationCounters is deliberately session-local. Rooted packing
// sessions execute on one scheduler worker, so deterministic integer updates
// require neither atomics nor a mutex.
type rootPackingOperationCounters struct {
	profile model.ConstellationRootPackingOperationProfile
}

func newRootPackingOperationCounters(config Config) *rootPackingOperationCounters {
	if !config.OperationProfiling {
		return nil
	}
	return &rootPackingOperationCounters{profile: model.ConstellationRootPackingOperationProfile{Version: SearchOperationProfileVersion}}
}

func (c *rootPackingOperationCounters) clone() *rootPackingOperationCounters {
	if c == nil {
		return nil
	}
	copy := *c
	return &copy
}

func (c *rootPackingOperationCounters) snapshot() *model.ConstellationRootPackingOperationProfile {
	if c == nil {
		return nil
	}
	copy := c.profile
	return &copy
}

func (c *rootPackingOperationCounters) sessionStarted() {
	if c != nil {
		c.profile.SessionsStarted++
	}
}
func (c *rootPackingOperationCounters) runCall() {
	if c != nil {
		c.profile.RunCalls++
	}
}
func (c *rootPackingOperationCounters) pauseReturn() {
	if c != nil {
		c.profile.PauseReturns++
	}
}
func (c *rootPackingOperationCounters) depthStarted() {
	if c != nil {
		c.profile.DepthsStarted++
	}
}
func (c *rootPackingOperationCounters) statePrepared() {
	if c != nil {
		c.profile.StatesPrepared++
	}
}
func (c *rootPackingOperationCounters) areaPrune() {
	if c != nil {
		c.profile.AreaPrunes++
	}
}
func (c *rootPackingOperationCounters) mrvSelection() {
	if c != nil {
		c.profile.MRVSelectionCalls++
	}
}
func (c *rootPackingOperationCounters) mrvInstance() {
	if c != nil {
		c.profile.MRVInstancesConsidered++
	}
}
func (c *rootPackingOperationCounters) mrvOption() {
	if c != nil {
		c.profile.MRVOptionChecks++
	}
}
func (c *rootPackingOperationCounters) mrvLegalPlacement() {
	if c != nil {
		c.profile.MRVLegalPlacements++
	}
}
func (c *rootPackingOperationCounters) ledgerChargeAttempt() {
	if c != nil {
		c.profile.LedgerChargeAttempts++
	}
}
func (c *rootPackingOperationCounters) ledgerChargeDenied() {
	if c != nil {
		c.profile.LedgerChargeDenied++
	}
}
func (c *rootPackingOperationCounters) candidateExpansion() {
	if c != nil {
		c.profile.CandidateExpansions++
	}
}
func (c *rootPackingOperationCounters) completeCandidate() {
	if c != nil {
		c.profile.CompleteCandidates++
	}
}
func (c *rootPackingOperationCounters) placementCopy(n int) {
	if c != nil {
		c.profile.PlacementCopyCalls++
		c.profile.PlacementElementsCopied += int64(n)
	}
}
func (c *rootPackingOperationCounters) feasibilityCall() {
	if c != nil {
		c.profile.FeasibilityCalls++
	}
}
func (c *rootPackingOperationCounters) feasibilityInstance() {
	if c != nil {
		c.profile.FeasibilityInstancesConsidered++
	}
}
func (c *rootPackingOperationCounters) feasibilityOption() {
	if c != nil {
		c.profile.FeasibilityOptionChecks++
	}
}
func (c *rootPackingOperationCounters) fragmentationEvaluation() {
	if c != nil {
		c.profile.FragmentationEvaluations++
	}
}
func (c *rootPackingOperationCounters) partialScoreEvaluation() {
	if c != nil {
		c.profile.PartialScoreEvaluations++
	}
}
func (c *rootPackingOperationCounters) stateKey(bytes int) {
	if c != nil {
		c.profile.StateKeyConstructions++
		c.profile.StateKeyBytes += int64(bytes)
	}
}
func (c *rootPackingOperationCounters) dedupLookup() {
	if c != nil {
		c.profile.DedupLookups++
	}
}
func (c *rootPackingOperationCounters) dedupHit() {
	if c != nil {
		c.profile.DedupHits++
	}
}
func (c *rootPackingOperationCounters) dedupReplacement() {
	if c != nil {
		c.profile.DedupReplacements++
	}
}
func (c *rootPackingOperationCounters) depthFinish(precut int) {
	if c != nil {
		c.profile.DepthFinishCalls++
		c.profile.PrecutStates += int64(precut)
	}
}
func (c *rootPackingOperationCounters) statesSorted(count int) {
	if c != nil {
		c.profile.StatesSorted += int64(count)
	}
}
func (c *rootPackingOperationCounters) comparatorCall() {
	if c != nil {
		c.profile.ComparatorCalls++
	}
}
