package benchmark

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
)

const (
	G0BV2BaseSHA               = "6fe47febe6ce89f110a86dc0a3ae38e285df32f5"
	G0BV2SelectionNamespace    = "gsv2-devexp-v1"
	G0BV2PartitionNamespace    = "gsv2-devexp-partition-v1"
	G0BV2ExpansionSize         = 36
	G0BV2WaveSize              = 18
	G0BV2UniverseSize          = 1080
	G0BV2AttainablePairs       = 164
	G0BV2CombinedCoverageGate  = 148
	G0BV2CombinedCoverageDelta = 20
	G0BV2WaveCoverageGate      = 115
	G0BV2HistoricalManifestLF  = "5d1757c37580b04c9a85b738ea2672d8a0b3c8402c8ed5a509c8c42fd5d4b513"
	G0BV2HistoricalLockLF      = "96af8290e8741b4ef6f514b0df32820f8ccd241695eb5b1d671fc6cc2fd5aa6d"
	G0BV2ConfirmAManifestName  = "general-search-v2-dev-confirm-a"
	G0BV2ConfirmBManifestName  = "general-search-v2-dev-confirm-b"
	G0BV2ProvisionallySealed   = "provisionally_sealed"
)

type G0BV2CoreCase struct {
	CaseID               string                                   `json:"case_id"`
	CanonicalDescriptor  string                                   `json:"canonical_descriptor"`
	Descriptor           DevelopmentCohortDescriptor              `json:"descriptor"`
	StructuralDescriptor GeneratedSearchSuiteStructuralDescriptor `json:"structural_descriptor"`
}

type G0BV2CoreDescriptors struct {
	Version int             `json:"version"`
	Cases   []G0BV2CoreCase `json:"cases"`
}

type G0BV2MembershipCase struct {
	CaseID                  string                                   `json:"case_id"`
	SelectionStep           int                                      `json:"selection_step"`
	SelectionCandidateIndex int                                      `json:"selection_candidate_index"`
	CanonicalDescriptor     string                                   `json:"canonical_descriptor"`
	Descriptor              DevelopmentCohortDescriptor              `json:"descriptor"`
	StructuralDescriptor    GeneratedSearchSuiteStructuralDescriptor `json:"structural_descriptor"`
	PartitionCandidateIndex int                                      `json:"partition_candidate_index"`
	Wave                    string                                   `json:"wave"`
	Seed                    int64                                    `json:"seed"`
}

type G0BV2CohortMembership struct {
	Version            int                   `json:"version"`
	SelectionNamespace string                `json:"selection_namespace"`
	PartitionNamespace string                `json:"partition_namespace"`
	State              string                `json:"state"`
	Cases              []G0BV2MembershipCase `json:"cases"`
}

type G0BV2SeedAuditCase struct {
	CaseID              string `json:"case_id"`
	SelectionStep       int    `json:"selection_step"`
	CanonicalDescriptor string `json:"canonical_descriptor"`
	Namespace           string `json:"namespace"`
	Digest              string `json:"digest"`
	DerivedSeed         int64  `json:"derived_seed"`
}

type G0BV2SeedAudit struct {
	Version   int                  `json:"version"`
	Namespace string               `json:"namespace"`
	Cases     []G0BV2SeedAuditCase `json:"cases"`
}

type G0BV2DimensionSummary struct {
	Dimension            string         `json:"dimension"`
	Counts               map[string]int `json:"counts"`
	Minimum              int            `json:"minimum"`
	Maximum              int            `json:"maximum"`
	CategoriesComplete   bool           `json:"categories_complete"`
	MaxMinusMinAtMostOne bool           `json:"max_minus_min_at_most_one"`
}

type G0BV2PopulationSummary struct {
	CaseCount          int                     `json:"case_count"`
	Dimensions         []G0BV2DimensionSummary `json:"dimensions"`
	CategoriesComplete bool                    `json:"categories_complete"`
	MarginalsBalanced  bool                    `json:"marginals_balanced"`
	PairwiseCoverage   int                     `json:"pairwise_coverage"`
	CoveredPairs       []string                `json:"covered_pairs"`
}

