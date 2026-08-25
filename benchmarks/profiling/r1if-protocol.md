# R1I-F protocol — post-R1I-E efficiency recalibration

## Status, exact question, and frozen baseline

R1I-F is an evidence-only recalibration in macro-stage 12/18. It starts from
the post-R1I-E `main` revision:

```text
9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b
```

Before creating the branch, fetch `origin/main`. If it is not the revision
above, inspect the complete delta and freeze the actual revision explicitly;
never reuse this protocol silently against another tree. Official solver
measurements use a clean detached clone of the frozen revision outside
OneDrive.

R1I-F answers only:

> After H2a, R1I-C, and R1I-E, what broad, removable, causally isolable, and
> semantically safe efficiency mechanism still justifies implementation?

It does not implement an optimization. It ends in exactly one of:

```text
PROMOTE: <one exact mechanism>

NEED MORE EVIDENCE: <exact missing evidence and instrumentation>

DECLINE: no efficiency mechanism currently justifies another optimization
```

Two mechanisms can never be promoted together.

## Frozen scope and protected corpus

The only selectable corpus is the fourteen `general-search-v2` development
cases `gsv2-013..026`. The validation cases `027..036`, public holdout cases
`001..006`, and private holdout cases `007..012` remain unmaterialized and
unexecuted. The longitudinal CPU slice is permanently:

```text
gsv2-013,gsv2-015,gsv2-016,gsv2-018,gsv2-021,gsv2-024
```

The principal seed variant is `general-search-v1`; `v4` is the control. Every
official run uses `workers=1`, `repeat=1`, and `diagnostic=false`.

R1I-F may add only `benchmarks/profiling/r1if-*`, evidence analyzers/scripts,
documentation, and a compact evidence bundle. It must not change production
solver Go files, scoring, objectives, ranking, scheduler, beam, archive,
bounds, node charging, budgets, catalog, generator, suite manifest/lock, or
Web/WASM behavior.

## Commit boundary

Before the first official measurement, commit all of:

- this protocol and its matrix/freeze rules;
- `r1if-analysis.mjs`;
- `r1if-profile-extract.ps1`;
- `r1if-freeze.ps1`;
- the canonical-profile and review-input schemas;
- inventory, breadth, overlap, benefit, complexity, and decision rules.

The intended commits are:

```text
1. docs(profiling): freeze R1I-F post-R1I-E selection protocol
2. docs(profiling): record R1I-F evidence and mechanism decision
```

No threshold or benefit formula may change after inspecting the new profile.

## Baseline audit

The premeasurement audit requires:

```powershell
git fetch origin
git status --porcelain
git rev-parse HEAD
git rev-parse origin/main
```

The measurement clone must be detached and clean at the frozen SHA. Audit that
`writePlacementKeyInt` exists, `placementKey` uses direct bounded formatting,
R1I-D and R1I-E findings/evidence exist, and `AGENTS.md` contains the manual
merge boundary.

## Clean builds and provenance

Build exactly once in the detached measurement clone:

```powershell
go build -buildvcs=true -o <artifacts>/binaries/solver.exe ./cmd/backpack-brawl-solver
go build -buildvcs=true -tags searchprofile -o <artifacts>/binaries/solver-searchprofile.exe ./cmd/backpack-brawl-solver
```

For both binaries, `go version -m` must report the frozen
`vcs.revision` and `vcs.modified=false`. No official measurement uses
`go run`. Record binary SHA-256; Go version; GOOS/GOARCH; Windows version and
build; CPU; logical processors; visible RAM; timezone; Git status; catalog
hash and lock-verified catalog digest; manifest, lock, and generator hashes.

## Blocking repository gates

Before collection, all of these must pass on the frozen tree:

```text
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
normal versus searchprofile-OFF semantic snapshot
validate-catalog
verify-search-suite for general-search-v2
CI-equivalent npm ci and npm run build in a separate clean Web/WASM clone
```

The Web build is isolated so generated WASM cannot dirty the measurement
clone. A failure is blocking and cannot be offset by performance results.

## Development-only materialization

Verify the v2 suite against `data/catalog.json` and its immutable lock, then
materialize only role `development`. Independently require exactly fourteen
JSON files named `gsv2-013.json` through `gsv2-026.json`. Provenance must state:

```text
validation_materialized=false
public_holdout_materialized=false
private_holdout_materialized=false
```

