# R1I-H findings — post-R1I-G efficiency recalibration

## Decision

```text
DECLINE: no remaining efficiency mechanism justifies another optimization
```

R1I-H changes no solver source. The result closes macro-stage 12, Efficiency
loop. After this PR is manually merged, the next owner-authorized phase is
macro-stage 13, Efficacy experiments. R1I-H does not start that phase.

## Frozen baseline and tooling

The authoritative solver revision is:

```text
0cac463a79238ecaea9d95af33468cc04dd5809b
```

`HEAD` and `origin/main` matched that revision. The owner's original checkout
contained unrelated WASM and analysis work, so it was preserved untouched.
All R1I-H work, gates, builds, and measurements used new clones outside
OneDrive. Official source and Web/WASM clones were detached, clean, and checked
out with `core.autocrlf=false` so the blocking `gofmt -l .` gate examined the
tracked LF content.

The successful tooling preflight ran before replacement collection and
reported:

```text
status=PASS
extractor_full_smoke=PASS
repository_tests=PASS
semantic_snapshot=PASS
catalog_and_suite_locks=PASS
web_wasm_build=PASS
binary_build_and_vcs=PASS
benchmark_scenarios_runs=0
```

It generated synthetic CPU and heap profiles through the semantic unit test
and ran the complete candidate-independent extractor over the expected profile
layout. The executed scripts were frozen in protocol/tooling commit:

```text
035fe2beeed40b72640280164ff8a6332cf5a076
```

The provenance bundle records both Git blob IDs and executed SHA-256 for the
protocol, collector, extractor, analyzer, and freeze script, plus both forms of
identity for the catalog, both suite manifests/locks, generator registry, and
GSV2 generator source.

### Invalidated first collection

The first complete bundle, `official-01`, froze successfully but its extractor
then exposed a PowerShell array-return parsing defect. In accordance with the
frozen protocol, that bundle was marked `decision_use=FORBIDDEN`; no number
from it is used here. The extractor was fixed, the full-extractor preflight was
added, a new tooling commit was made, and every gate and official solver run
was repeated from zero into `official-02`.

## Gates, corpus, and semantics

All blocking gates passed on the frozen tree:

```text
gofmt -l .
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
normal vs searchprofile-OFF semantic snapshot
catalog validation
GSV1 suite-lock verification
GSV2 suite-lock verification
production Web/WASM build
```

Only GSV2 role `development` was materialized. Independent enumeration found
exactly `gsv2-013..026`. Validation `027..036`, public holdout `001..006`, and
private holdout `007..012` were neither materialized nor run.

The `gsv2-013`, GSV1, 250k semantic smoke compared normal,
searchprofile/OFF, and searchprofile/ON reports after removing only frozen
timing/provenance and operation-profile fields. All deterministic solver state,
solutions, nodes, prunes, ordering, and budgets matched.

## Operation matrix and accounting

The analyzer validated the JSON envelopes and every run internally, rather
than trusting filenames:

| Variant | Cases | Budgets | Runs |
| --- | --- | --- | ---: |
| GSV1 | `013..026` | 250k, 1M | 28 |
| V4 | `013..026` | 1M | 14 |

All priority and outgoing identities passed, including authoritative
check/prune reconciliation. Forty-one runs had a bound profile; one zero-work
run omitted it with zero authoritative checks and prunes, as permitted.

The GSV1 1M aggregate is:

```text
priority calls                     234,300
geometry candidate checks      569,592,092
outgoing checks                  10,190,290
outgoing prunes                   6,275,226
logical index/map builds         10,190,290
index/map insertions            139,985,795
target-placement lookups        471,785,825
coverage-placement-key calls     27,072,721
```

Deterministic operation breadth is 14/14 for the outgoing static-match,
coverage-key, priority-geometry, and filtered-option candidates. CPU breadth
remains a separate classification.

## Fresh whole-program hierarchy

The combined GSV1 profile contains 236.50 sampled CPU seconds. The extractor
mechanically produced 50 eligible function entries, 54 hot source-line
entries, five `alloc_space` entries, and ten top project-owned
`alloc_objects` entries: 119 total. The classification maps every entry to one
candidate or an explicit exclusion; the analyzer verifies an exact key-set
match.

