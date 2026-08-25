import crypto from "node:crypto";
import fs from "node:fs";
import path from "node:path";

const artifactDir = process.argv[2] ? path.resolve(process.argv[2]) : undefined;
const outputDir = process.argv[3] ? path.resolve(process.argv[3]) : artifactDir ? path.join(artifactDir, "review") : undefined;
if (!artifactDir) throw new Error("usage: node r1if-analysis.mjs <artifact-dir> [output-dir]");

const expectedSHA = "9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b";
const expectedCases = Array.from({ length: 14 }, (_, index) => `gsv2-${String(index + 13).padStart(3, "0")}`);
const profileCases = ["gsv2-013", "gsv2-015", "gsv2-016", "gsv2-018", "gsv2-021", "gsv2-024"];
const prioritySites = ["constellation_filter", "repair_dfs", "plateau_prefilter", "plateau_dfs"];
const outgoingSites = ["search", "repair"];
const carryForward = [
  "plateau archive full reselection",
  "outgoing placement map/index",
  "outgoing static-star compatibility",
  "outgoing target placement lookup",
  "residual coveragePlacementKey work",
  "filteredRemovedOptions",
  "canonical-copy ranking/indexing",
  "physical instance ID construction",
  "residual priority geometry",
];
const promotionBars = { C0: 0.02, C1: 0.025, C2: 0.03 };

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

function readText(relativePath) {
  return fs.readFileSync(path.join(artifactDir, relativePath), "utf8");
}

function readJSON(relativePath) {
  return JSON.parse(readText(relativePath));
}

function ratio(numerator, denominator) {
  return denominator ? numerator / denominator : 0;
}

