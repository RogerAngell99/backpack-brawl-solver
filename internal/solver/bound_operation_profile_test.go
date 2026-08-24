//go:build searchprofile

package solver

import (
	"reflect"
	"sync/atomic"
	"testing"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

func TestProfiledPriorityUpperBoundMatchesNormalAndAccountsGeometryRegimes(t *testing.T) {
	catalog, instances, optionsByInstance, state, priorities, _ := boundAttributionFixture(t)
	want := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities)
	var profile model.PriorityUpperBoundSiteProfile
	got := partialRepairV3PriorityUpperBoundProfiled(catalog, state, optionsByInstance, priorities, &profile)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiled priority upper=%v, normal=%v", got, want)
	}
	recordPriorityUpperBoundResult(&profile, partialRepairTargetVectorFeasible(got, got))
	assertPriorityUpperBoundSiteProfileIdentities(t, profile)
	if profile.FixedFixedGeometryChecks == 0 || profile.RemovedSourceOptionChecksFixedTarget == 0 || profile.FixedSourceTargetOptionChecks == 0 || profile.RemovedSourceTargetOptionPairs == 0 {
		t.Fatalf("priority fixture did not exercise all geometry regimes: %+v", profile)
	}
	if profile.RemovedOptionRejectedFixedOverlap == 0 || profile.RemovedOptionRejectedOutsideFree == 0 || profile.RemovedOptionsRetained == 0 {
		t.Fatalf("priority fixture did not exercise exclusive option outcomes: %+v", profile)
	}
	if profile.SelfTargetSkips == 0 || profile.StarPositionHitTrue == 0 || profile.MatchingCalls == 0 || profile.AnchoredSourceInstances == 0 || profile.RemovedSourceInstances == 0 {
		t.Fatalf("priority fixture missed source/target paths: %+v", profile)
	}
	if profile.FixedPlacementInputs != int64(len(state.FixedPlacements)) || profile.CurrentPlacementInputs != int64(len(state.CurrentPlacements)) || profile.RemovedInstanceInputs != int64(len(state.RemovedInstances)) {
		t.Fatalf("priority logical inputs=%+v state=%+v", profile, state)
	}
	if profile.PriorityEntriesValidated != int64(len(priorities)) || len(instances) != 4 {
		t.Fatalf("priority validation=%+v instances=%d", profile, len(instances))
	}
}

