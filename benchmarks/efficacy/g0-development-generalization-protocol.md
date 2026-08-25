# G0 development generalization hardening protocol

Status: frozen before any official expansion descriptor is selected.

Base revision for G0-A:

```text
37a30c481a60b23fef7e6a60edc7fa6deea2ff85
```

## Threat and scope

The generated cases `gsv2-013..026` have been reused for profiling,
hypothesis selection, and the E1-A efficacy census. They are development data
and must now be treated as contaminated by tuning. G0 creates two independent
development-confirmation populations selected only from structural
descriptors and deterministic seeds. Selection must not consult any solver
outcome.

G0 runs inside efficacy macro-stage 13, before E1-A1 instrumentation:

```text
E1-A (NEED MORE EVIDENCE)
  -> G0 development generalization hardening
  -> E1-A1I neutral causal instrumentation
  -> E1-A1E causal evidence
  -> possible E1-B candidate
```

G0 does not establish universal generalization. It reduces dependence on the
same fourteen cases and, through V3, on one generator distribution. Validation
and holdouts remain necessary.

The final planned development population is:

| Cohort | V2 | V3 | Total | Initial state |
| --- | ---: | ---: | ---: | --- |
| Core | 14 | 0 | 14 | open |
| Confirm A | 18 | 15 | 33 | sealed |
| Confirm B | 18 | 15 | 33 | sealed |
| Total | 50 | 30 | 80 | mixed |

Validation `gsv2-027..036`, public holdout `gsv2-001..006`, and private
holdout `gsv2-007..012` are outside G0 and stay closed.

## Immutable existing population

`benchmarks/suites/general-search-v2.json` remains byte-for-byte unchanged.
Its frozen Git blob ID and LF-normalized content SHA-256 are:

```text
Git blob: dddd10268eeaea28ecf6d03004d540235b61cd35
LF-normalized SHA-256:
5d1757c37580b04c9a85b738ea2672d8a0b3c8402c8ed5a509c8c42fd5d4b513
```

Its lock also remains unchanged. Its frozen identities are:

```text
Git blob: 61baa41bd6e64d0fade196ab7fd8548524b23afa
LF-normalized SHA-256:
96af8290e8741b4ef6f514b0df32820f8ccd241695eb5b1d671fc6cc2fd5aa6d
```

No `gsv2-037` or later case may be appended to that manifest. New populations
receive new names, manifests, locks, and hashes. An executable test enforces
the manifest and lock LF-normalized content hashes across checkout platforms.

## Four manual merge boundaries

G0 is four sequential pull requests:

1. **G0-A:** this protocol, selector, partitioner, seed derivation, tests, and
   governance. It contains no official expansion case.
2. **G0-B:** 36 V2 descriptors, IDs, seeds, materializations, two manifests,
   two locks, and structural audit. It contains zero benchmark search runs.
3. **G0-C:** generator V3, validator, structural witness, determinism and
   outcome-blindness tests. It contains no official V3 case.
4. **G0-D:** 30 V3 descriptors, IDs, seeds, materializations, A/B manifests and
   locks, the cohort registry, seal enforcement, and structural audit. It
   contains zero benchmark search runs.

Each phase starts only after the prior pull request is manually merged. The
next phase must fetch `origin/main` and verify its new SHA. Pull requests may
not be stacked, merged, or configured for auto-merge by the agent.

## Absolute outcome-blind boundary

Selection, partitioning, case-ID assignment, seed derivation, materialization,
and structural acceptance may observe only:

- the requested structural descriptor and descriptor schema;
- catalog structure and canonical star compatibility;
- grid geometry, item area, shape, and rotation variants;
- recipe metadata if a later frozen descriptor explicitly needs it;
- structural packability-witness status;
- deterministic generator control-flow validity.

They must never observe or import a path that supplies:

- `Score`, `PriorityCounts`, `CraftCount`, `StarCount`, or `LayoutKey`;
- normal-search solution/no-solution status;
- solver nodes, runtime, phase work, repair or archive activity;
- incumbent traces, pruning, search statistics, or benchmark reports.

The structural packability question, "can this inventory be packed into this
grid?", is allowed. Normal search at any efficacy budget is forbidden during
corpus construction. Witness node counts are diagnostics for witness validity
only and cannot rank, replace, or remove cases.

