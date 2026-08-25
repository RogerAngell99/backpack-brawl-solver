# R1I-D findings — post-R1I-C recalibration

## Decision

```text
PROMOTE:
replace fmt.Fprintf decimal formatting in placementKey with exact direct byte
appends while preserving every key byte
```

R1I-D is evidence-only. No solver source changed on this branch. R1I-E may
start only after this documentation branch is merged and a new `main` revision
is frozen.

## Frozen collection

Official evidence was collected from clean detached revision:

```text
6952a35ef62f84646a01a887310363450c833b83
```

The protocol and derivation script were committed before collection in
`b488b0ad99dfec63326bec9e6694dc1ed72ea03b`. Both accepted binaries report the
frozen revision and `vcs.modified=false`. Collection used Go 1.25.6 on Windows
11, an Intel Core i5-11300H, eight logical processors, and `workers=1`.

The suite lock verified, and only the fourteen development scenarios
`gsv2-013..026` were materialized. Validation, public holdout, and private
holdout roles remained closed.

## Blocking gates

- Normal, searchprofile-OFF, and searchprofile-ON smoke reports for
  `gsv2-013` at 250k nodes were semantically identical after removing only the
  frozen timing/provenance/profile fields.
- The operation matrix contains 28 GSV1 runs and 14 V4 runs. Forty-one runs
  contain `bound-attribution-ops-v1`; the known `gsv2-017` GSV1 250k zero-work
  run contains no profile and reconciles to zero authoritative outgoing work.
- All priority outcome, removed-option, geometry, and hit identities passed at
  every site. All outgoing source, target, key, potential, popcount, map-build,
  and authoritative check/prune identities passed.
- Combined GSV1 CPU/heap, six separate GSV1 CPU profiles, and combined V4 CPU
  were produced by the normal binary. The normal reports contain no operation
  profile payload.

The GSV1 1M matrix retains the R1I-C logical reference values:

```text
priority calls                    234,300
geometry candidates               569,592,092
star-position hit calls           567,835,319
outgoing checks                    10,190,290
outgoing prunes                     6,275,226
placement-map builds              10,190,290
placement-map insertions         139,985,795
coverage-placement-key calls      27,072,721
target-placement lookups         471,785,825
```

## New CPU hierarchy

The combined GSV1 profile contains 287.12 sampled CPU seconds. R1I-C removed
the old priority static-predicate edge, and the post-merge hierarchy is now:

| Mechanism | Target CPU | Whole program | CPU breadth | V4 target CPU |
| --- | ---: | ---: | ---: | ---: |
| Per-observation full plateau archive reselection | 42.52 s | 14.81% | 2/6 | 44.43 s |
| `placementKey` calls into `fmt.Fprintf` | 15.25 s | 5.31% | 6/6 | 14.42 s |
| Outgoing `coveragePlacementKey` construction | 10.32 s | 3.59% | 6/6 | 9.82 s |
| Outgoing placement-map construction | 7.08 s | 2.47% | 6/6 | 7.65 s |
| Outgoing static star compatibility | 4.89 s | 1.70% | 6/6 | 4.35 s |
| Outgoing placed-target map lookups | 3.87 s | 1.35% | 6/6 | 3.95 s |

The archive reselection is the largest aggregate edge, but it appears only in
`gsv2-018` and `gsv2-024`. It also changes a structural selection mechanism
whose exact archive membership, order, diversity, admission/rejection counts,
and downstream bases must all remain identical. It is therefore not the
largest *broad and semantically safe* next step.

`placementKey` is different. Its two formatting call sites account for 6.58 s
and 8.67 s in the combined profile. The edge appears in all six individual
profiles and reproduces in V4. The function also owns or causes approximately
2.68 GB sampled `alloc_space` and 90.4 million sampled allocations. The exact
returned string is already the semantic contract, so a local formatter can be
differentially tested byte-for-byte without changing callers, ownership,
search order, or call frequency.

With the frozen conservative removal factor `E=0.75`:

```text
Benefit = (15.25 / 287.12) * 0.75
        = 3.9835% whole-program CPU
```

That clears the C0 bar. The outgoing coverage-key candidate also clears its
nominal bar, but it is nested inside the promoted placement-key formatting
group and cannot claim an independent additive benefit. The broader shared
formatter has a larger isolated edge and smaller ownership surface.

## Heap evidence

The heap profile sampled 220,475,298,007 allocation bytes. The most important
signals were:

```text
selectPlateauEntries       119,099,086,985 B cumulative
placementByInstanceID       18,102,562,454 B
filteredRemovedOptions       6,933,065,864 B cumulative
physicalInstanceIDs          3,032,737,712 B
placementKey                 2,682,891,239 B cumulative
```

Memory was not converted into CPU benefit. The placement-map and filtered-
option candidates retain strong memory evidence, but their isolated CPU
ceilings remain below their C1 promotion bars.

## Candidate and overlap audit

The frozen review inventory includes the top 20 flat solver-owned CPU nodes,
top 20 cumulative solver-owned CPU nodes, top 10 solver-owned allocation-
object sites, every isolated CPU edge at or above 1%, every isolated parent at
or above 1.5%, every solver-owned `alloc_space` site at or above 1%, and the
six carry-forward families. Each entry maps to a concrete mechanism or has an
explicit exclusion reason.

Ten mechanisms were scored. `candidate-scorecard.csv` records parent/child,
exclusive subregion, overlap group, complexity, semantic risk, CPU and
operation breadth, memory evidence, and the formula-derived benefit. No parent
and child benefit was added twice.

## Raw freeze

After the combined V4 control, no solver command was run again. The external
raw bundle at
`C:\r1id-artifacts\6952a35ef62f84646a01a887310363450c833b83` contains 40
payload files totaling 19,623,165 bytes. Forty-four raw/provenance entries are
read-only and hashed. The raw manifest hash is:

```text
7732bf3f2a3c725c67cf3bba666eef5eb943eee7b81416c874d1a1da21ee2059
```

The manifest was revalidated after all pprof/table derivation, and
`post_freeze_solver_runs=0`.

## R1I-E boundary

R1I-E should freeze the post-R1I-D `main` revision and test only direct
`placementKey` formatting. Its implementation contract should require:

- exact byte equality against the legacy `%03d` formatter for deterministic
  generated placements and edge values;
- explicit fallback/oracle behavior for negative or out-of-range integers;
- preservation of cell order, empty-cell output, separators, leading zeros,
  and all callers' keys;
- normal and searchprofile semantic equality, including every logical bound
  counter;
- targeted microbenchmarks for legacy/candidate `placementKey`,
  `coveragePlacementKey`, and a representative parent;
- the frozen 42-comparison semantic matrix, seven alternating pairs on six
  cases, causal pprof confirmation, and allocation gates.

No outgoing cache, archive rewrite, canonical-rank change, or call gating may
be combined with this experiment.