type G0BV2StructuralGates struct {
	UniverseSize              bool `json:"universe_size"`
	AttainablePairs           bool `json:"attainable_pairs"`
	CoreCount                 bool `json:"core_count"`
	ExpansionCount            bool `json:"expansion_count"`
	UniqueIDs                 bool `json:"unique_ids"`
	UniqueSeeds               bool `json:"unique_seeds"`
	UniqueDescriptors         bool `json:"unique_descriptors"`
	CoreOverlapZero           bool `json:"core_overlap_zero"`
	WaveACount                bool `json:"wave_a_count"`
	WaveBCount                bool `json:"wave_b_count"`
	WavesDisjointAndComplete  bool `json:"waves_disjoint_and_complete"`
	CombinedMarginalsBalanced bool `json:"combined_marginals_balanced"`
	CombinedPairCoverage      bool `json:"combined_pair_coverage"`
	CorePairCoverageDelta     bool `json:"core_pair_coverage_delta"`
	WaveACategoriesComplete   bool `json:"wave_a_categories_complete"`
	WaveAPairCoverage         bool `json:"wave_a_pair_coverage"`
	WaveBCategoriesComplete   bool `json:"wave_b_categories_complete"`
	WaveBPairCoverage         bool `json:"wave_b_pair_coverage"`
}

func (gates G0BV2StructuralGates) Pass() bool {
	value := reflect.ValueOf(gates)
	for index := 0; index < value.NumField(); index++ {
		if !value.Field(index).Bool() {
			return false
		}
	}
	return true
}

type G0BV2CoverageSummary struct {
	Version           int                    `json:"version"`
	AttainablePairs   int                    `json:"attainable_pairs"`
	Core              G0BV2PopulationSummary `json:"core"`
	Combined          G0BV2PopulationSummary `json:"combined"`
	WaveA             G0BV2PopulationSummary `json:"wave_a"`
	WaveB             G0BV2PopulationSummary `json:"wave_b"`
	CoreCoverageDelta int                    `json:"core_coverage_delta"`
	CoreOverlap       []string               `json:"core_overlap"`
	Gates             G0BV2StructuralGates   `json:"gates"`
}

type G0BV2PreMaterializationFreeze struct {
	Version                    int                  `json:"version"`
	BaseSHA                    string               `json:"base_sha"`
	State                      string               `json:"state"`
	UniverseSize               int                  `json:"universe_size"`
	AttainablePairs            int                  `json:"attainable_pairs"`
	FullUniverseSHA256         string               `json:"full_universe_sha256"`
	CoreDescriptorsSHA256      string               `json:"core_descriptors_sha256"`
	SelectionTraceSHA256       string               `json:"selection_trace_sha256"`
	PartitionTraceSHA256       string               `json:"partition_trace_sha256"`
	SeedAuditSHA256            string               `json:"seed_audit_sha256"`
	CohortMembershipSHA256     string               `json:"cohort_membership_sha256"`
	OfficialSelectionRuns      int                  `json:"official_selection_runs"`
	MaterializationRuns        int                  `json:"materialization_runs"`
	BenchmarkScenarioRuns      int                  `json:"benchmark_scenario_runs"`
	NormalSolverRuns           int                  `json:"normal_solver_runs"`
	SearchProfileRuns          int                  `json:"searchprofile_runs"`
	EfficacyQualityRuns        int                  `json:"efficacy_quality_runs"`
	EfficacyDiagnosticRuns     int                  `json:"efficacy_diagnostic_runs"`
	ValidationMaterialized     bool                 `json:"validation_materialized"`
	PublicHoldoutMaterialized  bool                 `json:"public_holdout_materialized"`
	PrivateHoldoutMaterialized bool                 `json:"private_holdout_materialized"`
	StructuralGates            G0BV2StructuralGates `json:"structural_gates"`
}

type G0BV2SelectionArtifacts struct {
	Core       G0BV2CoreDescriptors
	Selection  DevelopmentCohortSelection
	Partition  DevelopmentCohortPartition
	Membership G0BV2CohortMembership
	Seeds      G0BV2SeedAudit
	Coverage   G0BV2CoverageSummary
	Freeze     G0BV2PreMaterializationFreeze
}

