# DumboDB fieldDivergent results

## Scope and fixture lifecycle

These runs exercise only DumboDB revision
`8c8049898e512efa07c8fa913c49e90168ee5474`. MongoDB is not started and is not
an oracle for `fieldDivergent` behavior.

Each run owns its database. The harness creates the collection with
`mergeMode: "fieldDivergent"`, seeds the complete fixture, executes the
workload, stops and joins every writer, and only then reads the final state.
Failed and inconclusive databases are retained for investigation. The report
records the requested mode, exact server revision, fixture dimensions, outcome
accounting, final-state checks, and bounded failure samples.

## Deterministic branch reconciliation

The nine-case semantic matrix in `field-divergent-correctness.md` passes for
both inline documents and 8,192-byte payload documents. All 18 runs have a
`conclusivePass` verdict. The merging cases preserve the exact expected state;
the divergent cases reject the merge, preserve the destination state, and
record a conflict.

A negative control ran a convergent matrix row with `fieldTouched`. It failed
the `fieldDivergent` verdict oracle as expected, demonstrating that the matrix
detects a wrong collection mode.

The retained reports use these patterns:

- `evidence/dumbodb-field-divergent-matrix-inline-*.json`
- `evidence/dumbodb-field-divergent-matrix-oob-*.json`

## Concurrent preflights

Each row below ran 1,000 operations with 16 workers and seed 1 against both an
inline fixture and an 8,192-byte payload fixture. There were no rejected or
indeterminate outcomes in these preflights.

| Scenario | Inline | Out-of-band | Interpretation |
|---|---|---|---|
| Numeric CAS | pass | pass | Equal proposals may converge and coalesce. |
| Blind increment | pass | pass | Equal increments may converge and coalesce. |
| Identical set | pass | pass | Identical values may converge. |
| Same-field set | pass | pass | The final value identifies an issued write; this is a weak last-value oracle. |
| UUID CAS | fail | fail | Multiple distinct proposals from the same observed token were acknowledged. |
| Divergent numeric CAS | fail | fail | Multiple distinct proposals from the same observed generation were acknowledged. |
| Disjoint-field set | fail | fail | Acknowledged per-worker field values were absent or stale after writers stopped. |

The pass conditions deliberately allow convergent writes to coalesce. The
three failures are zero-tolerance correctness violations, not unexpected
coalescing and not rate comparisons. Identical-set supplies the convergent
counterpart to the divergent-value workloads.

The retained reports use these patterns:

- `evidence/dumbodb-field-divergent-inline-*-preflight.json`
- `evidence/dumbodb-field-divergent-oob-*-preflight.json`

The identical pass/fail pattern at both payload sizes rules out DumboDB's
inline versus out-of-band document representation as the cause.

## Sustained UUID CAS result

The 30-minute UUID CAS soak used 32 workers and seed 1. It completed 1,499,121
attempts:

| Outcome | Count |
|---|---:|
| Matched | 440,675 |
| No match | 1,056,848 |
| Rejected | 48 |
| Indeterminate | 1,550 |

Only 25,720 observed tokens were unique, and the ledger found 414,955 duplicate
matches. The `oneMatchPerObservedGeneration` check therefore failed. This is a
hard failure: distinct subtype-4 UUID proposals based on one token are
divergent, so at most one may be accepted.

All bounded error samples report server error 225, `session was taken over by a
newer connection on this lsid`. Session sweeping was delayed to one hour and
the server remained alive, distinguishing this from the separately tracked
session-sweeper panic. The lsid defect is tracked as `workspace-1bk.9.6.1`.

The complete report is
`evidence/dumbodb-field-divergent-uuid-cas-30m.json`.

## Current diagnosis

Explicit branch reconciliation implements the tested `fieldDivergent`
contract correctly. The ordinary concurrent update path does not enforce the
same contract: it acknowledges divergent writes from a shared base and loses
acknowledged disjoint fields. That server defect is tracked as
`workspace-1bk.9.4.1`. The harness keeps these failures visible and does not
weaken its oracle to make current DumboDB behavior pass.

## Invocation

Build the standalone runner, start the intended DumboDB revision, and give each
run a unique database. For example:

```sh
GOWORK=off go build -o /tmp/concurrency-field-divergent ./cmd/concurrency
/tmp/concurrency-field-divergent \
  -uri=mongodb://127.0.0.1:27018 \
  -scenario=uuid-cas \
  -duration=30m \
  -workers=32 \
  -seed=1 \
  -database=field_divergent_uuid_cas_run \
  -collection=documents \
  -merge-mode=fieldDivergent \
  -output=field-divergent-uuid-cas.json
```

Use `-payload-bytes=8192` for the out-of-band storage dimension. Matrix
scenario names are `field-divergent-matrix-` followed by a row name from the
correctness document, such as `same-field-different-values`.
