package benchmark

// SearchSuiteV2DevelopmentCohortSchema returns the frozen six-dimensional V2
// descriptor schema used for G0 development expansion.
func SearchSuiteV2DevelopmentCohortSchema() DevelopmentCohortSchema {
	return DevelopmentCohortSchema{
		Version: developmentCohortAlgorithmVersion,
		Dimensions: []DevelopmentCohortDimension{
			{Name: "grid_topology", Values: []string{GridTopologyFull, GridTopologyBottleneck, GridTopologyHoles, GridTopologyTwoLobes, GridTopologyNarrowCorridors}},
			{Name: "density_band", Values: []string{DensityBandD60, DensityBandD75, DensityBandD90, DensityBandD97}},
			{Name: "source_multiplicity", Values: []string{SourceMultiplicityOneOne, SourceMultiplicityTwoOne, SourceMultiplicityTwoTwo}},
			{Name: "target_overlap", Values: []string{TargetOverlapMostlyExclusive, TargetOverlapMixed, TargetOverlapMostlyShared}},
			{Name: "copy_symmetry", Values: []string{CopySymmetryLow, CopySymmetryHigh}},
			{Name: "rotation_entropy", Values: []string{RotationEntropyLow, RotationEntropyMedium, RotationEntropyHigh}},
		},
	}
}

// DevelopmentCohortDescriptorFromV2 converts the existing V2 descriptor to
// the generator-neutral categorical representation used by the selector.
func DevelopmentCohortDescriptorFromV2(descriptor GeneratedSearchSuiteStructuralDescriptor) (DevelopmentCohortDescriptor, error) {
	if err := descriptor.Validate(); err != nil {
		return DevelopmentCohortDescriptor{}, err
	}
	return DevelopmentCohortDescriptor{Values: []string{
		descriptor.GridTopology,
		descriptor.DensityBand,
		descriptor.SourceMultiplicity,
		descriptor.TargetOverlap,
		descriptor.CopySymmetry,
		descriptor.RotationEntropy,
	}}, nil
}

// DevelopmentCohortDescriptorToV2 reverses the generator-neutral conversion.
func DevelopmentCohortDescriptorToV2(descriptor DevelopmentCohortDescriptor) (GeneratedSearchSuiteStructuralDescriptor, error) {
	schema := SearchSuiteV2DevelopmentCohortSchema()
	if err := schema.validateDescriptor(descriptor); err != nil {
		return GeneratedSearchSuiteStructuralDescriptor{}, err
	}
	converted := GeneratedSearchSuiteStructuralDescriptor{
		GridTopology:       descriptor.Values[0],
		DensityBand:        descriptor.Values[1],
		SourceMultiplicity: descriptor.Values[2],
		TargetOverlap:      descriptor.Values[3],
		CopySymmetry:       descriptor.Values[4],
		RotationEntropy:    descriptor.Values[5],
	}
	if err := converted.Validate(); err != nil {
		return GeneratedSearchSuiteStructuralDescriptor{}, err
	}
	return converted, nil
}
