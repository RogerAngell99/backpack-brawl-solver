# R1I-H protocol — post-R1I-G efficiency recalibration

## Frozen question and baseline

R1I-H is an evidence-only selection gate inside macro-stage 12/18. It answers
exactly:

> After R1I-G, is there still a broad, causally isolable, semantically safe
> efficiency mechanism with enough ROI to justify another implementation?

The frozen solver revision is:

```text
0cac463a79238ecaea9d95af33468cc04dd5809b
```

Before work starts, fetch `origin/main` and require `HEAD == origin/main ==` the
frozen revision in a clean checkout. If the owner's current checkout contains
unrelated work, preserve it and use a new clean clone outside OneDrive. If
`origin/main` changes before collection, stop, audit the delta, and explicitly
freeze a new revision; never reuse this SHA silently.

R1I-H ends in exactly one of:

```text
PROMOTE: <one exact mechanism>

NEED MORE EVIDENCE: <the exact missing profile/counter, instrumentation site,
and hypothesis it would resolve>

DECLINE: no remaining efficiency mechanism justifies another optimization
```

It never implements an optimization. `PROMOTE` may name exactly one mechanism.
`DECLINE` closes macro-stage 12 and makes macro-stage 13, Efficacy experiments,
the next owner-authorized phase.

## Evidence-only boundary

Production `.go` files must have zero changes. R1I-H may add or edit only:

```text
benchmarks/profiling/r1ih-protocol.md
benchmarks/profiling/r1ih-collect.ps1
benchmarks/profiling/r1ih-profile-extract.ps1
benchmarks/profiling/r1ih-analysis.mjs
benchmarks/profiling/r1ih-freeze.ps1
benchmarks/profiling/r1ih-findings.md
benchmarks/profiling/r1ih-evidence/*
benchmarks/profiling/README.md
```

It must not alter the solver, objective, bounds, archive, scheduler, beam,
ranking, node charging, budgets, catalog, generators, suites, locks, or
production Web/WASM behavior.

## Tooling preflight and commit boundary

The intended history is exactly two commits:

```text
1. docs(profiling): freeze R1I-H post-R1I-G selection protocol
2. docs(profiling): record R1I-H evidence and efficiency decision
```

Before the first official solver run, write the protocol, collector, extractor,
analyzer, and freeze tool. Run:

```powershell
./benchmarks/profiling/r1ih-collect.ps1 -Mode Preflight ...
```

Preflight validates PowerShell and Node syntax, required paths, the exact
generator locations, Go and pprof availability, repository tests and semantic
snapshots, clean detached revision metadata, binary builds and VCS metadata,
catalog and both suite locks, CLI help, and the production Web/WASM build. It
may compile and run unit tests, but the `Preflight` control path returns before
the collector's first `benchmark-scenarios` command. Its record must state:

```text
status=PASS
benchmark_scenarios_runs=0
```

It also records SHA-256 for the five tooling files. Official mode rejects the
record unless those hashes still match the files about to execute. Fix any
preflight failure, discard its output, and rerun from a new clean preflight
directory. Only a passing preflight may be followed by commit 1.

After commit 1, collection and analysis criteria are immutable. If a tooling
bug is found after official collection starts: stop, invalidate the entire
partial bundle, fix the tool, make a new protocol/tooling commit, and recollect
from zero.

## Clean official builds and provenance

Official measurement uses a clean detached clone outside OneDrive at the
frozen SHA. On Windows, create it with `core.autocrlf=false` before checkout so
`gofmt -l .` examines the tracked LF content rather than a line-ending-converted
worktree. A separate clean detached clone with the same checkout rule is used
for the production Web/WASM gate. Build exactly once from the measurement clone:

```powershell
go build -buildvcs=true -o <artifacts>/binaries/r1ih-normal.exe ./cmd/backpack-brawl-solver
go build -buildvcs=true -tags searchprofile -o <artifacts>/binaries/r1ih-searchprofile.exe ./cmd/backpack-brawl-solver
```

Both binaries must report `vcs.revision` equal to the frozen SHA and
`vcs.modified=false`. No official run uses `go run`.

Provenance records binary SHA-256, Go version, GOOS/GOARCH, CPU, logical CPU
count, visible RAM, Windows caption/version/build, timezone, and clean Git
status. It records both Git blob IDs and checkout SHA-256 for the catalog, GSV1
and GSV2 manifests/locks, generator registry, and generator v2 source. It also
records the commit blob and executed SHA-256 for:

```text
protocol_git_blob / protocol_sha256
collector_git_blob / collector_sha256
extractor_git_blob / extractor_sha256
analysis_git_blob / analysis_sha256
freeze_script_git_blob / freeze_script_sha256
```

## Blocking gates

Official mode repeats all blocking gates before profiling:

