package catalog

import (
	"fmt"

	"backpack-brawl-solver/internal/model"
)

const (
	HeroFilterAny            = "any"
	HeroFilterAll            = "all"
	HeroFilterShared         = "shared"
	HeroExcludeStrict        = "strict"
	HeroExcludeExclusiveOnly = "exclusive_only"
	HeroUnknownExclude       = "exclude"
	HeroUnknownInclude       = "include"
	HeroUnknownError         = "error"
)

func FilterForHeroes(source model.Catalog, filter model.HeroFilter) (model.Catalog, error) {
	filter = normalizeHeroFilter(filter)
	if err := validateHeroFilter(source, filter); err != nil {
		return model.Catalog{}, err
	}
	if !heroFilterActive(filter) {
		return source, nil
	}

	items := make(map[string]model.Item, len(source.Items))
	for itemID, item := range source.Items {
		keep, err := heroScopeMatches(item.HeroScope, filter)
		if err != nil {
			return model.Catalog{}, fmt.Errorf("item %q: %w", itemID, err)
		}
		if keep {
			items[itemID] = item
		}
	}

	recipes := make([]model.Recipe, 0, len(source.Recipes))
	for _, recipe := range source.Recipes {
		keep, err := heroScopeMatches(recipe.HeroScope, filter)
		if err != nil {
			return model.Catalog{}, fmt.Errorf("recipe %q: %w", recipe.Result, err)
		}
		if !keep || !recipeItemsExist(items, recipe) {
			continue
		}
		recipes = append(recipes, recipe)
	}

	return model.Catalog{Heroes: source.Heroes, Items: items, Recipes: recipes}, nil
}

func normalizeHeroFilter(filter model.HeroFilter) model.HeroFilter {
	if filter.Mode == "" {
		filter.Mode = HeroFilterAny
	}
	if filter.ExcludeMode == "" {
		filter.ExcludeMode = HeroExcludeStrict
	}
	if filter.UnknownPolicy == "" {
		filter.UnknownPolicy = HeroUnknownExclude
	}
	return filter
}

func validateHeroFilter(source model.Catalog, filter model.HeroFilter) error {
	if filter.Mode != HeroFilterAny && filter.Mode != HeroFilterAll && filter.Mode != HeroFilterShared {
		return fmt.Errorf("hero filter mode %q is invalid", filter.Mode)
	}
	if filter.ExcludeMode != HeroExcludeStrict && filter.ExcludeMode != HeroExcludeExclusiveOnly {
		return fmt.Errorf("hero filter exclude_mode %q is invalid", filter.ExcludeMode)
	}
	if filter.UnknownPolicy != HeroUnknownExclude && filter.UnknownPolicy != HeroUnknownInclude && filter.UnknownPolicy != HeroUnknownError {
		return fmt.Errorf("hero filter unknown_policy %q is invalid", filter.UnknownPolicy)
	}
	known := make(map[string]bool, len(source.Heroes))
	for _, hero := range source.Heroes {
		known[hero.ID] = true
	}
	for _, heroID := range append(append([]string{}, filter.IncludeHeroes...), filter.ExcludeHeroes...) {
		if !known[heroID] {
			return fmt.Errorf("hero filter references unknown hero %q", heroID)
		}
	}
	if filter.Mode == HeroFilterShared && len(source.Heroes) == 0 {
		return fmt.Errorf("shared hero filter requires catalog hero metadata")
	}
	return nil
}

func heroFilterActive(filter model.HeroFilter) bool {
	return filter.Mode == HeroFilterShared || len(filter.IncludeHeroes) > 0 || len(filter.ExcludeHeroes) > 0
}

func heroScopeMatches(scope *model.HeroScope, filter model.HeroFilter) (bool, error) {
	if scope == nil || scope.Status != "confirmed" || scope.Kind == "unknown" {
		switch filter.UnknownPolicy {
		case HeroUnknownInclude:
			return true, nil
		case HeroUnknownError:
			return false, fmt.Errorf("hero scope is unknown")
		default:
			return false, nil
		}
	}
	available := make(map[string]bool, len(scope.AvailableTo))
	for _, heroID := range scope.AvailableTo {
		available[heroID] = true
	}
	if filter.Mode == HeroFilterShared && scope.Kind != "shared" {
		return false, nil
	}
	if len(filter.IncludeHeroes) > 0 {
		if filter.Mode == HeroFilterAll {
			for _, heroID := range filter.IncludeHeroes {
				if !available[heroID] {
					return false, nil
				}
			}
		} else {
			matches := false
			for _, heroID := range filter.IncludeHeroes {
				if available[heroID] {
					matches = true
					break
				}
			}
			if !matches {
				return false, nil
			}
		}
	}
	if len(filter.ExcludeHeroes) > 0 {
		if filter.ExcludeMode == HeroExcludeExclusiveOnly {
			allExcluded := true
			for heroID := range available {
				if !contains(filter.ExcludeHeroes, heroID) {
					allExcluded = false
					break
				}
			}
			if allExcluded {
				return false, nil
			}
		} else {
			for _, heroID := range filter.ExcludeHeroes {
				if available[heroID] {
					return false, nil
				}
			}
		}
	}
	return true, nil
}

func recipeItemsExist(items map[string]model.Item, recipe model.Recipe) bool {
	if _, ok := items[recipe.Result]; !ok {
		return false
	}
	if _, ok := items[recipe.Anchor]; !ok {
		return false
	}
	for _, ingredient := range recipe.Ingredients {
		if _, ok := items[ingredient]; !ok {
			return false
		}
	}
	return true
}

func contains(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
