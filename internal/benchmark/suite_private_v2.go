package benchmark

import (
	"fmt"
	"sort"

	"backpack-brawl-solver/internal/model"
)

// VerifySearchSuiteV2PrivateHoldouts validates a protected private-seed map
// against the public commitments, then exercises the normal v2 structural
// generator and packing witness. It intentionally returns no scenarios or
// seed values, so callers cannot accidentally publish the holdout corpus.
func VerifySearchSuiteV2PrivateHoldouts(catalog model.Catalog, manifest SearchSuiteManifest, privateSeeds map[string]int64) error {
	if err := ValidateSearchSuiteManifestForGenerator(SearchSuiteGeneratorV2, manifest); err != nil {
		return err
	}
	entries := make([]GeneratedSearchSuiteCase, 0)
	for _, entry := range manifest.Generated {
		if entry.Role == SuiteRolePrivateHoldout {
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].ID < entries[right].ID })
	if len(entries) == 0 {
		return fmt.Errorf("v2 suite has no private holdouts")
	}
	if len(privateSeeds) != len(entries) {
		return fmt.Errorf("private seed map has %d entries; want %d", len(privateSeeds), len(entries))
	}
	for _, entry := range entries {
		seed, exists := privateSeeds[entry.PrivateSeedID]
		if !exists {
			return fmt.Errorf("private seed map lacks %q", entry.PrivateSeedID)
		}
		if seed <= 0 {
			return fmt.Errorf("private seed %q must be a positive int64", entry.PrivateSeedID)
		}
		if got := SearchSuiteV2PrivateSeedCommitment(entry.PrivateSeedID, seed); got != entry.PrivateSeedCommitment {
			return fmt.Errorf("private seed commitment mismatch for %q", entry.PrivateSeedID)
		}
		privateEntry := entry
		privateEntry.Role = SuiteRoleDevelopment
		privateEntry.PrivateSeedID = ""
		privateEntry.Seed = &seed
		if _, _, err := materializeGeneratedSearchSuiteCaseV2WithDiagnostics(catalog, privateEntry); err != nil {
			return fmt.Errorf("private holdout %q: %w", entry.ID, err)
		}
	}
	for privateSeedID := range privateSeeds {
		known := false
		for _, entry := range entries {
			if entry.PrivateSeedID == privateSeedID {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("private seed map contains unknown ID %q", privateSeedID)
		}
	}
	return nil
}
