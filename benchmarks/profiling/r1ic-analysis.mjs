import fs from "node:fs";
import path from "node:path";

const root = process.argv[2];
const outputPath = process.argv[3];
if (!root) throw new Error("usage: node r1ic-analysis.mjs <artifact-root> [summary-output]");

const baselineRevision = "19159d2e970e3457d730824983fc70fe649f9202";
const candidateRevision = "4ee1cf690a019ee28f5548d360c800ddec4007a3";
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
  return JSON.parse(fs.readFileSync(path.join(root, relativePath), "utf8"));
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

function normalize(value, ignoredFields) {
  if (Array.isArray(value)) return value.map((entry) => normalize(entry, ignoredFields));
  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.keys(value)
        .filter((key) => !ignoredFields.has(key))
        .sort()
        .map((key) => [key, normalize(value[key], ignoredFields)]),
    );
  }
  return value;
}

function assertDeepEqual(actual, expected, message) {
  const actualJSON = JSON.stringify(actual);
  const expectedJSON = JSON.stringify(expected);
  if (actualJSON !== expectedJSON) throw new Error(`${message}: normalized JSON differs`);
}

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
    `${label} source matches`,
  );
  assertEqual(
    site.placed_source_target_iterations,
    site.self_target_skips + site.target_placement_lookups,
    `${label} target scans`,
  );
  assertEqual(site.target_placement_lookups, site.placed_targets_found + site.unplaced_targets, `${label} target lookups`);
  assertEqual(site.source_hits_target_calls, site.placed_targets_found, `${label} hit calls`);
  assertEqual(site.coverage_placement_key_calls, site.placed_source_iterations, `${label} coverage keys`);
  assertEqual(site.placed_potential_lookups, site.placed_source_iterations, `${label} placed lookups`);
  assertEqual(site.free_potential_lookups, site.free_source_iterations, `${label} free lookups`);
  assertEqual(site.popcount_calls, site.placed_source_iterations + site.free_source_iterations, `${label} popcounts`);
  assertEqual(site.placed_map_builds, site.checks, `${label} map builds`);
}