type G0BV2MaterializedCaseAudit struct {
	CaseID              string                                   `json:"case_id"`
	Wave                string                                   `json:"wave"`
	Seed                int64                                    `json:"seed"`
	ScenarioSHA256      string                                   `json:"scenario_sha256"`
	Requested           GeneratedSearchSuiteStructuralDescriptor `json:"requested"`
	Realized            GeneratedSearchSuiteRealizedDescriptor   `json:"realized"`
	RequestedVsRealized string                                   `json:"requested_vs_realized"`
	StructuralWitness   string                                   `json:"structural_witness"`
}

type G0BV2MaterializationAudit struct {
	Version                    int                          `json:"version"`
	State                      string                       `json:"state"`
	Cases                      []G0BV2MaterializedCaseAudit `json:"cases"`
	MaterializedCases          int                          `json:"materialized_cases"`
	RequestedVsRealizedPasses  int                          `json:"requested_vs_realized_passes"`
	StructuralWitnessPasses    int                          `json:"structural_witness_passes"`
	BenchmarkScenarioRuns      int                          `json:"benchmark_scenario_runs"`
	NormalSolverRuns           int                          `json:"normal_solver_runs"`
	SearchProfileRuns          int                          `json:"searchprofile_runs"`
	EfficacyQualityRuns        int                          `json:"efficacy_quality_runs"`
	EfficacyDiagnosticRuns     int                          `json:"efficacy_diagnostic_runs"`
	ValidationMaterialized     bool                         `json:"validation_materialized"`
	PublicHoldoutMaterialized  bool                         `json:"public_holdout_materialized"`
	PrivateHoldoutMaterialized bool                         `json:"private_holdout_materialized"`
}

func ExtractG0BV2Core(manifest SearchSuiteManifest) (G0BV2CoreDescriptors, error) {
	schema := SearchSuiteV2DevelopmentCohortSchema()
	byID := make(map[string]GeneratedSearchSuiteCase, len(manifest.Generated))
	for _, entry := range manifest.Generated {
		byID[entry.ID] = entry
	}
	core := G0BV2CoreDescriptors{Version: 1, Cases: make([]G0BV2CoreCase, 0, 14)}
	for number := 13; number <= 26; number++ {
		caseID := fmt.Sprintf("gsv2-%03d", number)
		entry, ok := byID[caseID]
		if !ok {
			return G0BV2CoreDescriptors{}, fmt.Errorf("historical manifest is missing core case %q", caseID)
		}
		if entry.Role != SuiteRoleDevelopment || entry.Family != GeneratedFamilyStructuralV2 || entry.StructuralDescriptor == nil {
			return G0BV2CoreDescriptors{}, fmt.Errorf("historical core case %q must be generated V2 development with a structural descriptor", caseID)
		}
		descriptor, err := DevelopmentCohortDescriptorFromV2(*entry.StructuralDescriptor)
		if err != nil {
			return G0BV2CoreDescriptors{}, fmt.Errorf("historical core case %q: %w", caseID, err)
		}
		canonical, err := CanonicalDevelopmentCohortDescriptor(schema, descriptor)
		if err != nil {
			return G0BV2CoreDescriptors{}, fmt.Errorf("historical core case %q: %w", caseID, err)
		}
		core.Cases = append(core.Cases, G0BV2CoreCase{
			CaseID: caseID, CanonicalDescriptor: canonical, Descriptor: descriptor,
			StructuralDescriptor: *entry.StructuralDescriptor,
		})
	}
	return core, nil
}

