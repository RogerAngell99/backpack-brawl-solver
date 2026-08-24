package model

import "fmt"

type PrioritySemantics string

const (
	PrioritySemanticsLegacyIncomingV1      PrioritySemantics = "legacy-incoming-v1"
	PrioritySemanticsOutgoingV2            PrioritySemantics = "outgoing-v2"
	PrioritySemanticsOutgoingPerInstanceV3 PrioritySemantics = "outgoing-per-instance-v3"
)

func (semantics PrioritySemantics) IsOutgoing() bool {
	return semantics == PrioritySemanticsOutgoingV2 || semantics == PrioritySemanticsOutgoingPerInstanceV3
}

type Coord struct {
	Row int
	Col int
}

func (c Coord) String() string {
	return fmt.Sprintf("(%d, %d)", c.Row, c.Col)
}

type Star struct {
	Offset            Coord
	TargetTypes       []string
	TargetItems       []string
	ExcludeSourceItem bool
	RuleStatus        string
	EffectText        string

	CompiledTargetTypeMask      uint32 `json:"-"`
	CompiledTargetItemID        uint16 `json:"-"`
	CompiledTargetItemLen       uint8  `json:"-"`
	CompiledReady               bool   `json:"-"`
	CompiledTargetItemsComplete bool   `json:"-"`
}

type StarCondition struct {
	Class        string          `json:"class"`
	Any          bool            `json:"any,omitempty"`
	Conditions   []StarCondition `json:"conditions,omitempty"`
	ItemType     string          `json:"item_type,omitempty"`
	StatType     string          `json:"stat_type,omitempty"`
	DefinitionID string          `json:"definition_id,omitempty"`
	Definition   *ItemDefinition `json:"definition,omitempty"`
	PlayerStat   any             `json:"player_stat,omitempty"`
	Mode         any             `json:"mode,omitempty"`
}

type ItemDefinition struct {
	Class string `json:"class,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
}

type Hero struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	EnglishName string `json:"english_name,omitempty"`
	NPC         bool   `json:"npc,omitempty"`
}

type HeroScope struct {
	AvailableTo []string `json:"available_to,omitempty"`
	Kind        string   `json:"kind"`
	Status      string   `json:"status"`
	Source      string   `json:"source,omitempty"`
}

type HeroFilter struct {
	IncludeHeroes []string `json:"include_heroes,omitempty"`
	ExcludeHeroes []string `json:"exclude_heroes,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	ExcludeMode   string   `json:"exclude_mode,omitempty"`
	UnknownPolicy string   `json:"unknown_policy,omitempty"`
}

type Item struct {
	ID            string
	Name          string
	Types         []string
	Shape         []Coord
	Stars         []Star
	StatTypes     []string
	StarCondition *StarCondition `json:"star_condition_graph,omitempty"`
	HeroScope     *HeroScope     `json:"hero_scope,omitempty"`
	CountsAs      []ItemAlias
	AbilityText   string
	SourceURL     string
	ImageURL      string
	ImagePath     string
	NeedsReview   bool
	Rotations     []int

	CompiledTypeMask         uint32 `json:"-"`
	CompiledItemID           uint16 `json:"-"`
	CompiledAliasItemID      uint16 `json:"-"`
	CompiledAliasItemLen     uint8  `json:"-"`
	CompiledItemRefLen       uint8  `json:"-"`
	CompiledReady            bool   `json:"-"`
	CompiledItemRefsComplete bool   `json:"-"`
}

type ItemAlias struct {
	ItemID string
	Count  int
}

type Recipe struct {
	Result               string
	Anchor               string
	Ingredients          []string
	SourceURL            string
	HeroScope            *HeroScope
	CompiledRequirements RecipeRequirements `json:"-"`
}

type RecipeRequirements struct {
	Items  [16]string
	Counts [16]int
	Len    int
	Ready  bool
}

func BuildRecipeRequirements(anchor string, ingredients []string) RecipeRequirements {
	var requirements RecipeRequirements
	for _, ingredient := range ingredients {
		requirements.add(ingredient, 1)
	}
	requirements.add(anchor, -1)
	requirements.compactAndSort()
	requirements.Ready = true
	return requirements
}

func (requirements *RecipeRequirements) add(itemID string, count int) {
	for idx := 0; idx < requirements.Len; idx++ {
		if requirements.Items[idx] == itemID {
			requirements.Counts[idx] += count
			return
		}
	}
	requirements.Items[requirements.Len] = itemID
	requirements.Counts[requirements.Len] = count
	requirements.Len++
}

func (requirements *RecipeRequirements) compactAndSort() {
	writeIndex := 0
	for readIndex := 0; readIndex < requirements.Len; readIndex++ {
		if requirements.Counts[readIndex] <= 0 {
			continue
		}
		requirements.Items[writeIndex] = requirements.Items[readIndex]
		requirements.Counts[writeIndex] = requirements.Counts[readIndex]
		writeIndex++
	}
	requirements.Len = writeIndex
	for idx := 1; idx < requirements.Len; idx++ {
		itemID := requirements.Items[idx]
		count := requirements.Counts[idx]
		insertAt := idx
		for insertAt > 0 && requirements.Items[insertAt-1] > itemID {
			requirements.Items[insertAt] = requirements.Items[insertAt-1]
			requirements.Counts[insertAt] = requirements.Counts[insertAt-1]
			insertAt--
		}
		requirements.Items[insertAt] = itemID
		requirements.Counts[insertAt] = count
	}
}

type CoverageGroup struct {
	Name    string
	Sources []string
	Targets []string
}

type Catalog struct {
	Heroes  []Hero
	Items   map[string]Item
	Recipes []Recipe
}

type Variant struct {
	Rotation int
	Cells    []Coord
	Stars    []Star
}

type InventoryInstance struct {
	InstanceID    string
	ItemID        string
	OriginalIndex int
}

type StarPosition struct {
	Star     Star
	Position Coord
}

type Placement struct {
	InstanceID    string
	ItemID        string
	OriginalIndex int
	Rotation      int
	Origin        Coord
	Cells         []Coord
	StarPositions []StarPosition
	Mask          uint64
	AdjacentMask  uint64
}

type CraftActivation struct {
	RecipeResult        string
	AnchorInstance      string
	IngredientInstances []string
}

type StarActivation struct {
	SourceInstance string
	TargetInstance string
	StarPosition   Coord
	EffectText     string
}

type Score struct {
	CraftCount                    int   `json:"crafts"`
	StarCount                     int   `json:"stars"`
	ItemCount                     int   `json:"items"`
	StarTargetBreadth             int   `json:"star_target_breadth,omitempty"`
	StarReciprocalPairs           int   `json:"star_reciprocal_pairs,omitempty"`
	StarSourceDefinitionDiversity int   `json:"star_source_definition_diversity,omitempty"`
	PriorityCounts                []int `json:"priority_counts,omitempty"`
}

// ComparePriorityCounts orders priority vectors lexicographically, treating
// omitted trailing entries as zero.
func ComparePriorityCounts(left []int, right []int) int {
	maxLen := len(left)
	if len(right) > maxLen {
		maxLen = len(right)
	}
	for index := 0; index < maxLen; index++ {
		leftValue := 0
		if index < len(left) {
			leftValue = left[index]
		}
		rightValue := 0
		if index < len(right) {
			rightValue = right[index]
		}
		if leftValue != rightValue {
			return leftValue - rightValue
		}
	}
	return 0
}

// CompareScores is the canonical solver objective order. A positive result
// means left is better; it intentionally excludes layout-key tie breaking.
func CompareScores(left Score, right Score) int {
	if compare := ComparePriorityCounts(left.PriorityCounts, right.PriorityCounts); compare != 0 {
		return compare
	}
	if left.CraftCount != right.CraftCount {
		return left.CraftCount - right.CraftCount
	}
	if left.StarCount != right.StarCount {
		return left.StarCount - right.StarCount
	}
	if left.ItemCount != right.ItemCount {
		return left.ItemCount - right.ItemCount
	}
	if left.StarTargetBreadth != right.StarTargetBreadth {
		return left.StarTargetBreadth - right.StarTargetBreadth
	}
	if left.StarReciprocalPairs != right.StarReciprocalPairs {
		return left.StarReciprocalPairs - right.StarReciprocalPairs
	}
	if left.StarSourceDefinitionDiversity != right.StarSourceDefinitionDiversity {
		return left.StarSourceDefinitionDiversity - right.StarSourceDefinitionDiversity
	}
	return 0
}

// SearchPhaseWork keeps charged DFS/seed nodes separate from local heuristic
// moves, which do not consume MaxNodes.
type SearchPhaseWork struct {
	Phase             string `json:"phase"`
	BudgetScope       string `json:"budget_scope,omitempty"`
	ChargedNodes      int64  `json:"charged_nodes,omitempty"`
	UnchargedMoves    int64  `json:"uncharged_moves,omitempty"`
	Candidates        int64  `json:"candidates,omitempty"`
	BestScore         *Score `json:"best_score,omitempty"`
	Eligible          bool   `json:"eligible"`
	Invoked           bool   `json:"invoked"`
	SkipReason        string `json:"skip_reason,omitempty"`
	TerminationReason string `json:"termination_reason,omitempty"`
	NodesReserved     int64  `json:"nodes_reserved,omitempty"`
	NodesConsumed     int64  `json:"nodes_consumed,omitempty"`
	NodesReturned     int64  `json:"nodes_returned,omitempty"`
	ReturnTarget      string `json:"return_target,omitempty"`
	StartStageNodes   int64  `json:"start_stage_nodes,omitempty"`
	EndStageNodes     int64  `json:"end_stage_nodes,omitempty"`
}

