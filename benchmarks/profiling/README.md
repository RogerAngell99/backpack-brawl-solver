# P0 and P0.1 profiling protocol

P0 measures a fixed `general-search-v2` development population without changing a search policy, beam, quota, ranking, pruning rule, generator, or suite lock. Generated scenarios are materialized locally; do not materialize or select `validation`, `public_holdout`, or `private_holdout` cases for P0.

`p0-profile-set.json` freezes the population and budgets. The fourteen operation-count cases are `gsv2-013` through `gsv2-026`; CPU and heap profiling use the frozen six-case slice. Verify the suite before every collection:

```powershell
go run ./cmd/backpack-brawl-solver verify-search-suite `
  --manifest benchmarks/suites/general-search-v2.json `
  --lock benchmarks/suites/general-search-v2.lock
go run ./cmd/backpack-brawl-solver materialize-search-suite `
  --manifest benchmarks/suites/general-search-v2.json `
  --lock benchmarks/suites/general-search-v2.lock `
  --roles development `
  --out $env:TEMP\p0-gsv2-development
```

## Operation counts

Build with `searchprofile`, keep `--workers 1`, and leave diagnostics off. Do not request a CPU or heap profile in this run; the counters deliberately measure logical work, not timing.

```powershell
go run -tags searchprofile ./cmd/backpack-brawl-solver benchmark-scenarios `
  --dir $env:TEMP\p0-gsv2-development `
  --scenarios gsv2-013,gsv2-014,gsv2-015,gsv2-016,gsv2-017,gsv2-018,gsv2-019,gsv2-020,gsv2-021,gsv2-022,gsv2-023,gsv2-024,gsv2-025,gsv2-026 `
  --budgets 250000,1000000 --repeat 1 --workers 1 `
  --constellation-seed-variant general-search-v1 --operation-profile `
  --out $env:TEMP\p0-general-search-v1-operations.json
go run -tags searchprofile ./cmd/backpack-brawl-solver summarize-operation-profile `
  --out $env:TEMP\p0-general-search-v1-operations-summary.json `
  $env:TEMP\p0-general-search-v1-operations.json
```

Repeat the operation-count command with `--constellation-seed-variant v4`. The scheduler-opportunity pass uses the same fourteen cases with budgets `250000,500000,1000000,5000000` and `general-search-v1`. The summary calls quota returned by a family **returned capacity** or **reservation turnover**; it does not claim exact node-token reallocation.

## CPU and heap profiles

Use a normal build (no `searchprofile`) with diagnostics and operation profiling both disabled. The CLI starts CPU profiling around the benchmark harness and writes the heap profile after a garbage collection.

```powershell
go run ./cmd/backpack-brawl-solver benchmark-scenarios `
  --dir $env:TEMP\p0-gsv2-development `
  --scenarios gsv2-013,gsv2-015,gsv2-016,gsv2-018,gsv2-021,gsv2-024 `
  --budgets 1000000 --repeat 1 --workers 1 `
  --constellation-seed-variant general-search-v1 `
  --cpu-profile $env:TEMP\p0-general-search-v1.cpu.pprof `
  --heap-profile $env:TEMP\p0-general-search-v1.heap.pprof `
  --out $env:TEMP\p0-general-search-v1-profile.json
go tool pprof -top $env:TEMP\p0-general-search-v1.cpu.pprof
go tool pprof -top -cum $env:TEMP\p0-general-search-v1.cpu.pprof
go tool pprof -sample_index=alloc_objects -top $env:TEMP\p0-general-search-v1.heap.pprof
go tool pprof -sample_index=alloc_space -top $env:TEMP\p0-general-search-v1.heap.pprof
go tool pprof -sample_index=inuse_space -top $env:TEMP\p0-general-search-v1.heap.pprof
```

Use `5M` only when the initial CPU profile has too few samples. Repeat timing-oriented commands at least seven times and report median and IQR. Keep `.pprof` files as local or CI artifacts, not Git files.

## Microbenchmarks and invariants

Run the deterministic microbenchmarks with `-count=10 -benchmem`, preserving the raw output for `benchstat`:

```powershell
go test ./internal/solver -run '^$' -bench 'Benchmark.*' -benchmem -count=10
```

P0 correctness gates run in both build variants. They prove profiling is rejected without the tag, preserves rooted-packing outputs with the tag, keeps resumable work counters stable across allocation partitions, and keeps terminal projection from mutating resumable state. `run_calls` and `pause_returns` intentionally describe scheduler slicing, so they are the two lifecycle counters excluded from the partition-invariant work comparison.

The repository versions this protocol, profile set, schema, summaries, and findings template. It does not version machine-dependent `.pprof` binaries or results before measurements exist.

