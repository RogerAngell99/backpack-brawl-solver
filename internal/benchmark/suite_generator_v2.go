package benchmark

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"math/bits"
	"math/rand"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

const searchSuiteGeneratorV2Attempts = 64

// materializeGeneratedSearchSuiteCaseV2 is deliberately structural: its
// only score-adjacent operation is canonical catalog compatibility matching.
// It never imports solver code or evaluates a score to select a case.
func materializeGeneratedSearchSuiteCaseV2(catalog model.Catalog, entry GeneratedSearchSuiteCase) (scenario.Scenario, error) {
	generated, _, err := materializeGeneratedSearchSuiteCaseV2WithDiagnostics(catalog, entry)
	return generated, err
}

// v2MaterializationDiagnostics is deliberately ephemeral. It records only
// generator-control-flow reasons so tests can prove that witness exhaustion
// did not filter accepted corpus cases; it is never locked or benchmarked.
type v2MaterializationDiagnostics struct {
	AcceptedAttempt int
	Rejections      map[string]int
}

func materializeGeneratedSearchSuiteCaseV2WithDiagnostics(catalog model.Catalog, entry GeneratedSearchSuiteCase) (scenario.Scenario, v2MaterializationDiagnostics, error) {
	diagnostics := v2MaterializationDiagnostics{AcceptedAttempt: -1, Rejections: map[string]int{}}
	if err := validateGeneratedSearchSuiteCaseV2(entry); err != nil {
		return scenario.Scenario{}, diagnostics, err
	}
	if entry.Role == SuiteRolePrivateHoldout || entry.Seed == nil {
		return scenario.Scenario{}, diagnostics, fmt.Errorf("v2 generated case %q has no public seed", entry.ID)
	}
	descriptor := *entry.StructuralDescriptor
	pairs, err := viableV2SourcePairs(catalog, descriptor)
	if err != nil {
		return scenario.Scenario{}, diagnostics, err
	}
	if len(pairs) == 0 {
		return scenario.Scenario{}, diagnostics, fmt.Errorf("cannot materialize case %q: no eligible source pair for requested target overlap %s", entry.ID, descriptor.TargetOverlap)
	}
	for attempt := 0; attempt < searchSuiteGeneratorV2Attempts; attempt++ {
		generated, rejection, err := materializeGeneratedSearchSuiteCaseV2Attempt(catalog, entry.ID, *entry.Seed, descriptor, append([]v2SourcePair(nil), pairs...), attempt)
		if err != nil {
			return scenario.Scenario{}, diagnostics, err
		}
		if rejection != "" {
			diagnostics.Rejections[rejection]++
			continue
		}
		diagnostics.AcceptedAttempt = attempt
		return generated, diagnostics, nil
	}
	return scenario.Scenario{}, diagnostics, fmt.Errorf("cannot materialize case %q\n\ndescriptor:\n  topology=%s\n  density=%s\n  multiplicity=%s\n  overlap=%s\n  symmetry=%s\n  rotation=%s\n\nattempts: %d\n\nrejections:\n  insufficient_targets: %d\n  filler_area_unsatisfied: %d\n  witness_unpacked: %d",
		entry.ID,
		descriptor.GridTopology, descriptor.DensityBand, descriptor.SourceMultiplicity, descriptor.TargetOverlap, descriptor.CopySymmetry, descriptor.RotationEntropy,
		searchSuiteGeneratorV2Attempts,
		diagnostics.Rejections["insufficient_targets"], diagnostics.Rejections["filler_area_unsatisfied"], diagnostics.Rejections["witness_unpacked"],
	)
}

func materializeGeneratedSearchSuiteCaseV2Attempt(catalog model.Catalog, caseID string, seed int64, descriptor GeneratedSearchSuiteStructuralDescriptor, pairs []v2SourcePair, attempt int) (scenario.Scenario, string, error) {
	return materializeGeneratedSearchSuiteCaseV2AttemptWithWitnessMaxNodes(catalog, caseID, seed, descriptor, pairs, attempt, searchSuiteGeneratorV2WitnessMaxNodes)
}

