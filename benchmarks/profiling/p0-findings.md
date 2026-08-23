# P0 findings — rooted-packing instrumentation and profiling

## Environment and integrity

- Measured commit: `5015875f589feb5217802e673c9b00cb5c2fcecb` in a clean detached worktree at `C:\src\backpack-brawl-solver-p0`.
- Go: `go1.25.6 windows/amd64`.
- Host: Windows 11 Home `10.0.26200`, Intel i5-11300H (4 cores / 8 logical processors), balanced power plan.
- Collection date: 2026-08-22.
- Raw artifact directory: `C:\p0-artifacts\5015875f589f`; `SHA256SUMS.txt` hashes its collected files.
- The compact evidence needed to audit this record is versioned in [`p0-evidence/`](p0-evidence/README.md): operation/scheduler summaries; top, targeted, and caller CPU extracts; allocation reports; microbenchmarks; and compact timing CSV/JSON. Raw `.pprof` files and multi-megabyte raw reports remain outside Git.
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

- **Generic packing-seed feasibility / canonical-copy ordering:** `packingFeasibility` is 16.74% cumulative CPU and sits on the `packingSeedSearch` path. Its main descendant, `placementRespectsCanonicalCopyOrder`, is 19.64% cumulative and receives 81.92% of its samples from `packingFeasibility`; do not add these overlapping percentages. `placementKey` is 21.21% cumulative, with 88.85% of its samples in `fmt.Fprintf`; 87.17% of `placementKey` callers are canonical-copy-order checks.
- **Runtime / GC:** `runtime.gcDrain` is 34.76% cumulative, consistent with the allocation profile. The feasibility path's repeated string construction is an important contributor.
- **Plateau archive selection:** `selectPlateauEntries` is 11.00% cumulative CPU, but it is archive selection rather than the rooted-packing frontier finish targeted by P1e.
- **Rooted-packing-specific mechanisms are small in this whole-program profile:** `constellationRootMRVFeasibilityWithOperations` is 1.74%, `constellationRootPackingFinishMRVDepthWithOperations` is 0.14%, and `constellationRootMRVStateKey` is 0.14% cumulative CPU. The root-MRV function is the function measured by the rooted-packing operation counters; the generic `packingFeasibility` function is used only for rooted-session initialization there, while it is repeatedly called for packing-seed children.

The `1 / (1 - 0.1674 × r) = 1.04–1.09×` Amdahl range applies only to an experiment that removes 25–50% of the **generic `packingFeasibility` region**. It is not a bound for a rooted-packing-only domain cache; the directly profiled rooted-MRV region is 1.74% cumulative CPU. This record therefore does not use 16.74% to promote a rooted-only change.

[`p0-cpu-targeted.txt`](p0-evidence/p0-cpu-targeted.txt) makes the four feasibility/rooted targets above auditable, and [`p0-cpu-callers.txt`](p0-evidence/p0-cpu-callers.txt) records the caller-edge evidence for the 81.92% and 87.17% shares.

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

## Attribution and follow-up hypotheses

P0 does **not** establish that one optimization is supported by three measurements of the same mechanism. The whole-program signal and the operation/microbenchmark signals are split across two routes:

| Route | Whole-program evidence | Operation and microbenchmark evidence | Scope implication |
| --- | --- | --- | --- |
| Generic packing seed | `packingFeasibility`: 16.74% cumulative CPU; `placementKey` formatting/canonical ordering is allocation-heavy | None in this rooted-only counter set | Any domain optimization that claims the 4–9% Amdahl range must change this route. |
| Rooted packing | `constellationRootMRVFeasibilityWithOperations`: 1.74% cumulative CPU | 539 feasibility option checks/expansion at 1M median; linear MRV-feasibility microbenchmark | A rooted-only cache has a much smaller directly observed whole-program ceiling and reaches only 8/14 development cases at 1M. |

This leaves two distinct, unselected planning hypotheses:

1. **P2-root — rooted-packing incremental domains.** Maintain legal domains in `constellationRootPackingSession` and avoid the root-MRV rescan. Its primary whole-program evaluation uses all 14 frozen development cases; the rooted-reaching subset is secondary, mechanism-only analysis with membership frozen from the P0 baseline. It must not cite the generic 16.74% Amdahl bound.
2. **P2-global — a shared feasibility/domain engine.** Change both `packingSeedSearch` and rooted packing. This may address the generic hotspot, but requires separate equivalence gates for each phase and evidence that its domain maintenance removes enough of `packingFeasibility` to justify the broader scope.

There is also a competing, smaller hypothesis: **H2 — canonical-order representation.** The `placementKey`/`fmt.Fprintf` allocation and CPU evidence shows that making canonical-copy-order checks cheaper (for example, with a precomputed canonical rank/key) may be more direct than reducing scan count. H2 is not implemented or selected here; it must be compared with P2-global/P2-root during planning rather than silently folded into either one.

## Hypotheses not promoted by the current evidence

- **P1a option template + retag:** `PlacementOptions` is expensive in isolation, but no corresponding whole-program CPU/allocation hotspot was observed.
- **P1b fragmentation cache:** frequent but allocation-free and cheap in microbenchmarks; no visible CPU hotspot.
- **P1c placement copies:** around 10–13 copied elements per expansion, but `memmove` cannot be attributed to this mechanism and direct rooted-packing allocation evidence is small.
- **P1d compact dedup key:** state-key volume is sizeable, but the specific rooted-packing state-key path is only 0.14% CPU; the large generic `placementKey` string cost belongs to canonical-copy-order scans, not evidence to compact the rooted-packing dedup key.
- **P1e top-K:** frontiers are much larger than the beam in multiple cases, but the specific finish/sort path is only 0.14% CPU. The 11% plateau-archive selection path is a different mechanism.
- **X1 ledger batching:** neither whole-program profile nor the with/without-ledger microbenchmarks support it.

## Required P0.1 before an implementation PR

No P1/P2 implementation is selected by this record. Before planning or implementing one, P0.1 must distinguish scan multiplicity from canonical-order cost in generic `packingFeasibility`: generic feasibility calls, remaining-instance checks, option checks, canonical-order calls, `placementKey` calls/bytes/allocations, and packing-seed-only CPU and allocation attribution. This approximates `cost = number_of_checks × cost_per_check`, so a later plan can compare P2-global (primarily fewer checks) against H2 (primarily cheaper checks). P0.1 should also stamp the measured binary with its Git revision rather than report `build_revision: unknown`.

The implementation plan must state its scope and gates in advance:

- **Semantic invariants that must remain exactly equal for P2-root/P2-global/H2:** deterministic score; canonical layout key/hash; `NodesExplored`; node charges; expansion sequence; feasibility boolean outcomes; MRV-selected instance; legal-option order; root-node budget consumption; scheduler allocations/termination reasons; beam contents/evictions; depths; and ranking/policy behavior. Any change to pruning, node charges, MRV choice, ranking, or frontier membership reclassifies the proposal as a new search mechanism rather than an efficiency experiment.
- **Work counters expected to change for a feasibility optimization:** `FeasibilityOptionChecks`, `FeasibilityInstancesConsidered`, and potentially `MRVOptionChecks` and allocations. A successful optimization should reduce its intended work; equality is not a valid success criterion for these counters.
- **Scope-specific evaluation:** the primary E1 comparison uses all 14 frozen development cases for whole-program time, allocations, and semantic equivalence. The P0-baseline rooted-reaching subset is secondary analysis only, with membership not recomputed after an optimization; it reports rooted-specific counters and session microbenchmarks. P2-global additionally proves equivalence independently in generic packing seed and rooted packing. Do not combine this work with scheduler changes, policy tuning, beam changes, ranking changes, or dedup-key redesign.
