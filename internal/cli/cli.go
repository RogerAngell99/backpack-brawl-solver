package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"backpack-brawl-solver/internal/benchmark"
	"backpack-brawl-solver/internal/catalog"
	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
	"backpack-brawl-solver/internal/render"
	"backpack-brawl-solver/internal/scenario"
	"backpack-brawl-solver/internal/solver"
	"backpack-brawl-solver/internal/wikihtml"
)

func Run(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}

	switch args[0] {
	case "validate-catalog":
		return runValidateCatalog(args[1:], stdout, stderr)
	case "review-catalog":
		return runReviewCatalog(args[1:], stdout, stderr)
	case "solve":
		return runSolve(args[1:], stdout, stderr)
	case "import-wiki":
		return runImportWiki(args[1:], stdout, stderr)
	case "import-html":
		return runImportHTML(args[1:], stdout, stderr)
	case "benchmark-scenarios":
		return runBenchmarkScenarios(args[1:], stdout, stderr)
	case "compare-benchmarks":
		return runCompareBenchmarks(args[1:], stdout, stderr)
	case "compare-constellation-benchmarks":
		return runCompareConstellationBenchmarks(args[1:], stdout, stderr)
	case "summarize-operation-profile":
		return runSummarizeOperationProfile(args[1:], stdout, stderr)
	case "materialize-search-suite":
		return runMaterializeSearchSuite(args[1:], stdout, stderr)
	case "freeze-search-suite":
		return runFreezeSearchSuite(args[1:], stdout, stderr)
	case "verify-search-suite":
		return runVerifySearchSuite(args[1:], stdout, stderr)
	case "verify-private-search-suite":
		return runVerifyPrivateSearchSuite(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
}

func runBenchmarkScenarios(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("benchmark-scenarios", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	dir := flags.String("dir", filepath.Join("benchmarks", "scenarios"), "Directory containing scenario JSON files")
	scenariosText := flags.String("scenarios", "", "Comma-separated scenario filenames (without .json), or all when empty")
	budgetsText := flags.String("budgets", benchmark.FormatBudgets(benchmark.DefaultBudgets), "Comma-separated node budgets")
	repeat := flags.Int("repeat", 3, "Number of repetitions per scenario and budget")
	workers := flags.Int("workers", 1, "Number of solver workers")
	top := flags.Int("top", 1, "Number of solutions to keep")
	repairSearchMode := flags.String("repair-search-mode", benchmark.RepairSearchModeScenario, "Repair search override: scenario, on, or off")
	plateauVariant := flags.String("plateau-variant", solver.DefaultPlateauVariant, "Plateau LNS policy: legacy-large-off, large-16, large-16-18, or large-16-18-20")
	diagnostic := flags.Bool("diagnostic", false, "Record deterministic incumbent and plateau diagnostics (requires --workers 1)")
	operationProfile := flags.Bool("operation-profile", false, "Record deterministic rooted-packing operation counts (requires -tags searchprofile)")
	cpuProfile := flags.String("cpu-profile", "", "Write a CPU profile for this benchmark run")
	heapProfile := flags.String("heap-profile", "", "Write a heap profile after this benchmark run")
	constellationSeedV1 := flags.Bool("constellation-seed-v1", false, "Enable constellation seed v1")
	constellationSeedVariant := flags.String("constellation-seed-variant", "", "Constellation seed experiment: v1, v2, v3, v4, v5, v5.1, or general-search-v1")
	constellationFeasibilityProbe := flags.Bool("constellation-feasibility-probe", false, "Diagnose constellation root completion (requires --diagnostic)")
	constellationCompletionOptimizationProbe := flags.Bool("constellation-completion-optimization-probe", false, "Exactly optimize completed MRV constellation roots (requires --diagnostic)")
	constellationCandidatePoolFeasibilitySweep := flags.Bool("constellation-candidate-pool-feasibility-sweep", false, "Classify V4 constellation pool candidates (requires --diagnostic)")
	constellationCandidateCompletionOptimizationProbe := flags.Bool("constellation-candidate-completion-optimization-probe", false, "Exactly optimize one V4 constellation candidate (requires --diagnostic)")
	constellationCandidateCompletionOptimizationCandidateID := flags.String("constellation-candidate-completion-optimization-candidate-id", "", "Stable V4 candidate SHA-256 ID")
	constellationCandidateCompletionOptimizationStage := flags.String("constellation-candidate-completion-optimization-stage", "", "Optional target stage: single, prefix-5m, or remainder-15m")
	constellationCandidateCompletionOptimizationNodeBudget := flags.Int64("constellation-candidate-completion-optimization-node-budget", 0, "Dedicated diagnostic node budget for one V4 candidate")
	constellationCandidateCompletionOptimizationInitialWitnessLayoutKey := flags.String("constellation-candidate-completion-optimization-initial-witness-layout-key", "", "Validated lower-bound witness layout key")
	constellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint := flags.String("constellation-candidate-completion-optimization-initial-witness-semantic-fingerprint", "", "Semantic fingerprint paired with witness layout key")
	constellationForcedCandidateRootedPackingProbe := flags.Bool("constellation-forced-candidate-rooted-packing-probe", false, "Replay one V4 candidate through normal rooted packing (requires --diagnostic)")
	constellationForcedCandidateRootedPackingCandidateID := flags.String("constellation-forced-candidate-rooted-packing-candidate-id", "", "Stable V4 candidate SHA-256 ID")
	constellationForcedCandidateRootedPackingSlot := flags.Int("constellation-forced-candidate-rooted-packing-slot", 0, "Counterfactual root slot 1 through 4")
	constellationForcedCandidateRootedPackingStage := flags.String("constellation-forced-candidate-rooted-packing-stage", "", "Optional target stage: single, prefix-5m, or remainder-15m")
	constellationForcedCandidateRootedPackingBeamWidth := flags.Int("constellation-forced-candidate-rooted-packing-beam-width", 0, "Forced replay beam width; zero keeps normal V4 width")
	constellationForcedCandidateRootedPackingRanking := flags.String("constellation-forced-candidate-rooted-packing-ranking", "", "Forced replay ranking: baseline or priority-score-first")
	constellationForcedCandidateRootedPackingShadowWitnessLayoutKey := flags.String("constellation-forced-candidate-rooted-packing-shadow-witness-layout-key", "", "Shadow-only witness layout key")
	constellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint := flags.String("constellation-forced-candidate-rooted-packing-shadow-witness-semantic-fingerprint", "", "Semantic fingerprint paired with the shadow witness")
	constellationParentFrontierHedgeProbe := flags.Bool("constellation-parent-frontier-hedge-probe", false, "Replay the selected V5 parent-frontier family inside one diagnostic slot quota (requires --diagnostic)")
	constellationParentFrontierHedgeProbeStage := flags.String("constellation-parent-frontier-hedge-probe-stage", "", "Optional target stage: single, prefix-5m, or remainder-15m")
	out := flags.String("out", "", "Path to write benchmark JSON; stdout when empty")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	setFlags := explicitlySetFlags(flags)
	if *repeat <= 0 {
		fmt.Fprintln(stderr, "ERROR: --repeat must be positive")
		return 2
	}
	if *workers <= 0 {
		fmt.Fprintln(stderr, "ERROR: --workers must be positive")
		return 2
	}
	if *top <= 0 {
		fmt.Fprintln(stderr, "ERROR: --top must be positive")
		return 2
	}
	if *operationProfile && !solver.OperationProfilingAvailable() {
		fmt.Fprintln(stderr, "ERROR: operation profiling requires a binary built with -tags searchprofile")
		return 2
	}
	if *operationProfile && *diagnostic {
		fmt.Fprintln(stderr, "ERROR: --operation-profile and --diagnostic must be run separately")
		return 2
	}
	if *operationProfile && (*cpuProfile != "" || *heapProfile != "") {
		fmt.Fprintln(stderr, "ERROR: --operation-profile cannot be combined with --cpu-profile or --heap-profile")
		return 2
	}
	budgets, err := benchmark.ParseBudgets(*budgetsText)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if err := benchmark.ValidateRepairSearchMode(*repairSearchMode); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if err := benchmark.ValidatePlateauVariant(*plateauVariant); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if setFlags["constellation-seed-variant"] {
		if err := solver.ValidateConstellationSeedVariant(*constellationSeedVariant); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
	}
	if *constellationSeedV1 && *constellationSeedVariant != "" && *constellationSeedVariant != solver.ConstellationSeedVariantV1 {
		fmt.Fprintln(stderr, "ERROR: constellation seed v1 alias conflicts with explicit variant")
		return 2
	}
	if *constellationFeasibilityProbe && !*diagnostic {
		fmt.Fprintln(stderr, "ERROR: constellation feasibility probe requires --diagnostic")
		return 2
	}
	if *constellationCompletionOptimizationProbe && !*diagnostic {
		fmt.Fprintln(stderr, "ERROR: constellation completion optimization probe requires --diagnostic")
		return 2
	}
	if *constellationCompletionOptimizationProbe {
		if err := solver.ValidateConstellationCompletionOptimizationProbeVariant(*constellationSeedVariant); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
	}
	if *constellationFeasibilityProbe && *constellationCompletionOptimizationProbe {
		fmt.Fprintln(stderr, "ERROR: constellation feasibility and completion optimization probes cannot run together")
		return 2
	}
	if *constellationCandidatePoolFeasibilitySweep && !*diagnostic {
		fmt.Fprintln(stderr, "ERROR: constellation candidate pool feasibility sweep requires --diagnostic")
		return 2
	}
	if *constellationCandidatePoolFeasibilitySweep && *constellationSeedVariant != solver.ConstellationSeedVariantV4 {
		fmt.Fprintf(stderr, "ERROR: constellation candidate pool feasibility sweep requires --constellation-seed-variant %s\n", solver.ConstellationSeedVariantV4)
		return 2
	}
	if *constellationCandidatePoolFeasibilitySweep && (*constellationFeasibilityProbe || *constellationCompletionOptimizationProbe) {
		fmt.Fprintln(stderr, "ERROR: constellation candidate pool feasibility sweep cannot run with another constellation probe")
		return 2
	}
	if *constellationCandidateCompletionOptimizationProbe && !*diagnostic {
		fmt.Fprintln(stderr, "ERROR: constellation candidate completion optimization probe requires --diagnostic")
		return 2
	}
	if *constellationCandidateCompletionOptimizationProbe {
		if err := solver.ValidateConstellationCandidateCompletionOptimizationTarget(*constellationSeedVariant, *constellationCandidateCompletionOptimizationCandidateID, *constellationCandidateCompletionOptimizationStage); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
		if *constellationCandidateCompletionOptimizationNodeBudget < 0 {
			fmt.Fprintln(stderr, "ERROR: constellation candidate completion optimization node budget must be non-negative")
			return 2
		}
	}
	if *constellationCandidateCompletionOptimizationProbe && (*constellationFeasibilityProbe || *constellationCompletionOptimizationProbe || *constellationCandidatePoolFeasibilitySweep) {
		fmt.Fprintln(stderr, "ERROR: constellation candidate completion optimization probe cannot run with another constellation probe")
		return 2
	}
	if *constellationForcedCandidateRootedPackingProbe && !*diagnostic {
		fmt.Fprintln(stderr, "ERROR: constellation forced candidate rooted packing probe requires --diagnostic")
		return 2
	}
	if *constellationForcedCandidateRootedPackingProbe {
		if err := solver.ValidateConstellationForcedCandidateRootedPackingTarget(*constellationSeedVariant, *constellationForcedCandidateRootedPackingCandidateID, *constellationForcedCandidateRootedPackingSlot, *constellationForcedCandidateRootedPackingStage); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
		if (*constellationForcedCandidateRootedPackingShadowWitnessLayoutKey == "") != (*constellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint == "") {
			fmt.Fprintln(stderr, "ERROR: forced rooted packing shadow witness layout key and semantic fingerprint must be supplied together")
			return 2
		}
		if *constellationForcedCandidateRootedPackingBeamWidth < 0 {
			fmt.Fprintln(stderr, "ERROR: constellation forced candidate rooted packing beam width must be non-negative")
			return 2
		}
		if err := solver.ValidateConstellationForcedCandidateRootedPackingRanking(*constellationForcedCandidateRootedPackingRanking); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
	}
	if *constellationForcedCandidateRootedPackingProbe && (*constellationFeasibilityProbe || *constellationCompletionOptimizationProbe || *constellationCandidatePoolFeasibilitySweep || *constellationCandidateCompletionOptimizationProbe) {
		fmt.Fprintln(stderr, "ERROR: constellation forced candidate rooted packing probe cannot run with another constellation probe")
		return 2
	}
	if *constellationParentFrontierHedgeProbe && !*diagnostic {
		fmt.Fprintln(stderr, "ERROR: constellation parent-frontier hedge probe requires --diagnostic")
		return 2
	}
	if *constellationParentFrontierHedgeProbe {
		if err := solver.ValidateConstellationParentFrontierHedgeProbeTarget(*constellationSeedVariant, *constellationParentFrontierHedgeProbeStage); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
	}
	if *constellationParentFrontierHedgeProbe && (*constellationFeasibilityProbe || *constellationCompletionOptimizationProbe || *constellationCandidatePoolFeasibilitySweep || *constellationCandidateCompletionOptimizationProbe || *constellationForcedCandidateRootedPackingProbe) {
		fmt.Fprintln(stderr, "ERROR: constellation parent-frontier hedge probe cannot run with another constellation probe")
		return 2
	}

	var cpuProfileFile *os.File
	if *cpuProfile != "" {
		cpuProfileFile, err = os.Create(*cpuProfile)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: create CPU profile: %v\n", err)
			return 1
		}
		if err := pprof.StartCPUProfile(cpuProfileFile); err != nil {
			cpuProfileFile.Close()
			fmt.Fprintf(stderr, "ERROR: start CPU profile: %v\n", err)
			return 1
		}
	}
	report, err := benchmark.RunScenarios(benchmark.RunConfig{
		CatalogPath:                              *catalogPath,
		ScenarioDir:                              *dir,
		Scenarios:                                splitCSV(*scenariosText),
		Budgets:                                  budgets,
		Repeat:                                   *repeat,
		Workers:                                  *workers,
		Top:                                      *top,
		RepairSearchMode:                         *repairSearchMode,
		PlateauVariant:                           *plateauVariant,
		Diagnostic:                               *diagnostic,
		OperationProfiling:                       *operationProfile,
		ConstellationSeedV1:                      *constellationSeedV1,
		ConstellationSeedVariant:                 *constellationSeedVariant,
		ConstellationFeasibilityProbe:            *constellationFeasibilityProbe,
		ConstellationCompletionOptimizationProbe: *constellationCompletionOptimizationProbe,
		ConstellationCandidatePoolFeasibilitySweep:                                    *constellationCandidatePoolFeasibilitySweep,
		ConstellationCandidateCompletionOptimizationProbe:                             *constellationCandidateCompletionOptimizationProbe,
		ConstellationCandidateCompletionOptimizationCandidateID:                       *constellationCandidateCompletionOptimizationCandidateID,
		ConstellationCandidateCompletionOptimizationStage:                             *constellationCandidateCompletionOptimizationStage,
		ConstellationCandidateCompletionOptimizationNodeBudget:                        *constellationCandidateCompletionOptimizationNodeBudget,
		ConstellationCandidateCompletionOptimizationInitialWitnessLayoutKey:           *constellationCandidateCompletionOptimizationInitialWitnessLayoutKey,
		ConstellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint: *constellationCandidateCompletionOptimizationInitialWitnessSemanticFingerprint,
		ConstellationForcedCandidateRootedPackingProbe:                                *constellationForcedCandidateRootedPackingProbe,
		ConstellationForcedCandidateRootedPackingCandidateID:                          *constellationForcedCandidateRootedPackingCandidateID,
		ConstellationForcedCandidateRootedPackingSlot:                                 *constellationForcedCandidateRootedPackingSlot,
		ConstellationForcedCandidateRootedPackingStage:                                *constellationForcedCandidateRootedPackingStage,
		ConstellationForcedCandidateRootedPackingBeamWidth:                            *constellationForcedCandidateRootedPackingBeamWidth,
		ConstellationForcedCandidateRootedPackingRanking:                              *constellationForcedCandidateRootedPackingRanking,
		ConstellationForcedCandidateRootedPackingShadowWitnessLayoutKey:               *constellationForcedCandidateRootedPackingShadowWitnessLayoutKey,
		ConstellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint:     *constellationForcedCandidateRootedPackingShadowWitnessSemanticFingerprint,
		ConstellationParentFrontierHedgeProbe:                                         *constellationParentFrontierHedgeProbe,
		ConstellationParentFrontierHedgeProbeStage:                                    *constellationParentFrontierHedgeProbeStage,
	})
	if cpuProfileFile != nil {
		pprof.StopCPUProfile()
		if closeErr := cpuProfileFile.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close CPU profile: %w", closeErr)
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if *heapProfile != "" {
		runtime.GC()
		heapProfileFile, profileErr := os.Create(*heapProfile)
		if profileErr == nil {
			profileErr = pprof.WriteHeapProfile(heapProfileFile)
			if closeErr := heapProfileFile.Close(); profileErr == nil && closeErr != nil {
				profileErr = closeErr
			}
		}
		if profileErr != nil {
			fmt.Fprintf(stderr, "ERROR: write heap profile: %v\n", profileErr)
			return 1
		}
	}
	content, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if *out == "" {
		fmt.Fprintln(stdout, string(content))
		return 0
	}
	if err := os.WriteFile(*out, append(content, '\n'), 0o600); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote %d benchmark runs to %s\n", len(report.Runs), *out)
	return 0
}

func runCompareBenchmarks(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("compare-benchmarks", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 2 {
		fmt.Fprintln(stderr, "ERROR: compare-benchmarks expects baseline and current JSON files")
		return 2
	}

	baseline, err := benchmark.LoadReport(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	current, err := benchmark.LoadReport(flags.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	comparison, err := benchmark.CompareReports(baseline, current)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	benchmark.FormatComparison(stdout, comparison)
	if comparison.ScoreLosses > 0 {
		return 1
	}
	return 0
}

func runCompareConstellationBenchmarks(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("compare-constellation-benchmarks", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "Optional path for comparison JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 2 {
		fmt.Fprintln(stderr, "ERROR: compare-constellation-benchmarks expects baseline and current JSON files")
		return 2
	}
	baseline, err := benchmark.LoadReport(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	current, err := benchmark.LoadReport(flags.Arg(1))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	comparison, err := benchmark.CompareConstellationExperimentReports(baseline, current)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	benchmark.FormatConstellationExperimentComparison(stdout, comparison)
	if *out != "" {
		content, err := json.MarshalIndent(comparison, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
		if err := os.WriteFile(*out, append(content, '\n'), 0o600); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Wrote constellation comparison to %s\n", *out)
	}
	if comparison.ScoreLosses > 0 {
		return 1
	}
	return 0
}

func runSummarizeOperationProfile(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("summarize-operation-profile", flag.ContinueOnError)
	flags.SetOutput(stderr)
	out := flags.String("out", "", "Optional path for deterministic summary JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "ERROR: summarize-operation-profile expects one benchmark report JSON file")
		return 2
	}
	report, err := benchmark.LoadReport(flags.Arg(0))
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	summary := benchmark.SummarizeOperationProfile(report)
	benchmark.FormatOperationProfileSummary(stdout, summary)
	if *out == "" {
		return 0
	}
	content, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if err := os.WriteFile(*out, append(content, '\n'), 0o600); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Wrote operation profile summary to %s\n", *out)
	return 0
}

func runMaterializeSearchSuite(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("materialize-search-suite", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", filepath.Join("benchmarks", "suites", "general-search-v1.json"), "Path to suite manifest")
	lockPath := flags.String("lock", filepath.Join("benchmarks", "suites", "general-search-v1.lock"), "Path to immutable suite lock")
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	outDir := flags.String("out", "", "Output directory for public generated scenarios")
	rolesText := flags.String("roles", "development,validation", "Comma-separated suite roles")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *outDir == "" {
		fmt.Fprintln(stderr, "ERROR: materialize-search-suite requires --out")
		return 2
	}
	if err := benchmark.VerifySearchSuiteLock(*manifestPath, *catalogPath, *lockPath); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	manifest, err := benchmark.LoadSearchSuiteManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	loadedCatalog, err := catalog.Load(*catalogPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	lock, err := benchmark.LoadSearchSuiteLock(*lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	generated, err := benchmark.MaterializeSearchSuiteCases(lock.GeneratorVersion, loadedCatalog, manifest, splitCSV(*rolesText)...)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	for _, generatedScenario := range generated {
		content, err := benchmark.MarshalSearchSuiteScenario(generatedScenario)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
		path := filepath.Join(*outDir, generatedScenario.Name+".json")
		if err := os.WriteFile(path, content, 0o600); err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
	}
	fmt.Fprintf(stdout, "Materialized %d public suite scenarios to %s\n", len(generated), *outDir)
	return 0
}

func runFreezeSearchSuite(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("freeze-search-suite", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", filepath.Join("benchmarks", "suites", "general-search-v1.json"), "Path to suite manifest")
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	generatorVersion := flags.String("generator-version", "", "Explicit historical search suite generator version")
	outPath := flags.String("out", "", "Path for new immutable suite lock")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *outPath == "" {
		fmt.Fprintln(stderr, "ERROR: freeze-search-suite requires --out")
		return 2
	}
	if *generatorVersion == "" {
		fmt.Fprintln(stderr, "ERROR: freeze-search-suite requires --generator-version")
		return 2
	}
	lock, err := benchmark.ObserveSearchSuite(*manifestPath, *catalogPath, *generatorVersion)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	if err := benchmark.WriteSearchSuiteLock(*outPath, lock); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Suite frozen: %s\n", lock.SuiteName)
	fmt.Fprintf(stdout, "Wrote immutable lock to %s\n", *outPath)
	return 0
}

func runVerifySearchSuite(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("verify-search-suite", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", filepath.Join("benchmarks", "suites", "general-search-v1.json"), "Path to suite manifest")
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	lockPath := flags.String("lock", filepath.Join("benchmarks", "suites", "general-search-v1.lock"), "Path to immutable suite lock")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if err := benchmark.VerifySearchSuiteLock(*manifestPath, *catalogPath, *lockPath); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	lock, err := benchmark.LoadSearchSuiteLock(*lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Suite verified: %s\n", lock.SuiteName)
	fmt.Fprintf(stdout, "%d static cases\n", len(lock.StaticCases))
	fmt.Fprintf(stdout, "%d public generated cases\n", len(lock.GeneratedCases))
	fmt.Fprintf(stdout, "%d private holdout declarations\n", len(lock.PrivateCases))
	fmt.Fprintf(stdout, "catalog %s\n", lock.CatalogSHA256)
	fmt.Fprintf(stdout, "generator %s\n", lock.GeneratorVersion)
	return 0
}

func runVerifyPrivateSearchSuite(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("verify-private-search-suite", flag.ContinueOnError)
	flags.SetOutput(stderr)
	manifestPath := flags.String("manifest", filepath.Join("benchmarks", "suites", "general-search-v2.json"), "Path to v2 suite manifest")
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	lockPath := flags.String("lock", filepath.Join("benchmarks", "suites", "general-search-v2.lock"), "Path to immutable v2 suite lock")
	seedsPath := flags.String("seeds", "", "Path to protected JSON object mapping private seed IDs to int64 seeds")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *seedsPath == "" {
		fmt.Fprintln(stderr, "ERROR: verify-private-search-suite requires --seeds")
		return 2
	}
	if err := benchmark.VerifySearchSuiteLock(*manifestPath, *catalogPath, *lockPath); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	lock, err := benchmark.LoadSearchSuiteLock(*lockPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if lock.GeneratorVersion != benchmark.SearchSuiteGeneratorV2 {
		fmt.Fprintf(stderr, "ERROR: verify-private-search-suite requires generator %s\n", benchmark.SearchSuiteGeneratorV2)
		return 2
	}
	privateSeeds, err := loadPrivateSearchSuiteSeeds(*seedsPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	manifest, err := benchmark.LoadSearchSuiteManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	loadedCatalog, err := catalog.Load(*catalogPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if err := benchmark.VerifySearchSuiteV2PrivateHoldouts(loadedCatalog, manifest, privateSeeds); err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "Private v2 holdouts verified: %d\n", len(privateSeeds))
	return 0
}

func loadPrivateSearchSuiteSeeds(path string) (map[string]int64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(content)))
	decoder.DisallowUnknownFields()
	seeds := map[string]int64{}
	if err := decoder.Decode(&seeds); err != nil {
		return nil, fmt.Errorf("decode private seeds: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("private seeds contains unexpected trailing JSON value")
		}
		return nil, fmt.Errorf("decode private seed trailing data: %w", err)
	}
	return seeds, nil
}

func runReviewCatalog(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("review-catalog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	loaded, err := catalog.Load(*catalogPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, render.CatalogReviewText(loaded))
	return 0
}

func runValidateCatalog(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("validate-catalog", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	loaded, err := catalog.Load(*catalogPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	warnings, errors := catalog.Validate(loaded)
	for _, warning := range warnings {
		fmt.Fprintf(stdout, "WARNING: %s\n", warning)
	}
	for _, validationError := range errors {
		fmt.Fprintf(stdout, "ERROR: %s\n", validationError)
	}
	if len(errors) > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "Catalog valid: %d items, %d recipes\n", len(loaded.Items), len(loaded.Recipes))
	return 0
}

func runSolve(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("solve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	catalogPath := flags.String("catalog", catalog.DefaultPath, "Path to catalog JSON")
	scenarioPath := flags.String("scenario", "", "Path to a scenario JSON file")
	itemsSpec := flags.String("items", "", "Comma-separated item ids, with optional id:count syntax")
	gridText := flags.String("grid", "", "Inline 6x9 grid, rows separated by / or newlines")
	gridFile := flags.String("grid-file", "", "Path to a text file containing a 6x9 grid")
	top := flags.Int("top", 3, "Number of solutions to print")
	maxNodes := flags.Int64("max-nodes", 200000, "Search node limit; 0 means no limit")
	noSkips := flags.Bool("no-skips", false, "Require every inventory item to be placed")
	stopOnCoverageCeiling := flags.Bool("stop-on-coverage-ceiling", false, "Stop early when the top coverage bucket reaches its theoretical ceiling")
	repairSearch := flags.Bool("repair-search", true, "Run deterministic repair search for limited solves")
	hero := flags.String("hero", "", "Include items available to this hero or comma-separated heroes")
	excludeHero := flags.String("exclude-hero", "", "Exclude items available to this hero or comma-separated heroes")
	heroMode := flags.String("hero-mode", "", "Hero filter mode: any, all, or shared")
	excludeMode := flags.String("hero-exclude-mode", "", "Hero exclusion mode: strict or exclusive_only")
	unknownHeroPolicy := flags.String("hero-unknown-policy", "", "Unknown hero scope policy: exclude, include, or error")
	jsonOutput := flags.Bool("json", false, "Print machine-readable JSON")
	workers := flags.Int("workers", runtime.NumCPU(), "Number of search workers")
	operationProfile := flags.Bool("operation-profile", false, "Record deterministic rooted-packing operation counts (requires -tags searchprofile and --workers 1)")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	setFlags := explicitlySetFlags(flags)
	itemIDs, effectiveGridText, effectiveGridFile, effectiveTop, effectiveMaxNodes, effectiveNoSkips, effectiveStopOnCoverageCeiling, effectiveRepairSearch, effectiveWorkers, effectivePriorities, effectiveCoverageGroups, err := solveInputsFromFlags(
		*scenarioPath,
		*itemsSpec,
		*gridText,
		*gridFile,
		*top,
		*maxNodes,
		*noSkips,
		*stopOnCoverageCeiling,
		*repairSearch,
		*workers,
		setFlags,
	)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	prioritySemantics := model.PrioritySemanticsLegacyIncomingV1
	if *scenarioPath != "" {
		loadedScenario, err := scenario.Load(*scenarioPath)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 2
		}
		if loadedScenario.PrioritySemantics != "" {
			prioritySemantics = loadedScenario.PrioritySemantics
		}
	}

	if effectiveTop <= 0 {
		fmt.Fprintln(stderr, "ERROR: --top must be positive")
		return 2
	}
	if effectiveWorkers <= 0 {
		fmt.Fprintln(stderr, "ERROR: --workers must be positive")
		return 2
	}
	if *operationProfile && !solver.OperationProfilingAvailable() {
		fmt.Fprintln(stderr, "ERROR: operation profiling requires a binary built with -tags searchprofile")
		return 2
	}
	if *operationProfile && effectiveWorkers != 1 {
		fmt.Fprintln(stderr, "ERROR: operation profiling requires --workers 1")
		return 2
	}
	if effectiveMaxNodes < 0 {
		fmt.Fprintln(stderr, "ERROR: --max-nodes must be non-negative")
		return 2
	}

	loaded, err := catalog.Load(*catalogPath)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	heroFilter, err := effectiveHeroFilter(*scenarioPath, *hero, *excludeHero, *heroMode, *excludeMode, *unknownHeroPolicy, setFlags)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	loaded, err = catalog.FilterForHeroes(loaded, heroFilter)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}
	for _, itemID := range itemIDs {
		if _, ok := loaded.Items[itemID]; !ok {
			fmt.Fprintf(stderr, "ERROR: item %q is unavailable for the selected hero filter\n", itemID)
			return 2
		}
	}
	gridMask, err := gridMaskFromArgs(effectiveGridText, effectiveGridFile)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}

	startedAt := time.Now()
	solutions, err := solver.SolveLayout(loaded, itemIDs, gridMask, solver.Config{
		TopN:                  effectiveTop,
		AllowSkips:            !effectiveNoSkips,
		MaxNodes:              effectiveMaxNodes,
		Workers:               effectiveWorkers,
		PrioritySemantics:     prioritySemantics,
		Priorities:            effectivePriorities,
		CoverageGroups:        effectiveCoverageGroups,
		StopOnCoverageCeiling: effectiveStopOnCoverageCeiling,
		RepairSearch:          effectiveRepairSearch && effectiveMaxNodes > 0,
		OperationProfiling:    *operationProfile,
	})
	elapsed := time.Since(startedAt)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}

	if *jsonOutput {
		content, err := render.SolutionsJSON(solutions)
		if err != nil {
			fmt.Fprintf(stderr, "ERROR: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, string(content))
		return 0
	}

	if len(solutions) == 0 {
		fmt.Fprintf(stdout, "Solved in %s\n", formatDuration(elapsed))
		fmt.Fprintln(stdout, "No solutions found.")
		return 1
	}
	applyNodesPerSecond(solutions, elapsed)
	fmt.Fprintf(stdout, "Solved in %s\n", formatDuration(elapsed))
	for idx, solution := range solutions {
		fmt.Fprintf(stdout, "=== Solution %d ===\n", idx+1)
		fmt.Fprintln(stdout, render.SolutionText(solution, gridMask))
	}
	return 0
}

func formatDuration(duration time.Duration) string {
	if duration < time.Second {
		return fmt.Sprintf("%d ms", duration.Milliseconds())
	}
	return fmt.Sprintf("%.2f s", duration.Seconds())
}

func applyNodesPerSecond(solutions []model.Solution, elapsed time.Duration) {
	if elapsed <= 0 {
		return
	}
	for idx := range solutions {
		if solutions[idx].Search.NodesExplored == 0 {
			continue
		}
		solutions[idx].Search.NodesPerSecond = float64(solutions[idx].Search.NodesExplored) / elapsed.Seconds()
	}
}

func solveInputsFromFlags(
	scenarioPath string,
	itemsSpec string,
	gridText string,
	gridFile string,
	top int,
	maxNodes int64,
	noSkips bool,
	stopOnCoverageCeiling bool,
	repairSearch bool,
	workers int,
	setFlags map[string]bool,
) ([]string, string, string, int, int64, bool, bool, bool, int, []string, []model.CoverageGroup, error) {
	effectiveItemsSpec := itemsSpec
	effectiveGridText := gridText
	effectiveGridFile := gridFile
	effectiveTop := top
	effectiveMaxNodes := maxNodes
	effectiveNoSkips := noSkips
	effectiveStopOnCoverageCeiling := stopOnCoverageCeiling
	effectiveRepairSearch := repairSearch
	effectiveWorkers := workers
	var effectivePriorities []string
	var effectiveCoverageGroups []model.CoverageGroup

	if scenarioPath != "" {
		loadedScenario, err := scenario.Load(scenarioPath)
		if err != nil {
			return nil, "", "", 0, 0, false, false, false, 0, nil, nil, err
		}
		effectivePriorities = append([]string(nil), loadedScenario.Priorities...)
		effectiveCoverageGroups = loadedScenario.ModelCoverageGroups()
		if !setFlags["grid"] && !setFlags["grid-file"] && len(loadedScenario.Grid) > 0 {
			effectiveGridText = loadedScenario.GridText()
			effectiveGridFile = ""
		}
		if !setFlags["top"] && loadedScenario.Top != nil {
			effectiveTop = *loadedScenario.Top
		}
		if !setFlags["max-nodes"] && loadedScenario.MaxNodes != nil {
			effectiveMaxNodes = *loadedScenario.MaxNodes
		}
		if !setFlags["no-skips"] && loadedScenario.NoSkips != nil {
			effectiveNoSkips = *loadedScenario.NoSkips
		}
		if !setFlags["stop-on-coverage-ceiling"] && loadedScenario.StopOnCoverageCeiling != nil {
			effectiveStopOnCoverageCeiling = *loadedScenario.StopOnCoverageCeiling
		}
		if !setFlags["repair-search"] && loadedScenario.RepairSearch != nil {
			effectiveRepairSearch = *loadedScenario.RepairSearch
		}
		if !setFlags["workers"] && loadedScenario.Workers != nil {
			effectiveWorkers = *loadedScenario.Workers
		}
		if !setFlags["items"] {
			return loadedScenario.ItemIDs(), effectiveGridText, effectiveGridFile, effectiveTop, effectiveMaxNodes, effectiveNoSkips, effectiveStopOnCoverageCeiling, effectiveRepairSearch, effectiveWorkers, effectivePriorities, effectiveCoverageGroups, nil
		}
	}

	if effectiveItemsSpec == "" {
		return nil, "", "", 0, 0, false, false, false, 0, nil, nil, fmt.Errorf("--items is required unless --scenario is provided")
	}
	itemIDs, err := solver.ParseInventorySpec(effectiveItemsSpec)
	if err != nil {
		return nil, "", "", 0, 0, false, false, false, 0, nil, nil, err
	}
	return itemIDs, effectiveGridText, effectiveGridFile, effectiveTop, effectiveMaxNodes, effectiveNoSkips, effectiveStopOnCoverageCeiling, effectiveRepairSearch, effectiveWorkers, effectivePriorities, effectiveCoverageGroups, nil
}

func explicitlySetFlags(flags *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	flags.Visit(func(flag *flag.Flag) {
		set[flag.Name] = true
	})
	return set
}

func effectiveHeroFilter(scenarioPath, heroText, excludeText, mode, excludeMode, unknownPolicy string, setFlags map[string]bool) (model.HeroFilter, error) {
	filter := model.HeroFilter{}
	if scenarioPath != "" {
		loadedScenario, err := scenario.Load(scenarioPath)
		if err != nil {
			return model.HeroFilter{}, err
		}
		filter = loadedScenario.HeroFilter
	}
	if setFlags["hero"] {
		filter.IncludeHeroes = splitHeroIDs(heroText)
	}
	if setFlags["exclude-hero"] {
		filter.ExcludeHeroes = splitHeroIDs(excludeText)
	}
	if setFlags["hero-mode"] {
		filter.Mode = mode
	}
	if setFlags["hero-exclude-mode"] {
		filter.ExcludeMode = excludeMode
	}
	if setFlags["hero-unknown-policy"] {
		filter.UnknownPolicy = unknownPolicy
	}
	return filter, nil
}

func splitHeroIDs(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		seen[part] = true
		result = append(result, part)
	}
	return result
}

func runImportWiki(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("import-wiki", flag.ContinueOnError)
	flags.SetOutput(stderr)
	if err := flags.Parse(args); err != nil {
		return 2
	}
	fmt.Fprintln(stdout, "Wiki import is a planned helper, not required by the v1 solver.")
	fmt.Fprintln(stdout, "Direct wiki.gg access can be blocked by Cloudflare, so imported data must be reviewed.")
	if flags.NArg() > 0 {
		fmt.Fprintln(stdout, "Requested URLs:")
		for _, url := range flags.Args() {
			fmt.Fprintf(stdout, "  %s\n", url)
		}
	}
	fmt.Fprintln(stdout, "For now, add or edit items in data/catalog.json and run validate-catalog.")
	return 0
}

func runImportHTML(args []string, stdout io.Writer, stderr io.Writer) int {
	flags := flag.NewFlagSet("import-html", flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := flags.Bool("json", false, "Print machine-readable JSON instead of review text")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() < 1 {
		fmt.Fprintln(stderr, "ERROR: import-html expects one or more saved HTML files or directories")
		return 2
	}

	proposals, err := wikihtml.ExtractPaths(flags.Args())
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	if !*jsonOutput {
		fmt.Fprintln(stdout, wikihtml.ReviewText(proposals))
		return 0
	}

	var content []byte
	if len(proposals) == 1 {
		content, err = wikihtml.MarshalProposal(proposals[0])
	} else {
		content, err = wikihtml.MarshalProposals(proposals)
	}
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 1
	}
	fmt.Fprintln(stdout, string(content))
	return 0
}

func gridMaskFromArgs(gridText string, gridFile string) (uint64, error) {
	if gridFile != "" {
		content, err := os.ReadFile(gridFile)
		if err != nil {
			return 0, err
		}
		return geometry.ParseGridText(string(content))
	}
	if gridText != "" {
		return geometry.ParseGridText(gridText)
	}
	return geometry.FullGridMask(), nil
}

func printUsage(writer io.Writer) {
	fmt.Fprintln(writer, "Usage: backpack-brawl-solver <command> [options]")
	fmt.Fprintln(writer, "")
	fmt.Fprintln(writer, "Commands:")
	fmt.Fprintln(writer, "  validate-catalog")
	fmt.Fprintln(writer, "  review-catalog")
	fmt.Fprintln(writer, "  solve")
	fmt.Fprintln(writer, "    --hero <id[,id]> --exclude-hero <id[,id]>")
	fmt.Fprintln(writer, "  import-wiki")
	fmt.Fprintln(writer, "  import-html")
	fmt.Fprintln(writer, "  benchmark-scenarios")
	fmt.Fprintln(writer, "  compare-benchmarks")
	fmt.Fprintln(writer, "  compare-constellation-benchmarks")
	fmt.Fprintln(writer, "  summarize-operation-profile")
	fmt.Fprintln(writer, "  freeze-search-suite")
	fmt.Fprintln(writer, "  verify-search-suite")
	fmt.Fprintln(writer, "  verify-private-search-suite")
	fmt.Fprintln(writer, "  materialize-search-suite")
}

func splitCSV(value string) []string {
	var values []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
