# R1I-C protocol — static priority-star compatibility

## Question and frozen revisions

R1I-C tests one causal hypothesis selected by R1I-B:

> Can an immutable precomputed compatibility relation remove repeated
> `StarMatchesCatalogItems` work from the priority upper bound without
> changing any logical solver result?

The implementation branch starts at the post-R1I-B `main` revision
`19159d2e970e3457d730824983fc70fe649f9202`. Official A/B collection compares
that revision with a frozen candidate revision containing only the
implementation and tests. Evidence documentation is committed only after the
candidate revision has been frozen and measured.

## Semantic contract

R1I-C may reduce cost per priority-bound call. It must not change:

- priority upper vectors, calls, prune outcomes, or matching;
- geometry candidates, overlap checks, or the meanings and values of any
  `bound-attribution-ops-v1` counter;
- solutions, scores, priority counts, layout keys, canonical hashes, node
  counts, node charging, phase budgets, stop reasons, or prune counters;
- candidate order, ranking, beam contents, scheduler allocation, quotas,
  plateau policy, outgoing bounds, or final evaluation;
- the catalog, generator, suite lock, persisted formats, or public API.

The relation is owned by `priorityBoundContext`, is built once per solve/stage,
is immutable after construction, and is shared without synchronization. It is
not process-global and is not stored in `model.Placement` or public `Config`.

The bounded representation is `[64][]uint64`, indexed by source
`OriginalIndex` and star slot. Each word is the set of compatible target
`OriginalIndex` values. Construction is accepted only when there are 1–64
instances, every original index is unique and in `[0, 64)`, and every selected
source item exists. Unsupported or malformed domains return `nil`; the priority
bound then uses the exact legacy predicate path.

The structural priority ceiling, diagnostic relaxed star bound, outgoing
bound, `EvaluateStars`, and placement-option generation remain unchanged. In
particular, R1I-C does not reuse the relation in the outgoing bound or ceiling.

## Implementation boundary

For each source instance, star slot, and target instance, the priority path
resolves static compatibility once and reuses the boolean through the relevant
placement-option loop. Geometry remains a separate check containing only:

1. valid star index;
2. in-bounds star position;
3. the target mask containing the star cell.

`starPositionHitsTarget` remains the legacy oracle and fallback. R1I-C keeps
the option-loop structure even when cached compatibility is false; eliminating
whole geometry loops is a separate experiment.

The only production call sites receiving the relation are constellation
filtering, ordinary repair DFS, plateau prefiltering, and plateau DFS.
`searchprofile` mirrors the cached path while incrementing exactly the same
logical counters as the legacy implementation. No profile schema or counter
is added.

## Blocking tests and verification

The implementation must include:

- exhaustive table-entry equivalence with
  `scoring.StarMatchesCatalogItems`, including target types/items, `CountsAs`,
  source exclusion, unknown rule status, compound conditions, same item,
  duplicate inventory items, and zero-star sources;
- fallback tests for nil relations, invalid/duplicate/out-of-range original
  indexes, more than 64 instances, missing sources, and invalid star indexes;
- rotation invariance between catalog stars and normalized placement stars;
- deterministic fixture and generated-state differential tests comparing a
  nil legacy relation with the cached relation across all four source/target
  regimes, overlaps, out-of-free options, self targets, early success, empty
  options, rotations, and duplicate source/target items;
- exact structural equality of the complete priority site profile between
  legacy and cached paths under `-tags searchprofile`.

Before benchmarking, run:

```powershell
gofmt
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
go run ./cmd/backpack-brawl-solver verify-search-suite `
  --manifest benchmarks/suites/general-search-v2.json `
  --catalog data/catalog.json `
  --lock benchmarks/suites/general-search-v2.lock
```

Run the same Web/WASM check required by CI. Any semantic difference is an
unconditional failure, even if performance improves.

## Microbenchmarks

Collect at least the following with `-benchmem -count=10` and retain raw
output:

```text
BenchmarkPriorityStarCompatibilityDirect
BenchmarkPriorityStarCompatibilityCached
BenchmarkPartialRepairPriorityUpperLegacy
BenchmarkPartialRepairPriorityUpperCached
BenchmarkBuildPriorityStarCompatibility
```

Microbenchmarks are diagnostic and cannot produce a KEEP decision.

## Official semantic A/B

Build clean normal and `searchprofile` executables for the baseline and frozen
candidate with `-buildvcs=true`. Record `go version -m`, require the expected
revision and `vcs.modified=false`, and hash each binary.

The mandatory first smoke is `gsv2-013`, GSV1, 250k nodes, `workers=1`.
Baseline and candidate must match after removing timing and build provenance
only. Candidate profiling ON must match candidate profiling OFF semantically.

The full closed development matrix is:

| Policy | Cases | Budget | Comparison |
| --- | --- | ---: | --- |
| GSV1 | `gsv2-013`–`gsv2-026` | 250k | baseline × candidate |
| GSV1 | `gsv2-013`–`gsv2-026` | 1M | baseline × candidate |
| V4 | `gsv2-013`–`gsv2-026` | 1M | baseline × candidate |

All 42 comparisons must be bit-exact for semantic and deterministic search
fields. At GSV1 1M the aggregate logical counters must retain, among all other
fields, these R1I-B reference values:

```text
priority calls                       234,300
fixed-source/removed-target checks   506,804,547
geometry candidates                  569,592,092
starPositionHitCalls                 567,835,319
matching calls                       612,353
```

Validation and holdout roles remain closed.

## Official timing and causal profile

Use normal builds, GSV1, 1M nodes, `workers=1`, and the frozen cases
`013, 015, 016, 018, 021, 024`. Run seven alternating baseline/candidate pairs
per scenario. Report each median, IQR, candidate/base ratio, speedup, and the
aggregate paired ratio.

After timing, collect one candidate CPU/heap profile on the same six-case
slice. Compare caller edges, not global function totals. The expected causal
signature is that the priority-bound caller edge into
`StarMatchesCatalogItems` approximately disappears; calls from outgoing or
other unchanged paths may remain. The former edge was 17.88 sampled CPU
seconds in the R1I-B profile.

## Frozen decision rule

R1I-C is **KEEP** only if all of the following hold:

1. every semantic, test, race, suite, and Web/WASM gate passes;
2. every R1I logical counter remains equal;
3. median paired whole-program improvement is at least 2%;
4. at least five of six scenarios do not regress in median time;
5. no scenario regresses more than 2% in median time;
6. targeted CPU evidence removes at least 50% of the old priority static-
   predicate caller edge.

If wall time improves by 0–2% while the CPU profile clearly removes the
hotspot, the decision is **NEED MORE EVIDENCE** and only pair count may be
increased. If the candidate is slower, the decision is **REVERT** regardless
of microbenchmark results.

The evidence commit is documentation-only and records the frozen revisions,
binary and artifact hashes, semantic comparisons, logical counter comparison,
timing distributions, targeted CPU caller edges, and the final decision.
