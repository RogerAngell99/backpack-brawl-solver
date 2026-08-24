# R1I comparative bound-internal attribution protocol

## Status and frozen baseline

R1I-A is an instrumentation-only change based on:

```text
60e11811b1cd8c6767aa4ae8e4f948adb69a2845
```

The profile contract is frozen before implementation as:

```text
bound-attribution-ops-v1
```

The operation-profile summarizer produced by this change is:

```text
operation-profile-summary-v3
```

R1I-A must not add a cache, memoization, precomputed relations, call gating,
ranking changes, scheduler changes, beam changes, a more aggressive upper
vector, or any other bound/search-policy optimization. Official decision
evidence is collected only after this instrumentation has been reviewed,
merged, and rebuilt from a newly frozen clean `main` revision.

R1I-A also fixes a pre-existing omission of the authoritative outgoing checks
and prunes from the coverage-ceiling early-return projection. Search behavior
is unchanged, but those pre-existing telemetry fields may therefore differ
from historical output on that path. The correction is regression-tested in
the normal build and recorded explicitly rather than treated as new R1I work.

## Question

R1 found two broad, general post-H2a hotspots:

| Family | Whole-program CPU | Spread | Existing evidence |
| --- | ---: | ---: | --- |
| Priority upper bound | 8.14% | 6/6 | Cost is concentrated in target, option, and geometry work. |
| Outgoing bound | 10.34% | 6/6 | Aggregate prune rate was 61.58%; map, key, and target scans are material. |

R1I asks:

> Which internal mechanism produces the largest removable cost without
> changing bound semantics?

It does not ask which parent function has the largest pprof percentage or how
to make either bound more aggressive.

## Collection architecture

Counters are deterministic integers. Every search task, repair task, and
constellation/plateau preparation session owns a local collector. Profiles are
merged only after a result returns, using fixed structs and integer addition.
There is no shared collector, map of arbitrary site names, mutex, atomic, or
scheduling-dependent update order.

All profiled algorithm mirrors and counter updates are compiled only with the
`searchprofile` build tag. A normal build keeps the production algorithms and
has `searchOperationProfilingAvailable == false`, allowing profiling branches
to be removed at compile time.

## Profile schema

`BoundAttributionOperationProfile` has one version plus two independent
families: `PriorityUpper` and `Outgoing`.

Priority attribution uses four fixed physical sites:

```text
ConstellationFilter
RepairDFS
PlateauPrefilter
PlateauDFS
```

The priority wrapper also records:

```text
ConstellationFilterInvocations
ConstellationStatesInput
ConstellationStatesRetained
ConstellationStatesRejected
```

Each priority site records these exact counters:

```text
Calls
FeasibleResults
RejectedResults
InvalidPriorityReturns
PriorityEntriesValidated

FixedPlacementInputs
CurrentPlacementInputs
AnchoredPlacements
RemovedInstanceInputs
RemovedInstances

RemovedOptionCandidates
RemovedOptionRejectedFixedOverlap
RemovedOptionRejectedOutsideFree
RemovedOptionsRetained

UniquePrioritySourceItems
AnchoredSourceInstances
RemovedSourceInstances
StarSlots

FixedTargetChecks
RemovedTargetChecks
SelfTargetSkips

FixedFixedGeometryChecks
RemovedSourceOptionChecksFixedTarget
FixedSourceTargetOptionChecks
RemovedSourceTargetOptionPairs

GeometryCandidateChecks
GeometryOverlapRejects
StarPositionHitCalls
StarPositionHitTrue
SlotTargetHits

MatchingCalls
```

`PriorityEntriesValidated` counts entries accepted before the algorithm can
proceed. The invalid entry itself is not included. Input counters describe the
logical inputs and deduplicated results of the call-level preparation; an
internal helper's repeated construction of anchored placements is not counted
as a second logical input. The four geometry-regime counters classify every
candidate pair exclusively. `SlotTargetHits` counts successful slot/target
relations, not matching results.

Outgoing attribution uses two fixed physical sites:

```text
Search
Repair
```

Each outgoing site records:

```text
Checks
PrunedNodes

PlacedMapBuilds
PlacedMapInsertions
PlacedMaskInstanceChecks

PriorityIterations
SourceInstanceIterations
PrioritySourceMatches
ZeroStarSourceSkips

PlacedSourceIterations
FreeSourceIterations

PlacedSourceTargetIterations
SelfTargetSkips
TargetPlacementLookups
PlacedTargetsFound
UnplacedTargets

SourceHitsTargetCalls
SourceHitsTargetTrue

CoveragePlacementKeyCalls
PlacedPotentialLookups
FreePotentialLookups

PopcountCalls
StarCountClamps
```

`PlacedMapInsertions` counts insertion attempts, so it remains meaningful even
if malformed future input repeats an instance ID. `Checks` and `PrunedNodes`
are snapshots copied from the existing authoritative search/repair counters;
the profiler must not increment a duplicate top-level call/prune counter.

## Blocking accounting identities

These identities must hold per applicable site and in aggregate:

```text
Calls = FeasibleResults + RejectedResults

ConstellationStatesInput =
    ConstellationStatesRetained + ConstellationStatesRejected

RemovedOptionCandidates =
    RemovedOptionRejectedFixedOverlap +
    RemovedOptionRejectedOutsideFree +
    RemovedOptionsRetained

GeometryCandidateChecks =
    FixedFixedGeometryChecks +
    RemovedSourceOptionChecksFixedTarget +
    FixedSourceTargetOptionChecks +
    RemovedSourceTargetOptionPairs

GeometryCandidateChecks = GeometryOverlapRejects + StarPositionHitCalls
StarPositionHitTrue = SlotTargetHits

PrioritySourceMatches =
    ZeroStarSourceSkips + PlacedSourceIterations + FreeSourceIterations

PlacedSourceTargetIterations = SelfTargetSkips + TargetPlacementLookups
TargetPlacementLookups = PlacedTargetsFound + UnplacedTargets
SourceHitsTargetCalls = PlacedTargetsFound
CoveragePlacementKeyCalls = PlacedSourceIterations
PlacedPotentialLookups = PlacedSourceIterations
FreePotentialLookups = FreeSourceIterations
PopcountCalls = PlacedSourceIterations + FreeSourceIterations
```

For an execution aggregate, outgoing attribution must also satisfy:

```text
profile.Search.Checks + profile.Repair.Checks =
    SearchStats.OutgoingBoundChecks

profile.Search.PrunedNodes + profile.Repair.PrunedNodes =
    SearchStats.OutgoingBoundPrunedNodes
```

The ordinary repair priority site's `RejectedResults` must equal the ordinary
repair flow's existing `PriorityBoundPruned`. Plateau DFS is kept separate
because its repair target belongs to the plateau mechanism.

## Semantic contract

Normal build, `searchprofile` with operation profiling disabled, and
`searchprofile` with operation profiling enabled must agree on every semantic
or deterministic search field, including:

```text
Score and all tie-break fields
PriorityCounts
LayoutKey
CanonicalLayoutHash
NodesExplored and charged node counts
phase budgets and scheduler allocations
termination reasons
coverage, exact, outgoing, and priority prune outcomes
packing/root counters
candidate ordering, beam contents, MRV, and feasibility decisions
incumbent sequence after elapsed timing is ignored
```

Only the new operation profile may appear when profiling is enabled. A changed
prune outcome fails R1I-A even if the final solution is unchanged.

## Summarization contract

Raw benchmark JSON contains integers. Summary v3 may derive ratios but never
mixes elapsed time into operation attribution or recommends an optimization.

Priority derived metrics include rejection rate, anchored/removed/options per
call, option retention rate, sources and slots per call, fixed/removed target
checks per call, geometry candidates and hit calls per call, overlap reject
rate, and hit true rate.

Outgoing derived metrics include prune rate, map insertions per check, source
matches per check, placed-source fraction, targets per placed source, placed
target fraction, hit calls per check, coverage-key calls per check, and
potential lookups per check.

If a report contains more than one bound-attribution profile version, profiles
and derived ratios remain separated under `BoundAttributionByVersion`; they
must never be silently added into one aggregate.

## Validation gates

R1I-A requires:

1. Unit fixtures covering fixed/fixed, removed/fixed, fixed/removed,
   removed/removed, overlap, outside-free, self-target, early-success,
   zero-star, and invalid-priority paths.
2. Call-site tests proving constellation, ordinary repair, plateau prefilter,
   plateau DFS, outgoing search, and outgoing repair remain separate.
3. Aggregation tests for equal versions and rejection/separation of
   incompatible versions.
4. Early-return tests proving coverage-ceiling and priority-ceiling exits
   retain every bound counter and operation profile produced before the stop.
5. Three-way semantic equivalence is automated in two linked gates: CI emits
   the same timing/profile-normalized snapshot from the normal and
   searchprofile-OFF builds, while the tagged solver test compares
   searchprofile OFF and ON in-process. Only elapsed timing and operation
   profiles may be removed before comparison.
6. `gofmt`, `git diff --check`, `go test ./...`,
   `go test -tags searchprofile ./...`, solver race tests, the
   `general-search-v2` suite lock, and the same Web/WASM build performed by CI.
7. No diff to the catalog, suite generator, or
   `benchmarks/suites/general-search-v2.lock`.

## Post-merge collection (R1I-B input)

Official collection is out of scope for R1I-A. After merge, freeze the new
`main` SHA, build clean normal and searchprofile binaries with VCS metadata,
require `vcs.revision` to match and `vcs.modified=false`, and hash binaries and
artifacts.

The closed development corpus is `gsv2-013` through `gsv2-026`; validation and
holdouts remain unopened. The future matrix is:

| Run | Cases | Budget | Build |
| --- | --- | ---: | --- |
| GSV1 operation | 14 development cases | 250k | searchprofile |
| GSV1 operation | 14 development cases | 1M | searchprofile |
| V4 control | 14 development cases | 1M | searchprofile |
| CPU/heap | 013, 015, 016, 018, 021, 024 | 1M | normal |

An official `gsv2-013`, 250k, GSV1, `workers=1` smoke run must precede the full
matrix and abort on version, identity, build provenance, outgoing cross-check,
or ON/OFF semantic failure.

## Decision rule

R1I-B will combine whole-program CPU fraction `f` with a conservative estimate
of the plausibly removable fraction `r`:

```text
heuristic benefit = f * r
```

A structurally complex change should normally show roughly 3% or more
plausible whole-program benefit. R1I-B selects exactly one named internal
mechanism, requests more evidence, or declines promotion. Counter magnitude
alone is not a decision rule.

Outgoing currently prunes frequently, so the preferred future experiment
keeps the same calls, upper vectors, and prune outcomes while reducing cost per
call. Changing when the bound runs is a search-mechanism experiment and cannot
be treated as an efficiency-only follow-up.
