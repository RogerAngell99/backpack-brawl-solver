package solver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"backpack-brawl-solver/internal/model"
)

const (
	defaultMinAllocatedNodesPerTask int64 = 128
	defaultMaxTasksPerWorker              = 256
	defaultInitialSplitDepth              = 2

	PlateauVariantLegacyLargeOff = "legacy-large-off"
	PlateauVariantLarge16        = "large-16"
	PlateauVariantLarge1618      = "large-16-18"
	PlateauVariantLarge161820    = "large-16-18-20"
	// Deep repair remains benchmark opt-in until it beats the legacy profile.
	DefaultPlateauVariant = PlateauVariantLegacyLargeOff

	ConstellationSeedVariantV1  = "v1"
	ConstellationSeedVariantV2  = "v2"
	ConstellationSeedVariantV3  = "v3"
	ConstellationSeedVariantV4  = "v4"
	ConstellationSeedVariantV5  = "v5"
	ConstellationSeedVariantV51 = "v5.1"
	// ConstellationSeedVariantGeneralSearchV1 preserves V4 root construction
	// and rooted packing, but allocates the selected roots progressively.
	ConstellationSeedVariantGeneralSearchV1 = "general-search-v1"

	constellationPackingStrategyFixedOrder  = "fixed_order"
	constellationPackingStrategyStateMRV    = "state_mrv"
	constellationOptimizationScopeFirstTwo  = "first_two_completed"
	constellationOptimizationScopeAll       = "all_completed"
	constellationRootSchedulerProgressiveV1 = "progressive_round_robin_v1"
)

// PlateauLevelPolicy resolves one deterministic plateau LNS neighborhood band.
// A zero maximum mandatory size or selection cap is unbounded.
type PlateauLevelPolicy struct {
	MaxNeighborhoodSize int   `json:"max_neighborhood_size"`
	MinMandatorySize    int   `json:"min_mandatory_size"`
	MaxMandatorySize    int   `json:"max_mandatory_size"`
	QuotaBps            int64 `json:"quota_bps"`
	MaxSelected         int   `json:"max_selected"`
	MaxSelectedPerBase  int   `json:"max_selected_per_base"`
	MinNodeBudget       int64 `json:"min_node_budget"`
}

