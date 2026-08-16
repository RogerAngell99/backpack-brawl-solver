package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

type jsonSolution struct {
	LayoutKey           string                  `json:"layout_key,omitempty"`
	Score               jsonScore               `json:"score"`
	Search              jsonSearch              `json:"search"`
	Coverage            *jsonCoverage           `json:"coverage,omitempty"`
	CoverageGroups      []jsonCoverage          `json:"coverage_groups,omitempty"`
	LooseStarPriorities []jsonLooseStarPriority `json:"loose_star_priorities,omitempty"`
	Placements          []jsonPlacement         `json:"placements"`
	Crafts              []jsonCraft             `json:"crafts"`
	Stars               []jsonStar              `json:"stars"`
}

type jsonScore struct {
	Crafts         int   `json:"crafts"`
	Stars          int   `json:"stars"`
	Items          int   `json:"items"`
	PriorityCounts []int `json:"priority_counts,omitempty"`
}

type jsonSearch struct {
	NodesExplored               int64                `json:"nodes_explored"`
	NodesPerSecond              float64              `json:"nodes_per_second,omitempty"`
	SetupMS                     int64                `json:"setup_ms,omitempty"`
	SeedMS                      int64                `json:"seed_ms,omitempty"`
	RepairMS                    int64                `json:"repair_ms,omitempty"`
	SearchMS                    int64                `json:"search_ms,omitempty"`
	RefineMS                    int64                `json:"refine_ms,omitempty"`
	Backend                     string               `json:"backend,omitempty"`
	ServerElapsedMS             int64                `json:"server_elapsed_ms,omitempty"`
	RemoteWorkers               int                  `json:"remote_workers,omitempty"`
	MaxNodesApplied             int64                `json:"max_nodes_applied,omitempty"`
	MaxNodesCapped              bool                 `json:"max_nodes_capped,omitempty"`
	Limited                     bool                 `json:"limited"`
	Refined                     bool                 `json:"refined"`
	CoverageSources             []string             `json:"coverage_sources,omitempty"`
	CoverageTargetCount         int                  `json:"coverage_target_count,omitempty"`
	CoverageCeiling             []jsonCoverageBucket `json:"coverage_ceiling,omitempty"`
	CoverageCeilingReached      bool                 `json:"coverage_ceiling_reached,omitempty"`
	CoverageBoundChecks         int64                `json:"coverage_bound_checks,omitempty"`
	CoveragePrunedNodes         int64                `json:"coverage_pruned_nodes,omitempty"`
	ExactBoundChecks            int64                `json:"exact_bound_checks,omitempty"`
	ExactBoundPrunedNodes       int64                `json:"exact_bound_pruned_nodes,omitempty"`
	CoverageSeedNodes           int64                `json:"coverage_seed_nodes,omitempty"`
	CoverageSeedCandidates      int                  `json:"coverage_seed_candidates,omitempty"`
	CoverageSeedBest            string               `json:"coverage_seed_best,omitempty"`
	ParallelTasks               int                  `json:"parallel_tasks,omitempty"`
	ParallelWorkersUsed         int                  `json:"parallel_workers_used,omitempty"`
	RefineMovesChecked          int64                `json:"refine_moves_checked,omitempty"`
	RefineImprovements          int                  `json:"refine_improvements,omitempty"`
	RefineBestDelta             string               `json:"refine_best_delta,omitempty"`
	RepairNodes                 int64                `json:"repair_nodes,omitempty"`
	RepairIterations            int                  `json:"repair_iterations,omitempty"`
	RepairImprovements          int                  `json:"repair_improvements,omitempty"`
	RepairCandidates            int                  `json:"repair_candidates,omitempty"`
	RepairBest                  string               `json:"repair_best,omitempty"`
	RepairParallelTasks         int                  `json:"repair_parallel_tasks,omitempty"`
	RepairParallelWorkersUsed   int                  `json:"repair_parallel_workers_used,omitempty"`
	StoppedAfterCoverageCeiling bool                 `json:"stopped_after_coverage_ceiling,omitempty"`
}

