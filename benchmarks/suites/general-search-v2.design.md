# general-search-v2 structural design

`general-search-v2` retains the ten static cases from `general-search-v1` and
adds 36 generated declarations from `search-suite-generator-v2`:

- 14 development cases
- 10 validation cases
- 6 public holdouts
- 6 private holdouts

The generated rows form a pairwise covering array across these frozen
descriptor dimensions: grid topology, density band, source multiplicity,
target overlap, copy symmetry, and rotation entropy. Each public and private
holdout independently covers every value in every dimension.

Public seeds are derived as the positive first 63 bits of
`SHA-256("general-search-v2/public/" + caseID)`. Private holdouts intentionally
publish only a private seed identifier and their requested descriptor.

The rows were selected through structural coverage and catalog feasibility
only. No solver benchmark, score, search-node count, completion time, or
solver-produced layout was used to select them. The lock was frozen after
generator validation and the score-blind packing witness succeeded for all
public cases.
