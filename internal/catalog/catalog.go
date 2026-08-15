package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"backpack-brawl-solver/internal/model"
)

const DefaultPath = "data/catalog.json"

type rawCatalog struct {
	Items   []rawItem   `json:"items"`
	Recipes []rawRecipe `json:"recipes"`
}

type rawItem struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Types       []string   `json:"types"`
	Shape       [][]int    `json:"shape"`
	Stars       []rawStar  `json:"stars"`
	CountsAs    []rawAlias `json:"counts_as"`
	AbilityText string     `json:"ability_text"`
	SourceURL   string     `json:"source_url"`
	ImageURL    string     `json:"image_url"`
	ImagePath   string     `json:"image_path"`
	NeedsReview bool       `json:"needs_review"`
	Rotations   []int      `json:"rotations"`
}

type rawStar struct {
	Offset            []int    `json:"offset"`
	TargetTypes       []string `json:"target_types"`
	TargetItems       []string `json:"target_items"`
	ExcludeSourceItem bool     `json:"exclude_source_item"`
	RuleStatus        string   `json:"rule_status"`
	EffectText        string   `json:"effect_text"`
}

type rawRecipe struct {
	Result      string   `json:"result"`
	Anchor      string   `json:"anchor"`
	Ingredients []string `json:"ingredients"`
	SourceURL   string   `json:"source_url"`
}

type rawAlias struct {
	ItemID string `json:"item_id"`
	Count  int    `json:"count"`
}

func Load(path string) (model.Catalog, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return model.Catalog{}, err
	}
	return Parse(content)
}

func Parse(content []byte) (model.Catalog, error) {
	var raw rawCatalog
	if err := json.Unmarshal(content, &raw); err != nil {
		return model.Catalog{}, err
	}

	items := make(map[string]model.Item, len(raw.Items))
	for _, rawItem := range raw.Items {
		item, err := loadItem(rawItem)
		if err != nil {
			return model.Catalog{}, err
		}
		if _, exists := items[item.ID]; exists {
			return model.Catalog{}, fmt.Errorf("%s: duplicate item id", item.ID)
		}
		items[item.ID] = item
	}
	compileStarMetadata(items)

	recipes := make([]model.Recipe, 0, len(raw.Recipes))
	for _, recipe := range raw.Recipes {
		recipes = append(recipes, model.Recipe{
			Result:               recipe.Result,
			Anchor:               recipe.Anchor,
			Ingredients:          recipe.Ingredients,
			SourceURL:            recipe.SourceURL,
			CompiledRequirements: model.BuildRecipeRequirements(recipe.Anchor, recipe.Ingredients),
		})
	}

	return model.Catalog{Items: items, Recipes: recipes}, nil
}

func compileStarMetadata(items map[string]model.Item) {
	itemIDs := sortedItemIDs(items)
	itemIndex := make(map[string]uint16, len(itemIDs))
	for idx, itemID := range itemIDs {
		itemIndex[itemID] = uint16(idx)
	}

	typeIDs, typeMasksReady := catalogTypeIDs(items)
	typeIndex := make(map[string]uint8, len(typeIDs))
	if typeMasksReady {
		for idx, typeID := range typeIDs {
			typeIndex[typeID] = uint8(idx)
		}
	}

	for _, itemID := range itemIDs {
		item := items[itemID]
		item.CompiledTypeMask, item.CompiledReady = compiledTypeMask(item.Types, typeIndex, typeMasksReady)
		item.CompiledItemRefLen, item.CompiledItemRefsComplete = compileItemRefs(&item, itemIndex)
		for starIndex := range item.Stars {
			star := &item.Stars[starIndex]
			star.CompiledTargetTypeMask, star.CompiledReady = compiledTypeMask(star.TargetTypes, typeIndex, typeMasksReady)
			star.CompiledTargetItemLen, star.CompiledTargetItemsComplete = compileTargetItemRef(star, itemIndex)
		}
		items[itemID] = item
	}
}