| Exact target | CPU | Whole CPU | Breadth | V4 CPU | Class | `E_min` | Result |
| --- | ---: | ---: | --- | ---: | --- | ---: | --- |
| Full plateau reselection | 40.30s | 17.04% | concentrated, 2/6 | 45.15s | C2 | 17.61% | reject: breadth/risk/E unknown |
| Plateau sorting child | 16.84s | 7.12% | concentrated, 2/6 | 20.58s | C2 | 42.13% | reject: nested/breadth/risk |
| Star-score evaluation | 4.80s | 2.03% | broad, 5/6 material | 4.50s | C1 | 123.18% | reject: impossible bar |
| Outgoing static-match edge | 4.49s | 1.90% | broad, 5/6 material | 4.56s | C1 | 131.68% | reject: impossible bar |
| Canonical-copy comparisons | 3.97s | 1.68% | broad, 5/6 material | 4.14s | C1 | 148.93% | reject: impossible bar |
| Canonical layout identity | 3.90s | 1.65% | concentrated, 2/6 | 3.89s | C1 | 151.60% | reject: overlap/breadth |
| Priority residual geometry | 3.05s | 1.29% | broad, 5/6 material | 2.95s | C1 | 193.85% | reject: impossible bar |
| Shared `placementKey` residual | 2.94s | 1.24% | broad, 4/6 material | 3.06s | C1 | 201.11% | reject: overlap/impossible bar |
| Physical instance IDs | 2.89s | 1.22% | concentrated, 1/6 material | 3.11s | C1 | 204.58% | reject: breadth/impossible bar |
| `coveragePlacementKey` residual | 2.53s | 1.07% | broad, 4/6 material | 2.46s | C1 | 233.70% | reject: impossible bar |
| `filteredRemovedOptions` | 2.19s | 0.93% | broad, 5/6 material | 2.36s | C1 | 269.98% | reject: impossible bar |
| Packing comparator allocation | 0.58s | 0.25% | concentrated, 2/6 material | 0.69s | C1 | 1,019.40% | reject: impossible bar |

`E_min` is computed before assigning an estimated removal fraction. Every broad
C1 target has `E_min > 100%`: even deleting the entire sampled region at zero
replacement cost would miss the 2.5% bar. Assigning a favorable arbitrary `E`
would therefore be both methodologically invalid and decision-irrelevant.
Those rows retain `E=unknown`.

There is no mechanically supported broad C0 mechanism at or above 2.0%. The
new star-score hotspot reaches 2.03% as a full parent, but any credible reuse
requires a cache or bounded precompute and is therefore C1. It also lacks a
serialized evaluation/reuse counter and an exclusive removable child. Because
its raw ceiling is already below C1, extra instrumentation cannot change the
decision; this is a rejection, not `NEED MORE EVIDENCE`.

R1I-G's index builder is now only 0.41 sampled seconds, 0.17% of whole CPU. No
placement-index residual is mechanically eligible as another implementation.

## Plateau, runtime CPU, and allocation attribution

Runtime functions dominate the flat ranking: `runtime.scanobject` alone owns
35.26s (14.91%), with `findObject`, barriers, clearing, copying, and other GC
work also high. These are not mechanisms. Fresh heap attribution identifies
the responsible project callers:

```text
total alloc_space                         201,376,837,591 bytes
selectPlateauEntries flat                 114,849,399,448 bytes  57.03%
plateauArchive.observe flat                52,884,757,863 bytes  26.26%
combined exclusive plateau flat ownership                     83.29%

total alloc_objects                           207,422,391
total inuse_space                              32,841,279 bytes
```

The archive is therefore both the largest project CPU edge and the main source
of allocation/GC pressure. But it remains present and material only in
`gsv2-018` and `gsv2-024`. There is no archive operation-breadth counter, and
any rewrite must preserve admissions/rejections, exact selected set and order,
signature diversity, downstream base enumeration, and search trajectory. The
sort child is nested in the same overlap group and cannot be added to the
40.30s parent. Size alone does not clear the frozen broadness and semantic-risk
gates.

Other notable fresh allocation sources—physical instance IDs, placement keys,
canonical placement keys, filtered options, and the packing comparator—do not
have sufficient broad CPU ceilings to justify another efficiency experiment.
Allocation evidence supports causal attribution; it is not converted into
CPU benefit.

## Why `DECLINE`, not `NEED MORE EVIDENCE`

`NEED MORE EVIDENCE` is reserved for an objective missing datum that could
change selection. No such candidate remains:

- all broad C1 regions fail even perfect-removal `E_min`;
- the only large C2 parent and its sort child are objectively concentrated and
  have unacceptable archive/search-trajectory risk;
- the strongest new parent has no C0 replacement and falls below the C1 bar;
- remaining costs are fragmented, overlapping, concentrated, or consequences
  of the plateau allocation regime.

Additional counters could describe these regions more finely, but cannot make
any current broad C1 ceiling reach 2.5%. R1I-H therefore declines another
implementation and ends the Efficiency loop.

## Raw freeze

The authoritative external bundle is:

```text
C:\r1ih-artifacts\0cac463a79238ecaea9d95af33468cc04dd5809b-official-02
```

Immediately after the final combined V4 solver run:

```text
raw_file_count=80
raw_total_bytes=21307949
raw_manifest_sha256=cf676f97e64780ba30c01a1e511ca60276dcecd7d8faeb63fa0eb5cc353712c7
raw_manifest_revalidation=PASS
raw_read_only_revalidation=PASS
post_freeze_solver_runs=0
```

Extraction and analysis were derived-only. After writing the scorecard and
summaries, the analyzer rehashed every raw payload and reported:

```text
post_analysis_raw_hash_revalidation=PASS
```

## Boundary

This branch opens one profiling/documentation PR and stops. It does not change
solver source, implement another optimization, start macro-stage 13, merge,
enable auto-merge, or move `main`.
