package benchmark

import "fmt"

const (
	GeneratedFamilyStructuralV2 = "star-source-structural-v2"

	GridTopologyFull            = "full"
	GridTopologyBottleneck      = "bottleneck"
	GridTopologyHoles           = "holes"
	GridTopologyTwoLobes        = "two-lobes"
	GridTopologyNarrowCorridors = "narrow-corridors"

	DensityBandD60 = "d60"
	DensityBandD75 = "d75"
	DensityBandD90 = "d90"
	DensityBandD97 = "d97"

	SourceMultiplicityOneOne = "1/1"
	SourceMultiplicityTwoOne = "2/1"
	SourceMultiplicityTwoTwo = "2/2"

	TargetOverlapMostlyExclusive = "mostly-exclusive"
	TargetOverlapMixed           = "mixed"
	TargetOverlapMostlyShared    = "mostly-shared"

	CopySymmetryLow  = "low"
	CopySymmetryHigh = "high"

	RotationEntropyLow    = "low"
	RotationEntropyMedium = "medium"
	RotationEntropyHigh   = "high"
)

// GeneratedSearchSuiteStructuralDescriptor is the public, requested
// structural population of a v2 generated case. It deliberately uses stable
// categories rather than implementation thresholds or solver measurements.
type GeneratedSearchSuiteStructuralDescriptor struct {
	GridTopology       string `json:"grid_topology"`
	DensityBand        string `json:"density_band"`
	SourceMultiplicity string `json:"source_multiplicity"`
	TargetOverlap      string `json:"target_overlap"`
	CopySymmetry       string `json:"copy_symmetry"`
	RotationEntropy    string `json:"rotation_entropy"`
}

// GeneratedSearchSuiteRealizedDescriptor is independently measured from a
// completed scenario. It intentionally contains no solver-result fields.
type GeneratedSearchSuiteRealizedDescriptor struct {
	UsableCells              int `json:"usable_cells"`
	InventoryArea            int `json:"inventory_area"`
	DensityBPS               int `json:"density_bps"`
	ConnectedComponents      int `json:"connected_components"`
	ArticulationCells        int `json:"articulation_cells"`
	InteriorBlockedCells     int `json:"interior_blocked_cells"`
	CorridorCells            int `json:"corridor_cells"`
	SourceACopies            int `json:"source_a_copies"`
	SourceBCopies            int `json:"source_b_copies"`
	AOnlyTargets             int `json:"a_only_targets"`
	BOnlyTargets             int `json:"b_only_targets"`
	SharedTargets            int `json:"shared_targets"`
	NeutralFillerInstances   int `json:"neutral_filler_instances"`
	DuplicateFillerInstances int `json:"duplicate_filler_instances"`
	DuplicateFillerGroups    int `json:"duplicate_filler_groups"`
	RotationVariants1        int `json:"rotation_variants_1"`
	RotationVariants2        int `json:"rotation_variants_2"`
	RotationVariants3Plus    int `json:"rotation_variants_3_plus"`
}

// SearchSuiteLockedStructuralDescriptor records requested structural input and
// the public scenario's independently realized structure. Private cases carry
// only Requested so their seed and materialization remain undisclosed.
type SearchSuiteLockedStructuralDescriptor struct {
	Requested GeneratedSearchSuiteStructuralDescriptor `json:"requested"`
	Realized  *GeneratedSearchSuiteRealizedDescriptor  `json:"realized,omitempty"`
}

func (descriptor GeneratedSearchSuiteStructuralDescriptor) Validate() error {
	if !oneOf(descriptor.GridTopology,
		GridTopologyFull, GridTopologyBottleneck, GridTopologyHoles, GridTopologyTwoLobes, GridTopologyNarrowCorridors) {
		return fmt.Errorf("unsupported grid_topology %q", descriptor.GridTopology)
	}
	if !oneOf(descriptor.DensityBand, DensityBandD60, DensityBandD75, DensityBandD90, DensityBandD97) {
		return fmt.Errorf("unsupported density_band %q", descriptor.DensityBand)
	}
	if !oneOf(descriptor.SourceMultiplicity, SourceMultiplicityOneOne, SourceMultiplicityTwoOne, SourceMultiplicityTwoTwo) {
		return fmt.Errorf("unsupported source_multiplicity %q", descriptor.SourceMultiplicity)
	}
	if !oneOf(descriptor.TargetOverlap, TargetOverlapMostlyExclusive, TargetOverlapMixed, TargetOverlapMostlyShared) {
		return fmt.Errorf("unsupported target_overlap %q", descriptor.TargetOverlap)
	}
	if !oneOf(descriptor.CopySymmetry, CopySymmetryLow, CopySymmetryHigh) {
		return fmt.Errorf("unsupported copy_symmetry %q", descriptor.CopySymmetry)
	}
	if !oneOf(descriptor.RotationEntropy, RotationEntropyLow, RotationEntropyMedium, RotationEntropyHigh) {
		return fmt.Errorf("unsupported rotation_entropy %q", descriptor.RotationEntropy)
	}
	return nil
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func sourceMultiplicityCountsV2(value string) (int, int, error) {
	switch value {
	case SourceMultiplicityOneOne:
		return 1, 1, nil
	case SourceMultiplicityTwoOne:
		return 2, 1, nil
	case SourceMultiplicityTwoTwo:
		return 2, 2, nil
	default:
		return 0, 0, fmt.Errorf("unsupported source_multiplicity %q", value)
	}
}

func targetOverlapCountsV2(value string) (int, int, int, error) {
	switch value {
	case TargetOverlapMostlyExclusive:
		return 3, 3, 0, nil
	case TargetOverlapMixed:
		return 2, 2, 2, nil
	case TargetOverlapMostlyShared:
		return 1, 1, 4, nil
	default:
		return 0, 0, 0, fmt.Errorf("unsupported target_overlap %q", value)
	}
}
