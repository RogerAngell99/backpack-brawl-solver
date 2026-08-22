package benchmark

import (
	"fmt"
	"math/bits"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
	"backpack-brawl-solver/internal/scoring"
)

type v2TopologyMetrics struct {
	components         int
	componentSizes     []int
	articulations      int
	interiorBlocked    int
	corridorCells      int
	relevantBottleneck bool
}

// AnalyzeGeneratedSearchSuiteStructureV2 measures only the final scenario.
// It has no access to generator state, witness state, RNG, score, or solver.
func AnalyzeGeneratedSearchSuiteStructureV2(catalog model.Catalog, generated scenario.Scenario) (GeneratedSearchSuiteRealizedDescriptor, error) {
	if err := generated.Validate(); err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	if len(generated.Priorities) != 2 || generated.PrioritySemantics != model.PrioritySemanticsOutgoingPerInstanceV3 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario requires exactly two outgoing-per-instance-v3 priorities")
	}
	if len(generated.CoverageGroups) != 0 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario must not define coverage groups")
	}
	sourceA, err := v2PrioritySourceID(generated.Priorities[0])
	if err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	sourceB, err := v2PrioritySourceID(generated.Priorities[1])
	if err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	if sourceA == sourceB {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario priorities must name distinct star sources")
	}
	gridMask, err := geometry.ParseGridText(generated.GridText())
	if err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	metrics := analyzeTopologyV2(gridMask)
	realized := GeneratedSearchSuiteRealizedDescriptor{
		UsableCells:          bits.OnesCount64(gridMask),
		ConnectedComponents:  metrics.components,
		ArticulationCells:    metrics.articulations,
		InteriorBlockedCells: metrics.interiorBlocked,
		CorridorCells:        metrics.corridorCells,
	}

	starDefinitions := 0
	for itemID, count := range generated.Items {
		item, exists := catalog.Items[itemID]
		if !exists {
			return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario references unknown item %q", itemID)
		}
		if count <= 0 {
			return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario item %q has non-positive count", itemID)
		}
		realized.InventoryArea += len(item.Shape) * count
		if len(item.Stars) > 0 {
			starDefinitions++
		}
	}
	if starDefinitions != 2 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario must contain exactly two star-bearing definitions, got %d", starDefinitions)
	}
	sourceAItem, exists := catalog.Items[sourceA]
	if !exists || len(sourceAItem.Stars) == 0 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 source A %q is not a star-bearing item", sourceA)
	}
	sourceBItem, exists := catalog.Items[sourceB]
	if !exists || len(sourceBItem.Stars) == 0 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 source B %q is not a star-bearing item", sourceB)
	}
	realized.SourceACopies = generated.Items[sourceA]
	realized.SourceBCopies = generated.Items[sourceB]
	if realized.SourceACopies == 0 || realized.SourceBCopies == 0 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario must include both priority source definitions")
	}

	for _, itemID := range sortedScenarioItemIDsV2(generated.Items) {
		if itemID == sourceA || itemID == sourceB {
			continue
		}
		item := catalog.Items[itemID]
		count := generated.Items[itemID]
		if len(item.Stars) != 0 {
			return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 non-source item %q must not bear stars", itemID)
		}
		matchesA := sourceTargetsItemV2(sourceA, sourceAItem, itemID, item)
		matchesB := sourceTargetsItemV2(sourceB, sourceBItem, itemID, item)
		switch {
		case matchesA && matchesB:
			realized.SharedTargets += count
		case matchesA:
			realized.AOnlyTargets += count
		case matchesB:
			realized.BOnlyTargets += count
		default:
			realized.NeutralFillerInstances += count
			if count >= 2 {
				realized.DuplicateFillerInstances += count
				realized.DuplicateFillerGroups++
			}
			variants, err := geometry.VariantsForItem(item)
			if err != nil {
				return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("neutral filler %q variants: %w", itemID, err)
			}
			switch len(variants) {
			case 1:
				realized.RotationVariants1 += count
			case 2:
				realized.RotationVariants2 += count
			default:
				realized.RotationVariants3Plus += count
			}
		}
	}
	if realized.UsableCells == 0 {
		return GeneratedSearchSuiteRealizedDescriptor{}, fmt.Errorf("v2 scenario has no usable cells")
	}
	realized.DensityBPS = realized.InventoryArea * 10000 / realized.UsableCells
	return realized, nil
}

