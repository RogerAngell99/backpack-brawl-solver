# R1 compact review evidence

This directory is the compact, reviewable evidence for
[`../r1-findings.md`](../r1-findings.md). It contains profiling documentation
only—no solver, policy, suite, generator, or benchmark-harness change.

## Provenance

Measured source: `f94bb285c8e895696c76a1beacd4942b35a056d1`.

The official collection used a fresh clean clone at `C:\r1-measure`, detached
at that SHA, rather than the linked worktree. Both freshly built executables
were built with `-buildvcs=true`; their metadata records the frozen revision
and `vcs.modified=false`. The full Go/OS/CPU/OS and binary metadata is in
[`provenance.txt`](provenance.txt). Binary SHA-256 values were:

```text
solver.exe:               8ECB6B2F8453A36F5A52B478BF27CE29B5635E573FDA1C8AAA3C0FBFD22358BA
solver-searchprofile.exe: 66DFC659DDF4CA09518599514FA0799C3E3861BD49F3483E1F05AC4509C39E48
```

The suite was verified before materialization. It used `general-search-v2`,
catalog SHA-256 `0ab356afc52fa8479b627d4a917bc7f31d4cad1b6e021fd618ec17ef773c57c6`,
and lock SHA-256 `b65771885c98d6a2fc93cfca64e013fdcc94485f92ed935d1426a304cd37e775`.
Only the fourteen generated development cases `gsv2-013` through `gsv2-026`
were materialized. Validation and public/private holdouts were not touched.

Raw artifacts were collected at
`C:\r1-artifacts\f94bb285c8e895696c76a1beacd4942b35a056d1`, then frozen
before this evidence was copied. [`SHA256SUMS.txt`](SHA256SUMS.txt) is the
recursive artifact manifest. Raw binaries, materialized scenarios, raw JSON,
and `.pprof` files remain outside Git.

## Versioned compact artifacts

| Versioned file | Original artifact | SHA-256 of original artifact |
| --- | --- | --- |
| `provenance.txt` | `r1-provenance.txt` | `B813A0B391538AFC6C8C333E1E67478F76530DFCDCA4AAC7091821128A0C650E` |
| `operations-gsv1-summary.json` | `r1-operations-gsv1-summary.json` | `6B864A35C68450F048AB2168B8254B7C41D7564EB54388EDD78AB28A405A4706` |
| `operations-v4-summary.json` | `r1-operations-v4-summary.json` | `1C5B551DEB41FF25BFBC6C0D1BE9FD7C56F1084301251A6C4905CB2CA941D1BB` |
| `cpu-top.txt` | `r1-cpu-top.txt` | `8FF83ADB0356C3E3004AF71213A3C2B0624EE9AA9B8A15B5AF08F1EA092F3652` |
| `cpu-top-cum.txt` | `r1-cpu-top-cum.txt` | `EDDB7603C8F4997E00FFD519F8673956802A319E2D0631E1927C9B9A6B5C5B5B` |
| `cpu-tree.txt` | `r1-cpu-tree.txt` | `EE7D5F746624E730D09F4B7042F24688CBEEEA7488F343839525744B22B83041` |
| `cpu-targeted.txt` | `r1-cpu-targeted.txt` | `FDBF0E4597EADE7401C3E8983CE3071BABACECBB24BD2315CDEF815BFFF869BE` |
| `cpu-callers.txt` | `r1-cpu-callers.txt` | `C82D0B75E38D13A928D45971635730C9981849607520471B8A66587498383A27` |
| `heap-alloc-objects.txt` | `r1-heap-alloc-objects.txt` | `F439338C9504E9203EBE458C2E167D650B4263924C1CFCE2887F80816CC92407` |
| `heap-alloc-space.txt` | `r1-heap-alloc-space.txt` | `0438D51576792684EE8C7EDC5C429B5E859C8874AC18D383496870FCC514ACBA` |
| `scenario-attribution.csv` | `r1-scenario-attribution.csv` | `8F375C2A3B040DCC6F31E0341CB6A8D347955D8E6AB940D8724DEB3E876A7ABE` |

The unversioned operation reports are `r1-operations-gsv1.json`
(`1AB3DA673D80C0AD596E9A93469612FD86B4DB6AD0777D6139E972623DF52107`)
and `r1-operations-v4.json`
(`F2C79032D200B38A358C2F59DCA4502F44A3C890A4F6D4072FD847AC0ACFDB8F`).
The aggregate normal-build report is `r1-profile.json`
(`9F49D0C0733D9ED0342B46F2E73A1AB1BA0ED8A1731923B0AFB5850F86F10E60`);
its CPU and heap profiles have SHA-256
`B6E9E38B55673FD85CB74E804EC34D19511B2804D6B90BC9835DB460C1938097`
and `E2BECE5026B59C7F44E33FC06F4D669BC3964CBF5FD2568CC1288BAE16D5B96C`.
The six individual CPU profiles and their hashes are recorded in the manifest.

## Collection map

- GSV1 operation counts: fourteen development cases at 250k and 1M,
  `workers=1`, `searchprofile` binary.
- V4 operation control: the same fourteen cases at 1M, `workers=1`,
  `searchprofile` binary.
- Aggregate CPU/heap: normal binary, GSV1, 1M, `013,015,016,018,021,024`,
  `workers=1`; 154.84s elapsed and 292.13 sampled CPU-seconds.
- Scenario spread: one normal-build CPU profile per case in that six-case
  slice. [`scenario-attribution.csv`](scenario-attribution.csv) is generated
  directly from those raw profiles.

## Review guide

1. Confirm the binary/source provenance above.
2. Read the CPU/heap views and caller/source extracts before the conclusion.
3. Check the GSV1/V4 deterministic operation equivalence.
4. Check scenario spread before treating a hotspot as general.
5. Confirm the resulting R1 decision requests instrumentation only; it does
   not include an optimization.

