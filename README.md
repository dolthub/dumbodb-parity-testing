# dumbodb-parity-testing

Black-box test suites that exercise DumboDB against the systems it claims
parity with: MongoDB 8 on the wire side, and Dolt on the versioned-storage
side. Every test in this repo runs against the real comparison target; there
are no hardcoded expected values.

The repo contains three independent suites. Pick the one that matches what
you are investigating:

| Suite             | Compares          | Measures                          | Directory     |
|-------------------|-------------------|-----------------------------------|---------------|
| Parity tests      | DumboDB vs Mongo  | Response equivalence (correctness)| `tests/`      |
| Perf benchmarks   | DumboDB vs Mongo  | Per-operation latency (ns/op)     | `benchmarks/` |
| Storage + merge   | DumboDB vs Dolt   | On-disk growth, merge time        | `storage/`    |

## Prerequisites

- Go 1.22+
- Docker (every suite uses containers for at least one of its servers)
- A built `dumbodb` binary or the [dolthub/dumbodb](https://github.com/dolthub/dumbodb)
  source tree. The parity suite expects the binary on `PATH` (or running on
  `DUMBODB_URI`); the benchmark runner builds a Docker image from source; the
  storage suite expects `dolthub/dumbodb:latest` published to a registry.

This module is intentionally outside the workspace `go.work` at the repo
root. Build and run with `GOWORK=off`:

    GOWORK=off go build ./...
    GOWORK=off go vet  ./...

---

## 1. Parity tests (`tests/`)

**What they do.** Each test runs the same MongoDB wire-protocol operation
against MongoDB 8 and DumboDB side-by-side, then diffs the responses through
a fuzzy comparator that normalises non-deterministic values (ObjectIds,
timestamps) before comparing. MongoDB is the oracle; if the two responses
match, the test passes.

**Where the suite lives.** ~30 `*_test.go` files under `tests/` cover CRUD,
aggregation stages and expressions, indexes, geo, projections, cursors,
transactions, BSON types, and a handful of documentation-derived suites
(`mongodb_*_test.go`, `w3schools_test.go`).

**Modes.** Every test declares a `DumboDBSupport` level on the `TestCase`:

| Mode               | Meaning                                                  |
|--------------------|----------------------------------------------------------|
| `DumboDBFull`      | Run on both servers; divergence is a CI failure          |
| `DumboDBMongoOnly` | Run on MongoDB only; DumboDB skipped (unsupported)       |
| `DumboDBXFail`     | Run on both; divergence is recorded but not a CI failure |

**Running.** Bring up both servers and point the harness at them:

    docker run -d --name mongo8   -p 27017:27017 mongo:8.0
    /path/to/dumbodb --port 27018 &

    GOWORK=off go test ./tests/...
    GOWORK=off go test ./tests/ -run TestInsertOne   # single test

Connection URIs are overridable:

| Variable      | Default                       |
|---------------|-------------------------------|
| `MONGO_URI`   | `mongodb://localhost:27017`   |
| `DUMBODB_URI` | `mongodb://localhost:27018`   |

**Result legend.** PASS (matched), FAIL (unexpected divergence, breaks CI),
SKIP (`MongoOnly` mode), XFAIL (`XFail` mode, divergence expected).

At the end of a run the harness prints:

    Parity Summary
      Matching:    42
      Diverging:    0
      Mongo-only:   5
      XFail:        3
      Total:       50

CI exits non-zero only when `Diverging > 0`.

**Known divergences.** `known-divergences.txt` lists features where DumboDB
intentionally differs. The file is managed by the project owner only; see
`AGENTS.md` for the governance rule.

---

## 2. Performance benchmarks (`benchmarks/`)

**What they do.** Measure per-operation latency (ns/op) of MongoDB
wire-protocol operations against one server at a time, then compare the two
ratios. The benchmark suite is deliberately separate from the parity suite:
parity needs both servers under simultaneous load to compare responses;
benchmarks need isolation so neither server pollutes the other's numbers.

**What's covered.** CRUD (`InsertOne`/`Many`, `Find`, `Find_FilterEq`,
`Find_FilterRange`, `UpdateOne`/`Many`, `Replace`, `Delete*`,
`CountDocuments`, `Distinct`) and a small aggregation set (`$match+$group`,
`$sort+$limit`, `$project`). Read and write benchmarks have scaled variants
at 10K and 50K documents, with and without an index on the filter field; see
`benchmarks/README.md` for the full matrix and the reasoning behind which
operations gain from indexes at which sizes.

