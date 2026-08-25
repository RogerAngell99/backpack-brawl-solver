# E1-A efficacy census and candidate-selection protocol

Status: frozen before official measurement.

## Question and immutable boundary

E1-A asks whether a different search policy can produce a semantically better
solution under the same node budget. Nodes, not wall time, are the comparison
currency.

The frozen solver baseline is:

```text
11644a7d88bd4e4bdd1f97977f8aad5e59391293
```

E1-A may add benchmark tooling, evidence, and documentation only. It must not
change solver behavior, materialize validation (`gsv2-027..036`), materialize
either holdout (`gsv2-001..012`), or inspect private seeds. Its pull request
must stop for manual review.

## Semantic comparison

Scores are compared exactly in the solver's canonical lexicographic order:

```text
PriorityCounts
CraftCount
StarCount
ItemCount
StarTargetBreadth
StarReciprocalPairs
StarSourceDefinitionDiversity
```

`LayoutKey` and `CanonicalLayoutHash` never count as efficacy. A different
layout at an identical seven-component score is a semantic tie and is reported
separately as `LAYOUT-ONLY CHANGE`.

The analyzer records the first differing component, candidate and baseline
values, signed candidate-minus-baseline delta, and (for `PriorityCounts`) the
first differing priority index. Difference levels are frozen as:

```text
0 PriorityCounts
1 Crafts
2 Stars
3 Items
4 StarTargetBreadth
5 StarReciprocalPairs
6 StarSourceDefinitionDiversity
7 semantic tie
```

Levels 0 through 2 are high-severity differences.

## Corpus and matrices

Only the locked GSV2 development role is materialized. The collector rejects
anything other than exactly `gsv2-013..026`.

All runs use:

```text
workers=1
repeat=1
operation-profile=false
repair-search=scenario
plateau-variant=default
TopN=benchmark default
```

The non-diagnostic quality matrix is authoritative for semantic comparison:

```text
general-search-v1: 14 cases x [250k, 1M] = 28 runs
v4:                14 cases x [1M]       = 14 runs
v5:                14 cases x [1M]       = 14 runs
v5.1:              14 cases x [1M]       = 14 runs
total quality runs                              70
```

`PhaseWork`, exact return accounting, incumbent events, and root diagnostics
are diagnostic-only report fields. Therefore the collector runs a diagnostic
twin of the same 70-run matrix for attribution only. Every diagnostic twin
must have the same semantic score, layout key, canonical hash, normal nodes,
configured budget, consumed budget, and unused nodes as its quality run. A
pair mismatch invalidates the entire bundle. Diagnostic elapsed time is never
used for candidate selection.

## Per-run census

The analyzer records:

- scenario, budget, variant, semantic score, layout/hash identity;
- configured, consumed, and unused normal nodes;
- execution budget and first-complete node;
- `SeedBest`, `SearchBest`, `PostRepairBest`, and `RefineBest`;
- the first checkpoint and exact incumbent-trace phase that produced the final
  semantic score;
- for every phase, charged nodes, returned nodes, total-budget share,
  incumbent before/after, semantic improvement count, and dead-spend status.

A phase is `dead_spend` for a run when it charges at least 5% of `MaxNodes` and
produces no strict semantic incumbent improvement. Returned capacity and final
unused nodes are reported as accounting facts, not automatically labelled
waste.

## Predeclared families

No new family may be added after observing official results without marking it
post-hoc. The frozen audit families are:

1. phase-budget allocation;
2. constellation root allocation;
3. constellation root selection;
4. packing completion;
5. repair allocation;
6. repair selection;
7. plateau/LNS;
8. seed portfolio.

Risk classes are:

```text
Q0 redistributes budget among existing paths
Q1 changes ranking, beam, scheduler, or candidate selection
Q2 changes structural state/archive/neighborhood generation
Q3 changes objective, score, admissibility, or bounds
```

Only Q0/Q1 may be selected by E1-A. Q2 requires a later protocol with stronger
evidence. Q3 is forbidden.

## Mechanical evidence gates

The following gates are evaluated on the 14 baseline diagnostic runs at 1M
unless a gate explicitly names a control comparison:

