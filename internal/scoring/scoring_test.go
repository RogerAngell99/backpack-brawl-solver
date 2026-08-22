package scoring_test

import (
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
	"backpack-brawl-solver/internal/solver"
)

func loadTestCatalog(t *testing.T) model.Catalog {
	t.Helper()
	// These placement regressions intentionally use the curated fixture; production uses the runtime projection.
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog-curated.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return loaded
}

func TestCauldronCatalogEntry(t *testing.T) {
	cat := loadTestCatalog(t)
	item, ok := cat.Items["cauldron"]
	if !ok {
		t.Fatal("cauldron missing from catalog")
	}
	assertStringSlice(t, item.Types, []string{"Accessory"})
	if len(item.Shape) != 4 {
		t.Fatalf("shape cell count=%d want 4", len(item.Shape))
	}
	if len(item.Stars) != 0 {
		t.Fatalf("stars=%d want 0", len(item.Stars))
	}
	if item.ImagePath != "assets/items/cauldron.png" {
		t.Fatalf("image path=%q want assets/items/cauldron.png", item.ImagePath)
	}
}

func TestPotionAndArmorCatalogEntries(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		itemID     string
		types      []string
		shapeCells int
		starCount  int
	}{
		{itemID: "water_potion", types: []string{"Potion"}, shapeCells: 1},
		{itemID: "endurance_potion", types: []string{"Potion"}, shapeCells: 2},
		{itemID: "antivenom", types: []string{"Potion"}, shapeCells: 2},
		{itemID: "prickly_potion", types: []string{"Potion"}, shapeCells: 2},
		{itemID: "skull_cap", types: []string{"Armor"}, shapeCells: 4},
		{itemID: "fur_boots", types: []string{"Armor"}, shapeCells: 4, starCount: 2},
	}
	for _, tt := range tests {
		t.Run(tt.itemID, func(t *testing.T) {
			item, ok := cat.Items[tt.itemID]
			if !ok {
				t.Fatalf("%s missing from catalog", tt.itemID)
			}
			assertStringSlice(t, item.Types, tt.types)
			if len(item.Shape) != tt.shapeCells {
				t.Fatalf("shape cell count=%d want %d", len(item.Shape), tt.shapeCells)
			}
			if len(item.Stars) != tt.starCount {
				t.Fatalf("stars=%d want %d", len(item.Stars), tt.starCount)
			}
		})
	}
}

func place(t *testing.T, cat model.Catalog, itemID string, instanceIndex int, origin model.Coord, rotation int) model.Placement {
	t.Helper()
	itemIDs := make([]string, instanceIndex+1)
	for idx := range itemIDs {
		itemIDs[idx] = itemID
	}
	instance := solver.ExpandInventory(itemIDs)[instanceIndex]
	options, err := solver.PlacementOptions(cat, instance, geometry.FullGridMask())
	if err != nil {
		t.Fatalf("PlacementOptions returned error: %v", err)
	}
	for _, option := range options {
		if option.Origin == origin && option.Rotation == rotation {
			return option
		}
	}
	t.Fatalf("no placement for %s at %v rotation %d", itemID, origin, rotation)
	return model.Placement{}
}

func TestKiwiStarCountsJellyBeanTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "kiwi_dewdrop", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "kiwi_dewdrop", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 2 {
		t.Fatalf("len(stars)=%d want 2", len(stars))
	}
	if stars[0].SourceInstance != "kiwi_dewdrop#0" || stars[0].TargetInstance != "kiwi_dewdrop#1" {
		t.Fatalf("unexpected first star: %+v", stars[0])
	}
}

func TestKiwiStarIgnoresNonJellyBeanTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "kiwi_dewdrop", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestUnknownStarRuleNeverMatches(t *testing.T) {
	item := model.Item{ID: "target", Types: []string{"Weapon"}}
	star := model.Star{RuleStatus: "unknown"}

	if scoring.StarMatchesItem("source", "target", &item, &star) {
		t.Fatal("unknown star rule must not match any target")
	}
}

func TestEvaluateScoreOnlyMatchesFullEvaluation(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		name           string
		placements     []model.Placement
		priorities     []string
		coverageGroups []model.CoverageGroup
	}{
		{
			name: "no priorities",
			placements: []model.Placement{
				place(t, cat, "apple", 0, model.Coord{Row: 0, Col: 0}, 0),
				place(t, cat, "banana", 1, model.Coord{Row: 0, Col: 0}, 0),
			},
		},
		{
			name: "coverage groups and loose stars",
			placements: []model.Placement{
				place(t, cat, "apple", 0, model.Coord{Row: 0, Col: 0}, 0),
				place(t, cat, "banana", 1, model.Coord{Row: 0, Col: 0}, 0),
				place(t, cat, "spice", 2, model.Coord{Row: 2, Col: 0}, 0),
				place(t, cat, "piercing_lance", 3, model.Coord{Row: 4, Col: 1}, 90),
				place(t, cat, "power_stone", 4, model.Coord{Row: 3, Col: 1}, 90),
				place(t, cat, "fine_sword", 5, model.Coord{Row: 3, Col: 4}, 90),
			},
			priorities: []string{"star_source:banana", "craft:endurance_potion"},
			coverageGroups: []model.CoverageGroup{
				{Name: "Weapons", Sources: []string{"piercing_lance", "power_stone"}, Targets: []string{"fine_sword", "piercing_lance"}},
				{Name: "Food", Sources: []string{"spice"}},
			},
		},
		{
			name: "counts as target item filter",
			placements: []model.Placement{
				place(t, cat, "mana_crystal", 0, model.Coord{Row: 4, Col: 2}, 0),
				place(t, cat, "starlight_potion", 1, model.Coord{Row: 1, Col: 2}, 0),
			},
			priorities: []string{"star_source:mana_crystal"},
		},
		{
			name: "coverage group with loose counts-as star",
			placements: []model.Placement{
				place(t, cat, "power_stone", 0, model.Coord{Row: 3, Col: 1}, 90),
				place(t, cat, "fine_sword", 1, model.Coord{Row: 3, Col: 4}, 90),
				place(t, cat, "mana_crystal", 2, model.Coord{Row: 4, Col: 2}, 0),
				place(t, cat, "starlight_potion", 3, model.Coord{Row: 1, Col: 2}, 0),
			},
			priorities: []string{"star_source:mana_crystal"},
			coverageGroups: []model.CoverageGroup{
				{Name: "Weapons", Sources: []string{"power_stone"}},
			},
		},
		{
			name: "global priority order with coverage group",
			placements: []model.Placement{
				place(t, cat, "power_stone", 0, model.Coord{Row: 3, Col: 1}, 90),
				place(t, cat, "fine_sword", 1, model.Coord{Row: 3, Col: 4}, 90),
				place(t, cat, "mana_crystal", 2, model.Coord{Row: 4, Col: 2}, 0),
				place(t, cat, "starlight_potion", 3, model.Coord{Row: 1, Col: 2}, 0),
			},
			priorities: []string{"star_source:mana_crystal", "coverage_group:0", "craft:missing"},
			coverageGroups: []model.CoverageGroup{
				{Name: "Weapons", Sources: []string{"power_stone"}},
			},
		},
		{
			name: "craft priority",
			placements: []model.Placement{
				place(t, cat, "water_potion", 0, model.Coord{Row: 0, Col: 0}, 0),
				place(t, cat, "banana", 1, model.Coord{Row: 0, Col: 0}, 0),
			},
			priorities: []string{"craft:endurance_potion"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			full := scoring.EvaluateLayoutWithCoverageGroups(cat, tt.placements, tt.priorities, tt.coverageGroups).Score
			scoreOnly := scoring.EvaluateScoreOnlyWithCoverageGroups(cat, tt.placements, tt.priorities, tt.coverageGroups)
			if !reflect.DeepEqual(scoreOnly, full) {
				t.Fatalf("score-only=%+v full=%+v", scoreOnly, full)
			}
		})
	}
}

func TestOutgoingV2ScoresSourceTargetsWithoutImplicitCoverage(t *testing.T) {
	cat := loadTestCatalog(t)
	placements := []model.Placement{
		place(t, cat, "mana_crystal", 0, model.Coord{Row: 4, Col: 2}, 0),
		place(t, cat, "starlight_potion", 1, model.Coord{Row: 1, Col: 2}, 0),
	}
	priorities := []string{"star_source:mana_crystal"}
	full := scoring.EvaluateLayoutWithCoverageGroupsAndSemantics(
		cat,
		placements,
		priorities,
		nil,
		model.PrioritySemanticsOutgoingV2,
	)
	scoreOnly := scoring.EvaluateScoreOnlyWithCoverageGroupsAndSemantics(
		cat,
		placements,
		priorities,
		nil,
		model.PrioritySemanticsOutgoingV2,
	)
	if !reflect.DeepEqual(scoreOnly, full.Score) {
		t.Fatalf("score-only=%+v full=%+v", scoreOnly, full.Score)
	}
	if full.StarCoverage != nil {
		t.Fatalf("outgoing-v2 synthesized coverage=%+v", full.StarCoverage)
	}
	if !reflect.DeepEqual(full.Score.PriorityCounts, []int{1}) {
		t.Fatalf("priority counts=%v want [1]", full.Score.PriorityCounts)
	}
	if !reflect.DeepEqual(full.LooseStarPriorities, []model.LooseStarPriority{{SourceItemID: "mana_crystal", TargetCount: 1}}) {
		t.Fatalf("loose priorities=%+v", full.LooseStarPriorities)
	}
}

func TestOutgoingPerInstanceV3CountsSharedTargetsForEachCopy(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("spice#0", "spice", 0),
		minimalPlacement("spice#1", "spice", 1),
		minimalPlacement("spice#2", "spice", 2),
		minimalPlacement("banana#3", "banana", 3),
		minimalPlacement("pitahaya#4", "pitahaya", 4),
		minimalPlacement("spicy_sausage#5", "spicy_sausage", 5),
		minimalPlacement("tender_sausage#6", "tender_sausage", 6),
	}
	targets := []string{"banana#3", "pitahaya#4", "spicy_sausage#5", "tender_sausage#6"}
	stars := make([]model.StarActivation, 0, 12)
	for source := 0; source < 3; source++ {
		for _, target := range targets {
			stars = append(stars, model.StarActivation{SourceInstance: "spice#" + strconv.Itoa(source), TargetInstance: target})
		}
	}
	// An accidental duplicate star position must not create another static link.
	stars = append(stars, model.StarActivation{SourceInstance: "spice#0", TargetInstance: "banana#3"})

	v2Counts, _, _, v2Loose := scoring.EvaluatePriorityScoreWithCoverageGroupsAndSemantics(
		model.Catalog{}, placements, nil, stars, []string{"star_source:spice"}, nil, model.PrioritySemanticsOutgoingV2,
	)
	if !reflect.DeepEqual(v2Counts, []int{4}) || len(v2Loose) != 1 || v2Loose[0].TargetCount != 4 || v2Loose[0].LinkCount != 0 {
		t.Fatalf("outgoing-v2 counts=%v loose=%+v want four shared targets", v2Counts, v2Loose)
	}

	v3Counts, _, _, v3Loose := scoring.EvaluatePriorityScoreWithCoverageGroupsAndSemantics(
		model.Catalog{}, placements, nil, stars, []string{"star_source:spice"}, nil, model.PrioritySemanticsOutgoingPerInstanceV3,
	)
	if !reflect.DeepEqual(v3Counts, []int{12}) {
		t.Fatalf("outgoing-per-instance-v3 counts=%v want [12]", v3Counts)
	}
	if len(v3Loose) != 1 || v3Loose[0].TargetCount != 4 || v3Loose[0].LinkCount != 12 {
		t.Fatalf("outgoing-per-instance-v3 loose=%+v want four targets and twelve links", v3Loose)
	}
	wantPerCopy := []model.StarInstanceTargetCount{
		{SourceInstance: "spice#0", TargetCount: 4},
		{SourceInstance: "spice#1", TargetCount: 4},
		{SourceInstance: "spice#2", TargetCount: 4},
	}
	if !reflect.DeepEqual(v3Loose[0].InstanceTargetCounts, wantPerCopy) {
		t.Fatalf("per-copy counts=%+v want %+v", v3Loose[0].InstanceTargetCounts, wantPerCopy)
	}
}