**Running.** The `cmd/compare` runner manages containers end-to-end. It
builds a `dumbodb-bench:local` image from a local DumboDB source checkout,
pulls `mongo:8.0`, waits for both to accept connections, runs the
benchmarks, and tears the containers down:

    go run ./benchmarks/cmd/compare \
        -benchtime=2s \
        -csv benchmarks/results.csv

Output is a markdown-ish table on stdout plus optional CSV for downstream
tooling (regression gates, public comparison pages).

    Benchmark                DumboDB (ms/op)  MongoDB (ms/op)  Ratio
    BenchmarkCountDocuments  33.454           1.241            26.95x
    BenchmarkFindOne_ById    2.458            0.434             5.66x
    BenchmarkInsertOne       5.586            0.513            10.89x

Useful flags (full list in `benchmarks/README.md`):

| Flag              | Purpose                                                         |
|-------------------|-----------------------------------------------------------------|
| `-bench`          | Regex of benchmarks to run                                      |
| `-benchtime`      | Wall-clock budget per benchmark                                 |
| `-f`              | Keep both containers alive after the run (for `mongosh` probing)|
| `-dumbodb-src`    | Path to the DumboDB repo used as the Docker build context       |
| `-no-containers`  | Skip container management; assume servers already running       |
| `-test-timeout`   | `-timeout` for `go test`; raise for the 50K-scale variants      |

To run a single benchmark against an already-running server, bypass the
runner:

    GOWORK=off go test ./benchmarks \
        -run='^$' -bench='^BenchmarkInsertOne$' \
        -benchtime=2s -args \
        -bench.target-uri=mongodb://localhost:27018 \
        -bench.target-name=dumbodb

---

## 3. Storage and merge tests (`storage/`)

**What they do.** Compare DumboDB and Dolt directly on versioned-storage
workloads: insert a base dataset, branch, mutate a small subset, merge back,
and measure (a) the on-disk byte delta and (b) the wall-clock merge time.
This is the suite that exposes DumboDB's current O(N) secondary-index
rebuild on merge versus Dolt's O(diff) incremental merge.

**Tests.**

- `TestMergeStorage_LargeBaseTinyDiff` - 100k base docs, 100-doc diff;
  prints a table of storage before/after/delta and merge duration for each
  backend.
- `TestMergeTime_ScalesWithBase` - sweeps base sizes at 1k / 10k / 100k with
  a fixed diff; surfaces the linear vs. flat merge-time pattern.
- `TestIndexLookup_PostMerge` - 1000 secondary-index lookups after a merge;
  logs p50 and p99 per backend.
- `BenchmarkMerge_LargeBaseTinyDiff` - the merge step from the first test
  expressed as a Go benchmark for `benchstat` use across commits.

**Running.** Both servers must already be running as Docker containers with
their data directories bind-mounted; the tests `du` those host paths to
measure storage. Four env vars are required:

| Variable           | Purpose                                              |
|--------------------|------------------------------------------------------|
| `DUMBODB_URI`      | MongoDB URI for the DumboDB container                |
| `DUMBODB_DATA_DIR` | Host path bind-mounted into the DumboDB container    |
| `DOLT_URI`         | MySQL DSN for `dolt sql-server`                      |
| `DOLT_DATA_DIR`    | Host path bind-mounted into the Dolt container       |

The repo ships a wrapper that brings both containers up, wipes their data
dirs on first use, exports the four env vars, and runs the suite:

    ./run-storage-tests.sh
    ./run-storage-tests.sh -run TestMergeStorage_LargeBaseTinyDiff

Or manually:

    GOWORK=off go test ./storage/... -v \
        -run TestMergeStorage_LargeBaseTinyDiff -timeout 30m

The 100k-base tests take tens of minutes; budget accordingly.
