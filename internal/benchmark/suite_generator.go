package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/scenario"
)

const (
	SearchSuiteGeneratorV1 = "search-suite-generator-v1"
	SearchSuiteGeneratorV2 = "search-suite-generator-v2"
)

type searchSuiteGeneratorFunc func(model.Catalog, GeneratedSearchSuiteCase) (scenario.Scenario, error)
type searchSuiteGeneratorEntryValidator func(GeneratedSearchSuiteCase) error

type searchSuiteGeneratorRegistration struct {
	version       string
	validateEntry searchSuiteGeneratorEntryValidator
	materialize   searchSuiteGeneratorFunc
}

var searchSuiteGenerators = []searchSuiteGeneratorRegistration{
	{
		version:       SearchSuiteGeneratorV1,
		validateEntry: validateGeneratedSearchSuiteCaseV1,
		materialize:   materializeGeneratedSearchSuiteCaseV1,
	},
	{
		version:       SearchSuiteGeneratorV2,
		validateEntry: validateGeneratedSearchSuiteCaseV2,
		materialize:   materializeGeneratedSearchSuiteCaseV2,
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
	_, err := lookupSearchSuiteGeneratorRegistration(version)
	return err
}

func lookupSearchSuiteGenerator(version string) (searchSuiteGeneratorFunc, error) {
	registration, err := lookupSearchSuiteGeneratorRegistration(version)
	if err != nil {
		return nil, err
	}
	return registration.materialize, nil
}

func lookupSearchSuiteGeneratorRegistration(version string) (searchSuiteGeneratorRegistration, error) {
	if version == "" {
		return searchSuiteGeneratorRegistration{}, fmt.Errorf("search suite generator version is required")
	}
	for _, generator := range searchSuiteGenerators {
		if generator.version != version {
			continue
		}
		if generator.materialize == nil || generator.validateEntry == nil {
			return searchSuiteGeneratorRegistration{}, fmt.Errorf("search suite generator version %q is incomplete", version)
		}
		return generator, nil
	}
	return searchSuiteGeneratorRegistration{}, unsupportedSearchSuiteGeneratorVersionError(version)
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
	registration, err := lookupSearchSuiteGeneratorRegistration(generatorVersion)
	if err != nil {
		return scenario.Scenario{}, err
	}
	if err := registration.validateEntry(entry); err != nil {
		return scenario.Scenario{}, err
	}
	return registration.materialize(catalog, entry)
}

// ValidateSearchSuiteManifestForGenerator layers version-specific generated
// population semantics on top of the generator-neutral manifest schema.
func ValidateSearchSuiteManifestForGenerator(version string, manifest SearchSuiteManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	registration, err := lookupSearchSuiteGeneratorRegistration(version)
	if err != nil {
		return err
	}
	for _, entry := range manifest.Generated {
		if err := registration.validateEntry(entry); err != nil {
			return err
		}
	}
	return nil
}

func validateGeneratedSearchSuiteCaseV1(entry GeneratedSearchSuiteCase) error {
	if entry.StructuralDescriptor != nil {
		return fmt.Errorf("v1 generated case %q must not have a structural_descriptor", entry.ID)
	}
	if entry.PrivateSeedCommitment != "" {
		return fmt.Errorf("v1 generated case %q must not have a private_seed_commitment", entry.ID)
	}
	if entry.Role == SuiteRolePrivateHoldout {
		if entry.Family != GeneratedFamilyPrivate {
			return fmt.Errorf("v1 private holdout %q must use family %q", entry.ID, GeneratedFamilyPrivate)
		}
		return nil
	}
	if !oneOf(entry.Family, GeneratedFamilySparse, GeneratedFamilyDuplicated, GeneratedFamilyLoose) {
		return fmt.Errorf("v1 generated case %q has unsupported family %q", entry.ID, entry.Family)
	}
	return nil
}

func validateGeneratedSearchSuiteCaseV2(entry GeneratedSearchSuiteCase) error {
	if entry.Family != GeneratedFamilyStructuralV2 {
		return fmt.Errorf("v2 generated case %q must use family %q", entry.ID, GeneratedFamilyStructuralV2)
	}
	if entry.StructuralDescriptor == nil {
		return fmt.Errorf("v2 generated case %q requires structural_descriptor", entry.ID)
	}
	if err := entry.StructuralDescriptor.Validate(); err != nil {
		return fmt.Errorf("v2 generated case %q structural_descriptor: %w", entry.ID, err)
	}
	if entry.Role == SuiteRolePrivateHoldout {
		if err := validateSHA256("v2 private holdout "+entry.ID+" private_seed_commitment", entry.PrivateSeedCommitment); err != nil {
			return err
		}
	} else if entry.PrivateSeedCommitment != "" {
		return fmt.Errorf("v2 public generated case %q must not have a private_seed_commitment", entry.ID)
	}
	return nil
}

// SearchSuiteV2PrivateSeedCommitment binds a private seed identifier to its
// secret seed without disclosing the seed. CI recomputes this from the
// protected seed map before it materializes private holdouts.
func SearchSuiteV2PrivateSeedCommitment(privateSeedID string, seed int64) string {
	payload := SearchSuiteGeneratorV2 + "\x00private-seed\x00" + privateSeedID + "\x00" + strconv.FormatInt(seed, 10)
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}