// StarUpperBounds are diagnostic-only optimistic bounds. They are never used
// to prune the search.
type StarUpperBounds struct {
	Structural       int `json:"structural"`
	Compatible       int `json:"compatible"`
	Available        int `json:"available"`
	GeometricRelaxed int `json:"geometric_relaxed"`
}

type IncumbentEvent struct {
	Sequence        int64    `json:"sequence"`
	Reasons         []string `json:"reasons"`
	Phase           string   `json:"phase"`
	PhaseLocalNodes int64    `json:"phase_local_nodes,omitempty"`
	PhaseLocalMoves int64    `json:"phase_local_moves,omitempty"`
	// GlobalBudgetConsumed is legacy stage-local consumption for traces
	// produced inside a scheduler stage. Use the explicit fields below for
	// unambiguous multi-stage diagnostics.
	GlobalBudgetConsumed    int64           `json:"global_budget_consumed"`
	StageBudgetConsumed     int64           `json:"stage_budget_consumed,omitempty"`
	ExecutionBudgetConsumed int64           `json:"execution_budget_consumed,omitempty"`
	UnchargedWork           int64           `json:"uncharged_work,omitempty"`
	CompletionMoves         int64           `json:"completion_moves,omitempty"`
	RefineMoves             int64           `json:"refine_moves,omitempty"`
	ElapsedMS               int64           `json:"elapsed_ms"`
	ConfiguredMaxNodes      int64           `json:"configured_max_nodes"`
	Score                   Score           `json:"score"`
	LayoutKey               string          `json:"layout_key"`
	CanonicalLayoutHash     string          `json:"canonical_layout_hash"`
	StarUpperBounds         StarUpperBounds `json:"star_upper_bounds"`
}

type PriorityCeilingStats struct {
	Evaluations          int64 `json:"evaluations"`
	CanonicalLayoutCount int   `json:"canonical_layout_count"`
	StarMin              int   `json:"star_min"`
	StarP50              int   `json:"star_p50"`
	StarP90              int   `json:"star_p90"`
	StarMax              int   `json:"star_max"`
	BestScore            Score `json:"best_score"`
}

type PlateauStats struct {
	FirstPriorityCeilingBudgetPercent float64 `json:"first_priority_ceiling_budget_percent,omitempty"`
	LastScoreImprovementBudgetPercent float64 `json:"last_score_improvement_budget_percent,omitempty"`
	Timing                            string  `json:"timing,omitempty"`
	Strong                            bool    `json:"strong"`
}

// PlateauLink keeps the literal copy identifiers used by an evaluated layout.
// Canonical signatures are stored separately so audit output never loses the
// original instance labels.
type PlateauLink struct {
	SourceInstance string `json:"source_instance"`
	TargetInstance string `json:"target_instance"`
	StarPosition   Coord  `json:"star_position"`
}

// ReferenceDelta describes the structural difference from a diagnostic-only
// reference layout. Copy labels are ignored for placement and canonical-link
// comparisons; literal-link fields preserve those labels for auditing.
type ReferenceDelta struct {
	MovedItems              int `json:"moved_items"`
	RotationChanges         int `json:"rotation_changes"`
	ExactLiteralLinksLost   int `json:"exact_literal_links_lost"`
	ExactLiteralLinksGained int `json:"exact_literal_links_gained"`
	CanonicalLinksLost      int `json:"canonical_links_lost"`
	CanonicalLinksGained    int `json:"canonical_links_gained"`
	StructuralDistance      int `json:"structural_distance"`
}

// ReferenceDistance identifies the closest priority-ceiling candidate seen
// during diagnostics. Delta.StructuralDistance is its scalar distance.
type ReferenceDistance struct {
	Delta               ReferenceDelta `json:"delta"`
	LayoutKey           string         `json:"layout_key"`
	CanonicalLayoutHash string         `json:"canonical_layout_hash"`
}

type PlateauSample struct {
	Score                  Score                     `json:"score"`
	Placements             []Placement               `json:"placements"`
	LayoutKey              string                    `json:"layout_key"`
	CanonicalLayoutHash    string                    `json:"canonical_layout_hash"`
	LiteralLinks           []PlateauLink             `json:"literal_links"`
	CanonicalLinkSignature string                    `json:"canonical_link_signature"`
	StarsBySource          []StarInstanceTargetCount `json:"stars_by_source"`
	StarsBySourceItem      map[string]int            `json:"stars_by_source_item"`
	ReferenceDelta         *ReferenceDelta           `json:"reference_delta,omitempty"`
	DeltaToReference       string                    `json:"delta_to_reference,omitempty"`
	MissingReferenceLinks  []PlateauLink             `json:"missing_reference_links,omitempty"`
	SourceDeltaToReference map[string]int            `json:"source_delta_to_reference,omitempty"`
}

type ScoreFrequency struct {
	Score Score `json:"score"`
	Count int64 `json:"count"`
}

type PlateauOperatorStats struct {
	Operator                     string `json:"operator"`
	NeighborhoodSize             int    `json:"neighborhood_size"`
	Nodes                        int64  `json:"nodes"`
	PriorityPreservingCandidates int64  `json:"priority_preserving_candidates"`
	PriorityBoundPruned          int64  `json:"priority_bound_pruned,omitempty"`
	CompletedBelowPriority       int64  `json:"completed_below_priority,omitempty"`
	PriorityPreservingComplete   int64  `json:"priority_preserving_complete,omitempty"`
	CompareScoreImprovement      int64  `json:"compare_score_improvement,omitempty"`
	BestScore                    Score  `json:"best_score"`
}

type UBStarsBucket struct {
	Gap                 int   `json:"gap"`
	Candidates          int64 `json:"candidates"`
	PromisingCandidates int64 `json:"promising_candidates"`
	ImprovedCandidates  int64 `json:"improved_candidates"`
}

// PartialUBStarsBucket is kept separate from complete-layout gap buckets.
// It describes the relaxed repair state used only to prioritize LNS work.
type PartialUBStarsBucket struct {
	FixedStars         int   `json:"fixed_stars"`
	PartialUBStars     int   `json:"partial_ub_stars"`
	Headroom           int   `json:"headroom"`
	OverIncumbent      int   `json:"over_incumbent"`
	Candidates         int64 `json:"candidates"`
	ImprovedCandidates int64 `json:"improved_candidates"`
}

type PlateauClosureStats struct {
	NeighborhoodSize       int                 `json:"neighborhood_size"`
	Attempts               int64               `json:"attempts"`
	UniqueClosures         int64               `json:"unique_closures"`
	PriorityFeasible       int64               `json:"priority_feasible"`
	Enqueued               int64               `json:"enqueued"`
	ClosureTooLarge        int64               `json:"closure_too_large"`
	MandatorySizeMin       int                 `json:"mandatory_size_min"`
	MandatorySizeMax       int                 `json:"mandatory_size_max"`
	OptionalSizeMin        int                 `json:"optional_size_min"`
	OptionalSizeMax        int                 `json:"optional_size_max"`
	MandatorySizeHistogram []ClosureSizeBucket `json:"mandatory_size_histogram,omitempty"`
	OptionalSizeHistogram  []ClosureSizeBucket `json:"optional_size_histogram,omitempty"`
}

type ClosureSizeBucket struct {
	Size  int   `json:"size"`
	Count int64 `json:"count"`
}

// PlateauLevelStats records candidate selection and bounded work for one
// resolved plateau LNS level.
type PlateauLevelStats struct {
	MaxNeighborhoodSize    int   `json:"max_neighborhood_size"`
	MinMandatorySize       int   `json:"min_mandatory_size"`
	MaxMandatorySize       int   `json:"max_mandatory_size"`
	QuotaBps               int64 `json:"quota_bps"`
	MaxSelected            int   `json:"max_selected"`
	MaxSelectedPerBase     int   `json:"max_selected_per_base"`
	MinNodeBudget          int64 `json:"min_node_budget"`
	CandidatesMatchingBand int64 `json:"candidates_matching_band,omitempty"`
	RejectedBelowBand      int64 `json:"rejected_below_band,omitempty"`
	RejectedAboveBand      int64 `json:"rejected_above_band,omitempty"`
	Selected               int64 `json:"selected,omitempty"`
	PerBaseDrops           int64 `json:"per_base_drops,omitempty"`
	SelectedCapDrops       int64 `json:"selected_cap_drops,omitempty"`
	QuotaAllocated         int64 `json:"quota_allocated,omitempty"`
	QuotaConsumed          int64 `json:"quota_consumed,omitempty"`
	QuotaCarried           int64 `json:"quota_carried,omitempty"`
	Tasks                  int64 `json:"tasks,omitempty"`
	Nodes                  int64 `json:"nodes,omitempty"`
	Improvements           int64 `json:"improvements,omitempty"`
}

// PlateauArchiveStats exposes archive behaviour without serializing the
// potentially large diagnostic samples unless diagnostics were requested.
type PlateauArchiveStats struct {
	Capacity                 int                    `json:"capacity,omitempty"`
	Size                     int                    `json:"size,omitempty"`
	Admissions               int64                  `json:"admissions,omitempty"`
	Rejections               int64                  `json:"rejections,omitempty"`
	SignatureDiversity       int                    `json:"signature_diversity,omitempty"`
	BaseOrigins              []string               `json:"base_origins,omitempty"`
	OperatorStats            []PlateauOperatorStats `json:"operator_stats,omitempty"`
	UBStarsBuckets           []UBStarsBucket        `json:"ub_stars_buckets,omitempty"`
	PartialUBStarsBuckets    []PartialUBStarsBucket `json:"partial_ub_stars_buckets,omitempty"`
	ClosureStats             []PlateauClosureStats  `json:"closure_stats,omitempty"`
	LevelStats               []PlateauLevelStats    `json:"level_stats,omitempty"`
	Samples                  []PlateauSample        `json:"samples,omitempty"`
	LinkFrequency            map[string]int64       `json:"link_frequency,omitempty"`
	SourceFrequency          map[string]int64       `json:"source_frequency,omitempty"`
	TargetFrequency          map[string]int64       `json:"target_frequency,omitempty"`
	StarsBySource            map[string]int64       `json:"stars_by_source,omitempty"`
	StarsBySourceItem        map[string]int64       `json:"stars_by_source_item,omitempty"`
	ScoreDistribution        []ScoreFrequency       `json:"score_distribution,omitempty"`
	ReferenceEvaluations     int64                  `json:"reference_evaluations,omitempty"`
	MinimumReferenceDistance *ReferenceDistance     `json:"minimum_reference_distance,omitempty"`
}