function normalizeSymbol(symbol) {
  return symbol.replace(/ \(inline\)$/, "");
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

function assertArrayEqual(actual, expected, message) {
  assertEqual(JSON.stringify(actual), JSON.stringify(expected), message);
}

function validateReportEnvelope(report, label, { budgets, variant, operationProfiling, cases }) {
  assertEqual(report.build_revision, expectedSHA, `${label} build_revision`);
  assertEqual(report.repeat, 1, `${label} repeat`);
  assertEqual(report.workers, 1, `${label} workers`);
  assertEqual(report.operation_profiling ?? false, operationProfiling, `${label} operation_profiling`);
  assertEqual(report.diagnostic, false, `${label} diagnostic`);
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
    assert(cases.includes(run.scenario), `${label} unexpected run scenario ${run.scenario}`);
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
  "bound_operation_profile", "root_packing_operation_profile", "operation_profile",
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

function validateFreeze() {
  const manifestText = readText("provenance/RAW-SHA256SUMS.txt");
  const entries = manifestText.trim().split(/\r?\n/).map((line) => {
    const match = line.match(/^([0-9a-f]{64})  (.+)$/);
    assert(match, `invalid raw manifest line: ${line}`);
    return { hash: match[1], relativePath: match[2] };
  });
  assert(entries.length > 0, "raw manifest is empty");
  for (const entry of entries) {
    const absolute = path.resolve(artifactDir, entry.relativePath);
    assert(absolute.startsWith(`${artifactDir}${path.sep}`), `manifest path escaped artifact root: ${entry.relativePath}`);
    const actual = crypto.createHash("sha256").update(fs.readFileSync(absolute)).digest("hex");
    assertEqual(actual, entry.hash, `raw manifest hash ${entry.relativePath}`);
  }
  const manifestHash = crypto.createHash("sha256").update(fs.readFileSync(path.join(artifactDir, "provenance/RAW-SHA256SUMS.txt"))).digest("hex");
  const manifestHashLine = readText("provenance/RAW-SHA256SUMS.sha256").trim();
  assertEqual(manifestHashLine, `${manifestHash}  provenance/RAW-SHA256SUMS.txt`, "separate manifest hash");
  const freezeRecord = readText("provenance/freeze-record.txt");
  assert(freezeRecord.includes(`raw_file_count=${entries.length}`), "freeze record file count mismatch");
  assert(freezeRecord.includes(`raw_manifest_sha256=${manifestHash}`), "freeze record manifest hash mismatch");
  assert(freezeRecord.includes("raw_manifest_revalidation=PASS"), "freeze record lacks hash revalidation");
  assert(freezeRecord.includes("raw_read_only_revalidation=PASS"), "freeze record lacks read-only revalidation");
  assert(freezeRecord.includes("post_freeze_solver_runs=0"), "freeze record does not declare zero post-freeze solver runs");
  return { entries: entries.length, manifestHash };
}

const smokeNormal = readJSON("smoke/r1if-smoke-normal.json");
const smokeTaggedOff = readJSON("smoke/r1if-smoke-tagged-off.json");
const smokeTaggedOn = readJSON("smoke/r1if-smoke-tagged-on.json");
for (const [label, report, operationProfiling] of [
  ["normal", smokeNormal, false],
  ["tagged-off", smokeTaggedOff, false],
  ["tagged-on", smokeTaggedOn, true],
]) {
  validateReportEnvelope(report, `smoke ${label}`, {
    budgets: [250000], variant: "general-search-v1", operationProfiling, cases: ["gsv2-013"],
  });
}
assertSemanticEqual(smokeTaggedOff, smokeNormal, "smoke normal/tagged-off semantics");
assertSemanticEqual(smokeTaggedOn, smokeTaggedOff, "smoke tagged-off/tagged-on semantics");
validateProfiledRun(smokeTaggedOn.runs[0], "smoke/tagged-on");

const gsv1 = readJSON("operations/r1if-gsv1.json");
const v4 = readJSON("operations/r1if-v4.json");
const gsv1Summary = readJSON("operations/r1if-gsv1-summary.json");
const v4Summary = readJSON("operations/r1if-v4-summary.json");
const gsv1Presence = validateOperationMatrix(gsv1, "GSV1 operations", [250000, 1000000], "general-search-v1");
const v4Presence = validateOperationMatrix(v4, "V4 operations", [1000000], "v4");
assertEqual(gsv1Summary.version, "operation-profile-summary-v3", "GSV1 summary version");
assertEqual(v4Summary.version, "operation-profile-summary-v3", "V4 summary version");

const combinedGSV1 = readJSON("profiles/r1if-gsv1-profile.json");
const combinedV4 = readJSON("profiles/r1if-v4-profile.json");
validateNormalReport(combinedGSV1, "combined GSV1 profile", profileCases, "general-search-v1");
validateNormalReport(combinedV4, "combined V4 profile", profileCases, "v4");
for (const scenario of profileCases) {
  validateNormalReport(readJSON(`profiles/per-case/${scenario}.json`), `per-case ${scenario}`, [scenario], "general-search-v1");
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
const canonical = readJSON("derived/canonical-profile-data.json");
assertEqual(canonical.version, "r1if-canonical-profile-v1", "canonical profile version");
assertEqual(canonical.generated_by, "r1if-profile-extract.ps1", "canonical extractor");
assertEqual(canonical.binary, "binaries/solver.exe", "canonical binary");
assertArrayEqual(Object.keys(canonical.cpu_profiles.per_case).sort(), [...profileCases].sort(), "canonical per-case profiles");

function cpuProfile(id) {
  if (id === "combined_gsv1") return canonical.cpu_profiles.combined_gsv1;
  if (id === "combined_v4") return canonical.cpu_profiles.combined_v4;
  const profile = canonical.cpu_profiles.per_case[id];
  assert(profile, `unknown CPU profile ${id}`);
  return profile;
}

function findFunctionRow(profile, symbol, allowMissing = false) {
  const rows = profile.top.filter((row) => normalizeSymbol(row.symbol) === symbol);
  assert(rows.length <= 1, `ambiguous function metric ${profile.id}/${symbol}`);
  if (rows.length === 0) {
    if (allowMissing) return null;
    throw new Error(`missing function metric ${profile.id}/${symbol}`);
  }
  return rows[0];
}

function resolveCpuMetric(profileId, metric, allowMissing = false) {
  const profile = cpuProfile(profileId);
  if (metric.kind === "function") {
    const row = findFunctionRow(profile, metric.symbol, allowMissing);
    return row ? row[metric.value] : 0;
  }
  assertEqual(metric.kind, "source_lines", `unknown CPU metric kind for ${profileId}`);
  const extract = profile.source_extracts[metric.extract_id];
  assert(extract, `missing source extract ${profileId}/${metric.extract_id}`);
  const routines = extract.routines.filter((routine) => normalizeSymbol(routine.symbol) === metric.routine);
  if (routines.length === 0 && allowMissing) return 0;
  assertEqual(routines.length, 1, `source routine ${profileId}/${metric.extract_id}/${metric.routine}`);
  let total = 0;
  for (const lineNumber of metric.lines) {
    const lines = routines[0].lines.filter((line) => line.line === lineNumber);
    assertEqual(lines.length, 1, `source line ${profileId}/${metric.extract_id}/${metric.routine}:${lineNumber}`);
    total += lines[0][`${metric.value}_seconds`];
  }
  return total;
}

function resolveHeapMetric(metric) {
  const profile = canonical.heap_profiles[metric.sample_index];
  assert(profile, `unknown heap sample index ${metric.sample_index}`);
  const rows = profile.top.filter((row) => normalizeSymbol(row.symbol) === metric.symbol);
  assert(rows.length <= 1, `ambiguous heap metric ${metric.sample_index}/${metric.symbol}`);
  return rows.length ? rows[0][metric.value] : 0;
}

function candidateOperationValues(candidate, aggregate) {
  const values = {};
  for (const metric of candidate.operation_metrics) {
    assert(["priority", "outgoing"].includes(metric.family), `${candidate.id} operation family`);
    const family = aggregate[metric.family];
    assert(Object.hasOwn(family, metric.field), `${candidate.id} unknown operation metric ${metric.family}.${metric.field}`);
    values[`${metric.family}.${metric.field}`] = family[metric.field];
  }
  return values;
}

function caseOperationValues(candidate, scenario) {
  const row = gsv1OneMillion.cases.find((entry) => entry.scenario === scenario);
  assert(row, `missing GSV1 operation row ${scenario}`);
  return candidateOperationValues(candidate, row);
}

function validateCitation(relativePath, candidateID) {
  const absolute = path.resolve(artifactDir, relativePath);
  assert(absolute.startsWith(`${artifactDir}${path.sep}`), `${candidateID} citation escaped artifact root`);
  assert(fs.existsSync(absolute), `${candidateID} citation missing: ${relativePath}`);
}

const input = readJSON("review-input/r1if-candidates.json");
assertEqual(input.version, "r1if-review-input-v1", "review input version");
assertEqual(input.frozen_sha, expectedSHA, "review input frozen SHA");
assert(Array.isArray(input.candidates) && input.candidates.length > 0, "candidate list is empty");
assert(Array.isArray(input.inventory) && input.inventory.length > 0, "inventory is empty");

const candidatesByID = new Map();
for (const candidate of input.candidates) {
  assert(candidate.id && !candidatesByID.has(candidate.id), `duplicate or empty candidate id ${candidate.id}`);
  assert(candidate.mechanism && !["outgoing", "priority", "runtime.memmove", "runtime.duffcopy"].includes(candidate.mechanism), `${candidate.id} is not exact`);
  assert(Number.isFinite(candidate.plausible_removal_fraction) && candidate.plausible_removal_fraction >= 0 && candidate.plausible_removal_fraction <= 1, `${candidate.id} removal fraction`);
  assert(Object.hasOwn(promotionBars, candidate.complexity_class), `${candidate.id} complexity class`);
  assert(candidate.overlap_group, `${candidate.id} overlap group`);
  assert(["PROMOTE", "REJECT", "MORE_EVIDENCE"].includes(candidate.disposition), `${candidate.id} disposition`);
  assert(Array.isArray(candidate.operation_metrics), `${candidate.id} operation metrics`);
  assert(candidate.operation_breadth_note, `${candidate.id} operation breadth note`);
  if (candidate.operation_metrics.length === 0) {
    assert(/unavailable|absent|no serialized/i.test(candidate.operation_breadth_note), `${candidate.id} must explain missing operation breadth`);
  }
  assert(Array.isArray(candidate.extract_citations) && candidate.extract_citations.length > 0, `${candidate.id} extract citations`);
  for (const citation of candidate.extract_citations) validateCitation(citation, candidate.id);
  for (const gate of ["causal_region_isolable", "semantic_risk_acceptable", "no_stronger_overlapping_explanation"]) {
    assert(typeof candidate.promotion_gate?.[gate] === "boolean", `${candidate.id} promotion gate ${gate}`);
  }
  if (/archive/i.test(`${candidate.family} ${candidate.mechanism}`)) {
    for (const field of ["admission_semantics", "selected_set_equivalence", "selected_order_equivalence", "signature_diversity", "downstream_base_selection"]) {
      assert(candidate.archive_semantics?.[field], `${candidate.id} archive semantics ${field}`);
    }
  }
  candidatesByID.set(candidate.id, candidate);
}

const allowedInventoryKinds = new Set([
  "cpu_edge_ge_1pct", "cpu_parent_ge_1_5pct", "cpu_flat_project_owned_top20", "cpu_cum_project_owned_top20",
  "heap_alloc_space_ge_1pct", "heap_alloc_objects_project_owned_top10", "carry_forward",
]);
for (const entry of input.inventory) {
  assert(allowedInventoryKinds.has(entry.kind), `unknown inventory kind ${entry.kind}`);
  assert(Boolean(entry.candidate_id) !== Boolean(entry.exclusion_reason), `inventory must map or exclude exactly once: ${entry.kind}/${entry.symbol ?? entry.mechanism}`);
  if (entry.candidate_id) assert(candidatesByID.has(entry.candidate_id), `inventory maps to unknown candidate ${entry.candidate_id}`);
}

function projectOwned(rows) {
  return rows.filter((row) => normalizeSymbol(row.symbol).startsWith("backpack-brawl-solver/internal/"));
}

function validateRankedInventory(kind, expectedRows) {
  const entries = input.inventory.filter((entry) => entry.kind === kind).sort((a, b) => a.rank - b.rank);
  assertEqual(entries.length, expectedRows.length, `${kind} inventory count`);
  entries.forEach((entry, index) => {
    assertEqual(entry.rank, index + 1, `${kind} rank ${index + 1}`);
    assertEqual(entry.symbol, normalizeSymbol(expectedRows[index].symbol), `${kind} symbol rank ${index + 1}`);
  });
}

const combinedCpu = canonical.cpu_profiles.combined_gsv1;
validateRankedInventory("cpu_flat_project_owned_top20", projectOwned(combinedCpu.top).slice(0, 20));
validateRankedInventory("cpu_cum_project_owned_top20", projectOwned(combinedCpu.top_cum).slice(0, 20));
validateRankedInventory("heap_alloc_objects_project_owned_top10", projectOwned(canonical.heap_profiles.alloc_objects.top).slice(0, 10));

const expectedAllocSpace = projectOwned(canonical.heap_profiles.alloc_space.top)
  .filter((row) => ratio(row.flat, canonical.heap_profiles.alloc_space.total) >= 0.01);
const allocEntries = input.inventory.filter((entry) => entry.kind === "heap_alloc_space_ge_1pct");
assertEqual(allocEntries.length, expectedAllocSpace.length, "heap alloc_space >=1% inventory count");
assertArrayEqual(
  allocEntries.map((entry) => entry.symbol).sort(),
  expectedAllocSpace.map((row) => normalizeSymbol(row.symbol)).sort(),
  "heap alloc_space >=1% inventory symbols",
);

const carryEntries = input.inventory.filter((entry) => entry.kind === "carry_forward");
assertArrayEqual(carryEntries.map((entry) => entry.mechanism).sort(), [...carryForward].sort(), "carry-forward registry");

for (const entry of input.inventory.filter((candidate) => candidate.kind === "cpu_edge_ge_1pct" || candidate.kind === "cpu_parent_ge_1_5pct")) {
  assert(entry.metric, `${entry.kind}/${entry.symbol} lacks canonical metric`);
  const value = resolveCpuMetric("combined_gsv1", entry.metric);
  const threshold = entry.kind === "cpu_edge_ge_1pct" ? 0.01 : 0.015;
  assert(ratio(value, combinedCpu.total_seconds) >= threshold, `${entry.kind}/${entry.symbol} below threshold`);
}

const frozen = validateFreeze();
const mappedCandidates = new Set(input.inventory.filter((entry) => entry.candidate_id).map((entry) => entry.candidate_id));
const scoreRows = [];
const candidateDerived = new Map();
for (const candidate of input.candidates) {
  const parentCPU = resolveCpuMetric("combined_gsv1", candidate.parent_metric);
  const targetCPU = resolveCpuMetric("combined_gsv1", candidate.target_metric);
  assert(targetCPU <= parentCPU + 1e-9, `${candidate.id} target exceeds parent`);
  const perCase = Object.fromEntries(profileCases.map((scenario) => [scenario, resolveCpuMetric(scenario, candidate.per_case_target_metric, true)]));
  const presentCases = profileCases.filter((scenario) => perCase[scenario] > 0).length;
  const materialCases = profileCases.filter((scenario) => ratio(perCase[scenario], cpuProfile(scenario).total_seconds) >= 0.01).length;
  const breadth = presentCases >= 5 && materialCases >= 4 ? "broad" : materialCases <= 2 ? "concentrated" : materialCases === 3 ? "ambiguous" : "not-broad";
  const operationValues = candidateOperationValues(candidate, gsv1OneMillion);
  const operationByCase = Object.fromEntries(expectedCases.map((scenario) => [scenario, caseOperationValues(candidate, scenario)]));
  const operationBreadth = candidate.operation_metrics.length
    ? expectedCases.filter((scenario) => Object.values(operationByCase[scenario]).some((value) => value > 0)).length
    : null;
  const operationVolume = Object.fromEntries(candidate.operation_metrics.map((metric) => {
    const key = `${metric.family}.${metric.field}`;
    return [key, operationValues[key]];
  }));
  const benefit = ratio(targetCPU, combinedCpu.total_seconds) * candidate.plausible_removal_fraction;
  const normalBar = promotionBars[candidate.complexity_class];
  const qualifying = mappedCandidates.has(candidate.id);
  const fullPromotionGate = qualifying && breadth === "broad" && benefit >= normalBar && candidate.exclusive_subregion &&
    candidate.promotion_gate.causal_region_isolable && candidate.promotion_gate.semantic_risk_acceptable &&
    candidate.promotion_gate.no_stronger_overlapping_explanation;
  if (candidate.disposition === "PROMOTE") assert(fullPromotionGate, `${candidate.id} is promoted without clearing every gate`);
  const allocSpace = candidate.alloc_space_metric ? resolveHeapMetric(candidate.alloc_space_metric) : 0;
  const allocObjects = candidate.alloc_objects_metric ? resolveHeapMetric(candidate.alloc_objects_metric) : 0;
  const v4Target = candidate.v4_target_metric ? resolveCpuMetric("combined_v4", candidate.v4_target_metric, true) : null;
  const row = {
    candidate_id: candidate.id,
    family: candidate.family,
    exact_mechanism: candidate.mechanism,
    parent: candidate.parent,
    child_exclusive_edge: candidate.child,
    overlap_group: candidate.overlap_group,
    parent_cpu_seconds: parentCPU,
    target_cpu_seconds: targetCPU,
    whole_program_fraction: ratio(parentCPU, combinedCpu.total_seconds),
    target_fraction_of_parent: ratio(targetCPU, parentCPU),
    plausible_removal_fraction: candidate.plausible_removal_fraction,
    heuristic_whole_program_benefit: benefit,
    per_case_cpu_seconds: JSON.stringify(perCase),
    cpu_present_cases_of_6: presentCases,
    cpu_material_cases_of_6: materialCases,
    cpu_breadth: breadth,
    operation_breadth_cases_of_14: operationBreadth ?? "unavailable",
    operation_volume: candidate.operation_metrics.length ? JSON.stringify(operationVolume) : "unavailable",
    operation_breadth_note: candidate.operation_breadth_note,
    v4_target_cpu_seconds: v4Target ?? "",
    alloc_space: allocSpace,
    alloc_space_fraction: ratio(allocSpace, canonical.heap_profiles.alloc_space.total),
    alloc_objects: allocObjects,
    complexity_class: candidate.complexity_class,
    semantic_risk: candidate.semantic_risk,
    evidence_quality: candidate.evidence_quality,
    promotion_bar: normalBar,
    qualifying_inventory_gate: qualifying ? "PASS" : "FAIL",
    breadth_gate: breadth === "broad" ? "PASS" : "FAIL",
    benefit_gate: benefit >= normalBar ? "PASS" : "FAIL",
    causal_isolation_gate: candidate.exclusive_subregion && candidate.promotion_gate.causal_region_isolable ? "PASS" : "FAIL",
    semantic_risk_gate: candidate.promotion_gate.semantic_risk_acceptable ? "PASS" : "FAIL",
    overlap_gate: candidate.promotion_gate.no_stronger_overlapping_explanation ? "PASS" : "FAIL",
    full_promotion_gate: fullPromotionGate ? "PASS" : "FAIL",
    decision: candidate.disposition,
    rationale: candidate.rationale,
  };
  scoreRows.push(row);
  candidateDerived.set(candidate.id, { candidate, row, perCase, operationByCase });

  if (ratio(targetCPU, combinedCpu.total_seconds) >= 0.01) {
    assert(input.inventory.some((entry) => entry.kind === "cpu_edge_ge_1pct" && entry.candidate_id === candidate.id), `${candidate.id} lacks >=1% edge inventory entry`);
  }
  if (ratio(parentCPU, combinedCpu.total_seconds) >= 0.015) {
    assert(input.inventory.some((entry) => entry.kind === "cpu_parent_ge_1_5pct" && entry.candidate_id === candidate.id), `${candidate.id} lacks >=1.5% parent inventory entry`);
  }
}

function csvEscape(value) {
  if (value === null || value === undefined) return "";
  const text = typeof value === "number" && !Number.isInteger(value)
    ? value.toFixed(12).replace(/0+$/, "").replace(/\.$/, "")
    : String(value);
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
const caseRows = [];
for (const { candidate, row, perCase, operationByCase } of candidateDerived.values()) {
  for (const scenario of profileCases) {
    const targetCPU = perCase[scenario];
    const totalCPU = cpuProfile(scenario).total_seconds;
    const operationValues = operationByCase[scenario];
    caseRows.push({
      candidate_id: candidate.id,
      family: candidate.family,
      exact_mechanism: candidate.mechanism,
      scenario,
      case_total_cpu_seconds: totalCPU,
      target_cpu_seconds: targetCPU,
      target_case_cpu_fraction: ratio(targetCPU, totalCPU),
      cpu_present: targetCPU > 0,
      cpu_material: ratio(targetCPU, totalCPU) >= 0.01,
      cpu_breadth: row.cpu_breadth,
      operation_counter_values: candidate.operation_metrics.length ? JSON.stringify(operationValues) : "unavailable",
      operation_present: candidate.operation_metrics.length ? Object.values(operationValues).some((value) => value > 0) : "unavailable",
    });
  }
}
writeCSV("case-attribution.csv", caseRows);

assert(["PROMOTE", "NEED_MORE_EVIDENCE", "DECLINE"].includes(input.decision.kind), `unknown final decision ${input.decision.kind}`);
const promoted = scoreRows.filter((row) => row.decision === "PROMOTE");
if (input.decision.kind === "PROMOTE") {
  assertEqual(promoted.length, 1, "promoted candidate count");
  assertEqual(input.decision.candidate_id, promoted[0].candidate_id, "final promoted candidate");
  assertEqual(input.decision.mechanism, promoted[0].exact_mechanism, "final promoted mechanism");
  assertEqual(promoted[0].full_promotion_gate, "PASS", "promoted candidate full gate");
} else {
  assertEqual(promoted.length, 0, "non-PROMOTE candidate count");
  assert(!input.decision.candidate_id, "non-PROMOTE decision names a candidate");
}
if (input.decision.kind === "NEED_MORE_EVIDENCE") {
  assert(input.decision.missing_data && input.decision.instrumentation, "NEED_MORE_EVIDENCE must name data and instrumentation");
  assert(scoreRows.some((row) => row.decision === "MORE_EVIDENCE"), "NEED_MORE_EVIDENCE has no candidate awaiting evidence");
}
if (input.decision.kind === "DECLINE") assert(input.decision.rationale, "DECLINE requires rationale");

const accountingLines = [
  "R1I-F accounting validation",
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
  "canonical_profile_extraction=PASS",
  "mechanical_inventory=PASS",
  "objective_breadth_classification=PASS",
  "benefit_formula=PASS",
  "overlap_groups_recorded=PASS",
  `raw_manifest_entries=${frozen.entries}`,
  `raw_manifest_sha256=${frozen.manifestHash}`,
  "post_freeze_solver_runs=0",
  "validation_materialized=false",
  "public_holdout_materialized=false",
  "private_holdout_materialized=false",
];
fs.writeFileSync(path.join(outputDir, "accounting-validation.txt"), `${accountingLines.join("\n")}\n`);

const summary = {
  version: "r1if-analysis-v1",
  frozen_sha: expectedSHA,
  smoke_semantics: "PASS",
  operation_accounting: "PASS",
  normal_profile_matrix: "PASS",
  raw_freeze: { status: "PASS", manifest_entries: frozen.entries, manifest_sha256: frozen.manifestHash, post_freeze_solver_runs: 0 },
  combined_cpu_seconds: combinedCpu.total_seconds,
  combined_heap_alloc_space: canonical.heap_profiles.alloc_space.total,
  gsv1_1m: {
    priority: gsv1OneMillion.priority,
    outgoing: gsv1OneMillion.outgoing,
    priority_rejection_rate: ratio(gsv1OneMillion.priority.rejected_results, gsv1OneMillion.priority.calls),
    outgoing_prune_rate: ratio(gsv1OneMillion.outgoing.pruned_nodes, gsv1OneMillion.outgoing.checks),
    map_insertions_per_check: ratio(gsv1OneMillion.outgoing.placed_map_insertions, gsv1OneMillion.outgoing.checks),
    coverage_keys_per_check: ratio(gsv1OneMillion.outgoing.coverage_placement_key_calls, gsv1OneMillion.outgoing.checks),
    targets_per_placed_source: ratio(gsv1OneMillion.outgoing.placed_source_target_iterations, gsv1OneMillion.outgoing.placed_source_iterations),
  },
  v4_1m: { priority: v4OneMillion.priority, outgoing: v4OneMillion.outgoing },
  candidates: scoreRows,
  decision: input.decision,
};
fs.writeFileSync(path.join(outputDir, "analysis-summary.json"), `${JSON.stringify(summary, null, 2)}\n`);

process.stdout.write(`${JSON.stringify({
  frozen_sha: expectedSHA,
  smoke_semantics: "PASS",
  operation_runs: gsv1.runs.length + v4.runs.length,
  candidate_count: scoreRows.length,
  case_attribution_rows: caseRows.length,
  raw_manifest_sha256: frozen.manifestHash,
  decision: input.decision,
}, null, 2)}\n`);
