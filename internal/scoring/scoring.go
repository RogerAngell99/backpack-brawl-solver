package scoring

import (
	"math/bits"
	"sort"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

func EvaluateLayout(catalog model.Catalog, placements []model.Placement) model.Evaluation {
	return EvaluateLayoutWithPriorities(catalog, placements, nil)
}

func EvaluateLayoutWithPriorities(catalog model.Catalog, placements []model.Placement, priorities []string) model.Evaluation {
	return EvaluateLayoutWithCoverageGroups(catalog, placements, priorities, nil)
}

func EvaluateLayoutWithCoverageGroups(
	catalog model.Catalog,
	placements []model.Placement,
	priorities []string,
	coverageGroups []model.CoverageGroup,
) model.Evaluation {
	crafts := EvaluateCrafts(catalog, placements)
	stars := EvaluateStars(catalog, placements)
	priorityCounts, starCoverage, starCoverageGroups, looseStarPriorities := EvaluatePriorityScoreWithCoverageGroups(catalog, placements, crafts, stars, priorities, coverageGroups)
	return model.Evaluation{
		Score: model.Score{
			CraftCount:     len(crafts),
			StarCount:      len(stars),
			ItemCount:      len(placements),
			PriorityCounts: priorityCounts,
		},
		Crafts:              crafts,
		Stars:               stars,
		StarCoverage:        starCoverage,
		StarCoverageGroups:  starCoverageGroups,
		LooseStarPriorities: looseStarPriorities,
	}
}

func EvaluateScoreOnlyWithCoverageGroups(
	catalog model.Catalog,
	placements []model.Placement,
	priorities []string,
	coverageGroups []model.CoverageGroup,
) model.Score {
	craftPriorities := craftPriorityResults(priorities)
	craftCount, craftPriorityCounts := evaluateCraftScoreOnly(catalog, placements, craftPriorities)
	starScore := evaluateStarScoreOnly(catalog, placements, priorities, coverageGroups)
	priorityCounts := make([]int, 0, len(starScore.legacyCounts)+len(craftPriorityCounts))

	if len(coverageGroups) > 0 {
		if globalPriorityOrderEnabled(priorities) {
			craftCountsByResult := craftPriorityCountsByResult(craftPriorities, craftPriorityCounts)
			groupSourceSet := coverageGroupSourceSet(normalizeCoverageGroups(coverageGroups))
			for _, priority := range priorities {
				kind, value, ok := parsePriority(priority)
				if !ok {
					priorityCounts = append(priorityCounts, 0)
					continue
				}
				switch kind {
				case "coverage_group":
					groupIndex, ok := parseCoverageGroupPriorityIndex(value)
					if !ok || groupIndex < 0 || groupIndex >= len(starScore.groupCounts) {
						priorityCounts = append(priorityCounts, 0)
						continue
					}
					priorityCounts = append(priorityCounts, starScore.groupCounts[groupIndex]...)
				case "star_source":
					if _, grouped := groupSourceSet[value]; grouped {
						priorityCounts = append(priorityCounts, 0)
					} else {
						priorityCounts = append(priorityCounts, starScore.looseCountsBySource[value])
					}
				case "craft":
					priorityCounts = append(priorityCounts, craftCountsByResult[value])
				default:
					priorityCounts = append(priorityCounts, 0)
				}
			}
		} else {
			for _, counts := range starScore.groupCounts {
				priorityCounts = append(priorityCounts, counts...)
			}
			for _, source := range starScore.looseSources {
				priorityCounts = append(priorityCounts, starScore.looseCountsBySource[source])
			}
			priorityCounts = append(priorityCounts, craftPriorityCounts...)
		}
	} else if len(priorities) > 0 {
		insertedCoverage := false
		craftIndex := 0
		for _, priority := range priorities {
			kind, _, ok := parsePriority(priority)
			if !ok {
				priorityCounts = append(priorityCounts, 0)
				continue
			}
			switch kind {
			case "star_source":
				if !insertedCoverage {
					priorityCounts = append(priorityCounts, starScore.legacyCounts...)
					insertedCoverage = true
				}
			case "craft":
				if craftIndex < len(craftPriorityCounts) {
					priorityCounts = append(priorityCounts, craftPriorityCounts[craftIndex])
					craftIndex++
				}
			default:
				priorityCounts = append(priorityCounts, 0)
			}
		}
	}
	if len(priorityCounts) == 0 && len(priorities) == 0 && len(coverageGroups) == 0 {
		priorityCounts = nil
	}

	return model.Score{
		CraftCount:     craftCount,
		StarCount:      starScore.starCount,
		ItemCount:      len(placements),
		PriorityCounts: priorityCounts,
	}
}

func EvaluatePriorityScore(
	catalog model.Catalog,
	placements []model.Placement,
	crafts []model.CraftActivation,
	stars []model.StarActivation,
	priorities []string,
) ([]int, *model.StarCoverageBreakdown) {
	counts, coverage, _, _ := EvaluatePriorityScoreWithCoverageGroups(catalog, placements, crafts, stars, priorities, nil)
	return counts, coverage
}

func EvaluatePriorityScoreWithCoverageGroups(
	catalog model.Catalog,
	placements []model.Placement,
	crafts []model.CraftActivation,
	stars []model.StarActivation,
	priorities []string,
	coverageGroups []model.CoverageGroup,
) ([]int, *model.StarCoverageBreakdown, []model.StarCoverageBreakdown, []model.LooseStarPriority) {
	if len(coverageGroups) > 0 {
		coverageGroups = normalizeCoverageGroups(coverageGroups)
		breakdowns := make([]model.StarCoverageBreakdown, 0, len(coverageGroups))
		groupCounts := make([][]int, 0, len(coverageGroups))
		for _, group := range coverageGroups {
			coverage := EvaluateStarCoverageGroup(catalog, placements, stars, group)
			if coverage == nil {
				groupCounts = append(groupCounts, nil)
				continue
			}
			groupCounts = append(groupCounts, coverageBucketCounts(coverage))
			breakdowns = append(breakdowns, *coverage)
		}
		groupSources := coverageGroupSourceSet(coverageGroups)
		looseStarPriorities := make([]model.LooseStarPriority, 0)
		looseCountsBySource := map[string]int{}
		seenLooseSources := map[string]struct{}{}
		for _, priority := range priorities {
			kind, value, ok := parsePriority(priority)
			if !ok || kind != "star_source" {
				continue
			}
			if _, grouped := groupSources[value]; grouped {
				continue
			}
			if _, seen := seenLooseSources[value]; seen {
				continue
			}
			seenLooseSources[value] = struct{}{}
			count := countLooseStarPriority(placements, stars, value)
			looseCountsBySource[value] = count
			looseStarPriorities = append(looseStarPriorities, model.LooseStarPriority{
				SourceItemID: value,
				TargetCount:  count,
			})
		}
		counts := make([]int, 0, len(coverageGroups)+len(looseStarPriorities)+len(priorities))
		if globalPriorityOrderEnabled(priorities) {
			for _, priority := range priorities {
				kind, value, ok := parsePriority(priority)
				if !ok {
					counts = append(counts, 0)
					continue
				}
				switch kind {
				case "coverage_group":
					groupIndex, ok := parseCoverageGroupPriorityIndex(value)
					if !ok || groupIndex < 0 || groupIndex >= len(groupCounts) {
						counts = append(counts, 0)
						continue
					}
					counts = append(counts, groupCounts[groupIndex]...)
				case "star_source":
					if _, grouped := groupSources[value]; grouped {
						counts = append(counts, 0)
					} else {
						counts = append(counts, looseCountsBySource[value])
					}
				case "craft":
					counts = append(counts, countCraftPriority(crafts, value))
				default:
					counts = append(counts, 0)
				}
			}
		} else {
			for _, groupCount := range groupCounts {
				counts = append(counts, groupCount...)
			}
			for _, loose := range looseStarPriorities {
				counts = append(counts, loose.TargetCount)
			}
			for _, priority := range priorities {
				kind, value, ok := parsePriority(priority)
				if !ok || kind != "craft" {
					continue
				}
				counts = append(counts, countCraftPriority(crafts, value))
			}
		}
		var first *model.StarCoverageBreakdown
		if len(breakdowns) > 0 {
			first = &breakdowns[0]
		}
		return counts, first, breakdowns, looseStarPriorities
	}

	if len(priorities) == 0 {
		return nil, nil, nil, nil
	}

	starSources := priorityStarSources(priorities)
	coverage := EvaluateStarCoverage(catalog, placements, stars, starSources)
	coverageCounts := coverageBucketCounts(coverage)

	var counts []int
	insertedCoverage := false
	for _, priority := range priorities {
		kind, value, ok := parsePriority(priority)
		if !ok {
			counts = append(counts, 0)
			continue
		}
		switch kind {
		case "star_source":
			if !insertedCoverage {
				counts = append(counts, coverageCounts...)
				insertedCoverage = true
			}
		case "craft":
			counts = append(counts, countCraftPriority(crafts, value))
		default:
			counts = append(counts, 0)
		}
	}
	if coverage != nil {
		return counts, coverage, nil, nil
	}
	return counts, nil, nil, nil
}

func EvaluatePriorityCounts(
	placements []model.Placement,
	crafts []model.CraftActivation,
	stars []model.StarActivation,
	priorities []string,
) []int {
	counts, _ := EvaluatePriorityScore(model.Catalog{}, placements, crafts, stars, priorities)
	return counts
}

func globalPriorityOrderEnabled(priorities []string) bool {
	for _, priority := range priorities {
		kind, _, ok := parsePriority(priority)
		if ok && kind == "coverage_group" {
			return true
		}
	}
	return false
}

func parseCoverageGroupPriorityIndex(value string) (int, bool) {
	index, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return index, true
}

func EvaluateStarCoverage(
	catalog model.Catalog,
	placements []model.Placement,
	stars []model.StarActivation,
	starSources []string,
) *model.StarCoverageBreakdown {
	if len(starSources) == 0 {
		return nil
	}
	return EvaluateStarCoverageGroup(catalog, placements, stars, model.CoverageGroup{Name: "Coverage", Sources: starSources})
}

func EvaluateStarCoverageGroup(
	catalog model.Catalog,
	placements []model.Placement,
	stars []model.StarActivation,
	group model.CoverageGroup,
) *model.StarCoverageBreakdown {
	starSources := uniqueNonEmptyStrings(group.Sources)
	if len(starSources) == 0 {
		return nil
	}
	targetItemIDs := uniqueNonEmptyStrings(group.Targets)
	targetItemFilter := stringSet(targetItemIDs)
	var coveredByTarget [64]uint64
	for _, star := range stars {
		if star.SourceInstance == star.TargetInstance {
			continue
		}
		sourceIndex := placementIndexByInstance(placements, star.SourceInstance)
		if sourceIndex < 0 {
			continue
		}
		sourcePriorityIndex := stringIndex(starSources, placements[sourceIndex].ItemID)
		if sourcePriorityIndex < 0 {
			continue
		}
		targetIndex := placementIndexByInstance(placements, star.TargetInstance)
		if targetIndex < 0 {
			continue
		}
		coveredByTarget[targetIndex] |= uint64(1) << uint(sourcePriorityIndex)
	}

	targets := make([]model.StarCoverageTarget, 0, len(placements))
	var bucketCounts [64]int
	for placementIndex, placement := range placements {
		explicitTarget := false
		if len(targetItemFilter) > 0 {
			if _, ok := targetItemFilter[placement.ItemID]; !ok {
				continue
			}
			explicitTarget = true
		}
		sourceMask := coveredByTarget[placementIndex]
		if sourceMask == 0 && !explicitTarget && !coverageTargetRelevant(catalog, placement, starSources) {
			continue
		}
		covered := coveredSourcesFromMask(starSources, sourceMask)
		coveredCount := bits.OnesCount64(sourceMask)
		if coveredCount > 0 {
			bucketCounts[coveredCount]++
		}
		targets = append(targets, model.StarCoverageTarget{
			TargetInstance: placement.InstanceID,
			TargetItemID:   placement.ItemID,
			CoveredSources: covered,
			CoveredCount:   coveredCount,
		})
	}

	buckets := make([]model.StarCoverageBucket, 0, len(starSources))
	for coveredCount := len(starSources); coveredCount >= 1; coveredCount-- {
		buckets = append(buckets, model.StarCoverageBucket{
			CoveredSources: coveredCount,
			TargetCount:    bucketCounts[coveredCount],
		})
	}

	return &model.StarCoverageBreakdown{
		Name:          group.Name,
		Sources:       append([]string(nil), starSources...),
		TargetItemIDs: append([]string(nil), targetItemIDs...),
		Buckets:       buckets,
		Targets:       targets,
	}
}

func normalizeCoverageGroups(groups []model.CoverageGroup) []model.CoverageGroup {
	normalized := make([]model.CoverageGroup, 0, len(groups))
	for idx, group := range groups {
		sources := uniqueNonEmptyStrings(group.Sources)
		if len(sources) == 0 {
			continue
		}
		name := strings.TrimSpace(group.Name)
		if name == "" {
			name = "Coverage " + strconv.Itoa(idx+1)
		}
		normalized = append(normalized, model.CoverageGroup{Name: name, Sources: sources, Targets: uniqueNonEmptyStrings(group.Targets)})
	}
	return normalized
}

func coverageGroupSourceSet(groups []model.CoverageGroup) map[string]struct{} {
	sources := map[string]struct{}{}
	for _, group := range groups {
		for _, source := range group.Sources {
			source = strings.TrimSpace(source)
			if source != "" {
				sources[source] = struct{}{}
			}
		}
	}
	return sources
}

func uniqueNonEmptyStrings(values []string) []string {
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || contains(result, value) {
			continue
		}
		result = append(result, value)
	}
	return result
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func countLooseStarPriority(placements []model.Placement, stars []model.StarActivation, sourceItemID string) int {
	targets := map[string]struct{}{}
	for _, star := range stars {
		sourceIndex := placementIndexByInstance(placements, star.SourceInstance)
		if sourceIndex < 0 || placements[sourceIndex].ItemID != sourceItemID {
			continue
		}
		targetIndex := placementIndexByInstance(placements, star.TargetInstance)
		if targetIndex < 0 {
			continue
		}
		targets[placements[targetIndex].InstanceID] = struct{}{}
	}
	return len(targets)
}

func coverageTargetRelevant(
	catalog model.Catalog,
	target model.Placement,
	starSources []string,
) bool {
	_, ok := catalog.Items[target.ItemID]
	if !ok {
		return false
	}
	for _, sourceID := range starSources {
		source, ok := catalog.Items[sourceID]
		if !ok {
			continue
		}
		for starIndex := range source.Stars {
			if StarMatchesCatalogItems(catalog, sourceID, target.ItemID, &source.Stars[starIndex]) {
				return true
			}
		}
	}
	return false
}

func coverageBucketCounts(coverage *model.StarCoverageBreakdown) []int {
	if coverage == nil {
		return nil
	}
	counts := make([]int, 0, len(coverage.Buckets))
	for _, bucket := range coverage.Buckets {
		counts = append(counts, bucket.TargetCount)
	}
	return counts
}

func coveredSourcesFromMask(starSources []string, sourceMask uint64) []string {
	if sourceMask == 0 {
		return nil
	}
	result := make([]string, 0, bits.OnesCount64(sourceMask))
	for idx, source := range starSources {
		if sourceMask&(uint64(1)<<uint(idx)) != 0 {
			result = append(result, source)
		}
	}
	return result
}

func priorityStarSources(priorities []string) []string {
	var sources []string
	for _, priority := range priorities {
		kind, value, ok := parsePriority(priority)
		if !ok || kind != "star_source" || contains(sources, value) {
			continue
		}
		sources = append(sources, value)
	}
	return sources
}

func placementIndexByInstance(placements []model.Placement, instanceID string) int {
	for idx, placement := range placements {
		if placement.InstanceID == instanceID {
			return idx
		}
	}
	return -1
}

func stringIndex(values []string, needle string) int {
	for idx, value := range values {
		if value == needle {
			return idx
		}
	}
	return -1
}

func countCraftPriority(crafts []model.CraftActivation, result string) int {
	count := 0
	for _, craft := range crafts {
		if craft.RecipeResult == result {
			count++
		}
	}
	return count
}

func parsePriority(priority string) (string, string, bool) {
	priority = strings.TrimSpace(priority)
	kind, value, ok := strings.Cut(priority, ":")
	if !ok {
		return "", "", false
	}
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind == "" || value == "" {
		return "", "", false
	}
	return kind, value, true
}

func StarMatchesItem(sourceID string, targetID string, item *model.Item, star *model.Star) bool {
	if item == nil || star == nil {
		return false
	}
	if star.RuleStatus == "unknown" {
		return false
	}
	if star.ExcludeSourceItem && sourceID == targetID {
		return false
	}
	if len(star.TargetItems) == 0 && len(star.TargetTypes) == 0 {
		return true
	}
	if compiledMatch, ok := compiledStarMatchesItem(item, star); ok {
		return compiledMatch
	}
	return starMatchesItemByStrings(targetID, item, star)
}

func itemMatchesStarItem(sourceID string, targetID string, item *model.Item, star *model.Star) bool {
	return StarMatchesItem(sourceID, targetID, item, star)
}

func compiledStarMatchesItem(item *model.Item, star *model.Star) (bool, bool) {
	if !star.CompiledReady || !item.CompiledReady {
		return false, false
	}
	if len(star.TargetItems) > 0 {
		if !star.CompiledTargetItemsComplete || !item.CompiledItemRefsComplete {
			return false, false
		}
		if compiledItemRefsOverlap(star.CompiledTargetItemID, star.CompiledTargetItemLen, item) {
			return true, true
		}
	}
	if star.CompiledTargetTypeMask != 0 && star.CompiledTargetTypeMask&item.CompiledTypeMask != 0 {
		return true, true
	}
	return false, true
}

func compiledItemRefsOverlap(targetItemID uint16, targetItemLen uint8, item *model.Item) bool {
	if targetItemLen == 0 || item.CompiledItemRefLen == 0 {
		return false
	}
	if targetItemID == item.CompiledItemID {
		return true
	}
	return item.CompiledAliasItemLen > 0 && targetItemID == item.CompiledAliasItemID
}

func starMatchesItemByStrings(targetID string, item *model.Item, star *model.Star) bool {
	if len(star.TargetItems) > 0 && targetMatchesItemFilter(targetID, item, star.TargetItems) {
		return true
	}
	for _, targetType := range star.TargetTypes {
		for _, itemType := range item.Types {
			if targetType == itemType {
				return true
			}
		}
	}
	return false
}

func targetMatchesItemFilter(targetID string, item *model.Item, targetItems []string) bool {
	if contains(targetItems, targetID) {
		return true
	}
	for _, alias := range item.CountsAs {
		if contains(targetItems, alias.ItemID) {
			return true
		}
	}
	return false
}

func itemMatchesStar(catalog model.Catalog, source model.Placement, target model.Placement, star *model.Star) bool {
	return EvaluateCatalogStarCondition(catalog, source.ItemID, target.ItemID, star) == ConditionTrue
}

type scoreOnlyCoverageGroup struct {
	sources         []string
	targetItemIDs   []string
	coveredByTarget [64]uint64
}

type starScoreOnlyResult struct {
	starCount           int
	legacyCounts        []int
	groupCounts         [][]int
	looseSources        []string
	looseCountsBySource map[string]int
}

func evaluateStarScoreOnly(
	catalog model.Catalog,
	placements []model.Placement,
	priorities []string,
	coverageGroups []model.CoverageGroup,
) starScoreOnlyResult {
	var groups []scoreOnlyCoverageGroup
	var looseSources []string
	var oldPrioritySources []string
	if len(coverageGroups) > 0 {
		normalized := normalizeCoverageGroups(coverageGroups)
		groupSourceSet := coverageGroupSourceSet(normalized)
		groups = make([]scoreOnlyCoverageGroup, 0, len(normalized))
		for _, group := range normalized {
			groups = append(groups, scoreOnlyCoverageGroup{
				sources:       group.Sources,
				targetItemIDs: group.Targets,
			})
		}
		seenLoose := map[string]struct{}{}
		for _, priority := range priorities {
			kind, value, ok := parsePriority(priority)
			if !ok || kind != "star_source" {
				continue
			}
			if _, grouped := groupSourceSet[value]; grouped {
				continue
			}
			if _, seen := seenLoose[value]; seen {
				continue
			}
			seenLoose[value] = struct{}{}
			looseSources = append(looseSources, value)
		}
	} else if len(priorities) > 0 {
		oldPrioritySources = priorityStarSources(priorities)
	}
	looseTargetMasks := make([]uint64, len(looseSources))
	var oldCoveredByTarget [64]uint64

	var cellOwner [geometry.GridCells]int
	for idx := range cellOwner {
		cellOwner[idx] = -1
	}
	for placementIndex, placement := range placements {
		for _, cell := range placement.Cells {
			cellOwner[geometry.CellIndex(cell)] = placementIndex
		}
	}

	var countedPairs [64]uint64
	starCount := 0
	for sourceIndex, source := range placements {
		for starPositionIndex := range source.StarPositions {
			starPosition := &source.StarPositions[starPositionIndex]
			if !geometry.InBounds(starPosition.Position) {
				continue
			}
			targetIndex := cellOwner[geometry.CellIndex(starPosition.Position)]
			if targetIndex < 0 || targetIndex == sourceIndex {
				continue
			}
			target := placements[targetIndex]
			if !itemMatchesStar(catalog, source, target, &starPosition.Star) {
				continue
			}
			targetBit := uint64(1) << uint(targetIndex)
			if countedPairs[sourceIndex]&targetBit != 0 {
				continue
			}
			countedPairs[sourceIndex] |= targetBit
			starCount++

			for groupIndex := range groups {
				sourcePriorityIndex := stringIndex(groups[groupIndex].sources, source.ItemID)
				if sourcePriorityIndex >= 0 {
					groups[groupIndex].coveredByTarget[targetIndex] |= uint64(1) << uint(sourcePriorityIndex)
				}
			}
			for looseIndex, looseSource := range looseSources {
				if looseSource == source.ItemID {
					looseTargetMasks[looseIndex] |= targetBit
				}
			}
			oldSourceIndex := stringIndex(oldPrioritySources, source.ItemID)
			if oldSourceIndex >= 0 {
				oldCoveredByTarget[targetIndex] |= uint64(1) << uint(oldSourceIndex)
			}
		}
	}

	result := starScoreOnlyResult{
		starCount:           starCount,
		looseSources:        looseSources,
		looseCountsBySource: map[string]int{},
	}
	if len(groups) > 0 {
		result.groupCounts = make([][]int, 0, len(groups))
		for _, group := range groups {
			result.groupCounts = append(result.groupCounts, scoreOnlyCoverageBucketCounts(catalog, placements, group.sources, group.targetItemIDs, group.coveredByTarget))
		}
		for looseIndex, targetMask := range looseTargetMasks {
			result.looseCountsBySource[looseSources[looseIndex]] = bits.OnesCount64(targetMask)
		}
		return result
	}
	if len(oldPrioritySources) > 0 {
		result.legacyCounts = scoreOnlyCoverageBucketCounts(catalog, placements, oldPrioritySources, nil, oldCoveredByTarget)
		return result
	}
	return result
}

func scoreOnlyCoverageBucketCounts(
	catalog model.Catalog,
	placements []model.Placement,
	starSources []string,
	targetItemIDs []string,
	coveredByTarget [64]uint64,
) []int {
	if len(starSources) == 0 {
		return nil
	}
	var bucketCounts [64]int
	for placementIndex, placement := range placements {
		explicitTarget := false
		if len(targetItemIDs) > 0 {
			if !contains(targetItemIDs, placement.ItemID) {
				continue
			}
			explicitTarget = true
		}
		sourceMask := coveredByTarget[placementIndex]
		if sourceMask == 0 && !explicitTarget && !coverageTargetRelevant(catalog, placement, starSources) {
			continue
		}
		coveredCount := bits.OnesCount64(sourceMask)
		if coveredCount > 0 {
			bucketCounts[coveredCount]++
		}
	}
	counts := make([]int, 0, len(starSources))
	for coveredCount := len(starSources); coveredCount >= 1; coveredCount-- {
		counts = append(counts, bucketCounts[coveredCount])
	}
	return counts
}

func EvaluateStars(catalog model.Catalog, placements []model.Placement) []model.StarActivation {
	var cellOwner [geometry.GridCells]int
	for idx := range cellOwner {
		cellOwner[idx] = -1
	}
	for placementIndex, placement := range placements {
		for _, cell := range placement.Cells {
			cellOwner[geometry.CellIndex(cell)] = placementIndex
		}
	}

	var countedPairs [64]uint64
	var activations []model.StarActivation
	for sourceIndex, source := range placements {
		for starPositionIndex := range source.StarPositions {
			starPosition := &source.StarPositions[starPositionIndex]
			if !geometry.InBounds(starPosition.Position) {
				continue
			}
			targetIndex := cellOwner[geometry.CellIndex(starPosition.Position)]
			if targetIndex < 0 || targetIndex == sourceIndex {
				continue
			}
			target := placements[targetIndex]
			if !itemMatchesStar(catalog, source, target, &starPosition.Star) {
				continue
			}
			targetBit := uint64(1) << uint(targetIndex)
			if countedPairs[sourceIndex]&targetBit != 0 {
				continue
			}
			countedPairs[sourceIndex] |= targetBit
			if activations == nil {
				capacity := len(placements)
				if capacity > 8 {
					capacity = 8
				}
				activations = make([]model.StarActivation, 0, capacity)
			}
			activations = append(activations, model.StarActivation{
				SourceInstance: source.InstanceID,
				TargetInstance: target.InstanceID,
				StarPosition:   starPosition.Position,
				EffectText:     starPosition.Star.EffectText,
			})
		}
	}
	if activations == nil {
		return []model.StarActivation{}
	}
	return activations
}

func EvaluateCrafts(catalog model.Catalog, placements []model.Placement) []model.CraftActivation {
	sortedPlacements := sortedPlacementsForCrafts(placements)
	var used uint64
	var activations []model.CraftActivation
	for recipeIndex := range catalog.Recipes {
		recipe := &catalog.Recipes[recipeIndex]
		requirements := compiledIngredientRequirements(recipe)
		for anchorIndex, anchor := range sortedPlacements {
			if used&(uint64(1)<<uint(anchorIndex)) != 0 || anchor.ItemID != recipe.Anchor {
				continue
			}
			var selected [64]int
			selectedCount := 0
			viable := true
			for reqIndex := 0; reqIndex < requirements.Len; reqIndex++ {
				found := 0
				for placementIndex, placement := range sortedPlacements {
					if placementIndex == anchorIndex ||
						used&(uint64(1)<<uint(placementIndex)) != 0 ||
						placement.ItemID != requirements.Items[reqIndex] {
						continue
					}
					if anchor.AdjacentMask&placement.Mask != 0 {
						selected[selectedCount] = placementIndex
						selectedCount++
						found++
						if found == requirements.Counts[reqIndex] {
							break
						}
					}
				}
				if found < requirements.Counts[reqIndex] {
					viable = false
					break
				}
			}
			if !viable {
				continue
			}
			used |= uint64(1) << uint(anchorIndex)
			for idx := 0; idx < selectedCount; idx++ {
				used |= uint64(1) << uint(selected[idx])
			}
			ingredientInstances := make([]string, 0, selectedCount)
			for idx := 0; idx < selectedCount; idx++ {
				ingredientInstances = append(ingredientInstances, sortedPlacements[selected[idx]].InstanceID)
			}
			activations = append(activations, model.CraftActivation{
				RecipeResult:        recipe.Result,
				AnchorInstance:      anchor.InstanceID,
				IngredientInstances: ingredientInstances,
			})
		}
	}
	return activations
}

func craftPriorityResults(priorities []string) []string {
	results := make([]string, 0)
	for _, priority := range priorities {
		kind, value, ok := parsePriority(priority)
		if ok && kind == "craft" {
			results = append(results, value)
		}
	}
	return results
}

func craftPriorityCountsByResult(priorityResults []string, priorityCounts []int) map[string]int {
	countsByResult := map[string]int{}
	for idx, result := range priorityResults {
		if idx < len(priorityCounts) {
			countsByResult[result] = priorityCounts[idx]
		}
	}
	return countsByResult
}

func evaluateCraftScoreOnly(catalog model.Catalog, placements []model.Placement, priorityResults []string) (int, []int) {
	sortedPlacements := sortedPlacementsForCrafts(placements)
	var used uint64
	craftCount := 0
	priorityCounts := make([]int, len(priorityResults))
	for recipeIndex := range catalog.Recipes {
		recipe := &catalog.Recipes[recipeIndex]
		requirements := compiledIngredientRequirements(recipe)
		for anchorIndex, anchor := range sortedPlacements {
			if used&(uint64(1)<<uint(anchorIndex)) != 0 || anchor.ItemID != recipe.Anchor {
				continue
			}
			var selected [64]int
			selectedCount := 0
			viable := true
			for reqIndex := 0; reqIndex < requirements.Len; reqIndex++ {
				found := 0
				for placementIndex, placement := range sortedPlacements {
					if placementIndex == anchorIndex ||
						used&(uint64(1)<<uint(placementIndex)) != 0 ||
						placement.ItemID != requirements.Items[reqIndex] {
						continue
					}
					if anchor.AdjacentMask&placement.Mask != 0 {
						selected[selectedCount] = placementIndex
						selectedCount++
						found++
						if found == requirements.Counts[reqIndex] {
							break
						}
					}
				}
				if found < requirements.Counts[reqIndex] {
					viable = false
					break
				}
			}
			if !viable {
				continue
			}
			used |= uint64(1) << uint(anchorIndex)
			for idx := 0; idx < selectedCount; idx++ {
				used |= uint64(1) << uint(selected[idx])
			}
			craftCount++
			for priorityIndex, result := range priorityResults {
				if recipe.Result == result {
					priorityCounts[priorityIndex]++
				}
			}
		}
	}
	return craftCount, priorityCounts
}

func compiledIngredientRequirements(recipe *model.Recipe) *model.RecipeRequirements {
	if recipe.CompiledRequirements.Ready {
		return &recipe.CompiledRequirements
	}
	requirements := model.BuildRecipeRequirements(recipe.Anchor, recipe.Ingredients)
	return &requirements
}

func sortedPlacementsForCrafts(placements []model.Placement) []model.Placement {
	if placementsSortedForCrafts(placements) {
		return placements
	}
	sortedPlacements := append([]model.Placement(nil), placements...)
	sort.Slice(sortedPlacements, func(i, j int) bool {
		if sortedPlacements[i].OriginalIndex != sortedPlacements[j].OriginalIndex {
			return sortedPlacements[i].OriginalIndex < sortedPlacements[j].OriginalIndex
		}
		return sortedPlacements[i].InstanceID < sortedPlacements[j].InstanceID
	})
	return sortedPlacements
}

func placementsSortedForCrafts(placements []model.Placement) bool {
	for idx := 1; idx < len(placements); idx++ {
		if placements[idx-1].OriginalIndex > placements[idx].OriginalIndex {
			return false
		}
		if placements[idx-1].OriginalIndex == placements[idx].OriginalIndex &&
			placements[idx-1].InstanceID > placements[idx].InstanceID {
			return false
		}
	}
	return true
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
