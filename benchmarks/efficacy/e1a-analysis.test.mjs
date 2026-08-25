import assert from "node:assert/strict";
import test from "node:test";

import {
  classifyDecision,
  compareScores,
  firstDifference,
  validateDiagnosticPair,
} from "./e1a-analysis.mjs";

test("semantic comparison follows the canonical lexicographic objective", () => {
  const difference = firstDifference(
    { priority_counts: [6, 0], crafts: 0, stars: 0 },
    { priority_counts: [5, 99], crafts: 99, stars: 99 },
  );
  assert.deepEqual(difference, {
    difference_level: 0,
    component: "PriorityCounts",
    priority_index: 0,
    baseline_value: 5,
    candidate_value: 6,
    signed_delta: 1,
  });
  assert.equal(compareScores({ priority_counts: [6] }, { priority_counts: [5, 99] }), 1);
  assert.equal(compareScores({ priority_counts: [1], crafts: 2, stars: 0 }, { priority_counts: [1], crafts: 1, stars: 99 }), 1);
});

test("omitted score fields and trailing priority zeroes are semantic ties", () => {
  assert.equal(compareScores({ priority_counts: [1] }, { priority_counts: [1, 0] }), 0);
  assert.equal(firstDifference({}, {}).difference_level, 7);
});

test("diagnostic twins must preserve deterministic quality fields", () => {
  const quality = pairedRun(false);
  const diagnostic = pairedRun(true);
  assert.equal(validateDiagnosticPair(quality, diagnostic), true);
  diagnostic.canonical_layout_hash = "different";
  assert.throws(() => validateDiagnosticPair(quality, diagnostic), /canonical hash/);
});

test("isolated v4 direct evidence outranks every indirect family", () => {
  const decision = classifyDecision({
    controlSummaries: [
      { variant: "v4", wins: 3, losses: 0, net_wins: 3, high_severity_losses: 0, criterion_c: true, isolated_promotion_gate: true },
      { variant: "v5", wins: 5, losses: 0, net_wins: 5, distinct_win_scenarios: 5, criterion_c: true },
    ],
    rootSummaries: [{ variant: "v4", criterion_d: true, distinct_scenarios: 4 }],
    repair: { criterion_e: true, material_cases: 12, improvement_cases: 10, final_producer_cases: 5 },
    phases: [{ phase: "dfs", criterion_a: true, material_cases: 14, aggregate_budget_share: 0.5 }],
    underutilization: { criterion_b: true, cases_at_least_5_percent: 14 },
    shortlist: [],
  });
  assert.equal(decision.kind, "PROMOTE");
  assert.equal(decision.candidate_id, "constellation-equal-root-allocation-v1");
  assert.equal(decision.evidence_gate, "C");
});

test("repair relevance requests the frozen per-neighborhood causal probe", () => {
  const decision = classifyDecision({
    controlSummaries: [
      { variant: "v4", criterion_c: false, isolated_promotion_gate: false },
      { variant: "v5", criterion_c: false },
      { variant: "v5.1", criterion_c: false },
    ],
    rootSummaries: [{ variant: "v4", criterion_d: false }],
    repair: { criterion_e: true, material_cases: 10, improvement_cases: 8, final_producer_cases: 3 },
    phases: [{ phase: "dfs", criterion_a: true, material_cases: 14, aggregate_budget_share: 0.5 }],
    underutilization: { criterion_b: true, cases_at_least_5_percent: 10 },
    shortlist: [],
  });
  assert.equal(decision.kind, "NEED MORE EVIDENCE");
  assert.equal(decision.candidate_id, "repair-weighted-v1");
  assert.match(decision.missing_evidence, /per-neighborhood/);
});

function pairedRun(diagnostic) {
  return {
    scenario: "gsv2-013",
    scenario_path: "scenarios/gsv2-013.json",
    budget: 250000,
    repeat: 1,
    priority_semantics: "outgoing-per-instance-v3",
    priorities: ["a", "b"],
    no_skips: true,
    repair_search: true,
    plateau_variant: "legacy-large-off",
    stop_on_coverage_ceiling: false,
    stop_on_priority_ceiling: false,
    score: { priority_counts: [4, 3], crafts: 0, stars: 7, items: 19 },
    layout_key: "layout",
    canonical_layout_hash: "hash",
    search: {
      diagnostics_enabled: diagnostic,
      nodes_explored: 237135,
      global_budget_consumed: 237135,
      unused_global_nodes: 12865,
      normal_budget_configured: 250000,
      normal_budget_consumed: 237135,
      execution_budget_configured: 250000,
      execution_budget_consumed: 237135,
      config_fingerprint: "config",
      execution_fingerprint: "execution",
    },
  };
}
