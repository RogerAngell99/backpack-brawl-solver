package solver

import (
	"math/bits"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

const maxExactBoundCoverageContexts = 16

type exactBoundContext struct {
	enabled bool

	remainingOriginalMask []uint64
	fullOriginalMask      uint64
	originalItemIndex     [64]int
	itemIDs               []string
	itemIndexByID         map[string]int

	possibleStarTargetMaskByOriginal [64]uint64
	recipes                          []exactBoundRecipe
	coverageContexts                 [maxExactBoundCoverageContexts]*coverageContext
	coverageContextByGroup           [maxExactBoundCoverageContexts]int
	coverageContextLen               int

	globalMode      bool
	groupMode       bool
	legacyMode      bool
	coverageMode    bool
	groupSourceSet  map[string]struct{}
	looseSources    []string
	priorityEntries []exactBoundPriorityEntry
}

type exactBoundRecipe struct {
	result      string
	anchorIndex int
	items       [16]int
	counts      [16]int
	len         int
}

type exactBoundPriorityEntry struct {
	kind       string
	value      string
	groupIndex int
}

type exactBoundState struct {
	placedOriginalMask  uint64
	droppedOriginalMask uint64
	coverageStates      [maxExactBoundCoverageContexts]coverageSearchState
}

func newExactBoundContext(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	ordered []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	config Config,
) *exactBoundContext {
	// Exact bounds are used only for exhaustive solves. Limited solves already use
	// seed/repair heuristics, and benchmark coverage showed per-node bounds reduce
	// nodes/sec there without improving score.
	if config.DisableExactBounds || config.TopN <= 0 || config.MaxNodes > 0 || len(instances) > 64 {
		return nil
	}
	if len(config.Priorities) == 0 && len(config.CoverageGroups) == 0 {
		return nil
	}
	ctx := &exactBoundContext{
		enabled:        true,
		itemIndexByID:  map[string]int{},
		groupSourceSet: map[string]struct{}{},
	}
	for idx := range ctx.originalItemIndex {
		ctx.originalItemIndex[idx] = -1
	}
	for idx := range ctx.coverageContextByGroup {
		ctx.coverageContextByGroup[idx] = -1
	}
	for _, instance := range instances {
		if instance.OriginalIndex < 0 || instance.OriginalIndex >= 64 {
			return nil
		}
		itemIndex, ok := ctx.itemIndexByID[instance.ItemID]
		if !ok {
			itemIndex = len(ctx.itemIDs)
			ctx.itemIndexByID[instance.ItemID] = itemIndex
			ctx.itemIDs = append(ctx.itemIDs, instance.ItemID)
		}
		ctx.originalItemIndex[instance.OriginalIndex] = itemIndex
		ctx.fullOriginalMask |= uint64(1) << uint(instance.OriginalIndex)
	}
	ctx.prepareRemainingOriginalMask(ordered)
	ctx.preparePossibleStarTargets(catalog, instances)
	ctx.prepareRecipes(catalog)
	if !ctx.preparePriorities(catalog, instances, optionsByInstance, ordered, config) {
		return nil
	}
	return ctx
}

func (ctx *exactBoundContext) prepareRemainingOriginalMask(ordered []model.InventoryInstance) {
	ctx.remainingOriginalMask = make([]uint64, len(ordered)+1)
	var mask uint64
	for idx := len(ordered) - 1; idx >= 0; idx-- {
		mask |= uint64(1) << uint(ordered[idx].OriginalIndex)
		ctx.remainingOriginalMask[idx] = mask
	}
}

func (ctx *exactBoundContext) preparePossibleStarTargets(catalog model.Catalog, instances []model.InventoryInstance) {
	for _, source := range instances {
		sourceItem := catalog.Items[source.ItemID]
		if len(sourceItem.Stars) == 0 {
			continue
		}
		var targetMask uint64
		for _, target := range instances {
			if source.OriginalIndex == target.OriginalIndex {
				continue
			}
			for starIndex := range sourceItem.Stars {
				if scoring.StarMatchesCatalogItems(catalog, source.ItemID, target.ItemID, &sourceItem.Stars[starIndex]) {
					targetMask |= uint64(1) << uint(target.OriginalIndex)
					break
				}
			}
		}
		ctx.possibleStarTargetMaskByOriginal[source.OriginalIndex] = targetMask
	}
}

func (ctx *exactBoundContext) prepareRecipes(catalog model.Catalog) {
	ctx.recipes = make([]exactBoundRecipe, 0, len(catalog.Recipes))
	for _, recipe := range catalog.Recipes {
		anchorIndex, ok := ctx.itemIndexByID[recipe.Anchor]
		if !ok {
			continue
		}
		requirements := recipe.CompiledRequirements
		if !requirements.Ready {
			requirements = model.BuildRecipeRequirements(recipe.Anchor, recipe.Ingredients)
		}
		compiled := exactBoundRecipe{
			result:      recipe.Result,
			anchorIndex: anchorIndex,
		}
		ready := true
		for reqIndex := 0; reqIndex < requirements.Len; reqIndex++ {
			itemIndex, ok := ctx.itemIndexByID[requirements.Items[reqIndex]]
			if !ok {
				ready = false
				break
			}
			compiled.items[compiled.len] = itemIndex
			compiled.counts[compiled.len] = requirements.Counts[reqIndex]
			compiled.len++
		}
		if ready {
			ctx.recipes = append(ctx.recipes, compiled)
		}
	}
}

func (ctx *exactBoundContext) preparePriorities(
	catalog model.Catalog,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
	ordered []model.InventoryInstance,
	config Config,
) bool {
	if len(config.CoverageGroups) > 0 {
		ctx.groupMode = true
		normalizedGroups := exactBoundNormalizeCoverageGroups(config.CoverageGroups)
		for groupIndex, group := range normalizedGroups {
			if ctx.coverageContextLen >= maxExactBoundCoverageContexts {
				return false
			}
			coverage := newCoverageContextFromSources(catalog, instances, optionsByInstance, group.Sources, group.Targets, 0)
			if coverage == nil {
				continue
			}
			coverage.prepareOrder(ordered)
			ctx.coverageContexts[ctx.coverageContextLen] = coverage
			if groupIndex < len(ctx.coverageContextByGroup) {
				ctx.coverageContextByGroup[groupIndex] = ctx.coverageContextLen
			}
			ctx.coverageContextLen++
			ctx.coverageMode = true
			for _, source := range group.Sources {
				ctx.groupSourceSet[source] = struct{}{}
			}
		}
		if globalPriorityOrderEnabledForSolver(config.Priorities) {
			ctx.globalMode = true
			for _, priority := range config.Priorities {
				kind, value, ok := parsePriorityForSolver(priority)
				if !ok {
					ctx.priorityEntries = append(ctx.priorityEntries, exactBoundPriorityEntry{kind: "", value: "", groupIndex: -1})
					continue
				}
				entry := exactBoundPriorityEntry{kind: kind, value: value, groupIndex: -1}
				if kind == "coverage_group" {
					if groupIndex, ok := parseCoverageGroupIndexForSolver(value); ok {
						entry.groupIndex = groupIndex
					}
				}
				ctx.priorityEntries = append(ctx.priorityEntries, entry)
			}
			return true
		}
		seenLoose := map[string]struct{}{}
		for _, priority := range config.Priorities {
			kind, value, ok := parsePriorityForSolver(priority)
			if !ok || kind != "star_source" {
				continue
			}
			if _, grouped := ctx.groupSourceSet[value]; grouped {
				continue
			}
			if _, seen := seenLoose[value]; seen {
				continue
			}
			seenLoose[value] = struct{}{}
			ctx.looseSources = append(ctx.looseSources, value)
		}
		for _, priority := range config.Priorities {
			kind, value, ok := parsePriorityForSolver(priority)
			if ok && kind == "craft" {
				ctx.priorityEntries = append(ctx.priorityEntries, exactBoundPriorityEntry{kind: kind, value: value})
			}
		}
		return true
	}

	if len(config.Priorities) == 0 {
		return true
	}
	ctx.legacyMode = true
	starSources := priorityStarSourcesForSolver(config.Priorities)
	if len(starSources) > 0 {
		if ctx.coverageContextLen >= maxExactBoundCoverageContexts {
			return false
		}
		coverage := newCoverageContextFromSources(catalog, instances, optionsByInstance, starSources, nil, coveragePriorityStart(config.Priorities))
		if coverage != nil {
			coverage.prepareOrder(ordered)
			ctx.coverageContexts[0] = coverage
			ctx.coverageContextLen = 1
			ctx.coverageMode = true
		}
	}
	for _, priority := range config.Priorities {
		kind, value, ok := parsePriorityForSolver(priority)
		if !ok {
			ctx.priorityEntries = append(ctx.priorityEntries, exactBoundPriorityEntry{kind: "", value: ""})
			continue
		}
		ctx.priorityEntries = append(ctx.priorityEntries, exactBoundPriorityEntry{kind: kind, value: value})
	}
	return true
}

func exactBoundNormalizeCoverageGroups(groups []model.CoverageGroup) []model.CoverageGroup {
	normalized := make([]model.CoverageGroup, 0, len(groups))
	for _, group := range groups {
		sources := uniqueCoverageSourcesForSolver(group.Sources)
		if len(sources) == 0 {
			continue
		}
		normalized = append(normalized, model.CoverageGroup{
			Sources: sources,
			Targets: uniqueCoverageSourcesForSolver(group.Targets),
		})
	}
	return normalized
}

func (ctx *exactBoundContext) initialState(catalog model.Catalog, ordered []model.InventoryInstance, index int, placements []model.Placement) exactBoundState {
	var state exactBoundState
	if ctx == nil || !ctx.enabled {
		return state
	}
	for _, placement := range placements {
		state.placedOriginalMask |= uint64(1) << uint(placement.OriginalIndex)
	}
	state.droppedOriginalMask = ctx.fullOriginalMask &^ ctx.activeMask(state, index)
	for idx := 0; idx < ctx.coverageContextLen; idx++ {
		state.coverageStates[idx] = ctx.coverageContexts[idx].initialState(catalog, ordered, index, placements)
	}
	return state
}

func (ctx *exactBoundContext) withPlacement(catalog model.Catalog, state exactBoundState, placement model.Placement, existing []model.Placement) exactBoundState {
	if ctx == nil || !ctx.enabled {
		return state
	}
	state.placedOriginalMask |= uint64(1) << uint(placement.OriginalIndex)
	for idx := 0; idx < ctx.coverageContextLen; idx++ {
		state.coverageStates[idx] = ctx.coverageContexts[idx].withPlacement(catalog, state.coverageStates[idx], placement, existing)
	}
	return state
}

func (ctx *exactBoundContext) withSkip(state exactBoundState, instance model.InventoryInstance) exactBoundState {
	if ctx == nil || !ctx.enabled {
		return state
	}
	state.droppedOriginalMask |= uint64(1) << uint(instance.OriginalIndex)
	for idx := 0; idx < ctx.coverageContextLen; idx++ {
		state.coverageStates[idx] = ctx.coverageContexts[idx].withSkip(state.coverageStates[idx], instance)
	}
	return state
}

func (ctx *exactBoundContext) shouldCheck(state exactBoundState) bool {
	return ctx != nil && ctx.enabled && (ctx.coverageMode || state.droppedOriginalMask != 0)
}

func (ctx *exactBoundContext) shouldPrune(state exactBoundState, index int, results []model.Solution, topN int) bool {
	if ctx == nil || !ctx.enabled || topN <= 0 || len(results) < topN {
		return false
	}
	activeMask := ctx.activeMask(state, index)
	if activeMask == ctx.fullOriginalMask && !ctx.coverageMode {
		return false
	}
	bound, ok := ctx.boundScoreFromActiveMask(state, index, activeMask)
	if !ok {
		return false
	}
	return compareScores(bound, results[len(results)-1].Evaluation.Score) < 0
}

func (ctx *exactBoundContext) boundScore(state exactBoundState, index int) (model.Score, bool) {
	if ctx == nil || !ctx.enabled {
		return model.Score{}, false
	}
	return ctx.boundScoreFromActiveMask(state, index, ctx.activeMask(state, index))
}

func (ctx *exactBoundContext) activeMask(state exactBoundState, index int) uint64 {
	activeMask := state.placedOriginalMask
	if index >= 0 && index < len(ctx.remainingOriginalMask) {
		activeMask |= ctx.remainingOriginalMask[index]
	}
	return activeMask
}

func (ctx *exactBoundContext) boundScoreFromActiveMask(state exactBoundState, index int, activeMask uint64) (model.Score, bool) {
	activeCounts := ctx.activeItemCounts(activeMask)
	priorityCounts, ok := ctx.boundPriorityCounts(state, index, activeMask, activeCounts)
	if !ok {
		return model.Score{}, false
	}
	return model.Score{
		CraftCount:     ctx.boundCraftCount(activeCounts, ""),
		StarCount:      ctx.boundStarCount(activeMask),
		ItemCount:      bits.OnesCount64(activeMask),
		PriorityCounts: priorityCounts,
	}, true
}

func (ctx *exactBoundContext) activeItemCounts(activeMask uint64) [64]int {
	var counts [64]int
	for mask := activeMask; mask != 0; mask &= mask - 1 {
		originalIndex := bits.TrailingZeros64(mask)
		itemIndex := ctx.originalItemIndex[originalIndex]
		if itemIndex >= 0 {
			counts[itemIndex]++
		}
	}
	return counts
}

func (ctx *exactBoundContext) boundStarCount(activeMask uint64) int {
	count := 0
	for mask := activeMask; mask != 0; mask &= mask - 1 {
		sourceOriginal := bits.TrailingZeros64(mask)
		count += bits.OnesCount64(ctx.possibleStarTargetMaskByOriginal[sourceOriginal] & activeMask)
	}
	return count
}

func (ctx *exactBoundContext) boundCraftCount(activeCounts [64]int, result string) int {
	total := 0
	for _, recipe := range ctx.recipes {
		if result != "" && recipe.result != result {
			continue
		}
		recipeBound := activeCounts[recipe.anchorIndex]
		for idx := 0; idx < recipe.len; idx++ {
			ingredientBound := 0
			if recipe.counts[idx] > 0 {
				ingredientBound = activeCounts[recipe.items[idx]] / recipe.counts[idx]
			}
			if ingredientBound < recipeBound {
				recipeBound = ingredientBound
			}
		}
		total += recipeBound
	}
	return total
}

func (ctx *exactBoundContext) boundPriorityCounts(state exactBoundState, index int, activeMask uint64, activeCounts [64]int) ([]int, bool) {
	if ctx.globalMode {
		counts := make([]int, 0, len(ctx.priorityEntries)+4)
		for _, entry := range ctx.priorityEntries {
			switch entry.kind {
			case "":
				counts = append(counts, 0)
			case "coverage_group":
				if entry.groupIndex < 0 || entry.groupIndex >= len(ctx.coverageContextByGroup) {
					counts = append(counts, 0)
					continue
				}
				ctxIndex := ctx.coverageContextByGroup[entry.groupIndex]
				if ctxIndex < 0 || ctxIndex >= ctx.coverageContextLen {
					counts = append(counts, 0)
					continue
				}
				counts = append(counts, ctx.coverageContexts[ctxIndex].upperBoundCounts(state.coverageStates[ctxIndex], index)...)
			case "star_source":
				if _, grouped := ctx.groupSourceSet[entry.value]; grouped {
					counts = append(counts, 0)
				} else {
					counts = append(counts, ctx.boundLooseStarTargets(entry.value, activeMask))
				}
			case "craft":
				counts = append(counts, ctx.boundCraftCount(activeCounts, entry.value))
			default:
				counts = append(counts, 0)
			}
		}
		return counts, true
	}
	if ctx.groupMode {
		counts := make([]int, 0, ctx.coverageContextLen+len(ctx.looseSources)+len(ctx.priorityEntries))
		for ctxIndex := 0; ctxIndex < ctx.coverageContextLen; ctxIndex++ {
			counts = append(counts, ctx.coverageContexts[ctxIndex].upperBoundCounts(state.coverageStates[ctxIndex], index)...)
		}
		for _, sourceID := range ctx.looseSources {
			counts = append(counts, ctx.boundLooseStarTargets(sourceID, activeMask))
		}
		for _, entry := range ctx.priorityEntries {
			if entry.kind == "craft" {
				counts = append(counts, ctx.boundCraftCount(activeCounts, entry.value))
			}
		}
		return counts, true
	}
	if ctx.legacyMode {
		counts := make([]int, 0, len(ctx.priorityEntries)+4)
		insertedCoverage := false
		for _, entry := range ctx.priorityEntries {
			switch entry.kind {
			case "":
				counts = append(counts, 0)
			case "star_source":
				if !insertedCoverage {
					if ctx.coverageContextLen > 0 {
						counts = append(counts, ctx.coverageContexts[0].upperBoundCounts(state.coverageStates[0], index)...)
					}
					insertedCoverage = true
				}
			case "craft":
				counts = append(counts, ctx.boundCraftCount(activeCounts, entry.value))
			default:
				counts = append(counts, 0)
			}
		}
		return counts, true
	}
	return nil, true
}

func (ctx *exactBoundContext) boundLooseStarTargets(sourceID string, activeMask uint64) int {
	var targetMask uint64
	for mask := activeMask; mask != 0; mask &= mask - 1 {
		sourceOriginal := bits.TrailingZeros64(mask)
		itemIndex := ctx.originalItemIndex[sourceOriginal]
		if itemIndex < 0 || ctx.itemIDs[itemIndex] != sourceID {
			continue
		}
		targetMask |= ctx.possibleStarTargetMaskByOriginal[sourceOriginal] & activeMask
	}
	return bits.OnesCount64(targetMask)
}
