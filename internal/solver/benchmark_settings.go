package solver

// BenchmarkSettings exposes the bounded-search constants that affect a
// reproducible quality run. They are data only; benchmarks cannot alter them.
type BenchmarkSettings struct {
	CoverageSeedBeamWidth                                                         int                      `json:"coverage_seed_beam_width"`
	StarSeedBeamWidth                                                             int                      `json:"star_seed_beam_width"`
	PackingSeedBeamWidth                                                          int                      `json:"packing_seed_beam_width"`
	V3SeedNodeBudget                                                              int64                    `json:"v3_seed_node_budget"`
	PackingSeedNodeBudget                                                         int64                    `json:"packing_seed_node_budget"`
	RepairMaxNeighborhoodSize                                                     int                      `json:"repair_max_neighborhood_size"`
	RepairMaxNeighborhoods                                                        int                      `json:"repair_max_neighborhoods"`
	RepairEliteCandidates                                                         int                      `json:"repair_elite_candidates"`
	RepairMinNodeBudget                                                           int64                    `json:"repair_min_node_budget"`
	PlateauArchiveCapacity                                                        int                      `json:"plateau_archive_capacity"`
	PlateauDiagnosticSamples                                                      int                      `json:"plateau_diagnostic_samples"`
	PlateauLNSBudgetPercent                                                       int64                    `json:"plateau_lns_budget_percent"`
	PlateauRefineBudgetPercent                                                    int64                    `json:"plateau_refine_budget_percent"`
	PlateauLevels                                                                 []PlateauLevelPolicy     `json:"plateau_levels"`
	StageSettings                                                                 []BenchmarkStageSettings `json:"stage_settings"`
	ConstellationSeedVersion                                                      string                   `json:"constellation_seed_version,omitempty"`
	ConstellationSeedVariant                                                      string                   `json:"constellation_seed_variant,omitempty"`
	ConstellationSeedShareBps                                                     int64                    `json:"constellation_seed_share_bps,omitempty"`
	ConstellationSeedNodeBudget                                                   int64                    `json:"constellation_seed_node_budget,omitempty"`
	ConstellationSeedBeamWidth                                                    int                      `json:"constellation_seed_beam_width,omitempty"`
	ConstellationSeedPackingBeamWidth                                             int                      `json:"constellation_seed_packing_beam_width,omitempty"`
	ConstellationSeedPackingStrategy                                              string                   `json:"constellation_seed_packing_strategy,omitempty"`
	ConstellationRootPackingScheduler                                             string                   `json:"constellation_root_packing_scheduler,omitempty"`
	ConstellationRootPackingInitialQuantumDivisor                                 int64                    `json:"constellation_root_packing_initial_quantum_divisor,omitempty"`
	ConstellationRootPackingRoundQuantumDivisor                                   int64                    `json:"constellation_root_packing_round_quantum_divisor,omitempty"`
	ConstellationSeedMaxSkeletons                                                 int                      `json:"constellation_seed_max_skeletons,omitempty"`
	ConstellationFeasibilityProbe                                                 bool                     `json:"constellation_feasibility_probe,omitempty"`
	ConstellationCompletionOptimizationProbe                                      bool                     `json:"constellation_completion_optimization_probe,omitempty"`
	ConstellationCompletionOptimizationProbeScope                                 string                   `json:"constellation_completion_optimization_probe_scope,omitempty"`
	ConstellationCandidatePoolFeasibilitySweep                                    bool                     `json:"constellation_candidate_pool_feasibility_sweep,omitempty"`
	ConstellationCandidateCompletionOptimizationProbe                             bool                     `json:"constellation_candidate_completion_optimization_probe,omitempty"`
	ConstellationCandidateCompletionOptimizationCandidateID                       string                   `json:"constellation_candidate_completion_optimization_candidate_id,omitempty"`
	ConstellationCandidateCompletionOptimizationStage                             string                   `json:"constellation_candidate_completion_optimization_stage,omitempty"`
	ConstellationCandidateCompletionOptimizationNodeBudget                        int64                    `json:"constellation_candidate_completion_optimization_node_budget,omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey           string                   `json:"constellation_candidate_completion_optimization_initial_witness_layout_key,omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint string                   `json:"constellation_candidate_completion_optimization_initial_witness_semantic_fingerprint,omitempty"`
	ConstellationForcedCandidateRootedPackingProbe                                bool                     `json:"constellation_forced_candidate_rooted_packing_probe,omitempty"`
	ConstellationForcedCandidateRootedPackingCandidateID                          string                   `json:"constellation_forced_candidate_rooted_packing_candidate_id,omitempty"`
	ConstellationForcedCandidateRootedPackingSlot                                 int                      `json:"constellation_forced_candidate_rooted_packing_slot,omitempty"`
	ConstellationForcedCandidateRootedPackingStage                                string                   `json:"constellation_forced_candidate_rooted_packing_stage,omitempty"`
	ConstellationForcedCandidateRootedPackingBeamWidth                            int                      `json:"constellation_forced_candidate_rooted_packing_beam_width,omitempty"`
	ConstellationForcedCandidateRootedPackingRanking                              string                   `json:"constellation_forced_candidate_rooted_packing_ranking,omitempty"`
	ConstellationParentFrontierHedgeProbe                                         bool                     `json:"constellation_parent_frontier_hedge_probe,omitempty"`
	ConstellationParentFrontierHedgeProbeStage                                    string                   `json:"constellation_parent_frontier_hedge_probe_stage,omitempty"`
	PlateauWalkDepth                                                              int                      `json:"plateau_walk_depth"`
	UnchargedCompletionMoves                                                      int64                    `json:"uncharged_completion_moves"`
	UnchargedRefineMoves                                                          int64                    `json:"uncharged_refine_moves"`
}

