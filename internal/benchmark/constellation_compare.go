package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"text/tabwriter"

	"backpack-brawl-solver/internal/model"
)

const (
	constellationComparisonModeVariant  = "v4_to_"
	constellationComparisonModeParallel = "v5_parallel"

	v5BehaviorNotEligible        = "not_eligible"
	v5BehaviorEligibleNoFrontier = "eligible_no_frontier"
	v5BehaviorFrontierSelected   = "frontier_selected"
	v5BehaviorDiagnosticsMissing = "diagnostics_unavailable"
	v5BehaviorNotApplicable      = "not_applicable"

	regressionNotApplicable = "not_applicable"
	regressionUnattributed  = "unattributed"
	regressionFrontier      = "frontier_replacement_effect"
	regressionPacking       = "packing_policy_effect_on_same_root"
	regressionMixed         = "mixed_effects"
)

// ConstellationExperimentComparison compares intentionally different V4/V5
// policies, or V5 runs across worker counts. It never weakens CompareReports,
// whose same-fingerprint requirement remains appropriate for baselines.
type ConstellationExperimentComparison struct {
	Mode        string                       `json:"mode"`
	PolicyDelta ConstellationPolicyDelta     `json:"policy_delta"`
	Rows        []ConstellationExperimentRow `json:"rows"`
	ScoreWins   int                          `json:"score_wins"`
	ScoreLosses int                          `json:"score_losses"`
	ScoreTies   int                          `json:"score_ties"`
}

type ConstellationPolicyDelta struct {
	BaselineVariant      string `json:"baseline_variant"`
	CurrentVariant       string `json:"current_variant"`
	BaselineWorkers      int    `json:"baseline_workers"`
	CurrentWorkers       int    `json:"current_workers"`
	BaselinePackingBeam  int    `json:"baseline_packing_beam"`
	CurrentPackingBeam   int    `json:"current_packing_beam"`
	BaselineStrategy     string `json:"baseline_strategy"`
	CurrentStrategy      string `json:"current_strategy"`
	BaselineMaxRoots     int    `json:"baseline_max_roots"`
	CurrentMaxRoots      int    `json:"current_max_roots"`
	BaselineShareBps     int64  `json:"baseline_share_bps"`
	CurrentShareBps      int64  `json:"current_share_bps"`
	ExecutionDiffAllowed bool   `json:"execution_fingerprint_diff_allowed"`
}

type ConstellationExperimentRow struct {
	Scenario                        string                          `json:"scenario"`
	Budget                          int64                           `json:"budget"`
	Repeat                          int                             `json:"repeat"`
	ScoreCmp                        int                             `json:"score_cmp"`
	ScoreStatus                     string                          `json:"score_status"`
	SameHash                        bool                            `json:"same_hash"`
	BaselineHash                    string                          `json:"baseline_hash,omitempty"`
	CurrentHash                     string                          `json:"current_hash,omitempty"`
	BaselineError                   string                          `json:"baseline_error,omitempty"`
	CurrentError                    string                          `json:"current_error,omitempty"`
	NormalNodeDelta                 int64                           `json:"normal_node_delta"`
	NormalNodeRatio                 float64                         `json:"normal_node_ratio,omitempty"`
	FirstCompletionNodeDelta        *int64                          `json:"first_completion_node_delta,omitempty"`
	V5BehaviorClass                 string                          `json:"v5_behavior_class"`
	RegressionAttribution           string                          `json:"regression_attribution"`
	SemanticPolicyFingerprint       string                          `json:"semantic_policy_fingerprint,omitempty"`
	ExecutionFingerprintChanged     bool                            `json:"execution_fingerprint_changed"`
	BaselineConstellationRootWinner ConstellationRootWinner         `json:"baseline_constellation_root_winner,omitempty"`
	CurrentConstellationRootWinner  ConstellationRootWinner         `json:"current_constellation_root_winner,omitempty"`
	RootPairs                       []ConstellationRootPair         `json:"root_pairs,omitempty"`
	DownstreamBudgetDeltaByPhase    []ConstellationPhaseBudgetDelta `json:"downstream_budget_delta_by_phase,omitempty"`
}

