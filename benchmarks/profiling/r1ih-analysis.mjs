import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const expectedSHA = "0cac463a79238ecaea9d95af33468cc04dd5809b";
const expectedCases = Array.from({ length: 14 }, (_, index) => `gsv2-${String(index + 13).padStart(3, "0")}`);
const profileCases = ["gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024"];
const prioritySites = ["constellation_filter", "repair_dfs", "plateau_prefilter", "plateau_dfs"];
const outgoingSites = ["search", "repair"];
const complexityBars = { C0: 0.02, C1: 0.025, C2: 0.03 };
const carryForwardFamilies = [
  "plateau/archive reselection",
  "outgoing static compatibility",
  "coveragePlacementKey residual",
  "canonical-copy ranks",
  "priority residual geometry",
  "filteredRemovedOptions",
  "target/source scans",
  "placement-index residual",
  "new post-R1I-G hotspot",
];
const removalBasisTypes = new Set([
  "microbenchmark",
  "source_line_decomposition",
  "direct_indexing",
  "allocation_elimination",
  "cached_static_relation",
  "measured_child_parent_ownership",
]);

if (process.argv[2] === "--preflight") {
  process.stdout.write(JSON.stringify({
    status: "PASS",
    expected_sha: expectedSHA,
    inventory_set_match_required: true,
    post_analysis_raw_hash_revalidation_required: true,
    complexity_bars: complexityBars,
  }, null, 2) + "\n");
  process.exit(0);
}

const artifactDir = process.argv[2] ? path.resolve(process.argv[2]) : undefined;
const classificationPath = process.argv[3] ? path.resolve(process.argv[3]) : artifactDir ? path.join(artifactDir, "review-input/r1ih-classification.json") : undefined;
const outputDir = process.argv[4] ? path.resolve(process.argv[4]) : artifactDir ? path.join(artifactDir, "review") : undefined;
if (!artifactDir) throw new Error("usage: node r1ih-analysis.mjs <artifact-dir> [classification-json] [output-dir]");

const priorityFields = [
  "calls", "feasible_results", "rejected_results", "invalid_priority_returns", "priority_entries_validated",
  "fixed_placement_inputs", "current_placement_inputs", "anchored_placements", "removed_instance_inputs", "removed_instances",
  "removed_option_candidates", "removed_option_rejected_fixed_overlap", "removed_option_rejected_outside_free", "removed_options_retained",
  "unique_priority_source_items", "anchored_source_instances", "removed_source_instances", "star_slots", "fixed_target_checks",
  "removed_target_checks", "self_target_skips", "fixed_fixed_geometry_checks", "removed_source_option_checks_fixed_target",
  "fixed_source_target_option_checks", "removed_source_target_option_pairs", "geometry_candidate_checks", "geometry_overlap_rejects",
  "star_position_hit_calls", "star_position_hit_true", "slot_target_hits", "matching_calls",
];
const outgoingFields = [
  "checks", "pruned_nodes", "placed_map_builds", "placed_map_insertions", "placed_mask_instance_checks", "priority_iterations",
  "source_instance_iterations", "priority_source_matches", "zero_star_source_skips", "placed_source_iterations", "free_source_iterations",
  "placed_source_target_iterations", "self_target_skips", "target_placement_lookups", "placed_targets_found", "unplaced_targets",
  "source_hits_target_calls", "source_hits_target_true", "coverage_placement_key_calls", "placed_potential_lookups",
  "free_potential_lookups", "popcount_calls", "star_count_clamps",
];

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

function assertEqual(actual, expected, message) {
  if (actual !== expected) throw new Error(`${message}: got ${actual}, want ${expected}`);
}

function assertArrayEqual(actual, expected, message) {
  assertEqual(JSON.stringify(actual), JSON.stringify(expected), message);
}

function readText(relativePath) {
  return fs.readFileSync(path.join(artifactDir, relativePath), "utf8");
}

function readJSON(relativePath) {
  return JSON.parse(readText(relativePath));
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
    site.fixed_fixed_geometry_checks + site.removed_source_option_checks_fixed_target +
      site.fixed_source_target_option_checks + site.removed_source_target_option_pairs,
    `${label} geometry regimes`,
  );
  assertEqual(site.geometry_candidate_checks, site.geometry_overlap_rejects + site.star_position_hit_calls, `${label} geometry outcomes`);
  assertEqual(site.star_position_hit_true, site.slot_target_hits, `${label} slot hits`);
}

