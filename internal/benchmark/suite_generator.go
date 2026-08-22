package benchmark

import (
	"fmt"
	"strings"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

const SearchSuiteGeneratorV1 = "search-suite-generator-v1"

// SupportedSearchSuiteGeneratorVersions returns all generator versions that
// this binary can materialize. Locks remain schema-valid even when their
// pinned version is not supported by this binary.
func SupportedSearchSuiteGeneratorVersions() []string {
	return []string{SearchSuiteGeneratorV1}
}

func ValidateSearchSuiteGeneratorVersion(version string) error {
	if version == "" {
		return fmt.Errorf("search suite generator version is required")
	}
	for _, supported := range SupportedSearchSuiteGeneratorVersions() {
		if version == supported {
			return nil
		}
	}
	return fmt.Errorf(
		"unsupported search suite generator version %q; supported versions: %s",
		version,
		strings.Join(SupportedSearchSuiteGeneratorVersions(), ", "),
	)
}

// MaterializeGeneratedSearchSuiteCase turns a published seed into a scenario
// using the explicitly requested historical generator. Private holdouts
// intentionally cannot be materialized locally.
func MaterializeGeneratedSearchSuiteCase(generatorVersion string, catalog model.Catalog, entry GeneratedSearchSuiteCase) (scenario.Scenario, error) {
	switch generatorVersion {
	case SearchSuiteGeneratorV1:
		return materializeGeneratedSearchSuiteCaseV1(catalog, entry)
	default:
		return scenario.Scenario{}, ValidateSearchSuiteGeneratorVersion(generatorVersion)
	}
}
