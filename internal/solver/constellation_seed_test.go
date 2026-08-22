package solver

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
)

func loadRuntimeCatalogForConstellationTest(t testing.TB) model.Catalog {
	t.Helper()
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load runtime catalog: %v", err)
	}
	return loaded
}

func TestConstellationSeedFindsSharedTargetV3Skeleton(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	items := []string{"left", "right", "food"}
	instances := ExpandInventory(items)
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	config := Config{
		TopN:                          3,
		AllowSkips:                    false,
		MaxNodes:                      1_000,
		Diagnostics:                   true,
		EnableConstellationSeedV1:     true,
		ConstellationFeasibilityProbe: true,
		PrioritySemantics:             model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:                    []string{"star_source:left", "star_source:right"},
	}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	config.policy = &policy
	config.priorityBounds = newPriorityBoundContext(catalog, instances, config.Priorities, config.PrioritySemantics)
	potential := newStarPotentialContext(catalog, instances, options, config.Priorities, config.PrioritySemantics)
	seed, diagnostics := constellationSeedSearch(catalog, instances, options, config, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	if diagnostics.Version != "v1" || diagnostics.SkeletonsDistinct == 0 || diagnostics.PriorityConstellations == 0 {
		t.Fatalf("constellation diagnostics=%+v", diagnostics)
	}
	if len(diagnostics.Skeletons) != diagnostics.SkeletonsDistinct || len(diagnostics.Roots) != diagnostics.PriorityConstellations {
		t.Fatalf("root provenance=%+v", diagnostics)
	}
	root := diagnostics.Roots[0]
	if root.ID == "" || root.SkeletonID == "" || !root.Completed || root.BestScore == nil || root.CandidateHash == "" || root.NodesConsumed > root.NodesReserved || (root.NodesConsumed > 0 && len(root.LayerWidths) == 0) {
		t.Fatalf("root diagnostic=%+v", root)
	}
	if root.FeasibilityStatus != "feasible" || root.ProbeTerminationReason != "root_packing_witness" || root.SearchExhausted == nil || *root.SearchExhausted || root.WitnessHash == "" || root.ProbeNodesAvailable == nil || root.ProbeNodesConsumed == nil || root.ProbeNodesReturned == nil || *root.ProbeNodesAvailable != root.NodesReserved-root.NodesConsumed || *root.ProbeNodesReturned != *root.ProbeNodesAvailable-*root.ProbeNodesConsumed {
		t.Fatalf("probe diagnostic=%+v", root)
	}
	if len(seed.Solutions) == 0 || !reflect.DeepEqual(seed.Solutions[0].Evaluation.Score.PriorityCounts, []int{1, 1}) {
		t.Fatalf("constellation solutions=%+v", seed.Solutions)
	}
	if seed.NodesExplored > policy.ConstellationSeedNodeBudget {
		t.Fatalf("nodes=%d budget=%d", seed.NodesExplored, policy.ConstellationSeedNodeBudget)
	}
}

func TestConstellationRootCompletionProbeClassifiesResults(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b"})
	option := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{
			InstanceID:    instance.InstanceID,
			ItemID:        instance.ItemID,
			OriginalIndex: instance.OriginalIndex,
			Origin:        model.Coord{Col: col},
			Cells:         []model.Coord{{Col: col}},
			Mask:          uint64(1) << uint(col),
		}
	}
	var charged int64
	reportNode := func(bool) bool {
		charged++
		return true
	}

	feasible := constellationRootCompletionProbe(catalog, instances, map[string][]model.Placement{
		"a#0": {option(instances[0], 0)},
		"b#1": {option(instances[1], 1)},
	}, constellationSkeleton{}, 0b11, 10, reportNode)
	if feasible.feasibilityStatus != "feasible" || feasible.terminationReason != "witness_found" || feasible.searchExhausted || feasible.nodes != 2 || feasible.witnessHash == "" {
		t.Fatalf("feasible probe=%+v", feasible)
	}
	if charged != feasible.nodes {
		t.Fatalf("charged=%d feasible nodes=%d", charged, feasible.nodes)
	}

	charged = 0
	infeasible := constellationRootCompletionProbe(catalog, instances, map[string][]model.Placement{
		"a#0": {option(instances[0], 0)},
		"b#1": {option(instances[1], 0)},
	}, constellationSkeleton{}, 0b11, 10, reportNode)
	if infeasible.feasibilityStatus != "infeasible_proven" || infeasible.terminationReason != "search_exhausted" || !infeasible.searchExhausted || infeasible.nodes != 1 || infeasible.witnessHash != "" {
		t.Fatalf("infeasible probe=%+v", infeasible)
	}
	if charged != infeasible.nodes {
		t.Fatalf("charged=%d infeasible nodes=%d", charged, infeasible.nodes)
	}

	unknown := constellationRootCompletionProbe(catalog, instances, map[string][]model.Placement{
		"a#0": {option(instances[0], 0)},
		"b#1": {option(instances[1], 1)},
	}, constellationSkeleton{}, 0b11, 1, reportNode)
	if unknown.feasibilityStatus != "unknown_budget" || unknown.terminationReason != "quota_exhausted" || unknown.searchExhausted || unknown.nodes != 1 || unknown.witnessHash != "" {
		t.Fatalf("unknown probe=%+v", unknown)
	}
}

