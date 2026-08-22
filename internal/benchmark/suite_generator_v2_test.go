package benchmark

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

func TestV2TopologyTemplatesMeetFrozenDefinitions(t *testing.T) {
	for _, topology := range []string{
		GridTopologyFull,
		GridTopologyBottleneck,
		GridTopologyHoles,
		GridTopologyTwoLobes,
		GridTopologyNarrowCorridors,
	} {
		t.Run(topology, func(t *testing.T) {
			descriptor := GeneratedSearchSuiteStructuralDescriptor{
				GridTopology: topology, DensityBand: DensityBandD60,
				SourceMultiplicity: SourceMultiplicityOneOne, TargetOverlap: TargetOverlapMostlyExclusive,
				CopySymmetry: CopySymmetryLow, RotationEntropy: RotationEntropyLow,
			}
			for seed := int64(0); seed < 8; seed++ {
				grid, err := chooseTopologyGridV2(topology, v2Random(seed, "topology", 0))
				if err != nil {
					t.Fatal(err)
				}
				mask, err := geometry.ParseGridText(strings.Join(grid, "\n"))
				if err != nil {
					t.Fatal(err)
				}
				if err := validateTopologyDescriptorAgainstGridV2(descriptor, mask); err != nil {
					t.Fatalf("grid=%v: %v", grid, err)
				}
			}
		})
	}
}

func TestSearchSuiteGeneratorV2MaterializesAndAuditsRealCatalog(t *testing.T) {
	loadedCatalog, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	seed := int64(8101)
	descriptor := GeneratedSearchSuiteStructuralDescriptor{
		GridTopology: GridTopologyFull, DensityBand: DensityBandD60,
		SourceMultiplicity: SourceMultiplicityOneOne, TargetOverlap: TargetOverlapMostlyExclusive,
		CopySymmetry: CopySymmetryLow, RotationEntropy: RotationEntropyLow,
	}
	entry := GeneratedSearchSuiteCase{ID: "v2-test", Family: GeneratedFamilyStructuralV2, Role: SuiteRoleDevelopment, Seed: &seed, StructuralDescriptor: &descriptor}
	first, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV2, loadedCatalog, entry)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	second, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV2, loadedCatalog, entry)
	if err != nil {
		t.Fatalf("materialize repeat: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("v2 generation is not deterministic")
	}
	if _, err := ValidateGeneratedSearchSuiteScenarioAgainstRequestedV2(loadedCatalog, first, descriptor); err != nil {
		t.Fatalf("audit generated scenario: %v", err)
	}
}

func TestV2SourcePairIsolationUsesCanonicalMatching(t *testing.T) {
	tests := []struct {
		name      string
		aStar     model.Star
		bStar     model.Star
		configure func(*model.Catalog)
		want      bool
	}{
		{name: "isolated", aStar: model.Star{TargetItems: []string{"a-target"}}, bStar: model.Star{TargetItems: []string{"b-target"}}, want: true},
		{name: "a targets itself", aStar: model.Star{TargetItems: []string{"a"}}, bStar: model.Star{TargetItems: []string{"b-target"}}},
		{name: "a targets b", aStar: model.Star{TargetItems: []string{"b"}}, bStar: model.Star{TargetItems: []string{"b-target"}}},
		{name: "b targets a", aStar: model.Star{TargetItems: []string{"a-target"}}, bStar: model.Star{TargetItems: []string{"a"}}},
		{name: "b targets itself", aStar: model.Star{TargetItems: []string{"a-target"}}, bStar: model.Star{TargetItems: []string{"b"}}},
		{name: "a targets b through alias", aStar: model.Star{TargetItems: []string{"b-alias"}}, bStar: model.Star{TargetItems: []string{"b-target"}}, configure: func(catalog *model.Catalog) {
			item := catalog.Items["b"]
			item.CountsAs = []model.ItemAlias{{ItemID: "b-alias"}}
			catalog.Items["b"] = item
		}},
		{name: "b targets a through type", aStar: model.Star{TargetItems: []string{"a-target"}}, bStar: model.Star{TargetTypes: []string{"source-type"}}, configure: func(catalog *model.Catalog) {
			item := catalog.Items["a"]
			item.Types = []string{"source-type"}
			catalog.Items["a"] = item
		}},
		{name: "exclude source permits self rule", aStar: model.Star{TargetItems: []string{"a"}, ExcludeSourceItem: true}, bStar: model.Star{TargetItems: []string{"b-target"}}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := v2PairCatalogForTest(test.aStar, test.bStar)
			if test.configure != nil {
				test.configure(&catalog)
			}
			if got := sourcePairIsIsolatedV2(catalog, "a", "b"); got != test.want {
				t.Fatalf("isolated=%v want %v", got, test.want)
			}
		})
	}
}

