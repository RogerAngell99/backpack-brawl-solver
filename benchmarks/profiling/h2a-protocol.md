# H2a — lazy canonical key materialization

Baseline: `ca4c4e70cb8438b29b6a1527b5adae7cc0f2890c` on `main`.

H2a is an efficiency-only experiment. It retains the existing lexical
`placementKey` representation and comparison. The candidate key is constructed
only after the first placement with the same `ItemID` and a different
`InstanceID` is encountered.

## Scope

Included:

- lazy candidate-key materialization in `placementRespectsCanonicalCopyOrder`;
- the same lazy flow in the `searchprofile` implementation;
- `packing-seed-feasibility-ops-v2` accounting with
  `candidate_placement_key_calls`;
- an eager test-only differential oracle and deterministic randomized tests.

Excluded:

- `Placement` fields, canonical ranks, caches, invalidation, or key rewrites;
- numeric tuple comparison, option ordering, scheduler, beam, ranking,
  policy, scoring, P2 domains, and suite-generation changes.

## Semantic contract

For every candidate placement `p` and partial state `E`, H2a must preserve the
eager baseline result exactly. It must continue to compare the same strings
from the same `placementKey` function.

The deterministic profile counters that describe search trajectory must remain
identical between baseline and candidate: candidate/feasibility checks,
overlap rejects, canonical calls/rejects, charges, expansions, feasibility
outcomes, existing placements scanned, same-item comparisons, rooted counters,
and scheduler outcomes. Final score, layout keys/hashes, node consumption,
seed counts, beam diagnostics, root outcomes, and scheduler allocation must
also match exactly.

Only canonical key-work counters may change. For v2:

```text
PlacementKeyCalls = CandidatePlacementKeyCalls + SameItemComparisons
CandidatePlacementKeyCalls <= Calls
CandidatePlacementKeyCalls <= SameItemComparisons
```

In particular, a canonical invocation with no same-item copy produces zero
candidate keys, zero total keys, and zero key bytes. V1 remains readable as the
eager contract; the summary keeps mixed v1/v2 reports separate instead of
aggregating them.

## Evaluation gates

Run E0 before performance collection:

- directed cases covering zero, one, two, and four matching copies;
- accepted and rejected orderings; lower/higher original index; interleaved
  items; same instance; equal keys; rotations `0/90/180/270`; and `-90`,
  `360`, `450`, and `1080`;
- fixed-seed randomized differential tests over 2–4 copies and other items.

Then validate the frozen development corpus `gsv2-013` through `gsv2-026` at
250k and 1M nodes with `workers=1`, checking all semantic invariants before
performance. Keep `general-search-v2.lock` unchanged.

The local gate is:

```powershell
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go run ./cmd/backpack-brawl-solver verify-search-suite `
  --manifest benchmarks/suites/general-search-v2.json `
  --lock benchmarks/suites/general-search-v2.lock
```

Use the existing `BenchmarkPlacementKey`, `BenchmarkCanonicalCopyOrder`, and
`BenchmarkPackingFeasibility` benchmarks with `-benchmem -count=10`.
`BenchmarkPlacementKey` itself should not change; the zero-copy canonical
benchmark should allocate no keys.

For whole-program timing, use the frozen six-case 1M slice
`013,015,016,018,021,024` with normal builds and `workers=1`. Exclude one
warm-up run per binary, then collect nine alternating baseline/candidate pairs.
Keep H2a only if the candidate median ratio is below `0.99`, it wins at least
eight of nine pairs, and no scenario has a median regression above 3%. If the
result is near the threshold, repeat the entire pre-registered experiment once;
do not select favorable samples or cases.

CPU and heap profiles are diagnostic only and must use normal builds. Expected
causal movement is lower `placementKey`/`fmt.Fprintf` work and allocation;
wall-clock benefit remains the final keep/rollback criterion.
