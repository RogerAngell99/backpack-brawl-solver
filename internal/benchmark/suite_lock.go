package benchmark

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/scenario"
)

const (
	SearchSuiteLockVersion      = 1
	SearchSuiteGeneratorVersion = "search-suite-generator-v1"
)

// SearchSuiteLock identifies one exact, public search-suite population.
// Manifest describes the intended population; the lock identifies the exact
// experimental inputs materialized from that intent. Suite role, lock hashes,
// generator versions, and seeds never flow into solver.Config.
type SearchSuiteLock struct {
	LockVersion      int    `json:"lock_version"`
	SuiteName        string `json:"suite_name"`
	ManifestSHA256   string `json:"manifest_sha256"`
	CatalogSHA256    string `json:"catalog_sha256"`
	GeneratorVersion string `json:"generator_version"`

	StaticCases    []SearchSuiteLockedStaticCase    `json:"static_cases"`
	GeneratedCases []SearchSuiteLockedGeneratedCase `json:"generated_cases"`
	PrivateCases   []SearchSuiteLockedPrivateCase   `json:"private_cases"`
}

type SearchSuiteLockedStaticCase struct {
	ID             string `json:"id"`
	Path           string `json:"path"`
	ScenarioSHA256 string `json:"scenario_sha256"`
}

type SearchSuiteLockedGeneratedCase struct {
	ID             string `json:"id"`
	Family         string `json:"family"`
	Role           string `json:"role"`
	Seed           int64  `json:"seed"`
	ScenarioSHA256 string `json:"scenario_sha256"`
}

type SearchSuiteLockedPrivateCase struct {
	ID            string `json:"id"`
	PrivateSeedID string `json:"private_seed_id"`
}

func LoadSearchSuiteLock(path string) (SearchSuiteLock, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SearchSuiteLock{}, err
	}
	var lock SearchSuiteLock
	if err := json.Unmarshal(content, &lock); err != nil {
		return SearchSuiteLock{}, err
	}
	if err := lock.Validate(); err != nil {
		return SearchSuiteLock{}, err
	}
	return lock, nil
}

func (lock SearchSuiteLock) Validate() error {
	if lock.LockVersion != SearchSuiteLockVersion {
		return fmt.Errorf("unsupported search suite lock version %d", lock.LockVersion)
	}
	if lock.SuiteName == "" {
		return fmt.Errorf("search suite lock requires suite_name")
	}
	if err := validateSHA256("manifest_sha256", lock.ManifestSHA256); err != nil {
		return err
	}
	if err := validateSHA256("catalog_sha256", lock.CatalogSHA256); err != nil {
		return err
	}
	if lock.GeneratorVersion == "" {
		return fmt.Errorf("search suite lock requires generator_version")
	}
	if err := validateStaticLockCases(lock.StaticCases); err != nil {
		return err
	}
	if err := validateGeneratedLockCases(lock.GeneratedCases); err != nil {
		return err
	}
	if err := validatePrivateLockCases(lock.PrivateCases); err != nil {
		return err
	}
	return validateDistinctLockCaseIDs(lock)
}

func validateDistinctLockCaseIDs(lock SearchSuiteLock) error {
	seen := map[string]string{}
	groups := []struct {
		kind string
		ids  []string
	}{
		{kind: "static", ids: staticCaseIDs(lock.StaticCases)},
		{kind: "generated", ids: generatedCaseIDs(lock.GeneratedCases)},
		{kind: "private", ids: privateCaseIDs(lock.PrivateCases)},
	}
	for _, group := range groups {
		for _, id := range group.ids {
			if previous, exists := seen[id]; exists {
				return fmt.Errorf("locked %s case ID %q duplicates a %s case", group.kind, id, previous)
			}
			seen[id] = group.kind
		}
	}
	return nil
}

func validateSHA256(field string, value string) error {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return fmt.Errorf("search suite lock %s must be a lowercase SHA-256", field)
	}
	if _, err := hex.DecodeString(value); err != nil {
		return fmt.Errorf("search suite lock %s must be a lowercase SHA-256: %w", field, err)
	}
	return nil
}