func TestConstellationCandidatePoolFeasibilitySweepUsesExactAnchorsAndRanks(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b"})
	placement := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{InstanceID: instance.InstanceID, ItemID: instance.ItemID, OriginalIndex: instance.OriginalIndex, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	a0 := placement(instances[0], 0)
	b0 := placement(instances[1], 0)
	b1 := placement(instances[1], 1)
	first := constellationSkeleton{occupied: a0.Mask, placed: []model.Placement{a0}, signature: "a", sourceGeometryKey: "raw-a", sourceGeometryOrbitKey: "orbit-a", exactKey: constellationExactKey([]model.Placement{a0})}
	second := constellationSkeleton{occupied: b0.Mask, placed: []model.Placement{b0}, signature: "b", sourceGeometryKey: "raw-b", sourceGeometryOrbitKey: "orbit-b", exactKey: constellationExactKey([]model.Placement{b0})}
	var charged int64
	sweep := constellationCandidatePoolFeasibilitySweep(catalog, instances, map[string][]model.Placement{
		"a#0": {a0},
		"b#1": {b0, b1},
	}, map[string]constellationSkeleton{first.exactKey: first, second.exactKey: second}, nil, Config{stageID: "single"}, 0b11, 10, func() (bool, string) {
		charged++
		return true, ""
	})
	if sweep.CandidateCount != 2 || sweep.FeasibleCount != 1 || sweep.InfeasibleProvenCount != 1 || sweep.UnknownBudgetCount != 0 || sweep.NodesAvailable != 10 || sweep.NodesConsumed != 1 || sweep.NodesReturned != 9 || charged != sweep.NodesConsumed {
		t.Fatalf("sweep=%+v charged=%d", sweep, charged)
	}
	byKey := map[string]model.ConstellationCandidateFeasibilityRecord{}
	for _, record := range sweep.Candidates {
		byKey[record.ExactAnchorKey] = record
	}
	firstRecord := byKey[first.exactKey]
	secondRecord := byKey[second.exactKey]
	if firstRecord.FeasibilityStatus != "feasible" || secondRecord.FeasibilityStatus != "infeasible_proven" || firstRecord.FreeMaskHex != secondRecord.FreeMaskHex || firstRecord.CandidateID == secondRecord.CandidateID || firstRecord.CandidateRank == secondRecord.CandidateRank || firstRecord.SweepRank == secondRecord.SweepRank {
		t.Fatalf("records first=%+v second=%+v", firstRecord, secondRecord)
	}
	if len(sweep.Orbits) != 2 || sweep.Orbits[0].StageID != "single" {
		t.Fatalf("orbit aggregates=%+v", sweep.Orbits)
	}
}

func TestConstellationCandidatePoolSweepSeparatesCanonicalAndSweepRanks(t *testing.T) {
	makeSkeleton := func(exact, signature, orbit string) constellationSkeleton {
		return constellationSkeleton{exactKey: exact, signature: signature, sourceGeometryKey: exact, sourceGeometryOrbitKey: orbit, score: model.Score{PriorityCounts: []int{1, 1}}}
	}
	first := makeSkeleton("a", "a", "orbit-a")
	second := makeSkeleton("b", "b", "orbit-a")
	third := makeSkeleton("c", "c", "orbit-b")
	ordered, ranks := orderedConstellationCandidatePool(map[string]constellationSkeleton{
		third.exactKey:  third,
		second.exactKey: second,
		first.exactKey:  first,
	})
	if ranks[first.exactKey] != 1 || ranks[second.exactKey] != 2 || ranks[third.exactKey] != 3 {
		t.Fatalf("canonical ranks=%+v", ranks)
	}
	if len(ordered) != 3 || ordered[0].exactKey != first.exactKey || ordered[1].exactKey != third.exactKey || ordered[2].exactKey != second.exactKey {
		t.Fatalf("sweep order=%+v", ordered)
	}
}

func TestConstellationCandidatePoolFeasibilitySweepReportsLedgerExhaustion(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b"})
	placement := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{InstanceID: instance.InstanceID, ItemID: instance.ItemID, OriginalIndex: instance.OriginalIndex, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	a0 := placement(instances[0], 0)
	b1 := placement(instances[1], 1)
	candidate := constellationSkeleton{occupied: a0.Mask, placed: []model.Placement{a0}, signature: "a", sourceGeometryKey: "raw-a", sourceGeometryOrbitKey: "orbit-a", exactKey: constellationExactKey([]model.Placement{a0})}
	sweep := constellationCandidatePoolFeasibilitySweep(catalog, instances, map[string][]model.Placement{
		"a#0": {a0},
		"b#1": {b1},
	}, map[string]constellationSkeleton{candidate.exactKey: candidate}, nil, Config{stageID: "single"}, 0b11, 10, func() (bool, string) {
		return false, ledgerStopSourceGlobal
	})
	if sweep.UnknownBudgetCount != 1 || sweep.NodesConsumed != 0 || len(sweep.Candidates) != 1 || sweep.Candidates[0].FeasibilityStatus != "unknown_budget" || sweep.Candidates[0].TerminationReason != "ledger_exhausted" || sweep.Candidates[0].StopSource != ledgerStopSourceGlobal {
		t.Fatalf("ledger sweep=%+v", sweep)
	}
}

func TestConstellationCandidateCompletionOptimizationTargetsStableCandidateID(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	candidate := constellationSkeleton{exactKey: constellationExactKey(nil), signature: "candidate", sourceGeometryKey: "geometry", sourceGeometryOrbitKey: "orbit", targetAssignmentKey: "target"}
	candidateID := constellationCandidateID(candidate.exactKey)
	config := Config{
		ConstellationSeedVariant:                                ConstellationSeedVariantV4,
		ConstellationCandidateCompletionOptimizationProbe:       true,
		ConstellationCandidateCompletionOptimizationCandidateID: candidateID,
		PrioritySemantics:                                       model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:                                              []string{"star_source:source"},
	}
	policy := resolveSearchPolicy(config, 100)
	config.policy = &policy
	var charged int64
	optimization := constellationCandidateCompletionOptimization(catalog, instances, options, map[string]constellationSkeleton{candidate.exactKey: candidate}, nil, config, grid, 100, func() (bool, string) {
		charged++
		return true, ""
	})
	if len(optimization.Attempts) != 1 {
		t.Fatalf("attempts=%+v", optimization.Attempts)
	}
	attempt := optimization.Attempts[0]
	if attempt.SelectionStatus != "accepted" || attempt.CandidateID != candidateID || attempt.Status != "optimal_proven" || attempt.BestScore == nil || attempt.BestScore.StarCount != 1 || attempt.InitialIncumbentAvailable || attempt.NodesConsumed != charged || attempt.NodesAvailable != attempt.NodesConsumed+attempt.NodesReturned {
		t.Fatalf("candidate optimization=%+v charged=%d", attempt, charged)
	}

	missing := config
	missing.ConstellationCandidateCompletionOptimizationCandidateID = strings.Repeat("0", 64)
	missingPolicy := resolveSearchPolicy(missing, 100)
	missing.policy = &missingPolicy
	missingResult := constellationCandidateCompletionOptimization(catalog, instances, options, map[string]constellationSkeleton{candidate.exactKey: candidate}, nil, missing, grid, 100, func() (bool, string) { return true, "" })
	if len(missingResult.Attempts) != 1 || missingResult.Attempts[0].SelectionStatus != "target_not_found" {
		t.Fatalf("missing target=%+v", missingResult)
	}

	filtered := config
	filtered.ConstellationCandidateCompletionOptimizationStage = "prefix-5m"
	filteredPolicy := resolveSearchPolicy(filtered, 100)
	filtered.policy = &filteredPolicy
	filteredResult := constellationCandidateCompletionOptimization(catalog, instances, options, map[string]constellationSkeleton{candidate.exactKey: candidate}, nil, filtered, grid, 100, func() (bool, string) { return true, "" })
	if len(filteredResult.Attempts) != 1 || filteredResult.Attempts[0].SelectionStatus != "stage_not_selected" {
		t.Fatalf("filtered target=%+v", filteredResult)
	}
}

func TestConstellationCandidateWitnessPreservesExactTraversal(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	candidate := constellationSkeleton{exactKey: constellationExactKey(nil)}
	config := Config{PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:source"}}
	without := constellationRootCompletionOptimizationProbeWithCharge(catalog, instances, options, candidate, config, grid, 100, func() (bool, string) { return true, "" }, model.Solution{})
	if !without.hasBest || len(without.bestPlacements) == 0 {
		t.Fatalf("missing baseline witness=%+v", without)
	}
	semantic := constellationCandidateSemanticFingerprint(catalog, instances, candidate, config, grid)
	baseSolution := model.Solution{Placements: without.bestPlacements, Evaluation: model.Evaluation{Score: without.bestScore}, LayoutKey: without.bestLayoutKey, CanonicalLayoutHash: without.bestHash}
	witness := constellationCandidateCompletionWitness(semantic, constellationCandidateID(candidate.exactKey), candidate.exactKey, baseSolution)
	restored, err := constellationCandidateWitnessFromLayoutKey(witness.LayoutKey, witness.SemanticFingerprint, semantic, candidate, instances, options, catalog, config)
	if err != nil {
		t.Fatalf("restore witness: %v", err)
	}
	with := constellationRootCompletionOptimizationProbeWithCharge(catalog, instances, options, candidate, config, grid, 100, func() (bool, string) { return true, "" }, restored)
	if without.nodes != with.nodes || without.terminalCompletions != with.terminalCompletions || without.areaPrunes != with.areaPrunes || without.zeroDomainPrunes != with.zeroDomainPrunes || without.transpositionPrunes != with.transpositionPrunes || compareScores(without.bestScore, with.bestScore) != 0 || without.bestLayoutKey != with.bestLayoutKey {
		t.Fatalf("witness changed exact traversal without=%+v with=%+v", without, with)
	}
	withDiagnosticBudget := config
	withDiagnosticBudget.ConstellationCandidateCompletionOptimizationNodeBudget = 1_000_000
	if got := constellationCandidateSemanticFingerprint(catalog, instances, candidate, withDiagnosticBudget, grid); got != semantic {
		t.Fatalf("semantic fingerprint changed with diagnostic budget: %q want %q", got, semantic)
	}
	if _, err := constellationCandidateWitnessFromLayoutKey(witness.LayoutKey, strings.Repeat("0", 64), semantic, candidate, instances, options, catalog, config); err == nil {
		t.Fatal("accepted witness with mismatched semantic fingerprint")
	}
}

func TestConstellationCandidateCompletionOptimizationRequiresValidTarget(t *testing.T) {
	_, err := SolveLayout(model.Catalog{}, []string{"item"}, 1, Config{Diagnostics: true, ConstellationSeedVariant: ConstellationSeedVariantV4, ConstellationCandidateCompletionOptimizationProbe: true, ConstellationCandidateCompletionOptimizationCandidateID: "bad"})
	if err == nil || !strings.Contains(err.Error(), "candidate id") {
		t.Fatalf("invalid candidate target err=%v", err)
	}
	validID := strings.Repeat("0", 64)
	_, err = SolveLayout(model.Catalog{}, []string{"item"}, 1, Config{Diagnostics: true, ConstellationSeedVariant: ConstellationSeedVariantV4, ConstellationCandidateCompletionOptimizationProbe: true, ConstellationCandidateCompletionOptimizationCandidateID: validID, ConstellationCandidatePoolFeasibilitySweep: true})
	if err == nil || !strings.Contains(err.Error(), "cannot run with another constellation probe") {
		t.Fatalf("combined candidate probes err=%v", err)
	}
}

func TestConstellationFeasibilityProbeRequiresDiagnostics(t *testing.T) {
	_, err := SolveLayout(model.Catalog{}, []string{"item"}, 1, Config{ConstellationFeasibilityProbe: true})
	if err == nil || err.Error() != "constellation feasibility probe requires diagnostics" {
		t.Fatalf("probe without diagnostics err=%v", err)
	}
}

func TestConstellationCompletionOptimizationProbeFindsExactBestAndReportsCutoff(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {ID: "source", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	config := Config{AllowSkips: false, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:source"}}

	var expected model.Score
	hasExpected := false
	for _, source := range options["source#0"] {
		for _, food := range options["food#1"] {
			if source.Mask&food.Mask != 0 {
				continue
			}
			score := evaluateScoreForConfig(catalog, []model.Placement{source, food}, config)
			if !hasExpected || compareScores(score, expected) > 0 {
				hasExpected = true
				expected = score
			}
		}
	}
	if !hasExpected {
		t.Fatal("missing independent completion")
	}

	var charged int64
	reportNode := func(bool) bool {
		charged++
		return true
	}
	optimal := constellationRootCompletionOptimizationProbe(catalog, instances, options, constellationSkeleton{}, config, grid, 100, reportNode, model.Solution{})
	if optimal.status != "optimal_proven" || optimal.terminationReason != "search_exhausted" || !optimal.searchExhausted || !optimal.hasBest || compareScores(optimal.bestScore, expected) != 0 || optimal.terminalCompletions == 0 {
		t.Fatalf("optimal probe=%+v expected=%+v", optimal, expected)
	}
	if charged != optimal.nodes {
		t.Fatalf("charged=%d nodes=%d", charged, optimal.nodes)
	}

	charged = 0
	cutoff := constellationRootCompletionOptimizationProbe(catalog, instances, options, constellationSkeleton{}, config, grid, 3, reportNode, model.Solution{})
	if cutoff.status != "unknown_budget" || cutoff.terminationReason != "quota_exhausted" || cutoff.searchExhausted || !cutoff.hasBest || compareScores(cutoff.bestScore, expected) != 0 || cutoff.nodes != 3 {
		t.Fatalf("cutoff probe=%+v expected=%+v", cutoff, expected)
	}
	if charged != cutoff.nodes {
		t.Fatalf("charged=%d nodes=%d", charged, cutoff.nodes)
	}
}

func TestConstellationCompletionOptimizationProbeClassifiesStructuralCases(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b"})
	option := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{InstanceID: instance.InstanceID, ItemID: instance.ItemID, OriginalIndex: instance.OriginalIndex, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	infeasible := constellationRootCompletionOptimizationProbe(catalog, instances, map[string][]model.Placement{
		"a#0": {option(instances[0], 0)},
		"b#1": nil,
	}, constellationSkeleton{}, Config{}, 0b11, 0, func(bool) bool { return true }, model.Solution{})
	if infeasible.status != "infeasible_proven" || infeasible.terminationReason != "initial_domain_wipeout" || !infeasible.searchExhausted || infeasible.nodes != 0 {
		t.Fatalf("infeasible probe=%+v", infeasible)
	}

	completeRoot := constellationSkeleton{occupied: 0b11, placed: []model.Placement{option(instances[0], 0), option(instances[1], 1)}}
	complete := constellationRootCompletionOptimizationProbe(catalog, instances, nil, completeRoot, Config{}, 0b11, 0, func(bool) bool { return true }, model.Solution{})
	if complete.status != "optimal_proven" || complete.terminationReason != "root_complete" || !complete.searchExhausted || !complete.hasBest || complete.nodes != 0 {
		t.Fatalf("complete probe=%+v", complete)
	}
}

func TestConstellationCompletionOptimizationProbeRequiresDiagnosticsAndV3(t *testing.T) {
	_, err := SolveLayout(model.Catalog{}, []string{"item"}, 1, Config{ConstellationCompletionOptimizationProbe: true})
	if err == nil || err.Error() != "constellation completion optimization probe requires diagnostics" {
		t.Fatalf("probe without diagnostics err=%v", err)
	}
	_, err = SolveLayout(model.Catalog{}, []string{"item"}, 1, Config{Diagnostics: true, ConstellationCompletionOptimizationProbe: true, ConstellationSeedVariant: ConstellationSeedVariantV2})
	if err == nil || !strings.Contains(err.Error(), "requires constellation seed variant") {
		t.Fatalf("probe without V3 err=%v", err)
	}
	_, err = SolveLayout(model.Catalog{}, []string{"item"}, 1, Config{Diagnostics: true, ConstellationCompletionOptimizationProbe: true, ConstellationFeasibilityProbe: true, ConstellationSeedVariant: ConstellationSeedVariantV3})
	if err == nil || !strings.Contains(err.Error(), "cannot run together") {
		t.Fatalf("combined probes err=%v", err)
	}
}

func TestConstellationRootMRVSelectionUsesFewestLegalPlacements(t *testing.T) {
	instances := ExpandInventory([]string{"a", "b"})
	option := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{
			InstanceID:    instance.InstanceID,
			ItemID:        instance.ItemID,
			OriginalIndex: instance.OriginalIndex,
			Origin:        model.Coord{Col: col},
			Cells:         []model.Coord{{Col: col}},
			Mask:          uint64(1) << uint(col),
		}
	}
	options := map[string][]model.Placement{
		"a#0": {option(instances[0], 0), option(instances[0], 1)},
		"b#1": {option(instances[1], 0)},
	}
	selected, legal, ok := constellationRootMRVSelection(instances, 0b11, options, 0, nil)
	if !ok || selected != 1 || len(legal) != 1 {
		t.Fatalf("MRV selection index=%d legal=%d ok=%t", selected, len(legal), ok)
	}
	options["b#1"] = append(options["b#1"], option(instances[1], 1))
	selected, legal, ok = constellationRootMRVSelection(instances, 0b11, options, 0, nil)
	if !ok || selected != 0 || len(legal) != 2 {
		t.Fatalf("MRV tie selection index=%d legal=%d ok=%t", selected, len(legal), ok)
	}
}

func TestConstellationRootMRVStateKeyPreservesRemainingInstancesAndPlacements(t *testing.T) {
	placement := func(instanceID string, originalIndex int, col int) model.Placement {
		return model.Placement{
			InstanceID:    instanceID,
			ItemID:        "item",
			OriginalIndex: originalIndex,
			Origin:        model.Coord{Col: col},
			Cells:         []model.Coord{{Col: col}},
			Mask:          uint64(1) << uint(col),
		}
	}
	base := constellationRootMRVState{
		packingSeedState: packingSeedState{occupied: 0b11, placed: []model.Placement{placement("item#0", 0, 0), placement("item#1", 1, 1)}},
		remainingMask:    0b100,
	}
	withDifferentRemaining := base
	withDifferentRemaining.remainingMask = 0b1000
	withDifferentPlacement := base
	withDifferentPlacement.placed = []model.Placement{placement("item#0", 0, 1), placement("item#1", 1, 0)}
	if constellationRootMRVStateKey(base) == constellationRootMRVStateKey(withDifferentRemaining) {
		t.Fatal("MRV state key merged different remaining instances")
	}
	if constellationRootMRVStateKey(base) == constellationRootMRVStateKey(withDifferentPlacement) {
		t.Fatal("MRV state key merged different instance placements")
	}
}

func TestConstellationRootMRVPackingReportsZeroDomainPrunes(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"c": {ID: "c", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b", "c"})
	option := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{InstanceID: instance.InstanceID, ItemID: instance.ItemID, OriginalIndex: instance.OriginalIndex, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	options := map[string][]model.Placement{
		"a#0": {option(instances[0], 0), option(instances[0], 1)},
		"b#1": {option(instances[1], 0)},
		"c#2": {option(instances[2], 0)},
	}
	config := Config{}
	policy := resolveSearchPolicy(config, 100)
	policy.ConstellationSeedPackingStrategy = constellationPackingStrategyStateMRV
	config.policy = &policy
	result := constellationRootPackingMRV(catalog, instances, options, constellationSkeleton{signature: "root"}, config, 0b111, 100, func(bool) bool { return true })
	if result.terminationReason != "no_states" || result.hardPruned == 0 || len(result.mrvDepths) != 1 || result.mrvDepths[0].ZeroDomainPrunes == 0 {
		t.Fatalf("MRV result=%+v", result)
	}
}

func constellationRootPackingSessionFixture() (model.Catalog, []model.InventoryInstance, map[string][]model.Placement, constellationSkeleton, Config, uint64) {
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
	options := map[string][]model.Placement{
		"a#0": {placement(instances[0], 0), placement(instances[0], 1), placement(instances[0], 2)},
		"b#1": {placement(instances[1], 0), placement(instances[1], 1), placement(instances[1], 3)},
		"c#2": {placement(instances[2], 1), placement(instances[2], 2), placement(instances[2], 3)},
	}
	config := Config{TopN: 4}
	policy := resolveSearchPolicy(config, 20)
	policy.ConstellationSeedPackingStrategy = constellationPackingStrategyStateMRV
	policy.ConstellationSeedPackingBeamWidth = 2
	config.policy = &policy
	return catalog, instances, options, constellationSkeleton{rootID: "root", signature: "root"}, config, 0b1111
}

func TestConstellationRootPackingSessionMatchesFullRunAtDepthBoundary(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	var fullCharged int64
	fullSession := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool {
		fullCharged++
		return true
	})
	full := fullSession.Run(20)
	if !fullSession.Done() || len(full.solutions) == 0 {
		t.Fatalf("full session=%+v", full)
	}
	legacyFull := constellationRootPackingMRVLegacy(catalog, instances, options, root, config, gridMask, 20, func(bool) bool { return true })
	if !reflect.DeepEqual(full, legacyFull) {
		t.Fatalf("session full=%+v legacy full=%+v", full, legacyFull)
	}

	var partitionedCharged int64
	partitionedSession := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool {
		partitionedCharged++
		return true
	})
	boundary := partitionedSession.Run(3)
	if partitionedSession.depthActive || boundary.terminationReason != "paused_allocation" || len(boundary.mrvDepths) != 1 || boundary.mrvDepths[0].Depth != 1 {
		t.Fatalf("first allocation did not end at depth boundary: session=%+v result=%+v", partitionedSession, boundary)
	}
	partitioned := partitionedSession.Run(17)
	if !partitionedSession.Done() {
		t.Fatal("partitioned session did not complete")
	}
	if fullCharged != full.nodes || partitionedCharged != partitioned.nodes || !reflect.DeepEqual(full, partitioned) {
		t.Fatalf("full=%+v charged=%d partitioned=%+v charged=%d", full, fullCharged, partitioned, partitionedCharged)
	}
}

type constellationRootPackingSessionStateCheckpoint struct {
	Key           string
	Placements    string
	Occupied      uint64
	Restricted    int
	Flexibility   int
	Fragmentation int
	Score         model.Score
	RemainingMask uint64
	RemainingArea int
}

type constellationRootPackingSessionNextCheckpoint struct {
	Class string
	State constellationRootPackingSessionStateCheckpoint
}

type constellationRootPackingSessionCountersCheckpoint struct {
	Nodes              int64
	Candidates         int
	Deduplicated       int64
	HardPruned         int64
	SymmetryPruned     int64
	BeamEvictions      int64
	FirstCompleteNodes int64
	DistinctNextItems  int
	SelectedItemIDs    []string
}

// constellationRootPackingSessionCheckpoint is test-only state used to prove
// that splitting an allocation leaves an identical resumable search.
type constellationRootPackingSessionCheckpoint struct {
	Initialized   bool
	Done          bool
	Depth         int
	DepthActive   bool
	StateIndex    int
	StatePrepared bool
	OptionIndex   int
	Selected      model.InventoryInstance
	Options       []string
	NextMask      uint64
	NextArea      int
	Remaining     []model.InventoryInstance
	States        []constellationRootPackingSessionStateCheckpoint
	NextByClass   []constellationRootPackingSessionNextCheckpoint
	Counters      constellationRootPackingSessionCountersCheckpoint
	LayerWidths   []model.PackingSeedLayerWidth
	MRVDepths     []string
	DepthInfo     string
	ShadowDepth   string
	ShadowTrace   string
}

func checkpointConstellationRootPackingState(state constellationRootMRVState) constellationRootPackingSessionStateCheckpoint {
	return constellationRootPackingSessionStateCheckpoint{
		Key:           state.key,
		Placements:    constellationExactKey(state.placed),
		Occupied:      state.occupied,
		Restricted:    state.restricted,
		Flexibility:   state.flexibility,
		Fragmentation: state.fragmentation,
		Score:         cloneScore(state.score),
		RemainingMask: state.remainingMask,
		RemainingArea: state.remainingArea,
	}
}

func canonicalSessionCheckpointJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func constellationRootPackingSessionCheckpointForTest(session *constellationRootPackingSession) constellationRootPackingSessionCheckpoint {
	checkpoint := constellationRootPackingSessionCheckpoint{
		Initialized:   session.initialized,
		Done:          session.done,
		Depth:         session.depth,
		DepthActive:   session.depthActive,
		StateIndex:    session.stateIndex,
		StatePrepared: session.statePrepared,
		OptionIndex:   session.optionIndex,
		Selected:      session.selected,
		NextMask:      session.nextMask,
		NextArea:      session.nextArea,
		Remaining:     append([]model.InventoryInstance(nil), session.remaining...),
		States:        make([]constellationRootPackingSessionStateCheckpoint, 0, len(session.states)),
		NextByClass:   make([]constellationRootPackingSessionNextCheckpoint, 0, len(session.nextByClass)),
		Counters: constellationRootPackingSessionCountersCheckpoint{
			Nodes:              session.result.nodes,
			Candidates:         session.result.candidates,
			Deduplicated:       session.result.deduplicated,
			HardPruned:         session.result.hardPruned,
			SymmetryPruned:     session.result.symmetryPruned,
			BeamEvictions:      session.result.beamEvictions,
			FirstCompleteNodes: session.result.firstCompleteNodes,
			DistinctNextItems:  session.result.distinctNextItems,
		},
		LayerWidths: append([]model.PackingSeedLayerWidth(nil), session.result.layerWidths...),
		MRVDepths:   make([]string, 0, len(session.result.mrvDepths)),
		DepthInfo:   canonicalSessionCheckpointJSON(session.depthInfo),
		ShadowDepth: canonicalSessionCheckpointJSON(session.shadowDepth),
	}
	for _, option := range session.options {
		checkpoint.Options = append(checkpoint.Options, option.InstanceID+"|"+placementKey(option))
	}
	for _, state := range session.states {
		checkpoint.States = append(checkpoint.States, checkpointConstellationRootPackingState(state))
	}
	for class, state := range session.nextByClass {
		checkpoint.NextByClass = append(checkpoint.NextByClass, constellationRootPackingSessionNextCheckpoint{
			Class: class,
			State: checkpointConstellationRootPackingState(state),
		})
	}
	sort.Slice(checkpoint.NextByClass, func(i, j int) bool {
		return checkpoint.NextByClass[i].Class < checkpoint.NextByClass[j].Class
	})
	for itemID := range session.selectedItemIDs {
		checkpoint.Counters.SelectedItemIDs = append(checkpoint.Counters.SelectedItemIDs, itemID)
	}
	sort.Strings(checkpoint.Counters.SelectedItemIDs)
	for _, depth := range session.result.mrvDepths {
		checkpoint.MRVDepths = append(checkpoint.MRVDepths, canonicalSessionCheckpointJSON(depth))
	}
	if session.shadow != nil {
		checkpoint.ShadowTrace = canonicalSessionCheckpointJSON(session.shadow.trace)
	}
	return checkpoint
}

func completeConstellationRootPackingSessionForTest(t *testing.T, session *constellationRootPackingSession) constellationRootPackingResult {
	t.Helper()
	var result constellationRootPackingResult
	for rounds := 0; !session.Done() && rounds < 20; rounds++ {
		result = session.Run(20)
	}
	if !session.Done() {
		t.Fatalf("session did not complete: %+v", session)
	}
	return result
}

func findConstellationRootPackingPauseForTest(
	t *testing.T,
	catalog model.Catalog,
	instances []model.InventoryInstance,
	options map[string][]model.Placement,
	root constellationSkeleton,
	config Config,
	gridMask uint64,
	matches func(*constellationRootPackingSession) bool,
) int64 {
	t.Helper()
	for allocation := int64(1); allocation <= 20; allocation++ {
		session := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
		result := session.Run(allocation)
		if !session.Done() && result.terminationReason == "paused_allocation" && matches(session) {
			return allocation
		}
	}
	t.Fatal("did not reach requested resumable checkpoint")
	return 0
}

func TestConstellationRootPackingSessionCheckpointMatchesPartitionedPauses(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	allCompleteChildren := func(session *constellationRootPackingSession) bool {
		if len(session.nextByClass) == 0 {
			return false
		}
		for _, state := range session.nextByClass {
			if state.remainingMask != 0 {
				return false
			}
		}
		return true
	}
	scenarios := []struct {
		name    string
		matches func(*constellationRootPackingSession) bool
	}{
		{
			name: "depth_boundary",
			matches: func(session *constellationRootPackingSession) bool {
				return !session.depthActive && session.depth > 1
			},
		},
		{
			name: "mid_state",
			matches: func(session *constellationRootPackingSession) bool {
				return session.depthActive && !session.statePrepared && session.stateIndex > 0 && session.stateIndex < len(session.states)
			},
		},
		{
			name: "mid_option",
			matches: func(session *constellationRootPackingSession) bool {
				return session.depthActive && session.statePrepared && session.optionIndex > 0 && session.optionIndex < len(session.options)
			},
		},
		{
			name: "mid_last_depth_with_unexpanded_parents",
			matches: func(session *constellationRootPackingSession) bool {
				return session.depthActive && session.depth == len(session.remaining) && session.stateIndex < len(session.states)-1 && allCompleteChildren(session)
			},
		},
	}
	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			allocation := findConstellationRootPackingPauseForTest(t, catalog, instances, options, root, config, gridMask, scenario.matches)
			monolithic := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
			monolithicPause := monolithic.Run(allocation)
			partitioned := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
			partitioned.Run(allocation - 1)
			partitionedPause := partitioned.Run(1)
			if monolithicPause.terminationReason != "paused_allocation" || partitionedPause.terminationReason != "paused_allocation" || monolithic.Done() || partitioned.Done() {
				t.Fatalf("pause became terminal: monolithic=%+v partitioned=%+v", monolithicPause, partitionedPause)
			}
			if scenario.name == "mid_last_depth_with_unexpanded_parents" && (len(monolithicPause.solutions) != 0 || monolithicPause.candidates != 0 || len(monolithicPause.mrvDepths) != monolithic.depth-1) {
				t.Fatalf("pause previewed an unfinished last depth: session=%+v result=%+v", monolithic, monolithicPause)
			}
			if !reflect.DeepEqual(constellationRootPackingSessionCheckpointForTest(monolithic), constellationRootPackingSessionCheckpointForTest(partitioned)) {
				t.Fatalf("resume checkpoint differs after allocation %d: monolithic=%+v partitioned=%+v", allocation, constellationRootPackingSessionCheckpointForTest(monolithic), constellationRootPackingSessionCheckpointForTest(partitioned))
			}
			monolithicFinal := completeConstellationRootPackingSessionForTest(t, monolithic)
			partitionedFinal := completeConstellationRootPackingSessionForTest(t, partitioned)
			if !reflect.DeepEqual(monolithicFinal, partitionedFinal) {
				t.Fatalf("resume final differs after allocation %d: monolithic=%+v partitioned=%+v", allocation, monolithicFinal, partitionedFinal)
			}
		})
	}
}

func TestConstellationRootPackingSessionPartitionProperty(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	reference := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
	referenceResult := completeConstellationRootPackingSessionForTest(t, reference)
	if len(referenceResult.solutions) == 0 {
		t.Fatalf("reference result=%+v", referenceResult)
	}
	forcedConfig := config
	forcedConfig.forcedRootPackingReplay = true
	forcedConfig.ConstellationForcedCandidateRootedPackingBeamWidth = 2
	forcedConfig.ConstellationForcedCandidateRootedPackingRanking = constellationRootPackingRankingPriorityScoreFirst
	forcedPolicy := *config.policy
	forcedPolicy.ConstellationForcedCandidateRootedPackingBeamWidth = 2
	forcedPolicy.ConstellationForcedCandidateRootedPackingRanking = constellationRootPackingRankingPriorityScoreFirst
	forcedConfig.policy = &forcedPolicy
	configurations := []struct {
		name      string
		newConfig func() Config
	}{
		{name: "normal", newConfig: func() Config { return config }},
		{name: "shadow", newConfig: func() Config {
			withShadow := config
			withShadow.constellationRootPackingCollector = &constellationRootPackingCollector{shadow: newConstellationShadowReference(root, referenceResult.solutions[0], "partition-property")}
			return withShadow
		}},
		{name: "forced_replay", newConfig: func() Config { return forcedConfig }},
	}
	partitions := [][]int64{
		{20}, {1, 19}, {2, 18}, {3, 17}, {4, 16}, {5, 15}, {6, 14}, {7, 13},
		{8, 12}, {9, 11}, {10, 10}, {1, 1, 18}, {1, 2, 17}, {1, 3, 16},
		{2, 2, 16}, {2, 3, 15}, {3, 3, 14}, {1, 1, 1, 17}, {1, 1, 2, 16},
		{1, 2, 2, 15}, {2, 2, 2, 14}, {1, 2, 3, 14}, {2, 3, 4, 11}, {1, 1, 1, 1, 16},
	}
	for _, configuration := range configurations {
		t.Run(configuration.name, func(t *testing.T) {
			baselineConfig := configuration.newConfig()
			baseline := newConstellationRootPackingSession(catalog, instances, options, root, baselineConfig, gridMask, func(bool) bool { return true })
			baselineResult := baseline.Run(20)
			if !baseline.Done() {
				t.Fatalf("baseline did not complete: %+v", baseline)
			}
			baselineCheckpoint := constellationRootPackingSessionCheckpointForTest(baseline)
			for index, partition := range partitions {
				partitionConfig := configuration.newConfig()
				session := newConstellationRootPackingSession(catalog, instances, options, root, partitionConfig, gridMask, func(bool) bool { return true })
				var result constellationRootPackingResult
				for _, allocation := range partition {
					result = session.Run(allocation)
				}
				if !session.Done() || !reflect.DeepEqual(result, baselineResult) || !reflect.DeepEqual(constellationRootPackingSessionCheckpointForTest(session), baselineCheckpoint) {
					t.Fatalf("partition %d %v diverged: result=%+v checkpoint=%+v", index, partition, result, constellationRootPackingSessionCheckpointForTest(session))
				}
			}
		})
	}
}

func TestConstellationRootPackingMRVFinalizesOneShotBudget(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	session := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
	paused := session.Run(1)
	if paused.terminationReason != "paused_allocation" || session.Done() {
		t.Fatalf("session pause=%+v", paused)
	}
	final := session.FinalizeBudgetExhausted()
	if !session.Done() || final.terminationReason != "budget_exhausted" || !reflect.DeepEqual(final, session.Run(1)) {
		t.Fatalf("finalized session=%+v", final)
	}
	wrapper := constellationRootPackingMRV(catalog, instances, options, root, config, gridMask, 1, func(bool) bool { return true })
	if !reflect.DeepEqual(wrapper, final) || wrapper.terminationReason != "budget_exhausted" {
		t.Fatalf("wrapper=%+v final=%+v", wrapper, final)
	}
	zeroBudget := constellationRootPackingMRV(catalog, instances, options, root, config, gridMask, 0, func(bool) bool { return true })
	if zeroBudget.terminationReason != "no_budget" || zeroBudget.nodes != 0 {
		t.Fatalf("zero-budget wrapper=%+v", zeroBudget)
	}
	reported := 0
	rejected := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool {
		reported++
		return reported == 1
	})
	rejectedResult := rejected.Run(2)
	legacyReported := 0
	legacyRejected := constellationRootPackingMRVLegacy(catalog, instances, options, root, config, gridMask, 2, func(bool) bool {
		legacyReported++
		return legacyReported == 1
	})
	if !rejected.Done() || !reflect.DeepEqual(rejectedResult, legacyRejected) {
		t.Fatalf("reporter exhaustion result=%+v legacy=%+v", rejectedResult, legacyRejected)
	}
}

func TestConstellationRootPackingSessionTerminalProjectionMatchesLegacyCutoffs(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	for allocation := int64(0); allocation <= 12; allocation++ {
		t.Run("allocation_"+strconv.FormatInt(allocation, 10), func(t *testing.T) {
			session := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true })
			result := session.Run(allocation)
			if !session.Done() {
				result = session.FinalizeBudgetExhausted()
			}
			legacy := constellationRootPackingMRVLegacy(catalog, instances, options, root, config, gridMask, allocation, func(bool) bool { return true })
			if !reflect.DeepEqual(result, legacy) {
				t.Fatalf("allocation=%d terminal=%+v legacy=%+v", allocation, result, legacy)
			}
		})
	}
}

