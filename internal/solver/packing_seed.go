package solver

import (
	"math/bits"
	"sort"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

// packingSeedSearch is deliberately objective-agnostic until its final tie
// break. It produces complete layouts for later objective search and repair;
// a high partial star score cannot keep a state that has made another item
// impossible to place.
type packingSeedState struct {
	occupied      uint64
	placed        []model.Placement
	restricted    int
	flexibility   int
	fragmentation int
	score         model.Score
	key           string
}

func packingSeedSearch(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
	gridMask uint64,
	nodeBudget int64,
	progress *progressTracker,
) coverageSeedResult {
	if nodeBudget <= 0 || len(ordered) == 0 {
		return coverageSeedResult{}
	}

	beamWidth := policyForConfig(config).PackingSeedBeamWidth
	states := []packingSeedState{{
		restricted:    len(ordered),
		flexibility:   len(ordered),
		fragmentation: freeSpaceFragmentation(gridMask, 0),
	}}
	remainingCells := remainingCellCounts(catalog, ordered)
	var nodes int64
	var symmetryPruned int64
	var deduplicated int64
	var hardPruned int64
	var beamEvictions int64
	var packingDiagnostics model.PackingSeedDiagnostics
	var statesByItemCount [25]int64
	var progressBatch int64
	exhausted := false
	reportNode := func() bool {
		if !chargeNode(config, config.tracePhase) {
			exhausted = true
			return false
		}
		nodes++
		if progress == nil {
			return true
		}
		progressBatch++
		if progressBatch >= progressNodeInterval {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
			progressBatch = 0
		}
		return true
	}
	flushProgress := func() {
		if progress != nil && progressBatch > 0 {
			progress.addNodes(ProgressPhaseSeed, progressBatch, false)
		}
	}

	for index, instance := range ordered {
		if exhausted || len(states) == 0 || (config.Context != nil && config.Context.Err() != nil) {
			break
		}
		nextByClass := make(map[string]packingSeedState, beamWidth*4)
		for _, state := range states {
			if exhausted || nodes >= nodeBudget {
				exhausted = true
				break
			}
			if remainingCells[index] > bits.OnesCount64(gridMask&^state.occupied) {
				hardPruned++
				continue
			}
			for _, option := range optionsByInstance[instance.InstanceID] {
				if option.Mask&state.occupied != 0 {
					continue
				}
				if !placementRespectsCanonicalCopyOrder(option, state.placed) {
					symmetryPruned++
					continue
				}
				if !reportNode() {
					break
				}
				nextPlaced, _ := insertPlacementSorted(append([]model.Placement(nil), state.placed...), option)
				nextOccupied := state.occupied | option.Mask
				restricted, flexibility, feasible := packingFeasibility(ordered[index+1:], optionsByInstance, nextOccupied, nextPlaced)
				if !feasible {
					hardPruned++
					if nodes >= nodeBudget {
						exhausted = true
					}
					continue
				}
				candidate := packingSeedState{
					occupied:      nextOccupied,
					placed:        nextPlaced,
					restricted:    restricted,
					flexibility:   flexibility,
					fragmentation: freeSpaceFragmentation(gridMask, nextOccupied),
					score:         evaluateScoreForConfig(catalog, nextPlaced, config),
					key:           coverageSeedAppendKey(state.key, option),
				}
				classKey := packingStateClassKey(candidate)
				if previous, exists := nextByClass[classKey]; exists {
					deduplicated++
					if !packingSeedStateLess(candidate, previous) {
						continue
					}
				}
				nextByClass[classKey] = candidate
				if nodes >= nodeBudget {
					exhausted = true
					break
				}
			}
		}

		next := make([]packingSeedState, 0, len(nextByClass))
		for _, state := range nextByClass {
			next = append(next, state)
		}
		sort.Slice(next, func(i, j int) bool { return packingSeedStateLess(next[i], next[j]) })
		if len(next) > beamWidth {
			beamEvictions += int64(len(next) - beamWidth)
			clear(next[beamWidth:])
			next = next[:beamWidth]
		}
		states = next
		packingDiagnostics.LayerWidths = append(packingDiagnostics.LayerWidths, model.PackingSeedLayerWidth{
			Depth:  index + 1,
			States: len(states),
		})
		for _, state := range states {
			depth := len(state.placed)
			if depth > packingDiagnostics.MaxDepth {
				packingDiagnostics.MaxDepth = depth
			}
			if depth >= 21 && depth <= 24 {
				statesByItemCount[depth]++
			}
		}
	}

	results := make([]model.Solution, 0, config.TopN)
	candidateCount := 0
	for _, state := range states {
		if len(state.placed) != len(instances) {
			continue
		}
		candidateCount++
		results = insertCandidateWithScoreOnlyFilter(catalog, results, state.placed, instances, config)
	}
	flushProgress()
	return coverageSeedResult{
		Solutions:              results,
		NodesExplored:          nodes,
		CandidateCount:         candidateCount,
		SymmetryPrunedBranches: symmetryPruned,
		StatesDeduplicated:     deduplicated,
		HardPrunedNodes:        hardPruned,
		PackingDiagnostics: model.PackingSeedDiagnostics{
			MaxDepth: packingDiagnostics.MaxDepth,
			StatesByItemCount: []model.PackingSeedItemCount{
				{Items: 21, States: statesByItemCount[21]},
				{Items: 22, States: statesByItemCount[22]},
				{Items: 23, States: statesByItemCount[23]},
				{Items: 24, States: statesByItemCount[24]},
			},
			LayerWidths:        packingDiagnostics.LayerWidths,
			HardDeadPruned:     hardPruned,
			SymmetryPruned:     symmetryPruned,
			StatesDeduplicated: deduplicated,
			BeamEvictions:      beamEvictions,
		},
	}
}

// Packing needs depth before breadth. A generic star beam can spend its whole
// budget expanding a few early layers; this narrower beam reaches complete
// no-skip layouts and hands them to objective search and LNS.
func packingSeedBeamWidth(nodeBudget int64) int {
	switch {
	case nodeBudget <= 50_000:
		return 32
	case nodeBudget <= 250_000:
		return 64
	case nodeBudget <= 1_000_000:
		return 128
	default:
		return 256
	}
}

func packingFeasibility(
	remaining []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	occupied uint64,
	placements []model.Placement,
) (restricted int, flexibility int, feasible bool) {
	if len(remaining) == 0 {
		return 0, 0, true
	}
	restricted = int(^uint(0) >> 1)
	for _, instance := range remaining {
		legal := 0
		for _, option := range optionsByInstance[instance.InstanceID] {
			if option.Mask&occupied == 0 && placementRespectsCanonicalCopyOrder(option, placements) {
				legal++
			}
		}
		if legal == 0 {
			return 0, 0, false
		}
		if legal < restricted {
			restricted = legal
		}
		flexibility += legal
	}
	return restricted, flexibility, true
}

func packingSeedStateLess(left, right packingSeedState) bool {
	return packingSeedStateFirstDecisive(left, right, true).compare > 0
}

type packingSeedStateDecision struct {
	compare       int
	component     string
	priorityIndex int
	hasPriority   bool
	advantage     int
}

type packingSeedComparatorComponent struct {
	name           string
	priorityIndex  int
	hasPriority    bool
	higherIsBetter bool
	key            bool
}

const (
	constellationRootPackingRankingBaseline           = "baseline"
	constellationRootPackingRankingPriorityScoreFirst = "priority-score-first"
)

func packingSeedStatePrimaryCompare(left, right packingSeedState) int {
	return packingSeedStateFirstDecisive(left, right, false).compare
}

func packingSeedStateFirstDecisive(left, right packingSeedState, includeKey bool) packingSeedStateDecision {
	maxPriorities := len(left.score.PriorityCounts)
	if len(right.score.PriorityCounts) > maxPriorities {
		maxPriorities = len(right.score.PriorityCounts)
	}
	for _, component := range packingSeedStateComparatorComponents(maxPriorities, includeKey) {
		if compare := packingSeedStateComponentCompare(left, right, component); compare != 0 {
			decision := packingSeedStateDecision{
				compare:       compare,
				component:     component.name,
				hasPriority:   component.hasPriority,
				priorityIndex: component.priorityIndex,
			}
			if !component.key {
				decision.advantage = compare
				if decision.advantage < 0 {
					decision.advantage = -decision.advantage
				}
			}
			return decision
		}
	}
	return packingSeedStateDecision{component: "exact_equal"}
}

func packingSeedStateComparatorComponents(maxPriorities int, includeKey bool) []packingSeedComparatorComponent {
	components := []packingSeedComparatorComponent{
		{name: "restricted", higherIsBetter: true},
		{name: "flexibility", higherIsBetter: true},
		{name: "fragmentation", higherIsBetter: false},
	}
	for index := 0; index < maxPriorities; index++ {
		components = append(components, packingSeedComparatorComponent{name: "priority_count", priorityIndex: index, hasPriority: true, higherIsBetter: true})
	}
	components = append(components,
		packingSeedComparatorComponent{name: "craft_count", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_count", higherIsBetter: true},
		packingSeedComparatorComponent{name: "item_count", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_target_breadth", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_reciprocal_pairs", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_source_definition_diversity", higherIsBetter: true},
	)
	if includeKey {
		components = append(components, packingSeedComparatorComponent{name: "key_lexical", key: true})
	}
	return components
}

func packingSeedPriorityScoreFirstComparatorComponents(maxPriorities int, includeKey bool) []packingSeedComparatorComponent {
	components := packingSeedPriorityComponents(maxPriorities)
	components = append(components, packingSeedScoreTailComponents(true)...)
	components = append(components,
		packingSeedComparatorComponent{name: "restricted", higherIsBetter: true},
		packingSeedComparatorComponent{name: "flexibility", higherIsBetter: true},
		packingSeedComparatorComponent{name: "fragmentation", higherIsBetter: false},
	)
	if includeKey {
		components = append(components, packingSeedComparatorComponent{name: "key_lexical", key: true})
	}
	return components
}

func packingSeedStateComponentCompare(left, right packingSeedState, component packingSeedComparatorComponent) int {
	if component.key {
		if left.key < right.key {
			return 1
		}
		if left.key > right.key {
			return -1
		}
		return 0
	}
	leftValue := packingSeedStateComponentValue(left, component)
	rightValue := packingSeedStateComponentValue(right, component)
	compare := leftValue - rightValue
	if !component.higherIsBetter {
		compare = rightValue - leftValue
	}
	return compare
}

func packingSeedStateComponentValue(state packingSeedState, component packingSeedComparatorComponent) int {
	switch component.name {
	case "restricted":
		return state.restricted
	case "flexibility":
		return state.flexibility
	case "fragmentation":
		return state.fragmentation
	case "priority_count":
		if component.priorityIndex < len(state.score.PriorityCounts) {
			return state.score.PriorityCounts[component.priorityIndex]
		}
		return 0
	case "craft_count":
		return state.score.CraftCount
	case "star_count":
		return state.score.StarCount
	case "item_count":
		return state.score.ItemCount
	case "star_target_breadth":
		return state.score.StarTargetBreadth
	case "star_reciprocal_pairs":
		return state.score.StarReciprocalPairs
	case "star_source_definition_diversity":
		return state.score.StarSourceDefinitionDiversity
	default:
		return 0
	}
}

type packingCounterfactualVariant struct {
	id         string
	tuple      string
	components []packingSeedComparatorComponent
}

func packingCounterfactualVariants(maxPriorities int) []packingCounterfactualVariant {
	priority := packingSeedPriorityComponents(maxPriorities)
	structural := []packingSeedComparatorComponent{
		{name: "restricted", higherIsBetter: true},
		{name: "flexibility", higherIsBetter: true},
		{name: "fragmentation", higherIsBetter: false},
	}
	scoreTail := packingSeedScoreTailComponents(true)
	scoreTailWithoutStars := packingSeedScoreTailComponents(false)
	key := packingSeedComparatorComponent{name: "key_lexical", key: true}
	baseline := packingSeedStateComparatorComponents(maxPriorities, true)
	variantB := append(append(append([]packingSeedComparatorComponent{}, priority...), packingSeedComparatorComponent{name: "star_count", higherIsBetter: true}), structural...)
	variantB = append(variantB, scoreTailWithoutStars...)
	variantB = append(variantB, key)
	variantC := []packingSeedComparatorComponent{{name: "restricted", higherIsBetter: true}}
	variantC = append(variantC, priority...)
	variantC = append(variantC, scoreTail...)
	variantC = append(variantC, packingSeedComparatorComponent{name: "flexibility", higherIsBetter: true}, packingSeedComparatorComponent{name: "fragmentation", higherIsBetter: false}, key)
	variantD := packingSeedPriorityScoreFirstComparatorComponents(maxPriorities, true)
	return []packingCounterfactualVariant{
		{id: "baseline", tuple: "restricted>flexibility>fragmentation>P>T>key", components: baseline},
		{id: "B", tuple: "P>star_count>restricted>flexibility>fragmentation>T_without_star>key", components: variantB},
		{id: "C", tuple: "restricted>P>T>flexibility>fragmentation>key", components: variantC},
		{id: "D", tuple: "P>T>restricted>flexibility>fragmentation>key", components: variantD},
	}
}

func packingSeedPriorityComponents(maxPriorities int) []packingSeedComparatorComponent {
	components := make([]packingSeedComparatorComponent, 0, maxPriorities)
	for index := 0; index < maxPriorities; index++ {
		components = append(components, packingSeedComparatorComponent{name: "priority_count", priorityIndex: index, hasPriority: true, higherIsBetter: true})
	}
	return components
}

func packingSeedScoreTailComponents(includeStars bool) []packingSeedComparatorComponent {
	components := []packingSeedComparatorComponent{{name: "craft_count", higherIsBetter: true}}
	if includeStars {
		components = append(components, packingSeedComparatorComponent{name: "star_count", higherIsBetter: true})
	}
	return append(components,
		packingSeedComparatorComponent{name: "item_count", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_target_breadth", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_reciprocal_pairs", higherIsBetter: true},
		packingSeedComparatorComponent{name: "star_source_definition_diversity", higherIsBetter: true},
	)
}

func packingSeedStateVariantFirstDecisive(left, right packingSeedState, components []packingSeedComparatorComponent) packingSeedStateDecision {
	for _, component := range components {
		if compare := packingSeedStateComponentCompare(left, right, component); compare != 0 {
			decision := packingSeedStateDecision{
				compare:       compare,
				component:     component.name,
				hasPriority:   component.hasPriority,
				priorityIndex: component.priorityIndex,
			}
			if !component.key {
				decision.advantage = compare
				if decision.advantage < 0 {
					decision.advantage = -decision.advantage
				}
			}
			return decision
		}
	}
	return packingSeedStateDecision{component: "exact_equal"}
}

func packingStateClassKey(state packingSeedState) string {
	counts := map[string]int{}
	for _, placement := range state.placed {
		counts[placement.ItemID]++
	}
	itemIDs := make([]string, 0, len(counts))
	for itemID := range counts {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)
	var builder strings.Builder
	builder.Grow(16 + len(itemIDs)*12)
	builder.WriteString(strconv.FormatUint(state.occupied, 16))
	for _, itemID := range itemIDs {
		builder.WriteByte('|')
		builder.WriteString(itemID)
		builder.WriteByte(':')
		builder.WriteString(strconv.Itoa(counts[itemID]))
	}
	return builder.String()
}

func freeSpaceFragmentation(gridMask uint64, occupied uint64) int {
	free := gridMask &^ occupied
	components := 0
	for free != 0 {
		bit := free & -free
		free &^= bit
		components++
		frontier := bit
		for frontier != 0 {
			current := frontier & -frontier
			frontier &^= current
			index := bits.TrailingZeros64(current)
			row := index / geometry.GridCols
			col := index % geometry.GridCols
			for _, delta := range [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}} {
				nextRow := row + delta[0]
				nextCol := col + delta[1]
				if nextRow < 0 || nextRow >= geometry.GridRows || nextCol < 0 || nextCol >= geometry.GridCols {
					continue
				}
				next := uint64(1) << uint(nextRow*geometry.GridCols+nextCol)
				if free&next == 0 {
					continue
				}
				free &^= next
				frontier |= next
			}
		}
	}
	return components
}