## P0.1A / H2a — packing-seed feasibility instrumentation

P0.1A added the independent `packing-seed-feasibility-ops-v1` eager canonical-key contract. H2a adds `packing-seed-feasibility-ops-v2`, which records lazy candidate-key materialization with `candidate_placement_key_calls`. Existing P0.1 artifacts remain v1 and are not rewritten. The summary reads both contracts but preserves mixed versions separately rather than aggregating them. The existing `root-packing-ops-v1` contract keeps its original meaning.

With a `searchprofile` binary and `--operation-profile`, the packing-seed phase emits `search.packing_seed_operation_profile`. Its counters separate the candidate loop from generic `packingFeasibility`:

- candidate options, overlap rejects, charge attempts/denials, expansions, and direct canonical-copy-order work;
- feasibility calls, remaining instances, options, overlap rejects, legal placements, dead returns, and internal canonical-copy-order work;
- canonical calls/rejects, existing placements scanned, same-item comparisons, candidate key calls, and logical `placementKey` calls/bytes for each origin.

The summary command now writes `operation-profile-summary-v3`. Its `packing_seed_feasibility` section contains raw counts, while `packing_seed_feasibility_derived` contains weighted ratios such as feasibility calls per state, options per call/expansion, canonical calls per option, key bytes per expansion, and rejection/dead-return rates. V3 continues to read P0 report JSON that has only the rooted profile.

## R1I-A — comparative bound-internal attribution

R1I-A adds the independent `bound-attribution-ops-v1` contract. A
`searchprofile` binary with `--operation-profile` emits
`search.bound_operation_profile` with fixed call sites for priority
constellation filtering, ordinary repair DFS, plateau prefiltering, plateau
repair DFS, outgoing search, and outgoing repair. Counters are task/session
local and aggregate only after results return.

Summary v3 exposes raw `bound_attribution`, weighted
`bound_attribution_derived` ratios, and `bound_attribution_by_version` when an
input mixes incompatible contracts. Raw counts remain deterministic integers;
the reducer does not consume elapsed time or recommend an optimization. The
complete frozen schema, identities, semantic gates, and post-merge collection
matrix are in [`r1i-protocol.md`](r1i-protocol.md).

## R1I-C — static priority-star compatibility experiment

R1I-C implements the single mechanism promoted by R1I-B: an immutable,
per-stage source/star/target compatibility relation used only inside the
priority upper bound. Its frozen representation, fallback behavior, semantic
matrix, timing protocol, causal CPU check, exclusions, and decision thresholds
are in [`r1ic-protocol.md`](r1ic-protocol.md).

R1I-C is **KEEP**: 42/42 semantic A/B comparisons and all logical profiles are
exact, the aggregate paired wall improvement is 14.90%, all six timing cases
improve, and the old priority static-predicate caller edge is eliminated. See
[`r1ic-findings.md`](r1ic-findings.md) and
[`r1ic-evidence/`](r1ic-evidence/README.md).

## R1I-D — post-R1I-C recalibration

R1I-D rebuilt the CPU/heap and operation hierarchy on post-R1I-C `main`, with
six separate GSV1 CPU profiles and a combined V4 control. It is evidence-only
and changes no solver code. The exact decision is **PROMOTE** direct byte
formatting inside `placementKey` while preserving the existing key string
byte-for-byte. The targeted formatter edge is 15.25 of 287.12 sampled CPU
seconds, appears in all six profiled cases, and reproduces at 14.42 seconds in
V4. See [`r1id-protocol.md`](r1id-protocol.md),
[`r1id-findings.md`](r1id-findings.md), and
[`r1id-evidence/`](r1id-evidence/README.md).

## R1I-E — exact direct placement-key formatting

R1I-E replaces the two hot `fmt.Fprintf` sites in `placementKey` with exact
three-byte writes for values in `0..999`, while retaining `%03d` as the exact
fallback outside that domain. It is **KEEP**: all 42 semantic comparisons and
logical profiles are exact, the aggregate median paired wall improvement is
18.41%, all six timing cases improve, and the targeted formatter edge falls
from 14.13 sampled seconds to zero. Whole-profile allocation space falls by
0.36%. See [`r1ie-protocol.md`](r1ie-protocol.md),
[`r1ie-findings.md`](r1ie-findings.md), and
[`r1ie-evidence/`](r1ie-evidence/README.md).

## R1I-F — post-R1I-E recalibration