func TestV2AnalyzerRejectsSourceToSourceTargetsAndScenarioContractDrift(t *testing.T) {
	testCatalog := v2PairCatalogForTest(model.Star{TargetItems: []string{"b"}}, model.Star{TargetItems: []string{"b-target"}})
	scenario := v2ScenarioForTest(map[string]int{"a": 1, "b": 1})
	if _, err := AnalyzeGeneratedSearchSuiteStructureV2(testCatalog, scenario); err == nil || !strings.Contains(err.Error(), "must not target source B") {
		t.Fatalf("source-to-source analyzer err=%v", err)
	}

	loadedCatalog, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	seed := int64(8101)
	descriptor := validStructuralDescriptorForTest()
	entry := GeneratedSearchSuiteCase{ID: "contract-test", Family: GeneratedFamilyStructuralV2, Role: SuiteRoleDevelopment, Seed: &seed, StructuralDescriptor: &descriptor}
	generated, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV2, loadedCatalog, entry)
	if err != nil {
		t.Fatalf("materialize: %v", err)
	}
	noSkips := false
	generated.NoSkips = &noSkips
	if _, err := AnalyzeGeneratedSearchSuiteStructureV2(loadedCatalog, generated); err == nil || !strings.Contains(err.Error(), "no_skips=true") {
		t.Fatalf("scenario contract err=%v", err)
	}
	if err := validateV2GeneratedCandidate(loadedCatalog, generated, descriptor); err == nil || !strings.Contains(err.Error(), "violates structural invariant") {
		t.Fatalf("structural audit must be fatal, err=%v", err)
	}
}

func TestV2WitnessExhaustionIsFatalAndPublicCorpusHasNone(t *testing.T) {
	loadedCatalog, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	manifest, err := LoadSearchSuiteManifest(filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json"))
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}

	for _, entry := range manifest.Generated {
		if entry.Role == SuiteRolePrivateHoldout {
			continue
		}
		generated, diagnostics, err := materializeGeneratedSearchSuiteCaseV2WithDiagnostics(loadedCatalog, entry)
		if err != nil {
			t.Fatalf("public case %q: %v", entry.ID, err)
		}
		if diagnostics.Rejections["witness_exhausted"] != 0 {
			t.Fatalf("public case %q retried after witness exhaustion: %+v", entry.ID, diagnostics.Rejections)
		}
		if generated.Name != entry.ID || diagnostics.AcceptedAttempt < 0 {
			t.Fatalf("public case %q did not materialize deterministically: %+v", entry.ID, diagnostics)
		}
	}

	entry := manifest.Generated[0]
	pairs, err := viableV2SourcePairs(loadedCatalog, *entry.StructuralDescriptor)
	if err != nil || len(pairs) == 0 {
		t.Fatalf("source pairs for exhaustion test: %v", err)
	}
	_, rejection, err := materializeGeneratedSearchSuiteCaseV2AttemptWithWitnessMaxNodes(loadedCatalog, entry.ID, *entry.Seed, *entry.StructuralDescriptor, pairs, 0, 0)
	if rejection != "" || err == nil || !strings.Contains(err.Error(), "witness exhausted") {
		t.Fatalf("witness exhaustion must be fatal, rejection=%q err=%v", rejection, err)
	}
}