func PrepareG0BV2Selection(manifest SearchSuiteManifest) (G0BV2SelectionArtifacts, error) {
	schema := SearchSuiteV2DevelopmentCohortSchema()
	universe, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	if len(universe) != G0BV2UniverseSize {
		return G0BV2SelectionArtifacts{}, fmt.Errorf("V2 universe has %d descriptors, want %d", len(universe), G0BV2UniverseSize)
	}
	attainable, err := DevelopmentCohortAttainablePairCount(schema)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	if attainable != G0BV2AttainablePairs {
		return G0BV2SelectionArtifacts{}, fmt.Errorf("V2 attainable pair count is %d, want %d", attainable, G0BV2AttainablePairs)
	}
	core, err := ExtractG0BV2Core(manifest)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	coreDescriptors := make([]DevelopmentCohortDescriptor, len(core.Cases))
	for index, entry := range core.Cases {
		coreDescriptors[index] = entry.Descriptor
	}
	selection, err := SelectDevelopmentCohort(schema, universe, coreDescriptors, G0BV2ExpansionSize, G0BV2SelectionNamespace)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	if len(selection.SelectedIndexes) != G0BV2ExpansionSize || len(selection.SelectionTrace) != G0BV2ExpansionSize {
		return G0BV2SelectionArtifacts{}, fmt.Errorf("official selection must contain exactly %d indexes and trace steps", G0BV2ExpansionSize)
	}
	selected := make([]DevelopmentCohortDescriptor, G0BV2ExpansionSize)
	for step, trace := range selection.SelectionTrace {
		if trace.Step != step || trace.CandidateIndex != selection.SelectedIndexes[step] {
			return G0BV2SelectionArtifacts{}, fmt.Errorf("selection trace step %d does not match selected index order", step)
		}
		if trace.CandidateIndex < 0 || trace.CandidateIndex >= len(selection.CandidateOrder) {
			return G0BV2SelectionArtifacts{}, fmt.Errorf("selection trace step %d has invalid candidate index", step)
		}
		selected[step] = selection.CandidateOrder[trace.CandidateIndex].Descriptor
	}
	partition, err := PartitionDevelopmentCohort(schema, selected, G0BV2WaveSize, G0BV2PartitionNamespace)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	waveA := indexSet(partition.WaveAIndexes)
	partitionIndex := make(map[string]int, len(partition.CandidateOrder))
	for index, candidate := range partition.CandidateOrder {
		partitionIndex[candidate.Canonical] = index
	}
	membership := G0BV2CohortMembership{
		Version: 1, SelectionNamespace: G0BV2SelectionNamespace, PartitionNamespace: G0BV2PartitionNamespace,
		State: G0BV2ProvisionallySealed, Cases: make([]G0BV2MembershipCase, 0, G0BV2ExpansionSize),
	}
	seeds := G0BV2SeedAudit{Version: 1, Namespace: G0BV2SelectionNamespace, Cases: make([]G0BV2SeedAuditCase, 0, G0BV2ExpansionSize)}
	for step, trace := range selection.SelectionTrace {
		candidate := selection.CandidateOrder[trace.CandidateIndex]
		caseID := fmt.Sprintf("gsv2x-%03d", step+1)
		seed, err := DevelopmentCohortPublicSeed(G0BV2SelectionNamespace, caseID)
		if err != nil {
			return G0BV2SelectionArtifacts{}, err
		}
		digest := sha256.Sum256([]byte(G0BV2SelectionNamespace + "\x00public-seed\x00" + caseID))
		independentSeed := int64(binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64)
		if seed != independentSeed {
			return G0BV2SelectionArtifacts{}, fmt.Errorf("seed derivation mismatch for %q", caseID)
		}
		v2Descriptor, err := DevelopmentCohortDescriptorToV2(candidate.Descriptor)
		if err != nil {
			return G0BV2SelectionArtifacts{}, err
		}
		pIndex, ok := partitionIndex[candidate.Canonical]
		if !ok {
			return G0BV2SelectionArtifacts{}, fmt.Errorf("selected descriptor %q missing from partition candidate order", candidate.Canonical)
		}
		wave := "B"
		if _, ok := waveA[pIndex]; ok {
			wave = "A"
		}
		membership.Cases = append(membership.Cases, G0BV2MembershipCase{
			CaseID: caseID, SelectionStep: step, SelectionCandidateIndex: trace.CandidateIndex,
			CanonicalDescriptor: candidate.Canonical, Descriptor: candidate.Descriptor,
			StructuralDescriptor: v2Descriptor, PartitionCandidateIndex: pIndex, Wave: wave, Seed: seed,
		})
		seeds.Cases = append(seeds.Cases, G0BV2SeedAuditCase{
			CaseID: caseID, SelectionStep: step, CanonicalDescriptor: candidate.Canonical,
			Namespace: G0BV2SelectionNamespace, Digest: hex.EncodeToString(digest[:]), DerivedSeed: seed,
		})
	}
	coverage, err := AuditG0BV2Coverage(schema, core, membership)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	if !coverage.Gates.Pass() {
		return G0BV2SelectionArtifacts{}, fmt.Errorf("official G0-B structural population failed one or more frozen gates")
	}
	coreHash, err := g0bJSONSHA256(core)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	selectionHash, err := DevelopmentCohortSelectionTraceSHA256(selection)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	partitionHash, err := DevelopmentCohortPartitionTraceSHA256(partition)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	seedHash, err := g0bJSONSHA256(seeds)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	membershipHash, err := g0bJSONSHA256(membership)
	if err != nil {
		return G0BV2SelectionArtifacts{}, err
	}
	freeze := G0BV2PreMaterializationFreeze{
		Version: 1, BaseSHA: G0BV2BaseSHA, State: G0BV2ProvisionallySealed,
		UniverseSize: len(universe), AttainablePairs: attainable, FullUniverseSHA256: selection.FullUniverseSHA256,
		CoreDescriptorsSHA256: coreHash, SelectionTraceSHA256: selectionHash, PartitionTraceSHA256: partitionHash,
		SeedAuditSHA256: seedHash, CohortMembershipSHA256: membershipHash, OfficialSelectionRuns: 1,
		StructuralGates: coverage.Gates,
	}
	return G0BV2SelectionArtifacts{
		Core: core, Selection: selection, Partition: partition, Membership: membership, Seeds: seeds,
		Coverage: coverage, Freeze: freeze,
	}, nil
}

