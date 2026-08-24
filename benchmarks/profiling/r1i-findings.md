# R1I-B findings — bound-internal mechanism selection

## Decision summary

**PROMOTE: precompute the static source-item/star-slot to target-item
compatibility used by `starPositionHitsTarget` inside the priority upper
bound.**

This is one internal cost-per-call mechanism. It does not change when the
bound runs, its upper vector, matching, pruning, ranking, scheduling, beam
contents, or budgets. Implementation belongs in a separate PR.

The conservative CPU-sample heuristic is:

```text
priority parent f = 24.44 / 289.08 = 8.4544%
static StarMatchesCatalogItems caller edge q = 17.88 / 24.44 = 73.1588%
conservative removable fraction e = 60%
r = q * e = 43.8953%
f * r = 3.7111% of whole-program CPU samples
```

This is not a wall-clock forecast. It combines a measured non-overlapping
caller edge with a deliberately conservative estimate of what a bounded
boolean lookup can remove.

## Integrity and scope

- Measured revision:
  `ba1bc16d9b3ea904746f7833caa63579731c9c47`, clean and detached.
- Both binaries report the measured revision and `vcs.modified=false`.
- Only `general-search-v2` development cases `gsv2-013` through `gsv2-026`
  were materialized. Validation and both holdout groups remained closed.
- The mandatory `gsv2-013`/250k/GSV1 OFF×ON smoke passed after removing only
  timing/timestamp fields, operation-profiling markers, and operation-profile
  payloads. No semantic or deterministic search field differed.
- All priority/outgoing profile identities passed in the 42-run operation
  matrix. Search+repair outgoing checks/prunes reconciled with the
  authoritative fields.
- GSV1 has one absent profile at `gsv2-017`/250k. That run performed zero
  attributed work and has zero authoritative outgoing checks/prunes; it is
  not lost accounting.
- CPU/heap used the normal binary on the frozen six-case GSV1 1M slice.
- The 26 raw files were hashed and marked read-only before review derivation.
  The solver was not run after the freeze.

Full provenance and the raw artifact manifest are in
[`r1i-evidence/`](r1i-evidence/README.md).

## Deterministic operation evidence

### Priority upper bound

At GSV1 1M, all four priority sites together recorded:

| Counter | Value |
| --- | ---: |
| Calls | 234,300 |
| Rejected results | 129,413 (55.2339%) |
| Removed option candidates | 72,579,678 |
| Removed options retained | 51,083,725 (70.3830%) |
| Geometry candidates | 569,592,092 |
| Geometry overlap rejects | 1,756,773 (0.3084%) |
| `starPositionHitsTarget` calls | 567,835,319 |
| `starPositionHitsTarget` true | 3,349,629 (0.5899%) |
| Matching calls | 612,353 |

The four exclusive geometry regimes are decisive:

| Regime | Checks | Share | Breadth at GSV1 1M |
| --- | ---: | ---: | ---: |
| Fixed source × fixed target | 32,240,226 | 5.6602% | 14/14 |
| Removed-source option × fixed target | 453,007 | 0.0795% | 2/14 |
| Fixed source × removed-target option | 506,804,547 | 88.9768% | 14/14 |
| Removed-source option × removed-target option | 30,094,312 | 5.2835% | 2/14 |

The previously plausible removed×removed precomputation is therefore not the
general mechanism. It appears only in `gsv2-019` and `gsv2-026`. The broad
mechanism is repeated testing of one concrete fixed source against removed
target options.

The V4 1M control has exactly the same 234,300 priority calls, 569,592,092
geometry checks, 506,804,547 fixed-source/removed-target checks,
567,835,319 hit calls, 3,349,629 true results, and 612,353 matching calls.
The signal is not a GSV1-only counter trajectory.

### Outgoing bound

At GSV1 1M, search+repair recorded:

| Counter | Value / derived ratio |
| --- | ---: |
| Checks | 10,190,290 |
| Pruned nodes | 6,275,226 (61.5804%) |
| Placement-map builds | 10,190,290 |
| Placement-map insertions | 139,985,795 (13.7372/check) |
| Priority source matches | 30,142,272 |
| Placed-source iterations | 27,072,721 |
| Free-source iterations | 3,069,551 |
| Placed-source target iterations | 498,858,546 (18.4266/placed source) |
| Target placement lookups | 471,785,825 |
| Placed targets found | 344,287,241 |
| Unplaced targets | 127,498,584 |
| `sourceHitsTargetWithCatalog` calls | 344,287,241 |
| `sourceHitsTargetWithCatalog` true | 48,710,268 (14.1481%) |
| `coveragePlacementKey` calls | 27,072,721 (2.6567/check) |
| Popcount calls | 30,142,272 |

Every predefined outgoing mechanism is present in 14/14 cases. The V4
control differs by only 215 checks (0.00211%), 76 prunes (0.00121%), 351 key
calls (0.00130%), and similarly small target/map deltas. These counters prove
multiplicity and breadth, but the candidate ranking below uses pprof for cost.

Per-case and per-site rows are in
[`case-attribution.csv`](r1i-evidence/case-attribution.csv); exact GSV1/V4
deltas are in
[`control-comparison.csv`](r1i-evidence/control-comparison.csv).

## CPU and allocation evidence

The aggregate normal-build profile contains 289.08 sampled CPU-seconds over
153.80 seconds. CPU percentages below are nested caller/subregion evidence;
they are not added together.

### Priority

