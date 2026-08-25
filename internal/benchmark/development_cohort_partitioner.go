package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
)

// DevelopmentCohortPartitionObjective is the frozen partition quality tuple.
// The first two fields are exact rational discrepancies. Hamming-distance
// fields are maximized and MembershipSHA256 is the final deterministic tie.
type DevelopmentCohortPartitionObjective struct {
	MarginalDiscrepancy      string `json:"marginal_discrepancy"`
	PairwiseDiscrepancy      string `json:"pairwise_discrepancy"`
	MinimumWithinWaveHamming int    `json:"minimum_within_wave_hamming"`
	TotalWithinWaveHamming   int    `json:"total_within_wave_hamming"`
	MembershipSHA256         string `json:"membership_sha256"`
}

// DevelopmentCohortPartitionSwap records each strict one-swap improvement.
type DevelopmentCohortPartitionSwap struct {
	Iteration        int                                 `json:"iteration"`
	RemovedFromWaveA int                                 `json:"removed_from_wave_a"`
	AddedToWaveA     int                                 `json:"added_to_wave_a"`
	Objective        DevelopmentCohortPartitionObjective `json:"objective"`
}

// DevelopmentCohortPartition is a complete reproducible structural audit for
// splitting a selected cohort into two independent waves.
type DevelopmentCohortPartition struct {
	Version          int                                 `json:"version"`
	Namespace        string                              `json:"namespace"`
	CandidateOrder   []DevelopmentCohortCandidate        `json:"candidate_order"`
	WaveAIndexes     []int                               `json:"wave_a_indexes"`
	WaveBIndexes     []int                               `json:"wave_b_indexes"`
	InitialObjective DevelopmentCohortPartitionObjective `json:"initial_objective"`
	FinalObjective   DevelopmentCohortPartitionObjective `json:"final_objective"`
	SwapTrace        []DevelopmentCohortPartitionSwap    `json:"swap_trace"`
}

type developmentCohortPartitionObjectiveInternal struct {
	marginalDiscrepancy      *big.Rat
	pairwiseDiscrepancy      *big.Rat
	minimumWithinWaveHamming int
	totalWithinWaveHamming   int
	membershipSHA256         string
}

type developmentCohortPartitionGreedyMetric struct {
	marginalDiscrepancy *big.Rat
	pairwiseDiscrepancy *big.Rat
	minimumHamming      int
	tieBreakSHA256      string
	canonical           string
}