type ConstellationPhaseBudgetDelta struct {
	Phase            string `json:"phase"`
	BaselineCharged  int64  `json:"baseline_charged"`
	CurrentCharged   int64  `json:"current_charged"`
	ChargedDelta     int64  `json:"charged_delta"`
	BaselineReturned int64  `json:"baseline_returned"`
	CurrentReturned  int64  `json:"current_returned"`
	ReturnedDelta    int64  `json:"returned_delta"`
}

type ConstellationRootWinner struct {
	RootID string       `json:"root_id,omitempty"`
	Score  *model.Score `json:"score,omitempty"`
	Hash   string       `json:"hash,omitempty"`
}

type ConstellationRootPair struct {
	Kind             string       `json:"kind"`
	BaselineRootID   string       `json:"baseline_root_id,omitempty"`
	CurrentRootID    string       `json:"current_root_id,omitempty"`
	BaselineScore    *model.Score `json:"baseline_score,omitempty"`
	CurrentScore     *model.Score `json:"current_score,omitempty"`
	ScoreCmp         int          `json:"score_cmp"`
	BaselineComplete bool         `json:"baseline_complete"`
	CurrentComplete  bool         `json:"current_complete"`
}

func CompareConstellationExperimentReports(baseline Report, current Report) (ConstellationExperimentComparison, error) {
	mode, err := validateConstellationExperimentReports(baseline, current)
	if err != nil {
		return ConstellationExperimentComparison{}, err
	}
	baselineRuns, err := runsByKey(baseline.Runs)
	if err != nil {
		return ConstellationExperimentComparison{}, fmt.Errorf("baseline %w", err)
	}
	currentRuns, err := runsByKey(current.Runs)
	if err != nil {
		return ConstellationExperimentComparison{}, fmt.Errorf("current %w", err)
	}
	if err := sameRunKeys(baselineRuns, currentRuns); err != nil {
		return ConstellationExperimentComparison{}, err
	}
	keys := make([]string, 0, len(currentRuns))
	for key := range currentRuns {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	comparison := ConstellationExperimentComparison{
		Mode:        mode,
		PolicyDelta: constellationPolicyDelta(baseline, current),
		Rows:        make([]ConstellationExperimentRow, 0, len(keys)),
	}
	for _, key := range keys {
		base := baselineRuns[key]
		next := currentRuns[key]
		if err := validateConstellationRunInvariants(base, next, mode); err != nil {
			return ConstellationExperimentComparison{}, fmt.Errorf("run key %s: %w", key, err)
		}
		row := compareConstellationExperimentRun(base, next, mode)
		comparison.Rows = append(comparison.Rows, row)
		switch row.ScoreStatus {
		case "WIN":
			comparison.ScoreWins++
		case "LOSS":
			comparison.ScoreLosses++
		default:
			comparison.ScoreTies++
		}
	}
	return comparison, nil
}

func validateConstellationExperimentReports(baseline Report, current Report) (string, error) {
	if baseline.CatalogSHA256 != current.CatalogSHA256 {
		return "", fmt.Errorf("catalog SHA-256 differs")
	}
	if !reflect.DeepEqual(baseline.Budgets, current.Budgets) || baseline.Repeat != current.Repeat || baseline.Top != current.Top || baseline.RepairSearchMode != current.RepairSearchMode || baseline.PlateauVariant != current.PlateauVariant || baseline.Diagnostic != current.Diagnostic {
		return "", fmt.Errorf("benchmark invariants differ")
	}
	switch {
	case baseline.ConstellationSeedVariant == "v4" && (current.ConstellationSeedVariant == "v5" || current.ConstellationSeedVariant == "v5.1" || current.ConstellationSeedVariant == "general-search-v1"):
		if baseline.Workers != current.Workers {
			return "", fmt.Errorf("V4-to-V5 comparison requires equal worker counts")
		}
		return constellationComparisonModeVariant + current.ConstellationSeedVariant, nil
	case (baseline.ConstellationSeedVariant == "v5" || baseline.ConstellationSeedVariant == "v5.1") && baseline.ConstellationSeedVariant == current.ConstellationSeedVariant:
		if baseline.Workers == current.Workers {
			return "", fmt.Errorf("parallel V5 comparison requires distinct worker counts")
		}
		return constellationComparisonModeParallel, nil
	default:
		return "", fmt.Errorf("comparison requires V4 baseline with an approved constellation experiment, or matching V5-family runs with distinct workers")
	}
}

func runsByKey(runs []Run) (map[string]Run, error) {
	indexed := make(map[string]Run, len(runs))
	for _, run := range runs {
		key := runKey(run)
		if _, exists := indexed[key]; exists {
			return nil, fmt.Errorf("contains duplicate run key %s", key)
		}
		indexed[key] = run
	}
	return indexed, nil
}

func sameRunKeys(baseline map[string]Run, current map[string]Run) error {
	for key := range baseline {
		if _, exists := current[key]; !exists {
			return fmt.Errorf("current report is missing run key %s", key)
		}
	}
	for key := range current {
		if _, exists := baseline[key]; !exists {
			return fmt.Errorf("baseline report is missing run key %s", key)
		}
	}
	return nil
}

func validateConstellationRunInvariants(baseline Run, current Run, mode string) error {
	if baseline.ScenarioPath != current.ScenarioPath || baseline.PrioritySemantics != current.PrioritySemantics || !reflect.DeepEqual(baseline.Priorities, current.Priorities) || baseline.NoSkips != current.NoSkips || baseline.RepairSearch != current.RepairSearch || baseline.PlateauVariant != current.PlateauVariant || baseline.StopOnCoverageCeiling != current.StopOnCoverageCeiling || baseline.StopOnPriorityCeiling != current.StopOnPriorityCeiling {
		return fmt.Errorf("scenario invariants differ")
	}
	if mode == constellationComparisonModeParallel {
		if semanticPolicyFingerprint(baseline) != semanticPolicyFingerprint(current) {
			return fmt.Errorf("semantic policy fingerprint differs")
		}
	}
	return nil
}

func compareConstellationExperimentRun(baseline Run, current Run, mode string) ConstellationExperimentRow {
	compare := compareExperimentScore(current, baseline)
	row := ConstellationExperimentRow{
		Scenario:                        current.Scenario,
		Budget:                          current.Budget,
		Repeat:                          current.Repeat,
		ScoreCmp:                        scoreCompareSign(compare),
		ScoreStatus:                     scoreCompareStatus(compare),
		SameHash:                        baseline.CanonicalLayoutHash != "" && baseline.CanonicalLayoutHash == current.CanonicalLayoutHash,
		BaselineHash:                    baseline.CanonicalLayoutHash,
		CurrentHash:                     current.CanonicalLayoutHash,
		BaselineError:                   baseline.Error,
		CurrentError:                    current.Error,
		NormalNodeDelta:                 normalNodes(current) - normalNodes(baseline),
		V5BehaviorClass:                 v5BehaviorClass(current),
		RegressionAttribution:           regressionNotApplicable,
		ExecutionFingerprintChanged:     baseline.Search.ExecutionFingerprint != current.Search.ExecutionFingerprint,
		BaselineConstellationRootWinner: constellationRootWinner(baseline.Search.ConstellationSeedDiagnostics),
		CurrentConstellationRootWinner:  constellationRootWinner(current.Search.ConstellationSeedDiagnostics),
	}
	if baselineNodes := normalNodes(baseline); baselineNodes > 0 {
		row.NormalNodeRatio = float64(normalNodes(current)) / float64(baselineNodes)
	}
	if baseline.Search.FirstCompleteNodes > 0 && current.Search.FirstCompleteNodes > 0 {
		delta := current.Search.FirstCompleteNodes - baseline.Search.FirstCompleteNodes
		row.FirstCompletionNodeDelta = &delta
	}
	if mode == constellationComparisonModeParallel {
		row.SemanticPolicyFingerprint = semanticPolicyFingerprint(current)
	}
	row.RootPairs = constellationRootPairs(baseline.Search.ConstellationSeedDiagnostics, current.Search.ConstellationSeedDiagnostics)
	if current.ConstellationSeedVariant == "v5.1" {
		row.DownstreamBudgetDeltaByPhase = downstreamBudgetDeltaByPhase(baseline.Search.PhaseWork, current.Search.PhaseWork)
	}
	if row.ScoreStatus == "LOSS" {
		row.RegressionAttribution = regressionAttribution(row.RootPairs)
	}
	return row
}

func downstreamBudgetDeltaByPhase(baseline []model.SearchPhaseWork, current []model.SearchPhaseWork) []ConstellationPhaseBudgetDelta {
	baselineByPhase := make(map[string]model.SearchPhaseWork, len(baseline))
	for _, phase := range baseline {
		baselineByPhase[phase.Phase] = phase
	}
	result := make([]ConstellationPhaseBudgetDelta, 0)
	for _, phase := range current {
		if phase.Phase == "coverage_seed" || phase.Phase == "packing_seed" || phase.Phase == "constellation_seed_v1" {
			continue
		}
		previous, exists := baselineByPhase[phase.Phase]
		if !exists {
			continue
		}
		delta := ConstellationPhaseBudgetDelta{
			Phase:            phase.Phase,
			BaselineCharged:  previous.ChargedNodes,
			CurrentCharged:   phase.ChargedNodes,
			ChargedDelta:     phase.ChargedNodes - previous.ChargedNodes,
			BaselineReturned: previous.NodesReturned,
			CurrentReturned:  phase.NodesReturned,
			ReturnedDelta:    phase.NodesReturned - previous.NodesReturned,
		}
		if delta.ChargedDelta != 0 || delta.ReturnedDelta != 0 {
			result = append(result, delta)
		}
	}
	return result
}

func constellationPolicyDelta(baseline Report, current Report) ConstellationPolicyDelta {
	base := firstConstellationSettings(baseline.Runs)
	next := firstConstellationSettings(current.Runs)
	return ConstellationPolicyDelta{
		BaselineVariant:      baseline.ConstellationSeedVariant,
		CurrentVariant:       current.ConstellationSeedVariant,
		BaselineWorkers:      baseline.Workers,
		CurrentWorkers:       current.Workers,
		BaselinePackingBeam:  base.ConstellationSeedPackingBeamWidth,
		CurrentPackingBeam:   next.ConstellationSeedPackingBeamWidth,
		BaselineStrategy:     base.ConstellationSeedPackingStrategy,
		CurrentStrategy:      next.ConstellationSeedPackingStrategy,
		BaselineMaxRoots:     base.ConstellationSeedMaxSkeletons,
		CurrentMaxRoots:      next.ConstellationSeedMaxSkeletons,
		BaselineShareBps:     base.ConstellationSeedShareBps,
		CurrentShareBps:      next.ConstellationSeedShareBps,
		ExecutionDiffAllowed: baseline.ConstellationSeedVariant != current.ConstellationSeedVariant || baseline.Workers != current.Workers,
	}
}

func firstConstellationSettings(runs []Run) solverSettings {
	for _, run := range runs {
		settings := run.SolverSettings
		if settings.ConstellationSeedVersion == "" && settings.ConstellationSeedNodeBudget == 0 {
			continue
		}
		return solverSettings{
			ConstellationSeedPackingBeamWidth: settings.ConstellationSeedPackingBeamWidth,
			ConstellationSeedPackingStrategy:  settings.ConstellationSeedPackingStrategy,
			ConstellationSeedMaxSkeletons:     settings.ConstellationSeedMaxSkeletons,
			ConstellationSeedShareBps:         settings.ConstellationSeedShareBps,
		}
	}
	if len(runs) == 0 {
		return solverSettings{}
	}
	settings := runs[0].SolverSettings
	return solverSettings{ConstellationSeedPackingBeamWidth: settings.ConstellationSeedPackingBeamWidth, ConstellationSeedPackingStrategy: settings.ConstellationSeedPackingStrategy, ConstellationSeedMaxSkeletons: settings.ConstellationSeedMaxSkeletons, ConstellationSeedShareBps: settings.ConstellationSeedShareBps}
}

type solverSettings struct {
	ConstellationSeedPackingBeamWidth int
	ConstellationSeedPackingStrategy  string
	ConstellationSeedMaxSkeletons     int
	ConstellationSeedShareBps         int64
}

func normalNodes(run Run) int64 {
	if run.Search.NormalBudgetConsumed > 0 {
		return run.Search.NormalBudgetConsumed
	}
	return run.Search.NodesExplored
}

func compareExperimentScore(current Run, baseline Run) int {
	if current.Error != "" && baseline.Error != "" {
		return 0
	}
	if current.Error != "" {
		return -1
	}
	if baseline.Error != "" {
		return 1
	}
	return model.CompareScores(scoreSummaryModelScore(current.Score), scoreSummaryModelScore(baseline.Score))
}

func scoreCompareSign(compare int) int {
	switch {
	case compare > 0:
		return 1
	case compare < 0:
		return -1
	default:
		return 0
	}
}

func scoreCompareStatus(compare int) string {
	switch {
	case compare > 0:
		return "WIN"
	case compare < 0:
		return "LOSS"
	default:
		return "TIE"
	}
}

func v5BehaviorClass(run Run) string {
	if run.ConstellationSeedVariant == "general-search-v1" {
		return v5BehaviorNotApplicable
	}
	diagnostics := run.Search.ConstellationSeedDiagnostics
	if diagnostics == nil {
		return v5BehaviorNotEligible
	}
	if diagnostics.Version != "v5" && diagnostics.Version != "v5.1" {
		return v5BehaviorDiagnosticsMissing
	}
	if diagnostics.RelaxationFrontierSelected {
		return v5BehaviorFrontierSelected
	}
	return v5BehaviorEligibleNoFrontier
}

func constellationRootWinner(diagnostics *model.ConstellationSeedDiagnostics) ConstellationRootWinner {
	if diagnostics == nil || diagnostics.ConstellationRootWinnerID == "" {
		return ConstellationRootWinner{}
	}
	result := ConstellationRootWinner{RootID: diagnostics.ConstellationRootWinnerID, Hash: diagnostics.ConstellationRootWinnerHash}
	if diagnostics.ConstellationRootWinnerScore != nil {
		score := cloneModelScore(*diagnostics.ConstellationRootWinnerScore)
		result.Score = &score
	}
	return result
}

func constellationRootPairs(baseline *model.ConstellationSeedDiagnostics, current *model.ConstellationSeedDiagnostics) []ConstellationRootPair {
	if baseline == nil || current == nil {
		return nil
	}
	baselineRoots := rootsByExactKey(baseline)
	currentRoots := rootsByExactKey(current)
	for exactKey, root := range currentRoots {
		if root.ParentGuardedFrontier != nil {
			return parentGuardedFrontierRootPairs(baselineRoots, currentRoots, exactKey, root)
		}
	}
	pairs := make([]ConstellationRootPair, 0, len(currentRoots))
	frontierRootID := current.RelaxationFrontierRootID
	if frontierRootID != "" {
		frontier, exists := rootByID(current.Roots, frontierRootID)
		if exists {
			parent, parentExists := baselineRoots[current.RelaxationFrontierParentExactKey]
			pairs = append(pairs, constellationRootPair("frontier_replacement", parent, parentExists, frontier, true))
		}
	}
	exactKeys := make([]string, 0, len(currentRoots))
	for exactKey := range currentRoots {
		exactKeys = append(exactKeys, exactKey)
	}
	sort.Strings(exactKeys)
	for _, exactKey := range exactKeys {
		currentRoot := currentRoots[exactKey]
		if currentRoot.ID == frontierRootID {
			continue
		}
		baselineRoot, exists := baselineRoots[exactKey]
		if !exists {
			continue
		}
		pairs = append(pairs, constellationRootPair("same_root", baselineRoot, true, currentRoot, true))
	}
	return pairs
}

func parentGuardedFrontierRootPairs(baselineRoots map[string]model.ConstellationRootDiagnostic, currentRoots map[string]model.ConstellationRootDiagnostic, familyExactKey string, familyRoot model.ConstellationRootDiagnostic) []ConstellationRootPair {
	pairs := make([]ConstellationRootPair, 0, len(currentRoots)+1)
	if baselineRoot, exists := baselineRoots[familyExactKey]; exists {
		family := familyRoot.ParentGuardedFrontier
		pairs = append(pairs, constellationRootPairScores("same_root_packing_policy", baselineRoot, true, familyRoot.ID, family.Parent.Completed, family.Parent.BestScore))
		pairs = append(pairs, constellationRootPairScores("parent_guarded_frontier", baselineRoot, true, familyRoot.ID, family.FamilyWinnerMember != "none", family.FamilyBestScore))
	}
	exactKeys := make([]string, 0, len(currentRoots))
	for exactKey := range currentRoots {
		if exactKey != familyExactKey {
			exactKeys = append(exactKeys, exactKey)
		}
	}
	sort.Strings(exactKeys)
	for _, exactKey := range exactKeys {
		baselineRoot, exists := baselineRoots[exactKey]
		if !exists {
			continue
		}
		pairs = append(pairs, constellationRootPair("same_root_packing_policy", baselineRoot, true, currentRoots[exactKey], true))
	}
	return pairs
}

func rootsByExactKey(diagnostics *model.ConstellationSeedDiagnostics) map[string]model.ConstellationRootDiagnostic {
	exactBySkeleton := make(map[string]string, len(diagnostics.Skeletons))
	for _, skeleton := range diagnostics.Skeletons {
		exactBySkeleton[skeleton.ID] = skeleton.ExactKey
	}
	roots := make(map[string]model.ConstellationRootDiagnostic, len(diagnostics.Roots))
	for _, root := range diagnostics.Roots {
		if exactKey := exactBySkeleton[root.SkeletonID]; exactKey != "" {
			roots[exactKey] = root
		}
	}
	return roots
}

func rootByID(roots []model.ConstellationRootDiagnostic, rootID string) (model.ConstellationRootDiagnostic, bool) {
	for _, root := range roots {
		if root.ID == rootID {
			return root, true
		}
	}
	return model.ConstellationRootDiagnostic{}, false
}

func constellationRootPair(kind string, baseline model.ConstellationRootDiagnostic, baselineExists bool, current model.ConstellationRootDiagnostic, currentExists bool) ConstellationRootPair {
	pair := ConstellationRootPair{Kind: kind}
	if baselineExists {
		pair.BaselineRootID = baseline.ID
		pair.BaselineComplete = baseline.Completed
		if baseline.BestScore != nil {
			score := cloneModelScore(*baseline.BestScore)
			pair.BaselineScore = &score
		}
	}
	if currentExists {
		pair.CurrentRootID = current.ID
		pair.CurrentComplete = current.Completed
		if current.BestScore != nil {
			score := cloneModelScore(*current.BestScore)
			pair.CurrentScore = &score
		}
	}
	pair.ScoreCmp = compareOptionalScores(pair.CurrentScore, pair.BaselineScore)
	return pair
}

func constellationRootPairScores(kind string, baseline model.ConstellationRootDiagnostic, baselineExists bool, currentRootID string, currentComplete bool, currentScore *model.Score) ConstellationRootPair {
	pair := ConstellationRootPair{Kind: kind, CurrentRootID: currentRootID, CurrentComplete: currentComplete}
	if baselineExists {
		pair.BaselineRootID = baseline.ID
		pair.BaselineComplete = baseline.Completed
		if baseline.BestScore != nil {
			score := cloneModelScore(*baseline.BestScore)
			pair.BaselineScore = &score
		}
	}
	if currentScore != nil {
		score := cloneModelScore(*currentScore)
		pair.CurrentScore = &score
	}
	pair.ScoreCmp = compareOptionalScores(pair.CurrentScore, pair.BaselineScore)
	return pair
}

func compareOptionalScores(left *model.Score, right *model.Score) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	return model.CompareScores(*left, *right)
}