// BenchmarkStageSettings describes the policy actually used by one scheduled
// search stage.
type BenchmarkStageSettings struct {
	ID             string               `json:"id"`
	NodeLimit      int64                `json:"node_limit"`
	PlateauVariant string               `json:"plateau_variant"`
	PlateauLevels  []PlateauLevelPolicy `json:"plateau_levels"`
}

func SettingsForBenchmark(maxNodes int64, plateauVariants ...string) BenchmarkSettings {
	plateauVariant := ""
	if len(plateauVariants) > 0 {
		plateauVariant = plateauVariants[0]
	}
	return SettingsForBenchmarkConfig(Config{MaxNodes: maxNodes, PlateauVariant: plateauVariant})
}

// SettingsForBenchmarkConfig reports the policy actually resolved for a
// benchmark solve, including opt-in seed experiments.
func SettingsForBenchmarkConfig(config Config) BenchmarkSettings {
	policy := resolveSearchPolicy(config, config.MaxNodes)
	stages := configuredSearchStages(config)
	stageSettings := make([]BenchmarkStageSettings, 0, len(stages))
	for _, stage := range stages {
		stageSettings = append(stageSettings, BenchmarkStageSettings{
			ID:             stage.ID,
			NodeLimit:      stage.NodeLimit,
			PlateauVariant: effectivePlateauVariant(stage.Policy.PlateauVariant, stage.NodeLimit),
			PlateauLevels:  append([]PlateauLevelPolicy(nil), stage.Policy.PlateauLevels...),
		})
	}
	v3SeedBudget := policy.StarSeedNodeBudget
	packingBudget := policy.PackingSeedNodeBudget
	return BenchmarkSettings{
		CoverageSeedBeamWidth:                                               coverageSeedBeamWidth,
		StarSeedBeamWidth:                                                   policy.StarSeedBeamWidth,
		PackingSeedBeamWidth:                                                packingSeedBeamWidth(packingBudget),
		V3SeedNodeBudget:                                                    v3SeedBudget,
		PackingSeedNodeBudget:                                               packingBudget,
		RepairMaxNeighborhoodSize:                                           repairMaxNeighborhoodSize,
		RepairMaxNeighborhoods:                                              repairMaxNeighborhoods,
		RepairEliteCandidates:                                               repairEliteCandidateLimit,
		RepairMinNodeBudget:                                                 repairMinNodeBudget,
		PlateauArchiveCapacity:                                              plateauArchiveCapacity,
		PlateauDiagnosticSamples:                                            plateauDiagnosticSampleLimit,
		PlateauLNSBudgetPercent:                                             plateauLNSBudgetPercent,
		PlateauRefineBudgetPercent:                                          plateauRefineBudgetPercent,
		PlateauLevels:                                                       append([]PlateauLevelPolicy(nil), policy.PlateauLevels...),
		StageSettings:                                                       stageSettings,
		ConstellationSeedVersion:                                            policy.ConstellationSeedVersion,
		ConstellationSeedVariant:                                            policy.ConstellationSeedVariant,
		ConstellationSeedShareBps:                                           policy.ConstellationSeedShareBps,
		ConstellationSeedNodeBudget:                                         policy.ConstellationSeedNodeBudget,
		ConstellationSeedBeamWidth:                                          policy.ConstellationSeedBeamWidth,
		ConstellationSeedPackingBeamWidth:                                   policy.ConstellationSeedPackingBeamWidth,
		ConstellationSeedPackingStrategy:                                    policy.ConstellationSeedPackingStrategy,
		ConstellationRootPackingScheduler:                                   policy.ConstellationRootPackingScheduler,
		ConstellationRootPackingInitialQuantumDivisor:                       policy.ConstellationRootPackingInitialQuantumDivisor,
		ConstellationRootPackingRoundQuantumDivisor:                         policy.ConstellationRootPackingRoundQuantumDivisor,
		ConstellationSeedMaxSkeletons:                                       policy.ConstellationSeedMaxSkeletons,
		ConstellationFeasibilityProbe:                                       policy.ConstellationFeasibilityProbe,
		ConstellationCompletionOptimizationProbe:                            policy.ConstellationCompletionOptimizationProbe,
		ConstellationCompletionOptimizationProbeScope:                       policy.ConstellationCompletionOptimizationProbeScope,
		ConstellationCandidatePoolFeasibilitySweep:                          policy.ConstellationCandidatePoolFeasibilitySweep,
		ConstellationCandidateCompletionOptimizationProbe:                   policy.ConstellationCandidateCompletionOptimizationProbe,
		ConstellationCandidateCompletionOptimizationCandidateID:             policy.ConstellationCandidateCompletionOptimizationCandidateID,
		ConstellationCandidateCompletionOptimizationStage:                   policy.ConstellationCandidateCompletionOptimizationStage,
		ConstellationCandidateCompletionOptimizationNodeBudget:              policy.ConstellationCandidateCompletionOptimizationNodeBudget,
		ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey: policy.ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey,
		ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint: policy.ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint,
		ConstellationForcedCandidateRootedPackingProbe:                                policy.ConstellationForcedCandidateRootedPackingProbe,
		ConstellationForcedCandidateRootedPackingCandidateID:                          policy.ConstellationForcedCandidateRootedPackingCandidateID,
		ConstellationForcedCandidateRootedPackingSlot:                                 policy.ConstellationForcedCandidateRootedPackingSlot,
		ConstellationForcedCandidateRootedPackingStage:                                policy.ConstellationForcedCandidateRootedPackingStage,
		ConstellationForcedCandidateRootedPackingBeamWidth:                            policy.ConstellationForcedCandidateRootedPackingBeamWidth,
		ConstellationForcedCandidateRootedPackingRanking:                              policy.ConstellationForcedCandidateRootedPackingRanking,
		ConstellationParentFrontierHedgeProbe:                                         policy.ConstellationParentFrontierHedgeProbe,
		ConstellationParentFrontierHedgeProbeStage:                                    policy.ConstellationParentFrontierHedgeProbeStage,
		PlateauWalkDepth:         plateauWalkMaxDepth,
		UnchargedCompletionMoves: completionMoveLimit(defaultRefineMoveLimit(config.MaxNodes)),
		UnchargedRefineMoves:     defaultRefineMoveLimit(config.MaxNodes),
	}
}