| Region | CPU samples | Whole program |
| --- | ---: | ---: |
| `filterConstellationPriorityFeasibleStates` | 24.44s cumulative | 8.45% |
| `partialRepairV3PriorityUpperBound` | 24.35s cumulative | 8.42% |
| `partialRepairSlotCanHitRemovedTarget` | 21.76s cumulative | 7.53% |
| Fixed-source/removed-target loop condition, source line 274 | 20.02s cumulative | 6.93% |
| `starPositionHitsTarget` | 19.12s cumulative | 6.61% |
| `StarMatchesCatalogItems` caller edge from `starPositionHitsTarget` | 17.88s | 6.19% |
| `filteredRemovedOptions` | 2.12s cumulative | 0.73% |
| `partialRepairMaximumSlotMatching` caller edge | 0.05s | 0.017% |

The source annotation attributes 20.02s to the fixed-source branch of
`partialRepairSlotCanHitRemovedTarget`. Inside `starPositionHitsTarget`, the
catalog predicate receives 17.88s of the 19.12s cumulative cost. It accepts
only the catalog, source item ID, target item ID, and the star definition;
the result is static with respect to placement geometry.

This is the causal bridge that operation counts alone could not provide:
506.8 million broad fixed-source/removed-target checks repeatedly evaluate a
static item/star compatibility predicate. Geometry still needs its bounds and
target-mask checks, but the expensive catalog predicate need not be recomputed.

Allocation corroboration does not change the CPU conclusion.
`filteredRemovedOptions` allocates 6,657.26 MB flat (3.16% of allocation
space) and about 2.73 million objects, but its CPU ceiling remains 0.73% of
the whole program. Matching is the negative control: 612,353 calls but only
0.05 sampled CPU-seconds.

### Outgoing

| Region inside `upperPriorityCounts` | CPU samples | Whole program |
| --- | ---: | ---: |
| Parent `upperPriorityCounts` | 30.31s cumulative | 10.48% |
| `coveragePlacementKey` caller edge | 9.00s | 3.11% |
| `placementByInstanceID` caller edge | 6.31s | 2.18% |
| `sourceHitsTargetWithCatalog` caller edge | 6.06s | 2.10% |

`placementByInstanceID` has a strong allocation signal: 17,493.80 MB flat
(8.29% of allocation space) and about 12.18 million flat objects. The parent
still has only 6.31 CPU-seconds attributable to this child, so even an 80%
removal yields about 1.75% whole-program benefit.

The outgoing key candidate is the strongest rejected sibling. Its 9.00s
caller edge is broad and causal, but even an 85% removal yields 2.65%, below
the approximately 3% bar. It remains a valid later candidate; it is not the
R1I-B promotion.

The focused extracts are
[`cpu-targeted.txt`](r1i-evidence/cpu-targeted.txt),
[`cpu-callers.txt`](r1i-evidence/cpu-callers.txt),
[`priority-source.txt`](r1i-evidence/priority-source.txt), and
[`outgoing-source.txt`](r1i-evidence/outgoing-source.txt).

## Candidate scorecard

`f × r` is a CPU-sample heuristic. `f` is the parent fraction, `q` is the
addressable child fraction of that parent, `e` is the conservatively removable
fraction of the child, and `r = q × e`.

| Candidate | CPU subregion | Conservative `e` | `f × r` | Breadth | Result |
| --- | ---: | ---: | ---: | ---: | --- |
| Static star compatibility in priority | 17.88s | 60% | **3.71%** | 14/14 | **Promote** |
| Outgoing `coveragePlacementKey` | 9.00s | 85% | 2.65% | 14/14 | Below bar |
| Outgoing placement map rebuild | 6.31s | 80% | 1.75% | 14/14 | Below bar |
| Outgoing source→target scan | 6.06s | 50% | 1.05% | 14/14 | Below bar |
| `filteredRemovedOptions` | 2.12s | 80% | 0.59% | 14/14 | Below bar |
| Priority matching | 0.05s | 100% | 0.017% | 14/14 | Negative control |
| Outgoing lookup/popcount | not isolated | — | — | 14/14 | Fails CPU/causal isolation |
| Full placement-pair geometry relation | overlaps 20.02s line | — | — | 14/14 | Dominated by narrower child |

The full placement-pair relation is deliberately not scored independently:
17.88s of its 20.02s source line is the promoted static predicate. Adding or
comparing those nested regions as independent CPU would double count. A full
placement-pair cache also has a much larger memory and invalidation surface
than the selected item/star compatibility relation.

The machine-readable scorecard is
[`candidate-scorecard.csv`](r1i-evidence/candidate-scorecard.csv).

## Promoted mechanism contract

The separate implementation PR should precompute a bounded compatibility
relation equivalent to:

```text
(source item, star definition/slot, target item)
    -> StarMatchesCatalogItems result
```

A dense target-item bitset per source-item/star slot is one plausible design.
The exact representation is secondary to the semantic contract:

1. Build the relation from the active catalog; do not use a stale process-
   global cache.
2. Prove every table entry equals the existing
   `scoring.StarMatchesCatalogItems` result across all catalog source stars
   and target items.
3. Preserve invalid-index and self-target behavior in
   `starPositionHitsTarget`.
4. Preserve geometry bounds, target-mask checks, hit truth, matching, upper
   vectors, bound calls, and prune outcomes.
5. Keep `StarPositionHitCalls`, `StarPositionHitTrue`, and all R1I accounting
   meanings unchanged. Counters describe logical work, not whether the
   predicate came from a table.
6. Do not change scheduler, ranking, beam, budgets, or bound call frequency.
7. Run deterministic normal/searchprofile-OFF/searchprofile-ON equivalence
   and an A/B benchmark against this frozen R1I-B baseline.

The change is isolated and reversible: remove the relation and restore the
direct predicate call. It does not require a persisted format or external API
change.

## Final decision

**PROMOTE: precompute the static source-item/star-slot to target-item
compatibility used by `starPositionHitsTarget` inside the priority upper
bound.**

R1I-B makes no solver change. The implementation and its deterministic A/B
evidence must be a separate PR.