type jsonPlacement struct {
	InstanceID    string  `json:"instance_id"`
	ItemID        string  `json:"item_id"`
	Rotation      int     `json:"rotation"`
	Origin        []int   `json:"origin"`
	Cells         [][]int `json:"cells"`
	StarPositions [][]int `json:"star_positions"`
}

type jsonCraft struct {
	Result              string   `json:"result"`
	AnchorInstance      string   `json:"anchor_instance"`
	IngredientInstances []string `json:"ingredient_instances"`
}

type jsonStar struct {
	SourceInstance string `json:"source_instance"`
	TargetInstance string `json:"target_instance"`
	StarPosition   []int  `json:"star_position"`
	EffectText     string `json:"effect_text"`
}

type jsonCoverage struct {
	Name          string               `json:"name,omitempty"`
	Sources       []string             `json:"sources"`
	TargetItemIDs []string             `json:"target_item_ids,omitempty"`
	Buckets       []jsonCoverageBucket `json:"buckets"`
	Targets       []jsonCoverageTarget `json:"targets"`
	Summary       string               `json:"summary"`
}

type jsonCoverageBucket struct {
	CoveredSources int `json:"covered_sources"`
	TargetCount    int `json:"target_count"`
}

type jsonCoverageTarget struct {
	TargetInstance string   `json:"target_instance"`
	TargetItemID   string   `json:"target_item_id"`
	CoveredSources []string `json:"covered_sources"`
	CoveredCount   int      `json:"covered_count"`
}

type jsonLooseStarPriority struct {
	SourceItemID string `json:"source_item_id"`
	TargetCount  int    `json:"target_count"`
}