func materializeGeneratedSearchSuiteCaseV2AttemptWithWitnessMaxNodes(catalog model.Catalog, caseID string, seed int64, descriptor GeneratedSearchSuiteStructuralDescriptor, pairs []v2SourcePair, attempt int, witnessMaxNodes int) (scenario.Scenario, string, error) {
	topologyRandom := v2Random(seed, "topology", attempt)
	grid, err := chooseTopologyGridV2(descriptor.GridTopology, topologyRandom)
	if err != nil {
		return scenario.Scenario{}, "", err
	}
	gridMask, err := geometry.ParseGridText(strings.Join(grid, "\n"))
	if err != nil {
		return scenario.Scenario{}, "", fmt.Errorf("v2 topology %q: %w", descriptor.GridTopology, err)
	}
	if err := validateTopologyDescriptorAgainstGridV2(descriptor, gridMask); err != nil {
		return scenario.Scenario{}, "", fmt.Errorf("v2 topology %q is invalid: %w", descriptor.GridTopology, err)
	}

	sourceRandom := v2Random(seed, "source-pair", attempt)
	pair := chooseV2SourcePair(pairs, sourceRandom)

	targetRandom := v2Random(seed, "targets", attempt)
	selectedTargets, err := chooseV2Targets(catalog, pair, descriptor, targetRandom)
	if err != nil {
		if err == errV2InsufficientTargets {
			return scenario.Scenario{}, "insufficient_targets", nil
		}
		return scenario.Scenario{}, "", err
	}
	sourceACopies, sourceBCopies, _ := sourceMultiplicityCountsV2(descriptor.SourceMultiplicity)
	items := map[string]int{pair.SourceA: sourceACopies, pair.SourceB: sourceBCopies}
	for _, itemID := range selectedTargets {
		items[itemID]++
	}
	mandatoryArea, err := v2InventoryArea(catalog, items)
	if err != nil {
		return scenario.Scenario{}, "", err
	}
	fillerRandom := v2Random(seed, "fillers", attempt)
	fillerItems, ok, err := chooseV2Fillers(catalog, pair, descriptor, mandatoryArea, bits.OnesCount64(gridMask), fillerRandom)
	if err != nil {
		return scenario.Scenario{}, "", err
	}
	if !ok {
		return scenario.Scenario{}, "filler_area_unsatisfied", nil
	}
	for itemID, count := range fillerItems {
		items[itemID] += count
	}
	top, workers := 1, 1
	noSkips, repairSearch := true, true
	generated := scenario.Scenario{
		Name:              caseID,
		Grid:              grid,
		Items:             items,
		Top:               &top,
		Workers:           &workers,
		NoSkips:           &noSkips,
		RepairSearch:      &repairSearch,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:" + pair.SourceA, "star_source:" + pair.SourceB},
	}
	if err := generated.Validate(); err != nil {
		return scenario.Scenario{}, "", err
	}
	if err := validateV2GeneratedCandidate(catalog, generated, descriptor); err != nil {
		return scenario.Scenario{}, "", err
	}
	witnessStatus, err := verifyGeneratedSearchSuiteV2PackabilityWithMaxNodes(catalog, generated, witnessMaxNodes)
	if err != nil {
		return scenario.Scenario{}, "", err
	}
	switch witnessStatus {
	case v2WitnessPackable:
		return generated, "", nil
	case v2WitnessUnpackable:
		return scenario.Scenario{}, "witness_unpacked", nil
	case v2WitnessExhausted:
		return scenario.Scenario{}, "", fmt.Errorf("v2 packing witness exhausted its fixed %d-node budget", witnessMaxNodes)
	default:
		return scenario.Scenario{}, "", fmt.Errorf("unknown v2 witness status %q", witnessStatus)
	}
}

func validateV2GeneratedCandidate(catalog model.Catalog, generated scenario.Scenario, descriptor GeneratedSearchSuiteStructuralDescriptor) error {
	if _, err := ValidateGeneratedSearchSuiteScenarioAgainstRequestedV2(catalog, generated, descriptor); err != nil {
		return fmt.Errorf("v2 generated candidate violates structural invariant: %w", err)
	}
	return nil
}

func chooseV2SourcePair(pairs []v2SourcePair, random *rand.Rand) v2SourcePair {
	random.Shuffle(len(pairs), func(left, right int) { pairs[left], pairs[right] = pairs[right], pairs[left] })
	return pairs[0]
}

func searchSuiteV2Seed(base int64, label string, attempt int) int64 {
	payload := fmt.Sprintf("%s\x00%d\x00%s\x00%d", SearchSuiteGeneratorV2, base, label, attempt)
	digest := sha256.Sum256([]byte(payload))
	return int64(binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64)
}

