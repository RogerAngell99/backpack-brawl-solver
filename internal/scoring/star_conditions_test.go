package scoring_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

type starConditionFixture struct {
	Name      string                 `json:"name"`
	Condition model.StarCondition    `json:"condition"`
	Context   starConditionTestInput `json:"context"`
	Want      string                 `json:"want"`
}

type starConditionTestInput struct {
	SourceID        string   `json:"source_id"`
	TargetID        string   `json:"target_id"`
	TargetTypes     []string `json:"target_types"`
	TargetStatTypes []string `json:"target_stat_types"`
	TargetBagEmpty  *bool    `json:"target_bag_empty"`
	TargetActivated *bool    `json:"target_activated"`
}

func TestStarConditionFixtures(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "star_conditions.json"))
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var fixtures []starConditionFixture
	if err := json.Unmarshal(content, &fixtures); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			target := model.Item{ID: fixture.Context.TargetID, Types: fixture.Context.TargetTypes, StatTypes: fixture.Context.TargetStatTypes}
			context := scoring.StarConditionContext{
				SourceID:        fixture.Context.SourceID,
				TargetID:        fixture.Context.TargetID,
				Target:          &target,
				TargetBagEmpty:  fixture.Context.TargetBagEmpty,
				TargetActivated: fixture.Context.TargetActivated,
			}
			got := scoring.EvaluateStarCondition(&fixture.Condition, context)
			want := map[string]scoring.ConditionState{
				"false":   scoring.ConditionFalse,
				"true":    scoring.ConditionTrue,
				"unknown": scoring.ConditionUnknown,
			}[fixture.Want]
			if got != want {
				t.Fatalf("state=%v want %s", got, fixture.Want)
			}
		})
	}
}

func TestEvaluateWatermelonConditionRequiresDifferentFood(t *testing.T) {
	condition := model.StarCondition{
		Class: "Model.CompoundStarCondition",
		Any:   false,
		Conditions: []model.StarCondition{
			{Class: "Model.OtherItemIsOfType", ItemType: "Food"},
			{Class: "Model.DefinitionIsDifferent"},
		},
	}
	food := model.Item{ID: "apple", Types: []string{"Food"}}
	weapon := model.Item{ID: "dagger", Types: []string{"Melee Weapon"}}

	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{
		SourceID: "watermelon",
		TargetID: "apple",
		Target:   &food,
	}); got != scoring.ConditionTrue {
		t.Fatalf("different food state=%v want true", got)
	}
	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{
		SourceID: "watermelon",
		TargetID: "watermelon",
		Target:   &food,
	}); got != scoring.ConditionFalse {
		t.Fatalf("same definition state=%v want false", got)
	}
	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{
		SourceID: "watermelon",
		TargetID: "dagger",
		Target:   &weapon,
	}); got != scoring.ConditionFalse {
		t.Fatalf("non-food state=%v want false", got)
	}
}

func TestEvaluateItemStatCondition(t *testing.T) {
	condition := model.StarCondition{
		Class:    "Model.OtherItemHasStatOfType",
		StatType: "Model.ItemStats.CactusCount, Assembly-CSharp, Version=0.0.0.0",
	}
	withStat := model.Item{ID: "cactus", StatTypes: []string{"CactusCount"}}
	withoutStat := model.Item{ID: "pitahaya", StatTypes: []string{"Damage"}}

	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{Target: &withStat}); got != scoring.ConditionTrue {
		t.Fatalf("matching stat state=%v want true", got)
	}
	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{Target: &withoutStat}); got != scoring.ConditionFalse {
		t.Fatalf("missing stat state=%v want false", got)
	}
}

func TestEvaluateTargetDependentConditionsNeedTarget(t *testing.T) {
	emptyBag := model.StarCondition{Class: "Model.OtherIsEmptyBag"}
	activated := model.StarCondition{Class: "Model.OtherItemHasItemActivatedSignal"}

	if got := scoring.EvaluateStarCondition(&emptyBag, scoring.StarConditionContext{}); got != scoring.ConditionUnknown {
		t.Fatalf("empty bag without context=%v want unknown", got)
	}
	if got := scoring.EvaluateStarCondition(&activated, scoring.StarConditionContext{}); got != scoring.ConditionUnknown {
		t.Fatalf("activation without context=%v want unknown", got)
	}
	target := model.Item{ID: "gold_bar"}
	active := false
	if got := scoring.EvaluateStarCondition(&activated, scoring.StarConditionContext{Target: &target, TargetActivated: &active}); got != scoring.ConditionTrue {
		t.Fatalf("placed target state=%v want true", got)
	}
}

func TestEvaluateStaticConditionsUseUnknownWithoutTarget(t *testing.T) {
	typeCondition := model.StarCondition{Class: "Model.OtherItemIsOfType", ItemType: "Food"}
	if got := scoring.EvaluateStarCondition(&typeCondition, scoring.StarConditionContext{}); got != scoring.ConditionUnknown {
		t.Fatalf("type without target=%v want unknown", got)
	}

	exactCondition := model.StarCondition{
		Class:      "Model.OtherItemIsExactly",
		Definition: &model.ItemDefinition{ID: "Rock"},
	}
	if got := scoring.EvaluateStarCondition(&exactCondition, scoring.StarConditionContext{Target: &model.Item{}}); got != scoring.ConditionUnknown {
		t.Fatalf("exact without target definition=%v want unknown", got)
	}
}