- **A — material dead spend:** a phase charges at least 5% of `MaxNodes` in at
  least 4 cases and has no strict semantic improvement in at least half of its
  material cases, with a minimum of 4 dead cases.
- **B — underutilization:** final unused nodes are at least 5% of `MaxNodes` in
  at least 4 cases.
- **C — existing policy:** a control has semantic wins in at least 3 distinct
  1M cases. An isolated control may be promoted only with zero high-severity
  losses and `wins - losses >= 2`.
- **D — matched-root starvation:** the same rooted-packing input completes
  under a control with a better root score while the baseline copy terminates
  for budget in at least 3 distinct cases.
- **E — repair relevance:** repair charges at least 5% in at least 4 cases,
  records strict semantic improvements in at least 4 cases, and first produces
  the final semantic score in at least 2 cases.

The existing controls have these causal classifications, frozen before data:

- `v4` versus `general-search-v1` isolates equal per-root allocation versus the
  progressive scheduler while retaining V4 construction and rooted packing;
- `v5` and `v5.1` change multiple root-selection/packing decisions and can
  identify a family, but cannot by themselves justify one exact mechanism.

## Exactly one decision

The analyzer applies the following tier order. Ties inside a tier use greater
scenario breadth, greater total material-node share, then stable candidate ID.

1. If isolated `v4` passes C, `PROMOTE` the exact equal-per-root allocation
   mechanism to opt-in E1-B.
2. Otherwise, if `v5` or `v5.1` passes C, return `NEED MORE EVIDENCE` for the
   highest-ranked control and require a one-variable shadow replay that
   separates root selection from packing beam/allocation.
3. Otherwise, if isolated matched-root evidence passes D, `PROMOTE` that exact
   root-allocation mechanism.
4. Otherwise, if repair passes E, return `NEED MORE EVIDENCE` for
   `repair-weighted-v1`. The exact missing probe must record, without affecting
   search: elite-base ID, stable neighborhood rank/key/operator, allocated and
   consumed nodes, first strict semantic-improvement node, best semantic delta,
   and whether that neighborhood first produced the final score.
5. Otherwise, if any phase passes A, return `NEED MORE EVIDENCE` for the
   highest-ranked phase and require a deterministic shadow budget-reallocation
   replay proving that removed quota reaches a live downstream frontier.
6. Otherwise, if B passes, return `NEED MORE EVIDENCE` for deterministic
   end-of-stage reclaim and require a probe proving a live chargeable DFS
   frontier exists when capacity is returned.
7. Otherwise return `DECLINE`.

This tiering deliberately prevents aggregate repair activity from being used
as proof that rank-weighted repair allocation works. E1-A may select that
mechanism only as `NEED MORE EVIDENCE` until the missing per-neighborhood
causal attribution exists.

## Accounting gates

Every official run must satisfy:

```text
NormalBudgetConfigured == MaxNodes
GlobalBudgetConsumed <= MaxNodes
NormalBudgetConsumed == GlobalBudgetConsumed
ExecutionBudgetConsumed <= ExecutionBudgetConfigured
sum(PhaseWork.ChargedNodes) == GlobalBudgetConsumed
all charged/reserved/consumed/returned values >= 0
```

The collector and analyzer reject run errors, duplicate/missing run keys,
unexpected variants, unexpected cases, catalog/hash drift, revision drift,
diagnostic-pair drift, or accounting failure.

## Freeze and provenance

Official collection uses a clean detached measurement clone at the frozen SHA
and a separate clean tooling checkout at the committed protocol/tooling SHA.
The record includes:

- exact solver and tooling SHAs and Git blob IDs;
- Go, OS, CPU, logical processor, visible RAM, and timezone information;
- catalog, suite manifest, suite lock, collector, analyzer, and protocol hashes;
- binary metadata and hash;
- exact matrix commands and materialized case list;
- raw file count/bytes, `RAW-SHA256SUMS.txt`, and manifest hash;
- read-only raw files, post-analysis hash validation, and
  `post_freeze_solver_runs=0`.

Any tooling or collection defect invalidates the bundle. After a fix, every
official run is recollected into a new empty bundle. Large raw reports remain
external; the pull request contains compact derived evidence and the raw
manifest needed for audit.

