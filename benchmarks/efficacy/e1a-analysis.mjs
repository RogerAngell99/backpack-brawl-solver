#!/usr/bin/env node

import { createHash } from "node:crypto";
import { readFile, mkdir, writeFile, stat } from "node:fs/promises";
import path from "node:path";
import process from "node:process";
import { pathToFileURL } from "node:url";

export const EXPECTED_SHA = "11644a7d88bd4e4bdd1f97977f8aad5e59391293";
export const EXPECTED_CASES = Array.from({ length: 14 }, (_, index) => `gsv2-${String(index + 13).padStart(3, "0")}`);

const REPORT_SPECS = [
  { id: "quality-general-search-v1", kind: "quality", variant: "general-search-v1", budgets: [250000, 1000000], file: "raw/quality/general-search-v1.json" },
  { id: "quality-v4", kind: "quality", variant: "v4", budgets: [1000000], file: "raw/quality/v4.json" },
  { id: "quality-v5", kind: "quality", variant: "v5", budgets: [1000000], file: "raw/quality/v5.json" },
  { id: "quality-v5.1", kind: "quality", variant: "v5.1", budgets: [1000000], file: "raw/quality/v5.1.json" },
  { id: "diagnostic-general-search-v1", kind: "diagnostic", variant: "general-search-v1", budgets: [250000, 1000000], file: "raw/diagnostic/general-search-v1.json" },
  { id: "diagnostic-v4", kind: "diagnostic", variant: "v4", budgets: [1000000], file: "raw/diagnostic/v4.json" },
  { id: "diagnostic-v5", kind: "diagnostic", variant: "v5", budgets: [1000000], file: "raw/diagnostic/v5.json" },
  { id: "diagnostic-v5.1", kind: "diagnostic", variant: "v5.1", budgets: [1000000], file: "raw/diagnostic/v5.1.json" },
];

const SCORE_COMPONENTS = [
  { level: 1, name: "Crafts", key: "crafts" },
  { level: 2, name: "Stars", key: "stars" },
  { level: 3, name: "Items", key: "items" },
  { level: 4, name: "StarTargetBreadth", key: "star_target_breadth" },
  { level: 5, name: "StarReciprocalPairs", key: "star_reciprocal_pairs" },
  { level: 6, name: "StarSourceDefinitionDiversity", key: "star_source_definition_diversity" },
];

const CONTROL_MECHANISMS = {
  v4: {
    candidate_id: "constellation-equal-root-allocation-v1",
    family: "constellation root allocation",
    risk_class: "Q1",
    isolated: true,
    exact_mechanism: "replace the progressive general-search-v1 constellation root scheduler with deterministic equal per-root V4 allocation while retaining V4 root construction and state-MRV rooted packing",
  },
  v5: {
    candidate_id: "constellation-v5-family-isolation",
    family: "constellation root selection and packing",
    risk_class: "Q1",
    isolated: false,
    exact_mechanism: "isolate V5 frontier root selection from its packing-beam and allocation changes with a one-variable deterministic shadow replay",
  },
  "v5.1": {
    candidate_id: "constellation-v5.1-family-isolation",
    family: "constellation root selection and packing",
    risk_class: "Q1",
    isolated: false,
    exact_mechanism: "isolate V5.1 parent/frontier selection from its packing-beam and allocation changes with a one-variable deterministic shadow replay",
  },
};

function asInteger(value, fallback = 0) {
  return Number.isInteger(value) ? value : fallback;
}

function normalizeScore(score = {}) {
  return {
    priority_counts: Array.isArray(score.priority_counts) ? score.priority_counts.map((value) => asInteger(value)) : [],
    crafts: asInteger(score.crafts),
    stars: asInteger(score.stars),
    items: asInteger(score.items),
    star_target_breadth: asInteger(score.star_target_breadth),
    star_reciprocal_pairs: asInteger(score.star_reciprocal_pairs),
    star_source_definition_diversity: asInteger(score.star_source_definition_diversity),
  };
}

export function firstDifference(candidateInput, baselineInput) {
  const candidate = normalizeScore(candidateInput);
  const baseline = normalizeScore(baselineInput);
  const priorityLength = Math.max(candidate.priority_counts.length, baseline.priority_counts.length);
  for (let index = 0; index < priorityLength; index += 1) {
    const candidateValue = candidate.priority_counts[index] ?? 0;
    const baselineValue = baseline.priority_counts[index] ?? 0;
    if (candidateValue !== baselineValue) {
      return {
        difference_level: 0,
        component: "PriorityCounts",
        priority_index: index,
        baseline_value: baselineValue,
        candidate_value: candidateValue,
        signed_delta: candidateValue - baselineValue,
      };
    }
  }
  for (const component of SCORE_COMPONENTS) {
    const candidateValue = candidate[component.key];
    const baselineValue = baseline[component.key];
    if (candidateValue !== baselineValue) {
      return {
        difference_level: component.level,
        component: component.name,
        priority_index: null,
        baseline_value: baselineValue,
        candidate_value: candidateValue,
        signed_delta: candidateValue - baselineValue,
      };
    }
  }
  return {
    difference_level: 7,
    component: "semantic_tie",
    priority_index: null,
    baseline_value: null,
    candidate_value: null,
    signed_delta: 0,
  };
}

export function compareScores(candidate, baseline) {
  const difference = firstDifference(candidate, baseline);
  return Math.sign(difference.signed_delta);
}

function scoresEqual(left, right) {
  return compareScores(left, right) === 0;
}

function scoreText(score) {
  const value = normalizeScore(score);
  return JSON.stringify([
    value.priority_counts,
    value.crafts,
    value.stars,
    value.items,
    value.star_target_breadth,
    value.star_reciprocal_pairs,
    value.star_source_definition_diversity,
  ]);
}