func validateStaticLockCases(cases []SearchSuiteLockedStaticCase) error {
	ids := make([]string, 0, len(cases))
	for _, entry := range cases {
		if entry.ID == "" || entry.Path == "" {
			return fmt.Errorf("locked static case requires id and path")
		}
		if err := validateSHA256(fmt.Sprintf("static case %q scenario_sha256", entry.ID), entry.ScenarioSHA256); err != nil {
			return err
		}
		ids = append(ids, entry.ID)
	}
	return validateSortedUniqueLockIDs("static", ids)
}

func validateGeneratedLockCases(cases []SearchSuiteLockedGeneratedCase) error {
	ids := make([]string, 0, len(cases))
	for _, entry := range cases {
		if entry.ID == "" || entry.Family == "" || entry.Role == "" {
			return fmt.Errorf("locked generated case requires id, family, and role")
		}
		if entry.Role == SuiteRolePrivateHoldout {
			return fmt.Errorf("locked generated case %q must not be a private holdout", entry.ID)
		}
		if err := validateSHA256(fmt.Sprintf("generated case %q scenario_sha256", entry.ID), entry.ScenarioSHA256); err != nil {
			return err
		}
		ids = append(ids, entry.ID)
	}
	return validateSortedUniqueLockIDs("generated", ids)
}

func validatePrivateLockCases(cases []SearchSuiteLockedPrivateCase) error {
	ids := make([]string, 0, len(cases))
	for _, entry := range cases {
		if entry.ID == "" || entry.PrivateSeedID == "" {
			return fmt.Errorf("locked private case requires id and private_seed_id")
		}
		ids = append(ids, entry.ID)
	}
	return validateSortedUniqueLockIDs("private", ids)
}

func validateSortedUniqueLockIDs(kind string, ids []string) error {
	for index, id := range ids {
		if index > 0 {
			if ids[index-1] == id {
				return fmt.Errorf("duplicate locked %s case ID %q", kind, id)
			}
			if ids[index-1] > id {
				return fmt.Errorf("locked %s cases must be sorted by ID", kind)
			}
		}
	}
	return nil
}

