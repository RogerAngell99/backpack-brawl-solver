# R1I-E findings — exact direct `placementKey` formatting

## Decision summary

**R1I-E KEEP.** Direct bounded decimal writes preserve every key byte and
logical search result while removing the full targeted formatting edge.

```text
Semantic matrix comparisons:       42/42 PASS
Logical profile differences:       0
Timing-pair semantic differences:  0

Aggregate median paired ratio:     0.815936
Median paired wall improvement:    18.4064%
Non-regressing scenarios:          6/6
Worst scenario median result:      2.7646% faster

Target formatter edge:
baseline:                          14.13 sampled seconds
candidate fallback:                 0.00 sampled seconds
measured removal:                 100%

Whole-profile alloc_space:         -0.3562%
Representative hot-path allocs:     4 -> 2 allocs/op

Decision: KEEP direct placement-key formatting
```

## What changed

`placementKey` now reserves the exact valid-domain string size and writes each
bounded integer as three decimal bytes. Separators and cell traversal remain
in their original order. A private helper sends every negative or wider value
through the unchanged `%03d` formatter, preserving minimum width, zero
padding, sign placement, and wide-integer behavior over the full `int` domain.

No caller, cache, map representation, ranking, archive policy, candidate
order, scheduler rule, budget, or operation-profile mirror changed.

The proof layer exhaustively compares `0..999`, checks signed boundaries and
minimum/maximum `int`, exercises empty, reordered, duplicate, and wide-value
fixtures, compares 10,000 deterministic generated placements, proves stable
ordering over 2,048 generated placements, and checks
`coveragePlacementKey` composition against the legacy test-only oracle.

## Correctness and scope

The clean candidate passed normal and tagged full tests, both solver race
suites, the normal/tagged-off cross-build snapshot, pinned suite verification,
and the CI-equivalent Web/WASM production build. The branch changes only the
frozen protocol/analyzer, the local production formatter, and its tests and
benchmarks before this documentation commit.

Only `general-search-v2` development cases `gsv2-013..026` were materialized.
Validation, public holdout, and private holdout roles remained closed.

The five-way smoke is exact. The full tagged matrix contains 28 GSV1 and 14
V4 comparisons. All 42 are exact after timing-only normalization, including
every packing-seed, rooted-packing, bound-attribution, node, budget, phase,
stop, score, key, candidate-order, archive, and scheduler field. Of 84
baseline/candidate matrix runs, 82 contain complete logical profiles; the two
expected `gsv2-017` GSV1 250k zero-work runs reconcile to zero.

## Whole-program timing

Normal binaries built once from clean detached clones ran GSV1 at 1M nodes,
`workers=1`, in seven alternating pairs per scenario.

| Scenario | Baseline median | Candidate median | Candidate/base | Speedup | Base IQR | Candidate IQR |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| `gsv2-013` | 10,752 ms | 7,500 ms | 0.6975 | 30.25% | 303 ms | 186 ms |
| `gsv2-015` | 8,275 ms | 5,760 ms | 0.6961 | 30.39% | 133 ms | 115 ms |
| `gsv2-016` | 9,385 ms | 7,668 ms | 0.8170 | 18.30% | 107 ms | 44 ms |
| `gsv2-018` | 55,136 ms | 53,512 ms | 0.9705 | 2.95% | 999 ms | 864 ms |
| `gsv2-021` | 14,719 ms | 12,040 ms | 0.8180 | 18.20% | 58 ms | 71 ms |
| `gsv2-024` | 44,599 ms | 43,366 ms | 0.9724 | 2.76% | 506 ms | 483 ms |

The median of all 42 paired candidate/base ratios is `0.815936`, an 18.4064%
improvement. All six scenario medians improve; the worst ratio is `0.972354`,
well inside the frozen 2% regression ceiling.

## Causal CPU and allocation evidence

The combined baseline profile contains 273.78 sampled CPU seconds.
`placementKey` is 15.21s cumulative; its header and cell formatter lines are
5.94s and 8.19s, totaling the targeted 14.13s. The candidate contains 258.07
sampled seconds, `placementKey` falls to 2.98s cumulative, and the fallback
`fmt.Fprintf` line has no sample. The targeted edge removal is therefore 100%.

The already-existing `coveragePlacementKey` caller benefits without a cache
or ownership change: its cumulative CPU falls from 9.78s to 2.43s.

Heap sampling records:

```text
                           baseline          candidate       change
alloc_space total      221,144,341,413    220,356,550,800    -0.3562%
alloc_objects total        258,420,017        219,121,247   -15.2074%
placementKey cumulative      2,572.60 MB         1,704.56 MB
```

The four-cell microbenchmark falls from 739.4ns, 144 B, and 4 allocs/op to
115.3ns, 80 B, and 2 allocs/op. All one-, four-, and eight-cell candidate
fixtures allocate less than their legacy equivalents.

## Frozen gate evaluation

| Gate | Result |
| --- | --- |
| Unit, tagged, race, suite, Web/WASM, and cross-build | PASS |
| Five-way smoke and all logical profiles exact | PASS |
| 42/42 semantic comparisons exact | PASS |
| Aggregate paired improvement at least 2% | PASS — 18.4064% |
| At least 5/6 timing cases non-regressing | PASS — 6/6 |
| No timing case regresses more than 2% | PASS — no regressions |
| Target formatter edge reduced at least 50% | PASS — 100% |
| Whole-profile `alloc_space` not materially worse | PASS — 0.3562% lower |
| Hot-path allocs/op do not increase | PASS — 4 to 2 |

The frozen analyzer therefore returns **KEEP**, not NEED MORE EVIDENCE or
REVERT.

## Raw freeze

After the candidate combined profile, no solver command ran again. The
external bundle at
`C:\r1ie-e-artifacts\4289c6f5e8c63fe4d2073132ddee57cea7d0f590`
contains 152 payload files totaling 40,240,402 bytes. All 153 payload/manifest
entries are read-only and revalidated. The separate manifest SHA-256 is:

```text
2ce8fde1bdb1a7c1ac0a9e5c910b10bf70109c57a62041e87212fcd03fdb7422
```

`post_freeze_solver_runs=0`.

Compact review evidence is in
[`r1ie-evidence/`](r1ie-evidence/README.md).