func TestConstellationRootPackingSessionMatchesFullRunMidOptionWithShadow(t *testing.T) {
	catalog, instances, options, root, config, gridMask := constellationRootPackingSessionFixture()
	baseline := newConstellationRootPackingSession(catalog, instances, options, root, config, gridMask, func(bool) bool { return true }).Run(20)
	if len(baseline.solutions) == 0 {
		t.Fatalf("baseline=%+v", baseline)
	}

	fullConfig := config
	fullConfig.constellationRootPackingCollector = &constellationRootPackingCollector{shadow: newConstellationShadowReference(root, baseline.solutions[0], "session-test")}
	full := newConstellationRootPackingSession(catalog, instances, options, root, fullConfig, gridMask, func(bool) bool { return true }).Run(20)
	legacyConfig := config
	legacyConfig.constellationRootPackingCollector = &constellationRootPackingCollector{shadow: newConstellationShadowReference(root, baseline.solutions[0], "session-test")}
	legacy := constellationRootPackingMRVLegacy(catalog, instances, options, root, legacyConfig, gridMask, 20, func(bool) bool { return true })
	if !reflect.DeepEqual(full, legacy) {
		t.Fatalf("session shadow=%+v legacy shadow=%+v", full, legacy)
	}

	partitionedConfig := config
	partitionedConfig.constellationRootPackingCollector = &constellationRootPackingCollector{shadow: newConstellationShadowReference(root, baseline.solutions[0], "session-test")}
	partitionedSession := newConstellationRootPackingSession(catalog, instances, options, root, partitionedConfig, gridMask, func(bool) bool { return true })
	partitionedSession.Run(1)
	if !partitionedSession.depthActive || partitionedSession.stateIndex != 0 || !partitionedSession.statePrepared || partitionedSession.optionIndex != 1 {
		t.Fatalf("first allocation was not mid-option: session=%+v", partitionedSession)
	}
	depthBoundary := partitionedSession.Run(2)
	if partitionedSession.depthActive || len(depthBoundary.mrvDepths) != 1 {
		t.Fatalf("second allocation did not complete first depth: session=%+v result=%+v", partitionedSession, depthBoundary)
	}
	partitionedSession.Run(1)
	if !partitionedSession.depthActive || partitionedSession.stateIndex != 0 || !partitionedSession.statePrepared || partitionedSession.optionIndex != 1 {
		t.Fatalf("third allocation was not mid-depth/mid-option: session=%+v", partitionedSession)
	}
	partitioned := partitionedSession.Run(16)
	if !partitionedSession.Done() || full.shadowTrace == nil || !reflect.DeepEqual(full, partitioned) {
		t.Fatalf("full=%+v partitioned=%+v", full, partitioned)
	}
}

