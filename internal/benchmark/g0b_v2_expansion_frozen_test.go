package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
