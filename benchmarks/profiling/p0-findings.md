# P0 findings — rooted-packing instrumentation and profiling

## Environment and integrity

- Measured commit: `5015875f589feb5217802e673c9b00cb5c2fcecb` in a clean detached worktree at `C:\src\backpack-brawl-solver-p0`.
- Go: `go1.25.6 windows/amd64`.
- Host: Windows 11 Home `10.0.26200`, Intel i5-11300H (4 cores / 8 logical processors), balanced power plan.
- Collection date: 2026-08-22.
- Artifact directory: `C:\p0-artifacts\5015875f589f`; `SHA256SUMS.txt` hashes its collected files.
- Both correctness gates passed: `go test ./...` and `go test -tags searchprofile ./...`.
- The `general-search-v2` lock verified and only its fourteen development cases (`gsv2-013`–`gsv2-026`) were materialized. Validation and holdouts were not used.
- Preflight (`gsv2-013`, 250k) matched normal and `searchprofile` builds for score, canonical-layout hash, nodes explored (237,135), budget consumption, and termination.
- The benchmark reports show the expected catalog SHA (`173a43…925ebe`) but `build_revision: unknown` because these runs used `go run` in this Windows worktree. The detached, clean worktree plus `commit.txt` are the source-commit evidence; this is a reporting limitation, not a mixture of binaries.
- The scheduler pass reproduced all 28 corresponding 250k/1M rooted-packing operation profiles from the GSV1 operation pass exactly.

## Operation profile

The profile was collected with `workers=1`, `searchprofile`, `repeat=1`, and no CPU/heap profiler. Rooted packing was reached in four development scenarios at 250k and eight at 1M; an absent rooted-packing profile means that mechanism was not reached, not a zero-cost run. V4 produced materially identical operation distributions.

| GSV1 metric per candidate expansion | 250k: P25 / median / P75 (n=4) | 1M: P25 / median / P75 (n=8) |
| --- | ---: | ---: |
| MRV option checks | 36 / 41 / 59 | 26 / 37 / 57 |
| Feasibility option checks | 631 / 809 / 940 | 286 / 539 / 746 |
| Placement elements copied | 9.6 / 10.2 / 10.3 | 12.0 / 12.8 / 13.2 |
| State-key bytes | 538 / 570 / 597 | 598 / 646 / 688 |
| Pre-cut states per depth finish | 227 / 244 / 286 | 691 / 846 / 887 |

Across the GSV1 operation pass, rooted packing executed 226,952 candidate expansions, 132,133,299 feasibility option checks (582.2/expansion), 8,901,822 MRV option checks (39.2/expansion), 2,742,779 copied placement elements, and 146,328,450 state-key bytes. It had zero dedup hits and zero dedup replacements.

The high `PrecutStates / DepthFinishCalls` is real, but it is not enough to promote P1e: the direct rooted-packing finish/sort path accounts for only 0.14% cumulative CPU in the whole-program profile below.

## CPU profile

CPU/heap profiling used the frozen six-case slice (`gsv2-013`, `-015`, `-016`, `-018`, `-021`, `-024`) at 1M with normal build, `workers=1`, GSV1, and no diagnostics or operation counters. The CPU profile lasted 225.35 seconds and captured 375.38 CPU-seconds of samples, so no 5M supplemental profile was needed.

Flat top five:

1. `runtime.scanobject` — 11.79%
2. `runtime.findObject` — 6.32%
3. `runtime.duffcopy` — 4.08%
4. `fmt.(*pp).doPrintf` — 3.72%
5. `runtime.(*mspan).base` — 3.47%

The important call-stack grouping is more useful than those flat frames:

- **Feasibility / canonical-copy ordering:** `packingFeasibility` is 16.74% cumulative CPU. Its main descendant, `placementRespectsCanonicalCopyOrder`, is 19.64% cumulative and receives 81.92% of its samples from `packingFeasibility`; do not add these overlapping percentages. `placementKey` is 21.21% cumulative, with 88.85% of its samples in `fmt.Fprintf`; 87.17% of `placementKey` callers are canonical-copy-order checks.
- **Runtime / GC:** `runtime.gcDrain` is 34.76% cumulative, consistent with the allocation profile. The feasibility path's repeated string construction is an important contributor.
- **Plateau archive selection:** `selectPlateauEntries` is 11.00% cumulative CPU, but it is archive selection rather than the rooted-packing frontier finish targeted by P1e.
- **Rooted-packing-specific mechanisms are small in this whole-program profile:** `constellationRootMRVFeasibilityWithOperations` is 1.74%, `constellationRootPackingFinishMRVDepthWithOperations` is 0.14%, and `constellationRootMRVStateKey` is 0.14% cumulative CPU.

Using only the non-overlapping `packingFeasibility` region as a conservative whole-program bound, removing 25–50% of that work predicts roughly `1 / (1 - 0.1674 × r) = 1.04–1.09×` speedup. This is deliberately not inflated by adding overlapping canonical-order or GC frames.

## Allocation profile

- `alloc_objects`: `strings.(*Builder).Write` is 51.62% flat and `placementKey` is 75.91% cumulative. This is the same canonical-copy-order string-building path identified above.
- `alloc_space`: `selectPlateauEntries` is 49.35% flat / 51.16% cumulative, and `plateauArchive.observe` is 22.53% flat / 75.57% cumulative. These allocations are not evidence for a rooted-packing top-K change by themselves.
- `inuse_space`: `constellationRootPackingSession.expandOption` is 72.40% flat; this captures retained end-of-profile state, not total allocation pressure.

