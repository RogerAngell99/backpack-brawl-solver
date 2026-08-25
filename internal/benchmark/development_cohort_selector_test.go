package benchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestDevelopmentCohortSelectorIsDeterministicAndInputOrderInvariant(t *testing.T) {
	schema := developmentCohortFixtureSchema()
	universe, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	core := []DevelopmentCohortDescriptor{
		{Values: []string{"a", "x", "low"}},
		{Values: []string{"b", "y", "high"}},
	}
	first, err := SelectDevelopmentCohort(schema, universe, core, 6, "fixture-selection-v1")
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]DevelopmentCohortDescriptor(nil), universe...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	second, err := SelectDevelopmentCohort(schema, reversed, core, 6, "fixture-selection-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("selection changed with input order\nfirst=%+v\nsecond=%+v", first, second)
	}
	if len(first.SelectedIndexes) != 6 || len(first.SelectionTrace) != 6 {
		t.Fatalf("selected=%d trace=%d", len(first.SelectedIndexes), len(first.SelectionTrace))
	}

	seen := map[string]struct{}{}
	for _, index := range first.SelectedIndexes {
		candidate := first.CandidateOrder[index]
		if _, exists := seen[candidate.Canonical]; exists {
			t.Fatalf("duplicate selected descriptor %s", candidate.Canonical)
		}
		seen[candidate.Canonical] = struct{}{}
	}
	for _, descriptor := range core {
		canonical, err := CanonicalDevelopmentCohortDescriptor(schema, descriptor)
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := seen[canonical]; exists {
			t.Fatalf("selector repeated core descriptor %s", canonical)
		}
	}
	assertDevelopmentCohortCategoriesRepresented(t, schema, append(core, selectedDevelopmentCohortDescriptors(first)...))
}

func TestDevelopmentCohortSelectorTieUsesOnlyDomainSeparatedHash(t *testing.T) {
	schema := DevelopmentCohortSchema{Version: 1, Dimensions: []DevelopmentCohortDimension{
		{Name: "left", Values: []string{"a", "b"}},
		{Name: "right", Values: []string{"x", "y"}},
	}}
	universe, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SelectDevelopmentCohort(schema, universe, nil, 1, "fixture-tie-v1")
	if err != nil {
		t.Fatal(err)
	}
	wantIndex := -1
	wantHash := ""
	for index, candidate := range selection.CandidateOrder {
		digest := sha256.Sum256([]byte("fixture-tie-v1\x00" + candidate.Canonical))
		hash := hex.EncodeToString(digest[:])
		if wantIndex < 0 || hash < wantHash {
			wantIndex, wantHash = index, hash
		}
	}
	if got := selection.SelectedIndexes[0]; got != wantIndex {
		t.Fatalf("tie selected index %d, want hash-minimum index %d", got, wantIndex)
	}
	metric := selection.SelectionTrace[0].Metric
	if metric.NewPairwiseCoverage != 1 || metric.MinimumHammingDistance != 2 || metric.TieBreakSHA256 != wantHash {
		t.Fatalf("unexpected frozen tie metric: %+v", metric)
	}
}

func TestDevelopmentCohortSelectorGoldenTrace(t *testing.T) {
	schema := developmentCohortFixtureSchema()
	universe, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	selection, err := SelectDevelopmentCohort(schema, universe, []DevelopmentCohortDescriptor{
		{Values: []string{"a", "x", "low"}},
		{Values: []string{"c", "y", "high"}},
	}, 7, "fixture-golden-selection-v1")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := DevelopmentCohortSelectionTraceSHA256(selection)
	if err != nil {
		t.Fatal(err)
	}
	const want = "fdfc6dc7b5f368bdf1876977ba39e8b0cec0cd410e69e88d47bd1b22366643ed"
	if hash != want {
		t.Fatalf("selection trace hash=%s want=%s", hash, want)
	}
}