func TestV2PrivateHoldoutCommitmentIsVerifiedBeforeMaterialization(t *testing.T) {
	loadedCatalog, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	seed := int64(8123)
	privateSeedID := "fixture-private-seed"
	descriptor := validStructuralDescriptorForTest()
	manifest := SearchSuiteManifest{
		Version: 1, Name: "fixture-private-v2", Budgets: []int64{1}, Workers: 1, BaselinePolicy: "v4",
		Generated: []GeneratedSearchSuiteCase{{
			ID: "private-v2", Family: GeneratedFamilyStructuralV2, Role: SuiteRolePrivateHoldout,
			PrivateSeedID: privateSeedID, PrivateSeedCommitment: SearchSuiteV2PrivateSeedCommitment(privateSeedID, seed),
			StructuralDescriptor: &descriptor,
		}},
	}
	if err := VerifySearchSuiteV2PrivateHoldouts(loadedCatalog, manifest, map[string]int64{privateSeedID: seed}); err != nil {
		t.Fatalf("verify private holdout: %v", err)
	}
	if err := VerifySearchSuiteV2PrivateHoldouts(loadedCatalog, manifest, map[string]int64{privateSeedID: seed + 1}); err == nil || !strings.Contains(err.Error(), "commitment mismatch") {
		t.Fatalf("commitment mismatch err=%v", err)
	}
}

func TestV2PairAndTargetSelectionDoNotPreferMinimumArea(t *testing.T) {
	pairs := []v2SourcePair{{SourceA: "small-a", SourceB: "small-b"}, {SourceA: "large-a", SourceB: "large-b"}, {SourceA: "other-a", SourceB: "other-b"}}
	seenPairs := map[string]bool{}
	seenTargets := map[string]bool{}
	for seed := int64(0); seed < 64; seed++ {
		pair := chooseV2SourcePair(append([]v2SourcePair(nil), pairs...), v2Random(seed, "source-pair", 0))
		seenPairs[pair.SourceA+"/"+pair.SourceB] = true
		targets := shuffleV2IDs([]string{"area-1", "area-2", "area-4"}, v2Random(seed, "targets", 0))
		seenTargets[targets[0]] = true
	}
	if len(seenPairs) != len(pairs) {
		t.Fatalf("source pair selection omitted eligible pairs: %v", seenPairs)
	}
	if !seenTargets["area-4"] {
		t.Fatalf("target selection never selected a non-minimum-area candidate: %v", seenTargets)
	}
}

func TestV2EligiblePairsIncludeBothOrientationsForTwoOne(t *testing.T) {
	catalog := v2PairCatalogForTest(model.Star{TargetItems: []string{"a-1", "a-2", "a-3"}}, model.Star{TargetItems: []string{"b-1", "b-2", "b-3"}})
	for _, itemID := range []string{"a-1", "a-2", "a-3", "b-1", "b-2", "b-3", "neutral"} {
		catalog.Items[itemID] = model.Item{ID: itemID, Shape: []model.Coord{{}}}
	}
	descriptor := GeneratedSearchSuiteStructuralDescriptor{
		GridTopology: GridTopologyFull, DensityBand: DensityBandD60,
		SourceMultiplicity: SourceMultiplicityTwoOne, TargetOverlap: TargetOverlapMostlyExclusive,
		CopySymmetry: CopySymmetryLow, RotationEntropy: RotationEntropyLow,
	}
	pairs, err := viableV2SourcePairs(catalog, descriptor)
	if err != nil {
		t.Fatalf("eligible pairs: %v", err)
	}
	seen := map[string]bool{}
	for _, pair := range pairs {
		seen[pair.SourceA+"/"+pair.SourceB] = true
	}
	if !seen["a/b"] || !seen["b/a"] {
		t.Fatalf("2/1 pairs lack both orientations: %v", seen)
	}
}