type PackingSeedLayerWidth struct {
	Depth  int `json:"depth"`
	States int `json:"states"`
}

type PackingSeedItemCount struct {
	Items  int   `json:"items"`
	States int64 `json:"states"`
}

// ConstellationRootPackingDepthDiagnostic records one dynamic-MRV beam layer.
type ConstellationRootPackingDepthDiagnostic struct {
	Depth                     int              `json:"depth"`
	SelectedInstanceHistogram map[string]int64 `json:"selected_instance_histogram,omitempty"`
	MinLegalPlacements        int              `json:"min_legal_placements"`
	MaxLegalPlacements        int              `json:"max_legal_placements"`
	ZeroDomainPrunes          int64            `json:"zero_domain_prunes"`
	StatesBeforeExpansion     int              `json:"states_before_expansion"`
	StatesAfterDedup          int              `json:"states_after_dedup"`
	BeamEvictions             int64            `json:"beam_evictions"`
	StatesRetained            int              `json:"states_retained"`
}

type PackingSeedDiagnostics struct {
	MaxDepth           int                     `json:"max_depth"`
	StatesByItemCount  []PackingSeedItemCount  `json:"states_by_item_count,omitempty"`
	LayerWidths        []PackingSeedLayerWidth `json:"layer_widths,omitempty"`
	HardDeadPruned     int64                   `json:"hard_dead_pruned,omitempty"`
	SymmetryPruned     int64                   `json:"symmetry_pruned,omitempty"`
	StatesDeduplicated int64                   `json:"states_deduplicated,omitempty"`
	BeamEvictions      int64                   `json:"beam_evictions,omitempty"`
}

const (
	// PackingSeedFeasibilityProfileVersionV1 is the eager canonical-key
	// accounting contract preserved for previously collected P0.1 artifacts.
	PackingSeedFeasibilityProfileVersionV1 = "packing-seed-feasibility-ops-v1"
	// PackingSeedFeasibilityProfileVersionV2 records lazy candidate-key
	// materialization introduced by H2a.
	PackingSeedFeasibilityProfileVersionV2 = "packing-seed-feasibility-ops-v2"
)

// PackingSeedCanonicalCopyOrderOperationProfile attributes canonical-copy
// ordering work to one caller inside packing-seed search. It is
// measurement-only and does not participate in search policy decisions.
type PackingSeedCanonicalCopyOrderOperationProfile struct {
	Calls                      int64 `json:"calls"`
	Rejects                    int64 `json:"rejects"`
	ExistingScanned            int64 `json:"existing_scanned"`
	SameItemComparisons        int64 `json:"same_item_comparisons"`
	CandidatePlacementKeyCalls int64 `json:"candidate_placement_key_calls"`
	PlacementKeyCalls          int64 `json:"placement_key_calls"`
	PlacementKeyBytes          int64 `json:"placement_key_bytes"`
}

// PackingSeedFeasibilityOperationProfile is a deterministic count of generic
// packing feasibility and canonical-copy-order work performed by
// packingSeedSearch. It deliberately remains separate from rooted dynamic-MRV
// packing telemetry.
type PackingSeedFeasibilityOperationProfile struct {
	Version                        string                                        `json:"version"`
	SearchCalls                    int64                                         `json:"search_calls"`
	StatesVisited                  int64                                         `json:"states_visited"`
	CandidateOptionChecks          int64                                         `json:"candidate_option_checks"`
	CandidateOverlapRejects        int64                                         `json:"candidate_overlap_rejects"`
	CandidateChargeAttempts        int64                                         `json:"candidate_charge_attempts"`
	CandidateChargeDenied          int64                                         `json:"candidate_charge_denied"`
	CandidateExpansions            int64                                         `json:"candidate_expansions"`
	FeasibilityCalls               int64                                         `json:"feasibility_calls"`
	FeasibilityInstancesConsidered int64                                         `json:"feasibility_instances_considered"`
	FeasibilityOptionChecks        int64                                         `json:"feasibility_option_checks"`
	FeasibilityOverlapRejects      int64                                         `json:"feasibility_overlap_rejects"`
	FeasibilityLegalPlacements     int64                                         `json:"feasibility_legal_placements"`
	FeasibilityDeadReturns         int64                                         `json:"feasibility_dead_returns"`
	CandidateCanonical             PackingSeedCanonicalCopyOrderOperationProfile `json:"candidate_canonical"`
	FeasibilityCanonical           PackingSeedCanonicalCopyOrderOperationProfile `json:"feasibility_canonical"`
}

const BoundAttributionProfileVersion = "bound-attribution-ops-v1"

// PriorityUpperBoundSiteProfile attributes deterministic work and outcomes to
// one physical priority-upper-bound call site. It is measurement-only and does
// not participate in bound results or search policy.
type PriorityUpperBoundSiteProfile struct {
	Calls                                int64 `json:"calls"`
	FeasibleResults                      int64 `json:"feasible_results"`
	RejectedResults                      int64 `json:"rejected_results"`
	InvalidPriorityReturns               int64 `json:"invalid_priority_returns"`
	PriorityEntriesValidated             int64 `json:"priority_entries_validated"`
	FixedPlacementInputs                 int64 `json:"fixed_placement_inputs"`
	CurrentPlacementInputs               int64 `json:"current_placement_inputs"`
	AnchoredPlacements                   int64 `json:"anchored_placements"`
	RemovedInstanceInputs                int64 `json:"removed_instance_inputs"`
	RemovedInstances                     int64 `json:"removed_instances"`
	RemovedOptionCandidates              int64 `json:"removed_option_candidates"`
	RemovedOptionRejectedFixedOverlap    int64 `json:"removed_option_rejected_fixed_overlap"`
	RemovedOptionRejectedOutsideFree     int64 `json:"removed_option_rejected_outside_free"`
	RemovedOptionsRetained               int64 `json:"removed_options_retained"`
	UniquePrioritySourceItems            int64 `json:"unique_priority_source_items"`
	AnchoredSourceInstances              int64 `json:"anchored_source_instances"`
	RemovedSourceInstances               int64 `json:"removed_source_instances"`
	StarSlots                            int64 `json:"star_slots"`
	FixedTargetChecks                    int64 `json:"fixed_target_checks"`
	RemovedTargetChecks                  int64 `json:"removed_target_checks"`
	SelfTargetSkips                      int64 `json:"self_target_skips"`
	FixedFixedGeometryChecks             int64 `json:"fixed_fixed_geometry_checks"`
	RemovedSourceOptionChecksFixedTarget int64 `json:"removed_source_option_checks_fixed_target"`
	FixedSourceTargetOptionChecks        int64 `json:"fixed_source_target_option_checks"`
	RemovedSourceTargetOptionPairs       int64 `json:"removed_source_target_option_pairs"`
	GeometryCandidateChecks              int64 `json:"geometry_candidate_checks"`
	GeometryOverlapRejects               int64 `json:"geometry_overlap_rejects"`
	StarPositionHitCalls                 int64 `json:"star_position_hit_calls"`
	StarPositionHitTrue                  int64 `json:"star_position_hit_true"`
	SlotTargetHits                       int64 `json:"slot_target_hits"`
	MatchingCalls                        int64 `json:"matching_calls"`
}

// PriorityUpperBoundOperationProfile keeps the four physical callers fixed so
// profiles cannot acquire accidental high-cardinality site labels.
type PriorityUpperBoundOperationProfile struct {
	ConstellationFilterInvocations int64                         `json:"constellation_filter_invocations"`
	ConstellationStatesInput       int64                         `json:"constellation_states_input"`
	ConstellationStatesRetained    int64                         `json:"constellation_states_retained"`
	ConstellationStatesRejected    int64                         `json:"constellation_states_rejected"`
	ConstellationFilter            PriorityUpperBoundSiteProfile `json:"constellation_filter"`
	RepairDFS                      PriorityUpperBoundSiteProfile `json:"repair_dfs"`
	PlateauPrefilter               PriorityUpperBoundSiteProfile `json:"plateau_prefilter"`
	PlateauDFS                     PriorityUpperBoundSiteProfile `json:"plateau_dfs"`
}