func TestDevelopmentCohortPartitionIsBalancedDeterministicAndLocallyStable(t *testing.T) {
	schema := developmentCohortFixtureSchema()
	cohort, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	first, err := PartitionDevelopmentCohort(schema, cohort, 6, "fixture-partition-v1")
	if err != nil {
		t.Fatal(err)
	}
	reversed := append([]DevelopmentCohortDescriptor(nil), cohort...)
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	second, err := PartitionDevelopmentCohort(schema, reversed, 6, "fixture-partition-v1")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("partition changed with cohort input order")
	}
	if len(first.WaveAIndexes) != 6 || len(first.WaveBIndexes) != 6 {
		t.Fatalf("wave sizes A=%d B=%d", len(first.WaveAIndexes), len(first.WaveBIndexes))
	}
	if len(first.GreedyTrace) != 6 {
		t.Fatalf("greedy trace steps=%d want=6", len(first.GreedyTrace))
	}
	descriptors := make([]DevelopmentCohortDescriptor, len(first.CandidateOrder))
	for index, candidate := range first.CandidateOrder {
		descriptors[index] = candidate.Descriptor
	}
	greedyWaveA := map[int]struct{}{}
	for stepIndex, step := range first.GreedyTrace {
		if step.Step != stepIndex {
			t.Fatalf("greedy trace step=%d at index=%d", step.Step, stepIndex)
		}
		if step.CandidateIndex < 0 || step.CandidateIndex >= len(first.CandidateOrder) {
			t.Fatalf("greedy trace candidate index=%d is out of range", step.CandidateIndex)
		}
		if _, exists := greedyWaveA[step.CandidateIndex]; exists {
			t.Fatalf("greedy trace repeats candidate %d", step.CandidateIndex)
		}
		provisional := cloneDevelopmentCohortIndexSet(greedyWaveA)
		provisional[step.CandidateIndex] = struct{}{}
		wantMetric := publicDevelopmentCohortPartitionGreedyMetric(developmentCohortPartitionGreedyMetricFor(
			schema,
			descriptors,
			provisional,
			6,
			first.CandidateOrder[step.CandidateIndex],
			"fixture-partition-v1",
		))
		if !reflect.DeepEqual(step.Metric, wantMetric) {
			t.Fatalf("greedy trace step %d metric=%+v want=%+v", stepIndex, step.Metric, wantMetric)
		}
		greedyWaveA[step.CandidateIndex] = struct{}{}
	}
	initialObjective := publicDevelopmentCohortPartitionObjective(
		developmentCohortPartitionObjectiveFor(schema, descriptors, greedyWaveA, 6, "fixture-partition-v1"),
	)
	if !reflect.DeepEqual(initialObjective, first.InitialObjective) {
		t.Fatalf("greedy trace objective=%+v want initial=%+v", initialObjective, first.InitialObjective)
	}
	for _, swap := range first.SwapTrace {
		if _, exists := greedyWaveA[swap.RemovedFromWaveA]; !exists {
			t.Fatalf("swap %d removes absent candidate %d", swap.Iteration, swap.RemovedFromWaveA)
		}
		if _, exists := greedyWaveA[swap.AddedToWaveA]; exists {
			t.Fatalf("swap %d adds existing candidate %d", swap.Iteration, swap.AddedToWaveA)
		}
		delete(greedyWaveA, swap.RemovedFromWaveA)
		greedyWaveA[swap.AddedToWaveA] = struct{}{}
	}
	reconstructedWaveA, _ := developmentCohortPartitionIndexes(len(first.CandidateOrder), greedyWaveA)
	if !reflect.DeepEqual(reconstructedWaveA, first.WaveAIndexes) {
		t.Fatalf("greedy and swap traces reconstruct Wave A %v, want %v", reconstructedWaveA, first.WaveAIndexes)
	}
	seen := map[int]string{}
	for _, index := range first.WaveAIndexes {
		seen[index] = "A"
	}
	for _, index := range first.WaveBIndexes {
		if previous := seen[index]; previous != "" {
			t.Fatalf("candidate %d appears in waves %s and B", index, previous)
		}
		seen[index] = "B"
	}
	if len(seen) != len(cohort) {
		t.Fatalf("partition covers %d of %d descriptors", len(seen), len(cohort))
	}
	assertDevelopmentCohortCategoriesRepresented(t, schema, partitionDevelopmentCohortDescriptors(first, first.WaveAIndexes))
	assertDevelopmentCohortCategoriesRepresented(t, schema, partitionDevelopmentCohortDescriptors(first, first.WaveBIndexes))

	waveA := map[int]struct{}{}
	for _, index := range first.WaveAIndexes {
		waveA[index] = struct{}{}
	}
	current := developmentCohortPartitionObjectiveFor(schema, descriptors, waveA, 6, "fixture-partition-v1")
	for _, removeIndex := range first.WaveAIndexes {
		for _, addIndex := range first.WaveBIndexes {
			provisional := cloneDevelopmentCohortIndexSet(waveA)
			delete(provisional, removeIndex)
			provisional[addIndex] = struct{}{}
			candidate := developmentCohortPartitionObjectiveFor(schema, descriptors, provisional, 6, "fixture-partition-v1")
			if developmentCohortPartitionObjectiveLess(candidate, current) {
				t.Fatalf("final partition admits improving swap A[%d] for B[%d]", removeIndex, addIndex)
			}
		}
	}
}