// SearchSuiteV2PublicSeed derives a published v2 seed solely from its neutral
// case ID. This makes seed selection auditable and independent of generated
// scenario outcomes.
func SearchSuiteV2PublicSeed(caseID string) int64 {
	digest := sha256.Sum256([]byte("general-search-v2/public/" + caseID))
	return int64(binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64)
}

func v2Random(base int64, label string, attempt int) *rand.Rand {
	return rand.New(rand.NewSource(searchSuiteV2Seed(base, label, attempt)))
}

type v2CompatibilityProfile struct {
	ItemID  string
	Matches []uint64
}

type v2SourcePair struct {
	SourceA string
	SourceB string
	AOnly   []uint64
	BOnly   []uint64
	Shared  []uint64
	Neutral []uint64
	Targets []string
}

func viableV2SourcePairs(catalog model.Catalog, descriptor GeneratedSearchSuiteStructuralDescriptor) ([]v2SourcePair, error) {
	targets := v2CandidateNonStarItemIDs(catalog)
	if len(targets) == 0 {
		return nil, nil
	}
	profiles := make([]v2CompatibilityProfile, 0)
	for _, itemID := range sortedCatalogItemIDsV2(catalog) {
		item := catalog.Items[itemID]
		if len(item.Shape) == 0 || len(item.Shape) > 4 || len(item.Stars) == 0 {
			continue
		}
		matches := make([]uint64, (len(targets)+63)/64)
		for index, targetID := range targets {
			if sourceTargetsItemV2(itemID, item, targetID, catalog.Items[targetID]) {
				matches[index/64] |= uint64(1) << uint(index%64)
			}
		}
		if anyV2Bits(matches) {
			profiles = append(profiles, v2CompatibilityProfile{ItemID: itemID, Matches: matches})
		}
	}
	wantAOnly, wantBOnly, wantShared, _ := targetOverlapCountsV2(descriptor.TargetOverlap)
	pairs := make([]v2SourcePair, 0)
	fullTargetMask := make([]uint64, (len(targets)+63)/64)
	for index := range targets {
		fullTargetMask[index/64] |= uint64(1) << uint(index%64)
	}
	for left := 0; left < len(profiles); left++ {
		for right := 0; right < len(profiles); right++ {
			if left == right || !sourcePairIsIsolatedV2(catalog, profiles[left].ItemID, profiles[right].ItemID) {
				continue
			}
			aOnly, bOnly, shared, neutral := make([]uint64, len(fullTargetMask)), make([]uint64, len(fullTargetMask)), make([]uint64, len(fullTargetMask)), make([]uint64, len(fullTargetMask))
			for word := range fullTargetMask {
				aOnly[word] = profiles[left].Matches[word] &^ profiles[right].Matches[word]
				bOnly[word] = profiles[right].Matches[word] &^ profiles[left].Matches[word]
				shared[word] = profiles[left].Matches[word] & profiles[right].Matches[word]
				neutral[word] = fullTargetMask[word] &^ (profiles[left].Matches[word] | profiles[right].Matches[word])
			}
			if countV2Bits(aOnly) < wantAOnly || countV2Bits(bOnly) < wantBOnly || countV2Bits(shared) < wantShared || !anyV2Bits(neutral) {
				continue
			}
			pairs = append(pairs, v2SourcePair{
				SourceA: profiles[left].ItemID, SourceB: profiles[right].ItemID,
				AOnly: aOnly, BOnly: bOnly, Shared: shared, Neutral: neutral, Targets: targets,
			})
		}
	}
	sort.Slice(pairs, func(left, right int) bool {
		if pairs[left].SourceA != pairs[right].SourceA {
			return pairs[left].SourceA < pairs[right].SourceA
		}
		return pairs[left].SourceB < pairs[right].SourceB
	})
	return pairs, nil
}

// sourcePairIsIsolatedV2 keeps source multiplicity from introducing hidden
// source-to-source priority opportunities. It is intentionally evaluated with
// the canonical StarMatchesItem semantics, including aliases and exclusions.
func sourcePairIsIsolatedV2(catalog model.Catalog, sourceA string, sourceB string) bool {
	a, aExists := catalog.Items[sourceA]
	b, bExists := catalog.Items[sourceB]
	if !aExists || !bExists {
		return false
	}
	return !sourceTargetsItemV2(sourceA, a, sourceA, a) &&
		!sourceTargetsItemV2(sourceA, a, sourceB, b) &&
		!sourceTargetsItemV2(sourceB, b, sourceA, a) &&
		!sourceTargetsItemV2(sourceB, b, sourceB, b)
}

