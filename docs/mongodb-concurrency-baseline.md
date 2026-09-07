# MongoDB 8.0.28 Corrected Concurrency Baseline

This baseline supersedes the aggregate-only CAS run that completed at
2026-09-07T21:20:38Z. That earlier run checked only final version and modified
counts. It did not retain per-observed-generation edges and therefore could not
support its claim that two operations observing the same version never both
matched. Its throughput and match-rate measurements remain historical data,
but it is not correctness evidence.

## Environment

The corrected sustained baseline ran against MongoDB Community Server 8.0.28 on
Debian 12. The server and runner were on the same host.

    scenario: cas
    duration: 30m
    workers: 32
    seed: 1
    payload bytes: 0
    CAS delay: 0s
    latency scope: read-and-update
    workload started: 2026-09-07T23:03:32.432489171Z
    workload finished: 2026-09-07T23:33:32.433571669Z

Command:

    go run ./cmd/concurrency \
      -scenario=cas \
      -duration=30m \
      -workers=32 \
      -seed=1 \
      -database=mongodb_cas_corrected_rerun_20260907 \
      -output=docs/evidence/mongodb-8.0.28-cas-corrected-30m.json

The complete machine-readable report is stored at
`docs/evidence/mongodb-8.0.28-cas-corrected-30m.json` with SHA-256
`e25eef402e6068369872af1164f549c39eef0a772d04bcb54d5718b0b3445751`.

## Results

    attempts: 26,630,519
    matched: 2,066,934
    no match: 24,563,585
    modified: 2,066,934
    command errors: 0
    client errors: 0
    operations per second: 14,794.7

The CAS match rate was 0.0776152. Its 95 percent Wilson interval was
[0.0775137, 0.0777169]. The no-match rate was 0.9223848 with interval
[0.9222831, 0.9224863].

The upper bound of the 95 percent interval for each unobserved error rate was
0.0000001443.

Latency buckets:

    under 100 us: 0
    100 us to 1 ms: 1,889,750
    1 ms to 10 ms: 24,676,184
    10 ms to 100 ms: 64,585
    100 ms to 1 s: 0
    1 s or greater: 0

## Correctness

All five zero-tolerance checks passed:

    stored version = matched updates = 2,066,934
    modified updates = matched updates = 2,066,934
    matched causal edges = matched updates = 2,066,934
    duplicate observed-generation matches = 0
    invalid generation edges = 0
    highest observed generation = 2,066,933

Every attempt had exactly one terminal outcome. The matched edges form one
complete chain from generation 0 to the stored version. No observed generation
matched more than once, directly establishing that two concurrent operations
based on the same version did not both match in this run. Losing contenders
returned a successful command with no matching document.

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
- the final stored operation was one of the matched attempts.

This confirms the same MongoDB atomic CAS behavior when contenders propose
distinct UUID replacements instead of converging integer increments.
