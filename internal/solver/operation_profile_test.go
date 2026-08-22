//go:build searchprofile

package solver

import (
	"reflect"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/geometry"
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

func TestOperationProfilingPreservesFullGeneralSearchPipeline(t *testing.T) {
	catalog := coverageCeilingTestCatalog()
	items := []string{"left_source", "right_source", "weapon", "weapon", "weapon", "weapon"}
	config := Config{
		TopN:                     1,
		AllowSkips:               false,
		MaxNodes:                 200_000,
		Workers:                  1,
		Diagnostics:              true,
		RepairSearch:             false,
		ConstellationSeedVariant: ConstellationSeedVariantGeneralSearchV1,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:left_source", "star_source:right_source"},
	}
	withoutProfile, err := SolveLayout(catalog, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("profile off SolveLayout: %v", err)
	}
	profiledConfig := config
	profiledConfig.OperationProfiling = true
	withProfile, err := SolveLayout(catalog, items, geometry.FullGridMask(), profiledConfig)
	if err != nil {
		t.Fatalf("profile on SolveLayout: %v", err)
	}
	if len(withoutProfile) != 1 || len(withProfile) != 1 {
		t.Fatalf("solution counts off=%d on=%d", len(withoutProfile), len(withProfile))
	}
	off := operationProfilePipelineSnapshotForTest(withoutProfile[0])
	on := operationProfilePipelineSnapshotForTest(withProfile[0])
	if len(off.Roots) < 2 {
		t.Fatalf("fixture did not exercise multiple root families: roots=%+v diagnostics=%+v", off.Roots, withoutProfile[0].Search.ConstellationSeedDiagnostics)
	}
	if !reflect.DeepEqual(on, off) {
		t.Fatalf("operation profiling changed full pipeline result:\n off=%+v\n  on=%+v", off, on)
	}
	if withProfile[0].Search.ConstellationSeedDiagnostics.RootPackingOperationProfile == nil {
		t.Fatal("profiled full pipeline did not retain aggregate operation profile")
	}
}

func TestOperationProfilingRequiresSingleWorker(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{"a": {ID: "a", Shape: []model.Coord{{}}}}}
	_, err := SolveLayout(catalog, []string{"a"}, 1, Config{OperationProfiling: true, Workers: 2})
	if err == nil || !strings.Contains(err.Error(), "requires exactly one worker") {
		t.Fatalf("operation profiling worker error=%v", err)
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

type operationProfilePipelineSnapshot struct {
	Score         model.Score
	LayoutKey     string
	CanonicalHash string
	NodesExplored int64
	Roots         []operationProfilePipelineRootSnapshot
}

type operationProfilePipelineRootSnapshot struct {
	ID                     string
	NodesConsumed          int64
	TerminationReason      string
	MRVDepths              []model.ConstellationRootPackingDepthDiagnostic
	BeamEvictions          int64
	FamilyAllocationRounds []model.ConstellationRootPackingAllocationRound
	FamilyTotalConsumed    int64
	FamilyTotalReturned    int64
}

func operationProfilePipelineSnapshotForTest(solution model.Solution) operationProfilePipelineSnapshot {
	diagnostics := solution.Search.ConstellationSeedDiagnostics
	snapshot := operationProfilePipelineSnapshot{
		Score:         cloneScore(solution.Evaluation.Score),
		LayoutKey:     solution.LayoutKey,
		CanonicalHash: solution.CanonicalLayoutHash,
		NodesExplored: solution.Search.NodesExplored,
		Roots:         make([]operationProfilePipelineRootSnapshot, 0, len(diagnostics.Roots)),
	}
	for _, root := range diagnostics.Roots {
		snapshot.Roots = append(snapshot.Roots, operationProfilePipelineRootSnapshot{
			ID:                     root.ID,
			NodesConsumed:          root.NodesConsumed,
			TerminationReason:      root.TerminationReason,
			MRVDepths:              append([]model.ConstellationRootPackingDepthDiagnostic(nil), root.MRVDepths...),
			BeamEvictions:          root.BeamEvictions,
			FamilyAllocationRounds: append([]model.ConstellationRootPackingAllocationRound(nil), root.FamilyAllocationRounds...),
			FamilyTotalConsumed:    root.FamilyTotalConsumed,
			FamilyTotalReturned:    root.FamilyTotalReturned,
		})
	}
	return snapshot
}
