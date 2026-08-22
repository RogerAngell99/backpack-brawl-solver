package benchmark

import (
	"fmt"
	"io"
	"sort"

	"backpack-brawl-solver/internal/model"
)

// OperationProfileSummaryReport is a deterministic reduction of benchmark
// report JSON. It keeps operation counts separate from scheduler opportunity
// telemetry so neither is mistaken for a CPU profile.
type OperationProfileSummaryReport struct {
	Version   string                            `json:"version"`
	Scenarios []OperationProfileScenarioSummary `json:"scenarios"`
	Aggregate OperationProfileScenarioSummary   `json:"aggregate"`
}

type OperationProfileScenarioSummary struct {
	Scenario              string                                          `json:"scenario,omitempty"`
	Budget                int64                                           `json:"budget,omitempty"`
	Runs                  int                                             `json:"runs"`
	RootedPacking         *model.ConstellationRootPackingOperationProfile `json:"rooted_packing_operations,omitempty"`
	PerCandidateExpansion *OperationProfilePerCandidateExpansion          `json:"per_candidate_expansion,omitempty"`
	Scheduler             *SchedulerOpportunitySummary                    `json:"scheduler,omitempty"`
}

type OperationProfilePerCandidateExpansion struct {
	MRVOptionChecks         float64 `json:"mrv_option_checks"`
	FeasibilityOptionChecks float64 `json:"feasibility_option_checks"`
	PlacementElementsCopied float64 `json:"placement_elements_copied"`
	StateKeyBytes           float64 `json:"state_key_bytes"`
}

// SchedulerOpportunitySummary intentionally calls returned nodes "returned
// capacity". The scheduler does not preserve quota-token provenance, so a
// later charge cannot be attributed to a particular earlier return.
type SchedulerOpportunitySummary struct {
	FamilyCount              int                          `json:"family_count"`
	FamiliesCompleted        int                          `json:"families_completed"`
	FamiliesBudgetExhausted  int                          `json:"families_budget_exhausted"`
	FamiliesHardDead         int                          `json:"families_hard_dead"`
	FamiliesNoStates         int                          `json:"families_no_states"`
	TerminationReasonCounts  []SchedulerTerminationCount  `json:"termination_reason_counts"`
	AllocationRounds         int                          `json:"allocation_rounds"`
	ReservedTotal            int64                        `json:"reserved_total"`
	ReservationTurnover      int64                        `json:"reservation_turnover"`
	ConsumedTotal            int64                        `json:"consumed_total"`
	ReturnedCapacityTotal    int64                        `json:"returned_capacity_total"`
	ReturnedFractionBPS      int64                        `json:"returned_fraction_bps"`
	ConsumedPerFamily        OperationProfileDistribution `json:"consumed_per_family"`
	RoundsPerFamily          OperationProfileDistribution `json:"rounds_per_family"`
	FinalDepth               OperationProfileDistribution `json:"final_depth"`
	FirstCompleteFamilyCount int                          `json:"first_complete_family_count"`
	FirstCompleteNodes       OperationProfileDistribution `json:"first_complete_nodes"`
}

// SchedulerTerminationCount retains every terminal family reason so new
// scheduler outcomes cannot silently disappear from P0 opportunity telemetry.
type SchedulerTerminationCount struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type OperationProfileDistribution struct {
	Min int64 `json:"min"`
	P50 int64 `json:"p50"`
	P90 int64 `json:"p90"`
	Max int64 `json:"max"`
}

// SummarizeOperationProfile groups a benchmark report by scenario and node
// budget, then emits a pure aggregate across all groups. It never reads timing
// fields or makes a policy recommendation.
func SummarizeOperationProfile(report Report) OperationProfileSummaryReport {
	groups := make(map[operationProfileGroupKey][]Run)
	for _, run := range report.Runs {
		groups[operationProfileGroupKey{scenario: run.Scenario, budget: run.Budget}] = append(groups[operationProfileGroupKey{scenario: run.Scenario, budget: run.Budget}], run)
	}
	keys := make([]operationProfileGroupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].scenario != keys[j].scenario {
			return keys[i].scenario < keys[j].scenario
		}
		return keys[i].budget < keys[j].budget
	})

	summary := OperationProfileSummaryReport{Version: "operation-profile-summary-v1", Scenarios: make([]OperationProfileScenarioSummary, 0, len(keys))}
	for _, key := range keys {
		entry := summarizeOperationProfileRuns(groups[key])
		entry.Scenario = key.scenario
		entry.Budget = key.budget
		summary.Scenarios = append(summary.Scenarios, entry)
	}
	summary.Aggregate = summarizeOperationProfileRuns(report.Runs)
	return summary
}

type operationProfileGroupKey struct {
	scenario string
	budget   int64
}

