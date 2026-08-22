package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/catalog"
)

func TestCanonicalJSONSHA256IgnoresFormattingAndObjectOrder(t *testing.T) {
	first, err := canonicalJSONSHA256([]byte("{\n  \"alpha\": [1, 2], \"beta\": {\"x\": true}\n}"))
	if err != nil {
		t.Fatalf("hash first JSON: %v", err)
	}
	second, err := canonicalJSONSHA256([]byte("{\"beta\":{\"x\":true},\"alpha\":[1,2]}"))
	if err != nil {
		t.Fatalf("hash second JSON: %v", err)
	}
	if first != second {
		t.Fatalf("canonical hashes differ: %s != %s", first, second)
	}
}

func TestCanonicalJSONSHA256RejectsTrailingJSON(t *testing.T) {
	if _, err := canonicalJSONSHA256([]byte("{} {}")); err == nil || !strings.Contains(err.Error(), "trailing") {
		t.Fatalf("err=%v want trailing JSON error", err)
	}
}

func TestObserveSearchSuiteIsStableAcrossFormatting(t *testing.T) {
	paths := writeSearchSuiteFixture(t)
	first, err := ObserveSearchSuite(paths.manifest, paths.catalog)
	if err != nil {
		t.Fatalf("observe suite first: %v", err)
	}
	if err := os.WriteFile(paths.manifest, []byte(`{
  "generated": [
    {
      "seed": 7,
      "role": "development",
      "family": "star-source-sparse",
      "id": "generated"
    },
    {
      "private_seed_id": "fixture-private-01",
      "family": "private",
      "id": "private",
      "role": "private_holdout"
    }
  ],
  "scenarios": [
    {
      "tags": ["fixture"],
      "path": "scenarios/static.json",
      "role": "development",
      "id": "static"
    }
  ],
  "baseline_policy": "v4",
  "workers": 1,
  "budgets": [100],
  "name": "fixture-suite",
  "version": 1
}
`), 0o600); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}
	second, err := ObserveSearchSuite(paths.manifest, paths.catalog)
	if err != nil {
		t.Fatalf("observe suite second: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("observed locks differ:\nfirst=%+v\nsecond=%+v", first, second)
	}
}

func TestWriteSearchSuiteLockRefusesOverwrite(t *testing.T) {
	paths := writeSearchSuiteFixture(t)
	lock, err := ObserveSearchSuite(paths.manifest, paths.catalog)
	if err != nil {
		t.Fatalf("observe suite: %v", err)
	}
	if err := WriteSearchSuiteLock(paths.lock, lock); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("overwrite err=%v", err)
	}
	before, err := os.ReadFile(paths.lock)
	if err != nil {
		t.Fatalf("read lock: %v", err)
	}
	if err := WriteSearchSuiteLock(paths.lock, lock); err == nil || !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Fatalf("overwrite err=%v", err)
	}
	after, err := os.ReadFile(paths.lock)
	if err != nil {
		t.Fatalf("read lock after overwrite: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("existing lock changed after refused overwrite")
	}
}