func SolutionText(solution model.Solution, gridMask uint64) string {
	labels := placementLabels(solution.Placements)
	cellValues := map[model.Coord]string{}
	for _, placement := range solution.Placements {
		for _, cell := range placement.Cells {
			cellValues[cell] = labels[placement.InstanceID]
		}
	}

	var builder strings.Builder
	score := solution.Evaluation.Score
	fmt.Fprintf(&builder, "Score: crafts=%d, stars=%d, items=%d\n", score.CraftCount, score.StarCount, score.ItemCount)
	if len(score.PriorityCounts) > 0 {
		fmt.Fprintf(&builder, "Priority score: %s\n", formatInts(score.PriorityCounts))
	}
	if solution.Evaluation.StarCoverage != nil {
		fmt.Fprintf(&builder, "Coverage: %s\n", coverageSummary(*solution.Evaluation.StarCoverage))
		fmt.Fprintf(&builder, "Coverage sources: %s\n", strings.Join(solution.Evaluation.StarCoverage.Sources, ", "))
		if len(solution.Evaluation.StarCoverage.TargetItemIDs) > 0 {
			fmt.Fprintf(&builder, "Coverage targets filter: %s\n", strings.Join(solution.Evaluation.StarCoverage.TargetItemIDs, ", "))
		}
	}
	if len(solution.Evaluation.StarCoverageGroups) > 1 {
		builder.WriteString("Coverage groups:\n")
		for _, group := range solution.Evaluation.StarCoverageGroups {
			name := valueOrNone(group.Name)
			fmt.Fprintf(&builder, "  %s: %s (sources: %s)\n", name, coverageSummary(group), strings.Join(group.Sources, ", "))
			if len(group.TargetItemIDs) > 0 {
				fmt.Fprintf(&builder, "    targets: %s\n", strings.Join(group.TargetItemIDs, ", "))
			}
		}
	}
	if len(solution.Evaluation.LooseStarPriorities) > 0 {
		builder.WriteString("Loose stars: ")
		parts := make([]string, 0, len(solution.Evaluation.LooseStarPriorities))
		for _, priority := range solution.Evaluation.LooseStarPriorities {
			parts = append(parts, fmt.Sprintf("%s=%d", priority.SourceItemID, priority.TargetCount))
		}
		builder.WriteString(strings.Join(parts, ", "))
		builder.WriteByte('\n')
	}
	fmt.Fprintf(
		&builder,
		"Search: nodes=%d%s, limited=%t, refined=%t\n",
		solution.Search.NodesExplored,
		formatNodesPerSecond(solution.Search.NodesPerSecond),
		solution.Search.Limited,
		solution.Search.Refined,
	)
	if solution.Search.Limited {
		builder.WriteString("Warning: best found, not guaranteed optimal.\n")
	}
	if solution.Search.ParallelTasks > 0 {
		fmt.Fprintf(&builder, "Parallel search: tasks=%d, workers=%d\n", solution.Search.ParallelTasks, solution.Search.ParallelWorkersUsed)
	}
	if solution.Search.RefineMovesChecked > 0 {
		fmt.Fprintf(
			&builder,
			"Refine: moves=%d, improvements=%d%s\n",
			solution.Search.RefineMovesChecked,
			solution.Search.RefineImprovements,
			formatRefineDelta(solution.Search.RefineBestDelta),
		)
	}
	if len(solution.Search.CoverageCeiling) > 0 {
		fmt.Fprintf(&builder, "Coverage ceiling: %s\n", coverageBucketSummary(solution.Search.CoverageCeiling, len(solution.Search.CoverageSources)))
		fmt.Fprintf(&builder, "Coverage targets considered: %d\n", solution.Search.CoverageTargetCount)
		if solution.Search.CoverageCeilingReached {
			builder.WriteString("Coverage ceiling reached.\n")
		}
		if solution.Search.StoppedAfterCoverageCeiling {
			builder.WriteString("Stopped after coverage ceiling.\n")
		}
		if solution.Search.CoverageBoundChecks > 0 {
			fmt.Fprintf(&builder, "Coverage pruning: checks=%d, pruned=%d\n", solution.Search.CoverageBoundChecks, solution.Search.CoveragePrunedNodes)
		}
		if solution.Search.ExactBoundChecks > 0 {
			fmt.Fprintf(&builder, "Exact bounds: checks=%d, pruned=%d\n", solution.Search.ExactBoundChecks, solution.Search.ExactBoundPrunedNodes)
		}
		if solution.Search.CoverageSeedNodes > 0 {
			fmt.Fprintf(
				&builder,
				"Coverage seed: nodes=%d, candidates=%d%s\n",
				solution.Search.CoverageSeedNodes,
				solution.Search.CoverageSeedCandidates,
				formatSeedBest(solution.Search.CoverageSeedBest),
			)
		}
	}
	if solution.Search.RepairNodes > 0 {
		fmt.Fprintf(
			&builder,
			"Repair search: nodes=%d, iterations=%d, candidates=%d, improvements=%d%s\n",
			solution.Search.RepairNodes,
			solution.Search.RepairIterations,
			solution.Search.RepairCandidates,
			solution.Search.RepairImprovements,
			formatSeedBest(solution.Search.RepairBest),
		)
		if solution.Search.RepairParallelTasks > 0 {
			fmt.Fprintf(&builder, "Repair parallel: tasks=%d, workers=%d\n", solution.Search.RepairParallelTasks, solution.Search.RepairParallelWorkersUsed)
		}
	}
	builder.WriteString("Layout:\n")
	for row := 0; row < geometry.GridRows; row++ {
		var values []string
		for col := 0; col < geometry.GridCols; col++ {
			coord := model.Coord{Row: row, Col: col}
			bit := uint64(1) << geometry.CellIndex(coord)
			if gridMask&bit == 0 {
				values = append(values, "#")
			} else if value, ok := cellValues[coord]; ok {
				values = append(values, value)
			} else {
				values = append(values, ".")
			}
		}
		builder.WriteString(strings.Join(values, " "))
		builder.WriteByte('\n')
	}

	builder.WriteString("Items:\n")
	for _, placement := range solution.Placements {
		fmt.Fprintf(
			&builder,
			"  %s %s: rotation=%d, origin=%s, cells=%s\n",
			labels[placement.InstanceID],
			placement.InstanceID,
			placement.Rotation,
			placement.Origin,
			formatCoords(placement.Cells),
		)
	}

	builder.WriteString("Crafts:\n")
	if len(solution.Evaluation.Crafts) == 0 {
		builder.WriteString("  none\n")
	} else {
		for _, craft := range solution.Evaluation.Crafts {
			fmt.Fprintf(
				&builder,
				"  %s: anchor=%s, ingredients=%s\n",
				craft.RecipeResult,
				craft.AnchorInstance,
				strings.Join(craft.IngredientInstances, ", "),
			)
		}
	}

	builder.WriteString("Stars:\n")
	if len(solution.Evaluation.Stars) == 0 {
		builder.WriteString("  none\n")
	} else {
		for _, star := range solution.Evaluation.Stars {
			effect := ""
			if star.EffectText != "" {
				effect = " (" + star.EffectText + ")"
			}
			fmt.Fprintf(
				&builder,
				"  %s -> %s at %s%s\n",
				star.SourceInstance,
				star.TargetInstance,
				star.StarPosition,
				effect,
			)
		}
	}
	if solution.Evaluation.StarCoverage != nil {
		builder.WriteString("Coverage targets:\n")
		for _, target := range solution.Evaluation.StarCoverage.Targets {
			sources := "none"
			if len(target.CoveredSources) > 0 {
				sources = strings.Join(target.CoveredSources, ", ")
			}
			fmt.Fprintf(
				&builder,
				"  %s: %d/%d (%s)\n",
				target.TargetInstance,
				target.CoveredCount,
				len(solution.Evaluation.StarCoverage.Sources),
				sources,
			)
		}
	}

	return strings.TrimRight(builder.String(), "\n")
}