func catalogTypeIDs(items map[string]model.Item) ([]string, bool) {
	seen := map[string]struct{}{}
	for _, item := range items {
		for _, itemType := range item.Types {
			seen[itemType] = struct{}{}
		}
		for _, star := range item.Stars {
			for _, targetType := range star.TargetTypes {
				seen[targetType] = struct{}{}
			}
		}
	}
	typeIDs := make([]string, 0, len(seen))
	for typeID := range seen {
		typeIDs = append(typeIDs, typeID)
	}
	sort.Strings(typeIDs)
	return typeIDs, len(typeIDs) <= 32
}

func compiledTypeMask(values []string, typeIndex map[string]uint8, ready bool) (uint32, bool) {
	if !ready {
		return 0, false
	}
	var mask uint32
	for _, value := range values {
		idx, ok := typeIndex[value]
		if !ok {
			return 0, false
		}
		mask |= uint32(1) << uint(idx)
	}
	return mask, true
}

func compileItemRefs(item *model.Item, itemIndex map[string]uint16) (uint8, bool) {
	complete := true
	if id, ok := itemIndex[item.ID]; ok {
		item.CompiledItemID = id
	} else {
		complete = false
	}
	length := uint8(1)
	for _, alias := range item.CountsAs {
		id, ok := itemIndex[alias.ItemID]
		if !ok {
			complete = false
			continue
		}
		if item.CompiledAliasItemLen > 0 && item.CompiledAliasItemID == id {
			continue
		}
		if item.CompiledAliasItemLen > 0 {
			complete = false
			continue
		}
		item.CompiledAliasItemID = id
		item.CompiledAliasItemLen = 1
		length++
	}
	return length, complete
}

func compileTargetItemRef(star *model.Star, itemIndex map[string]uint16) (uint8, bool) {
	if len(star.TargetItems) == 0 {
		return 0, true
	}
	complete := true
	for _, value := range star.TargetItems {
		id, ok := itemIndex[value]
		if !ok {
			complete = false
			continue
		}
		if star.CompiledTargetItemLen > 0 && star.CompiledTargetItemID == id {
			continue
		}
		if star.CompiledTargetItemLen > 0 {
			complete = false
			continue
		}
		star.CompiledTargetItemID = id
		star.CompiledTargetItemLen = 1
	}
	return star.CompiledTargetItemLen, complete
}

func loadItem(raw rawItem) (model.Item, error) {
	shape, err := loadCoords(raw.Shape, "shape")
	if err != nil {
		return model.Item{}, fmt.Errorf("%s: %w", raw.ID, err)
	}

	stars := make([]model.Star, 0, len(raw.Stars))
	for idx, rawStar := range raw.Stars {
		if len(rawStar.Offset) != 2 {
			return model.Item{}, fmt.Errorf("%s: stars[%d].offset must be [row, col]", raw.ID, idx)
		}
		stars = append(stars, model.Star{
			Offset:            model.Coord{Row: rawStar.Offset[0], Col: rawStar.Offset[1]},
			TargetTypes:       rawStar.TargetTypes,
			TargetItems:       rawStar.TargetItems,
			ExcludeSourceItem: rawStar.ExcludeSourceItem,
			RuleStatus:        starRuleStatus(rawStar.RuleStatus),
			EffectText:        rawStar.EffectText,
		})
	}

	countsAs := make([]model.ItemAlias, 0, len(raw.CountsAs))
	for _, alias := range raw.CountsAs {
		countsAs = append(countsAs, model.ItemAlias{
			ItemID: alias.ItemID,
			Count:  alias.Count,
		})
	}

	return model.Item{
		ID:          raw.ID,
		Name:        raw.Name,
		Types:       raw.Types,
		Shape:       shape,
		Stars:       stars,
		CountsAs:    countsAs,
		AbilityText: raw.AbilityText,
		SourceURL:   raw.SourceURL,
		ImageURL:    raw.ImageURL,
		ImagePath:   raw.ImagePath,
		NeedsReview: raw.NeedsReview,
		Rotations:   raw.Rotations,
	}, nil
}

func starRuleStatus(value string) string {
	if value == "unknown" {
		return value
	}
	return "known"
}

