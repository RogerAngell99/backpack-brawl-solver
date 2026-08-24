package solver

import "backpack-brawl-solver/internal/model"

// SearchOperationProfileVersion identifies the deterministic rooted-packing
// counter contract serialized in benchmark reports.
const SearchOperationProfileVersion = "root-packing-ops-v1"

// PackingSeedFeasibilityProfileVersion identifies the deterministic
// packing-seed feasibility counter contract serialized in benchmark reports.
const PackingSeedFeasibilityProfileVersion = model.PackingSeedFeasibilityProfileVersionV2

type boundPriorityAttributionSite uint8

const (
	boundPriorityConstellationFilter boundPriorityAttributionSite = iota
	boundPriorityRepairDFS
	boundPriorityPlateauPrefilter
	boundPriorityPlateauDFS
)

type boundOutgoingAttributionSite uint8

const (
	boundOutgoingSearch boundOutgoingAttributionSite = iota
	boundOutgoingRepair
)

// OperationProfilingAvailable reports whether this binary contains the
// benchmark-only counter implementation.
func OperationProfilingAvailable() bool {
	return searchOperationProfilingAvailable
}

func mergePackingSeedFeasibilityOperationProfiles(total *model.PackingSeedFeasibilityOperationProfile, next *model.PackingSeedFeasibilityOperationProfile) *model.PackingSeedFeasibilityOperationProfile {
	if next == nil {
		return total
	}
	if total == nil {
		copy := *next
		return &copy
	}
	total.SearchCalls += next.SearchCalls
	total.StatesVisited += next.StatesVisited
	total.CandidateOptionChecks += next.CandidateOptionChecks
	total.CandidateOverlapRejects += next.CandidateOverlapRejects
	total.CandidateChargeAttempts += next.CandidateChargeAttempts
	total.CandidateChargeDenied += next.CandidateChargeDenied
	total.CandidateExpansions += next.CandidateExpansions
	total.FeasibilityCalls += next.FeasibilityCalls
	total.FeasibilityInstancesConsidered += next.FeasibilityInstancesConsidered
	total.FeasibilityOptionChecks += next.FeasibilityOptionChecks
	total.FeasibilityOverlapRejects += next.FeasibilityOverlapRejects
	total.FeasibilityLegalPlacements += next.FeasibilityLegalPlacements
	total.FeasibilityDeadReturns += next.FeasibilityDeadReturns
	addPackingSeedCanonicalCopyOrderOperationProfile(&total.CandidateCanonical, next.CandidateCanonical)
	addPackingSeedCanonicalCopyOrderOperationProfile(&total.FeasibilityCanonical, next.FeasibilityCanonical)
	return total
}

func addPackingSeedCanonicalCopyOrderOperationProfile(total *model.PackingSeedCanonicalCopyOrderOperationProfile, next model.PackingSeedCanonicalCopyOrderOperationProfile) {
	total.Calls += next.Calls
	total.Rejects += next.Rejects
	total.ExistingScanned += next.ExistingScanned
	total.SameItemComparisons += next.SameItemComparisons
	total.CandidatePlacementKeyCalls += next.CandidatePlacementKeyCalls
	total.PlacementKeyCalls += next.PlacementKeyCalls
	total.PlacementKeyBytes += next.PlacementKeyBytes
}

func mergeBoundAttributionOperationProfiles(total *model.BoundAttributionOperationProfile, next *model.BoundAttributionOperationProfile) *model.BoundAttributionOperationProfile {
	if next == nil {
		return total
	}
	if total == nil {
		copy := *next
		return &copy
	}
	if total.Version != next.Version {
		panic("cannot merge incompatible bound attribution profile versions")
	}
	total.PriorityUpper.ConstellationFilterInvocations += next.PriorityUpper.ConstellationFilterInvocations
	total.PriorityUpper.ConstellationStatesInput += next.PriorityUpper.ConstellationStatesInput
	total.PriorityUpper.ConstellationStatesRetained += next.PriorityUpper.ConstellationStatesRetained
	total.PriorityUpper.ConstellationStatesRejected += next.PriorityUpper.ConstellationStatesRejected
	addPriorityUpperBoundSiteProfile(&total.PriorityUpper.ConstellationFilter, next.PriorityUpper.ConstellationFilter)
	addPriorityUpperBoundSiteProfile(&total.PriorityUpper.RepairDFS, next.PriorityUpper.RepairDFS)
	addPriorityUpperBoundSiteProfile(&total.PriorityUpper.PlateauPrefilter, next.PriorityUpper.PlateauPrefilter)
	addPriorityUpperBoundSiteProfile(&total.PriorityUpper.PlateauDFS, next.PriorityUpper.PlateauDFS)
	addOutgoingBoundSiteProfile(&total.Outgoing.Search, next.Outgoing.Search)
	addOutgoingBoundSiteProfile(&total.Outgoing.Repair, next.Outgoing.Repair)
	return total
}

