# H2a development evidence — KEEP

Baseline: `ca4c4e70cb8438b29b6a1527b5adae7cc0f2890c`.

Candidate: `33adbab25ef2084ee6175a32342776c90b5016ce`.

Collection used clean local clones (Go `1.25.6`, Windows/amd64, 11th Gen Intel
Core i5-11300H) because Go does not stamp VCS metadata for this repository's
linked worktree layout. Both normal and `searchprofile` binaries reported their
respective revision with `vcs.modified=false`. Raw JSON and pprof artifacts are
temporary collection outputs, not repository artifacts.

## Correctness gates

All local gates passed:

```text
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
verify-search-suite general-search-v2
GOOS=js GOARCH=wasm go build ./cmd/wasm-solver
```

The test-only eager oracle passed its directed cases and 10,000 fixed-seed
randomized 2–4-copy differential cases. The `searchprofile` v2 tests also
prove the new accounting identities and retain v1 reports without silently
aggregating v1 and v2 together.

## E1 structural equality

The full frozen development corpus `gsv2-013` through `gsv2-026` was run with
`workers=1`, `general-search-v1`, and budgets `250k,1M` (28 runs per revision).
After excluding only revision/timing metadata, the profile version, and the
three permitted key-work counters, baseline and candidate JSON were exactly
identical. This includes final outputs, node use, scheduler/root outcomes, and
all non-key operation counters.

| Counter | Baseline | Candidate | Result |
| --- | ---: | ---: | --- |
| Canonical calls | 465,322,685 | 465,322,685 | exact |
| Placement-key calls | 473,665,199 | 15,585,793 | -96.71% |

The v2 aggregate reports 7,243,279 candidate-key constructions and 8,342,514
same-item comparisons, satisfying `placement_key_calls = candidate_key_calls +
same_item_comparisons`. A v1 report read through the H2a summarizer retains its
eager interpretation (`candidate keys = calls`).

## Microbenchmarks

The prescribed three benchmark families were collected with `-benchmem
-count=10` on clean baseline and candidate checkouts.

- `BenchmarkPlacementKey` itself remained unchanged (for example, one cell:
  80 B/op and 3 allocs/op in both revisions).
- `BenchmarkCanonicalCopyOrder/same_item_copies=0` fell from roughly 330 ns/op,
  80 B/op, and 3 allocs/op to roughly 4 ns/op, 0 B/op, and 0 allocs/op.
- `BenchmarkPackingFeasibility/remaining=24/all_unique` fell from roughly
  65–67 µs/op, 15,365 B/op, and 576 allocs/op to roughly 2.3–2.4 µs/op,
  0 B/op, and 0 allocs/op.
- Duplicate-copy benchmark variants remained effectively unchanged, as H2a
  still constructs and compares the same lexical keys whenever a copy exists.

## Whole-program timing

The normal binaries ran one excluded warm-up each, followed by nine alternating
pairs on the frozen six-case 1M slice
`013,015,016,018,021,024`, with `workers=1`.

| Measure | Result |
| --- | ---: |
| Median candidate / baseline block ratio | 0.7053 |
| Candidate faster pairs | 9 / 9 |
| Worst per-scenario median ratio | 0.7731 |

Per-scenario median ratios were 0.66 (`013`), 0.64 (`015`), 0.38 (`016`),
0.77 (`018`), 0.77 (`021`), and 0.75 (`024`). H2a passes the pre-registered
gate: ratio below 0.99, at least 8 of 9 wins, and no scenario median regression
above 3%.

## CPU and heap diagnostics

Normal-build CPU/heap profiles used the same frozen six-case 1M slice:

| Measure | Baseline | Candidate |
| --- | ---: | ---: |
| `placementKey` CPU cumulative | 76.21 s (21.25%) | 15.05 s (5.34%) |
| canonical-copy-order CPU cumulative | 70.80 s (19.74%) | 8.08 s (2.87%) |
| `placementKey` cumulative allocated objects | 523.3 M (75.85%) | 86.3 M (34.16%) |
| `placementKey` cumulative allocated space | 14.33 GB (6.57%) | 1.17 GB (0.57%) |

These profiles support the intended causal chain. They are diagnostic; the
paired wall-clock result is the keep criterion.

## Decision

**KEEP H2a.** The implementation preserves the exact lexical canonical-order
contract and all measured search/output invariants while providing a repeatable
whole-program speedup. Reprofile before considering any H2b representation
change; do not add a rank, cache, or `Placement` field merely because those
ideas remain available.
