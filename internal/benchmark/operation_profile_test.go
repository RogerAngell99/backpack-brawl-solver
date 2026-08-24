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
	packingSeedProfile := &model.PackingSeedFeasibilityOperationProfile{
		Version:                        "packing-seed-feasibility-ops-v1",
		SearchCalls:                    1,
		StatesVisited:                  10,
		CandidateOptionChecks:          40,
		CandidateOverlapRejects:        10,
		CandidateChargeAttempts:        21,
		CandidateChargeDenied:          1,
		CandidateExpansions:            20,
		FeasibilityCalls:               30,
		FeasibilityInstancesConsidered: 60,
		FeasibilityOptionChecks:        90,
		FeasibilityOverlapRejects:      20,
		FeasibilityLegalPlacements:     65,
		FeasibilityDeadReturns:         5,
		CandidateCanonical: model.PackingSeedCanonicalCopyOrderOperationProfile{
			Calls: 30, PlacementKeyCalls: 35, SameItemComparisons: 5, PlacementKeyBytes: 200,
		},
		FeasibilityCanonical: model.PackingSeedCanonicalCopyOrderOperationProfile{
			Calls: 70, Rejects: 5, PlacementKeyCalls: 95, SameItemComparisons: 25, PlacementKeyBytes: 1_000,
		},
	}
	report := Report{Runs: []Run{
		{
			Scenario: "alpha",
			Budget:   1_000,
			Search: SearchSummary{PackingSeedOperationProfile: packingSeedProfile, ConstellationSeedDiagnostics: &model.ConstellationSeedDiagnostics{
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
	if summary.Version != "operation-profile-summary-v3" || alpha.PackingSeedFeasibility == nil || alpha.PackingSeedFeasibility.FeasibilityOptionChecks != 90 || alpha.PackingSeedFeasibilityDerived == nil || alpha.PackingSeedFeasibilityDerived.FeasibilityCallsPerState != 3 || alpha.PackingSeedFeasibilityDerived.PlacementKeyBytesPerCandidateExpansion != 60 || alpha.PackingSeedFeasibilityDerived.FeasibilityCanonicalCallsPerOptionCheck != float64(7)/9 || alpha.PackingSeedFeasibilityDerived.FeasibilityCandidatePlacementKeyCallsPerCanonicalCall != 1 {
		t.Fatalf("packing-seed summary=%+v", alpha)
	}
	if alpha.Scheduler == nil || alpha.Scheduler.FamilyCount != 1 || alpha.Scheduler.FamiliesCompleted != 1 || alpha.Scheduler.ReturnedCapacityTotal != 2 || alpha.Scheduler.ReturnedFractionBPS != 2_000 || alpha.Scheduler.FinalDepth.P50 != 3 {
		t.Fatalf("alpha scheduler summary=%+v", alpha.Scheduler)
	}
	if summary.Aggregate.RootedPacking == nil || summary.Aggregate.RootedPacking.CandidateExpansions != 20 {
		t.Fatalf("aggregate=%+v", summary.Aggregate)
	}
	var formatted strings.Builder
	FormatOperationProfileSummary(&formatted, summary)
	if !strings.Contains(formatted.String(), "returned capacity") || !strings.Contains(formatted.String(), "Rooted packing operations") || !strings.Contains(formatted.String(), "Packing-seed feasibility") {
		t.Fatalf("formatted summary=%q", formatted.String())
	}
}

func TestSummarizeOperationProfileV3ReadsP0V1StyleReports(t *testing.T) {
	report := Report{Runs: []Run{{
		Scenario: "p0-fixture",
		Budget:   1_000,
		Search: SearchSummary{ConstellationSeedDiagnostics: &model.ConstellationSeedDiagnostics{
			RootPackingOperationProfile: &model.ConstellationRootPackingOperationProfile{Version: "root-packing-ops-v1", CandidateExpansions: 1},
		}},
	}}}
	summary := SummarizeOperationProfile(report)
	if summary.Version != "operation-profile-summary-v3" || summary.Scenarios[0].RootedPacking == nil || summary.Scenarios[0].PackingSeedFeasibility != nil || summary.Scenarios[0].PackingSeedFeasibilityDerived != nil {
		t.Fatalf("v1 compatibility summary=%+v", summary)
	}
}

func TestSummarizeOperationProfileSeparatesPackingSeedSchemaVersions(t *testing.T) {
	v1 := &model.PackingSeedFeasibilityOperationProfile{
		Version: model.PackingSeedFeasibilityProfileVersionV1,
		CandidateCanonical: model.PackingSeedCanonicalCopyOrderOperationProfile{
			Calls: 5, SameItemComparisons: 2, PlacementKeyCalls: 7,
		},
	}
	v2 := &model.PackingSeedFeasibilityOperationProfile{
		Version: model.PackingSeedFeasibilityProfileVersionV2,
		CandidateCanonical: model.PackingSeedCanonicalCopyOrderOperationProfile{
			Calls: 5, SameItemComparisons: 2, CandidatePlacementKeyCalls: 1, PlacementKeyCalls: 3,
		},
	}
	report := Report{Runs: []Run{
		{Scenario: "mixed", Budget: 1_000, Search: SearchSummary{PackingSeedOperationProfile: v1}},
		{Scenario: "mixed", Budget: 1_000, Search: SearchSummary{PackingSeedOperationProfile: v2}},
	}}
	summary := SummarizeOperationProfile(report)
	entry := summary.Scenarios[0]
	if entry.PackingSeedFeasibility != nil || entry.PackingSeedFeasibilityDerived != nil {
		t.Fatalf("mixed schemas were aggregated: %+v", entry)
	}
	if len(entry.PackingSeedFeasibilityByVersion) != 2 {
		t.Fatalf("version-separated profiles=%+v", entry.PackingSeedFeasibilityByVersion)
	}
	first, second := entry.PackingSeedFeasibilityByVersion[0], entry.PackingSeedFeasibilityByVersion[1]
	if first.Version != model.PackingSeedFeasibilityProfileVersionV1 || first.Runs != 1 || first.Profile.CandidateCanonical.PlacementKeyCalls != 7 || first.Derived.FeasibilityCandidatePlacementKeyCallsPerCanonicalCall != 0 {
		t.Fatalf("v1 separate profile=%+v", first)
	}
	if second.Version != model.PackingSeedFeasibilityProfileVersionV2 || second.Runs != 1 || second.Profile.CandidateCanonical.CandidatePlacementKeyCalls != 1 || second.Derived.FeasibilityCandidatePlacementKeyCallsPerCanonicalCall != 0 {
		t.Fatalf("v2 separate profile=%+v", second)
	}
	var formatted strings.Builder
	FormatOperationProfileSummary(&formatted, summary)
	if !strings.Contains(formatted.String(), "incompatible packing-seed profile versions retained separately") {
		t.Fatalf("missing version-mismatch warning: %q", formatted.String())
	}
}

func TestSummarizeOperationProfileAggregatesAndDerivesBoundAttribution(t *testing.T) {
	first := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	first.PriorityUpper.ConstellationFilter = model.PriorityUpperBoundSiteProfile{
		Calls: 2, FeasibleResults: 1, RejectedResults: 1, AnchoredPlacements: 4,
		RemovedInstances: 6, RemovedOptionCandidates: 10, RemovedOptionsRetained: 6,
		UniquePrioritySourceItems: 2, AnchoredSourceInstances: 2, RemovedSourceInstances: 4,
		StarSlots: 8, FixedTargetChecks: 12, RemovedTargetChecks: 18,
		GeometryCandidateChecks: 10, GeometryOverlapRejects: 2,
		StarPositionHitCalls: 8, StarPositionHitTrue: 4, SlotTargetHits: 4,
	}
	first.Outgoing.Search = model.OutgoingBoundSiteProfile{
		Checks: 4, PrunedNodes: 2, PlacedMapInsertions: 12,
		PrioritySourceMatches: 8, PlacedSourceIterations: 6, FreeSourceIterations: 2,
		PlacedSourceTargetIterations: 24, TargetPlacementLookups: 18, PlacedTargetsFound: 9,
		SourceHitsTargetCalls: 9, CoveragePlacementKeyCalls: 6,
		PlacedPotentialLookups: 6, FreePotentialLookups: 2,
	}
	second := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	second.PriorityUpper.RepairDFS = model.PriorityUpperBoundSiteProfile{Calls: 1, FeasibleResults: 1}
	second.Outgoing.Repair = model.OutgoingBoundSiteProfile{Checks: 6, PrunedNodes: 3}
	report := Report{Runs: []Run{
		{Scenario: "bounds", Budget: 1_000, Search: SearchSummary{BoundOperationProfile: first}},
		{Scenario: "bounds", Budget: 1_000, Search: SearchSummary{BoundOperationProfile: second}},
	}}

	entry := SummarizeOperationProfile(report).Scenarios[0]
	if entry.BoundAttribution == nil || entry.BoundAttribution.PriorityUpper.ConstellationFilter.Calls != 2 || entry.BoundAttribution.PriorityUpper.RepairDFS.Calls != 1 || entry.BoundAttribution.Outgoing.Search.Checks != 4 || entry.BoundAttribution.Outgoing.Repair.Checks != 6 {
		t.Fatalf("bound attribution aggregate=%+v", entry.BoundAttribution)
	}
	derived := entry.BoundAttributionDerived
	if derived == nil || derived.PriorityUpper.ConstellationFilter.RejectionRate != 0.5 || derived.PriorityUpper.ConstellationFilter.RemovedOptionRetentionRate != 0.6 || derived.PriorityUpper.ConstellationFilter.GeometryOverlapRejectRate != 0.2 || derived.Outgoing.Search.PruneRate != 0.5 || derived.Outgoing.Aggregate.PruneRate != 0.5 || derived.Outgoing.Search.TargetsPerPlacedSource != 4 {
		t.Fatalf("bound attribution derived=%+v", derived)
	}
	var formatted strings.Builder
	FormatOperationProfileSummary(&formatted, OperationProfileSummaryReport{Aggregate: entry})
	if !strings.Contains(formatted.String(), "Bound attribution") || !strings.Contains(formatted.String(), model.BoundAttributionProfileVersion) {
		t.Fatalf("formatted bound summary=%q", formatted.String())
	}
}

func TestSummarizeOperationProfileSeparatesBoundAttributionVersions(t *testing.T) {
	v1 := &model.BoundAttributionOperationProfile{Version: model.BoundAttributionProfileVersion}
	v1.Outgoing.Search.Checks = 3
	v2 := &model.BoundAttributionOperationProfile{Version: "bound-attribution-ops-v2"}
	v2.Outgoing.Search.Checks = 5
	report := Report{Runs: []Run{
		{Scenario: "mixed", Budget: 1_000, Search: SearchSummary{BoundOperationProfile: v2}},
		{Scenario: "mixed", Budget: 1_000, Search: SearchSummary{BoundOperationProfile: v1}},
	}}

	entry := SummarizeOperationProfile(report).Scenarios[0]
	if entry.BoundAttribution != nil || entry.BoundAttributionDerived != nil || len(entry.BoundAttributionByVersion) != 2 {
		t.Fatalf("mixed bound schemas were aggregated: %+v", entry)
	}
	if entry.BoundAttributionByVersion[0].Version != model.BoundAttributionProfileVersion || entry.BoundAttributionByVersion[0].Profile.Outgoing.Search.Checks != 3 || entry.BoundAttributionByVersion[1].Version != "bound-attribution-ops-v2" || entry.BoundAttributionByVersion[1].Profile.Outgoing.Search.Checks != 5 {
		t.Fatalf("version-separated bound profiles=%+v", entry.BoundAttributionByVersion)
	}
	var formatted strings.Builder
	FormatOperationProfileSummary(&formatted, OperationProfileSummaryReport{Aggregate: entry})
	if !strings.Contains(formatted.String(), "incompatible bound-attribution profile versions retained separately") {
		t.Fatalf("missing bound version warning: %q", formatted.String())
	}
}

func TestSummarizeOperationProfileAccountsForEveryFamilyTermination(t *testing.T) {
	report := Report{Runs: []Run{{
		Scenario: "alpha",
		Budget:   1_000,
		Search: SearchSummary{ConstellationSeedDiagnostics: &model.ConstellationSeedDiagnostics{
			Roots: []model.ConstellationRootDiagnostic{
				{FamilyID: "single/completed", FamilyTerminationReason: "completed"},
				{FamilyID: "single/budget", FamilyTerminationReason: "budget_exhausted"},
				{FamilyID: "single/dead", FamilyTerminationReason: "hard_dead"},
				{FamilyID: "single/empty", FamilyTerminationReason: "no_states"},
				{FamilyID: "single/future", FamilyTerminationReason: "future_terminal_reason"},
			},
		}},
	}}}

	summary := SummarizeOperationProfile(report).Scenarios[0].Scheduler
	if summary == nil || summary.FamilyCount != 5 || summary.FamiliesCompleted != 1 || summary.FamiliesBudgetExhausted != 1 || summary.FamiliesHardDead != 1 || summary.FamiliesNoStates != 1 {
		t.Fatalf("scheduler summary=%+v", summary)
	}
	var counted int
	for _, termination := range summary.TerminationReasonCounts {
		counted += termination.Count
	}
	if counted != summary.FamilyCount {
		t.Fatalf("termination counts=%+v sum=%d family_count=%d", summary.TerminationReasonCounts, counted, summary.FamilyCount)
	}
	if got := summary.TerminationReasonCounts; len(got) != 5 || got[0].Reason != "budget_exhausted" || got[1].Reason != "completed" || got[2].Reason != "future_terminal_reason" || got[3].Reason != "hard_dead" || got[4].Reason != "no_states" {
		t.Fatalf("termination counts=%+v", got)
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
