package solver

import (
	"math/rand"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	catalogpkg "backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scoring"
)

func TestPriorityStarCompatibilityMatchesProductionCatalogExhaustive(t *testing.T) {
	catalog, err := catalogpkg.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load runtime catalog: %v", err)
	}
	itemIDs := make([]string, 0, len(catalog.Items))
	for itemID := range catalog.Items {
		itemIDs = append(itemIDs, itemID)
	}
	sort.Strings(itemIDs)

	const targetsPerRelation = 63
	for _, sourceItemID := range itemIDs {
		source := catalog.Items[sourceItemID]
		if len(source.Stars) == 0 {
			continue
		}
		for first := 0; first < len(itemIDs); first += targetsPerRelation {
			last := minInt(first+targetsPerRelation, len(itemIDs))
			instances := []model.InventoryInstance{{
				InstanceID:    sourceItemID + "#source",
				ItemID:        sourceItemID,
				OriginalIndex: 0,
			}}
			for targetOffset, targetItemID := range itemIDs[first:last] {
				instances = append(instances, model.InventoryInstance{
					InstanceID:    targetItemID + "#target",
					ItemID:        targetItemID,
					OriginalIndex: targetOffset + 1,
				})
			}
			compatibility := newPriorityStarCompatibility(catalog, instances, []string{sourceItemID})
			if compatibility == nil {
				t.Fatalf("nil compatibility for source %q target range [%d,%d)", sourceItemID, first, last)
			}
			for starIndex := range source.Stars {
				for targetOffset, targetItemID := range itemIDs[first:last] {
					got, cached := compatibility.match(0, starIndex, targetOffset+1)
					want := scoring.StarMatchesCatalogItems(catalog, sourceItemID, targetItemID, &source.Stars[starIndex])
					if !cached || got != want {
						t.Fatalf("source=%q star=%d target=%q: match=(%t,%t) want=(%t,true)", sourceItemID, starIndex, targetItemID, got, cached, want)
					}
				}
			}
		}
	}
}

func TestPriorityStarCompatibilityConditionCoverage(t *testing.T) {
	catalog := priorityStarConditionCatalog()
	itemIDs := make([]string, 0, len(catalog.Items))
	sourceItemIDs := make([]string, 0)
	for itemID, item := range catalog.Items {
		itemIDs = append(itemIDs, itemID)
		if len(item.Stars) > 0 {
			sourceItemIDs = append(sourceItemIDs, itemID)
		}
	}
	sort.Strings(itemIDs)
	sort.Strings(sourceItemIDs)
	instances := ExpandInventory(itemIDs)
	compatibility := newPriorityStarCompatibility(catalog, instances, sourceItemIDs)
	if compatibility == nil {
		t.Fatal("condition fixture did not produce compatibility")
	}
	boundContext := newPriorityBoundContext(
		catalog,
		instances,
		[]string{"star_source:filter_source"},
		model.PrioritySemanticsOutgoingPerInstanceV3,
	)
	if boundContext == nil || boundContext.staticStarCompatibility() == nil {
		t.Fatal("priority bound context did not own the compatibility relation")
	}
	instanceByItem := make(map[string]model.InventoryInstance, len(instances))
	for _, instance := range instances {
		instanceByItem[instance.ItemID] = instance
	}

	for _, sourceItemID := range sourceItemIDs {
		source := catalog.Items[sourceItemID]
		sourceInstance := instanceByItem[sourceItemID]
		for starIndex := range source.Stars {
			for _, targetItemID := range itemIDs {
				targetInstance := instanceByItem[targetItemID]
				got, cached := compatibility.match(sourceInstance.OriginalIndex, starIndex, targetInstance.OriginalIndex)
				want := scoring.StarMatchesCatalogItems(catalog, sourceItemID, targetItemID, &source.Stars[starIndex])
				if !cached || got != want {
					t.Fatalf("source=%q star=%d target=%q: match=(%t,%t) want=(%t,true)", sourceItemID, starIndex, targetItemID, got, cached, want)
				}
			}
		}
	}

	zero := instanceByItem["zero_source"]
	if match, cached := compatibility.match(zero.OriginalIndex, 0, zero.OriginalIndex); match || cached {
		t.Fatalf("zero-star source unexpectedly returned match=(%t,%t)", match, cached)
	}
	duplicateInstances := ExpandInventory([]string{"filter_source", "filter_source", "food"})
	duplicateCompatibility := newPriorityStarCompatibility(catalog, duplicateInstances, []string{"filter_source"})
	if duplicateCompatibility == nil {
		t.Fatal("duplicate item instances should be supported")
	}
	first := duplicateCompatibility.slotsBySourceOriginal[0]
	second := duplicateCompatibility.slotsBySourceOriginal[1]
	if len(first) == 0 || len(second) == 0 || &first[0] != &second[0] {
		t.Fatal("duplicate source items did not share the immutable slot slice")
	}
}

