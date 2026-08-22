package benchmark

import (
	"fmt"
	"strings"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

const SearchSuiteGeneratorV1 = "search-suite-generator-v1"

type searchSuiteGeneratorFunc func(model.Catalog, GeneratedSearchSuiteCase) (scenario.Scenario, error)

type searchSuiteGeneratorRegistration struct {
	version     string
	materialize searchSuiteGeneratorFunc
}

var searchSuiteGenerators = []searchSuiteGeneratorRegistration{
	{
		version:     SearchSuiteGeneratorV1,
		materialize: materializeGeneratedSearchSuiteCaseV1,
	},
}

// SupportedSearchSuiteGeneratorVersions returns all generator versions that
// this binary can materialize. Locks remain schema-valid even when their
// pinned version is not supported by this binary.
func SupportedSearchSuiteGeneratorVersions() []string {
	versions := make([]string, len(searchSuiteGenerators))
	for index, generator := range searchSuiteGenerators {
		versions[index] = generator.version
	}
	return versions
}

func ValidateSearchSuiteGeneratorVersion(version string) error {
	_, err := lookupSearchSuiteGenerator(version)
	return err
}

func lookupSearchSuiteGenerator(version string) (searchSuiteGeneratorFunc, error) {
	if version == "" {
		return nil, fmt.Errorf("search suite generator version is required")
	}
	for _, generator := range searchSuiteGenerators {
		if generator.version != version {
			continue
		}
		if generator.materialize == nil {
			return nil, fmt.Errorf("search suite generator version %q has no materializer", version)
		}
		return generator.materialize, nil
	}
	return nil, unsupportedSearchSuiteGeneratorVersionError(version)
}

func unsupportedSearchSuiteGeneratorVersionError(version string) error {
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
	generator, err := lookupSearchSuiteGenerator(generatorVersion)
	if err != nil {
		return scenario.Scenario{}, err
	}
	return generator(catalog, entry)
}
