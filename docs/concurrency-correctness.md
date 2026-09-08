# Concurrency Correctness Harness

## Scope

The harness characterizes MongoDB 8.0 behavior and runs validated workloads
against DumboDB. MongoDB applies each single-document write atomically; it does
not expose a three-way document merge policy. DumboDB mode-specific phases use
the same lifecycle while applying merge-mode-specific correctness oracles.

DumboDB now defaults a collection to `fieldTouched`, which preserves the same
CAS safety property. The MongoDB results remain the behavioral baseline for
that comparison; `fieldTouched` is not a name for MongoDB behavior, and a
DumboDB run passing these workloads is evidence about DumboDB, not a claim that
the two implementations agree on everything.

## Terms

An attempt is one command sent by one worker. Every attempt has exactly one
terminal client-visible outcome:

- `matched`: the command succeeded and matched one document.
- `noMatch`: the command succeeded and matched no document.
- `rejected`: the server definitively rejected the operation before applying it.
- `indeterminate`: the driver could not establish whether the server applied the
  operation, including timeout and connection loss.

`modified` is an attribute of a matched outcome, not a separate terminal
outcome. An update may match a document without changing its stored value.

The harness must not count an indeterminate outcome as either applied or unapplied. Such
an outcome is indeterminate unless a scenario-specific operation identifier can
be recovered from stored state.

## Global accounting laws

For every completed run:

```text
attempts = matched + noMatch + rejected + indeterminate
0 <= modified <= matched
```

Every worker must stop producing attempts before final state is read. All
issued operations must reach a terminal client-visible outcome before final
reconciliation starts.

A terminal client-visible outcome does not prove that the server has quiesced.
In particular, a command canceled after it was sent can still be applied by the
server after the driver returns. On interruption, the runner stops issuing new
work, allows a bounded grace period for in-flight server work, and then reads
final state with a fresh bounded context. The report is marked truncated and
is not a conclusive pass. The grace period reduces but cannot eliminate the
uncertainty from an indeterminate client response, so the run database is
preserved for investigation.

The runner fails on an accounting imbalance, an unreadable final state, an
unexpected document shape, or a violated scenario invariant. Statistical
comparisons never soften these failures.

Every report has one of three verdicts. `conclusivePass` means every evaluable
hard check passed with no indeterminate operations or interruption. `failed`
means an evaluable zero-tolerance invariant failed. `inconclusive` means no
evaluable invariant failed, but interruption or an indeterminate operation made
exact conservation unknowable. Such conservation checks are marked skipped,
and the command exits with status 3 rather than the failure status 1.

## Initial scenarios

### CAS increment

The document starts as:

```text
{ _id: "counter", version: 0 }
```

Each attempt first observes a version and submits:

```text
updateOne(
  { _id: "counter", version: observed },
  { $inc: { version: 1 } }
)
```

For acknowledged responses:

```text
matched is either 0 or 1
modified = matched
final version = initial version + matched
```

The last equation is the primary CAS conservation law. It is independent of
the ratio of matches to no-matches and therefore independent of scheduler
timing. Two attempts based on the same observed version may race, but at most
one may match.

### Blind increment

Each attempt submits `$inc: { version: 1 }` using only the document identity as
its filter. With no indeterminate outcomes:

```text
matched = attempts - rejected
modified = matched
final version = initial version + matched
```

This scenario verifies preservation of acknowledged commutative writes under
contention.

### UUID compare-and-set

The document holds a binary subtype-4 UUID token and an integer applied count.
Each attempt reads the current token and submits a conditional update that
replaces it with a deterministic unique UUID and increments applied in the same
atomic update.

After all writers drain:

    final applied = matched
    modified = matched
    final token is a valid binary subtype-4 UUID
    final token belongs to a matched update

This exercises compare-and-set without convergent result values. Two contenders
that observe the same token propose different replacements, and at most one may
match.

### Disjoint field set

