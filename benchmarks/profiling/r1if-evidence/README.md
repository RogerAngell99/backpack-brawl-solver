# R1I-F compact evidence bundle

This directory is the review-sized projection of the external read-only R1I-F
bundle collected from frozen revision:

```text
9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b
```

Protocol commit before collection:

```text
76ec03bafaf361ddc1d2c1870973bd0c6b747860
```

The raw bundle is at:

```text
C:\r1if-artifacts\9c804a566a166fd96cb7b385a0ca9dfc43bcbb9b-official-01
```

It has 76 payload files, 21,284,679 bytes, separately hashed manifest
`0ca77ccb8170a5cd865366207ea0e8d467a4f7c7f238461fe5acbcbbd52711a9`,
read-only revalidation PASS, and `post_freeze_solver_runs=0`.

## Corpus and result

Only development cases `gsv2-013..026` were materialized. Validation, public
holdout, and private holdout roles remained closed.

```text
Decision: PROMOTE outgoing bounded OriginalIndex placement index
Solver source changed: no
```

The promoted candidate combines two exclusive sibling edges inside
`outgoingBoundContext.upperPriorityCounts`: 5.29s map construction and 3.72s
placed-target lookup. The 9.01s target is material in 6/6 CPU cases, has 14/14
operation breadth, reproduces at 9.23s in V4, and yields 3.0801% conservative
heuristic benefit against the C1 2.5% bar.

## Files

- `provenance.txt`, `freeze-record.txt`, `SHA256SUMS.txt`, and
  `manifest-sha256.txt` establish build and raw-bundle provenance.
- `accounting-validation.txt` records smoke, matrix, accounting, breadth,
  inventory, freeze, and protected-corpus gates.
- `analysis-summary.json`, `candidate-scorecard.csv`, and
  `case-attribution.csv` are analyzer outputs.
- `review-input.json` contains only human mechanism boundaries, metric
  selectors, overlap/risk classifications, and rationales; CPU/heap values and
  benefit are resolved/calculated by the frozen extractor/analyzer.
- `operations-*-summary.json` are the deterministic v3 GSV1/V4 summaries.
- `cpu-top*.txt`, `cpu-callers.txt`, and `cpu-tree.txt` are the combined GSV1
  pprof projections.
- `heap-*.txt` are the combined alloc-space, alloc-object, and in-use views.
- `outgoing-placement-index-*.txt` contain the combined, V4, and six per-case
  annotated source evidence for the promoted sibling lines.
- `plateau-archive-*.txt` retain the largest rejected causal region.

Large benchmark reports, binaries, raw pprof files, per-case full pprof
projections, and the 2.9 MB canonical extractor JSON remain external. They are
reproducible from the versioned `r1if-profile-extract.ps1` and protected by the
raw manifest.
