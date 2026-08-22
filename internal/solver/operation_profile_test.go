//go:build searchprofile

package solver

import (
	"reflect"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestOperationProfilingPreservesRootPackingSearchResult(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	withoutProfile := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true }).Run(20)
	profiledConfig := config
	profiledConfig.OperationProfiling = true
	withProfile := newConstellationRootPackingSession(catalog, instances, options, root, profiledConfig, gridMask, func(bool) bool { return true }).Run(20)
	if !reflect.DeepEqual(rootPackingResultWithoutOperationProfile(withProfile), withoutProfile) {
		t.Fatalf("operation profiling changed search result: off=%+v on=%+v", withoutProfile, withProfile)
	}
	profile := withProfile.operationProfile
	if profile == nil || profile.Version != SearchOperationProfileVersion {
		t.Fatalf("operation profile=%+v", profile)
	}
	if profile.CandidateExpansions != withProfile.nodes || profile.LedgerChargeAttempts != withProfile.nodes {
		t.Fatalf("node accounting result=%+v profile=%+v", withProfile, profile)
	}
	if profile.MRVSelectionCalls == 0 || profile.FeasibilityCalls == 0 || profile.StateKeyConstructions == 0 || profile.DepthFinishCalls == 0 {
		t.Fatalf("expected rooted-packing counters, got %+v", profile)
	}
}

func TestOperationProfilingResumableWorkCountersMatchPartitionedExecution(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	config.OperationProfiling = true
	monolithic := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
	monolithicResult := monolithic.Run(20)
	partitioned := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
	partitioned.Run(1)
	partitionedResult := partitioned.Run(19)
	if !reflect.DeepEqual(rootPackingResultWithoutOperationProfile(partitionedResult), rootPackingResultWithoutOperationProfile(monolithicResult)) {
		t.Fatalf("partition changed result: monolithic=%+v partitioned=%+v", monolithicResult, partitionedResult)
	}
	if !reflect.DeepEqual(operationProfileWithoutLifecycle(partitionedResult.operationProfile), operationProfileWithoutLifecycle(monolithicResult.operationProfile)) {
		t.Fatalf("partition changed operation work: monolithic=%+v partitioned=%+v", monolithicResult.operationProfile, partitionedResult.operationProfile)
	}
	if partitionedResult.operationProfile.RunCalls <= monolithicResult.operationProfile.RunCalls || partitionedResult.operationProfile.PauseReturns <= monolithicResult.operationProfile.PauseReturns {
		t.Fatalf("lifecycle counters did not record resumed calls: monolithic=%+v partitioned=%+v", monolithicResult.operationProfile, partitionedResult.operationProfile)
	}
}

func TestOperationProfileTerminalProjectionDoesNotMutateResumableState(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	config.OperationProfiling = true
	session := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
	paused := session.Run(1)
	if session.Done() || paused.operationProfile == nil {
		t.Fatalf("expected a profiled resumable pause: session=%+v result=%+v", session, paused)
	}
	beforeProfile := session.operations.snapshot()
	beforeCheckpoint := constellationRootPackingSessionCheckpointForTest(session)
	projected := session.terminalProjection()
	if projected.operationProfile == nil {
		t.Fatal("terminal projection did not include profile snapshot")
	}
	if !reflect.DeepEqual(beforeProfile, session.operations.snapshot()) || !reflect.DeepEqual(beforeCheckpoint, constellationRootPackingSessionCheckpointForTest(session)) {
		t.Fatalf("terminal projection mutated resumable state: before=%+v after=%+v", beforeCheckpoint, constellationRootPackingSessionCheckpointForTest(session))
	}
}

func TestRootPackingOperationProfilesAggregateWithoutDoubleCounting(t *testing.T) {
	profiles := []model.ConstellationRootDiagnostic{
		{OperationProfile: &model.ConstellationRootPackingOperationProfile{Version: SearchOperationProfileVersion, CandidateExpansions: 3, MRVOptionChecks: 7}},
		{OperationProfile: &model.ConstellationRootPackingOperationProfile{Version: SearchOperationProfileVersion, CandidateExpansions: 5, MRVOptionChecks: 11}},
	}
	aggregate := aggregateRootPackingOperationProfiles(profiles)
	if aggregate == nil || aggregate.CandidateExpansions != 8 || aggregate.MRVOptionChecks != 18 {
		t.Fatalf("aggregate=%+v", aggregate)
	}
}

func rootPackingResultWithoutOperationProfile(result constellationRootPackingResult) constellationRootPackingResult {
	result.operationProfile = nil
	return result
}

func operationProfileWithoutLifecycle(profile *model.ConstellationRootPackingOperationProfile) *model.ConstellationRootPackingOperationProfile {
	if profile == nil {
		return nil
	}
	copy := *profile
	copy.RunCalls = 0
	copy.PauseReturns = 0
	return &copy
}
