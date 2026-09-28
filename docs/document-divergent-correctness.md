# documentDivergent correctness oracle

## Scope

These tests exercise DumboDB collections created with
`mergeMode: "documentDivergent"`. MongoDB is not run and is not an oracle for
this mode. The verdict table in `workspace-1bk.9.8` is the source of truth.

For each document, `base` is the common ancestor and `ours` and `theirs` are
the two sides being reconciled. `documentDivergent` conflicts when both sides
changed the document and their complete resulting documents differ. It merges
when only one side changed or when both sides produced byte-identical canonical
BSON documents.

Equality is evaluated after resolving DumboDB's adaptive storage. Two equal
logical documents must compare by their canonical BSON bytes, not by an inline
value versus an out-of-band address. Field order and BSON types remain part of
the stored-byte contract.

The suite tests explicit branch merging separately from ordinary auto-commit
reconciliation and operation replay. A branch conflict does not imply that an
ordinary write returns an error: a refused operation may replay against the new
tip and either apply or return no match.

## Deterministic branch matrix

| Row | Scenario | Verdict | Required destination state |
|---:|---|---|---|
| 1 | Only one side changes the document | merge | The changed side is retained. |
| 2 | Each side changes a different field | conflict | The destination side is unchanged and a conflict is recorded. |
| 3 | Both sides set one field to the same value | merge | The identical resulting document is retained once. |
| 4 | Both sides set one field to the same value and one side changes another field | conflict | The destination side is unchanged and a conflict is recorded. |
| 5 | Both sides set one field to different values | conflict | The destination side is unchanged and a conflict is recorded. |
| 6 | One side modifies and the other deletes | conflict | The destination side is unchanged and a conflict is recorded. |
| 7 | Both sides add the same `_id` with identical content | merge | One byte-identical document is retained. |
| 8 | Both sides add the same `_id` with different content | conflict | The destination document is retained and a conflict is recorded. |
| 9 | Both sides delete the document | merge | The document remains absent. |

Rows 2 and 4 distinguish document granularity from `fieldDivergent`. Row 3
distinguishes the divergent trigger from both touched modes. Rows 3, 7, and 9
must run with 8,192-byte payloads as well as inline documents so pointer
representation cannot masquerade as document divergence.

## Global concurrent rules

All writers stop and join before final-state reconciliation. Outcome accounting
always balances:

```text
attempts = matched + noMatch + rejected + indeterminate
0 <= modified <= matched
```

Indeterminate operations are never assumed applied or unapplied. Conservation
checks affected by them are skipped, while independently evaluable structural
checks remain hard failures.

The branch matrix proves the policy verdict. Concurrent scenarios prove that
the ordinary write boundary invokes that policy and then handles refusal or
convergence correctly. Final state alone is not always a mode discriminator
because operation replay can serialize writes that initially conflicted.

## Conditional writes

### Convergent numeric compare-and-set

Contenders observe version `v`, filter on `v`, and propose the same complete
document with version `v + 1`. Proposals from one base are byte-identical, so
`documentDivergent` permits them to merge and coalesce. More than one client
may receive a match for one observed generation.

With no indeterminate outcomes:

```text
stored version = number of unique observed generations in the accepted chain
stored version <= matched
all matched edges are successive
observed generations form one complete chain
```

Duplicate matches are measured convergence, not a failure. A stored version
greater than matched, a non-successive edge, or a gap in the chain is a hard
failure.

### UUID compare-and-set

Each operation proposes a distinct subtype-4 UUID, so complete resulting
documents differ. At most one proposal from an observed token may match. The
loser is refused and replayed; its stale token filter then returns no match.

```text
stored applied = matched
modified = matched
at most one match per observed token
matched edges form one complete chain
the final token belongs to the final matched operation
```

### Divergent numeric compare-and-set

Each contender proposes a distinct value while incrementing a generation.
Complete documents differ, so the strict UUID-style single-winner chain rules
apply. This is the divergent counterpart to convergent numeric CAS.

## Unconditional writes and replay

### Blind increment

Two increments from the same base produce the same complete document, so they
may merge and coalesce without replay. Acknowledged matches may therefore
exceed the stored counter:

```text
0 <= stored version <= matched
modified = matched
matched = attempts - rejected - indeterminate
```

The coalesced count is reported. It is permitted behavior for this mode and
must not be presented as an implementation data-loss defect.

### Disjoint field set

Two workers changing different fields produce different complete documents,
so their explicit merge conflicts. An ordinary `_id`-filtered update can then
replay against the winner and serialize successfully. Every worker's final
field must equal its latest matched value. Missing or stale acknowledged fields
are hard failures.

The deterministic row is the granularity discriminator; the concurrent row
checks refusal/replay conservation. It must not require a client-visible error
when replay succeeded.

### Same-field and identical-value set

Different same-field values produce different complete documents and serialize
through conflict and replay. The final value must belong to a matched issued
operation. Identical complete documents may converge; all non-rejected attempts
must match and the common result must remain stored. A matched replay that did
not modify bytes is permitted.

## Whole-document ingestion discriminator

The field-level `identical-set` workload is insufficient to establish the
intended idempotent-ingestion behavior. A dedicated workload writes complete,
canonically ordered BSON documents from a synchronized common base:

- In the convergent phase, contenders write the same full document. Both may
  match, the merge must succeed, and the exact common document must remain.
- In the divergent phase, contenders write different full documents. They
  must not both be accepted from the same conditional base; the accepted
  operation chain and exact final document must agree.
- In the same-value-plus-disjoint phase, contenders share one equal field but
  differ elsewhere. This is whole-document divergence and must follow the
  divergent phase, not field-level convergence.

The workload uses operation identifiers and a document generation in its
filter so a refused loser becomes a conclusive no-match instead of silently
serializing. It runs with inline and 8,192-byte documents.

## Storage and evidence

Every deterministic row and concurrent scenario runs at payload sizes 0 and
8,192. Identical-result rows assert exact resolved BSON equality at both sizes;
conflict rows assert an unchanged destination and a retained conflict record.

Reports record the exact DumboDB revision, requested mode, seed, workers,
payload size, outcome accounting, final checks, convergence measurements, and
bounded errors. Tests run against DumboDB `fcc433c` or later. Missing or empty
reports fail the suite.
