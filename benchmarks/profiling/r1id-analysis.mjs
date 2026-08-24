import fs from "node:fs";
import path from "node:path";

const artifactDir = process.argv[2];
const outputDir = process.argv[3] ?? (artifactDir ? path.join(artifactDir, "review") : undefined);
if (!artifactDir) {
  throw new Error("usage: node r1id-analysis.mjs <artifact-dir> [output-dir]");
}

const expectedSHA = "6952a35ef62f84646a01a887310363450c833b83";
const expectedCases = Array.from({ length: 14 }, (_, index) => `gsv2-${String(index + 13).padStart(3, "0")}`);
const profileCases = ["gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024"];
const prioritySites = ["constellation_filter", "repair_dfs", "plateau_prefilter", "plateau_dfs"];
const outgoingSites = ["search", "repair"];
const priorityFields = [
  "calls",
  "feasible_results",
  "rejected_results",
  "invalid_priority_returns",
  "priority_entries_validated",
  "fixed_placement_inputs",
  "current_placement_inputs",
  "anchored_placements",
  "removed_instance_inputs",
  "removed_instances",
  "removed_option_candidates",
  "removed_option_rejected_fixed_overlap",
  "removed_option_rejected_outside_free",
  "removed_options_retained",
  "unique_priority_source_items",
  "anchored_source_instances",
  "removed_source_instances",
  "star_slots",
  "fixed_target_checks",
  "removed_target_checks",
  "self_target_skips",
  "fixed_fixed_geometry_checks",
  "removed_source_option_checks_fixed_target",
  "fixed_source_target_option_checks",
  "removed_source_target_option_pairs",
  "geometry_candidate_checks",
  "geometry_overlap_rejects",
  "star_position_hit_calls",
  "star_position_hit_true",
  "slot_target_hits",
  "matching_calls",
];
const outgoingFields = [
  "checks",
  "pruned_nodes",
  "placed_map_builds",
  "placed_map_insertions",
  "placed_mask_instance_checks",
  "priority_iterations",
  "source_instance_iterations",
  "priority_source_matches",
  "zero_star_source_skips",
  "placed_source_iterations",
  "free_source_iterations",
  "placed_source_target_iterations",
  "self_target_skips",
  "target_placement_lookups",
  "placed_targets_found",
  "unplaced_targets",
  "source_hits_target_calls",
  "source_hits_target_true",
  "coverage_placement_key_calls",
  "placed_potential_lookups",
  "free_potential_lookups",
  "popcount_calls",
  "star_count_clamps",
];

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) throw new Error(`${message}: got ${actual}, want ${expected}`);
}

function readJSON(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(artifactDir, relativePath), "utf8"));
}

function ratio(numerator, denominator) {
  return denominator ? numerator / denominator : 0;
}

function blankCounters(fields) {
  return Object.fromEntries(fields.map((field) => [field, 0]));
}

function addCounters(target, source, fields) {
  for (const field of fields) target[field] += source?.[field] ?? 0;
}

function totalPriority(profile) {
  const total = blankCounters(priorityFields);
  for (const site of prioritySites) addCounters(total, profile.priority_upper[site], priorityFields);
  return total;
}

function totalOutgoing(profile) {
  const total = blankCounters(outgoingFields);
  for (const site of outgoingSites) addCounters(total, profile.outgoing[site], outgoingFields);
  return total;
}

function validatePrioritySite(site, label) {
  assertEqual(site.calls, site.feasible_results + site.rejected_results, `${label} calls/outcomes`);
  assertEqual(
    site.removed_option_candidates,
    site.removed_option_rejected_fixed_overlap + site.removed_option_rejected_outside_free + site.removed_options_retained,
    `${label} removed options`,
  );
  assertEqual(
    site.geometry_candidate_checks,
    site.fixed_fixed_geometry_checks +
      site.removed_source_option_checks_fixed_target +
      site.fixed_source_target_option_checks +
      site.removed_source_target_option_pairs,
    `${label} geometry regimes`,
  );
  assertEqual(
    site.geometry_candidate_checks,
    site.geometry_overlap_rejects + site.star_position_hit_calls,
    `${label} geometry outcomes`,
  );
  assertEqual(site.star_position_hit_true, site.slot_target_hits, `${label} slot hits`);
}

