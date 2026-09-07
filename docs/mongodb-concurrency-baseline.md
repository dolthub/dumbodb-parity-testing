# MongoDB 8.0.28 Concurrency Baseline

## Environment

The initial sustained baseline ran against MongoDB Community Server 8.0.28 on
Debian 12. The server and runner were on the same host.

    scenario: cas
    duration: 30m
    workers: 32
    seed: 1
    payload bytes: 0
    started: 2026-09-07T20:50:38.139770087Z
    finished: 2026-09-07T21:20:38.142640916Z

Command:

    go run ./cmd/concurrency \
      -scenario=cas \
      -duration=30m \
      -workers=32 \
      -seed=1 \
      -output=/tmp/mongodb-8.0.28-cas-30m.json

## Results

    attempts: 20,413,088
    matched: 1,755,590
    no match: 18,657,498
    modified: 1,755,590
    command errors: 0
    client errors: 0
    operations per second: 11,340.6

The CAS match rate was 0.0860032. Its 95 percent Wilson interval was
[0.0858816, 0.0861249]. The no-match rate was 0.9139968 with interval
[0.9138751, 0.9141184].

The upper bound of the 95 percent interval for each unobserved error rate was
0.0000001882.

Latency buckets:

    under 100 us: 0
    100 us to 1 ms: 1,073,569
    1 ms to 10 ms: 19,033,819
    10 ms to 100 ms: 305,700
    100 ms to 1 s: 0
    1 s or greater: 0

## Correctness

Both zero-tolerance checks passed:

    stored version = matched updates = 1,755,590
    modified updates = matched updates = 1,755,590

Every attempt had exactly one terminal outcome. Two concurrent operations that
read the same version did not both match. MongoDB atomically re-evaluated the
version predicate when applying each update, so losing contenders returned a
successful command with no matching document.

This is the observed MongoDB CAS contract that the later DumboDB baseline will
use. It is comparable to the safety goal of fieldTouched, but MongoDB does not
implement or expose a merge mode.

## Scenario preflight

Before the sustained run, each initial scenario completed 5,000 operations
with 16 workers and a 20,000-byte retained payload:

- CAS increment preserved every matched increment.
- Blind increment matched, modified, and stored all 5,000 increments.
- Disjoint field sets retained each worker's last acknowledged value.
- Same field sets matched all writes and retained an issued value.

These preflights establish functional coverage. Only the CAS scenario has a
30-minute statistical baseline so far.

## UUID CAS expansion

A later UUID compare-and-set run used 32 workers, a 20,000-byte retained
payload, and 100,000 attempts. It produced 9,041 matched and modified updates,
90,959 clean no-matches, and zero errors. After all writers drained:

- the stored applied count was exactly 9,041;
- the final token was BSON binary subtype 4 with 16 bytes;
- the final token matched the deterministic UUID for the final stored operation;
- the final stored operation was one of the issued attempts.

This confirms the same MongoDB atomic CAS behavior when contenders propose
distinct UUID replacements instead of converging integer increments.
