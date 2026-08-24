//go:build searchprofile

package solver

import (
	"fmt"
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
	semanticOff := operationProfileSemanticSolutionForTest(withoutProfile[0])
	semanticOn := operationProfileSemanticSolutionForTest(withProfile[0])
	if !reflect.DeepEqual(semanticOn, semanticOff) {
		t.Fatalf("operation profiling changed semantic/search state at %s", firstOperationProfileDifference(reflect.ValueOf(semanticOff), reflect.ValueOf(semanticOn), "solution"))
	}
	if withProfile[0].Search.ConstellationSeedDiagnostics.RootPackingOperationProfile == nil {
		t.Fatal("profiled full pipeline did not retain aggregate operation profile")
	}
	packingProfile := withProfile[0].Search.PackingSeedOperationProfile
	if packingProfile == nil || packingProfile.Version != PackingSeedFeasibilityProfileVersion || packingProfile.SearchCalls != 1 {
		t.Fatalf("packing-seed operation profile=%+v", packingProfile)
	}
	assertPackingSeedFeasibilityOperationProfileIdentities(t, packingProfile)
	boundProfile := withProfile[0].Search.BoundOperationProfile
	assertBoundAttributionOperationProfileIdentities(t, boundProfile)
	if boundProfile.PriorityUpper.ConstellationFilter.Calls == 0 || boundProfile.Outgoing.Search.Checks == 0 {
		t.Fatalf("full pipeline did not exercise comparative bound sites: %+v", boundProfile)
	}
	if boundProfile.Outgoing.Search.Checks+boundProfile.Outgoing.Repair.Checks != withProfile[0].Search.OutgoingBoundChecks {
		t.Fatalf("outgoing check cross-check failed: profile=%+v stats=%+v", boundProfile.Outgoing, withProfile[0].Search)
	}
	if boundProfile.Outgoing.Search.PrunedNodes+boundProfile.Outgoing.Repair.PrunedNodes != withProfile[0].Search.OutgoingBoundPrunedNodes {
		t.Fatalf("outgoing prune cross-check failed: profile=%+v stats=%+v", boundProfile.Outgoing, withProfile[0].Search)
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

func TestProfiledCanonicalCopyOrderMatchesNormalAndAccountsPlacementKeys(t *testing.T) {
	tests := []struct {
		name      string
		placement model.Placement
		existing  []model.Placement
	}{
		{
			name:      "unique item",
			placement: operationProfilePlacement("unique#0", "unique", 0, 0),
		},
		{
			name:      "two copies canonical",
			placement: operationProfilePlacement("copy#1", "copy", 1, 1),
			existing:  []model.Placement{operationProfilePlacement("copy#0", "copy", 0, 0)},
		},
		{
			name:      "two copies noncanonical",
			placement: operationProfilePlacement("copy#1", "copy", 1, 0),
			existing:  []model.Placement{operationProfilePlacement("copy#0", "copy", 0, 1)},
		},
		{
			name:      "three copies canonical",
			placement: operationProfilePlacement("copy#2", "copy", 2, 2),
			existing: []model.Placement{
				operationProfilePlacement("copy#0", "copy", 0, 0),
				operationProfilePlacement("copy#1", "copy", 1, 1),
			},
		},
		{
			name:      "other item is ignored",
			placement: operationProfilePlacement("copy#1", "copy", 1, 0),
			existing:  []model.Placement{operationProfilePlacement("other#0", "other", 0, 1)},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			want := placementRespectsCanonicalCopyOrder(test.placement, test.existing)
			var profile model.PackingSeedCanonicalCopyOrderOperationProfile
			got := placementRespectsCanonicalCopyOrderProfiled(test.placement, test.existing, &profile)
			if got != want {
				t.Fatalf("profiled canonical=%t, normal=%t", got, want)
			}
			if profile.Calls != 1 {
				t.Fatalf("canonical calls=%d, want 1", profile.Calls)
			}
			if profile.PlacementKeyCalls != profile.CandidatePlacementKeyCalls+profile.SameItemComparisons {
				t.Fatalf("placement key identity failed: %+v", profile)
			}
			if profile.CandidatePlacementKeyCalls > profile.SameItemComparisons {
				t.Fatalf("candidate key materialized without a same-item comparison: %+v", profile)
			}
			if profile.CandidatePlacementKeyCalls > profile.Calls {
				t.Fatalf("candidate key calls exceeded canonical calls: %+v", profile)
			}
			if test.name == "unique item" && (profile.CandidatePlacementKeyCalls != 0 || profile.PlacementKeyCalls != 0 || profile.PlacementKeyBytes != 0) {
				t.Fatalf("unique item constructed canonical keys: %+v", profile)
			}
			wantRejects := int64(0)
			if !want {
				wantRejects = 1
			}
			if profile.Rejects != wantRejects {
				t.Fatalf("canonical rejects=%d, want %d: %+v", profile.Rejects, wantRejects, profile)
			}
		})
	}
}

func TestProfiledPackingFeasibilityMatchesNormalAndAccountsChecks(t *testing.T) {
	for _, test := range []struct {
		name       string
		remaining  []model.InventoryInstance
		options    map[string][]model.Placement
		occupied   uint64
		placements []model.Placement
	}{
		{
			name: "feasible with overlap and canonical rejection",
			remaining: []model.InventoryInstance{
				{InstanceID: "copy#1", ItemID: "copy", OriginalIndex: 1},
				{InstanceID: "unique#0", ItemID: "unique", OriginalIndex: 2},
			},
			options: map[string][]model.Placement{
				"copy#1": {
					operationProfilePlacement("copy#1", "copy", 1, 0),
					operationProfilePlacement("copy#1", "copy", 1, 2),
				},
				"unique#0": {operationProfilePlacement("unique#0", "unique", 2, 3)},
			},
			occupied:   uint64(1) << 1,
			placements: []model.Placement{operationProfilePlacement("copy#0", "copy", 0, 1)},
		},
		{
			name: "dead remaining instance",
			remaining: []model.InventoryInstance{
				{InstanceID: "copy#1", ItemID: "copy", OriginalIndex: 1},
				{InstanceID: "unique#0", ItemID: "unique", OriginalIndex: 2},
			},
			options: map[string][]model.Placement{
				"copy#1":   {operationProfilePlacement("copy#1", "copy", 1, 2)},
				"unique#0": {operationProfilePlacement("unique#0", "unique", 2, 1)},
			},
			occupied:   uint64(1) << 1,
			placements: []model.Placement{operationProfilePlacement("copy#0", "copy", 0, 1)},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wantRestricted, wantFlexibility, wantFeasible := packingFeasibility(test.remaining, test.options, test.occupied, test.placements)
			counters := newPackingSeedFeasibilityOperationCounters(Config{OperationProfiling: true})
			gotRestricted, gotFlexibility, gotFeasible := packingSeedFeasibilityProfiled(test.remaining, test.options, test.occupied, test.placements, counters)
			if gotRestricted != wantRestricted || gotFlexibility != wantFlexibility || gotFeasible != wantFeasible {
				t.Fatalf("profiled feasibility=(%d, %d, %t), normal=(%d, %d, %t)", gotRestricted, gotFlexibility, gotFeasible, wantRestricted, wantFlexibility, wantFeasible)
			}
			profile := counters.snapshot()
			if profile.FeasibilityCalls != 1 {
				t.Fatalf("feasibility calls=%d, want 1", profile.FeasibilityCalls)
			}
			assertPackingSeedFeasibilityOperationProfileIdentities(t, profile)
		})
	}
}