```text
gofmt -l .                       (must print nothing)
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
normal versus searchprofile-OFF semantic snapshot
catalog validation
GSV1 suite-lock verification
GSV2 suite-lock verification
production Web/WASM npm ci and build
```

Any failure aborts collection.

## Closed corpus

Only GSV2 development cases `gsv2-013..026` are selectable. Verify the GSV2
suite and materialize only:

```text
--roles development
```

The collector independently enumerates the output directory and requires
exactly fourteen files named `gsv2-013.json` through `gsv2-026.json`. Any
additional or missing JSON aborts. Validation `027..036`, public holdout
`001..006`, and private holdout `007..012` remain unmaterialized and unrun.

All runs use `workers=1`, `repeat=1`, and `diagnostic=false`. The principal
variant is `general-search-v1`; V4 is a control. The fixed CPU slice is:

```text
gsv2-013,gsv2-015,gsv2-016,gsv2-018,gsv2-021,gsv2-024
```

## Semantic smoke

Run `gsv2-013`, GSV1, 250k, one worker, once with:

1. normal binary;
2. searchprofile binary with operation profiling OFF;
3. searchprofile binary with operation profiling ON.

The analyzer validates each JSON envelope and compares normalized reports after
removing only timing/provenance fields and, where applicable, operation-profile
markers and payloads. Solutions, nodes, prunes, ordering, budgets, and all
other deterministic fields must remain identical.

## Operation-profile matrix and immutable accounting

The official operation matrix is:

| Variant | Cases | Budgets | Runs |
| --- | --- | --- | ---: |
| GSV1 | `013..026` | `250000,1000000` | 28 |
| V4 | `013..026` | `1000000` | 14 |

The analyzer reads report content rather than filenames. It requires the
frozen build revision, exact scenarios, variant, budget, `workers=1`,
`repeat=1`, `diagnostic=false`, operation profiling state, and exact matrix
cardinality.

For every present `bound-attribution-ops-v1` profile it preserves:

```text
Priority calls = feasible results + rejected results
Constellation states input = retained + rejected
Removed option candidates = fixed-overlap rejects + outside-free rejects + retained
Geometry candidates = fixed/fixed + removed/fixed + fixed/removed + removed/removed
Geometry candidates = overlap rejects + star-position-hit calls
Star-position-hit true = slot-target hits

Priority-source matches = zero-star skips + placed sources + free sources
Placed-source target iterations = self skips + target-placement lookups
Target-placement lookups = placed targets found + unplaced targets
Source-hits-target calls = placed targets found
Coverage-placement-key calls = placed-source iterations
Placed-potential lookups = placed-source iterations
Free-potential lookups = free-source iterations
Popcount calls = placed-source iterations + free-source iterations
Search checks + Repair checks = authoritative outgoing checks
Search prunes + Repair prunes = authoritative outgoing prunes
```

A zero-work run may omit the operation payload only when the authoritative
checks and prunes are both zero. No counter or schema changes in R1I-H.

## CPU, per-case attribution, V4, and heap

Use the normal binary at GSV1, 1M, one worker on the six-case slice for one
combined CPU profile and one heap profile. The heap is read with sample indexes
`alloc_space`, `alloc_objects`, and `inuse_space`. Collect six independent
GSV1 CPU profiles with identical settings, then one combined V4 CPU profile on
the same slice. The combined GSV1 profile is the official whole-program
ranking; V4 classifies reproduction but cannot select a winner alone.

The heap question is “what allocates now?” Old allocation percentages are
historical context only.

## Mechanical extraction and inventory

After raw freeze, `r1ih-profile-extract.ps1` produces canonical JSON plus:

```text
cpu-top.tsv
cpu-top-cum.tsv
cpu-project-owned.tsv
cpu-hot-source-lines.tsv
heap-alloc-space.tsv
heap-alloc-objects.tsv
case-attribution.tsv
```

It also emits full text views for flat/cumulative CPU, project callers, callee
tree, annotated solver source, all three heap sample types, V4, and each case.
It extracts the entire project-owned solver namespace, not a candidate symbol
registry.

A mechanical inventory entry exists when any condition is true:

```text
project-owned flat CPU >= 1.0%
project-owned cumulative CPU >= 1.5%
top-20 project-owned flat CPU
top-20 project-owned cumulative CPU
hot project source line flat or cumulative CPU >= 1.0%
project-owned alloc_space >= 1.0%
top-10 project-owned alloc_objects
```

The extractor assigns a stable inventory key and all triggering conditions.
The post-freeze classification file must map every key to a candidate or to an
explicit exclusion with rationale. The analyzer requires an exact set match;
nothing can disappear. Runtime primitives such as allocation, hashing,
`memmove`, and GC are not mechanisms and must be explained through the
project-owned caller that causes them.

Carry-forward families are reviewed without priority from old scores:

