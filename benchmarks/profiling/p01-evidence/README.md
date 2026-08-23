# P0.1 compact review evidence

This directory contains the compact, reviewable evidence behind
[`../p01-findings.md`](../p01-findings.md). It records the P0.1B decision
only; it contains no solver, policy, ranking, beam, budget, suite, or
generator change.

The measured source was
`9cfeb6cf875b10860bf72eb00f775b84c569e800`. Collection used only the fourteen
`general-search-v2` `development` cases and `workers=1`. GSV1 operation counts
cover 250k and 1M; the V4 control covers 1M; the normal-build CPU/heap profile
covers the frozen six-case 1M slice.

The first clean linked worktree was rejected before the smoke run because Go
1.25.6 did not stamp VCS metadata from its linked-worktree `.git` file. The
official `rerun-01` instead used a separate clean local clone, checked out
detached at the same SHA. Both binaries then passed the required
`vcs.revision` and `vcs.modified=false` gates; the complete `go version -m`
output is versioned in `p01-provenance.txt`.

The raw artifacts, binaries, materialized scenarios, and `.pprof` files remain
outside Git at
`C:\p01-artifacts\9cfeb6cf875b10860bf72eb00f775b84c569e800\rerun-01`.
They were frozen before analysis. `SHA256SUMS.txt` records the source hashes
needed to audit the compact files below.

## Environment

- Go: `go1.25.6 windows/amd64`.
- CPU: 11th Gen Intel(R) Core(TM) i5-11300H @ 3.10GHz; 4 cores / 8 logical processors; reported maximum clock 3110 MHz.
- OS: Microsoft Windows 11 Home, `10.0.26200` build `26200`.
- Power scheme: Balanced (`381b4222-f694-41f0-9685-ff5bb260df2e`).
- Suite: `general-search-v2`, catalog SHA-256 `0ab356afc52fa8479b627d4a917bc7f31d4cad1b6e021fd618ec17ef773c57c6`, lock SHA-256 `b65771885c98d6a2fc93cfca64e013fdcc94485f92ed935d1426a304cd37e775`.

| Versioned file | Original artifact | SHA-256 of original artifact |
| --- | --- | --- |
| `p01-provenance.txt` | whitespace-normalized transcription of `p01-provenance.txt` | `7c778505af69aa940ae68c6a5fc86fced8c46046e99391a83558b4045047d73a` |
| `p01-operations-gsv1-summary.json` | `p01-operations-gsv1-summary.json` | `62dd75434b02538203cf02672eda2c106fc3e499dbd8760fabb7fd8a30095a81` |
| `p01-operations-v4-summary.json` | `p01-operations-v4-summary.json` | `fddd7b9f0bb350c5c766f2a23a3b33998bb68efb28c3be51ae811df4fa77dd48` |
| `p01-cpu-top.txt` | generated from `p01-gsv1-1m.cpu.pprof` | `ed61b5bb5c53a011afcf63fe3553b49b8debf5eebfe42c96d42adbea34d63809` |
| `p01-cpu-top-cum.txt` | generated from `p01-gsv1-1m.cpu.pprof` | `ed61b5bb5c53a011afcf63fe3553b49b8debf5eebfe42c96d42adbea34d63809` |
| `p01-cpu-packing-seed.txt` | generated from `p01-gsv1-1m.cpu.pprof` | `ed61b5bb5c53a011afcf63fe3553b49b8debf5eebfe42c96d42adbea34d63809` |
| `p01-cpu-targeted.txt` | generated from `p01-gsv1-1m.cpu.pprof` | `ed61b5bb5c53a011afcf63fe3553b49b8debf5eebfe42c96d42adbea34d63809` |
| `p01-cpu-callers.txt` | generated from `p01-gsv1-1m.cpu.pprof` | `ed61b5bb5c53a011afcf63fe3553b49b8debf5eebfe42c96d42adbea34d63809` |
| `p01-heap-alloc-objects.txt` | generated from `p01-gsv1-1m.heap.pprof` | `37cd1de52a142778ad57922701090ec6ec44920dbc86be7ce994d88861715619` |
| `p01-heap-alloc-space.txt` | generated from `p01-gsv1-1m.heap.pprof` | `37cd1de52a142778ad57922701090ec6ec44920dbc86be7ce994d88861715619` |
| `p01-heap-packing-seed-objects.txt` | generated from `p01-gsv1-1m.heap.pprof` | `37cd1de52a142778ad57922701090ec6ec44920dbc86be7ce994d88861715619` |
| `p01-heap-packing-seed-space.txt` | generated from `p01-gsv1-1m.heap.pprof` | `37cd1de52a142778ad57922701090ec6ec44920dbc86be7ce994d88861715619` |
| `p01-microbench.txt` | `p01-microbench.txt` | `bdc75cc5fae1d05790a1be0d7b79dd0fbf234483f779015a710185867009f1e7` |

The unversioned raw operation reports are
`p01-operations-gsv1.json` (`1fce991c9510a7059271d18611f5650fc66ed06c575d755523c28362e772c7dc`)
and `p01-operations-v4.json`
(`e268e642b7d45c1ff16bb6953939ff7153d2a54990da55c4f871cd31f1e65358`).
The raw normal-build report is `p01-gsv1-1m-profile.json`
(`8705e3abcf519ccc4ca481817cf312b97ab5d051e1a406e32f4094f73b05b6c8`).
The suite lock SHA-256 is
`b65771885c98d6a2fc93cfca64e013fdcc94485f92ed935d1426a304cd37e775`.
