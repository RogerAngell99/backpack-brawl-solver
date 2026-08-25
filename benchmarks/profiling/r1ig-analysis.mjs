import fs from "node:fs";
import path from "node:path";

const artifactDir = process.argv[2];
const outputDir = process.argv[3] ?? (artifactDir ? path.join(artifactDir, "review") : undefined);
if (!artifactDir) throw new Error("usage: node r1ig-analysis.mjs <artifact-dir> [output-dir]");

const rawDir = path.join(artifactDir, "raw");
const baselineRevision = "4c6b443e3abee2cb63953f53134cc7fd8f04593b";
const expectedCases = Array.from({ length: 14 }, (_, index) => `gsv2-${String(index + 13).padStart(3, "0")}`);
const timingCases = ["gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024"];
const prioritySites = ["constellation_filter", "repair_dfs", "plateau_prefilter", "plateau_dfs"];
const outgoingSites = ["search", "repair"];

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) throw new Error(`${message}: got ${actual}, want ${expected}`);
}

function readJSON(root, relativePath) {
  return JSON.parse(fs.readFileSync(path.join(root, relativePath), "utf8"));
}

function normalize(value, ignored) {
  if (Array.isArray(value)) return value.map((entry) => normalize(entry, ignored));
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value)
        .filter((key) => !ignored.has(key))
        .sort()
        .map((key) => [key, normalize(value[key], ignored)]),
    );
  }
  return value;
}

function assertDeepEqual(actual, expected, ignored, message) {
  assertEqual(JSON.stringify(normalize(actual, ignored)), JSON.stringify(normalize(expected, ignored)), message);
}

const timingFields = new Set([
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
]);
const semanticFields = new Set([
  ...timingFields,
  "operation_profiling",
  "packing_seed_operation_profile",
  "bound_operation_profile",
  "root_packing_operation_profile",
  "operation_profile",
]);

function validatePrioritySite(site, label) {
  assertEqual(site.calls, site.feasible_results + site.rejected_results, `${label} outcomes`);
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
  assertEqual(site.geometry_candidate_checks, site.geometry_overlap_rejects + site.star_position_hit_calls, `${label} geometry outcomes`);
  assertEqual(site.star_position_hit_true, site.slot_target_hits, `${label} slot hits`);
}

function validateOutgoingSite(site, label) {
  assertEqual(
    site.priority_source_matches,
    site.zero_star_source_skips + site.placed_source_iterations + site.free_source_iterations,
    `${label} sources`,
  );
  assertEqual(site.placed_source_target_iterations, site.self_target_skips + site.target_placement_lookups, `${label} target scan`);
  assertEqual(site.target_placement_lookups, site.placed_targets_found + site.unplaced_targets, `${label} target lookup`);
  assertEqual(site.source_hits_target_calls, site.placed_targets_found, `${label} hit calls`);
  assertEqual(site.coverage_placement_key_calls, site.placed_source_iterations, `${label} coverage keys`);
  assertEqual(site.placed_potential_lookups, site.placed_source_iterations, `${label} placed potential`);
  assertEqual(site.free_potential_lookups, site.free_source_iterations, `${label} free potential`);
  assertEqual(site.popcount_calls, site.placed_source_iterations + site.free_source_iterations, `${label} popcounts`);
  assertEqual(site.placed_map_builds, site.checks, `${label} logical map builds`);
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
  assertEqual(profile.priority_upper.constellation_filter.calls, profile.priority_upper.constellation_states_input, `${label} constellation calls`);
  for (const site of outgoingSites) validateOutgoingSite(profile.outgoing[site], `${label}/${site}`);
  assertEqual(
    profile.outgoing.search.checks + profile.outgoing.repair.checks,
    run.search.outgoing_bound_checks ?? 0,
    `${label} authoritative checks`,
  );
  assertEqual(
    profile.outgoing.search.pruned_nodes + profile.outgoing.repair.pruned_nodes,
    run.search.outgoing_bound_pruned_nodes ?? 0,
    `${label} authoritative prunes`,
  );
  return true;
}

