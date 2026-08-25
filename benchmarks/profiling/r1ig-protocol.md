# R1I-G protocol — bounded outgoing placement index

## Frozen baseline and exact mechanism

R1I-G starts from the post-R1I-F `main` revision:

```text
4c6b443e3abee2cb63953f53134cc7fd8f04593b
```

Before branch creation, `origin/main` must equal that SHA and the measurement
worktree must be clean. If `origin/main` changes, inspect and explicitly freeze
the new delta; never reuse this protocol or collected evidence silently.

R1I-G tests exactly one mechanism:

> Inside the outgoing upper bound, replace repeated construction of the
> `map[string]model.Placement` lookup and its source/target string lookups with
> a bounded `OriginalIndex` placement index, while retaining the literal legacy
> map implementation whenever the static inventory or dynamic placements do
> not prove equivalence.

This is an execution-cost experiment. It must preserve the same upper vector,
prunes, nodes, solutions, ordering, and logical operation counters. It must not
improve, gate, or otherwise change the search.

## Four-commit discipline

The branch contains these logical commits in order:

```text
1. docs(profiling): freeze R1I-G bounded outgoing index protocol
2. perf(solver): index outgoing placements by OriginalIndex
3. test(solver): prove bounded outgoing index equivalence
4. docs(profiling): record R1I-G A/B evidence
```

Commit 3 is the measured candidate. No `.go` file may change after commit 3
and before or during official collection. Any correction invalidates the
candidate and every collected artifact; create a new candidate SHA and repeat
build, smoke, semantics, timing, profiling, and freeze from the beginning.

## Scope boundary

Production changes are limited to:

```text
internal/solver/outgoing_bound.go
internal/solver/bound_operation_profile_enabled.go
an optional private outgoing-index file
```

Corresponding tests, benchmarks, R1I-G scripts, compact evidence, and profiling
documentation are allowed. Archive, plateau, scheduler, beam, priority bound,
star compatibility, `coveragePlacementKey`, static-star caches, call gating,
node charging, budgets, objectives, ranking, catalog, generator, suite, and
Web/WASM behavior are out of scope.

## Frozen representation and static domain

The intended per-call index is value storage, not placement or pointer storage:

```go
type outgoingPlacementIndex struct {
    positionPlusOne [64]uint8
    presentMask     uint64
}
```

`0` means absent and `1..64` means `placements[position-1]`. The fast-path
builder must perform zero heap allocations. An immutable domain is attempted
once in `newOutgoingBoundContext`, conceptually containing:

```go
type outgoingPlacementIndexDomain struct {
    instanceIDByOriginal [64]string
    inventoryMask        uint64
}
```

The domain exists only when all of these hold:

```text
len(instances) <= 64
every OriginalIndex is in 0..63
OriginalIndex values are unique
every InstanceID is non-empty
InstanceID values are unique
```

A one-time string set used to validate the context is allowed. Any violation
sets the domain to nil, and every bound call uses the exact legacy map path.

## Dynamic validation and exact fallback

For every placement, the fast-path builder requires:

```text
OriginalIndex in 0..63
OriginalIndex present in the inventory mask
OriginalIndex not already present in this call
InstanceID exactly equal to instanceIDByOriginal[OriginalIndex]
```

Any violation abandons the index and calls the literal legacy implementation.
This covers duplicate indices, duplicate IDs, inconsistent ID/index pairs,
unknown placements, empty IDs, and out-of-range indices. `ItemID` is
deliberately not validated: the legacy map keys only by `InstanceID` and
returns the complete supplied placement.

The legacy helper preserves map `last-write-wins` exactly. The indexed path
does not approximate duplicate behavior; it refuses every dynamic duplicate
and delegates to legacy. No interface, closure, or function-valued lookup may
be introduced into the hot loop.

## Indexed upper-bound contract

On a valid domain, `presentMask` is the placed mask. Source and target lookup
use each inventory instance's `OriginalIndex`, `positionPlusOne`, and direct
slice access. The remaining traversal and conditions stay unchanged:

```text
priority traversal
source traversal
target traversal
InstanceID self-target check
sourceHitsTargetWithCatalog
coveragePlacementKey
potential lookup
popcount
star-count clamp
upper accumulation
```

`upperPriorityCountsLegacy` remains a real implementation and a test oracle.
The primary proof compares the complete legacy and indexed upper vectors on
the same state, not merely final search results.

