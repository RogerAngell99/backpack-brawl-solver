package catalog_test

import (
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
)

func heroCatalog() model.Catalog {
	shared := &model.HeroScope{AvailableTo: []string{"celeste", "fern", "ronan"}, Kind: "shared", Status: "confirmed"}
	ronan := &model.HeroScope{AvailableTo: []string{"ronan"}, Kind: "hero_specific", Status: "confirmed"}
	celeste := &model.HeroScope{AvailableTo: []string{"celeste"}, Kind: "hero_specific", Status: "confirmed"}
	multi := &model.HeroScope{AvailableTo: []string{"celeste", "ronan"}, Kind: "multi_hero", Status: "confirmed"}
	return model.Catalog{
		Heroes: []model.Hero{{ID: "celeste"}, {ID: "fern"}, {ID: "ronan"}},
		Items: map[string]model.Item{
			"shared":  {ID: "shared", HeroScope: shared},
			"ronan":   {ID: "ronan", HeroScope: ronan},
			"celeste": {ID: "celeste", HeroScope: celeste},
			"multi":   {ID: "multi", HeroScope: multi},
			"unknown": {ID: "unknown"},
		},
		Recipes: []model.Recipe{
			{Result: "ronan", Anchor: "ronan", Ingredients: []string{"shared"}, HeroScope: ronan},
			{Result: "celeste", Anchor: "celeste", Ingredients: []string{"shared"}, HeroScope: celeste},
		},
	}
}

func itemIDs(catalog model.Catalog) map[string]bool {
	result := make(map[string]bool, len(catalog.Items))
	for itemID := range catalog.Items {
		result[itemID] = true
	}
	return result
}

func TestFilterForHeroIncludesShared(t *testing.T) {
	filtered, err := catalog.FilterForHeroes(heroCatalog(), model.HeroFilter{IncludeHeroes: []string{"ronan"}})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	got := itemIDs(filtered)
	for _, itemID := range []string{"shared", "ronan", "multi"} {
		if !got[itemID] {
			t.Fatalf("item %q missing from Ronan filter: %v", itemID, got)
		}
	}
	for _, itemID := range []string{"celeste", "unknown"} {
		if got[itemID] {
			t.Fatalf("item %q unexpectedly included: %v", itemID, got)
		}
	}
}

func TestFilterForSharedOnly(t *testing.T) {
	filtered, err := catalog.FilterForHeroes(heroCatalog(), model.HeroFilter{Mode: catalog.HeroFilterShared})
	if err != nil {
		t.Fatalf("filter: %v", err)
	}
	got := itemIDs(filtered)
	if !got["shared"] || len(got) != 1 {
		t.Fatalf("shared filter=%v want only shared", got)
	}
}

func TestFilterForHeroExclusionModes(t *testing.T) {
	strict, err := catalog.FilterForHeroes(heroCatalog(), model.HeroFilter{ExcludeHeroes: []string{"ronan"}})
	if err != nil {
		t.Fatalf("strict filter: %v", err)
	}
	if got := itemIDs(strict); got["shared"] || got["ronan"] || got["multi"] || !got["celeste"] {
		t.Fatalf("strict exclusion=%v", got)
	}

	exclusiveOnly, err := catalog.FilterForHeroes(heroCatalog(), model.HeroFilter{
		ExcludeHeroes: []string{"ronan"},
		ExcludeMode:   catalog.HeroExcludeExclusiveOnly,
	})
	if err != nil {
		t.Fatalf("exclusive-only filter: %v", err)
	}
	if got := itemIDs(exclusiveOnly); !got["shared"] || !got["multi"] || got["ronan"] {
		t.Fatalf("exclusive-only exclusion=%v", got)
	}
}

func TestFilterUnknownPolicy(t *testing.T) {
	include, err := catalog.FilterForHeroes(heroCatalog(), model.HeroFilter{
		IncludeHeroes: []string{"ronan"},
		UnknownPolicy: catalog.HeroUnknownInclude,
	})
	if err != nil {
		t.Fatalf("include unknown filter: %v", err)
	}
	if !itemIDs(include)["unknown"] {
		t.Fatal("unknown item should be included by explicit policy")
	}

	_, err = catalog.FilterForHeroes(heroCatalog(), model.HeroFilter{
		IncludeHeroes: []string{"ronan"},
		UnknownPolicy: catalog.HeroUnknownError,
	})
	if err == nil {
		t.Fatal("unknown policy error should fail")
	}
}