// ResolvedSearchPolicy contains every budget-derived decision used by one
// bounded search stage. Keeping it independent from Config.MaxNodes makes a
// stage reproducible when it is embedded in a larger execution.
type ResolvedSearchPolicy struct {
	NodeLimit                                                                     int64
	CandidateLimit                                                                int
	TopN                                                                          int
	RefineMoveLimit                                                               int64
	CompletionMoveLimit                                                           int64
	CoverageSeedNodeBudget                                                        int64
	StarSeedNodeBudget                                                            int64
	PackingSeedNodeBudget                                                         int64
	ConstellationSeedVersion                                                      string `json:",omitempty"`
	ConstellationSeedVariant                                                      string `json:",omitempty"`
	ConstellationSeedShareBps                                                     int64  `json:",omitempty"`
	ConstellationSeedNodeBudget                                                   int64  `json:",omitempty"`
	ConstellationSeedBeamWidth                                                    int    `json:",omitempty"`
	ConstellationSeedMaxSkeletons                                                 int    `json:",omitempty"`
	ConstellationSeedConstructionBps                                              int64  `json:",omitempty"`
	ConstellationSeedSourceOptionLimit                                            int    `json:",omitempty"`
	ConstellationSeedTargetOptionLimit                                            int    `json:",omitempty"`
	ConstellationSeedTargetInstanceLimit                                          int    `json:",omitempty"`
	ConstellationSeedPackingBeamWidth                                             int    `json:",omitempty"`
	ConstellationSeedPackingStrategy                                              string `json:",omitempty"`
	ConstellationRootPackingScheduler                                             string `json:",omitempty"`
	ConstellationRootPackingInitialQuantumDivisor                                 int64  `json:",omitempty"`
	ConstellationRootPackingRoundQuantumDivisor                                   int64  `json:",omitempty"`
	ConstellationSeedMaxSourceGeometries                                          int    `json:",omitempty"`
	ConstellationSeedMaxPerSourceGeometry                                         int    `json:",omitempty"`
	ConstellationSeedSourceGeometryBeamCount                                      int    `json:",omitempty"`
	ConstellationFeasibilityProbe                                                 bool   `json:",omitempty"`
	ConstellationCompletionOptimizationProbe                                      bool   `json:",omitempty"`
	ConstellationCompletionOptimizationProbeScope                                 string `json:",omitempty"`
	ConstellationCandidatePoolFeasibilitySweep                                    bool   `json:",omitempty"`
	ConstellationCandidateCompletionOptimizationProbe                             bool   `json:",omitempty"`
	ConstellationCandidateCompletionOptimizationCandidateID                       string `json:",omitempty"`
	ConstellationCandidateCompletionOptimizationStage                             string `json:",omitempty"`
	ConstellationCandidateCompletionOptimizationNodeBudget                        int64  `json:",omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey           string `json:",omitempty"`
	ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint string `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingProbe                                bool   `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingCandidateID                          string `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingSlot                                 int    `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingStage                                string `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingBeamWidth                            int    `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingRanking                              string `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey               string `json:",omitempty"`
	ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint     string `json:",omitempty"`
	ConstellationParentFrontierHedgeProbe                                         bool   `json:",omitempty"`
	ConstellationParentFrontierHedgeProbeStage                                    string `json:",omitempty"`
	CoverageSeedBeamWidth                                                         int
	StarSeedBeamWidth                                                             int
	PackingSeedBeamWidth                                                          int
	RepairBudgetPercent                                                           int64
	RepairMinNodeBudget                                                           int64
	RepairMaxNeighborhoodSize                                                     int
	RepairMaxNeighborhoods                                                        int
	RepairEliteCandidates                                                         int
	RepairMaxSplitDepth                                                           int
	RepairMaxTasks                                                                int
	RepairMinParallelBudget                                                       int64
	RepairTargetTasksPerWorker                                                    int
	MinAllocatedNodesPerTask                                                      int64
	MaxTasksPerWorker                                                             int
	MaxInitialSplitDepth                                                          int
	PlateauArchiveCapacity                                                        int
	PlateauLNSBudgetPercent                                                       int64
	PlateauRefineBudgetPercent                                                    int64
	PlateauVariant                                                                string
	PlateauLevels                                                                 []PlateauLevelPolicy
	PlateauWalkDepth                                                              int
	StopOnCoverageCeiling                                                         bool
	StopOnPriorityCeiling                                                         bool
	AllowSkips                                                                    bool
	RepairSearch                                                                  bool
	DisableExactBounds                                                            bool
	DisableOutgoingBounds                                                         bool
	PrioritySemantics                                                             string
	Priorities                                                                    []string
	CoverageGroups                                                                []model.CoverageGroup
}

