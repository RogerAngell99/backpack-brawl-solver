package model

import "fmt"

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
	CraftCount     int
	StarCount      int
	ItemCount      int
	PriorityCounts []int
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
	SourceItemID string
	TargetCount  int
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
	NodesExplored               int64
	NodesPerSecond              float64
	SetupMS                     int64
	SeedMS                      int64
	RepairMS                    int64
	SearchMS                    int64
	RefineMS                    int64
	Backend                     string
	ServerElapsedMS             int64
	RemoteWorkers               int
	MaxNodesApplied             int64
	MaxNodesCapped              bool
	Limited                     bool
	Refined                     bool
	CoverageSources             []string
	CoverageTargetCount         int
	CoverageCeiling             []StarCoverageBucket
	CoverageCeilingReached      bool
	CoverageBoundChecks         int64
	CoveragePrunedNodes         int64
	ExactBoundChecks            int64
	ExactBoundPrunedNodes       int64
	CoverageSeedNodes           int64
	CoverageSeedCandidates      int
	CoverageSeedBest            string
	ParallelTasks               int
	ParallelWorkersUsed         int
	RefineMovesChecked          int64
	RefineImprovements          int
	RefineBestDelta             string
	RepairNodes                 int64
	RepairIterations            int
	RepairImprovements          int
	RepairCandidates            int
	RepairBest                  string
	RepairParallelTasks         int
	RepairParallelWorkersUsed   int
	StoppedAfterCoverageCeiling bool
}

type Solution struct {
	Placements []Placement
	Evaluation Evaluation
	LayoutKey  string
	Search     SearchStats
}
