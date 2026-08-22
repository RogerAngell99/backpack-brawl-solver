package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime/debug"
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

var DefaultBudgets = []int64{250000, 1000000, 5000000, 20000000, 100000000}

const (
	RepairSearchModeScenario = "scenario"
	RepairSearchModeOn       = "on"
	RepairSearchModeOff      = "off"

	outgoingPerInstanceFoodScenarioName = "outgoing-per-instance-food"
)

type RunConfig struct {
	CatalogPath                                                                   string
	ScenarioDir                                                                   string
	Scenarios                                                                     []string
	Budgets                                                                       []int64
	Repeat                                                                        int
	Workers                                                                       int
	Top                                                                           int
	RepairSearchMode                                                              string
	PlateauVariant                                                                string
	Diagnostic                                                                    bool
	OperationProfiling                                                            bool
	ConstellationSeedV1                                                           bool
	ConstellationSeedVariant                                                      string
	ConstellationFeasibilityProbe                                                 bool
	ConstellationCompletionOptimizationProbe                                      bool
	ConstellationCandidatePoolFeasibilitySweep                                    bool
	ConstellationCandidateCompletionOptimizationProbe                             bool
	ConstellationCandidateCompletionOptimizationCandidateID                       string
	ConstellationCandidateCompletionOptimizationStage                             string
	ConstellationCandidateCompletionOptimizationNodeBudget                        int64
	ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey           string
	ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint string
	ConstellationForcedCandidateRootedPackingProbe                                bool
	ConstellationForcedCandidateRootedPackingCandidateID                          string
	ConstellationForcedCandidateRootedPackingSlot                                 int
	ConstellationForcedCandidateRootedPackingStage                                string
	ConstellationForcedCandidateRootedPackingBeamWidth                            int
	ConstellationForcedCandidateRootedPackingRanking                              string
	ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey               string
	ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint     string
	ConstellationParentFrontierHedgeProbe                                         bool
	ConstellationParentFrontierHedgeProbeStage                                    string
}

type Report struct {
	GeneratedAt                                                                   string  `json:"generated_at"`
	CatalogPath                                                                   string  `json:"catalog_path"`
	ScenarioDir                                                                   string  `json:"scenario_dir"`
	Budgets                                                                       []int64 `json:"budgets"`
	Repeat                                                                        int     `json:"repeat"`
	Workers                                                                       int     `json:"workers"`
	Top                                                                           int     `json:"top"`
	RepairSearchMode                                                              string  `json:"repair_search_mode"`
	PlateauVariant                                                                string  `json:"plateau_variant"`
	Diagnostic                                                                    bool    `json:"diagnostic"`
	OperationProfiling                                                            bool    `json:"operation_profiling,omitempty"`
	ConstellationSeedV1                                                           bool    `json:"constellation_seed_v1"`
	ConstellationSeedVariant                                                      string  `json:"constellation_seed_variant,omitempty"`
	ConstellationFeasibilityProbe                                                 bool    `json:"constellation_feasibility_probe"`
	ConstellationCompletionOptimizationProbe                                      bool    `json:"constellation_completion_optimization_probe"`
	ConstellationCandidatePoolFeasibilitySweep                                    bool    `json:"constellation_candidate_pool_feasibility_sweep"`
	ConstellationCandidateCompletionOptimizationProbe                             bool    `json:"constellation_candidate_completion_optimization_probe"`
	ConstellationCandidateCompletionOptimizationCandidateID                       string  `json:"constellation_candidate_completion_optimization_candidate_id,omitempty"`
	ConstellationCandidateCompletionOptimizationStage                             string  `json:"constellation_candidate_completion_optimization_stage,omitempty"`
	ConstellationCandidateCompletionOptimizationNodeBudget                        int64   `json:"constellation_candidate_completion_optimization_node_budget,omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey           string  `json:"constellation_candidate_completion_optimization_initial_witness_layout_key,omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint string  `json:"constellation_candidate_completion_optimization_initial_witness_semantic_fingerprint,omitempty"`
	ConstellationForcedCandidateRootedPackingProbe                                bool    `json:"constellation_forced_candidate_rooted_packing_probe"`
	ConstellationForcedCandidateRootedPackingCandidateID                          string  `json:"constellation_forced_candidate_rooted_packing_candidate_id,omitempty"`
	ConstellationForcedCandidateRootedPackingSlot                                 int     `json:"constellation_forced_candidate_rooted_packing_slot,omitempty"`
	ConstellationForcedCandidateRootedPackingStage                                string  `json:"constellation_forced_candidate_rooted_packing_stage,omitempty"`
	ConstellationForcedCandidateRootedPackingBeamWidth                            int     `json:"constellation_forced_candidate_rooted_packing_beam_width,omitempty"`
	ConstellationForcedCandidateRootedPackingRanking                              string  `json:"constellation_forced_candidate_rooted_packing_ranking,omitempty"`
	ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey               string  `json:"constellation_forced_candidate_rooted_packing_shadow_witness_layout_key,omitempty"`
	ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint     string  `json:"constellation_forced_candidate_rooted_packing_shadow_witness_semantic_fingerprint,omitempty"`
	ConstellationParentFrontierHedgeProbe                                         bool    `json:"constellation_parent_frontier_hedge_probe"`
	ConstellationParentFrontierHedgeProbeStage                                    string  `json:"constellation_parent_frontier_hedge_probe_stage,omitempty"`
	CatalogSHA256                                                                 string  `json:"catalog_sha256"`
	BuildRevision                                                                 string  `json:"build_revision"`
	Runs                                                                          []Run   `json:"runs"`
}