func TestOutgoingPerInstanceV3ScoreOnlyMatchesFullEvaluation(t *testing.T) {
	cat := model.Catalog{Items: map[string]model.Item{
		"source": {
			ID:        "source",
			Shape:     []model.Coord{{Row: 0, Col: 0}},
			Rotations: []int{0},
			Stars:     []model.Star{{Offset: model.Coord{Row: 0, Col: 1}, TargetTypes: []string{"Food"}}},
		},
		"food": {
			ID:        "food",
			Types:     []string{"Food"},
			Shape:     []model.Coord{{Row: 0, Col: 0}},
			Rotations: []int{0},
		},
	}}
	placements := []model.Placement{
		{InstanceID: "source#0", ItemID: "source", Cells: []model.Coord{{Row: 0, Col: 0}}, StarPositions: []model.StarPosition{{Star: cat.Items["source"].Stars[0], Position: model.Coord{Row: 0, Col: 1}}}},
		{InstanceID: "food#1", ItemID: "food", Cells: []model.Coord{{Row: 0, Col: 1}}},
		{InstanceID: "source#2", ItemID: "source", Cells: []model.Coord{{Row: 1, Col: 0}}, StarPositions: []model.StarPosition{{Star: cat.Items["source"].Stars[0], Position: model.Coord{Row: 1, Col: 1}}}},
		{InstanceID: "food#3", ItemID: "food", Cells: []model.Coord{{Row: 1, Col: 1}}},
	}
	priorities := []string{"star_source:source"}
	full := scoring.EvaluateLayoutWithCoverageGroupsAndSemantics(cat, placements, priorities, nil, model.PrioritySemanticsOutgoingPerInstanceV3)
	fast := scoring.EvaluateScoreOnlyWithCoverageGroupsAndSemantics(cat, placements, priorities, nil, model.PrioritySemanticsOutgoingPerInstanceV3)
	if !reflect.DeepEqual(fast, full.Score) {
		t.Fatalf("score-only=%+v full=%+v", fast, full.Score)
	}
	if !reflect.DeepEqual(full.Score.PriorityCounts, []int{2}) || len(full.LooseStarPriorities) != 1 || full.LooseStarPriorities[0].LinkCount != 2 {
		t.Fatalf("full evaluation=%+v", full)
	}
}

func TestStarMatchesItemCompiledMatchesFallback(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		name         string
		sourceID     string
		starItemID   string
		targetItemID string
		want         bool
	}{
		{name: "any item", sourceID: "life_essence", starItemID: "life_essence", targetItemID: "apple", want: true},
		{name: "target type matches", sourceID: "power_stone", starItemID: "power_stone", targetItemID: "fine_sword", want: true},
		{name: "target type misses", sourceID: "power_stone", starItemID: "power_stone", targetItemID: "cactus", want: false},
		{name: "target item matches counts-as", sourceID: "mana_crystal", starItemID: "mana_crystal", targetItemID: "starlight_potion", want: true},
		{name: "target item misses", sourceID: "mana_crystal", starItemID: "mana_crystal", targetItemID: "mana_potion", want: false},
		{name: "exclude source item", sourceID: "spice", starItemID: "spice", targetItemID: "spice", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			star := cat.Items[tt.starItemID].Stars[0]
			target := cat.Items[tt.targetItemID]
			fallbackTarget := uncompiledItem(target)
			fallbackStar := uncompiledStar(star)
			compiled := scoring.StarMatchesItem(tt.sourceID, tt.targetItemID, &target, &star)
			fallback := scoring.StarMatchesItem(tt.sourceID, tt.targetItemID, &fallbackTarget, &fallbackStar)
			if compiled != tt.want {
				t.Fatalf("compiled match=%v want %v", compiled, tt.want)
			}
			if fallback != compiled {
				t.Fatalf("fallback match=%v compiled=%v", fallback, compiled)
			}
		})
	}
}

func uncompiledItem(item model.Item) model.Item {
	item.CompiledTypeMask = 0
	item.CompiledItemID = 0
	item.CompiledAliasItemID = 0
	item.CompiledAliasItemLen = 0
	item.CompiledItemRefLen = 0
	item.CompiledReady = false
	item.CompiledItemRefsComplete = false
	return item
}

func uncompiledStar(star model.Star) model.Star {
	star.CompiledTargetTypeMask = 0
	star.CompiledTargetItemID = 0
	star.CompiledTargetItemLen = 0
	star.CompiledReady = false
	star.CompiledTargetItemsComplete = false
	return star
}

func TestSpinegrowthStarCountsSameTargetOnce(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "spinegrowth_breastplate", 0, model.Coord{Row: 0, Col: 1}, 0)
	target := place(t, cat, "poison_potion", 1, model.Coord{Row: 0, Col: 3}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1", len(stars))
	}
}

func TestBabyCoreCreeperIgnoresWrongTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "baby_core_creeper", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestBabyCoreCreeperCountsMoltenCoreCreeperTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["molten_core_creeper"] = model.Item{
		ID:    "molten_core_creeper",
		Name:  "Molten Core Creeper",
		Types: []string{"Pet"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	source := place(t, cat, "baby_core_creeper", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "molten_core_creeper", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestKnightSigilStarCountsArmorTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "knight_s_sigil", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "armor_kit", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "knight_s_sigil#0" || stars[0].TargetInstance != "armor_kit#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestKnightSigilStarIgnoresNonArmorTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "knight_s_sigil", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "golden_tail", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "knight_s_sigil#0") != 0 {
		t.Fatalf("unexpected knight_s_sigil stars: %+v", stars)
	}
}

func TestShamanTalismanStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "shaman_s_talisman", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "royal_seax", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestShamanTalismanStarCountsDoombringerTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "shaman_s_talisman", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "doombringer", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestBatCountsAsPetStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "shaman_s_talisman", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "bat", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "shaman_s_talisman#0" || stars[0].TargetInstance != "bat#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestRuneOfRlyehStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "rune_of_r_lyeh", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "ragnarok", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestVenomousPincerStarIgnoresInvalidTargetType(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "venomous_pincer", 0, model.Coord{Row: 3, Col: 3}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 3, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestWeaponsRackStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "weapons_rack", 0, model.Coord{Row: 0, Col: 0}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestWeaponsRackStarIgnoresInvalidTargetType(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "weapons_rack", 0, model.Coord{Row: 0, Col: 0}, 0)
	target := place(t, cat, "spinegrowth_breastplate", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestPowerStoneStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "power_stone", 0, model.Coord{Row: 4, Col: 3}, 0)
	target := place(t, cat, "champion_s_ripper", 1, model.Coord{Row: 0, Col: 3}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestGlovesOfPowerStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "gloves_of_power", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestGlovesOfPowerStarCountsRangedWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "gloves_of_power", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "piercing_lance", 1, model.Coord{Row: 1, Col: 3}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestGlovesOfPowerStarIgnoresInvalidTargetType(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "gloves_of_power", 0, model.Coord{Row: 1, Col: 2}, 0)
	target := place(t, cat, "spinegrowth_breastplate", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	var gloveStars int
	for _, star := range stars {
		if star.SourceInstance == "gloves_of_power#0" {
			gloveStars++
		}
	}
	if gloveStars != 0 {
		t.Fatalf("gloves stars=%d want 0: %+v", gloveStars, stars)
	}
}

func TestIronBarStarCountsAnyTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "iron_bar", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "iron_bar#0" || stars[0].TargetInstance != "cactus#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestWhetstoneStarCountsWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "whetstone", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "whetstone#0" || stars[0].TargetInstance != "fine_sword#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestWhetstoneStarIgnoresNonWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "whetstone", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestAmethystPendantStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "amethyst_pendant", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "amethyst_pendant#0" || stars[0].TargetInstance != "fine_sword#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestAmethystPendantStarCountsRangedWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "amethyst_pendant", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "piercing_lance", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "amethyst_pendant#0" || stars[0].TargetInstance != "piercing_lance#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestAmethystPendantStarIgnoresNonWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "amethyst_pendant", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestMagicEssenceStarCountsAnyTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "magic_essence", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "magic_essence#0" || stars[0].TargetInstance != "cactus#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestBeltStarsCountPotionTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"leather_belt", "vanguard_belt", "magic_belt"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, itemID, 0, model.Coord{Row: 1, Col: 1}, 0)
			target := place(t, cat, "water_potion", 1, model.Coord{Row: 1, Col: 0}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, itemID+"#0") != 1 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestBeltStarsIgnoreNonPotionTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"leather_belt", "vanguard_belt", "magic_belt"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, itemID, 0, model.Coord{Row: 1, Col: 1}, 0)
			target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 0}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, itemID+"#0") != 0 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestNewFoodStarsCountOtherFoodTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"lucky_tuna", "carp", "fly_agaric", "coconut", "carrot", "watermelon", "ginseng_root"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, itemID, 0, model.Coord{Row: 1, Col: 1}, 0)
			target := place(t, cat, "spice", 1, model.Coord{Row: 1, Col: 0}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, itemID+"#0") != 1 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestNewFoodStarsIgnoreSameItemTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		itemID       string
		targetOrigin model.Coord
	}{
		{itemID: "lucky_tuna", targetOrigin: model.Coord{Row: 2, Col: 0}},
		{itemID: "carp", targetOrigin: model.Coord{Row: 2, Col: 1}},
		{itemID: "fly_agaric", targetOrigin: model.Coord{Row: 1, Col: 2}},
		{itemID: "coconut", targetOrigin: model.Coord{Row: 1, Col: 2}},
		{itemID: "carrot", targetOrigin: model.Coord{Row: 1, Col: 2}},
		{itemID: "watermelon", targetOrigin: model.Coord{Row: 1, Col: 1}},
		{itemID: "ginseng_root", targetOrigin: model.Coord{Row: 2, Col: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.itemID, func(t *testing.T) {
			source := place(t, cat, tt.itemID, 0, model.Coord{Row: 2, Col: 3}, 0)
			target := place(t, cat, tt.itemID, 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.itemID+"#0") != 0 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestRatomelonStarsCountRatAndFoodTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	ratSource := place(t, cat, "ratomelon", 0, model.Coord{Row: 2, Col: 2}, 0)
	ratTarget := place(t, cat, "brown_rat", 1, model.Coord{Row: 2, Col: 0}, 0)

	ratStars := scoring.EvaluateStars(cat, []model.Placement{ratSource, ratTarget})

	if countStarsFromSource(ratStars, "ratomelon#0") != 1 {
		t.Fatalf("unexpected rat stars: %+v", ratStars)
	}

	foodSource := place(t, cat, "ratomelon", 0, model.Coord{Row: 2, Col: 2}, 0)
	foodTarget := place(t, cat, "apple", 1, model.Coord{Row: 2, Col: 1}, 0)

	foodStars := scoring.EvaluateStars(cat, []model.Placement{foodSource, foodTarget})

	if countStarsFromSource(foodStars, "ratomelon#0") != 1 {
		t.Fatalf("unexpected food stars: %+v", foodStars)
	}
}

func TestLifeEssenceStarCountsDiagonalAnyTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "life_essence", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "life_essence#0" || stars[0].TargetInstance != "cactus#1" {
		t.Fatalf("unexpected star: %+v", stars[0])
	}
}

func TestDiscordantHarpStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "discordant_harp", 0, model.Coord{Row: 0, Col: 2}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "discordant_harp#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestDiscordantHarpStarCountsRangedWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "discordant_harp", 0, model.Coord{Row: 0, Col: 2}, 0)
	target := place(t, cat, "piercing_lance", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "discordant_harp#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestDiscordantHarpStarCountsPetTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "discordant_harp", 0, model.Coord{Row: 0, Col: 2}, 0)
	target := place(t, cat, "baby_core_creeper", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "discordant_harp#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestDiscordantHarpStarIgnoresInvalidTargetType(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "discordant_harp", 0, model.Coord{Row: 0, Col: 4}, 0)
	target := place(t, cat, "spinegrowth_breastplate", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "discordant_harp#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestGlovesOfPowerIsInvalidTargetForWeaponPetStarSources(t *testing.T) {
	cat := loadTestCatalog(t)
	cases := []struct {
		name         string
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{name: "weapons_rack", sourceID: "weapons_rack", sourceOrigin: model.Coord{Row: 0, Col: 0}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{name: "power_stone", sourceID: "power_stone", sourceOrigin: model.Coord{Row: 4, Col: 3}, targetOrigin: model.Coord{Row: 3, Col: 1}},
		{name: "rune_of_r_lyeh", sourceID: "rune_of_r_lyeh", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{name: "shaman_s_talisman", sourceID: "shaman_s_talisman", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 1}},
		{name: "venomous_pincer", sourceID: "venomous_pincer", sourceOrigin: model.Coord{Row: 3, Col: 3}, targetOrigin: model.Coord{Row: 0, Col: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := place(t, cat, tc.sourceID, 0, tc.sourceOrigin, 0)
			target := place(t, cat, "gloves_of_power", 1, tc.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			var matchingSourceStars int
			for _, star := range stars {
				if star.SourceInstance == tc.sourceID+"#0" {
					matchingSourceStars++
				}
			}
			if matchingSourceStars != 0 {
				t.Fatalf("%s stars=%d want 0: %+v", tc.sourceID, matchingSourceStars, stars)
			}
		})
	}
}

func TestCatalogMeleeWeaponsAreValidFilteredStarTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"wooden_club", "wooden_sword", "iron_sword", "fine_sword", "amethyst_blade", "champion_s_ripper", "hooked_blade", "training_blade", "dagger", "jagged_blade", "knight_s_razor", "mallet", "fanged_blade"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, "shaman_s_talisman", 0, model.Coord{Row: 2, Col: 2}, 0)
			target := place(t, cat, itemID, 1, model.Coord{Row: 0, Col: 1}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if len(stars) != 1 {
				t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
			}
		})
	}
}

func TestCatalogRangedWeaponsAreValidFilteredStarTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"rock", "wooden_stick", "piercing_lance", "searing_wand", "blazing_rod", "hungry_wand", "tournament_lance", "errant_lance", "rhongomiant"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, "shaman_s_talisman", 0, model.Coord{Row: 2, Col: 2}, 0)
			target := place(t, cat, itemID, 1, model.Coord{Row: 1, Col: 1}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if len(stars) != 1 {
				t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
			}
		})
	}
}