func TestConstellationV3UsesMRVWithoutDiagnosticProbe(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"left":   {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right":  {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"filler": {ID: "filler", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"left", "right", "food", "filler"})
	grid := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	config := Config{
		TopN:                     3,
		AllowSkips:               false,
		MaxNodes:                 1_000,
		Diagnostics:              true,
		ConstellationSeedVariant: ConstellationSeedVariantV3,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:left", "star_source:right"},
	}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	config.policy = &policy
	config.priorityBounds = newPriorityBoundContext(catalog, instances, config.Priorities, config.PrioritySemantics)
	potential := newStarPotentialContext(catalog, instances, options, config.Priorities, config.PrioritySemantics)
	first, firstDiagnostics := constellationSeedSearch(catalog, instances, options, config, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	second, secondDiagnostics := constellationSeedSearch(catalog, instances, options, config, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(firstDiagnostics, secondDiagnostics) {
		t.Fatal("V3 MRV packing was not deterministic")
	}
	withReference := config
	withReference.DiagnosticReference = []model.Placement{{InstanceID: "filler#3", ItemID: "filler"}}
	withReferenceSeed, withReferenceDiagnostics := constellationSeedSearch(catalog, instances, options, withReference, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(first, withReferenceSeed) || !reflect.DeepEqual(firstDiagnostics, withReferenceDiagnostics) {
		t.Fatal("diagnostic reference changed V3 MRV packing")
	}
	if firstDiagnostics.Version != ConstellationSeedVariantV3 || len(firstDiagnostics.Roots) == 0 {
		t.Fatalf("V3 diagnostics=%+v", firstDiagnostics)
	}
	root := firstDiagnostics.Roots[0]
	if !root.Completed || root.PackingStrategy != constellationPackingStrategyStateMRV || root.FirstCompleteNodes == 0 || root.DistinctNextItemsSelected == 0 || len(root.MRVDepths) == 0 || root.ProbeNodesAvailable != nil || root.ProbeNodesConsumed != nil || root.ProbeNodesReturned != nil || root.SearchExhausted != nil || root.WitnessHash != "" {
		t.Fatalf("V3 root=%+v", root)
	}

	withOptimization := config
	withOptimization.ConstellationCompletionOptimizationProbe = true
	optimizationPolicy := resolveSearchPolicy(withOptimization, withOptimization.MaxNodes)
	withOptimization.policy = &optimizationPolicy
	optimizedSeed, optimizedDiagnostics := constellationSeedSearch(catalog, instances, options, withOptimization, grid, optimizationPolicy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(first.Solutions, optimizedSeed.Solutions) {
		t.Fatalf("optimization probe altered seed candidates normal=%+v optimized=%+v", first.Solutions, optimizedSeed.Solutions)
	}
	optimizedRoot := optimizedDiagnostics.Roots[0]
	if optimizedRoot.ExactCompletionEligible == nil || !*optimizedRoot.ExactCompletionEligible || optimizedRoot.ExactCompletionNodesAvailable == nil || optimizedRoot.ExactCompletionNodesConsumed == nil || optimizedRoot.ExactCompletionNodesReturned == nil || optimizedRoot.ExactCompletionStatus != "optimal_proven" || optimizedRoot.ExactCompletionSearchExhausted == nil || !*optimizedRoot.ExactCompletionSearchExhausted || optimizedRoot.ExactCompletionBestScore == nil || *optimizedRoot.ExactCompletionNodesReturned != *optimizedRoot.ExactCompletionNodesAvailable-*optimizedRoot.ExactCompletionNodesConsumed || optimizedRoot.ExactCompletionInitialIncumbentFromRootPacking == nil || !*optimizedRoot.ExactCompletionInitialIncumbentFromRootPacking {
		t.Fatalf("optimization root=%+v", optimizedRoot)
	}
	withOptimizationReference := withOptimization
	withOptimizationReference.DiagnosticReference = []model.Placement{{InstanceID: "filler#3", ItemID: "filler"}}
	optimizedReferenceSeed, optimizedReferenceDiagnostics := constellationSeedSearch(catalog, instances, options, withOptimizationReference, grid, optimizationPolicy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(optimizedSeed, optimizedReferenceSeed) || !reflect.DeepEqual(optimizedDiagnostics, optimizedReferenceDiagnostics) {
		t.Fatal("diagnostic reference changed V3 completion optimization probe")
	}
}

func TestConstellationFeasibilityProbeChangesPolicyFingerprint(t *testing.T) {
	base := Config{MaxNodes: 5_000_000, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	withProbe := base
	withProbe.ConstellationFeasibilityProbe = true
	if resolvedPolicyFingerprint(resolveSearchPolicy(base, base.MaxNodes)) == resolvedPolicyFingerprint(resolveSearchPolicy(withProbe, withProbe.MaxNodes)) {
		t.Fatal("probe selection did not change resolved policy fingerprint")
	}
	withOptimization := base
	withOptimization.ConstellationSeedVariant = ConstellationSeedVariantV3
	withOptimization.ConstellationCompletionOptimizationProbe = true
	withoutOptimization := withOptimization
	withoutOptimization.ConstellationCompletionOptimizationProbe = false
	if resolvedPolicyFingerprint(resolveSearchPolicy(withOptimization, withOptimization.MaxNodes)) == resolvedPolicyFingerprint(resolveSearchPolicy(withoutOptimization, withoutOptimization.MaxNodes)) {
		t.Fatal("completion optimization selection did not change resolved policy fingerprint")
	}
	v3Policy := resolveSearchPolicy(withOptimization, withOptimization.MaxNodes)
	if v3Policy.ConstellationCompletionOptimizationProbeScope != constellationOptimizationScopeFirstTwo || !constellationCompletionOptimizationProbeTargetsRoot(v3Policy, 0) || constellationCompletionOptimizationProbeTargetsRoot(v3Policy, 2) {
		t.Fatalf("V3 optimization scope=%q", v3Policy.ConstellationCompletionOptimizationProbeScope)
	}
	v4Config := withOptimization
	v4Config.ConstellationSeedVariant = ConstellationSeedVariantV4
	v4Policy := resolveSearchPolicy(v4Config, v4Config.MaxNodes)
	if v4Policy.ConstellationCompletionOptimizationProbeScope != constellationOptimizationScopeAll || !constellationCompletionOptimizationProbeTargetsRoot(v4Policy, 3) {
		t.Fatalf("V4 optimization scope=%q", v4Policy.ConstellationCompletionOptimizationProbeScope)
	}
}

func TestConstellationSeedEligibilityIsNarrowAndOracleBlind(t *testing.T) {
	base := Config{
		EnableConstellationSeedV1: true,
		PrioritySemantics:         model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:                []string{"star_source:left", "star_source:right"},
	}
	if !constellationSeedEligible(base, 1_000) {
		t.Fatal("eligible V3 profile did not resolve constellation policy")
	}
	withCoverage := base
	withCoverage.CoverageGroups = []model.CoverageGroup{{Name: "coverage", Sources: []string{"left"}}}
	if constellationSeedEligible(withCoverage, 1_000) {
		t.Fatal("coverage profile enabled constellation seed")
	}
	manyPriorities := base
	manyPriorities.Priorities = append(manyPriorities.Priorities, "star_source:third")
	if constellationSeedEligible(manyPriorities, 1_000) {
		t.Fatal("three-priority profile enabled constellation seed")
	}
	debugProfile := base
	debugProfile.Priorities = []string{"star_source:a", "star_source:b", "star_source:c", "star_source:d", "star_source:e", "star_source:f", "star_source:g", "star_source:h", "star_source:i", "star_source:j", "star_source:k", "star_source:l"}
	if constellationSeedEligible(debugProfile, 1_000) || resolveSearchPolicy(debugProfile, 1_000_000).ConstellationSeedVersion != "" {
		t.Fatal("twelve-priority debug profile enabled constellation seed")
	}
	legacy := base
	legacy.PrioritySemantics = model.PrioritySemanticsOutgoingV2
	if constellationSeedEligible(legacy, 1_000) {
		t.Fatal("V2 profile enabled constellation seed")
	}
}

func TestConstellationPolicyUsesFifteenPercentWithoutChangingPacking(t *testing.T) {
	config := Config{
		EnableConstellationSeedV1: true,
		PrioritySemantics:         model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:                []string{"star_source:left", "star_source:right"},
	}
	policy := resolveSearchPolicy(config, 1_000_000)
	if policy.ConstellationSeedNodeBudget != 37_500 || policy.PackingSeedNodeBudget != 87_500 || policy.StarSeedNodeBudget-policy.PackingSeedNodeBudget-policy.ConstellationSeedNodeBudget != 125_000 {
		t.Fatalf("seed partition=%+v", policy)
	}
	if policy.ConstellationSeedShareBps != 1_500 || policy.ConstellationSeedVersion != "v1" || policy.ConstellationSeedPackingBeamWidth != 64 {
		t.Fatalf("constellation policy=%+v", policy)
	}
	disabled := config
	disabled.EnableConstellationSeedV1 = false
	if got := resolveSearchPolicy(disabled, 1_000_000); got.ConstellationSeedNodeBudget != 0 || got.PackingSeedNodeBudget != 125_000 || got.StarSeedNodeBudget-got.PackingSeedNodeBudget != 125_000 {
		t.Fatalf("disabled policy=%+v", got)
	}
}

func TestConstellationDisabledPolicyAndDiagnosticReferenceDoNotAlterSeed(t *testing.T) {
	config := Config{
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:left", "star_source:right"},
	}
	baseline := resolveSearchPolicy(config, 1_000_000)
	explicitDisabled := config
	explicitDisabled.EnableConstellationSeedV1 = false
	if got := resolveSearchPolicy(explicitDisabled, 1_000_000); !reflect.DeepEqual(got, baseline) {
		t.Fatalf("explicit disabled policy=%+v want %+v", got, baseline)
	}

	catalog := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"left", "right", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	enabled := Config{TopN: 3, AllowSkips: false, MaxNodes: 1_000, EnableConstellationSeedV1: true, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: config.Priorities}
	policy := resolveSearchPolicy(enabled, enabled.MaxNodes)
	enabled.policy = &policy
	enabled.priorityBounds = newPriorityBoundContext(catalog, instances, enabled.Priorities, enabled.PrioritySemantics)
	potential := newStarPotentialContext(catalog, instances, options, enabled.Priorities, enabled.PrioritySemantics)
	first, firstDiagnostics := constellationSeedSearch(catalog, instances, options, enabled, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	withReference := enabled
	withReference.DiagnosticReference = []model.Placement{{InstanceID: "left#0", ItemID: "left"}}
	second, secondDiagnostics := constellationSeedSearch(catalog, instances, options, withReference, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	if first.NodesExplored != second.NodesExplored || first.CandidateCount != second.CandidateCount || !reflect.DeepEqual(first.Solutions, second.Solutions) || !reflect.DeepEqual(firstDiagnostics, secondDiagnostics) {
		t.Fatal("diagnostic reference changed constellation search")
	}
}

func TestConstellationSeedReportsChargedDiagnosticPhase(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	solutions, err := SolveLayout(catalog, []string{"left", "right", "food"}, grid, Config{
		TopN:                      1,
		AllowSkips:                false,
		MaxNodes:                  1_000,
		Workers:                   1,
		Diagnostics:               true,
		EnableConstellationSeedV1: true,
		PrioritySemantics:         model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:                []string{"star_source:left", "star_source:right"},
	})
	if err != nil || len(solutions) != 1 {
		t.Fatalf("constellation solve results=%d err=%v", len(solutions), err)
	}
	stats := solutions[0].Search
	if stats.ConstellationSeedNodes == 0 || stats.ConstellationSeedDiagnostics.SkeletonsDistinct == 0 {
		t.Fatalf("constellation stats=%+v", stats)
	}
	var charged int64
	var constellationPhase int64
	for _, phase := range stats.PhaseWork {
		charged += phase.ChargedNodes
		if phase.Phase == tracePhaseConstellationSeed {
			constellationPhase = phase.ChargedNodes
		}
	}
	if constellationPhase != stats.ConstellationSeedNodes || charged != stats.GlobalBudgetConsumed || charged > 1_000 {
		t.Fatalf("charged=%d constellation=%d/%d global=%d", charged, constellationPhase, stats.ConstellationSeedNodes, stats.GlobalBudgetConsumed)
	}
	settings := SettingsForBenchmarkConfig(Config{MaxNodes: 1_000, EnableConstellationSeedV1: true, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}})
	if settings.ConstellationSeedVersion != "v1" || settings.ConstellationSeedNodeBudget == 0 {
		t.Fatalf("benchmark settings=%+v", settings)
	}
}

func TestPriorityCeilingDoesNotSuppressEligibleRepair(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"left", "right", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	placements := []model.Placement{
		testPlacement(t, options["left#0"], model.Coord{Col: 0}, 0),
		testPlacement(t, options["right#1"], model.Coord{Col: 2}, 0),
		testPlacement(t, options["food#2"], model.Coord{Col: 1}, 0),
	}
	config := Config{TopN: 1, AllowSkips: false, RepairSearch: true, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	incumbent := model.Solution{Placements: placements, Evaluation: evaluateLayoutForConfig(catalog, placements, config), LayoutKey: layoutKey(placements, instances), CanonicalLayoutHash: canonicalLayoutHash(placements)}
	if !reflect.DeepEqual(incumbent.Evaluation.Score.PriorityCounts, []int{1, 1}) {
		t.Fatalf("incumbent priority=%v", incumbent.Evaluation.Score.PriorityCounts)
	}
	result := repairSearch(catalog, instances, options, config, nil, nil, grid, 10_000, []model.Solution{incumbent}, nil)
	if result.TerminationReason != "no_eligible_neighborhoods" || result.NodesExplored != 0 {
		t.Fatalf("priority ceiling repair=%+v", result)
	}
}

func TestConstellationSourcePoolContainsCanonicalFixtureSources(t *testing.T) {
	cat := loadRuntimeCatalogForConstellationTest(t)
	items := []string{
		"banana", "cactrio", "champion_s_ripper", "cleansing_crown", "death_essence", "discordant_harp", "donut",
		"ginseng_root", "ginseng_root", "green_snapper", "hooded_cowl", "longing_begonia", "pitahaya", "pitahaya",
		"spice", "spice", "spice", "spicy_sausage", "spiked_sickle", "spirit_biscuit", "steadfast_boots", "tender_sausage",
		"thornwall", "twinmaw",
	}
	instances := ExpandInventory(items)
	options := testOptionsByInstance(t, cat, instances)
	config := Config{EnableConstellationSeedV1: true, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:spirit_biscuit", "star_source:spice"}}
	policy := resolveSearchPolicy(config, 1_000_000)
	sources, _ := constellationSources(cat, instances, config.Priorities)
	fixture := OutgoingPerInstanceFoodDiagnosticReference()
	byID := placementByInstanceID(fixture)
	spices := []model.Placement{byID["spice#14"], byID["spice#15"], byID["spice#16"]}
	sort.Slice(spices, func(i, j int) bool { return placementKey(spices[i]) < placementKey(spices[j]) })
	spiceIndex := 0
	for _, source := range sources {
		want := byID[source.InstanceID]
		if source.ItemID == "spice" {
			want = spices[spiceIndex]
			spiceIndex++
		}
		found := false
		for _, option := range constellationSourceOptions(options[source.InstanceID], 0, policy.ConstellationSeedSourceOptionLimit, source, sources) {
			if option.Rotation == want.Rotation && option.Origin == want.Origin {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("source pool omitted fixture-compatible %s at %s", source.InstanceID, placementKey(want))
		}
	}
}

func TestConstellationPrioritySourceGeometryKeyIgnoresCopyLabels(t *testing.T) {
	sources := map[string]struct{}{"spirit": {}, "spice": {}}
	base := []model.Placement{
		{InstanceID: "spirit#0", ItemID: "spirit", Cells: []model.Coord{{Row: 0, Col: 0}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 0, Col: 1}}}},
		{InstanceID: "spice#1", ItemID: "spice", Cells: []model.Coord{{Row: 1, Col: 0}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 1, Col: 1}}}},
		{InstanceID: "spice#2", ItemID: "spice", Cells: []model.Coord{{Row: 1, Col: 2}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 1, Col: 3}}}},
		{InstanceID: "spice#3", ItemID: "spice", Cells: []model.Coord{{Row: 2, Col: 1}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 2, Col: 2}}}},
		{InstanceID: "target#4", ItemID: "target", Cells: []model.Coord{{Row: 3, Col: 3}}},
	}
	permuted := append([]model.Placement(nil), base...)
	permuted[1].InstanceID, permuted[2].InstanceID = permuted[2].InstanceID, permuted[1].InstanceID
	if got, want := constellationPrioritySourceGeometryKey(permuted, sources), constellationPrioritySourceGeometryKey(base, sources); got != want {
		t.Fatalf("permuted source geometry=%q want %q", got, want)
	}
	moved := append([]model.Placement(nil), base...)
	moved[1].Cells = []model.Coord{{Row: 1, Col: 1}}
	moved[1].StarPositions = []model.StarPosition{{Position: model.Coord{Row: 1, Col: 2}}}
	if constellationPrioritySourceGeometryKey(moved, sources) == constellationPrioritySourceGeometryKey(base, sources) {
		t.Fatal("moved Spice retained source geometry key")
	}
	targetMoved := append([]model.Placement(nil), base...)
	targetMoved[4].Cells = []model.Coord{{Row: 4, Col: 4}}
	if constellationPrioritySourceGeometryKey(targetMoved, sources) != constellationPrioritySourceGeometryKey(base, sources) {
		t.Fatal("unrelated target changed source geometry key")
	}
}

func TestConstellationTopBottomGeometryOrbitPreservesSourceStarGeometry(t *testing.T) {
	sources := map[string]struct{}{"spirit": {}, "spice": {}}
	base := []model.Placement{
		{ItemID: "spirit", Cells: []model.Coord{{Row: 1, Col: 3}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 0, Col: 3}}, {Position: model.Coord{Row: 2, Col: 4}}}},
		{ItemID: "spice", Cells: []model.Coord{{Row: 4, Col: 4}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 3, Col: 4}}, {Position: model.Coord{Row: 5, Col: 4}}}},
	}
	reflected := make([]model.Placement, 0, len(base))
	for _, placement := range base {
		copy := model.Placement{ItemID: placement.ItemID}
		for _, cell := range placement.Cells {
			copy.Cells = append(copy.Cells, topBottomReflectedCoord(cell))
		}
		for _, star := range placement.StarPositions {
			copy.StarPositions = append(copy.StarPositions, model.StarPosition{Position: topBottomReflectedCoord(star.Position)})
		}
		reflected = append(reflected, copy)
	}
	if constellationPrioritySourceGeometryKey(base, sources) == constellationPrioritySourceGeometryKey(reflected, sources) {
		t.Fatal("raw geometry key collapsed top-bottom reflection")
	}
	if constellationPrioritySourceGeometryOrbitKey(base, sources) != constellationPrioritySourceGeometryOrbitKey(reflected, sources) {
		t.Fatal("top-bottom reflected geometry has different orbit key")
	}
	axis := []model.Placement{{ItemID: "spirit", Cells: []model.Coord{{Row: 2, Col: 4}, {Row: 3, Col: 4}}, StarPositions: []model.StarPosition{{Position: model.Coord{Row: 2, Col: 3}}, {Position: model.Coord{Row: 3, Col: 3}}}}}
	if constellationPrioritySourceGeometryOrbitKey(axis, sources) != constellationPrioritySourceGeometryKey(axis, sources) {
		t.Fatal("top-bottom symmetric geometry did not retain raw key")
	}
	changed := append([]model.Placement(nil), base...)
	changed[0].StarPositions = []model.StarPosition{{Position: model.Coord{Row: 0, Col: 4}}}
	if constellationPrioritySourceGeometryOrbitKey(base, sources) == constellationPrioritySourceGeometryOrbitKey(changed, sources) {
		t.Fatal("changed source star retained orbit key")
	}
	mask := (uint64(1) << 3) | (uint64(1) << 49)
	if topBottomReflectedMask(topBottomReflectedMask(mask)) != mask || topBottomMaskOrbitKey(mask) != topBottomMaskOrbitKey(topBottomReflectedMask(mask)) {
		t.Fatal("top-bottom free-mask orbit is not an involution")
	}
}

