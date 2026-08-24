import fs from "node:fs";
import path from "node:path";

const artifactDir = process.argv[2];
const outputDir = process.argv[3] ?? (artifactDir ? path.join(artifactDir, "review") : undefined);
if (!artifactDir) throw new Error("usage: node r1ie-analysis.mjs <artifact-dir> [output-dir]");

const baselineRevision = "91af7e6e6f6469e13ae792c7d199ebad92883ea1";
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

function readJSON(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(artifactDir, relativePath), "utf8"));
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
const deterministicIgnoredFields = new Set([
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

function validateMatrix(report, revision, variant, budgets, label) {
  assertEqual(report.build_revision, revision, `${label} revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.operation_profiling, true, `${label} operation profile`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
  assertEqual(report.constellation_seed_variant, variant, `${label} variant`);
  assertEqual(report.runs.length, expectedCases.length * budgets.length, `${label} run count`);
  for (const scenario of expectedCases) {
    for (const budget of budgets) {
      assertEqual(report.runs.filter((run) => run.scenario === scenario && run.budget === budget).length, 1, `${label} ${scenario}/${budget}`);
    }
  }
  return report.runs.map((run) => validateProfiledRun(run, `${label}/${run.scenario}/${run.budget}`));
}

function compareRuns(baseline, candidate, label) {
  let comparisons = 0;
  for (const baseRun of baseline.runs) {
    const candidateRun = candidate.runs.find(
      (run) => run.scenario === baseRun.scenario && run.budget === baseRun.budget && run.repeat === baseRun.repeat,
    );
    assert(candidateRun, `${label} missing candidate ${baseRun.scenario}/${baseRun.budget}`);
    assertDeepEqual(candidateRun, baseRun, deterministicIgnoredFields, `${label} deterministic ${baseRun.scenario}/${baseRun.budget}`);
    comparisons++;
  }
  return comparisons;
}

function validateNormalProfile(report, revision, label) {
  assertEqual(report.build_revision, revision, `${label} revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.operation_profiling ?? false, false, `${label} operation profile`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
  assertEqual(report.constellation_seed_variant, "general-search-v1", `${label} variant`);
  assertEqual(report.runs.length, timingCases.length, `${label} run count`);
  for (const scenario of timingCases) {
    assertEqual(report.runs.filter((run) => run.scenario === scenario && run.budget === 1000000).length, 1, `${label}/${scenario}`);
  }
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

const config = readJSON("review-input/r1ie-config.json");
assertEqual(config.version, "r1ie-review-input-v1", "config version");
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
const smoke = Object.fromEntries(smokeNames.map((name) => [name, readJSON(path.join("reports", name))]));
for (const name of smokeNames.slice(1)) {
  assertDeepEqual(smoke[name], smoke[smokeNames[0]], semanticIgnoredFields, `smoke semantics ${name}`);
}
assertEqual(smoke[smokeNames[0]].build_revision, baselineRevision, "smoke baseline revision");
assertEqual(smoke[smokeNames[1]].build_revision, candidateRevision, "smoke candidate revision");
assertEqual(smoke[smokeNames[2]].build_revision, candidateRevision, "smoke tagged-off revision");
validateProfiledRun(smoke[smokeNames[3]].runs[0], "smoke/base-profiled");
validateProfiledRun(smoke[smokeNames[4]].runs[0], "smoke/candidate-profiled");
assertDeepEqual(
  smoke[smokeNames[4]].runs[0],
  smoke[smokeNames[3]].runs[0],
  deterministicIgnoredFields,
  "smoke profiled deterministic equality",
);

const baseGSV1 = readJSON("reports/matrix-base-gsv1.json");
const candidateGSV1 = readJSON("reports/matrix-candidate-gsv1.json");
const baseV4 = readJSON("reports/matrix-base-v4.json");
const candidateV4 = readJSON("reports/matrix-candidate-v4.json");
const presence = [
  ...validateMatrix(baseGSV1, baselineRevision, "general-search-v1", [250000, 1000000], "base GSV1"),
  ...validateMatrix(candidateGSV1, candidateRevision, "general-search-v1", [250000, 1000000], "candidate GSV1"),
  ...validateMatrix(baseV4, baselineRevision, "v4", [1000000], "base V4"),
  ...validateMatrix(candidateV4, candidateRevision, "v4", [1000000], "candidate V4"),
];
const semanticComparisons = compareRuns(baseGSV1, candidateGSV1, "GSV1") + compareRuns(baseV4, candidateV4, "V4");
assertEqual(semanticComparisons, 42, "semantic comparison count");

const timingDir = path.join(artifactDir, "reports", "timing");
const timingPattern = /^timing-(gsv2-\d{3})-pair(\d{2})-([ab])-(base|candidate)\.json$/;
const timingEntries = new Map();
for (const filename of fs.readdirSync(timingDir)) {
  const match = filename.match(timingPattern);
  if (!match) continue;
  const [, scenario, pairText, position, role] = match;
  const key = `${scenario}/${pairText}`;
  const entry = timingEntries.get(key) ?? { scenario, pair: Number(pairText) };
  entry[position] = role;
  entry[role] = readJSON(path.join("reports", "timing", filename));
  timingEntries.set(key, entry);
}

const timingRows = [];
const allPairRatios = [];
for (const scenario of timingCases) {
  const pairs = [...timingEntries.values()].filter((entry) => entry.scenario === scenario).sort((a, b) => a.pair - b.pair);
  assertEqual(pairs.length, 7, `${scenario} timing pairs`);
  const baseElapsed = [];
  const candidateElapsed = [];
  const pairRatios = [];
  for (const pair of pairs) {
    assertEqual(pair.a, pair.pair % 2 ? "base" : "candidate", `${scenario}/${pair.pair} A order`);
    assertEqual(pair.b, pair.pair % 2 ? "candidate" : "base", `${scenario}/${pair.pair} B order`);
    assertEqual(pair.base.build_revision, baselineRevision, `${scenario}/${pair.pair} base revision`);
    assertEqual(pair.candidate.build_revision, candidateRevision, `${scenario}/${pair.pair} candidate revision`);
    assertDeepEqual(pair.candidate.runs[0], pair.base.runs[0], semanticIgnoredFields, `${scenario}/${pair.pair} timing semantics`);
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

validateNormalProfile(readJSON("profiles/base-profile.json"), baselineRevision, "base profile");
validateNormalProfile(readJSON("profiles/candidate-profile.json"), candidateRevision, "candidate profile");

fs.mkdirSync(outputDir, { recursive: true });
writeCSV("timing.csv", timingRows);
const aggregateRatio = median(allPairRatios);
const nonRegressing = timingRows.filter((row) => row.median_ratio <= 1).length;
const worstRatio = Math.max(...timingRows.map((row) => row.median_ratio));
const causalReduction = 1 - config.causal.candidate_formatter_edge_seconds / config.causal.baseline_formatter_edge_seconds;
const allocSpaceRegression = config.causal.candidate_alloc_space_bytes / config.causal.baseline_alloc_space_bytes - 1;
const gates = {
  repository: Object.values(config.repository_gates).every((value) => value === "PASS"),
  smoke_semantics: true,
  semantic_42: semanticComparisons === 42,
  wall_2_percent: aggregateRatio <= 0.98,
  breadth_5_of_6: nonRegressing >= 5,
  max_regression_2_percent: worstRatio <= 1.02,
  causal_50_percent: causalReduction >= 0.5,
  alloc_space_no_material_regression: allocSpaceRegression <= config.causal.max_alloc_space_regression,
  hot_path_allocs_no_increase: config.causal.candidate_hot_path_allocs_per_op <= config.causal.baseline_hot_path_allocs_per_op,
};
const allKeepGates = Object.values(gates).every(Boolean);
let derivedDecision;
if (allKeepGates) {
  derivedDecision = "KEEP";
} else if (
  gates.repository &&
  gates.smoke_semantics &&
  gates.semantic_42 &&
  aggregateRatio <= 1 &&
  !gates.wall_2_percent &&
  gates.causal_50_percent &&
  gates.alloc_space_no_material_regression &&
  gates.hot_path_allocs_no_increase
) {
  derivedDecision = "NEED_MORE_EVIDENCE";
} else {
  derivedDecision = "REVERT";
}
assertEqual(config.decision, derivedDecision, "frozen decision");

const summary = {
  version: "r1ie-analysis-v1",
  baseline_revision: baselineRevision,
  candidate_revision: candidateRevision,
  smoke_semantics: "PASS",
  matrix_semantic_comparisons: semanticComparisons,
  matrix_logical_profiles: "PASS",
  profiled_runs: presence.filter(Boolean).length,
  zero_work_runs: presence.filter((value) => !value).length,
  timing: {
    scenarios: timingRows,
    aggregate_median_paired_ratio: aggregateRatio,
    aggregate_paired_speedup_percent: (1 - aggregateRatio) * 100,
    non_regressing_scenarios: nonRegressing,
    worst_scenario_ratio: worstRatio,
  },
  causal: {
    ...config.causal,
    formatter_edge_reduction_fraction: causalReduction,
    alloc_space_regression_fraction: allocSpaceRegression,
  },
  repository_gates: config.repository_gates,
  gates,
  decision: derivedDecision,
};
fs.writeFileSync(path.join(outputDir, "analysis-summary.json"), `${JSON.stringify(summary, null, 2)}\n`);
fs.writeFileSync(
  path.join(outputDir, "accounting-validation.txt"),
  [
    "R1I-E accounting validation",
    `baseline_revision=${baselineRevision}`,
    `candidate_revision=${candidateRevision}`,
    "smoke_semantics=PASS",
    `semantic_comparisons=${semanticComparisons}`,
    "logical_profile_equality=PASS",
    "priority_identities=PASS",
    "outgoing_identities=PASS",
    `profiled_runs=${presence.filter(Boolean).length}`,
    `zero_work_runs=${presence.filter((value) => !value).length}`,
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
  causal_reduction_percent: causalReduction * 100,
  decision: derivedDecision,
}, null, 2)}\n`);
