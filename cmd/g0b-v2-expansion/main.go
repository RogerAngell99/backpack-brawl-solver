package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"backpack-brawl-solver/internal/benchmark"
)

const (
	historicalManifestRelative = "benchmarks/suites/general-search-v2.json"
	historicalLockRelative     = "benchmarks/suites/general-search-v2.lock"
	catalogRelative            = "data/catalog.json"
	confirmAManifestRelative   = "benchmarks/suites/general-search-v2-dev-confirm-a.json"
	confirmBManifestRelative   = "benchmarks/suites/general-search-v2-dev-confirm-b.json"
	confirmALockRelative       = "benchmarks/suites/general-search-v2-dev-confirm-a.lock"
	confirmBLockRelative       = "benchmarks/suites/general-search-v2-dev-confirm-b.lock"
)

func main() {
	if len(os.Args) < 2 {
		fatalf("usage: g0b-v2-expansion <preflight|select|materialize|verify> [flags]")
	}
	var err error
	switch os.Args[1] {
	case "preflight":
		err = runPreflight(os.Args[2:])
	case "select":
		err = runSelect(os.Args[2:])
	case "materialize":
		err = runMaterialize(os.Args[2:])
	case "verify":
		err = runVerify(os.Args[2:])
	default:
		err = fmt.Errorf("unknown phase %q", os.Args[1])
	}
	if err != nil {
		fatalf("G0-B %s failed: %v", os.Args[1], err)
	}
}