func ValidateGeneratedSearchSuiteRealizedAgainstRequestedV2(requested GeneratedSearchSuiteStructuralDescriptor, realized GeneratedSearchSuiteRealizedDescriptor) error {
	if err := requested.Validate(); err != nil {
		return err
	}
	metrics := v2TopologyMetrics{
		components:      realized.ConnectedComponents,
		articulations:   realized.ArticulationCells,
		interiorBlocked: realized.InteriorBlockedCells,
		corridorCells:   realized.CorridorCells,
	}
	switch requested.GridTopology {
	case GridTopologyFull:
		if realized.UsableCells != geometry.GridCells || metrics.components != 1 {
			return fmt.Errorf("realized full topology requires 54 usable cells and one component")
		}
	case GridTopologyBottleneck:
		if metrics.components != 1 || metrics.articulations < 1 {
			return fmt.Errorf("realized bottleneck topology requires an articulation")
		}
	case GridTopologyHoles:
		if metrics.components != 1 || metrics.interiorBlocked < 3 {
			return fmt.Errorf("realized holes topology requires one component and at least three interior blocked cells")
		}
	case GridTopologyTwoLobes:
		if metrics.components != 2 {
			return fmt.Errorf("realized two-lobes topology requires exactly two components")
		}
		// Component balance is checked by the scenario-level validator, where
		// the original grid graph and component sizes are available.
	case GridTopologyNarrowCorridors:
		if metrics.components != 1 || metrics.articulations < 4 || metrics.corridorCells*100 < realized.UsableCells*20 {
			return fmt.Errorf("realized narrow-corridors topology requires one component, four articulations, and 20%% corridor cells")
		}
	}
	minDensity, maxDensity, _ := densityBandBPSV2(requested.DensityBand)
	if realized.DensityBPS < minDensity || realized.DensityBPS > maxDensity {
		return fmt.Errorf("realized density %d bps is outside %s [%d,%d]", realized.DensityBPS, requested.DensityBand, minDensity, maxDensity)
	}
	wantA, wantB, _ := sourceMultiplicityCountsV2(requested.SourceMultiplicity)
	if realized.SourceACopies != wantA || realized.SourceBCopies != wantB {
		return fmt.Errorf("realized source multiplicity is %d/%d, want %s", realized.SourceACopies, realized.SourceBCopies, requested.SourceMultiplicity)
	}
	wantAOnly, wantBOnly, wantShared, _ := targetOverlapCountsV2(requested.TargetOverlap)
	if realized.AOnlyTargets != wantAOnly || realized.BOnlyTargets != wantBOnly || realized.SharedTargets != wantShared {
		return fmt.Errorf("realized target overlap is %d/%d/%d, want %d/%d/%d", realized.AOnlyTargets, realized.BOnlyTargets, realized.SharedTargets, wantAOnly, wantBOnly, wantShared)
	}
	if realized.NeutralFillerInstances == 0 {
		return fmt.Errorf("realized v2 scenario requires neutral fillers")
	}
	switch requested.CopySymmetry {
	case CopySymmetryLow:
		if realized.DuplicateFillerInstances != 0 || realized.DuplicateFillerGroups != 0 {
			return fmt.Errorf("realized low copy symmetry contains duplicate fillers")
		}
	case CopySymmetryHigh:
		if realized.DuplicateFillerGroups < 2 || realized.DuplicateFillerInstances*100 < realized.NeutralFillerInstances*60 {
			return fmt.Errorf("realized high copy symmetry does not reach 60%% duplicated fillers in two groups")
		}
	}
	switch requested.RotationEntropy {
	case RotationEntropyLow:
		if realized.RotationVariants1 != realized.NeutralFillerInstances {
			return fmt.Errorf("realized low rotation entropy contains non-single-variant fillers")
		}
	case RotationEntropyMedium:
		if realized.RotationVariants2 != realized.NeutralFillerInstances {
			return fmt.Errorf("realized medium rotation entropy contains fillers outside two variants")
		}
	case RotationEntropyHigh:
		if realized.RotationVariants3Plus != realized.NeutralFillerInstances {
			return fmt.Errorf("realized high rotation entropy contains fillers below three variants")
		}
	}
	return nil
}

