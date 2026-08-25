# R1I-H compact evidence

This directory contains the compact, reviewable evidence for the R1I-H
post-R1I-G recalibration at frozen solver revision
`0cac463a79238ecaea9d95af33468cc04dd5809b`.

The authoritative large, read-only bundle is external:

```text
C:\r1ih-artifacts\0cac463a79238ecaea9d95af33468cc04dd5809b-official-02
```

Its raw manifest contains 80 files totaling 21,307,949 bytes and hashes to:

```text
cf676f97e64780ba30c01a1e511ca60276dcecd7d8faeb63fa0eb5cc353712c7
```

`official-01` was invalidated after its frozen extractor exposed a tooling
defect. It is explicitly forbidden for decision use. `official-02` recollected
all gates and solver runs from zero after a new full-extractor preflight and
tooling commit.

Key compact artifacts:

- `provenance.txt` and `preflight-record.txt`: revision, instrument blobs and
  executed hashes, environment, builds, suite identities, and formal preflight;
- `freeze-record.txt`, `SHA256SUMS.txt`, and `manifest-sha256.txt`: raw freeze;
- `accounting-validation.txt`: semantic smoke, 42-run matrices, all identities,
  mechanical inventory, and post-analysis revalidation;
- `analysis-summary.json`, `candidate-scorecard.csv`,
  `case-attribution.csv`, and `operation-summary.json`: analyzer outputs;
- `review-input.json`: exhaustive 119-entry candidate/exclusion classification;
- CPU flat/cumulative/caller/tree and hot-source extracts;
- heap `alloc_space`, `alloc_objects`, and `inuse_space` extracts;
- `historical-delta.md`: secondary R1I-D/F/H context.

The exact decision is:

```text
DECLINE: no remaining efficiency mechanism justifies another optimization
```

All raw hashes were revalidated after analysis and
`post_freeze_solver_runs=0`.