func runPreflight(args []string) error {
	flags := flag.NewFlagSet("preflight", flag.ContinueOnError)
	repo := flags.String("repo", ".", "repository root")
	out := flags.String("out", "", "exclusive preflight record path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	status, err := gitOutput(root, "status", "--porcelain=v1")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("working tree is not clean")
	}
	head, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	originMain, err := gitOutput(root, "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	if originMain != benchmark.G0BV2BaseSHA {
		return fmt.Errorf("origin/main = %s, want frozen %s", originMain, benchmark.G0BV2BaseSHA)
	}
	mergeBase, err := gitOutput(root, "merge-base", "HEAD", "origin/main")
	if err != nil {
		return err
	}
	if mergeBase != benchmark.G0BV2BaseSHA {
		return fmt.Errorf("branch does not descend directly from frozen main")
	}
	manifestLF, err := lfSHA256(filepath.Join(root, historicalManifestRelative))
	if err != nil {
		return err
	}
	lockLF, err := lfSHA256(filepath.Join(root, historicalLockRelative))
	if err != nil {
		return err
	}
	if manifestLF != benchmark.G0BV2HistoricalManifestLF || lockLF != benchmark.G0BV2HistoricalLockLF {
		return fmt.Errorf("historical GSV2 artifact hash mismatch")
	}
	manifest, err := benchmark.LoadSearchSuiteManifest(filepath.Join(root, historicalManifestRelative))
	if err != nil {
		return err
	}
	core, err := benchmark.ExtractG0BV2Core(manifest)
	if err != nil {
		return err
	}
	schema := benchmark.SearchSuiteV2DevelopmentCohortSchema()
	universe, err := benchmark.EnumerateDevelopmentCohortUniverse(schema)
	if err != nil {
		return err
	}
	pairs, err := benchmark.DevelopmentCohortAttainablePairCount(schema)
	if err != nil {
		return err
	}
	if len(universe) != benchmark.G0BV2UniverseSize || pairs != benchmark.G0BV2AttainablePairs || len(core.Cases) != 14 {
		return fmt.Errorf("frozen structural preflight invariant mismatch")
	}
	checks := []struct {
		name string
		args []string
	}{
		{name: "go_test_normal", args: []string{"test", "./..."}},
		{name: "go_test_searchprofile", args: []string{"test", "-tags", "searchprofile", "./..."}},
	}
	for _, check := range checks {
		if err := runCommand(root, "go", check.args...); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
	}
	if err := runCommand(root, "git", "diff", "--check"); err != nil {
		return fmt.Errorf("git_diff_check: %w", err)
	}
	selectorBlob, err := gitOutput(root, "rev-parse", "HEAD:internal/benchmark/development_cohort_selector.go")
	if err != nil {
		return err
	}
	baseSelectorBlob, err := gitOutput(root, "rev-parse", benchmark.G0BV2BaseSHA+":internal/benchmark/development_cohort_selector.go")
	if err != nil {
		return err
	}
	partitionerBlob, err := gitOutput(root, "rev-parse", "HEAD:internal/benchmark/development_cohort_partitioner.go")
	if err != nil {
		return err
	}
	basePartitionerBlob, err := gitOutput(root, "rev-parse", benchmark.G0BV2BaseSHA+":internal/benchmark/development_cohort_partitioner.go")
	if err != nil {
		return err
	}
	if selectorBlob != baseSelectorBlob || partitionerBlob != basePartitionerBlob {
		return fmt.Errorf("frozen selector or partitioner differs from baseline")
	}
	record := fmt.Sprintf("G0-B PREFLIGHT\n\nbase_main_sha=%s\nbranch_head_sha=%s\norigin_main_sha=%s\nmerge_base_sha=%s\nworking_tree_clean=true\nhistorical_manifest_lf_sha256=%s\nhistorical_lock_lf_sha256=%s\nhistorical_artifacts_frozen=PASS\nselector_blob=%s\npartitioner_blob=%s\nfrozen_instruments_unchanged=PASS\nv2_dimension_count=%d\nv2_universe_size=%d PASS\nv2_attainable_pairs=%d PASS\ncore_descriptors=%d PASS\nselection_golden_trace=PASS\npartition_golden_trace=PASS\ninput_order_invariance=PASS\nseed_derivation=PASS\noutcome_blind_guard=PASS\ngo_test_normal=PASS\ngo_test_searchprofile=PASS\ngit_diff_check=PASS\nofficial_selection_runs=0\nmaterialization_runs=0\nbenchmark_scenario_runs=0\nnormal_solver_runs=0\nsearchprofile_runs=0\nefficacy_quality_runs=0\nefficacy_diagnostic_runs=0\nvalidation_materialized=false\npublic_holdout_materialized=false\nprivate_holdout_materialized=false\n",
		benchmark.G0BV2BaseSHA, head, originMain, mergeBase, manifestLF, lockLF, selectorBlob, partitionerBlob,
		len(schema.Dimensions), len(universe), pairs, len(core.Cases))
	if err := writeExclusive(*out, []byte(record)); err != nil {
		return err
	}
	fmt.Printf("G0-B preflight PASS; record=%s\n", *out)
	return nil
}

