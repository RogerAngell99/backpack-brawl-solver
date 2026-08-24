# R1 findings — completed post-H2a bound attribution

## Integrity and scope

- Measured source: `f94bb285c8e895696c76a1beacd4942b35a056d1`, in a clean
  detached clone. Both measured binaries report that revision and
  `vcs.modified=false`.
- The verified `general-search-v2` corpus was restricted to `gsv2-013` through
  `gsv2-026` with `workers=1`. No validation, public-holdout, or
  private-holdout case was materialized or measured.
- GSV1 operations cover 14 cases × {250k, 1M}; the V4 control covers 14 cases
  × 1M. CPU/heap uses the frozen normal-build six-case GSV1 1M slice.
- Review completion used only the already frozen raw JSON and aggregate/six
  individual CPU profiles. It did not rebuild binaries, rerun a benchmark, or
  change the solver.
- The compact evidence and hashes are in
  [`r1-evidence/`](r1-evidence/README.md).

## Deterministic operation corroboration

At GSV1 1M, generic packing-seed feasibility performed 576,653 calls, scanned
5,401,599 remaining instances, and checked 564,015,914 placement options.
Those packing-seed counters, including 314,584,815 legal placements and
317,079,262 feasibility-internal canonical calls, are identical in the V4 1M
control. The scan multiplicity is shared structural work, not a GSV1-only
route.

H2a changed the cost per canonical check. The same GSV1 1M pass records
5,113,768 actual candidate-key constructions: 5,025,823 inside feasibility
and 87,945 in the candidate loop. Its 10,992,413 total placement-key calls
are the sum of those lazy constructions and same-item comparisons; see the
v2 accounting in
[`operations-gsv1-summary.json`](r1-evidence/operations-gsv1-summary.json).

The outgoing bound already has deterministic top-level telemetry. Across the
fourteen GSV1 1M runs it performed 10,190,290
`outgoing_bound_checks`, pruning 6,275,226 nodes (61.5804%). The V4 1M
control is effectively the same: 10,190,505 checks and 6,275,302 prunes
(61.5799%). The normal-build six-case CPU slice records 4,447,244 checks and
2,964,148 prunes (66.6513%). Per-scenario/budget values are in
[`outgoing-bound-effectiveness.csv`](r1-evidence/outgoing-bound-effectiveness.csv).

At the search and repair call sites, `OutgoingBoundChecks` increments after
the non-nil/top-N guards and immediately before `shouldPrune`; the pruned
counter records its true result. It therefore already establishes effective
top-level bound invocation and pruning efficacy. It does **not** decompose the
internal map, source/target, geometry, key, or potential-lookup work.

No equivalent deterministic decomposition exists for the priority-upper-bound
family. Its CPU source establishes a concrete work chain, but counters are
needed to quantify its call multiplicity, state filtering, and inner
source/target/option work.

## CPU, allocation, and scenario attribution

The aggregate normal-build CPU profile has 292.13 sampled CPU-seconds. The
following are non-summed parent/self regions or caller-attributed regions;
nested values are not added together.

| Region / hypothesis | Aggregate CPU evidence | Allocation evidence | Spread and result |
| --- | ---: | --- | --- |
| P2-global scan body | `packingFeasibility`: 5.20s, 1.78% | no leading allocation signal | Structural checks are huge, but its own CPU is too small. |
| H2b residual | `placementKey`: 15.93s, 5.45%; canonical order: 7.98s, 2.73% | 0.84 GB flat `placementKey` allocation space (0.40%; 2.52 GB cumulative) | Partly nested in outgoing-bound work; do not treat it as independent. |
| Plateau/archive | `plateauArchive.observe`: 51.41s, 17.60%; child selector: 40.59s, 13.89% | selector: 110.23 GB flat, 52.09% allocation space | Material only in `018` and `024`; fails the 4/6 breadth signal. |
| Priority upper bound | `filterConstellationPriorityFeasibleStates`: 23.79s, 8.14%; child `partialRepairV3PriorityUpperBound`: 23.71s, 8.12% | `filteredRemovedOptions`: 6.58 GB flat, 3.11% allocation space | Sampled in all 6 slice cases; requires deterministic internal attribution. |
| Outgoing bound | `upperPriorityCounts`: 30.22s, 10.34% | 69.51M cumulative allocated objects (26.93%, overlapping) | Present in all 6 CPU profiles and already shown to prune frequently. |

