package benchmark

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"backpack-brawl-solver/internal/model"
)

func TestSummarizeOperationProfileGroupsAndDerivesSchedulerTelemetry(t *testing.T) {
	profile := &model.ConstellationRootPackingOperationProfile{
		Version:                 "root-packing-ops-v1",
		CandidateExpansions:     10,
		MRVOptionChecks:         40,
		FeasibilityOptionChecks: 70,
		PlacementElementsCopied: 30,
		StateKeyBytes:           120,
	}
	report := Report{Runs: []Run{
		{
			Scenario: "alpha",
			Budget:   1_000,
			Search: SearchSummary{ConstellationSeedDiagnostics: &model.ConstellationSeedDiagnostics{
				RootPackingOperationProfile: profile,
				Roots: []model.ConstellationRootDiagnostic{{
					FamilyID:                "single/root-a",
					FamilyTotalConsumed:     8,
					FamilyTerminationReason: "completed",
					FirstCompleteNodes:      5,
					MRVDepths:               []model.ConstellationRootPackingDepthDiagnostic{{Depth: 3}},
					FamilyAllocationRounds:  []model.ConstellationRootPackingAllocationRound{{Round: 1, Reserved: 10, Consumed: 8, Returned: 2}},
					OperationProfile:        profile,
				}},
			}},
		},
		{
			Scenario: "beta",
			Budget:   1_000,
			Search: SearchSummary{ConstellationSeedDiagnostics: &model.ConstellationSeedDiagnostics{
				RootPackingOperationProfile: profile,
			}},
		},
	}}

	summary := SummarizeOperationProfile(report)
	if len(summary.Scenarios) != 2 || summary.Scenarios[0].Scenario != "alpha" || summary.Scenarios[1].Scenario != "beta" {
		t.Fatalf("scenario groups=%+v", summary.Scenarios)
	}
	alpha := summary.Scenarios[0]
	if alpha.RootedPacking == nil || alpha.RootedPacking.CandidateExpansions != 10 || alpha.PerCandidateExpansion == nil || alpha.PerCandidateExpansion.FeasibilityOptionChecks != 7 {
		t.Fatalf("alpha operation summary=%+v", alpha)
	}
	if alpha.Scheduler == nil || alpha.Scheduler.FamilyCount != 1 || alpha.Scheduler.FamiliesCompleted != 1 || alpha.Scheduler.ReturnedCapacityTotal != 2 || alpha.Scheduler.ReturnedFractionBPS != 2_000 || alpha.Scheduler.FinalDepth.P50 != 3 {
		t.Fatalf("alpha scheduler summary=%+v", alpha.Scheduler)
	}
	if summary.Aggregate.RootedPacking == nil || summary.Aggregate.RootedPacking.CandidateExpansions != 20 {
		t.Fatalf("aggregate=%+v", summary.Aggregate)
	}
	var formatted strings.Builder
	FormatOperationProfileSummary(&formatted, summary)
	if !strings.Contains(formatted.String(), "returned capacity") || !strings.Contains(formatted.String(), "Rooted packing operations") {
		t.Fatalf("formatted summary=%q", formatted.String())
	}
}

func TestP0ProfileSetUsesOnlyFrozenV2DevelopmentCases(t *testing.T) {
	profileSetPath := filepath.Join("..", "..", "benchmarks", "profiling", "p0-profile-set.json")
	content, err := os.ReadFile(profileSetPath)
	if err != nil {
		t.Fatal(err)
	}
	var profileSet struct {
		Suite            string   `json:"suite"`
		AllowedRole      string   `json:"allowed_role"`
		OperationCaseIDs []string `json:"operation_case_ids"`
		CPUHeapCaseIDs   []string `json:"cpu_heap_case_ids"`
		OperationBudgets []int64  `json:"operation_budgets"`
		SchedulerBudgets []int64  `json:"scheduler_budgets"`
		CPUHeapBudget    int64    `json:"cpu_heap_budget"`
		Workers          int      `json:"workers"`
	}
	if err := json.Unmarshal(content, &profileSet); err != nil {
		t.Fatal(err)
	}
	if profileSet.Suite != "general-search-v2" || profileSet.AllowedRole != SuiteRoleDevelopment || profileSet.Workers != 1 || len(profileSet.OperationCaseIDs) != 14 || len(profileSet.CPUHeapCaseIDs) != 6 || profileSet.CPUHeapBudget != 1_000_000 {
		t.Fatalf("profile set=%+v", profileSet)
	}
	manifest, err := LoadSearchSuiteManifest(filepath.Join("..", "..", "benchmarks", "suites", "general-search-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	rolesByID := make(map[string]string, len(manifest.Generated))
	for _, generated := range manifest.Generated {
		rolesByID[generated.ID] = generated.Role
	}
	for _, id := range append(profileSet.OperationCaseIDs, profileSet.CPUHeapCaseIDs...) {
		if rolesByID[id] != SuiteRoleDevelopment {
			t.Fatalf("profile case %q role=%q, want %q", id, rolesByID[id], SuiteRoleDevelopment)
		}
	}
}