type Run struct {
	Scenario                                                                      string                   `json:"scenario"`
	ScenarioPath                                                                  string                   `json:"scenario_path"`
	Budget                                                                        int64                    `json:"budget"`
	Repeat                                                                        int                      `json:"repeat"`
	RepairSearch                                                                  bool                     `json:"repair_search"`
	PlateauVariant                                                                string                   `json:"plateau_variant"`
	ConstellationSeedV1                                                           bool                     `json:"constellation_seed_v1"`
	ConstellationSeedVariant                                                      string                   `json:"constellation_seed_variant,omitempty"`
	ConstellationFeasibilityProbe                                                 bool                     `json:"constellation_feasibility_probe"`
	ConstellationCompletionOptimizationProbe                                      bool                     `json:"constellation_completion_optimization_probe"`
	ConstellationCandidatePoolFeasibilitySweep                                    bool                     `json:"constellation_candidate_pool_feasibility_sweep"`
	ConstellationCandidateCompletionOptimizationProbe                             bool                     `json:"constellation_candidate_completion_optimization_probe"`
	ConstellationCandidateCompletionOptimizationCandidateID                       string                   `json:"constellation_candidate_completion_optimization_candidate_id,omitempty"`
	ConstellationCandidateCompletionOptimizationStage                             string                   `json:"constellation_candidate_completion_optimization_stage,omitempty"`
	ConstellationCandidateCompletionOptimizationNodeBudget                        int64                    `json:"constellation_candidate_completion_optimization_node_budget,omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey           string                   `json:"constellation_candidate_completion_optimization_initial_witness_layout_key,omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint string                   `json:"constellation_candidate_completion_optimization_initial_witness_semantic_fingerprint,omitempty"`
	ConstellationForcedCandidateRootedPackingProbe                                bool                     `json:"constellation_forced_candidate_rooted_packing_probe"`
	ConstellationForcedCandidateRootedPackingCandidateID                          string                   `json:"constellation_forced_candidate_rooted_packing_candidate_id,omitempty"`
	ConstellationForcedCandidateRootedPackingSlot                                 int                      `json:"constellation_forced_candidate_rooted_packing_slot,omitempty"`
	ConstellationForcedCandidateRootedPackingStage                                string                   `json:"constellation_forced_candidate_rooted_packing_stage,omitempty"`
	ConstellationForcedCandidateRootedPackingBeamWidth                            int                      `json:"constellation_forced_candidate_rooted_packing_beam_width,omitempty"`
	ConstellationForcedCandidateRootedPackingRanking                              string                   `json:"constellation_forced_candidate_rooted_packing_ranking,omitempty"`
	ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey               string                   `json:"constellation_forced_candidate_rooted_packing_shadow_witness_layout_key,omitempty"`
	ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint     string                   `json:"constellation_forced_candidate_rooted_packing_shadow_witness_semantic_fingerprint,omitempty"`
	ConstellationParentFrontierHedgeProbe                                         bool                     `json:"constellation_parent_frontier_hedge_probe"`
	ConstellationParentFrontierHedgeProbeStage                                    string                   `json:"constellation_parent_frontier_hedge_probe_stage,omitempty"`
	OperationProfiling                                                            bool                     `json:"operation_profiling,omitempty"`
	PrioritySemantics                                                             model.PrioritySemantics  `json:"priority_semantics"`
	Priorities                                                                    []string                 `json:"priorities"`
	NoSkips                                                                       bool                     `json:"no_skips"`
	StopOnCoverageCeiling                                                         bool                     `json:"stop_on_coverage_ceiling"`
	StopOnPriorityCeiling                                                         bool                     `json:"stop_on_priority_ceiling"`
	SolverSettings                                                                solver.BenchmarkSettings `json:"solver_settings"`
	ElapsedMS                                                                     int64                    `json:"elapsed_ms"`
	NodesPerSecond                                                                float64                  `json:"nodes_per_second,omitempty"`
	Score                                                                         ScoreSummary             `json:"score"`
	LayoutKey                                                                     string                   `json:"layout_key,omitempty"`
	CanonicalLayoutHash                                                           string                   `json:"canonical_layout_hash,omitempty"`
	Search                                                                        SearchSummary            `json:"search"`
	CoverageSummaries                                                             []CoverageSummary        `json:"coverage_summaries,omitempty"`
	LooseStarPriorities                                                           []LooseStarSummary       `json:"loose_star_priorities,omitempty"`
	Crafts                                                                        []string                 `json:"crafts,omitempty"`
	Solution                                                                      json.RawMessage          `json:"solution,omitempty"`
	Error                                                                         string                   `json:"error,omitempty"`
}

type ScoreSummary struct {
	PriorityCounts                []int `json:"priority_counts,omitempty"`
	Crafts                        int   `json:"crafts"`
	Stars                         int   `json:"stars"`
	Items                         int   `json:"items"`
	StarTargetBreadth             int   `json:"star_target_breadth,omitempty"`
	StarReciprocalPairs           int   `json:"star_reciprocal_pairs,omitempty"`
	StarSourceDefinitionDiversity int   `json:"star_source_definition_diversity,omitempty"`
}