func resolveSearchPolicy(config Config, nodeLimit int64) ResolvedSearchPolicy {
	policy := ResolvedSearchPolicy{
		NodeLimit:                                  nodeLimit,
		CandidateLimit:                             candidateLimit(config.TopN),
		TopN:                                       config.TopN,
		RefineMoveLimit:                            config.MaxRefineMoves,
		CoverageSeedBeamWidth:                      coverageSeedBeamWidth,
		RepairBudgetPercent:                        repairBudgetPercent,
		RepairMinNodeBudget:                        repairMinNodeBudget,
		RepairMaxNeighborhoodSize:                  repairMaxNeighborhoodSize,
		RepairMaxNeighborhoods:                     repairMaxNeighborhoods,
		RepairEliteCandidates:                      repairEliteCandidateLimit,
		RepairMaxSplitDepth:                        repairMaxSplitDepth,
		RepairMaxTasks:                             repairMaxTasks,
		RepairMinParallelBudget:                    repairMinParallelBudget,
		RepairTargetTasksPerWorker:                 repairTargetTasksPerWorker,
		MinAllocatedNodesPerTask:                   defaultMinAllocatedNodesPerTask,
		MaxTasksPerWorker:                          defaultMaxTasksPerWorker,
		MaxInitialSplitDepth:                       defaultInitialSplitDepth,
		PlateauArchiveCapacity:                     plateauArchiveCapacity,
		PlateauLNSBudgetPercent:                    plateauLNSBudgetPercent,
		PlateauRefineBudgetPercent:                 plateauRefineBudgetPercent,
		PlateauVariant:                             resolvedPlateauVariant(config.PlateauVariant),
		PlateauWalkDepth:                           plateauWalkMaxDepth,
		StopOnCoverageCeiling:                      config.StopOnCoverageCeiling && config.TopN == 1,
		StopOnPriorityCeiling:                      config.StopOnPriorityCeiling,
		AllowSkips:                                 config.AllowSkips,
		RepairSearch:                               config.RepairSearch,
		DisableExactBounds:                         config.DisableExactBounds,
		DisableOutgoingBounds:                      config.DisableOutgoingBounds,
		ConstellationFeasibilityProbe:              config.ConstellationFeasibilityProbe,
		ConstellationCompletionOptimizationProbe:   config.ConstellationCompletionOptimizationProbe,
		ConstellationCandidatePoolFeasibilitySweep: config.ConstellationCandidatePoolFeasibilitySweep,
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
		PrioritySemantics: string(config.PrioritySemantics),
		Priorities:        append([]string(nil), config.Priorities...),
		CoverageGroups:    append([]model.CoverageGroup(nil), config.CoverageGroups...),
	}
	policy.PlateauLevels = resolvedPlateauLevelsForVariant(nodeLimit, policy.PlateauVariant)
	if policy.RefineMoveLimit <= 0 {
		policy.RefineMoveLimit = defaultRefineMoveLimit(nodeLimit)
	}
	policy.CompletionMoveLimit = completionMoveLimit(policy.RefineMoveLimit)
	policy.CoverageSeedNodeBudget = seedNodeBudget(nodeLimit)
	policy.StarSeedNodeBudget = policy.CoverageSeedNodeBudget
	if config.PrioritySemantics == "outgoing-per-instance-v3" {
		policy.StarSeedNodeBudget = perInstanceSeedNodeBudget(nodeLimit)
		policy.PackingSeedNodeBudget = policy.StarSeedNodeBudget / 2
		if constellationSeedEligible(config, nodeLimit) {
			policy.ConstellationSeedVariant = resolvedConstellationSeedVariant(config)
			policy.ConstellationSeedVersion = policy.ConstellationSeedVariant
			policy.ConstellationSeedPackingStrategy = constellationPackingStrategyFixedOrder
			if constellationSeedUsesStateMRV(policy.ConstellationSeedVariant) {
				policy.ConstellationSeedPackingStrategy = constellationPackingStrategyStateMRV
			}
			if policy.ConstellationCompletionOptimizationProbe {
				if policy.ConstellationSeedVariant == ConstellationSeedVariantV4 || policy.ConstellationSeedVariant == ConstellationSeedVariantGeneralSearchV1 || policy.ConstellationSeedVariant == ConstellationSeedVariantV5 || policy.ConstellationSeedVariant == ConstellationSeedVariantV51 {
					policy.ConstellationCompletionOptimizationProbeScope = constellationOptimizationScopeAll
				} else {
					policy.ConstellationCompletionOptimizationProbeScope = constellationOptimizationScopeFirstTwo
				}
			}
			policy.ConstellationSeedShareBps = 1_500
			policy.ConstellationSeedNodeBudget = policy.StarSeedNodeBudget * policy.ConstellationSeedShareBps / 10_000
			if policy.ConstellationSeedNodeBudget < 1 && policy.StarSeedNodeBudget >= 10 {
				policy.ConstellationSeedNodeBudget = 1
			}
			// The packing seed consistently leaves substantial reserve unused on
			// this no-skips profile. Fund V1 from that reserve so the established
			// directed star seed retains its legacy allocation.
			policy.PackingSeedNodeBudget -= policy.ConstellationSeedNodeBudget
			if policy.PackingSeedNodeBudget < 0 {
				policy.PackingSeedNodeBudget = 0
			}
			policy.ConstellationSeedBeamWidth = 24
			policy.ConstellationSeedMaxSkeletons = 4
			policy.ConstellationSeedConstructionBps = 6_000
			policy.ConstellationSeedSourceOptionLimit = 0
			policy.ConstellationSeedTargetOptionLimit = 12
			policy.ConstellationSeedTargetInstanceLimit = 12
			policy.ConstellationSeedPackingBeamWidth = packingSeedBeamWidth(policy.PackingSeedNodeBudget)
			if policy.ConstellationSeedVariant == ConstellationSeedVariantV5 || policy.ConstellationSeedVariant == ConstellationSeedVariantV51 {
				policy.ConstellationSeedPackingBeamWidth = 128
			}
			if constellationSeedUsesProgressiveRootScheduler(policy.ConstellationSeedVariant) {
				policy.ConstellationRootPackingScheduler = constellationRootSchedulerProgressiveV1
				policy.ConstellationRootPackingInitialQuantumDivisor = 2
				policy.ConstellationRootPackingRoundQuantumDivisor = 2
			}
			policy.ConstellationSeedMaxSourceGeometries = 4
			policy.ConstellationSeedMaxPerSourceGeometry = 1
			policy.ConstellationSeedSourceGeometryBeamCount = 24
		}
	}
	policy.StarSeedBeamWidth = starSeedBeamWidth(policy.StarSeedNodeBudget - policy.PackingSeedNodeBudget - policy.ConstellationSeedNodeBudget)
	policy.PackingSeedBeamWidth = packingSeedBeamWidth(policy.PackingSeedNodeBudget)
	return policy
}

