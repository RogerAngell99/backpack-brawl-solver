# R1I-D protocol — post-R1I-C recalibration and mechanism selection

## Status, question, and frozen baseline

R1I-D is an evidence-only recalibration based on the post-R1I-C `main`
revision:

```text
6952a35ef62f84646a01a887310363450c833b83
```

Before collection, fetch `origin/main` and abort if it does not resolve to that
exact revision. If `main` has advanced, review the intervening diff and freeze
a new protocol rather than silently accepting a different baseline.

R1I-D asks one question:

> What is now the largest broad, removable, and semantically safe cost in the
> post-R1I-C solver?

The R1I-C candidate profile is prior evidence only. It cannot select the next
mechanism. R1I-D does not implement a cache, change a representation, alter a
bound, or optimize the solver.

## Frozen scope

Collection uses only:

- `general-search-v2` development cases `gsv2-013` through `gsv2-026`;
- the frozen CPU/heap slice `013,015,016,018,021,024`;
- `general-search-v1` as the principal seed variant and `v4` as control;
- `workers=1`, `repeat=1`, normal plateau/search settings, and no diagnostics;
- the normal binary for CPU/heap and the `searchprofile` binary only for
  deterministic operation counters.

Validation, public holdout, and private holdout roles remain unmaterialized.
R1I-D must not alter the algorithm, representation, pruning, scheduler, beam,
quotas, budgets, scoring, catalog, generator, suite manifest, or suite lock.

## Clean collection and provenance

Official collection runs in a clean detached clone outside OneDrive:

```powershell
git fetch origin main
$SHA = git rev-parse origin/main
if ($SHA -ne "6952a35ef62f84646a01a887310363450c833b83") {
  throw "R1I-D baseline changed; review and refreeze"
}

git clone <repo> C:\r1id-measure
git -C C:\r1id-measure checkout --detach $SHA
if (git -C C:\r1id-measure status --porcelain) {
  throw "R1I-D requires a clean detached worktree"
}
```

Build once with `-buildvcs=true`:

```powershell
$ArtifactDir = "C:\r1id-artifacts\$SHA"
go build -buildvcs=true -o "$ArtifactDir\binaries\solver.exe" `
  ./cmd/backpack-brawl-solver
go build -buildvcs=true -tags searchprofile `
  -o "$ArtifactDir\binaries\solver-searchprofile.exe" `
  ./cmd/backpack-brawl-solver
```

For both binaries, `go version -m` must contain the frozen `vcs.revision` and
`vcs.modified=false`. Record the Git SHA/status, Go version, OS/build, CPU,
visible RAM, binary metadata and SHA-256, plus SHA-256 for the catalog, suite
manifest, and suite lock. No official measurement uses `go run`.

Verify the suite before materialization. Materialize exactly `development`,
then independently verify that the resulting directory contains exactly the
fourteen expected case IDs. Provenance must state:

```text
validation_materialized=false
public_holdout_materialized=false
private_holdout_materialized=false
```

## Blocking three-way smoke

Before the matrix, run `gsv2-013`, GSV1, 250k, `workers=1` once with each of:

1. normal binary;
2. `searchprofile` binary with operation profiling off;
3. `searchprofile` binary with operation profiling on.

The three reports must match after removing only timing, build provenance when
needed, and operation-profile payloads. Score, `PriorityCounts`, `LayoutKey`,
canonical layout hash, nodes, budgets, stop reasons, prune counters/outcomes,
phase results, bound outcomes, packing/root counters, and all deterministic
search fields remain compared. Any difference aborts collection.

## Operation-profile matrix

Use the tagged binary with `--operation-profile`:

| Variant | Cases | Budgets |
| --- | --- | --- |
| GSV1 | `013..026` | `250000,1000000` |
| V4 | `013..026` | `1000000` |

The derivation script validates every `bound-attribution-ops-v1` priority and
outgoing identity, including reconciliation with authoritative outgoing
checks and prunes. These runs measure logical breadth and density, never time.

## Normal CPU and heap profiles

Collect a fresh combined GSV1 profile from the normal binary:

```text
cases=013,015,016,018,021,024
budget=1000000
variant=general-search-v1
workers=1
repeat=1
```

Write CPU, heap, and benchmark report artifacts simultaneously. Derive at
least:

```text
cpu-top.txt
cpu-top-cum.txt
cpu-callers.txt
cpu-tree.txt
heap-alloc-space.txt
heap-alloc-objects.txt
heap-inuse-space.txt
```

Collect six additional GSV1 CPU profiles, one per frozen case, with the same
budget and settings. These profiles establish CPU breadth and distinguish a
broad mechanism from a one-case spike. Finally collect one combined V4 CPU
profile on the six-case slice as a control. A candidate that disappears under
V4 is marked seed-specific in the scorecard.

## Mechanical candidate inventory

Before examining results, a solver-owned mechanism qualifies for the inventory
when at least one condition holds:

```text
isolable edge >= 1.0% of combined GSV1 CPU
isolable parent cumulative CPU >= 1.5%
top-20 flat solver-owned node
top-20 cumulative solver-owned node
alloc_space call site >= 1.0%
top-10 alloc_objects solver-owned call site
```

Runtime primitives such as `runtime.duffcopy`, maps, hashing, or `memmove` are
not mechanisms. Attribute them to the solver-owned caller that causes the
work. Every qualifying entry is either mapped to a concrete candidate or
given an explicit exclusion rationale in the frozen review input consumed by
`r1id-analysis.mjs`.

Known families are carry-forward comparison points, not winners:

- outgoing static star compatibility;
- outgoing placement index/map construction;
- outgoing `coveragePlacementKey` construction;
- outgoing source-to-target scanning;
- residual `filteredRemovedOptions` work;
- residual priority geometry;
- every new mechanism admitted by the mechanical CPU/heap rules.

Do not use `outgoing` or a pprof primitive as a candidate name. The candidate
must name an addressable code mechanism.

## Scorecard and double-counting rules

The review input records, for every candidate:

```text
family and exact mechanism
parent CPU and targeted exclusive edge CPU
plausible removal fraction E
combined CPU qualification and six per-case target samples
V4 control target CPU when isolable
operation counter basis
alloc_space and alloc_objects attribution
complexity class and semantic risk
parent, child, exclusive subregion, and overlap group
evidence quality, disposition, and rationale
```

The script derives:

```text
F = parent CPU / total CPU
Q = target edge / parent CPU
Benefit = F * Q * E
        = (target edge / total CPU) * E
