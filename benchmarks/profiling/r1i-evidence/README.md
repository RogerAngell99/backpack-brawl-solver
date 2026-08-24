# R1I-B evidence bundle

## Frozen source and scope

R1I-B measured `ba1bc16d9b3ea904746f7833caa63579731c9c47` in a clean,
detached clone at `C:\r1i-measure`. Both binaries were built once with
`-buildvcs=true`, report that exact revision, and report
`vcs.modified=false`.

The verified `general-search-v2` corpus was materialized with role
`development` only. The materialized population is exactly `gsv2-013`
through `gsv2-026`; validation and public/private holdouts were not
materialized or measured.

Raw collection used:

| Pass | Cases | Budget | Build |
| --- | --- | ---: | --- |
| Smoke OFF/ON | `gsv2-013` | 250k | `searchprofile` |
| GSV1 operations | `gsv2-013`–`026` | 250k and 1M | `searchprofile` |
| V4 operations | `gsv2-013`–`026` | 1M | `searchprofile` |
| CPU/heap | `013,015,016,018,021,024` | 1M | normal |

Every pass used `repeat=1` and `workers=1`. Operation collection did not use
diagnostics or CPU/heap profiling. CPU/heap collection used the normal binary
and did not contain an operation profile.

## Provenance and freeze

The raw artifacts are at
`C:\r1i-artifacts\ba1bc16d9b3ea904746f7833caa63579731c9c47`.
The 26 raw files were recursively hashed and marked read-only before any
review derivation. The solver was not run after that freeze.

Important hashes are:

| Artifact | SHA-256 |
| --- | --- |
| Normal binary | `8575676DE1A00E731781A39255C8787CF4D2A777A2F05B49F3B378DB3ABFDFB2` |
| `searchprofile` binary | `03D5914EAE9D27459D54C40F74BC6FFA9FA38B80570D59D75E9C0489A11A0626` |
| GSV1 raw report | `BC520DDC1C85E790D5EDB2DD3A76E20D6F1D38EF186913DA2A95EA15FD5E36CB` |
| V4 raw report | `3104A0404E7087842EFDF6A1E37D46FF9A7168128F258149B9D1F3BCE50A9E13` |
| Normal benchmark report | `9E6E792F0A50699D030723B7D70C7E0B240D62819DA9AEEAB0E22144BB5F0B30` |
| CPU profile | `980980140C0207760F69EAF08CDF07AEF4935BDE43A4B7292E34F2BF7994AF6E` |
| Heap profile | `F893D2FBDDFCEACFA50B0C86A33FA8CC7114DE2D5DFC369D0CE264AEB45FB056` |
| Smoke OFF | `85E384A64F6FB80D3A28DA98BBC8C7DE8B43A2D380B9EA8D442DCFABBDA7B71D` |
| Smoke ON | `728DD2274F21D39FBDE1154606C14E71FFC351AD47DF9DD18875F9D2D3F3E7DD` |

[`SHA256SUMS.txt`](SHA256SUMS.txt) is the complete raw manifest. The source
manifest file SHA-256 is
`6B8BEF8D2EEF6A6359C77B7F901F9096A41266ABBA20A8F6229C29C4B3053127`;
the lock file SHA-256 is
`B65771885C98D6A2FC93CFCA64E013FDCC94485F92ED935D1426A304CD37E775`.
The suite verifier reported semantic catalog SHA-256
`0ab356afc52fa8479b627d4a917bc7f31d4cad1b6e021fd618ec17ef773c57c6`;
the raw `data/catalog.json` file SHA-256 is
`173A43CD20AED017BE5836A870EA2D249B30D3A07294BE7FD08AFEA50C925EBE`.

[`provenance.txt`](provenance.txt) records build metadata, Go environment,
host details, matrix, timestamps, and the freeze boundary.

## Integrity gates

The mandatory `gsv2-013`/250k/GSV1 smoke passed:

- revision, scenario, budget, `bound-attribution-ops-v1`, and
  `operation-profile-summary-v3` matched the frozen contract;
- OFF and ON were identical after removing only timestamp/timing fields,
  the operation-profiling marker, and the operation-profile payloads;
- all per-site priority and outgoing identities passed;
- outgoing search+repair checks and prunes reconciled with the authoritative
  `SearchStats` fields.

