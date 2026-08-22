package solver

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestGeneralSearchV1ResolvesProgressivePolicyAndFingerprint(t *testing.T) {
	base := Config{
		MaxNodes:                 1_000,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:left", "star_source:right"},
		ConstellationSeedVariant: ConstellationSeedVariantV4,
	}
	general := base
	general.ConstellationSeedVariant = ConstellationSeedVariantGeneralSearchV1
	v4Policy := resolveSearchPolicy(base, base.MaxNodes)
	policy := resolveSearchPolicy(general, general.MaxNodes)
	if policy.ConstellationRootPackingScheduler != constellationRootSchedulerProgressiveV1 || policy.ConstellationRootPackingInitialQuantumDivisor != 2 || policy.ConstellationRootPackingRoundQuantumDivisor != 2 {
		t.Fatalf("general-search policy=%+v", policy)
	}
	if policy.ConstellationSeedPackingStrategy != v4Policy.ConstellationSeedPackingStrategy || policy.ConstellationSeedPackingBeamWidth != v4Policy.ConstellationSeedPackingBeamWidth || policy.ConstellationSeedMaxSkeletons != v4Policy.ConstellationSeedMaxSkeletons {
		t.Fatalf("general-search no longer preserves V4 packing policy: v4=%+v general=%+v", v4Policy, policy)
	}
	if resolvedPolicyFingerprint(v4Policy) == resolvedPolicyFingerprint(policy) {
		t.Fatal("general-search policy did not change the policy fingerprint")
	}
	settings := SettingsForBenchmarkConfig(general)
	if settings.ConstellationRootPackingScheduler != constellationRootSchedulerProgressiveV1 || settings.ConstellationRootPackingInitialQuantumDivisor != 2 || settings.ConstellationRootPackingRoundQuantumDivisor != 2 {
		t.Fatalf("benchmark settings=%+v", settings)
	}
	resolved := resolveConstellationRootPackingSchedulerPolicy(policy, "prefix-5m", 24, 3)
	if resolved.StageID != "prefix-5m" || resolved.AvailablePackingBudget != 24 || resolved.FamilyCount != 3 || resolved.InitialQuantum != 4 || resolved.RoundQuantum != 4 {
		t.Fatalf("resolved scheduler policy=%+v", resolved)
	}
	encoded, err := json.Marshal(resolved)
	if err != nil || !reflect.DeepEqual(string(encoded), `{"stage_id":"prefix-5m","name":"progressive_round_robin_v1","available_packing_budget":24,"family_count":3,"initial_quantum":4,"round_quantum":4}`) {
		t.Fatalf("serialized scheduler policy=%s err=%v", encoded, err)
	}
}

func TestConstellationProgressiveRootPackingIsStableAndReclaimsDeadFamilyRounds(t *testing.T) {
	catalog, instances, firstOptions, roots := progressiveRootPackingFixture(false)
	_, _, secondOptions, reversedRoots := progressiveRootPackingFixture(true)
	first := runProgressiveRootPacking(catalog, instances, firstOptions, roots, 2)
	second := runProgressiveRootPacking(catalog, instances, secondOptions, reversedRoots, 2)
	if !reflect.DeepEqual(progressiveRootPackingSummary(first), progressiveRootPackingSummary(second)) {
		t.Fatalf("map construction changed allocation: first=%+v second=%+v", progressiveRootPackingSummary(first), progressiveRootPackingSummary(second))
	}
	if first.nodesConsumed > first.policy.AvailablePackingBudget {
		t.Fatalf("consumed=%d budget=%d", first.nodesConsumed, first.policy.AvailablePackingBudget)
	}
	var charged int64
	var effectiveQuota int64
	for _, family := range first.families {
		charged += family.result.nodes
		if family.result.nodes > family.nodesReserved {
			t.Fatalf("family=%s consumed=%d reserved=%d", family.familyID, family.result.nodes, family.nodesReserved)
		}
		for _, allocation := range family.rounds {
			effectiveQuota += allocation.Reserved - allocation.Returned
		}
	}
	if charged != first.nodesConsumed {
		t.Fatalf("family consumption=%d schedule consumption=%d", charged, first.nodesConsumed)
	}
	if effectiveQuota != first.nodesConsumed || effectiveQuota > first.policy.AvailablePackingBudget {
		t.Fatalf("effective quota=%d consumed=%d budget=%d", effectiveQuota, first.nodesConsumed, first.policy.AvailablePackingBudget)
	}

	dead := first.families[0]
	living := first.families[1]
	if dead.familyID != "single/root-dead" || dead.result.terminationReason != "hard_dead" || len(dead.rounds) != 1 || dead.rounds[0].Reserved != 1 || dead.rounds[0].Consumed != 0 || dead.rounds[0].Returned != 1 {
		t.Fatalf("dead family=%+v", dead)
	}
	if living.familyID != "single/root-living" || living.result.terminationReason != "completed" || len(living.rounds) != 2 || living.rounds[1].Round != 2 || living.rounds[1].Reserved != 1 || living.rounds[1].Consumed != 1 {
		t.Fatalf("living family did not receive reclaimed later round: %+v", living)
	}
}

