# DumboDB vs MongoDB performance benchmarks

This directory holds a performance benchmark suite that measures the per-operation
latency of MongoDB wire-protocol operations against **one target at a time**.
A dual-run comparator (`cmd/compare`) orchestrates runs against both DumboDB and
MongoDB and emits a side-by-side table.

The suite is deliberately **separate from the parity harness in `tests/`**. The
parity harness runs the same operation against both servers *simultaneously* and
compares responses - correct behavior is its contract, not timing. Benchmarks
need isolation: one server under load at a time, no cross-contamination.

## Goals

1. **Find hotspots in DumboDB** (primary)
2. **Produce a public-facing comparison table** (DumboDB vs MongoDB)
3. Serve as a regression gate (lower priority - ratios are stable enough
   for humans, not yet stable enough for CI)

## Scope - what's covered today

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

**Scaled index variants** (in `scaled_indexed_bench_test.go`): `Find_FilterEq`,
`Find_FilterRange`, `UpdateMany`, `DeleteMany`, and `CountDocuments` each have
`_10K`, `_10K_Indexed`, `_50K`, `_50K_Indexed` variants. The 1K baseline is
too small for MongoDB's indexes to show their advantage - full-collection
scans finish before the index-lookup overhead pays off. At 10K the read-side
indexes (`Find_FilterRange`, `CountDocuments`) cleanly cross a 2x speedup on
MongoDB; 50K confirms the trend and pushes the ratios further. `Find_FilterEq`,
`UpdateMany`, and `DeleteMany` plateau below 2x even at 50K because their
filter (one of ten `grp` values) returns ~10% of the collection - fetching
that many docs through the index is no cheaper than a sequential scan, and
for the write benchmarks the dominant cost is the writes themselves, not the
candidate lookup.

**Point-lookup variants** (in `point_lookup_scaled_bench_test.go`):
`PointLookup` has `_10K`, `_10K_Indexed`, `_50K`, `_50K_Indexed` variants that
filter on the *unique* field `i`, so every lookup returns exactly one document -
the result set is constant across N, unlike `Find_FilterEq`/`Find_FilterRange`
whose result sets grow with the collection. That isolates pure seek cost: the
indexed variant's latency stays near-flat from 10K to 50K (log-N seek) while the
unindexed variant grows linearly (full scan). This is the black-box latency
companion to dumbodb's white-box node-fetch proof (`workspace-da6.1`), which
counts prolly-tree nodes per seek and reaches larger N in-process.

## Scope - deferred

These are enumerated in `bd pa-xp1` but not implemented in the first cut:

- `$unwind + $group`, `$lookup` (join)
- `CreateCollection`, `Drop` (they'd be their own benchmarks - mostly DDL timing)
- Scaling dimensions beyond 1K x small docs (10K, 100K; medium, large). Helpers
  in `bench.go` already parameterize both - add parameterized sub-benchmarks when
  we want the extra surface area.
- dolt-specific commands - the task explicitly scopes us to the MongoDB wire protocol.

## Running

Prerequisite: **Docker**. The runner manages its own containers - it pulls
`dolthub/dumbodb:latest` and `mongo:8.0`, starts both, waits for readiness,
runs the benchmarks, then tears everything down (unless `-f` is given). No
manual server setup required.

### One-shot comparison (recommended)

```bash
go run ./benchmarks/cmd/compare \
    -benchtime=2s \
    -csv benchmarks/results.csv
```

By default the runner measures the released `dolthub/dumbodb:latest` image. To
pin a specific release, pass `-dumbodb-image=dolthub/dumbodb:v0.1.1` (or set
`DUMBODB_IMAGE`). To benchmark an unreleased commit from a local checkout,
pass `-dumbodb-src=/path/to/dumbodb` and the runner will build
`dumbodb-bench:local` from that tree via `benchmarks/Dockerfile.dumbodb`
instead of pulling.

Example output:

```
test_name           dumbodb_latency    mongodb_latency    multiplier
count_documents     33.454             1.241              26.95x
find_one_by_id      2.458              0.434              5.66x
insert_one          5.586              0.513              10.89x
```

`multiplier` is `dumbodb / mongodb`: `26.95x` means DumboDB takes 26.95x as
long as MongoDB; values under `1.00x` mean DumboDB is faster.

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
re-pull or rebuild - handy while iterating on a specific benchmark.

Flags:

| Flag | Default | Purpose |
|---|---|---|
| `-bench`           | `^Benchmark` | Regex of benchmarks to run |
| `-benchtime`       | `2s` | Go's `-benchtime` (wall-clock per benchmark) |
| `-count`           | `1` | Go's `-count` (repetitions) |
| `-csv`             | `""` | If set, write CSV results here |
| `-v`               | `false` | Stream `go test` stderr |
| `-f`               | `false` | Keep containers running after the run (for investigation) |
| `-dumbodb-image`   | `dolthub/dumbodb:latest` | Image to pull and run as DumboDB (env: `DUMBODB_IMAGE`). Ignored when `-dumbodb-src` is set. |
| `-dumbodb-src`     | `""` | If set, build DumboDB from this source directory via `benchmarks/Dockerfile.dumbodb` instead of pulling `-dumbodb-image` (env: `DUMBODB_SRC`). |
| `-no-containers`   | `false` | Skip container management; expect servers already at `:27017` / `:27018` |
| `-health-timeout`  | `60s` | How long to wait for each container to accept connections |
| `-test-timeout`    | `10m` | `-timeout` passed to `go test`. Bump to `45m` or higher when running the 50K-scale `*_50K` benchmarks - DumboDB's seed step alone takes ~30 minutes at that size. |

The runner reuses an already-present DumboDB image rather than re-pulling, so
mutable tags like `:latest` do not auto-refresh. Run `docker pull
dolthub/dumbodb:latest` (or `docker image rm`) by hand when you want a fresh
copy.

### Container layout

| Container       | Image (default mode)      | Host URI                    |
|-----------------|---------------------------|-----------------------------|
| `mongodb-bench` | `mongo:8.0`               | `mongodb://localhost:27017` |
| `dumbodb-bench` | `dolthub/dumbodb:latest`  | `mongodb://localhost:27018` |

With `-dumbodb-src` set, `dumbodb-bench` runs the locally-built
`dumbodb-bench:local` image instead. Both container names are fixed (not
randomized) so the `-f` investigation workflow and any shell aliases you build
on top can rely on them. If a container with the same name exists but is
stopped/dead, the runner removes it and starts fresh.

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

1. **Table** (stdout) - human-readable during development.
2. **CSV** (`-csv <path>`) - one header row plus one row per benchmark:

   ```csv
   name,dumbodb_ns_per_op,mongodb_ns_per_op,multiplier
   BenchmarkInsertOne,5585953.00,512888.00,10.89
   ```

   `ns_per_op` columns are fixed-point with two decimal places. `multiplier`
   is `dumbodb / mongodb` - `2.00` means DumboDB takes 2x as long as MongoDB,
   `0.50` means it is twice as fast. A missing side (benchmark ran against
   only one target) leaves the corresponding column blank, and `multiplier`
   is blank as well. Downstream tools (public-facing comparison pages,
   regression gates) should consume the CSV.

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
4. Keep iterations independent - do not rely on collection state carrying across
   iterations unless you explicitly design for it (see `BenchmarkDeleteOne` for
   how to reseed).
