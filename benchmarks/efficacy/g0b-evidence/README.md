# G0-B blind V2 development expansion evidence

G0-B froze 36 new `star-source-structural-v2` development cases without
observing a solver-search outcome. IDs follow the official selection trace,
seeds are derived from `gsv2-devexp-v1`, and the frozen population is split
into two provisionally sealed 18-case waves.

## Result

| Gate | Result |
| --- | --- |
| V2 universe | 1,080 PASS |
| Attainable categorical pairs | 164 PASS |
| Historical core | 14 PASS |
| Expansion | 36 PASS |
| Core overlap | 0 PASS |
| Confirm-A / Confirm-B | 18 / 18 PASS |
| Combined pair coverage | 164 / 164 PASS |
| Core coverage delta | +36 PASS |
| Confirm-A pair coverage | 142 / 164 PASS |
| Confirm-B pair coverage | 139 / 164 PASS |
| Materialization | 36 / 36 PASS |
| Requested versus realized | 36 / 36 PASS |
| Structural packability witness | 36 / 36 PASS |
| Search benchmark runs | 0 |

The historical `general-search-v2` manifest and lock retain their frozen Git
blobs and LF-normalized hashes. The materialization audit contains only
`gsv2x-001..036`; validation and public/private holdouts were not members of
the G0-B materialization population.

## Freeze sequence

1. `3889bfbc9f71689f6b5c3caa7f868eac70958930` added the narrow orchestration,
   preflight, structural audit, and outcome-blind import guard.
2. `10b63784a8db0407b2c74fa48aef3937dd71284a` committed the descriptor, ID,
   seed, and A/B schedule before any materialization.
3. `3a2b672bdf72dde2a595df4651f003fbeda287a1` committed the two manifests,
   locks, realized-structure audit, and independent verifier.

The first materialization-phase invocation stopped in its integrity precheck
before creating a manifest, lock, scenario, or witness run. The precheck had
compared typed-JSON and generic-map JSON hashes with different key order. The
verifier was corrected without changing any frozen selection artifact; Git
then confirmed those artifacts were byte-identical to the selection-freeze
commit before the successful materialization invocation.

## Seal status

Confirm-A and Confirm-B are `provisionally_sealed`. G0-B did not run 250k,
500k, 1M, 5M, diagnostic, repair-attribution, or smoke search on either wave.
The authoritative multi-generator registry remains deferred to G0-D.

Decision: **KEEP**.
