package benchmark

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
)

const developmentCohortAlgorithmVersion = 1

// DevelopmentCohortDimension defines one ordered categorical dimension in an
// outcome-blind descriptor space. Names and values are part of the frozen
// sampling contract and therefore must remain stable within a schema version.
type DevelopmentCohortDimension struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// DevelopmentCohortSchema defines the categorical descriptor space consumed
// by the generalization-hardening selector. It deliberately cannot carry
// solver outputs, scores, timings, or search diagnostics.
type DevelopmentCohortSchema struct {
	Version    int                          `json:"version"`
	Dimensions []DevelopmentCohortDimension `json:"dimensions"`
}

// DevelopmentCohortDescriptor contains one value for each schema dimension,
// in schema order.
type DevelopmentCohortDescriptor struct {
	Values []string `json:"values"`
}

// DevelopmentCohortCandidate is one descriptor in canonical universe order.
// Indexes reported by selection and partition results refer to this order.
type DevelopmentCohortCandidate struct {
	Index          int                         `json:"index"`
	Descriptor     DevelopmentCohortDescriptor `json:"descriptor"`
	Canonical      string                      `json:"canonical"`
	TieBreakSHA256 string                      `json:"tie_break_sha256"`
}

// DevelopmentCohortSelectionMetric is the frozen lexicographic tuple used at
// one greedy selection step. MarginalImbalance is an exact rational number.
type DevelopmentCohortSelectionMetric struct {
	MarginalImbalance      string `json:"marginal_imbalance"`
	NewPairwiseCoverage    int    `json:"new_pairwise_coverage"`
	MinimumHammingDistance int    `json:"minimum_hamming_distance"`
	TieBreakSHA256         string `json:"tie_break_sha256"`
}

// DevelopmentCohortSelectionStep records why one descriptor won a selection
// step without containing any materialized scenario or solver measurement.
type DevelopmentCohortSelectionStep struct {
	Step           int                              `json:"step"`
	CandidateIndex int                              `json:"candidate_index"`
	Metric         DevelopmentCohortSelectionMetric `json:"metric"`
}

// DevelopmentCohortSelection is the reproducible structural selection audit.
// Existing core descriptors remain inputs only and are not copied into it.
type DevelopmentCohortSelection struct {
	Version            int                              `json:"version"`
	Namespace          string                           `json:"namespace"`
	FullUniverseSHA256 string                           `json:"full_universe_sha256"`
	CandidateOrder     []DevelopmentCohortCandidate     `json:"candidate_order"`
	SelectedIndexes    []int                            `json:"selected_indexes"`
	SelectionTrace     []DevelopmentCohortSelectionStep `json:"selection_trace"`
}

type developmentCohortSelectionMetricInternal struct {
	marginalImbalance      *big.Rat
	newPairwiseCoverage    int
	minimumHammingDistance int
	tieBreakSHA256         string
	canonical              string
}

// EnumerateDevelopmentCohortUniverse returns the schema's Cartesian product in
// schema value order. Selection itself canonicalizes the input, so callers do
// not need to preserve this enumeration order.
func EnumerateDevelopmentCohortUniverse(schema DevelopmentCohortSchema) ([]DevelopmentCohortDescriptor, error) {
	if err := schema.validate(); err != nil {
		return nil, err
	}
	universe := []DevelopmentCohortDescriptor{{Values: make([]string, len(schema.Dimensions))}}
	for dimensionIndex, dimension := range schema.Dimensions {
		next := make([]DevelopmentCohortDescriptor, 0, len(universe)*len(dimension.Values))
		for _, prefix := range universe {
			for _, value := range dimension.Values {
				values := append([]string(nil), prefix.Values...)
				values[dimensionIndex] = value
				next = append(next, DevelopmentCohortDescriptor{Values: values})
			}
		}
		universe = next
	}
	return universe, nil
}