func TestPriorityStarCompatibilityRejectsUnsafeDomains(t *testing.T) {
	catalog := priorityStarConditionCatalog()
	valid := ExpandInventory([]string{"filter_source", "food"})
	tests := []struct {
		name      string
		instances []model.InventoryInstance
		sources   []string
	}{
		{name: "empty", sources: []string{"filter_source"}},
		{name: "negative original", instances: []model.InventoryInstance{{InstanceID: "source", ItemID: "filter_source", OriginalIndex: -1}}, sources: []string{"filter_source"}},
		{name: "original at 64", instances: []model.InventoryInstance{{InstanceID: "source", ItemID: "filter_source", OriginalIndex: 64}}, sources: []string{"filter_source"}},
		{name: "duplicate original", instances: []model.InventoryInstance{{InstanceID: "source", ItemID: "filter_source", OriginalIndex: 1}, {InstanceID: "food", ItemID: "food", OriginalIndex: 1}}, sources: []string{"filter_source"}},
		{name: "missing source", instances: valid, sources: []string{"missing"}},
	}
	tooMany := make([]model.InventoryInstance, 65)
	for index := range tooMany {
		tooMany[index] = model.InventoryInstance{InstanceID: "instance", ItemID: "food", OriginalIndex: index}
	}
	tests = append(tests, struct {
		name      string
		instances []model.InventoryInstance
		sources   []string
	}{name: "more than 64", instances: tooMany, sources: []string{"filter_source"}})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := newPriorityStarCompatibility(catalog, test.instances, test.sources); got != nil {
				t.Fatalf("newPriorityStarCompatibility()=%+v want nil", got)
			}
		})
	}
}

func TestPriorityStarCompatibilityLookupFallbacks(t *testing.T) {
	catalog := priorityStarConditionCatalog()
	instances := ExpandInventory([]string{"filter_source", "food", "zero_source"})
	compatibility := newPriorityStarCompatibility(catalog, instances, []string{"filter_source"})
	if compatibility == nil {
		t.Fatal("expected compatibility")
	}
	tests := []struct {
		name   string
		ctx    *priorityStarCompatibility
		source int
		star   int
		target int
	}{
		{name: "nil relation", source: 0, star: 0, target: 1},
		{name: "negative source", ctx: compatibility, source: -1, star: 0, target: 1},
		{name: "large source", ctx: compatibility, source: 64, star: 0, target: 1},
		{name: "negative target", ctx: compatibility, source: 0, star: 0, target: -1},
		{name: "large target", ctx: compatibility, source: 0, star: 0, target: 64},
		{name: "target without entry", ctx: compatibility, source: 0, star: 0, target: 63},
		{name: "negative star", ctx: compatibility, source: 0, star: -1, target: 1},
		{name: "large star", ctx: compatibility, source: 0, star: 99, target: 1},
		{name: "source without entry", ctx: compatibility, source: 2, star: 0, target: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if match, cached := test.ctx.match(test.source, test.star, test.target); match || cached {
				t.Fatalf("match=(%t,%t) want (false,false)", match, cached)
			}
		})
	}
	var nilContext *priorityBoundContext
	if nilContext.staticStarCompatibility() != nil {
		t.Fatal("nil priority context returned a compatibility relation")
	}
}

