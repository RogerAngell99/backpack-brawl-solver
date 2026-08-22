package benchmark

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

func TestSearchSuiteGeneratorsAreSupported(t *testing.T) {
	if err := ValidateSearchSuiteGeneratorVersion(SearchSuiteGeneratorV1); err != nil {
		t.Fatalf("validate v1: %v", err)
	}
	if err := ValidateSearchSuiteGeneratorVersion(SearchSuiteGeneratorV2); err != nil {
		t.Fatalf("validate v2: %v", err)
	}
	versions := SupportedSearchSuiteGeneratorVersions()
	if !reflect.DeepEqual(versions, []string{SearchSuiteGeneratorV1, SearchSuiteGeneratorV2}) {
		t.Fatalf("supported versions=%v", versions)
	}
}

func TestEverySupportedSearchSuiteGeneratorHasMaterializer(t *testing.T) {
	for _, version := range SupportedSearchSuiteGeneratorVersions() {
		generator, err := lookupSearchSuiteGenerator(version)
		if err != nil {
			t.Fatalf("lookup %q: %v", version, err)
		}
		if generator == nil {
			t.Fatalf("generator %q has no materializer", version)
		}
	}
}

func TestSearchSuiteGeneratorRejectsUnsupportedVersion(t *testing.T) {
	for _, version := range []string{"", "search-suite-generator-v3"} {
		err := ValidateSearchSuiteGeneratorVersion(version)
		if err == nil {
			t.Fatalf("version %q unexpectedly supported", version)
		}
	}
	if err := ValidateSearchSuiteGeneratorVersion("search-suite-generator-v3"); !strings.Contains(err.Error(), `unsupported search suite generator version "search-suite-generator-v3"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestMaterializeGeneratedSearchSuiteCaseDispatchesV1(t *testing.T) {
	seed := int64(7)
	catalog := model.Catalog{Items: map[string]model.Item{
		"source-a": {ID: "source-a", Shape: []model.Coord{{}}, Stars: []model.Star{{TargetTypes: []string{"Food"}}}},
		"source-b": {ID: "source-b", Shape: []model.Coord{{}}, Stars: []model.Star{{TargetTypes: []string{"Food"}}}},
		"food-a":   {ID: "food-a", Shape: []model.Coord{{}}, Types: []string{"Food"}},
		"food-b":   {ID: "food-b", Shape: []model.Coord{{}}, Types: []string{"Food"}},
	}}
	entry := GeneratedSearchSuiteCase{ID: "case", Family: GeneratedFamilySparse, Role: SuiteRoleDevelopment, Seed: &seed}
	got, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV1, catalog, entry)
	if err != nil {
		t.Fatalf("materialize v1: %v", err)
	}
	top := 1
	workers := 1
	noSkips := true
	repair := true
	want := scenario.Scenario{
		Name:              "case",
		Grid:              []string{"111111111", "111111111", "111111111", "111111111", "111111111", "111111111"},
		Items:             map[string]int{"food-a": 1, "food-b": 1, "source-a": 1, "source-b": 1},
		Top:               &top,
		Workers:           &workers,
		NoSkips:           &noSkips,
		RepairSearch:      &repair,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:source-a", "star_source:source-b"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("generated=%#v want=%#v", got, want)
	}
}

func TestMaterializeSearchSuiteCasesRejectsUnsupportedVersion(t *testing.T) {
	_, err := MaterializeSearchSuiteCases("search-suite-generator-v3", model.Catalog{}, SearchSuiteManifest{})
	if err == nil || !strings.Contains(err.Error(), `unsupported search suite generator version "search-suite-generator-v3"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestSearchSuiteGeneratorV1MatchesCommittedLock(t *testing.T) {
	manifestPath := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v1.json")
	lockPath := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v1.lock")
	catalogPath := filepath.Join("..", "..", "data", "catalog.json")

	manifest, err := LoadSearchSuiteManifest(manifestPath)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	lock, err := LoadSearchSuiteLock(lockPath)
	if err != nil {
		t.Fatalf("load lock: %v", err)
	}
	if lock.GeneratorVersion != SearchSuiteGeneratorV1 || len(lock.GeneratedCases) != 4 {
		t.Fatalf("lock generator=%q generated=%d", lock.GeneratorVersion, len(lock.GeneratedCases))
	}
	loadedCatalog, err := catalog.Load(catalogPath)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	entries := make(map[string]GeneratedSearchSuiteCase, len(manifest.Generated))
	for _, entry := range manifest.Generated {
		entries[entry.ID] = entry
	}
	for _, pinned := range lock.GeneratedCases {
		entry, ok := entries[pinned.ID]
		if !ok {
			t.Fatalf("lock references missing manifest case %q", pinned.ID)
		}
		generated, err := MaterializeGeneratedSearchSuiteCase(SearchSuiteGeneratorV1, loadedCatalog, entry)
		if err != nil {
			t.Fatalf("materialize %q: %v", pinned.ID, err)
		}
		content, err := MarshalSearchSuiteScenario(generated)
		if err != nil {
			t.Fatalf("serialize %q: %v", pinned.ID, err)
		}
		hash, err := canonicalJSONSHA256(content)
		if err != nil {
			t.Fatalf("hash %q: %v", pinned.ID, err)
		}
		if hash != pinned.ScenarioSHA256 {
			t.Errorf("%s hash=%s want %s", pinned.ID, hash, pinned.ScenarioSHA256)
		}
	}
}

func TestVerifySearchSuiteLockUsesPinnedGeneratorVersion(t *testing.T) {
	paths := writeSearchSuiteFixture(t)
	lock := readFixtureLock(t, paths.lock)
	lock.GeneratorVersion = "search-suite-generator-does-not-exist"
	writeFixtureLock(t, paths.lock, lock)
	err := VerifySearchSuiteLock(paths.manifest, paths.catalog, paths.lock)
	if err == nil || !strings.Contains(err.Error(), `unsupported search suite generator version "search-suite-generator-does-not-exist"`) {
		t.Fatalf("err=%v", err)
	}
}