func loadCoords(values [][]int, field string) ([]model.Coord, error) {
	coords := make([]model.Coord, 0, len(values))
	for idx, value := range values {
		if len(value) != 2 {
			return nil, fmt.Errorf("%s[%d] must be [row, col]", field, idx)
		}
		coords = append(coords, model.Coord{Row: value[0], Col: value[1]})
	}
	return coords, nil
}

func Validate(catalog model.Catalog) (warnings []string, errors []string) {
	itemIDs := sortedItemIDs(catalog.Items)
	for _, itemID := range itemIDs {
		item := catalog.Items[itemID]
		if len(item.Shape) == 0 {
			errors = append(errors, fmt.Sprintf("%s: shape must contain at least one cell", item.ID))
		}
		seenCells := map[model.Coord]bool{}
		for _, coord := range item.Shape {
			if seenCells[coord] {
				errors = append(errors, fmt.Sprintf("%s: shape contains duplicate cells", item.ID))
				break
			}
			seenCells[coord] = true
		}
		rotations := item.Rotations
		if len(rotations) == 0 {
			rotations = []int{0, 90, 180, 270}
		}
		for _, rotation := range rotations {
			if rotation != 0 && rotation != 90 && rotation != 180 && rotation != 270 {
				errors = append(errors, fmt.Sprintf("%s: invalid rotation %d", item.ID, rotation))
			}
		}
		warnedMissing := map[string]bool{}
		for _, star := range item.Stars {
			var missing []string
			for _, targetItem := range star.TargetItems {
				if _, ok := catalog.Items[targetItem]; !ok {
					missing = append(missing, targetItem)
				}
			}
			if len(missing) > 0 {
				key := stringsKey(missing)
				if !warnedMissing[key] {
					warnings = append(warnings, fmt.Sprintf("%s: star references missing target item(s): %s", item.ID, key))
					warnedMissing[key] = true
				}
			}
		}
		warnedMissingAlias := map[string]bool{}
		for _, alias := range item.CountsAs {
			if alias.ItemID == "" {
				errors = append(errors, fmt.Sprintf("%s: counts_as item_id must not be empty", item.ID))
			}
			if alias.Count <= 0 {
				errors = append(errors, fmt.Sprintf("%s: counts_as %q count must be positive", item.ID, alias.ItemID))
			}
			if _, ok := catalog.Items[alias.ItemID]; alias.ItemID != "" && !ok && !warnedMissingAlias[alias.ItemID] {
				warnings = append(warnings, fmt.Sprintf("%s: counts_as item %q is not in catalog yet", item.ID, alias.ItemID))
				warnedMissingAlias[alias.ItemID] = true
			}
		}
		if item.NeedsReview {
			warnings = append(warnings, fmt.Sprintf("%s: marked as needs_review", item.ID))
		}
	}

	for _, recipe := range catalog.Recipes {
		if _, ok := catalog.Items[recipe.Anchor]; !ok {
			warnings = append(warnings, fmt.Sprintf("%s: recipe anchor %q is not in catalog yet", recipe.Result, recipe.Anchor))
		}
		anchorInIngredients := false
		warnedMissingIngredients := map[string]bool{}
		for _, ingredient := range recipe.Ingredients {
			if ingredient == recipe.Anchor {
				anchorInIngredients = true
			}
			if ingredient == recipe.Anchor {
				continue
			}
			if _, ok := catalog.Items[ingredient]; !ok && !warnedMissingIngredients[ingredient] {
				warnings = append(warnings, fmt.Sprintf("%s: recipe ingredient %q is not in catalog yet", recipe.Result, ingredient))
				warnedMissingIngredients[ingredient] = true
			}
		}
		if !anchorInIngredients {
			errors = append(errors, fmt.Sprintf("%s: recipe anchor must be listed in ingredients", recipe.Result))
		}
		if _, ok := catalog.Items[recipe.Result]; !ok {
			warnings = append(warnings, fmt.Sprintf("%s: recipe result is not an item in catalog", recipe.Result))
		}
	}

	return warnings, errors
}

func sortedItemIDs(items map[string]model.Item) []string {
	ids := make([]string, 0, len(items))
	for id := range items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func stringsKey(values []string) string {
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	result := ""
	for idx, value := range copied {
		if idx > 0 {
			result += ", "
		}
		result += value
	}
	return result
}