## Blocking semantic smoke

Run `gsv2-013`, GSV1, 250k, one worker, no diagnostics, once with:

1. the normal binary;
2. the tagged binary with profiling OFF;
3. the tagged binary with operation profiling ON.

Normal equals tagged-OFF exactly after timing fields. Tagged-OFF equals
tagged-ON after removing only the operation-profile marker/payload and timing
fields. Score and priority counts, craft/star/item metrics, layout key,
canonical hash, placements, nodes and charging, all budgets, ordering,
scheduler, beam, archive, stop/limited/refined state, bound checks/prunes and
vectors remain compared. Any deterministic difference aborts R1I-F.

## Official operation matrix

Only the tagged binary with `--operation-profile` is used:

| Variant | Cases | Budgets | Runs |
| --- | --- | --- | ---: |
| GSV1 | `013..026` | `250000,1000000` | 28 |
| V4 | `013..026` | `1000000` | 14 |

The analyzer reads the JSON payload itself and verifies top-level
`constellation_seed_variant`, budgets, workers, repeat, operation profiling,
diagnostic state, and build revision. It also verifies each run's scenario,
budget, repeat, variant, and operation-profiling fields rather than trusting
filenames.

For every present `bound-attribution-ops-v1` profile, these identities are
immutable:

```text
Priority calls = feasible + rejected
Constellation input = retained + rejected
Removed candidates = fixed-overlap rejects + outside-free rejects + retained
Geometry candidates = the four geometry regimes
Geometry candidates = overlap rejects + star-position-hit calls
Star-position-hit true = slot-target hits

Outgoing priority-source matches = zero-star skips + placed sources + free sources
Placed-source target iterations = self skips + target placement lookups
Target placement lookups = placed targets + unplaced targets
Source-hits-target calls = placed targets
Coverage-placement-key calls = placed-source iterations
Placed-potential lookups = placed-source iterations
Free-potential lookups = free-source iterations
Popcount calls = placed sources + free sources
Search checks + repair checks = authoritative outgoing checks
Search prunes + repair prunes = authoritative outgoing prunes
```

A zero-work run may omit the profile only when authoritative checks and prunes
are both zero.

## Whole-program CPU and heap

Use the normal binary only. The principal combined run is GSV1, 1M,
`workers=1`, on the six frozen cases, collecting together:

```text
CPU pprof
heap pprof (alloc_space, alloc_objects, inuse_space)
normal benchmark JSON
```

Collect six additional GSV1 CPU profiles, one for each frozen case with the
same settings. Collect one combined V4 CPU profile on the same six cases.
Operation profiling and diagnostics remain off in all normal reports.

## Canonical extraction and reduced manual input

`r1if-profile-extract.ps1` is the only supported path from raw pprof to
critical scorecard numbers:

```text
raw pprof
  -> versioned pprof commands
  -> canonical-profile-data.json and cited text extracts
  -> r1if-analysis.mjs
  -> scorecard and decision
```

It produces combined `cpu-top`, `cpu-top-cum`, `cpu-callers`, and `cpu-tree`;
all three heap views; V4 and per-case views; annotated source extracts for the
frozen carry-forward registry; and machine-readable function/source-line
samples. Extra symbols may be supplied after discovery, but the invocation and
generated extract are recorded. This is derived analysis, not a solver run.

Review input selects canonical function or annotated source-line metrics. It
does not contain CPU seconds, heap bytes/objects, whole-program fractions, or
benefit calculations. Human classifications must name parent/caller, child or
exclusive source lines, overlap group, and one or more extractor citations.

## Mechanical candidate inventory

A mechanism enters when at least one condition holds:

```text
isolable exclusive edge >= 1.0% combined GSV1 CPU
isolable parent cumulative >= 1.5%
project-owned top-20 flat CPU
project-owned top-20 cumulative CPU
>= 1.0% whole-profile alloc_space
project-owned top-10 alloc_objects
```

`r1if-analysis.mjs` derives the ranked function/allocation lists from canonical
data and requires every entry to map to exactly one candidate or carry an
explicit exclusion. Runtime/standard-library primitives are assigned to the
responsible project caller; they are never mechanisms.

The carry-forward registry is mandatory but starts with no winner:

```text
plateau archive full reselection
outgoing placement map/index
outgoing static-star compatibility
outgoing target placement lookup
residual coveragePlacementKey work
filteredRemovedOptions
canonical-copy ranking/indexing
physical instance ID construction
residual priority geometry
newly discovered hotspots
```