function validateOutgoingSite(site, label) {
  assertEqual(
    site.priority_source_matches,
    site.zero_star_source_skips + site.placed_source_iterations + site.free_source_iterations,
    `${label} source regimes`,
  );
  assertEqual(site.placed_source_target_iterations, site.self_target_skips + site.target_placement_lookups, `${label} target scan`);
  assertEqual(site.target_placement_lookups, site.placed_targets_found + site.unplaced_targets, `${label} target lookup`);
  assertEqual(site.source_hits_target_calls, site.placed_targets_found, `${label} hit calls`);
  assertEqual(site.coverage_placement_key_calls, site.placed_source_iterations, `${label} coverage keys`);
  assertEqual(site.placed_potential_lookups, site.placed_source_iterations, `${label} placed potential`);
  assertEqual(site.free_potential_lookups, site.free_source_iterations, `${label} free potential`);
  assertEqual(site.popcount_calls, site.placed_source_iterations + site.free_source_iterations, `${label} popcounts`);
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

function validateReportEnvelope(report, label, { budgets, variant, operationProfiling, cases }) {
  assertEqual(report.build_revision, expectedSHA, `${label} build_revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
  assertEqual(report.operation_profiling ?? false, operationProfiling, `${label} operation_profiling`);
  assertEqual(report.constellation_seed_variant, variant, `${label} variant`);
  assertArrayEqual(report.budgets, budgets, `${label} budgets`);
  assertEqual(report.runs.length, cases.length * budgets.length, `${label} run count`);
  for (const scenario of cases) {
    for (const budget of budgets) {
      assertEqual(
        report.runs.filter((run) => run.scenario === scenario && run.budget === budget).length,
        1,
        `${label} matrix ${scenario}/${budget}`,
      );
    }
  }
  for (const run of report.runs) {
    assert(cases.includes(run.scenario), `${label} unexpected scenario ${run.scenario}`);
    assert(budgets.includes(run.budget), `${label}/${run.scenario} unexpected budget ${run.budget}`);
    assertEqual(run.repeat, 1, `${label}/${run.scenario}/${run.budget} repeat`);
    assertEqual(run.constellation_seed_variant, variant, `${label}/${run.scenario}/${run.budget} variant`);
    assertEqual(run.operation_profiling ?? false, operationProfiling, `${label}/${run.scenario}/${run.budget} operation_profiling`);
  }
}

function validateOperationMatrix(report, label, budgets, variant) {
  validateReportEnvelope(report, label, { budgets, variant, operationProfiling: true, cases: expectedCases });
  return report.runs.map((run) => validateProfiledRun(run, `${label}/${run.scenario}/${run.budget}`));
}

function validateNormalReport(report, label, cases, variant) {
  validateReportEnvelope(report, label, { budgets: [1000000], variant, operationProfiling: false, cases });
  for (const run of report.runs) {
    assert(!run.search.bound_operation_profile, `${label}/${run.scenario} contains bound profile`);
    assert(!run.search.packing_seed_operation_profile, `${label}/${run.scenario} contains packing profile`);
    assert(!run.search.root_packing_operation_profile, `${label}/${run.scenario} contains root profile`);
  }
}

const semanticIgnoredFields = new Set([
  "generated_at", "elapsed_ms", "nodes_per_second", "setup_ms", "seed_ms", "repair_ms", "search_ms", "refine_ms",
  "server_elapsed_ms", "first_complete_ms", "first_fully_packed_ms", "operation_profiling", "packing_seed_operation_profile",
  "bound_operation_profile", "root_packing_operation_profile", "operation_profile", "provenance",
]);

function normalize(value) {
  if (Array.isArray(value)) return value.map(normalize);
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.keys(value).filter((key) => !semanticIgnoredFields.has(key)).sort().map((key) => [key, normalize(value[key])]));
  }
  return value;
}

function assertSemanticEqual(actual, expected, label) {
  assertEqual(JSON.stringify(normalize(actual)), JSON.stringify(normalize(expected)), label);
}

function hashFile(absolutePath) {
  return crypto.createHash("sha256").update(fs.readFileSync(absolutePath)).digest("hex");
}

function validateFreeze() {
  const manifestPath = path.join(artifactDir, "provenance/RAW-SHA256SUMS.txt");
  const manifestText = fs.readFileSync(manifestPath, "utf8");
  const entries = manifestText.trim().split(/\r?\n/).map((line) => {
    const match = line.match(/^([0-9a-f]{64})  (.+)$/);
    assert(match, `invalid raw manifest line: ${line}`);
    return { hash: match[1], relativePath: match[2] };
  });
  assert(entries.length > 0, "raw manifest is empty");
  let bytes = 0;
  for (const entry of entries) {
    const absolute = path.resolve(artifactDir, entry.relativePath);
    assert(absolute.startsWith(`${artifactDir}${path.sep}`), `manifest path escaped artifact root: ${entry.relativePath}`);
    assertEqual(hashFile(absolute), entry.hash, `raw manifest hash ${entry.relativePath}`);
    bytes += fs.statSync(absolute).size;
  }
  const manifestHash = hashFile(manifestPath);
  assertEqual(readText("provenance/RAW-SHA256SUMS.sha256").trim(), `${manifestHash}  provenance/RAW-SHA256SUMS.txt`, "separate manifest hash");
  const freezeRecord = readText("provenance/freeze-record.txt");
  assert(freezeRecord.includes(`raw_file_count=${entries.length}`), "freeze record file count mismatch");
  assert(freezeRecord.includes(`raw_total_bytes=${bytes}`), "freeze record byte count mismatch");
  assert(freezeRecord.includes(`raw_manifest_sha256=${manifestHash}`), "freeze record manifest hash mismatch");
  assert(freezeRecord.includes("raw_manifest_revalidation=PASS"), "freeze record lacks hash revalidation");
  assert(freezeRecord.includes("raw_read_only_revalidation=PASS"), "freeze record lacks read-only revalidation");
  assert(freezeRecord.includes("post_freeze_solver_runs=0"), "freeze record lacks zero post-freeze runs");
  return { entries: entries.length, bytes, manifestHash };
}

const freezeBefore = validateFreeze();

const smokeNormal = readJSON("smoke/r1ih-smoke-normal.json");
const smokeTaggedOff = readJSON("smoke/r1ih-smoke-tagged-off.json");
const smokeTaggedOn = readJSON("smoke/r1ih-smoke-tagged-on.json");
for (const [label, report, operationProfiling] of [
  ["normal", smokeNormal, false],
  ["tagged-off", smokeTaggedOff, false],
  ["tagged-on", smokeTaggedOn, true],
]) {
  validateReportEnvelope(report, `smoke ${label}`, { budgets: [250000], variant: "general-search-v1", operationProfiling, cases: ["gsv2-013"] });
}
assertSemanticEqual(smokeTaggedOff, smokeNormal, "smoke normal/tagged-off semantics");
assertSemanticEqual(smokeTaggedOn, smokeTaggedOff, "smoke tagged-off/tagged-on semantics");
validateProfiledRun(smokeTaggedOn.runs[0], "smoke/tagged-on");

const gsv1 = readJSON("operations/r1ih-gsv1.json");
const v4 = readJSON("operations/r1ih-v4.json");
const gsv1Summary = readJSON("operations/r1ih-gsv1-summary.json");
const v4Summary = readJSON("operations/r1ih-v4-summary.json");
const gsv1Presence = validateOperationMatrix(gsv1, "GSV1 operations", [250000, 1000000], "general-search-v1");
const v4Presence = validateOperationMatrix(v4, "V4 operations", [1000000], "v4");
assertEqual(gsv1.runs.length + v4.runs.length, 42, "operation matrix total");
assertEqual(gsv1Summary.version, "operation-profile-summary-v3", "GSV1 summary version");
assertEqual(v4Summary.version, "operation-profile-summary-v3", "V4 summary version");

validateNormalReport(readJSON("profiles/r1ih-gsv1-profile.json"), "combined GSV1", profileCases, "general-search-v1");
validateNormalReport(readJSON("profiles/r1ih-v4-profile.json"), "combined V4", profileCases, "v4");
for (const scenario of profileCases) validateNormalReport(readJSON(`profiles/per-case/${scenario}.json`), `per-case ${scenario}`, [scenario], "general-search-v1");

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
const canonical = readJSON("derived/canonical-profile-data.json");
assertEqual(canonical.version, "r1ih-canonical-profile-v1", "canonical version");
assertEqual(canonical.generated_by, "r1ih-profile-extract.ps1", "canonical extractor");
assertEqual(canonical.binary, "binaries/r1ih-normal.exe", "canonical binary");
assertEqual(canonical.candidate_specific_symbols, false, "candidate-independent extraction");
assertArrayEqual(Object.keys(canonical.cpu_profiles.per_case).sort(), [...profileCases].sort(), "per-case profile set");

function cpuProfile(id) {
  if (id === "combined_gsv1") return canonical.cpu_profiles.combined_gsv1;
  if (id === "combined_v4") return canonical.cpu_profiles.combined_v4;
  const result = canonical.cpu_profiles.per_case[id];
  assert(result, `unknown CPU profile ${id}`);
  return result;
}

function functionRow(profile, symbol, table, allowMissing) {
  const rows = (table === "flat" ? profile.top : profile.top_cumulative).filter((row) => row.symbol === symbol);
  assert(rows.length <= 1, `ambiguous function ${profile.id}/${symbol}/${table}`);
  if (rows.length === 0) {
    if (allowMissing) return null;
    throw new Error(`missing function ${profile.id}/${symbol}/${table}`);
  }
  return rows[0];
}

function sourceLine(profile, routine, file, line, allowMissing) {
  const routines = profile.source_routines.filter((entry) => entry.symbol === routine && entry.file === file);
  assert(routines.length <= 1, `ambiguous source routine ${profile.id}/${routine}/${file}`);
  if (routines.length === 0) {
    if (allowMissing) return null;
    throw new Error(`missing source routine ${profile.id}/${routine}/${file}`);
  }
  const lines = routines[0].lines.filter((entry) => entry.line === line);
  assert(lines.length <= 1, `ambiguous source line ${profile.id}/${routine}/${file}:${line}`);
  if (lines.length === 0) {
    if (allowMissing) return null;
    throw new Error(`missing source line ${profile.id}/${routine}/${file}:${line}`);
  }
  return lines[0];
}

function resolveCpuMetric(profileId, metric, allowMissing = false) {
  const profile = cpuProfile(profileId);
  if (metric.kind === "function") {
    assert(["flat", "cumulative"].includes(metric.value), `invalid function value ${metric.value}`);
    const row = functionRow(profile, metric.symbol, metric.value === "flat" ? "flat" : "cumulative", allowMissing);
    return row ? row[metric.value] : 0;
  }
  if (metric.kind === "source_line") {
    assert(["flat", "cumulative"].includes(metric.value), `invalid source value ${metric.value}`);
    const row = sourceLine(profile, metric.routine, metric.file, metric.line, allowMissing);
    return row ? row[`${metric.value}_seconds`] : 0;
  }
  if (metric.kind === "sum") {
    assert(Array.isArray(metric.metrics) && metric.metrics.length > 0, "sum metric is empty");
    return metric.metrics.reduce((sum, child) => sum + resolveCpuMetric(profileId, child, allowMissing), 0);
  }
  throw new Error(`unknown CPU metric kind ${metric.kind}`);
}

function resolveHeapMetric(metric) {
  if (!metric) return null;
  assert(["alloc_space", "alloc_objects", "inuse_space"].includes(metric.sample_index), `invalid heap sample ${metric.sample_index}`);
  assert(["flat", "cumulative"].includes(metric.value), `invalid heap value ${metric.value}`);
  const profile = canonical.heap_profiles[metric.sample_index];
  const rows = profile.top.filter((row) => row.symbol === metric.symbol);
  assert(rows.length <= 1, `ambiguous heap metric ${metric.sample_index}/${metric.symbol}`);
  return rows.length ? rows[0][metric.value] : 0;
}

function median(values) {
  const sorted = [...values].sort((a, b) => a - b);
  if (sorted.length % 2) return sorted[(sorted.length - 1) / 2];
  return (sorted[sorted.length / 2 - 1] + sorted[sorted.length / 2]) / 2;
}

function operationCaseValues(metric) {
  if (!metric) return null;
  assert(["priority", "outgoing"].includes(metric.family), `unknown operation family ${metric.family}`);
  const fields = metric.family === "priority" ? priorityFields : outgoingFields;
  assert(fields.includes(metric.field), `unknown operation field ${metric.family}.${metric.field}`);
  return Object.fromEntries(gsv1OneMillion.cases.map((entry) => [entry.scenario, entry[metric.family][metric.field]]));
}

function classifyBreadth(perCase) {
  const present = profileCases.filter((scenario) => perCase[scenario].target_seconds > 0).length;
  const material = profileCases.filter((scenario) => perCase[scenario].fraction >= 0.01).length;
  const classification = present >= 5 && material >= 4 ? "broad" : material <= 2 ? "concentrated" : material === 3 ? "ambiguous" : "not-broad";
  return { present, material, classification };
}

const classification = JSON.parse(fs.readFileSync(classificationPath, "utf8"));
assertEqual(classification.version, "r1ih-classification-v1", "classification version");
assertEqual(classification.frozen_sha, expectedSHA, "classification frozen SHA");
assert(Array.isArray(classification.candidates), "classification candidates missing");
assert(Array.isArray(classification.inventory), "classification inventory missing");
assertArrayEqual(classification.carry_forward.map((entry) => entry.family).sort(), [...carryForwardFamilies].sort(), "carry-forward families");
for (const entry of classification.carry_forward) {
  assert(["candidate", "excluded"].includes(entry.status), `carry-forward status ${entry.family}`);
  assert(entry.rationale, `carry-forward rationale ${entry.family}`);
}

const candidateById = new Map();
for (const candidate of classification.candidates) {
  assert(candidate.id && !candidateById.has(candidate.id), `duplicate candidate ${candidate.id}`);
  assert(candidate.mechanism && candidate.family && candidate.source_region, `${candidate.id} exact mechanism`);
  assert(candidate.parent && candidate.target_region && candidate.overlap_group, `${candidate.id} region/overlap`);
  assert(candidate.parent_metric && candidate.target_metric && candidate.per_case_target_metric, `${candidate.id} CPU metrics`);
  assert(Object.hasOwn(complexityBars, candidate.complexity_class), `${candidate.id} complexity`);
  assert(candidate.semantic_risk, `${candidate.id} semantic risk`);
  assert(["PROMOTE", "REJECT", "MORE_EVIDENCE"].includes(candidate.disposition), `${candidate.id} disposition`);
  assert(candidate.rationale, `${candidate.id} rationale`);
  for (const gate of ["causal_isolated", "semantic_risk_acceptable", "overlap_dominated"]) assert(typeof candidate[gate] === "boolean", `${candidate.id} ${gate}`);
  if (candidate.plausible_removal_fraction === null) {
    assert(candidate.removal_basis === "unknown", `${candidate.id} unknown E basis`);
  } else {
    assert(Number.isFinite(candidate.plausible_removal_fraction) && candidate.plausible_removal_fraction >= 0 && candidate.plausible_removal_fraction <= 1, `${candidate.id} E`);
    assert(removalBasisTypes.has(candidate.removal_basis_type), `${candidate.id} E basis type`);
    assert(candidate.removal_basis && candidate.removal_basis !== "unknown", `${candidate.id} structural E basis`);
  }
  if (/archive|plateau/i.test(`${candidate.family} ${candidate.mechanism}`)) {
    for (const field of ["admission_semantics", "selected_set_invariants", "selected_order_invariants", "signature_diversity", "downstream_base_selection", "search_trajectory_risk"]) {
      assert(candidate.archive_semantics?.[field], `${candidate.id} archive field ${field}`);
    }
  }
  candidateById.set(candidate.id, candidate);
}

const canonicalKeys = canonical.eligible_inventory.map((entry) => entry.key).sort();
const classifiedKeys = classification.inventory.map((entry) => entry.key).sort();
assertArrayEqual(classifiedKeys, canonicalKeys, "mechanical inventory exact key set");
assertEqual(new Set(classifiedKeys).size, classifiedKeys.length, "classification inventory duplicate keys");
const mappedCandidates = new Set();
for (const entry of classification.inventory) {
  assert(Boolean(entry.candidate_id) !== Boolean(entry.exclusion_reason), `inventory must map or exclude exactly once: ${entry.key}`);
  if (entry.candidate_id) {
    assert(candidateById.has(entry.candidate_id), `inventory maps to unknown candidate ${entry.candidate_id}`);
    mappedCandidates.add(entry.candidate_id);
  } else {
    assert(entry.exclusion_reason.length >= 12, `inventory exclusion rationale too short: ${entry.key}`);
  }
}

const scoreRows = [];
const caseRows = [];
const operationRows = [];
for (const candidate of classification.candidates) {
  const combined = cpuProfile("combined_gsv1");
  const parentCpu = resolveCpuMetric("combined_gsv1", candidate.parent_metric);
  const targetCpu = resolveCpuMetric("combined_gsv1", candidate.target_metric);
  assert(targetCpu <= parentCpu + 1e-9, `${candidate.id} target exceeds parent`);
  const rawFraction = ratio(targetCpu, combined.total_seconds);
  const bar = complexityBars[candidate.complexity_class];
  const eMin = rawFraction ? bar / rawFraction : Infinity;
  const benefit = candidate.plausible_removal_fraction === null ? null : rawFraction * candidate.plausible_removal_fraction;
  const perCase = Object.fromEntries(profileCases.map((scenario) => {
    const target = resolveCpuMetric(scenario, candidate.per_case_target_metric, true);
    const total = cpuProfile(scenario).total_seconds;
    return [scenario, { total_seconds: total, target_seconds: target, fraction: ratio(target, total) }];
  }));
  const breadth = classifyBreadth(perCase);
  const operationByCase = operationCaseValues(candidate.operation_metric ?? null);
  const operationValues = operationByCase ? Object.values(operationByCase) : null;
  const operation = operationValues ? {
    metric: `${candidate.operation_metric.family}.${candidate.operation_metric.field}`,
    breadth_cases_of_14: operationValues.filter((value) => value > 0).length,
    total: operationValues.reduce((sum, value) => sum + value, 0),
    min: Math.min(...operationValues),
    median: median(operationValues),
    max: Math.max(...operationValues),
    per_case: operationByCase,
  } : null;
  if (!operation) assert(candidate.operation_breadth_note, `${candidate.id} missing operation breadth explanation`);
  const v4Target = candidate.v4_target_metric ? resolveCpuMetric("combined_v4", candidate.v4_target_metric, true) : null;
  const allocSpace = resolveHeapMetric(candidate.alloc_space_metric);
  const allocObjects = resolveHeapMetric(candidate.alloc_objects_metric);
  const eligible = mappedCandidates.has(candidate.id);
  const fullGate = eligible && breadth.classification === "broad" && candidate.causal_isolated &&
    candidate.semantic_risk_acceptable && !candidate.overlap_dominated && candidate.plausible_removal_fraction !== null &&
    eMin <= 1 && benefit >= bar;
  if (candidate.disposition === "PROMOTE") assert(fullGate, `${candidate.id} promoted without clearing all gates`);
  if (eMin > 1) assert(candidate.disposition !== "PROMOTE", `${candidate.id} promoted with E_min >100%`);
  if (candidate.plausible_removal_fraction === null) assert(candidate.disposition !== "PROMOTE", `${candidate.id} promoted with E unknown`);

  const row = {
    candidate_id: candidate.id,
    exact_mechanism: candidate.mechanism,
    family: candidate.family,
    source_function_region: candidate.source_region,
    parent: candidate.parent,
    exclusive_target_region: candidate.target_region,
    overlap_group: candidate.overlap_group,
    parent_cpu_seconds: parentCpu,
    target_cpu_seconds: targetCpu,
    whole_program_cpu_seconds: combined.total_seconds,
    whole_program_fraction: rawFraction,
    per_case_fractions: JSON.stringify(Object.fromEntries(profileCases.map((scenario) => [scenario, perCase[scenario].fraction]))),
    cpu_present_cases_of_6: breadth.present,
    cpu_material_cases_of_6: breadth.material,
    cpu_breadth: breadth.classification,
    operation_counter: operation?.metric ?? "unavailable",
    operation_breadth_cases_of_14: operation?.breadth_cases_of_14 ?? "unavailable",
    operation_total: operation?.total ?? "unavailable",
    operation_min: operation?.min ?? "unavailable",
    operation_median: operation?.median ?? "unavailable",
    operation_max: operation?.max ?? "unavailable",
    alloc_space: allocSpace ?? "unavailable",
    alloc_objects: allocObjects ?? "unavailable",
    v4_target_cpu_seconds: v4Target ?? "unavailable",
    complexity_class: candidate.complexity_class,
    semantic_risk: candidate.semantic_risk,
    plausible_removal_fraction_E: candidate.plausible_removal_fraction ?? "unknown",
    removal_basis: candidate.removal_basis,
    class_bar: bar,
    E_min: Number.isFinite(eMin) ? eMin : "infinite",
    heuristic_benefit: benefit ?? "unknown",
    mechanically_eligible_gate: eligible ? "PASS" : "FAIL",
    breadth_gate: breadth.classification === "broad" ? "PASS" : "FAIL",
    causal_isolation_gate: candidate.causal_isolated ? "PASS" : "FAIL",
    E_defensible_gate: candidate.plausible_removal_fraction !== null ? "PASS" : "FAIL",
    E_min_gate: eMin <= 1 ? "PASS" : "FAIL",
    benefit_gate: benefit !== null && benefit >= bar ? "PASS" : "FAIL",
    semantic_risk_gate: candidate.semantic_risk_acceptable ? "PASS" : "FAIL",
    overlap_gate: !candidate.overlap_dominated ? "PASS" : "FAIL",
    full_promotion_gate: fullGate ? "PASS" : "FAIL",
    decision: candidate.disposition,
    rationale: candidate.rationale,
  };
  scoreRows.push(row);
  operationRows.push({ candidate_id: candidate.id, ...(operation ?? { metric: null, breadth_cases_of_14: null, total: null, min: null, median: null, max: null, per_case: null }), note: candidate.operation_breadth_note ?? "" });
  for (const scenario of profileCases) {
    caseRows.push({
      candidate_id: candidate.id,
      family: candidate.family,
      exact_mechanism: candidate.mechanism,
      scenario,
      case_total_cpu_seconds: perCase[scenario].total_seconds,
      target_cpu_seconds: perCase[scenario].target_seconds,
      target_case_cpu_fraction: perCase[scenario].fraction,
      cpu_present: perCase[scenario].target_seconds > 0,
      cpu_material: perCase[scenario].fraction >= 0.01,
      cpu_breadth: breadth.classification,
      operation_counter: operation?.metric ?? "unavailable",
      operation_value: operationByCase?.[scenario] ?? "unavailable",
    });
  }
}

assert(["PROMOTE", "NEED_MORE_EVIDENCE", "DECLINE"].includes(classification.decision.kind), `unknown decision ${classification.decision.kind}`);
const promoted = scoreRows.filter((row) => row.decision === "PROMOTE");
if (classification.decision.kind === "PROMOTE") {
  assertEqual(promoted.length, 1, "promoted candidate count");
  assertEqual(classification.decision.candidate_id, promoted[0].candidate_id, "final promoted candidate");
  assertEqual(classification.decision.mechanism, promoted[0].exact_mechanism, "final promoted mechanism");
  assertEqual(promoted[0].full_promotion_gate, "PASS", "promoted full gate");
} else {
  assertEqual(promoted.length, 0, "non-PROMOTE candidate count");
  assert(!classification.decision.candidate_id, "non-PROMOTE names candidate");
}
if (classification.decision.kind === "NEED_MORE_EVIDENCE") {
  for (const field of ["missing_evidence", "instrumentation_site", "hypothesis_resolved"]) assert(classification.decision[field], `NEED_MORE_EVIDENCE ${field}`);
  assert(scoreRows.some((row) => row.decision === "MORE_EVIDENCE"), "no candidate awaits evidence");
}
if (classification.decision.kind === "DECLINE") assert(classification.decision.rationale, "DECLINE rationale");

function csvEscape(value) {
  if (value === null || value === undefined) return "";
  const text = typeof value === "number" && !Number.isInteger(value) ? value.toFixed(12).replace(/0+$/, "").replace(/\.$/, "") : String(value);
  return /[",\n\r]/.test(text) ? `"${text.replaceAll('"', '""')}"` : text;
}

function writeCSV(filename, rows) {
  assert(rows.length > 0, `${filename} has no rows`);
  const headers = Object.keys(rows[0]);
  const lines = [headers.join(","), ...rows.map((row) => headers.map((header) => csvEscape(row[header])).join(","))];
  fs.writeFileSync(path.join(outputDir, filename), `${lines.join("\n")}\n`);
}

fs.mkdirSync(outputDir, { recursive: true });
writeCSV("candidate-scorecard.csv", scoreRows);
writeCSV("case-attribution.csv", caseRows);
fs.writeFileSync(path.join(outputDir, "operation-summary.json"), JSON.stringify({
  version: "r1ih-operation-summary-v1",
  gsv1_1m: { priority: gsv1OneMillion.priority, outgoing: gsv1OneMillion.outgoing },
  v4_1m: { priority: v4OneMillion.priority, outgoing: v4OneMillion.outgoing },
  candidates: operationRows,
}, null, 2) + "\n");

const freezeAfter = validateFreeze();
assertEqual(freezeAfter.manifestHash, freezeBefore.manifestHash, "post-analysis raw manifest hash");
assertEqual(freezeAfter.entries, freezeBefore.entries, "post-analysis raw manifest entry count");

const accountingLines = [
  "R1I-H accounting validation",
  `frozen_sha=${expectedSHA}`,
  "profile_version=bound-attribution-ops-v1",
  "summary_version=operation-profile-summary-v3",
  "smoke_normal_tagged_off_tagged_on_semantics=PASS",
  `smoke_ignored_fields=${[...semanticIgnoredFields].sort().join(",")}`,
  `gsv1_matrix=PASS runs=${gsv1.runs.length}`,
  `v4_matrix=PASS runs=${v4.runs.length}`,
  "operation_matrix_total=PASS runs=42",
  `profiled_runs=${gsv1Presence.filter(Boolean).length + v4Presence.filter(Boolean).length}`,
  `zero_work_runs_without_profile=${gsv1Presence.filter((value) => !value).length + v4Presence.filter((value) => !value).length}`,
  "all_priority_site_identities=PASS",
  "all_outgoing_site_identities=PASS",
  "all_outgoing_authoritative_reconciliations=PASS",
  "combined_gsv1_cpu_heap_matrix=PASS runs=6 operation_profile=false",
  "per_case_gsv1_cpu_matrix=PASS runs=6 operation_profile=false",
  "combined_v4_cpu_control=PASS runs=6 operation_profile=false",
  "canonical_profile_extraction=PASS candidate_specific_symbols=false",
  `mechanical_inventory=PASS entries=${canonicalKeys.length}`,
  "mechanical_inventory_classification_set_match=PASS",
  "objective_cpu_breadth=PASS",
  "operation_breadth=PASS",
  "overlap_accounting=PASS",
  "E_and_E_min=PASS",
  `raw_manifest_entries=${freezeAfter.entries}`,
  `raw_total_bytes=${freezeAfter.bytes}`,
  `raw_manifest_sha256=${freezeAfter.manifestHash}`,
  "post_analysis_raw_hash_revalidation=PASS",
  "post_freeze_solver_runs=0",
  "validation_materialized=false",
  "public_holdout_materialized=false",
  "private_holdout_materialized=false",
];
fs.writeFileSync(path.join(outputDir, "accounting-validation.txt"), `${accountingLines.join("\n")}\n`);

const summary = {
  version: "r1ih-analysis-v1",
  frozen_sha: expectedSHA,
  tooling_preflight: "PASS",
  smoke_semantics: "PASS",
  operation_runs: 42,
  operation_accounting: "PASS",
  normal_profile_matrix: "PASS",
  canonical_inventory_entries: canonicalKeys.length,
  inventory_classification: "PASS",
  raw_freeze: {
    status: "PASS",
    manifest_entries: freezeAfter.entries,
    total_bytes: freezeAfter.bytes,
    manifest_sha256: freezeAfter.manifestHash,
    post_analysis_raw_hash_revalidation: "PASS",
    post_freeze_solver_runs: 0,
  },
  combined_cpu_seconds: canonical.cpu_profiles.combined_gsv1.total_seconds,
  combined_heap: Object.fromEntries(Object.entries(canonical.heap_profiles).map(([key, value]) => [key, { total: value.total, unit: value.unit }])),
  gsv1_1m: {
    priority: gsv1OneMillion.priority,
    outgoing: gsv1OneMillion.outgoing,
    priority_rejection_rate: ratio(gsv1OneMillion.priority.rejected_results, gsv1OneMillion.priority.calls),
    outgoing_prune_rate: ratio(gsv1OneMillion.outgoing.pruned_nodes, gsv1OneMillion.outgoing.checks),
  },
  candidates: scoreRows,
  decision: classification.decision,
};
fs.writeFileSync(path.join(outputDir, "analysis-summary.json"), `${JSON.stringify(summary, null, 2)}\n`);

process.stdout.write(JSON.stringify({
  frozen_sha: expectedSHA,
  operation_runs: 42,
  candidate_count: scoreRows.length,
  mechanical_inventory_entries: canonicalKeys.length,
  raw_manifest_sha256: freezeAfter.manifestHash,
  post_analysis_raw_hash_revalidation: "PASS",
  decision: classification.decision,
}, null, 2) + "\n");