func TestProfiledPriorityUpperBoundAccountsInvalidAndZeroStarSources(t *testing.T) {
	catalog, _, optionsByInstance, state, _, _ := boundAttributionFixture(t)
	var invalid model.PriorityUpperBoundSiteProfile
	got := partialRepairV3PriorityUpperBoundProfiled(catalog, state, optionsByInstance, []string{"craft:missing"}, &invalid)
	if got != nil {
		t.Fatalf("invalid priority upper=%v, want nil", got)
	}
	recordPriorityUpperBoundResult(&invalid, partialRepairTargetVectorFeasible(got, []int{99}))
	assertPriorityUpperBoundSiteProfileIdentities(t, invalid)
	if invalid.InvalidPriorityReturns != 1 || invalid.PriorityEntriesValidated != 0 || invalid.FeasibleResults != 1 {
		t.Fatalf("invalid priority profile=%+v", invalid)
	}

	zeroCatalog := model.Catalog{Items: map[string]model.Item{
		"zero": {ID: "zero", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	zeroInstances := ExpandInventory([]string{"zero"})
	zeroOptions := testOptionsByInstance(t, zeroCatalog, zeroInstances)
	zeroState := partialRepairState{RemovedInstances: zeroInstances, FreeCells: 1}
	var zero model.PriorityUpperBoundSiteProfile
	zeroUpper := partialRepairV3PriorityUpperBoundProfiled(zeroCatalog, zeroState, zeroOptions, []string{"star_source:zero"}, &zero)
	recordPriorityUpperBoundResult(&zero, partialRepairTargetVectorFeasible(zeroUpper, []int{0}))
	assertPriorityUpperBoundSiteProfileIdentities(t, zero)
	if !reflect.DeepEqual(zeroUpper, []int{0}) || zero.RemovedSourceInstances != 1 || zero.StarSlots != 0 || zero.MatchingCalls != 0 {
		t.Fatalf("zero-star priority profile=%+v upper=%v", zero, zeroUpper)
	}
}

func TestProfiledOutgoingUpperBoundMatchesNormalAndAccountsInternals(t *testing.T) {
	catalog, instances, optionsByInstance, _, _, _ := boundAttributionFixture(t)
	catalog.Items["zero"] = model.Item{ID: "zero", Shape: []model.Coord{{}}, Rotations: []int{0}}
	instances = append(instances, model.InventoryInstance{InstanceID: "zero#4", ItemID: "zero", OriginalIndex: 4})
	zeroOptions, err := PlacementOptions(catalog, instances[len(instances)-1], uint64(0b111111))
	if err != nil {
		t.Fatal(err)
	}
	optionsByInstance["zero#4"] = zeroOptions
	config := Config{
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source", "star_source:zero"},
	}
	ctx := newOutgoingBoundContext(catalog, instances, optionsByInstance, config, nil)
	if ctx == nil {
		t.Fatal("expected outgoing bound context")
	}
	placements := []model.Placement{
		partialRepairBoundPlacementAt(t, optionsByInstance["source#0"], 1),
		partialRepairBoundPlacementAt(t, optionsByInstance["food#2"], 2),
		partialRepairBoundPlacementAt(t, optionsByInstance["zero#4"], 4),
	}
	want := ctx.upperPriorityCounts(placements)
	var profile model.OutgoingBoundSiteProfile
	got := ctx.upperPriorityCountsProfiled(placements, &profile)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("profiled outgoing upper=%v, normal=%v", got, want)
	}
	assertOutgoingBoundSiteProfileIdentities(t, profile)
	if profile.PlacedMapBuilds != 1 || profile.PlacedMapInsertions != int64(len(placements)) || profile.ZeroStarSourceSkips == 0 || profile.PlacedSourceIterations == 0 || profile.FreeSourceIterations == 0 || profile.SourceHitsTargetCalls == 0 {
		t.Fatalf("outgoing fixture missed internal paths: %+v", profile)
	}
}

func TestBoundAttributionPhysicalCallSitesRemainSeparate(t *testing.T) {
	catalog, instances, optionsByInstance, state, priorities, gridMask := boundAttributionFixture(t)
	config := Config{
		TopN:               1,
		AllowSkips:         true,
		Workers:            1,
		OperationProfiling: true,
		PrioritySemantics:  model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:         priorities,
		priorityBounds:     &priorityBoundContext{ceiling: []int{0}},
	}

	constellationCounters := newBoundOperationCounters(config)
	constellationStates := []constellationSeedState{{placed: state.FixedPlacements, occupied: partialRepairOccupied(state.FixedPlacements)}}
	filtered := filterConstellationPriorityFeasibleStates(catalog, instances, optionsByInstance, gridMask, config, constellationStates, constellationCounters)
	constellationProfile := constellationCounters.snapshot()
	if len(filtered) != 1 || constellationProfile.PriorityUpper.ConstellationFilter.Calls != 1 || constellationProfile.PriorityUpper.RepairDFS.Calls != 0 || constellationProfile.PriorityUpper.PlateauPrefilter.Calls != 0 || constellationProfile.PriorityUpper.PlateauDFS.Calls != 0 {
		t.Fatalf("constellation attribution leaked sites: %+v", constellationProfile.PriorityUpper)
	}
	if constellationProfile.PriorityUpper.ConstellationFilterInvocations != 1 || constellationProfile.PriorityUpper.ConstellationStatesInput != 1 || constellationProfile.PriorityUpper.ConstellationStatesRetained != 1 {
		t.Fatalf("constellation wrapper profile=%+v", constellationProfile.PriorityUpper)
	}

	basePlacements := []model.Placement{
		partialRepairBoundPlacementAt(t, optionsByInstance["source#0"], 1),
		partialRepairBoundPlacementAt(t, optionsByInstance["source#1"], 3),
		partialRepairBoundPlacementAt(t, optionsByInstance["food#2"], 2),
		partialRepairBoundPlacementAt(t, optionsByInstance["food#3"], 4),
	}
	base := model.Solution{Placements: basePlacements, LayoutKey: "base"}
	instanceByID := make(map[string]model.InventoryInstance, len(instances))
	for _, instance := range instances {
		instanceByID[instance.InstanceID] = instance
	}
	plateauCounters := newBoundOperationCounters(config)
	potentials := prioritizePlateauNeighborhoods(
		catalog,
		instanceByID,
		optionsByInstance,
		[]model.Solution{base},
		[]repairNeighborhood{{Key: "plateau", BaseLayoutKey: "base", InstanceIDs: []string{"source#1", "food#3"}}},
		config,
		gridMask,
		model.Score{PriorityCounts: []int{0}},
		plateauCounters,
	)
	plateauProfile := plateauCounters.snapshot()
	if len(potentials) != 1 || plateauProfile.PriorityUpper.PlateauPrefilter.Calls != 1 || plateauProfile.PriorityUpper.ConstellationFilter.Calls != 0 || plateauProfile.PriorityUpper.RepairDFS.Calls != 0 || plateauProfile.PriorityUpper.PlateauDFS.Calls != 0 {
		t.Fatalf("plateau prefilter attribution leaked sites: profile=%+v potentials=%+v", plateauProfile.PriorityUpper, potentials)
	}

	incumbent := model.Solution{Placements: basePlacements, Evaluation: model.Evaluation{Score: model.Score{PriorityCounts: []int{0}}}}
	repairInstances := []model.InventoryInstance{instances[1], instances[3]}
	task := repairSearchTask{
		Occupied:      partialRepairOccupied(state.anchoredPlacements()),
		Placements:    state.anchoredPlacements(),
		NodeBudget:    100,
		HasNodeBudget: true,
		PartialState:  state,
	}
	repairConfig := config
	repairConfig.repairPriorityTarget = []int{0}
	repairConfig.tracePhase = tracePhasePreRepair
	outgoing := newOutgoingBoundContext(catalog, instances, optionsByInstance, repairConfig, nil)
	ordinary := runRepairTask(catalog, instances, repairInstances, optionsByInstance, task, []model.Solution{incumbent}, repairConfig, gridMask, nil, nil, outgoing)
	ordinaryProfile := ordinary.BoundOperationProfile
	if ordinaryProfile == nil || ordinaryProfile.PriorityUpper.RepairDFS.Calls == 0 || ordinaryProfile.PriorityUpper.PlateauDFS.Calls != 0 || ordinaryProfile.PriorityUpper.RepairDFS.RejectedResults != ordinary.PriorityBoundPruned || ordinaryProfile.Outgoing.Repair.Checks != ordinary.OutgoingBoundChecks || ordinaryProfile.Outgoing.Repair.PrunedNodes != ordinary.OutgoingBoundPrunedNodes || ordinaryProfile.Outgoing.Search.Checks != 0 {
		t.Fatalf("ordinary repair attribution=%+v result=%+v", ordinaryProfile, ordinary)
	}

	plateauConfig := repairConfig
	plateauConfig.tracePhase = tracePhasePlateauLNS
	plateauDFS := runRepairTask(catalog, instances, repairInstances, optionsByInstance, task, []model.Solution{incumbent}, plateauConfig, gridMask, nil, nil, nil)
	plateauDFSProfile := plateauDFS.BoundOperationProfile
	if plateauDFSProfile == nil || plateauDFSProfile.PriorityUpper.PlateauDFS.Calls == 0 || plateauDFSProfile.PriorityUpper.RepairDFS.Calls != 0 || plateauDFSProfile.PriorityUpper.PlateauDFS.RejectedResults != plateauDFS.PriorityBoundPruned {
		t.Fatalf("plateau DFS attribution=%+v result=%+v", plateauDFSProfile, plateauDFS)
	}

	searchConfig := config
	searchConfig.repairPriorityTarget = nil
	searchConfig.tracePhase = tracePhaseDFS
	searchTask := searchTask{NodeBudget: 100, HasNodeBudget: true}
	var stop atomic.Bool
	search := runTask(catalog, instances, instances, optionsByInstance, searchTask, searchConfig, gridMask, nil, nil, outgoing, config.priorityBounds, []model.Solution{incumbent}, &stop, nil)
	searchProfile := search.BoundOperationProfile
	if searchProfile == nil || searchProfile.Outgoing.Search.Checks == 0 || searchProfile.Outgoing.Search.Checks != search.OutgoingBoundChecks || searchProfile.Outgoing.Search.PrunedNodes != search.OutgoingBoundPrunedNodes || searchProfile.Outgoing.Repair.Checks != 0 {
		t.Fatalf("search outgoing attribution=%+v result=%+v", searchProfile, search)
	}
}

func TestPriorityCeilingExitPreservesSearchBoundAttribution(t *testing.T) {
	catalog := repairTestCatalog()
	items := []string{"source", "weapon", "blocker"}
	instances := ExpandInventory(items)
	optionsByInstance := testOptionsByInstance(t, catalog, instances)
	config := Config{
		TopN:                  1,
		AllowSkips:            false,
		MaxNodes:              500,
		Workers:               1,
		PrioritySemantics:     model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:            []string{"star_source:source"},
		StopOnPriorityCeiling: true,
		OperationProfiling:    true,
	}
	initialPlacements := []model.Placement{
		testPlacement(t, optionsByInstance[instances[0].InstanceID], model.Coord{Row: 0, Col: 0}, 0),
		testPlacement(t, optionsByInstance[instances[1].InstanceID], model.Coord{Row: 2, Col: 2}, 0),
		testPlacement(t, optionsByInstance[instances[2].InstanceID], model.Coord{Row: 0, Col: 1}, 0),
	}
	initial := buildSolution(catalog, initialPlacements, instances, config.Priorities)
	initial.Evaluation = evaluateLayoutForConfig(catalog, initialPlacements, config)
	config.stageIncumbents = []model.Solution{initial}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	policy.CoverageSeedNodeBudget = 0
	policy.StarSeedNodeBudget = 0
	policy.PackingSeedNodeBudget = 0
	policy.CandidateLimit = 1
	config.policy = &policy

	solutions, err := solveLayoutStage(catalog, items, geometry.FullGridMask(), config)
	if err != nil {
		t.Fatalf("SolveLayout: %v", err)
	}
	if len(solutions) != 1 {
		t.Fatalf("solutions=%d want 1", len(solutions))
	}
	stats := solutions[0].Search
	if !stats.PriorityCeilingReached || !stats.StoppedAfterPriorityCeiling {
		t.Fatalf("search stats=%+v want priority-ceiling exit", stats)
	}
	if stats.NodesExplored <= 0 || stats.CoverageSeedNodes != 0 {
		t.Fatalf("fixture did not execute DFS after the seed: %+v", stats)
	}
	profile := stats.BoundOperationProfile
	assertBoundAttributionOperationProfileIdentities(t, profile)
	if stats.OutgoingBoundChecks == 0 || profile.Outgoing.Search.Checks != stats.OutgoingBoundChecks {
		t.Fatalf("priority-ceiling exit lost DFS outgoing checks: profile=%+v stats=%+v", profile.Outgoing, stats)
	}
	if profile.Outgoing.Search.PrunedNodes != stats.OutgoingBoundPrunedNodes || profile.Outgoing.Repair.Checks != 0 || profile.Outgoing.Repair.PrunedNodes != 0 {
		t.Fatalf("priority-ceiling outgoing reconciliation failed: profile=%+v stats=%+v", profile.Outgoing, stats)
	}
}

func TestBoundAttributionProfilesAggregateAndRejectVersionMismatch(t *testing.T) {
	first := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	first.PriorityUpper.RepairDFS.Calls = 2
	first.Outgoing.Search.Checks = 3
	second := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	second.PriorityUpper.RepairDFS.Calls = 5
	second.Outgoing.Search.Checks = 7
	aggregate := mergeBoundAttributionOperationProfiles(nil, first)
	aggregate = mergeBoundAttributionOperationProfiles(aggregate, second)
	if aggregate.PriorityUpper.RepairDFS.Calls != 7 || aggregate.Outgoing.Search.Checks != 10 {
		t.Fatalf("bound profile aggregate=%+v", aggregate)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("incompatible bound profile versions were silently merged")
		}
	}()
	mergeBoundAttributionOperationProfiles(aggregate, &model.BoundAttributionOperationProfile{Version: "bound-attribution-ops-v2"})
}

func boundAttributionFixture(t *testing.T) (model.Catalog, []model.InventoryInstance, map[string][]model.Placement, partialRepairState, []string, uint64) {
	t.Helper()
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {
			ID:        "source",
			Shape:     []model.Coord{{}},
			Stars:     []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}, {Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}, {Offset: model.Coord{Col: 2}, TargetTypes: []string{"Food"}}},
			Rotations: []int{0},
		},
		"food": {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	gridMask := mustParseGridForTest(t, "111111000/000000000/000000000/000000000/000000000/000000000")
	instances := ExpandInventory([]string{"source", "source", "food", "food"})
	optionsByInstance := testOptionsForGrid(t, catalog, instances, gridMask)
	fixedSource := partialRepairBoundPlacementAt(t, optionsByInstance["source#0"], 1)
	currentFood := partialRepairBoundPlacementAt(t, optionsByInstance["food#2"], 2)
	state := partialRepairState{
		FixedPlacements:   []model.Placement{fixedSource},
		CurrentPlacements: []model.Placement{currentFood},
		RemovedInstances:  []model.InventoryInstance{instances[1], instances[3]},
		FreeCells:         gridMask &^ fixedSource.Mask &^ currentFood.Mask &^ (uint64(1) << 5),
	}
	return catalog, instances, optionsByInstance, state, []string{"star_source:source"}, gridMask
}