Workers own stable top-level fields in one shared document and write a
monotonic sequence value to their field. The final value of each worker field
must equal that worker's greatest acknowledged sequence number. This verifies
that acknowledged writes to different fields are not lost. A worker issues one
operation at a time, so its acknowledged sequence slot cannot be overwritten
by an earlier in-flight operation.

### Same field set

Workers write unique operation sequence values to one shared field. MongoDB may
serialize them in any order, so the final value need not be the greatest issued
sequence number. The final value must identify one acknowledged matched write,
and every matched response must be internally valid. This scenario measures
contention outcomes but cannot by itself prove that intermediate overwritten
writes were durable.

## Workload dimensions

The MongoDB and current DumboDB workloads vary:

- worker count;
- small inline-shaped documents and large payload documents;
- seed-driven CAS read/update delay;
- run duration and operation limit;
- deterministic workload seed.

Multiple clients, explicit connection-pool sizing, and synchronized burst
release are planned dimensions. The current runner uses one client with the
driver's default pool and continuously releases operations.

Document size is a workload dimension even though MongoDB has no DumboDB
inline/out-of-band storage boundary. Both products receive the same generated
documents.

## Statistics

Hard invariants are evaluated per run and have zero tolerance.

Scheduler-sensitive observations are reported as rates with their sample size:

- CAS match and no-match rate;
- operations per second;
- response latency distribution;
- rejected and indeterminate rate;
- observed contention-window width.

Latency covers the complete read-and-update attempt for CAS scenarios and only
the update command for blind increment and field-set scenarios. Reports record
this scope explicitly, so latency distributions with different scopes are not
treated as comparable. Throughput uses only the worker execution window; setup,
final reconciliation, and interruption grace time are excluded.

MongoDB and DumboDB raw counts are not required to match. Comparisons use
normalized rates and confidence intervals. A statistical difference is reported
separately from a correctness failure.

No confidence threshold may be chosen until the MongoDB baseline contains at
least one sustained 30-minute run and hundreds of thousands of attempts for the
scenario under evaluation.

## Reproducibility and evidence

Every run records:

- target URI with credentials removed;
- server product and version;
- start time and duration;
- scenario and all workload dimensions;
- PRNG seed;
- aggregate outcome counts;
- final-state observations;
- invariant results;
- bounded examples of failed or indeterminate operations.

The generator must derive all workload choices from the recorded seed. Runtime
scheduling is not reproducible, so a seed reproduces inputs and coordination
strategy, not an exact interleaving.

## Phase status

The MongoDB-only phase gate required the runner to:

1. complete the initial scenario set;
2. balance every accounting equation;
3. detect an intentionally corrupted ledger and every scenario's final state;
4. emit bounded machine-readable evidence;
5. complete a sustained MongoDB 8.0.28 characterization run.

That gate is complete. The `fieldTouched` baseline explicitly configures its
collection mode. The `fieldDivergent` phase now covers deterministic branch
reconciliation, concurrent workloads, both adaptive storage paths, and a
30-minute soak. Its contract and results are documented in
`field-divergent-correctness.md` and `dumbodb-field-divergent-results.md`.
Coverage of the remaining configurable modes is subsequent work.

## DumboDB status

DumboDB failed these workloads when they were first run against it, and passes
them now. Both results are kept: `evidence/dumbodb-current-cas-30m.json` is the
failing run and `evidence/dumbodb-fixed-cas-30m.json` the passing one, so the
difference is inspectable rather than asserted. See `dumbodb-cas-results.md`.

Not yet covered, and the reason the mode axis is not the whole remaining story:
every scenario here reconciles at the end of the command. A fork that outlives
the command -- `--session-isolation`, or an explicit transaction -- acknowledges
a write before its boundary runs, so conservation there has to be counted
against acknowledged BOUNDARIES rather than acknowledged writes. That needs its
own accounting, not a new expectation on the existing one.