function validateOutgoingSite(site, label) {
  assertEqual(
    site.priority_source_matches,
    site.zero_star_source_skips + site.placed_source_iterations + site.free_source_iterations,
    `${label} source regimes`,
  );
  assertEqual(
    site.placed_source_target_iterations,
    site.self_target_skips + site.target_placement_lookups,
    `${label} target scan`,
  );
  assertEqual(site.target_placement_lookups, site.placed_targets_found + site.unplaced_targets, `${label} target lookup`);
  assertEqual(site.source_hits_target_calls, site.placed_targets_found, `${label} hit calls`);
  assertEqual(site.coverage_placement_key_calls, site.placed_source_iterations, `${label} coverage keys`);
  assertEqual(site.placed_potential_lookups, site.placed_source_iterations, `${label} placed potential`);
  assertEqual(site.free_potential_lookups, site.free_source_iterations, `${label} free potential`);
  assertEqual(site.popcount_calls, site.placed_source_iterations + site.free_source_iterations, `${label} popcounts`);
  assertEqual(site.placed_map_builds, site.checks, `${label} map builds`);
}

function validateProfiledRun(run, label) {
  const profile = run.search.bound_operation_profile;
  if (!profile) {
    assertEqual(run.search.outgoing_bound_checks ?? 0, 0, `${label} absent-profile checks`);
    assertEqual(run.search.outgoing_bound_pruned_nodes ?? 0, 0, `${label} absent-profile prunes`);
    return false;
  }
  assertEqual(profile.version, "bound-attribution-ops-v1", `${label} profile version`);
  for (const site of prioritySites) validatePrioritySite(profile.priority_upper[site], `${label}/${site}`);
  assertEqual(
    profile.priority_upper.constellation_states_input,
    profile.priority_upper.constellation_states_retained + profile.priority_upper.constellation_states_rejected,
    `${label} constellation outcomes`,
  );
  assertEqual(
    profile.priority_upper.constellation_filter.calls,
    profile.priority_upper.constellation_states_input,
    `${label} constellation calls`,
  );
  for (const site of outgoingSites) validateOutgoingSite(profile.outgoing[site], `${label}/${site}`);
  assertEqual(
    profile.outgoing.search.checks + profile.outgoing.repair.checks,
    run.search.outgoing_bound_checks ?? 0,
    `${label} authoritative outgoing checks`,
  );
  assertEqual(
    profile.outgoing.search.pruned_nodes + profile.outgoing.repair.pruned_nodes,
    run.search.outgoing_bound_pruned_nodes ?? 0,
    `${label} authoritative outgoing prunes`,
  );
  return true;
}

