package catalog_test

import (
	"path/filepath"
	"testing"

	"backpack-brawl-solver/internal/catalog"
)

func TestRuntimeProjectionWinsForUpdatedFields(t *testing.T) {
	loaded, err := catalog.Load(filepath.Join("..", "..", "data", "catalog.json"))
	if err != nil {
		t.Fatalf("load runtime projection: %v", err)
	}

	checks := map[string][]string{
		"prickly_potion": {"Potion", "Absorbable"},
		"skull_cap":      {"Armor", "Helmet"},
		"fur_boots":      {"Armor", "Boots"},
	}
	for itemID, wantTypes := range checks {
		item, ok := loaded.Items[itemID]
		if !ok {
			t.Fatalf("%s missing from runtime projection", itemID)
		}
		if len(item.Types) != len(wantTypes) {
			t.Fatalf("%s types=%v want %v", itemID, item.Types, wantTypes)
		}
		for index, want := range wantTypes {
			if item.Types[index] != want {
				t.Fatalf("%s types=%v want %v", itemID, item.Types, wantTypes)
			}
		}
	}

	if len(loaded.Items["wizard_robe"].Shape) != 4 {
		t.Fatalf("wizard_robe runtime shape cells=%d want 4", len(loaded.Items["wizard_robe"].Shape))
	}
	if len(loaded.Items["leather_boots"].Shape) != 3 {
		t.Fatalf("leather_boots runtime shape cells=%d want 3", len(loaded.Items["leather_boots"].Shape))
	}
}
