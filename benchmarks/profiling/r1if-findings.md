# R1I-F findings — post-R1I-E efficiency recalibration

## Decision

```text
PROMOTE:
replace each outgoing-bound placementByInstanceID string map and its
placed-target string lookups with a bounded OriginalIndex placement index,
retaining exact fallback for malformed or duplicate index/ID domains
```

R1I-F is evidence-only. No production solver source changed. A later R1I-G
may test only this mechanism after this PR is manually reviewed and merged and
the new `main` SHA is fetched and verified.

## Frozen collection

Official evidence was collected from a clean detached clone of:

```text
9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b
```

The protocol, schemas, collection/freeze scripts, pprof extractor, and analyzer
were committed before the first official measurement in:

```text
76ec03bafaf361ddc1d2c1870973bd0c6b747860
```

Both accepted binaries report the frozen revision and `vcs.modified=false`.
Collection used Go 1.25.6 on Windows 11, an Intel Core i5-11300H, eight logical
processors, and `workers=1`.

Only the fourteen `general-search-v2` development cases `gsv2-013..026` were
materialized. Validation `027..036`, public holdout `001..006`, and private
holdout `007..012` remained unmaterialized and unexecuted.

## Blocking gates and accounting

All repository gates passed before collection:

```text
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
normal versus searchprofile-OFF semantic snapshot
catalog validation and general-search-v2 lock verification
CI-equivalent npm ci and Web/WASM production build
```

Normal, searchprofile-OFF, and searchprofile-ON smoke reports for `gsv2-013`
at 250k nodes were semantically identical after removing only frozen timing
and operation-profile fields. The analyzer verified the scenario, variant,
budget, worker, repeat, profiling, diagnostic, and build-revision metadata from
the JSON payloads rather than their filenames.

The operation matrix contains 28 GSV1 and 14 V4 runs. Forty-one contain a
complete `bound-attribution-ops-v1` profile; the known GSV1 250k zero-work run
contains no profile and reconciles to zero authoritative outgoing work. Every
priority, geometry, hit, outgoing source/target/key/potential/popcount, and
authoritative check/prune identity passed.

The GSV1 1M aggregate remains logically unchanged from R1I-D:

```text
priority calls                    234,300
geometry candidates               569,592,092
outgoing checks                    10,190,290
outgoing prunes                     6,275,226
placement-map builds              10,190,290
placement-map insertions         139,985,795
target-placement lookups         471,785,825
placed targets found             344,287,241
coverage-placement-key calls      27,072,721
```

## Post-R1I-E hierarchy

The combined GSV1 profile contains 219.39 sampled CPU seconds. Critical values
were resolved from versioned canonical pprof extraction, not entered as CPU or
heap numbers in the human review input.

| Mechanism | Target CPU | Whole CPU | CPU breadth | V4 CPU | Heuristic benefit | Result |
| --- | ---: | ---: | --- | ---: | ---: | --- |
| Full plateau archive reselection | 35.11s | 16.00% | concentrated, 2/6 material | 38.83s | 12.00% | reject: breadth/risk |
| Outgoing bounded placement index | 9.01s | 4.11% | broad, 6/6 material | 9.23s | 3.0801% | **PROMOTE** |
| Outgoing static-star compatibility | 4.58s | 2.09% | broad, 5/6 material | 4.58s | 1.5657% | below C1 |
| Canonical-copy rank/index work | 4.06s | 1.85% | broad, 5/6 material | 4.25s | 0.9253% | below C1 |
| Residual priority geometry | 3.00s | 1.37% | broad, 5/6 material | 2.83s | 0.6837% | below C1 |
| Residual `placementKey` buffer | 2.81s | 1.28% | broad, 4/6 material | 3.20s | 0.6404% | below C0 |
| Physical instance ID construction | 2.74s | 1.25% | concentrated, 2/6 material | 2.80s | 0.8742% | breadth/below C1 |
| Residual `coveragePlacementKey` | 2.48s | 1.13% | broad, 4/6 material | 2.55s | 0.8478% | below C1/overlap |
| `filteredRemovedOptions` | 1.48s | 0.67% | broad, 4/6 material | 1.72s | 0.5059% | below C1 |