Files implementing the selector and partitioner are statically checked against
an explicit forbidden solver import and identifier list. G0-C extends the same
guard to V3 generator files.

## Generic descriptor contract

The version-1 selector consumes:

```text
ordered categorical schema
complete descriptor universe
existing core descriptor population
requested cohort size
domain-separation namespace
```

Every schema has at least two uniquely named dimensions and every dimension
has at least two unique categorical values. Names, values, and the namespace
must be non-empty and contain no NUL. A descriptor supplies exactly one valid
value per dimension in schema order. The universe must be non-empty and contain
no exact descriptor duplicate.

The canonical descriptor is compact JSON encoding of this ordered array:

```json
[
  ["dimension_name", "value"],
  ["next_dimension", "value"]
]
```

Universe input order has no meaning. The selector sorts canonical descriptors
bytewise and reports that order as `candidate_order`. Existing core
descriptors contribute to marginal counts, pairwise coverage, and distance,
but exact core descriptors are ineligible for reselection. Repeated core
descriptors would count repeatedly because the core is a population, though
the frozen V2 core has no exact duplicate.

The full-universe hash is:

```text
SHA256(
  namespace + NUL +
  "development-cohort-universe-v1" + NUL +
  canonicalDescriptor[0] + LF + ... + canonicalDescriptor[n-1]
)
```

## Frozen selection algorithm

Selection is greedy and deterministic. At each step, for each eligible
unselected descriptor `c`, form `P = core + already_selected + c` and the
lexicographic tuple:

```text
(
  marginal_imbalance(P),
  -new_pairwise_coverage(c),
  -minimum_hamming_distance(c, core + already_selected),
  SHA256(namespace + NUL + canonicalDescriptor(c)),
  canonicalDescriptor(c)
)
```

The smallest tuple wins.

For dimension `d` with `k_d` values, population size `n`, and category count
`count(d,v)`, marginal imbalance is the exact rational:

```text
sum over d,v of (k_d * count(d,v) - n)^2 / k_d^2
```

This is squared distance from a uniform marginal without floating-point
rounding. The trace stores the reduced exact rational.

Pairwise coverage is calculated over every unordered pair of dimensions.
`new_pairwise_coverage(c)` counts value pairs contributed by `c` that are not
already present in `core + already_selected`.

Hamming distance is the number of dimensions whose values differ. The metric
is the minimum distance from `c` to any core or already-selected descriptor;
if that population is empty, the distance is the dimension count.

The canonical descriptor fallback is reached only if SHA-256 collides. The
algorithm returns an exact number of unique non-core descriptors or fails; it
never silently substitutes a different method.

The selection audit contains algorithm version, namespace, full-universe hash,
canonical candidate order, selected indexes, and the winning tuple at every
step. Its compact JSON SHA-256 is the `selection_trace_hash` used by later
freeze records. Synthetic golden tests freeze the algorithm without selecting
the official population in G0-A.

## Frozen A/B partition algorithm

Partitioning receives the already selected descriptor population, Wave A
size, and its own namespace. It canonicalizes input order and rejects exact
duplicates.

Let `N` be total cohort size, `A` the required Wave A size, `T_v` the full
cohort count of one marginal value, and `a_v` its provisional Wave A count.
The exact marginal discrepancy is:

```text
sum over dimension d and value v of
  (N * a_v - A * T_v)^2 / k_d
```

For each dimensional value pair `p`, pairwise discrepancy is:

```text
sum over dimension pairs (i,j) and their values p of
  (N * a_p - A * T_p)^2 / (k_i * k_j)
```

Wave A is filled greedily. At each addition the smallest tuple wins:

```text
(
  provisional marginal discrepancy,
  provisional pairwise discrepancy,
  -minimum within-provisional-A Hamming distance,
  SHA256(partition_namespace + NUL + "wave-a" + NUL + canonical),
  canonical
)
```

Every greedy winner is recorded in `greedy_trace` with its zero-based step,
canonical candidate index, exact marginal discrepancy, exact pairwise
discrepancy, minimum Hamming distance, and domain-separated tie-break hash.
Thus `partition-trace.json` audits the initial Wave A construction without
requiring the implementation to be rerun.

Wave B is the complement. The complete partition objective is:

```text
(
  marginal discrepancy,
  pairwise discrepancy,
  -min(minimum Hamming within A, minimum Hamming within B),
  -(total within-A Hamming + total within-B Hamming),
  membership SHA-256
)
```

