# DumboDB CAS results

## What changed

These workloads were run against DumboDB twice, and the harness's oracle did
not move between them. The first run failed; the second passes. What changed is
the server.

| | before | after |
|---|---|---|
| revision | `7b226dac4ef1` | `f5cbcf347ef3` |
| verdict | failed | conclusivePass |
| attempts | 708,744 | 1,592,780 |
| matched | 527,882 | 85,173 |
| no match | 180,862 | 1,507,607 |
| rejected | 0 | 0 |
| indeterminate | 0 | 0 |
| duplicate matches | **505,663** | **0** |
| storedVersionEqualsMatched | version=22219 matched=527882 | version=85173 matched=85173 |

Both runs used the `cas` scenario, 30 minutes, 32 workers, seed 1, against a
server started with `--session-sweep-period=1h`.

The duplicate-match count is the whole story. A compare-and-swap that the
server acknowledged with `n:1` but did not apply is a lost update, and in the
first run 505,663 of 527,882 acknowledged matches were of that kind: the
stored counter reached 22219 while the server had claimed 527,882 successful
increments. The primary conservation law, `final version = initial + matched`,
failed by two orders of magnitude.

Two further numbers are worth reading carefully rather than as wins.

The matched count fell from 527,882 to 85,173. That is not a regression: the
earlier figure counted acknowledgements the server did not honour, and 85,173
is what the document actually recorded. The stored counter agrees with it
exactly.

Attempt throughput roughly doubled, 885 operations per second against about
394 before. Some of that is the server no longer performing writes it would
discard -- it applied 85,173 updates in the second run against 527,882 claimed
in the first -- so this is not a like-for-like performance comparison and
should not be quoted as one.

The indeterminate count is zero. A 30-minute soak on the earlier server
recorded 1,550 indeterminate outcomes, every sampled one being code 225,
`session was taken over by a newer connection on this lsid`. That defect is
fixed, and an indeterminate outcome is what stops a run reaching a verdict at
all, so its absence is what makes this run conclusive rather than merely
passing.

## Scenario matrix

Every scenario at both payload sizes, 1000 operations, 16 workers, seed 1,
against `f5cbcf347ef3`. The payload dimension matters because DumboDB stores a
document inline or out of band depending on size, and the merge has to behave
the same either way.

| Scenario | Payload | Verdict | Attempts | Matched | Rejected | Indeterminate |
|---|---|---|---:|---:|---:|---:|
| cas | inline | conclusivePass | 1000 | 125 | 0 | 0 |
| cas | 8192B | conclusivePass | 1000 | 127 | 0 | 0 |
| uuid-cas | inline | conclusivePass | 1000 | 129 | 0 | 0 |
| uuid-cas | 8192B | conclusivePass | 1000 | 124 | 0 | 0 |
| blind-inc | inline | conclusivePass | 1000 | 1000 | 0 | 0 |
| blind-inc | 8192B | conclusivePass | 1000 | 1000 | 0 | 0 |
| disjoint-set | inline | conclusivePass | 1000 | 1000 | 0 | 0 |
| disjoint-set | 8192B | conclusivePass | 1000 | 1000 | 0 | 0 |
| same-set | inline | conclusivePass | 1000 | 1000 | 0 | 0 |
| same-set | 8192B | conclusivePass | 1000 | 1000 | 0 | 0 |

`blind-inc`, `disjoint-set` and `same-set` match on every attempt, which is the
point: an acknowledged write that is refused at its boundary is re-applied
against the new tip rather than reported as a non-match, so a blind increment
still lands. Only the two compare-and-swap scenarios have non-matches, and
those are real: a precondition that no longer holds.

## What this does not cover

Every scenario here reconciles at the end of the command. A fork that outlives
the command -- `--session-isolation`, or an explicit transaction -- acknowledges
a write before its boundary runs, so an acknowledgement is provisional and
conservation has to be counted against acknowledged boundaries instead. That
needs its own accounting rather than a new expectation on this one.

These runs declare no `mergeMode`, so they exercise the default, `fieldTouched`.
The per-mode axis is being built on the `codex-tests-cas` branch.

## Reproducing

```sh
GOWORK=off go build -o /tmp/concurrency ./cmd/concurrency
/tmp/concurrency \
  -uri=mongodb://127.0.0.1:27071 \
  -scenario=cas -duration=30m -workers=32 -seed=1 \
  -database=dumbo_cas_fixed_30m -collection=documents \
  -output=dumbodb-fixed-cas-30m.json
```

Build the server with its revision stamped, or the report records
`Revision: unknown` and the evidence cannot say which binary it measured:

```sh
go build -ldflags "-X github.com/dolthub/dumbodb/internal/version.GitVersion=$(git rev-parse HEAD)" \
  -o /tmp/dumbodb ./cmd/dumbodb
```