// OutgoingBoundSiteProfile attributes the existing outgoing check/prune totals
// and the deterministic work inside upperPriorityCounts to one physical flow.
type OutgoingBoundSiteProfile struct {
	Checks                       int64 `json:"checks"`
	PrunedNodes                  int64 `json:"pruned_nodes"`
	PlacedMapBuilds              int64 `json:"placed_map_builds"`
	PlacedMapInsertions          int64 `json:"placed_map_insertions"`
	PlacedMaskInstanceChecks     int64 `json:"placed_mask_instance_checks"`
	PriorityIterations           int64 `json:"priority_iterations"`
	SourceInstanceIterations     int64 `json:"source_instance_iterations"`
	PrioritySourceMatches        int64 `json:"priority_source_matches"`
	ZeroStarSourceSkips          int64 `json:"zero_star_source_skips"`
	PlacedSourceIterations       int64 `json:"placed_source_iterations"`
	FreeSourceIterations         int64 `json:"free_source_iterations"`
	PlacedSourceTargetIterations int64 `json:"placed_source_target_iterations"`
	SelfTargetSkips              int64 `json:"self_target_skips"`
	TargetPlacementLookups       int64 `json:"target_placement_lookups"`
	PlacedTargetsFound           int64 `json:"placed_targets_found"`
	UnplacedTargets              int64 `json:"unplaced_targets"`
	SourceHitsTargetCalls        int64 `json:"source_hits_target_calls"`
	SourceHitsTargetTrue         int64 `json:"source_hits_target_true"`
	CoveragePlacementKeyCalls    int64 `json:"coverage_placement_key_calls"`
	PlacedPotentialLookups       int64 `json:"placed_potential_lookups"`
	FreePotentialLookups         int64 `json:"free_potential_lookups"`
	PopcountCalls                int64 `json:"popcount_calls"`
	StarCountClamps              int64 `json:"star_count_clamps"`
}

type OutgoingBoundOperationProfile struct {
	Search OutgoingBoundSiteProfile `json:"search"`
	Repair OutgoingBoundSiteProfile `json:"repair"`
}

// BoundAttributionOperationProfile is the independent, versioned R1I contract
// for comparative priority and outgoing bound attribution.
type BoundAttributionOperationProfile struct {
	Version       string                             `json:"version"`
	PriorityUpper PriorityUpperBoundOperationProfile `json:"priority_upper"`
	Outgoing      OutgoingBoundOperationProfile      `json:"outgoing"`
}

// ConstellationRootPackingOperationProfile is a deterministic count of work
// performed by one rooted dynamic-MRV packing session. It is measurement-only:
// none of these fields participate in ranking, pruning, allocation, or search
// policy decisions.
type ConstellationRootPackingOperationProfile struct {
	Version                        string `json:"version"`
	SessionsStarted                int64  `json:"sessions_started"`
	RunCalls                       int64  `json:"run_calls"`
	PauseReturns                   int64  `json:"pause_returns"`
	DepthsStarted                  int64  `json:"depths_started"`
	StatesPrepared                 int64  `json:"states_prepared"`
	AreaPrunes                     int64  `json:"area_prunes"`
	MRVSelectionCalls              int64  `json:"mrv_selection_calls"`
	MRVInstancesConsidered         int64  `json:"mrv_instances_considered"`
	MRVOptionChecks                int64  `json:"mrv_option_checks"`
	MRVLegalPlacements             int64  `json:"mrv_legal_placements"`
	LedgerChargeAttempts           int64  `json:"ledger_charge_attempts"`
	LedgerChargeDenied             int64  `json:"ledger_charge_denied"`
	CandidateExpansions            int64  `json:"candidate_expansions"`
	CompleteCandidates             int64  `json:"complete_candidates"`
	PlacementCopyCalls             int64  `json:"placement_copy_calls"`
	PlacementElementsCopied        int64  `json:"placement_elements_copied"`
	FeasibilityCalls               int64  `json:"feasibility_calls"`
	FeasibilityInstancesConsidered int64  `json:"feasibility_instances_considered"`
	FeasibilityOptionChecks        int64  `json:"feasibility_option_checks"`
	FragmentationEvaluations       int64  `json:"fragmentation_evaluations"`
	PartialScoreEvaluations        int64  `json:"partial_score_evaluations"`
	StateKeyConstructions          int64  `json:"state_key_constructions"`
	StateKeyBytes                  int64  `json:"state_key_bytes"`
	DedupLookups                   int64  `json:"dedup_lookups"`
	DedupHits                      int64  `json:"dedup_hits"`
	DedupReplacements              int64  `json:"dedup_replacements"`
	DepthFinishCalls               int64  `json:"depth_finish_calls"`
	PrecutStates                   int64  `json:"precut_states"`
	StatesSorted                   int64  `json:"states_sorted"`
}

// ConstellationRootPackingSchedulerPolicy records the fully resolved root-only
// allocation policy after the construction budget and selected root count are known.
type ConstellationRootPackingSchedulerPolicy struct {
	StageID                string `json:"stage_id"`
	Name                   string `json:"name"`
	AvailablePackingBudget int64  `json:"available_packing_budget"`
	FamilyCount            int    `json:"family_count"`
	InitialQuantum         int64  `json:"initial_quantum"`
	RoundQuantum           int64  `json:"round_quantum"`
}

// ConstellationRootPackingAllocationRound records one root-family allocation.
// Returned nodes were not charged and may be allocated to a later living family;
// aggregate reservations can therefore include a recycled quota token.
type ConstellationRootPackingAllocationRound struct {
	Round    int   `json:"round"`
	Reserved int64 `json:"reserved"`
	Consumed int64 `json:"consumed"`
	Returned int64 `json:"returned"`
}

// ConstellationSeedDiagnostics describes the oracle-blind constellation macro-seed
// experiment. It is search telemetry only; skeletons are never serialized.
type ConstellationSeedDiagnostics struct {
	Version                                   string                                        `json:"version,omitempty"`
	ShareBps                                  int64                                         `json:"share_bps,omitempty"`
	ConstructionNodes                         int64                                         `json:"construction_nodes,omitempty"`
	PackingNodes                              int64                                         `json:"packing_nodes,omitempty"`
	PackingBeamWidth                          int                                           `json:"packing_beam_width,omitempty"`
	StatesGenerated                           int64                                         `json:"states_generated,omitempty"`
	StatesDeduplicated                        int64                                         `json:"states_deduplicated,omitempty"`
	SourceStatesRetained                      int                                           `json:"source_states_retained,omitempty"`
	TargetInstancesConsidered                 int                                           `json:"target_instances_considered,omitempty"`
	TargetStatesRetained                      int                                           `json:"target_states_retained,omitempty"`
	SkeletonsReached                          int                                           `json:"skeletons_reached,omitempty"`
	SkeletonsDistinct                         int                                           `json:"skeletons_distinct,omitempty"`
	RootsCompleted                            int                                           `json:"roots_completed,omitempty"`
	PriorityConstellations                    int                                           `json:"priority_constellations,omitempty"`
	PrioritySourceGeometryCount               int                                           `json:"priority_source_geometry_count,omitempty"`
	CandidatePrioritySourceGeometryCount      int                                           `json:"candidate_priority_source_geometry_count,omitempty"`
	CandidatePrioritySourceGeometryOrbitCount int                                           `json:"candidate_priority_source_geometry_orbit_count,omitempty"`
	SelectedPrioritySourceGeometryCount       int                                           `json:"selected_priority_source_geometry_count,omitempty"`
	SelectedPrioritySourceGeometryOrbitCount  int                                           `json:"selected_priority_source_geometry_orbit_count,omitempty"`
	CandidateRootFreeMaskCount                int                                           `json:"candidate_root_free_mask_count,omitempty"`
	CandidateRootFreeMaskOrbitCount           int                                           `json:"candidate_root_free_mask_orbit_count,omitempty"`
	PoolSweepNodes                            int64                                         `json:"pool_sweep_nodes,omitempty"`
	CandidateCompletionOptimizationNodes      int64                                         `json:"candidate_completion_optimization_nodes,omitempty"`
	ForcedCandidateRootedPackingNodes         int64                                         `json:"forced_candidate_rooted_packing_nodes,omitempty"`
	ParentFrontierHedgeNodes                  int64                                         `json:"parent_frontier_hedge_nodes,omitempty"`
	PriorityTargetAssignmentCount             int                                           `json:"priority_target_assignment_count,omitempty"`
	RootFreeMaskCount                         int                                           `json:"root_free_mask_count,omitempty"`
	RelaxationFrontierExists                  bool                                          `json:"relaxation_frontier_exists,omitempty"`
	RelaxationFrontierSelected                bool                                          `json:"relaxation_frontier_selected,omitempty"`
	RelaxationFrontierRootID                  string                                        `json:"relaxation_frontier_root_id,omitempty"`
	RelaxationFrontierParentExactKey          string                                        `json:"relaxation_frontier_parent_exact_key,omitempty"`
	RelaxationFrontierSize                    int                                           `json:"relaxation_frontier_size,omitempty"`
	ConstellationRootWinnerID                 string                                        `json:"constellation_root_winner_id,omitempty"`
	ConstellationRootWinnerScore              *Score                                        `json:"constellation_root_winner_score,omitempty"`
	ConstellationRootWinnerHash               string                                        `json:"constellation_root_winner_hash,omitempty"`
	ConstellationSeedFinalScore               *Score                                        `json:"constellation_seed_final_score,omitempty"`
	ConstellationSeedFinalHash                string                                        `json:"constellation_seed_final_hash,omitempty"`
	RootPackingScheduler                      *ConstellationRootPackingSchedulerPolicy      `json:"root_packing_scheduler,omitempty"`
	RootPackingOperationProfile               *ConstellationRootPackingOperationProfile     `json:"root_packing_operation_profile,omitempty"`
	CandidatePoolFeasibilitySweep             *ConstellationCandidatePoolFeasibilitySweep   `json:"candidate_pool_feasibility_sweep,omitempty"`
	CandidateCompletionOptimization           *ConstellationCandidateCompletionOptimization `json:"candidate_completion_optimization,omitempty"`
	ForcedCandidateRootedPacking              *ConstellationForcedCandidateRootedPacking    `json:"forced_candidate_rooted_packing,omitempty"`
	ParentFrontierHedge                       *ConstellationParentFrontierHedge             `json:"parent_frontier_hedge,omitempty"`
	Skeletons                                 []ConstellationSkeletonDiagnostic             `json:"skeletons,omitempty"`
	Roots                                     []ConstellationRootDiagnostic                 `json:"roots,omitempty"`
}