type SearchSummary struct {
	NodesExplored                 int64                               `json:"nodes_explored"`
	NodesPerSecond                float64                             `json:"nodes_per_second,omitempty"`
	SetupMS                       int64                               `json:"setup_ms,omitempty"`
	Limited                       bool                                `json:"limited"`
	Refined                       bool                                `json:"refined"`
	CoverageSources               []string                            `json:"coverage_sources,omitempty"`
	CoverageTargetCount           int                                 `json:"coverage_target_count,omitempty"`
	CoverageCeiling               []CoverageBucketSummary             `json:"coverage_ceiling,omitempty"`
	CoverageCeilingReached        bool                                `json:"coverage_ceiling_reached,omitempty"`
	PriorityCeiling               []int                               `json:"priority_ceiling,omitempty"`
	PriorityCeilingReached        bool                                `json:"priority_ceiling_reached,omitempty"`
	CoverageBoundChecks           int64                               `json:"coverage_bound_checks,omitempty"`
	CoveragePrunedNodes           int64                               `json:"coverage_pruned_nodes,omitempty"`
	ExactBoundChecks              int64                               `json:"exact_bound_checks,omitempty"`
	ExactBoundPrunedNodes         int64                               `json:"exact_bound_pruned_nodes,omitempty"`
	OutgoingBoundChecks           int64                               `json:"outgoing_bound_checks,omitempty"`
	OutgoingBoundPrunedNodes      int64                               `json:"outgoing_bound_pruned_nodes,omitempty"`
	CoverageSeedNodes             int64                               `json:"coverage_seed_nodes,omitempty"`
	CoverageSeedCandidates        int                                 `json:"coverage_seed_candidates,omitempty"`
	CoverageSeedBest              string                              `json:"coverage_seed_best,omitempty"`
	StarSeedNodes                 int64                               `json:"star_seed_nodes,omitempty"`
	StarSeedCandidates            int                                 `json:"star_seed_candidates,omitempty"`
	PackingSeedNodes              int64                               `json:"packing_seed_nodes,omitempty"`
	PackingSeedCandidates         int                                 `json:"packing_seed_candidates,omitempty"`
	PackingSeedHardPruned         int64                               `json:"packing_seed_hard_pruned,omitempty"`
	PackingSeedStatesDeduplicated int64                               `json:"packing_seed_states_deduplicated,omitempty"`
	SymmetryPrunedBranches        int64                               `json:"symmetry_pruned_branches,omitempty"`
	FirstCompletePhase            string                              `json:"first_complete_phase,omitempty"`
	FirstCompleteNodes            int64                               `json:"first_complete_nodes,omitempty"`
	FirstCompleteMS               int64                               `json:"first_complete_ms,omitempty"`
	SeedBest                      ScoreSummary                        `json:"seed_best"`
	SearchBest                    ScoreSummary                        `json:"search_best"`
	PostRepairBest                ScoreSummary                        `json:"post_repair_best"`
	RefineBest                    ScoreSummary                        `json:"refine_best"`
	InitialBestPriorityCounts     []int                               `json:"initial_best_priority_counts,omitempty"`
	SeedBestPriorityCounts        []int                               `json:"seed_best_priority_counts,omitempty"`
	SearchBestPriorityCounts      []int                               `json:"search_best_priority_counts,omitempty"`
	PostRepairBestPriorityCounts  []int                               `json:"post_repair_best_priority_counts,omitempty"`
	RefineBestPriorityCounts      []int                               `json:"refine_best_priority_counts,omitempty"`
	ParallelTasks                 int                                 `json:"parallel_tasks,omitempty"`
	ParallelWorkersUsed           int                                 `json:"parallel_workers_used,omitempty"`
	RefineMovesChecked            int64                               `json:"refine_moves_checked,omitempty"`
	RefineImprovements            int                                 `json:"refine_improvements,omitempty"`
	RefineBestDelta               string                              `json:"refine_best_delta,omitempty"`
	RepairNodes                   int64                               `json:"repair_nodes,omitempty"`
	RepairIterations              int                                 `json:"repair_iterations,omitempty"`
	RepairImprovements            int                                 `json:"repair_improvements,omitempty"`
	RepairCandidates              int                                 `json:"repair_candidates,omitempty"`
	RepairBest                    string                              `json:"repair_best,omitempty"`
	RepairParallelTasks           int                                 `json:"repair_parallel_tasks,omitempty"`
	RepairParallelWorkersUsed     int                                 `json:"repair_parallel_workers_used,omitempty"`
	StoppedAfterCoverageCeiling   bool                                `json:"stopped_after_coverage_ceiling,omitempty"`
	StoppedAfterPriorityCeiling   bool                                `json:"stopped_after_priority_ceiling,omitempty"`
	DiagnosticsEnabled            bool                                `json:"diagnostics_enabled,omitempty"`
	GlobalBudgetConsumed          int64                               `json:"global_budget_consumed,omitempty"`
	UnusedGlobalNodes             int64                               `json:"unused_global_nodes,omitempty"`
	NormalBudgetConfigured        int64                               `json:"normal_budget_configured,omitempty"`
	NormalBudgetConsumed          int64                               `json:"normal_budget_consumed,omitempty"`
	DiagnosticBudgetConfigured    int64                               `json:"diagnostic_budget_configured,omitempty"`
	DiagnosticBudgetConsumed      int64                               `json:"diagnostic_budget_consumed,omitempty"`
	ExecutionBudgetConfigured     int64                               `json:"execution_budget_configured,omitempty"`
	ExecutionBudgetConsumed       int64                               `json:"execution_budget_consumed,omitempty"`
	UnchargedWork                 int64                               `json:"uncharged_work,omitempty"`
	PhaseWork                     []model.SearchPhaseWork             `json:"phase_work,omitempty"`
	IncumbentTrace                []model.IncumbentEvent              `json:"incumbent_trace,omitempty"`
	PriorityCeilingStats          *model.PriorityCeilingStats         `json:"priority_ceiling_stats,omitempty"`
	Plateau                       model.PlateauStats                  `json:"plateau"`
	StarUpperBounds               model.StarUpperBounds               `json:"star_upper_bounds"`
	FirstFullyPackedPhase         string                              `json:"first_fully_packed_phase,omitempty"`
	FirstFullyPackedNodes         int64                               `json:"first_fully_packed_nodes,omitempty"`
	FirstFullyPackedMS            int64                               `json:"first_fully_packed_ms,omitempty"`
	PackingSeedDiagnostics        model.PackingSeedDiagnostics        `json:"packing_seed_diagnostics,omitempty"`
	ConstellationSeedNodes        int64                               `json:"constellation_seed_nodes,omitempty"`
	ConstellationSeedCandidates   int                                 `json:"constellation_seed_candidates,omitempty"`
	ConstellationSeedDiagnostics  *model.ConstellationSeedDiagnostics `json:"constellation_seed_diagnostics,omitempty"`
	ConfigFingerprint             string                              `json:"config_fingerprint,omitempty"`
	ExecutionFingerprint          string                              `json:"execution_fingerprint,omitempty"`
	Stages                        []model.SearchStageStats            `json:"stages,omitempty"`
	TaskAllocation                *model.TaskAllocationStats          `json:"task_allocation,omitempty"`
	PlateauArchive                *model.PlateauArchiveStats          `json:"plateau_archive,omitempty"`
	PlateauLNSNodes               int64                               `json:"plateau_lns_nodes,omitempty"`
	PlateauRefineNodes            int64                               `json:"plateau_refine_nodes,omitempty"`
	PlateauRefineWalkLength       int                                 `json:"plateau_refine_walk_length,omitempty"`
	PlateauRefineMaxValley        int                                 `json:"plateau_refine_max_valley,omitempty"`
	PlateauRefineImproved         bool                                `json:"plateau_refine_improved,omitempty"`
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
	LinkCount    int    `json:"link_count,omitempty"`
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

func ValidatePlateauVariant(value string) error {
	return solver.ValidatePlateauVariant(value)
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
	if config.Diagnostic && config.Workers != 1 {
		return Report{}, fmt.Errorf("diagnostic runs require exactly one worker")
	}
	if config.OperationProfiling && !solver.OperationProfilingAvailable() {
		return Report{}, fmt.Errorf("operation profiling requires a binary built with -tags searchprofile")
	}
	if config.OperationProfiling && config.Diagnostic {
		return Report{}, fmt.Errorf("operation profiling and diagnostic runs must be separate")
	}
	if config.OperationProfiling && config.Workers != 1 {
		return Report{}, fmt.Errorf("operation profiling requires exactly one worker")
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
	if config.PlateauVariant == "" {
		config.PlateauVariant = solver.DefaultPlateauVariant
	}
	if err := ValidatePlateauVariant(config.PlateauVariant); err != nil {
		return Report{}, err
	}
	if config.ConstellationSeedVariant != "" {
		if err := solver.ValidateConstellationSeedVariant(config.ConstellationSeedVariant); err != nil {
			return Report{}, err
		}
	}
	if config.ConstellationSeedV1 && config.ConstellationSeedVariant != "" && config.ConstellationSeedVariant != solver.ConstellationSeedVariantV1 {
		return Report{}, fmt.Errorf("constellation seed v1 alias conflicts with explicit variant")
	}
	if config.ConstellationFeasibilityProbe && !config.Diagnostic {
		return Report{}, fmt.Errorf("constellation feasibility probe requires diagnostic runs")
	}
	if config.ConstellationCompletionOptimizationProbe && !config.Diagnostic {
		return Report{}, fmt.Errorf("constellation completion optimization probe requires diagnostic runs")
	}
	if config.ConstellationCompletionOptimizationProbe {
		if err := solver.ValidateConstellationCompletionOptimizationProbeVariant(config.ConstellationSeedVariant); err != nil {
			return Report{}, err
		}
	}
	if config.ConstellationFeasibilityProbe && config.ConstellationCompletionOptimizationProbe {
		return Report{}, fmt.Errorf("constellation feasibility and completion optimization probes cannot run together")
	}
	if config.ConstellationCandidatePoolFeasibilitySweep && !config.Diagnostic {
		return Report{}, fmt.Errorf("constellation candidate pool feasibility sweep requires diagnostic runs")
	}
	if config.ConstellationCandidatePoolFeasibilitySweep && config.ConstellationSeedVariant != solver.ConstellationSeedVariantV4 {
		return Report{}, fmt.Errorf("constellation candidate pool feasibility sweep requires constellation seed variant %q", solver.ConstellationSeedVariantV4)
	}
	if config.ConstellationCandidatePoolFeasibilitySweep && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe) {
		return Report{}, fmt.Errorf("constellation candidate pool feasibility sweep cannot run with another constellation probe")
	}
	if config.ConstellationCandidateCompletionOptimizationProbe && !config.Diagnostic {
		return Report{}, fmt.Errorf("constellation candidate completion optimization probe requires diagnostic runs")
	}
	if config.ConstellationCandidateCompletionOptimizationProbe {
		if err := solver.ValidateConstellationCandidateCompletionOptimizationTarget(config.ConstellationSeedVariant, config.ConstellationCandidateCompletionOptimizationCandidateID, config.ConstellationCandidateCompletionOptimizationStage); err != nil {
			return Report{}, err
		}
		if config.ConstellationCandidateCompletionOptimizationNodeBudget < 0 {
			return Report{}, fmt.Errorf("constellation candidate completion optimization node budget must be non-negative")
		}
		if (config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey == "") != (config.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint == "") {
			return Report{}, fmt.Errorf("candidate completion optimization witness layout key and semantic fingerprint must be supplied together")
		}
	}
	if config.ConstellationCandidateCompletionOptimizationProbe && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe || config.ConstellationCandidatePoolFeasibilitySweep) {
		return Report{}, fmt.Errorf("constellation candidate completion optimization probe cannot run with another constellation probe")
	}
	if config.ConstellationForcedCandidateRootedPackingProbe && !config.Diagnostic {
		return Report{}, fmt.Errorf("constellation forced candidate rooted packing probe requires diagnostic runs")
	}
	if config.ConstellationForcedCandidateRootedPackingProbe {
		if err := solver.ValidateConstellationForcedCandidateRootedPackingTarget(config.ConstellationSeedVariant, config.ConstellationForcedCandidateRootedPackingCandidateID, config.ConstellationForcedCandidateRootedPackingSlot, config.ConstellationForcedCandidateRootedPackingStage); err != nil {
			return Report{}, err
		}
		if (config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey == "") != (config.ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint == "") {
			return Report{}, fmt.Errorf("forced rooted packing shadow witness layout key and semantic fingerprint must be supplied together")
		}
		if config.ConstellationForcedCandidateRootedPackingBeamWidth < 0 {
			return Report{}, fmt.Errorf("constellation forced candidate rooted packing beam width must be non-negative")
		}
		if err := solver.ValidateConstellationForcedCandidateRootedPackingRanking(config.ConstellationForcedCandidateRootedPackingRanking); err != nil {
			return Report{}, err
		}
	}
	if config.ConstellationForcedCandidateRootedPackingProbe && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe || config.ConstellationCandidatePoolFeasibilitySweep || config.ConstellationCandidateCompletionOptimizationProbe) {
		return Report{}, fmt.Errorf("constellation forced candidate rooted packing probe cannot run with another constellation probe")
	}
	if config.ConstellationParentFrontierHedgeProbe {
		if !config.Diagnostic {
			return Report{}, fmt.Errorf("constellation parent-frontier hedge probe requires diagnostic runs")
		}
		if err := solver.ValidateConstellationParentFrontierHedgeProbeTarget(config.ConstellationSeedVariant, config.ConstellationParentFrontierHedgeProbeStage); err != nil {
			return Report{}, err
		}
	}
	if config.ConstellationParentFrontierHedgeProbe && (config.ConstellationFeasibilityProbe || config.ConstellationCompletionOptimizationProbe || config.ConstellationCandidatePoolFeasibilitySweep || config.ConstellationCandidateCompletionOptimizationProbe || config.ConstellationForcedCandidateRootedPackingProbe) {
		return Report{}, fmt.Errorf("constellation parent-frontier hedge probe cannot run with another constellation probe")
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
	files = filterScenarioFiles(files, config.Scenarios)
	if len(files) == 0 {
		return Report{}, fmt.Errorf("no selected scenario JSON files found in %s", config.ScenarioDir)
	}

	report := Report{
		GeneratedAt:                              time.Now().UTC().Format(time.RFC3339),
		CatalogPath:                              config.CatalogPath,
		ScenarioDir:                              config.ScenarioDir,
		Budgets:                                  append([]int64(nil), config.Budgets...),
		Repeat:                                   config.Repeat,
		Workers:                                  config.Workers,
		Top:                                      config.Top,
		RepairSearchMode:                         config.RepairSearchMode,
		PlateauVariant:                           config.PlateauVariant,
		Diagnostic:                               config.Diagnostic,
		OperationProfiling:                       config.OperationProfiling,
		ConstellationSeedV1:                      config.ConstellationSeedV1,
		ConstellationSeedVariant:                 config.ConstellationSeedVariant,
		ConstellationFeasibilityProbe:            config.ConstellationFeasibilityProbe,
		ConstellationCompletionOptimizationProbe: config.ConstellationCompletionOptimizationProbe,
		ConstellationCandidatePoolFeasibilitySweep:                                    config.ConstellationCandidatePoolFeasibilitySweep,
		ConstellationCandidateCompletionOptimizationProbe:                             config.ConstellationCandidateCompletionOptimizationProbe,
		ConstellationCandidateCompletionOptimizationCandidateID:                       config.ConstellationCandidateCompletionOptimizationCandidateID,
		ConstellationCandidateCompletionOptimizationStage:                             config.ConstellationCandidateCompletionOptimizationStage,
		ConstellationCandidateCompletionOptimizationNodeBudget:                        config.ConstellationCandidateCompletionOptimizationNodeBudget,
		ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey:           config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey,
		ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint: config.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint,
		ConstellationForcedCandidateRootedPackingProbe:                                config.ConstellationForcedCandidateRootedPackingProbe,
		ConstellationForcedCandidateRootedPackingCandidateID:                          config.ConstellationForcedCandidateRootedPackingCandidateID,
		ConstellationForcedCandidateRootedPackingSlot:                                 config.ConstellationForcedCandidateRootedPackingSlot,
		ConstellationForcedCandidateRootedPackingStage:                                config.ConstellationForcedCandidateRootedPackingStage,
		ConstellationForcedCandidateRootedPackingBeamWidth:                            config.ConstellationForcedCandidateRootedPackingBeamWidth,
		ConstellationForcedCandidateRootedPackingRanking:                              config.ConstellationForcedCandidateRootedPackingRanking,
		ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey:               config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey,
		ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint:     config.ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint,
		ConstellationParentFrontierHedgeProbe:                                         config.ConstellationParentFrontierHedgeProbe,
		ConstellationParentFrontierHedgeProbeStage:                                    config.ConstellationParentFrontierHedgeProbeStage,
		CatalogSHA256: catalogSHA256(config.CatalogPath),
		BuildRevision: buildRevision(),
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

func filterScenarioFiles(files []string, names []string) []string {
	if len(names) == 0 {
		return files
	}
	selected := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		selected[strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))] = true
	}
	filtered := make([]string, 0, len(files))
	for _, file := range files {
		name := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		if selected[name] {
			filtered = append(filtered, file)
		}
	}
	return filtered
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
		Scenario:                                 name,
		ScenarioPath:                             path,
		Budget:                                   budget,
		Repeat:                                   repeat,
		PlateauVariant:                           config.PlateauVariant,
		ConstellationSeedV1:                      config.ConstellationSeedV1,
		ConstellationSeedVariant:                 config.ConstellationSeedVariant,
		ConstellationFeasibilityProbe:            config.ConstellationFeasibilityProbe,
		ConstellationCompletionOptimizationProbe: config.ConstellationCompletionOptimizationProbe,
		ConstellationCandidatePoolFeasibilitySweep:                                    config.ConstellationCandidatePoolFeasibilitySweep,
		ConstellationCandidateCompletionOptimizationProbe:                             config.ConstellationCandidateCompletionOptimizationProbe,
		ConstellationCandidateCompletionOptimizationCandidateID:                       config.ConstellationCandidateCompletionOptimizationCandidateID,
		ConstellationCandidateCompletionOptimizationStage:                             config.ConstellationCandidateCompletionOptimizationStage,
		ConstellationCandidateCompletionOptimizationNodeBudget:                        config.ConstellationCandidateCompletionOptimizationNodeBudget,
		ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey:           config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey,
		ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint: config.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint,
		ConstellationForcedCandidateRootedPackingProbe:                                config.ConstellationForcedCandidateRootedPackingProbe,
		ConstellationForcedCandidateRootedPackingCandidateID:                          config.ConstellationForcedCandidateRootedPackingCandidateID,
		ConstellationForcedCandidateRootedPackingSlot:                                 config.ConstellationForcedCandidateRootedPackingSlot,
		ConstellationForcedCandidateRootedPackingStage:                                config.ConstellationForcedCandidateRootedPackingStage,
		ConstellationForcedCandidateRootedPackingBeamWidth:                            config.ConstellationForcedCandidateRootedPackingBeamWidth,
		ConstellationForcedCandidateRootedPackingRanking:                              config.ConstellationForcedCandidateRootedPackingRanking,
		ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey:               config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey,
		ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint:     config.ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint,
		ConstellationParentFrontierHedgeProbe:                                         config.ConstellationParentFrontierHedgeProbe,
		ConstellationParentFrontierHedgeProbeStage:                                    config.ConstellationParentFrontierHedgeProbeStage,
		OperationProfiling: config.OperationProfiling,
		SolverSettings:     solver.SettingsForBenchmark(budget, config.PlateauVariant),
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
	run.NoSkips = noSkips
	run.PrioritySemantics = loadedScenario.PrioritySemantics
	run.Priorities = append([]string(nil), loadedScenario.Priorities...)
	stopOnCoverageCeiling := false
	if loadedScenario.StopOnCoverageCeiling != nil {
		stopOnCoverageCeiling = *loadedScenario.StopOnCoverageCeiling
	}
	run.StopOnCoverageCeiling = stopOnCoverageCeiling
	stopOnPriorityCeiling := false
	if loadedScenario.StopOnPriorityCeiling != nil {
		stopOnPriorityCeiling = *loadedScenario.StopOnPriorityCeiling
	}
	run.StopOnPriorityCeiling = stopOnPriorityCeiling
	repairSearch := budget > 0
	if loadedScenario.RepairSearch != nil {
		repairSearch = *loadedScenario.RepairSearch
	}
	repairSearch = effectiveRepairSearch(config.RepairSearchMode, repairSearch, budget)
	run.RepairSearch = repairSearch

	solveConfig := solver.Config{
		TopN:                                     config.Top,
		AllowSkips:                               !noSkips,
		MaxNodes:                                 budget,
		Workers:                                  config.Workers,
		PrioritySemantics:                        loadedScenario.PrioritySemantics,
		Priorities:                               append([]string(nil), loadedScenario.Priorities...),
		CoverageGroups:                           loadedScenario.ModelCoverageGroups(),
		StopOnCoverageCeiling:                    stopOnCoverageCeiling,
		StopOnPriorityCeiling:                    stopOnPriorityCeiling,
		RepairSearch:                             repairSearch,
		PlateauVariant:                           config.PlateauVariant,
		Diagnostics:                              config.Diagnostic,
		OperationProfiling:                       config.OperationProfiling,
		EnableConstellationSeedV1:                config.ConstellationSeedV1,
		ConstellationSeedVariant:                 config.ConstellationSeedVariant,
		ConstellationFeasibilityProbe:            config.ConstellationFeasibilityProbe,
		ConstellationCompletionOptimizationProbe: config.ConstellationCompletionOptimizationProbe,
		ConstellationCandidatePoolFeasibilitySweep:                                    config.ConstellationCandidatePoolFeasibilitySweep,
		ConstellationCandidateCompletionOptimizationProbe:                             config.ConstellationCandidateCompletionOptimizationProbe,
		ConstellationCandidateCompletionOptimizationCandidateID:                       config.ConstellationCandidateCompletionOptimizationCandidateID,
		ConstellationCandidateCompletionOptimizationStage:                             config.ConstellationCandidateCompletionOptimizationStage,
		ConstellationCandidateCompletionOptimizationNodeBudget:                        config.ConstellationCandidateCompletionOptimizationNodeBudget,
		ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey:           config.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey,
		ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint: config.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint,
		ConstellationForcedCandidateRootedPackingProbe:                                config.ConstellationForcedCandidateRootedPackingProbe,
		ConstellationForcedCandidateRootedPackingCandidateID:                          config.ConstellationForcedCandidateRootedPackingCandidateID,
		ConstellationForcedCandidateRootedPackingSlot:                                 config.ConstellationForcedCandidateRootedPackingSlot,
		ConstellationForcedCandidateRootedPackingStage:                                config.ConstellationForcedCandidateRootedPackingStage,
		ConstellationForcedCandidateRootedPackingBeamWidth:                            config.ConstellationForcedCandidateRootedPackingBeamWidth,
		ConstellationForcedCandidateRootedPackingRanking:                              config.ConstellationForcedCandidateRootedPackingRanking,
		ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey:               config.ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey,
		ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint:     config.ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint,
		ConstellationParentFrontierHedgeProbe:                                         config.ConstellationParentFrontierHedgeProbe,
		ConstellationParentFrontierHedgeProbeStage:                                    config.ConstellationParentFrontierHedgeProbeStage,
	}
	if reference := diagnosticReferenceForScenario(name, config.Diagnostic); reference != nil {
		solveConfig.DiagnosticReference = reference
	}
	run.SolverSettings = solver.SettingsForBenchmarkConfig(solveConfig)

	startedAt := time.Now()
	solutions, err := solver.SolveLayout(loadedCatalog, loadedScenario.ItemIDs(), gridMask, solveConfig)
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
	run.CanonicalLayoutHash = best.CanonicalLayoutHash
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

