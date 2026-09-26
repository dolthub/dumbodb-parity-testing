# DumboDB fieldTouched concurrency baseline

These runs exercise DumboDB revision
`8c8049898e512efa07c8fa913c49e90168ee5474`. Each run creates its collection
with `mergeMode: "fieldTouched"` before seeding data. MongoDB is not run.

All final-state checks occur after the workers stop. Each preflight uses 16
workers, 1,000 attempts, and seed 1.

| Scenario | Matched | Stored or retained state | Verdict |
|---|---:|---|---|
| CAS | 265 | version 82; 183 duplicate matches | failed |
| UUID CAS | 169 | applied 77; 92 duplicate matches | failed |
| Blind increment | 1,000 | version 145 | failed |
| Disjoint set | 1,000 | 13 of 16 worker fields lost their latest acknowledgement | failed |
| Same-field set | 1,000 | final value was issued | conclusive pass |

The four failures are hard correctness failures, not statistical differences.
There were no rejected or indeterminate operations. In particular, blind
increment acknowledged and reported modification for all 1,000 writes while
only 145 increments remained in the final document.

The same-field pass has a weaker last-writer oracle: it establishes only that
the final value was issued and does not show that `fieldTouched` reconciliation
ran.

Raw reports:

- `evidence/dumbodb-field-touched-cas-preflight.json`
- `evidence/dumbodb-field-touched-uuid-cas-preflight.json`
- `evidence/dumbodb-field-touched-blind-inc-preflight.json`
- `evidence/dumbodb-field-touched-disjoint-set-preflight.json`
- `evidence/dumbodb-field-touched-same-set-preflight.json`