func SolutionsJSON(solutions []model.Solution) ([]byte, error) {
	out := make([]jsonSolution, 0, len(solutions))
	for _, solution := range solutions {
		out = append(out, toJSONSolution(solution))
	}
	return json.MarshalIndent(out, "", "  ")
}

func CatalogReviewText(catalog model.Catalog) string {
	recipesByResult := map[string][]model.Recipe{}
	for _, recipe := range catalog.Recipes {
		recipesByResult[recipe.Result] = append(recipesByResult[recipe.Result], recipe)
	}
	for result := range recipesByResult {
		sort.Slice(recipesByResult[result], func(i, j int) bool {
			return strings.Join(recipesByResult[result][i].Ingredients, "\x00") < strings.Join(recipesByResult[result][j].Ingredients, "\x00")
		})
	}

	var builder strings.Builder
	for idx, itemID := range sortedItemIDs(catalog.Items) {
		if idx > 0 {
			builder.WriteString("\n")
		}
		item := catalog.Items[itemID]
		fmt.Fprintf(&builder, "=== %s (%s) ===\n", item.Name, item.ID)
		fmt.Fprintf(&builder, "Types: %s\n", joinOrNone(item.Types, ", "))
		fmt.Fprintf(&builder, "Source: %s\n", valueOrNone(item.SourceURL))
		fmt.Fprintf(&builder, "Image URL: %s\n", valueOrNone(item.ImageURL))
		fmt.Fprintf(&builder, "Image path: %s\n", valueOrNone(item.ImagePath))
		if len(item.CountsAs) > 0 {
			fmt.Fprintf(&builder, "Counts as: %s\n", countsAsText(item.CountsAs))
		}
		fmt.Fprintf(&builder, "Shape: %s\n", shapeSummary(item.Shape))
		builder.WriteString("Grid (#=item, *=star, .=empty):\n")
		builder.WriteString(itemReviewGrid(item.Shape, item.Stars))
		builder.WriteByte('\n')
		builder.WriteString("Star filters:\n")
		if len(item.Stars) == 0 {
			builder.WriteString("  none\n")
		} else {
			for _, star := range item.Stars {
				fmt.Fprintf(&builder, "  %s -> %s", star.Offset, starTargetText(star))
				if star.EffectText != "" {
					fmt.Fprintf(&builder, " (%s)", star.EffectText)
				}
				builder.WriteByte('\n')
			}
		}
		builder.WriteString("Recipe:\n")
		recipes := recipesByResult[item.ID]
		if len(recipes) == 0 {
			builder.WriteString("  none\n")
		} else {
			for _, recipe := range recipes {
				fmt.Fprintf(&builder, "  %s = %s (anchor: %s)\n", recipe.Result, strings.Join(recipe.Ingredients, " + "), recipe.Anchor)
			}
		}
		if item.AbilityText != "" {
			fmt.Fprintf(&builder, "Ability preview: %s\n", compactPreview(item.AbilityText, 220))
		}
	}
	return strings.TrimRight(builder.String(), "\n")
}

