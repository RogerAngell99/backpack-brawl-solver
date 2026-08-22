package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

const (
	SuiteRoleDevelopment           = "development"
	SuiteRoleExperimentDevelopment = "experiment_development"
	SuiteRoleValidation            = "validation"
	SuiteRolePublicHoldout         = "public_holdout"
	SuiteRolePrivateHoldout        = "private_holdout"

	GeneratedFamilySparse     = "star-source-sparse"
	GeneratedFamilyDuplicated = "star-source-duplicated"
	GeneratedFamilyLoose      = "star-source-loose"
	GeneratedFamilyPrivate    = "private"
)

// SearchSuiteManifest describes an immutable experiment population. Scenario
// files and generated seeds are intentionally separate from Scenario so the
// solver objective cannot inspect a case's suite role or structural tags.
type SearchSuiteManifest struct {
	Version        int                        `json:"version"`
	Name           string                     `json:"name"`
	Budgets        []int64                    `json:"budgets"`
	Workers        int                        `json:"workers"`
	BaselinePolicy string                     `json:"baseline_policy"`
	Scenarios      []SearchSuiteScenario      `json:"scenarios"`
	Generated      []GeneratedSearchSuiteCase `json:"generated"`
}

type SearchSuiteScenario struct {
	ID   string   `json:"id"`
	Path string   `json:"path"`
	Role string   `json:"role"`
	Tags []string `json:"tags"`
}

type GeneratedSearchSuiteCase struct {
	ID                    string `json:"id"`
	Family                string `json:"family"`
	Role                  string `json:"role"`
	Seed                  *int64 `json:"seed,omitempty"`
	PrivateSeedID         string `json:"private_seed_id,omitempty"`
	PrivateSeedCommitment string `json:"private_seed_commitment,omitempty"`

	StructuralDescriptor *GeneratedSearchSuiteStructuralDescriptor `json:"structural_descriptor,omitempty"`
}

type ResolvedSearchSuiteManifest struct {
	Manifest       SearchSuiteManifest `json:"manifest"`
	ManifestSHA256 string              `json:"manifest_sha256"`
	ScenarioSHA256 map[string]string   `json:"scenario_sha256"`
}

func LoadSearchSuiteManifest(path string) (SearchSuiteManifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return SearchSuiteManifest{}, err
	}
	var manifest SearchSuiteManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return SearchSuiteManifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return SearchSuiteManifest{}, err
	}
	return manifest, nil
}

func ResolveSearchSuiteManifest(path string) (ResolvedSearchSuiteManifest, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return ResolvedSearchSuiteManifest{}, err
	}
	manifest, err := LoadSearchSuiteManifest(path)
	if err != nil {
		return ResolvedSearchSuiteManifest{}, err
	}
	resolved := ResolvedSearchSuiteManifest{
		Manifest:       manifest,
		ScenarioSHA256: make(map[string]string, len(manifest.Scenarios)),
	}
	resolved.ManifestSHA256, err = canonicalJSONSHA256(content)
	if err != nil {
		return ResolvedSearchSuiteManifest{}, fmt.Errorf("canonicalize manifest: %w", err)
	}
	root := searchSuiteRoot(path)
	for _, entry := range manifest.Scenarios {
		scenarioContent, err := os.ReadFile(filepath.Join(root, entry.Path))
		if err != nil {
			return ResolvedSearchSuiteManifest{}, fmt.Errorf("%s: %w", entry.ID, err)
		}
		resolved.ScenarioSHA256[entry.ID], err = canonicalJSONSHA256(scenarioContent)
		if err != nil {
			return ResolvedSearchSuiteManifest{}, fmt.Errorf("%s: canonicalize scenario: %w", entry.ID, err)
		}
	}
	return resolved, nil
}

func searchSuiteRoot(manifestPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(manifestPath)))
}

func (manifest SearchSuiteManifest) Validate() error {
	if manifest.Version != 1 || manifest.Name == "" || manifest.Workers <= 0 || manifest.BaselinePolicy == "" {
		return fmt.Errorf("suite manifest requires version 1, name, workers, and baseline policy")
	}
	if len(manifest.Budgets) == 0 {
		return fmt.Errorf("suite manifest requires budgets")
	}
	for index, budget := range manifest.Budgets {
		if budget <= 0 || (index > 0 && budget <= manifest.Budgets[index-1]) {
			return fmt.Errorf("suite budgets must be strictly increasing positive values")
		}
	}
	seen := map[string]struct{}{}
	for _, entry := range manifest.Scenarios {
		if err := validateSearchSuiteCase(entry.ID, entry.Role, seen); err != nil {
			return err
		}
		if entry.Path == "" || len(entry.Tags) == 0 {
			return fmt.Errorf("scenario %q requires path and tags", entry.ID)
		}
	}
	for _, entry := range manifest.Generated {
		if err := validateSearchSuiteCase(entry.ID, entry.Role, seen); err != nil {
			return err
		}
		if entry.Family == "" {
			return fmt.Errorf("generated case %q requires family", entry.ID)
		}
		if entry.Role == SuiteRolePrivateHoldout {
			if entry.PrivateSeedID == "" || entry.Seed != nil {
				return fmt.Errorf("private holdout %q requires only a private seed ID", entry.ID)
			}
			continue
		}
		if entry.Seed == nil || entry.PrivateSeedID != "" {
			return fmt.Errorf("generated case %q requires a public seed", entry.ID)
		}
	}
	return nil
}

func validateSearchSuiteCase(id string, role string, seen map[string]struct{}) error {
	if id == "" {
		return fmt.Errorf("suite case requires ID")
	}
	if _, exists := seen[id]; exists {
		return fmt.Errorf("duplicate suite case ID %q", id)
	}
	seen[id] = struct{}{}
	switch role {
	case SuiteRoleDevelopment, SuiteRoleExperimentDevelopment, SuiteRoleValidation, SuiteRolePublicHoldout, SuiteRolePrivateHoldout:
		return nil
	default:
		return fmt.Errorf("suite case %q has unsupported role %q", id, role)
	}
}

// MaterializeSearchSuiteCases returns only public generated cases for the
// requested roles. Private holdouts are never materialized by this helper.
func MaterializeSearchSuiteCases(generatorVersion string, catalog model.Catalog, manifest SearchSuiteManifest, roles ...string) ([]scenario.Scenario, error) {
	if err := ValidateSearchSuiteGeneratorVersion(generatorVersion); err != nil {
		return nil, err
	}
	requested := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		requested[role] = struct{}{}
	}
	if _, requestedPrivate := requested[SuiteRolePrivateHoldout]; requestedPrivate {
		privateIDs := make([]string, 0)
		for _, entry := range manifest.Generated {
			if entry.Role == SuiteRolePrivateHoldout {
				privateIDs = append(privateIDs, entry.ID)
			}
		}
		sort.Strings(privateIDs)
		if len(privateIDs) > 0 {
			return nil, fmt.Errorf("private holdout %q cannot be materialized by the public suite materializer", privateIDs[0])
		}
	}
	result := make([]scenario.Scenario, 0, len(manifest.Generated))
	for _, entry := range manifest.Generated {
		if entry.Role == SuiteRolePrivateHoldout {
			continue
		}
		if len(requested) > 0 {
			if _, ok := requested[entry.Role]; !ok {
				continue
			}
		}
		generated, err := MaterializeGeneratedSearchSuiteCase(generatorVersion, catalog, entry)
		if err != nil {
			return nil, err
		}
		result = append(result, generated)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}
