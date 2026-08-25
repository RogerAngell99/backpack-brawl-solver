package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

var (
	g0bOfficialOnce      sync.Once
	g0bOfficialArtifacts G0BV2SelectionArtifacts
	g0bOfficialErr       error
)

func TestG0BV2OfficialSelectionTraceFrozen(t *testing.T) {
	actual := officialG0BV2Artifacts(t)
	var expected DevelopmentCohortSelection
	loadG0BV2Evidence(t, "selection-trace.json", &expected)
	if !reflect.DeepEqual(actual.Selection, expected) {
		t.Fatal("official G0-B V2 selection trace differs from the frozen artifact")
	}
}

func TestG0BV2OfficialPartitionTraceFrozen(t *testing.T) {
	actual := officialG0BV2Artifacts(t)
	var expected DevelopmentCohortPartition
	loadG0BV2Evidence(t, "partition-trace.json", &expected)
	if !reflect.DeepEqual(actual.Partition, expected) {
		t.Fatal("official G0-B V2 partition trace differs from the frozen artifact")
	}
}

func TestG0BV2SeedAuditFrozen(t *testing.T) {
	actual := officialG0BV2Artifacts(t)
	var expected G0BV2SeedAudit
	loadG0BV2Evidence(t, "seed-audit.json", &expected)
	if !reflect.DeepEqual(actual.Seeds, expected) {
		t.Fatal("official G0-B V2 seed audit differs from the frozen artifact")
	}
}

func TestG0BV2OfficialMembershipFrozen(t *testing.T) {
	actual := officialG0BV2Artifacts(t)
	var expected G0BV2CohortMembership
	loadG0BV2Evidence(t, "cohort-membership.json", &expected)
	if !reflect.DeepEqual(actual.Membership, expected) {
		t.Fatal("official G0-B V2 membership differs from the frozen artifact")
	}
}

func TestG0BV2CombinedMarginalsBalanced(t *testing.T) {
	coverage := officialG0BV2Artifacts(t).Coverage
	if !coverage.Combined.MarginalsBalanced {
		t.Fatal("combined G0-B V2 population marginals are not balanced")
	}
}

func TestG0BV2CombinedPairCoverage(t *testing.T) {
	coverage := officialG0BV2Artifacts(t).Coverage
	if coverage.Combined.PairwiseCoverage < G0BV2CombinedCoverageGate || coverage.CoreCoverageDelta < G0BV2CombinedCoverageDelta {
		t.Fatalf("combined coverage = %d with core delta %d", coverage.Combined.PairwiseCoverage, coverage.CoreCoverageDelta)
	}
}

func TestG0BV2EachWavePairCoverage(t *testing.T) {
	coverage := officialG0BV2Artifacts(t).Coverage
	if coverage.WaveA.PairwiseCoverage < G0BV2WaveCoverageGate || coverage.WaveB.PairwiseCoverage < G0BV2WaveCoverageGate {
		t.Fatalf("wave coverage = A:%d B:%d", coverage.WaveA.PairwiseCoverage, coverage.WaveB.PairwiseCoverage)
	}
	if !coverage.WaveA.CategoriesComplete || !coverage.WaveB.CategoriesComplete {
		t.Fatal("one or both G0-B V2 waves omit a schema category")
	}
}

func TestG0BV2NoCoreDescriptorDuplicate(t *testing.T) {
	coverage := officialG0BV2Artifacts(t).Coverage
	if len(coverage.CoreOverlap) != 0 {
		t.Fatalf("G0-B V2 expansion overlaps core: %v", coverage.CoreOverlap)
	}
}

func TestG0BV2CoverageReconstructedFromManifests(t *testing.T) {
	historical := loadG0BV2Manifest(t, "general-search-v2.json")
	manifestA := loadG0BV2Manifest(t, "general-search-v2-dev-confirm-a.json")
	manifestB := loadG0BV2Manifest(t, "general-search-v2-dev-confirm-b.json")
	actual, err := AuditG0BV2CoverageFromManifests(historical, manifestA, manifestB)
	if err != nil {
		t.Fatal(err)
	}
	var expected G0BV2CoverageSummary
	loadG0BV2Evidence(t, "v2-coverage-summary.json", &expected)
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("manifest-derived G0-B structural audit differs from the frozen coverage summary")
	}
}

func TestG0BV2ConfirmAWaveMatchesFrozenMembership(t *testing.T) {
	assertG0BV2ConfirmWaveMatchesFrozenMembership(t, "A", "general-search-v2-dev-confirm-a.json")
}

func TestG0BV2ConfirmBWaveMatchesFrozenMembership(t *testing.T) {
	assertG0BV2ConfirmWaveMatchesFrozenMembership(t, "B", "general-search-v2-dev-confirm-b.json")
}