function runKey(run) {
  return `${run.scenario}|${run.budget}|${run.repeat}`;
}

function indexRuns(report) {
  const indexed = new Map();
  for (const run of report.runs ?? []) {
    const key = runKey(run);
    if (indexed.has(key)) {
      throw new Error(`duplicate run key ${key}`);
    }
    indexed.set(key, run);
  }
  return indexed;
}

function sameJSON(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

function validateReport(report, spec) {
  const errors = [];
  if (report.build_revision !== EXPECTED_SHA) errors.push(`build_revision=${report.build_revision}`);
  if (report.workers !== 1) errors.push(`workers=${report.workers}`);
  if (report.repeat !== 1) errors.push(`repeat=${report.repeat}`);
  if (report.constellation_seed_variant !== spec.variant) errors.push(`variant=${report.constellation_seed_variant}`);
  if (Boolean(report.diagnostic) !== (spec.kind === "diagnostic")) errors.push(`diagnostic=${report.diagnostic}`);
  if (Boolean(report.operation_profiling)) errors.push("operation_profiling must be false");
  if (!sameJSON(report.budgets, spec.budgets)) errors.push(`budgets=${JSON.stringify(report.budgets)}`);
  const expectedKeys = new Set(EXPECTED_CASES.flatMap((scenario) => spec.budgets.map((budget) => `${scenario}|${budget}|1`)));
  const actual = indexRuns(report);
  for (const key of expectedKeys) if (!actual.has(key)) errors.push(`missing run ${key}`);
  for (const key of actual.keys()) if (!expectedKeys.has(key)) errors.push(`unexpected run ${key}`);
  for (const run of report.runs ?? []) {
    if (run.error) errors.push(`${runKey(run)} error=${run.error}`);
    if (run.constellation_seed_variant !== spec.variant) errors.push(`${runKey(run)} run variant=${run.constellation_seed_variant}`);
    if (run.repeat !== 1) errors.push(`${runKey(run)} repeat=${run.repeat}`);
    if (run.operation_profiling) errors.push(`${runKey(run)} operation profiling enabled`);
    if (spec.kind === "diagnostic" && !run.search?.diagnostics_enabled) errors.push(`${runKey(run)} diagnostics missing`);
    if (spec.kind === "quality" && run.search?.diagnostics_enabled) errors.push(`${runKey(run)} diagnostics unexpectedly enabled`);
  }
  if (errors.length > 0) throw new Error(`${spec.id} invalid: ${errors.join("; ")}`);
}

function semanticInvariant(run) {
  return {
    scenario: run.scenario,
    scenario_path: path.basename(run.scenario_path ?? ""),
    budget: run.budget,
    repeat: run.repeat,
    priority_semantics: run.priority_semantics,
    priorities: run.priorities ?? [],
    no_skips: Boolean(run.no_skips),
    repair_search: Boolean(run.repair_search),
    plateau_variant: run.plateau_variant ?? "",
    stop_on_coverage_ceiling: Boolean(run.stop_on_coverage_ceiling),
    stop_on_priority_ceiling: Boolean(run.stop_on_priority_ceiling),
  };
}

export function validateDiagnosticPair(quality, diagnostic) {
  const failures = [];
  if (!sameJSON(semanticInvariant(quality), semanticInvariant(diagnostic))) failures.push("semantic run invariants");
  if (!scoresEqual(quality.score, diagnostic.score)) failures.push("semantic score");
  if ((quality.layout_key ?? "") !== (diagnostic.layout_key ?? "")) failures.push("layout key");
  if ((quality.canonical_layout_hash ?? "") !== (diagnostic.canonical_layout_hash ?? "")) failures.push("canonical hash");
  const fields = ["nodes_explored", "global_budget_consumed", "unused_global_nodes", "normal_budget_configured", "normal_budget_consumed", "execution_budget_configured", "execution_budget_consumed"];
  for (const field of fields) {
    if (asInteger(quality.search?.[field]) !== asInteger(diagnostic.search?.[field])) failures.push(field);
  }
  if ((quality.search?.config_fingerprint ?? "") !== (diagnostic.search?.config_fingerprint ?? "")) failures.push("config fingerprint");
  if ((quality.search?.execution_fingerprint ?? "") !== (diagnostic.search?.execution_fingerprint ?? "")) failures.push("execution fingerprint");
  if (failures.length > 0) throw new Error(`diagnostic pair ${runKey(quality)} differs: ${failures.join(", ")}`);
  return true;
}

function validateAccounting(run) {
  const search = run.search ?? {};
  const configured = asInteger(search.normal_budget_configured);
  const global = asInteger(search.global_budget_consumed);
  const normal = asInteger(search.normal_budget_consumed);
  const unused = asInteger(search.unused_global_nodes);
  const executionConfigured = asInteger(search.execution_budget_configured);
  const executionConsumed = asInteger(search.execution_budget_consumed);
  if (configured !== run.budget) throw new Error(`${runKey(run)} configured ${configured} != budget ${run.budget}`);
  if (global < 0 || global > run.budget) throw new Error(`${runKey(run)} global consumed ${global} outside budget`);
  if (normal !== global) throw new Error(`${runKey(run)} normal ${normal} != global ${global}`);
  if (unused !== run.budget - global) throw new Error(`${runKey(run)} unused ${unused} does not reconcile`);
  if (executionConfigured < 0 || executionConsumed < 0 || executionConsumed > executionConfigured) throw new Error(`${runKey(run)} execution budget does not reconcile`);
  const phases = search.phase_work ?? [];
  let phaseCharged = 0;
  for (const phase of phases) {
    for (const field of ["charged_nodes", "nodes_reserved", "nodes_consumed", "nodes_returned", "uncharged_moves", "candidates"]) {
      if (asInteger(phase[field]) < 0) throw new Error(`${runKey(run)} phase ${phase.phase} negative ${field}`);
    }
    phaseCharged += asInteger(phase.charged_nodes);
  }
  if (phaseCharged !== global) throw new Error(`${runKey(run)} phase charged ${phaseCharged} != global ${global}`);
  return { configured, global, normal, unused, executionConfigured, executionConsumed, phaseCharged };
}

function firstCheckpointProducingFinal(run) {
  const checkpoints = [
    ["seed_best", run.search?.seed_best],
    ["search_best", run.search?.search_best],
    ["post_repair_best", run.search?.post_repair_best],
    ["refine_best", run.search?.refine_best],
  ];
  for (const [name, score] of checkpoints) {
    if (score && scoresEqual(score, run.score)) return name;
  }
  return "unattributed";
}

function phaseRowsForRun(run, variant) {
  const trace = [...(run.search?.incumbent_trace ?? [])].sort((left, right) => asInteger(left.sequence) - asInteger(right.sequence));
  const phases = run.search?.phase_work ?? [];
  const firstFinalEvent = trace.find((event) => scoresEqual(event.score, run.score));
  let incumbent = null;
  const rows = [];
  for (const phase of phases) {
    const before = incumbent ? normalizeScore(incumbent) : null;
    let improvements = 0;
    const events = trace.filter((event) => event.phase === phase.phase);
    for (const event of events) {
      if (incumbent === null || compareScores(event.score, incumbent) > 0) {
        incumbent = event.score;
        improvements += 1;
      }
    }
    const charged = asInteger(phase.charged_nodes);
    const share = run.budget > 0 ? charged / run.budget : 0;
    rows.push({
      scenario: run.scenario,
      budget: run.budget,
      variant,
      phase: phase.phase,
      eligible: Boolean(phase.eligible),
      invoked: Boolean(phase.invoked),
      skip_reason: phase.skip_reason ?? "",
      termination_reason: phase.termination_reason ?? "",
      charged_nodes: charged,
      returned_nodes: asInteger(phase.nodes_returned),
      reserved_nodes: asInteger(phase.nodes_reserved),
      consumed_nodes: asInteger(phase.nodes_consumed),
      uncharged_moves: asInteger(phase.uncharged_moves),
      candidates: asInteger(phase.candidates),
      share_of_total_budget: share,
      best_before: before ? scoreText(before) : "",
      best_after: incumbent ? scoreText(incumbent) : "",
      phase_best: phase.best_score ? scoreText(phase.best_score) : "",
      semantic_improvements: improvements,
      semantic_improvement: improvements > 0,
      first_produced_final_semantic_score: Boolean(firstFinalEvent && firstFinalEvent.phase === phase.phase),
      dead_spend: charged >= run.budget * 0.05 && improvements === 0,
    });
  }
  return rows;
}

function comparisonRow(baseline, candidate, baselineDiagnostic, candidateDiagnostic, variant) {
  if (!sameJSON(semanticInvariant(baseline), semanticInvariant(candidate))) {
    throw new Error(`control invariant mismatch ${variant} ${runKey(candidate)}`);
  }
  const difference = firstDifference(candidate.score, baseline.score);
  const compare = Math.sign(difference.signed_delta);
  const semanticStatus = compare > 0 ? "WIN" : compare < 0 ? "LOSS" : "TIE";
  const layoutKeyEqual = (baseline.layout_key ?? "") === (candidate.layout_key ?? "");
  const canonicalHashEqual = (baseline.canonical_layout_hash ?? "") === (candidate.canonical_layout_hash ?? "");
  const layoutOnly = compare === 0 && (!layoutKeyEqual || !canonicalHashEqual);
  return {
    scenario: candidate.scenario,
    budget: candidate.budget,
    baseline_variant: "general-search-v1",
    candidate_variant: variant,
    semantic_status: semanticStatus,
    display_status: layoutOnly ? "LAYOUT-ONLY CHANGE" : semanticStatus,
    difference_level: difference.difference_level,
    first_differing_component: difference.component,
    priority_index: difference.priority_index,
    baseline_value: difference.baseline_value,
    candidate_value: difference.candidate_value,
    signed_delta: difference.signed_delta,
    baseline_score: scoreText(baseline.score),
    candidate_score: scoreText(candidate.score),
    canonical_hash_equal: canonicalHashEqual,
    layout_key_equal: layoutKeyEqual,
    baseline_hash: baseline.canonical_layout_hash ?? "",
    candidate_hash: candidate.canonical_layout_hash ?? "",
    configured_nodes: candidate.budget,
    baseline_consumed_nodes: asInteger(baseline.search?.global_budget_consumed),
    candidate_consumed_nodes: asInteger(candidate.search?.global_budget_consumed),
    baseline_unused_nodes: asInteger(baseline.search?.unused_global_nodes),
    candidate_unused_nodes: asInteger(candidate.search?.unused_global_nodes),
    baseline_first_complete_node: asInteger(baselineDiagnostic.search?.first_complete_nodes),
    candidate_first_complete_node: asInteger(candidateDiagnostic.search?.first_complete_nodes),
    baseline_final_phase: firstFinalPhase(baselineDiagnostic),
    candidate_final_phase: firstFinalPhase(candidateDiagnostic),
  };
}

function firstFinalPhase(run) {
  const trace = [...(run.search?.incumbent_trace ?? [])].sort((left, right) => asInteger(left.sequence) - asInteger(right.sequence));
  return trace.find((event) => scoresEqual(event.score, run.score))?.phase ?? "";
}

function summarizeControls(rows) {
  const summaries = [];
  for (const variant of ["v4", "v5", "v5.1"]) {
    const selected = rows.filter((row) => row.candidate_variant === variant);
    const wins = selected.filter((row) => row.semantic_status === "WIN").length;
    const losses = selected.filter((row) => row.semantic_status === "LOSS").length;
    const ties = selected.length - wins - losses;
    const highSeverityLosses = selected.filter((row) => row.semantic_status === "LOSS" && row.difference_level <= 2).length;
    summaries.push({
      variant,
      wins,
      losses,
      ties,
      layout_only_changes: selected.filter((row) => row.display_status === "LAYOUT-ONLY CHANGE").length,
      net_wins: wins - losses,
      distinct_win_scenarios: new Set(selected.filter((row) => row.semantic_status === "WIN").map((row) => row.scenario)).size,
      high_severity_losses: highSeverityLosses,
      criterion_c: wins >= 3,
      isolated_promotion_gate: variant === "v4" && wins >= 3 && highSeverityLosses === 0 && wins - losses >= 2,
    });
  }
  return summaries;
}

function summarizePhases(phaseRows) {
  const selected = phaseRows.filter((row) => row.variant === "general-search-v1" && row.budget === 1000000);
  const byPhase = new Map();
  for (const row of selected) {
    const rows = byPhase.get(row.phase) ?? [];
    rows.push(row);
    byPhase.set(row.phase, rows);
  }
  return [...byPhase.entries()].map(([phase, rows]) => {
    const material = rows.filter((row) => row.share_of_total_budget >= 0.05);
    const dead = material.filter((row) => row.dead_spend);
    const totalCharged = rows.reduce((sum, row) => sum + row.charged_nodes, 0);
    const totalReturned = rows.reduce((sum, row) => sum + row.returned_nodes, 0);
    return {
      phase,
      cases: rows.length,
      invoked_cases: rows.filter((row) => row.invoked).length,
      material_cases: material.length,
      dead_material_cases: dead.length,
      improvement_cases: rows.filter((row) => row.semantic_improvement).length,
      final_producer_cases: rows.filter((row) => row.first_produced_final_semantic_score).length,
      total_charged_nodes: totalCharged,
      total_returned_nodes: totalReturned,
      aggregate_budget_share: totalCharged / (EXPECTED_CASES.length * 1000000),
      criterion_a: material.length >= 4 && dead.length >= 4 && dead.length * 2 >= material.length,
    };
  }).sort((left, right) => left.phase.localeCompare(right.phase));
}

function repairEvidence(phaseRows) {
  const selected = phaseRows.filter((row) => row.variant === "general-search-v1" && row.budget === 1000000 && (row.phase === "pre_repair" || row.phase === "post_repair"));
  const byScenario = new Map();
  for (const row of selected) {
    const value = byScenario.get(row.scenario) ?? { charged: 0, improvements: 0, final: false };
    value.charged += row.charged_nodes;
    value.improvements += row.semantic_improvements;
    value.final ||= row.first_produced_final_semantic_score;
    byScenario.set(row.scenario, value);
  }
  const values = [...byScenario.values()];
  const materialCases = values.filter((value) => value.charged >= 50000).length;
  const improvementCases = values.filter((value) => value.improvements > 0).length;
  const finalProducerCases = values.filter((value) => value.final).length;
  return {
    material_cases: materialCases,
    improvement_cases: improvementCases,
    final_producer_cases: finalProducerCases,
    total_charged_nodes: values.reduce((sum, value) => sum + value.charged, 0),
    aggregate_budget_share: values.reduce((sum, value) => sum + value.charged, 0) / (EXPECTED_CASES.length * 1000000),
    criterion_e: materialCases >= 4 && improvementCases >= 4 && finalProducerCases >= 2,
  };
}

function underutilizationEvidence(baselineRuns) {
  const rows = baselineRuns.filter((run) => run.budget === 1000000).map((run) => ({
    scenario: run.scenario,
    unused_nodes: asInteger(run.search?.unused_global_nodes),
    unused_share: asInteger(run.search?.unused_global_nodes) / run.budget,
  }));
  const material = rows.filter((row) => row.unused_share >= 0.05);
  return {
    cases_at_least_5_percent: material.length,
    scenarios: material.map((row) => row.scenario),
    total_unused_nodes: rows.reduce((sum, row) => sum + row.unused_nodes, 0),
    criterion_b: material.length >= 4,
  };
}

function rootIndex(run) {
  const diagnostics = run.search?.constellation_seed_diagnostics ?? {};
  const exactBySkeleton = new Map((diagnostics.skeletons ?? []).map((skeleton) => [skeleton.id, skeleton.exact_key ?? ""]));
  const result = new Map();
  for (const root of diagnostics.roots ?? []) {
    const key = root.root_packing_input_key || exactBySkeleton.get(root.skeleton_id) || "";
    if (!key) continue;
    const roots = result.get(key) ?? [];
    roots.push(root);
    result.set(key, roots);
  }
  return result;
}

function rootStarvationRows(baselineReports, diagnosticReports) {
  const rows = [];
  const baseline = indexRuns(diagnosticReports.get("general-search-v1"));
  for (const variant of ["v4", "v5", "v5.1"]) {
    const candidate = indexRuns(diagnosticReports.get(variant));
    for (const scenario of EXPECTED_CASES) {
      const key = `${scenario}|1000000|1`;
      const baselineRun = baseline.get(key);
      const candidateRun = candidate.get(key);
      const baseRoots = rootIndex(baselineRun);
      const candidateRoots = rootIndex(candidateRun);
      for (const [rootKey, baseList] of baseRoots.entries()) {
        const candidateList = candidateRoots.get(rootKey) ?? [];
        for (const baseRoot of baseList) {
          const baseReason = `${baseRoot.termination_reason ?? ""} ${baseRoot.family_termination_reason ?? ""}`.toLowerCase();
          if (!baseReason.includes("budget")) continue;
          const better = candidateList.find((root) => root.completed && root.best_score && (!baseRoot.best_score || compareScores(root.best_score, baseRoot.best_score) > 0));
          if (!better) continue;
          rows.push({
            scenario,
            candidate_variant: variant,
            root_key_sha256: createHash("sha256").update(rootKey).digest("hex"),
            baseline_completed: Boolean(baseRoot.completed),
            baseline_termination: baseRoot.termination_reason ?? baseRoot.family_termination_reason ?? "",
            baseline_nodes: asInteger(baseRoot.nodes_consumed || baseRoot.family_total_consumed),
            candidate_completed: Boolean(better.completed),
            candidate_termination: better.termination_reason ?? better.family_termination_reason ?? "",
            candidate_nodes: asInteger(better.nodes_consumed || better.family_total_consumed),
            baseline_score: baseRoot.best_score ? scoreText(baseRoot.best_score) : "",
            candidate_score: scoreText(better.best_score),
          });
        }
      }
    }
  }
  return rows;
}

function summarizeRootStarvation(rows) {
  return ["v4", "v5", "v5.1"].map((variant) => {
    const selected = rows.filter((row) => row.candidate_variant === variant);
    const scenarios = new Set(selected.map((row) => row.scenario));
    return { variant, matched_roots: selected.length, distinct_scenarios: scenarios.size, criterion_d: scenarios.size >= 3 };
  });
}

function phaseCandidate(phase) {
  return {
    candidate_id: `phase-reallocation-${phase.phase}`,
    family: phase.phase.includes("plateau") ? "plateau/LNS" : phase.phase.includes("seed") ? "seed portfolio" : "phase-budget allocation",
    risk_class: "Q0",
    exact_mechanism: `reduce the ${phase.phase} reservation and transfer the reclaimed node tokens to a deterministic live downstream DFS frontier before repair completion`,
    evidence_gate: "A",
    breadth: phase.material_cases,
    material_share: phase.aggregate_budget_share,
    supported_decision: "NEED MORE EVIDENCE",
    missing_evidence: "deterministic shadow budget-reallocation replay proving that removed quota reaches a live downstream frontier",
  };
}

function buildShortlist(controlSummaries, rootSummaries, repair, phases, underutilization) {
  const shortlist = [];
  for (const control of controlSummaries) {
    if (!control.criterion_c) continue;
    const mechanism = CONTROL_MECHANISMS[control.variant];
    shortlist.push({
      ...mechanism,
      evidence_gate: "C",
      breadth: control.distinct_win_scenarios,
      material_share: 0,
      supported_decision: control.isolated_promotion_gate ? "PROMOTE" : "NEED MORE EVIDENCE",
      missing_evidence: mechanism.isolated ? "" : "one-variable shadow replay separating root selection from packing beam and allocation",
    });
  }
  const v4Roots = rootSummaries.find((entry) => entry.variant === "v4");
  if (v4Roots?.criterion_d) {
    shortlist.push({
      ...CONTROL_MECHANISMS.v4,
      evidence_gate: "D",
      breadth: v4Roots.distinct_scenarios,
      material_share: 0,
      supported_decision: "PROMOTE",
      missing_evidence: "",
    });
  }
  if (repair.criterion_e) {
    shortlist.push({
      candidate_id: "repair-weighted-v1",
      family: "repair allocation",
      risk_class: "Q0",
      exact_mechanism: "replace equal per-neighborhood repair quota within each elite base with deterministic rank-weighted allocation while preserving equal budget across elite bases and an exploration floor",
      evidence_gate: "E",
      breadth: repair.improvement_cases,
      material_share: repair.aggregate_budget_share,
      supported_decision: "NEED MORE EVIDENCE",
      missing_evidence: "per-neighborhood elite-base ID, stable rank/key/operator, allocated and consumed nodes, first strict semantic-improvement node, best semantic delta, and final-score producer flag",
    });
  }
  for (const phase of phases.filter((entry) => entry.criterion_a)) shortlist.push(phaseCandidate(phase));
  if (underutilization.criterion_b) {
    shortlist.push({
      candidate_id: "stage-headroom-reclaim-v1",
      family: "phase-budget allocation",
      risk_class: "Q0",
      exact_mechanism: "reclaim unused stage headroom into one final deterministic normal DFS continuation before repair/refine completion",
      evidence_gate: "B",
      breadth: underutilization.cases_at_least_5_percent,
      material_share: underutilization.total_unused_nodes / (EXPECTED_CASES.length * 1000000),
      supported_decision: "NEED MORE EVIDENCE",
      missing_evidence: "probe proving a live chargeable DFS frontier exists at the exact point capacity is returned",
    });
  }
  const unique = new Map();
  for (const entry of shortlist) {
    const key = `${entry.candidate_id}|${entry.evidence_gate}`;
    unique.set(key, entry);
  }
  return [...unique.values()].sort((left, right) => left.candidate_id.localeCompare(right.candidate_id) || left.evidence_gate.localeCompare(right.evidence_gate));
}

export function classifyDecision({ controlSummaries, rootSummaries, repair, phases, underutilization, shortlist }) {
  const v4 = controlSummaries.find((entry) => entry.variant === "v4");
  if (v4?.isolated_promotion_gate) {
    return {
      kind: "PROMOTE",
      candidate_id: CONTROL_MECHANISMS.v4.candidate_id,
      family: CONTROL_MECHANISMS.v4.family,
      risk_class: CONTROL_MECHANISMS.v4.risk_class,
      exact_mechanism: CONTROL_MECHANISMS.v4.exact_mechanism,
      evidence_gate: "C",
      rationale: `isolated v4 control recorded ${v4.wins} wins, ${v4.losses} losses, net ${v4.net_wins}, and zero high-severity losses`,
      missing_evidence: "",
    };
  }
  const nonIsolated = controlSummaries
    .filter((entry) => (entry.variant === "v5" || entry.variant === "v5.1") && entry.criterion_c)
    .sort((left, right) => right.distinct_win_scenarios - left.distinct_win_scenarios || right.net_wins - left.net_wins || left.variant.localeCompare(right.variant))[0];
  if (nonIsolated) {
    const mechanism = CONTROL_MECHANISMS[nonIsolated.variant];
    return {
      kind: "NEED MORE EVIDENCE",
      candidate_id: mechanism.candidate_id,
      family: mechanism.family,
      risk_class: mechanism.risk_class,
      exact_mechanism: mechanism.exact_mechanism,
      evidence_gate: "C",
      rationale: `${nonIsolated.variant} recorded ${nonIsolated.wins} semantic wins, but changes root selection, packing beam, and allocation together`,
      missing_evidence: "one-variable deterministic shadow replay separating root selection from packing beam and allocation",
    };
  }
  const v4Roots = rootSummaries.find((entry) => entry.variant === "v4");
  if (v4Roots?.criterion_d) {
    return {
      kind: "PROMOTE",
      candidate_id: CONTROL_MECHANISMS.v4.candidate_id,
      family: CONTROL_MECHANISMS.v4.family,
      risk_class: CONTROL_MECHANISMS.v4.risk_class,
      exact_mechanism: CONTROL_MECHANISMS.v4.exact_mechanism,
      evidence_gate: "D",
      rationale: `equal allocation completed a better matched root after baseline budget termination in ${v4Roots.distinct_scenarios} scenarios`,
      missing_evidence: "",
    };
  }
  if (repair.criterion_e) {
    return {
      kind: "NEED MORE EVIDENCE",
      candidate_id: "repair-weighted-v1",
      family: "repair allocation",
      risk_class: "Q0",
      exact_mechanism: "replace equal per-neighborhood repair quota within each elite base with deterministic rank-weighted allocation while preserving equal budget across elite bases and an exploration floor",
      evidence_gate: "E",
      rationale: `repair is material in ${repair.material_cases} cases, improves the semantic incumbent in ${repair.improvement_cases}, and first produces the final score in ${repair.final_producer_cases}, but aggregate reports cannot attribute productivity to neighborhood rank or quota`,
      missing_evidence: "record per-neighborhood elite-base ID, stable rank/key/operator, allocated and consumed nodes, first strict semantic-improvement node, best semantic delta, and final-score producer flag without changing search",
    };
  }
  const phase = phases.filter((entry) => entry.criterion_a).sort((left, right) => right.material_cases - left.material_cases || right.aggregate_budget_share - left.aggregate_budget_share || left.phase.localeCompare(right.phase))[0];
  if (phase) {
    const candidate = phaseCandidate(phase);
    return {
      kind: "NEED MORE EVIDENCE",
      candidate_id: candidate.candidate_id,
      family: candidate.family,
      risk_class: candidate.risk_class,
      exact_mechanism: candidate.exact_mechanism,
      evidence_gate: "A",
      rationale: `${phase.phase} is material in ${phase.material_cases} cases and dead in ${phase.dead_material_cases}, but removal has not been shown to fund a live downstream frontier`,
      missing_evidence: candidate.missing_evidence,
    };
  }
  if (underutilization.criterion_b) {
    return {
      kind: "NEED MORE EVIDENCE",
      candidate_id: "stage-headroom-reclaim-v1",
      family: "phase-budget allocation",
      risk_class: "Q0",
      exact_mechanism: "reclaim unused stage headroom into one final deterministic normal DFS continuation before repair/refine completion",
      evidence_gate: "B",
      rationale: `${underutilization.cases_at_least_5_percent} scenarios return at least 5% of MaxNodes unused, but reclaimability is not established`,
      missing_evidence: "probe proving a live chargeable DFS frontier exists at the exact point capacity is returned",
    };
  }
  return {
    kind: "DECLINE",
    candidate_id: "none",
    family: "none",
    risk_class: "none",
    exact_mechanism: "none",
    evidence_gate: "none",
    rationale: "no Q0/Q1 family passed the frozen A-E evidence gates",
    missing_evidence: "",
  };
}

function runCensusRows(reports, phaseRows) {
  const phaseByRun = new Map();
  for (const row of phaseRows) {
    const key = `${row.variant}|${row.scenario}|${row.budget}`;
    const value = phaseByRun.get(key) ?? [];
    value.push(row);
    phaseByRun.set(key, value);
  }
  const rows = [];
  for (const [variant, report] of reports.entries()) {
    for (const run of report.runs) {
      const phases = phaseByRun.get(`${variant}|${run.scenario}|${run.budget}`) ?? [];
      rows.push({
        scenario: run.scenario,
        budget: run.budget,
        variant,
        semantic_score: scoreText(run.score),
        canonical_hash: run.canonical_layout_hash ?? "",
        layout_key: run.layout_key ?? "",
        elapsed_ms: asInteger(run.elapsed_ms),
        configured_nodes: asInteger(run.search?.normal_budget_configured),
        consumed_nodes: asInteger(run.search?.global_budget_consumed),
        unused_nodes: asInteger(run.search?.unused_global_nodes),
        first_complete_node: asInteger(run.search?.first_complete_nodes),
        first_complete_phase: run.search?.first_complete_phase ?? "",
        final_checkpoint: firstCheckpointProducingFinal(run),
        final_phase: phases.find((phase) => phase.first_produced_final_semantic_score)?.phase ?? "",
        seed_best: scoreText(run.search?.seed_best),
        search_best: scoreText(run.search?.search_best),
        post_repair_best: scoreText(run.search?.post_repair_best),
        refine_best: scoreText(run.search?.refine_best),
        repair_nodes: asInteger(run.search?.repair_nodes),
        repair_improvements: asInteger(run.search?.repair_improvements),
        repair_candidates: asInteger(run.search?.repair_candidates),
        constellation_seed_nodes: asInteger(run.search?.constellation_seed_nodes),
      });
    }
  }
  return rows.sort((left, right) => left.variant.localeCompare(right.variant) || left.scenario.localeCompare(right.scenario) || left.budget - right.budget);
}

function csvEscape(value) {
  if (value === null || value === undefined) return "";
  const text = typeof value === "boolean" ? (value ? "true" : "false") : String(value);
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

function csv(rows, columns = null) {
  const headers = columns ?? (rows.length > 0 ? Object.keys(rows[0]) : []);
  return `${headers.join(",")}\n${rows.map((row) => headers.map((header) => csvEscape(row[header])).join(",")).join("\n")}${rows.length > 0 ? "\n" : ""}`;
}

async function sha256File(file) {
  return createHash("sha256").update(await readFile(file)).digest("hex");
}

async function validateFrozenManifest(artifactDir) {
  const manifestPath = path.join(artifactDir, "raw-manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  let bytes = 0;
  for (const entry of manifest.files ?? []) {
    const file = path.join(artifactDir, ...entry.path.split("/"));
    const fileStat = await stat(file);
    const hash = await sha256File(file);
    if (fileStat.size !== entry.bytes || hash !== entry.sha256) throw new Error(`raw manifest mismatch ${entry.path}`);
    bytes += fileStat.size;
  }
  if ((manifest.files ?? []).length !== manifest.raw_file_count || bytes !== manifest.raw_bytes) throw new Error("raw manifest aggregate mismatch");
  return { raw_file_count: manifest.raw_file_count, raw_bytes: bytes, manifest_sha256: await sha256File(manifestPath) };
}

function findingsMarkdown(summary) {
  const controls = summary.control_summaries.map((row) => `| ${row.variant} | ${row.wins} | ${row.ties} | ${row.losses} | ${row.layout_only_changes} | ${row.high_severity_losses} | ${row.criterion_c ? "PASS" : "FAIL"} |`).join("\n");
  const phases = summary.phase_summary.filter((row) => row.material_cases > 0).sort((left, right) => right.total_charged_nodes - left.total_charged_nodes).map((row) => `| ${row.phase} | ${row.material_cases} | ${row.dead_material_cases} | ${row.improvement_cases} | ${row.final_producer_cases} | ${(100 * row.aggregate_budget_share).toFixed(2)}% | ${row.criterion_a ? "PASS" : "FAIL"} |`).join("\n");
  return `# E1-A efficacy findings

Baseline: \`${EXPECTED_SHA}\`. Only locked development cases \`gsv2-013..026\` were materialized. Validation and both holdouts remained closed.

The official matrix contains ${summary.matrices.quality_runs} non-diagnostic quality runs and ${summary.matrices.diagnostic_runs} diagnostic twins. All diagnostic pairs, node ledgers, catalog identities, run keys, and frozen raw hashes passed validation.

## Existing-policy controls at 1M

| Control | Wins | Ties | Losses | Layout-only | High-severity losses | Gate C |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
${controls}

## Baseline phase census at 1M

| Phase | Material cases | Dead material cases | Improvement cases | Final producer cases | Aggregate budget share | Gate A |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
${phases}

Repair is material in ${summary.repair_evidence.material_cases} cases, records strict semantic incumbent improvements in ${summary.repair_evidence.improvement_cases}, and first produces the final semantic score in ${summary.repair_evidence.final_producer_cases}. Final unused nodes reach 5% of \`MaxNodes\` in ${summary.underutilization.cases_at_least_5_percent} cases.

## Decision

\`\`\`text
${summary.decision.kind}: ${summary.decision.candidate_id}
${summary.decision.exact_mechanism}
\`\`\`

Rationale: ${summary.decision.rationale}.

${summary.decision.missing_evidence ? `Exact missing evidence: ${summary.decision.missing_evidence}.` : "No additional E1-A evidence is required before implementing the selected opt-in E1-B mechanism."}

This decision selects one mechanism only. It does not authorize E1-B, validation materialization, default promotion, or any solver change in this pull request.
`;
}

async function loadReports(artifactDir) {
  const reports = new Map();
  const diagnosticReports = new Map();
  let catalogSHA = null;
  for (const spec of REPORT_SPECS) {
    const report = JSON.parse(await readFile(path.join(artifactDir, ...spec.file.split("/")), "utf8"));
    validateReport(report, spec);
    if (catalogSHA === null) catalogSHA = report.catalog_sha256;
    if (report.catalog_sha256 !== catalogSHA) throw new Error(`${spec.id} catalog SHA differs`);
    (spec.kind === "quality" ? reports : diagnosticReports).set(spec.variant, report);
  }
  return { reports, diagnosticReports, catalogSHA };
}

async function analyze(artifactDir, outDir) {
  const preManifest = await validateFrozenManifest(artifactDir);
  const { reports, diagnosticReports, catalogSHA } = await loadReports(artifactDir);
  let pairedRuns = 0;
  const phaseRows = [];
  for (const variant of reports.keys()) {
    const quality = indexRuns(reports.get(variant));
    const diagnostic = indexRuns(diagnosticReports.get(variant));
    for (const [key, qualityRun] of quality.entries()) {
      const diagnosticRun = diagnostic.get(key);
      if (!diagnosticRun) throw new Error(`missing diagnostic twin ${variant} ${key}`);
      validateDiagnosticPair(qualityRun, diagnosticRun);
      validateAccounting(diagnosticRun);
      pairedRuns += 1;
      phaseRows.push(...phaseRowsForRun(diagnosticRun, variant));
    }
  }

  const comparisons = [];
  const baselineQuality = indexRuns(reports.get("general-search-v1"));
  const baselineDiagnostic = indexRuns(diagnosticReports.get("general-search-v1"));
  for (const variant of ["v4", "v5", "v5.1"]) {
    const candidateQuality = indexRuns(reports.get(variant));
    const candidateDiagnostic = indexRuns(diagnosticReports.get(variant));
    for (const scenario of EXPECTED_CASES) {
      const key = `${scenario}|1000000|1`;
      comparisons.push(comparisonRow(baselineQuality.get(key), candidateQuality.get(key), baselineDiagnostic.get(key), candidateDiagnostic.get(key), variant));
    }
  }
  const controlSummaries = summarizeControls(comparisons);
  const phases = summarizePhases(phaseRows);
  const repair = repairEvidence(phaseRows);
  const underutilization = underutilizationEvidence(reports.get("general-search-v1").runs);
  const starvationRows = rootStarvationRows(reports, diagnosticReports);
  const rootSummaries = summarizeRootStarvation(starvationRows);
  const shortlist = buildShortlist(controlSummaries, rootSummaries, repair, phases, underutilization);
  const decision = classifyDecision({ controlSummaries, rootSummaries, repair, phases, underutilization, shortlist });
  const census = runCensusRows(diagnosticReports, phaseRows);
  const summary = {
    schema_version: 1,
    baseline_sha: EXPECTED_SHA,
    catalog_sha256: catalogSHA,
    corpus: { role: "development", cases: EXPECTED_CASES, validation_materialized: false, public_holdout_materialized: false, private_holdout_materialized: false },
    matrices: { quality_runs: [...reports.values()].reduce((sum, report) => sum + report.runs.length, 0), diagnostic_runs: [...diagnosticReports.values()].reduce((sum, report) => sum + report.runs.length, 0), paired_runs: pairedRuns },
    raw_manifest: preManifest,
    accounting: { status: "PASS", diagnostic_pairs: pairedRuns, ledger_runs: pairedRuns },
    control_summaries: controlSummaries,
    phase_summary: phases,
    repair_evidence: repair,
    underutilization,
    root_starvation_summary: rootSummaries,
    shortlist,
    decision,
  };

  await mkdir(outDir, { recursive: true });
  await writeFile(path.join(outDir, "analysis-summary.json"), `${JSON.stringify(summary, null, 2)}\n`, "utf8");
  await writeFile(path.join(outDir, "semantic-comparisons.csv"), csv(comparisons), "utf8");
  await writeFile(path.join(outDir, "run-census.csv"), csv(census), "utf8");
  await writeFile(path.join(outDir, "phase-productivity.csv"), csv(phaseRows), "utf8");
  await writeFile(path.join(outDir, "phase-summary.csv"), csv(phases), "utf8");
  await writeFile(path.join(outDir, "root-starvation.csv"), csv(starvationRows, ["scenario", "candidate_variant", "root_key_sha256", "baseline_completed", "baseline_termination", "baseline_nodes", "candidate_completed", "candidate_termination", "candidate_nodes", "baseline_score", "candidate_score"]), "utf8");
  await writeFile(path.join(outDir, "candidate-shortlist.csv"), csv(shortlist), "utf8");
  await writeFile(path.join(outDir, "decision.json"), `${JSON.stringify(decision, null, 2)}\n`, "utf8");
  await writeFile(path.join(outDir, "e1a-findings.generated.md"), findingsMarkdown(summary), "utf8");
  await writeFile(path.join(outDir, "accounting-validation.txt"), `status=PASS\nbaseline_sha=${EXPECTED_SHA}\ndiagnostic_pairs=${pairedRuns}\nledger_runs=${pairedRuns}\nquality_runs=${summary.matrices.quality_runs}\ndiagnostic_runs=${summary.matrices.diagnostic_runs}\nvalidation_materialized=false\npublic_holdout_materialized=false\nprivate_holdout_materialized=false\n`, "utf8");
  const postManifest = await validateFrozenManifest(artifactDir);
  if (!sameJSON(preManifest, postManifest)) throw new Error("raw manifest changed during analysis");
  await writeFile(path.join(outDir, "post-analysis-validation.txt"), `status=PASS\nraw_file_count=${postManifest.raw_file_count}\nraw_bytes=${postManifest.raw_bytes}\nraw_manifest_sha256=${postManifest.manifest_sha256}\npost_freeze_solver_runs=0\n`, "utf8");
  return summary;
}

function parseArgs(argv) {
  const args = { artifact: "", out: "" };
  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    if (arg === "--artifact") args.artifact = argv[++index] ?? "";
    else if (arg === "--out") args.out = argv[++index] ?? "";
    else if (arg === "--preflight") args.preflight = true;
    else throw new Error(`unknown argument ${arg}`);
  }
  return args;
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  if (args.preflight) {
    const win = firstDifference({ priority_counts: [6, 1] }, { priority_counts: [5, 99] });
    if (win.difference_level !== 0 || win.priority_index !== 0 || win.signed_delta !== 1) throw new Error("semantic preflight failed");
    process.stdout.write("E1-A analyzer preflight PASS\n");
    return;
  }
  if (!args.artifact || !args.out) throw new Error("usage: node e1a-analysis.mjs --artifact <official-bundle> --out <derived-dir>");
  const summary = await analyze(path.resolve(args.artifact), path.resolve(args.out));
  process.stdout.write(`${summary.decision.kind}: ${summary.decision.candidate_id}\n`);
}

const invokedPath = process.argv[1] ? pathToFileURL(path.resolve(process.argv[1])).href : "";
if (invokedPath === import.meta.url) {
  main().catch((error) => {
    process.stderr.write(`ERROR: ${error.stack ?? error.message}\n`);
    process.exitCode = 1;
  });
}
