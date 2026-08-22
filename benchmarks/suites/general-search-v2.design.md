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
`SHA-256("general-search-v2/public/" + caseID)`. Private holdouts publish a
private seed identifier, a SHA-256 commitment, and their requested descriptor;
their seed values are stored only in the GitHub Actions repository secret
`GENERAL_SEARCH_V2_PRIVATE_SEEDS`. The secret is a JSON object from private
seed ID to a positive `int64`. CI recomputes
`SHA-256("search-suite-generator-v2" + NUL + "private-seed" + NUL + privateSeedID + NUL + seed)`
and fails before materialization if any commitment, membership, or protected
case generation differs. The secret is written to a mode-0600 temporary file
only for that check and is then removed.

For every generated case, source pairs are eligible only when neither source
targets itself or the other source under the canonical star-matching rule.
All eligible ordered source pairs and each eligible target class are shuffled
from the case seed; their area is not used to rank selection. The lock records
realized instance and definition counts, source areas and rotation variants,
and target/filler area histograms as audit data rather than selection inputs.

Topology labels are template families, not mutually exclusive topology
classes: a concrete `holes` grid may also have articulation cells, and a
`two-lobes` grid may contain corridor cells. Each template has its own frozen
structural predicate, and those predicates are checked independently.

The v2 packing witness is score-blind and has a fixed 2,000,000-node cap. A
cap exhaustion is a fatal generation error, never an attempt rejection; only a
proved unpackable candidate may trigger a deterministic retry. The committed
public corpus is checked to complete without witness exhaustion.

The ten inherited static cases, including their craft interactions, remain in
the suite intentionally. They exercise the broader real-solver surface; the
36 v2 generated rows are the separately defined structural star-source corpus.

The rows were selected through structural coverage and catalog feasibility
only. No solver benchmark, score, search-node count, completion time, or
solver-produced layout was used to select them. The lock was frozen after
generator validation and the score-blind packing witness succeeded for all
public cases.
