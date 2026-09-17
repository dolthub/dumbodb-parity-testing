# documentTouched correctness oracle

## Scope

These tests exercise DumboDB collections created with
`mergeMode: "documentTouched"`. MongoDB is not run and is not an oracle for
this mode. The verdict table in `workspace-1bk.9.8` is the source of truth.

For each document, `base` is the common ancestor and `ours` and `theirs` are
the two sides being reconciled. `documentTouched` conflicts whenever both
sides changed the document relative to `base`, regardless of whether they
changed different fields or produced identical content.

The suite tests two paths separately:

- Explicit `doltMerge` tests the branch-merge verdict and conflict record.
- Ordinary writes test the auto-commit boundary, refusal, and operation replay.

A merge conflict and a client-visible error are not equivalent. An ordinary
write refused at its boundary is replayed against the new tip. The replay can
return no match or can apply successfully, depending on its filter.

## Deterministic branch matrix

| Row | Scenario | Verdict | Required destination state |
|---:|---|---|---|
| 1 | Only one side changes the document | merge | The changed side is retained. |
| 2 | Each side changes a different field | conflict | The destination side is unchanged and a conflict is recorded. |
| 3 | Both sides set one field to the same value | conflict | The destination side is unchanged and a conflict is recorded. |
| 4 | Both sides set one field to the same value and one side changes another field | conflict | The destination side is unchanged and a conflict is recorded. |
| 5 | Both sides set one field to different values | conflict | The destination side is unchanged and a conflict is recorded. |
| 6 | One side modifies and the other deletes | conflict | The destination side is unchanged and a conflict is recorded. |
| 7 | Both sides add the same `_id` with identical content | conflict | The destination document is retained and a conflict is recorded. |
| 8 | Both sides add the same `_id` with different content | conflict | The destination document is retained and a conflict is recorded. |
| 9 | Both sides delete the document | conflict | The destination remains absent and a conflict is recorded. |

Row 1 is the only merge. Rows 2 and 3 are the primary negative controls:
`fieldTouched` merges row 2, while `fieldDivergent` and
`documentDivergent` merge row 3.

## Global concurrent rules

Workers stop and join before final-state reconciliation. Every attempt has
exactly one terminal client-visible outcome:

```text
attempts = matched + noMatch + rejected + indeterminate
0 <= modified <= matched
```

An indeterminate outcome is never counted as applied or unapplied unless its
operation identity is recovered from stored state. Conservation checks that
cannot survive an indeterminate outcome are skipped, while independently
evaluable structural checks remain hard failures.

The suite records internal policy coverage separately from the public result.
The branch matrix proves that a bilateral touch conflicts. An ordinary-write
scenario proves that the resulting refusal is replayed correctly. It must not
require a client error merely because the branch matrix says `conflict`.

## Conditional writes

### Numeric compare-and-set

Each attempt observes version `v`, filters on that value, and proposes
`v + 1`. Although competing proposals converge to the same document value,
`documentTouched` refuses the second bilateral touch. Replaying the loser
against the new tip makes its stale filter return a clean no-match.

With no indeterminate outcomes:

```text
stored version = matched
modified = matched
at most one match per observed generation
matched edges form one complete successive chain
```

At least one match and, under a contention preflight, at least one no-match are
required so a completely idle or serialized-away test cannot pass vacuously.

### UUID compare-and-set

Each attempt proposes a distinct subtype-4 UUID and increments `applied` in the
same conditional update. The strict CAS rules apply:

```text
stored applied = matched
modified = matched
at most one match per observed token
the matched edges form a complete chain
the final token belongs to the recorded final matched operation
```

### Divergent numeric compare-and-set

Each attempt proposes a distinct value while incrementing a generation. It has
the same single-winner chain rules as UUID CAS. This workload proves that the
oracle does not depend on proposals being byte-identical.

## Replay-safe unconditional writes

### Blind increment

The filter contains only `_id`. A refused increment is replayed against the
new tip, where it still matches and increments again from the current value.
For conclusive outcomes:

```text
noMatch = 0
stored version = matched
modified = matched
matched = attempts - rejected
```

Rejected operations remain conclusive losses and are not included in the
stored count. The sustained hot-document test also requires forward progress:
matched must increase throughout the workload, throughput must remain nonzero,
and retry exhaustion must be reported rather than hidden.

### Disjoint field set

The explicit branch row conflicts because both sides touched one document.
Ordinary writes use an `_id` filter, so a refused write can replay and serialize
after the winner. Every worker's final field must equal that worker's latest
matched value. A stale or missing acknowledged field is a hard failure.

This final state does not by itself distinguish `documentTouched` from a field
mode; the branch row supplies the granularity discriminator. The ordinary path
checks replay conservation and guards against bypassing reconciliation.

### Same-field and identical-value set

An unconditional same-field write can replay and serialize. The final value
must identify a matched issued operation. For an identical-value workload, all
non-rejected attempts must match and the convergent value must remain stored;
matched-but-unmodified replay results are permitted because the winner may
already have stored that value.

These workloads measure replay safety and progress. Numeric CAS is the
ordinary-write discriminator for the rule that even convergent bilateral
touches conflict.

## Storage and evidence

Every deterministic row and concurrent scenario runs with an inline payload
and an 8,192-byte payload. The verdict, exact final document, and conflict
record must be identical across storage representations.

Every report records the exact DumboDB revision, requested merge mode, seed,
worker count, payload size, outcome accounting, final checks, and bounded error
samples. Tests run against DumboDB `fcc433c` or later. Missing or empty reports
are failures, not expected-negative results.