// PartitionDevelopmentCohort creates Wave A with a deterministic greedy
// structural balance pass, assigns the complement to Wave B, then applies the
// lexicographically best strict A/B swap until no improving swap remains.
func PartitionDevelopmentCohort(
	schema DevelopmentCohortSchema,
	cohort []DevelopmentCohortDescriptor,
	waveASize int,
	namespace string,
) (DevelopmentCohortPartition, error) {
	if err := schema.validate(); err != nil {
		return DevelopmentCohortPartition{}, err
	}
	if err := validateDevelopmentCohortNamespace(namespace); err != nil {
		return DevelopmentCohortPartition{}, err
	}
	if waveASize <= 0 || waveASize >= len(cohort) {
		return DevelopmentCohortPartition{}, fmt.Errorf("wave A size must be between 1 and cohort size - 1")
	}
	candidates, _, err := canonicalDevelopmentCohortCandidates(schema, cohort, namespace+"\x00partition-candidates-v1")
	if err != nil {
		return DevelopmentCohortPartition{}, err
	}
	descriptors := make([]DevelopmentCohortDescriptor, len(candidates))
	for index, candidate := range candidates {
		descriptors[index] = candidate.Descriptor
	}

	waveA := make(map[int]struct{}, waveASize)
	for len(waveA) < waveASize {
		bestIndex := -1
		var best developmentCohortPartitionGreedyMetric
		for index, candidate := range candidates {
			if _, selected := waveA[index]; selected {
				continue
			}
			provisional := cloneDevelopmentCohortIndexSet(waveA)
			provisional[index] = struct{}{}
			metric := developmentCohortPartitionGreedyMetricFor(
				schema,
				descriptors,
				provisional,
				waveASize,
				candidate,
				namespace,
			)
			if bestIndex < 0 || developmentCohortPartitionGreedyMetricLess(metric, best) {
				bestIndex = index
				best = metric
			}
		}
		if bestIndex < 0 {
			return DevelopmentCohortPartition{}, fmt.Errorf("partitioner exhausted candidates while filling wave A")
		}
		waveA[bestIndex] = struct{}{}
	}

	initial := developmentCohortPartitionObjectiveFor(schema, descriptors, waveA, waveASize, namespace)
	current := initial
	swaps := make([]DevelopmentCohortPartitionSwap, 0)
	for iteration := 1; ; iteration++ {
		bestRemove, bestAdd := -1, -1
		bestObjective := current
		waveAIndexes, waveBIndexes := developmentCohortPartitionIndexes(len(descriptors), waveA)
		for _, removeIndex := range waveAIndexes {
			for _, addIndex := range waveBIndexes {
				provisional := cloneDevelopmentCohortIndexSet(waveA)
				delete(provisional, removeIndex)
				provisional[addIndex] = struct{}{}
				objective := developmentCohortPartitionObjectiveFor(schema, descriptors, provisional, waveASize, namespace)
				if developmentCohortPartitionObjectiveLess(objective, bestObjective) {
					bestRemove, bestAdd = removeIndex, addIndex
					bestObjective = objective
				}
			}
		}
		if bestRemove < 0 {
			break
		}
		delete(waveA, bestRemove)
		waveA[bestAdd] = struct{}{}
		current = bestObjective
		swaps = append(swaps, DevelopmentCohortPartitionSwap{
			Iteration:        iteration,
			RemovedFromWaveA: bestRemove,
			AddedToWaveA:     bestAdd,
			Objective:        publicDevelopmentCohortPartitionObjective(bestObjective),
		})
	}

	waveAIndexes, waveBIndexes := developmentCohortPartitionIndexes(len(descriptors), waveA)
	return DevelopmentCohortPartition{
		Version:          developmentCohortAlgorithmVersion,
		Namespace:        namespace,
		CandidateOrder:   candidates,
		WaveAIndexes:     waveAIndexes,
		WaveBIndexes:     waveBIndexes,
		InitialObjective: publicDevelopmentCohortPartitionObjective(initial),
		FinalObjective:   publicDevelopmentCohortPartitionObjective(current),
		SwapTrace:        swaps,
	}, nil
}