The membership hash is:

```text
SHA256(
  partition_namespace + NUL + "partition-v1" + NUL +
  sorted canonical Wave A descriptors joined by NUL
)
```

After the greedy split, the partitioner evaluates every one-for-one A/B swap,
applies the lexicographically best strict improvement, and repeats until no
one-swap improvement exists. Every greedy choice, applied swap, and objective
is recorded. Strict improvement in a total ordering guarantees termination.
Synthetic golden tests freeze the complete trace.

Official partition namespaces are frozen as:

```text
gsv2-devexp-partition-v1
gsv3-devexp-partition-v1
```

## Mechanical IDs and seeds

Official selection and seed namespaces are:

```text
V2: gsv2-devexp-v1
V3: gsv3-devexp-v1
```

V2 IDs are `gsv2x-001..gsv2x-036`. V3 IDs are
`gsv3x-001..gsv3x-030`. IDs follow selection-trace order; there is no manual
reordering.

For either family, the public seed is:

```text
digest = SHA256(namespace + NUL + "public-seed" + NUL + caseID)
seed   = big-endian uint64(digest[0:8]) AND MaxInt64
```

The versioned implementation returns an error for an empty or NUL-containing
namespace or case ID. Seed audits record case ID, namespace, derived seed, and
the derivation digest. No seed is typed manually into an official manifest.

The required construction order is:

```text
descriptor schedule -> IDs -> seeds -> commit/freeze -> materialization
```

Generating a pool and choosing appealing materializations is forbidden.

## V2 expansion contract

V2 uses the existing `star-source-structural-v2` family and
`search-suite-generator-v2` unchanged. Its schema order and categorical values
are:

| Dimension | Values |
| --- | --- |
| `grid_topology` | `full`, `bottleneck`, `holes`, `two-lobes`, `narrow-corridors` |
| `density_band` | `d60`, `d75`, `d90`, `d97` |
| `source_multiplicity` | `1/1`, `2/1`, `2/2` |
| `target_overlap` | `mostly-exclusive`, `mixed`, `mostly-shared` |
| `copy_symmetry` | `low`, `high` |
| `rotation_entropy` | `low`, `medium`, `high` |

The Cartesian universe has exactly 1,080 descriptors. The only core input is
the fourteen generated development descriptors `gsv2-013..026`; static
development scenarios are not part of this structural population.

G0-B selects 36 descriptors and partitions them 18/18. It creates:

```text
benchmarks/suites/general-search-v2-dev-confirm-a.json
benchmarks/suites/general-search-v2-dev-confirm-b.json
```

Both manifests use `role=development`, `workers=1`, the frozen efficacy
budgets and baseline policy, and contain no result field. Locks may contain
requested and realized structural descriptors, scenario hash, catalog hash,
and generator version; they contain no outcome.

V2 structural gates fixed before selection are:

- 36 unique IDs, seeds, selected descriptors, and materializations;
- no selected descriptor duplicates a core descriptor;
- every V2 category appears in `core + expansion`;
- within each combined-population marginal, maximum count minus minimum count
  is at most one;
- combined pairwise coverage is at least 148 of the 164 attainable categorical
  value pairs (the ceiling of 90%) and exceeds core coverage by at least 20
  pairs;
- each 18-case wave contains every category and covers at least 115 of the 164
  attainable value pairs (the ceiling of 70%);
- every materialization passes requested-versus-realized validation and the
  structural packability witness;
- `benchmark_scenario_runs=0`.

The deliberately conservative pairwise thresholds prevent post-selection
gate tuning while allowing finite-wave combinatorial limits. Reports publish
the actual numbers without claiming real-user representativeness.

## Materialization failure and immutability

V2 retains its frozen 64 deterministic materialization attempts and 2,000,000
node structural witness. V3 freezes the same attempt and witness ceilings.

If any descriptor-plus-seed fails to materialize under its generator limit,
the case is not removed and the next descriptor is not substituted. Before
any solver outcome exists, the entire cohort proposal may be invalidated, the
protocol or generator version corrected in a new reviewed commit, and the
entire official population regenerated.

After the first solver outcome on any official population version, the
population is immutable. Difficulty, runtime, no solution at an efficacy
budget, repair inactivity, unexpected score, or an uninteresting effect are
never valid removal reasons.