func TestGeneralSearchV2StructuralDesignCoverage(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json")
	manifest, err := LoadSearchSuiteManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV2, manifest); err != nil {
		t.Fatalf("validate v2 manifest: %v", err)
	}
	if len(manifest.Scenarios) != 10 || len(manifest.Generated) != 36 {
		t.Fatalf("static=%d generated=%d", len(manifest.Scenarios), len(manifest.Generated))
	}
	roles := map[string]int{}
	seenIDs := map[string]struct{}{}
	for _, entry := range manifest.Generated {
		if _, exists := seenIDs[entry.ID]; exists {
			t.Fatalf("duplicate generated ID %q", entry.ID)
		}
		seenIDs[entry.ID] = struct{}{}
		roles[entry.Role]++
		if entry.Role == SuiteRolePrivateHoldout {
			if entry.Seed != nil || entry.PrivateSeedID == "" {
				t.Fatalf("private entry leaks or lacks seed metadata: %+v", entry)
			}
			continue
		}
		if entry.Seed == nil || *entry.Seed != SearchSuiteV2PublicSeed(entry.ID) {
			t.Fatalf("public seed for %q is not hash-derived", entry.ID)
		}
	}
	if roles[SuiteRoleDevelopment] != 14 || roles[SuiteRoleValidation] != 10 || roles[SuiteRolePublicHoldout] != 6 || roles[SuiteRolePrivateHoldout] != 6 {
		t.Fatalf("unexpected generated roles: %v", roles)
	}
	assertPairwiseStructuralCoverageV2(t, manifest.Generated)
	assertHoldoutMarginalCoverageV2(t, manifest.Generated, SuiteRolePublicHoldout)
	assertHoldoutMarginalCoverageV2(t, manifest.Generated, SuiteRolePrivateHoldout)
}

func TestVerifyCommittedGeneralSearchV2Lock(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json")
	lockPath := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.lock")
	catalogPath := filepath.Join("..", "..", "data", "catalog.json")
	if err := VerifySearchSuiteLock(manifestPath, catalogPath, lockPath); err != nil {
		t.Fatalf("verify v2 lock: %v", err)
	}
}

func TestV2WitnessDistinguishesPackableImpossibleAndExhausted(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"one": {ID: "one", Shape: []model.Coord{{}}},
		"bar": {ID: "bar", Shape: []model.Coord{{}, {Col: 1}}},
	}}
	packable := scenario.Scenario{Name: "packable", Grid: []string{"110000000", "000000000", "000000000", "000000000", "000000000", "000000000"}, Items: map[string]int{"one": 2}}
	status, err := verifyGeneratedSearchSuiteV2Packability(catalog, packable)
	if err != nil || status != v2WitnessPackable {
		t.Fatalf("packable status=%q err=%v", status, err)
	}
	impossible := scenario.Scenario{Name: "impossible", Grid: []string{"100000000", "000000000", "000000000", "000000000", "000000000", "000000000"}, Items: map[string]int{"bar": 1}}
	status, err = verifyGeneratedSearchSuiteV2Packability(catalog, impossible)
	if err != nil || status != v2WitnessUnpackable {
		t.Fatalf("impossible status=%q err=%v", status, err)
	}
	status, err = verifyGeneratedSearchSuiteV2PackabilityWithMaxNodes(catalog, packable, 0)
	if err != nil || status != v2WitnessExhausted {
		t.Fatalf("exhausted status=%q err=%v", status, err)
	}
}

func TestV2WitnessCoversRotationsCopiesComponentsDensityAndDeterminism(t *testing.T) {
	catalog := model.Catalog{Items: map[string]model.Item{
		"one": {ID: "one", Shape: []model.Coord{{}}},
		"bar": {ID: "bar", Shape: []model.Coord{{}, {Col: 1}}},
	}}
	rotationRequired := scenario.Scenario{Name: "rotation", Grid: []string{"100000000", "100000000", "000000000", "000000000", "000000000", "000000000"}, Items: map[string]int{"bar": 1}}
	disconnectedCopies := scenario.Scenario{Name: "copies", Grid: []string{"110000000", "000000000", "110000000", "000000000", "000000000", "000000000"}, Items: map[string]int{"bar": 2}}
	dense := scenario.Scenario{Name: "dense", Grid: []string{"111111111", "111111111", "111111111", "111111111", "111111111", "111100000"}, Items: map[string]int{"one": 48}}
	for _, generated := range []scenario.Scenario{rotationRequired, disconnectedCopies, dense} {
		first, err := verifyGeneratedSearchSuiteV2Packability(catalog, generated)
		if err != nil || first != v2WitnessPackable {
			t.Fatalf("%s first status=%q err=%v", generated.Name, first, err)
		}
		second, err := verifyGeneratedSearchSuiteV2Packability(catalog, generated)
		if err != nil || second != first {
			t.Fatalf("%s second status=%q want %q err=%v", generated.Name, second, first, err)
		}
	}
}

