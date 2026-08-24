# R1I-D evidence bundle

This directory contains compact, reviewable derivations from the read-only
external R1I-D artifact bundle. Large binaries, raw benchmark reports, per-case
profiles, and `.pprof` files remain under:

```text
C:\r1id-artifacts\6952a35ef62f84646a01a887310363450c833b83
```

Files:

- `provenance.txt`: frozen revision, system, build metadata, hashes, suite
  verification, development-only materialization, and freeze record;
- `accounting-validation.txt`: three-way smoke, matrix, profile-shape, identity,
  inventory, overlap, and closed-corpus gates;
- `analysis-summary.json`: operation totals, formula-derived candidate rows,
  and the exact PROMOTE decision;
- `candidate-scorecard.csv`: ten addressable mechanisms with CPU, breadth,
  memory, complexity, risk, overlap, and disposition;
- `case-attribution.csv`: six per-case CPU and operation-attribution rows for
  each mechanism;
- `cpu-top.txt`, `cpu-top-cum.txt`, `cpu-callers.txt`: combined GSV1 CPU views;
- `heap-alloc-space.txt`, `heap-alloc-objects.txt`, `heap-inuse-space.txt`:
  combined GSV1 heap views;
- `operations-gsv1-summary.json`, `operations-v4-summary.json`: deterministic
  operation-profile summaries;
- `SHA256SUMS.txt`: hashes of every committed evidence file other than the
  manifest itself.

The external raw manifest SHA-256 is
`7732bf3f2a3c725c67cf3bba666eef5eb943eee7b81416c874d1a1da21ee2059`.
It was revalidated after derivation. No solver command ran after the raw freeze.