// ConstellationParentFrontierHedge records an additive diagnostic replay of
// one V5 family slot. Parent and frontier searches are independent; only their
// final local merge may select a family best.
type ConstellationParentFrontierHedge struct {
	RequestedStageID                       string                                    `json:"requested_stage_id,omitempty"`
	HighParentConsumptionProbeThresholdBps int64                                     `json:"high_parent_consumption_probe_threshold_bps"`
	Attempts                               []ConstellationParentFrontierHedgeAttempt `json:"attempts,omitempty"`
}

type ConstellationParentFrontierHedgeAttempt struct {
	StageID                 string                                 `json:"stage_id"`
	SelectionStatus         string                                 `json:"selection_status"`
	FamilySlotID            string                                 `json:"family_slot_id,omitempty"`
	SlotCount               int                                    `json:"slot_count,omitempty"`
	FamilyMemberCount       int                                    `json:"family_member_count,omitempty"`
	RootMemberExecutions    int                                    `json:"root_member_executions,omitempty"`
	ParentExactKey          string                                 `json:"parent_exact_key,omitempty"`
	FrontierExactKey        string                                 `json:"frontier_exact_key,omitempty"`
	TotalQuota              int64                                  `json:"total_quota,omitempty"`
	NormalSlotNodesConsumed int64                                  `json:"normal_slot_nodes_consumed,omitempty"`
	NormalSlotNodesReturned int64                                  `json:"normal_slot_nodes_returned,omitempty"`
	Parent                  ConstellationParentFrontierHedgeMember `json:"parent"`
	Frontier                ConstellationParentFrontierHedgeMember `json:"frontier"`
	FamilyConsumed          int64                                  `json:"family_consumed,omitempty"`
	FamilyReturned          int64                                  `json:"family_returned,omitempty"`
	HypotheticalReturnDelta int64                                  `json:"hypothetical_return_delta,omitempty"`
	FamilyBestScore         *Score                                 `json:"family_best_score,omitempty"`
	FamilyBestHash          string                                 `json:"family_best_hash,omitempty"`
	FamilyWinnerMember      string                                 `json:"family_winner_member,omitempty"`
}

type ConstellationParentFrontierHedgeMember struct {
	Policy              string `json:"policy"`
	PackingBeamWidth    int    `json:"packing_beam_width,omitempty"`
	PackingStrategy     string `json:"packing_strategy,omitempty"`
	Reserved            int64  `json:"reserved"`
	Consumed            int64  `json:"consumed"`
	Returned            int64  `json:"returned"`
	ConsumedFractionBps int64  `json:"consumed_fraction_bps,omitempty"`
	ResidualFractionBps int64  `json:"residual_fraction_bps,omitempty"`
	Invoked             bool   `json:"invoked"`
	SkippedReason       string `json:"skipped_reason,omitempty"`
	Completed           bool   `json:"completed"`
	TerminationReason   string `json:"termination_reason,omitempty"`
	FirstCompleteNodes  int64  `json:"first_complete_nodes,omitempty"`
	BestScore           *Score `json:"best_score,omitempty"`
	BestHash            string `json:"best_hash,omitempty"`
}

type ConstellationForcedCandidateRootedPacking struct {
	RequestedCandidateID string                                             `json:"requested_candidate_id"`
	RequestedSlot        int                                                `json:"requested_slot"`
	RequestedStageID     string                                             `json:"requested_stage_id,omitempty"`
	Attempts             []ConstellationForcedCandidateRootedPackingAttempt `json:"attempts,omitempty"`
}

type ConstellationForcedCandidateRootedPackingAttempt struct {
	StageID                   string                                    `json:"stage_id"`
	CandidateID               string                                    `json:"candidate_id"`
	CandidateRank             int                                       `json:"candidate_rank,omitempty"`
	SweepRank                 int                                       `json:"sweep_rank,omitempty"`
	SelectionStatus           string                                    `json:"selection_status"`
	ForcedRootSlot            int                                       `json:"forced_root_slot"`
	BaselinePackingBeamWidth  int                                       `json:"baseline_packing_beam_width,omitempty"`
	EffectivePackingBeamWidth int                                       `json:"effective_packing_beam_width,omitempty"`
	PackingRanking            string                                    `json:"packing_ranking,omitempty"`
	PackingStrategy           string                                    `json:"packing_strategy,omitempty"`
	ReplacedRootID            string                                    `json:"replaced_root_id,omitempty"`
	NormalSlotNodesReserved   int64                                     `json:"normal_slot_nodes_reserved,omitempty"`
	NormalSlotNodesConsumed   int64                                     `json:"normal_slot_nodes_consumed,omitempty"`
	NormalSlotNodesReturned   int64                                     `json:"normal_slot_nodes_returned,omitempty"`
	ExactAnchorKey            string                                    `json:"exact_anchor_key,omitempty"`
	PartialScore              *Score                                    `json:"partial_score,omitempty"`
	SourceGeometryKey         string                                    `json:"source_geometry_key,omitempty"`
	SourceGeometryOrbitKey    string                                    `json:"source_geometry_orbit_key,omitempty"`
	TargetAssignmentKey       string                                    `json:"target_assignment_key,omitempty"`
	AnchoredInstanceIDs       []string                                  `json:"anchored_instance_ids,omitempty"`
	NodesAvailable            int64                                     `json:"nodes_available,omitempty"`
	NodesConsumed             int64                                     `json:"nodes_consumed,omitempty"`
	NodesReturned             int64                                     `json:"nodes_returned,omitempty"`
	StopSource                string                                    `json:"stop_source,omitempty"`
	Completed                 bool                                      `json:"completed"`
	CandidateCount            int                                       `json:"candidate_count,omitempty"`
	TerminationReason         string                                    `json:"termination_reason,omitempty"`
	FirstCompleteNodes        int64                                     `json:"first_complete_nodes,omitempty"`
	BestScore                 *Score                                    `json:"best_score,omitempty"`
	BestLayoutKey             string                                    `json:"best_layout_key,omitempty"`
	BestHash                  string                                    `json:"best_hash,omitempty"`
	BeamEvictions             int64                                     `json:"beam_evictions,omitempty"`
	HardDeadPruned            int64                                     `json:"hard_dead_pruned,omitempty"`
	SymmetryPruned            int64                                     `json:"symmetry_pruned,omitempty"`
	StatesDeduplicated        int64                                     `json:"states_deduplicated,omitempty"`
	MRVDepths                 []ConstellationRootPackingDepthDiagnostic `json:"mrv_depths,omitempty"`
	ShadowReferenceUsed       bool                                      `json:"shadow_reference_used"`
	WitnessUsedForSearch      bool                                      `json:"witness_used_for_search"`
	ShadowWitnessTrace        *ConstellationForcedCandidateShadowTrace  `json:"shadow_witness_trace,omitempty"`
	MaxShadowPrecutRank       *int                                      `json:"max_shadow_precut_rank,omitempty"`
	MaxShadowRetainedRank     *int                                      `json:"max_shadow_retained_rank,omitempty"`
	WitnessUsed               bool                                      `json:"witness_used"`
	ExactSearchUsed           bool                                      `json:"exact_search_used"`
}

type ConstellationForcedCandidateShadowTrace struct {
	SemanticFingerprint    string                                    `json:"semantic_fingerprint,omitempty"`
	WitnessHash            string                                    `json:"witness_hash,omitempty"`
	ValidationStatus       string                                    `json:"validation_status"`
	ValidationReason       string                                    `json:"validation_reason,omitempty"`
	Canonicalization       string                                    `json:"canonicalization"`
	FirstLossDepth         int                                       `json:"first_loss_depth,omitempty"`
	FirstLossStage         string                                    `json:"first_loss_stage,omitempty"`
	FirstBeamEvictionDepth int                                       `json:"first_beam_eviction_depth,omitempty"`
	Depths                 []ConstellationForcedCandidateShadowDepth `json:"depths,omitempty"`
}

type ConstellationForcedCandidateShadowDepth struct {
	Depth                         int                                                  `json:"depth"`
	StatesBeforeExpansion         int                                                  `json:"states_before_expansion"`
	StatesBeforeWitnessCompatible int                                                  `json:"states_before_witness_compatible"`
	ShadowSymmetryPruned          int64                                                `json:"shadow_symmetry_pruned"`
	ShadowFeasibilityPruned       int64                                                `json:"shadow_feasibility_pruned"`
	Generated                     int                                                  `json:"generated"`
	GeneratedWitnessCompatible    int                                                  `json:"generated_witness_compatible"`
	Deduplicated                  int                                                  `json:"deduplicated"`
	DedupWitnessCompatible        int                                                  `json:"dedup_witness_compatible"`
	PrecutStates                  int                                                  `json:"precut_states"`
	PrecutWitnessCompatible       int                                                  `json:"precut_witness_compatible"`
	BestPrecutRank                int                                                  `json:"best_precut_rank,omitempty"`
	BestPrecutBetterCount         int                                                  `json:"best_precut_better_count,omitempty"`
	BestPrecutTieCount            int                                                  `json:"best_precut_tie_count,omitempty"`
	CutoffTieCrossed              bool                                                 `json:"cutoff_tie_crossed,omitempty"`
	RetainedStates                int                                                  `json:"retained_states"`
	RetainedWitnessCompatible     int                                                  `json:"retained_witness_compatible"`
	BestRetainedRank              int                                                  `json:"best_retained_rank,omitempty"`
	RankingAutopsy                *ConstellationForcedCandidateShadowRankingAutopsy    `json:"ranking_autopsy,omitempty"`
	CounterfactualReranking       *ConstellationForcedCandidateCounterfactualReranking `json:"counterfactual_reranking,omitempty"`
}