func diagnosticReferenceForScenario(name string, diagnostic bool) []model.Placement {
	if !diagnostic || name != outgoingPerInstanceFoodScenarioName {
		return nil
	}
	return solver.OutgoingPerInstanceFoodDiagnosticReference()
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
		PriorityCounts:                append([]int(nil), score.PriorityCounts...),
		Crafts:                        score.CraftCount,
		Stars:                         score.StarCount,
		Items:                         score.ItemCount,
		StarTargetBreadth:             score.StarTargetBreadth,
		StarReciprocalPairs:           score.StarReciprocalPairs,
		StarSourceDefinitionDiversity: score.StarSourceDefinitionDiversity,
	}
}

func searchSummary(search model.SearchStats) SearchSummary {
	return SearchSummary{
		NodesExplored:                 search.NodesExplored,
		NodesPerSecond:                search.NodesPerSecond,
		SetupMS:                       search.SetupMS,
		Limited:                       search.Limited,
		Refined:                       search.Refined,
		CoverageSources:               append([]string(nil), search.CoverageSources...),
		CoverageTargetCount:           search.CoverageTargetCount,
		CoverageCeiling:               coverageBucketSummaries(search.CoverageCeiling),
		CoverageCeilingReached:        search.CoverageCeilingReached,
		PriorityCeiling:               append([]int(nil), search.PriorityCeiling...),
		PriorityCeilingReached:        search.PriorityCeilingReached,
		CoverageBoundChecks:           search.CoverageBoundChecks,
		CoveragePrunedNodes:           search.CoveragePrunedNodes,
		ExactBoundChecks:              search.ExactBoundChecks,
		ExactBoundPrunedNodes:         search.ExactBoundPrunedNodes,
		OutgoingBoundChecks:           search.OutgoingBoundChecks,
		OutgoingBoundPrunedNodes:      search.OutgoingBoundPrunedNodes,
		CoverageSeedNodes:             search.CoverageSeedNodes,
		CoverageSeedCandidates:        search.CoverageSeedCandidates,
		CoverageSeedBest:              search.CoverageSeedBest,
		StarSeedNodes:                 search.StarSeedNodes,
		StarSeedCandidates:            search.StarSeedCandidates,
		PackingSeedNodes:              search.PackingSeedNodes,
		PackingSeedCandidates:         search.PackingSeedCandidates,
		PackingSeedHardPruned:         search.PackingSeedHardPruned,
		PackingSeedStatesDeduplicated: search.PackingSeedStatesDeduplicated,
		SymmetryPrunedBranches:        search.SymmetryPrunedBranches,
		FirstCompletePhase:            search.FirstCompletePhase,
		FirstCompleteNodes:            search.FirstCompleteNodes,
		FirstCompleteMS:               search.FirstCompleteMS,
		SeedBest:                      scoreSummary(search.SeedBestScore),
		SearchBest:                    scoreSummary(search.SearchBestScore),
		PostRepairBest:                scoreSummary(search.PostRepairBestScore),
		RefineBest:                    scoreSummary(search.RefineBestScore),
		InitialBestPriorityCounts:     append([]int(nil), search.InitialBestPriorityCounts...),
		SeedBestPriorityCounts:        append([]int(nil), search.SeedBestPriorityCounts...),
		SearchBestPriorityCounts:      append([]int(nil), search.SearchBestPriorityCounts...),
		PostRepairBestPriorityCounts:  append([]int(nil), search.PostRepairBestPriorityCounts...),
		RefineBestPriorityCounts:      append([]int(nil), search.RefineBestPriorityCounts...),
		ParallelTasks:                 search.ParallelTasks,
		ParallelWorkersUsed:           search.ParallelWorkersUsed,
		RefineMovesChecked:            search.RefineMovesChecked,
		RefineImprovements:            search.RefineImprovements,
		RefineBestDelta:               search.RefineBestDelta,
		RepairNodes:                   search.RepairNodes,
		RepairIterations:              search.RepairIterations,
		RepairImprovements:            search.RepairImprovements,
		RepairCandidates:              search.RepairCandidates,
		RepairBest:                    search.RepairBest,
		RepairParallelTasks:           search.RepairParallelTasks,
		RepairParallelWorkersUsed:     search.RepairParallelWorkersUsed,
		StoppedAfterCoverageCeiling:   search.StoppedAfterCoverageCeiling,
		StoppedAfterPriorityCeiling:   search.StoppedAfterPriorityCeiling,
		DiagnosticsEnabled:            search.DiagnosticsEnabled,
		GlobalBudgetConsumed:          search.GlobalBudgetConsumed,
		UnusedGlobalNodes:             search.UnusedGlobalNodes,
		NormalBudgetConfigured:        search.NormalBudgetConfigured,
		NormalBudgetConsumed:          search.NormalBudgetConsumed,
		DiagnosticBudgetConfigured:    search.DiagnosticBudgetConfigured,
		DiagnosticBudgetConsumed:      search.DiagnosticBudgetConsumed,
		ExecutionBudgetConfigured:     search.ExecutionBudgetConfigured,
		ExecutionBudgetConsumed:       search.ExecutionBudgetConsumed,
		UnchargedWork:                 search.UnchargedWork,
		PhaseWork:                     diagnosticPhaseWork(search),
		IncumbentTrace:                append([]model.IncumbentEvent(nil), search.IncumbentTrace...),
		PriorityCeilingStats:          search.PriorityCeilingStats,
		Plateau:                       search.Plateau,
		StarUpperBounds:               search.StarUpperBounds,
		FirstFullyPackedPhase:         search.FirstFullyPackedPhase,
		FirstFullyPackedNodes:         search.FirstFullyPackedNodes,
		FirstFullyPackedMS:            search.FirstFullyPackedMS,
		PackingSeedDiagnostics:        search.PackingSeedDiagnostics,
		ConstellationSeedNodes:        search.ConstellationSeedNodes,
		ConstellationSeedCandidates:   search.ConstellationSeedCandidates,
		ConstellationSeedDiagnostics:  constellationSeedDiagnostics(search),
		ConfigFingerprint:             search.ConfigFingerprint,
		ExecutionFingerprint:          search.ExecutionFingerprint,
		Stages:                        diagnosticStages(search),
		TaskAllocation:                diagnosticTaskAllocation(search),
		PlateauArchive:                diagnosticPlateauArchive(search),
		PlateauLNSNodes:               search.PlateauLNSNodes,
		PlateauRefineNodes:            search.PlateauRefineNodes,
		PlateauRefineWalkLength:       search.PlateauRefineWalkLength,
		PlateauRefineMaxValley:        search.PlateauRefineMaxValley,
		PlateauRefineImproved:         search.PlateauRefineImproved,
	}
}