func TestVerifySearchSuiteLockDetectsDrift(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, paths searchSuiteFixturePaths)
		message string
	}{
		{
			name: "manifest content",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				writeFixtureManifest(t, paths.manifest, 7, GeneratedFamilySparse, true)
			},
			message: "manifest SHA-256 mismatch",
		},
		{
			name: "catalog content",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				writeFile(t, paths.catalog, strings.Replace(fixtureCatalog, "\"food-a\"", "\"food-a-renamed\"", 1))
			},
			message: "catalog SHA-256 mismatch",
		},
		{
			name: "static scenario",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				writeFile(t, paths.static, strings.Replace(fixtureStaticScenario, "\"static\"", "\"changed\"", 1))
			},
			message: "static case \"static\" changed",
		},
		{
			name: "generated seed",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				writeFixtureManifest(t, paths.manifest, 11, GeneratedFamilySparse, false)
			},
			message: "generated case \"generated\" seed mismatch",
		},
		{
			name: "generated family",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				writeFixtureManifest(t, paths.manifest, 7, GeneratedFamilyLoose, false)
			},
			message: "generated case \"generated\" family mismatch",
		},
		{
			name: "generated role",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				content, err := os.ReadFile(paths.manifest)
				if err != nil {
					t.Fatalf("read manifest: %v", err)
				}
				writeFile(t, paths.manifest, strings.Replace(string(content), "\"role\": \"development\",\n      \"seed\": 7", "\"role\": \"validation\",\n      \"seed\": 7", 1))
			},
			message: "generated case \"generated\" role mismatch",
		},
		{
			name: "private declaration",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				content, err := os.ReadFile(paths.manifest)
				if err != nil {
					t.Fatalf("read manifest: %v", err)
				}
				writeFile(t, paths.manifest, strings.Replace(string(content), "fixture-private-01", "fixture-private-02", 1))
			},
			message: "private case \"private\" private seed ID mismatch",
		},
		{
			name: "generator version",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				lock := readFixtureLock(t, paths.lock)
				lock.GeneratorVersion = "search-suite-generator-v2"
				writeFixtureLock(t, paths.lock, lock)
			},
			message: "generator version mismatch",
		},
		{
			name: "generated scenario hash",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				lock := readFixtureLock(t, paths.lock)
				lock.GeneratedCases[0].ScenarioSHA256 = strings.Repeat("0", 64)
				writeFixtureLock(t, paths.lock, lock)
			},
			message: "generated case \"generated\" changed",
		},
		{
			name: "missing static case",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				writeFile(t, paths.manifest, `{
  "version": 1,
  "name": "fixture-suite",
  "budgets": [100],
  "workers": 1,
  "baseline_policy": "v4",
  "scenarios": [],
  "generated": [
    {"id": "generated", "family": "star-source-sparse", "role": "development", "seed": 7},
    {"id": "private", "family": "private", "role": "private_holdout", "private_seed_id": "fixture-private-01"}
  ]
}`)
			},
			message: "static case \"static\" is missing",
		},
		{
			name: "missing static scenario file",
			mutate: func(t *testing.T, paths searchSuiteFixturePaths) {
				t.Helper()
				if err := os.Remove(paths.static); err != nil {
					t.Fatalf("remove static scenario: %v", err)
				}
			},
			message: "static case \"static\"",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			paths := writeSearchSuiteFixture(t)
			test.mutate(t, paths)
			err := VerifySearchSuiteLock(paths.manifest, paths.catalog, paths.lock)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("verify err=%v want %q", err, test.message)
			}
		})
	}
}

func TestPublicGeneratedScenarioMatchesPinnedHash(t *testing.T) {
	paths := writeSearchSuiteFixture(t)
	manifest, err := LoadSearchSuiteManifest(paths.manifest)
	if err != nil {
		t.Fatalf("load manifest: %v", err)
	}
	loadedCatalog, err := catalog.Load(paths.catalog)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	generated, err := MaterializeSearchSuiteCases(loadedCatalog, manifest, SuiteRoleDevelopment)
	if err != nil {
		t.Fatalf("materialize suite: %v", err)
	}
	if len(generated) != 1 {
		t.Fatalf("generated=%d want 1", len(generated))
	}
	content, err := MarshalSearchSuiteScenario(generated[0])
	if err != nil {
		t.Fatalf("serialize generated scenario: %v", err)
	}
	hash, err := canonicalJSONSHA256(content)
	if err != nil {
		t.Fatalf("hash generated scenario: %v", err)
	}
	lock := readFixtureLock(t, paths.lock)
	if lock.GeneratedCases[0].ScenarioSHA256 != hash {
		t.Fatalf("generated hash=%s lock hash=%s", hash, lock.GeneratedCases[0].ScenarioSHA256)
	}
	encoded, err := json.Marshal(lock.PrivateCases[0])
	if err != nil {
		t.Fatalf("marshal private case: %v", err)
	}
	if strings.Contains(string(encoded), "seed\":") {
		t.Fatalf("private case leaks a seed: %s", encoded)
	}
}

