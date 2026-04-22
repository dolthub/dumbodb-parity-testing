# DumboDB vs MongoDB performance benchmarks

This directory holds a performance benchmark suite that measures the per-operation
latency of MongoDB wire-protocol operations against **one target at a time**.
A dual-run comparator (`cmd/compare`) orchestrates runs against both DumboDB and
MongoDB and emits a side-by-side table.

The suite is deliberately **separate from the parity harness in `tests/`**. The
parity harness runs the same operation against both servers *simultaneously* and
compares responses — correct behavior is its contract, not timing. Benchmarks
need isolation: one server under load at a time, no cross-contamination.

## Goals

1. **Find hotspots in DumboDB** (primary)
2. **Produce a public-facing comparison table** (DumboDB vs MongoDB)
3. Serve as a regression gate (lower priority — ratios are stable enough
   for humans, not yet stable enough for CI)

## Scope — what's covered today

**CRUD**: `InsertOne`, `InsertMany` (batches of 100 and 1000), `FindOne` (by
`_id`), `Find` (full scan), `Find` with equality filter, `Find` with range
filter, `UpdateOne`, `UpdateMany`, `ReplaceOne`, `DeleteOne`, `DeleteMany`,
`CountDocuments`, `Distinct`.

**Aggregation**: `$match + $group`, `$sort + $limit`, `$project`.

**Dataset**: 1,000 small (~100 byte) documents per benchmark. A single
collection is seeded fresh per benchmark; the server used depends on the
`-bench.target-uri` flag. All documents share a fixed shape
(`{_id, i, grp, tag, payload}`) so filter benchmarks remain comparable across
size tiers when we add them.

## Scope — deferred

These are enumerated in `bd pa-xp1` but not implemented in the first cut:

- `$unwind + $group`, `$lookup` (join)
- `CreateCollection`, `Drop` (they'd be their own benchmarks — mostly DDL timing)
- Scaling dimensions beyond 1K × small docs (10K, 100K; medium, large). Helpers
  in `bench.go` already parameterize both — add parameterized sub-benchmarks when
  we want the extra surface area.
- dolt-specific commands — the task explicitly scopes us to the MongoDB wire protocol.

## Running

Prerequisite: **Docker**. The runner manages its own containers — it builds a
`dumbodb-bench:local` image from the product (dongo) repo, pulls `mongo:8.0`,
starts both, waits for readiness, runs the benchmarks, then tears everything
down (unless `-f` is given). No manual server setup required.

### One-shot comparison (recommended)

```bash
go run ./benchmarks/cmd/compare \
    -benchtime=2s \
    -json benchmarks/results.json
```

The first run builds the DumboDB image (a minute or two; CGO-enabled Go build);
subsequent runs hit Docker's layer cache and finish in seconds when the product
source hasn't changed.

Example output:

```
Benchmark                DumboDB (ms/op)  MongoDB (ms/op)  Ratio (Dumbo/Mongo)
---------                ---------------  ---------------  -------------------
BenchmarkCountDocuments  33.454           1.241            26.95x
BenchmarkFindOne_ById    2.458            0.434            5.66x
BenchmarkInsertOne       5.586            0.513            10.89x
```

### Investigation flow (keep servers alive)

```bash
go run ./benchmarks/cmd/compare -bench='^BenchmarkUpdateMany$' -f
# ... runner finishes, prints:
#   Servers still running (-f):
#     DumboDB: mongodb://localhost:27018
#     MongoDB: mongodb://localhost:27017

mongosh mongodb://localhost:27018   # poke at DumboDB state
mongosh mongodb://localhost:27017   # compare with MongoDB

# When done:
docker stop dumbodb-bench mongodb-bench && docker rm dumbodb-bench mongodb-bench
```

A second `compare -f` invocation will reuse the running containers rather than
rebuild — handy while iterating on a specific benchmark.

Flags:

| Flag | Default | Purpose |
|---|---|---|
| `-bench`           | `^Benchmark` | Regex of benchmarks to run |
| `-benchtime`       | `2s` | Go's `-benchtime` (wall-clock per benchmark) |
| `-count`           | `1` | Go's `-count` (repetitions) |
| `-json`            | `""` | If set, write JSON results here |
| `-v`               | `false` | Stream `go test` stderr |
| `-f`               | `false` | Keep containers running after the run (for investigation) |
| `-dumbodb-src`     | `/home/ubuntu/dongo` | Path to the product (dongo) repo; used as Docker build context |
| `-no-containers`   | `false` | Skip container management; expect servers already at `:27017` / `:27018` |
| `-health-timeout`  | `60s` | How long to wait for each container to accept connections |

### Container layout

| Container       | Image                | Host URI                    |
|-----------------|----------------------|-----------------------------|
| `mongodb-bench` | `mongo:8.0`          | `mongodb://localhost:27017` |
| `dumbodb-bench` | `dumbodb-bench:local`| `mongodb://localhost:27018` |

Both are fixed names (not randomized) so the `-f` investigation workflow and
any shell aliases you build on top can rely on them. If a container with the
same name exists but is stopped/dead, the runner removes it and starts fresh.

### Running a single target directly (bypassing the runner)

Requires the target server to be already reachable.

```bash
go test ./benchmarks \
    -run='^$' -bench='^BenchmarkInsertOne$' \
    -benchtime=2s \
    -args \
    -bench.target-uri=mongodb://localhost:27018 \
    -bench.target-name=dumbodb
```

`-bench.target-name` is a short label (e.g. `dumbodb`, `mongodb`) used in the
generated database name. Each benchmark creates a uniquely-named database
(`bench_<target>_<op>_<unixNano>`) and drops it on cleanup.

## Output format

The comparator emits two artifacts:

1. **Table** (stdout) — human-readable during development.
2. **JSON** (`-json <path>`) — one array element per benchmark:

   ```json
   {
     "name": "BenchmarkInsertOne",
     "dumbodb_ns_per_op": 5585953,
     "mongodb_ns_per_op": 512888,
     "ratio": 10.89
   }
   ```

   A missing side (benchmark ran against only one target) omits that field and
   the ratio. Downstream tools (public-facing comparison pages, regression gates)
   should consume the JSON.

## Notes on measurement hygiene

- **Dataset seeding is untimed**: benchmarks that operate on a pre-populated
  collection (Find, Update, Delete, Count, Distinct, Aggregate) seed the
  collection before `b.ResetTimer()`.
- **Deletion reseeds off the clock**: `BenchmarkDeleteOne` / `DeleteMany` refill
  the collection between timed iterations so they never run dry.
- **PRNG is seeded** (`-bench.seed`, default `42`): document content is
  reproducible across runs.
- **Each benchmark gets its own database**: no cross-contamination between
  benchmarks in the same run. The DB is dropped in `b.Cleanup`.

## Adding a benchmark

1. Write a `BenchmarkXxx(b *testing.B)` function in a new or existing `*_bench_test.go`.
2. Use `withSeededCollection(b, label, n, size)` for operations on populated data
   or `withEmptyCollection(b, label)` for insert-style ops.
3. Call `b.ResetTimer()` after setup, `b.StopTimer()` / `b.StartTimer()` around
   any per-iteration setup that shouldn't count.
4. Keep iterations independent — do not rely on collection state carrying across
   iterations unless you explicitly design for it (see `BenchmarkDeleteOne` for
   how to reseed).