// DevelopmentCohortPartitionTraceSHA256 hashes the complete partition audit.
func DevelopmentCohortPartitionTraceSHA256(partition DevelopmentCohortPartition) (string, error) {
	encoded, err := json.Marshal(partition)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func developmentCohortPartitionGreedyMetricFor(
	schema DevelopmentCohortSchema,
	descriptors []DevelopmentCohortDescriptor,
	waveA map[int]struct{},
	waveASize int,
	candidate DevelopmentCohortCandidate,
	namespace string,
) developmentCohortPartitionGreedyMetric {
	waveADescriptors := developmentCohortDescriptorsAtIndexes(descriptors, waveA)
	digest := sha256.Sum256([]byte(namespace + "\x00wave-a\x00" + candidate.Canonical))
	return developmentCohortPartitionGreedyMetric{
		marginalDiscrepancy: developmentCohortPartitionMarginalDiscrepancy(schema, descriptors, waveADescriptors, waveASize),
		pairwiseDiscrepancy: developmentCohortPartitionPairwiseDiscrepancy(schema, descriptors, waveADescriptors, waveASize),
		minimumHamming:      developmentCohortMinimumWithinSetHamming(waveADescriptors, len(schema.Dimensions)),
		tieBreakSHA256:      hex.EncodeToString(digest[:]),
		canonical:           candidate.Canonical,
	}
}

func developmentCohortPartitionGreedyMetricLess(left developmentCohortPartitionGreedyMetric, right developmentCohortPartitionGreedyMetric) bool {
	if comparison := left.marginalDiscrepancy.Cmp(right.marginalDiscrepancy); comparison != 0 {
		return comparison < 0
	}
	if comparison := left.pairwiseDiscrepancy.Cmp(right.pairwiseDiscrepancy); comparison != 0 {
		return comparison < 0
	}
	if left.minimumHamming != right.minimumHamming {
		return left.minimumHamming > right.minimumHamming
	}
	if left.tieBreakSHA256 != right.tieBreakSHA256 {
		return left.tieBreakSHA256 < right.tieBreakSHA256
	}
	return left.canonical < right.canonical
}

func developmentCohortPartitionObjectiveFor(
	schema DevelopmentCohortSchema,
	descriptors []DevelopmentCohortDescriptor,
	waveA map[int]struct{},
	waveASize int,
	namespace string,
) developmentCohortPartitionObjectiveInternal {
	waveADescriptors := developmentCohortDescriptorsAtIndexes(descriptors, waveA)
	waveBDescriptors := make([]DevelopmentCohortDescriptor, 0, len(descriptors)-len(waveA))
	canonicalWaveA := make([]string, 0, len(waveA))
	for index, descriptor := range descriptors {
		if _, exists := waveA[index]; exists {
			canonical, _ := CanonicalDevelopmentCohortDescriptor(schema, descriptor)
			canonicalWaveA = append(canonicalWaveA, canonical)
			continue
		}
		waveBDescriptors = append(waveBDescriptors, descriptor)
	}
	sort.Strings(canonicalWaveA)
	digest := sha256.Sum256([]byte(namespace + "\x00partition-v1\x00" + stringsJoinWithNUL(canonicalWaveA)))
	minimumA := developmentCohortMinimumWithinSetHamming(waveADescriptors, len(schema.Dimensions))
	minimumB := developmentCohortMinimumWithinSetHamming(waveBDescriptors, len(schema.Dimensions))
	minimum := minimumA
	if minimumB < minimum {
		minimum = minimumB
	}
	return developmentCohortPartitionObjectiveInternal{
		marginalDiscrepancy:      developmentCohortPartitionMarginalDiscrepancy(schema, descriptors, waveADescriptors, waveASize),
		pairwiseDiscrepancy:      developmentCohortPartitionPairwiseDiscrepancy(schema, descriptors, waveADescriptors, waveASize),
		minimumWithinWaveHamming: minimum,
		totalWithinWaveHamming:   developmentCohortTotalWithinSetHamming(waveADescriptors) + developmentCohortTotalWithinSetHamming(waveBDescriptors),
		membershipSHA256:         hex.EncodeToString(digest[:]),
	}
}

func developmentCohortPartitionObjectiveLess(left developmentCohortPartitionObjectiveInternal, right developmentCohortPartitionObjectiveInternal) bool {
	if comparison := left.marginalDiscrepancy.Cmp(right.marginalDiscrepancy); comparison != 0 {
		return comparison < 0
	}
	if comparison := left.pairwiseDiscrepancy.Cmp(right.pairwiseDiscrepancy); comparison != 0 {
		return comparison < 0
	}
	if left.minimumWithinWaveHamming != right.minimumWithinWaveHamming {
		return left.minimumWithinWaveHamming > right.minimumWithinWaveHamming
	}
	if left.totalWithinWaveHamming != right.totalWithinWaveHamming {
		return left.totalWithinWaveHamming > right.totalWithinWaveHamming
	}
	return left.membershipSHA256 < right.membershipSHA256
}

func developmentCohortPartitionMarginalDiscrepancy(
	schema DevelopmentCohortSchema,
	full []DevelopmentCohortDescriptor,
	waveA []DevelopmentCohortDescriptor,
	waveASize int,
) *big.Rat {
	result := new(big.Rat)
	fullSize := int64(len(full))
	waveASize64 := int64(waveASize)
	for dimensionIndex, dimension := range schema.Dimensions {
		fullCounts := make(map[string]int64, len(dimension.Values))
		waveACounts := make(map[string]int64, len(dimension.Values))
		for _, descriptor := range full {
			fullCounts[descriptor.Values[dimensionIndex]]++
		}
		for _, descriptor := range waveA {
			waveACounts[descriptor.Values[dimensionIndex]]++
		}
		denominator := big.NewInt(int64(len(dimension.Values)))
		for _, value := range dimension.Values {
			delta := big.NewInt(fullSize*waveACounts[value] - waveASize64*fullCounts[value])
			delta.Mul(delta, delta)
			result.Add(result, new(big.Rat).SetFrac(delta, denominator))
		}
	}
	return result
}

func developmentCohortPartitionPairwiseDiscrepancy(
	schema DevelopmentCohortSchema,
	full []DevelopmentCohortDescriptor,
	waveA []DevelopmentCohortDescriptor,
	waveASize int,
) *big.Rat {
	result := new(big.Rat)
	fullSize := int64(len(full))
	waveASize64 := int64(waveASize)
	for left := 0; left < len(schema.Dimensions); left++ {
		for right := left + 1; right < len(schema.Dimensions); right++ {
			fullCounts := developmentCohortPairCounts(full, left, right)
			waveACounts := developmentCohortPairCounts(waveA, left, right)
			denominator := big.NewInt(int64(len(schema.Dimensions[left].Values) * len(schema.Dimensions[right].Values)))
			for _, leftValue := range schema.Dimensions[left].Values {
				for _, rightValue := range schema.Dimensions[right].Values {
					key := developmentCohortPairKey(left, leftValue, right, rightValue)
					delta := big.NewInt(fullSize*waveACounts[key] - waveASize64*fullCounts[key])
					delta.Mul(delta, delta)
					result.Add(result, new(big.Rat).SetFrac(delta, denominator))
				}
			}
		}
	}
	return result
}

func developmentCohortPairCounts(descriptors []DevelopmentCohortDescriptor, left int, right int) map[string]int64 {
	counts := map[string]int64{}
	for _, descriptor := range descriptors {
		counts[developmentCohortPairKey(left, descriptor.Values[left], right, descriptor.Values[right])]++
	}
	return counts
}

func developmentCohortMinimumWithinSetHamming(descriptors []DevelopmentCohortDescriptor, dimensionCount int) int {
	if len(descriptors) < 2 {
		return dimensionCount
	}
	minimum := dimensionCount
	for left := 0; left < len(descriptors); left++ {
		for right := left + 1; right < len(descriptors); right++ {
			distance := developmentCohortHammingDistance(descriptors[left], descriptors[right])
			if distance < minimum {
				minimum = distance
			}
		}
	}
	return minimum
}

func developmentCohortTotalWithinSetHamming(descriptors []DevelopmentCohortDescriptor) int {
	total := 0
	for left := 0; left < len(descriptors); left++ {
		for right := left + 1; right < len(descriptors); right++ {
			total += developmentCohortHammingDistance(descriptors[left], descriptors[right])
		}
	}
	return total
}

func developmentCohortDescriptorsAtIndexes(descriptors []DevelopmentCohortDescriptor, indexes map[int]struct{}) []DevelopmentCohortDescriptor {
	result := make([]DevelopmentCohortDescriptor, 0, len(indexes))
	for index, descriptor := range descriptors {
		if _, exists := indexes[index]; exists {
			result = append(result, descriptor)
		}
	}
	return result
}

func developmentCohortPartitionIndexes(size int, waveA map[int]struct{}) ([]int, []int) {
	waveAIndexes := make([]int, 0, len(waveA))
	waveBIndexes := make([]int, 0, size-len(waveA))
	for index := 0; index < size; index++ {
		if _, exists := waveA[index]; exists {
			waveAIndexes = append(waveAIndexes, index)
		} else {
			waveBIndexes = append(waveBIndexes, index)
		}
	}
	return waveAIndexes, waveBIndexes
}

func cloneDevelopmentCohortIndexSet(source map[int]struct{}) map[int]struct{} {
	result := make(map[int]struct{}, len(source)+1)
	for index := range source {
		result[index] = struct{}{}
	}
	return result
}

func publicDevelopmentCohortPartitionObjective(objective developmentCohortPartitionObjectiveInternal) DevelopmentCohortPartitionObjective {
	return DevelopmentCohortPartitionObjective{
		MarginalDiscrepancy:      objective.marginalDiscrepancy.RatString(),
		PairwiseDiscrepancy:      objective.pairwiseDiscrepancy.RatString(),
		MinimumWithinWaveHamming: objective.minimumWithinWaveHamming,
		TotalWithinWaveHamming:   objective.totalWithinWaveHamming,
		MembershipSHA256:         objective.membershipSHA256,
	}
}

func stringsJoinWithNUL(values []string) string {
	result := ""
	for index, value := range values {
		if index > 0 {
			result += "\x00"
		}
		result += value
	}
	return result
}