func assertBoundAttributionOperationProfileIdentities(t *testing.T, profile *model.BoundAttributionOperationProfile) {
	t.Helper()
	if profile == nil || profile.Version != model.BoundAttributionProfileVersion {
		t.Fatalf("bound operation profile=%+v", profile)
	}
	priority := profile.PriorityUpper
	for name, site := range map[string]model.PriorityUpperBoundSiteProfile{
		"constellation":     priority.ConstellationFilter,
		"repair":            priority.RepairDFS,
		"plateau-prefilter": priority.PlateauPrefilter,
		"plateau-dfs":       priority.PlateauDFS,
	} {
		t.Run(name, func(t *testing.T) { assertPriorityUpperBoundSiteProfileIdentities(t, site) })
	}
	if priority.ConstellationStatesInput != priority.ConstellationStatesRetained+priority.ConstellationStatesRejected {
		t.Fatalf("constellation state identity failed: %+v", priority)
	}
	if priority.ConstellationFilter.Calls != priority.ConstellationStatesInput {
		t.Fatalf("constellation call/input identity failed: %+v", priority)
	}
	assertOutgoingBoundSiteProfileIdentities(t, profile.Outgoing.Search)
	assertOutgoingBoundSiteProfileIdentities(t, profile.Outgoing.Repair)
}

