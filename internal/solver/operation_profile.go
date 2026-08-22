package solver

import "backpack-brawl-solver/internal/model"

// SearchOperationProfileVersion identifies the deterministic rooted-packing
// counter contract serialized in benchmark reports.
const SearchOperationProfileVersion = "root-packing-ops-v1"

// OperationProfilingAvailable reports whether this binary contains the
// benchmark-only counter implementation.
func OperationProfilingAvailable() bool {
	return searchOperationProfilingAvailable
}

func aggregateRootPackingOperationProfiles(roots []model.ConstellationRootDiagnostic) *model.ConstellationRootPackingOperationProfile {
	var aggregate *model.ConstellationRootPackingOperationProfile
	for _, root := range roots {
		if root.OperationProfile == nil {
			continue
		}
		if aggregate == nil {
			copy := *root.OperationProfile
			aggregate = &copy
			continue
		}
		addRootPackingOperationProfile(aggregate, root.OperationProfile)
	}
	return aggregate
}

func addRootPackingOperationProfile(total *model.ConstellationRootPackingOperationProfile, next *model.ConstellationRootPackingOperationProfile) {
	if total == nil || next == nil {
		return
	}
	total.SessionsStarted += next.SessionsStarted
	total.RunCalls += next.RunCalls
	total.PauseReturns += next.PauseReturns
	total.DepthsStarted += next.DepthsStarted
	total.StatesPrepared += next.StatesPrepared
	total.AreaPrunes += next.AreaPrunes
	total.MRVSelectionCalls += next.MRVSelectionCalls
	total.MRVInstancesConsidered += next.MRVInstancesConsidered
	total.MRVOptionChecks += next.MRVOptionChecks
	total.MRVLegalPlacements += next.MRVLegalPlacements
	total.LedgerChargeAttempts += next.LedgerChargeAttempts
	total.LedgerChargeDenied += next.LedgerChargeDenied
	total.CandidateExpansions += next.CandidateExpansions
	total.CompleteCandidates += next.CompleteCandidates
	total.PlacementCopyCalls += next.PlacementCopyCalls
	total.PlacementElementsCopied += next.PlacementElementsCopied
	total.FeasibilityCalls += next.FeasibilityCalls
	total.FeasibilityInstancesConsidered += next.FeasibilityInstancesConsidered
	total.FeasibilityOptionChecks += next.FeasibilityOptionChecks
	total.FragmentationEvaluations += next.FragmentationEvaluations
	total.PartialScoreEvaluations += next.PartialScoreEvaluations
	total.StateKeyConstructions += next.StateKeyConstructions
	total.StateKeyBytes += next.StateKeyBytes
	total.DedupLookups += next.DedupLookups
	total.DedupHits += next.DedupHits
	total.DedupReplacements += next.DedupReplacements
	total.DepthFinishCalls += next.DepthFinishCalls
	total.PrecutStates += next.PrecutStates
	total.StatesSorted += next.StatesSorted
}
