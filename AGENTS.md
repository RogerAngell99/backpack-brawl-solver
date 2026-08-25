# Repository Agent Instructions

## MANDATORY MANUAL MERGE BOUNDARY

- Never merge a pull request.
- Never enable auto-merge.
- Never push directly to `main`.
- Never update `main` or move its ref.
- Every implementation or profiling task ends by opening a pull request and stopping.
- A dependent phase may start only after the user manually merges the preceding pull request and the new `main` SHA is fetched and verified.
- Do not create stacked pull requests unless the user explicitly requests them.
- "Approved", "looks good", "review passed", or CI success do not authorize a merge.
- Only an explicit user instruction saying to merge authorizes a merge.

End every pull-request handoff with this status block:

```text
STATUS: READY FOR MANUAL REVIEW

PR: #...
Base SHA: ...
Head SHA: ...
Decision: PROMOTE / KEEP / NEED MORE EVIDENCE / DECLINE / REVERT
CI: PASS / FAIL / PENDING
Solver source changed: yes/no
Evidence frozen: yes/no

No merge or auto-merge was performed.
STOP: awaiting manual owner review.
```