func summarizeOperationProfileRuns(runs []Run) OperationProfileScenarioSummary {
	summary := OperationProfileScenarioSummary{Runs: len(runs)}
	var roots []model.ConstellationRootDiagnostic
	for _, run := range runs {
		if run.Search.ConstellationSeedDiagnostics == nil {
			continue
		}
		diagnostics := run.Search.ConstellationSeedDiagnostics
		if diagnostics.RootPackingOperationProfile != nil {
			summary.RootedPacking = mergeOperationProfiles(summary.RootedPacking, diagnostics.RootPackingOperationProfile)
		} else {
			for _, root := range diagnostics.Roots {
				summary.RootedPacking = mergeOperationProfiles(summary.RootedPacking, root.OperationProfile)
			}
		}
		roots = append(roots, diagnostics.Roots...)
	}
	if summary.RootedPacking != nil && summary.RootedPacking.CandidateExpansions > 0 {
		candidateExpansions := float64(summary.RootedPacking.CandidateExpansions)
		summary.PerCandidateExpansion = &OperationProfilePerCandidateExpansion{
			MRVOptionChecks:         float64(summary.RootedPacking.MRVOptionChecks) / candidateExpansions,
			FeasibilityOptionChecks: float64(summary.RootedPacking.FeasibilityOptionChecks) / candidateExpansions,
			PlacementElementsCopied: float64(summary.RootedPacking.PlacementElementsCopied) / candidateExpansions,
			StateKeyBytes:           float64(summary.RootedPacking.StateKeyBytes) / candidateExpansions,
		}
	}
	summary.Scheduler = summarizeSchedulerOpportunity(roots)
	return summary
}

func mergeOperationProfiles(total *model.ConstellationRootPackingOperationProfile, next *model.ConstellationRootPackingOperationProfile) *model.ConstellationRootPackingOperationProfile {
	if next == nil {
		return total
	}
	if total == nil {
		copy := *next
		return &copy
	}
	total.SessionsStarted += next.SessionsStarted
	total.RunCalls += next.RunCalls
	total.PauseReturns += next.PauseReturns
	total.DepthsStarted += next.DepthsStarted
	total.StatesPrepared += next.StatesPrepared
	total.AreaPrunes += next.AreaPrunes
	total.MRVSelectionCalls += next.MRVSelectionCalls
	total.MRVInstancesConsidered += next.MRVInstancesConsidered
	total.MRVOptionChecks += next.MRVOptionChecks
	total.MRVLegalPlacements += next.MRVLegalPlacements
	total.LedgerChargeAttempts += next.LedgerChargeAttempts
	total.LedgerChargeDenied += next.LedgerChargeDenied
	total.CandidateExpansions += next.CandidateExpansions
	total.CompleteCandidates += next.CompleteCandidates
	total.PlacementCopyCalls += next.PlacementCopyCalls
	total.PlacementElementsCopied += next.PlacementElementsCopied
	total.FeasibilityCalls += next.FeasibilityCalls
	total.FeasibilityInstancesConsidered += next.FeasibilityInstancesConsidered
	total.FeasibilityOptionChecks += next.FeasibilityOptionChecks
	total.FragmentationEvaluations += next.FragmentationEvaluations
	total.PartialScoreEvaluations += next.PartialScoreEvaluations
	total.StateKeyConstructions += next.StateKeyConstructions
	total.StateKeyBytes += next.StateKeyBytes
	total.DedupLookups += next.DedupLookups
	total.DedupHits += next.DedupHits
	total.DedupReplacements += next.DedupReplacements
	total.DepthFinishCalls += next.DepthFinishCalls
	total.PrecutStates += next.PrecutStates
	total.StatesSorted += next.StatesSorted
	return total
}