func toJSONSolution(solution model.Solution) jsonSolution {
	placements := make([]jsonPlacement, 0, len(solution.Placements))
	for _, placement := range solution.Placements {
		cells := make([][]int, 0, len(placement.Cells))
		for _, cell := range placement.Cells {
			cells = append(cells, []int{cell.Row, cell.Col})
		}
		starPositions := make([][]int, 0, len(placement.StarPositions))
		for _, starPosition := range placement.StarPositions {
			starPositions = append(starPositions, []int{starPosition.Position.Row, starPosition.Position.Col})
		}
		placements = append(placements, jsonPlacement{
			InstanceID:    placement.InstanceID,
			ItemID:        placement.ItemID,
			Rotation:      placement.Rotation,
			Origin:        []int{placement.Origin.Row, placement.Origin.Col},
			Cells:         cells,
			StarPositions: starPositions,
		})
	}

	crafts := make([]jsonCraft, 0, len(solution.Evaluation.Crafts))
	for _, craft := range solution.Evaluation.Crafts {
		crafts = append(crafts, jsonCraft{
			Result:              craft.RecipeResult,
			AnchorInstance:      craft.AnchorInstance,
			IngredientInstances: craft.IngredientInstances,
		})
	}

	stars := make([]jsonStar, 0, len(solution.Evaluation.Stars))
	for _, star := range solution.Evaluation.Stars {
		stars = append(stars, jsonStar{
			SourceInstance: star.SourceInstance,
			TargetInstance: star.TargetInstance,
			StarPosition:   []int{star.StarPosition.Row, star.StarPosition.Col},
			EffectText:     star.EffectText,
		})
	}

	return jsonSolution{
		LayoutKey: solution.LayoutKey,
		Score: jsonScore{
			Crafts:         solution.Evaluation.Score.CraftCount,
			Stars:          solution.Evaluation.Score.StarCount,
			Items:          solution.Evaluation.Score.ItemCount,
			PriorityCounts: cloneInts(solution.Evaluation.Score.PriorityCounts),
		},
		Search: jsonSearch{
			NodesExplored:               solution.Search.NodesExplored,
			NodesPerSecond:              solution.Search.NodesPerSecond,
			SetupMS:                     solution.Search.SetupMS,
			SeedMS:                      solution.Search.SeedMS,
			RepairMS:                    solution.Search.RepairMS,
			SearchMS:                    solution.Search.SearchMS,
			RefineMS:                    solution.Search.RefineMS,
			Backend:                     solution.Search.Backend,
			ServerElapsedMS:             solution.Search.ServerElapsedMS,
			RemoteWorkers:               solution.Search.RemoteWorkers,
			MaxNodesApplied:             solution.Search.MaxNodesApplied,
			MaxNodesCapped:              solution.Search.MaxNodesCapped,
			Limited:                     solution.Search.Limited,
			Refined:                     solution.Search.Refined,
			CoverageSources:             cloneStrings(solution.Search.CoverageSources),
			CoverageTargetCount:         solution.Search.CoverageTargetCount,
			CoverageCeiling:             toJSONCoverageBuckets(solution.Search.CoverageCeiling),
			CoverageCeilingReached:      solution.Search.CoverageCeilingReached,
			CoverageBoundChecks:         solution.Search.CoverageBoundChecks,
			CoveragePrunedNodes:         solution.Search.CoveragePrunedNodes,
			ExactBoundChecks:            solution.Search.ExactBoundChecks,
			ExactBoundPrunedNodes:       solution.Search.ExactBoundPrunedNodes,
			CoverageSeedNodes:           solution.Search.CoverageSeedNodes,
			CoverageSeedCandidates:      solution.Search.CoverageSeedCandidates,
			CoverageSeedBest:            solution.Search.CoverageSeedBest,
			ParallelTasks:               solution.Search.ParallelTasks,
			ParallelWorkersUsed:         solution.Search.ParallelWorkersUsed,
			RefineMovesChecked:          solution.Search.RefineMovesChecked,
			RefineImprovements:          solution.Search.RefineImprovements,
			RefineBestDelta:             solution.Search.RefineBestDelta,
			RepairNodes:                 solution.Search.RepairNodes,
			RepairIterations:            solution.Search.RepairIterations,
			RepairImprovements:          solution.Search.RepairImprovements,
			RepairCandidates:            solution.Search.RepairCandidates,
			RepairBest:                  solution.Search.RepairBest,
			RepairParallelTasks:         solution.Search.RepairParallelTasks,
			RepairParallelWorkersUsed:   solution.Search.RepairParallelWorkersUsed,
			StoppedAfterCoverageCeiling: solution.Search.StoppedAfterCoverageCeiling,
		},
		Coverage:            toJSONCoverage(solution.Evaluation.StarCoverage),
		CoverageGroups:      toJSONCoverageGroups(solution.Evaluation.StarCoverageGroups),
		LooseStarPriorities: toJSONLooseStarPriorities(solution.Evaluation.LooseStarPriorities),
		Placements:          placements,
		Crafts:              crafts,
		Stars:               stars,
	}
}