func TestEvaluateCatalogStarConditionPreservesUnknown(t *testing.T) {
	source := model.Item{ID: "source", Stars: []model.Star{{RuleStatus: "unknown"}}}
	catalogData := model.Catalog{Items: map[string]model.Item{
		"source": source,
		"target": {ID: "target", Types: []string{"Food"}},
	}}
	state := scoring.EvaluateCatalogStarCondition(catalogData, "source", "target", &source.Stars[0])
	if state != scoring.ConditionUnknown {
		t.Fatalf("unknown star state=%v want unknown", state)
	}
}

func TestEvaluateCatalogMissingItemPreservesUnknown(t *testing.T) {
	source := model.Item{
		ID:            "source",
		Stars:         []model.Star{{}},
		StarCondition: &model.StarCondition{Class: "Model.OtherItemIsOfType", ItemType: "Food"},
	}
	catalogData := model.Catalog{Items: map[string]model.Item{"source": source}}
	state := scoring.EvaluateCatalogStarCondition(catalogData, "source", "missing", &source.Stars[0])
	if state != scoring.ConditionUnknown {
		t.Fatalf("missing target state=%v want unknown", state)
	}
}

func TestEvaluateCompoundUsesThreeValuedLogic(t *testing.T) {
	condition := model.StarCondition{
		Class: "Model.CompoundStarCondition",
		Any:   false,
		Conditions: []model.StarCondition{
			{Class: "Model.DefinitionIsDifferent"},
			{Class: "Model.OtherItemHasItemActivatedSignal"},
		},
	}

	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{
		SourceID: "source",
		TargetID: "target",
	}); got != scoring.ConditionUnknown {
		t.Fatalf("compound unknown state=%v want unknown", got)
	}
	active := true
	target := model.Item{ID: "target"}
	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{
		SourceID:        "source",
		TargetID:        "target",
		Target:          &target,
		TargetActivated: &active,
	}); got != scoring.ConditionTrue {
		t.Fatalf("compound true state=%v want true", got)
	}
}

func TestEvaluateCompoundTruthTables(t *testing.T) {
	states := []struct {
		name      string
		condition model.StarCondition
	}{
		{name: "true", condition: model.StarCondition{Class: "Model.DefinitionIsSame"}},
		{name: "false", condition: model.StarCondition{Class: "Model.DefinitionIsDifferent"}},
		{name: "unknown", condition: model.StarCondition{Class: "Model.UnsupportedCondition"}},
	}
	context := scoring.StarConditionContext{SourceID: "same", TargetID: "same"}
	lookup := map[string]scoring.ConditionState{
		"false":   scoring.ConditionFalse,
		"true":    scoring.ConditionTrue,
		"unknown": scoring.ConditionUnknown,
	}

	for _, any := range []bool{true, false} {
		for _, left := range states {
			for _, right := range states {
				name := map[bool]string{true: "any", false: "all"}[any] + "/" + left.name + "+" + right.name
				t.Run(name, func(t *testing.T) {
					condition := model.StarCondition{
						Class: "Model.CompoundStarCondition",
						Any:   any,
						Conditions: []model.StarCondition{
							left.condition,
							right.condition,
						},
					}
					got := scoring.EvaluateStarCondition(&condition, context)
					wantName := "unknown"
					leftState := lookup[left.name]
					rightState := lookup[right.name]
					if any {
						switch {
						case leftState == scoring.ConditionTrue || rightState == scoring.ConditionTrue:
							wantName = "true"
						case leftState == scoring.ConditionFalse && rightState == scoring.ConditionFalse:
							wantName = "false"
						}
					} else {
						switch {
						case leftState == scoring.ConditionFalse || rightState == scoring.ConditionFalse:
							wantName = "false"
						case leftState == scoring.ConditionTrue && rightState == scoring.ConditionTrue:
							wantName = "true"
						}
					}
					if got != lookup[wantName] {
						t.Fatalf("state=%v want %s", got, wantName)
					}
				})
			}
		}
	}
}

func TestEvaluateNestedCompoundUsesThreeValuedLogic(t *testing.T) {
	condition := model.StarCondition{
		Class: "Model.CompoundStarCondition",
		Any:   true,
		Conditions: []model.StarCondition{
			{
				Class: "Model.CompoundStarCondition",
				Any:   false,
				Conditions: []model.StarCondition{
					{Class: "Model.DefinitionIsSame"},
					{Class: "Model.UnsupportedCondition"},
				},
			},
			{Class: "Model.DefinitionIsDifferent"},
		},
	}
	if got := scoring.EvaluateStarCondition(&condition, scoring.StarConditionContext{SourceID: "same", TargetID: "same"}); got != scoring.ConditionUnknown {
		t.Fatalf("nested compound state=%v want unknown", got)
	}
}

