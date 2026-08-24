# R1I-C evidence bundle

## Decision

**KEEP static priority-star compatibility.** The candidate preserves every
semantic and logical search result, improves the aggregate median paired wall
time by **14.90%**, improves all six frozen timing scenarios, and removes the
entire previously measured priority caller edge into
`StarMatchesCatalogItems`.

The frozen baseline is
`19159d2e970e3457d730824983fc70fe649f9202`; the frozen candidate containing
implementation and tests is
`4ee1cf690a019ee28f5548d360c800ddec4007a3`. Both were built from independent,
clean, detached clones with `-buildvcs=true`; all four binaries report the
expected revision and `vcs.modified=false`.

## Closed scope

Only `general-search-v2` development cases `gsv2-013` through `gsv2-026`
were materialized. Validation and both holdout groups remained closed.

The collection contains:

| Pass | Cases | Budget | Runs |
| --- | --- | ---: | ---: |
| Five-way smoke | `gsv2-013` | 250k | 5 |
| GSV1 profiled A/B | `gsv2-013`–`026` | 250k and 1M | 56 |
| V4 profiled A/B | `gsv2-013`–`026` | 1M | 28 |
| Alternating normal-build timing | `013,015,016,018,021,024` | 1M | 84 |
| Candidate CPU/heap | same six cases | 1M | 6 |

Every run used `workers=1`. Timing used seven pairs per scenario in the order
AB, BA, AB, BA, AB, BA, AB.

## Integrity results

- Five-way smoke semantic comparisons: 4/4 PASS.
- Full profiled baseline/candidate comparisons: 42/42 PASS.
- Timing-pair semantic comparisons: 42/42 PASS.
- Logical R1I profile differences: 0.
- Priority prune differences: 0.
- Node-count and deterministic search differences: 0.
- Normal, tagged, both race suites, suite lock, and Web/WASM: PASS.

At GSV1 1M, the candidate retains the frozen R1I-B logical totals:

| Counter | Value |
| --- | ---: |
| Priority calls | 234,300 |
| Fixed-source/removed-target checks | 506,804,547 |
| Geometry candidates | 569,592,092 |
| Logical `starPositionHitsTarget` calls | 567,835,319 |
| Matching calls | 612,353 |

[`accounting-validation.txt`](accounting-validation.txt) is the compact gate
record. [`analysis-summary.json`](analysis-summary.json) contains the exact
machine-readable comparison and timing summary.

## Timing result

The aggregate median of all 42 paired candidate/base ratios is `0.851044`, a
**14.8956%** speedup. All six scenario medians improve. The smallest gain is
1.8549% (`gsv2-018`); the largest is 26.6179% (`gsv2-021`). No scenario
regresses.

See [`timing.csv`](timing.csv) for medians, quartiles, IQRs, median ratios, and
paired ratios.

## Causal CPU result

The R1I-B baseline measured 17.88 sampled seconds on the priority
`starPositionHitsTarget → StarMatchesCatalogItems` caller edge. In the
candidate profile:

- that priority edge has no samples, a 100% measured removal;
- the remaining `starPositionHitsTarget` total is 0.02s and is called only by
  the unchanged diagnostic `starUpperBoundContext.matchingForSource`;
- global `StarMatchesCatalogItems` remains 5.00s, of which 4.96s is the
  deliberately unchanged outgoing path;
- `priorityGeometryCandidateHits` is 0.59s cumulative and the compatibility
  lookup itself is 0.01s;
- `partialRepairV3PriorityUpperBound` falls from 24.35s to 5.37s cumulative;
- `partialRepairSlotCanHitRemovedTarget` falls from 21.76s to 2.97s cumulative.

The caller evidence is in [`cpu-callers.txt`](cpu-callers.txt), with the wider
focused view in [`cpu-targeted.txt`](cpu-targeted.txt) and source attribution in
[`priority-source.txt`](priority-source.txt).

## Microbenchmarks

`benchstat` reports:

| Benchmark | Time | Allocations |
| --- | ---: | ---: |
| Direct static predicate | 61.13 ns | 0 B / 0 allocs |
| Cached relation lookup | 12.59 ns | 0 B / 0 allocs |
| Legacy priority upper fixture | 60.38 µs | 18.44 KiB / 43 allocs |
| Cached priority upper fixture | 18.26 µs | 18.44 KiB / 43 allocs |
| Relation construction | 1.872 µs | 1.773 KiB / 2 allocs |

The cached lookup is about 79.4% faster than the direct predicate, and the
fixture priority upper bound is about 69.8% faster with identical allocations.
[`microbench.txt`](microbench.txt) is the compact `benchstat` output; the raw
ten-run benchmark output remains in the frozen external bundle.

## Provenance and raw freeze

The 38,011,530-byte raw bundle is at
`C:\Users\roger\AppData\Local\Temp\r1ic-evidence-4ee1cf6\artifacts`.
It contains 128 files. All were marked read-only after a 127-entry recursive
hash manifest was created; no solver ran after the freeze. The manifest hash
is `AD6A48B7308F334B36DA56182C56D91F28F7DD0BFC0BB1CC11D7328C31A7334A`.

Important raw hashes:

| Artifact | SHA-256 |
| --- | --- |
| Baseline normal binary | `6889E76234CF3F865300657E77509EE9BF93CD63194C94B023EDBB03AF1186B0` |
| Baseline profiled binary | `A84DF392B8AF0C6AB407D1F3D364191B86E30B9ABD930A76F4788E1E3E494A80` |
| Candidate normal binary | `35B11C7E82CB217A933C01161CF8CD6DB56037A6F424AB6D433A6169878FAA78` |
| Candidate profiled binary | `553F35154700D1B1CD6740F824D31E27EAB3B31DFE66A757E4DA0798275DA831` |
| GSV1 baseline matrix | `1701EE7E33609391A81641C25191CA55A57938C8C34D0F81D593C5D5CDBFECD1` |
| GSV1 candidate matrix | `DDFD3ACEB6E059678CDE41324BA75867322CAEFE0763ED4FF9C4865AF29FB1BD` |
| V4 baseline matrix | `0FA258E086E7965A819AA9FE1C4B981E5071B09803B93FBA65A66616A480FBCA` |
| V4 candidate matrix | `FE877E85702ADFF272405CB3844669BA772FF99F795A8B22C993EE889E5EC9FD` |
| Candidate CPU profile | `099F9188563C7FB5F953C08BBB266171268A4D1E9DABF06BCF6D5C8053C3DD3B` |
| Candidate heap profile | `07B9959A38B511025C0F9D9E5FA3863AC464F386900D7A50D4CAEDCF92E18688` |

[`SHA256SUMS.txt`](SHA256SUMS.txt) is the complete external manifest, and
[`provenance.txt`](provenance.txt) records source, build, host, matrix, and
freeze details.

## Review order

1. Confirm the two revisions and four binary metadata records in provenance.
2. Read the semantic/accounting gate and machine-readable summary.
3. Inspect timing distributions rather than individual runs.
4. Confirm the causal caller edge, not the global predicate total.
5. Apply the frozen decision rule in
   [`../r1ic-protocol.md`](../r1ic-protocol.md).