func toJSONLooseStarPriorities(priorities []model.LooseStarPriority) []jsonLooseStarPriority {
	if len(priorities) == 0 {
		return nil
	}
	out := make([]jsonLooseStarPriority, 0, len(priorities))
	for _, priority := range priorities {
		out = append(out, jsonLooseStarPriority{
			SourceItemID: priority.SourceItemID,
			TargetCount:  priority.TargetCount,
		})
	}
	return out
}

func toJSONCoverage(coverage *model.StarCoverageBreakdown) *jsonCoverage {
	if coverage == nil {
		return nil
	}
	targets := make([]jsonCoverageTarget, 0, len(coverage.Targets))
	for _, target := range coverage.Targets {
		targets = append(targets, jsonCoverageTarget{
			TargetInstance: target.TargetInstance,
			TargetItemID:   target.TargetItemID,
			CoveredSources: cloneStrings(target.CoveredSources),
			CoveredCount:   target.CoveredCount,
		})
	}
	return &jsonCoverage{
		Name:          coverage.Name,
		Sources:       cloneStrings(coverage.Sources),
		TargetItemIDs: cloneStrings(coverage.TargetItemIDs),
		Buckets:       toJSONCoverageBuckets(coverage.Buckets),
		Targets:       targets,
		Summary:       coverageSummary(*coverage),
	}
}

func toJSONCoverageGroups(groups []model.StarCoverageBreakdown) []jsonCoverage {
	out := make([]jsonCoverage, 0, len(groups))
	for idx := range groups {
		coverage := toJSONCoverage(&groups[idx])
		if coverage != nil {
			out = append(out, *coverage)
		}
	}
	return out
}

func toJSONCoverageBuckets(buckets []model.StarCoverageBucket) []jsonCoverageBucket {
	out := make([]jsonCoverageBucket, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, jsonCoverageBucket{
			CoveredSources: bucket.CoveredSources,
			TargetCount:    bucket.TargetCount,
		})
	}
	return out
}

func cloneStrings(values []string) []string {
	out := make([]string, 0, len(values))
	return append(out, values...)
}

func cloneInts(values []int) []int {
	out := make([]int, 0, len(values))
	return append(out, values...)
}

func placementLabels(placements []model.Placement) map[string]string {
	alphabet := "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	labels := map[string]string{}
	for idx, placement := range placements {
		if idx < len(alphabet) {
			labels[placement.InstanceID] = string(alphabet[idx])
		} else {
			labels[placement.InstanceID] = "?"
		}
	}
	return labels
}

