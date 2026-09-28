# fieldDivergent correctness oracle

## Scope

These tests exercise DumboDB collections created with
`mergeMode: "fieldDivergent"`. MongoDB is not run and is not an oracle for this
mode. The oracle comes from DumboDB's three-way merge contract.

For each document, `base` is the common ancestor and `ours` and `theirs` are
the two sides being reconciled. A top-level field is divergent when both sides
changed it relative to `base` and the resulting values differ.

## Deterministic merge matrix

| Row | Scenario | Expected verdict | Required final state |
|---:|---|---|---|
| 1 | Only one side changes the document | merge | The changed side is retained. |
| 2 | Each side changes a different field | merge | Both independent changes are retained. |
| 3 | Both sides set one field to the same value | merge | The convergent value is retained once. |
| 4 | Both sides set one field to the same value and one side changes another field | merge | The convergent value and disjoint change are retained. |
| 5 | Both sides set one field to different values | conflict | Neither side is silently selected. |
| 6 | One side modifies and the other deletes | conflict | The conflict remains unresolved. |
| 7 | Both sides add the same `_id` with identical content | merge | One identical document is retained. |
| 8 | Both sides add the same `_id` with different content | conflict | Neither document is silently selected. |
| 9 | Both sides delete the document | merge | The document remains absent. |

A reported merge is a failure if its final state does not equal the required
state. A reported conflict is a failure if the merge changes the destination
document or loses the conflict record required for resolution.

## Concurrent workload oracles

Every workload runs only after creating its collection with `fieldDivergent`.
Workers must stop before final-state reconciliation begins. Outcome accounting
must balance even when the server reports conflicts or the client cannot
determine an outcome.

### Numeric compare-and-set

Two operations that observe generation `v` and both propose `v + 1` converge.
`fieldDivergent` permits them to merge, so multiple matches for one observed
generation and a stored version below the matched count are measurements, not
hard failures. The matched edges must still be successive, the stored version
must equal the length of the observed-generation chain, and no acknowledged
operation may produce a value outside that chain.

### UUID compare-and-set

Each operation proposes a distinct subtype-4 UUID. Two operations based on the
same observed token therefore diverge and must not both be accepted. There may
be at most one accepted operation for each observed token. The final token must
belong to the accepted operation recorded by `lastOperation`, `applied` must
equal the accepted-chain length, and the chain must be complete.

The losing client outcome depends on the write path: operation replay may
produce a successful command with no match, while merge reconciliation may
report a conflict. Both are conclusive losses. A second acknowledgement from
the same observed token is always a hard failure.

### Blind increment

Equal increments based on the same document generation can converge and
coalesce under this mode. The difference between acknowledged increments and
the stored counter is recorded as a convergence rate rather than a hard
failure. Rejected and indeterminate operations remain separate outcomes.

### Disjoint fields

Concurrent changes to different top-level fields must merge. Every worker's
final field must retain its latest acknowledged value. A missing field or an
older value is a hard failure.

### Same field

Writes of an identical value may merge. Writes of different values diverge and
must not both be silently accepted from the same base. Separate workloads are
required because a last-value-was-issued check alone cannot distinguish these
cases.

## Evidence rules

Every report records the exact DumboDB revision, requested merge mode, seed,
workers, payload size, client outcomes, conflict observations, and post-run
state. Deterministic semantic failures have zero tolerance. Scheduler-sensitive
convergence and conflict frequencies are reported as rates with sample counts.