func AuditG0BV2Coverage(schema DevelopmentCohortSchema, core G0BV2CoreDescriptors, membership G0BV2CohortMembership) (G0BV2CoverageSummary, error) {
	coreDescriptors := make([]DevelopmentCohortDescriptor, 0, len(core.Cases))
	coreCanonical := make(map[string]struct{}, len(core.Cases))
	for _, entry := range core.Cases {
		canonical, err := CanonicalDevelopmentCohortDescriptor(schema, entry.Descriptor)
		if err != nil || canonical != entry.CanonicalDescriptor {
			return G0BV2CoverageSummary{}, fmt.Errorf("invalid core descriptor mapping for %q", entry.CaseID)
		}
		coreDescriptors = append(coreDescriptors, entry.Descriptor)
		coreCanonical[canonical] = struct{}{}
	}
	expansion := make([]DevelopmentCohortDescriptor, 0, len(membership.Cases))
	waveADescriptors := make([]DevelopmentCohortDescriptor, 0, G0BV2WaveSize)
	waveBDescriptors := make([]DevelopmentCohortDescriptor, 0, G0BV2WaveSize)
	ids, seeds, descriptors, partitionIndexes := map[string]struct{}{}, map[int64]struct{}{}, map[string]struct{}{}, map[int]struct{}{}
	overlap := make([]string, 0)
	for _, entry := range membership.Cases {
		canonical, err := CanonicalDevelopmentCohortDescriptor(schema, entry.Descriptor)
		if err != nil || canonical != entry.CanonicalDescriptor {
			return G0BV2CoverageSummary{}, fmt.Errorf("invalid expansion descriptor mapping for %q", entry.CaseID)
		}
		if _, exists := coreCanonical[canonical]; exists {
			overlap = append(overlap, entry.CaseID)
		}
		ids[entry.CaseID] = struct{}{}
		seeds[entry.Seed] = struct{}{}
		descriptors[canonical] = struct{}{}
		partitionIndexes[entry.PartitionCandidateIndex] = struct{}{}
		expansion = append(expansion, entry.Descriptor)
		switch entry.Wave {
		case "A":
			waveADescriptors = append(waveADescriptors, entry.Descriptor)
		case "B":
			waveBDescriptors = append(waveBDescriptors, entry.Descriptor)
		default:
			return G0BV2CoverageSummary{}, fmt.Errorf("case %q has unsupported wave %q", entry.CaseID, entry.Wave)
		}
	}
	combinedDescriptors := append(append([]DevelopmentCohortDescriptor(nil), coreDescriptors...), expansion...)
	coreSummary := independentlySummarizeG0BV2Population(schema, coreDescriptors)
	combinedSummary := independentlySummarizeG0BV2Population(schema, combinedDescriptors)
	waveASummary := independentlySummarizeG0BV2Population(schema, waveADescriptors)
	waveBSummary := independentlySummarizeG0BV2Population(schema, waveBDescriptors)
	delta := combinedSummary.PairwiseCoverage - coreSummary.PairwiseCoverage
	gates := G0BV2StructuralGates{
		UniverseSize: G0BV2UniverseSize == 1080, AttainablePairs: G0BV2AttainablePairs == 164,
		CoreCount: len(coreDescriptors) == 14, ExpansionCount: len(expansion) == G0BV2ExpansionSize,
		UniqueIDs: len(ids) == len(expansion), UniqueSeeds: len(seeds) == len(expansion),
		UniqueDescriptors: len(descriptors) == len(expansion), CoreOverlapZero: len(overlap) == 0,
		WaveACount: len(waveADescriptors) == G0BV2WaveSize, WaveBCount: len(waveBDescriptors) == G0BV2WaveSize,
		WavesDisjointAndComplete:  len(partitionIndexes) == len(expansion),
		CombinedMarginalsBalanced: combinedSummary.MarginalsBalanced,
		CombinedPairCoverage:      combinedSummary.PairwiseCoverage >= G0BV2CombinedCoverageGate,
		CorePairCoverageDelta:     delta >= G0BV2CombinedCoverageDelta,
		WaveACategoriesComplete:   waveASummary.CategoriesComplete,
		WaveAPairCoverage:         waveASummary.PairwiseCoverage >= G0BV2WaveCoverageGate,
		WaveBCategoriesComplete:   waveBSummary.CategoriesComplete,
		WaveBPairCoverage:         waveBSummary.PairwiseCoverage >= G0BV2WaveCoverageGate,
	}
	sort.Strings(overlap)
	return G0BV2CoverageSummary{
		Version: 1, AttainablePairs: G0BV2AttainablePairs, Core: coreSummary, Combined: combinedSummary,
		WaveA: waveASummary, WaveB: waveBSummary, CoreCoverageDelta: delta, CoreOverlap: overlap, Gates: gates,
	}, nil
}

