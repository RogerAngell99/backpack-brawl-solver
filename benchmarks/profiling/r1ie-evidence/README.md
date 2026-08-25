# R1I-E evidence bundle

## Decision

**KEEP exact direct `placementKey` formatting.** Every semantic and logical
gate passes, all six timing scenarios improve, the aggregate median paired
speedup is **18.4064%**, the targeted formatter edge is fully removed, and
whole-profile allocation space falls slightly.

The frozen baseline is
`91af7e6e6f6469e13ae792c7d199ebad92883ea1`; the official candidate is
`4289c6f5e8c63fe4d2073132ddee57cea7d0f590`. Both normal binaries and both
searchprofile binaries were built from independent clean detached clones with
`-buildvcs=true`. Their metadata contains the expected revision and
`vcs.modified=false`.

## Collection inventory

Only development cases `gsv2-013..026` were materialized. Validation and both
holdout classes remained closed. Every solver run used `workers=1`.

| Pass | Cases | Budget | Solver runs |
| --- | --- | ---: | ---: |
| Five-way smoke | `gsv2-013` | 250k | 5 |
| GSV1 profiled A/B | `013..026` | 250k and 1M | 56 |
| V4 profiled A/B | `013..026` | 1M | 28 |
| Alternating normal timing | `013,015,016,018,021,024` | 1M | 84 |
| Combined baseline/candidate CPU+heap | same six cases | 1M | 12 |

Timing used seven pairs per scenario in the exact order
`AB BA AB BA AB BA AB`.

## Integrity results

- Five-way smoke semantics: PASS.
- Profiled semantic comparisons: 42/42 PASS.
- Timing-pair semantic comparisons: 42/42 PASS.
- Logical profile differences: 0.
- Profiled matrix runs: 82; expected zero-work runs: 2.
- Normal, tagged, both race suites, suite lock, cross-build snapshot, and
  Web/WASM: PASS.
- Normal and tagged-off cross-build snapshot SHA-256:
  `5885839cdcf66656f3ebf632421744f0dab653c7c52eea100fc5ca9c86ccdf2c`.

[`accounting-validation.txt`](accounting-validation.txt) is the compact gate
record. [`analysis-summary.json`](analysis-summary.json) is the exact output
of the analyzer frozen before implementation.

## Performance result

The aggregate median of all 42 paired candidate/base ratios is `0.815936`, an
18.4064% speedup. All six scenario medians improve, from 2.7646% to 30.3927%.
See [`timing.csv`](timing.csv) for medians, quartiles, IQRs, and every paired
ratio.

The combined CPU profiles show:

```text
target fmt edges                 14.13s -> 0.00s
placementKey cumulative         15.21s -> 2.98s
coveragePlacementKey cumulative  9.78s -> 2.43s
```

Whole-profile `alloc_space` falls 0.3562%, allocation objects fall 15.2074%,
and the representative four-cell microbenchmark falls from 4 to 2 allocs/op.
See [`cpu-targeted.txt`](cpu-targeted.txt) and
[`microbench.txt`](microbench.txt).

## Provenance and raw freeze

The read-only external bundle is at:

```text
C:\r1ie-e-artifacts\4289c6f5e8c63fe4d2073132ddee57cea7d0f590
```

It contains 152 payload files totaling 40,240,402 bytes. Including the
separate manifest, 153 entries are read-only. Every payload hash and the
manifest hash revalidated after all derivation; no solver ran after freeze.
The external manifest SHA-256 is
`2ce8fde1bdb1a7c1ac0a9e5c910b10bf70109c57a62041e87212fcd03fdb7422`.

[`provenance.txt`](provenance.txt) records build, host, scope, and freeze
details. [`SHA256SUMS.txt`](SHA256SUMS.txt) records the critical raw hashes
plus the external manifest hash.

## Review order

1. Confirm revisions and binary VCS metadata in provenance.
2. Read the accounting gate and analyzer summary.
3. Inspect the six timing distributions rather than isolated samples.
4. Confirm the two formatter source lines disappear, not merely global
   `fmt.Fprintf` cost.
5. Apply the frozen decision rule in
   [`../r1ie-protocol.md`](../r1ie-protocol.md).