func TestG0BV2ConfirmManifestsContainOnlyDevelopment(t *testing.T) {
	for _, name := range []string{"general-search-v2-dev-confirm-a.json", "general-search-v2-dev-confirm-b.json"} {
		manifest := loadG0BV2Manifest(t, name)
		if len(manifest.Scenarios) != 0 || len(manifest.Generated) != G0BV2WaveSize {
			t.Fatalf("%s must contain no static cases and exactly %d generated cases", name, G0BV2WaveSize)
		}
		historical := loadG0BV2Manifest(t, "general-search-v2.json")
		if manifest.Version != historical.Version || manifest.Workers != historical.Workers ||
			manifest.BaselinePolicy != historical.BaselinePolicy || !reflect.DeepEqual(manifest.Budgets, historical.Budgets) {
			t.Fatalf("%s metadata differs from the frozen historical GSV2 policy", name)
		}
		for _, entry := range manifest.Generated {
			if entry.Role != SuiteRoleDevelopment || entry.Family != GeneratedFamilyStructuralV2 || entry.Seed == nil || entry.StructuralDescriptor == nil {
				t.Fatalf("%s contains a non-development or incomplete generated case %q", name, entry.ID)
			}
		}
	}
}

func TestG0BV2ConfirmManifestsContainNoResultFields(t *testing.T) {
	for _, name := range []string{"general-search-v2-dev-confirm-a.json", "general-search-v2-dev-confirm-b.json"} {
		content, err := os.ReadFile(filepath.Join("..", "..", "benchmarks", "suites", name))
		if err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(string(content))
		for _, field := range []string{"\"score\"", "\"nodes\"", "\"runtime\"", "\"difficulty\"", "\"solution\"", "\"phase_stats\"", "\"repair_count\""} {
			if strings.Contains(lower, field) {
				t.Fatalf("%s contains forbidden result field %s", name, field)
			}
		}
	}
}

func TestG0BV2ConfirmALockReproduces(t *testing.T) {
	verifyG0BV2ConfirmLock(t, "general-search-v2-dev-confirm-a.json", "general-search-v2-dev-confirm-a.lock")
}

func TestG0BV2ConfirmBLockReproduces(t *testing.T) {
	verifyG0BV2ConfirmLock(t, "general-search-v2-dev-confirm-b.json", "general-search-v2-dev-confirm-b.lock")
}

func officialG0BV2Artifacts(t *testing.T) G0BV2SelectionArtifacts {
	t.Helper()
	g0bOfficialOnce.Do(func() {
		manifest, err := LoadSearchSuiteManifest(filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json"))
		if err != nil {
			g0bOfficialErr = err
			return
		}
		g0bOfficialArtifacts, g0bOfficialErr = PrepareG0BV2Selection(manifest)
	})
	if g0bOfficialErr != nil {
		t.Fatal(g0bOfficialErr)
	}
	return g0bOfficialArtifacts
}

func loadG0BV2Evidence(t *testing.T, name string, target any) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "..", "benchmarks", "efficacy", "g0b-evidence", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(content, target); err != nil {
		t.Fatal(err)
	}
}

func assertG0BV2ConfirmWaveMatchesFrozenMembership(t *testing.T, wave string, name string) {
	t.Helper()
	manifest := loadG0BV2Manifest(t, name)
	expected := make(map[string]G0BV2MembershipCase)
	for _, entry := range officialG0BV2Artifacts(t).Membership.Cases {
		if entry.Wave == wave {
			expected[entry.CaseID] = entry
		}
	}
	if len(manifest.Generated) != len(expected) {
		t.Fatalf("%s generated count = %d, want %d", name, len(manifest.Generated), len(expected))
	}
	for _, entry := range manifest.Generated {
		member, ok := expected[entry.ID]
		if !ok || entry.Seed == nil || *entry.Seed != member.Seed || entry.StructuralDescriptor == nil || *entry.StructuralDescriptor != member.StructuralDescriptor {
			t.Fatalf("%s case %q differs from frozen Wave %s membership", name, entry.ID, wave)
		}
	}
}

func loadG0BV2Manifest(t *testing.T, name string) SearchSuiteManifest {
	t.Helper()
	manifest, err := LoadSearchSuiteManifest(filepath.Join("..", "..", "benchmarks", "suites", name))
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}

func verifyG0BV2ConfirmLock(t *testing.T, manifestName string, lockName string) {
	t.Helper()
	root := filepath.Join("..", "..")
	if err := VerifySearchSuiteLock(
		filepath.Join(root, "benchmarks", "suites", manifestName),
		filepath.Join(root, "data", "catalog.json"),
		filepath.Join(root, "benchmarks", "suites", lockName),
	); err != nil {
		t.Fatal(err)
	}
}