func TestProductionCatalogLoadsRuntimeGraphs(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	watermelon := loaded.Items["watermelon"]
	if watermelon.StarCondition == nil {
		t.Fatal("watermelon runtime graph missing")
	}
	if len(watermelon.Stars) == 0 {
		t.Fatal("watermelon stars missing")
	}
	if !scoring.StarMatchesCatalogItems(loaded, "watermelon", "apple", &watermelon.Stars[0]) {
		t.Fatal("watermelon should match a different Food target")
	}
	if scoring.StarMatchesCatalogItems(loaded, "watermelon", "watermelon", &watermelon.Stars[0]) {
		t.Fatal("watermelon should reject the same definition")
	}
}

func TestEvaluateStarsUsesRuntimeGraph(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	watermelon := loaded.Items["watermelon"]
	activations := scoring.EvaluateStars(loaded, []model.Placement{
		{
			InstanceID: "watermelon#0",
			ItemID:     "watermelon",
			Cells:      []model.Coord{{Row: 0, Col: 0}},
			StarPositions: []model.StarPosition{{
				Position: model.Coord{Row: 0, Col: 1},
				Star:     watermelon.Stars[0],
			}},
		},
		{
			InstanceID: "apple#0",
			ItemID:     "apple",
			Cells:      []model.Coord{{Row: 0, Col: 1}},
		},
	})
	if len(activations) != 1 {
		t.Fatalf("runtime graph activations=%d want 1: %+v", len(activations), activations)
	}
}

func TestProductionCatalogEvaluatesErrantLanceStat(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	errantLance := loaded.Items["errant_lance"]
	if errantLance.StarCondition == nil || len(errantLance.Stars) == 0 {
		t.Fatal("errant lance runtime condition missing")
	}
	for _, test := range []struct {
		target string
		want   bool
	}{
		{target: "cactus", want: true},
		{target: "cactrio", want: true},
		{target: "pitahaya", want: false},
	} {
		if got := scoring.StarMatchesCatalogItems(loaded, "errant_lance", test.target, &errantLance.Stars[0]); got != test.want {
			t.Errorf("errant lance target %s=%v want %v", test.target, got, test.want)
		}
	}
}

func TestProductionCatalogEvaluatesCurrentInventoryStructuralConditions(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	tests := []struct {
		name   string
		source string
		target string
		want   bool
	}{
		{name: "carp food target", source: "carp", target: "tender_sausage", want: true},
		{name: "carp same definition", source: "carp", target: "carp", want: false},
		{name: "pet collar pet target", source: "pet_collar", target: "stinkbug", want: true},
		{name: "pet collar non-pet target", source: "pet_collar", target: "bronze_bar", want: false},
		{name: "weapons rack melee target", source: "weapons_rack", target: "training_blade", want: true},
		{name: "weapons rack non-weapon target", source: "weapons_rack", target: "bronze_bar", want: false},
		{name: "venomous pincer pet target", source: "venomous_pincer", target: "stinkbug", want: true},
		{name: "traveler hat same target", source: "traveler_s_hat", target: "traveler_s_hat", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := loaded.Items[test.source]
			if len(source.Stars) == 0 {
				t.Fatalf("source %s has no stars", test.source)
			}
			if got := scoring.StarMatchesCatalogItems(loaded, test.source, test.target, &source.Stars[0]); got != test.want {
				t.Fatalf("match=%v want %v", got, test.want)
			}
		})
	}
}

func TestProductionMagicEssenceTargetsAnyPlacedItem(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	magicEssence := loaded.Items["magic_essence"]
	if magicEssence.StarCondition == nil || len(magicEssence.Stars) == 0 {
		t.Fatal("magic essence runtime condition missing")
	}
	for _, targetID := range []string{"gold_bar", "medium_bag"} {
		if !scoring.StarMatchesCatalogItems(loaded, "magic_essence", targetID, &magicEssence.Stars[0]) {
			t.Fatalf("magic essence should target placed %s", targetID)
		}
	}
}

func TestProductionCatalogEvaluatesExactDefinition(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	steelForgeHammer := loaded.Items["steel_forge_hammer"]
	if steelForgeHammer.StarCondition == nil || len(steelForgeHammer.Stars) == 0 {
		t.Fatal("steel forge hammer exact condition missing")
	}
	for _, test := range []struct {
		name   string
		target string
		want   bool
	}{
		{name: "exact rock", target: "rock", want: true},
		{name: "melee weapon alternative", target: "training_blade", want: true},
		{name: "melee mining pick alternative", target: "mining_pick", want: true},
		{name: "same broad type but different definition", target: "lava_rock", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := scoring.StarMatchesCatalogItems(loaded, "steel_forge_hammer", test.target, &steelForgeHammer.Stars[0]); got != test.want {
				t.Fatalf("target %s=%v want %v", test.target, got, test.want)
			}
		})
	}
}