func TestDevelopmentCohortPartitionGoldenTrace(t *testing.T) {
	schema := developmentCohortFixtureSchema()
	cohort, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	partition, err := PartitionDevelopmentCohort(schema, cohort, 6, "fixture-golden-partition-v1")
	if err != nil {
		t.Fatal(err)
	}
	hash, err := DevelopmentCohortPartitionTraceSHA256(partition)
	if err != nil {
		t.Fatal(err)
	}
	const want = "cfd9c99b69935ef3d14dd10d9f25109521590fa36035c05bc64b3d50bc41d39b"
	if hash != want {
		t.Fatalf("partition trace hash=%s want=%s", hash, want)
	}
}

func TestDevelopmentCohortV2SchemaAndConversionsAreFrozen(t *testing.T) {
	schema := SearchSuiteV2DevelopmentCohortSchema()
	universe, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	if len(universe) != 1080 {
		t.Fatalf("V2 structural universe=%d want=1080", len(universe))
	}
	for _, descriptor := range []GeneratedSearchSuiteStructuralDescriptor{
		validStructuralDescriptorForTest(),
		{
			GridTopology: GridTopologyNarrowCorridors, DensityBand: DensityBandD97,
			SourceMultiplicity: SourceMultiplicityTwoTwo, TargetOverlap: TargetOverlapMostlyShared,
			CopySymmetry: CopySymmetryHigh, RotationEntropy: RotationEntropyHigh,
		},
	} {
		generic, err := DevelopmentCohortDescriptorFromV2(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip, err := DevelopmentCohortDescriptorToV2(generic)
		if err != nil || !reflect.DeepEqual(roundTrip, descriptor) {
			t.Fatalf("round trip=%+v want=%+v err=%v", roundTrip, descriptor, err)
		}
	}
}

func TestDevelopmentCohortAttainablePairCountsAreFrozen(t *testing.T) {
	v2Pairs, err := DevelopmentCohortAttainablePairCount(SearchSuiteV2DevelopmentCohortSchema())
	if err != nil {
		t.Fatal(err)
	}
	if v2Pairs != 164 {
		t.Fatalf("V2 attainable categorical value pairs=%d want=164", v2Pairs)
	}

	v3Schema := DevelopmentCohortSchema{Version: 1, Dimensions: []DevelopmentCohortDimension{
		{Name: "grid_topology", Values: []string{"full", "bottleneck", "holes", "two-lobes", "narrow-corridors"}},
		{Name: "density", Values: []string{"sparse", "dense", "very-dense"}},
		{Name: "source_count", Values: []string{"2", "3"}},
		{Name: "source_copies", Values: []string{"singleton", "mixed"}},
		{Name: "compatibility_graph", Values: []string{"mostly-exclusive", "mixed", "mostly-shared"}},
		{Name: "target_count", Values: []string{"small", "large"}},
		{Name: "filler_symmetry", Values: []string{"low", "high"}},
		{Name: "rotation_entropy", Values: []string{"low", "high"}},
	}}
	v3Pairs, err := DevelopmentCohortAttainablePairCount(v3Schema)
	if err != nil {
		t.Fatal(err)
	}
	if v3Pairs != 189 {
		t.Fatalf("V3 attainable categorical value pairs=%d want=189", v3Pairs)
	}
}

func TestDevelopmentCohortSeedDerivationIsMechanicalAndDomainSeparated(t *testing.T) {
	first, err := DevelopmentCohortPublicSeed("fixture-seeds-v1", "case-001")
	if err != nil {
		t.Fatal(err)
	}
	repeat, _ := DevelopmentCohortPublicSeed("fixture-seeds-v1", "case-001")
	otherCase, _ := DevelopmentCohortPublicSeed("fixture-seeds-v1", "case-002")
	otherNamespace, _ := DevelopmentCohortPublicSeed("fixture-seeds-v2", "case-001")
	if first < 0 || first != repeat || first == otherCase || first == otherNamespace {
		t.Fatalf("seeds first=%d repeat=%d other-case=%d other-namespace=%d", first, repeat, otherCase, otherNamespace)
	}
}

func TestDevelopmentCohortSelectorRejectsInvalidInputs(t *testing.T) {
	schema := developmentCohortFixtureSchema()
	universe, err := EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		universe  []DevelopmentCohortDescriptor
		core      []DevelopmentCohortDescriptor
		size      int
		namespace string
		want      string
	}{
		{name: "empty namespace", universe: universe, size: 1, want: "namespace"},
		{name: "zero size", universe: universe, size: 0, namespace: "fixture", want: "positive"},
		{name: "duplicate universe", universe: append(universe, universe[0]), size: 1, namespace: "fixture", want: "repeats"},
		{name: "invalid core", universe: universe, core: []DevelopmentCohortDescriptor{{Values: []string{"invalid", "x", "low"}}}, size: 1, namespace: "fixture", want: "existing core"},
		{name: "too large", universe: universe[:1], core: []DevelopmentCohortDescriptor{universe[0]}, size: 1, namespace: "fixture", want: "exceeds"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := SelectDevelopmentCohort(schema, test.universe, test.core, test.size, test.namespace)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("err=%v want substring %q", err, test.want)
			}
		})
	}
	if _, err := PartitionDevelopmentCohort(schema, universe, len(universe), "fixture"); err == nil || !strings.Contains(err.Error(), "wave A size") {
		t.Fatalf("partition size err=%v", err)
	}
}