An objective structural defect is limited to descriptor violation, invalid
manifest, hash mismatch, nondeterminism, structural unpackability, or invalid
catalog reference. If discovered after an outcome was observed, the whole
population version is marked contaminated/invalid and replaced by a new
version; an individual case is never silently swapped.

## V3 independent generator contract

G0-C introduces `search-suite-generator-v3` and family
`star-source-graph-v3`. V3 changes the conceptual distribution from V2's two
fixed sources and A-only/B-only/shared targets to two or three sources and an
explicit target-to-source compatibility graph.

The V3 descriptor is a separate type with exactly these fields and values:

| Dimension | Values |
| --- | --- |
| Grid topology | `full`, `bottleneck`, `holes`, `two-lobes`, `narrow-corridors` |
| Density | `sparse`, `dense`, `very-dense` |
| Source count | `2`, `3` |
| Source copies | `singleton`, `mixed` |
| Compatibility graph | `mostly-exclusive`, `mixed`, `mostly-shared` |
| Target count | `small`, `large` |
| Filler symmetry | `low`, `high` |
| Rotation entropy | `low`, `high` |

The Cartesian universe has exactly 1,440 descriptors. V2 entries require a V2
descriptor and no V3 descriptor; V3 entries require a V3 descriptor and no V2
descriptor. V1 and historical V2 materialization must remain unchanged.

V3 density bands are inclusive realized inventory-area basis points:

```text
sparse:     6000..7200
dense:      8000..9000
very-dense: 9300..9800
```

Selected source definitions have at least one star, valid shapes, and valid
rotation metadata. For every selected source pair, neither source may target
itself or another selected source under canonical item/type/alias matching.
The scenario contains exactly the requested number of star-bearing
definitions; targets and fillers have no stars.

`singleton` gives every source one copy. `mixed` gives one source two copies
and all others one; the duplicated source is chosen by a seed-domain-separated
shuffle, never manually.

Each target definition has one inventory instance. Its degree is the number of
selected sources able to target it. For three sources, the fixed counts below
are degree-1 / degree-2 / degree-3:

| Target band | Profile | Counts |
| --- | --- | --- |
| small (6) | mostly-exclusive | 4 / 1 / 1 |
| small (6) | mixed | 2 / 2 / 2 |
| small (6) | mostly-shared | 1 / 2 / 3 |
| large (10) | mostly-exclusive | 7 / 2 / 1 |
| large (10) | mixed | 3 / 4 / 3 |
| large (10) | mostly-shared | 2 / 3 / 5 |

For two sources, degree-1 is exclusive and degree-2 is shared:

| Target band | Profile | Degree-1 / degree-2 |
| --- | --- | --- |
| small (6) | mostly-exclusive | 5 / 1 |
| small (6) | mixed | 3 / 3 |
| small (6) | mostly-shared | 1 / 5 |
| large (10) | mostly-exclusive | 8 / 2 |
| large (10) | mixed | 5 / 5 |
| large (10) | mostly-shared | 2 / 8 |

Exclusive targets are divided among sources as evenly as mathematically
possible; any remainder assignment uses a seed-domain-separated shuffle.

Fillers are structurally neutral: no selected source targets them, they have
no stars, and they introduce no source definition. Low filler symmetry gives
every filler definition one copy. High filler symmetry requires at least two
definitions with count at least two and at least 60% of filler instances to
belong to repeated definitions.

Rotation entropy is measured on filler instances. Low requires every filler
instance's definition to have exactly one distinct geometric variant. High
requires at least 50% of filler instances to come from definitions with at
least two variants and at least 25% to come from definitions with at least
three variants. Fractions use ceiling division, and both quotas must be met.

The priority list contains exactly one `star_source:<itemID>` entry per selected
source under outgoing-per-instance-v3 semantics. Priority order is a
seed-domain-separated shuffle. V3 never orders sources by an observed result
or human difficulty judgment.

V3 uses 64 deterministic materialization attempts and a 2,000,000-node
structural packability witness. Witness exhaustion is fatal, not a rejection
that advances to another attempt. Other structurally valid control-flow
rejections may advance the deterministic attempt. Witness node count is absent
from sampling and locking.

G0-C may use synthetic test seeds but selects no official descriptor or ID.

## V3 official selection gates

