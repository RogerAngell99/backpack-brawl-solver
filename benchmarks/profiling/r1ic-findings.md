# R1I-C findings — static priority-star compatibility

## Decision summary

**R1I-C KEEP.** An immutable `[64][]uint64` priority-star compatibility
relation removes repeated static catalog evaluation from the priority upper
bound without changing any solver result or logical work counter.

```text
Semantic differences:             0
Logical R1I counter differences:  0
Priority prune differences:       0
Node-count differences:           0

Aggregate median paired ratio:    0.851044
Median paired wall improvement:   14.8956%
Non-regressing scenarios:         6/6
Worst scenario median result:     1.8549% faster

Priority static-predicate edge:
baseline:                         17.88 sampled seconds
candidate priority edge:          0 sampled seconds
measured removal:                 100%

Decision: KEEP static priority-star compatibility
```

## What changed

`priorityBoundContext` now owns an immutable execution-local relation mapping
source `OriginalIndex` and star slot to a target-index bitset. Construction
deduplicates source item work and rejects unsafe index domains by returning
`nil`. Missing or unsafe entries execute the unchanged legacy predicate.

The priority upper bound resolves compatibility once per source/star/target
instance and reuses it across placement options. It still traverses the same
options, performs the same overlap checks, executes the same geometry logic,
builds the same matching graph, and returns the same upper vector. The
diagnostic relaxed bound, structural ceiling, outgoing bound, evaluation,
catalog, suite, scheduler, and profile schema remain unchanged.

## Correctness evidence

The proof layer covers the complete production catalog relation, explicit
target-type/item and `CountsAs` filters, source exclusion, unknown rules, all
supported star-condition graph classes, compound ANY/ALL, duplicate items,
zero-star sources, invalid domains, missing entries, and rotation invariance.

Legacy/cached differential testing covers all four fixed/removed regimes,
overlap, outside-free filtering, self targets, early success, empty options,
rotations, duplicate sources and targets, and 500 generated partial states.
Under `searchprofile`, 209 states require structural equality of the entire
`PriorityUpperBoundSiteProfile`.

Repository gates passed in normal, tagged, normal race, tagged race, pinned
suite, normal-vs-tagged-off snapshot, and Web/WASM builds. The official smoke
and 42-run profiled matrix found no semantic or logical-counter difference.

## Whole-program timing

Normal binaries were built once from clean detached clones. Six GSV1 1M cases
ran in seven alternating pairs each with `workers=1`.

| Scenario | Baseline median | Candidate median | Candidate/base | Speedup | Base IQR | Candidate IQR |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `gsv2-013` | 12,359 ms | 10,582 ms | 0.8562 | 14.38% | 226 ms | 190 ms |
| `gsv2-015` | 9,433 ms | 8,221 ms | 0.8715 | 12.85% | 193 ms | 54 ms |
| `gsv2-016` | 10,462 ms | 9,160 ms | 0.8755 | 12.45% | 277 ms | 205 ms |
| `gsv2-018` | 54,504 ms | 53,493 ms | 0.9815 | 1.85% | 1,159 ms | 2,868 ms |
| `gsv2-021` | 19,964 ms | 14,650 ms | 0.7338 | 26.62% | 569 ms | 398 ms |
| `gsv2-024` | 52,774 ms | 44,195 ms | 0.8374 | 16.26% | 257 ms | 816 ms |

The median of all 42 paired candidate/base ratios is `0.851044`, or a
14.8956% improvement. All six scenario medians improve, and none approaches
the 2% regression limit.

## Causal profile

The baseline R1I-B caller tree attributed 17.88s to
`starPositionHitsTarget → StarMatchesCatalogItems` inside the priority bound.
The candidate caller tree has no priority caller edge into that predicate.
The only remaining `starPositionHitsTarget` sample is 0.02s from the unchanged
diagnostic star upper bound. Global predicate cost remains because outgoing was
deliberately excluded: 4.96s of the candidate's 5.00s global predicate total
comes from `sourceHitsTargetWithCatalog`.

The priority path now records 0.59s in `priorityGeometryCandidateHits`, 0.09s
in the geometry-only helper, and 0.01s in the relation lookup. The priority
parent drops from 24.35s to 5.37s cumulative, while the dominant removed-target
function drops from 21.76s to 2.97s.

This is the expected causal signature: static predicate work disappeared from
priority while unrelated callers remained.

## Frozen gate evaluation

| Gate | Result |
| --- | --- |
| All semantic/test/race/suite/Web gates | PASS |
| All R1I logical counters equal | PASS |
| Aggregate median paired improvement ≥2% | PASS — 14.8956% |
| At least 5/6 scenarios non-regressing | PASS — 6/6 |
| No scenario regresses >2% | PASS — no regressions |
| Old caller edge reduced ≥50% | PASS — 100% |

The result is therefore **KEEP**, not NEED MORE EVIDENCE or REVERT.

## Next-step boundary

No next optimization is selected here. The required next action is a fresh CPU
profile review, because removing this priority hotspot materially reorganized
the profile. Reusing the relation in outgoing or returning to
`coveragePlacementKey` remains out of scope for R1I-C.

Full provenance and compact evidence are in
[`r1ic-evidence/`](r1ic-evidence/README.md).
