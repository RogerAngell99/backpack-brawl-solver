package benchmark

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

type Comparison struct {
	Rows            []ComparisonRow
	Summaries       []ComparisonSummary
	Wins            int
	Losses          int
	Ties            int
	ScoreLosses     int
	TimeRegressions int
}

type ComparisonRow struct {
	Scenario      string
	Budget        int64
	Repeat        int
	Status        string
	Baseline      ScoreSummary
	Current       ScoreSummary
	BaselineMS    int64
	CurrentMS     int64
	BaselineNPS   float64
	CurrentNPS    float64
	ScoreLoss     bool
	LayoutOnly    bool
	BaselineError string
	CurrentError  string
}

type ComparisonSummary struct {
	Scenario                   string
	Budget                     int64
	Wins                       int
	Losses                     int
	Ties                       int
	BaselineMedianElapsedMS    float64
	CurrentMedianElapsedMS     float64
	BaselineMedianNodesPerSec  float64
	CurrentMedianNodesPerSec   float64
	BaselineCeilingRate        float64
	CurrentCeilingRate         float64
	CurrentBestPriorityCounts  []int
	CurrentWorstPriorityCounts []int
	TimeRegression             bool
}

func CompareReports(baseline Report, current Report) (Comparison, error) {
	baselineRuns := map[string]Run{}
	for _, run := range baseline.Runs {
		key := runKey(run)
		if _, exists := baselineRuns[key]; exists {
			return Comparison{}, fmt.Errorf("baseline contains duplicate run key %s", key)
		}
		baselineRuns[key] = run
	}

	currentRuns := map[string]Run{}
	for _, run := range current.Runs {
		key := runKey(run)
		if _, exists := currentRuns[key]; exists {
			return Comparison{}, fmt.Errorf("current contains duplicate run key %s", key)
		}
		currentRuns[key] = run
	}
	for key := range baselineRuns {
		if _, ok := currentRuns[key]; !ok {
			return Comparison{}, fmt.Errorf("current report is missing run key %s", key)
		}
	}
	for key := range currentRuns {
		if _, ok := baselineRuns[key]; !ok {
			return Comparison{}, fmt.Errorf("baseline report is missing run key %s", key)
		}
	}

	keys := make([]string, 0, len(currentRuns))
	for key := range currentRuns {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var comparison Comparison
	summaryData := map[string]*summaryAccumulator{}
	for _, key := range keys {
		base := baselineRuns[key]
		next := currentRuns[key]
		if base.Search.ExecutionFingerprint != "" && next.Search.ExecutionFingerprint != "" {
			if base.Search.ExecutionFingerprint != next.Search.ExecutionFingerprint {
				return Comparison{}, fmt.Errorf("run key %s has different execution fingerprints", key)
			}
		} else if base.Search.ConfigFingerprint != next.Search.ConfigFingerprint {
			return Comparison{}, fmt.Errorf("run key %s has different configuration fingerprints", key)
		}
		row := compareRun(base, next)
		comparison.Rows = append(comparison.Rows, row)
		switch row.Status {
		case "WIN":
			comparison.Wins++
		case "LOSS":
			comparison.Losses++
		default:
			comparison.Ties++
		}
		if row.ScoreLoss {
			comparison.ScoreLosses++
		}

		summaryKey := fmt.Sprintf("%s|%020d", next.Scenario, next.Budget)
		acc := summaryData[summaryKey]
		if acc == nil {
			acc = &summaryAccumulator{scenario: next.Scenario, budget: next.Budget}
			summaryData[summaryKey] = acc
		}
		acc.add(row, base, next)
	}

	summaryKeys := make([]string, 0, len(summaryData))
	for key := range summaryData {
		summaryKeys = append(summaryKeys, key)
	}
	sort.Strings(summaryKeys)
	for _, key := range summaryKeys {
		summary := summaryData[key].summary()
		if summary.TimeRegression {
			comparison.TimeRegressions++
		}
		comparison.Summaries = append(comparison.Summaries, summary)
	}
	return comparison, nil
}

func FormatComparison(writer io.Writer, comparison Comparison) {
	fmt.Fprintf(
		writer,
		"Pairs: %d | wins=%d losses=%d ties=%d | score losses=%d | time regressions=%d\n\n",
		len(comparison.Rows),
		comparison.Wins,
		comparison.Losses,
		comparison.Ties,
		comparison.ScoreLosses,
		comparison.TimeRegressions,
	)

	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Scenario\tBudget\tW/L/T\tElapsed ms base/current\tNodes/sec base/current\tCeiling base/current\tBest priority\tWorst priority")
	for _, summary := range comparison.Summaries {
		fmt.Fprintf(
			tw,
			"%s\t%d\t%d/%d/%d\t%.0f/%.0f\t%.0f/%.0f\t%s/%s\t%s\t%s\n",
			summary.Scenario,
			summary.Budget,
			summary.Wins,
			summary.Losses,
			summary.Ties,
			summary.BaselineMedianElapsedMS,
			summary.CurrentMedianElapsedMS,
			summary.BaselineMedianNodesPerSec,
			summary.CurrentMedianNodesPerSec,
			ratioPercent(summary.BaselineCeilingRate),
			ratioPercent(summary.CurrentCeilingRate),
			intSliceText(summary.CurrentBestPriorityCounts),
			intSliceText(summary.CurrentWorstPriorityCounts),
		)
	}
	_ = tw.Flush()

	var regressions []ComparisonRow
	for _, row := range comparison.Rows {
		if row.ScoreLoss {
			regressions = append(regressions, row)
		}
	}
	if len(regressions) > 0 {
		fmt.Fprintln(writer, "\nScore regressions:")
		for _, row := range regressions {
			fmt.Fprintf(
				writer,
				"- %s budget=%d repeat=%d baseline=%s current=%s\n",
				row.Scenario,
				row.Budget,
				row.Repeat,
				scoreText(row.Baseline),
				scoreText(row.Current),
			)
		}
	}

	var timeRegressions []ComparisonSummary
	for _, summary := range comparison.Summaries {
		if summary.TimeRegression {
			timeRegressions = append(timeRegressions, summary)
		}
	}
	if len(timeRegressions) > 0 {
		fmt.Fprintln(writer, "\nTime regressions on score ties (>10% median elapsed):")
		for _, summary := range timeRegressions {
			fmt.Fprintf(
				writer,
				"- %s budget=%d baseline=%.0fms current=%.0fms\n",
				summary.Scenario,
				summary.Budget,
				summary.BaselineMedianElapsedMS,
				summary.CurrentMedianElapsedMS,
			)
		}
	}
}

func compareRun(baseline Run, current Run) ComparisonRow {
	row := ComparisonRow{
		Scenario:      current.Scenario,
		Budget:        current.Budget,
		Repeat:        current.Repeat,
		Baseline:      baseline.Score,
		Current:       current.Score,
		BaselineMS:    baseline.ElapsedMS,
		CurrentMS:     current.ElapsedMS,
		BaselineNPS:   baseline.NodesPerSecond,
		CurrentNPS:    current.NodesPerSecond,
		BaselineError: baseline.Error,
		CurrentError:  current.Error,
	}
	scoreCompare := compareRunScore(current, baseline)
	switch {
	case scoreCompare > 0:
		row.Status = "WIN"
	case scoreCompare < 0:
		row.Status = "LOSS"
	default:
		row.Status = "TIE"
	}
	scoreOnlyCompare := compareScoreOnly(current.Score, baseline.Score)
	row.ScoreLoss = scoreOnlyCompare < 0 || (current.Error != "" && baseline.Error == "")
	row.LayoutOnly = scoreOnlyCompare == 0 && row.Status != "TIE"
	return row
}

func runKey(run Run) string {
	return fmt.Sprintf("%s|%020d|%020d", run.Scenario, run.Budget, run.Repeat)
}

type summaryAccumulator struct {
	scenario        string
	budget          int64
	wins            int
	losses          int
	ties            int
	baselineElapsed []int64
	currentElapsed  []int64
	baselineNodes   []float64
	currentNodes    []float64
	baselineCeiling int
	currentCeiling  int
	ceilingTotal    int
	currentScores   []ScoreSummary
}

func (acc *summaryAccumulator) add(row ComparisonRow, baseline Run, current Run) {
	switch row.Status {
	case "WIN":
		acc.wins++
	case "LOSS":
		acc.losses++
	default:
		acc.ties++
	}
	acc.baselineElapsed = append(acc.baselineElapsed, baseline.ElapsedMS)
	acc.currentElapsed = append(acc.currentElapsed, current.ElapsedMS)
	acc.baselineNodes = append(acc.baselineNodes, baseline.NodesPerSecond)
	acc.currentNodes = append(acc.currentNodes, current.NodesPerSecond)
	if baseline.Search.CoverageCeilingReached {
		acc.baselineCeiling++
	}
	if current.Search.CoverageCeilingReached {
		acc.currentCeiling++
	}
	acc.ceilingTotal++
	acc.currentScores = append(acc.currentScores, current.Score)
}

func (acc *summaryAccumulator) summary() ComparisonSummary {
	currentBest, currentWorst := bestWorstPriorityCounts(acc.currentScores)
	baselineMedianElapsed := medianInt64(acc.baselineElapsed)
	currentMedianElapsed := medianInt64(acc.currentElapsed)
	timeRegression := acc.losses == 0 && acc.wins == 0 && baselineMedianElapsed > 0 && currentMedianElapsed > baselineMedianElapsed*1.10
	return ComparisonSummary{
		Scenario:                   acc.scenario,
		Budget:                     acc.budget,
		Wins:                       acc.wins,
		Losses:                     acc.losses,
		Ties:                       acc.ties,
		BaselineMedianElapsedMS:    baselineMedianElapsed,
		CurrentMedianElapsedMS:     currentMedianElapsed,
		BaselineMedianNodesPerSec:  medianFloat64(acc.baselineNodes),
		CurrentMedianNodesPerSec:   medianFloat64(acc.currentNodes),
		BaselineCeilingRate:        percent(acc.baselineCeiling, acc.ceilingTotal),
		CurrentCeilingRate:         percent(acc.currentCeiling, acc.ceilingTotal),
		CurrentBestPriorityCounts:  append([]int(nil), currentBest...),
		CurrentWorstPriorityCounts: append([]int(nil), currentWorst...),
		TimeRegression:             timeRegression,
	}
}

func bestWorstPriorityCounts(scores []ScoreSummary) ([]int, []int) {
	if len(scores) == 0 {
		return nil, nil
	}
	best := append([]int(nil), scores[0].PriorityCounts...)
	worst := append([]int(nil), scores[0].PriorityCounts...)
	for _, score := range scores[1:] {
		if comparePriorityCounts(score.PriorityCounts, best) > 0 {
			best = append([]int(nil), score.PriorityCounts...)
		}
		if comparePriorityCounts(score.PriorityCounts, worst) < 0 {
			worst = append([]int(nil), score.PriorityCounts...)
		}
	}
	return best, worst
}

func intSliceText(values []int) string {
	if len(values) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, fmt.Sprintf("%d", value))
	}
	return "[" + strings.Join(parts, "/") + "]"
}