func TestVerifyCommittedGeneralSearchV1Lock(t *testing.T) {
	manifest := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v1.json")
	lock := filepath.Join("..", "..", "benchmarks", "suites", "general-search-v1.lock")
	catalogPath := filepath.Join("..", "..", "data", "catalog.json")
	if err := VerifySearchSuiteLock(manifest, catalogPath, lock); err != nil {
		t.Fatalf("verify committed lock: %v", err)
	}
}

type searchSuiteFixturePaths struct {
	manifest string
	catalog  string
	static   string
	lock     string
}

func writeSearchSuiteFixture(t *testing.T) searchSuiteFixturePaths {
	t.Helper()
	root := t.TempDir()
	paths := searchSuiteFixturePaths{
		manifest: filepath.Join(root, "benchmarks", "suites", "fixture.json"),
		catalog:  filepath.Join(root, "data", "catalog.json"),
		static:   filepath.Join(root, "scenarios", "static.json"),
		lock:     filepath.Join(root, "benchmarks", "suites", "fixture.lock"),
	}
	writeFile(t, paths.catalog, fixtureCatalog)
	writeFile(t, paths.static, fixtureStaticScenario)
	writeFixtureManifest(t, paths.manifest, 7, GeneratedFamilySparse, false)
	lock, err := ObserveSearchSuite(paths.manifest, paths.catalog)
	if err != nil {
		t.Fatalf("observe fixture suite: %v", err)
	}
	if err := WriteSearchSuiteLock(paths.lock, lock); err != nil {
		t.Fatalf("write fixture lock: %v", err)
	}
	return paths
}

func writeFixtureManifest(t *testing.T, path string, seed int64, family string, changedBudget bool) {
	t.Helper()
	budget := 100
	if changedBudget {
		budget = 101
	}
	writeFile(t, path, `{
  "version": 1,
  "name": "fixture-suite",
  "budgets": [`+strconv.Itoa(budget)+`],
  "workers": 1,
  "baseline_policy": "v4",
  "scenarios": [
    {
      "id": "static",
      "path": "scenarios/static.json",
      "role": "development",
      "tags": ["fixture"]
    }
  ],
  "generated": [
    {
      "id": "generated",
      "family": "`+family+`",
      "role": "development",
      "seed": `+strconv.FormatInt(seed, 10)+`
    },
    {
      "id": "private",
      "family": "private",
      "role": "private_holdout",
      "private_seed_id": "fixture-private-01"
    }
  ]
}`)
}

func readFixtureLock(t *testing.T, path string) SearchSuiteLock {
	t.Helper()
	lock, err := LoadSearchSuiteLock(path)
	if err != nil {
		t.Fatalf("load fixture lock: %v", err)
	}
	return lock
}

func writeFixtureLock(t *testing.T, path string, lock SearchSuiteLock) {
	t.Helper()
	content, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		t.Fatalf("marshal fixture lock: %v", err)
	}
	writeFile(t, path, string(append(content, '\n')))
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

const fixtureStaticScenario = `{
  "name": "static",
  "grid": ["111111111", "111111111", "111111111", "111111111", "111111111", "111111111"],
  "items": {"food-a": 1}
}`

const fixtureCatalog = `{
  "items": [
    {
      "id": "source-a",
      "shape": [[0, 0]],
      "stars": [{"offset": [0, 0], "target_types": ["Food"]}]
    },
    {
      "id": "source-b",
      "shape": [[0, 0]],
      "stars": [{"offset": [0, 0], "target_types": ["Food"]}]
    },
    {"id": "food-a", "shape": [[0, 0]], "types": ["Food"]},
    {"id": "food-b", "shape": [[0, 0]], "types": ["Food"]},
    {"id": "food-c", "shape": [[0, 0]], "types": ["Food"]}
  ],
  "recipes": []
}`