function validateReport(report, revision, variant, scenarios, budgets, profiled, label) {
  assertEqual(report.build_revision, revision, `${label} revision`);
  assertEqual(report.constellation_seed_variant, variant, `${label} variant`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
  assertEqual(report.operation_profiling ?? false, profiled, `${label} operation profile`);
  assertEqual(report.runs.length, scenarios.length * budgets.length, `${label} run count`);
  for (const scenario of scenarios) {
    for (const budget of budgets) {
      assertEqual(
        report.runs.filter((run) => run.scenario === scenario && run.budget === budget && run.repeat === 1).length,
        1,
        `${label}/${scenario}/${budget}`,
      );
    }
  }
  return profiled ? report.runs.map((run) => validateProfiledRun(run, `${label}/${run.scenario}/${run.budget}`)) : [];
}

function compareRuns(baseline, candidate, label) {
  let comparisons = 0;
  for (const baseRun of baseline.runs) {
    const candidateRun = candidate.runs.find(
      (run) => run.scenario === baseRun.scenario && run.budget === baseRun.budget && run.repeat === baseRun.repeat,
    );
    assert(candidateRun, `${label} missing ${baseRun.scenario}/${baseRun.budget}`);
    assertDeepEqual(candidateRun, baseRun, timingFields, `${label} deterministic ${baseRun.scenario}/${baseRun.budget}`);
    comparisons++;
  }
  return comparisons;
}

function median(values) {
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 ? sorted[middle] : (sorted[middle - 1] + sorted[middle]) / 2;
}

function quartiles(values) {
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return {
    q1: median(sorted.slice(0, middle)),
    q3: median(sorted.slice(sorted.length % 2 ? middle + 1 : middle)),
  };
}

function csvEscape(value) {
  const text = value === undefined || value === null ? "" : String(value);
  return /[",\r\n]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

function writeCSV(filename, rows) {
  assert(rows.length, `${filename} has no rows`);
  const headers = Object.keys(rows[0]);
  const lines = [headers.join(","), ...rows.map((row) => headers.map((header) => csvEscape(row[header])).join(","))];
  fs.writeFileSync(path.join(outputDir, filename), `${lines.join("\n")}\n`);
}

const config = readJSON(artifactDir, "review-input/r1ig-config.json");
assertEqual(config.version, "r1ig-review-input-v1", "config version");
assertEqual(config.baseline_revision, baselineRevision, "config baseline");
const candidateRevision = config.candidate_revision;
assert(/^[0-9a-f]{40}$/.test(candidateRevision), "invalid candidate revision");

const smokeNames = [
  "smoke-base-normal.json",
  "smoke-candidate-normal.json",
  "smoke-candidate-tagged-off.json",
  "smoke-base-profiled.json",
  "smoke-candidate-profiled.json",
];
const smoke = Object.fromEntries(smokeNames.map((name) => [name, readJSON(rawDir, path.join("reports", name))]));
for (const [name, revision, profiled] of [
  [smokeNames[0], baselineRevision, false],
  [smokeNames[1], candidateRevision, false],
  [smokeNames[2], candidateRevision, false],
  [smokeNames[3], baselineRevision, true],
  [smokeNames[4], candidateRevision, true],
]) {
  validateReport(smoke[name], revision, "general-search-v1", ["gsv2-013"], [250000], profiled, name);
}
for (const name of smokeNames.slice(1)) {
  assertDeepEqual(smoke[name], smoke[smokeNames[0]], semanticFields, `smoke semantics ${name}`);
}
assertDeepEqual(smoke[smokeNames[4]].runs[0], smoke[smokeNames[3]].runs[0], timingFields, "smoke profiled exact");

const baseGSV1 = readJSON(rawDir, "reports/matrix-base-gsv1.json");
const candidateGSV1 = readJSON(rawDir, "reports/matrix-candidate-gsv1.json");
const baseV4 = readJSON(rawDir, "reports/matrix-base-v4.json");
const candidateV4 = readJSON(rawDir, "reports/matrix-candidate-v4.json");
const profiledPresence = [
  ...validateReport(baseGSV1, baselineRevision, "general-search-v1", expectedCases, [250000, 1000000], true, "base GSV1"),
  ...validateReport(candidateGSV1, candidateRevision, "general-search-v1", expectedCases, [250000, 1000000], true, "candidate GSV1"),
  ...validateReport(baseV4, baselineRevision, "v4", expectedCases, [1000000], true, "base V4"),
  ...validateReport(candidateV4, candidateRevision, "v4", expectedCases, [1000000], true, "candidate V4"),
];
const semanticComparisons = compareRuns(baseGSV1, candidateGSV1, "GSV1") + compareRuns(baseV4, candidateV4, "V4");
assertEqual(semanticComparisons, 42, "semantic comparison count");

const timingPattern = /^timing-(gsv2-\d{3})-pair(\d{2})-([ab])-(base|candidate)\.json$/;
const timingEntries = new Map();
for (const filename of fs.readdirSync(path.join(rawDir, "reports", "timing"))) {
  const match = filename.match(timingPattern);
  if (!match) continue;
  const [, scenario, pairText, position, role] = match;
  const key = `${scenario}/${pairText}`;
  const entry = timingEntries.get(key) ?? { scenario, pair: Number(pairText) };
  entry[position] = role;
  entry[role] = readJSON(rawDir, path.join("reports", "timing", filename));
  timingEntries.set(key, entry);
}

const timingRows = [];
const allPairRatios = [];
for (const scenario of timingCases) {
  const pairs = [...timingEntries.values()].filter((entry) => entry.scenario === scenario).sort((left, right) => left.pair - right.pair);
  assertEqual(pairs.length, 7, `${scenario} timing pairs`);
  const baseElapsed = [];
  const candidateElapsed = [];
  const pairRatios = [];
  for (const pair of pairs) {
    assertEqual(pair.a, pair.pair % 2 ? "base" : "candidate", `${scenario}/${pair.pair} A order`);
    assertEqual(pair.b, pair.pair % 2 ? "candidate" : "base", `${scenario}/${pair.pair} B order`);
    validateReport(pair.base, baselineRevision, "general-search-v1", [scenario], [1000000], false, `${scenario}/${pair.pair}/base`);
    validateReport(pair.candidate, candidateRevision, "general-search-v1", [scenario], [1000000], false, `${scenario}/${pair.pair}/candidate`);
    assertDeepEqual(pair.candidate.runs[0], pair.base.runs[0], semanticFields, `${scenario}/${pair.pair} timing semantics`);
    const baseMS = pair.base.runs[0].elapsed_ms;
    const candidateMS = pair.candidate.runs[0].elapsed_ms;
    baseElapsed.push(baseMS);
    candidateElapsed.push(candidateMS);
    pairRatios.push(candidateMS / baseMS);
    allPairRatios.push(candidateMS / baseMS);
  }
  const baseMedian = median(baseElapsed);
  const candidateMedian = median(candidateElapsed);
  const baseQ = quartiles(baseElapsed);
  const candidateQ = quartiles(candidateElapsed);
  timingRows.push({
    scenario,
    base_median_ms: baseMedian,
    base_q1_ms: baseQ.q1,
    base_q3_ms: baseQ.q3,
    base_iqr_ms: baseQ.q3 - baseQ.q1,
    candidate_median_ms: candidateMedian,
    candidate_q1_ms: candidateQ.q1,
    candidate_q3_ms: candidateQ.q3,
    candidate_iqr_ms: candidateQ.q3 - candidateQ.q1,
    median_ratio: candidateMedian / baseMedian,
    median_speedup_percent: (1 - candidateMedian / baseMedian) * 100,
    paired_ratios: pairRatios.map((value) => value.toFixed(9)).join(";"),
    median_paired_ratio: median(pairRatios),
  });
}

validateReport(readJSON(rawDir, "profiles/base-gsv1.json"), baselineRevision, "general-search-v1", timingCases, [1000000], false, "base GSV1 profile");
validateReport(readJSON(rawDir, "profiles/candidate-gsv1.json"), candidateRevision, "general-search-v1", timingCases, [1000000], false, "candidate GSV1 profile");
validateReport(readJSON(rawDir, "profiles/base-v4.json"), baselineRevision, "v4", timingCases, [1000000], false, "base V4 profile");
validateReport(readJSON(rawDir, "profiles/candidate-v4.json"), candidateRevision, "v4", timingCases, [1000000], false, "candidate V4 profile");

fs.mkdirSync(outputDir, { recursive: true });
writeCSV("timing.csv", timingRows);
const aggregateRatio = median(allPairRatios);
const baseMedianSum = timingRows.reduce((sum, row) => sum + row.base_median_ms, 0);
const candidateMedianSum = timingRows.reduce((sum, row) => sum + row.candidate_median_ms, 0);
const weightedRatio = candidateMedianSum / baseMedianSum;
const nonRegressing = timingRows.filter((row) => row.median_ratio <= 1).length;
const worstRatio = Math.max(...timingRows.map((row) => row.median_ratio));

const causal = config.causal;
const baselineRegion = causal.baseline_map_builder_seconds + causal.baseline_target_lookup_seconds;
const candidateRegion = causal.candidate_index_builder_seconds + causal.candidate_target_access_seconds;
const causalReduction = 1 - candidateRegion / baselineRegion;
const allocSpaceChange = causal.candidate_alloc_space_bytes / causal.baseline_alloc_space_bytes - 1;
const allocObjectChange = causal.candidate_alloc_objects / causal.baseline_alloc_objects - 1;

const gates = {
  repository: Object.values(config.repository_gates).every((value) => value === "PASS"),
  smoke_semantics: true,
  semantic_42: semanticComparisons === 42,
  logical_profiles: true,
  wall_2_percent: aggregateRatio <= 0.98,
  breadth_5_of_6: nonRegressing >= 5,
  max_regression_2_percent: worstRatio <= 1.02,
  sum_medians_non_regressing: weightedRatio <= 1,
  causal_61_percent: causalReduction >= 0.61,
  indexed_builder_zero_alloc: causal.candidate_index_builder_allocs_per_op === 0,
  alloc_space_within_1_percent: allocSpaceChange <= 0.01,
  alloc_objects_within_1_percent: allocObjectChange <= 0.01,
  outgoing_map_alloc_substantially_reduced: causal.outgoing_map_alloc_substantially_reduced === true,
};
const keep = Object.values(gates).every(Boolean);
const correctnessAndCausal =
  gates.repository &&
  gates.smoke_semantics &&
  gates.semantic_42 &&
  gates.logical_profiles &&
  gates.causal_61_percent &&
  gates.indexed_builder_zero_alloc &&
  gates.alloc_space_within_1_percent &&
  gates.alloc_objects_within_1_percent &&
  gates.outgoing_map_alloc_substantially_reduced;
let derivedDecision;
if (keep) {
  derivedDecision = "KEEP";
} else if (correctnessAndCausal) {
  derivedDecision = "NEED_MORE_EVIDENCE";
} else {
  derivedDecision = "REVERT";
}
assertEqual(config.decision, derivedDecision, "frozen decision");

const summary = {
  version: "r1ig-analysis-v1",
  baseline_revision: baselineRevision,
  candidate_revision: candidateRevision,
  smoke_semantics: "PASS",
  matrix_semantic_comparisons: semanticComparisons,
  matrix_logical_profiles: "PASS",
  profiled_runs: profiledPresence.filter(Boolean).length,
  zero_work_runs: profiledPresence.filter((value) => !value).length,
  timing: {
    scenarios: timingRows,
    aggregate_median_paired_ratio: aggregateRatio,
    aggregate_paired_speedup_percent: (1 - aggregateRatio) * 100,
    baseline_sum_of_medians_ms: baseMedianSum,
    candidate_sum_of_medians_ms: candidateMedianSum,
    weighted_sum_median_ratio: weightedRatio,
    weighted_improvement_percent: (1 - weightedRatio) * 100,
    non_regressing_scenarios: nonRegressing,
    worst_scenario_ratio: worstRatio,
  },
  causal: {
    ...causal,
    baseline_target_region_seconds: baselineRegion,
    candidate_target_region_seconds: candidateRegion,
    target_region_reduction_fraction: causalReduction,
    alloc_space_change_fraction: allocSpaceChange,
    alloc_objects_change_fraction: allocObjectChange,
  },
  repository_gates: config.repository_gates,
  gates,
  decision: derivedDecision,
};
fs.writeFileSync(path.join(outputDir, "analysis-summary.json"), `${JSON.stringify(summary, null, 2)}\n`);
fs.writeFileSync(
  path.join(outputDir, "accounting-validation.txt"),
  [
    "R1I-G accounting validation",
    `baseline_revision=${baselineRevision}`,
    `candidate_revision=${candidateRevision}`,
    "smoke_semantics=PASS",
    `semantic_comparisons=${semanticComparisons}`,
    "logical_profile_equality=PASS",
    "priority_identities=PASS",
    "outgoing_identities=PASS",
    `profiled_runs=${profiledPresence.filter(Boolean).length}`,
    `zero_work_runs=${profiledPresence.filter((value) => !value).length}`,
    "validation_materialized=false",
    "public_holdout_materialized=false",
    "private_holdout_materialized=false",
    `decision=${derivedDecision}`,
  ].join("\n") + "\n",
);

process.stdout.write(`${JSON.stringify({
  baseline_revision: baselineRevision,
  candidate_revision: candidateRevision,
  semantic_comparisons: semanticComparisons,
  aggregate_paired_speedup_percent: summary.timing.aggregate_paired_speedup_percent,
  weighted_improvement_percent: summary.timing.weighted_improvement_percent,
  causal_reduction_percent: causalReduction * 100,
  alloc_space_change_percent: allocSpaceChange * 100,
  alloc_objects_change_percent: allocObjectChange * 100,
  decision: derivedDecision,
}, null, 2)}\n`);