func TestConstellationV4DoesNotEnableProgressiveRootPacking(t *testing.T) {
	config := Config{
		MaxNodes:                 1_000,
		ConstellationSeedVariant: ConstellationSeedVariantV4,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:left", "star_source:right"},
	}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	if constellationSeedUsesProgressiveRootScheduler(policy.ConstellationSeedVariant) || policy.ConstellationRootPackingScheduler != "" || policy.ConstellationRootPackingInitialQuantumDivisor != 0 || policy.ConstellationRootPackingRoundQuantumDivisor != 0 {
		t.Fatalf("V4 unexpectedly resolved progressive scheduling: %+v", policy)
	}
	_, diagnostics := constellationSchedulerSeedFixture(t, ConstellationSeedVariantV4)
	if diagnostics.RootPackingScheduler != nil || len(diagnostics.Roots) == 0 {
		t.Fatalf("V4 unexpectedly recorded progressive scheduler diagnostics: %+v", diagnostics)
	}
	for _, root := range diagnostics.Roots {
		if root.FamilyID != "" || len(root.FamilyAllocationRounds) != 0 || root.FamilyTotalQuota != 0 || root.FamilyTotalConsumed != 0 || root.FamilyTotalReturned != 0 || root.FamilyTerminationReason != "" {
			t.Fatalf("V4 root diagnostics changed: %+v", root)
		}
	}
}

func TestGeneralSearchV1ReportsRootFamilyDiagnostics(t *testing.T) {
	seed, diagnostics := constellationSchedulerSeedFixture(t, ConstellationSeedVariantGeneralSearchV1)
	if diagnostics.RootPackingScheduler == nil || diagnostics.RootPackingScheduler.StageID != "single" || diagnostics.RootPackingScheduler.Name != constellationRootSchedulerProgressiveV1 || len(diagnostics.Roots) == 0 {
		t.Fatalf("general-search diagnostics=%+v seed=%+v", diagnostics, seed)
	}
	for _, root := range diagnostics.Roots {
		if !strings.HasPrefix(root.FamilyID, "single/root-") || root.FamilyTotalQuota != root.NodesReserved || root.FamilyTotalConsumed != root.NodesConsumed || root.FamilyTerminationReason != root.TerminationReason {
			t.Fatalf("root family diagnostic=%+v", root)
		}
		var returned int64
		for _, allocation := range root.FamilyAllocationRounds {
			if allocation.Reserved != allocation.Consumed+allocation.Returned {
				t.Fatalf("allocation accounting=%+v", allocation)
			}
			returned += allocation.Returned
		}
		if returned != root.FamilyTotalReturned {
			t.Fatalf("root returned=%d family returned=%d", returned, root.FamilyTotalReturned)
		}
	}
}