type ConstellationForcedCandidateCounterfactualReranking struct {
	Version             string                                              `json:"version"`
	PrioritySchemaWidth int                                                 `json:"priority_schema_width"`
	FrontierStates      int                                                 `json:"frontier_states"`
	BeamWidth           int                                                 `json:"beam_width"`
	Variants            []ConstellationForcedCandidateCounterfactualVariant `json:"variants,omitempty"`
}

type ConstellationForcedCandidateCounterfactualVariant struct {
	ID                       string                                         `json:"id"`
	ComparatorTuple          string                                         `json:"comparator_tuple"`
	CompatibleStateCount     int                                            `json:"compatible_state_count"`
	ShadowComponents         *ConstellationForcedCandidateRankingComponents `json:"shadow_ranking_components,omitempty"`
	BestComponents           *ConstellationForcedCandidateRankingComponents `json:"best_ranking_components,omitempty"`
	CutoffComponents         *ConstellationForcedCandidateRankingComponents `json:"cutoff_ranking_components,omitempty"`
	StrictlyPrecedingStates  int                                            `json:"strictly_preceding_states"`
	FullRankStart            int                                            `json:"full_rank_start"`
	FullRankEnd              int                                            `json:"full_rank_end"`
	FullComparatorTieCount   int                                            `json:"full_comparator_tie_count"`
	KeylessRankStart         int                                            `json:"keyless_rank_start"`
	KeylessRankEnd           int                                            `json:"keyless_rank_end"`
	KeylessTupleTieCount     int                                            `json:"keyless_tuple_tie_count"`
	ActualBeamFit            bool                                           `json:"actual_beam_fit"`
	ActualBeamFitAmbiguous   bool                                           `json:"actual_beam_fit_ambiguous,omitempty"`
	FullGuaranteedBeamFit    bool                                           `json:"full_guaranteed_beam_fit"`
	FullPossibleBeamFit      bool                                           `json:"full_possible_beam_fit"`
	KeylessGuaranteedBeamFit bool                                           `json:"keyless_guaranteed_beam_fit"`
	KeylessPossibleBeamFit   bool                                           `json:"keyless_possible_beam_fit"`
	KeylessTieCrossesBeam    bool                                           `json:"keyless_tie_crosses_beam,omitempty"`
	FirstDecisiveComponents  []ConstellationShadowDecisiveComponent         `json:"first_decisive_components,omitempty"`
}

type ConstellationForcedCandidateShadowRankingAutopsy struct {
	Version                        string                                         `json:"version"`
	Comparator                     string                                         `json:"comparator"`
	StrictlyPrecedingStates        int                                            `json:"strictly_preceding_states"`
	StrictlyPrecedingPercentileBps int                                            `json:"strictly_preceding_percentile_bps"`
	FullComparatorTieBeforeStates  int                                            `json:"full_comparator_tie_before_states,omitempty"`
	ShadowComponents               ConstellationForcedCandidateRankingComponents  `json:"shadow_ranking_components"`
	CutoffComponents               *ConstellationForcedCandidateRankingComponents `json:"cutoff_ranking_components,omitempty"`
	BestComponents                 *ConstellationForcedCandidateRankingComponents `json:"best_ranking_components,omitempty"`
	FirstDecisiveComponents        []ConstellationShadowDecisiveComponent         `json:"first_decisive_components,omitempty"`
	ShadowComponentPercentiles     []ConstellationShadowComponentPercentile       `json:"shadow_component_percentiles,omitempty"`
}

type ConstellationForcedCandidateRankingComponents struct {
	Restricted                    int    `json:"restricted"`
	Flexibility                   int    `json:"flexibility"`
	Fragmentation                 int    `json:"fragmentation"`
	PriorityCounts                []int  `json:"priority_counts,omitempty"`
	CraftCount                    int    `json:"craft_count"`
	StarCount                     int    `json:"star_count"`
	ItemCount                     int    `json:"item_count"`
	StarTargetBreadth             int    `json:"star_target_breadth"`
	StarReciprocalPairs           int    `json:"star_reciprocal_pairs"`
	StarSourceDefinitionDiversity int    `json:"star_source_definition_diversity"`
	Key                           string `json:"key"`
}

type ConstellationShadowDecisiveComponent struct {
	Component              string `json:"component"`
	PriorityIndex          *int   `json:"priority_index,omitempty"`
	PrefixEqualCount       int    `json:"prefix_equal_count"`
	BetterAtComponentCount int    `json:"better_at_component_count"`
	WorseAtComponentCount  int    `json:"worse_at_component_count"`
	EqualAtComponentCount  int    `json:"equal_at_component_count"`
	AdvantageP50           *int   `json:"advantage_p50,omitempty"`
	AdvantageP90           *int   `json:"advantage_p90,omitempty"`
	AdvantageP100          *int   `json:"advantage_p100,omitempty"`
}

type ConstellationShadowComponentPercentile struct {
	Component           string `json:"component"`
	PriorityIndex       *int   `json:"priority_index,omitempty"`
	BetterCount         int    `json:"better_count"`
	EqualCount          int    `json:"equal_count"`
	BetterPercentileBps int    `json:"better_percentile_bps"`
}

// ConstellationCandidateCompletionOptimization describes a stage-local exact
// completion diagnostic for a candidate selected by stable ID.
type ConstellationCandidateCompletionOptimization struct {
	RequestedCandidateID string                                    `json:"requested_candidate_id"`
	RequestedStageID     string                                    `json:"requested_stage_id,omitempty"`
	Attempts             []ConstellationCandidateCompletionAttempt `json:"attempts,omitempty"`
}

type ConstellationCandidateCompletionAttempt struct {
	StageID                       string                                   `json:"stage_id"`
	CandidateID                   string                                   `json:"candidate_id"`
	CandidateRank                 int                                      `json:"candidate_rank,omitempty"`
	SweepRank                     int                                      `json:"sweep_rank,omitempty"`
	SelectionStatus               string                                   `json:"selection_status"`
	ExactAnchorKey                string                                   `json:"exact_anchor_key,omitempty"`
	Signature                     string                                   `json:"signature,omitempty"`
	PartialScore                  *Score                                   `json:"partial_score,omitempty"`
	SourceGeometryKey             string                                   `json:"source_geometry_key,omitempty"`
	SourceGeometryOrbitKey        string                                   `json:"source_geometry_orbit_key,omitempty"`
	TargetAssignmentKey           string                                   `json:"target_assignment_key,omitempty"`
	AnchoredInstanceIDs           []string                                 `json:"anchored_instance_ids,omitempty"`
	RemainingInstanceIDs          []string                                 `json:"remaining_instance_ids,omitempty"`
	InitialOccupiedMaskHex        string                                   `json:"initial_occupied_mask_hex,omitempty"`
	InitialFreeMaskHex            string                                   `json:"initial_free_mask_hex,omitempty"`
	NodesAvailable                int64                                    `json:"nodes_available,omitempty"`
	NodesConsumed                 int64                                    `json:"nodes_consumed,omitempty"`
	NodesReturned                 int64                                    `json:"nodes_returned,omitempty"`
	Status                        string                                   `json:"status,omitempty"`
	TerminationReason             string                                   `json:"termination_reason,omitempty"`
	StopSource                    string                                   `json:"stop_source,omitempty"`
	SearchExhausted               bool                                     `json:"search_exhausted,omitempty"`
	InitialIncumbentAvailable     bool                                     `json:"initial_incumbent_available"`
	InitialIncumbentSource        string                                   `json:"initial_incumbent_source,omitempty"`
	InitialIncumbentHash          string                                   `json:"initial_incumbent_hash,omitempty"`
	InitialWitnessRejectionReason string                                   `json:"initial_witness_rejection_reason,omitempty"`
	SemanticFingerprint           string                                   `json:"semantic_fingerprint,omitempty"`
	InitialWitness                *ConstellationCandidateCompletionWitness `json:"initial_witness,omitempty"`
	BestWitness                   *ConstellationCandidateCompletionWitness `json:"best_witness,omitempty"`
	BestScore                     *Score                                   `json:"best_score,omitempty"`
	BestLayoutKey                 string                                   `json:"best_layout_key,omitempty"`
	BestHash                      string                                   `json:"best_hash,omitempty"`
	TerminalCompletions           int64                                    `json:"terminal_completions,omitempty"`
	AreaPrunes                    int64                                    `json:"area_prunes,omitempty"`
	ZeroDomainPrunes              int64                                    `json:"zero_domain_prunes,omitempty"`
	TranspositionPrunes           int64                                    `json:"transposition_prunes,omitempty"`
	FirstCompleteNodes            *int64                                   `json:"first_complete_nodes,omitempty"`
	FirstIncumbentNodes           *int64                                   `json:"first_incumbent_nodes,omitempty"`
	FinalBestFirstSeenNodes       *int64                                   `json:"final_best_first_seen_nodes,omitempty"`
	// FirstBestNodes is retained for compatibility and means the first strict
	// in-probe score improvement, not the first occurrence of final best score.
	FirstBestNodes    int64                                              `json:"first_best_nodes,omitempty"`
	ScoreImprovements []ConstellationCandidateCompletionScoreImprovement `json:"score_improvement_trace,omitempty"`
}

type ConstellationCandidateCompletionWitness struct {
	SchemaVersion       string                                   `json:"schema_version"`
	SemanticFingerprint string                                   `json:"semantic_fingerprint"`
	CandidateID         string                                   `json:"candidate_id"`
	ExactAnchorKey      string                                   `json:"exact_anchor_key"`
	Score               Score                                    `json:"score"`
	LayoutKey           string                                   `json:"layout_key"`
	CanonicalLayoutHash string                                   `json:"canonical_layout_hash"`
	Placements          []ConstellationCandidateWitnessPlacement `json:"placements"`
}