// ObserveSearchSuite materializes the public suite population without writing
// a lock. All *_sha256 fields are SHA-256 hashes of canonical JSON: decode
// with UseNumber, require one complete JSON value, marshal it, then hash it.
func ObserveSearchSuite(manifestPath string, catalogPath string) (SearchSuiteLock, error) {
	manifestContent, err := os.ReadFile(manifestPath)
	if err != nil {
		return SearchSuiteLock{}, err
	}
	manifest, err := LoadSearchSuiteManifest(manifestPath)
	if err != nil {
		return SearchSuiteLock{}, err
	}
	manifestSHA256, err := canonicalJSONSHA256(manifestContent)
	if err != nil {
		return SearchSuiteLock{}, fmt.Errorf("canonicalize manifest: %w", err)
	}

	catalogContent, err := os.ReadFile(catalogPath)
	if err != nil {
		return SearchSuiteLock{}, err
	}
	catalogSHA256, err := canonicalJSONSHA256(catalogContent)
	if err != nil {
		return SearchSuiteLock{}, fmt.Errorf("canonicalize catalog: %w", err)
	}
	loadedCatalog, err := catalog.Load(catalogPath)
	if err != nil {
		return SearchSuiteLock{}, err
	}

	lock := SearchSuiteLock{
		LockVersion:      SearchSuiteLockVersion,
		SuiteName:        manifest.Name,
		ManifestSHA256:   manifestSHA256,
		CatalogSHA256:    catalogSHA256,
		GeneratorVersion: SearchSuiteGeneratorVersion,
		StaticCases:      make([]SearchSuiteLockedStaticCase, 0, len(manifest.Scenarios)),
		GeneratedCases:   make([]SearchSuiteLockedGeneratedCase, 0, len(manifest.Generated)),
		PrivateCases:     make([]SearchSuiteLockedPrivateCase, 0),
	}
	root := searchSuiteRoot(manifestPath)
	for _, entry := range manifest.Scenarios {
		content, err := os.ReadFile(filepath.Join(root, entry.Path))
		if err != nil {
			return SearchSuiteLock{}, fmt.Errorf("static case %q: %w", entry.ID, err)
		}
		scenarioSHA256, err := canonicalJSONSHA256(content)
		if err != nil {
			return SearchSuiteLock{}, fmt.Errorf("static case %q: canonicalize scenario: %w", entry.ID, err)
		}
		lock.StaticCases = append(lock.StaticCases, SearchSuiteLockedStaticCase{
			ID:             entry.ID,
			Path:           entry.Path,
			ScenarioSHA256: scenarioSHA256,
		})
	}
	for _, entry := range manifest.Generated {
		if entry.Role == SuiteRolePrivateHoldout {
			lock.PrivateCases = append(lock.PrivateCases, SearchSuiteLockedPrivateCase{
				ID:            entry.ID,
				PrivateSeedID: entry.PrivateSeedID,
			})
			continue
		}
		generated, err := MaterializeGeneratedSearchSuiteCase(loadedCatalog, entry)
		if err != nil {
			return SearchSuiteLock{}, fmt.Errorf("generated case %q: %w", entry.ID, err)
		}
		content, err := MarshalSearchSuiteScenario(generated)
		if err != nil {
			return SearchSuiteLock{}, fmt.Errorf("generated case %q: serialize scenario: %w", entry.ID, err)
		}
		scenarioSHA256, err := canonicalJSONSHA256(content)
		if err != nil {
			return SearchSuiteLock{}, fmt.Errorf("generated case %q: canonicalize scenario: %w", entry.ID, err)
		}
		lock.GeneratedCases = append(lock.GeneratedCases, SearchSuiteLockedGeneratedCase{
			ID:             entry.ID,
			Family:         entry.Family,
			Role:           entry.Role,
			Seed:           *entry.Seed,
			ScenarioSHA256: scenarioSHA256,
		})
	}
	sort.Slice(lock.StaticCases, func(left, right int) bool { return lock.StaticCases[left].ID < lock.StaticCases[right].ID })
	sort.Slice(lock.GeneratedCases, func(left, right int) bool { return lock.GeneratedCases[left].ID < lock.GeneratedCases[right].ID })
	sort.Slice(lock.PrivateCases, func(left, right int) bool { return lock.PrivateCases[left].ID < lock.PrivateCases[right].ID })
	if err := lock.Validate(); err != nil {
		return SearchSuiteLock{}, err
	}
	return lock, nil
}

