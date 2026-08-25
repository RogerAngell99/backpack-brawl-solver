# R1I-D / R1I-F / R1I-H historical delta

Historical profiles are context only; fresh R1I-H evidence controls the
decision. Sample totals and absolute seconds may vary with runtime/GC and are
not a wall-time A/B.

| Stage | Frozen state | Combined GSV1 samples | Leading eligible observations | Decision |
| --- | --- | ---: | --- | --- |
| R1I-D | post-R1I-C | 287.12s | plateau 42.52s/14.81% concentrated; direct placement formatting 15.25s/5.31% broad; coverage key 10.32s/3.59% broad | PROMOTE direct placement formatting |
| R1I-F | post-R1I-E | 219.39s | plateau 35.11s/16.00% concentrated; outgoing bounded index 9.01s/4.11% broad; static compatibility 4.58s | PROMOTE bounded outgoing index |
| R1I-H | post-R1I-G | 236.50s | plateau 40.30s/17.04% concentrated; new star-score parent 4.80s/2.03% broad but C1; outgoing static edge 4.49s/1.90% | DECLINE another efficiency implementation |

Longitudinal carry-forward examples:

| Mechanism | R1I-D | R1I-F | R1I-H |
| --- | ---: | ---: | ---: |
| Full plateau reselection | 42.52s / 14.81% | 35.11s / 16.00% | 40.30s / 17.04% |
| Outgoing static compatibility | 4.89s / 1.70% | 4.58s / 2.09% | 4.49s exclusive edge / 1.90% |
| `coveragePlacementKey` | 10.32s / 3.59% | 2.48s / 1.13% | 2.53s / 1.07% |
| Placement map/index target | 7.08s map build plus 3.87s target lookups | 9.01s combined target | post-R1I-G builder 0.41s / 0.17%, below inventory threshold |

R1I-E and R1I-G removed the two successive broad mechanisms that cleared
their frozen bars. R1I-H finds no third broad region with a sufficient class
ceiling. The plateau family remains large but continues to be dominated by two
cases and by structural archive semantics.