G0-D selects 30 V3 descriptors and partitions them 15/15. With no V3 core, the
selected marginal counts must be exact:

```text
topology:       6 each
density:       10 each
source count:  15 / 15
source copies: 15 / 15
compatibility: 10 each
target count:  15 / 15
symmetry:      15 / 15
rotation:      15 / 15
```

Each wave must have topology count 3 each, density count 5 each,
compatibility count 5 each, and each binary marginal count difference at most
one. Each wave must contain every category and cover at least 123 of the 189
attainable V3 categorical value pairs (the ceiling of 65%). The official audit
publishes the exact pairwise count and both-wave distance distributions.

All 30 IDs, seeds, descriptors, and materializations must be unique and
reproducible. Every requested-versus-realized structural validation and
packability witness must pass. `benchmark_scenario_runs=0` is mandatory.

## Structural audit and freeze artifacts

G0-B and G0-D each produce, in a committed audit directory:

```text
descriptor-marginals.csv
pairwise-coverage.csv
distance-summary.csv
realized-structure.csv
seed-audit.txt
materialization-audit.txt
full-universe-hash.txt
candidate-order.txt
selected-indexes.txt
selection-trace.json
partition-trace.json
freeze-record.txt
```

The freeze record includes manifest and lock Git blob IDs plus LF-normalized
content SHA-256 values, sampler and generator commits, catalog Git blob,
universe hash, selection trace hash, partition trace hash, and seed-derivation
hash. It explicitly records zero benchmark scenario runs. Manifests and
registries contain no output fields.

## Cohort states and seal governance

Development has three methodological states:

| State | May guide tuning? | Blind? |
| --- | --- | --- |
| `open-development` | yes | no |
| `sealed-development-confirmation` | no | yes |
| `consumed-development-confirmation` | yes, after consumption | no |

Core starts open. Confirm A and Confirm B start sealed. Structural descriptor
inspection, seed derivation, scenario materialization, requested-versus-realized
validation, and packability validation do not consume a cohort. The first
normal solver-search outcome that can reveal quality, difficulty, or search
behavior consumes it irreversibly.

G0-D creates `benchmarks/suites/development-cohorts-v1.json` and collector
enforcement. A sealed cohort cannot be searched unless a matching unseal
record already exists in the commit being measured.

Confirm A may be opened once, only after hypothesis, instrumentation, analysis,
and decision rules are frozen. Its unseal record includes:

```text
cohort
purpose
solver_sha
probe_sha
protocol_sha
analysis_sha
decision_rule_sha
opened_by_protocol_commit
```

Confirm B additionally requires one concrete frozen candidate:

```text
candidate_sha
variant
parameters
quality_gates
```

Every confirmation raw report records cohort ID, unseal protocol commit,
unseal record hash, and whether it is the first opening. After an opening, the
registry state advances to consumed/open development; it can never be called
blind again.

If Confirm A falsifies the causal hypothesis, it is not retuned and rerun as
blind. If Confirm B rejects a candidate, its formula is not changed and tested
again on B. A later candidate needs a newly preselected Confirm C. Validation
is used only after a candidate survives Confirm B, and a validation failure is
a candidate failure rather than tuning input.

Confirmation cohorts must not be spent on retrospective efficiency profiling
while sealed. Such auditing is allowed only after efficacy has already consumed
the cohort.

## Required CI

Every G0 pull request runs:

```text
gofmt -l .
git diff --check
go test ./...
go test -tags searchprofile ./...
go test -race ./internal/solver/...
go test -race -tags searchprofile ./internal/solver/...
suite validation and lock reproduction
npm ci
npm run build
```

G0-C and G0-D add generator determinism, V1/V2 historical reproduction,
wrong-descriptor rejection, outcome-blind import guard, structural lock
reproduction, and sampler/partition golden hashes.

## G0 completion gate

G0 completes only when the original V2 suite is unchanged; the selector,
seeds, and partitions reproduce exactly; 36 V2 and 30 V3 cases pass all
structural gates; Confirm A and B each contain 33 sealed cases; validation and
holdouts have not been materialized; and confirmation benchmark search runs
remain zero.

At that point the project resumes E1-A1I on Core14. Confirm A is reserved for
one frozen causal confirmation, Confirm B for one frozen concrete candidate,
and validation and holdouts remain downstream.