func addPriorityUpperBoundSiteProfile(total *model.PriorityUpperBoundSiteProfile, next model.PriorityUpperBoundSiteProfile) {
	total.Calls += next.Calls
	total.FeasibleResults += next.FeasibleResults
	total.RejectedResults += next.RejectedResults
	total.InvalidPriorityReturns += next.InvalidPriorityReturns
	total.PriorityEntriesValidated += next.PriorityEntriesValidated
	total.FixedPlacementInputs += next.FixedPlacementInputs
	total.CurrentPlacementInputs += next.CurrentPlacementInputs
	total.AnchoredPlacements += next.AnchoredPlacements
	total.RemovedInstanceInputs += next.RemovedInstanceInputs
	total.RemovedInstances += next.RemovedInstances
	total.RemovedOptionCandidates += next.RemovedOptionCandidates
	total.RemovedOptionRejectedFixedOverlap += next.RemovedOptionRejectedFixedOverlap
	total.RemovedOptionRejectedOutsideFree += next.RemovedOptionRejectedOutsideFree
	total.RemovedOptionsRetained += next.RemovedOptionsRetained
	total.UniquePrioritySourceItems += next.UniquePrioritySourceItems
	total.AnchoredSourceInstances += next.AnchoredSourceInstances
	total.RemovedSourceInstances += next.RemovedSourceInstances
	total.StarSlots += next.StarSlots
	total.FixedTargetChecks += next.FixedTargetChecks
	total.RemovedTargetChecks += next.RemovedTargetChecks
	total.SelfTargetSkips += next.SelfTargetSkips
	total.FixedFixedGeometryChecks += next.FixedFixedGeometryChecks
	total.RemovedSourceOptionChecksFixedTarget += next.RemovedSourceOptionChecksFixedTarget
	total.FixedSourceTargetOptionChecks += next.FixedSourceTargetOptionChecks
	total.RemovedSourceTargetOptionPairs += next.RemovedSourceTargetOptionPairs
	total.GeometryCandidateChecks += next.GeometryCandidateChecks
	total.GeometryOverlapRejects += next.GeometryOverlapRejects
	total.StarPositionHitCalls += next.StarPositionHitCalls
	total.StarPositionHitTrue += next.StarPositionHitTrue
	total.SlotTargetHits += next.SlotTargetHits
	total.MatchingCalls += next.MatchingCalls
}

func addOutgoingBoundSiteProfile(total *model.OutgoingBoundSiteProfile, next model.OutgoingBoundSiteProfile) {
	total.Checks += next.Checks
	total.PrunedNodes += next.PrunedNodes
	total.PlacedMapBuilds += next.PlacedMapBuilds
	total.PlacedMapInsertions += next.PlacedMapInsertions
	total.PlacedMaskInstanceChecks += next.PlacedMaskInstanceChecks
	total.PriorityIterations += next.PriorityIterations
	total.SourceInstanceIterations += next.SourceInstanceIterations
	total.PrioritySourceMatches += next.PrioritySourceMatches
	total.ZeroStarSourceSkips += next.ZeroStarSourceSkips
	total.PlacedSourceIterations += next.PlacedSourceIterations
	total.FreeSourceIterations += next.FreeSourceIterations
	total.PlacedSourceTargetIterations += next.PlacedSourceTargetIterations
	total.SelfTargetSkips += next.SelfTargetSkips
	total.TargetPlacementLookups += next.TargetPlacementLookups
	total.PlacedTargetsFound += next.PlacedTargetsFound
	total.UnplacedTargets += next.UnplacedTargets
	total.SourceHitsTargetCalls += next.SourceHitsTargetCalls
	total.SourceHitsTargetTrue += next.SourceHitsTargetTrue
	total.CoveragePlacementKeyCalls += next.CoveragePlacementKeyCalls
	total.PlacedPotentialLookups += next.PlacedPotentialLookups
	total.FreePotentialLookups += next.FreePotentialLookups
	total.PopcountCalls += next.PopcountCalls
	total.StarCountClamps += next.StarCountClamps
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