func assertPriorityUpperBoundSiteProfileIdentities(t testing.TB, profile model.PriorityUpperBoundSiteProfile) {
	t.Helper()
	if profile.Calls != profile.FeasibleResults+profile.RejectedResults {
		t.Fatalf("priority outcome identity failed: %+v", profile)
	}
	if profile.RemovedOptionCandidates != profile.RemovedOptionRejectedFixedOverlap+profile.RemovedOptionRejectedOutsideFree+profile.RemovedOptionsRetained {
		t.Fatalf("priority option identity failed: %+v", profile)
	}
	regimes := profile.FixedFixedGeometryChecks + profile.RemovedSourceOptionChecksFixedTarget + profile.FixedSourceTargetOptionChecks + profile.RemovedSourceTargetOptionPairs
	if profile.GeometryCandidateChecks != regimes {
		t.Fatalf("priority geometry regime identity failed: %+v", profile)
	}
	if profile.GeometryCandidateChecks != profile.GeometryOverlapRejects+profile.StarPositionHitCalls {
		t.Fatalf("priority geometry outcome identity failed: %+v", profile)
	}
	if profile.StarPositionHitTrue != profile.SlotTargetHits {
		t.Fatalf("priority hit identity failed: %+v", profile)
	}
}

func assertOutgoingBoundSiteProfileIdentities(t testing.TB, profile model.OutgoingBoundSiteProfile) {
	t.Helper()
	if profile.PrioritySourceMatches != profile.ZeroStarSourceSkips+profile.PlacedSourceIterations+profile.FreeSourceIterations {
		t.Fatalf("outgoing source identity failed: %+v", profile)
	}
	if profile.PlacedSourceTargetIterations != profile.SelfTargetSkips+profile.TargetPlacementLookups {
		t.Fatalf("outgoing target scan identity failed: %+v", profile)
	}
	if profile.TargetPlacementLookups != profile.PlacedTargetsFound+profile.UnplacedTargets {
		t.Fatalf("outgoing target placement identity failed: %+v", profile)
	}
	if profile.SourceHitsTargetCalls != profile.PlacedTargetsFound {
		t.Fatalf("outgoing hit call identity failed: %+v", profile)
	}
	if profile.CoveragePlacementKeyCalls != profile.PlacedSourceIterations || profile.PlacedPotentialLookups != profile.PlacedSourceIterations || profile.FreePotentialLookups != profile.FreeSourceIterations {
		t.Fatalf("outgoing potential lookup identity failed: %+v", profile)
	}
	if profile.PopcountCalls != profile.PlacedSourceIterations+profile.FreeSourceIterations {
		t.Fatalf("outgoing popcount identity failed: %+v", profile)
	}
	if profile.Checks != 0 && profile.PlacedMapBuilds != profile.Checks {
		t.Fatalf("outgoing check/map identity failed: %+v", profile)
	}
}
