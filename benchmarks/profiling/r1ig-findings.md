# R1I-G findings — bounded outgoing placement index

## Decision

**R1I-G KEEP.** The bounded `OriginalIndex` placement index preserves every
measured search result and logical work counter while removing most of the
promoted map-build/target-lookup region.

```text
Frozen base:               4c6b443e3abee2cb63953f53134cc7fd8f04593b
Measured candidate:        a0f3f39552d908f76865a7307177e8bdd618c9e0
Semantic comparisons:      42/42 PASS
Logical profiles:          exact
Aggregate paired speedup:  24.64%
Weighted improvement:      10.86%
Causal-region reduction:   89.38%
Alloc-space change:        -8.49%
Alloc-object change:       -3.64%
Decision:                  KEEP
```

No `.go` file changed after the measured candidate was frozen. The later
collector/extractor edits only corrected provenance and candidate-only-symbol
paths before/after the official solver runs; they do not affect the measured
binaries.

## Mechanism and fallback

The outgoing context validates once that inventory `OriginalIndex` and
`InstanceID` values form a unique, non-empty domain bounded to 64 entries. A
valid call builds this stack value:

```go
type outgoingPlacementIndex struct {
    positionPlusOne [64]uint8
    presentMask     uint64
}
```

Each placement must have an in-range inventory index, appear only once, and
match the inventory ID at that index. Any unsafe static or dynamic state calls
the literal legacy map implementation. This preserves map last-write-wins
instead of approximating duplicates by index. Placement `ItemID` is not
validated, because the legacy lookup keys only by `InstanceID` and returns the
complete supplied placement.

The searchprofile mirror uses the same representation but retains
`PlacedMapBuilds`, `PlacedMapInsertions`, `PlacedMaskInstanceChecks`, and
`TargetPlacementLookups` as historical logical counters.

## Correctness evidence

All repository gates passed on clean detached candidate
`a0f3f39552d908f76865a7307177e8bdd618c9e0`:

```text
gofmt                                      PASS
git diff --check                          PASS
go test ./...                             PASS
go test -tags searchprofile ./...         PASS
go test -race ./internal/solver/...       PASS
go test -race -tags searchprofile ...     PASS
normal vs searchprofile-OFF snapshot      PASS
catalog validation                        PASS
GSV1 and GSV2 suite locks                 PASS
Web/WASM production build                 PASS
catalog/generator/suite unchanged         PASS
```

The specific suite covers all frozen malformed-domain classes, including
static and dynamic out-of-range indices, duplicate inventory indices/IDs,
duplicate placement indices/IDs, explicit last-write-wins behavior, unknown
placements, ID/index mismatch, item-ID divergence, all 64 indices, and sparse
domains. Fixed-seed corpora compare 1,000 valid states and 1,000 malformed
states. A separate 1,000-state searchprofile corpus requires complete
`OutgoingBoundSiteProfile` equality.

The official profiled matrix passed all 42 comparisons:

| Variant | Cases | Budgets | Result |
| --- | --- | --- | ---: |
| GSV1 | `013..026` | 250k and 1M | 28/28 PASS |
| V4 | `013..026` | 1M | 14/14 PASS |

The analyzer retains final solutions, all score fields, placement/layout
identity, nodes and charging, budgets and stop reasons, scheduler, beam,
archive, phases, outgoing checks/prunes, and complete operation profiles. Only
timing and build provenance are ignored. Two zero-work runs correctly omit the
profile; all other 82 profiled runs satisfy the frozen accounting identities.

## Timing

Normal binaries ran GSV1, 1M nodes, `workers=1`, seven pairs per case in
`AB BA AB BA AB BA AB` order.

| Case | Base median | Candidate median | Base Q1–Q3 | Candidate Q1–Q3 | Median speedup | Median paired ratio |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| 013 | 7,668 ms | 4,859 ms | 7,592–7,737 | 4,840–5,010 | 36.63% | 0.6368 |
| 015 | 5,795 ms | 4,623 ms | 5,768–5,848 | 4,589–4,635 | 20.22% | 0.7997 |
| 016 | 7,754 ms | 5,668 ms | 7,720–7,823 | 5,647–5,777 | 26.90% | 0.7333 |
| 018 | 53,360 ms | 52,305 ms | 52,774–73,974 | 49,903–63,935 | 1.98% | 0.9660 |
| 021 | 12,640 ms | 8,778 ms | 12,506–12,765 | 8,646–8,899 | 30.55% | 0.6945 |
| 024 | 53,977 ms | 49,632 ms | 46,545–54,530 | 42,317–49,837 | 8.05% | 0.9139 |

```text
Median of all 42 paired ratios:       0.753629584  (24.64% improvement)
Sum of six baseline medians:          141,194 ms
Sum of six candidate medians:         125,865 ms
Candidate/base sum-median ratio:      0.891433064  (10.86% improvement)
Non-regressing scenario medians:      6/6
Worst scenario median ratio:          0.980228636
```

Case 018 was noisy, including one slow candidate pair, but its frozen
scenario-median gate still improves by 1.98% and its median paired ratio
improves by 3.40%. No timing threshold was changed after collection.

## Causal CPU

The exact GSV1 source-line region is the baseline map-builder caller plus
target-map lookup versus candidate bounded builder plus both direct target
array/slice access lines:

| Region | Baseline | Candidate |
| --- | ---: | ---: |
| Builder | 7.34 s | 0.39 s |
| Target lookup/access | 3.77 s | 0.79 s |
| Total causal region | 11.11 s | 1.18 s |
| Region reduction |  | **89.38%** |

The V4 control reproduces the result: 11.10 s baseline versus 1.46 s
candidate, an 86.85% reduction. Neither sum includes geometry,
`coveragePlacementKey`, static compatibility, or popcount.

## Allocation evidence

```text
Combined GSV1 alloc_space:
  baseline   220,492,800,388 bytes
  candidate  201,775,909,120 bytes
  change     -8.49%

Combined GSV1 alloc_objects:
  baseline   216,583,997
  candidate  208,710,348
  change     -3.64%
```

`placementByInstanceID` accounts for 17,361.37 MiB and 10,976,358 objects in
the baseline heap profile and disappears from the candidate top allocation
profile. The valid-domain indexed builder reports `0 B/op / 0 allocs/op` at
all tested sizes 1, 4, 8, 16, 32, and 64. At 64 placements, the standalone
builder median falls from 12,375 ns and 37,560 B/11 allocs to 337.85 ns and
zero allocation; the full upper-bound fixture falls from 48,959 ns,
37,136 B/42 allocs to 20,594.5 ns, 904 B/33 allocs.

## Freeze and protected corpus

The final candidate V4 CPU control was the last solver run. The raw bundle was
then frozen immediately:

```text
raw_file_count=123
raw_total_bytes=39227586
manifest_sha256=c4b7d5bc35da830814669d43bb143f81350a836cd5b030a26ca99211a64847ee
read_only=true
hash_revalidation=PASS
post_freeze_solver_runs=0
validation_materialized=false
public_holdout_materialized=false
private_holdout_materialized=false
```

All subsequent CPU/heap extraction, semantic comparison, timing aggregation,
and decision derivation used the frozen artifacts read-only. A second complete
manifest revalidation also passed.