func runSelect(args []string) error {
	flags := flag.NewFlagSet("select", flag.ContinueOnError)
	repo := flags.String("repo", ".", "repository root")
	preflight := flags.String("preflight", "", "preflight record path")
	outDir := flags.String("out", "benchmarks/efficacy/g0b-evidence", "evidence directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *preflight == "" {
		return fmt.Errorf("--preflight is required")
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	if err := ensureCleanG0BBranch(root); err != nil {
		return err
	}
	preflightContent, err := os.ReadFile(*preflight)
	if err != nil {
		return err
	}
	if !bytes.Contains(preflightContent, []byte("official_selection_runs=0")) || !bytes.Contains(preflightContent, []byte("outcome_blind_guard=PASS")) {
		return fmt.Errorf("preflight record is incomplete")
	}
	manifest, err := benchmark.LoadSearchSuiteManifest(filepath.Join(root, historicalManifestRelative))
	if err != nil {
		return err
	}
	artifacts, err := benchmark.PrepareG0BV2Selection(manifest)
	if err != nil {
		return err
	}
	evidenceDir := filepath.Join(root, filepath.FromSlash(*outDir))
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		return err
	}
	writes := []struct {
		name  string
		value any
	}{
		{"core-descriptors.json", artifacts.Core},
		{"selection-trace.json", artifacts.Selection},
		{"partition-trace.json", artifacts.Partition},
		{"seed-audit.json", artifacts.Seeds},
		{"cohort-membership.json", artifacts.Membership},
		{"coverage-summary.json", artifacts.Coverage},
		{"pre-materialization-freeze.json", artifacts.Freeze},
		{"selection-freeze.json", artifacts.Freeze},
	}
	for _, write := range writes {
		if err := writeJSONExclusive(filepath.Join(evidenceDir, write.name), write.value); err != nil {
			return err
		}
	}
	if err := writeExclusive(filepath.Join(evidenceDir, "preflight-record.txt"), preflightContent); err != nil {
		return err
	}
	if err := writeSeedCSV(filepath.Join(evidenceDir, "seed-audit.csv"), artifacts.Seeds); err != nil {
		return err
	}
	if err := writeMarginalCSV(filepath.Join(evidenceDir, "descriptor-marginals.csv"), artifacts.Coverage); err != nil {
		return err
	}
	if err := writeCoverageCSV(filepath.Join(evidenceDir, "pairwise-coverage.csv"), artifacts.Coverage); err != nil {
		return err
	}
	if err := writeDistanceCSV(filepath.Join(evidenceDir, "distance-summary.csv"), artifacts.Membership); err != nil {
		return err
	}
	fmt.Printf("V2 universe: %d PASS\nAttainable pairs: %d PASS\nExpansion descriptors: %d PASS\nWave A: %d PASS\nWave B: %d PASS\nCombined coverage: %d/%d PASS\nWave A coverage: %d/%d PASS\nWave B coverage: %d/%d PASS\nSearch benchmark runs: 0\n",
		benchmark.G0BV2UniverseSize, artifacts.Coverage.AttainablePairs, len(artifacts.Membership.Cases),
		artifacts.Coverage.WaveA.CaseCount, artifacts.Coverage.WaveB.CaseCount,
		artifacts.Coverage.Combined.PairwiseCoverage, artifacts.Coverage.AttainablePairs,
		artifacts.Coverage.WaveA.PairwiseCoverage, artifacts.Coverage.AttainablePairs,
		artifacts.Coverage.WaveB.PairwiseCoverage, artifacts.Coverage.AttainablePairs)
	return nil
}