// SelectDevelopmentCohort applies the frozen stratified deterministic
// max-coverage algorithm. It minimizes exact marginal imbalance, then
// maximizes new pairwise coverage and minimum Hamming distance, and finally
// minimizes a domain-separated SHA-256 tie-break.
func SelectDevelopmentCohort(
	schema DevelopmentCohortSchema,
	universe []DevelopmentCohortDescriptor,
	existingCore []DevelopmentCohortDescriptor,
	requestedSize int,
	namespace string,
) (DevelopmentCohortSelection, error) {
	if err := schema.validate(); err != nil {
		return DevelopmentCohortSelection{}, err
	}
	if err := validateDevelopmentCohortNamespace(namespace); err != nil {
		return DevelopmentCohortSelection{}, err
	}
	if requestedSize <= 0 {
		return DevelopmentCohortSelection{}, fmt.Errorf("requested cohort size must be positive")
	}

	candidates, universeHash, err := canonicalDevelopmentCohortCandidates(schema, universe, namespace)
	if err != nil {
		return DevelopmentCohortSelection{}, err
	}
	coreCanonical := make(map[string]struct{}, len(existingCore))
	population := make([]DevelopmentCohortDescriptor, 0, len(existingCore)+requestedSize)
	for index, descriptor := range existingCore {
		canonical, err := CanonicalDevelopmentCohortDescriptor(schema, descriptor)
		if err != nil {
			return DevelopmentCohortSelection{}, fmt.Errorf("existing core descriptor %d: %w", index, err)
		}
		coreCanonical[canonical] = struct{}{}
		population = append(population, cloneDevelopmentCohortDescriptor(descriptor))
	}

	eligible := 0
	for _, candidate := range candidates {
		if _, exists := coreCanonical[candidate.Canonical]; !exists {
			eligible++
		}
	}
	if requestedSize > eligible {
		return DevelopmentCohortSelection{}, fmt.Errorf("requested cohort size %d exceeds %d descriptors outside the existing core", requestedSize, eligible)
	}

	result := DevelopmentCohortSelection{
		Version:            developmentCohortAlgorithmVersion,
		Namespace:          namespace,
		FullUniverseSHA256: universeHash,
		CandidateOrder:     candidates,
		SelectedIndexes:    make([]int, 0, requestedSize),
		SelectionTrace:     make([]DevelopmentCohortSelectionStep, 0, requestedSize),
	}
	selected := make(map[int]struct{}, requestedSize)
	coveredPairs := developmentCohortPairwiseCoverage(population)

	for step := 0; step < requestedSize; step++ {
		bestIndex := -1
		var best developmentCohortSelectionMetricInternal
		for index, candidate := range candidates {
			if _, isCore := coreCanonical[candidate.Canonical]; isCore {
				continue
			}
			if _, isSelected := selected[index]; isSelected {
				continue
			}
			candidatePopulation := appendCloneDevelopmentCohortDescriptor(population, candidate.Descriptor)
			metric := developmentCohortSelectionMetricInternal{
				marginalImbalance:      developmentCohortMarginalImbalance(schema, candidatePopulation),
				newPairwiseCoverage:    developmentCohortNewPairwiseCoverage(coveredPairs, candidate.Descriptor),
				minimumHammingDistance: developmentCohortMinimumHammingDistance(candidate.Descriptor, population, len(schema.Dimensions)),
				tieBreakSHA256:         candidate.TieBreakSHA256,
				canonical:              candidate.Canonical,
			}
			if bestIndex < 0 || developmentCohortSelectionMetricLess(metric, best) {
				bestIndex = index
				best = metric
			}
		}
		if bestIndex < 0 {
			return DevelopmentCohortSelection{}, fmt.Errorf("selector exhausted candidates at step %d", step)
		}

		selected[bestIndex] = struct{}{}
		selectedDescriptor := candidates[bestIndex].Descriptor
		population = append(population, cloneDevelopmentCohortDescriptor(selectedDescriptor))
		developmentCohortAddPairwiseCoverage(coveredPairs, selectedDescriptor)
		result.SelectedIndexes = append(result.SelectedIndexes, bestIndex)
		result.SelectionTrace = append(result.SelectionTrace, DevelopmentCohortSelectionStep{
			Step:           step,
			CandidateIndex: bestIndex,
			Metric: DevelopmentCohortSelectionMetric{
				MarginalImbalance:      best.marginalImbalance.RatString(),
				NewPairwiseCoverage:    best.newPairwiseCoverage,
				MinimumHammingDistance: best.minimumHammingDistance,
				TieBreakSHA256:         best.tieBreakSHA256,
			},
		})
	}

	return result, nil
}