## Searchprofile counter semantics

The searchprofile build uses the same bounded representation and fallback.
Existing public counters retain their historical logical-work meaning even
when the physical string map is absent. In particular:

```text
PlacedMapBuilds
PlacedMapInsertions
TargetPlacementLookups
```

still count their logical sites exactly. The schema is not renamed. The
profile-enabled implementation must preserve the complete
`OutgoingBoundSiteProfile`, including all map/mask, priority/source/target,
potential, geometry, popcount, and clamp counters.

## Differential test matrix

One table-driven differential suite covers at least:

1. empty inventory;
2. empty placements;
3. one instance;
4. indices 0 and 63;
5. negative index;
6. index 64;
7. a much larger index;
8. inventory larger than 64;
9. duplicate inventory `OriginalIndex`;
10. duplicate inventory `InstanceID`;
11. empty inventory `InstanceID`;
12. duplicate placement index;
13. duplicate placement `InstanceID`;
14. repeated ID with different placements proving last-write-wins;
15. placement ID absent from inventory;
16. valid placement index with the wrong ID;
17. missing source placement;
18. missing target placement;
19. reordered placements;
20. duplicate physical copies;
21. multiple rotations;
22. placement `ItemID` different from inventory;
23. all 64 indices occupied;
24. a sparse domain with gaps.

Unsafe cases prove both fast-path refusal and exact legacy result equality.
Valid cases compare legacy and indexed upper vectors directly.

Two fixed-seed generated corpora contain at least 1,000 states each. The valid
corpus varies inventory size, placed subset/order, rotations, source/target
state, priority items, and missing placements. The malformed corpus applies
controlled bad-index, duplicate-index, duplicate-ID, empty-ID, ID/index
mismatch, and unknown-placement mutations. The seed is recorded in tests.

A searchprofile-only differential runs legacy-profiled and indexed-profiled on
the same states and requires exact equality of the entire
`OutgoingBoundSiteProfile`:

```text
PlacedMapBuilds, PlacedMapInsertions, PlacedMaskInstanceChecks,
PriorityIterations, SourceInstanceIterations, PrioritySourceMatches,
ZeroStarSourceSkips, PlacedSourceIterations, FreeSourceIterations,
PlacedSourceTargetIterations, SelfTargetSkips, TargetPlacementLookups,
PlacedTargetsFound, UnplacedTargets, SourceHitsTargetCalls,
SourceHitsTargetTrue, CoveragePlacementKeyCalls, PlacedPotentialLookups,
FreePotentialLookups, PopcountCalls, StarCountClamps
```

## Blocking correctness gates

Before official collection:

```powershell
gofmt
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
```

Also require normal versus tagged-OFF semantic equality, suite-lock and catalog
verification, and the CI-equivalent production Web/WASM build. No catalog,
generator, suite manifest, or suite-lock diff is allowed.

## Microbenchmark gate

Benchmarks separately compare:

```text
legacy placementByInstanceID vs bounded index builder
legacy upperPriorityCounts vs indexed upperPriorityCounts
```

at `1,4,8,16,32,64` placements, reporting ns/op, B/op, and allocs/op. The
valid-domain indexed builder must report `0 B/op` and `0 allocs/op`; failure
blocks official A/B collection until the implementation is corrected.

## Clean detached builds and provenance

After commit 3, create clean detached BASE and CANDIDATE clones outside
OneDrive:

```text
BASE      4c6b443e3abee2cb63953f53134cc7fd8f04593b
CANDIDATE <commit-3 SHA>
```

Build base-normal, base-searchprofile, candidate-normal, and
candidate-searchprofile once with `-buildvcs=true`. Every binary must report
the expected `vcs.revision` and `vcs.modified=false`; record SHA-256, Go/OS/CPU
provenance, visible RAM, timezone, Git status, catalog hash, suite manifest
hash, suite lock hash, and generator hash. Official runs never use `go run`.

Verify and materialize only the fourteen `general-search-v2` development cases
`gsv2-013..026`. Record:

```text
validation_materialized=false
public_holdout_materialized=false
private_holdout_materialized=false
```

## Smoke and official semantic matrix

Smoke uses `gsv2-013`, GSV1, 250k, `workers=1`:

```text
BASE normal
CANDIDATE normal
CANDIDATE searchprofile OFF
BASE searchprofile ON
CANDIDATE searchprofile ON
```