## Scheduler opportunity

The returned-capacity figures describe reservation turnover only. The scheduler does not retain provenance for returned tokens, so this report does **not** claim that returned nodes were reallocated to better families.

| Budget | Rooted scenarios | Families: completed / budget exhausted / hard dead / no states / other | Returned capacity / turnover |
| --- | ---: | ---: | ---: |
| 250k | 4 | 0 / 16 / 0 / 0 / 0 | 0 / 17,764 (0.00%) |
| 500k | 8 | 4 / 21 / 0 / 7 / 0 | 5,818 / 96,549 (6.03%) |
| 1M | 8 | 9 / 12 / 0 / 11 / 0 | 34,986 / 244,174 (14.33%) |
| 5M | 8 | 13 / 8 / 0 / 11 / 0 | 101,523 / 397,198 (25.56%) |

There is genuine, budget-dependent reservation turnover and heterogeneous termination at 1M/5M. It is useful evidence for a future scheduler study, but it does not specify an M1b redistribution formula and is outside the single P1x selection of this record.

## Microbenchmarks and timing

- MRV feasibility grows approximately linearly with remaining items: about 89 µs / 20 KB / 768 allocs at 8 items, 178 µs / 40 KB / 1,536 allocs at 16, and 265 µs / 61 KB / 2,304 allocs at 24.
- Fragmentation is cheap and allocation-free in the microbenchmarks (roughly 115–530 ns depending on shape).
- Root state-key construction grows with state size (about 2.0 µs / 808 B / 21 allocs at 4 placed items, 5.6 µs / 2.3 KB / 54 at 12, and 11.4 µs / 4.6 KB / 101 at 24), but its rooted-packing-specific CPU path is only 0.14%.
- Finish-depth microbenchmarks grow steeply with synthetic frontier size, but the associated rooted-packing sort path is likewise only 0.14% CPU in the complete solver.
- Ledger microbenchmarks show no clear end-to-end advantage for batching: the 10k session medians with and without ledger are both about 1.95 s.

Timing used the six CPU-profile cases at 1M, normal build, `workers=1`, GSV1 and V4, with seven repetitions. The first GSV1 block was retained in the artifact directory but discarded as a whole after `gsv2-018` and `gsv2-024` accelerated monotonically across its seven runs. The full GSV1 rerun reproduced a smaller within-process warm-up pattern rather than a single OS outlier; the official statistics below retain all seven rerun samples and use median/IQR.

| Scenario | GSV1 elapsed median (IQR) | V4 elapsed median (IQR) | GSV1 / V4 elapsed |
| --- | ---: | ---: | ---: |
| gsv2-013 | 19.192 s (0.128 s) | 19.271 s (0.309 s) | 1.00× |
| gsv2-015 | 14.890 s (0.164 s) | 14.765 s (0.422 s) | 1.01× |
| gsv2-016 | 28.254 s (0.163 s) | 28.159 s (0.178 s) | 1.00× |
| gsv2-018 | 63.794 s (1.616 s) | 69.506 s (0.878 s) | 0.92× |
| gsv2-021 | 24.916 s (0.136 s) | 26.227 s (0.276 s) | 0.95× |
| gsv2-024 | 67.042 s (1.366 s) | 82.042 s (0.548 s) | 0.82× |

Timing confirms that the policy remains competitive or faster in the two more expensive measured cases, but it is not used to attribute the underlying mechanism.

## Hypotheses supported

### P2 — incremental domains

P2 is supported by all three required signals:

1. **Whole program:** the feasibility/canonical-copy-order path is a 16.74% cumulative CPU region, with repeated allocation and GC pressure from `placementKey` string formatting.
2. **Operation multiplicity:** feasibility option checks have a 1M median of 539 per candidate expansion (P25/P75 286/746), versus 37 MRV option checks; this occurs in all eight rooted-packing development scenarios that reach the mechanism at 1M.
3. **Microbenchmark:** MRV feasibility cost and allocations scale approximately linearly with remaining inventory.

The proposed P2 experiment is limited to maintaining and updating legal domains after each placement, so it avoids rescanning every remaining instance's options. It must preserve canonical-copy-order semantics, beam/ranking policy, quotas, and solver outputs; it is not a scheduler or tuning change.

## Hypotheses rejected for the first follow-up

- **P1a option template + retag:** `PlacementOptions` is expensive in isolation, but no corresponding whole-program CPU/allocation hotspot was observed.
- **P1b fragmentation cache:** frequent but allocation-free and cheap in microbenchmarks; no visible CPU hotspot.
- **P1c placement copies:** around 10–13 copied elements per expansion, but `memmove` cannot be attributed to this mechanism and direct rooted-packing allocation evidence is small.
- **P1d compact dedup key:** state-key volume is sizeable, but the specific rooted-packing state-key path is only 0.14% CPU; the large generic `placementKey` string cost belongs to canonical-copy-order scans, not evidence to compact the rooted-packing dedup key.
- **P1e top-K:** frontiers are much larger than the beam in multiple cases, but the specific finish/sort path is only 0.14% CPU. The 11% plateau-archive selection path is a different mechanism.
- **X1 ledger batching:** neither whole-program profile nor the with/without-ledger microbenchmarks support it.

## Recommended next experiment

**Choose P2 — incremental domains — as the single next optimization experiment.**

Success should be judged on the same development corpus first: preserve deterministic output/counters, then reduce feasibility scans and demonstrate a consistent whole-program time/allocation improvement. Do not make any scheduler change, policy tuning, beam change, ranking change, or dedup-key redesign in the same PR.