func TestDevelopmentCohortImplementationHasOutcomeBlindDependencyBoundary(t *testing.T) {
	files, err := filepath.Glob("development_cohort_*.go")
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"backpack-brawl-solver/internal/solver",
		"SolveLayout",
		"CompareScores",
		"SolutionLess",
		"SearchStats",
		"RepairNodes",
	}
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(content), token) {
				t.Errorf("%s crosses outcome-blind boundary with %q", path, token)
			}
		}
	}
}

func TestGeneralSearchV2ManifestRemainsByteFrozen(t *testing.T) {
	assertLFNormalizedFileSHA256(t,
		filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json"),
		"5d1757c37580b04c9a85b738ea2672d8a0b3c8402c8ed5a509c8c42fd5d4b513",
	)
}

func TestGeneralSearchV2LockRemainsByteFrozen(t *testing.T) {
	assertLFNormalizedFileSHA256(t,
		filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.lock"),
		"96af8290e8741b4ef6f514b0df32820f8ccd241695eb5b1d671fc6cc2fd5aa6d",
	)
}

func assertLFNormalizedFileSHA256(t *testing.T, path string, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Git stores these text files with LF. Normalize the checkout transport so
	// core.autocrlf cannot make the repository-content hash platform-dependent.
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	digest := sha256.Sum256(content)
	if got := hex.EncodeToString(digest[:]); got != want {
		t.Fatalf("%s LF-normalized SHA-256=%s want frozen=%s", path, got, want)
	}
}

func developmentCohortFixtureSchema() DevelopmentCohortSchema {
	return DevelopmentCohortSchema{Version: 1, Dimensions: []DevelopmentCohortDimension{
		{Name: "topology", Values: []string{"a", "b", "c"}},
		{Name: "density", Values: []string{"x", "y"}},
		{Name: "symmetry", Values: []string{"low", "high"}},
	}}
}

func selectedDevelopmentCohortDescriptors(selection DevelopmentCohortSelection) []DevelopmentCohortDescriptor {
	result := make([]DevelopmentCohortDescriptor, 0, len(selection.SelectedIndexes))
	for _, index := range selection.SelectedIndexes {
		result = append(result, selection.CandidateOrder[index].Descriptor)
	}
	return result
}

func partitionDevelopmentCohortDescriptors(partition DevelopmentCohortPartition, indexes []int) []DevelopmentCohortDescriptor {
	result := make([]DevelopmentCohortDescriptor, 0, len(indexes))
	for _, index := range indexes {
		result = append(result, partition.CandidateOrder[index].Descriptor)
	}
	return result
}

func assertDevelopmentCohortCategoriesRepresented(t *testing.T, schema DevelopmentCohortSchema, descriptors []DevelopmentCohortDescriptor) {
	t.Helper()
	for dimensionIndex, dimension := range schema.Dimensions {
		seen := map[string]struct{}{}
		for _, descriptor := range descriptors {
			seen[descriptor.Values[dimensionIndex]] = struct{}{}
		}
		missing := make([]string, 0)
		for _, value := range dimension.Values {
			if _, exists := seen[value]; !exists {
				missing = append(missing, value)
			}
		}
		sort.Strings(missing)
		if len(missing) > 0 {
			t.Fatalf("dimension %q lacks values %v", dimension.Name, missing)
		}
	}
}