func formatCoords(coords []model.Coord) string {
	var parts []string
	for _, coord := range coords {
		parts = append(parts, coord.String())
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatInts(values []int) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func formatNodesPerSecond(value float64) string {
	if value <= 0 {
		return ""
	}
	return fmt.Sprintf(", nodes/sec=%.0f", value)
}

func formatSeedBest(value string) string {
	if value == "" {
		return ""
	}
	return ", best=" + value
}

func formatRefineDelta(value string) string {
	if value == "" {
		return ""
	}
	return ", " + value
}

func coverageSummary(coverage model.StarCoverageBreakdown) string {
	return coverageBucketSummary(coverage.Buckets, len(coverage.Sources))
}

func coverageBucketSummary(buckets []model.StarCoverageBucket, totalSources int) string {
	parts := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		parts = append(parts, fmt.Sprintf("%d/%d=%d", bucket.CoveredSources, totalSources, bucket.TargetCount))
	}
	return strings.Join(parts, ", ")
}

func sortedItemIDs(items map[string]model.Item) []string {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func joinOrNone(values []string, sep string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, sep)
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func shapeSummary(shape []model.Coord) string {
	if len(shape) == 0 {
		return "0 cells"
	}
	maxRow := shape[0].Row
	maxCol := shape[0].Col
	for _, cell := range shape {
		if cell.Row > maxRow {
			maxRow = cell.Row
		}
		if cell.Col > maxCol {
			maxCol = cell.Col
		}
	}
	cellWord := "cells"
	if len(shape) == 1 {
		cellWord = "cell"
	}
	return fmt.Sprintf("%d %s x %d %s box, %d %s", maxRow+1, pluralWord(maxRow+1, "row", "rows"), maxCol+1, pluralWord(maxCol+1, "col", "cols"), len(shape), cellWord)
}

func pluralWord(count int, singular string, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func itemReviewGrid(shape []model.Coord, stars []model.Star) string {
	minRow, minCol, maxRow, maxCol := itemReviewBounds(shape, stars)
	rows := maxRow - minRow + 1
	cols := maxCol - minCol + 1
	cells := make([][]string, rows)
	for row := range cells {
		cells[row] = make([]string, cols)
		for col := range cells[row] {
			cells[row][col] = "."
		}
	}
	for _, star := range stars {
		cells[star.Offset.Row-minRow][star.Offset.Col-minCol] = "*"
	}
	for _, cell := range shape {
		cells[cell.Row-minRow][cell.Col-minCol] = "#"
	}

	lines := make([]string, 0, rows)
	for _, row := range cells {
		lines = append(lines, strings.Join(row, " "))
	}
	return strings.Join(lines, "\n")
}

func itemReviewBounds(shape []model.Coord, stars []model.Star) (int, int, int, int) {
	minRow, minCol, maxRow, maxCol := 0, 0, 0, 0
	initialized := false
	visit := func(coord model.Coord) {
		if !initialized {
			minRow, minCol, maxRow, maxCol = coord.Row, coord.Col, coord.Row, coord.Col
			initialized = true
			return
		}
		if coord.Row < minRow {
			minRow = coord.Row
		}
		if coord.Col < minCol {
			minCol = coord.Col
		}
		if coord.Row > maxRow {
			maxRow = coord.Row
		}
		if coord.Col > maxCol {
			maxCol = coord.Col
		}
	}
	for _, cell := range shape {
		visit(cell)
	}
	for _, star := range stars {
		visit(star.Offset)
	}
	return minRow, minCol, maxRow, maxCol
}

func starTargetText(star model.Star) string {
	if star.RuleStatus == "unknown" {
		return "rule unresolved"
	}
	suffix := ""
	if star.ExcludeSourceItem {
		suffix = ", excluding same item"
	}
	if len(star.TargetTypes) == 0 && len(star.TargetItems) == 0 {
		return "any item" + suffix
	}
	var parts []string
	if len(star.TargetTypes) > 0 {
		parts = append(parts, "types: "+strings.Join(star.TargetTypes, ", "))
	}
	if len(star.TargetItems) > 0 {
		parts = append(parts, "items: "+strings.Join(star.TargetItems, ", "))
	}
	return strings.Join(parts, " OR ") + suffix
}

func countsAsText(aliases []model.ItemAlias) string {
	parts := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		parts = append(parts, fmt.Sprintf("%s x%d", alias.ItemID, alias.Count))
	}
	return strings.Join(parts, ", ")
}

func compactPreview(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return strings.TrimSpace(value[:limit-3]) + "..."
}