Remove only timing/provenance fields (and the profile payload only for ON/OFF
cross-mode checks). Require complete deterministic equality.

The official profiled baseline/candidate matrix is:

| Variant | Cases | Budgets | Comparisons |
| --- | --- | --- | ---: |
| GSV1 | `013..026` | `250000,1000000` | 28 |
| V4 | `013..026` | `1000000` | 14 |

All 42 comparisons must exactly preserve final solutions, score and all score
components, layout key/hash, nodes and charging, budgets/stops, scheduler,
beam, archive, phase outcomes, outgoing checks/prunes, and the entire logical
operation profile. One mismatch forces REVERT until explained and recollected
under a new candidate.

## Frozen timing protocol

Use normal binaries, GSV1, 1M, `workers=1`, `repeat=1`, cases
`013,015,016,018,021,024`, and seven pairs per case:

```text
AB BA AB BA AB BA AB
```

This is 84 solver runs. The analyzer validates scenario, variant, 1M budget,
one worker, one repeat, and build revision from each JSON. Per case report
baseline/candidate medians, Q1, Q3, IQR, seven paired ratios, median paired
ratio, and speedup. Aggregates report the median of all paired ratios, the sum
of six baseline medians, the sum of six candidate medians, and the
time-weighted/sum-median ratio. Equal-case and time-weighted results remain
distinct.

## Causal CPU and memory gates

Using normal binaries, collect combined baseline/candidate GSV1 six-case CPU,
combined baseline/candidate V4 CPU, and GSV1 heap profiles for `alloc_space`,
`alloc_objects`, and `inuse_space`.

R1I-F measured the baseline causal region as:

```text
placementByInstanceID construction  5.29 s
target map lookup                    3.72 s
                                   -------
target region                        9.01 s
whole CPU                          219.39 s
```

The baseline region is approximately 4.1068% of CPU. The frozen causal gate
is at least 61% reduction in the semantically corresponding region. Baseline
uses the `placementByInstanceID` caller edge plus target-lookup source line;
candidate uses the bounded-index builder line plus target-indexed-access line.
Do not include `sourceHitsTargetWithCatalog`, `coveragePlacementKey`, static
compatibility, or popcount.

The indexed builder must add no heap allocation. Combined candidate global
`alloc_space` and `alloc_objects` must each be no greater than baseline times
1.01; any change over 1% requires investigation. The outgoing per-call map
allocation must be substantially reduced.

## Frozen decision rule

R1I-G is **KEEP** only when all of these hold simultaneously:

```text
all correctness gates PASS
42/42 semantic A/B exact
logical profiles exact
nodes and prunes exact
aggregate median paired wall improvement >= 2%
at least 5/6 scenario medians non-regressing
no scenario median more than 2% slower
sum-of-six medians does not regress
target causal region reduction >= 61%
fast index builder introduces no heap allocation
global alloc_space and alloc_objects do not regress materially
```

Choose **NEED MORE EVIDENCE** only when semantics and causal removal pass but
timing remains inconclusive or noisy. Choose **REVERT** for any semantic,
node, prune, or logical-counter difference; material timing or memory
regression; or falsified causal hypothesis. Thresholds do not change after
results are seen.

## Artifact freeze and PR boundary

Immediately after the final solver run, hash every raw artifact, record file
count and total bytes, write and separately hash the manifest, mark the raw
bundle read-only, revalidate all hashes, and record
`post_freeze_solver_runs=0`. Only derived analysis is allowed afterward. A
post-freeze candidate correction invalidates the bundle and requires complete
recollection.

Before opening the PR, fetch and require `origin/main` still equals the frozen
baseline. If it advanced, do not rebase measured code or claim the evidence
still applies; stop and assess whether complete recollection is necessary.

Regardless of KEEP, NEED MORE EVIDENCE, or REVERT, commit the compact evidence,
push the branch, and open one PR to `main`. The PR records base, measured
candidate, evidence head, decision, 42/42 status, equal-case and weighted
timing, causal reduction, both allocation changes, and protected-corpus state.
For REVERT recommend DO NOT MERGE; for NEED MORE EVIDENCE recommend DO NOT
MERGE YET.

Do not merge, enable auto-merge, push to or move `main`, create a dependent
branch, start R1I-H, or perform a post-R1I-G reprofile. Stop after the PR for
manual owner review.