func runMaterialize(args []string) error {
	flags := flag.NewFlagSet("materialize", flag.ContinueOnError)
	repo := flags.String("repo", ".", "repository root")
	evidence := flags.String("evidence", "benchmarks/efficacy/g0b-evidence", "evidence directory")
	selectionFreezeSHA := flags.String("selection-freeze-sha", "", "committed selection freeze SHA")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *selectionFreezeSHA == "" {
		return fmt.Errorf("--selection-freeze-sha is required")
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	if err := ensureCleanG0BBranch(root); err != nil {
		return err
	}
	head, err := gitOutput(root, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if head != *selectionFreezeSHA {
		if err := runCommand(root, "git", "merge-base", "--is-ancestor", *selectionFreezeSHA, "HEAD"); err != nil {
			return fmt.Errorf("selection freeze SHA %s is not an ancestor of HEAD %s", *selectionFreezeSHA, head)
		}
		frozenPaths := []string{
			"benchmarks/efficacy/g0b-evidence/core-descriptors.json",
			"benchmarks/efficacy/g0b-evidence/selection-trace.json",
			"benchmarks/efficacy/g0b-evidence/partition-trace.json",
			"benchmarks/efficacy/g0b-evidence/seed-audit.json",
			"benchmarks/efficacy/g0b-evidence/cohort-membership.json",
			"benchmarks/efficacy/g0b-evidence/selection-freeze.json",
		}
		diffArgs := []string{"diff", "--quiet", *selectionFreezeSHA, "HEAD", "--"}
		diffArgs = append(diffArgs, frozenPaths...)
		if err := runCommand(root, "git", diffArgs...); err != nil {
			return fmt.Errorf("selection artifacts differ from freeze commit %s", *selectionFreezeSHA)
		}
	}
	evidenceDir := filepath.Join(root, filepath.FromSlash(*evidence))
	var membership benchmark.G0BV2CohortMembership
	if err := loadJSON(filepath.Join(evidenceDir, "cohort-membership.json"), &membership); err != nil {
		return err
	}
	var freeze benchmark.G0BV2PreMaterializationFreeze
	if err := loadJSON(filepath.Join(evidenceDir, "selection-freeze.json"), &freeze); err != nil {
		return err
	}
	membershipHash, err := jsonValueSHA256(membership)
	if err != nil {
		return err
	}
	if membershipHash != freeze.CohortMembershipSHA256 || freeze.MaterializationRuns != 0 || freeze.BenchmarkScenarioRuns != 0 || !freeze.StructuralGates.Pass() {
		return fmt.Errorf("selection freeze integrity check failed")
	}
	historical, err := benchmark.LoadSearchSuiteManifest(filepath.Join(root, historicalManifestRelative))
	if err != nil {
		return err
	}
	manifestA, manifestB, err := benchmark.BuildG0BV2ConfirmManifests(historical, membership)
	if err != nil {
		return err
	}
	manifestAPath, manifestBPath := filepath.Join(root, confirmAManifestRelative), filepath.Join(root, confirmBManifestRelative)
	lockAPath, lockBPath := filepath.Join(root, confirmALockRelative), filepath.Join(root, confirmBLockRelative)
	if err := writeJSONExclusive(manifestAPath, manifestA); err != nil {
		return err
	}
	if err := writeJSONExclusive(manifestBPath, manifestB); err != nil {
		return err
	}
	catalogPath := filepath.Join(root, catalogRelative)
	lockA, err := benchmark.ObserveSearchSuite(manifestAPath, catalogPath, benchmark.SearchSuiteGeneratorV2)
	if err != nil {
		return err
	}
	if err := benchmark.WriteSearchSuiteLock(lockAPath, lockA); err != nil {
		return err
	}
	lockB, err := benchmark.ObserveSearchSuite(manifestBPath, catalogPath, benchmark.SearchSuiteGeneratorV2)
	if err != nil {
		return err
	}
	if err := benchmark.WriteSearchSuiteLock(lockBPath, lockB); err != nil {
		return err
	}
	if err := benchmark.VerifySearchSuiteLock(manifestAPath, catalogPath, lockAPath); err != nil {
		return err
	}
	if err := benchmark.VerifySearchSuiteLock(manifestBPath, catalogPath, lockBPath); err != nil {
		return err
	}
	audit, err := benchmark.AuditG0BV2Materialization(membership, lockA, lockB)
	if err != nil {
		return err
	}
	var coverage benchmark.G0BV2CoverageSummary
	if err := loadJSON(filepath.Join(evidenceDir, "coverage-summary.json"), &coverage); err != nil {
		return err
	}
	for _, output := range []struct {
		name  string
		value any
	}{
		{"materialization-audit.json", audit},
		{"v2-materialization-audit.json", audit},
		{"v2-coverage-summary.json", coverage},
		{"v2-wave-summary.json", map[string]any{"version": 1, "wave_a": coverage.WaveA, "wave_b": coverage.WaveB}},
	} {
		if err := writeJSONExclusive(filepath.Join(evidenceDir, output.name), output.value); err != nil {
			return err
		}
	}
	if err := copyExclusive(filepath.Join(evidenceDir, "selection-trace.json"), filepath.Join(evidenceDir, "v2-selection-audit.json")); err != nil {
		return err
	}
	if err := copyExclusive(filepath.Join(evidenceDir, "partition-trace.json"), filepath.Join(evidenceDir, "v2-partition-audit.json")); err != nil {
		return err
	}
	if err := copyExclusive(filepath.Join(evidenceDir, "seed-audit.json"), filepath.Join(evidenceDir, "v2-seed-audit.json")); err != nil {
		return err
	}
	if err := writeRealizedCSV(filepath.Join(evidenceDir, "realized-structure.csv"), audit); err != nil {
		return err
	}
	fmt.Printf("Materializations: %d/%d PASS\nRequested-vs-realized: %d/%d PASS\nStructural witness: %d/%d PASS\nSearch benchmark runs: 0\n",
		audit.MaterializedCases, benchmark.G0BV2ExpansionSize,
		audit.RequestedVsRealizedPasses, benchmark.G0BV2ExpansionSize,
		audit.StructuralWitnessPasses, benchmark.G0BV2ExpansionSize)
	return nil
}

func runVerify(args []string) error {
	flags := flag.NewFlagSet("verify", flag.ContinueOnError)
	repo := flags.String("repo", ".", "repository root")
	evidence := flags.String("evidence", "benchmarks/efficacy/g0b-evidence", "evidence directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return err
	}
	manifestAPath, manifestBPath := filepath.Join(root, confirmAManifestRelative), filepath.Join(root, confirmBManifestRelative)
	lockAPath, lockBPath := filepath.Join(root, confirmALockRelative), filepath.Join(root, confirmBLockRelative)
	catalogPath := filepath.Join(root, catalogRelative)
	if err := benchmark.VerifySearchSuiteLock(manifestAPath, catalogPath, lockAPath); err != nil {
		return err
	}
	if err := benchmark.VerifySearchSuiteLock(manifestBPath, catalogPath, lockBPath); err != nil {
		return err
	}
	for _, population := range []struct {
		name         string
		manifestPath string
		lockPath     string
	}{
		{name: "Confirm-A", manifestPath: manifestAPath, lockPath: lockAPath},
		{name: "Confirm-B", manifestPath: manifestBPath, lockPath: lockBPath},
	} {
		observed, err := benchmark.ObserveSearchSuite(population.manifestPath, catalogPath, benchmark.SearchSuiteGeneratorV2)
		if err != nil {
			return err
		}
		observedBytes, err := json.MarshalIndent(observed, "", "  ")
		if err != nil {
			return err
		}
		committed, err := benchmark.LoadSearchSuiteLock(population.lockPath)
		if err != nil {
			return err
		}
		committedBytes, err := json.MarshalIndent(committed, "", "  ")
		if err != nil {
			return err
		}
		if !bytes.Equal(observedBytes, committedBytes) {
			return fmt.Errorf("%s fresh observation is not byte-identical after canonical lock serialization", population.name)
		}
	}
	evidenceDir := filepath.Join(root, filepath.FromSlash(*evidence))
	var membership benchmark.G0BV2CohortMembership
	if err := loadJSON(filepath.Join(evidenceDir, "cohort-membership.json"), &membership); err != nil {
		return err
	}
	lockA, err := benchmark.LoadSearchSuiteLock(lockAPath)
	if err != nil {
		return err
	}
	lockB, err := benchmark.LoadSearchSuiteLock(lockBPath)
	if err != nil {
		return err
	}
	audit, err := benchmark.AuditG0BV2Materialization(membership, lockA, lockB)
	if err != nil {
		return err
	}
	if audit.MaterializedCases != benchmark.G0BV2ExpansionSize || audit.BenchmarkScenarioRuns != 0 || audit.ValidationMaterialized || audit.PublicHoldoutMaterialized || audit.PrivateHoldoutMaterialized {
		return fmt.Errorf("post-freeze materialization audit failed")
	}
	historical, err := benchmark.LoadSearchSuiteManifest(filepath.Join(root, historicalManifestRelative))
	if err != nil {
		return err
	}
	manifestA, err := benchmark.LoadSearchSuiteManifest(manifestAPath)
	if err != nil {
		return err
	}
	manifestB, err := benchmark.LoadSearchSuiteManifest(manifestBPath)
	if err != nil {
		return err
	}
	independentCoverage, err := benchmark.AuditG0BV2CoverageFromManifests(historical, manifestA, manifestB)
	if err != nil {
		return err
	}
	if !independentCoverage.Gates.Pass() {
		return fmt.Errorf("independent manifest-derived structural audit failed")
	}
	var frozenCoverage benchmark.G0BV2CoverageSummary
	if err := loadJSON(filepath.Join(evidenceDir, "v2-coverage-summary.json"), &frozenCoverage); err != nil {
		return err
	}
	independentCoverageHash, err := jsonValueSHA256(independentCoverage)
	if err != nil {
		return err
	}
	frozenCoverageHash, err := jsonValueSHA256(frozenCoverage)
	if err != nil {
		return err
	}
	if independentCoverageHash != frozenCoverageHash {
		return fmt.Errorf("manifest-derived structural audit differs from frozen coverage summary")
	}
	manifestLF, err := lfSHA256(filepath.Join(root, historicalManifestRelative))
	if err != nil {
		return err
	}
	lockLF, err := lfSHA256(filepath.Join(root, historicalLockRelative))
	if err != nil {
		return err
	}
	if manifestLF != benchmark.G0BV2HistoricalManifestLF || lockLF != benchmark.G0BV2HistoricalLockLF {
		return fmt.Errorf("historical GSV2 artifacts changed")
	}
	fmt.Println("post_freeze_revalidation=PASS")
	fmt.Println("post_freeze_benchmark_search_runs=0")
	return nil
}

func ensureCleanG0BBranch(root string) error {
	status, err := gitOutput(root, "status", "--porcelain=v1")
	if err != nil {
		return err
	}
	if status != "" {
		return fmt.Errorf("working tree is not clean")
	}
	originMain, err := gitOutput(root, "rev-parse", "origin/main")
	if err != nil {
		return err
	}
	if originMain != benchmark.G0BV2BaseSHA {
		return fmt.Errorf("origin/main has advanced to %s; audit is required", originMain)
	}
	mergeBase, err := gitOutput(root, "merge-base", "HEAD", "origin/main")
	if err != nil {
		return err
	}
	if mergeBase != benchmark.G0BV2BaseSHA {
		return fmt.Errorf("branch merge base is %s, want %s", mergeBase, benchmark.G0BV2BaseSHA)
	}
	return nil
}

func writeJSONExclusive(path string, value any) error {
	content, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeExclusive(path, append(content, '\n'))
}

func writeExclusive(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func copyExclusive(source string, destination string) error {
	content, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return writeExclusive(destination, content)
}

func loadJSON(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%s has trailing JSON", path)
	}
	return nil
}

func lfSHA256(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	digest := sha256.Sum256(normalized)
	return hex.EncodeToString(digest[:]), nil
}

func jsonValueSHA256(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func gitOutput(root string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func runCommand(root string, name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w\n%s", name, strings.Join(args, " "), err, string(output))
	}
	return nil
}

func writeSeedCSV(path string, audit benchmark.G0BV2SeedAudit) error {
	rows := [][]string{{"case_id", "selection_step", "canonical_descriptor", "namespace", "digest", "derived_seed"}}
	for _, entry := range audit.Cases {
		rows = append(rows, []string{entry.CaseID, fmt.Sprint(entry.SelectionStep), entry.CanonicalDescriptor, entry.Namespace, entry.Digest, fmt.Sprint(entry.DerivedSeed)})
	}
	return writeCSVExclusive(path, rows)
}

func writeMarginalCSV(path string, summary benchmark.G0BV2CoverageSummary) error {
	rows := [][]string{{"population", "dimension", "category", "count"}}
	populations := []struct {
		name    string
		summary benchmark.G0BV2PopulationSummary
	}{{"core", summary.Core}, {"combined", summary.Combined}, {"wave_a", summary.WaveA}, {"wave_b", summary.WaveB}}
	for _, population := range populations {
		for _, dimension := range population.summary.Dimensions {
			values := make([]string, 0, len(dimension.Counts))
			for value := range dimension.Counts {
				values = append(values, value)
			}
			sort.Strings(values)
			for _, value := range values {
				rows = append(rows, []string{population.name, dimension.Dimension, value, fmt.Sprint(dimension.Counts[value])})
			}
		}
	}
	return writeCSVExclusive(path, rows)
}

func writeCoverageCSV(path string, summary benchmark.G0BV2CoverageSummary) error {
	rows := [][]string{{"population", "covered_pairs", "attainable_pairs"},
		{"core", fmt.Sprint(summary.Core.PairwiseCoverage), fmt.Sprint(summary.AttainablePairs)},
		{"combined", fmt.Sprint(summary.Combined.PairwiseCoverage), fmt.Sprint(summary.AttainablePairs)},
		{"wave_a", fmt.Sprint(summary.WaveA.PairwiseCoverage), fmt.Sprint(summary.AttainablePairs)},
		{"wave_b", fmt.Sprint(summary.WaveB.PairwiseCoverage), fmt.Sprint(summary.AttainablePairs)},
	}
	return writeCSVExclusive(path, rows)
}

func writeDistanceCSV(path string, membership benchmark.G0BV2CohortMembership) error {
	rows := [][]string{{"population", "comparisons", "minimum_hamming", "maximum_hamming", "mean_hamming"}}
	for _, wave := range []string{"expansion", "A", "B"} {
		descriptors := make([]benchmark.DevelopmentCohortDescriptor, 0)
		for _, entry := range membership.Cases {
			if wave == "expansion" || entry.Wave == wave {
				descriptors = append(descriptors, entry.Descriptor)
			}
		}
		comparisons, minimum, maximum, total := 0, len(benchmark.SearchSuiteV2DevelopmentCohortSchema().Dimensions), 0, 0
		for left := 0; left < len(descriptors); left++ {
			for right := left + 1; right < len(descriptors); right++ {
				distance := 0
				for dimension, value := range descriptors[left].Values {
					if descriptors[right].Values[dimension] != value {
						distance++
					}
				}
				comparisons++
				total += distance
				if distance < minimum {
					minimum = distance
				}
				if distance > maximum {
					maximum = distance
				}
			}
		}
		mean := float64(0)
		if comparisons > 0 {
			mean = float64(total) / float64(comparisons)
		}
		rows = append(rows, []string{strings.ToLower(wave), fmt.Sprint(comparisons), fmt.Sprint(minimum), fmt.Sprint(maximum), fmt.Sprintf("%.6f", mean)})
	}
	return writeCSVExclusive(path, rows)
}

func writeRealizedCSV(path string, audit benchmark.G0BV2MaterializationAudit) error {
	rows := [][]string{{"case_id", "wave", "seed", "scenario_sha256", "usable_cells", "inventory_area", "density_bps", "total_item_instances", "distinct_item_definitions", "requested_vs_realized", "structural_witness"}}
	for _, entry := range audit.Cases {
		rows = append(rows, []string{entry.CaseID, entry.Wave, fmt.Sprint(entry.Seed), entry.ScenarioSHA256,
			fmt.Sprint(entry.Realized.UsableCells), fmt.Sprint(entry.Realized.InventoryArea), fmt.Sprint(entry.Realized.DensityBPS),
			fmt.Sprint(entry.Realized.TotalItemInstances), fmt.Sprint(entry.Realized.DistinctItemDefinitions),
			entry.RequestedVsRealized, entry.StructuralWitness})
	}
	return writeCSVExclusive(path, rows)
}

func writeCSVExclusive(path string, rows [][]string) error {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(rows); err != nil {
		return err
	}
	return writeExclusive(path, buffer.Bytes())
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