func constellationSeedEligible(config Config, nodeLimit int64) bool {
	if !constellationSeedEnabled(config) || nodeLimit <= 0 || config.AllowSkips || config.PrioritySemantics != model.PrioritySemanticsOutgoingPerInstanceV3 || len(config.CoverageGroups) != 0 || len(config.Priorities) != 2 {
		return false
	}
	for _, priority := range config.Priorities {
		kind, _, ok := parsePriorityForSolver(priority)
		if !ok || kind != "star_source" {
			return false
		}
	}
	return true
}

func constellationSeedEnabled(config Config) bool {
	return config.EnableConstellationSeedV1 || config.ConstellationSeedVariant != ""
}

// ValidateConstellationSeedVariant verifies the explicit experiment selector.
func ValidateConstellationSeedVariant(variant string) error {
	switch variant {
	case ConstellationSeedVariantV1, ConstellationSeedVariantV2, ConstellationSeedVariantV3, ConstellationSeedVariantV4, ConstellationSeedVariantV5, ConstellationSeedVariantV51, ConstellationSeedVariantGeneralSearchV1:
		return nil
	default:
		return fmt.Errorf("constellation seed variant must be %q, %q, %q, %q, %q, %q, or %q", ConstellationSeedVariantV1, ConstellationSeedVariantV2, ConstellationSeedVariantV3, ConstellationSeedVariantV4, ConstellationSeedVariantV5, ConstellationSeedVariantV51, ConstellationSeedVariantGeneralSearchV1)
	}
}

func constellationSeedUsesSourceGeometry(variant string) bool {
	return variant == ConstellationSeedVariantV2 || variant == ConstellationSeedVariantV3 || variant == ConstellationSeedVariantV4 || variant == ConstellationSeedVariantV5 || variant == ConstellationSeedVariantV51 || variant == ConstellationSeedVariantGeneralSearchV1
}

func constellationSeedUsesStateMRV(variant string) bool {
	return variant == ConstellationSeedVariantV3 || variant == ConstellationSeedVariantV4 || variant == ConstellationSeedVariantV5 || variant == ConstellationSeedVariantV51 || variant == ConstellationSeedVariantGeneralSearchV1
}

func constellationSeedUsesOrbitDiversity(variant string) bool {
	return variant == ConstellationSeedVariantV4 || variant == ConstellationSeedVariantV5 || variant == ConstellationSeedVariantV51 || variant == ConstellationSeedVariantGeneralSearchV1
}

func completionOptimizationProbeVariantEligible(variant string) bool {
	return variant == ConstellationSeedVariantV3 || variant == ConstellationSeedVariantV4 || variant == ConstellationSeedVariantV5 || variant == ConstellationSeedVariantV51 || variant == ConstellationSeedVariantGeneralSearchV1
}

func constellationSeedUsesProgressiveRootScheduler(variant string) bool {
	return variant == ConstellationSeedVariantGeneralSearchV1
}

// ValidateConstellationCompletionOptimizationProbeVariant verifies that the
// exact completion diagnostic runs only with an MRV constellation variant.
func ValidateConstellationCompletionOptimizationProbeVariant(variant string) error {
	if completionOptimizationProbeVariantEligible(variant) {
		return nil
	}
	return fmt.Errorf("constellation completion optimization probe requires constellation seed variant %q, %q, %q, %q, or %q", ConstellationSeedVariantV3, ConstellationSeedVariantV4, ConstellationSeedVariantV5, ConstellationSeedVariantV51, ConstellationSeedVariantGeneralSearchV1)
}