type ConstellationCandidateWitnessPlacement struct {
	InstanceID string `json:"instance_id"`
	ItemID     string `json:"item_id"`
	Rotation   int    `json:"rotation"`
	Origin     Coord  `json:"origin"`
}

type ConstellationCandidateCompletionScoreImprovement struct {
	Nodes     int64  `json:"nodes"`
	Score     Score  `json:"score"`
	LayoutKey string `json:"layout_key"`
	Hash      string `json:"hash"`
}

// ConstellationCandidatePoolFeasibilitySweep describes one stage-local,
// oracle-blind classification of the bounded V4 candidate pool.
type ConstellationCandidatePoolFeasibilitySweep struct {
	StageID               string                                    `json:"stage_id"`
	SweepOrderPolicy      string                                    `json:"sweep_order_policy"`
	CandidateCount        int                                       `json:"candidate_count"`
	SelectedRootCount     int                                       `json:"selected_root_count"`
	NodesAvailable        int64                                     `json:"nodes_available"`
	NodesConsumed         int64                                     `json:"nodes_consumed"`
	NodesReturned         int64                                     `json:"nodes_returned"`
	FeasibleCount         int                                       `json:"feasible_count"`
	InfeasibleProvenCount int                                       `json:"infeasible_proven_count"`
	UnknownBudgetCount    int                                       `json:"unknown_budget_count"`
	Candidates            []ConstellationCandidateFeasibilityRecord `json:"candidates,omitempty"`
	Orbits                []ConstellationCandidateFeasibilityOrbit  `json:"orbits,omitempty"`
}

type ConstellationCandidateFeasibilityRecord struct {
	StageID                string   `json:"stage_id"`
	CandidateID            string   `json:"candidate_id"`
	CandidateRank          int      `json:"candidate_rank"`
	SweepRank              int      `json:"sweep_rank"`
	ExactAnchorKey         string   `json:"exact_anchor_key"`
	Signature              string   `json:"signature"`
	PartialScore           Score    `json:"partial_score"`
	SourceGeometryKey      string   `json:"source_geometry_key"`
	SourceGeometryOrbitKey string   `json:"source_geometry_orbit_key"`
	TargetAssignmentKey    string   `json:"target_assignment_key"`
	OccupiedMaskHex        string   `json:"occupied_mask_hex"`
	FreeMaskHex            string   `json:"free_mask_hex"`
	FreeMaskOrbitKey       string   `json:"free_mask_orbit_key"`
	AnchoredInstanceIDs    []string `json:"anchored_instance_ids,omitempty"`
	RemainingInstanceIDs   []string `json:"remaining_instance_ids,omitempty"`
	SelectedRootID         string   `json:"selected_root_id,omitempty"`
	AttemptKind            string   `json:"attempt_kind"`
	NodesAvailable         int64    `json:"nodes_available"`
	NodesConsumed          int64    `json:"nodes_consumed"`
	NodesReturned          int64    `json:"nodes_returned"`
	FeasibilityStatus      string   `json:"feasibility_status"`
	TerminationReason      string   `json:"termination_reason"`
	StopSource             string   `json:"stop_source,omitempty"`
	SearchExhausted        bool     `json:"search_exhausted"`
	WitnessHash            string   `json:"witness_hash,omitempty"`
}

type ConstellationCandidateFeasibilityOrbit struct {
	StageID                  string `json:"stage_id"`
	SourceGeometryOrbitKey   string `json:"source_geometry_orbit_key"`
	CandidateCount           int    `json:"candidate_count"`
	SelectedRootCount        int    `json:"selected_root_count"`
	DistinctRawGeometries    int    `json:"distinct_raw_geometries"`
	DistinctFreeMasks        int    `json:"distinct_free_masks"`
	DistinctFreeMaskOrbits   int    `json:"distinct_free_mask_orbits"`
	FeasibleCount            int    `json:"feasible_count"`
	InfeasibleProvenCount    int    `json:"infeasible_proven_count"`
	UnknownBudgetCount       int    `json:"unknown_budget_count"`
	NodesConsumed            int64  `json:"nodes_consumed"`
	BestPartialScoreFeasible *Score `json:"best_partial_score_feasible,omitempty"`
}

// ConstellationSkeletonDiagnostic identifies one oracle-blind priority macrostate.
// It is emitted only with diagnostics and never participates in search ranking.
type ConstellationSkeletonDiagnostic struct {
	ID                             string        `json:"id"`
	Signature                      string        `json:"signature"`
	ExactKey                       string        `json:"exact_key"`
	AnchorCount                    int           `json:"anchor_count"`
	Score                          Score         `json:"score"`
	PriorityLinks                  []PlateauLink `json:"priority_links,omitempty"`
	PrioritySourceGeometryKey      string        `json:"priority_source_geometry_key,omitempty"`
	PrioritySourceGeometryOrbitKey string        `json:"priority_source_geometry_orbit_key,omitempty"`
	PriorityTargetAssignmentKey    string        `json:"priority_target_assignment_key,omitempty"`
	SelectionPolicy                string        `json:"selection_policy,omitempty"`
	RelaxedFromExactKey            string        `json:"relaxed_from_exact_key,omitempty"`
	FrontierExactKey               string        `json:"frontier_exact_key,omitempty"`
	RelaxationFrontierSize         int           `json:"relaxation_frontier_size,omitempty"`
}

// ConstellationRootDiagnostic records one rooted-packing attempt derived from
// a skeleton. Candidate identity is present only when packing completed.
type ConstellationRootDiagnostic struct {
	ID                                             string                                    `json:"id"`
	SkeletonID                                     string                                    `json:"skeleton_id"`
	FamilyID                                       string                                    `json:"family_id,omitempty"`
	FamilyAllocationRounds                         []ConstellationRootPackingAllocationRound `json:"family_allocation_rounds,omitempty"`
	FamilyTotalQuota                               int64                                     `json:"family_total_quota,omitempty"`
	FamilyTotalConsumed                            int64                                     `json:"family_total_consumed,omitempty"`
	FamilyTotalReturned                            int64                                     `json:"family_total_returned,omitempty"`
	FamilyTerminationReason                        string                                    `json:"family_termination_reason,omitempty"`
	NodesReserved                                  int64                                     `json:"nodes_reserved"`
	NodesConsumed                                  int64                                     `json:"nodes_consumed"`
	PackingBeamWidth                               int                                       `json:"packing_beam_width"`
	LayerWidths                                    []PackingSeedLayerWidth                   `json:"layer_widths,omitempty"`
	BeamEvictions                                  int64                                     `json:"beam_evictions,omitempty"`
	HardDeadPruned                                 int64                                     `json:"hard_dead_pruned,omitempty"`
	SymmetryPruned                                 int64                                     `json:"symmetry_pruned,omitempty"`
	StatesDeduplicated                             int64                                     `json:"states_deduplicated,omitempty"`
	Completed                                      bool                                      `json:"completed"`
	CandidateCount                                 int                                       `json:"candidate_count,omitempty"`
	BestScore                                      *Score                                    `json:"best_score,omitempty"`
	CandidateLayoutKey                             string                                    `json:"candidate_layout_key,omitempty"`
	CandidateHash                                  string                                    `json:"candidate_hash,omitempty"`
	SourceGeometryKey                              string                                    `json:"source_geometry_key,omitempty"`
	SourceGeometryOrbitKey                         string                                    `json:"source_geometry_orbit_key,omitempty"`
	InitialOccupiedMaskHex                         string                                    `json:"initial_occupied_mask_hex,omitempty"`
	InitialFreeMaskHex                             string                                    `json:"initial_free_mask_hex,omitempty"`
	InitialFreeCellCount                           int                                       `json:"initial_free_cell_count,omitempty"`
	AnchoredInstanceIDs                            []string                                  `json:"anchored_instance_ids,omitempty"`
	RemainingPackingOrder                          []string                                  `json:"remaining_packing_order,omitempty"`
	InitialRestricted                              int                                       `json:"initial_restricted,omitempty"`
	InitialFlexibility                             int                                       `json:"initial_flexibility,omitempty"`
	InitialFragmentation                           int                                       `json:"initial_fragmentation,omitempty"`
	TerminationReason                              string                                    `json:"termination_reason,omitempty"`
	RootPackingInputKey                            string                                    `json:"root_packing_input_key,omitempty"`
	PackingStrategy                                string                                    `json:"packing_strategy,omitempty"`
	SelectionPolicy                                string                                    `json:"selection_policy,omitempty"`
	RelaxedFromExactKey                            string                                    `json:"relaxed_from_exact_key,omitempty"`
	FrontierExactKey                               string                                    `json:"frontier_exact_key,omitempty"`
	RelaxationFrontierSize                         int                                       `json:"relaxation_frontier_size,omitempty"`
	ParentGuardedFrontier                          *ConstellationParentFrontierHedgeAttempt  `json:"parent_guarded_frontier,omitempty"`
	FirstCompleteNodes                             int64                                     `json:"first_complete_nodes,omitempty"`
	DistinctNextItemsSelected                      int                                       `json:"distinct_next_items_selected,omitempty"`
	MRVDepths                                      []ConstellationRootPackingDepthDiagnostic `json:"mrv_depths,omitempty"`
	OperationProfile                               *ConstellationRootPackingOperationProfile `json:"operation_profile,omitempty"`
	ProbeNodesAvailable                            *int64                                    `json:"probe_nodes_available,omitempty"`
	ProbeNodesConsumed                             *int64                                    `json:"probe_nodes_consumed,omitempty"`
	ProbeNodesReturned                             *int64                                    `json:"probe_nodes_returned,omitempty"`
	ProbeTerminationReason                         string                                    `json:"probe_termination_reason,omitempty"`
	FeasibilityStatus                              string                                    `json:"feasibility_status,omitempty"`
	SearchExhausted                                *bool                                     `json:"search_exhausted,omitempty"`
	WitnessHash                                    string                                    `json:"witness_hash,omitempty"`
	ExactCompletionEligible                        *bool                                     `json:"exact_completion_eligible,omitempty"`
	ExactCompletionSkipReason                      string                                    `json:"exact_completion_skip_reason,omitempty"`
	ExactCompletionNodesAvailable                  *int64                                    `json:"exact_completion_nodes_available,omitempty"`
	ExactCompletionNodesConsumed                   *int64                                    `json:"exact_completion_nodes_consumed,omitempty"`
	ExactCompletionNodesReturned                   *int64                                    `json:"exact_completion_nodes_returned,omitempty"`
	ExactCompletionStatus                          string                                    `json:"exact_completion_status,omitempty"`
	ExactCompletionTerminationReason               string                                    `json:"exact_completion_termination_reason,omitempty"`
	ExactCompletionStopSource                      string                                    `json:"exact_completion_stop_source,omitempty"`
	ExactCompletionSearchExhausted                 *bool                                     `json:"exact_completion_search_exhausted,omitempty"`
	ExactCompletionInitialIncumbentFromRootPacking *bool                                     `json:"exact_completion_initial_incumbent_from_root_packing,omitempty"`
	ExactCompletionBestScore                       *Score                                    `json:"exact_completion_best_score,omitempty"`
	ExactCompletionBestLayoutKey                   string                                    `json:"exact_completion_best_layout_key,omitempty"`
	ExactCompletionBestHash                        string                                    `json:"exact_completion_best_hash,omitempty"`
	ExactCompletionTerminalCompletions             int64                                     `json:"exact_completion_terminal_completions,omitempty"`
	ExactCompletionAreaPrunes                      int64                                     `json:"exact_completion_area_prunes,omitempty"`
	ExactCompletionZeroDomainPrunes                int64                                     `json:"exact_completion_zero_domain_prunes,omitempty"`
	ExactCompletionTranspositionPrunes             int64                                     `json:"exact_completion_transposition_prunes,omitempty"`
}