`coveragePlacementKey` is now 2.48s cumulative, so the old pre-R1I-E 9.78s
evidence was not reused. The remaining `placementKey` helper is only 2.81s
cumulative, and the newly introduced `writePlacementKeyInt` is 0.90s.

## Promoted causal region

Within `outgoingBoundContext.upperPriorityCounts`, two sibling source lines are
non-overlapping and share one exact replacement representation:

```text
outgoing_bound.go:59  placementByInstanceID construction    5.29s
outgoing_bound.go:82  placed-target string-map lookup       3.72s
                                                            -----
exclusive target                                      =     9.01s
```

The source-line sum is 43.48% of the 20.72s parent. With the frozen C1 removal
factor and bar:

```text
F = 20.72 / 219.39
Q =  9.01 / 20.72
E = 0.75

Benefit = F × Q × E
        = 9.01 / 219.39 × 0.75
        = 3.0801312731%

C1 bar = 2.5%  -> PASS
```

The six independent profiles establish objective breadth:

| Case | Total CPU | Target CPU | Case fraction | Material |
| --- | ---: | ---: | ---: | --- |
| `gsv2-013` | 10.49s | 1.64s | 15.63% | yes |
| `gsv2-015` | 7.22s | 0.85s | 11.77% | yes |
| `gsv2-016` | 10.85s | 1.12s | 10.32% | yes |
| `gsv2-018` | 102.09s | 1.04s | 1.02% | yes |
| `gsv2-021` | 16.91s | 1.89s | 11.18% | yes |
| `gsv2-024` | 71.27s | 2.32s | 3.26% | yes |

The same exclusive source lines total 9.23s in V4. Map builds, insertions,
target lookups, and successful finds are positive in all fourteen GSV1 1M
development cases. The exact map builder also owns 17,997,286,906 sampled
allocation bytes and 11,196,279 sampled allocations; memory is supporting
causal evidence, not converted into CPU benefit.

## Semantic boundary for R1I-G

The promoted experiment is one bounded representation local to the outgoing
upper bound. A conforming implementation must:

- use `OriginalIndex` only when indices are bounded, unique, and consistent
  with the instance domain;
- preserve empty/duplicate `InstanceID`, duplicate index, invalid index, and
  last-write string-map behavior through an exact legacy fallback;
- preserve source and target found/not-found results for every lookup;
- preserve all priority vectors, outgoing checks/prunes, node charging,
  budgets, ordering, scheduler/beam/archive outcomes, and final solutions;
- claim only the source-line 59 and 82 exclusive region, not the whole outgoing
  parent or an overlapping coverage/static-star edge.

No cache, static-star relation, coverage-key reuse, archive change, bound call
gating, or search-policy change may be combined with this experiment.

## Archive decision

Archive reselection remains the largest raw edge and dominates allocation
space, but it is present and material only in `gsv2-018` and `gsv2-024`. There
is no serialized archive-operation breadth counter, and any implementation
would have to preserve admission/rejection counts, selected set and order,
signature diversity, and downstream repair-base enumeration. It therefore
fails the frozen broadness and semantic-risk gates despite a large theoretical
benefit.

## Raw freeze

After the combined V4 control, no solver command ran again. The external
read-only bundle at:

```text
C:\r1if-artifacts\9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b-official-01
```

contains 76 raw payload files totaling 21,284,679 bytes. All payloads and the
manifest are read-only and revalidated. The separate raw-manifest hash is:

```text
0ca77ccb8170a5cd865366207ea0e8d467a4f7c7f238461fe5acbcbbd52711a9
```

`post_freeze_solver_runs=0`. Pprof extraction and candidate analysis occurred
only after freeze and read only those payloads.

## Boundary

This branch opens a documentation/profiling PR and stops. It does not implement
the promoted index, run an A/B for it, create R1I-G, merge, enable auto-merge,
or move `main`.