```text
plateau/archive reselection
outgoing static compatibility
coveragePlacementKey residual
canonical-copy ranks
priority residual geometry
filteredRemovedOptions
target/source scans
placement-index residual
any new post-R1I-G hotspot
```

## Breadth, scorecard, overlap, and removal fraction

For each of the six independent profiles:

```text
case_fraction = target CPU / total case CPU
present       = target CPU > 0
material      = case_fraction >= 1%
broad         = present >= 5/6 AND material >= 4/6
concentrated  = material <= 2/6
ambiguous     = material == 3/6
```

Every other combination is `not-broad`. An ambiguous mechanism needs more
evidence before promotion. When an appropriate deterministic counter exists,
operation breadth is positive cases out of fourteen, with total, minimum,
median, maximum, and per-case values. CPU breadth and operation breadth remain
separate.

Each candidate records an exact mechanism and source/function region, family,
parent, exclusive target, overlap group, combined target CPU and whole-program
fraction, six case fractions, CPU breadth, operation metric and breadth,
allocation evidence, V4 reproduction, complexity, semantic risk, a plausible
removal fraction `E`, its structural basis, decision, and rationale.

Complexity bars are:

| Class | Scope | Bar |
| --- | --- | ---: |
| C0 | local; no state/cache/complex ownership/trajectory change | 2.0% |
| C1 | bounded local cache, private precompute/index/relation | 2.5% |
| C2 | archive/scheduler/cross-subsystem lifetime/order/representation | 3.0% |

The analyzer computes:

```text
RawFraction = TargetCPU / WholeCPU
E_min       = ClassBar / RawFraction
Benefit     = RawFraction * E
```

If `E_min > 100%`, rejection is automatic. `E` must be supported by a
microbenchmark, source-line decomposition, direct-index replacement,
allocation elimination, cached static relation, or measured parent/child
ownership. Otherwise `E=unknown`, benefit remains unknown, and promotion is
forbidden. `NEED MORE EVIDENCE` is available only when exact instrumentation
could resolve that objective gap.

Every candidate has an `overlap_group`. Parent/child regions are alternatives,
not additive benefits; only exclusive regions receive independent credit.

## Archive-specific rule

An archive/plateau candidate is not promoted by aggregate CPU alone. It must
record CPU breadth, operation breadth or its exact absence, selected-set and
selected-order invariants, signature diversity, downstream base selection,
admission semantics, and search-trajectory risk. A concentrated C2 archive
mechanism receives strong negative weight.

## Frozen decision rule

`PROMOTE` requires exactly one candidate that is mechanically eligible, broad,
causally isolated, has defensible `E`, reaches its class bar, has acceptable
semantic risk, and is not dominated by an overlapping candidate.

`NEED MORE EVIDENCE` requires a promising hotspot plus one exact objective gap.
It names the missing counter/profile, precise instrumentation site, and the
hypothesis resolved. R1I-H does not add the instrumentation.

`DECLINE` is required when no candidate clears all gates, including when no C0
can reach 2%, no C1 can reach 2.5%, and no acceptable broad C2 can reach 3%, or
when remaining cost is fragmented, concentrated, unavoidable runtime, or too
semantically expensive.

Historical R1I-D/F/H values are a compact secondary table only. Fresh R1I-H
profiles alone control the decision. R1I-H has no candidate and therefore no
paired timing matrix.

## Raw freeze and post-analysis revalidation

Immediately after the final official solver run:

1. enumerate all raw files and record count and bytes;
2. compute `RAW-SHA256SUMS.txt` and a separate manifest hash;
3. mark payloads and manifests read-only;
4. revalidate every hash and read-only flag;
5. record `post_freeze_solver_runs=0`.

No solver command may run afterward. Extraction and analysis are derived-only.
The analyzer re-hashes the complete raw manifest both before and after writing
derived outputs and requires:

```text
post_analysis_raw_hash_revalidation=PASS
```

Large binaries, reports, and pprof files remain in the external frozen bundle.
The committed evidence bundle is compact.

## Definition of done and PR stop

R1I-H is complete only with the frozen baseline, preflight PASS before commit
and collection, immutable tooling commit, clean detached binaries, all gates,
development-only corpus, semantic smoke, 42 operation runs, combined and six
per-case GSV1 CPU profiles, combined V4 control, three heap views, every
accounting identity, complete inventory, breadth, operation breadth, overlap,
`E` and `E_min`, scorecard, exactly one final decision, raw freeze PASS,
post-analysis revalidation PASS, and `post_freeze_solver_runs=0`.

Then open one PR to `main` titled:

```text
docs(profiling): recalibrate post-R1I-G hotspots
```

The PR states frozen base and evidence head, zero solver source changes, exact
corpus protection, operation/accounting status, and decision. Do not merge,
enable auto-merge, push `main`, begin R1I-I/R1I-H-A/macro-stage 13, or create a
stacked PR. Stop for manual owner review.