var errV2InsufficientTargets = fmt.Errorf("insufficient v2 targets")

func chooseV2Targets(catalog model.Catalog, pair v2SourcePair, descriptor GeneratedSearchSuiteStructuralDescriptor, random *rand.Rand) ([]string, error) {
	wantAOnly, wantBOnly, wantShared, _ := targetOverlapCountsV2(descriptor.TargetOverlap)
	aOnly := shuffleV2IDs(v2IDsFromBits(pair.Targets, pair.AOnly), random)
	bOnly := shuffleV2IDs(v2IDsFromBits(pair.Targets, pair.BOnly), random)
	shared := shuffleV2IDs(v2IDsFromBits(pair.Targets, pair.Shared), random)
	if len(aOnly) < wantAOnly || len(bOnly) < wantBOnly || len(shared) < wantShared {
		return nil, errV2InsufficientTargets
	}
	selected := make([]string, 0, 6)
	selected = append(selected, aOnly[:wantAOnly]...)
	selected = append(selected, bOnly[:wantBOnly]...)
	selected = append(selected, shared[:wantShared]...)
	return selected, nil
}

type v2FillerCandidate struct {
	ItemID string
	Area   int
}

func chooseV2Fillers(catalog model.Catalog, pair v2SourcePair, descriptor GeneratedSearchSuiteStructuralDescriptor, mandatoryArea int, usableCells int, random *rand.Rand) (map[string]int, bool, error) {
	if mandatoryArea > usableCells {
		return nil, false, nil
	}
	candidates, err := v2FillerCandidates(catalog, pair, descriptor.RotationEntropy, random)
	if err != nil {
		return nil, false, err
	}
	if len(candidates) == 0 {
		return nil, false, nil
	}
	for _, targetArea := range densityTargetAreasV2(descriptor.DensityBand, usableCells, mandatoryArea) {
		required := targetArea - mandatoryArea
		var fillers map[string]int
		var ok bool
		switch descriptor.CopySymmetry {
		case CopySymmetryLow:
			fillers, ok = chooseLowSymmetryFillersV2(candidates, required)
		case CopySymmetryHigh:
			fillers, ok = chooseHighSymmetryFillersV2(candidates, required)
		}
		if ok {
			return fillers, true, nil
		}
	}
	return nil, false, nil
}

func v2FillerCandidates(catalog model.Catalog, pair v2SourcePair, entropy string, random *rand.Rand) ([]v2FillerCandidate, error) {
	ids := v2IDsFromBits(pair.Targets, pair.Neutral)
	result := make([]v2FillerCandidate, 0, len(ids))
	for _, itemID := range ids {
		item := catalog.Items[itemID]
		variants, err := geometry.VariantsForItem(item)
		if err != nil {
			return nil, fmt.Errorf("filler %q variants: %w", itemID, err)
		}
		matchesEntropy := (entropy == RotationEntropyLow && len(variants) == 1) ||
			(entropy == RotationEntropyMedium && len(variants) == 2) ||
			(entropy == RotationEntropyHigh && len(variants) >= 3)
		if matchesEntropy {
			result = append(result, v2FillerCandidate{ItemID: itemID, Area: len(item.Shape)})
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ItemID < result[right].ItemID })
	random.Shuffle(len(result), func(left, right int) { result[left], result[right] = result[right], result[left] })
	return result, nil
}

func densityTargetAreasV2(band string, usableCells int, mandatoryArea int) []int {
	minimumBPS, maximumBPS, preferredBPS := densityBandBPSV2(band)
	areas := make([]int, 0)
	for area := mandatoryArea; area <= usableCells; area++ {
		bps := area * 10000 / usableCells
		if bps >= minimumBPS && bps <= maximumBPS {
			areas = append(areas, area)
		}
	}
	sort.Slice(areas, func(left, right int) bool {
		leftDistance := absV2(areas[left]*10000/usableCells - preferredBPS)
		rightDistance := absV2(areas[right]*10000/usableCells - preferredBPS)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		return areas[left] < areas[right]
	})
	return areas
}

func chooseLowSymmetryFillersV2(candidates []v2FillerCandidate, required int) (map[string]int, bool) {
	if required <= 0 {
		return nil, false
	}
	solutions := make([]map[string]int, required+1)
	solutions[0] = map[string]int{}
	for _, candidate := range candidates {
		for area := required - candidate.Area; area >= 0; area-- {
			if solutions[area] == nil || solutions[area+candidate.Area] != nil {
				continue
			}
			solution := cloneV2Counts(solutions[area])
			solution[candidate.ItemID] = 1
			solutions[area+candidate.Area] = solution
		}
	}
	if solutions[required] == nil {
		return nil, false
	}
	return solutions[required], true
}