```

Counter volume is never substituted for CPU. Memory evidence remains memory
evidence. Parent and child regions in the same overlap group cannot be added
as independent whole-program benefits. Every experiment may claim only its
exclusive targeted region.

Complexity classes and normal promotion bars are:

| Class | Meaning | Plausible whole-program bar |
| --- | --- | ---: |
| C0 | trivial/local substitution or reuse | about 2% |
| C1 | local cache or internal representation change | 2–3% |
| C2 | ownership/schema/gating or multi-subsystem change | at least 3% |

## Frozen R1I-D decision

R1I-D ends with exactly one of:

```text
PROMOTE: <one exact mechanism>

NEED MORE EVIDENCE:
<missing datum and the instrumentation that would resolve it>

DECLINE:
no mechanism justifies another optimization now
```

If a large parent has no isolable child/caller, select `NEED MORE EVIDENCE`
and specify an instrumentation-only follow-up. Do not implement from an
unattributed pprof parent.

## Artifact freeze and committed bundle

After the final solver run:

1. hash all raw artifacts;
2. record raw file count and byte total;
3. mark raw files read-only;
4. hash the manifest separately;
5. set `post_freeze_solver_runs=0`;
6. perform only derivation from existing files.

Large binaries, raw reports, and `.pprof` files remain in the external
read-only artifact directory. The documentation-only evidence commit contains:

```text
benchmarks/profiling/r1id-protocol.md
benchmarks/profiling/r1id-analysis.mjs
benchmarks/profiling/r1id-findings.md
benchmarks/profiling/r1id-evidence/README.md
benchmarks/profiling/r1id-evidence/provenance.txt
benchmarks/profiling/r1id-evidence/SHA256SUMS.txt
benchmarks/profiling/r1id-evidence/accounting-validation.txt
benchmarks/profiling/r1id-evidence/analysis-summary.json
benchmarks/profiling/r1id-evidence/candidate-scorecard.csv
benchmarks/profiling/r1id-evidence/case-attribution.csv
benchmarks/profiling/r1id-evidence/cpu-top.txt
benchmarks/profiling/r1id-evidence/cpu-top-cum.txt
benchmarks/profiling/r1id-evidence/cpu-callers.txt
benchmarks/profiling/r1id-evidence/heap-alloc-space.txt
benchmarks/profiling/r1id-evidence/heap-alloc-objects.txt
benchmarks/profiling/r1id-evidence/heap-inuse-space.txt
benchmarks/profiling/r1id-evidence/operations-gsv1-summary.json
benchmarks/profiling/r1id-evidence/operations-v4-summary.json
```

There is no solver commit on the R1I-D branch.

## Conditional R1I-E boundary

R1I-E starts only after an exact R1I-D `PROMOTE`. It freezes the then-current
post-R1I-D `main` revision and tests exactly the promoted mechanism. Its branch
uses four commits: frozen protocol, implementation, tests, and documentation-
only evidence. The candidate SHA is the tests commit; no `.go` file changes
after measurement.

R1I-E preserves solutions, scores, priority counts, layouts, hashes, nodes,
node charging, budgets, stops, candidate ordering, beam/scheduler/quotas,
prune outcomes and vectors, plus logical bound call counts. Changing bound
frequency is a separate search-mechanism experiment.

The later A/B gate consists of 42 semantic comparisons, seven alternating
AB/BA timing pairs on each frozen CPU case, causal CPU/heap confirmation,
targeted microbenchmarks, memory checks, and all repository/race/Web/WASM
gates. KEEP requires at least 2% aggregate paired improvement, at least five
of six non-regressing cases, no case over 2% slower, at least 50% reduction in
the selected causal edge, no material `alloc_space` regression, and no new
allocation per hot-path call. Until R1I-D promotes a mechanism, none of this
implementation work is authorized.