func TestConstellationShadowReferenceMatchesUnanchoredCopyClasses(t *testing.T) {
	placement := func(instanceID string, originalIndex int, col int) model.Placement {
		return model.Placement{InstanceID: instanceID, ItemID: "copy", OriginalIndex: originalIndex, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	witness := model.Solution{Placements: []model.Placement{placement("copy#0", 0, 0), placement("copy#1", 1, 1)}}
	shadow := newConstellationShadowReference(constellationSkeleton{}, witness, "semantic")
	if !shadow.compatible([]model.Placement{placement("copy#1", 1, 0)}) {
		t.Fatal("shadow rejected physically compatible swapped copy")
	}
	if shadow.compatible([]model.Placement{placement("copy#1", 1, 2)}) {
		t.Fatal("shadow accepted placement outside witness multiset")
	}
	anchored := constellationSkeleton{placed: []model.Placement{placement("copy#0", 0, 0)}}
	anchoredShadow := newConstellationShadowReference(anchored, witness, "semantic")
	if anchoredShadow.compatible([]model.Placement{placement("copy#0", 0, 1)}) {
		t.Fatal("shadow accepted an anchored literal mismatch")
	}
}

func TestPackingComparatorTraceAgreesWithPackingOrder(t *testing.T) {
	base := packingSeedState{
		restricted:    2,
		flexibility:   3,
		fragmentation: 2,
		score: model.Score{
			PriorityCounts:                []int{1, 1},
			CraftCount:                    1,
			StarCount:                     2,
			ItemCount:                     3,
			StarTargetBreadth:             2,
			StarReciprocalPairs:           1,
			StarSourceDefinitionDiversity: 2,
		},
		key: "z",
	}
	cases := []struct {
		name      string
		component string
		mutate    func(*packingSeedState)
	}{
		{"restricted", "restricted", func(state *packingSeedState) { state.restricted++ }},
		{"flexibility", "flexibility", func(state *packingSeedState) { state.flexibility++ }},
		{"fragmentation", "fragmentation", func(state *packingSeedState) { state.fragmentation-- }},
		{"priority", "priority_count", func(state *packingSeedState) { state.score.PriorityCounts[1]++ }},
		{"crafts", "craft_count", func(state *packingSeedState) { state.score.CraftCount++ }},
		{"stars", "star_count", func(state *packingSeedState) { state.score.StarCount++ }},
		{"items", "item_count", func(state *packingSeedState) { state.score.ItemCount++ }},
		{"breadth", "star_target_breadth", func(state *packingSeedState) { state.score.StarTargetBreadth++ }},
		{"reciprocal", "star_reciprocal_pairs", func(state *packingSeedState) { state.score.StarReciprocalPairs++ }},
		{"diversity", "star_source_definition_diversity", func(state *packingSeedState) { state.score.StarSourceDefinitionDiversity++ }},
		{"key", "key_lexical", func(state *packingSeedState) { state.key = "a" }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			left := base
			left.score = cloneScore(base.score)
			testCase.mutate(&left)
			decision := packingSeedStateFirstDecisive(left, base, true)
			if decision.component != testCase.component || decision.compare <= 0 || !packingSeedStateLess(left, base) || packingSeedStateLess(base, left) {
				t.Fatalf("decision=%+v left=%+v base=%+v", decision, left, base)
			}
			if testCase.component == "key_lexical" {
				if packingSeedStatePrimaryCompare(left, base) != 0 {
					t.Fatalf("key changed primary compare=%d", packingSeedStatePrimaryCompare(left, base))
				}
			} else if packingSeedStatePrimaryCompare(left, base) <= 0 {
				t.Fatalf("primary compare=%d want positive", packingSeedStatePrimaryCompare(left, base))
			}
		})
	}
}

func TestConstellationShadowRankingAutopsyDecomposesPrecutOrder(t *testing.T) {
	shadow := packingSeedState{restricted: 1, flexibility: 1, fragmentation: 2, score: model.Score{PriorityCounts: []int{1}, StarCount: 1}, key: "z"}
	states := []constellationRootMRVState{
		{packingSeedState: packingSeedState{restricted: 2, flexibility: 1, fragmentation: 2, score: cloneScore(shadow.score), key: "z"}},
		{packingSeedState: packingSeedState{restricted: 1, flexibility: 2, fragmentation: 2, score: cloneScore(shadow.score), key: "z"}},
		{packingSeedState: packingSeedState{restricted: 1, flexibility: 1, fragmentation: 1, score: cloneScore(shadow.score), key: "z"}},
		{packingSeedState: packingSeedState{restricted: 1, flexibility: 1, fragmentation: 2, score: cloneScore(shadow.score), key: "a"}},
		{packingSeedState: shadow},
	}
	autopsy := constellationShadowRankingAutopsy(states, 4, 3, Config{})
	if autopsy == nil || autopsy.StrictlyPrecedingStates != 4 || autopsy.FullComparatorTieBeforeStates != 0 || autopsy.ShadowComponents.Key != "z" || autopsy.CutoffComponents == nil || autopsy.BestComponents == nil {
		t.Fatalf("autopsy=%+v", autopsy)
	}
	decisive := map[string]model.ConstellationShadowDecisiveComponent{}
	for _, component := range autopsy.FirstDecisiveComponents {
		key := component.Component
		if component.PriorityIndex != nil {
			key += ":" + strconv.Itoa(*component.PriorityIndex)
		}
		decisive[key] = component
	}
	for _, key := range []string{"restricted", "flexibility", "fragmentation", "key_lexical"} {
		if decisive[key].BetterAtComponentCount != 1 {
			t.Fatalf("component %s=%+v", key, decisive[key])
		}
	}
	for index := 0; index < 4; index++ {
		decision := packingSeedStateFirstDecisive(states[index].packingSeedState, shadow, true)
		if decision.compare <= 0 || !packingSeedStateLess(states[index].packingSeedState, shadow) || packingSeedStateLess(shadow, states[index].packingSeedState) {
			t.Fatalf("autopsy pair %d decision=%+v", index, decision)
		}
	}
	var decisiveTotal int
	for _, component := range autopsy.FirstDecisiveComponents {
		decisiveTotal += component.BetterAtComponentCount
	}
	if decisiveTotal != autopsy.StrictlyPrecedingStates {
		t.Fatalf("decisive total=%d strictly preceding=%d", decisiveTotal, autopsy.StrictlyPrecedingStates)
	}
	if decisive["restricted"].PrefixEqualCount != 5 || decisive["flexibility"].PrefixEqualCount != 4 || decisive["fragmentation"].PrefixEqualCount != 3 || decisive["key_lexical"].PrefixEqualCount != 2 {
		t.Fatalf("prefix decomposition=%+v", decisive)
	}
	if decisive["restricted"].AdvantageP50 == nil || *decisive["restricted"].AdvantageP50 != 1 || decisive["key_lexical"].AdvantageP50 != nil {
		t.Fatalf("advantages restricted=%+v key=%+v", decisive["restricted"], decisive["key_lexical"])
	}
}

func TestConstellationForcedRootPackingPriorityScoreFirstRanking(t *testing.T) {
	priorityFirst := packingSeedState{
		restricted: 1,
		score:      model.Score{PriorityCounts: []int{2, 1}, StarCount: 5},
		key:        "a",
	}
	packingFirst := packingSeedState{
		restricted: 2,
		score:      model.Score{PriorityCounts: []int{1, 1}, StarCount: 6},
		key:        "b",
	}
	if constellationRootPackingStateLess(Config{forcedRootPackingReplay: true}, priorityFirst, packingFirst) {
		t.Fatal("baseline ranking preferred priority over feasibility")
	}
	config := Config{
		ConstellationForcedCandidateRootedPackingRanking: constellationRootPackingRankingPriorityScoreFirst,
		forcedRootPackingReplay:                          true,
	}
	if !constellationRootPackingStateLess(config, priorityFirst, packingFirst) || constellationRootPackingStateLess(config, packingFirst, priorityFirst) {
		t.Fatal("priority-score-first ranking did not prefer priority")
	}
	components := constellationRootPackingComparatorComponents(config, 2, true)
	if len(components) == 0 || components[0].name != "priority_count" || components[len(components)-1].name != "key_lexical" {
		t.Fatalf("components=%+v", components)
	}
}

func TestValidateConstellationForcedCandidateRootedPackingRanking(t *testing.T) {
	for _, ranking := range []string{"", constellationRootPackingRankingBaseline, constellationRootPackingRankingPriorityScoreFirst} {
		if err := ValidateConstellationForcedCandidateRootedPackingRanking(ranking); err != nil {
			t.Fatalf("ranking %q: %v", ranking, err)
		}
	}
	if err := ValidateConstellationForcedCandidateRootedPackingRanking("invalid"); err == nil {
		t.Fatal("invalid ranking was accepted")
	}
}

func TestConstellationShadowCounterfactualRerankingUsesExistingComponents(t *testing.T) {
	placement := func(col int) model.Placement {
		return model.Placement{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	shadowPlacement := placement(2)
	shadowReference := &constellationShadowReference{
		anchoredByInstance: map[string]string{},
		unanchoredByItem:   map[string]map[string]int{"item": {placementKey(shadowPlacement): 1}},
	}
	shadow := packingSeedState{restricted: 1, flexibility: 10, fragmentation: 2, score: model.Score{PriorityCounts: []int{1}, StarCount: 10}, key: "z"}
	states := []constellationRootMRVState{
		{packingSeedState: packingSeedState{restricted: 3, flexibility: 1, fragmentation: 2, score: model.Score{PriorityCounts: []int{1}, StarCount: 1}, key: "a", placed: []model.Placement{placement(0)}}},
		{packingSeedState: packingSeedState{restricted: 2, flexibility: 1, fragmentation: 2, score: model.Score{PriorityCounts: []int{1}, StarCount: 1}, key: "b", placed: []model.Placement{placement(1)}}},
		{packingSeedState: packingSeedState{restricted: shadow.restricted, flexibility: shadow.flexibility, fragmentation: shadow.fragmentation, score: cloneScore(shadow.score), key: shadow.key, placed: []model.Placement{shadowPlacement}}},
	}
	before := append([]constellationRootMRVState(nil), states...)
	reranking := constellationShadowCounterfactualReranking(states, shadowReference, 2)
	if !reflect.DeepEqual(states, before) || reranking == nil || len(reranking.Variants) != 4 {
		t.Fatalf("reranking=%+v states=%+v want %+v", reranking, states, before)
	}
	byID := map[string]model.ConstellationForcedCandidateCounterfactualVariant{}
	for _, variant := range reranking.Variants {
		byID[variant.ID] = variant
		var decisiveTotal int
		for _, component := range variant.FirstDecisiveComponents {
			decisiveTotal += component.BetterAtComponentCount
		}
		if decisiveTotal != variant.StrictlyPrecedingStates {
			t.Fatalf("variant %s decisive=%d predecessors=%d", variant.ID, decisiveTotal, variant.StrictlyPrecedingStates)
		}
	}
	if baseline := byID["baseline"]; baseline.FullRankStart != 3 || baseline.ActualBeamFit || baseline.FullPossibleBeamFit || baseline.KeylessPossibleBeamFit {
		t.Fatalf("baseline=%+v", baseline)
	}
	for _, id := range []string{"B", "D"} {
		variant := byID[id]
		if variant.FullRankStart != 1 || !variant.ActualBeamFit || !variant.FullGuaranteedBeamFit || variant.ComparatorTuple == "" {
			t.Fatalf("variant %s=%+v", id, variant)
		}
	}
	if variant := byID["C"]; variant.FullRankStart != 3 || variant.ActualBeamFit {
		t.Fatalf("variant C=%+v", variant)
	}

	zeroPadded := packingSeedState{score: model.Score{PriorityCounts: []int{1}}, key: "a"}
	withZero := packingSeedState{score: model.Score{PriorityCounts: []int{1, 0}}, key: "b"}
	for _, variant := range packingCounterfactualVariants(2) {
		decision := packingSeedStateVariantFirstDecisive(zeroPadded, withZero, variant.components)
		if decision.component != "key_lexical" || decision.compare <= 0 {
			t.Fatalf("variant %s priority padding decision=%+v", variant.id, decision)
		}
	}
}

func TestConstellationCounterfactualRerankingDistinguishesActualAndKeylessFit(t *testing.T) {
	placement := func(col int) model.Placement {
		return model.Placement{InstanceID: "item#0", ItemID: "item", Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	shadowPlacement := placement(1)
	shadowReference := &constellationShadowReference{anchoredByInstance: map[string]string{}, unanchoredByItem: map[string]map[string]int{"item": {placementKey(shadowPlacement): 1}}}
	shared := model.Score{PriorityCounts: []int{1}, StarCount: 1}
	states := []constellationRootMRVState{
		{packingSeedState: packingSeedState{score: cloneScore(shared), key: "a", placed: []model.Placement{placement(0)}}},
		{packingSeedState: packingSeedState{score: cloneScore(shared), key: "z", placed: []model.Placement{shadowPlacement}}},
	}
	reranking := constellationShadowCounterfactualReranking(states, shadowReference, 1)
	baseline := reranking.Variants[0]
	if baseline.FullRankStart != 2 || baseline.FullRankEnd != 2 || !baseline.KeylessPossibleBeamFit || !baseline.KeylessTieCrossesBeam || baseline.KeylessGuaranteedBeamFit || baseline.ActualBeamFit {
		t.Fatalf("keyless fit=%+v", baseline)
	}
}

func TestConstellationV4SelectsOrbitDiversityBeforeRawFallbacks(t *testing.T) {
	makeSkeleton := func(exact, raw, orbit, target string) constellationSkeleton {
		return constellationSkeleton{exactKey: exact, signature: exact, sourceGeometryKey: raw, sourceGeometryOrbitKey: orbit, targetAssignmentKey: target, score: model.Score{PriorityCounts: []int{1, 1}, StarCount: 1}}
	}
	candidates := map[string]constellationSkeleton{
		"a":           makeSkeleton("a", "raw-a", "orbit-a", "target-a"),
		"a-reflected": makeSkeleton("a-reflected", "raw-a-reflected", "orbit-a", "target-b"),
		"b":           makeSkeleton("b", "raw-b", "orbit-b", "target-c"),
		"c":           makeSkeleton("c", "raw-c", "orbit-c", "target-d"),
		"d":           makeSkeleton("d", "raw-d", "orbit-d", "target-e"),
	}
	selected := selectConstellationV4Skeletons(candidates, 4)
	if len(selected) != 4 {
		t.Fatalf("selected=%d want 4", len(selected))
	}
	if _, left := selected["a"]; !left {
		if _, reflected := selected["a-reflected"]; !reflected {
			t.Fatal("missing orbit-a representative")
		}
	}
	if _, a := selected["a"]; a {
		if _, reflected := selected["a-reflected"]; reflected {
			t.Fatal("selected both reflected orbit-a skeletons before distinct orbits")
		}
	}
	for _, key := range []string{"b", "c", "d"} {
		if _, exists := selected[key]; !exists {
			t.Fatalf("missing distinct orbit candidate %q", key)
		}
	}

	fallback := selectConstellationV4Skeletons(map[string]constellationSkeleton{
		"a":           makeSkeleton("a", "raw-a", "orbit-a", "target-a"),
		"a-reflected": makeSkeleton("a-reflected", "raw-a-reflected", "orbit-a", "target-b"),
	}, 2)
	if len(fallback) != 2 {
		t.Fatalf("raw fallback selected=%d want 2", len(fallback))
	}
	targetFallback := selectConstellationV4Skeletons(map[string]constellationSkeleton{
		"a": makeSkeleton("a", "raw-a", "orbit-a", "target-a"),
		"b": makeSkeleton("b", "raw-a", "orbit-a", "target-b"),
	}, 2)
	if len(targetFallback) != 2 {
		t.Fatalf("target fallback selected=%d want 2", len(targetFallback))
	}
}

func TestConstellationRelaxationFrontierRequiresMatchingAnchorsAndSignals(t *testing.T) {
	placement := func(instanceID string, col int) model.Placement {
		return model.Placement{InstanceID: instanceID, ItemID: instanceID, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	makeSkeleton := func(exact string, placements []model.Placement, stars int) constellationSkeleton {
		return constellationSkeleton{
			exactKey:            exact,
			placed:              placements,
			score:               model.Score{PriorityCounts: []int{6, 12}, StarCount: stars, ItemCount: len(placements)},
			sourceGeometryKey:   "source",
			targetAssignmentKey: "targets",
		}
	}
	strict := makeSkeleton("strict", []model.Placement{placement("source#0", 0), placement("target#1", 1), placement("extra#2", 2)}, 50)
	lessRelaxed := makeSkeleton("less-relaxed", []model.Placement{placement("source#0", 0), placement("target#1", 1)}, 50)
	frontier := makeSkeleton("frontier", []model.Placement{placement("source#0", 0)}, 50)
	movedAnchor := makeSkeleton("moved", []model.Placement{placement("source#0", 0), placement("target#1", 3)}, 50)
	differentSignals := makeSkeleton("different", []model.Placement{placement("source#0", 0)}, 51)
	candidates := map[string]constellationSkeleton{
		strict.exactKey:           strict,
		lessRelaxed.exactKey:      lessRelaxed,
		frontier.exactKey:         frontier,
		movedAnchor.exactKey:      movedAnchor,
		differentSignals.exactKey: differentSignals,
	}
	selected := constellationRelaxationFrontier(candidates, strict)
	if len(selected) != 1 || selected[0].exactKey != frontier.exactKey {
		t.Fatalf("frontier=%+v", selected)
	}
	if !constellationSkeletonStrictlyRelaxes(frontier, strict) || constellationSkeletonStrictlyRelaxes(movedAnchor, strict) || constellationSkeletonStrictlyRelaxes(differentSignals, strict) {
		t.Fatalf("relaxation matching frontier=%v moved=%v signals=%v", constellationSkeletonStrictlyRelaxes(frontier, strict), constellationSkeletonStrictlyRelaxes(movedAnchor, strict), constellationSkeletonStrictlyRelaxes(differentSignals, strict))
	}
}

func TestConstellationV5ReplacesOneStrictRootWithRelaxationFrontier(t *testing.T) {
	placement := func(instanceID string, col int) model.Placement {
		return model.Placement{InstanceID: instanceID, ItemID: instanceID, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	makeSkeleton := func(exact, geometry, orbit string, placements []model.Placement) constellationSkeleton {
		return constellationSkeleton{
			exactKey:               exact,
			signature:              exact,
			placed:                 placements,
			score:                  model.Score{PriorityCounts: []int{6, 12}, StarCount: 50, ItemCount: len(placements)},
			sourceGeometryKey:      geometry,
			sourceGeometryOrbitKey: orbit,
			targetAssignmentKey:    "targets-" + geometry,
		}
	}
	strict := makeSkeleton("strict", "geometry-a", "orbit-a", []model.Placement{placement("source#0", 0), placement("target#1", 1)})
	relaxed := makeSkeleton("relaxed", "geometry-a", "orbit-a", []model.Placement{placement("source#0", 0)})
	other := []constellationSkeleton{
		makeSkeleton("b", "geometry-b", "orbit-b", []model.Placement{placement("b#0", 2)}),
		makeSkeleton("c", "geometry-c", "orbit-c", []model.Placement{placement("c#0", 3)}),
		makeSkeleton("d", "geometry-d", "orbit-d", []model.Placement{placement("d#0", 4)}),
	}
	candidates := map[string]constellationSkeleton{strict.exactKey: strict, relaxed.exactKey: relaxed}
	for _, candidate := range other {
		candidates[candidate.exactKey] = candidate
	}
	selected := selectConstellationV5Skeletons(candidates, 4)
	if len(selected) != 4 {
		t.Fatalf("selected roots=%d", len(selected))
	}
	if _, exists := selected[strict.exactKey]; exists {
		t.Fatal("strict root was retained alongside its frontier")
	}
	frontier, exists := selected[relaxed.exactKey]
	if !exists || frontier.selectionPolicy != "v5_relaxation_frontier" || frontier.relaxedFromExactKey != strict.exactKey || frontier.relaxationFrontierSize != 1 {
		t.Fatalf("frontier=%+v", frontier)
	}
	for _, candidate := range other {
		if _, exists := selected[candidate.exactKey]; !exists {
			t.Fatalf("non-frontier root %q was removed", candidate.exactKey)
		}
	}
	guarded := selectConstellationV51Skeletons(candidates, 4)
	if len(guarded) != 4 {
		t.Fatalf("guarded roots=%d", len(guarded))
	}
	parent, exists := guarded[strict.exactKey]
	if !exists || parent.selectionPolicy != "parent_guarded_frontier" || parent.frontierExactKey != relaxed.exactKey || parent.relaxationFrontierSize != 1 {
		t.Fatalf("guarded parent=%+v", parent)
	}
	if _, exists := guarded[relaxed.exactKey]; exists {
		t.Fatal("guarded selection created a fifth root")
	}
}

func TestConstellationV4ReportsOrbitDiversityAndIsOracleBlind(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"left":   {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right":  {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"filler": {ID: "filler", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"left", "right", "food", "filler"})
	grid := mustParseGridForTest(t, "111100000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	config := Config{TopN: 3, AllowSkips: false, MaxNodes: 1_000, Diagnostics: true, ConstellationSeedVariant: ConstellationSeedVariantV4, PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	policy := resolveSearchPolicy(config, config.MaxNodes)
	config.policy = &policy
	config.priorityBounds = newPriorityBoundContext(catalog, instances, config.Priorities, config.PrioritySemantics)
	potential := newStarPotentialContext(catalog, instances, options, config.Priorities, config.PrioritySemantics)
	first, diagnostics := constellationSeedSearch(catalog, instances, options, config, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	withReference := config
	withReference.DiagnosticReference = []model.Placement{{InstanceID: "filler#3", ItemID: "filler"}}
	second, withReferenceDiagnostics := constellationSeedSearch(catalog, instances, options, withReference, grid, policy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(first, second) || !reflect.DeepEqual(diagnostics, withReferenceDiagnostics) {
		t.Fatal("diagnostic reference changed V4 orbit selection")
	}
	if diagnostics.Version != ConstellationSeedVariantV4 || diagnostics.SelectedPrioritySourceGeometryCount != diagnostics.PrioritySourceGeometryCount || diagnostics.CandidatePrioritySourceGeometryCount < diagnostics.SelectedPrioritySourceGeometryCount || diagnostics.CandidatePrioritySourceGeometryOrbitCount > diagnostics.CandidatePrioritySourceGeometryCount || diagnostics.SelectedPrioritySourceGeometryOrbitCount > diagnostics.SelectedPrioritySourceGeometryCount || diagnostics.CandidateRootFreeMaskOrbitCount > diagnostics.CandidateRootFreeMaskCount {
		t.Fatalf("V4 diagnostics=%+v", diagnostics)
	}
	if diagnostics.RootsCompleted > 0 && (diagnostics.ConstellationRootWinnerID == "" || diagnostics.ConstellationRootWinnerScore == nil || diagnostics.ConstellationRootWinnerHash == "" || diagnostics.ConstellationSeedFinalScore == nil || diagnostics.ConstellationSeedFinalHash == "") {
		t.Fatalf("missing constellation winner telemetry=%+v", diagnostics)
	}
	for _, skeleton := range diagnostics.Skeletons {
		if skeleton.PrioritySourceGeometryOrbitKey == "" {
			t.Fatalf("missing V4 skeleton orbit=%+v", skeleton)
		}
	}
	for _, root := range diagnostics.Roots {
		if root.SourceGeometryOrbitKey == "" {
			t.Fatalf("missing V4 root orbit=%+v", root)
		}
	}
	if policy.ConstellationSeedPackingStrategy != constellationPackingStrategyStateMRV {
		t.Fatalf("V4 packing strategy=%q", policy.ConstellationSeedPackingStrategy)
	}
	withSweep := config
	withSweep.ConstellationCandidatePoolFeasibilitySweep = true
	sweepPolicy := resolveSearchPolicy(withSweep, withSweep.MaxNodes)
	withSweep.policy = &sweepPolicy
	sweepSeed, sweepDiagnostics := constellationSeedSearch(catalog, instances, options, withSweep, grid, sweepPolicy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(first.Solutions, sweepSeed.Solutions) {
		t.Fatalf("sweep changed normal V4 seed candidates normal=%+v sweep=%+v", first.Solutions, sweepSeed.Solutions)
	}
	sweep := sweepDiagnostics.CandidatePoolFeasibilitySweep
	if sweep == nil || sweep.CandidateCount == 0 || len(sweep.Candidates) != sweep.CandidateCount || sweep.NodesConsumed != sweepDiagnostics.PoolSweepNodes || sweep.NodesAvailable != sweep.NodesConsumed+sweep.NodesReturned || sweepDiagnostics.ConstructionNodes+sweepDiagnostics.PackingNodes+sweepDiagnostics.PoolSweepNodes != sweepSeed.NodesExplored {
		t.Fatalf("V4 sweep=%+v diagnostics=%+v", sweep, sweepDiagnostics)
	}
	withSweepReference := withSweep
	withSweepReference.DiagnosticReference = []model.Placement{{InstanceID: "filler#3", ItemID: "filler"}}
	sweepReferenceSeed, sweepReferenceDiagnostics := constellationSeedSearch(catalog, instances, options, withSweepReference, grid, sweepPolicy.ConstellationSeedNodeBudget, potential, nil)
	if !reflect.DeepEqual(sweepSeed, sweepReferenceSeed) || !reflect.DeepEqual(sweepDiagnostics, sweepReferenceDiagnostics) {
		t.Fatal("diagnostic reference changed V4 candidate feasibility sweep")
	}
}

func TestConstellationV2RetainsOneSkeletonPerSourceGeometry(t *testing.T) {
	makeSkeleton := func(geometry string, stars int, signature string) constellationSkeleton {
		return constellationSkeleton{
			sourceGeometryKey: geometry,
			signature:         signature,
			exactKey:          signature,
			score:             model.Score{PriorityCounts: []int{1, 1}, StarCount: stars},
		}
	}
	skeletons := map[string]constellationSkeleton{}
	retainConstellationV2Skeleton(skeletons, makeSkeleton("a", 3, "a-low"), 4)
	retainConstellationV2Skeleton(skeletons, makeSkeleton("a", 4, "a-high"), 4)
	retainConstellationV2Skeleton(skeletons, makeSkeleton("b", 2, "b"), 4)
	retainConstellationV2Skeleton(skeletons, makeSkeleton("c", 2, "c"), 4)
	retainConstellationV2Skeleton(skeletons, makeSkeleton("d", 2, "d"), 4)
	retainConstellationV2Skeleton(skeletons, makeSkeleton("e", 1, "e"), 4)
	if len(skeletons) != 4 || skeletons["a"].signature != "a-high" {
		t.Fatalf("V2 skeletons=%+v", skeletons)
	}
	if _, exists := skeletons["e"]; exists {
		t.Fatal("worse fifth geometry displaced retained geometry")
	}
}

func TestConstellationV2ThroughV4KeepV1BudgetAndReportGeometry(t *testing.T) {
	base := Config{PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	v1Config := base
	v1Config.EnableConstellationSeedV1 = true
	v2Config := base
	v2Config.ConstellationSeedVariant = ConstellationSeedVariantV2
	v3Config := base
	v3Config.ConstellationSeedVariant = ConstellationSeedVariantV3
	v4Config := base
	v4Config.ConstellationSeedVariant = ConstellationSeedVariantV4
	v1 := resolveSearchPolicy(v1Config, 1_000_000)
	v2 := resolveSearchPolicy(v2Config, 1_000_000)
	v3 := resolveSearchPolicy(v3Config, 1_000_000)
	v4 := resolveSearchPolicy(v4Config, 1_000_000)
	if v1.ConstellationSeedVersion != ConstellationSeedVariantV1 || v2.ConstellationSeedVersion != ConstellationSeedVariantV2 || v3.ConstellationSeedVersion != ConstellationSeedVariantV3 || v4.ConstellationSeedVersion != ConstellationSeedVariantV4 {
		t.Fatalf("versions v1=%q v2=%q v3=%q v4=%q", v1.ConstellationSeedVersion, v2.ConstellationSeedVersion, v3.ConstellationSeedVersion, v4.ConstellationSeedVersion)
	}
	if v1.ConstellationSeedShareBps != v2.ConstellationSeedShareBps || v1.ConstellationSeedNodeBudget != v2.ConstellationSeedNodeBudget || v1.ConstellationSeedBeamWidth != v2.ConstellationSeedBeamWidth || v1.ConstellationSeedPackingBeamWidth != v2.ConstellationSeedPackingBeamWidth || v1.ConstellationSeedMaxSkeletons != v2.ConstellationSeedMaxSkeletons || v2.ConstellationSeedSourceGeometryBeamCount != 24 || v2.ConstellationSeedNodeBudget != v3.ConstellationSeedNodeBudget || v2.ConstellationSeedBeamWidth != v3.ConstellationSeedBeamWidth || v2.ConstellationSeedPackingBeamWidth != v3.ConstellationSeedPackingBeamWidth || v3.ConstellationSeedPackingStrategy != constellationPackingStrategyStateMRV || v3.ConstellationSeedNodeBudget != v4.ConstellationSeedNodeBudget || v3.ConstellationSeedBeamWidth != v4.ConstellationSeedBeamWidth || v3.ConstellationSeedPackingBeamWidth != v4.ConstellationSeedPackingBeamWidth || v4.ConstellationSeedPackingStrategy != constellationPackingStrategyStateMRV {
		t.Fatalf("variant budgets differ: v1=%+v v2=%+v v3=%+v v4=%+v", v1, v2, v3, v4)
	}

	catalog := model.Catalog{Items: map[string]model.Item{
		"left":  {ID: "left", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"right": {ID: "right", Shape: []model.Coord{{}}, Stars: []model.Star{{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}}}, Rotations: []int{0}},
		"food":  {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"left", "right", "food"})
	grid := mustParseGridForTest(t, "111000000/000000000/000000000/000000000/000000000/000000000")
	options := testOptionsForGrid(t, catalog, instances, grid)
	v2Config.TopN = 3
	v2Config.AllowSkips = false
	v2Config.MaxNodes = 1_000
	v2Config.Diagnostics = true
	v2Config.policy = &v2
	v2Config.priorityBounds = newPriorityBoundContext(catalog, instances, v2Config.Priorities, v2Config.PrioritySemantics)
	potential := newStarPotentialContext(catalog, instances, options, v2Config.Priorities, v2Config.PrioritySemantics)
	_, diagnostics := constellationSeedSearch(catalog, instances, options, v2Config, grid, v2.ConstellationSeedNodeBudget, potential, nil)
	if diagnostics.Version != ConstellationSeedVariantV2 || diagnostics.PrioritySourceGeometryCount != diagnostics.PriorityConstellations || diagnostics.RootFreeMaskCount > diagnostics.PriorityConstellations {
		t.Fatalf("V2 diagnostics=%+v", diagnostics)
	}
	if len(diagnostics.Skeletons) > 0 && diagnostics.Skeletons[0].PrioritySourceGeometryKey == "" {
		t.Fatalf("missing V2 geometry key: %+v", diagnostics.Skeletons[0])
	}
}

func TestConstellationV5KeepsV4RootBudgetAndUsesPriorityFirstPacking(t *testing.T) {
	base := Config{PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3, Priorities: []string{"star_source:left", "star_source:right"}}
	v4Config := base
	v4Config.ConstellationSeedVariant = ConstellationSeedVariantV4
	v5Config := base
	v5Config.ConstellationSeedVariant = ConstellationSeedVariantV5
	v4 := resolveSearchPolicy(v4Config, 5_000_000)
	v5 := resolveSearchPolicy(v5Config, 5_000_000)
	if v5.ConstellationSeedVersion != ConstellationSeedVariantV5 || v5.ConstellationSeedMaxSkeletons != 4 || v5.ConstellationSeedShareBps != v4.ConstellationSeedShareBps || v5.ConstellationSeedNodeBudget != v4.ConstellationSeedNodeBudget || v5.ConstellationSeedConstructionBps != v4.ConstellationSeedConstructionBps || v5.ConstellationSeedPackingStrategy != constellationPackingStrategyStateMRV || v5.ConstellationSeedPackingBeamWidth != 128 {
		t.Fatalf("V5 policy=%+v V4 policy=%+v", v5, v4)
	}
	v5Config.policy = &v5
	components := constellationRootPackingComparatorComponents(v5Config, 2, true)
	if len(components) == 0 || components[0].name != "priority_count" || components[len(components)-1].name != "key_lexical" {
		t.Fatalf("V5 comparator=%+v", components)
	}
	v51Config := base
	v51Config.ConstellationSeedVariant = ConstellationSeedVariantV51
	v51 := resolveSearchPolicy(v51Config, 5_000_000)
	if v51.ConstellationSeedVersion != ConstellationSeedVariantV51 || v51.ConstellationSeedMaxSkeletons != v5.ConstellationSeedMaxSkeletons || v51.ConstellationSeedShareBps != v5.ConstellationSeedShareBps || v51.ConstellationSeedNodeBudget != v5.ConstellationSeedNodeBudget || v51.ConstellationSeedPackingBeamWidth != 128 {
		t.Fatalf("V5.1 policy=%+v V5 policy=%+v", v51, v5)
	}
}

func TestConstellationV51IneligibleConfigurationsMatchV4Policy(t *testing.T) {
	base := Config{
		MaxNodes:          1_000_000,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:left", "star_source:right"},
	}
	cases := []struct {
		name   string
		mutate func(*Config)
	}{
		{"allow_skips", func(config *Config) { config.AllowSkips = true }},
		{"other_semantics", func(config *Config) { config.PrioritySemantics = model.PrioritySemanticsOutgoingV2 }},
		{"coverage_groups", func(config *Config) { config.CoverageGroups = []model.CoverageGroup{{Name: "group"}} }},
		{"wrong_priority_count", func(config *Config) { config.Priorities = []string{"star_source:left"} }},
		{"non_source_priority", func(config *Config) { config.Priorities = []string{"star_source:left", "craft:item"} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			v4Config := base
			v4Config.ConstellationSeedVariant = ConstellationSeedVariantV4
			testCase.mutate(&v4Config)
			v51Config := v4Config
			v51Config.ConstellationSeedVariant = ConstellationSeedVariantV51
			v4Policy := resolveSearchPolicy(v4Config, v4Config.MaxNodes)
			v51Policy := resolveSearchPolicy(v51Config, v51Config.MaxNodes)
			if !reflect.DeepEqual(v4Policy, v51Policy) || v4Policy.ConstellationSeedVersion != "" || v4Policy.ConstellationSeedNodeBudget != 0 || v4Policy.ConstellationSeedPackingBeamWidth != 0 || resolvedExecutionFingerprint(v4Config, configuredSearchStages(v4Config)) != resolvedExecutionFingerprint(v51Config, configuredSearchStages(v51Config)) {
				t.Fatalf("V4 policy=%+v V5.1 policy=%+v", v4Policy, v51Policy)
			}
		})
	}
}

func TestConstellationParentFrontierHedgeFreezesQuotaAndKeepsMembersLocal(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"a": {ID: "a", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"b": {ID: "b", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"a", "b"})
	placement := func(instance model.InventoryInstance, col int) model.Placement {
		return model.Placement{InstanceID: instance.InstanceID, ItemID: instance.ItemID, OriginalIndex: instance.OriginalIndex, Origin: model.Coord{Col: col}, Cells: []model.Coord{{Col: col}}, Mask: uint64(1) << uint(col)}
	}
	a := placement(instances[0], 0)
	b := placement(instances[1], 1)
	parent := constellationSkeleton{exactKey: "parent", placed: []model.Placement{a}}
	frontier := constellationSkeleton{exactKey: "frontier", rootID: "root-1", selectionPolicy: "v5_relaxation_frontier", relaxedFromExactKey: parent.exactKey}
	newConfig := func() Config {
		policy := ResolvedSearchPolicy{ConstellationSeedVariant: ConstellationSeedVariantV5, ConstellationSeedPackingStrategy: constellationPackingStrategyStateMRV, ConstellationSeedPackingBeamWidth: 128}
		return Config{
			TopN:                     1,
			ConstellationSeedVariant: ConstellationSeedVariantV5,
			policy:                   &policy,
			ledger:                   newNodeLedger(100, []SearchStage{{ID: "single", NodeLimit: 100}}),
			stageID:                  "single",
			constellationRootOrigins: map[string]string{},
		}
	}
	newSnapshot := func(quota int64) *constellationCandidateCompletionSnapshot {
		return &constellationCandidateCompletionSnapshot{
			candidates:        map[string]constellationSkeleton{parent.exactKey: parent},
			selectedSkeletons: map[string]constellationSkeleton{frontier.exactKey: frontier},
			selectedRoots:     map[string]constellationSelectedRootOutcome{frontier.exactKey: {rootID: "root-1", nodesReserved: quota}},
		}
	}
	options := map[string][]model.Placement{instances[0].InstanceID: {a}, instances[1].InstanceID: {b}}

	config := newConfig()
	hedge := constellationParentFrontierHedge(catalog, instances, options, newSnapshot(10), config, 0b11)
	if len(hedge.Attempts) != 1 {
		t.Fatalf("attempts=%+v", hedge)
	}
	attempt := hedge.Attempts[0]
	if attempt.SelectionStatus != "accepted" || !attempt.Parent.Invoked || !attempt.Frontier.Invoked || attempt.Frontier.Reserved != attempt.TotalQuota-attempt.Parent.Consumed || attempt.FamilyConsumed != attempt.Parent.Consumed+attempt.Frontier.Consumed || attempt.FamilyReturned != attempt.TotalQuota-attempt.FamilyConsumed || attempt.FamilyReturned < 0 || config.ledger.diagnosticTotal() != attempt.FamilyConsumed || len(config.constellationRootOrigins) != 0 {
		t.Fatalf("hedge=%+v diagnostic=%d origins=%+v", attempt, config.ledger.diagnosticTotal(), config.constellationRootOrigins)
	}
	if attempt.Parent.TerminationReason != "completed" {
		t.Fatalf("parent termination=%q", attempt.Parent.TerminationReason)
	}

	config = newConfig()
	noResidual := constellationParentFrontierHedge(catalog, instances, options, newSnapshot(1), config, 0b11)
	if len(noResidual.Attempts) != 1 {
		t.Fatalf("no residual attempts=%+v", noResidual)
	}
	attempt = noResidual.Attempts[0]
	if !attempt.Parent.Invoked || attempt.Parent.TerminationReason != "completed" || attempt.Frontier.Invoked || attempt.Frontier.SkippedReason != "no_residual_quota" || attempt.Frontier.Reserved != 0 || attempt.FamilyReturned != 0 || config.ledger.diagnosticTotal() != attempt.FamilyConsumed {
		t.Fatalf("no residual hedge=%+v diagnostic=%d", attempt, config.ledger.diagnosticTotal())
	}

	familyConfig := newConfig()
	familyResult := constellationParentGuardedFrontierPackingSearch(catalog, instances, options, parent, frontier, familyConfig, 0b11, 10, func(bool) bool { return true })
	if familyResult.parentGuardedFrontier == nil || familyResult.nodes != familyResult.parentGuardedFrontier.FamilyConsumed || familyResult.parentGuardedFrontier.Frontier.Reserved != 10-familyResult.parentGuardedFrontier.Parent.Consumed || len(familyConfig.constellationRootOrigins) == 0 {
		t.Fatalf("family result=%+v origins=%+v", familyResult, familyConfig.constellationRootOrigins)
	}
	isolatedConfig := newConfig()
	isolatedConfig.constellationRootPackingCollector = &constellationRootPackingCollector{promote: false}
	isolated := constellationRootPackingSearch(catalog, instances, options, frontier, isolatedConfig, 0b11, familyResult.parentGuardedFrontier.Frontier.Reserved, func(bool) bool { return true })
	if isolated.nodes != familyResult.parentGuardedFrontier.Frontier.Consumed {
		t.Fatalf("isolated frontier nodes=%d family=%d", isolated.nodes, familyResult.parentGuardedFrontier.Frontier.Consumed)
	}
	if len(isolated.solutions) > 0 && (familyResult.parentGuardedFrontier.Frontier.BestScore == nil || compareScores(isolated.solutions[0].Evaluation.Score, *familyResult.parentGuardedFrontier.Frontier.BestScore) != 0) {
		t.Fatalf("frontier was guided isolated=%+v family=%+v", isolated.solutions[0], familyResult.parentGuardedFrontier.Frontier)
	}
}
