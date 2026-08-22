package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
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
	ID            string `json:"id"`
	Family        string `json:"family"`
	Role          string `json:"role"`
	Seed          *int64 `json:"seed,omitempty"`
	PrivateSeedID string `json:"private_seed_id,omitempty"`
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
	digest := sha256.Sum256(content)
	resolved.ManifestSHA256 = hex.EncodeToString(digest[:])
	root := filepath.Dir(filepath.Dir(filepath.Dir(path)))
	for _, entry := range manifest.Scenarios {
		scenarioContent, err := os.ReadFile(filepath.Join(root, entry.Path))
		if err != nil {
			return ResolvedSearchSuiteManifest{}, fmt.Errorf("%s: %w", entry.ID, err)
		}
		digest := sha256.Sum256(scenarioContent)
		resolved.ScenarioSHA256[entry.ID] = hex.EncodeToString(digest[:])
	}
	return resolved, nil
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
		if entry.Family != GeneratedFamilySparse && entry.Family != GeneratedFamilyDuplicated && entry.Family != GeneratedFamilyLoose {
			return fmt.Errorf("generated case %q has unsupported family %q", entry.ID, entry.Family)
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

// MaterializeGeneratedSearchSuiteCase turns a published seed into a scenario
// without inspecting policy outcomes. The catalog and seed fully determine the
// result; private holdouts intentionally cannot be materialized locally.
func MaterializeGeneratedSearchSuiteCase(catalog model.Catalog, entry GeneratedSearchSuiteCase) (scenario.Scenario, error) {
	if entry.Seed == nil {
		return scenario.Scenario{}, fmt.Errorf("generated case %q has no public seed", entry.ID)
	}
	if entry.Family != GeneratedFamilySparse && entry.Family != GeneratedFamilyDuplicated && entry.Family != GeneratedFamilyLoose {
		return scenario.Scenario{}, fmt.Errorf("generated case %q has unsupported family %q", entry.ID, entry.Family)
	}
	random := rand.New(rand.NewSource(*entry.Seed))
	sources := sortedStarSources(catalog)
	if len(sources) < 2 {
		return scenario.Scenario{}, fmt.Errorf("catalog has fewer than two star sources")
	}
	random.Shuffle(len(sources), func(left, right int) { sources[left], sources[right] = sources[right], sources[left] })
	var sourceIDs []string
	var targetIDs []string
	for left := 0; left < len(sources)-1 && len(sourceIDs) == 0; left++ {
		for right := left + 1; right < len(sources); right++ {
			targets := compatibleTargetIDs(catalog, []string{sources[left], sources[right]})
			if len(targets) < 2 {
				continue
			}
			sourceIDs = []string{sources[left], sources[right]}
			targetIDs = targets
			break
		}
	}
	if len(sourceIDs) != 2 {
		return scenario.Scenario{}, fmt.Errorf("catalog has no compatible two-source family")
	}
	random.Shuffle(len(targetIDs), func(left, right int) { targetIDs[left], targetIDs[right] = targetIDs[right], targetIDs[left] })
	targetCount := 3
	copyCount := 1
	desiredItemCount := 14
	switch entry.Family {
	case GeneratedFamilyDuplicated:
		targetCount = 5
		copyCount = 2
		desiredItemCount = 18
	case GeneratedFamilyLoose:
		targetCount = 8
		desiredItemCount = 18
	}
	if targetCount > len(targetIDs) {
		targetCount = len(targetIDs)
	}
	items := map[string]int{sourceIDs[0]: copyCount, sourceIDs[1]: copyCount}
	for _, targetID := range targetIDs[:targetCount] {
		items[targetID]++
	}
	fillGeneratedSuiteInventory(catalog, random, items, sourceIDs, targetIDs[:targetCount], desiredItemCount)
	top := 1
	workers := 1
	noSkips := true
	repair := true
	generated := scenario.Scenario{
		Name:              entry.ID,
		Grid:              []string{"111111111", "111111111", "111111111", "111111111", "111111111", "111111111"},
		Items:             items,
		Top:               &top,
		Workers:           &workers,
		NoSkips:           &noSkips,
		RepairSearch:      &repair,
		PrioritySemantics: model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:        []string{"star_source:" + sourceIDs[0], "star_source:" + sourceIDs[1]},
	}
	return generated, generated.Validate()
}

func fillGeneratedSuiteInventory(catalog model.Catalog, random *rand.Rand, items map[string]int, sourceIDs []string, targetIDs []string, desiredItemCount int) {
	if desiredItemCount <= generatedSuiteItemCount(items) {
		return
	}
	excluded := make(map[string]struct{}, len(sourceIDs)+len(targetIDs))
	for _, itemID := range sourceIDs {
		excluded[itemID] = struct{}{}
	}
	for _, itemID := range targetIDs {
		excluded[itemID] = struct{}{}
	}
	fillers := make([]string, 0)
	for itemID, item := range catalog.Items {
		if _, excluded := excluded[itemID]; excluded || len(item.Shape) == 0 || len(item.Shape) > 3 {
			continue
		}
		fillers = append(fillers, itemID)
	}
	sort.Strings(fillers)
	random.Shuffle(len(fillers), func(left, right int) { fillers[left], fillers[right] = fillers[right], fillers[left] })
	for _, itemID := range fillers {
		if generatedSuiteItemCount(items) >= desiredItemCount || generatedSuiteArea(catalog, items)+len(catalog.Items[itemID].Shape) > 42 {
			break
		}
		items[itemID]++
	}
}

func generatedSuiteItemCount(items map[string]int) int {
	total := 0
	for _, count := range items {
		total += count
	}
	return total
}

func generatedSuiteArea(catalog model.Catalog, items map[string]int) int {
	total := 0
	for itemID, count := range items {
		total += len(catalog.Items[itemID].Shape) * count
	}
	return total
}

// MaterializeSearchSuiteCases returns only public generated cases for the
// requested roles. Private holdouts are never materialized by this helper.
func MaterializeSearchSuiteCases(catalog model.Catalog, manifest SearchSuiteManifest, roles ...string) ([]scenario.Scenario, error) {
	requested := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		requested[role] = struct{}{}
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
		generated, err := MaterializeGeneratedSearchSuiteCase(catalog, entry)
		if err != nil {
			return nil, err
		}
		result = append(result, generated)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func sortedStarSources(catalog model.Catalog) []string {
	result := make([]string, 0)
	for itemID, item := range catalog.Items {
		if len(item.Stars) > 0 && len(item.Shape) <= 4 {
			result = append(result, itemID)
		}
	}
	sort.Strings(result)
	return result
}

func compatibleTargetIDs(catalog model.Catalog, sourceIDs []string) []string {
	sourceSet := make(map[string]struct{}, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		sourceSet[sourceID] = struct{}{}
	}
	result := make([]string, 0)
	for itemID, item := range catalog.Items {
		if _, source := sourceSet[itemID]; source || len(item.Shape) > 4 {
			continue
		}
		if targetCompatibleWithSources(item, sourceIDs, catalog) {
			result = append(result, itemID)
		}
	}
	sort.Strings(result)
	return result
}

func targetCompatibleWithSources(target model.Item, sourceIDs []string, catalog model.Catalog) bool {
	for _, sourceID := range sourceIDs {
		source := catalog.Items[sourceID]
		for _, star := range source.Stars {
			for _, itemID := range star.TargetItems {
				if itemID == target.ID {
					return true
				}
			}
			for _, targetType := range star.TargetTypes {
				for _, itemType := range target.Types {
					if targetType == itemType {
						return true
					}
				}
			}
		}
	}
	return false
}
