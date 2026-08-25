# E1-A efficacy findings

Baseline: `11644a7d88bd4e4bdd1f97977f8aad5e59391293`. Only locked development cases `gsv2-013..026` were materialized. Validation and both holdouts remained closed.

Two pre-decision bundles were invalidated and excluded. `official-01` stopped on a collector strict-mode defect while reading an omitted optional report field. `official-02` stopped when the protocol incorrectly treated the frozen baseline's known `gsv2-017 @250k` no-solution outcome as a tooling error. Neither bundle was frozen or analyzed. The corrected tooling commit `a34cb1c25c084bc64216dfdbf056232483560795` passed a fresh zero-run preflight, and `official-03` recollected every run from zero.

The authoritative matrix contains 70 non-diagnostic quality runs and 70 diagnostic twins. All 70 pairs matched; 69 runs exposed and passed full node-ledger reconciliation, while one deterministic no-solution pair was explicitly recorded with unavailable ledger telemetry. Catalog identities, run keys, and frozen raw hashes passed validation.

## Existing-policy controls at 1M

| Control | Wins | Ties | Losses | Layout-only | High-severity losses | Gate C |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| v4 | 0 | 14 | 0 | 0 | 0 | FAIL |
| v5 | 0 | 14 | 0 | 1 | 0 | FAIL |
| v5.1 | 0 | 14 | 0 | 1 | 0 | FAIL |

No control produced a semantic win at 1M. The two layout changes are canonicalization-only ties and do not count as efficacy.

## Baseline phase census at 1M

| Phase | Material cases | Dead material cases | Improvement cases | Final producer cases | Aggregate budget share | Gate A |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| dfs | 14 | 13 | 1 | 0 | 51.20% | PASS |
| star_seed | 14 | 10 | 4 | 1 | 11.58% | PASS |
| post_repair | 12 | 10 | 2 | 2 | 10.99% | PASS |
| pre_repair | 10 | 1 | 10 | 8 | 10.60% | FAIL |
| packing_seed | 3 | 0 | 14 | 0 | 4.12% | FAIL |
| plateau_lns | 2 | 2 | 0 | 0 | 1.24% | FAIL |

Repair is material in 12 cases, records strict semantic incumbent improvements in 11, and first produces the final semantic score in 10. Combined pre/post repair charges 21.59% of the aggregate 1M budget. Final unused nodes reach 5% of `MaxNodes` in 8 cases. No control supplied matched-root starvation evidence.

The phase census also exposes broad dead spend in DFS, star seed, and post-repair. The frozen tier order nevertheless selects repair first because Gate E proves broad repair relevance, while the current aggregate telemetry cannot say whether high-ranked neighborhoods are the productive ones or whether equal allocation starves them. Implementing rank weighting now would therefore outrun the causal evidence.

## Decision

```text
NEED MORE EVIDENCE: repair-weighted-v1
replace equal per-neighborhood repair quota within each elite base with deterministic rank-weighted allocation while preserving equal budget across elite bases and an exploration floor
```

Rationale: repair is material in 12 cases, improves the semantic incumbent in 11, and first produces the final score in 10, but aggregate reports cannot attribute productivity to neighborhood rank or quota.

Exact missing evidence: record per-neighborhood elite-base ID, stable rank/key/operator, allocated and consumed nodes, first strict semantic-improvement node, best semantic delta, and final-score producer flag without changing search.

This decision selects one mechanism only. It does not authorize E1-B, validation materialization, default promotion, or any solver change in this pull request.
