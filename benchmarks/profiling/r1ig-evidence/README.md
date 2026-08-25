# R1I-G compact evidence bundle

This directory is the review-sized projection of the external read-only
R1I-G bundle. The official raw bundle contains the four binaries, five smoke
reports, four semantic-matrix reports, 84 timing reports, four combined
profile reports, four CPU pprof files, two heap pprof files, development-only
materialized scenarios, provenance, and microbenchmark output.

Key results:

```text
Decision: KEEP
Base: 4c6b443e3abee2cb63953f53134cc7fd8f04593b
Candidate: a0f3f39552d908f76865a7307177e8bdd618c9e0
Semantic comparisons: 42/42 PASS
Logical profiles: exact
Aggregate paired improvement: 24.64%
Weighted improvement: 10.86%
Causal target reduction: 89.38%
Alloc-space change: -8.49%
Alloc-object change: -3.64%
```

Files:

- `analysis-summary.json`: mechanical gates, timings, causal and memory math;
- `timing.csv`: per-case quartiles and all seven paired ratios;
- `accounting-validation.txt`: semantic/profile and protected-corpus status;
- `cpu-memory-summary.txt`: exact GSV1/V4 source-line and heap totals;
- `microbench-summary.txt`: median ns/op plus bytes/allocations at all sizes;
- `provenance.txt`: revisions, binary hashes, environment, and data hashes;
- `freeze-record.txt`: raw count/bytes, manifest hash, read-only revalidation,
  and `post_freeze_solver_runs=0`.

The complete methodology and interpretation are in
[`../r1ig-protocol.md`](../r1ig-protocol.md) and
[`../r1ig-findings.md`](../r1ig-findings.md).
