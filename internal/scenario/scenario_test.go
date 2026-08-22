package scenario

import (
	"os"
	"path/filepath"
	"testing"

	"backpack-brawl-solver/internal/geometry"
)

func TestLoadScenarioExpandsItemsDeterministically(t *testing.T) {
	path := writeScenario(t, `{
  "name": "test",
  "grid": [
    "111111111",
    "111111111",
    "111111111",
    "111111111",
    "111111111",
    "111111111"
  ],
  "items": {
    "thornwall": 1,
    "armor_kit": 2
  },
  "top": 5,
  "workers": 2,
  "max_nodes": 0,
  "no_skips": true,
  "stop_on_coverage_ceiling": true,
  "hero_filter": {"include_heroes": ["Warrior"], "exclude_mode": "exclusive_only"},
  "priorities": ["craft:spinegrowth_breastplate", "star_source:scalemail"],
  "coverage_groups": [
    {"name": "Armor", "sources": ["scalemail", "spinegrowth_breastplate"], "targets": ["thornwall"]}
  ]
}`)

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	wantItems := []string{"armor_kit", "armor_kit", "thornwall"}
	gotItems := loaded.ItemIDs()
	if len(gotItems) != len(wantItems) {
		t.Fatalf("len(gotItems)=%d want %d", len(gotItems), len(wantItems))
	}
	for idx := range wantItems {
		if gotItems[idx] != wantItems[idx] {
			t.Fatalf("gotItems[%d]=%q want %q", idx, gotItems[idx], wantItems[idx])
		}
	}
	if loaded.Top == nil || *loaded.Top != 5 {
		t.Fatalf("top=%v want 5", loaded.Top)
	}
	if loaded.Workers == nil || *loaded.Workers != 2 {
		t.Fatalf("workers=%v want 2", loaded.Workers)
	}
	if loaded.MaxNodes == nil || *loaded.MaxNodes != 0 {
		t.Fatalf("max_nodes=%v want 0", loaded.MaxNodes)
	}
	if loaded.NoSkips == nil || *loaded.NoSkips != true {
		t.Fatalf("no_skips=%v want true", loaded.NoSkips)
	}
	if loaded.StopOnCoverageCeiling == nil || *loaded.StopOnCoverageCeiling != true {
		t.Fatalf("stop_on_coverage_ceiling=%v want true", loaded.StopOnCoverageCeiling)
	}
	if len(loaded.HeroFilter.IncludeHeroes) != 1 || loaded.HeroFilter.IncludeHeroes[0] != "Warrior" {
		t.Fatalf("hero filter=%+v", loaded.HeroFilter)
	}
	if loaded.HeroFilter.ExcludeMode != "exclusive_only" {
		t.Fatalf("hero exclude mode=%q", loaded.HeroFilter.ExcludeMode)
	}
	wantPriorities := []string{"craft:spinegrowth_breastplate", "star_source:scalemail"}
	if len(loaded.Priorities) != len(wantPriorities) {
		t.Fatalf("len(priorities)=%d want %d", len(loaded.Priorities), len(wantPriorities))
	}
	for idx := range wantPriorities {
		if loaded.Priorities[idx] != wantPriorities[idx] {
			t.Fatalf("priority[%d]=%q want %q", idx, loaded.Priorities[idx], wantPriorities[idx])
		}
	}
	if len(loaded.CoverageGroups) != 1 {
		t.Fatalf("len(coverage_groups)=%d want 1", len(loaded.CoverageGroups))
	}
	if loaded.CoverageGroups[0].Name != "Armor" {
		t.Fatalf("coverage group name=%q want Armor", loaded.CoverageGroups[0].Name)
	}
	modelGroups := loaded.ModelCoverageGroups()
	if len(modelGroups) != 1 || len(modelGroups[0].Sources) != 2 {
		t.Fatalf("model coverage groups=%+v", modelGroups)
	}
	if len(modelGroups[0].Targets) != 1 || modelGroups[0].Targets[0] != "thornwall" {
		t.Fatalf("model coverage targets=%+v want thornwall", modelGroups[0].Targets)
	}
}

func TestLoadScenarioRejectsEmptyInventory(t *testing.T) {
	path := writeScenario(t, `{"items": {}}`)

	_, err := Load(path)

	if err == nil {
		t.Fatal("Load returned nil error")
	}
}

func TestScenarioValidateEnforcesGridInventoryCapacity(t *testing.T) {
	if err := (Scenario{Items: map[string]int{"one": geometry.GridCells}}).Validate(); err != nil {
		t.Fatalf("Validate at grid capacity returned error: %v", err)
	}
	if err := (Scenario{Items: map[string]int{"one": geometry.GridCells + 1}}).Validate(); err == nil {
		t.Fatal("Validate accepted inventory larger than the grid capacity")
	}
	maxInt := int(^uint(0) >> 1)
	if err := (Scenario{Items: map[string]int{"one": maxInt}}).Validate(); err == nil {
		t.Fatal("Validate accepted an overflowing inventory count")
	}
}

func TestScenarioValidatesPrioritySemanticsAndCoverageReferences(t *testing.T) {
	valid := writeScenario(t, `{
  "items": {"apple": 1},
  "priority_semantics": "outgoing-v2",
  "priorities": ["coverage_group:0"],
  "coverage_groups": [{"name": "Explicit", "sources": ["apple"]}]
}`)
	loaded, err := Load(valid)
	if err != nil {
		t.Fatalf("Load valid outgoing-v2 scenario: %v", err)
	}
	if loaded.PrioritySemantics != "outgoing-v2" {
		t.Fatalf("priority semantics=%q want outgoing-v2", loaded.PrioritySemantics)
	}
	perInstance := writeScenario(t, `{"items": {"apple": 1}, "priority_semantics": "outgoing-per-instance-v3"}`)
	loaded, err = Load(perInstance)
	if err != nil || loaded.PrioritySemantics != "outgoing-per-instance-v3" {
		t.Fatalf("Load valid outgoing-per-instance-v3: scenario=%+v err=%v", loaded, err)
	}
	invalidSemantics := writeScenario(t, `{"items": {"apple": 1}, "priority_semantics": "unknown"}`)
	if _, err := Load(invalidSemantics); err == nil {
		t.Fatal("Load accepted unsupported priority semantics")
	}
	invalidGroup := writeScenario(t, `{"items": {"apple": 1}, "priorities": ["coverage_group:0"]}`)
	if _, err := Load(invalidGroup); err == nil {
		t.Fatal("Load accepted missing coverage group reference")
	}
}

func writeScenario(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	return path
}