func summarizeSchedulerOpportunity(roots []model.ConstellationRootDiagnostic) *SchedulerOpportunitySummary {
	var familyRoots []model.ConstellationRootDiagnostic
	for _, root := range roots {
		if root.FamilyID != "" {
			familyRoots = append(familyRoots, root)
		}
	}
	if len(familyRoots) == 0 {
		return nil
	}
	summary := &SchedulerOpportunitySummary{FamilyCount: len(familyRoots)}
	consumed := make([]int64, 0, len(familyRoots))
	rounds := make([]int64, 0, len(familyRoots))
	depths := make([]int64, 0, len(familyRoots))
	firstComplete := make([]int64, 0, len(familyRoots))
	terminationCounts := make(map[string]int)
	for _, root := range familyRoots {
		termination := root.FamilyTerminationReason
		if termination == "" {
			termination = root.TerminationReason
		}
		if termination == "" {
			termination = "unknown"
		}
		terminationCounts[termination]++
		switch termination {
		case "completed":
			summary.FamiliesCompleted++
		case "budget_exhausted":
			summary.FamiliesBudgetExhausted++
		case "hard_dead":
			summary.FamiliesHardDead++
		case "no_states":
			summary.FamiliesNoStates++
		}
		consumed = append(consumed, root.FamilyTotalConsumed)
		rounds = append(rounds, int64(len(root.FamilyAllocationRounds)))
		depths = append(depths, finalRootPackingDepth(root.MRVDepths))
		if root.FirstCompleteNodes > 0 {
			summary.FirstCompleteFamilyCount++
			firstComplete = append(firstComplete, root.FirstCompleteNodes)
		}
		for _, allocation := range root.FamilyAllocationRounds {
			summary.AllocationRounds++
			summary.ReservedTotal += allocation.Reserved
			summary.ReservationTurnover += allocation.Reserved
			summary.ConsumedTotal += allocation.Consumed
			summary.ReturnedCapacityTotal += allocation.Returned
		}
	}
	if summary.ReservationTurnover > 0 {
		summary.ReturnedFractionBPS = summary.ReturnedCapacityTotal * 10_000 / summary.ReservationTurnover
	}
	summary.ConsumedPerFamily = operationProfileDistribution(consumed)
	summary.RoundsPerFamily = operationProfileDistribution(rounds)
	summary.FinalDepth = operationProfileDistribution(depths)
	summary.FirstCompleteNodes = operationProfileDistribution(firstComplete)
	terminationReasons := make([]string, 0, len(terminationCounts))
	for reason := range terminationCounts {
		terminationReasons = append(terminationReasons, reason)
	}
	sort.Strings(terminationReasons)
	for _, reason := range terminationReasons {
		summary.TerminationReasonCounts = append(summary.TerminationReasonCounts, SchedulerTerminationCount{Reason: reason, Count: terminationCounts[reason]})
	}
	return summary
}

func finalRootPackingDepth(depths []model.ConstellationRootPackingDepthDiagnostic) int64 {
	var maximum int64
	for _, depth := range depths {
		if int64(depth.Depth) > maximum {
			maximum = int64(depth.Depth)
		}
	}
	return maximum
}

func operationProfileDistribution(values []int64) OperationProfileDistribution {
	if len(values) == 0 {
		return OperationProfileDistribution{}
	}
	values = append([]int64(nil), values...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return OperationProfileDistribution{
		Min: values[0],
		P50: values[(len(values)-1)*50/100],
		P90: values[(len(values)-1)*90/100],
		Max: values[len(values)-1],
	}
}

// FormatOperationProfileSummary prints the compact human-facing companion to
// the JSON summary. Use the JSON report when consuming exact values in CI.
func FormatOperationProfileSummary(writer io.Writer, summary OperationProfileSummaryReport) {
	for _, entry := range append(append([]OperationProfileScenarioSummary(nil), summary.Scenarios...), summary.Aggregate) {
		label := entry.Scenario
		if label == "" {
			label = "aggregate"
		} else {
			label = fmt.Sprintf("%s @ %d", label, entry.Budget)
		}
		fmt.Fprintf(writer, "Rooted packing operations — %s\n", label)
		if entry.RootedPacking == nil {
			fmt.Fprintln(writer, "  no operation profile present")
		} else {
			profile := entry.RootedPacking
			fmt.Fprintf(writer, "  candidate expansions: %d\n  MRV option checks: %d\n  feasibility option checks: %d\n  partial score evaluations: %d\n  fragmentation evaluations: %d\n  state keys: %d\n  placement elements copied: %d\n", profile.CandidateExpansions, profile.MRVOptionChecks, profile.FeasibilityOptionChecks, profile.PartialScoreEvaluations, profile.FragmentationEvaluations, profile.StateKeyConstructions, profile.PlacementElementsCopied)
			if entry.PerCandidateExpansion != nil {
				per := entry.PerCandidateExpansion
				fmt.Fprintf(writer, "  per expansion — MRV: %.1f, feasibility: %.1f, placement elements: %.1f, key bytes: %.1f\n", per.MRVOptionChecks, per.FeasibilityOptionChecks, per.PlacementElementsCopied, per.StateKeyBytes)
			}
		}
		if entry.Scheduler != nil {
			scheduler := entry.Scheduler
			fmt.Fprintf(writer, "  scheduler — families: %d, completed: %d, budget exhausted: %d, hard dead: %d, no states: %d, returned capacity: %d/%d (%d bps)\n", scheduler.FamilyCount, scheduler.FamiliesCompleted, scheduler.FamiliesBudgetExhausted, scheduler.FamiliesHardDead, scheduler.FamiliesNoStates, scheduler.ReturnedCapacityTotal, scheduler.ReservationTurnover, scheduler.ReturnedFractionBPS)
			fmt.Fprintf(writer, "  scheduler distributions — consumed p50/p90: %d/%d, rounds p50/p90: %d/%d, final depth p50/p90: %d/%d, first-complete families: %d\n", scheduler.ConsumedPerFamily.P50, scheduler.ConsumedPerFamily.P90, scheduler.RoundsPerFamily.P50, scheduler.RoundsPerFamily.P90, scheduler.FinalDepth.P50, scheduler.FinalDepth.P90, scheduler.FirstCompleteFamilyCount)
		}
	}
}
