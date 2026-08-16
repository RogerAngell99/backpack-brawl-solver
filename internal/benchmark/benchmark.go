package benchmark

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/render"
	"backpack-brawl-solver/internal/scenario"
	"backpack-brawl-solver/internal/solver"
)

var DefaultBudgets = []int64{10000, 50000, 100000, 250000, 500000, 1000000}

const (
	RepairSearchModeScenario = "scenario"
	RepairSearchModeOn       = "on"
	RepairSearchModeOff      = "off"
)

type RunConfig struct {
	CatalogPath      string
	ScenarioDir      string
	Budgets          []int64
	Repeat           int
	Workers          int
	Top              int
	RepairSearchMode string
}

type Report struct {
	GeneratedAt      string  `json:"generated_at"`
	CatalogPath      string  `json:"catalog_path"`
	ScenarioDir      string  `json:"scenario_dir"`
	Budgets          []int64 `json:"budgets"`
	Repeat           int     `json:"repeat"`
	Workers          int     `json:"workers"`
	Top              int     `json:"top"`
	RepairSearchMode string  `json:"repair_search_mode"`
	Runs             []Run   `json:"runs"`
}

type Run struct {
	Scenario            string             `json:"scenario"`
	ScenarioPath        string             `json:"scenario_path"`
	Budget              int64              `json:"budget"`
	Repeat              int                `json:"repeat"`
	RepairSearch        bool               `json:"repair_search"`
	ElapsedMS           int64              `json:"elapsed_ms"`
	NodesPerSecond      float64            `json:"nodes_per_second,omitempty"`
	Score               ScoreSummary       `json:"score"`
	LayoutKey           string             `json:"layout_key,omitempty"`
	Search              SearchSummary      `json:"search"`
	CoverageSummaries   []CoverageSummary  `json:"coverage_summaries,omitempty"`
	LooseStarPriorities []LooseStarSummary `json:"loose_star_priorities,omitempty"`
	Crafts              []string           `json:"crafts,omitempty"`
	Solution            json.RawMessage    `json:"solution,omitempty"`
	Error               string             `json:"error,omitempty"`
}

type ScoreSummary struct {
	PriorityCounts []int `json:"priority_counts,omitempty"`
	Crafts         int   `json:"crafts"`
	Stars          int   `json:"stars"`
	Items          int   `json:"items"`
}

type SearchSummary struct {
	NodesExplored               int64                   `json:"nodes_explored"`
	NodesPerSecond              float64                 `json:"nodes_per_second,omitempty"`
	Limited                     bool                    `json:"limited"`
	Refined                     bool                    `json:"refined"`
	CoverageSources             []string                `json:"coverage_sources,omitempty"`
	CoverageTargetCount         int                     `json:"coverage_target_count,omitempty"`
	CoverageCeiling             []CoverageBucketSummary `json:"coverage_ceiling,omitempty"`
	CoverageCeilingReached      bool                    `json:"coverage_ceiling_reached,omitempty"`
	CoverageBoundChecks         int64                   `json:"coverage_bound_checks,omitempty"`
	CoveragePrunedNodes         int64                   `json:"coverage_pruned_nodes,omitempty"`
	ExactBoundChecks            int64                   `json:"exact_bound_checks,omitempty"`
	ExactBoundPrunedNodes       int64                   `json:"exact_bound_pruned_nodes,omitempty"`
	CoverageSeedNodes           int64                   `json:"coverage_seed_nodes,omitempty"`
	CoverageSeedCandidates      int                     `json:"coverage_seed_candidates,omitempty"`
	CoverageSeedBest            string                  `json:"coverage_seed_best,omitempty"`
	ParallelTasks               int                     `json:"parallel_tasks,omitempty"`
	ParallelWorkersUsed         int                     `json:"parallel_workers_used,omitempty"`
	RefineMovesChecked          int64                   `json:"refine_moves_checked,omitempty"`
	RefineImprovements          int                     `json:"refine_improvements,omitempty"`
	RefineBestDelta             string                  `json:"refine_best_delta,omitempty"`
	RepairNodes                 int64                   `json:"repair_nodes,omitempty"`
	RepairIterations            int                     `json:"repair_iterations,omitempty"`
	RepairImprovements          int                     `json:"repair_improvements,omitempty"`
	RepairCandidates            int                     `json:"repair_candidates,omitempty"`
	RepairBest                  string                  `json:"repair_best,omitempty"`
	RepairParallelTasks         int                     `json:"repair_parallel_tasks,omitempty"`
	RepairParallelWorkersUsed   int                     `json:"repair_parallel_workers_used,omitempty"`
	StoppedAfterCoverageCeiling bool                    `json:"stopped_after_coverage_ceiling,omitempty"`
}