func operationProfilePlacement(instanceID string, itemID string, originalIndex int, column int) model.Placement {
	cell := model.Coord{Col: column}
	return model.Placement{
		InstanceID:    instanceID,
		ItemID:        itemID,
		OriginalIndex: originalIndex,
		Origin:        cell,
		Cells:         []model.Coord{cell},
		Mask:          uint64(1) << uint(column),
	}
}

func assertPackingSeedFeasibilityOperationProfileIdentities(t testing.TB, profile *model.PackingSeedFeasibilityOperationProfile) {
	t.Helper()
	if profile == nil {
		t.Fatal("missing packing-seed feasibility profile")
	}
	if profile.CandidateOptionChecks != profile.CandidateOverlapRejects+profile.CandidateCanonical.Calls {
		t.Fatalf("candidate option identity failed: %+v", profile)
	}
	if profile.FeasibilityOptionChecks != profile.FeasibilityOverlapRejects+profile.FeasibilityCanonical.Calls {
		t.Fatalf("feasibility option identity failed: %+v", profile)
	}
	if profile.CandidateCanonical.PlacementKeyCalls != profile.CandidateCanonical.CandidatePlacementKeyCalls+profile.CandidateCanonical.SameItemComparisons {
		t.Fatalf("candidate key identity failed: %+v", profile.CandidateCanonical)
	}
	if profile.CandidateCanonical.CandidatePlacementKeyCalls > profile.CandidateCanonical.SameItemComparisons {
		t.Fatalf("candidate key materialization identity failed: %+v", profile.CandidateCanonical)
	}
	if profile.CandidateCanonical.CandidatePlacementKeyCalls > profile.CandidateCanonical.Calls {
		t.Fatalf("candidate key calls exceeded candidate canonical calls: %+v", profile.CandidateCanonical)
	}
	if profile.CandidateCanonical.Calls != profile.CandidateCanonical.Rejects+profile.CandidateChargeAttempts {
		t.Fatalf("candidate canonical identity failed: %+v", profile)
	}
	if profile.FeasibilityCanonical.PlacementKeyCalls != profile.FeasibilityCanonical.CandidatePlacementKeyCalls+profile.FeasibilityCanonical.SameItemComparisons {
		t.Fatalf("feasibility key identity failed: %+v", profile.FeasibilityCanonical)
	}
	if profile.FeasibilityCanonical.CandidatePlacementKeyCalls > profile.FeasibilityCanonical.SameItemComparisons {
		t.Fatalf("feasibility candidate key materialization identity failed: %+v", profile.FeasibilityCanonical)
	}
	if profile.FeasibilityCanonical.CandidatePlacementKeyCalls > profile.FeasibilityCanonical.Calls {
		t.Fatalf("candidate key calls exceeded feasibility canonical calls: %+v", profile.FeasibilityCanonical)
	}
	if profile.FeasibilityCanonical.Calls != profile.FeasibilityCanonical.Rejects+profile.FeasibilityLegalPlacements {
		t.Fatalf("feasibility canonical identity failed: %+v", profile)
	}
	if profile.CandidateChargeAttempts != profile.CandidateChargeDenied+profile.CandidateExpansions {
		t.Fatalf("candidate charge identity failed: %+v", profile)
	}
	if profile.SearchCalls > 0 && profile.FeasibilityCalls != profile.CandidateExpansions {
		t.Fatalf("search feasibility identity failed: %+v", profile)
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

func operationProfileSemanticSolutionForTest(solution model.Solution) model.Solution {
	copy := solution
	search := solution.Search
	search.NodesPerSecond = 0
	search.SetupMS = 0
	search.SeedMS = 0
	search.RepairMS = 0
	search.SearchMS = 0
	search.RefineMS = 0
	search.ServerElapsedMS = 0
	search.FirstCompleteMS = 0
	search.FirstFullyPackedMS = 0
	search.PackingSeedOperationProfile = nil
	search.BoundOperationProfile = nil
	search.IncumbentTrace = append([]model.IncumbentEvent(nil), search.IncumbentTrace...)
	for index := range search.IncumbentTrace {
		search.IncumbentTrace[index].ElapsedMS = 0
	}
	search.Stages = append([]model.SearchStageStats(nil), search.Stages...)
	for stageIndex := range search.Stages {
		search.Stages[stageIndex].IncumbentTrace = append([]model.IncumbentEvent(nil), search.Stages[stageIndex].IncumbentTrace...)
		for eventIndex := range search.Stages[stageIndex].IncumbentTrace {
			search.Stages[stageIndex].IncumbentTrace[eventIndex].ElapsedMS = 0
		}
	}
	diagnostics := search.ConstellationSeedDiagnostics
	diagnostics.RootPackingOperationProfile = nil
	diagnostics.Roots = append([]model.ConstellationRootDiagnostic(nil), diagnostics.Roots...)
	for index := range diagnostics.Roots {
		diagnostics.Roots[index].OperationProfile = nil
	}
	search.ConstellationSeedDiagnostics = diagnostics
	copy.Search = search
	return copy
}

func firstOperationProfileDifference(left reflect.Value, right reflect.Value, path string) string {
	if !left.IsValid() || !right.IsValid() {
		if left.IsValid() == right.IsValid() {
			return ""
		}
		return path
	}
	if left.Type() != right.Type() {
		return path + ".type"
	}
	switch left.Kind() {
	case reflect.Interface, reflect.Pointer:
		if left.IsNil() || right.IsNil() {
			if left.IsNil() == right.IsNil() {
				return ""
			}
			return path
		}
		return firstOperationProfileDifference(left.Elem(), right.Elem(), path)
	case reflect.Struct:
		for index := 0; index < left.NumField(); index++ {
			fieldPath := path + "." + left.Type().Field(index).Name
			if difference := firstOperationProfileDifference(left.Field(index), right.Field(index), fieldPath); difference != "" {
				return difference
			}
		}
		return ""
	case reflect.Slice, reflect.Array:
		if left.Len() != right.Len() {
			return path + ".length"
		}
		for index := 0; index < left.Len(); index++ {
			if difference := firstOperationProfileDifference(left.Index(index), right.Index(index), fmt.Sprintf("%s[%d]", path, index)); difference != "" {
				return difference
			}
		}
		return ""
	case reflect.Map:
		if left.Len() != right.Len() {
			return path + ".length"
		}
		for _, key := range left.MapKeys() {
			leftValue := left.MapIndex(key)
			rightValue := right.MapIndex(key)
			if !rightValue.IsValid() {
				return path + ".missing-key"
			}
			if difference := firstOperationProfileDifference(leftValue, rightValue, path+"[map]"); difference != "" {
				return difference
			}
		}
		return ""
	default:
		if !reflect.DeepEqual(left.Interface(), right.Interface()) {
			return path
		}
		return ""
	}
}
