# Concurrency Correctness Model

## Purpose

The runner must detect a narrow CAS race even when aggregate counts and final
state look plausible. Global totals are necessary for bookkeeping but are not
a CAS oracle. The oracle must retain the causal relationship between the value
an operation observed, the replacement it proposed, the server response, and
the settled state.

## Operation lifecycle

Every admitted operation receives a unique sequence number before execution.
It then reaches exactly one runner state:

- completed with a terminal outcome;
- abandoned before execution;
- indeterminate after execution began.

The runner maintains these independent counts:

    reserved = completed + abandoned
    completed = matched + noMatch + rejected + indeterminate

Reserved is internal scheduling state. Attempts in reports means completed
operations only. There must be one authoritative reported attempt count.

An invalid outcome or a failure to record an outcome is fatal to the run. A
worker may not silently exit and allow reconciliation to report success.

## Causal record

Every completed operation produces an operation record:

    sequence
    worker
    scenario
    observed generation
    proposed generation
    outcome
    modified
    started and finished time
    error labels and bounded error text

Fields that do not apply to a scenario remain absent. The current bounded
causal report retains generations and operation identities. UUID token values
are checked during final reconciliation but are not retained as fingerprints.

The aggregate ledger and causal oracle consume the same record. This prevents
the response counters and the CAS history from describing different sets of
operations.

## Outcome certainty

The outcome categories are:

- matched: the server acknowledged that one document matched.
- noMatch: the server acknowledged that no document matched.
- rejected: the server definitively rejected the operation before applying it.
- indeterminate: the client cannot prove whether the operation applied.

Network errors, context cancellation after execution begins, and write concern
failures are indeterminate. A MongoDB error type alone does not establish
certainty: a CommandError carrying the NetworkError label remains
indeterminate.

Exact final-state conservation checks are conclusive only when there are no
indeterminate operations, unless the scenario stores a unique operation marker
that can resolve each indeterminate outcome from settled state.

## CAS exclusivity

For each observed generation G, count matched operations whose predicate was
based on G:

    matchedByObservedGeneration[G] <= 1

Any value greater than one is a zero-tolerance CAS failure. This is independent
of aggregate match rate and final counter value.

The tracker may use chunked fixed-width counters indexed by generation.
Correct runs need one small counter per successful generation. On the first
duplicate it retains bounded evidence containing the observed generation and
the competing operation records.

## Integer CAS chain

An integer CAS attempt that observes version V proposes version V+1. For a
conclusive run:

    no observed version matched more than once
    every matched edge is V -> V+1
    final version = initial version + matched
    every integer generation from initial through final has one matched edge

The aggregate equality alone does not prove exclusivity. Both the per-generation
check and the final chain check are required.

## UUID CAS chain

The UUID document stores both a subtype-4 UUID and its integer generation. An
attempt observes token T at generation G and proposes a deterministic unique
token U at generation G+1. The conditional filter includes both T and G. The
atomic update installs U and G+1 and increments the applied count.

For a conclusive run:

    no observed generation matched more than once
    every matched edge is (G,T) -> (G+1,U)
    every proposed UUID is valid subtype 4
    final applied count = matched
    final generation = matched
    final token equals the matched proposal for the final generation

This proves a single chain from the initial token to settled state. Merely
proving that the final token was issued is insufficient.

## Non-CAS scenarios

Blind increments require final counter = matched when there are no
indeterminate outcomes.

For disjoint fields, a worker with matched writes must retain its last matched
value. A worker with no matched writes passes only when its field is absent. A
present field without a matched write is a failure.

Same-field replacement cannot prove durability of overwritten intermediate
values from final state alone. It is retained as contention characterization,
not as a strong write-conservation oracle.

## Bounded evidence

Aggregate counters, latency histograms, per-generation CAS counters, and the
first bounded set of invariant failures are retained. Full operation histories
are optional and bounded by configuration.

Per-generation tracking grows with successful CAS generations rather than total
attempts. This is acceptable for the expected multi-million-operation runs and
is reported as observed-generation slots, matched-operation words, and an
estimated tracker byte count.

## Stop and reconciliation

Normal duration expiry stops admission without canceling accepted operations.
The runner drains accepted operations before verification.

An external interrupt:

1. stops admission;
2. asks in-flight operations to stop;
3. waits through a bounded quiescence interval;
4. verifies using a fresh bounded context;
5. writes a report marked truncated;
6. retains the database automatically if verification or report writing fails.

A client-visible terminal response does not prove the server is quiescent after
cancellation. Interrupted runs with indeterminate operations are evidence, but
they cannot produce a conclusive conservation pass.

## Testing contract

The execution core uses a narrow adapter rather than concrete
mongo.Collection parameters. A deterministic fake must be able to inject:

- two matches from the same observed CAS generation;
- a corrupted final version or token;
- an invalid outcome that the ledger rejects;
- a network-labeled CommandError;
- a write-concern-only WriteException;
- interruption during an in-flight write.

Tests must prove each injected defect makes the appropriate check fail. The
runner and fake tests run under the race detector. Real MongoDB integration
tests then verify adapter and BSON behavior.

The issued-versus-recorded reconciliation is a defense-in-depth assertion. A
worker cannot intentionally bypass recording through the Scenario interface;
reaching that mismatch requires a worker panic or runner defect. It is reviewed
as an invariant rather than exposed through a test-only production hook.

## Baseline status

The corrected 30-minute MongoDB baseline establishes per-generation CAS
exclusivity and is the authoritative baseline. Its report predates the tracker
memory-size fields added in the subsequent review cleanup; all later reports
include them.