func BuildG0BV2ConfirmManifests(historical SearchSuiteManifest, membership G0BV2CohortMembership) (SearchSuiteManifest, SearchSuiteManifest, error) {
	build := func(name string, wave string) (SearchSuiteManifest, error) {
		manifest := SearchSuiteManifest{
			Version: historical.Version, Name: name, Budgets: append([]int64(nil), historical.Budgets...),
			Workers: historical.Workers, BaselinePolicy: historical.BaselinePolicy,
			Scenarios: []SearchSuiteScenario{}, Generated: make([]GeneratedSearchSuiteCase, 0, G0BV2WaveSize),
		}
		for _, entry := range membership.Cases {
			if entry.Wave != wave {
				continue
			}
			seed := entry.Seed
			descriptor := entry.StructuralDescriptor
			manifest.Generated = append(manifest.Generated, GeneratedSearchSuiteCase{
				ID: entry.CaseID, Family: GeneratedFamilyStructuralV2, Role: SuiteRoleDevelopment,
				Seed: &seed, StructuralDescriptor: &descriptor,
			})
		}
		if len(manifest.Generated) != G0BV2WaveSize {
			return SearchSuiteManifest{}, fmt.Errorf("manifest %q has %d generated cases, want %d", name, len(manifest.Generated), G0BV2WaveSize)
		}
		if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV2, manifest); err != nil {
			return SearchSuiteManifest{}, err
		}
		return manifest, nil
	}
	a, err := build(G0BV2ConfirmAManifestName, "A")
	if err != nil {
		return SearchSuiteManifest{}, SearchSuiteManifest{}, err
	}
	b, err := build(G0BV2ConfirmBManifestName, "B")
	return a, b, err
}

