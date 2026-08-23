# P0.1 findings — packing-seed feasibility attribution

## Integrity

- Measured source: `9cfeb6cf875b10860bf72eb00f775b84c569e800` (`vcs.modified=false` for both official binaries).
- Collection was clean, detached, and outside OneDrive. A Go 1.25.6 limitation prevented VCS stamping in the first linked worktree because its `.git` was a file; those binaries were rejected before smoke. The official `rerun-01` used a fresh, clean local clone detached at the same SHA, and its binaries passed the revision and cleanliness gates.
- The development-only `general-search-v2` suite was verified before materialization: catalog SHA-256 `0ab356afc52fa8479b627d4a917bc7f31d4cad1b6e021fd618ec17ef773c57c6`; lock SHA-256 `b65771885c98d6a2fc93cfca64e013fdcc94485f92ed935d1426a304cd37e775`.
- The smoke report used GSV1, `gsv2-013`, 250k, `workers=1`, and `packing-seed-feasibility-ops-v1`. Its candidate/feasibility accounting identities all passed.
- GSV1 operation collection contains 14 cases × {250k, 1M}; V4 contains the same 14 cases × 1M; the normal-build CPU/heap slice is `gsv2-013,015,016,018,021,024` × 1M. No validation, public-holdout, or private-holdout case was materialized.
- The CPU profile duration is 221.27 s with 367.09 s of samples, so the pre-registered 5 CPU-second threshold was exceeded and no supplemental 5M run was needed.

[`p01-evidence/`](p01-evidence/README.md) contains the VCS provenance, compact summaries, pprof extracts, microbenchmarks, and source hashes. Raw multi-megabyte reports, `.pprof` files, binaries, and materialized scenarios remain outside Git.

## Deterministic operation counts

The GSV1 1M population performed 576,653 packing-seed candidate expansions and exactly 576,653 `packingFeasibility` calls. Across the fourteen cases, generic feasibility re-examined 5,401,599 remaining instances and 564,015,914 placement options. Of those options, 317,079,262 (56.22%) reached the feasibility-internal canonical check, which constructed 322,848,031 placement keys and 9,410,374,580 logical key bytes. The candidate loop adds 689,296 keys and 20,776,008 bytes, for 323,537,327 keys and 9,431,150,588 bytes total.

The table gives unweighted scenario distributions for GSV1 at 1M. P25/P75 use linear interpolation over the fourteen sorted values; the raw per-scenario counts and weighted ratios are in [`p01-operations-gsv1-summary.json`](p01-evidence/p01-operations-gsv1-summary.json).

| Metric | Min | P25 | Median | P75 | Max |
| --- | ---: | ---: | ---: | ---: | ---: |
| Feasibility calls / state | 21.68 | 25.66 | 39.58 | 48.29 | 67.06 |
| Remaining instances / call | 4.38 | 7.71 | 8.56 | 9.82 | 18.51 |
| Option checks / call (and / expansion) | 478.5 | 839.8 | 901.6 | 1,032.6 | 1,520.0 |
| Feasibility canonical calls / option | 46.93% | 51.37% | 55.20% | 56.53% | 69.43% |
| Same-item comparisons / canonical call | 0.0000 | 0.0003 | 0.0169 | 0.0356 | 0.0435 |
| Placement-key calls / canonical call | 1.0000 | 1.0003 | 1.0169 | 1.0356 | 1.0435 |
| Key bytes / expansion | 8.54 KB | 13.60 KB | 14.18 KB | 16.65 KB | 32.44 KB |
| Dead-return rate | 0.00% | 0.04% | 0.59% | 1.80% | 3.81% |
| Overlap-reject rate | 30.57% | 43.47% | 44.80% | 48.63% | 53.07% |
| Canonical-reject rate | 0.00% | 0.02% | 0.77% | 1.42% | 1.83% |

The V4 control has a different execution fingerprint, but all fourteen 1M `packing_seed_feasibility` profiles are byte-for-byte identical to their GSV1 counterparts. Its medians are therefore identical to the table. The multiplicity is a structural property of the shared packing-seed route, not a GSV1-only trajectory effect. See [`p01-operations-v4-summary.json`](p01-evidence/p01-operations-v4-summary.json).

## CPU and allocation attribution

This section reports parent regions and caller edges rather than summing nested percentages.

- `packingSeedSearch` is 64.24 s cumulative (17.50% of 367.09 sampled CPU seconds); its `packingFeasibility` child is 60.90 s (16.59%).
- `placementRespectsCanonicalCopyOrder` is 71.79 s cumulative (19.56%), and `placementKey` is 78.47 s (21.38%). These regions overlap.
- The caller tree attributes 58.69 s (81.75% of canonical-order cumulative CPU) to `packingFeasibility`; canonical order attributes 68.13 s (86.82% of `placementKey` cumulative CPU) to `placementKey`; `placementKey` attributes 70.40 s (89.72% of its cumulative CPU) to `fmt.Fprintf`.
- The rooted-specific `constellationRootMRVFeasibilityWithOperations` edge is 5.89 s (1.60%), so P2-root still lacks a material whole-program signal.