R1I-F rebuilt the full CPU/heap and operation hierarchy after R1I-E with six
independent GSV1 CPU profiles and a combined V4 control. It is evidence-only
and changes no solver source. The exact decision is **PROMOTE** one bounded
`OriginalIndex` placement index for the outgoing upper bound, replacing both
the per-check `placementByInstanceID` string-map construction and placed-target
string lookups while retaining a legacy fallback for malformed domains. The
two exclusive sibling edges total 9.01 of 219.39 sampled GSV1 CPU seconds, are
material in all six CPU cases, reproduce at 9.23 seconds in V4, and clear the
C1 bar at 3.08% conservative heuristic benefit. See
[`r1if-protocol.md`](r1if-protocol.md),
[`r1if-findings.md`](r1if-findings.md), and
[`r1if-evidence/`](r1if-evidence/README.md).

## R1I-G — bounded outgoing placement index

R1I-G replaces per-check outgoing placement string-map construction and
source/target string lookups with a bounded `OriginalIndex` index. It retains
the literal legacy map path whenever inventory or placements do not prove a
unique valid 0..63 domain, including duplicate last-write-wins inputs.

R1I-G is **KEEP**: all 42 semantic comparisons and logical profiles are exact,
all six timing medians improve, aggregate paired improvement is 24.64%,
sum-median weighted improvement is 10.86%, and the causal map/index target
region falls 89.38%. Combined allocation space falls 8.49% and allocation
objects fall 3.64%; the indexed builder reports zero allocation at every
tested size. See [`r1ig-protocol.md`](r1ig-protocol.md),
[`r1ig-findings.md`](r1ig-findings.md), and
[`r1ig-evidence/`](r1ig-evidence/README.md).

The normal build keeps the existing feasibility and canonical implementations. The instrumented versions are selected only by the compile-time `searchprofile` tag with `--operation-profile`; CPU and heap collection must use the normal binary.

### P0.1 collection protocol — only after P0.1A is merged

Do not use `go run` for an official P0.1 collection. Start in a clean worktree outside OneDrive, freeze the post-merge SHA, and build both binaries with VCS metadata:

```powershell
if (git status --porcelain) {
  throw "P0.1 requires a clean worktree"
}
$SHA = git rev-parse HEAD
$ArtifactDir = "C:\p01-artifacts\$SHA"
New-Item -ItemType Directory -Force $ArtifactDir | Out-Null

go build -buildvcs=true -o "$ArtifactDir\solver.exe" ./cmd/backpack-brawl-solver
go build -buildvcs=true -tags searchprofile -o "$ArtifactDir\solver-searchprofile.exe" ./cmd/backpack-brawl-solver
$NormalMetadata = go version -m "$ArtifactDir\solver.exe"
$ProfiledMetadata = go version -m "$ArtifactDir\solver-searchprofile.exe"
$NormalMetadata
$ProfiledMetadata

$MetadataSets = @(
  ($NormalMetadata -join [Environment]::NewLine)
  ($ProfiledMetadata -join [Environment]::NewLine)
)
foreach ($Metadata in $MetadataSets) {
  if ($Metadata -notmatch "vcs.revision=$SHA") {
    throw "binary revision does not match $SHA"
  }
  if ($Metadata -notmatch "vcs.modified=false") {
    throw "binary was built from modified source"
  }
}

@(
  "git_sha=$SHA"
  "normal_binary_metadata:"
  $NormalMetadata
  "searchprofile_binary_metadata:"
  $ProfiledMetadata
) | Set-Content "$ArtifactDir\p01-provenance.txt"
```

Both metadata outputs must contain `vcs.revision = $SHA` and `vcs.modified = false`; the full output is retained in `p01-provenance.txt`. Run a 250k smoke report before the full collection and abort if its `build_revision` is `unknown` or differs from `$SHA`.

For operation counts, materialize only the fourteen `development` cases from `general-search-v2`, use `repeat=1`, `workers=1`, no diagnostics, and the tagged binary. Collect GSV1 at 250k and 1M, then V4 at 1M. CPU/heap profiling uses the normal binary, GSV1 at 1M, and the frozen six-case slice `gsv2-013`, `gsv2-015`, `gsv2-016`, `gsv2-018`, `gsv2-021`, and `gsv2-024`.

Run the new baselines separately and retain the raw output for the later evidence PR:

```powershell
go test ./internal/solver -run '^$' `
  -bench '^(BenchmarkPlacementKey|BenchmarkCanonicalCopyOrder|BenchmarkPackingFeasibility)$' `
  -benchmem -count=10
```

The P0.1B evidence PR, after collection, is documentation-only. It records the SHA, Go version, CPU/OS, binary hashes, `go version -m`, catalog and suite-lock hashes, compact summaries and pprof extracts, then selects exactly one of H2, P2-global, P2-root, or no change.
