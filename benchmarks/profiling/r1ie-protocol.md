# R1I-E protocol — exact direct `placementKey` formatting

## Frozen baseline and single mechanism

R1I-E starts from the post-R1I-D `main` revision:

```text
91af7e6e6f6469e13ae792c7d199ebad92883ea1
```

It tests exactly the mechanism promoted by R1I-D:

> Replace the two hot `fmt.Fprintf` decimal-formatting sites in `placementKey`
> with direct byte writes while preserving the exact returned key bytes.

This experiment may reserve exact builder capacity and use a direct fast path
for bounded non-negative placement coordinates/rotations. Unsupported,
negative, or out-of-range integer values must retain an exact legacy fallback
or prove exact formatting for the full integer domain.

R1I-E must not add an outgoing cache, reuse `coveragePlacementKey`, change
placement-map representation, alter archive selection, change canonical copy
ranking, gate any call, or combine another optimization with the formatter.

## Four-commit discipline

The implementation branch contains exactly these logical commits:

```text
1. docs(profiling): freeze R1I-E placement-key protocol
2. perf(solver): write bounded placement keys directly
3. test(solver): prove direct placement-key equivalence
4. docs(profiling): record R1I-E A/B evidence
```

The official candidate revision is commit 3. Commit 4 is restricted to
`docs/` and `benchmarks/profiling/`; no `.go` file may change after the
candidate is frozen.

## Exact semantic contract

Candidate `placementKey` output must be byte-for-byte equal to the baseline
for every input. The contract includes:

```text
minimum width 3
zero padding
negative sign placement
values wider than 3 digits
rotation/row/column field order
cell order
empty cells
all separators and trailing semicolons
```

The optimization must preserve:

- solutions, scores, priority counts, layout keys, and canonical hashes;
- node counts/charging, budgets, phases, stops, and limited/refined flags;
- candidate order, beam contents, scheduler allocation, quotas, and archive
  results;
- coverage, exact, outgoing, and priority vectors, calls, prunes, and outcomes;
- every `packing-seed`, rooted-packing, and `bound-attribution-ops-v1`
  logical counter;
- normal versus searchprofile-OFF and searchprofile-OFF versus ON semantics.

Any changed deterministic field converts the work into a search-mechanism
experiment and fails R1I-E regardless of timing.

## Implementation boundary

The intended production change is local to `placementKey` plus a private
formatting helper. A fast path may write three decimal digits directly for
values in `[0,999]`; all other integers use an exact compatibility path. The
function may call `strings.Builder.Grow` for the exact valid-domain key size.

No caller receives a new parameter or cache. `coveragePlacementKey`,
canonical placement keys, constellation keys, packing keys, outgoing bounds,
and operation-profile mirrors remain unchanged and benefit only because they
already call `placementKey`.

## Blocking tests before measurement

Add separate tests for the candidate helper and the whole key:

1. exhaustive valid fast-path integers `0..999` against `fmt.Sprintf("%03d")`;
2. explicit negative, boundary, and wide integers, including minimum/maximum
   `int`, against the legacy formatter;
3. empty placements and deterministic fixtures with reordered/duplicate
   cells, rotations, origins, and coordinates;
4. deterministic generated placements comparing legacy and candidate key
   bytes;
5. ordering equivalence for a generated placement population;
6. `coveragePlacementKey` equality when the underlying placement key uses the
   candidate;
7. searchprofile logical equivalence through the existing cross-build and
   in-process profile gates.

The legacy formatter remains test-only as the oracle. Production fallback is
allowed only for values outside the fast-path domain.

## Repository gates

Before official A/B collection, run:

```powershell
gofmt
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
```

Verify `general-search-v2` against its catalog and lock. Run the same Web/WASM
production build as CI. Normal and searchprofile-OFF semantic snapshots must
match. No catalog, generator, suite manifest, or suite lock diff is allowed.

## Microbenchmarks

The candidate test commit provides these focused benchmarks:

```text
BenchmarkPlacementKeyLegacy
BenchmarkPlacementKeyCandidate
BenchmarkPlacementKey
BenchmarkCoveragePlacementKey
```

Run with `-benchmem -count=10`. Record ns/op, B/op, and allocs/op. A
microbenchmark is diagnostic and cannot rescue a failed whole-program gate.

## Clean official A/B

After commit 3, create clean detached baseline and candidate clones outside
OneDrive. Build normal and searchprofile binaries once for each revision with
`-buildvcs=true`. For all four binaries require the expected `vcs.revision`
and `vcs.modified=false`, then hash them.

Record Go, OS/build, CPU, visible RAM, Git status, binary metadata/hashes,
catalog hash, suite-manifest hash, suite-lock hash, and the exact baseline and
candidate revisions. No official measurement uses `go run`.

Verify the suite and materialize only development `gsv2-013..026`. Validation,
public holdout, and private holdout remain closed.

## Smoke and semantic matrix

The first smoke is GSV1 `gsv2-013`, 250k, `workers=1`:

```text
baseline normal
candidate normal
candidate searchprofile OFF
baseline searchprofile ON
candidate searchprofile ON
```

After removing only timing/build provenance and, for ON/OFF comparisons, the
operation-profile payload, every deterministic field must match. Profiled
baseline and candidate reports must retain exact logical counter equality.

The full tagged A/B matrix is:

| Variant | Cases | Budgets | Comparisons |
| --- | --- | --- | ---: |
| GSV1 | `013..026` | `250000,1000000` | 28 |
| V4 | `013..026` | `1000000` | 14 |

All 42 baseline/candidate comparisons must be exact after timing-only
normalization. Logical profiles remain in the comparison.

## Whole-program timing

Use normal binaries, GSV1, 1M nodes, `workers=1`, and cases
`013,015,016,018,021,024`. Run seven pairs per case in alternating order:

```text
AB BA AB BA AB BA AB
```

Report baseline and candidate median, Q1, Q3, IQR, median ratio, all paired
ratios, per-case median paired ratio, and aggregate median paired ratio.

## Causal CPU/heap and allocation evidence

Collect combined baseline and candidate CPU/heap profiles on the timing slice
with the normal binaries. Compare source/caller edges, not global runtime
primitives. The target is the sum of the two `placementKey` formatting call
sites that measured 15.25 sampled seconds in R1I-D.

Candidate evidence must show at least 50% removal of that edge. Record
`placementKey` cumulative CPU, its formatter child edge, `coveragePlacementKey`
caller context, combined `alloc_space`, `alloc_objects`, and hot-path B/op and
allocs/op. The candidate must not introduce a new allocation per call or a
material whole-profile `alloc_space` regression.

## Frozen decision rule

R1I-E is **KEEP** only if:

1. every unit, tagged, race, suite, Web/WASM, and cross-build gate passes;
2. all 42 semantic comparisons and all logical profiles are exact;
3. aggregate median paired wall improvement is at least 2%;
4. at least five of six timing cases do not regress;
5. no timing case regresses more than 2%;
6. the two targeted formatting edges fall by at least 50%;
7. whole-profile `alloc_space` does not regress materially;
8. hot-path allocs/op do not increase.

If wall improvement is between 0% and 2% while the causal and all correctness
gates pass, decide **NEED MORE EVIDENCE** and only increase pair count. If the
candidate is slower, changes semantics, misses the causal edge, or regresses
memory materially, decide **REVERT**.

## Artifact freeze

After the last official solver run, hash raw artifacts, record count/bytes,
mark them read-only, hash the manifest separately, and set
`post_freeze_solver_runs=0`. Only derivation from frozen artifacts is allowed
afterward. The evidence commit includes compact provenance, semantic/timing
summaries, causal CPU/heap extracts, microbench output, hashes, and exactly one
KEEP / NEED MORE EVIDENCE / REVERT decision.
