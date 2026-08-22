# P0 rooted-packing profiling protocol

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
