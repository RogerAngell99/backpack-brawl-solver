package benchmark

import (
	"path/filepath"
	"reflect"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestSearchSuiteManifestResolvesStableScenarioHashes(t *testing.T) {
	path := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v1.json")
	resolved, err := ResolveSearchSuiteManifest(path)
	if err != nil {
		t.Fatalf("resolve suite: %v", err)
	}
	if resolved.Manifest.Name != "general-search-v1" || len(resolved.Manifest.Scenarios) == 0 || len(resolved.ScenarioSHA256) != len(resolved.Manifest.Scenarios) || resolved.ManifestSHA256 == "" {
		t.Fatalf("resolved=%+v", resolved)
	}
}

func TestMaterializeGeneratedSearchSuiteCaseIsDeterministic(t *testing.T) {
	seed := int64(7)
	catalog := model.Catalog{Items: map[string]model.Item{
		"source-a": {ID: "source-a", Shape: []model.Coord{{}}, Stars: []model.Star{{TargetTypes: []string{"Food"}}}},
		"source-b": {ID: "source-b", Shape: []model.Coord{{}}, Stars: []model.Star{{TargetTypes: []string{"Food"}}}},
		"food-a":   {ID: "food-a", Shape: []model.Coord{{}}, Types: []string{"Food"}},
		"food-b":   {ID: "food-b", Shape: []model.Coord{{}}, Types: []string{"Food"}},
		"food-c":   {ID: "food-c", Shape: []model.Coord{{}}, Types: []string{"Food"}},
	}}
	entry := GeneratedSearchSuiteCase{ID: "case", Family: GeneratedFamilySparse, Role: SuiteRoleDevelopment, Seed: &seed}
	first, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV1, catalog, entry)
	if err != nil {
		t.Fatalf("materialize first: %v", err)
	}
	second, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV1, catalog, entry)
	if err != nil || !reflect.DeepEqual(first, second) || first.NoSkips == nil || !*first.NoSkips || len(first.Priorities) != 2 {
		t.Fatalf("first=%+v second=%+v err=%v", first, second, err)
	}
}

func TestMaterializeSearchSuiteCasesNeverIncludesPrivateHoldouts(t *testing.T) {
	seed := int64(7)
	catalog := model.Catalog{Items: map[string]model.Item{
		"source-a": {ID: "source-a", Shape: []model.Coord{{}}, Stars: []model.Star{{TargetTypes: []string{"Food"}}}},
		"source-b": {ID: "source-b", Shape: []model.Coord{{}}, Stars: []model.Star{{TargetTypes: []string{"Food"}}}},
		"food-a":   {ID: "food-a", Shape: []model.Coord{{}}, Types: []string{"Food"}},
		"food-b":   {ID: "food-b", Shape: []model.Coord{{}}, Types: []string{"Food"}},
		"food-c":   {ID: "food-c", Shape: []model.Coord{{}}, Types: []string{"Food"}},
	}}
	manifest := SearchSuiteManifest{Generated: []GeneratedSearchSuiteCase{
		{ID: "dev", Family: GeneratedFamilySparse, Role: SuiteRoleDevelopment, Seed: &seed},
		{ID: "private", Family: GeneratedFamilyPrivate, Role: SuiteRolePrivateHoldout, PrivateSeedID: "private"},
	}}
	generated, err := MaterializeSearchSuiteCases(SearchSuiteGeneratorV1, catalog, manifest)
	if err != nil || len(generated) != 1 || generated[0].Name != "dev" {
		t.Fatalf("generated=%+v err=%v", generated, err)
	}
}

func TestMaterializeSearchSuiteCasesRejectsExplicitPrivateHoldoutRole(t *testing.T) {
	manifest := SearchSuiteManifest{Generated: []GeneratedSearchSuiteCase{
		{ID: "private-b", Family: GeneratedFamilyPrivate, Role: SuiteRolePrivateHoldout, PrivateSeedID: "private-b"},
		{ID: "private-a", Family: GeneratedFamilyPrivate, Role: SuiteRolePrivateHoldout, PrivateSeedID: "private-a"},
	}}
	_, err := MaterializeSearchSuiteCases(SearchSuiteGeneratorV1, model.Catalog{}, manifest, SuiteRolePrivateHoldout)
	if err == nil || err.Error() != `private holdout "private-a" cannot be materialized by the public suite materializer` {
		t.Fatalf("err=%v", err)
	}
}
