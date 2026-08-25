# E1-A compact evidence

This directory contains the compact, reviewable evidence for the E1-A efficacy census at frozen solver revision:

```text
11644a7d88bd4e4bdd1f97977f8aad5e59391293
```

The authoritative large, read-only bundle is external:

```text
C:\e1a-artifacts\11644a7d-e1a-official-03
```

Its raw manifest contains 39 files totaling 247,565,214 bytes and hashes to:

```text
46fe084450daae1bee408bc48cc26a2f0c0f216f73ae78ec0ad1575624b2c5b5
```

`official-01` was invalidated before freeze or analysis after an omitted optional report field exposed a collector strict-mode defect. `official-02` was invalidated before freeze or analysis after the protocol rejected the baseline's known deterministic `gsv2-017 @250k` no-solution outcome. Corrected tooling commit `a34cb1c25c084bc64216dfdbf056232483560795` passed a new zero-benchmark preflight, and `official-03` recollected all 140 runs from zero.

Key artifacts:

- `provenance.txt`, `preflight-record.txt`, `freeze-record.txt`, `RAW-SHA256SUMS.txt`, `raw-manifest.json`, and `manifest-sha256.txt` freeze the solver/tooling identities, environment, corpus, commands, and raw bytes;
- `accounting-validation.txt` and `post-analysis-validation.txt` prove 70 diagnostic pairs, 69 complete ledgers, one explicit no-solution telemetry exception, closed validation/holdouts, and unchanged post-analysis hashes;
- `semantic-comparisons.csv` records canonical W/L/T and layout-only status for every control comparison;
- `run-census.csv`, `phase-productivity.csv`, and `phase-summary.csv` contain the per-run/checkpoint and per-phase census;
- `root-starvation.csv`, `candidate-shortlist.csv`, `analysis-summary.json`, and `decision.json` contain the frozen gates and exact candidate decision.

The exact E1-A decision is:

```text
NEED MORE EVIDENCE: repair-weighted-v1
```

All raw hashes were revalidated after analysis and `post_freeze_solver_runs=0`.
