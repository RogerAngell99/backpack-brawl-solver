# P0 compact review evidence

This directory makes the compact, reviewable evidence behind `../p0-findings.md`
available in Git. It was captured from the clean worktree at
`5015875f589feb5217802e673c9b00cb5c2fcecb`; it does not contain an
implementation change.

The profile binaries (`*.pprof`) and the large raw operation reports remain
outside Git at `C:\p0-artifacts\5015875f589f`. The files here include the
targeted and caller extracts needed to audit the CPU/allocation attribution in
the decision record without that local directory.

| Versioned file | Original artifact | SHA-256 of original artifact |
| --- | --- | --- |
| `p0-operations-gsv1.summary.json` | `operations-gsv1.summary.json` | `2719595cde840621eae981b409a7ba94071f20533c90cf2441832ae3686af7cf` |
| `p0-operations-v4.summary.json` | `operations-v4.summary.json` | `46b95fb43c1c7974c00e75d09cd35e88dca229ca23636c46dc1cc409436d85b7` |
| `p0-scheduler-gsv1.summary.json` | `scheduler-gsv1.summary.json` | `772cf37260362d919f344e2db7b17786cc3d1ef0c52fa02b172b669983120bda` |
| `p0-cpu-top.txt` | `cpu-top.txt` | `69b765630fbd42e6cf83ac9d28d65d207eebbe3b0b0d4b5bd14ab70e1ae65efb` |
| `p0-cpu-top-cum.txt` | `cpu-top-cum.txt` | `130be481a686ef83863fe63bfec57fc1e2d1e8ee2c36d291d7776ae8446cb7c5` |
| `p0-cpu-targeted.txt` | generated targeted extract of `gsv1-1m.cpu.pprof` | `ba22e1d53e8f065f625bc42d9940c925ccf52b046a14b2c38a0677dc7a71dc28` |
| `p0-cpu-callers.txt` | generated caller extract of `gsv1-1m.cpu.pprof` | `ba22e1d53e8f065f625bc42d9940c925ccf52b046a14b2c38a0677dc7a71dc28` |
| `p0-heap-alloc-objects.txt` | `heap-alloc-objects.txt` | `6536f63f7fe23e0724ea79539ef849ab2ab82a5ceaa82576407589b07f411636` |
| `p0-heap-alloc-space.txt` | `heap-alloc-space.txt` | `037e3ba76ac32dc22ca233c798f0d02e248c885931eff6599aef53b8d8ae24d1` |
| `p0-heap-inuse-space.txt` | `heap-inuse-space.txt` | `45f80d8fc3cc34a5ecd9f01bede8f85fd49fc788f4bc82c82f6cc1e27b6945d2` |
| `p0-microbench.txt` | `microbench.txt` | `3448a7a717e2a850e595ebac88e6f88d4e91818dd10ad370f55d7e249ce0fa97` |

`p0-timing.json` and `p0-timing.csv` are compact derivations of the three
seven-repeat timing reports. They retain every elapsed-time sample, nodes
explored, and layout hash, while omitting each run's full solver result:

- discarded first GSV1 block: `timing-gsv1-1m.raw.json` — `ae75f07fd0a64b13ab5d0cfee19eefdc8035c373875a3b7606fb09721356ab71`;
- official GSV1 rerun: `timing-gsv1-1m-rerun.raw.json` — `fc2c38b36746ca89c7ca8d62a306a3da845543e318ffe5d7f686c22799232a43`;
- V4: `timing-v4-1m.raw.json` — `7e417824921f1f44962409b95b408a6d009b0bf9bce2df51ce594fdd8f0eb62e`.