// ValidateConstellationCandidateCompletionOptimizationTarget verifies the
// stage-local V4 candidate target supplied to the exact diagnostic.
func ValidateConstellationCandidateCompletionOptimizationTarget(variant string, candidateID string, stageID string) error {
	if variant != ConstellationSeedVariantV4 {
		return fmt.Errorf("constellation candidate completion optimization probe requires constellation seed variant %q", ConstellationSeedVariantV4)
	}
	if len(candidateID) != sha256.Size*2 || candidateID != strings.ToLower(candidateID) {
		return fmt.Errorf("constellation candidate completion optimization candidate id must be a lowercase SHA-256 hex string")
	}
	if _, err := hex.DecodeString(candidateID); err != nil {
		return fmt.Errorf("constellation candidate completion optimization candidate id must be a lowercase SHA-256 hex string")
	}
	switch stageID {
	case "", "single", "prefix-5m", "remainder-15m":
		return nil
	default:
		return fmt.Errorf("constellation candidate completion optimization stage must be empty, %q, %q, or %q", "single", "prefix-5m", "remainder-15m")
	}
}

// ValidateConstellationForcedCandidateRootedPackingTarget verifies the V4
// counterfactual rooted-packing replay target.
func ValidateConstellationForcedCandidateRootedPackingTarget(variant string, candidateID string, slot int, stageID string) error {
	if err := ValidateConstellationCandidateCompletionOptimizationTarget(variant, candidateID, stageID); err != nil {
		return err
	}
	if slot < 1 || slot > 4 {
		return fmt.Errorf("constellation forced candidate rooted packing slot must be 1 through 4")
	}
	return nil
}

func ValidateConstellationForcedCandidateRootedPackingRanking(ranking string) error {
	switch ranking {
	case "", constellationRootPackingRankingBaseline, constellationRootPackingRankingPriorityScoreFirst:
		return nil
	default:
		return fmt.Errorf("constellation forced candidate rooted packing ranking must be %q or %q", constellationRootPackingRankingBaseline, constellationRootPackingRankingPriorityScoreFirst)
	}
}

func ValidateConstellationParentFrontierHedgeProbeTarget(variant string, stageID string) error {
	if variant != ConstellationSeedVariantV5 {
		return fmt.Errorf("constellation parent-frontier hedge probe requires constellation seed variant %q", ConstellationSeedVariantV5)
	}
	switch stageID {
	case "", "single", "prefix-5m", "remainder-15m":
		return nil
	default:
		return fmt.Errorf("constellation parent-frontier hedge probe stage must be empty, %q, %q, or %q", "single", "prefix-5m", "remainder-15m")
	}
}

func resolvedConstellationForcedCandidateRootedPackingRanking(ranking string) string {
	if ranking == "" {
		return constellationRootPackingRankingBaseline
	}
	if err := ValidateConstellationForcedCandidateRootedPackingRanking(ranking); err != nil {
		panic(err)
	}
	return ranking
}

func resolvedConstellationSeedVariant(config Config) string {
	variant := config.ConstellationSeedVariant
	if variant == "" {
		if config.EnableConstellationSeedV1 {
			return ConstellationSeedVariantV1
		}
		return ""
	}
	if err := ValidateConstellationSeedVariant(variant); err != nil {
		panic(err)
	}
	if config.EnableConstellationSeedV1 && variant != ConstellationSeedVariantV1 {
		panic("constellation seed v1 alias conflicts with explicit variant")
	}
	return variant
}

func resolvedPlateauLevels(nodeLimit int64) []PlateauLevelPolicy {
	return resolvedPlateauLevelsForVariant(nodeLimit, resolvedPlateauVariant(DefaultPlateauVariant))
}