// CanonicalDevelopmentCohortDescriptor returns a stable JSON array of
// [dimension-name, value] pairs in schema order.
func CanonicalDevelopmentCohortDescriptor(schema DevelopmentCohortSchema, descriptor DevelopmentCohortDescriptor) (string, error) {
	if err := schema.validate(); err != nil {
		return "", err
	}
	if err := schema.validateDescriptor(descriptor); err != nil {
		return "", err
	}
	pairs := make([][2]string, len(schema.Dimensions))
	for index, dimension := range schema.Dimensions {
		pairs[index] = [2]string{dimension.Name, descriptor.Values[index]}
	}
	encoded, err := json.Marshal(pairs)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// DevelopmentCohortPublicSeed derives a non-negative seed solely from a
// frozen namespace and neutral case ID.
func DevelopmentCohortPublicSeed(namespace string, caseID string) (int64, error) {
	if err := validateDevelopmentCohortNamespace(namespace); err != nil {
		return 0, err
	}
	if caseID == "" || strings.ContainsRune(caseID, '\x00') {
		return 0, fmt.Errorf("development cohort case ID must be non-empty and contain no NUL")
	}
	digest := sha256.Sum256([]byte(namespace + "\x00public-seed\x00" + caseID))
	return int64(binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64), nil
}

// DevelopmentCohortSelectionTraceSHA256 hashes the complete structural audit
// record. The result contains no scenario materialization or solver outcome.
func DevelopmentCohortSelectionTraceSHA256(selection DevelopmentCohortSelection) (string, error) {
	encoded, err := json.Marshal(selection)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func (schema DevelopmentCohortSchema) validate() error {
	if schema.Version != developmentCohortAlgorithmVersion {
		return fmt.Errorf("development cohort schema requires version %d", developmentCohortAlgorithmVersion)
	}
	if len(schema.Dimensions) < 2 {
		return fmt.Errorf("development cohort schema requires at least two dimensions")
	}
	seenNames := map[string]struct{}{}
	for dimensionIndex, dimension := range schema.Dimensions {
		if dimension.Name == "" || strings.ContainsRune(dimension.Name, '\x00') {
			return fmt.Errorf("development cohort dimension %d requires a non-empty NUL-free name", dimensionIndex)
		}
		if _, exists := seenNames[dimension.Name]; exists {
			return fmt.Errorf("duplicate development cohort dimension %q", dimension.Name)
		}
		seenNames[dimension.Name] = struct{}{}
		if len(dimension.Values) < 2 {
			return fmt.Errorf("development cohort dimension %q requires at least two values", dimension.Name)
		}
		seenValues := map[string]struct{}{}
		for _, value := range dimension.Values {
			if value == "" || strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("development cohort dimension %q requires non-empty NUL-free values", dimension.Name)
			}
			if _, exists := seenValues[value]; exists {
				return fmt.Errorf("development cohort dimension %q repeats value %q", dimension.Name, value)
			}
			seenValues[value] = struct{}{}
		}
	}
	return nil
}

func (schema DevelopmentCohortSchema) validateDescriptor(descriptor DevelopmentCohortDescriptor) error {
	if len(descriptor.Values) != len(schema.Dimensions) {
		return fmt.Errorf("development cohort descriptor has %d values, want %d", len(descriptor.Values), len(schema.Dimensions))
	}
	for index, value := range descriptor.Values {
		if !oneOf(value, schema.Dimensions[index].Values...) {
			return fmt.Errorf("development cohort descriptor dimension %q has unsupported value %q", schema.Dimensions[index].Name, value)
		}
	}
	return nil
}

func validateDevelopmentCohortNamespace(namespace string) error {
	if namespace == "" || strings.ContainsRune(namespace, '\x00') {
		return fmt.Errorf("development cohort namespace must be non-empty and contain no NUL")
	}
	return nil
}

func canonicalDevelopmentCohortCandidates(
	schema DevelopmentCohortSchema,
	universe []DevelopmentCohortDescriptor,
	namespace string,
) ([]DevelopmentCohortCandidate, string, error) {
	if len(universe) == 0 {
		return nil, "", fmt.Errorf("development cohort universe must not be empty")
	}
	type canonicalDescriptor struct {
		canonical  string
		descriptor DevelopmentCohortDescriptor
	}
	ordered := make([]canonicalDescriptor, 0, len(universe))
	seen := make(map[string]struct{}, len(universe))
	for index, descriptor := range universe {
		canonical, err := CanonicalDevelopmentCohortDescriptor(schema, descriptor)
		if err != nil {
			return nil, "", fmt.Errorf("universe descriptor %d: %w", index, err)
		}
		if _, exists := seen[canonical]; exists {
			return nil, "", fmt.Errorf("development cohort universe repeats descriptor %s", canonical)
		}
		seen[canonical] = struct{}{}
		ordered = append(ordered, canonicalDescriptor{canonical: canonical, descriptor: cloneDevelopmentCohortDescriptor(descriptor)})
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].canonical < ordered[right].canonical })

	candidates := make([]DevelopmentCohortCandidate, len(ordered))
	canonicalOrder := make([]string, len(ordered))
	for index, candidate := range ordered {
		tieDigest := sha256.Sum256([]byte(namespace + "\x00" + candidate.canonical))
		candidates[index] = DevelopmentCohortCandidate{
			Index:          index,
			Descriptor:     candidate.descriptor,
			Canonical:      candidate.canonical,
			TieBreakSHA256: hex.EncodeToString(tieDigest[:]),
		}
		canonicalOrder[index] = candidate.canonical
	}
	universeDigest := sha256.Sum256([]byte(namespace + "\x00development-cohort-universe-v1\x00" + strings.Join(canonicalOrder, "\n")))
	return candidates, hex.EncodeToString(universeDigest[:]), nil
}