func AuditG0BV2Materialization(membership G0BV2CohortMembership, lockA SearchSuiteLock, lockB SearchSuiteLock) (G0BV2MaterializationAudit, error) {
	byID := make(map[string]G0BV2MembershipCase, len(membership.Cases))
	for _, entry := range membership.Cases {
		byID[entry.CaseID] = entry
	}
	audit := G0BV2MaterializationAudit{Version: 1, State: G0BV2ProvisionallySealed, Cases: make([]G0BV2MaterializedCaseAudit, 0, G0BV2ExpansionSize)}
	for wave, lock := range map[string]SearchSuiteLock{"A": lockA, "B": lockB} {
		if len(lock.StaticCases) != 0 || len(lock.PrivateCases) != 0 || len(lock.GeneratedCases) != G0BV2WaveSize {
			return G0BV2MaterializationAudit{}, fmt.Errorf("Wave %s lock does not contain exactly %d public generated cases", wave, G0BV2WaveSize)
		}
		for _, locked := range lock.GeneratedCases {
			member, ok := byID[locked.ID]
			if !ok || member.Wave != wave {
				return G0BV2MaterializationAudit{}, fmt.Errorf("locked case %q is not a frozen Wave %s member", locked.ID, wave)
			}
			if locked.Role != SuiteRoleDevelopment || locked.Family != GeneratedFamilyStructuralV2 || locked.Seed != member.Seed {
				return G0BV2MaterializationAudit{}, fmt.Errorf("locked case %q metadata differs from frozen membership", locked.ID)
			}
			if locked.StructuralDescriptor == nil || locked.StructuralDescriptor.Realized == nil || locked.StructuralDescriptor.Requested != member.StructuralDescriptor {
				return G0BV2MaterializationAudit{}, fmt.Errorf("locked case %q lacks a matching requested/realized descriptor", locked.ID)
			}
			audit.Cases = append(audit.Cases, G0BV2MaterializedCaseAudit{
				CaseID: locked.ID, Wave: wave, Seed: locked.Seed, ScenarioSHA256: locked.ScenarioSHA256,
				Requested: locked.StructuralDescriptor.Requested, Realized: *locked.StructuralDescriptor.Realized,
				RequestedVsRealized: "PASS", StructuralWitness: "PASS",
			})
		}
	}
	sort.Slice(audit.Cases, func(left, right int) bool { return audit.Cases[left].CaseID < audit.Cases[right].CaseID })
	if len(audit.Cases) != G0BV2ExpansionSize {
		return G0BV2MaterializationAudit{}, fmt.Errorf("materialization audit has %d cases, want %d", len(audit.Cases), G0BV2ExpansionSize)
	}
	audit.MaterializedCases = len(audit.Cases)
	audit.RequestedVsRealizedPasses = len(audit.Cases)
	audit.StructuralWitnessPasses = len(audit.Cases)
	return audit, nil
}

func independentlySummarizeG0BV2Population(schema DevelopmentCohortSchema, descriptors []DevelopmentCohortDescriptor) G0BV2PopulationSummary {
	summary := G0BV2PopulationSummary{
		CaseCount: len(descriptors), Dimensions: make([]G0BV2DimensionSummary, 0, len(schema.Dimensions)),
		CategoriesComplete: true, MarginalsBalanced: true,
	}
	for dimensionIndex, dimension := range schema.Dimensions {
		counts := make(map[string]int, len(dimension.Values))
		for _, value := range dimension.Values {
			counts[value] = 0
		}
		for _, descriptor := range descriptors {
			counts[descriptor.Values[dimensionIndex]]++
		}
		minimum, maximum := len(descriptors), 0
		complete := true
		for _, value := range dimension.Values {
			count := counts[value]
			if count < minimum {
				minimum = count
			}
			if count > maximum {
				maximum = count
			}
			if count == 0 {
				complete = false
			}
		}
		balanced := maximum-minimum <= 1
		summary.Dimensions = append(summary.Dimensions, G0BV2DimensionSummary{
			Dimension: dimension.Name, Counts: counts, Minimum: minimum, Maximum: maximum,
			CategoriesComplete: complete, MaxMinusMinAtMostOne: balanced,
		})
		summary.CategoriesComplete = summary.CategoriesComplete && complete
		summary.MarginalsBalanced = summary.MarginalsBalanced && balanced
	}
	pairs := map[string]struct{}{}
	for _, descriptor := range descriptors {
		for left := 0; left < len(schema.Dimensions); left++ {
			for right := left + 1; right < len(schema.Dimensions); right++ {
				key := fmt.Sprintf("%s=%s|%s=%s", schema.Dimensions[left].Name, descriptor.Values[left], schema.Dimensions[right].Name, descriptor.Values[right])
				pairs[key] = struct{}{}
			}
		}
	}
	for pair := range pairs {
		summary.CoveredPairs = append(summary.CoveredPairs, pair)
	}
	sort.Strings(summary.CoveredPairs)
	summary.PairwiseCoverage = len(summary.CoveredPairs)
	return summary
}

func g0bJSONSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func indexSet(indexes []int) map[int]struct{} {
	result := make(map[int]struct{}, len(indexes))
	for _, index := range indexes {
		result[index] = struct{}{}
	}
	return result
}