func TestPriorityStarCompatibilityIsRotationInvariant(t *testing.T) {
	catalog := priorityStarConditionCatalog()
	source := catalog.Items["filter_source"]
	source.Shape = []model.Coord{{Row: 0, Col: 0}, {Row: 0, Col: 1}}
	source.Rotations = []int{0, 90, 180, 270}
	catalog.Items["filter_source"] = source
	instances := ExpandInventory([]string{"filter_source", "food", "weapon", "alias_food"})
	compatibility := newPriorityStarCompatibility(catalog, instances, []string{"filter_source"})
	if compatibility == nil {
		t.Fatal("expected compatibility")
	}
	for _, rotation := range source.Rotations {
		variant, err := geometry.NormalizeVariant(source.Shape, source.Stars, rotation)
		if err != nil {
			t.Fatalf("NormalizeVariant(%d): %v", rotation, err)
		}
		if len(variant.Stars) != len(source.Stars) {
			t.Fatalf("rotation %d star count=%d want %d", rotation, len(variant.Stars), len(source.Stars))
		}
		for starIndex := range source.Stars {
			for targetOriginal := 1; targetOriginal < len(instances); targetOriginal++ {
				target := instances[targetOriginal]
				catalogMatch := scoring.StarMatchesCatalogItems(catalog, source.ID, target.ItemID, &source.Stars[starIndex])
				rotatedMatch := scoring.StarMatchesCatalogItems(catalog, source.ID, target.ItemID, &variant.Stars[starIndex])
				cachedMatch, cached := compatibility.match(0, starIndex, target.OriginalIndex)
				if !cached || catalogMatch != rotatedMatch || cachedMatch != catalogMatch {
					t.Fatalf("rotation=%d star=%d target=%q catalog=%t rotated=%t cached=(%t,%t)", rotation, starIndex, target.ItemID, catalogMatch, rotatedMatch, cachedMatch, cached)
				}
			}
		}
	}
}

func TestPartialRepairPriorityUpperCachedMatchesLegacy(t *testing.T) {
	catalog, instances, optionsByInstance, states, priorities := priorityCompatibilityBoundFixture(t)
	compatibility := newPriorityStarCompatibility(catalog, instances, []string{"source"})
	if compatibility == nil {
		t.Fatal("expected compatibility")
	}
	for stateIndex, state := range states {
		want := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, nil)
		got := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, compatibility)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("fixture state %d: cached upper=%v legacy=%v", stateIndex, got, want)
		}
	}
	emptyOptions := make(map[string][]model.Placement, len(optionsByInstance))
	for instanceID, options := range optionsByInstance {
		emptyOptions[instanceID] = options
	}
	emptyOptions[instances[1].InstanceID] = nil
	emptyOptions[instances[3].InstanceID] = nil
	wantEmpty := partialRepairV3PriorityUpperBound(catalog, states[4], emptyOptions, priorities, nil)
	gotEmpty := partialRepairV3PriorityUpperBound(catalog, states[4], emptyOptions, priorities, compatibility)
	if !reflect.DeepEqual(gotEmpty, wantEmpty) {
		t.Fatalf("empty options: cached upper=%v legacy=%v", gotEmpty, wantEmpty)
	}
	missingEntries := newPriorityStarCompatibility(catalog, instances, nil)
	wantFallback := partialRepairV3PriorityUpperBound(catalog, states[4], optionsByInstance, priorities, nil)
	gotFallback := partialRepairV3PriorityUpperBound(catalog, states[4], optionsByInstance, priorities, missingEntries)
	if !reflect.DeepEqual(gotFallback, wantFallback) {
		t.Fatalf("missing compatibility entries: cached upper=%v legacy=%v", gotFallback, wantFallback)
	}

	random := rand.New(rand.NewSource(0x52314943))
	for stateIndex := 0; stateIndex < 500; stateIndex++ {
		state := randomPriorityCompatibilityState(random, instances, optionsByInstance)
		want := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, nil)
		got := partialRepairV3PriorityUpperBound(catalog, state, optionsByInstance, priorities, compatibility)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("generated state %d: cached upper=%v legacy=%v state=%+v", stateIndex, got, want, state)
		}
	}
}

