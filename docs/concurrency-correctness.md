# Concurrency Correctness Harness

## Scope

The first implementation phase characterizes MongoDB 8.0 behavior. It does not
run DumboDB and does not test configurable merge modes. MongoDB applies each
single-document write atomically; it does not expose a three-way document merge
policy.

The eventual DumboDB `fieldTouched` mode is expected to preserve the same CAS
safety property, but that is a later comparison. `fieldTouched` is not a name
for MongoDB behavior.

## Terms

An attempt is one command sent by one worker. Every attempt has exactly one
terminal client-visible outcome:

- `matched`: the command succeeded and matched one document.
- `noMatch`: the command succeeded and matched no document.
- `commandError`: the server returned an error.
- `clientError`: the driver could not establish whether the server applied the
  operation, including timeout and connection loss.

`modified` is an attribute of a matched outcome, not a separate terminal
outcome. An update may match a document without changing its stored value.

The harness must not count a client error as either applied or unapplied. Such
an outcome is indeterminate unless a scenario-specific operation identifier can
be recovered from stored state.

## Global accounting laws

For every completed run:

```text
attempts = matched + noMatch + commandError + clientError
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
its filter. With no indeterminate client errors:

```text
matched = attempts - commandError
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
that acknowledged writes to different fields are not lost.

### Same field set

Workers write unique operation sequence values to one shared field. MongoDB may
serialize them in any order, so the final value need not be the greatest issued
sequence number. The final value must identify one acknowledged matched write,
and every matched response must be internally valid. This scenario measures
contention outcomes but cannot by itself prove that intermediate overwritten
writes were durable.

## Workload dimensions

The MongoDB phase varies:

- worker count;
- client count and connection-pool size;
- synchronized bursts versus continuously released operations;
- small inline-shaped documents and large payload documents;
- CAS read/update delay;
- run duration and operation limit;
- deterministic workload seed.

Document size is a workload dimension even though MongoDB has no DumboDB
inline/out-of-band storage boundary. The same generated documents will later be
used for DumboDB comparison.

## Statistics

Hard invariants are evaluated per run and have zero tolerance.

Scheduler-sensitive observations are reported as rates with their sample size:

- CAS match and no-match rate;
- operations per second;
- response latency distribution;
- server and client error rate;
- observed contention-window width.

MongoDB and DumboDB raw counts will not be required to match. Later comparisons
will use normalized rates and confidence intervals. A statistical difference
is reported separately from a correctness failure.

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

## Phase gate

DumboDB support must not begin until the MongoDB-only runner:

1. completes the initial scenario set;
2. balances every accounting equation;
3. detects an intentionally corrupted ledger or final state;
4. emits bounded machine-readable evidence;
5. completes a sustained MongoDB 8.0.28 characterization run.
