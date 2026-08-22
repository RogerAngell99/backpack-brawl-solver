package benchmark

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

// This cap belongs to corpus generation, never to a solver benchmark. A
// capped proof is reported as exhausted rather than as an unpackable scenario.
const searchSuiteGeneratorV2WitnessMaxNodes = 250_000

type v2WitnessStatus string

const (
	v2WitnessPackable   v2WitnessStatus = "packable"
	v2WitnessUnpackable v2WitnessStatus = "unpackable"
	v2WitnessExhausted  v2WitnessStatus = "exhausted"
)

type v2PackingOption struct {
	rotation int
	origin   model.Coord
	mask     uint64
}

func verifyGeneratedSearchSuiteV2Packability(catalog model.Catalog, generated scenario.Scenario) (v2WitnessStatus, error) {
	return verifyGeneratedSearchSuiteV2PackabilityWithMaxNodes(catalog, generated, searchSuiteGeneratorV2WitnessMaxNodes)
}

func verifyGeneratedSearchSuiteV2PackabilityWithMaxNodes(catalog model.Catalog, generated scenario.Scenario, maxNodes int) (v2WitnessStatus, error) {
	gridMask, err := geometry.ParseGridText(generated.GridText())
	if err != nil {
		return "", err
	}
	optionsByID := make(map[string][]v2PackingOption, len(generated.Items))
	itemIDs := sortedScenarioItemIDsV2(generated.Items)
	for _, itemID := range itemIDs {
		item, exists := catalog.Items[itemID]
		if !exists {
			return "", fmt.Errorf("witness references unknown item %q", itemID)
		}
		options, err := v2PackingOptionsForItem(item, gridMask)
		if err != nil {
			return "", fmt.Errorf("witness options for %q: %w", itemID, err)
		}
		if len(options) == 0 {
			return v2WitnessUnpackable, nil
		}
		optionsByID[itemID] = options
	}
	remaining := make(map[string]int, len(generated.Items))
	totalArea := 0
	for itemID, count := range generated.Items {
		remaining[itemID] = count
		totalArea += len(catalog.Items[itemID].Shape) * count
	}
	if totalArea > popcountV2(gridMask) {
		return v2WitnessUnpackable, nil
	}
	search := v2WitnessSearch{
		itemIDs:      itemIDs,
		optionsByID:  optionsByID,
		failedStates: map[string]struct{}{},
		maxNodes:     maxNodes,
	}
	packable, exhausted := search.place(0, remaining)
	if exhausted {
		return v2WitnessExhausted, nil
	}
	if !packable {
		return v2WitnessUnpackable, nil
	}
	return v2WitnessPackable, nil
}

type v2WitnessSearch struct {
	itemIDs      []string
	optionsByID  map[string][]v2PackingOption
	failedStates map[string]struct{}
	nodes        int
	maxNodes     int
}

func (search *v2WitnessSearch) place(occupied uint64, remaining map[string]int) (packable bool, exhausted bool) {
	search.nodes++
	if search.nodes > search.maxNodes {
		return false, true
	}
	itemID, options := search.selectMRV(occupied, remaining)
	if itemID == "" {
		return true, false
	}
	if len(options) == 0 {
		return false, false
	}
	key := search.stateKey(occupied, remaining)
	if _, exists := search.failedStates[key]; exists {
		return false, false
	}
	remaining[itemID]--
	for _, option := range options {
		packable, exhausted := search.place(occupied|option.mask, remaining)
		if exhausted {
			remaining[itemID]++
			return false, true
		}
		if packable {
			remaining[itemID]++
			return true, false
		}
	}
	remaining[itemID]++
	search.failedStates[key] = struct{}{}
	return false, false
}

func (search *v2WitnessSearch) selectMRV(occupied uint64, remaining map[string]int) (string, []v2PackingOption) {
	selectedID := ""
	var selectedOptions []v2PackingOption
	for _, itemID := range search.itemIDs {
		if remaining[itemID] == 0 {
			continue
		}
		options := make([]v2PackingOption, 0)
		for _, option := range search.optionsByID[itemID] {
			if option.mask&occupied == 0 {
				options = append(options, option)
			}
		}
		if selectedID == "" || len(options) < len(selectedOptions) || (len(options) == len(selectedOptions) && itemID < selectedID) {
			selectedID, selectedOptions = itemID, options
		}
	}
	return selectedID, selectedOptions
}

func (search *v2WitnessSearch) stateKey(occupied uint64, remaining map[string]int) string {
	var builder strings.Builder
	builder.Grow(24 + len(search.itemIDs)*8)
	builder.WriteString(strconv.FormatUint(occupied, 16))
	for _, itemID := range search.itemIDs {
		builder.WriteByte('|')
		builder.WriteString(itemID)
		builder.WriteByte('=')
		builder.WriteString(strconv.Itoa(remaining[itemID]))
	}
	return builder.String()
}

func v2PackingOptionsForItem(item model.Item, gridMask uint64) ([]v2PackingOption, error) {
	variants, err := geometry.VariantsForItem(item)
	if err != nil {
		return nil, err
	}
	options := make([]v2PackingOption, 0)
	for _, variant := range variants {
		for row := 0; row < geometry.GridRows; row++ {
			for col := 0; col < geometry.GridCols; col++ {
				origin := model.Coord{Row: row, Col: col}
				cells := geometry.TranslateCells(variant.Cells, origin)
				inBounds := true
				for _, cell := range cells {
					if !geometry.InBounds(cell) {
						inBounds = false
						break
					}
				}
				if !inBounds {
					continue
				}
				mask := geometry.MaskFromCells(cells)
				if mask&^gridMask != 0 {
					continue
				}
				options = append(options, v2PackingOption{rotation: variant.Rotation, origin: origin, mask: mask})
			}
		}
	}
	sort.Slice(options, func(left, right int) bool {
		if options[left].rotation != options[right].rotation {
			return options[left].rotation < options[right].rotation
		}
		if options[left].origin.Row != options[right].origin.Row {
			return options[left].origin.Row < options[right].origin.Row
		}
		if options[left].origin.Col != options[right].origin.Col {
			return options[left].origin.Col < options[right].origin.Col
		}
		return options[left].mask < options[right].mask
	})
	return options, nil
}

func popcountV2(mask uint64) int {
	count := 0
	for mask != 0 {
		mask &= mask - 1
		count++
	}
	return count
}