func v2PairCatalogForTest(aStar model.Star, bStar model.Star) model.Catalog {
	return model.Catalog{Items: map[string]model.Item{
		"a":        {ID: "a", Shape: []model.Coord{{}}, Stars: []model.Star{aStar}},
		"b":        {ID: "b", Shape: []model.Coord{{}}, Stars: []model.Star{bStar}},
		"a-target": {ID: "a-target", Shape: []model.Coord{{}}},
		"b-target": {ID: "b-target", Shape: []model.Coord{{}}},
	}}
}

func v2ScenarioForTest(items map[string]int) scenario.Scenario {
	top, workers := 1, 1
	noSkips, repair := true, true
	return scenario.Scenario{
		Name: "v2-test", Grid: []string{"111111111", "111111111", "111111111", "111111111", "111111111", "111111111"}, Items: items,
		Top: &top, Workers: &workers, NoSkips: &noSkips, RepairSearch: &repair,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:a", "star_source:b"},
	}
}

func assertPairwiseStructuralCoverageV2(t *testing.T, entries []GeneratedSearchSuiteCase) {
	t.Helper()
	values := [][]string{
		{GridTopologyFull, GridTopologyBottleneck, GridTopologyHoles, GridTopologyTwoLobes, GridTopologyNarrowCorridors},
		{DensityBandD60, DensityBandD75, DensityBandD90, DensityBandD97},
		{SourceMultiplicityOneOne, SourceMultiplicityTwoOne, SourceMultiplicityTwoTwo},
		{TargetOverlapMostlyExclusive, TargetOverlapMixed, TargetOverlapMostlyShared},
		{CopySymmetryLow, CopySymmetryHigh},
		{RotationEntropyLow, RotationEntropyMedium, RotationEntropyHigh},
	}
	for left := 0; left < len(values); left++ {
		for right := left + 1; right < len(values); right++ {
			seen := map[string]bool{}
			for _, entry := range entries {
				descriptor := *entry.StructuralDescriptor
				parts := structuralDescriptorValuesV2(descriptor)
				seen[parts[left]+"\x00"+parts[right]] = true
			}
			for _, leftValue := range values[left] {
				for _, rightValue := range values[right] {
					if !seen[leftValue+"\x00"+rightValue] {
						t.Fatalf("missing structural pair %d=%s, %d=%s", left, leftValue, right, rightValue)
					}
				}
			}
		}
	}
}

func assertHoldoutMarginalCoverageV2(t *testing.T, entries []GeneratedSearchSuiteCase, role string) {
	t.Helper()
	want := [][]string{
		{GridTopologyFull, GridTopologyBottleneck, GridTopologyHoles, GridTopologyTwoLobes, GridTopologyNarrowCorridors},
		{DensityBandD60, DensityBandD75, DensityBandD90, DensityBandD97},
		{SourceMultiplicityOneOne, SourceMultiplicityTwoOne, SourceMultiplicityTwoTwo},
		{TargetOverlapMostlyExclusive, TargetOverlapMixed, TargetOverlapMostlyShared},
		{CopySymmetryLow, CopySymmetryHigh},
		{RotationEntropyLow, RotationEntropyMedium, RotationEntropyHigh},
	}
	seen := make([]map[string]bool, len(want))
	for index := range seen {
		seen[index] = map[string]bool{}
	}
	for _, entry := range entries {
		if entry.Role != role {
			continue
		}
		for index, value := range structuralDescriptorValuesV2(*entry.StructuralDescriptor) {
			seen[index][value] = true
		}
	}
	for index, choices := range want {
		for _, choice := range choices {
			if !seen[index][choice] {
				t.Fatalf("%s holdout lacks dimension %d value %q", role, index, choice)
			}
		}
	}
}

func structuralDescriptorValuesV2(descriptor GeneratedSearchSuiteStructuralDescriptor) []string {
	return []string{descriptor.GridTopology, descriptor.DensityBand, descriptor.SourceMultiplicity, descriptor.TargetOverlap, descriptor.CopySymmetry, descriptor.RotationEntropy}
}