func constellationSchedulerSeedFixture(t *testing.T, variant string) (coverageSeedResult, model.ConstellationSeedDiagnostics) {
	t.Helper()
	catalog := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"left", "right", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	config := Config{
		TopN:                     1,
		MaxNodes:                 1_000,
		Diagnostics:              true,
		ConstellationSeedVariant: variant,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:left", "star_source:right"},
	}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	config.policy = &policy
	config.priorityBounds = newPriorityBoundContext(catalog, instances, config.Priorities, config.PrioritySemantics)
	potential := newStarPotentialContext(catalog, instances, options, config.Priorities, config.PrioritySemantics)
	seed, diagnostics := constellationSeedSearch(catalog, instances, options, config, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	if seed.NodesExplored > policy.ConstellationSeedNodeBudget {
		t.Fatalf("nodes=%d budget=%d", seed.NodesExplored, policy.ConstellationSeedNodeBudget)
	}
	return seed, diagnostics
}

type progressivePackingSummary struct {
	Policy   model.ConstellationRootPackingSchedulerPolicy
	Families []progressivePackingFamilySummary
}

type progressivePackingFamilySummary struct {
	FamilyID    string
	Reserved    int64
	Consumed    int64
	Termination string
	Rounds      []model.ConstellationRootPackingAllocationRound
}

func progressiveRootPackingSummary(schedule constellationRootPackingSchedule) progressivePackingSummary {
	summary := progressivePackingSummary{Policy: schedule.policy, Families: make([]progressivePackingFamilySummary, 0, len(schedule.families))}
	for _, family := range schedule.families {
		summary.Families = append(summary.Families, progressivePackingFamilySummary{
			FamilyID:    family.familyID,
			Reserved:    family.nodesReserved,
			Consumed:    family.result.nodes,
			Termination: family.result.terminationReason,
			Rounds:      append([]model.ConstellationRootPackingAllocationRound(nil), family.rounds...),
		})
	}
	return summary
}

func runProgressiveRootPacking(catalog model.Catalog, instances []model.InventoryInstance, options map[string][]model.Placement, roots []constellationSkeleton, budget int64) constellationRootPackingSchedule {
	config := Config{
		TopN:                     1,
		MaxNodes:                 100,
		ConstellationSeedVariant: ConstellationSeedVariantGeneralSearchV1,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:a", "star_source:b"},
	}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	config.policy = &policy
	return constellationProgressiveRootPacking(catalog, instances, options, roots, config, 0b111, budget, func(bool) bool { return true }, nil)
}

func progressiveRootPackingFixture(reverseMaps bool) (model.Catalog, []model.InventoryInstance, map[string][]model.Placement, []constellationSkeleton) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"c": {ID: "c", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b", "c"})
	placement := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{
			InstanceID:    instance.InstanceID,
			ItemID:        instance.ItemID,
			OriginalIndex: instance.OriginalIndex,
			Origin:        model.Coord{Col: col},
			Cells:         []model.Coord{{Col: col}},
			Mask:          uint64(1) << uint(col),
		}
	}
	options := make(map[string][]model.Placement, 2)
	if reverseMaps {
		options[instances[2].InstanceID] = []model.Placement{placement(instances[2], 1)}
		options[instances[1].InstanceID] = []model.Placement{placement(instances[1], 0)}
	} else {
		options[instances[1].InstanceID] = []model.Placement{placement(instances[1], 0)}
		options[instances[2].InstanceID] = []model.Placement{placement(instances[2], 1)}
	}
	dead := constellationSkeleton{rootID: "root-dead", signature: "dead", exactKey: "dead", occupied: 0b001, placed: []model.Placement{placement(instances[0], 0)}}
	living := constellationSkeleton{rootID: "root-living", signature: "living", exactKey: "living", occupied: 0b100, placed: []model.Placement{placement(instances[0], 2)}}
	rootMap := make(map[string]constellationSkeleton, 2)
	if reverseMaps {
		rootMap[living.rootID] = living
		rootMap[dead.rootID] = dead
	} else {
		rootMap[dead.rootID] = dead
		rootMap[living.rootID] = living
	}
	roots := make([]constellationSkeleton, 0, len(rootMap))
	for _, root := range rootMap {
		roots = append(roots, root)
	}
	return catalog, instances, options, roots
}
