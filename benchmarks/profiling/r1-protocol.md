# R1 — Post-H2a reprofile and next-bottleneck selection

## Status and measured source

R1 is a diagnostic, evidence-only stage after H2a. The frozen measured source
is `f94bb285c8e895696c76a1beacd4942b35a056d1` on `main`.

Do not silently substitute a later `main` commit. If a new executable commit
must be measured, update this protocol and repeat the protocol commit before
collecting data.

H2a reduced `placementKey` cumulative CPU from 21.25% to 5.34% and
canonical-copy-order cumulative CPU from 19.74% to 2.87%. Its pre-H2a hotspot
ranking is therefore not a valid basis for selecting the next optimization.

## Scope

Permitted work is existing-harness profiling, operation-count analysis,
CPU/heap profiling, caller/source attribution, corroborating microbenchmarks,
and evidence/decision documentation. This stage may select exactly one future
experiment, say that no further efficiency change is warranted, or request an
instrumentation-only R1I stage.

R1 must not implement H2b, P2-global, caching, canonical ranks, new pruning,
ranking or beam changes, scheduler redesign, generator/suite changes, or any
solver optimization. Validation and holdouts remain closed.

## Provenance and collection environment

Measure only a separate clean clone detached at the frozen SHA. Build fresh
normal and `searchprofile` executables with `go build -buildvcs=true`; both
must report `vcs.revision=f94bb285c8e895696c76a1beacd4942b35a056d1` and
`vcs.modified=false` in `go version -m`. Any missing, divergent, or modified
metadata aborts collection. Record executable SHA-256 values, full build
metadata, Go/OS/CPU data, and suite catalog/lock hashes.

The raw artifacts are frozen and read-only after recursive SHA-256 collection.
Raw `.pprof`, binaries, materialized suites, and large raw JSON are not
versioned; compact summaries/extracts and their raw-artifact hashes are.

## Corpus

Use only `general-search-v2` development generated cases `gsv2-013` through
`gsv2-026`, with an unchanged lock, `workers=1`, and the existing harness.
Do not materialize or measure validation (`027–036`), public holdout
(`001–006`), or private holdout (`007–012`) cases.

## Pass A — operation counts

Collect all fourteen development cases with `searchprofile` at GSV1 250k and
1M, then produce a compact operation summary. Also collect the fourteen cases
at V4 1M as a policy/trajectory control. Inspect feasibility calls, remaining
instances, option checks, candidate loop work, canonical residual work,
rooted packing counters, scheduler counters, dedup, and state-key work.

The main question for P2-global is not its historical scan count alone:
`packingFeasibility` must still have material CPU in its own scan body after
H2a, with large rescans across multiple cases. If scans remain numerous but
CPU moves elsewhere, scan multiplicity alone is not sufficient evidence.

## Pass B — CPU and heap

Use normal builds only, without operation profiling, on the historical GSV1
1M six-case slice `013,015,016,018,021,024`. Preserve flat top, cumulative
top, call tree, allocation-object, and allocation-space pprof extracts. Use
caller edges or a non-overlapping parent/self region for each hypothesis; do
not add nested percentages.

Initial source inspection may include packing-seed feasibility, canonical
residual/key construction, plateau/archive selection, rooted MRV feasibility,
sorting/ranking, and allocation owners. GC frames are consequences, not
candidate optimizations: follow allocation profiles to their owner.

If a credible new whole-program hotspot emerges, collect one CPU profile per
case in the six-case slice to measure scenario spread. A signal present in at
least four of six cases is especially strong. Use a supplemental 5M run only
when the 1M profile has insufficient samples, an unclear top hotspot, or a
deep-search-only signal; label it supplemental rather than replacing 1M.

## Attribution and decision gate

For every named hypothesis, document: a non-overlapping addressable CPU
region or caller edge, allocation signal, concrete deterministic operation
cause, scenario spread, implementation complexity/equivalence risk, and the
CPU-sample heuristic `f × r` (addressable non-overlapping fraction times a
plausible reduction). It is a heuristic, not a wall-clock prediction.

Promote a structural optimization only when all four are present:

1. whole-program CPU evidence;
2. a concrete operational cause;
3. generality across scenarios; and
4. a plausible whole-program benefit of roughly 3% or more that justifies its
   complexity.

H2b bears the burden of showing that its canonical/key residual is still
material. Plateau/archive must be separated into sorting, copying, allocation,
scoring, observation, or frequency before any optimization; otherwise the
answer is `NEED R1I INSTRUMENTATION`.

## Deliverables and commits

Version:

- `r1-evidence/README.md` with provenance, artifact locations/hashes, and
  review guide;
- compact GSV1/V4 operation summaries;
- CPU/heap/top/tree/targeted extracts;
- per-scenario attribution CSV when a credible candidate requires it;
- `r1-findings.md` with a single decision.

Keep three commits, in order:

1. `docs(profiling): freeze post-H2a reprofile protocol`
2. `docs(profiling): add post-H2a reprofile evidence`
3. `docs(profiling): record post-H2a bottleneck decision`

The evidence must support exactly one of `PROMOTE <named experiment>`,
`NO FURTHER EFFICIENCY CHANGE`, or `NEED R1I INSTRUMENTATION`. No executable
code belongs to this branch.