// ValidateGeneratedSearchSuiteScenarioAgainstRequestedV2 combines the
// independent final-scenario analyzer with topology checks that need the grid
// graph itself (component balance and relevant articulation splits).
func ValidateGeneratedSearchSuiteScenarioAgainstRequestedV2(catalog model.Catalog, generated scenario.Scenario, requested GeneratedSearchSuiteStructuralDescriptor) (GeneratedSearchSuiteRealizedDescriptor, error) {
	realized, err := AnalyzeGeneratedSearchSuiteStructureV2(catalog, generated)
	if err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	gridMask, err := geometry.ParseGridText(generated.GridText())
	if err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	if err := validateTopologyDescriptorAgainstGridV2(requested, gridMask); err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	if err := ValidateGeneratedSearchSuiteRealizedAgainstRequestedV2(requested, realized); err != nil {
		return GeneratedSearchSuiteRealizedDescriptor{}, err
	}
	return realized, nil
}

func validateTopologyDescriptorAgainstGridV2(requested GeneratedSearchSuiteStructuralDescriptor, gridMask uint64) error {
	metrics := analyzeTopologyV2(gridMask)
	realized := GeneratedSearchSuiteRealizedDescriptor{
		UsableCells:          bits.OnesCount64(gridMask),
		ConnectedComponents:  metrics.components,
		ArticulationCells:    metrics.articulations,
		InteriorBlockedCells: metrics.interiorBlocked,
		CorridorCells:        metrics.corridorCells,
	}
	if requested.GridTopology == GridTopologyTwoLobes {
		if len(metrics.componentSizes) != 2 || metrics.componentSizes[0]*100 < realized.UsableCells*30 || metrics.componentSizes[1]*100 < realized.UsableCells*30 {
			return fmt.Errorf("two-lobes topology components are not balanced")
		}
	}
	return validateTopologyOnlyV2(requested, realized, metrics.relevantBottleneck)
}

func validateTopologyOnlyV2(requested GeneratedSearchSuiteStructuralDescriptor, realized GeneratedSearchSuiteRealizedDescriptor, relevantBottleneck bool) error {
	switch requested.GridTopology {
	case GridTopologyFull:
		if realized.UsableCells != geometry.GridCells || realized.ConnectedComponents != 1 {
			return fmt.Errorf("full topology requires 54 usable cells and one component")
		}
	case GridTopologyBottleneck:
		if realized.ConnectedComponents != 1 || realized.ArticulationCells < 1 || !relevantBottleneck {
			return fmt.Errorf("bottleneck topology requires a relevant articulation")
		}
	case GridTopologyHoles:
		if realized.ConnectedComponents != 1 || realized.InteriorBlockedCells < 3 {
			return fmt.Errorf("holes topology requires one component and interior holes")
		}
	case GridTopologyTwoLobes:
		if realized.ConnectedComponents != 2 {
			return fmt.Errorf("two-lobes topology requires two components")
		}
	case GridTopologyNarrowCorridors:
		if realized.ConnectedComponents != 1 || realized.ArticulationCells < 4 || realized.CorridorCells*100 < realized.UsableCells*20 {
			return fmt.Errorf("narrow-corridors topology does not meet frozen thresholds")
		}
	}
	return nil
}

func analyzeTopologyV2(mask uint64) v2TopologyMetrics {
	metrics := v2TopologyMetrics{}
	visited := uint64(0)
	for cell := 0; cell < geometry.GridCells; cell++ {
		bit := uint64(1) << uint(cell)
		if mask&bit == 0 || visited&bit != 0 {
			continue
		}
		metrics.components++
		queue := []int{cell}
		visited |= bit
		size := 0
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			size++
			for _, neighbor := range v2Neighbors(current, mask) {
				neighborBit := uint64(1) << uint(neighbor)
				if visited&neighborBit == 0 {
					visited |= neighborBit
					queue = append(queue, neighbor)
				}
			}
		}
		metrics.componentSizes = append(metrics.componentSizes, size)
	}
	sort.Ints(metrics.componentSizes)
	for row := 1; row < geometry.GridRows-1; row++ {
		for col := 1; col < geometry.GridCols-1; col++ {
			if mask&(uint64(1)<<uint(row*geometry.GridCols+col)) == 0 {
				metrics.interiorBlocked++
			}
		}
	}
	for cell := 0; cell < geometry.GridCells; cell++ {
		if mask&(uint64(1)<<uint(cell)) == 0 {
			continue
		}
		if len(v2Neighbors(cell, mask)) <= 2 {
			metrics.corridorCells++
		}
	}
	metrics.articulations = len(v2ArticulationCells(mask))
	if metrics.components == 1 {
		for _, cell := range v2ArticulationCells(mask) {
			if v2RelevantArticulation(mask, cell) {
				metrics.relevantBottleneck = true
				break
			}
		}
	}
	return metrics
}