Old `coveragePlacementKey` values cannot justify a new implementation because
R1I-E reduced its cumulative CPU from about 9.78s to about 2.43s.

## CPU and operation breadth

For each case:

```text
present  := target CPU > 0
material := target CPU / total case CPU >= 1%
```

The analyzer classifies:

```text
broad        := present in >=5/6 AND material in >=4/6
concentrated := material in <=2/6
ambiguous    := material in 3/6
not-broad    := every remaining combination
```

When deterministic counters exist, operation breadth records cases with a
positive operation count out of fourteen plus total/per-case volume. Without
a counter, review input must state the exact missing attribution. CPU breadth
answers where a mechanism costs; operation breadth answers where it occurs.

## Scorecard, benefit, complexity, and overlap

Every candidate row contains exact mechanism/family; parent and exclusive
child/edge; overlap group; parent and target CPU; whole-program and parent
fractions; removal fraction `E`; heuristic benefit; per-case CPU and breadth;
operation breadth; V4 CPU; allocation evidence; complexity; semantic risk;
evidence quality; promotion bar/gates; disposition; and rationale.

The analyzer alone computes:

```text
F = parentCPU / totalCPU
Q = targetEdge / parentCPU
Benefit = F * Q * E = targetEdgeCPU / totalCPU * E
```

This is a selection heuristic, not a wall-clock prediction. Overlapping
parent/child mechanisms share `overlap_group` and cannot be added. Only an
exclusive edge receives independent benefit.

Complexity and frozen bars are:

| Class | Scope | Bar |
| --- | --- | ---: |
| C0 | local; no new ownership/cache/representation/allocation | 2.0% |
| C1 | local cache, precomputed relation, bounded subsystem ownership | 2.5% |
| C2 | cross-subsystem state, archive/scheduler/schema/complex lifetime | 3.0% |

## Archive-specific gate

`selectPlateauEntries` cannot be promoted by aggregate size alone. Any archive
candidate must record CPU breadth; operation breadth or its exact absence;
admission semantics; selected-set and selected-order equivalence feasibility;
signature diversity invariants; and downstream base-selection implications.
A 2/6 C2 mechanism remains concentrated even when its raw CPU is largest.

## Frozen decision rule

`PROMOTE` requires one inventoried candidate with broad CPU evidence, benefit
at or above its complexity bar, an isolable exclusive causal region,
acceptable semantic risk, and no stronger overlapping explanation.

`NEED MORE EVIDENCE` names the exact missing datum and instrumentation when a
hot parent lacks an isolable child, operation attribution is absent, sampling
is insufficient, or overlap blocks an independent estimate. Instrumentation
is not implemented in this PR.

`DECLINE` is required when no mechanism clears the full gate. Continuing to
optimize is not presumed.

## Raw freeze

Immediately after the final solver run:

1. enumerate raw payloads and record count/bytes;
2. hash each payload;
3. write and separately hash the manifest;
4. mark payloads and manifest read-only;
5. revalidate every hash;
6. record `post_freeze_solver_runs=0`.

After freeze, only pprof extraction and analyzer derivation are allowed. Any
new solver invocation invalidates the freeze and requires a new one.

Large reports, binaries, and pprof files remain in the external read-only
bundle. The compact committed bundle contains provenance, manifest, freeze
record, accounting, analysis summary, candidate/case tables, operation
summaries, combined CPU/caller/tree extracts, and heap extracts.

## Definition of done and PR boundary

R1I-F is complete only with the exact base and clean detached provenance;
precommitted protocol/analyzer; development-only materialization; semantic
smoke; 28+14 operation runs; combined and six per-case GSV1 profiles; combined
V4 control; all accounting identities; mechanical inventory; objective
breadth; overlap groups; exactly one final decision; read-only revalidated
raw bundle; `post_freeze_solver_runs=0`; and all repository gates passing.

Then open one PR to `main` titled:

```text
docs(profiling): recalibrate post-R1I-E hotspots
```

The PR states the frozen base/evidence head, corpus, all protected roles as
unmaterialized, zero solver source changes, and the decision. Do not merge,
enable auto-merge, move `main`, start R1I-G, implement a promoted mechanism,
run its A/B, or open a stacked PR. Stop for manual owner review.
