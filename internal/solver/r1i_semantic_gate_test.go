package solver

import (
	"encoding/json"
	"os"
	"testing"

	"backpack-brawl-solver/internal/geometry"
	"backpack-brawl-solver/internal/model"
)

const r1iSemanticSnapshotPathEnv = "R1I_SEMANTIC_SNAPSHOT"

// TestR1ICrossBuildSemanticSnapshot is invoked twice by CI: once with the
// normal build and once with searchprofile compiled in but profiling disabled.
// The tagged OFF/ON half of the three-way gate remains in
// TestOperationProfilingPreservesFullGeneralSearchPipeline.
func TestR1ICrossBuildSemanticSnapshot(t *testing.T) {
	outputPath := os.Getenv(r1iSemanticSnapshotPathEnv)
	if outputPath == "" {
		t.Skipf("set %s to emit the cross-build semantic snapshot", r1iSemanticSnapshotPathEnv)
	}
	catalog := coverageCeilingTestCatalog()
	items := []string{"left_source", "right_source", "weapon", "weapon", "weapon", "weapon"}
	solutions, err := SolveLayout(catalog, items, geometry.FullGridMask(), Config{
		TopN:                     1,
		AllowSkips:               false,
		MaxNodes:                 200_000,
		Workers:                  1,
		Diagnostics:              true,
		RepairSearch:             false,
		ConstellationSeedVariant: ConstellationSeedVariantGeneralSearchV1,
		PrioritySemantics:        model.PrioritySemanticsOutgoingPerInstanceV3,
		Priorities:               []string{"star_source:left_source", "star_source:right_source"},
	})
	if err != nil {
		t.Fatalf("SolveLayout: %v", err)
	}
	if len(solutions) != 1 {
		t.Fatalf("solutions=%d want 1", len(solutions))
	}
	snapshot, err := json.MarshalIndent(operationProfileSemanticSolutionForTest(solutions[0]), "", "  ")
	if err != nil {
		t.Fatalf("marshal semantic snapshot: %v", err)
	}
	if err := os.WriteFile(outputPath, append(snapshot, '\n'), 0o600); err != nil {
		t.Fatalf("write semantic snapshot: %v", err)
	}
}

func operationProfileSemanticSolutionForTest(solution model.Solution) model.Solution {
	copy := solution
	search := solution.Search
	search.NodesPerSecond = 0
	search.SetupMS = 0
	search.SeedMS = 0
	search.RepairMS = 0
	search.SearchMS = 0
	search.RefineMS = 0
	search.ServerElapsedMS = 0
	search.FirstCompleteMS = 0
	search.FirstFullyPackedMS = 0
	search.PackingSeedOperationProfile = nil
	search.BoundOperationProfile = nil
	search.IncumbentTrace = append([]model.IncumbentEvent(nil), search.IncumbentTrace...)
	for index := range search.IncumbentTrace {
		search.IncumbentTrace[index].ElapsedMS = 0
	}
	search.Stages = append([]model.SearchStageStats(nil), search.Stages...)
	for stageIndex := range search.Stages {
		search.Stages[stageIndex].IncumbentTrace = append([]model.IncumbentEvent(nil), search.Stages[stageIndex].IncumbentTrace...)
		for eventIndex := range search.Stages[stageIndex].IncumbentTrace {
			search.Stages[stageIndex].IncumbentTrace[eventIndex].ElapsedMS = 0
		}
	}
	diagnostics := search.ConstellationSeedDiagnostics
	diagnostics.RootPackingOperationProfile = nil
	diagnostics.Roots = append([]model.ConstellationRootDiagnostic(nil), diagnostics.Roots...)
	for index := range diagnostics.Roots {
		diagnostics.Roots[index].OperationProfile = nil
	}
	search.ConstellationSeedDiagnostics = diagnostics
	copy.Search = search
	return copy
}
