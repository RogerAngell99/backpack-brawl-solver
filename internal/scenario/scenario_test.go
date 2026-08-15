package scenario

import (
	"os"
	"path/filepath"
	"testing"
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

func writeScenario(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	return path
}