type v2HighFillerState struct {
	index      int
	area       int
	instances  int
	duplicates int
	groups     int
}

func chooseHighSymmetryFillersV2(candidates []v2FillerCandidate, required int) (map[string]int, bool) {
	if required <= 0 {
		return nil, false
	}
	limited := diverseV2HighFillerCandidates(candidates, 36)
	failed := map[v2HighFillerState]struct{}{}
	counts := map[string]int{}
	var search func(v2HighFillerState) bool
	search = func(state v2HighFillerState) bool {
		if state.area > required {
			return false
		}
		if state.index == len(limited) {
			return state.area == required && state.groups >= 2 && state.instances > 0 && state.duplicates*100 >= state.instances*60
		}
		if _, exists := failed[state]; exists {
			return false
		}
		candidate := limited[state.index]
		for count := 3; count >= 0; count-- {
			next := state
			next.index++
			next.area += count * candidate.Area
			next.instances += count
			if count >= 2 {
				next.duplicates += count
				if next.groups < 2 {
					next.groups++
				}
			}
			if count > 0 {
				counts[candidate.ItemID] = count
			}
			if search(next) {
				return true
			}
			delete(counts, candidate.ItemID)
		}
		failed[state] = struct{}{}
		return false
	}
	if !search(v2HighFillerState{}) {
		return nil, false
	}
	return cloneV2Counts(counts), true
}

func diverseV2HighFillerCandidates(candidates []v2FillerCandidate, limit int) []v2FillerCandidate {
	byArea := [5][]v2FillerCandidate{}
	for _, candidate := range candidates {
		if candidate.Area >= 1 && candidate.Area <= 4 {
			byArea[candidate.Area] = append(byArea[candidate.Area], candidate)
		}
	}
	result := make([]v2FillerCandidate, 0, limit)
	for index := 0; len(result) < limit; index++ {
		added := false
		for area := 1; area <= 4 && len(result) < limit; area++ {
			if index >= len(byArea[area]) {
				continue
			}
			result = append(result, byArea[area][index])
			added = true
		}
		if !added {
			break
		}
	}
	return result
}

func shuffleV2IDs(ids []string, random *rand.Rand) []string {
	result := append([]string(nil), ids...)
	sort.Strings(result)
	random.Shuffle(len(result), func(left, right int) { result[left], result[right] = result[right], result[left] })
	return result
}

func v2CandidateNonStarItemIDs(catalog model.Catalog) []string {
	ids := make([]string, 0)
	for _, itemID := range sortedCatalogItemIDsV2(catalog) {
		item := catalog.Items[itemID]
		if len(item.Shape) >= 1 && len(item.Shape) <= 4 && len(item.Stars) == 0 {
			ids = append(ids, itemID)
		}
	}
	return ids
}

func sortedCatalogItemIDsV2(catalog model.Catalog) []string {
	ids := make([]string, 0, len(catalog.Items))
	for itemID := range catalog.Items {
		ids = append(ids, itemID)
	}
	sort.Strings(ids)
	return ids
}

func v2IDsFromBits(ids []string, words []uint64) []string {
	result := make([]string, 0, countV2Bits(words))
	for index, itemID := range ids {
		if words[index/64]&(uint64(1)<<uint(index%64)) != 0 {
			result = append(result, itemID)
		}
	}
	return result
}

func anyV2Bits(words []uint64) bool {
	for _, word := range words {
		if word != 0 {
			return true
		}
	}
	return false
}

func countV2Bits(words []uint64) int {
	total := 0
	for _, word := range words {
		total += bits.OnesCount64(word)
	}
	return total
}

func v2InventoryArea(catalog model.Catalog, items map[string]int) (int, error) {
	total := 0
	for itemID, count := range items {
		item, exists := catalog.Items[itemID]
		if !exists {
			return 0, fmt.Errorf("unknown v2 item %q", itemID)
		}
		total += len(item.Shape) * count
	}
	return total, nil
}

func cloneV2Counts(source map[string]int) map[string]int {
	result := make(map[string]int, len(source))
	for itemID, count := range source {
		result[itemID] = count
	}
	return result
}

func absV2(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