func priorityStarConditionCatalog() model.Catalog {
	star := func() []model.Star { return []model.Star{{Offset: model.Coord{Col: 1}}} }
	return model.Catalog{Items: map[string]model.Item{
		"filter_source": {
			ID: "filter_source", Shape: []model.Coord{{}}, Rotations: []int{0, 90, 180, 270},
			Stars: []model.Star{
				{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}},
				{Offset: model.Coord{Row: 1}, TargetItems: []string{"wanted"}},
				{Offset: model.Coord{Col: -1}, ExcludeSourceItem: true},
			},
		},
		"unknown_source":   {ID: "unknown_source", Shape: []model.Coord{{}}, Stars: []model.Star{{RuleStatus: "unknown"}}, Rotations: []int{0}},
		"different_source": {ID: "different_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0}, StarCondition: &model.StarCondition{Class: "DefinitionIsDifferent"}},
		"same_source":      {ID: "same_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0}, StarCondition: &model.StarCondition{Class: "DefinitionIsSame"}},
		"type_source":      {ID: "type_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0}, StarCondition: &model.StarCondition{Class: "OtherItemIsOfType", ItemType: "Food"}},
		"stat_source":      {ID: "stat_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0}, StarCondition: &model.StarCondition{Class: "OtherItemHasStatOfType", StatType: "Haste"}},
		"exact_source":     {ID: "exact_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0}, StarCondition: &model.StarCondition{Class: "OtherItemIsExactly", Definition: &model.ItemDefinition{Name: "Exact Target"}}},
		"any_source": {
			ID: "any_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0},
			StarCondition: &model.StarCondition{Class: "CompoundStarCondition", Any: true, Conditions: []model.StarCondition{
				{Class: "OtherItemIsOfType", ItemType: "Missing"},
				{Class: "OtherItemHasStatOfType", StatType: "Haste"},
			}},
		},
		"all_source": {
			ID: "all_source", Shape: []model.Coord{{}}, Stars: star(), Rotations: []int{0},
			StarCondition: &model.StarCondition{Class: "CompoundStarCondition", Conditions: []model.StarCondition{
				{Class: "OtherItemIsOfType", ItemType: "Food"},
				{Class: "OtherItemHasStatOfType", StatType: "Haste"},
			}},
		},
		"zero_source":  {ID: "zero_source", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"food":         {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"weapon":       {ID: "weapon", Types: []string{"Weapon"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"wanted":       {ID: "wanted", Shape: []model.Coord{{}}, Rotations: []int{0}},
		"alias_food":   {ID: "alias_food", CountsAs: []model.ItemAlias{{ItemID: "wanted", Count: 1}}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"hasty_food":   {ID: "hasty_food", Types: []string{"Food"}, StatTypes: []string{"Haste"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"exact_target": {ID: "exact_target", Name: "Exact Target", Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
}

func priorityCompatibilityBoundFixture(t testing.TB) (
	model.Catalog,
	[]model.InventoryInstance,
	map[string][]model.Placement,
	[]partialRepairState,
	[]string,
) {
	t.Helper()
	catalog := model.Catalog{Items: map[string]model.Item{
		"source": {
			ID: "source", Shape: []model.Coord{{}}, Rotations: []int{0, 90, 180, 270},
			Stars: []model.Star{
				{Offset: model.Coord{Col: -1}, TargetTypes: []string{"Food"}},
				{Offset: model.Coord{Col: 1}, TargetTypes: []string{"Food"}},
				{Offset: model.Coord{Row: 1}, TargetItems: []string{"weapon"}},
			},
		},
		"food":   {ID: "food", Types: []string{"Food"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"weapon": {ID: "weapon", Types: []string{"Weapon"}, Shape: []model.Coord{{}}, Rotations: []int{0}},
		"mimic":  {ID: "mimic", CountsAs: []model.ItemAlias{{ItemID: "weapon", Count: 1}}, Shape: []model.Coord{{}}, Rotations: []int{0}},
	}}
	instances := ExpandInventory([]string{"source", "source", "food", "food", "weapon", "mimic"})
	gridMask := geometry.FullGridMask()
	allOptions := testOptionsForGrid(t, catalog, instances, gridMask)
	optionsByInstance := make(map[string][]model.Placement, len(allOptions))
	for instanceID, options := range allOptions {
		optionsByInstance[instanceID] = spreadPriorityCompatibilityOptions(options, 12)
	}

	fixedSource := optionsByInstance[instances[0].InstanceID][0]
	fixedFood := firstNonOverlappingPriorityOption(optionsByInstance[instances[2].InstanceID], fixedSource.Mask)
	overlappingFood := fixedSource
	overlappingFood.InstanceID = instances[2].InstanceID
	overlappingFood.ItemID = instances[2].ItemID
	overlappingFood.OriginalIndex = instances[2].OriginalIndex
	states := []partialRepairState{
		{FixedPlacements: []model.Placement{fixedSource, fixedFood}, FreeCells: gridMask &^ fixedSource.Mask &^ fixedFood.Mask},
		{FixedPlacements: []model.Placement{fixedFood}, RemovedInstances: []model.InventoryInstance{instances[0]}, FreeCells: gridMask &^ fixedFood.Mask},
		{FixedPlacements: []model.Placement{fixedSource}, RemovedInstances: []model.InventoryInstance{instances[2]}, FreeCells: gridMask &^ fixedSource.Mask},
		{RemovedInstances: []model.InventoryInstance{instances[0], instances[2]}, FreeCells: gridMask},
		{FixedPlacements: []model.Placement{fixedSource, fixedFood}, RemovedInstances: []model.InventoryInstance{instances[1], instances[3], instances[4], instances[5]}, FreeCells: gridMask &^ fixedSource.Mask &^ fixedFood.Mask},
		{FixedPlacements: []model.Placement{fixedSource, overlappingFood}, RemovedInstances: []model.InventoryInstance{instances[1], instances[3]}, FreeCells: gridMask &^ fixedSource.Mask},
		{FixedPlacements: []model.Placement{fixedSource}, CurrentPlacements: []model.Placement{fixedFood}, RemovedInstances: []model.InventoryInstance{instances[0], instances[2], instances[3]}, FreeCells: gridMask &^ fixedSource.Mask &^ fixedFood.Mask},
		{RemovedInstances: []model.InventoryInstance{instances[0], instances[1], instances[2]}, FreeCells: uint64(1)},
		{RemovedInstances: []model.InventoryInstance{instances[0], instances[2]}, FreeCells: gridMask},
	}
	return catalog, instances, optionsByInstance, states, []string{"star_source:source", "star_source:source"}
}

func spreadPriorityCompatibilityOptions(options []model.Placement, limit int) []model.Placement {
	if len(options) <= limit {
		return append([]model.Placement(nil), options...)
	}
	selected := make([]model.Placement, 0, limit)
	for index := 0; index < limit; index++ {
		selected = append(selected, options[index*(len(options)-1)/(limit-1)])
	}
	return selected
}

func firstNonOverlappingPriorityOption(options []model.Placement, occupied uint64) model.Placement {
	for _, option := range options {
		if option.Mask&occupied == 0 {
			return option
		}
	}
	return model.Placement{}
}

func randomPriorityCompatibilityState(
	random *rand.Rand,
	instances []model.InventoryInstance,
	optionsByInstance map[string][]model.Placement,
) partialRepairState {
	state := partialRepairState{FreeCells: geometry.FullGridMask()}
	occupied := uint64(0)
	for _, instanceIndex := range random.Perm(len(instances)) {
		instance := instances[instanceIndex]
		role := random.Intn(4)
		if role <= 1 {
			options := optionsByInstance[instance.InstanceID]
			start := random.Intn(len(options))
			var chosen model.Placement
			for offset := range options {
				option := options[(start+offset)%len(options)]
				if option.Mask&occupied == 0 {
					chosen = option
					break
				}
			}
			if chosen.InstanceID != "" {
				occupied |= chosen.Mask
				if role == 0 {
					state.FixedPlacements = append(state.FixedPlacements, chosen)
				} else {
					state.CurrentPlacements = append(state.CurrentPlacements, chosen)
					state.RemovedInstances = append(state.RemovedInstances, instance)
				}
				continue
			}
		}
		state.RemovedInstances = append(state.RemovedInstances, instance)
	}
	state.FreeCells &^= occupied
	for removals := random.Intn(4); removals > 0; removals-- {
		state.FreeCells &^= uint64(1) << uint(random.Intn(54))
	}
	return state
}
