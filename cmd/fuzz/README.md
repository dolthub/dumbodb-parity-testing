# cmd/fuzz

A differential fuzzer. It sends randomly generated hinted reads (find, find
with limit, count, aggregate, distinct) and hinted single-document writes
(findAndModify, update, replace, delete) to MongoDB and DumboDB and reports
every command whose results differ. The data mixes numeric types, strings,
null and missing fields, MinKey/MaxKey, arrays, nested arrays and embedded
documents; the indexes are ascending, descending, compound, sparse, partial,
multikey and dotted.

It is meant for long unattended runs, such as a couple of hours each morning:

    GOWORK=off go run ./cmd/fuzz \
        -dumbodb-bin /path/to/dumbodb -mongod-bin /path/to/mongod-8.0.28 \
        -duration 2h -report-dir ~/fuzz-reports

## Servers

- `-dumbodb-bin`/`-mongod-bin` start local binaries. Data and logs go under
  `<out>/servers`; data is removed at exit, logs are kept.
- Without them it runs Docker containers `mongodb-fuzz` (`-mongo-image`,
  default `mongo:8.0.28`) and `dumbodb-fuzz` (`-dumbodb-image`, or built from
  `-dumbodb-src`) on ports 27037/27038.
- `-mongo-uri`/`-dumbodb-uri` use servers that are already running.

Server handling is shared with `benchmarks/cmd/compare` through the `targets`
package.

## Cases

Each case is one seed: 40 documents, 18 indexes, and either `-read-steps`
filters (each run as 5 read commands) or `-write-steps` writes. Each write
runs on a reset collection and is followed by a read of the whole
collection. `-mode both` alternates by seed. A seed always generates the same
case, so `-seed N -cases 1` reruns one.

## Findings

- Results are compared after sorting document fields. Errors are compared by
  code, name and labels, as in the parity suite.
- A result with the same documents in a different order is `order-only`.
  MongoDB does not promise an order for unsorted queries, but a hinted scan's
  order decides which document a single-document write picks.
  `-ignore-order` keeps these from failing the run.
- Findings are grouped by shape: the command kind, the hinted index, and the
  filter with each value replaced by its type. The first finding of each
  shape is shrunk to the hinted index and the fewest documents that still
  diverge, and saved as a repro.

## Output

`<out>` (default `fuzz-results/<UTC time>`) holds:

- `findings.jsonl`: every finding.
- `repro/NNNN-<class>.txt`: one repro per shape.
- `summary.txt`.

`-report-dir`, `-alert-cmd` and `-email-from`/`-email-to`/`-email-region`
deliver the summary with the repros attached, as `compare` does.

## Replay

`go run ./cmd/fuzz -replay <repro file>`, with the same server flags, runs a
repro's commands on both servers and prints each result.

## Exit status

- 0: no divergences.
- 1: divergences.
- 2: the run itself failed, including a server that stopped answering. The
  report then ends with that server's log.
