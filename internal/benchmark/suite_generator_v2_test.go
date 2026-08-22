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