The same accounting validation passed for every populated profile in the
official matrix. `gsv2-017` at GSV1 250k returned before any attributed bound
work; its absent profile agrees with zero authoritative outgoing checks and
prunes. The ordinary repair priority counter is not independently serialized
in the benchmark summary; all matrix `repair_dfs` calls/rejections are zero,
and that reconciliation remains covered by the tagged solver regression test.

See [`accounting-validation.txt`](accounting-validation.txt) for the compact
gate record.

## Compact artifacts

| Versioned file | Source artifact | Source SHA-256 |
| --- | --- | --- |
| `operations-gsv1-summary.json` | `operations/r1i-gsv1-summary.json` | `A17B079B3B893E9A8C65070F7D6A78BD9B7444904690539D3F8A11B923014EEC` |
| `operations-v4-summary.json` | `operations/r1i-v4-summary.json` | `FF2E1B648C7F2FBBB4547AB4AA93B931626BE30B0684DB1EFB67981B496EDA9F` |
| `cpu-top.txt` | aggregate CPU profile | `4C81F77AF8472A41FFEBBFEC55B38F8C498E0AB802AA8045251E83AEFEC7D6DD` |
| `cpu-top-cum.txt` | aggregate CPU profile | `430897EABB4A8808A10E8F7B1B12CC7D91945440EC3EEE3CEA9E08B412A6EA7B` |
| `cpu-targeted.txt` | aggregate CPU profile | `B052821245A6BA44234110E1B755C2BAC7FC94793C9B02DAFC464EF84B59D5B3` |
| `cpu-callers.txt` | aggregate CPU profile; trailing table padding normalized | `E2C68E667A5BA2D92B9EEB655E7FA6CC9B84A44BEE38C16126B4E9E8476A46DA` |
| `priority-source.txt` | aggregate CPU profile | `26E5484123B31532B22736D432FFC7239E5BB4E113A4120DE339645EDD7DECF0` |
| `outgoing-source.txt` | aggregate CPU profile | `1D061D407AA2CD71DDC02CA4A9CEF96EDD7F8983DD4D85EF8102355708456BC5` |
| `heap-alloc-objects.txt` | aggregate heap profile | `4991D89E814DE317B0ADCEC2EEF1C3A3793743C0C311B859CE5EB800B7B5E5F5` |
| `heap-alloc-space.txt` | aggregate heap profile | `F6CE9BE1036C56355821CBA64A4EC947A9EE808BB9300A71CF501EEBE58796C6` |
| `case-attribution.csv` | frozen operation reports | `A90AC549399F47F2B32E4F06614018F3FF26C498AB8D7132EEF3B99B1C2121BF` |
| `candidate-scorecard.csv` | frozen operation reports + pprof extracts | `7F2E0E475D3CA6BF55193BB1FC780669C30FB28C1E0C4F4293720B98D998D7C2` |
| `control-comparison.csv` | frozen GSV1/V4 reports | `5F8F253E02B70B61326A5096F1745D64FAB11DACB17AC491DA7498B932EDAF4A` |
| `analysis-summary.json` | frozen reports + pprof extracts | `B86BF14A98ECAC734B21E54A5C9E02A517D74D528F1B1C3E7D8EB595E10A351F` |
| `accounting-validation.txt` | frozen reports | `BED350F332169CE0C020FB93B07F22622679BFA072242E30EB0998FC0A63313E` |

The external derivation source is
`review-01/analyze-r1i.mjs` in the raw artifact directory, SHA-256
`E6A25C4979AACF7AA7610A3D8195A5F2A66315D5E80A149F59BD06A065106C8E`.
It validates the matrix and identities before writing the CSV/JSON review
artifacts.

The SHA-256 in the table is the generated source-artifact hash. The versioned
`cpu-callers.txt` removes only `pprof`'s trailing table padding so the commit
passes repository whitespace checks; no data row or numeric value changed.

## Review order

1. Confirm revision, binary metadata, suite scope, and raw hashes.
2. Confirm the smoke and accounting gates.
3. Read CPU callers/source extracts before interpreting operation counts.
4. Use `case-attribution.csv` for breadth and concentration.
5. Review `candidate-scorecard.csv`; nested CPU regions are alternatives,
   not additive percentages.
6. Read the single R1I-B decision in
   [`../r1i-findings.md`](../r1i-findings.md).