function validateMatrix(report, label, budgets, variant) {
  assertEqual(report.build_revision, expectedSHA, `${label} revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.operation_profiling, true, `${label} operation profile`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
  assertEqual(report.constellation_seed_variant, variant, `${label} variant`);
  assertEqual(report.runs.length, expectedCases.length * budgets.length, `${label} run count`);
  for (const scenario of expectedCases) {
    for (const budget of budgets) {
      assertEqual(
        report.runs.filter((run) => run.scenario === scenario && run.budget === budget).length,
        1,
        `${label} matrix ${scenario}/${budget}`,
      );
    }
  }
  return report.runs.map((run) => validateProfiledRun(run, `${label}/${run.scenario}/${run.budget}`));
}

const semanticIgnoredFields = new Set([
  "generated_at",
  "build_revision",
  "elapsed_ms",
  "nodes_per_second",
  "setup_ms",
  "seed_ms",
  "repair_ms",
  "search_ms",
  "refine_ms",
  "server_elapsed_ms",
  "first_complete_ms",
  "first_fully_packed_ms",
  "operation_profiling",
  "packing_seed_operation_profile",
  "bound_operation_profile",
  "root_packing_operation_profile",
  "operation_profile",
]);

function normalize(value) {
  if (Array.isArray(value)) return value.map(normalize);
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value)
        .filter((key) => !semanticIgnoredFields.has(key))
        .sort()
        .map((key) => [key, normalize(value[key])]),
    );
  }
  return value;
}

function assertSemanticEqual(actual, expected, label) {
  assertEqual(JSON.stringify(normalize(actual)), JSON.stringify(normalize(expected)), label);
}

function validateNormalReport(report, label, cases, variant) {
  assertEqual(report.build_revision, expectedSHA, `${label} revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.operation_profiling ?? false, false, `${label} operation profile`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
  assertEqual(report.constellation_seed_variant, variant, `${label} variant`);
  assertEqual(report.runs.length, cases.length, `${label} run count`);
  for (const scenario of cases) {
    assertEqual(
      report.runs.filter((run) => run.scenario === scenario && run.budget === 1000000).length,
      1,
      `${label} matrix ${scenario}`,
    );
  }
  for (const run of report.runs) {
    assert(!run.search.bound_operation_profile, `${label}/${run.scenario} contains bound profile`);
    assert(!run.search.packing_seed_operation_profile, `${label}/${run.scenario} contains packing profile`);
  }
}

const smokeNormal = readJSON("smoke/r1id-smoke-normal.json");
const smokeTaggedOff = readJSON("smoke/r1id-smoke-tagged-off.json");
const smokeTaggedOn = readJSON("smoke/r1id-smoke-tagged-on.json");
for (const [label, report] of [
  ["normal", smokeNormal],
  ["tagged-off", smokeTaggedOff],
  ["tagged-on", smokeTaggedOn],
]) {
  assertEqual(report.build_revision, expectedSHA, `smoke ${label} revision`);
  assertEqual(report.workers, 1, `smoke ${label} workers`);
  assertEqual(report.repeat, 1, `smoke ${label} repeat`);
  assertEqual(report.constellation_seed_variant, "general-search-v1", `smoke ${label} variant`);
  assertEqual(report.runs.length, 1, `smoke ${label} run count`);
  assertEqual(report.runs[0].scenario, "gsv2-013", `smoke ${label} scenario`);
  assertEqual(report.runs[0].budget, 250000, `smoke ${label} budget`);
}
assertEqual(smokeNormal.operation_profiling ?? false, false, "smoke normal profile flag");
assertEqual(smokeTaggedOff.operation_profiling ?? false, false, "smoke tagged-off profile flag");
assertEqual(smokeTaggedOn.operation_profiling, true, "smoke tagged-on profile flag");
assertSemanticEqual(smokeTaggedOff, smokeNormal, "smoke normal/tagged-off semantics");
assertSemanticEqual(smokeTaggedOn, smokeNormal, "smoke normal/tagged-on semantics");
validateProfiledRun(smokeTaggedOn.runs[0], "smoke/tagged-on");

const gsv1 = readJSON("operations/r1id-gsv1.json");
const v4 = readJSON("operations/r1id-v4.json");
const gsv1Summary = readJSON("operations/r1id-gsv1-summary.json");
const v4Summary = readJSON("operations/r1id-v4-summary.json");
const gsv1Presence = validateMatrix(gsv1, "GSV1 operations", [250000, 1000000], "general-search-v1");
const v4Presence = validateMatrix(v4, "V4 operations", [1000000], "v4");
assertEqual(gsv1Summary.version, "operation-profile-summary-v3", "GSV1 summary version");
assertEqual(v4Summary.version, "operation-profile-summary-v3", "V4 summary version");

const combinedGSV1 = readJSON("profiles/r1id-gsv1-profile.json");
const combinedV4 = readJSON("profiles/r1id-v4-profile.json");
validateNormalReport(combinedGSV1, "combined GSV1 profile", profileCases, "general-search-v1");
validateNormalReport(combinedV4, "combined V4 profile", profileCases, "v4");
for (const scenario of profileCases) {
  const report = readJSON(`profiles/per-case/${scenario}.json`);
  validateNormalReport(report, `per-case ${scenario}`, [scenario], "general-search-v1");
}

function aggregateRuns(report, budget) {
  const priority = blankCounters(priorityFields);
  const outgoing = blankCounters(outgoingFields);
  const cases = [];
  for (const run of report.runs.filter((candidate) => candidate.budget === budget)) {
    const profile = run.search.bound_operation_profile;
    const currentPriority = profile ? totalPriority(profile) : blankCounters(priorityFields);
    const currentOutgoing = profile ? totalOutgoing(profile) : blankCounters(outgoingFields);
    addCounters(priority, currentPriority, priorityFields);
    addCounters(outgoing, currentOutgoing, outgoingFields);
    cases.push({ scenario: run.scenario, priority: currentPriority, outgoing: currentOutgoing });
  }
  return { priority, outgoing, cases };
}

const gsv1OneMillion = aggregateRuns(gsv1, 1000000);
const v4OneMillion = aggregateRuns(v4, 1000000);

const input = readJSON("review-input/r1id-candidates.json");
assertEqual(input.version, "r1id-candidate-input-v1", "candidate input version");
assertEqual(input.frozen_sha, expectedSHA, "candidate input revision");
assert(Number.isFinite(input.combined_cpu_seconds) && input.combined_cpu_seconds > 0, "invalid combined CPU total");
assert(Number.isFinite(input.combined_heap_alloc_space_bytes) && input.combined_heap_alloc_space_bytes > 0, "invalid heap alloc-space total");
assertEqual(Object.keys(input.per_case_total_cpu_seconds).sort().join(","), [...profileCases].sort().join(","), "per-case CPU keys");

const candidatesByID = new Map();
for (const candidate of input.candidates) {
  assert(candidate.id && !candidatesByID.has(candidate.id), `duplicate or empty candidate id: ${candidate.id}`);
  assert(candidate.mechanism && !["outgoing", "priority", "runtime.duffcopy", "runtime.memmove"].includes(candidate.mechanism), `${candidate.id} is not an exact mechanism`);
  assert(Number.isFinite(candidate.parent_cpu_seconds) && candidate.parent_cpu_seconds >= 0, `${candidate.id} parent CPU`);
  assert(Number.isFinite(candidate.target_edge_cpu_seconds) && candidate.target_edge_cpu_seconds >= 0, `${candidate.id} edge CPU`);
  assert(candidate.target_edge_cpu_seconds <= candidate.parent_cpu_seconds + 1e-9, `${candidate.id} edge exceeds parent`);
  assert(Number.isFinite(candidate.plausible_removal_fraction) && candidate.plausible_removal_fraction >= 0 && candidate.plausible_removal_fraction <= 1, `${candidate.id} removal fraction`);
  assert(["C0", "C1", "C2"].includes(candidate.complexity_class), `${candidate.id} complexity class`);
  assert(candidate.overlap_group, `${candidate.id} overlap group`);
  assert(["PROMOTE", "REJECT", "MORE_EVIDENCE"].includes(candidate.disposition), `${candidate.id} disposition`);
  assertEqual(Object.keys(candidate.per_case_target_cpu_seconds).sort().join(","), [...profileCases].sort().join(","), `${candidate.id} per-case CPU keys`);
  for (const [scenario, seconds] of Object.entries(candidate.per_case_target_cpu_seconds)) {
    assert(Number.isFinite(seconds) && seconds >= 0 && seconds <= input.per_case_total_cpu_seconds[scenario] + 1e-9, `${candidate.id}/${scenario} CPU seconds`);
  }
  candidatesByID.set(candidate.id, candidate);
}

const allowedInventoryKinds = new Set([
  "cpu_edge_ge_1pct",
  "cpu_parent_ge_1_5pct",
  "cpu_flat_solver_owned_top20",
  "cpu_cum_solver_owned_top20",
  "heap_alloc_space_ge_1pct",
  "heap_alloc_objects_solver_owned_top10",
  "carry_forward",
]);
for (const entry of input.inventory) {
  assert(allowedInventoryKinds.has(entry.kind), `unknown inventory kind: ${entry.kind}`);
  assert(entry.symbol || entry.mechanism, `inventory entry has no symbol/mechanism: ${entry.kind}`);
  assert(Boolean(entry.candidate_id) !== Boolean(entry.exclusion_reason), `inventory entry must map or exclude exactly once: ${entry.kind}/${entry.symbol ?? entry.mechanism}`);
  if (entry.candidate_id) assert(candidatesByID.has(entry.candidate_id), `inventory maps to unknown candidate: ${entry.candidate_id}`);
  if (entry.kind === "cpu_edge_ge_1pct") assert(entry.fraction >= 0.01, `edge inventory below 1%: ${entry.symbol}`);
  if (entry.kind === "cpu_parent_ge_1_5pct") assert(entry.fraction >= 0.015, `parent inventory below 1.5%: ${entry.symbol}`);
  if (entry.kind === "heap_alloc_space_ge_1pct") assert(entry.fraction >= 0.01, `alloc-space inventory below 1%: ${entry.symbol}`);
}
for (const [kind, count] of [
  ["cpu_flat_solver_owned_top20", 20],
  ["cpu_cum_solver_owned_top20", 20],
  ["heap_alloc_objects_solver_owned_top10", 10],
]) {
  const entries = input.inventory.filter((entry) => entry.kind === kind);
  assertEqual(entries.length, count, `${kind} inventory count`);
  assertEqual(entries.map((entry) => entry.rank).sort((a, b) => a - b).join(","), Array.from({ length: count }, (_, index) => index + 1).join(","), `${kind} ranks`);
}

function candidateOperationValues(candidate, aggregate) {
  const values = {};
  for (const metric of candidate.operation_metrics ?? []) {
    assert(["priority", "outgoing"].includes(metric.family), `${candidate.id} operation metric family`);
    const family = aggregate[metric.family];
    assert(Object.hasOwn(family, metric.field), `${candidate.id} unknown operation metric ${metric.family}.${metric.field}`);
    values[`${metric.family}.${metric.field}`] = family[metric.field];
  }
  return values;
}

function caseOperationValues(candidate, scenario) {
  const row = gsv1OneMillion.cases.find((entry) => entry.scenario === scenario);
  assert(row, `missing GSV1 operation row: ${scenario}`);
  return candidateOperationValues(candidate, row);
}

function csvEscape(value) {
  if (value === null || value === undefined) return "";
  const text = typeof value === "number" && !Number.isInteger(value) ? value.toFixed(12).replace(/0+$/, "").replace(/\.$/, "") : String(value);
  return /[",\n\r]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

function writeCSV(filename, rows) {
  assert(rows.length > 0, `${filename} has no rows`);
  const headers = Object.keys(rows[0]);
  const body = [headers.join(","), ...rows.map((row) => headers.map((header) => csvEscape(row[header])).join(","))].join("\n");
  fs.writeFileSync(path.join(outputDir, filename), `${body}\n`);
}

fs.mkdirSync(outputDir, { recursive: true });
const promotionBars = { C0: 0.02, C1: 0.025, C2: 0.03 };
const scoreRows = input.candidates.map((candidate) => {
  const operationValues = candidateOperationValues(candidate, gsv1OneMillion);
  const cpuBreadth = profileCases.filter((scenario) => candidate.per_case_target_cpu_seconds[scenario] > 0).length;
  const operationBreadth = expectedCases.filter((scenario) => {
    const values = caseOperationValues(candidate, scenario);
    return Object.values(values).some((value) => value > 0);
  }).length;
  const wholeProgramFraction = ratio(candidate.parent_cpu_seconds, input.combined_cpu_seconds);
  const targetFraction = ratio(candidate.target_edge_cpu_seconds, candidate.parent_cpu_seconds);
  const benefit = ratio(candidate.target_edge_cpu_seconds, input.combined_cpu_seconds) * candidate.plausible_removal_fraction;
  return {
    candidate_id: candidate.id,
    family: candidate.family,
    mechanism: candidate.mechanism,
    parent_cpu_seconds: candidate.parent_cpu_seconds,
    target_edge_cpu_seconds: candidate.target_edge_cpu_seconds,
    whole_program_fraction: wholeProgramFraction,
    target_fraction_of_parent: targetFraction,
    plausible_removal_fraction: candidate.plausible_removal_fraction,
    heuristic_whole_program_benefit: benefit,
    normal_promotion_bar: promotionBars[candidate.complexity_class],
    promotion_bar_gate: benefit >= promotionBars[candidate.complexity_class] ? "PASS" : "FAIL",
    cpu_breadth_cases_of_6: cpuBreadth,
    operation_breadth_cases_of_14: operationBreadth,
    operation_counter_basis: Object.keys(operationValues).join(";"),
    operation_counter_values: Object.values(operationValues).join(";"),
    v4_target_cpu_seconds: candidate.v4_target_cpu_seconds ?? "",
    alloc_space_bytes: candidate.alloc_space_bytes ?? 0,
    alloc_space_fraction: ratio(candidate.alloc_space_bytes ?? 0, input.combined_heap_alloc_space_bytes),
    alloc_objects: candidate.alloc_objects ?? 0,
    complexity_class: candidate.complexity_class,
    semantic_risk: candidate.semantic_risk,
    evidence_quality: candidate.evidence_quality,
    parent: candidate.parent ?? "",
    child: candidate.child ?? "",
    exclusive_subregion: candidate.exclusive_subregion ? "true" : "false",
    overlap_group: candidate.overlap_group,
    decision: candidate.disposition,
    rationale: candidate.rationale,
  };
});
writeCSV("candidate-scorecard.csv", scoreRows);

const caseRows = [];
for (const candidate of input.candidates) {
  for (const scenario of profileCases) {
    const operationValues = caseOperationValues(candidate, scenario);
    const targetCPU = candidate.per_case_target_cpu_seconds[scenario];
    caseRows.push({
      candidate_id: candidate.id,
      family: candidate.family,
      mechanism: candidate.mechanism,
      scenario,
      case_total_cpu_seconds: input.per_case_total_cpu_seconds[scenario],
      target_cpu_seconds: targetCPU,
      target_case_cpu_fraction: ratio(targetCPU, input.per_case_total_cpu_seconds[scenario]),
      cpu_present: targetCPU > 0 ? "true" : "false",
      operation_counter_basis: Object.keys(operationValues).join(";"),
      operation_counter_values: Object.values(operationValues).join(";"),
      operation_present: Object.values(operationValues).some((value) => value > 0) ? "true" : "false",
    });
  }
}
writeCSV("case-attribution.csv", caseRows);

const decisionKinds = new Set(["PROMOTE", "NEED_MORE_EVIDENCE", "DECLINE"]);
assert(decisionKinds.has(input.decision.kind), `unknown final decision: ${input.decision.kind}`);
const promoted = input.candidates.filter((candidate) => candidate.disposition === "PROMOTE");
if (input.decision.kind === "PROMOTE") {
  assertEqual(promoted.length, 1, "promoted candidate count");
  assertEqual(input.decision.candidate_id, promoted[0].id, "final promoted candidate");
  assertEqual(input.decision.mechanism, promoted[0].mechanism, "final promoted mechanism");
} else {
  assertEqual(promoted.length, 0, "non-PROMOTE candidate count");
  assert(!input.decision.candidate_id, "non-PROMOTE decision names a candidate");
}
if (input.decision.kind === "NEED_MORE_EVIDENCE") {
  assert(input.decision.missing_data && input.decision.instrumentation, "NEED_MORE_EVIDENCE must name missing data and instrumentation");
}
if (input.decision.kind === "DECLINE") {
  assert(input.decision.rationale, "DECLINE must include a rationale");
}

const accountingLines = [
  "R1I-D accounting validation",
  `frozen_sha=${expectedSHA}`,
  "profile_version=bound-attribution-ops-v1",
  "summary_version=operation-profile-summary-v3",
  "smoke_normal_tagged_off_tagged_on_semantics=PASS",
  `smoke_ignored_fields=${[...semanticIgnoredFields].sort().join(",")}`,
  `gsv1_matrix=PASS runs=${gsv1.runs.length}`,
  `v4_matrix=PASS runs=${v4.runs.length}`,
  `profiled_runs=${gsv1Presence.filter(Boolean).length + v4Presence.filter(Boolean).length}`,
  `zero_work_runs_without_profile=${gsv1Presence.filter((value) => !value).length + v4Presence.filter((value) => !value).length}`,
  "all_priority_site_identities=PASS",
  "all_outgoing_site_identities=PASS",
  "all_outgoing_authoritative_reconciliations=PASS",
  "combined_gsv1_cpu_heap_matrix=PASS runs=6 operation_profile=false",
  "per_case_gsv1_cpu_matrix=PASS runs=6 operation_profile=false",
  "combined_v4_cpu_control=PASS runs=6 operation_profile=false",
  "mechanical_inventory=PASS",
  "benefit_formula=PASS",
  "overlap_groups_recorded=PASS",
  "validation_materialized=false",
  "public_holdout_materialized=false",
  "private_holdout_materialized=false",
];
fs.writeFileSync(path.join(outputDir, "accounting-validation.txt"), `${accountingLines.join("\n")}\n`);

const summary = {
  version: "r1id-analysis-v1",
  frozen_sha: expectedSHA,
  smoke_semantics: "PASS",
  operation_accounting: "PASS",
  normal_profile_matrix: "PASS",
  combined_cpu_seconds: input.combined_cpu_seconds,
  combined_heap_alloc_space_bytes: input.combined_heap_alloc_space_bytes,
  gsv1_1m: {
    priority: gsv1OneMillion.priority,
    outgoing: gsv1OneMillion.outgoing,
    priority_rejection_rate: ratio(gsv1OneMillion.priority.rejected_results, gsv1OneMillion.priority.calls),
    outgoing_prune_rate: ratio(gsv1OneMillion.outgoing.pruned_nodes, gsv1OneMillion.outgoing.checks),
    map_insertions_per_check: ratio(gsv1OneMillion.outgoing.placed_map_insertions, gsv1OneMillion.outgoing.checks),
    coverage_keys_per_check: ratio(gsv1OneMillion.outgoing.coverage_placement_key_calls, gsv1OneMillion.outgoing.checks),
    targets_per_placed_source: ratio(gsv1OneMillion.outgoing.placed_source_target_iterations, gsv1OneMillion.outgoing.placed_source_iterations),
  },
  v4_1m: {
    priority: v4OneMillion.priority,
    outgoing: v4OneMillion.outgoing,
  },
  candidates: scoreRows,
  decision: input.decision,
};
fs.writeFileSync(path.join(outputDir, "analysis-summary.json"), `${JSON.stringify(summary, null, 2)}\n`);

process.stdout.write(`${JSON.stringify({
  frozen_sha: expectedSHA,
  smoke_semantics: "PASS",
  operation_runs: gsv1.runs.length + v4.runs.length,
  candidate_count: input.candidates.length,
  case_attribution_rows: caseRows.length,
  decision: input.decision,
}, null, 2)}\n`);