type StarCoverageTarget struct {
	TargetInstance string
	TargetItemID   string
	CoveredSources []string
	CoveredCount   int
}

type StarCoverageBucket struct {
	CoveredSources int
	TargetCount    int
}

type StarCoverageBreakdown struct {
	Name          string
	Sources       []string
	TargetItemIDs []string
	Buckets       []StarCoverageBucket
	Targets       []StarCoverageTarget
}

type LooseStarPriority struct {
	SourceItemID         string
	TargetCount          int
	LinkCount            int
	InstanceTargetCounts []StarInstanceTargetCount
}

// StarInstanceTargetCount records the distinct static targets reached by one
// source copy. It is used by outgoing-per-instance-v3 diagnostics.
type StarInstanceTargetCount struct {
	SourceInstance string
	TargetCount    int
}

type Evaluation struct {
	Score               Score
	Crafts              []CraftActivation
	Stars               []StarActivation
	StarCoverage        *StarCoverageBreakdown
	StarCoverageGroups  []StarCoverageBreakdown
	LooseStarPriorities []LooseStarPriority
}

type SearchStats struct {
	NodesExplored                 int64
	NodesPerSecond                float64
	SetupMS                       int64
	SeedMS                        int64
	RepairMS                      int64
	SearchMS                      int64
	RefineMS                      int64
	Backend                       string
	ServerElapsedMS               int64
	RemoteWorkers                 int
	MaxNodesApplied               int64
	MaxNodesCapped                bool
	Limited                       bool
	Refined                       bool
	CoverageSources               []string
	CoverageTargetCount           int
	CoverageCeiling               []StarCoverageBucket
	CoverageCeilingReached        bool
	PriorityCeiling               []int
	PriorityCeilingReached        bool
	CoverageBoundChecks           int64
	CoveragePrunedNodes           int64
	ExactBoundChecks              int64
	ExactBoundPrunedNodes         int64
	OutgoingBoundChecks           int64
	OutgoingBoundPrunedNodes      int64
	CoverageSeedNodes             int64
	CoverageSeedCandidates        int
	CoverageSeedBest              string
	StarSeedNodes                 int64
	StarSeedCandidates            int
	ConstellationSeedNodes        int64
	ConstellationSeedCandidates   int
	ConstellationSeedDiagnostics  ConstellationSeedDiagnostics
	PackingSeedNodes              int64
	PackingSeedCandidates         int
	PackingSeedHardPruned         int64
	PackingSeedStatesDeduplicated int64
	PackingSeedOperationProfile   *PackingSeedFeasibilityOperationProfile
	BoundOperationProfile         *BoundAttributionOperationProfile
	SymmetryPrunedBranches        int64
	FirstCompletePhase            string
	FirstCompleteNodes            int64
	FirstCompleteMS               int64
	SeedBestScore                 Score
	SearchBestScore               Score
	PostRepairBestScore           Score
	RefineBestScore               Score
	InitialBestPriorityCounts     []int
	SeedBestPriorityCounts        []int
	SearchBestPriorityCounts      []int
	PostRepairBestPriorityCounts  []int
	RefineBestPriorityCounts      []int
	ParallelTasks                 int
	ParallelWorkersUsed           int
	RefineMovesChecked            int64
	RefineImprovements            int
	RefineBestDelta               string
	CompletionMovesChecked        int64
	CompletionImprovements        int
	RepairNodes                   int64
	RepairIterations              int
	RepairImprovements            int
	RepairCandidates              int
	RepairBest                    string
	RepairParallelTasks           int
	RepairParallelWorkersUsed     int
	StoppedAfterCoverageCeiling   bool
	StoppedAfterPriorityCeiling   bool
	DiagnosticsEnabled            bool
	GlobalBudgetConsumed          int64
	UnusedGlobalNodes             int64
	NormalBudgetConfigured        int64
	NormalBudgetConsumed          int64
	DiagnosticBudgetConfigured    int64
	DiagnosticBudgetConsumed      int64
	ExecutionBudgetConfigured     int64
	ExecutionBudgetConsumed       int64
	UnchargedWork                 int64
	PhaseWork                     []SearchPhaseWork
	IncumbentTrace                []IncumbentEvent
	PriorityCeilingStats          *PriorityCeilingStats
	Plateau                       PlateauStats
	StarUpperBounds               StarUpperBounds
	FirstFullyPackedPhase         string
	FirstFullyPackedNodes         int64
	FirstFullyPackedMS            int64
	PackingSeedDiagnostics        PackingSeedDiagnostics
	ConfigFingerprint             string
	ExecutionFingerprint          string
	Stages                        []SearchStageStats
	TaskAllocation                TaskAllocationStats
	PlateauArchive                PlateauArchiveStats
	PlateauLNSNodes               int64
	PlateauRefineNodes            int64
	PlateauRefineWalkLength       int
	PlateauRefineMaxValley        int
	PlateauRefineImproved         bool
}

// SearchStageStats records the bounded work and carryover at one scheduler
// boundary. Rich diagnostic data remains behind the diagnostic serializers.
type SearchStageStats struct {
	ID                                string              `json:"id"`
	NodeLimit                         int64               `json:"node_limit"`
	StagePolicyFingerprint            string              `json:"stage_policy_fingerprint"`
	StageInputScore                   Score               `json:"stage_input_score"`
	StageOutputScore                  Score               `json:"stage_output_score"`
	NodesReserved                     int64               `json:"nodes_reserved"`
	NodesCharged                      int64               `json:"nodes_charged"`
	StageBudgetConsumed               int64               `json:"stage_budget_consumed,omitempty"`
	ExecutionBudgetConsumed           int64               `json:"execution_budget_consumed,omitempty"`
	DiagnosticNodesReserved           int64               `json:"diagnostic_nodes_reserved,omitempty"`
	DiagnosticNodesCharged            int64               `json:"diagnostic_nodes_charged,omitempty"`
	DiagnosticExecutionBudgetConsumed int64               `json:"diagnostic_execution_budget_consumed,omitempty"`
	ExecutionTotalBudgetConsumed      int64               `json:"execution_total_budget_consumed,omitempty"`
	FinalCarriedScore                 Score               `json:"final_carried_score"`
	TaskAllocation                    TaskAllocationStats `json:"task_allocation,omitempty"`
	PhaseWork                         []SearchPhaseWork   `json:"phase_work,omitempty"`
	IncumbentTrace                    []IncumbentEvent    `json:"incumbent_trace,omitempty"`
	PlateauArchive                    PlateauArchiveStats `json:"plateau_archive,omitempty"`
}

type TaskAllocationStats struct {
	SplitDepth                 int   `json:"split_depth,omitempty"`
	TasksGenerated             int   `json:"tasks_generated,omitempty"`
	TasksExecuted              int   `json:"tasks_executed,omitempty"`
	TasksPrunedBeforeExecution int   `json:"tasks_pruned_before_execution,omitempty"`
	AllocatedNodesMin          int64 `json:"allocated_nodes_min,omitempty"`
	AllocatedNodesP50          int64 `json:"allocated_nodes_p50,omitempty"`
	AllocatedNodesP90          int64 `json:"allocated_nodes_p90,omitempty"`
	AllocatedNodesMax          int64 `json:"allocated_nodes_max,omitempty"`
}

type Solution struct {
	Placements          []Placement
	Evaluation          Evaluation
	LayoutKey           string
	CanonicalLayoutHash string
	Search              SearchStats
}