For allocations, `placementKey` has 168,973,135 flat allocation objects (24.36%) and 523,951,914 cumulative objects (75.54%). In the packing-seed focus, `placementKey` accounts for 123,981,386 flat objects and 386,561,581 cumulative objects (55.74% of all sampled allocation objects); `strings.(*Builder).Write` contributes 262,577,219 flat objects along that route. The alloc-space profile is dominated by unrelated plateau work, so object attribution is the more direct signal for this hypothesis.

The supporting extracts are [`p01-cpu-packing-seed.txt`](p01-evidence/p01-cpu-packing-seed.txt), [`p01-cpu-targeted.txt`](p01-evidence/p01-cpu-targeted.txt), [`p01-cpu-callers.txt`](p01-evidence/p01-cpu-callers.txt), and the allocation extracts in [`p01-evidence/`](p01-evidence/README.md).

## Microbenchmarks

All microbenchmarks used `-count=10 -benchmem`; the source output is retained in [`p01-microbench.txt`](p01-evidence/p01-microbench.txt). Medians show that both the key representation and the feasibility scan are costly.

| Benchmark median | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `PlacementKey`, 1 cell | 328.8 | 80 | 3 |
| `PlacementKey`, 4 cells | 726.9 | 144 | 4 |
| `PlacementKey`, 8 cells | 1,191.0 | 272 | 5 |
| Canonical order, 0 same-item copies, accepted | 334.5 | 80 | 3 |
| Canonical order, 1 same-item copy, accepted | 678.0 | 160 | 6 |
| Canonical order, 2 same-item copies, accepted | 1,039.0 | 240 | 9 |
| Canonical order, 4 same-item copies, accepted | 1,731.0 | 400 | 15 |
| Feasibility, 8 remaining, unique | 22,420.0 | 5,121 | 192 |
| Feasibility, 16 remaining, unique | 44,249.0 | 10,243 | 384 |
| Feasibility, 24 remaining, unique | 67,636.5 | 15,365 | 576 |
| Feasibility, 24 remaining, duplicated | 132,142.0 | 30,730 | 1,152 |

## Competing hypotheses and decision

**Selected experiment: H2 — canonical representation.**

H2 has all three required evidence classes for the same route:

1. Whole-program: `placementKey` is 21.38% cumulative CPU, with 86.82% of that CPU called by canonical copy order; feasibility itself reaches canonical order for 58.69 sampled CPU seconds.
2. Deterministic counts: the fourteen GSV1 1M cases construct 323.5 million keys and 9.43 GB of logical key text; the median expansion constructs 528.1 keys and 14.18 KB of key bytes.
3. Microbench/allocation: canonical order rises from 334.5 ns / 80 B / 3 allocs with zero same-item copies to 1,731.0 ns / 400 B / 15 allocs with four. The heap profile independently attributes 55.74% of allocation objects to the packing-seed `placementKey` route.

P2-global also has a credible scan-multiplicity signal: the median feasibility call scans 901.6 options and the total is 564.0 million options. However, its principal measured CPU region is overwhelmingly nested canonical/key work, while an incremental-domain design has broader state ownership, invalidation, and equivalence risk. H2 addresses the same dominant nested cost locally, without changing the search route. Under the pre-registered tie rule, H2 is therefore the first experiment.

P2-root is not selected because its rooted-specific CPU edge is only 1.60%. `No change` is rejected because H2 has a material whole-program signal, deterministic key volume, and scaling allocation microbenchmarks.

## Frozen H2 implementation contract

This record selects the experiment, not its internal representation. Do not choose a concrete field, rank, cache, or ownership design until this evidence PR is reviewed.

- H2 is an efficiency-only change. It must preserve exact deterministic score; canonical layout key/hash; `NodesExplored`; charges and budget consumption; expansion sequence; feasibility outcomes; MRV choice; legal-option order; scheduler allocations and termination; beam contents/evictions; and ranking/policy behavior.
- E0 must compare canonical acceptance/rejection and order against the baseline across targeted duplicate-item fixtures and preserve the baseline solver output. Both normal and `searchprofile` builds remain tested.
- E1 must compare the frozen fourteen development cases at 250k and 1M with `workers=1`; semantic output and the listed invariants must be equal. The six-case normal-build CPU/heap slice and the three P0.1 microbenchmark families are the performance checks. Validation/public/private cases stay out of scope.
- Canonical-call accounting must remain meaningful. A successful H2 may reduce actual `placementKey` constructions, key bytes, allocations, and time, but it may not change canonical decisions or hide a semantic change by weakening instrumentation.
- Roll back the experiment if any invariant diverges, if the targeted canonical/key allocation signal does not decline, or if the preregistered whole-program comparison shows no reproducible net benefit after the local representation change. P2-global is reconsidered only after that outcome.