func v2ArticulationCells(mask uint64) []int {
	discovery := [geometry.GridCells]int{}
	low := [geometry.GridCells]int{}
	articulation := [geometry.GridCells]bool{}
	time := 0
	var visit func(int, int)
	visit = func(current int, parent int) {
		time++
		discovery[current] = time
		low[current] = time
		children := 0
		for _, neighbor := range v2Neighbors(current, mask) {
			if discovery[neighbor] == 0 {
				children++
				visit(neighbor, current)
				if low[neighbor] < low[current] {
					low[current] = low[neighbor]
				}
				if parent != -1 && low[neighbor] >= discovery[current] {
					articulation[current] = true
				}
			} else if neighbor != parent && discovery[neighbor] < low[current] {
				low[current] = discovery[neighbor]
			}
		}
		if parent == -1 && children > 1 {
			articulation[current] = true
		}
	}
	for cell := 0; cell < geometry.GridCells; cell++ {
		if mask&(uint64(1)<<uint(cell)) != 0 && discovery[cell] == 0 {
			visit(cell, -1)
		}
	}
	result := make([]int, 0)
	for cell, isArticulation := range articulation {
		if isArticulation {
			result = append(result, cell)
		}
	}
	return result
}

func v2RelevantArticulation(mask uint64, removed int) bool {
	remaining := mask &^ (uint64(1) << uint(removed))
	sizes := v2ComponentSizes(remaining)
	if len(sizes) < 2 {
		return false
	}
	usable := bits.OnesCount64(mask)
	large := 0
	for _, size := range sizes {
		if size*100 >= usable*20 {
			large++
		}
	}
	return large >= 2
}

func v2ComponentSizes(mask uint64) []int {
	metrics := analyzeTopologyV2WithoutArticulation(mask)
	return metrics.componentSizes
}

func analyzeTopologyV2WithoutArticulation(mask uint64) v2TopologyMetrics {
	metrics := v2TopologyMetrics{}
	visited := uint64(0)
	for cell := 0; cell < geometry.GridCells; cell++ {
		bit := uint64(1) << uint(cell)
		if mask&bit == 0 || visited&bit != 0 {
			continue
		}
		metrics.components++
		queue := []int{cell}
		visited |= bit
		size := 0
		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]
			size++
			for _, neighbor := range v2Neighbors(current, mask) {
				neighborBit := uint64(1) << uint(neighbor)
				if visited&neighborBit == 0 {
					visited |= neighborBit
					queue = append(queue, neighbor)
				}
			}
		}
		metrics.componentSizes = append(metrics.componentSizes, size)
	}
	return metrics
}

func v2Neighbors(cell int, mask uint64) []int {
	row, col := cell/geometry.GridCols, cell%geometry.GridCols
	result := make([]int, 0, 4)
	for _, neighbor := range [][2]int{{row - 1, col}, {row + 1, col}, {row, col - 1}, {row, col + 1}} {
		if neighbor[0] < 0 || neighbor[0] >= geometry.GridRows || neighbor[1] < 0 || neighbor[1] >= geometry.GridCols {
			continue
		}
		index := neighbor[0]*geometry.GridCols + neighbor[1]
		if mask&(uint64(1)<<uint(index)) != 0 {
			result = append(result, index)
		}
	}
	return result
}

func v2PrioritySourceID(priority string) (string, error) {
	kind, itemID, ok := strings.Cut(strings.TrimSpace(priority), ":")
	if !ok || kind != "star_source" || itemID == "" {
		return "", fmt.Errorf("v2 priority %q must be star_source:<item_id>", priority)
	}
	return itemID, nil
}

func sourceTargetsItemV2(sourceID string, source model.Item, targetID string, target model.Item) bool {
	for index := range source.Stars {
		if scoring.StarMatchesItem(sourceID, targetID, &target, &source.Stars[index]) {
			return true
		}
	}
	return false
}

func sortedScenarioItemIDsV2(items map[string]int) []string {
	ids := make([]string, 0, len(items))
	for itemID := range items {
		ids = append(ids, itemID)
	}
	sort.Strings(ids)
	return ids
}

func densityBandBPSV2(band string) (int, int, int) {
	switch band {
	case DensityBandD60:
		return 5700, 6300, 6000
	case DensityBandD75:
		return 7200, 7800, 7500
	case DensityBandD90:
		return 8700, 9300, 9000
	case DensityBandD97:
		return 9400, 9850, 9700
	default:
		return 0, 0, 0
	}
}
