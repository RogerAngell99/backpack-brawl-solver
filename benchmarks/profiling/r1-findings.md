# R1 findings — post-H2a next-bottleneck selection

## Integrity and scope

- Measured source: `f94bb285c8e895696c76a1beacd4942b35a056d1`, in a clean
  detached clone. Both measured binaries report that revision and
  `vcs.modified=false`.
- The verified `general-search-v2` corpus was restricted to `gsv2-013` through
  `gsv2-026` with `workers=1`. No validation, public-holdout, or
  private-holdout case was materialized or measured.
- GSV1 operations cover 14 cases × {250k, 1M}; the V4 control covers 14 cases
  × 1M. CPU/heap uses the frozen normal-build six-case GSV1 1M slice, followed
  by one CPU profile per slice case because a credible hotspot emerged.
- Raw artifacts were recursively hashed and frozen before interpretation. The
  compact evidence and hashes are in [`r1-evidence/`](r1-evidence/README.md).

## Deterministic operation corroboration

At GSV1 1M, generic packing-seed feasibility performed 576,653 calls, scanned
5,401,599 remaining instances, and checked 564,015,914 placement options.
Those exact counters, including 314,584,815 legal placements and 317,079,262
feasibility-internal canonical calls, are identical in the V4 1M control. The
scan multiplicity is therefore shared structural work, not a GSV1-only route.

H2a changed the cost per canonical check. The same GSV1 1M pass records only
5,113,768 actual candidate-key constructions: 5,025,823 inside feasibility
and 87,945 in the candidate loop. Its 10,992,413 total placement-key calls
are the sum of those lazy constructions and same-item comparisons; see the
v2 accounting in [`operations-gsv1-summary.json`](r1-evidence/operations-gsv1-summary.json).

The operation evidence alone does not justify P2-global: it proves large scan
multiplicity, but not material scan-body CPU after H2a. Conversely, the
existing harness has no counters for outgoing-bound calls or its inner
source/target work, so it cannot yet establish a deterministic cause for that
new CPU hotspot.

## CPU, allocation, and scenario attribution

The aggregate normal-build CPU profile has 292.13 sampled CPU-seconds. The
following are non-summed parent/self regions or caller-attributed regions;
nested values are not added together.

| Region / hypothesis | Aggregate CPU evidence | Allocation evidence | Spread and result |
| --- | ---: | --- | --- |
| P2-global scan body | `packingFeasibility`: 5.20s, 1.78% | no leading allocation signal | Structural checks are huge, but its own CPU is too small. |
| H2b residual | `placementKey`: 15.93s, 5.45%; canonical order: 7.98s, 2.73% | 0.84 GB flat `placementKey` allocation space (0.40%; 2.52 GB cumulative) | Present, but no longer the leading addressable region. |
| Plateau/archive | `plateauArchive.observe`: 51.41s, 17.60%; child selector: 40.59s, 13.89% | selector: 110.23 GB flat, 52.09% allocation space | Material only in `018` and `024`; fails the 4/6 breadth signal. |
| Outgoing bound | `upperPriorityCounts`: 30.22s, 10.34% | 69.51M cumulative allocated objects (26.93%, overlapping) | Present in all six CPU profiles (6.09–37.08%). |

The archive signal is clear but concentrated: `plateauArchive.observe` is
30.92s/25.35% in `018` and 19.97s/21.42% in `024`, and was not sampled in
the other four profiles. Its direct source path is clone/signature, append,
then full `selectPlateauEntries`; that selector groups and repeatedly sorts
archive entries. The CPU and allocation views are strong evidence for a
scenario-local archive investigation, not evidence for a general optimization.

The broad signal is `(*outgoingBoundContext).upperPriorityCounts`. Its caller
is `shouldPrune`; each invocation rebuilds `placedByID`, scans
priority-source instances, then for placed sources scans target instances and
calls `sourceHitsTargetWithCatalog`, before looking up the geometric
potential. Its focused caller tree attributes 9.00s to `coveragePlacementKey`,
6.79s to `placementByInstanceID`, and 5.46s to
`sourceHitsTargetWithCatalog` within this one 30.22s parent region. The
relevant source/edge evidence is in [`cpu-targeted.txt`](r1-evidence/cpu-targeted.txt)
and [`cpu-callers.txt`](r1-evidence/cpu-callers.txt).

[`scenario-attribution.csv`](r1-evidence/scenario-attribution.csv) contains
the direct per-profile values. It shows `upperPriorityCounts` at 6.13s,
3.90s, 4.26s, 3.36s, 6.09s, and 5.60s, respectively, across the frozen slice.

## Candidate gate

`f × r` below is a CPU-sample heuristic, not a wall-clock forecast. `f` is a
non-overlapping addressable parent region and `r` is a plausible reduction.

| Candidate | `f` | Plausible `r` | Heuristic | Gate outcome |
| --- | ---: | ---: | ---: | --- |
| P2-global | 1.78% | even 100% | at most 1.78% | Below the structural-change bar. |
| H2b residual | 5.45% | about 50% | about 2.7% | Below the ~3% structural bar; residual key work is not a priority. |
| Plateau/archive | 17.60% | unknown | not credible yet | Fails generality (2/6) and needs workload attribution. |
| Outgoing bound | 10.34% | unknown | not eligible yet | CPU and 6/6 spread pass, but the required operation cause is absent. |

## Decision

**NEED R1I INSTRUMENTATION — outgoing-bound attribution.**

R1 does not promote P2-global, H2b, plateau/archive, rooted packing, or an
allocation/GC rewrite. The only broad, material, non-overlapping candidate is
the outgoing-bound parent; its current evidence has whole-program CPU and
scenario spread, but lacks the required deterministic mechanism and a
defensible reduction estimate.

R1I must be a separate instrumentation-only PR. It should add deterministic
counters for:

1. `shouldPrune` calls, early exits before `topN`, `upperPriorityCounts`
   invocations, and true-prune outcomes;
2. `placedByID` map builds and entries inserted;
3. priority/source iterations, split by free and placed sources;
4. placed source/target-pair checks and `sourceHitsTargetWithCatalog` calls;
5. geometric-potential lookups and `coveragePlacementKey` constructions.

R1I must not cache bounds, alter pruning, change ranking/scheduling, or make
an outgoing-bound optimization. After it merges, re-freeze the new measured
SHA, recollect the affected development-only evidence, and make a fresh
single-experiment decision. This keeps the causal-corrobation gate intact.