type CoverageBucketSummary struct {
	CoveredSources int `json:"covered_sources"`
	TargetCount    int `json:"target_count"`
}

type CoverageSummary struct {
	Name          string   `json:"name,omitempty"`
	Sources       []string `json:"sources"`
	TargetItemIDs []string `json:"target_item_ids,omitempty"`
	Summary       string   `json:"summary"`
}

type LooseStarSummary struct {
	SourceItemID string `json:"source_item_id"`
	TargetCount  int    `json:"target_count"`
}

func ParseBudgets(value string) ([]int64, error) {
	if strings.TrimSpace(value) == "" {
		return nil, fmt.Errorf("budgets cannot be empty")
	}
	parts := strings.Split(value, ",")
	budgets := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		budget, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid budget %q: %w", part, err)
		}
		if budget < 0 {
			return nil, fmt.Errorf("budget must be non-negative: %d", budget)
		}
		budgets = append(budgets, budget)
	}
	if len(budgets) == 0 {
		return nil, fmt.Errorf("budgets cannot be empty")
	}
	return budgets, nil
}

func FormatBudgets(budgets []int64) string {
	parts := make([]string, 0, len(budgets))
	for _, budget := range budgets {
		parts = append(parts, strconv.FormatInt(budget, 10))
	}
	return strings.Join(parts, ",")
}

func ValidateRepairSearchMode(value string) error {
	switch value {
	case RepairSearchModeScenario, RepairSearchModeOn, RepairSearchModeOff:
		return nil
	default:
		return fmt.Errorf("repair search mode must be one of %q, %q, %q", RepairSearchModeScenario, RepairSearchModeOn, RepairSearchModeOff)
	}
}

func RunScenarios(config RunConfig) (Report, error) {
	if config.CatalogPath == "" {
		config.CatalogPath = catalog.DefaultPath
	}
	if config.ScenarioDir == "" {
		config.ScenarioDir = filepath.Join("benchmarks", "scenarios")
	}
	if len(config.Budgets) == 0 {
		config.Budgets = append([]int64(nil), DefaultBudgets...)
	}
	if config.Repeat <= 0 {
		config.Repeat = 3
	}
	if config.Workers <= 0 {
		config.Workers = 1
	}
	if config.Top <= 0 {
		config.Top = 1
	}
	if config.RepairSearchMode == "" {
		config.RepairSearchMode = RepairSearchModeScenario
	}
	if err := ValidateRepairSearchMode(config.RepairSearchMode); err != nil {
		return Report{}, err
	}

	loadedCatalog, err := catalog.Load(config.CatalogPath)
	if err != nil {
		return Report{}, err
	}
	files, err := scenarioFiles(config.ScenarioDir)
	if err != nil {
		return Report{}, err
	}
	if len(files) == 0 {
		return Report{}, fmt.Errorf("no scenario JSON files found in %s", config.ScenarioDir)
	}

	report := Report{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		CatalogPath:      config.CatalogPath,
		ScenarioDir:      config.ScenarioDir,
		Budgets:          append([]int64(nil), config.Budgets...),
		Repeat:           config.Repeat,
		Workers:          config.Workers,
		Top:              config.Top,
		RepairSearchMode: config.RepairSearchMode,
	}
	for _, path := range files {
		loadedScenario, err := scenario.Load(path)
		if err != nil {
			return Report{}, err
		}
		scenarioCatalog, err := catalog.FilterForHeroes(loadedCatalog, loadedScenario.HeroFilter)
		if err != nil {
			return Report{}, fmt.Errorf("%s: %w", path, err)
		}
		if err := validateScenarioItems(scenarioCatalog, loadedScenario, path); err != nil {
			return Report{}, err
		}
		name := loadedScenario.Name
		if strings.TrimSpace(name) == "" {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		for _, budget := range config.Budgets {
			for repeatIndex := 0; repeatIndex < config.Repeat; repeatIndex++ {
				report.Runs = append(report.Runs, runScenario(scenarioCatalog, loadedScenario, name, path, budget, repeatIndex+1, config))
			}
		}
	}
	return report, nil
}

func LoadReport(path string) (Report, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Report{}, err
	}
	var report Report
	if err := json.Unmarshal(content, &report); err != nil {
		return Report{}, err
	}
	if len(report.Runs) == 0 {
		return Report{}, fmt.Errorf("%s contains no runs", path)
	}
	return report, nil
}

func scenarioFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".json" {
			continue
		}
		files = append(files, filepath.Join(dir, entry.Name()))
	}
	sort.Strings(files)
	return files, nil
}

func validateScenarioItems(loadedCatalog model.Catalog, loadedScenario scenario.Scenario, path string) error {
	var missing []string
	for itemID := range loadedScenario.Items {
		if _, ok := loadedCatalog.Items[itemID]; !ok {
			missing = append(missing, itemID)
		}
	}
	for _, group := range loadedScenario.CoverageGroups {
		for _, source := range group.Sources {
			if _, ok := loadedCatalog.Items[source]; !ok {
				missing = append(missing, source)
			}
		}
		for _, target := range group.Targets {
			if _, ok := loadedCatalog.Items[target]; !ok {
				missing = append(missing, target)
			}
		}
	}
	for _, priority := range loadedScenario.Priorities {
		kind, value, ok := strings.Cut(priority, ":")
		if !ok {
			continue
		}
		if kind != "star_source" {
			continue
		}
		if _, ok := loadedCatalog.Items[value]; !ok {
			missing = append(missing, value)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return fmt.Errorf("%s references unknown item(s): %s", path, strings.Join(uniqueStrings(missing), ", "))
}

func runScenario(loadedCatalog model.Catalog, loadedScenario scenario.Scenario, name string, path string, budget int64, repeat int, config RunConfig) Run {
	run := Run{
		Scenario:     name,
		ScenarioPath: path,
		Budget:       budget,
		Repeat:       repeat,
	}
	gridMask, err := scenarioGridMask(loadedScenario)
	if err != nil {
		run.Error = err.Error()
		return run
	}

	noSkips := false
	if loadedScenario.NoSkips != nil {
		noSkips = *loadedScenario.NoSkips
	}
	stopOnCoverageCeiling := false
	if loadedScenario.StopOnCoverageCeiling != nil {
		stopOnCoverageCeiling = *loadedScenario.StopOnCoverageCeiling
	}
	repairSearch := budget > 0
	if loadedScenario.RepairSearch != nil {
		repairSearch = *loadedScenario.RepairSearch
	}
	repairSearch = effectiveRepairSearch(config.RepairSearchMode, repairSearch, budget)
	run.RepairSearch = repairSearch

	startedAt := time.Now()
	solutions, err := solver.SolveLayout(loadedCatalog, loadedScenario.ItemIDs(), gridMask, solver.Config{
		TopN:                  config.Top,
		AllowSkips:            !noSkips,
		MaxNodes:              budget,
		Workers:               config.Workers,
		Priorities:            append([]string(nil), loadedScenario.Priorities...),
		CoverageGroups:        loadedScenario.ModelCoverageGroups(),
		StopOnCoverageCeiling: stopOnCoverageCeiling,
		RepairSearch:          repairSearch,
	})
	elapsed := time.Since(startedAt)
	run.ElapsedMS = elapsed.Milliseconds()
	if err != nil {
		run.Error = err.Error()
		return run
	}
	if len(solutions) == 0 {
		run.Error = "no solutions found"
		return run
	}

	for idx := range solutions {
		if elapsed > 0 && solutions[idx].Search.NodesExplored > 0 {
			solutions[idx].Search.NodesPerSecond = float64(solutions[idx].Search.NodesExplored) / elapsed.Seconds()
		}
	}
	best := solutions[0]
	run.NodesPerSecond = best.Search.NodesPerSecond
	run.Score = scoreSummary(best.Evaluation.Score)
	run.LayoutKey = best.LayoutKey
	run.Search = searchSummary(best.Search)
	run.CoverageSummaries = coverageSummaries(best.Evaluation)
	run.LooseStarPriorities = looseStarSummaries(best.Evaluation.LooseStarPriorities)
	run.Crafts = craftSummaries(best.Evaluation.Crafts)

	solutionJSON, err := solutionRawJSON(best)
	if err != nil {
		run.Error = err.Error()
		return run
	}
	run.Solution = solutionJSON
	return run
}

func effectiveRepairSearch(mode string, scenarioValue bool, budget int64) bool {
	if budget <= 0 {
		return false
	}
	switch mode {
	case RepairSearchModeOn:
		return true
	case RepairSearchModeOff:
		return false
	default:
		return scenarioValue
	}
}

func scenarioGridMask(loadedScenario scenario.Scenario) (uint64, error) {
	if len(loadedScenario.Grid) == 0 {
		return geometry.FullGridMask(), nil
	}
	return geometry.ParseGridText(loadedScenario.GridText())
}

func solutionRawJSON(solution model.Solution) (json.RawMessage, error) {
	content, err := render.SolutionsJSON([]model.Solution{solution})
	if err != nil {
		return nil, err
	}
	var payload []json.RawMessage
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, err
	}
	if len(payload) != 1 {
		return nil, fmt.Errorf("unexpected rendered solution count %d", len(payload))
	}
	return payload[0], nil
}

func scoreSummary(score model.Score) ScoreSummary {
	return ScoreSummary{
		PriorityCounts: append([]int(nil), score.PriorityCounts...),
		Crafts:         score.CraftCount,
		Stars:          score.StarCount,
		Items:          score.ItemCount,
	}
}

func searchSummary(search model.SearchStats) SearchSummary {
	return SearchSummary{
		NodesExplored:               search.NodesExplored,
		NodesPerSecond:              search.NodesPerSecond,
		Limited:                     search.Limited,
		Refined:                     search.Refined,
		CoverageSources:             append([]string(nil), search.CoverageSources...),
		CoverageTargetCount:         search.CoverageTargetCount,
		CoverageCeiling:             coverageBucketSummaries(search.CoverageCeiling),
		CoverageCeilingReached:      search.CoverageCeilingReached,
		CoverageBoundChecks:         search.CoverageBoundChecks,
		CoveragePrunedNodes:         search.CoveragePrunedNodes,
		ExactBoundChecks:            search.ExactBoundChecks,
		ExactBoundPrunedNodes:       search.ExactBoundPrunedNodes,
		CoverageSeedNodes:           search.CoverageSeedNodes,
		CoverageSeedCandidates:      search.CoverageSeedCandidates,
		CoverageSeedBest:            search.CoverageSeedBest,
		ParallelTasks:               search.ParallelTasks,
		ParallelWorkersUsed:         search.ParallelWorkersUsed,
		RefineMovesChecked:          search.RefineMovesChecked,
		RefineImprovements:          search.RefineImprovements,
		RefineBestDelta:             search.RefineBestDelta,
		RepairNodes:                 search.RepairNodes,
		RepairIterations:            search.RepairIterations,
		RepairImprovements:          search.RepairImprovements,
		RepairCandidates:            search.RepairCandidates,
		RepairBest:                  search.RepairBest,
		RepairParallelTasks:         search.RepairParallelTasks,
		RepairParallelWorkersUsed:   search.RepairParallelWorkersUsed,
		StoppedAfterCoverageCeiling: search.StoppedAfterCoverageCeiling,
	}
}

func coverageBucketSummaries(buckets []model.StarCoverageBucket) []CoverageBucketSummary {
	if len(buckets) == 0 {
		return nil
	}
	out := make([]CoverageBucketSummary, 0, len(buckets))
	for _, bucket := range buckets {
		out = append(out, CoverageBucketSummary{
			CoveredSources: bucket.CoveredSources,
			TargetCount:    bucket.TargetCount,
		})
	}
	return out
}

func coverageSummaries(evaluation model.Evaluation) []CoverageSummary {
	var summaries []CoverageSummary
	if len(evaluation.StarCoverageGroups) > 0 {
		for _, coverage := range evaluation.StarCoverageGroups {
			summaries = append(summaries, coverageSummary(coverage))
		}
		return summaries
	}
	if evaluation.StarCoverage != nil {
		summaries = append(summaries, coverageSummary(*evaluation.StarCoverage))
	}
	return summaries
}

func coverageSummary(coverage model.StarCoverageBreakdown) CoverageSummary {
	return CoverageSummary{
		Name:          coverage.Name,
		Sources:       append([]string(nil), coverage.Sources...),
		TargetItemIDs: append([]string(nil), coverage.TargetItemIDs...),
		Summary:       coverageBucketText(coverage.Buckets, len(coverage.Sources)),
	}
}

func coverageBucketText(buckets []model.StarCoverageBucket, totalSources int) string {
	parts := make([]string, 0, len(buckets))
	for _, bucket := range buckets {
		parts = append(parts, fmt.Sprintf("%d/%d=%d", bucket.CoveredSources, totalSources, bucket.TargetCount))
	}
	return strings.Join(parts, ", ")
}

func looseStarSummaries(priorities []model.LooseStarPriority) []LooseStarSummary {
	if len(priorities) == 0 {
		return nil
	}
	out := make([]LooseStarSummary, 0, len(priorities))
	for _, priority := range priorities {
		out = append(out, LooseStarSummary{
			SourceItemID: priority.SourceItemID,
			TargetCount:  priority.TargetCount,
		})
	}
	return out
}

func craftSummaries(crafts []model.CraftActivation) []string {
	if len(crafts) == 0 {
		return nil
	}
	out := make([]string, 0, len(crafts))
	for _, craft := range crafts {
		out = append(out, fmt.Sprintf("%s:%s:%s", craft.RecipeResult, craft.AnchorInstance, strings.Join(craft.IngredientInstances, "+")))
	}
	sort.Strings(out)
	return out
}

func compareScoreOnly(left ScoreSummary, right ScoreSummary) int {
	if compare := comparePriorityCounts(left.PriorityCounts, right.PriorityCounts); compare != 0 {
		return compare
	}
	if left.Crafts != right.Crafts {
		return left.Crafts - right.Crafts
	}
	if left.Stars != right.Stars {
		return left.Stars - right.Stars
	}
	if left.Items != right.Items {
		return left.Items - right.Items
	}
	return 0
}

func compareRunScore(current Run, baseline Run) int {
	if current.Error != "" && baseline.Error != "" {
		return 0
	}
	if current.Error != "" {
		return -1
	}
	if baseline.Error != "" {
		return 1
	}
	if compare := compareScoreOnly(current.Score, baseline.Score); compare != 0 {
		return compare
	}
	if current.LayoutKey < baseline.LayoutKey {
		return 1
	}
	if current.LayoutKey > baseline.LayoutKey {
		return -1
	}
	return 0
}

func comparePriorityCounts(left []int, right []int) int {
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for idx := 0; idx < maxLen; idx++ {
		leftValue := 0
		if idx < len(left) {
			leftValue = left[idx]
		}
		rightValue := 0
		if idx < len(right) {
			rightValue = right[idx]
		}
		if leftValue != rightValue {
			return leftValue - rightValue
		}
	}
	return 0
}

func medianInt64(values []int64) float64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]int64(nil), values...)
	sort.Slice(copied, func(i, j int) bool { return copied[i] < copied[j] })
	mid := len(copied) / 2
	if len(copied)%2 == 1 {
		return float64(copied[mid])
	}
	return float64(copied[mid-1]+copied[mid]) / 2
}

func medianFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	copied := append([]float64(nil), values...)
	sort.Float64s(copied)
	mid := len(copied) / 2
	if len(copied)%2 == 1 {
		return copied[mid]
	}
	return (copied[mid-1] + copied[mid]) / 2
}

func percent(count int, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) * 100 / float64(total)
}

func ratioPercent(value float64) string {
	if value == 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "0.0%"
	}
	return fmt.Sprintf("%.1f%%", value)
}

func scoreText(score ScoreSummary) string {
	priority := "[]"
	if len(score.PriorityCounts) > 0 {
		parts := make([]string, 0, len(score.PriorityCounts))
		for _, value := range score.PriorityCounts {
			parts = append(parts, strconv.Itoa(value))
		}
		priority = "[" + strings.Join(parts, "/") + "]"
	}
	return fmt.Sprintf("%s c%d s%d i%d", priority, score.Crafts, score.Stars, score.Items)
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	var result []string
	last := ""
	for idx, value := range values {
		if idx == 0 || value != last {
			result = append(result, value)
			last = value
		}
	}
	return result
}