func resolvedPlateauLevelsForVariant(nodeLimit int64, variant string) []PlateauLevelPolicy {
	levels := []PlateauLevelPolicy{
		{MaxNeighborhoodSize: 8, QuotaBps: 2500},
		{MaxNeighborhoodSize: 9, QuotaBps: 2500},
		{MaxNeighborhoodSize: 10, QuotaBps: 2500},
		{MaxNeighborhoodSize: 12, QuotaBps: 2500},
	}
	if nodeLimit < 15_000_000 {
		return levels
	}
	deepLevels := []PlateauLevelPolicy{
		{MaxNeighborhoodSize: 16, MinMandatorySize: 13, MaxMandatorySize: 16, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
		{MaxNeighborhoodSize: 18, MinMandatorySize: 17, MaxMandatorySize: 18, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
		{MaxNeighborhoodSize: 20, MinMandatorySize: 19, MaxMandatorySize: 20, QuotaBps: 40, MaxSelected: 1, MaxSelectedPerBase: 1, MinNodeBudget: 5_000},
	}
	deepCount := 0
	switch variant {
	case PlateauVariantLegacyLargeOff:
		return levels
	case PlateauVariantLarge16:
		deepCount = 1
	case PlateauVariantLarge1618:
		deepCount = 2
	case PlateauVariantLarge161820:
		deepCount = 3
	default:
		panic(fmt.Sprintf("invalid plateau variant %q", variant))
	}
	for index := range levels {
		levels[index].QuotaBps = 2500 - int64(deepCount)*10
	}
	return append(levels, deepLevels[:deepCount]...)
}

// ValidatePlateauVariant verifies a named plateau LNS policy selection.
func ValidatePlateauVariant(variant string) error {
	switch variant {
	case PlateauVariantLegacyLargeOff, PlateauVariantLarge16, PlateauVariantLarge1618, PlateauVariantLarge161820:
		return nil
	default:
		return fmt.Errorf("plateau variant must be one of %q, %q, %q, %q", PlateauVariantLegacyLargeOff, PlateauVariantLarge16, PlateauVariantLarge1618, PlateauVariantLarge161820)
	}
}

func resolvedPlateauVariant(variant string) string {
	if variant == "" {
		variant = DefaultPlateauVariant
	}
	if err := ValidatePlateauVariant(variant); err != nil {
		panic(err)
	}
	return variant
}

func effectivePlateauVariant(variant string, nodeLimit int64) string {
	if nodeLimit < 15_000_000 {
		return PlateauVariantLegacyLargeOff
	}
	return variant
}

func policyForConfig(config Config) ResolvedSearchPolicy {
	if config.policy != nil {
		return *config.policy
	}
	return resolveSearchPolicy(config, config.MaxNodes)
}

func resolvedPolicyFingerprint(policy ResolvedSearchPolicy) string {
	encoded, err := json.Marshal(policy)
	if err != nil {
		panic(fmt.Sprintf("marshal resolved search policy: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func resolvedExecutionFingerprint(config Config, stages []SearchStage) string {
	type stageFingerprint struct {
		ID        string `json:"id"`
		NodeLimit int64  `json:"node_limit"`
		Policy    string `json:"stage_policy_fingerprint"`
	}
	value := struct {
		MaxNodes int64              `json:"max_nodes"`
		Workers  int                `json:"workers"`
		Stages   []stageFingerprint `json:"stages"`
	}{
		MaxNodes: config.MaxNodes,
		Workers:  config.Workers,
		Stages:   make([]stageFingerprint, 0, len(stages)),
	}
	for _, stage := range stages {
		value.Stages = append(value.Stages, stageFingerprint{
			ID:        stage.ID,
			NodeLimit: stage.NodeLimit,
			Policy:    resolvedPolicyFingerprint(stage.Policy),
		})
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal search execution: %v", err))
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

// SearchStage deliberately carries a fully resolved policy. Future schedules
// can therefore compose 1M -> 5M -> 20M without changing stage semantics.
type SearchStage struct {
	ID        string
	NodeLimit int64
	Policy    ResolvedSearchPolicy
}

func configuredSearchStages(config Config) []SearchStage {
	if config.MaxNodes != 20_000_000 {
		return []SearchStage{{
			ID:        "single",
			NodeLimit: config.MaxNodes,
			Policy:    resolveSearchPolicy(config, config.MaxNodes),
		}}
	}
	return []SearchStage{
		{ID: "prefix-5m", NodeLimit: 5_000_000, Policy: resolveSearchPolicy(config, 5_000_000)},
		{ID: "remainder-15m", NodeLimit: 15_000_000, Policy: resolveSearchPolicy(config, 15_000_000)},
	}
}