func developmentCohortSelectionMetricLess(left developmentCohortSelectionMetricInternal, right developmentCohortSelectionMetricInternal) bool {
	if comparison := left.marginalImbalance.Cmp(right.marginalImbalance); comparison != 0 {
		return comparison < 0
	}
	if left.newPairwiseCoverage != right.newPairwiseCoverage {
		return left.newPairwiseCoverage > right.newPairwiseCoverage
	}
	if left.minimumHammingDistance != right.minimumHammingDistance {
		return left.minimumHammingDistance > right.minimumHammingDistance
	}
	if left.tieBreakSHA256 != right.tieBreakSHA256 {
		return left.tieBreakSHA256 < right.tieBreakSHA256
	}
	return left.canonical < right.canonical
}

func developmentCohortMarginalImbalance(schema DevelopmentCohortSchema, population []DevelopmentCohortDescriptor) *big.Rat {
	result := new(big.Rat)
	populationSize := int64(len(population))
	for dimensionIndex, dimension := range schema.Dimensions {
		counts := make(map[string]int64, len(dimension.Values))
		for _, descriptor := range population {
			counts[descriptor.Values[dimensionIndex]]++
		}
		categoryCount := int64(len(dimension.Values))
		denominator := big.NewInt(categoryCount * categoryCount)
		for _, value := range dimension.Values {
			delta := big.NewInt(categoryCount*counts[value] - populationSize)
			delta.Mul(delta, delta)
			result.Add(result, new(big.Rat).SetFrac(delta, denominator))
		}
	}
	return result
}

func developmentCohortPairwiseCoverage(population []DevelopmentCohortDescriptor) map[string]struct{} {
	covered := map[string]struct{}{}
	for _, descriptor := range population {
		developmentCohortAddPairwiseCoverage(covered, descriptor)
	}
	return covered
}

func developmentCohortAddPairwiseCoverage(covered map[string]struct{}, descriptor DevelopmentCohortDescriptor) {
	for left := 0; left < len(descriptor.Values); left++ {
		for right := left + 1; right < len(descriptor.Values); right++ {
			covered[developmentCohortPairKey(left, descriptor.Values[left], right, descriptor.Values[right])] = struct{}{}
		}
	}
}

func developmentCohortNewPairwiseCoverage(covered map[string]struct{}, descriptor DevelopmentCohortDescriptor) int {
	count := 0
	for left := 0; left < len(descriptor.Values); left++ {
		for right := left + 1; right < len(descriptor.Values); right++ {
			key := developmentCohortPairKey(left, descriptor.Values[left], right, descriptor.Values[right])
			if _, exists := covered[key]; !exists {
				count++
			}
		}
	}
	return count
}

func developmentCohortPairKey(left int, leftValue string, right int, rightValue string) string {
	return fmt.Sprintf("%d\x00%s\x00%d\x00%s", left, leftValue, right, rightValue)
}

func developmentCohortMinimumHammingDistance(
	descriptor DevelopmentCohortDescriptor,
	population []DevelopmentCohortDescriptor,
	dimensionCount int,
) int {
	if len(population) == 0 {
		return dimensionCount
	}
	minimum := dimensionCount
	for _, other := range population {
		distance := developmentCohortHammingDistance(descriptor, other)
		if distance < minimum {
			minimum = distance
		}
	}
	return minimum
}

func developmentCohortHammingDistance(left DevelopmentCohortDescriptor, right DevelopmentCohortDescriptor) int {
	distance := 0
	for index, value := range left.Values {
		if value != right.Values[index] {
			distance++
		}
	}
	return distance
}

func cloneDevelopmentCohortDescriptor(descriptor DevelopmentCohortDescriptor) DevelopmentCohortDescriptor {
	return DevelopmentCohortDescriptor{Values: append([]string(nil), descriptor.Values...)}
}

func appendCloneDevelopmentCohortDescriptor(population []DevelopmentCohortDescriptor, descriptor DevelopmentCohortDescriptor) []DevelopmentCohortDescriptor {
	result := make([]DevelopmentCohortDescriptor, len(population), len(population)+1)
	copy(result, population)
	return append(result, cloneDevelopmentCohortDescriptor(descriptor))
}