// WriteSearchSuiteLock creates a lock exactly once. Existing locks are never
// overwritten: changing a frozen corpus requires a new suite name and lock.
func WriteSearchSuiteLock(path string, lock SearchSuiteLock) error {
	if err := lock.Validate(); err != nil {
		return err
	}
	content, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("refusing to overwrite existing search suite lock %q; create a new suite instead", path)
		}
		return err
	}
	if _, err := file.Write(append(content, '\n')); err != nil {
		closeErr := file.Close()
		if closeErr != nil {
			return fmt.Errorf("write lock: %w; close lock: %v", err, closeErr)
		}
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

// VerifySearchSuiteLock verifies both the suite structure and every pinned
// input hash before a caller consumes the suite.
func VerifySearchSuiteLock(manifestPath string, catalogPath string, lockPath string) error {
	expected, err := LoadSearchSuiteLock(lockPath)
	if err != nil {
		return fmt.Errorf("load search suite lock: %w", err)
	}
	observed, err := ObserveSearchSuite(manifestPath, catalogPath)
	if err != nil {
		return fmt.Errorf("observe search suite: %w", err)
	}
	if err := verifySearchSuiteLockStructure(expected, observed); err != nil {
		return err
	}
	if expected.ManifestSHA256 != observed.ManifestSHA256 {
		return sha256MismatchError("manifest", expected.ManifestSHA256, observed.ManifestSHA256)
	}
	if expected.CatalogSHA256 != observed.CatalogSHA256 {
		return sha256MismatchError("catalog", expected.CatalogSHA256, observed.CatalogSHA256)
	}
	if err := verifyStaticCaseHashes(expected.StaticCases, observed.StaticCases); err != nil {
		return err
	}
	if err := verifyGeneratedCaseHashes(expected.GeneratorVersion, expected.GeneratedCases, observed.GeneratedCases); err != nil {
		return err
	}
	return nil
}

func verifySearchSuiteLockStructure(expected SearchSuiteLock, observed SearchSuiteLock) error {
	if expected.LockVersion != SearchSuiteLockVersion {
		return fmt.Errorf("unsupported search suite lock version %d", expected.LockVersion)
	}
	if expected.SuiteName != observed.SuiteName {
		return fmt.Errorf("suite name mismatch:\n  expected: %s\n  actual:   %s", expected.SuiteName, observed.SuiteName)
	}
	if expected.GeneratorVersion != observed.GeneratorVersion {
		return fmt.Errorf("generator version mismatch:\n  expected: %s\n  actual:   %s", expected.GeneratorVersion, observed.GeneratorVersion)
	}
	if err := verifyStaticCaseStructure(expected.StaticCases, observed.StaticCases); err != nil {
		return err
	}
	if err := verifyGeneratedCaseStructure(expected.GeneratedCases, observed.GeneratedCases); err != nil {
		return err
	}
	return verifyPrivateCaseStructure(expected.PrivateCases, observed.PrivateCases)
}

func verifyStaticCaseStructure(expected []SearchSuiteLockedStaticCase, observed []SearchSuiteLockedStaticCase) error {
	if err := verifyCaseIDs("static", staticCaseIDs(expected), staticCaseIDs(observed)); err != nil {
		return err
	}
	actualByID := make(map[string]SearchSuiteLockedStaticCase, len(observed))
	for _, entry := range observed {
		actualByID[entry.ID] = entry
	}
	for _, entry := range expected {
		actual := actualByID[entry.ID]
		if entry.Path != actual.Path {
			return fmt.Errorf("static case %q path mismatch:\n  expected: %s\n  actual:   %s", entry.ID, entry.Path, actual.Path)
		}
	}
	return nil
}

func verifyGeneratedCaseStructure(expected []SearchSuiteLockedGeneratedCase, observed []SearchSuiteLockedGeneratedCase) error {
	if err := verifyCaseIDs("generated", generatedCaseIDs(expected), generatedCaseIDs(observed)); err != nil {
		return err
	}
	actualByID := make(map[string]SearchSuiteLockedGeneratedCase, len(observed))
	for _, entry := range observed {
		actualByID[entry.ID] = entry
	}
	for _, entry := range expected {
		actual := actualByID[entry.ID]
		if entry.Family != actual.Family {
			return fmt.Errorf("generated case %q family mismatch:\n  expected: %s\n  actual:   %s", entry.ID, entry.Family, actual.Family)
		}
		if entry.Role != actual.Role {
			return fmt.Errorf("generated case %q role mismatch:\n  expected: %s\n  actual:   %s", entry.ID, entry.Role, actual.Role)
		}
		if entry.Seed != actual.Seed {
			return fmt.Errorf("generated case %q seed mismatch:\n  expected: %d\n  actual:   %d", entry.ID, entry.Seed, actual.Seed)
		}
	}
	return nil
}

func verifyPrivateCaseStructure(expected []SearchSuiteLockedPrivateCase, observed []SearchSuiteLockedPrivateCase) error {
	if err := verifyCaseIDs("private", privateCaseIDs(expected), privateCaseIDs(observed)); err != nil {
		return err
	}
	actualByID := make(map[string]SearchSuiteLockedPrivateCase, len(observed))
	for _, entry := range observed {
		actualByID[entry.ID] = entry
	}
	for _, entry := range expected {
		actual := actualByID[entry.ID]
		if entry.PrivateSeedID != actual.PrivateSeedID {
			return fmt.Errorf("private case %q private seed ID mismatch:\n  expected: %s\n  actual:   %s", entry.ID, entry.PrivateSeedID, actual.PrivateSeedID)
		}
	}
	return nil
}

func verifyCaseIDs(kind string, expected []string, observed []string) error {
	expectedSet := make(map[string]struct{}, len(expected))
	for _, id := range expected {
		expectedSet[id] = struct{}{}
	}
	observedSet := make(map[string]struct{}, len(observed))
	for _, id := range observed {
		observedSet[id] = struct{}{}
	}
	for _, id := range expected {
		if _, ok := observedSet[id]; !ok {
			return fmt.Errorf("%s case %q is missing from the manifest", kind, id)
		}
	}
	for _, id := range observed {
		if _, ok := expectedSet[id]; !ok {
			return fmt.Errorf("unexpected %s case %q in the manifest", kind, id)
		}
	}
	return nil
}

func verifyStaticCaseHashes(expected []SearchSuiteLockedStaticCase, observed []SearchSuiteLockedStaticCase) error {
	actualByID := make(map[string]SearchSuiteLockedStaticCase, len(observed))
	for _, entry := range observed {
		actualByID[entry.ID] = entry
	}
	for _, entry := range expected {
		actual := actualByID[entry.ID]
		if entry.ScenarioSHA256 != actual.ScenarioSHA256 {
			return fmt.Errorf("static case %q changed:\n  path: %s\n  expected scenario SHA-256: %s\n  actual scenario SHA-256:   %s", entry.ID, entry.Path, entry.ScenarioSHA256, actual.ScenarioSHA256)
		}
	}
	return nil
}

func verifyGeneratedCaseHashes(generatorVersion string, expected []SearchSuiteLockedGeneratedCase, observed []SearchSuiteLockedGeneratedCase) error {
	actualByID := make(map[string]SearchSuiteLockedGeneratedCase, len(observed))
	for _, entry := range observed {
		actualByID[entry.ID] = entry
	}
	for _, entry := range expected {
		actual := actualByID[entry.ID]
		if entry.ScenarioSHA256 != actual.ScenarioSHA256 {
			return fmt.Errorf("generated case %q changed:\n  generator: %s\n  seed: %d\n  expected scenario SHA-256: %s\n  actual scenario SHA-256:   %s", entry.ID, generatorVersion, entry.Seed, entry.ScenarioSHA256, actual.ScenarioSHA256)
		}
	}
	return nil
}

func sha256MismatchError(subject string, expected string, actual string) error {
	return fmt.Errorf("%s SHA-256 mismatch:\n  expected: %s\n  actual:   %s", subject, expected, actual)
}

func staticCaseIDs(cases []SearchSuiteLockedStaticCase) []string {
	ids := make([]string, len(cases))
	for index, entry := range cases {
		ids[index] = entry.ID
	}
	return ids
}

func generatedCaseIDs(cases []SearchSuiteLockedGeneratedCase) []string {
	ids := make([]string, len(cases))
	for index, entry := range cases {
		ids[index] = entry.ID
	}
	return ids
}

func privateCaseIDs(cases []SearchSuiteLockedPrivateCase) []string {
	ids := make([]string, len(cases))
	for index, entry := range cases {
		ids[index] = entry.ID
	}
	return ids
}

// MarshalSearchSuiteScenario is shared by lock observation and public
// materialization so a scenario is hashed from the same JSON semantics written
// to disk.
func MarshalSearchSuiteScenario(value scenario.Scenario) ([]byte, error) {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}

func canonicalJSONSHA256(content []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return "", fmt.Errorf("unexpected trailing JSON value")
		}
		return "", fmt.Errorf("trailing JSON: %w", err)
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}
