package benchmark

import (
	"strings"
	"testing"
)

func TestGeneratedSearchSuiteStructuralDescriptorFrozenEnums(t *testing.T) {
	descriptor := validStructuralDescriptorForTest()
	if err := descriptor.Validate(); err != nil {
		t.Fatalf("valid descriptor: %v", err)
	}
	descriptor.RotationEntropy = "arbitrary"
	if err := descriptor.Validate(); err == nil || !strings.Contains(err.Error(), "rotation_entropy") {
		t.Fatalf("err=%v", err)
	}
}

func TestGeneratedSearchSuiteValidationIsVersioned(t *testing.T) {
	seed := int64(7)
	v1 := SearchSuiteManifest{Version: 1, Name: "v1", Budgets: []int64{1}, Workers: 1, BaselinePolicy: "v4", Generated: []GeneratedSearchSuiteCase{{
		ID: "v1", Family: GeneratedFamilySparse, Role: SuiteRoleDevelopment, Seed: &seed,
	}}}
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV1, v1); err != nil {
		t.Fatalf("valid v1: %v", err)
	}
	v1.Generated[0].StructuralDescriptor = ptrStructuralDescriptorForTest(validStructuralDescriptorForTest())
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV1, v1); err == nil || !strings.Contains(err.Error(), "must not have") {
		t.Fatalf("v1 descriptor err=%v", err)
	}

	v2 := SearchSuiteManifest{Version: 1, Name: "v2", Budgets: []int64{1}, Workers: 1, BaselinePolicy: "v4", Generated: []GeneratedSearchSuiteCase{{
		ID: "v2", Family: GeneratedFamilyStructuralV2, Role: SuiteRoleDevelopment, Seed: &seed,
	}}}
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV2, v2); err == nil || !strings.Contains(err.Error(), "requires structural_descriptor") {
		t.Fatalf("v2 missing descriptor err=%v", err)
	}
	v2.Generated[0].StructuralDescriptor = ptrStructuralDescriptorForTest(validStructuralDescriptorForTest())
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV2, v2); err != nil {
		t.Fatalf("valid v2: %v", err)
	}
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV1, v2); err == nil || !strings.Contains(err.Error(), "must not have") {
		t.Fatalf("v2 through v1 err=%v", err)
	}
}

func TestManifestGenericValidationDoesNotOwnGeneratorFamilies(t *testing.T) {
	seed := int64(7)
	manifest := SearchSuiteManifest{Version: 1, Name: "generic", Budgets: []int64{1}, Workers: 1, BaselinePolicy: "v4", Generated: []GeneratedSearchSuiteCase{{
		ID: "future", Family: "future-family", Role: SuiteRoleDevelopment, Seed: &seed,
	}}}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("generic manifest validation unexpectedly rejected family: %v", err)
	}
}

func validStructuralDescriptorForTest() GeneratedSearchSuiteStructuralDescriptor {
	return GeneratedSearchSuiteStructuralDescriptor{
		GridTopology: GridTopologyFull, DensityBand: DensityBandD60,
		SourceMultiplicity: SourceMultiplicityOneOne, TargetOverlap: TargetOverlapMostlyExclusive,
		CopySymmetry: CopySymmetryLow, RotationEntropy: RotationEntropyLow,
	}
}

func ptrStructuralDescriptorForTest(value GeneratedSearchSuiteStructuralDescriptor) *GeneratedSearchSuiteStructuralDescriptor {
	return &value
}