func constellationSeedDiagnostics(search model.SearchStats) *model.ConstellationSeedDiagnostics {
	diagnostics := search.ConstellationSeedDiagnostics
	if search.ConstellationSeedNodes == 0 && search.ConstellationSeedCandidates == 0 && reflect.ValueOf(diagnostics).IsZero() {
		return nil
	}
	return &diagnostics
}

func diagnosticPlateauArchive(search model.SearchStats) *model.PlateauArchiveStats {
	if !search.DiagnosticsEnabled {
		return nil
	}
	archive := search.PlateauArchive
	return &archive
}

func diagnosticPhaseWork(search model.SearchStats) []model.SearchPhaseWork {
	if !search.DiagnosticsEnabled {
		return nil
	}
	return append([]model.SearchPhaseWork(nil), search.PhaseWork...)
}

func diagnosticStages(search model.SearchStats) []model.SearchStageStats {
	if !search.DiagnosticsEnabled {
		return nil
	}
	return append([]model.SearchStageStats(nil), search.Stages...)
}

func diagnosticTaskAllocation(search model.SearchStats) *model.TaskAllocationStats {
	if !search.DiagnosticsEnabled {
		return nil
	}
	taskAllocation := search.TaskAllocation
	return &taskAllocation
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
			LinkCount:    priority.LinkCount,
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
	return model.CompareScores(scoreSummaryModelScore(left), scoreSummaryModelScore(right))
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
	return model.ComparePriorityCounts(left, right)
}

func scoreSummaryModelScore(summary ScoreSummary) model.Score {
	return model.Score{
		PriorityCounts:                append([]int(nil), summary.PriorityCounts...),
		CraftCount:                    summary.Crafts,
		StarCount:                     summary.Stars,
		ItemCount:                     summary.Items,
		StarTargetBreadth:             summary.StarTargetBreadth,
		StarReciprocalPairs:           summary.StarReciprocalPairs,
		StarSourceDefinitionDiversity: summary.StarSourceDefinitionDiversity,
	}
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
	return fmt.Sprintf("%s c%d s%d i%d t%d r%d d%d", priority, score.Crafts, score.Stars, score.Items, score.StarTargetBreadth, score.StarReciprocalPairs, score.StarSourceDefinitionDiversity)
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

func catalogSHA256(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}

func buildRevision() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			return setting.Value
		}
	}
	return "unknown"
}