The priority-upper parent and child are a single nested region, not 16.26%
combined. Its source attribution is concrete: the upper bound spends 21.54s
in `partialRepairStarUpperForItem`, 21.53s in
`partialRepairSourceStarUpper`, and 21.17s in
`partialRepairSlotCanHitRemovedTarget`; the latter spends 19.12s in
`starPositionHitsTarget` over removed target options. The matching stage is
only 0.07s cumulative. See
[`priority-bound-targeted.txt`](r1-evidence/priority-bound-targeted.txt) and
[`priority-bound-callers.txt`](r1-evidence/priority-bound-callers.txt).

The expanded
[`scenario-attribution.csv`](r1-evidence/scenario-attribution.csv) samples
the priority-upper parent at 2.17s, 1.61s, 1.69s, 1.15s, 6.60s, and 10.59s in
the frozen six-case order `013,015,016,018,021,024`. It is therefore broad
rather than a plateau-like two-case effect.

The outgoing-bound parent remains broad: `upperPriorityCounts` is 6.13s,
3.90s, 4.26s, 3.36s, 6.09s, and 5.60s across the same slice. Its focused
caller tree attributes 9.00s to `coveragePlacementKey`, 6.79s to
`placementByInstanceID`, and 5.46s to
`sourceHitsTargetWithCatalog` within its one 30.22s parent region. The
9.00s `coveragePlacementKey` child is approximately 3.08% of all CPU
samples and is nested within both the outgoing-bound and residual key
regions—those percentages must not be summed. See
[`cpu-targeted.txt`](r1-evidence/cpu-targeted.txt) and
[`cpu-callers.txt`](r1-evidence/cpu-callers.txt).

The archive signal remains clear but narrow:
`plateauArchive.observe` is 30.92s/25.35% in `018` and
19.97s/21.42% in `024`, and was not sampled in the other four profiles.
It is a scenario-local candidate, not evidence for a solver-wide change.

## Candidate gate

`f × r` below is a CPU-sample heuristic, not a wall-clock forecast. `f` is a
non-overlapping addressable parent region and `r` is a plausible reduction.

| Candidate | `f` | Plausible `r` | Heuristic | Gate outcome |
| --- | ---: | ---: | ---: | --- |
| P2-global | 1.78% | even 100% | at most 1.78% | Below the structural-change bar. |
| H2b global | 5.45% | unknown | not independently estimable | More than half of its sampled cost is in outgoing-bound `coveragePlacementKey`; a local H2-like intervention may emerge, but global H2b is not independently promotable. |
| Plateau/archive | 17.60% | unknown | not credible yet | Fails generality (2/6). |
| Priority upper bound | 8.14% | unknown | not eligible yet | CPU and 6/6 spread pass; its internal deterministic work/pruning data is absent. |
| Outgoing bound | 10.34% | unknown | not eligible yet | CPU, 6/6 spread, and 61.58% top-level prune rate pass; internal cost decomposition is absent. |

## Decision

**NEED R1I INSTRUMENTATION — comparative bound-internal attribution.**

R1 does not promote P2-global, global H2b, plateau/archive, rooted packing, or
an allocation/GC rewrite. Two sibling bound families are broad and material:
the priority upper bound (8.14%) and outgoing bound (10.34%). The current
evidence is sufficient to reject P2-global and a solver-wide plateau change,
but insufficient to choose one bound optimization or an H2-like local
intervention without internal deterministic attribution.

R1I is one instrumentation-only comparison PR with two separately reported
profiles:

1. **Priority-upper profile.** Count
   `filterConstellationPriorityFeasibleStates` input/retained/rejected
   states; `partialRepairV3PriorityUpperBound` calls split by constellation,
   repair, and plateau call site; anchored/current and removed instances;
   option candidates/retained options; source items and instances; star slots;
   fixed/removed target checks; source/target-option pairs;
   `starPositionHitsTarget` calls; and priority-vector rejections.
2. **Outgoing-internal profile.** Reuse, rather than duplicate,
   `OutgoingBoundChecks` and `OutgoingBoundPrunedNodes`. Add only the
   missing internal counters: `placedByID` map builds and entries; free versus
   placed source iterations; target iterations; `sourceHitsTargetWithCatalog`
   calls; `coveragePlacementKey` constructions; and geometric-potential
   lookups.

R1I must not cache bounds, alter pruning, change ranking/scheduling, or make
an optimization. After it merges, freeze the new measured SHA, recollect the
affected development-only evidence, compare the two bound profiles using the
same gate, and promote exactly one named optimization only if its causal
evidence and plausible benefit justify it.