function validateProfiledRun(run, label) {
  const profile = run.search.bound_operation_profile;
  if (!profile) {
    assertEqual(run.search.outgoing_bound_checks ?? 0, 0, `${label} missing-profile outgoing checks`);
    assertEqual(run.search.outgoing_bound_pruned_nodes ?? 0, 0, `${label} missing-profile outgoing prunes`);
    return false;
  }
  assertEqual(profile.version, "bound-attribution-ops-v1", `${label} profile version`);
  for (const site of prioritySites) validatePrioritySite(profile.priority_upper[site], `${label}/${site}`);
  assertEqual(
    profile.priority_upper.constellation_states_input,
    profile.priority_upper.constellation_states_retained + profile.priority_upper.constellation_states_rejected,
    `${label} constellation states`,
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

function validateReportShape(report, revision, variant, budgets, label) {
  assertEqual(report.build_revision, revision, `${label} revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.operation_profiling, true, `${label} operation profiling`);
  assertEqual(report.diagnostic, false, `${label} diagnostics`);
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
  for (const run of report.runs) validateProfiledRun(run, `${label}/${run.scenario}/${run.budget}`);
}

function compareReportRuns(baseline, candidate, label) {
  let comparisons = 0;
  for (const baselineRun of baseline.runs) {
    const candidateRun = candidate.runs.find(
      (run) => run.scenario === baselineRun.scenario && run.budget === baselineRun.budget && run.repeat === baselineRun.repeat,
    );
    assert(candidateRun, `${label} missing candidate run ${baselineRun.scenario}/${baselineRun.budget}`);
    assertDeepEqual(
      normalize(candidateRun, deterministicIgnoredFields),
      normalize(baselineRun, deterministicIgnoredFields),
      `${label} deterministic comparison ${baselineRun.scenario}/${baselineRun.budget}`,
    );
    comparisons++;
  }
  return comparisons;
}

function addPriorityTotals(total, run) {
  const profile = run.search.bound_operation_profile;
  if (!profile) return;
  for (const site of prioritySites) {
    for (const [key, value] of Object.entries(profile.priority_upper[site])) {
      if (typeof value === "number") total[key] = (total[key] ?? 0) + value;
    }
  }
}

function median(values) {
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  return sorted.length % 2 === 0 ? (sorted[middle - 1] + sorted[middle]) / 2 : sorted[middle];
}

function quartiles(values) {
  const sorted = [...values].sort((left, right) => left - right);
  const middle = Math.floor(sorted.length / 2);
  const lower = sorted.slice(0, middle);
  const upper = sorted.slice(sorted.length % 2 === 0 ? middle : middle + 1);
  return { q1: median(lower), q3: median(upper) };
}

const smokeNames = [
  "smoke-base-normal.json",
  "smoke-candidate-normal.json",
  "smoke-candidate-tagged-off.json",
  "smoke-base-profiled.json",
  "smoke-candidate-profiled.json",
];
const smokeReports = Object.fromEntries(smokeNames.map((name) => [name, readJSON(path.join("reports", name))]));
const smokeBaseline = smokeReports["smoke-base-normal.json"];
for (const name of smokeNames.slice(1)) {
  assertDeepEqual(
    normalize(smokeReports[name], semanticIgnoredFields),
    normalize(smokeBaseline, semanticIgnoredFields),
    `smoke semantics baseline vs ${name}`,
  );
}
assertEqual(smokeBaseline.build_revision, baselineRevision, "smoke baseline revision");
assertEqual(smokeReports["smoke-candidate-normal.json"].build_revision, candidateRevision, "smoke candidate revision");
const smokeBaseProfiled = smokeReports["smoke-base-profiled.json"].runs[0];
const smokeCandidateProfiled = smokeReports["smoke-candidate-profiled.json"].runs[0];
validateProfiledRun(smokeBaseProfiled, "smoke/base");
validateProfiledRun(smokeCandidateProfiled, "smoke/candidate");
assertDeepEqual(
  normalize(smokeCandidateProfiled, deterministicIgnoredFields),
  normalize(smokeBaseProfiled, deterministicIgnoredFields),
  "smoke profiled deterministic equality",
);

const summary = {
  version: "r1ic-analysis-v1",
  baseline_revision: baselineRevision,
  candidate_revision: candidateRevision,
  smoke_semantic_comparisons: smokeNames.length - 1,
  smoke_semantics: "PASS",
  smoke_logical_profiles: "PASS",
};

const matrixFiles = [
  "matrix-base-gsv1.json",
  "matrix-candidate-gsv1.json",
  "matrix-base-v4.json",
  "matrix-candidate-v4.json",
];
if (matrixFiles.every((name) => fs.existsSync(path.join(root, "reports", name)))) {
  const baseGSV1 = readJSON(path.join("reports", matrixFiles[0]));
  const candidateGSV1 = readJSON(path.join("reports", matrixFiles[1]));
  const baseV4 = readJSON(path.join("reports", matrixFiles[2]));
  const candidateV4 = readJSON(path.join("reports", matrixFiles[3]));
  validateReportShape(baseGSV1, baselineRevision, "general-search-v1", [250000, 1000000], "base GSV1");
  validateReportShape(candidateGSV1, candidateRevision, "general-search-v1", [250000, 1000000], "candidate GSV1");
  validateReportShape(baseV4, baselineRevision, "v4", [1000000], "base V4");
  validateReportShape(candidateV4, candidateRevision, "v4", [1000000], "candidate V4");
  const comparisons = compareReportRuns(baseGSV1, candidateGSV1, "GSV1") + compareReportRuns(baseV4, candidateV4, "V4");
  const totals = {};
  for (const run of candidateGSV1.runs.filter((run) => run.budget === 1000000)) addPriorityTotals(totals, run);
  const expectedTotals = {
    calls: 234300,
    fixed_source_target_option_checks: 506804547,
    geometry_candidate_checks: 569592092,
    star_position_hit_calls: 567835319,
    matching_calls: 612353,
  };
  for (const [field, expected] of Object.entries(expectedTotals)) assertEqual(totals[field], expected, `GSV1 1M ${field}`);
  summary.matrix_semantic_comparisons = comparisons;
  summary.matrix_semantics = "PASS";
  summary.matrix_logical_profiles = "PASS";
  summary.gsv1_1m_priority_totals = Object.fromEntries(Object.keys(expectedTotals).map((field) => [field, totals[field]]));
}

const timingDir = path.join(root, "reports", "timing");
if (fs.existsSync(timingDir)) {
  const pattern = /^timing-(gsv2-\d{3})-pair(\d{2})-([ab])-(base|candidate)\.json$/;
  const entries = new Map();
  for (const filename of fs.readdirSync(timingDir)) {
    const match = filename.match(pattern);
    if (!match) continue;
    const [, scenario, pairText, position, role] = match;
    const key = `${scenario}/${pairText}`;
    const entry = entries.get(key) ?? { scenario, pair: Number(pairText) };
    entry[position] = role;
    entry[role] = readJSON(path.join("reports", "timing", filename));
    entries.set(key, entry);
  }
  const scenarioSummaries = [];
  const allPairRatios = [];
  for (const scenario of timingCases) {
    const pairs = [...entries.values()].filter((entry) => entry.scenario === scenario).sort((left, right) => left.pair - right.pair);
    assertEqual(pairs.length, 7, `${scenario} timing pair count`);
    const baseElapsed = [];
    const candidateElapsed = [];
    const pairRatios = [];
    for (const pair of pairs) {
      const expectedA = pair.pair % 2 === 1 ? "base" : "candidate";
      const expectedB = pair.pair % 2 === 1 ? "candidate" : "base";
      assertEqual(pair.a, expectedA, `${scenario} pair ${pair.pair} position a`);
      assertEqual(pair.b, expectedB, `${scenario} pair ${pair.pair} position b`);
      assertEqual(pair.base.build_revision, baselineRevision, `${scenario} pair ${pair.pair} base revision`);
      assertEqual(pair.candidate.build_revision, candidateRevision, `${scenario} pair ${pair.pair} candidate revision`);
      assertEqual(pair.base.runs.length, 1, `${scenario} pair ${pair.pair} base run count`);
      assertEqual(pair.candidate.runs.length, 1, `${scenario} pair ${pair.pair} candidate run count`);
      assertDeepEqual(
        normalize(pair.candidate.runs[0], semanticIgnoredFields),
        normalize(pair.base.runs[0], semanticIgnoredFields),
        `${scenario} pair ${pair.pair} timing semantics`,
      );
      const baseMS = pair.base.runs[0].elapsed_ms;
      const candidateMS = pair.candidate.runs[0].elapsed_ms;
      baseElapsed.push(baseMS);
      candidateElapsed.push(candidateMS);
      pairRatios.push(candidateMS / baseMS);
      allPairRatios.push(candidateMS / baseMS);
    }
    const baseMedian = median(baseElapsed);
    const candidateMedian = median(candidateElapsed);
    const baseQuartiles = quartiles(baseElapsed);
    const candidateQuartiles = quartiles(candidateElapsed);
    scenarioSummaries.push({
      scenario,
      base_median_ms: baseMedian,
      base_q1_ms: baseQuartiles.q1,
      base_q3_ms: baseQuartiles.q3,
      base_iqr_ms: baseQuartiles.q3 - baseQuartiles.q1,
      candidate_median_ms: candidateMedian,
      candidate_q1_ms: candidateQuartiles.q1,
      candidate_q3_ms: candidateQuartiles.q3,
      candidate_iqr_ms: candidateQuartiles.q3 - candidateQuartiles.q1,
      median_ratio: candidateMedian / baseMedian,
      median_speedup_percent: (1 - candidateMedian / baseMedian) * 100,
      median_pair_ratio: median(pairRatios),
    });
  }
  const aggregateRatio = median(allPairRatios);
  const nonRegressing = scenarioSummaries.filter((row) => row.median_ratio <= 1).length;
  const worstRatio = Math.max(...scenarioSummaries.map((row) => row.median_ratio));
  summary.timing = {
    scenario_summaries: scenarioSummaries,
    aggregate_median_pair_ratio: aggregateRatio,
    aggregate_paired_speedup_percent: (1 - aggregateRatio) * 100,
    non_regressing_scenarios: nonRegressing,
    worst_scenario_ratio: worstRatio,
    wall_gate_2_percent: aggregateRatio <= 0.98 ? "PASS" : "FAIL",
    breadth_gate_5_of_6: nonRegressing >= 5 ? "PASS" : "FAIL",
    max_regression_gate_2_percent: worstRatio <= 1.02 ? "PASS" : "FAIL",
  };
}

const serialized = `${JSON.stringify(summary, null, 2)}\n`;
if (outputPath) fs.writeFileSync(outputPath, serialized);
process.stdout.write(serialized);