func TestErrantLanceStarCountsCactusAndCountsAsOnly(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "errant_lance", 0, model.Coord{Row: 1, Col: 1}, 0)
	cactus := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 1}, 0)
	cactrio := place(t, cat, "cactrio", 2, model.Coord{Row: 2, Col: 0}, 0)
	succulent := place(t, cat, "succulent", 3, model.Coord{Row: 2, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, cactus, cactrio, succulent})

	if len(stars) != 2 {
		t.Fatalf("len(stars)=%d want 2: %+v", len(stars), stars)
	}
	wantTargets := map[string]bool{"cactus#1": false, "cactrio#2": false}
	for _, star := range stars {
		if star.SourceInstance != "errant_lance#0" {
			t.Fatalf("unexpected source: %+v", star)
		}
		if _, ok := wantTargets[star.TargetInstance]; !ok {
			t.Fatalf("unexpected target: %+v", star)
		}
		wantTargets[star.TargetInstance] = true
	}
	for target, seen := range wantTargets {
		if !seen {
			t.Fatalf("missing target %s in stars: %+v", target, stars)
		}
	}
}

func TestRhongomiantStarCountsCactusAndCountsAsOnly(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "rhongomiant", 0, model.Coord{Row: 1, Col: 1}, 0)
	cactus := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 1}, 0)
	cactrio := place(t, cat, "cactrio", 2, model.Coord{Row: 3, Col: 0}, 0)
	succulent := place(t, cat, "succulent", 3, model.Coord{Row: 3, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, cactus, cactrio, succulent})

	if len(stars) != 2 {
		t.Fatalf("len(stars)=%d want 2: %+v", len(stars), stars)
	}
	wantTargets := map[string]bool{"cactus#1": false, "cactrio#2": false}
	for _, star := range stars {
		if star.SourceInstance != "rhongomiant#0" {
			t.Fatalf("unexpected source: %+v", star)
		}
		if _, ok := wantTargets[star.TargetInstance]; !ok {
			t.Fatalf("unexpected target: %+v", star)
		}
		wantTargets[star.TargetInstance] = true
	}
	for target, seen := range wantTargets {
		if !seen {
			t.Fatalf("missing target %s in stars: %+v", target, stars)
		}
	}
}

func TestBrainsquasherCountsAsMeleeWeaponStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "shaman_s_talisman", 0, model.Coord{Row: 3, Col: 2}, 0)
	target := place(t, cat, "brainsquasher", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "shaman_s_talisman#0" || stars[0].TargetInstance != "brainsquasher#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestEtherealCloakCountsAsArmorStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "knight_s_sigil", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "ethereal_cloak", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "knight_s_sigil#0" || stars[0].TargetInstance != "ethereal_cloak#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestMysteryStewCountsIngredientStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mystery_stew", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "spice", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "mystery_stew#0" || stars[0].TargetInstance != "spice#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestMysteryStewIgnoresNonIngredientStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mystery_stew", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "mallet", 1, model.Coord{Row: 0, Col: 1}, 90)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestLeatherBootsCountsAsArmorStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "knight_s_sigil", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "leather_boots", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "knight_s_sigil#0" || stars[0].TargetInstance != "leather_boots#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestSkullCapAndFurBootsCountAsArmorStarTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"skull_cap", "fur_boots", "wooden_buckler", "heater_shield", "reinforced_shield"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, "knight_s_sigil", 0, model.Coord{Row: 2, Col: 2}, 0)
			target := place(t, cat, itemID, 1, model.Coord{Row: 0, Col: 1}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if len(stars) != 1 {
				t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
			}
		})
	}
}

func TestBeltsCountAsArmorStarTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, itemID := range []string{"leather_belt", "vanguard_belt", "magic_belt"} {
		t.Run(itemID, func(t *testing.T) {
			source := place(t, cat, "knight_s_sigil", 0, model.Coord{Row: 2, Col: 2}, 0)
			target := place(t, cat, itemID, 1, model.Coord{Row: 1, Col: 2}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if len(stars) != 1 {
				t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
			}
		})
	}
}

func TestNewArmorStarSourcesCountArmorTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{sourceID: "young_squire", sourceOrigin: model.Coord{Row: 0, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 3}},
		{sourceID: "knight_s_blessing", sourceOrigin: model.Coord{Row: 0, Col: 3}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{sourceID: "ironwill_banner", sourceOrigin: model.Coord{Row: 0, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{sourceID: "hungering_ward", sourceOrigin: model.Coord{Row: 0, Col: 2}, targetOrigin: model.Coord{Row: 0, Col: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			target := place(t, cat, "skull_cap", 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.sourceID+"#0") != 1 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestNewArmorStarSourcesIgnoreNonArmorTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{sourceID: "young_squire", sourceOrigin: model.Coord{Row: 0, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 3}},
		{sourceID: "knight_s_blessing", sourceOrigin: model.Coord{Row: 0, Col: 3}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{sourceID: "ironwill_banner", sourceOrigin: model.Coord{Row: 0, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{sourceID: "hungering_ward", sourceOrigin: model.Coord{Row: 0, Col: 2}, targetOrigin: model.Coord{Row: 0, Col: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			target := place(t, cat, "cactus", 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.sourceID+"#0") != 0 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestFortressRingCountsArmorStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "fortress_ring", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "scalemail", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestFortressRingIgnoresNonArmorStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "fortress_ring", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestFurBootsCountsWeaponStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "fur_boots", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "fur_boots#0" || stars[0].TargetInstance != "fine_sword#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestFurBootsIgnoresNonWeaponStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "fur_boots", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 2, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestSteadfastBootsCountsWeaponStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "steadfast_boots", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 0, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "steadfast_boots#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestSteadfastBootsIgnoresNonWeaponStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "steadfast_boots", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 2, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "steadfast_boots#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestBrownRatCountsRatTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["test_rat"] = model.Item{
		ID:    "test_rat",
		Name:  "Test Rat",
		Types: []string{"Rat"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	source := place(t, cat, "brown_rat", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "test_rat", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
	if stars[0].SourceInstance != "brown_rat#0" || stars[0].TargetInstance != "test_rat#1" {
		t.Fatalf("unexpected star activation: %+v", stars[0])
	}
}

func TestBrownRatIgnoresNonRatTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "brown_rat", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "training_blade", 1, model.Coord{Row: 0, Col: 2}, 90)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestCursedIdolCountsSkullStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "cursed_idol", 0, model.Coord{Row: 3, Col: 3}, 0)
	target := place(t, cat, "golden_skull", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestCursedIdolIgnoresNonSkullStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "cursed_idol", 0, model.Coord{Row: 3, Col: 3}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestGamblersDiceCountsPlantStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "gambler_s_dice", 0, model.Coord{Row: 4, Col: 4}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 5}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestGamblersDiceIgnoresNonPlantStarTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "gambler_s_dice", 0, model.Coord{Row: 4, Col: 4}, 0)
	target := place(t, cat, "golden_skull", 1, model.Coord{Row: 0, Col: 5}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestManaCrystalCountsStarbloomTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mana_crystal", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "starbloom", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "mana_crystal#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestManaCrystalCountsStarlightPotionViaCountsAs(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mana_crystal", 0, model.Coord{Row: 4, Col: 2}, 0)
	target := place(t, cat, "starlight_potion", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "mana_crystal#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestManaCrystalIgnoresTargetWithoutStarbloomAlias(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mana_crystal", 0, model.Coord{Row: 4, Col: 2}, 0)
	target := place(t, cat, "mana_potion", 1, model.Coord{Row: 2, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "mana_crystal#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestMarshmallowCountsOtherFoodTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "marshmallow", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "apple", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "marshmallow#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestMarshmallowIgnoresSameItemTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "marshmallow", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "marshmallow", 1, model.Coord{Row: 0, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "marshmallow#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestWandOfBackdraftCountsWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "wand_of_backdraft", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 0, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "wand_of_backdraft#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestWandOfBackdraftIgnoresNonWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "wand_of_backdraft", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "wand_of_backdraft#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestBeltOfFirethrowingCountsPotionTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "belt_of_firethrowing", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "water_potion", 1, model.Coord{Row: 2, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "belt_of_firethrowing#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestBeltOfFirethrowingIgnoresNonPotionTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "belt_of_firethrowing", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "cactus", 1, model.Coord{Row: 2, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "belt_of_firethrowing#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestNewAnyStarSourcesCountAnyTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{sourceID: "wildfire_essence", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 1}},
		{sourceID: "conflagration_staff", sourceOrigin: model.Coord{Row: 1, Col: 2}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{sourceID: "starbloom", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			target := place(t, cat, "cactus", 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.sourceID+"#0") != 1 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestManaStewCountsOtherFoodTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mana_stew", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "apple", 1, model.Coord{Row: 2, Col: 1}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "mana_stew#0") != 1 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestManaStewIgnoresSameItemFoodTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "mana_stew", 0, model.Coord{Row: 2, Col: 2}, 0)
	target := place(t, cat, "mana_stew", 1, model.Coord{Row: 2, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if countStarsFromSource(stars, "mana_stew#0") != 0 {
		t.Fatalf("unexpected stars: %+v", stars)
	}
}

func TestNewWeaponPetStarSourcesCountPetTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{sourceID: "critical_focus", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{sourceID: "frostflame_orb", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 2}},
		{sourceID: "flame_cloak", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{sourceID: "wizard_hat", sourceOrigin: model.Coord{Row: 1, Col: 3}, targetOrigin: model.Coord{Row: 2, Col: 2}},
		{sourceID: "icicle_shard", sourceOrigin: model.Coord{Row: 1, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 1}},
		{sourceID: "frost_tome", sourceOrigin: model.Coord{Row: 1, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			target := place(t, cat, "baby_core_creeper", 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.sourceID+"#0") != 1 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestNewWeaponPetStarSourcesIgnoreInvalidTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{sourceID: "critical_focus", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{sourceID: "frostflame_orb", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 2}},
		{sourceID: "flame_cloak", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{sourceID: "wizard_hat", sourceOrigin: model.Coord{Row: 1, Col: 3}, targetOrigin: model.Coord{Row: 2, Col: 2}},
		{sourceID: "icicle_shard", sourceOrigin: model.Coord{Row: 1, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 1}},
		{sourceID: "frost_tome", sourceOrigin: model.Coord{Row: 1, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			target := place(t, cat, "cactus", 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.sourceID+"#0") != 0 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestFireVortexCountsRangedWeaponOnly(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "fire_vortex", 0, model.Coord{Row: 2, Col: 2}, 0)
	rangedTarget := place(t, cat, "searing_wand", 1, model.Coord{Row: 0, Col: 2}, 0)
	meleeTarget := place(t, cat, "dagger", 2, model.Coord{Row: 0, Col: 2}, 0)

	rangedStars := scoring.EvaluateStars(cat, []model.Placement{source, rangedTarget})
	if countStarsFromSource(rangedStars, "fire_vortex#0") != 1 {
		t.Fatalf("unexpected ranged stars: %+v", rangedStars)
	}

	meleeStars := scoring.EvaluateStars(cat, []model.Placement{source, meleeTarget})
	if countStarsFromSource(meleeStars, "fire_vortex#0") != 0 {
		t.Fatalf("unexpected melee stars: %+v", meleeStars)
	}
}

func TestNewScrollStarSourcesCountRangedOrPetTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, sourceID := range []string{"spell_scroll", "arcane_scroll", "frost_scroll", "phoenix_scroll", "wild_magic_scroll"} {
		t.Run(sourceID, func(t *testing.T) {
			source := place(t, cat, sourceID, 0, model.Coord{Row: 2, Col: 2}, 0)
			target := place(t, cat, "baby_core_creeper", 1, model.Coord{Row: 1, Col: 2}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, sourceID+"#0") != 1 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestNewScrollStarSourcesIgnoreMeleeTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	for _, sourceID := range []string{"spell_scroll", "arcane_scroll", "frost_scroll", "phoenix_scroll", "wild_magic_scroll"} {
		t.Run(sourceID, func(t *testing.T) {
			source := place(t, cat, sourceID, 0, model.Coord{Row: 2, Col: 2}, 0)
			target := place(t, cat, "dagger", 1, model.Coord{Row: 0, Col: 2}, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, sourceID+"#0") != 0 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestNewRatFoodStarSourcesCountRatAndFoodTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		ratOrigin    model.Coord
		foodOrigin   model.Coord
	}{
		{sourceID: "moldemort", sourceOrigin: model.Coord{Row: 2, Col: 2}, ratOrigin: model.Coord{Row: 1, Col: 1}, foodOrigin: model.Coord{Row: 1, Col: 2}},
		{sourceID: "blue_chilly_cheese", sourceOrigin: model.Coord{Row: 2, Col: 2}, ratOrigin: model.Coord{Row: 1, Col: 1}, foodOrigin: model.Coord{Row: 1, Col: 2}},
		{sourceID: "succulent_cheese", sourceOrigin: model.Coord{Row: 2, Col: 2}, ratOrigin: model.Coord{Row: 1, Col: 1}, foodOrigin: model.Coord{Row: 1, Col: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			ratTarget := place(t, cat, "brown_rat", 1, tt.ratOrigin, 0)
			foodTarget := place(t, cat, "apple", 2, tt.foodOrigin, 0)

			ratStars := scoring.EvaluateStars(cat, []model.Placement{source, ratTarget})
			if countStarsFromSource(ratStars, tt.sourceID+"#0") != 1 {
				t.Fatalf("unexpected rat stars: %+v", ratStars)
			}

			foodStars := scoring.EvaluateStars(cat, []model.Placement{source, foodTarget})
			if countStarsFromSource(foodStars, tt.sourceID+"#0") != 1 {
				t.Fatalf("unexpected food stars: %+v", foodStars)
			}
		})
	}
}

func TestNewRatFoodStarSourcesIgnoreSameFoodItemTargets(t *testing.T) {
	cat := loadTestCatalog(t)
	tests := []struct {
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{sourceID: "moldemort", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{sourceID: "blue_chilly_cheese", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 2}},
		{sourceID: "succulent_cheese", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 1, Col: 2}},
	}
	for _, tt := range tests {
		t.Run(tt.sourceID, func(t *testing.T) {
			source := place(t, cat, tt.sourceID, 0, tt.sourceOrigin, 0)
			target := place(t, cat, tt.sourceID, 1, tt.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if countStarsFromSource(stars, tt.sourceID+"#0") != 0 {
				t.Fatalf("unexpected stars: %+v", stars)
			}
		})
	}
}

func TestPiercingLanceStarCountsMeleeWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "piercing_lance", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestPiercingLanceStarCountsRangedWeaponTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["test_ranged_weapon"] = model.Item{
		ID:    "test_ranged_weapon",
		Name:  "Test Ranged Weapon",
		Types: []string{"Ranged Weapon"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	source := place(t, cat, "piercing_lance", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "test_ranged_weapon", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestPiercingLanceStarIgnoresInvalidTargetType(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "piercing_lance", 0, model.Coord{Row: 1, Col: 2}, 0)
	target := place(t, cat, "spinegrowth_breastplate", 1, model.Coord{Row: 1, Col: 0}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	var piercingLanceStars int
	for _, star := range stars {
		if star.SourceInstance == "piercing_lance#0" {
			piercingLanceStars++
		}
	}
	if piercingLanceStars != 0 {
		t.Fatalf("piercing lance stars=%d want 0: %+v", piercingLanceStars, stars)
	}
}

func TestPiercingLanceIsValidTargetForFilteredStarSources(t *testing.T) {
	cat := loadTestCatalog(t)
	cases := []struct {
		name         string
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{name: "weapons_rack", sourceID: "weapons_rack", sourceOrigin: model.Coord{Row: 0, Col: 0}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{name: "power_stone", sourceID: "power_stone", sourceOrigin: model.Coord{Row: 4, Col: 3}, targetOrigin: model.Coord{Row: 0, Col: 3}},
		{name: "rune_of_r_lyeh", sourceID: "rune_of_r_lyeh", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{name: "shaman_s_talisman", sourceID: "shaman_s_talisman", sourceOrigin: model.Coord{Row: 2, Col: 2}, targetOrigin: model.Coord{Row: 0, Col: 1}},
		{name: "venomous_pincer", sourceID: "venomous_pincer", sourceOrigin: model.Coord{Row: 3, Col: 3}, targetOrigin: model.Coord{Row: 0, Col: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := place(t, cat, tc.sourceID, 0, tc.sourceOrigin, 0)
			target := place(t, cat, "piercing_lance", 1, tc.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if len(stars) != 1 {
				t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
			}
		})
	}
}

func TestExcaliburIsValidTargetForFilteredStarSources(t *testing.T) {
	cat := loadTestCatalog(t)
	cases := []struct {
		name         string
		sourceID     string
		sourceOrigin model.Coord
		targetOrigin model.Coord
	}{
		{name: "weapons_rack", sourceID: "weapons_rack", sourceOrigin: model.Coord{Row: 0, Col: 0}, targetOrigin: model.Coord{Row: 1, Col: 0}},
		{name: "power_stone", sourceID: "power_stone", sourceOrigin: model.Coord{Row: 4, Col: 3}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{name: "rune_of_r_lyeh", sourceID: "rune_of_r_lyeh", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{name: "shaman_s_talisman", sourceID: "shaman_s_talisman", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 1}},
		{name: "venomous_pincer", sourceID: "venomous_pincer", sourceOrigin: model.Coord{Row: 3, Col: 3}, targetOrigin: model.Coord{Row: 0, Col: 0}},
		{name: "piercing_lance", sourceID: "piercing_lance", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
		{name: "gloves_of_power", sourceID: "gloves_of_power", sourceOrigin: model.Coord{Row: 1, Col: 1}, targetOrigin: model.Coord{Row: 0, Col: 2}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := place(t, cat, tc.sourceID, 0, tc.sourceOrigin, 0)
			target := place(t, cat, "excalibur", 1, tc.targetOrigin, 0)

			stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

			if len(stars) != 1 {
				t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
			}
		})
	}
}

func TestFoodStarCountsDifferentFoodTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "apple", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "spice", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 1 {
		t.Fatalf("len(stars)=%d want 1: %+v", len(stars), stars)
	}
}

func TestFoodStarIgnoresSameItemID(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "apple", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "apple", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestSpiceStarCountsDifferentFoodTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "spice", 0, model.Coord{Row: 4, Col: 4}, 0)
	target := place(t, cat, "apple", 1, model.Coord{Row: 3, Col: 4}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	var spiceStars int
	for _, star := range stars {
		if star.SourceInstance == "spice#0" {
			spiceStars++
		}
	}
	if spiceStars != 1 {
		t.Fatalf("spice stars=%d want 1: %+v", spiceStars, stars)
	}
}

func TestFoodStarIgnoresNonFoodTarget(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "apple", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "fine_sword", 1, model.Coord{Row: 1, Col: 2}, 0)

	stars := scoring.EvaluateStars(cat, []model.Placement{source, target})

	if len(stars) != 0 {
		t.Fatalf("len(stars)=%d want 0: %+v", len(stars), stars)
	}
}

func TestFoodStarPriorityRespectsSameItemExclusion(t *testing.T) {
	cat := loadTestCatalog(t)
	source := place(t, cat, "apple", 0, model.Coord{Row: 1, Col: 1}, 0)
	target := place(t, cat, "apple", 1, model.Coord{Row: 1, Col: 2}, 0)

	evaluation := scoring.EvaluateLayoutWithPriorities(cat, []model.Placement{source, target}, []string{"star_source:apple"})

	if len(evaluation.Score.PriorityCounts) != 1 || evaluation.Score.PriorityCounts[0] != 0 {
		t.Fatalf("priority counts=%v want [0]", evaluation.Score.PriorityCounts)
	}
	if evaluation.Score.StarCount != 0 {
		t.Fatalf("star count=%d want 0", evaluation.Score.StarCount)
	}
}

func TestCraftValidWhenAnchorTouchesAllIngredients(t *testing.T) {
	cat := loadTestCatalog(t)
	scalemail := place(t, cat, "scalemail", 0, model.Coord{Row: 1, Col: 1}, 0)
	thornwall := place(t, cat, "thornwall", 1, model.Coord{Row: 1, Col: 3}, 0)
	armorKit := place(t, cat, "armor_kit", 2, model.Coord{Row: 4, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{scalemail, thornwall, armorKit})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1", len(crafts))
	}
	if crafts[0].RecipeResult != "spinegrowth_breastplate" {
		t.Fatalf("recipe result=%q want spinegrowth_breastplate", crafts[0].RecipeResult)
	}
}

func TestCraftInvalidWhenIngredientDoesNotTouchAnchor(t *testing.T) {
	cat := loadTestCatalog(t)
	scalemail := place(t, cat, "scalemail", 0, model.Coord{Row: 1, Col: 1}, 0)
	thornwall := place(t, cat, "thornwall", 1, model.Coord{Row: 1, Col: 3}, 0)
	armorKit := place(t, cat, "armor_kit", 2, model.Coord{Row: 5, Col: 4}, 0)

	evaluation := scoring.EvaluateLayout(cat, []model.Placement{scalemail, thornwall, armorKit})

	if evaluation.Score.CraftCount != 0 {
		t.Fatalf("craft count=%d want 0", evaluation.Score.CraftCount)
	}
}

func TestExcaliburCraftValidWhenChampionRipperTouchesKnightSigil(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["knight_s_sigil"] = model.Item{
		ID:    "knight_s_sigil",
		Name:  "Knight's Sigil",
		Types: []string{"Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	championRipper := place(t, cat, "champion_s_ripper", 0, model.Coord{Row: 0, Col: 0}, 0)
	knightSigil := place(t, cat, "knight_s_sigil", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{championRipper, knightSigil})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "excalibur" {
		t.Fatalf("recipe result=%q want excalibur", crafts[0].RecipeResult)
	}
}

func TestDaggerCraftValidWhenLockpickDartTouchesIronBar(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["lockpick_dart"] = model.Item{
		ID:    "lockpick_dart",
		Name:  "Lockpick Dart",
		Types: []string{"Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	lockpickDart := place(t, cat, "lockpick_dart", 0, model.Coord{Row: 0, Col: 0}, 0)
	ironBar := place(t, cat, "iron_bar", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{lockpickDart, ironBar})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "dagger" {
		t.Fatalf("recipe result=%q want dagger", crafts[0].RecipeResult)
	}
}

func TestIronSwordCraftValidWhenWoodenSwordsTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstSword := place(t, cat, "wooden_sword", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondSword := place(t, cat, "wooden_sword", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstSword, secondSword})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "iron_sword" {
		t.Fatalf("recipe result=%q want iron_sword", crafts[0].RecipeResult)
	}
}

func TestFineSwordCraftValidWhenIronSwordTouchesWhetstone(t *testing.T) {
	cat := loadTestCatalog(t)
	ironSword := place(t, cat, "iron_sword", 0, model.Coord{Row: 0, Col: 0}, 0)
	whetstone := place(t, cat, "whetstone", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{ironSword, whetstone})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "fine_sword" {
		t.Fatalf("recipe result=%q want fine_sword", crafts[0].RecipeResult)
	}
}

func TestAmethystBladeCraftValidWhenFineSwordTouchesAmethystPendant(t *testing.T) {
	cat := loadTestCatalog(t)
	fineSword := place(t, cat, "fine_sword", 0, model.Coord{Row: 0, Col: 0}, 0)
	amethystPendant := place(t, cat, "amethyst_pendant", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{fineSword, amethystPendant})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "amethyst_blade" {
		t.Fatalf("recipe result=%q want amethyst_blade", crafts[0].RecipeResult)
	}
}

func TestSteelBarCraftValidWhenIronBarsTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstBar := place(t, cat, "iron_bar", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondBar := place(t, cat, "iron_bar", 1, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstBar, secondBar})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "steel_bar" {
		t.Fatalf("recipe result=%q want steel_bar", crafts[0].RecipeResult)
	}
}

func TestManaPotionCraftValidWhenWaterPotionTouchesNightDahlia(t *testing.T) {
	cat := loadTestCatalog(t)
	waterPotion := place(t, cat, "water_potion", 0, model.Coord{Row: 0, Col: 0}, 0)
	nightDahlia := place(t, cat, "night_dahlia", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{waterPotion, nightDahlia})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "mana_potion" {
		t.Fatalf("recipe result=%q want mana_potion", crafts[0].RecipeResult)
	}
}

func TestPoisonDaggerCraftValidWhenDaggerTouchesPoisonPotion(t *testing.T) {
	cat := loadTestCatalog(t)
	dagger := place(t, cat, "dagger", 0, model.Coord{Row: 0, Col: 0}, 0)
	poisonPotion := place(t, cat, "poison_potion", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{dagger, poisonPotion})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "poison_dagger" {
		t.Fatalf("recipe result=%q want poison_dagger", crafts[0].RecipeResult)
	}
}

func TestRatomelonCraftValidWhenMuckRatTouchesWatermelon(t *testing.T) {
	cat := loadTestCatalog(t)
	muckRat := place(t, cat, "muck_rat", 0, model.Coord{Row: 0, Col: 0}, 0)
	watermelon := place(t, cat, "watermelon", 1, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{muckRat, watermelon})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "ratomelon" {
		t.Fatalf("recipe result=%q want ratomelon", crafts[0].RecipeResult)
	}
}

func TestTwinmawCraftValidWhenDoubleEnderTouchesFangedAxe(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["double_ender"] = model.Item{
		ID:    "double_ender",
		Name:  "Double Ender",
		Types: []string{"Melee Weapon"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	doubleEnder := place(t, cat, "double_ender", 0, model.Coord{Row: 0, Col: 0}, 0)
	fangedAxe := place(t, cat, "fanged_axe", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{doubleEnder, fangedAxe})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "twinmaw" {
		t.Fatalf("recipe result=%q want twinmaw", crafts[0].RecipeResult)
	}
}

func TestFangedBladeCraftValidWhenJaggedBladeTouchesFangedAxe(t *testing.T) {
	cat := loadTestCatalog(t)
	jaggedBlade := place(t, cat, "jagged_blade", 0, model.Coord{Row: 0, Col: 0}, 0)
	fangedAxe := place(t, cat, "fanged_axe", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{jaggedBlade, fangedAxe})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "fanged_blade" {
		t.Fatalf("recipe result=%q want fanged_blade", crafts[0].RecipeResult)
	}
}

func TestErrantLanceCraftValidWhenTournamentLanceTouchesFangedBlade(t *testing.T) {
	cat := loadTestCatalog(t)
	tournamentLance := place(t, cat, "tournament_lance", 0, model.Coord{Row: 0, Col: 0}, 0)
	fangedBlade := place(t, cat, "fanged_blade", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{tournamentLance, fangedBlade})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "errant_lance" {
		t.Fatalf("recipe result=%q want errant_lance", crafts[0].RecipeResult)
	}
}

func TestRhongomiantCraftValidWhenErrantLanceTouchesKnightSigil(t *testing.T) {
	cat := loadTestCatalog(t)
	errantLance := place(t, cat, "errant_lance", 0, model.Coord{Row: 0, Col: 0}, 0)
	knightSigil := place(t, cat, "knight_s_sigil", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{errantLance, knightSigil})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "rhongomiant" {
		t.Fatalf("recipe result=%q want rhongomiant", crafts[0].RecipeResult)
	}
}

func TestTwinmawCraftValidWhenFangedBladesTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstBlade := place(t, cat, "fanged_blade", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondBlade := place(t, cat, "fanged_blade", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstBlade, secondBlade})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "twinmaw" {
		t.Fatalf("recipe result=%q want twinmaw", crafts[0].RecipeResult)
	}
}

func TestGoldBarCraftValidWhenBronzeBarsTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["bronze_bar"] = model.Item{
		ID:    "bronze_bar",
		Name:  "Bronze Bar",
		Types: []string{"Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	firstBar := place(t, cat, "bronze_bar", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondBar := place(t, cat, "bronze_bar", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstBar, secondBar})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "gold_bar" {
		t.Fatalf("recipe result=%q want gold_bar", crafts[0].RecipeResult)
	}
}

func TestGoldBarCraftValidWhenGoldOresTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["gold_ore"] = model.Item{
		ID:    "gold_ore",
		Name:  "Gold Ore",
		Types: []string{"Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	firstOre := place(t, cat, "gold_ore", 0, model.Coord{Row: 1, Col: 1}, 0)
	secondOre := place(t, cat, "gold_ore", 1, model.Coord{Row: 0, Col: 1}, 0)
	thirdOre := place(t, cat, "gold_ore", 2, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstOre, secondOre, thirdOre})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "gold_bar" {
		t.Fatalf("recipe result=%q want gold_bar", crafts[0].RecipeResult)
	}
}

func TestHeaterShieldCraftValidWhenWoodenBucklersTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstBuckler := place(t, cat, "wooden_buckler", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondBuckler := place(t, cat, "wooden_buckler", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstBuckler, secondBuckler})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "heater_shield" {
		t.Fatalf("recipe result=%q want heater_shield", crafts[0].RecipeResult)
	}
}

func TestReinforcedShieldCraftValidWhenHeaterShieldTouchesIronBar(t *testing.T) {
	cat := loadTestCatalog(t)
	heaterShield := place(t, cat, "heater_shield", 0, model.Coord{Row: 0, Col: 0}, 0)
	ironBar := place(t, cat, "iron_bar", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{heaterShield, ironBar})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "reinforced_shield" {
		t.Fatalf("recipe result=%q want reinforced_shield", crafts[0].RecipeResult)
	}
}

func TestSpikedShieldCraftValidWhenWoodenBucklerTouchesTusk(t *testing.T) {
	cat := loadTestCatalog(t)
	woodenBuckler := place(t, cat, "wooden_buckler", 0, model.Coord{Row: 0, Col: 0}, 0)
	tusk := place(t, cat, "tusk", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{woodenBuckler, tusk})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "spiked_shield" {
		t.Fatalf("recipe result=%q want spiked_shield", crafts[0].RecipeResult)
	}
}

func TestLeatherBeltCraftValidWhenSashesTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstSash := place(t, cat, "sash", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondSash := place(t, cat, "sash", 1, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstSash, secondSash})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "leather_belt" {
		t.Fatalf("recipe result=%q want leather_belt", crafts[0].RecipeResult)
	}
}

func TestVanguardBeltCraftValidWhenLeatherBeltTouchesArmorKit(t *testing.T) {
	cat := loadTestCatalog(t)
	leatherBelt := place(t, cat, "leather_belt", 0, model.Coord{Row: 0, Col: 0}, 0)
	armorKit := place(t, cat, "armor_kit", 1, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{leatherBelt, armorKit})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "vanguard_belt" {
		t.Fatalf("recipe result=%q want vanguard_belt", crafts[0].RecipeResult)
	}
}

func TestMagicBeltCraftValidWhenLeatherBeltTouchesMagicEssence(t *testing.T) {
	cat := loadTestCatalog(t)
	leatherBelt := place(t, cat, "leather_belt", 0, model.Coord{Row: 0, Col: 0}, 0)
	magicEssence := place(t, cat, "magic_essence", 1, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{leatherBelt, magicEssence})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "magic_belt" {
		t.Fatalf("recipe result=%q want magic_belt", crafts[0].RecipeResult)
	}
}

func TestBatCraftValidWhenBrownRatTouchesBooBar(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["boo_bar"] = model.Item{
		ID:    "boo_bar",
		Name:  "Boo Bar",
		Types: []string{"Food"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	brownRat := place(t, cat, "brown_rat", 0, model.Coord{Row: 0, Col: 0}, 0)
	booBar := place(t, cat, "boo_bar", 1, model.Coord{Row: 1, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{brownRat, booBar})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "bat" {
		t.Fatalf("recipe result=%q want bat", crafts[0].RecipeResult)
	}
}

func TestJaggedBladeCraftValidWhenWoodenSwordTouchesCactus(t *testing.T) {
	cat := loadTestCatalog(t)
	woodenSword := place(t, cat, "wooden_sword", 0, model.Coord{Row: 0, Col: 0}, 0)
	cactus := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{woodenSword, cactus})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "jagged_blade" {
		t.Fatalf("recipe result=%q want jagged_blade", crafts[0].RecipeResult)
	}
}

func TestKnightRazorCraftValidWhenJaggedBladeTouchesTrainingBlade(t *testing.T) {
	cat := loadTestCatalog(t)
	jaggedBlade := place(t, cat, "jagged_blade", 0, model.Coord{Row: 0, Col: 0}, 0)
	trainingBlade := place(t, cat, "training_blade", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{jaggedBlade, trainingBlade})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "knight_s_razor" {
		t.Fatalf("recipe result=%q want knight_s_razor", crafts[0].RecipeResult)
	}
}

func TestEndurancePotionCraftValidWhenWaterPotionTouchesBanana(t *testing.T) {
	cat := loadTestCatalog(t)
	waterPotion := place(t, cat, "water_potion", 0, model.Coord{Row: 0, Col: 0}, 0)
	banana := place(t, cat, "banana", 1, model.Coord{Row: 0, Col: 0}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{waterPotion, banana})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "endurance_potion" {
		t.Fatalf("recipe result=%q want endurance_potion", crafts[0].RecipeResult)
	}
}

func TestPricklyPotionCraftValidWhenWaterPotionTouchesCactus(t *testing.T) {
	cat := loadTestCatalog(t)
	waterPotion := place(t, cat, "water_potion", 0, model.Coord{Row: 0, Col: 0}, 0)
	cactus := place(t, cat, "cactus", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{waterPotion, cactus})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "prickly_potion" {
		t.Fatalf("recipe result=%q want prickly_potion", crafts[0].RecipeResult)
	}
}

func TestAntivenomCraftValidWhenWaterPotionTouchesPoisonApple(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["poison_apple"] = model.Item{
		ID:    "poison_apple",
		Name:  "Poison Apple",
		Types: []string{"Food"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	waterPotion := place(t, cat, "water_potion", 0, model.Coord{Row: 0, Col: 0}, 0)
	poisonApple := place(t, cat, "poison_apple", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{waterPotion, poisonApple})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "antivenom" {
		t.Fatalf("recipe result=%q want antivenom", crafts[0].RecipeResult)
	}
}

func TestFurBootsCraftValidWhenLeatherBootsTouchesGoldenTail(t *testing.T) {
	cat := loadTestCatalog(t)
	leatherBoots := place(t, cat, "leather_boots", 0, model.Coord{Row: 0, Col: 0}, 0)
	goldenTail := place(t, cat, "golden_tail", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{leatherBoots, goldenTail})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "fur_boots" {
		t.Fatalf("recipe result=%q want fur_boots", crafts[0].RecipeResult)
	}
}

func TestMysteryStewCraftValidWhenCauldronTouchesBrownRat(t *testing.T) {
	cat := loadTestCatalog(t)
	cauldron := place(t, cat, "cauldron", 0, model.Coord{Row: 0, Col: 0}, 0)
	brownRat := place(t, cat, "brown_rat", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{cauldron, brownRat})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "mystery_stew" {
		t.Fatalf("recipe result=%q want mystery_stew", crafts[0].RecipeResult)
	}
}

func TestMalletCraftValidWhenWoodenClubsTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstClub := place(t, cat, "wooden_club", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondClub := place(t, cat, "wooden_club", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstClub, secondClub})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "mallet" {
		t.Fatalf("recipe result=%q want mallet", crafts[0].RecipeResult)
	}
}

func TestBrainsquasherCraftValidWhenMalletsTouch(t *testing.T) {
	cat := loadTestCatalog(t)
	firstMallet := place(t, cat, "mallet", 0, model.Coord{Row: 0, Col: 0}, 0)
	secondMallet := place(t, cat, "mallet", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{firstMallet, secondMallet})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "brainsquasher" {
		t.Fatalf("recipe result=%q want brainsquasher", crafts[0].RecipeResult)
	}
}

func TestEtherealCloakCraftValidWhenSpeedEssenceTouchesThreads(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["speed_essence"] = model.Item{
		ID:    "speed_essence",
		Name:  "Speed Essence",
		Types: []string{"Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	cat.Items["ball_of_thread"] = model.Item{
		ID:    "ball_of_thread",
		Name:  "Ball of Thread",
		Types: []string{"Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	speedEssence := place(t, cat, "speed_essence", 0, model.Coord{Row: 1, Col: 1}, 0)
	firstThread := place(t, cat, "ball_of_thread", 1, model.Coord{Row: 0, Col: 1}, 0)
	secondThread := place(t, cat, "ball_of_thread", 2, model.Coord{Row: 1, Col: 0}, 0)
	thirdThread := place(t, cat, "ball_of_thread", 3, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{speedEssence, firstThread, secondThread, thirdThread})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "ethereal_cloak" {
		t.Fatalf("recipe result=%q want ethereal_cloak", crafts[0].RecipeResult)
	}
}

func TestManaShieldCraftValidWhenWoodenBucklerTouchesManaCrystal(t *testing.T) {
	cat := loadTestCatalog(t)
	woodenBuckler := place(t, cat, "wooden_buckler", 0, model.Coord{Row: 0, Col: 0}, 0)
	manaCrystal := place(t, cat, "mana_crystal", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{woodenBuckler, manaCrystal})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "mana_shield" {
		t.Fatalf("recipe result=%q want mana_shield", crafts[0].RecipeResult)
	}
}

func TestWickedStickCraftValidWhenWoodenStickTouchesStarbloom(t *testing.T) {
	cat := loadTestCatalog(t)
	woodenStick := place(t, cat, "wooden_stick", 0, model.Coord{Row: 0, Col: 0}, 0)
	starbloom := place(t, cat, "starbloom", 1, model.Coord{Row: 0, Col: 1}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{woodenStick, starbloom})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "wicked_stick" {
		t.Fatalf("recipe result=%q want wicked_stick", crafts[0].RecipeResult)
	}
}

func TestHungryWandCraftValidWhenSearingWandTouchesWickedStick(t *testing.T) {
	cat := loadTestCatalog(t)
	searingWand := place(t, cat, "searing_wand", 0, model.Coord{Row: 1, Col: 1}, 0)
	wickedStick := place(t, cat, "wicked_stick", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{searingWand, wickedStick})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "hungry_wand" {
		t.Fatalf("recipe result=%q want hungry_wand", crafts[0].RecipeResult)
	}
}

func TestStarlightPotionCraftValidWhenManaPotionTouchesStarblooms(t *testing.T) {
	cat := loadTestCatalog(t)
	manaPotion := place(t, cat, "mana_potion", 0, model.Coord{Row: 1, Col: 1}, 0)
	firstStarbloom := place(t, cat, "starbloom", 1, model.Coord{Row: 1, Col: 0}, 0)
	secondStarbloom := place(t, cat, "starbloom", 2, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{manaPotion, firstStarbloom, secondStarbloom})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "starlight_potion" {
		t.Fatalf("recipe result=%q want starlight_potion", crafts[0].RecipeResult)
	}
}

func TestCosmosWardCraftValidWhenElementalBarrierTouchesStarlightPotion(t *testing.T) {
	cat := loadTestCatalog(t)
	elementalBarrier := place(t, cat, "elemental_barrier", 0, model.Coord{Row: 0, Col: 0}, 0)
	starlightPotion := place(t, cat, "starlight_potion", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{elementalBarrier, starlightPotion})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "cosmos_ward" {
		t.Fatalf("recipe result=%q want cosmos_ward", crafts[0].RecipeResult)
	}
}

func TestFrostBarrierCraftValidWhenReinforcedShieldTouchesFrostEmblems(t *testing.T) {
	cat := loadTestCatalog(t)
	reinforcedShield := place(t, cat, "reinforced_shield", 0, model.Coord{Row: 0, Col: 0}, 0)
	firstEmblem := place(t, cat, "frost_emblem", 1, model.Coord{Row: 0, Col: 2}, 0)
	secondEmblem := place(t, cat, "frost_emblem", 2, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{reinforcedShield, firstEmblem, secondEmblem})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "frost_barrier" {
		t.Fatalf("recipe result=%q want frost_barrier", crafts[0].RecipeResult)
	}
}

func TestMoldemortCraftValidWhenBlueCheeseTouchesUnholySkull(t *testing.T) {
	cat := loadTestCatalog(t)
	blueCheese := place(t, cat, "blue_chilly_cheese", 0, model.Coord{Row: 1, Col: 1}, 0)
	unholySkull := place(t, cat, "unholy_skull", 1, model.Coord{Row: 1, Col: 3}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{blueCheese, unholySkull})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "moldemort" {
		t.Fatalf("recipe result=%q want moldemort", crafts[0].RecipeResult)
	}
}

func TestUnholySkullCraftValidWhenWeatheredSkullTouchesDeathEssence(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["weathered_skull"] = model.Item{
		ID:    "weathered_skull",
		Name:  "Weathered Skull",
		Types: []string{"Skull", "Accessory"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	weatheredSkull := place(t, cat, "weathered_skull", 0, model.Coord{Row: 1, Col: 1}, 0)
	deathEssence := place(t, cat, "death_essence", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{weatheredSkull, deathEssence})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "unholy_skull" {
		t.Fatalf("recipe result=%q want unholy_skull", crafts[0].RecipeResult)
	}
}

func TestBlueChillyCheeseCraftValidWhenSucculentCheeseTouchesFrostEmblem(t *testing.T) {
	cat := loadTestCatalog(t)
	succulentCheese := place(t, cat, "succulent_cheese", 0, model.Coord{Row: 1, Col: 1}, 0)
	frostEmblem := place(t, cat, "frost_emblem", 1, model.Coord{Row: 1, Col: 3}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{succulentCheese, frostEmblem})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "blue_chilly_cheese" {
		t.Fatalf("recipe result=%q want blue_chilly_cheese", crafts[0].RecipeResult)
	}
}

func TestSucculentCheeseCraftValidWhenCheeseTouchesSucculent(t *testing.T) {
	cat := loadTestCatalog(t)
	cat.Items["cheese"] = model.Item{
		ID:    "cheese",
		Name:  "Cheese",
		Types: []string{"Ingredient", "Food"},
		Shape: []model.Coord{{Row: 0, Col: 0}},
	}
	cheese := place(t, cat, "cheese", 0, model.Coord{Row: 1, Col: 1}, 0)
	succulent := place(t, cat, "succulent", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{cheese, succulent})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "succulent_cheese" {
		t.Fatalf("recipe result=%q want succulent_cheese", crafts[0].RecipeResult)
	}
}

func TestWildMagicScrollCraftValidWhenArcaneTouchesPhoenixAndFrostScrolls(t *testing.T) {
	cat := loadTestCatalog(t)
	arcaneScroll := place(t, cat, "arcane_scroll", 0, model.Coord{Row: 1, Col: 1}, 0)
	phoenixScroll := place(t, cat, "phoenix_scroll", 1, model.Coord{Row: 1, Col: 0}, 0)
	frostScroll := place(t, cat, "frost_scroll", 2, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{arcaneScroll, phoenixScroll, frostScroll})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "wild_magic_scroll" {
		t.Fatalf("recipe result=%q want wild_magic_scroll", crafts[0].RecipeResult)
	}
}

func TestArcaneScrollCraftValidWhenSpellScrollTouchesMagicEssence(t *testing.T) {
	cat := loadTestCatalog(t)
	spellScroll := place(t, cat, "spell_scroll", 0, model.Coord{Row: 1, Col: 1}, 0)
	magicEssence := place(t, cat, "magic_essence", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{spellScroll, magicEssence})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "arcane_scroll" {
		t.Fatalf("recipe result=%q want arcane_scroll", crafts[0].RecipeResult)
	}
}

func TestFrostScrollCraftValidWhenSpellScrollTouchesFrostEmblem(t *testing.T) {
	cat := loadTestCatalog(t)
	spellScroll := place(t, cat, "spell_scroll", 0, model.Coord{Row: 1, Col: 1}, 0)
	frostEmblem := place(t, cat, "frost_emblem", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{spellScroll, frostEmblem})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "frost_scroll" {
		t.Fatalf("recipe result=%q want frost_scroll", crafts[0].RecipeResult)
	}
}

func TestFrostDaggerCraftValidWhenDaggerTouchesFrostEmblem(t *testing.T) {
	cat := loadTestCatalog(t)
	dagger := place(t, cat, "dagger", 0, model.Coord{Row: 1, Col: 1}, 0)
	frostEmblem := place(t, cat, "frost_emblem", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{dagger, frostEmblem})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "frost_dagger" {
		t.Fatalf("recipe result=%q want frost_dagger", crafts[0].RecipeResult)
	}
}

func TestFlameCloakCraftValidWhenWizardRobeTouchesPhoenixFeather(t *testing.T) {
	cat := loadTestCatalog(t)
	wizardRobe := place(t, cat, "wizard_robe", 0, model.Coord{Row: 0, Col: 0}, 0)
	phoenixFeather := place(t, cat, "phoenix_feather", 1, model.Coord{Row: 0, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{wizardRobe, phoenixFeather})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "flame_cloak" {
		t.Fatalf("recipe result=%q want flame_cloak", crafts[0].RecipeResult)
	}
}

func TestPhoenixScrollCraftValidWhenSpellScrollTouchesPhoenixFeather(t *testing.T) {
	cat := loadTestCatalog(t)
	spellScroll := place(t, cat, "spell_scroll", 0, model.Coord{Row: 1, Col: 1}, 0)
	phoenixFeather := place(t, cat, "phoenix_feather", 1, model.Coord{Row: 1, Col: 2}, 0)

	crafts := scoring.EvaluateCrafts(cat, []model.Placement{spellScroll, phoenixFeather})

	if len(crafts) != 1 {
		t.Fatalf("len(crafts)=%d want 1: %+v", len(crafts), crafts)
	}
	if crafts[0].RecipeResult != "phoenix_scroll" {
		t.Fatalf("recipe result=%q want phoenix_scroll", crafts[0].RecipeResult)
	}
}

func TestPriorityCountsStarSource(t *testing.T) {
	cat := loadTestCatalog(t)
	scalemail := place(t, cat, "scalemail", 0, model.Coord{Row: 1, Col: 1}, 0)
	cactus := place(t, cat, "cactus", 1, model.Coord{Row: 1, Col: 0}, 0)

	evaluation := scoring.EvaluateLayoutWithPriorities(cat, []model.Placement{scalemail, cactus}, []string{"star_source:scalemail"})

	if len(evaluation.Score.PriorityCounts) != 1 || evaluation.Score.PriorityCounts[0] != 1 {
		t.Fatalf("priority counts=%v want [1]", evaluation.Score.PriorityCounts)
	}
}

func TestPriorityCountsCraftResult(t *testing.T) {
	cat := loadTestCatalog(t)
	scalemail := place(t, cat, "scalemail", 0, model.Coord{Row: 1, Col: 1}, 0)
	thornwall := place(t, cat, "thornwall", 1, model.Coord{Row: 1, Col: 3}, 0)
	armorKit := place(t, cat, "armor_kit", 2, model.Coord{Row: 4, Col: 1}, 0)

	evaluation := scoring.EvaluateLayoutWithPriorities(
		cat,
		[]model.Placement{scalemail, thornwall, armorKit},
		[]string{"craft:spinegrowth_breastplate"},
	)

	if len(evaluation.Score.PriorityCounts) != 1 || evaluation.Score.PriorityCounts[0] != 1 {
		t.Fatalf("priority counts=%v want [1]", evaluation.Score.PriorityCounts)
	}
}

func TestStarCoverageBucketsFullSetBeforePartial(t *testing.T) {
	cat := loadTestCatalog(t)
	placements := []model.Placement{
		minimalPlacement("rune_of_r_lyeh#0", "rune_of_r_lyeh", 0),
		minimalPlacement("venomous_pincer#1", "venomous_pincer", 1),
		minimalPlacement("shaman_s_talisman#2", "shaman_s_talisman", 2),
		minimalPlacement("royal_seax#3", "royal_seax", 3),
		minimalPlacement("ragnarok#4", "ragnarok", 4),
		minimalPlacement("doombringer#5", "doombringer", 5),
		minimalPlacement("royal_seax#6", "royal_seax", 6),
	}
	stars := []model.StarActivation{
		{SourceInstance: "rune_of_r_lyeh#0", TargetInstance: "royal_seax#3"},
		{SourceInstance: "venomous_pincer#1", TargetInstance: "royal_seax#3"},
		{SourceInstance: "shaman_s_talisman#2", TargetInstance: "royal_seax#3"},
		{SourceInstance: "rune_of_r_lyeh#0", TargetInstance: "ragnarok#4"},
		{SourceInstance: "venomous_pincer#1", TargetInstance: "ragnarok#4"},
		{SourceInstance: "rune_of_r_lyeh#0", TargetInstance: "doombringer#5"},
		{SourceInstance: "venomous_pincer#1", TargetInstance: "doombringer#5"},
	}

	counts, coverage := scoring.EvaluatePriorityScore(
		cat,
		placements,
		nil,
		stars,
		[]string{"star_source:rune_of_r_lyeh", "star_source:venomous_pincer", "star_source:shaman_s_talisman"},
	)

	assertIntSlice(t, counts, []int{1, 2, 0})
	if coverage == nil {
		t.Fatal("coverage is nil")
	}
	if len(coverage.Targets) != 4 {
		t.Fatalf("len(targets)=%d want 4: %+v", len(coverage.Targets), coverage.Targets)
	}
	if coverage.Targets[3].CoveredCount != 0 {
		t.Fatalf("zero coverage target=%+v", coverage.Targets[3])
	}
}

func TestPriorityCountsCraftPositionAroundCoverageBlock(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("rune_of_r_lyeh#0", "rune_of_r_lyeh", 0),
		minimalPlacement("venomous_pincer#1", "venomous_pincer", 1),
		minimalPlacement("doombringer#2", "doombringer", 2),
	}
	crafts := []model.CraftActivation{
		{RecipeResult: "ragnarok"},
		{RecipeResult: "royal_seax"},
	}
	stars := []model.StarActivation{
		{SourceInstance: "rune_of_r_lyeh#0", TargetInstance: "doombringer#2"},
		{SourceInstance: "venomous_pincer#1", TargetInstance: "doombringer#2"},
	}

	counts, _ := scoring.EvaluatePriorityScore(
		model.Catalog{},
		placements,
		crafts,
		stars,
		[]string{"craft:ragnarok", "star_source:rune_of_r_lyeh", "craft:royal_seax", "star_source:venomous_pincer"},
	)

	assertIntSlice(t, counts, []int{1, 1, 0, 1})
}

func TestStarCoverageDedupesSameSourceItemID(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("rune_of_r_lyeh#0", "rune_of_r_lyeh", 0),
		minimalPlacement("rune_of_r_lyeh#1", "rune_of_r_lyeh", 1),
		minimalPlacement("royal_seax#2", "royal_seax", 2),
	}
	stars := []model.StarActivation{
		{SourceInstance: "rune_of_r_lyeh#0", TargetInstance: "royal_seax#2"},
		{SourceInstance: "rune_of_r_lyeh#1", TargetInstance: "royal_seax#2"},
	}

	counts, _ := scoring.EvaluatePriorityScore(
		model.Catalog{},
		placements,
		nil,
		stars,
		[]string{"star_source:rune_of_r_lyeh"},
	)

	assertIntSlice(t, counts, []int{1})
}

func TestGroupedStarCoverageScoresGroupsSeparately(t *testing.T) {
	cat := loadTestCatalog(t)
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("piercing_lance#1", "piercing_lance", 1),
		minimalPlacement("fine_sword#2", "fine_sword", 2),
		minimalPlacement("spice#3", "spice", 3),
		minimalPlacement("apple#4", "apple", 4),
	}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "fine_sword#2"},
		{SourceInstance: "piercing_lance#1", TargetInstance: "fine_sword#2"},
		{SourceInstance: "spice#3", TargetInstance: "apple#4"},
	}

	counts, _, groups, loose := scoring.EvaluatePriorityScoreWithCoverageGroups(
		cat,
		placements,
		nil,
		stars,
		nil,
		[]model.CoverageGroup{
			{Name: "Weapons", Sources: []string{"power_stone", "piercing_lance"}},
			{Name: "Food", Sources: []string{"spice"}},
		},
	)

	assertIntSlice(t, counts, []int{1, 0, 1})
	if len(loose) != 0 {
		t.Fatalf("loose priorities=%+v want none", loose)
	}
	if len(groups) != 2 {
		t.Fatalf("len(groups)=%d want 2", len(groups))
	}
	if groups[0].Name != "Weapons" || groups[1].Name != "Food" {
		t.Fatalf("unexpected group names: %+v", groups)
	}
	if containsCoverageTarget(groups[0], "apple") {
		t.Fatalf("weapon coverage should not consider apple target: %+v", groups[0].Targets)
	}
	if containsCoverageTarget(groups[1], "fine_sword") {
		t.Fatalf("food coverage should not consider fine_sword target: %+v", groups[1].Targets)
	}
}

func TestGlobalPriorityOrderInterleavesLooseGroupsAndCrafts(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("piercing_lance#1", "piercing_lance", 1),
		minimalPlacement("mana_crystal#2", "mana_crystal", 2),
		minimalPlacement("fine_sword#3", "fine_sword", 3),
		minimalPlacement("starbloom#4", "starbloom", 4),
		minimalPlacement("starlight_potion#5", "starlight_potion", 5),
	}
	crafts := []model.CraftActivation{{RecipeResult: "crafted"}}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "fine_sword#3"},
		{SourceInstance: "piercing_lance#1", TargetInstance: "fine_sword#3"},
		{SourceInstance: "mana_crystal#2", TargetInstance: "starbloom#4"},
		{SourceInstance: "mana_crystal#2", TargetInstance: "starlight_potion#5"},
	}

	counts, _, _, loose := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		crafts,
		stars,
		[]string{"star_source:mana_crystal", "coverage_group:0", "craft:crafted"},
		[]model.CoverageGroup{{
			Name:    "Weapons",
			Sources: []string{"power_stone", "piercing_lance"},
			Targets: []string{"fine_sword"},
		}},
	)

	assertIntSlice(t, counts, []int{2, 1, 0, 1})
	if len(loose) != 1 || loose[0].SourceItemID != "mana_crystal" || loose[0].TargetCount != 2 {
		t.Fatalf("loose priorities=%+v want mana_crystal=2", loose)
	}
}

func TestGlobalPriorityOrderCanInterleaveCoverageGroups(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("piercing_lance#1", "piercing_lance", 1),
		minimalPlacement("mana_crystal#2", "mana_crystal", 2),
		minimalPlacement("fine_sword#3", "fine_sword", 3),
		minimalPlacement("starbloom#4", "starbloom", 4),
	}
	crafts := []model.CraftActivation{{RecipeResult: "crafted"}}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "fine_sword#3"},
		{SourceInstance: "piercing_lance#1", TargetInstance: "fine_sword#3"},
		{SourceInstance: "mana_crystal#2", TargetInstance: "starbloom#4"},
	}
	groups := []model.CoverageGroup{
		{Name: "Weapons", Sources: []string{"power_stone", "piercing_lance"}, Targets: []string{"fine_sword"}},
		{Name: "Bloom", Sources: []string{"mana_crystal"}, Targets: []string{"starbloom"}},
	}

	counts, _, _, loose := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		crafts,
		stars,
		[]string{"coverage_group:1", "craft:crafted", "coverage_group:0", "star_source:mana_crystal"},
		groups,
	)

	assertIntSlice(t, counts, []int{1, 1, 1, 0, 0})
	if len(loose) != 0 {
		t.Fatalf("loose priorities=%+v want none because mana_crystal is grouped", loose)
	}
}

func TestGroupedStarCoverageExplicitTargetsFilterTargets(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("piercing_lance#1", "piercing_lance", 1),
		minimalPlacement("gloves_of_power#2", "gloves_of_power", 2),
		minimalPlacement("excalibur#3", "excalibur", 3),
		minimalPlacement("fine_sword#4", "fine_sword", 4),
	}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "excalibur#3"},
		{SourceInstance: "piercing_lance#1", TargetInstance: "excalibur#3"},
		{SourceInstance: "gloves_of_power#2", TargetInstance: "excalibur#3"},
		{SourceInstance: "power_stone#0", TargetInstance: "fine_sword#4"},
		{SourceInstance: "piercing_lance#1", TargetInstance: "fine_sword#4"},
		{SourceInstance: "gloves_of_power#2", TargetInstance: "fine_sword#4"},
		{SourceInstance: "power_stone#0", TargetInstance: "piercing_lance#1"},
	}

	counts, _, groups, _ := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		nil,
		stars,
		nil,
		[]model.CoverageGroup{{
			Name:    "Weapons",
			Sources: []string{"piercing_lance", "power_stone", "gloves_of_power"},
			Targets: []string{"excalibur", "fine_sword"},
		}},
	)

	assertIntSlice(t, counts, []int{2, 0, 0})
	if len(groups) != 1 {
		t.Fatalf("len(groups)=%d want 1", len(groups))
	}
	assertStringSlice(t, groups[0].TargetItemIDs, []string{"excalibur", "fine_sword"})
	if containsCoverageTarget(groups[0], "piercing_lance") {
		t.Fatalf("explicit target filter should not include piercing_lance: %+v", groups[0].Targets)
	}
}

func TestGroupedStarCoverageExplicitTargetsKeepZeroCoverageTargets(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("excalibur#1", "excalibur", 1),
		minimalPlacement("fine_sword#2", "fine_sword", 2),
	}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "excalibur#1"},
	}

	counts, _, groups, _ := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		nil,
		stars,
		nil,
		[]model.CoverageGroup{{
			Name:    "Weapons",
			Sources: []string{"power_stone"},
			Targets: []string{"excalibur", "fine_sword"},
		}},
	)

	assertIntSlice(t, counts, []int{1})
	if len(groups) != 1 || len(groups[0].Targets) != 2 {
		t.Fatalf("coverage targets=%+v want excalibur and fine_sword", groups)
	}
	if groups[0].Targets[1].TargetItemID != "fine_sword" || groups[0].Targets[1].CoveredCount != 0 {
		t.Fatalf("zero coverage explicit target=%+v", groups[0].Targets[1])
	}
}

func TestGroupedStarCoverageIgnoresSelfSourceActivation(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("piercing_lance#0", "piercing_lance", 0),
		minimalPlacement("power_stone#1", "power_stone", 1),
	}
	stars := []model.StarActivation{
		{SourceInstance: "piercing_lance#0", TargetInstance: "piercing_lance#0"},
		{SourceInstance: "power_stone#1", TargetInstance: "piercing_lance#0"},
	}

	counts, _, groups, _ := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		nil,
		stars,
		nil,
		[]model.CoverageGroup{{
			Name:    "Weapons",
			Sources: []string{"piercing_lance", "power_stone"},
			Targets: []string{"piercing_lance"},
		}},
	)

	assertIntSlice(t, counts, []int{0, 1})
	if got := groups[0].Targets[0].CoveredSources; len(got) != 1 || got[0] != "power_stone" {
		t.Fatalf("covered sources=%+v want only power_stone", got)
	}
}

func TestGroupedStarCoveragePriorityOrder(t *testing.T) {
	weaponFirst := model.Solution{Evaluation: model.Evaluation{Score: model.Score{PriorityCounts: []int{1, 0, 0}}}}
	foodOnly := model.Solution{Evaluation: model.Evaluation{Score: model.Score{PriorityCounts: []int{0, 0, 3}}}}

	if !solver.SolutionLess(weaponFirst, foodOnly) {
		t.Fatalf("first coverage group should win: %v vs %v", weaponFirst.Evaluation.Score.PriorityCounts, foodOnly.Evaluation.Score.PriorityCounts)
	}
}

func TestLooseStarPrioritiesAfterCoverageGroups(t *testing.T) {
	cat := loadTestCatalog(t)
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("piercing_lance#1", "piercing_lance", 1),
		minimalPlacement("fine_sword#2", "fine_sword", 2),
		minimalPlacement("spinegrowth_breastplate#3", "spinegrowth_breastplate", 3),
		minimalPlacement("apple#4", "apple", 4),
		minimalPlacement("banana#5", "banana", 5),
	}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "fine_sword#2"},
		{SourceInstance: "piercing_lance#1", TargetInstance: "fine_sword#2"},
		{SourceInstance: "spinegrowth_breastplate#3", TargetInstance: "apple#4"},
		{SourceInstance: "spinegrowth_breastplate#3", TargetInstance: "banana#5"},
	}

	counts, _, groups, loose := scoring.EvaluatePriorityScoreWithCoverageGroups(
		cat,
		placements,
		nil,
		stars,
		[]string{"star_source:spinegrowth_breastplate"},
		[]model.CoverageGroup{{Name: "Weapons", Sources: []string{"power_stone", "piercing_lance"}}},
	)

	assertIntSlice(t, counts, []int{1, 0, 2})
	if len(groups) != 1 {
		t.Fatalf("len(groups)=%d want 1", len(groups))
	}
	if len(loose) != 1 || loose[0].SourceItemID != "spinegrowth_breastplate" || loose[0].TargetCount != 2 {
		t.Fatalf("loose=%+v want spinegrowth_breastplate=2", loose)
	}
}

func TestLooseStarPriorityDedupesTargets(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("spinegrowth_breastplate#0", "spinegrowth_breastplate", 0),
		minimalPlacement("apple#1", "apple", 1),
	}
	stars := []model.StarActivation{
		{SourceInstance: "spinegrowth_breastplate#0", TargetInstance: "apple#1"},
		{SourceInstance: "spinegrowth_breastplate#0", TargetInstance: "apple#1"},
	}

	counts, _, _, loose := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		nil,
		stars,
		[]string{"star_source:spinegrowth_breastplate"},
		[]model.CoverageGroup{{Name: "Other", Sources: []string{"power_stone"}}},
	)

	assertIntSlice(t, counts, []int{0, 1})
	if len(loose) != 1 || loose[0].TargetCount != 1 {
		t.Fatalf("loose=%+v want one deduped target", loose)
	}
}

func TestGroupedSourceIsNotAlsoLooseStarPriority(t *testing.T) {
	placements := []model.Placement{
		minimalPlacement("power_stone#0", "power_stone", 0),
		minimalPlacement("fine_sword#1", "fine_sword", 1),
	}
	stars := []model.StarActivation{
		{SourceInstance: "power_stone#0", TargetInstance: "fine_sword#1"},
	}

	counts, _, _, loose := scoring.EvaluatePriorityScoreWithCoverageGroups(
		model.Catalog{},
		placements,
		nil,
		stars,
		[]string{"star_source:power_stone"},
		[]model.CoverageGroup{{Name: "Weapons", Sources: []string{"power_stone"}}},
	)

	assertIntSlice(t, counts, []int{1})
	if len(loose) != 0 {
		t.Fatalf("grouped star source should not also be loose: %+v", loose)
	}
}

func containsCoverageTarget(group model.StarCoverageBreakdown, itemID string) bool {
	for _, target := range group.Targets {
		if target.TargetItemID == itemID {
			return true
		}
	}
	return false
}

func minimalPlacement(instanceID string, itemID string, originalIndex int) model.Placement {
	return model.Placement{
		InstanceID:    instanceID,
		ItemID:        itemID,
		OriginalIndex: originalIndex,
	}
}

func assertIntSlice(t *testing.T, got []int, want []int) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func assertStringSlice(t *testing.T, got []string, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func countStarsFromSource(stars []model.StarActivation, sourceInstance string) int {
	count := 0
	for _, star := range stars {
		if star.SourceInstance == sourceInstance {
			count++
		}
	}
	return count
}