func regressionAttribution(pairs []ConstellationRootPair) string {
	frontierRegression := false
	packingRegression := false
	for _, pair := range pairs {
		if pair.ScoreCmp >= 0 {
			continue
		}
		switch pair.Kind {
		case "frontier_replacement", "parent_guarded_frontier":
			frontierRegression = true
		case "same_root", "same_root_packing_policy":
			packingRegression = true
		}
	}
	switch {
	case frontierRegression && packingRegression:
		return regressionMixed
	case frontierRegression:
		return regressionFrontier
	case packingRegression:
		return regressionPacking
	default:
		return regressionUnattributed
	}
}

func semanticPolicyFingerprint(run Run) string {
	if len(run.Search.Stages) > 0 {
		parts := make([]string, 0, len(run.Search.Stages))
		complete := true
		for _, stage := range run.Search.Stages {
			if stage.StagePolicyFingerprint == "" {
				complete = false
			}
			parts = append(parts, stage.ID+":"+stage.StagePolicyFingerprint)
		}
		sort.Strings(parts)
		if complete {
			return strings.Join(parts, "|")
		}
	}
	encoded, err := json.Marshal(run.SolverSettings)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func cloneModelScore(score model.Score) model.Score {
	result := score
	result.PriorityCounts = append([]int(nil), score.PriorityCounts...)
	return result
}

func FormatConstellationExperimentComparison(writer io.Writer, comparison ConstellationExperimentComparison) {
	fmt.Fprintf(writer, "Mode: %s | score wins=%d losses=%d ties=%d\n", comparison.Mode, comparison.ScoreWins, comparison.ScoreLosses, comparison.ScoreTies)
	fmt.Fprintf(writer, "Policy: %s -> %s | workers %d -> %d | packing beam %d -> %d | roots %d -> %d | share %d -> %d bps\n\n", comparison.PolicyDelta.BaselineVariant, comparison.PolicyDelta.CurrentVariant, comparison.PolicyDelta.BaselineWorkers, comparison.PolicyDelta.CurrentWorkers, comparison.PolicyDelta.BaselinePackingBeam, comparison.PolicyDelta.CurrentPackingBeam, comparison.PolicyDelta.BaselineMaxRoots, comparison.PolicyDelta.CurrentMaxRoots, comparison.PolicyDelta.BaselineShareBps, comparison.PolicyDelta.CurrentShareBps)
	tw := tabwriter.NewWriter(writer, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "Scenario\tBudget\tRepeat\tScore\tSame hash\tNode delta\tNode ratio\tFirst completion delta\tV5 behavior\tAttribution")
	for _, row := range comparison.Rows {
		firstCompletionDelta := ""
		if row.FirstCompletionNodeDelta != nil {
			firstCompletionDelta = fmt.Sprintf("%d", *row.FirstCompletionNodeDelta)
		}
		fmt.Fprintf(tw, "%s\t%d\t%d\t%s\t%t\t%d\t%.4f\t%s\t%s\t%s\n", row.Scenario, row.Budget, row.Repeat, row.ScoreStatus, row.SameHash, row.NormalNodeDelta, row.NormalNodeRatio, firstCompletionDelta, row.V5BehaviorClass, row.RegressionAttribution)
	}
	_ = tw.Flush()
}
