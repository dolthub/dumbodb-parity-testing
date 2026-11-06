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
`CountDocuments`, `Distinct`. `InsertOne` and `InsertMany` use string `_id`s;
their `_ObjectId` variants use fresh driver ObjectIds, which increase as
server-generated ids do, so the pair shows whether `_id` order changes insert
cost.

**Aggregation**: `$match + $group`, `$sort + $limit`, `$project`.
`Agg_Sort_10K[_Indexed]` repeats `$sort + $limit` on 10K documents, the indexed
variant with an index on the sort field.

**Projection** (in `projection_bench_test.go`): `Find_Projection` returns
`{i, tag}` from the 1K small-doc set; `Find_Projection_10KB` and
`Find_Projection_FilterEq_10KB` do the same over 500 10KB documents, where the
projection discards almost all of each document, without and with a `grp`
filter; `Find_Projection_FilterEq_10KB_Indexed` adds an index on `grp`.
`Find_Projection_FilterEq_Sort_10KB_Indexed` also sorts on the indexed `grp`,
and `Find_Projection_Sort_10KB_Indexed` keeps the `grp` filter but indexes and
sorts on `i`, so the sort is served by walking the index. `Agg_Project_10KB` is the same `$project` as `Agg_Project` over those
10KB documents.

**Document-size variants** (in `sized_bench_test.go`, `typed_bench_test.go` and
`mutation_bench_test.go`): `InsertOne`, `FindOne` and `UpdateOne_SetField` run
at 1KB, 10KB, 100KB and 1MB documents, and `InsertOne` and `FindOne` also have
`TypedRealistic` variants at those sizes.
The nested-update benchmarks (`ArrayExtend`, `ArrayShorten`, `FieldInsert`,
`FieldRemove`, at depths 1, 3 and 5) run at 10KB, 100KB and 1MB. Benchmarks
that seed a collection seed fewer documents as documents grow, so each size
costs about the same to set up.

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
companion to dumbodb's white-box `TestSeekScaling_PointLookupIsLogarithmic`,
which counts prolly-tree nodes per seek and reaches larger N in-process.

**`_id` range variants** (in `id_range_scaled_bench_test.go`): every
collection has the `_id_` index, so these have no unindexed counterpart; the
10K-to-50K growth is the signal. `Find_IdRange_{Int,String,ObjectId,UUID}` at
`_10K`/`_50K` find a window of n/100 consecutive `_id`s
(`$gte`/`$lt`), sliding the window each iteration. The string, ObjectId, and
UUID (BSON binary subtype 4, in `uuid_bench_test.go`) keys are built so their
sort order matches insertion order. `Find_IdPaginate` at
`_10K`/`_50K` times one page of keyset pagination: `{_id: {$gt: last}}`
sorted on `_id` with a limit of 100, walking the collection page by page.
Before timing, each checks the result count (or, for pagination, the exact
page of `_id`s in order) and the `_id_` index use. DumboDB keys rows by a hash
of `_id` and scans for these, a known gap tracked by dolthub/dumbodb#207.

**UUID insert variants** (in `uuid_bench_test.go`):
`InsertMany_{UUIDv4,UUIDv7,UUIDv4String}_50K_{100,1000}` insert batches into a
collection seeded with 50K documents whose `_id`s are the same kind, so each
insert lands among existing keys; only the inserted documents are cleared.
Binary v4 UUIDs are random and land throughout the `_id` order, v7 UUIDs
increase and append at its end, and `UUIDv4String` stores v4 UUIDs as
36-character strings, as many applications do.

`CollatedPointLookup` (in `collated_point_lookup_scaled_bench_test.go`) is the
same at `_10K`/`_50K` `[_Indexed]` but on a *string* key with an fr_CA collation
on both the index and the query, so the index stores ICU sort keys. The indexed
variant proves the collated (sort-key) index seeks flat at scale end-to-end; its
gap over the unindexed collated scan is wider than the plain case, because a
collated scan pays per-document ICU comparison while the sort-key seek does not.

**Join variants** (in `lookup_bench_test.go`): 100 `orders` documents join into
the seeded collection of N documents, each matching exactly one foreign document
on the unique field `i`, so the joined result is the same at every N and any
growth with N is the cost of reaching the foreign side.
`Lookup_Equality` (`localField`/`foreignField`) has `_1K`, `_10K`, `_50K`
`[_Indexed]` variants; the indexed ones put an index on the foreign `i`.
`Lookup_Pipeline` is the same join in the `let`/`pipeline` form, and
`Lookup_Nested` adds a second `$lookup` inside the sub-pipeline, so the inner
join runs once per order. `GraphLookup` follows `grp -> i` from each order; every
traversal visits two documents. These are at `_10K` `[_Indexed]`.

**Index-use variants** (in `index_use_bench_test.go`): a query that silently
falls back to a collection scan still returns correct results, so only latency
shows it. Each `IndexUse_*` benchmark seeds 10K documents (50K for the `_50K`
variants of the core lookups and sorts, whose indexed latency should match the
10K figure) and, before timing,
runs the operation through `explain` (`executionStats`) and fails unless the
winning plan has an index scan on the expected index and, for `find`, the
documents examined stay within the results returned or a tenth of the
collection, whichever is more. Explain plans separately from the command, so
for reads the benchmark then runs the command itself and reads the change in
`serverStatus` `metrics.queryExecutor.scanned` and `scannedObjects`: every
document must come through an index key, and a `find`, or a pipeline of only
`$match`, `$sort`, and `$limit`, must read no more documents than it returned
or a tenth of the collection. A server without those
counters skips this part. The assertion runs against MongoDB as well, which
validates it.
Covered: flat equality, range, and `$in`; compound prefix and full; dotted,
deep dotted, and through-array equality, range, and `$in`; sort with limit,
ascending and descending, on a dotted field, on a multikey index holding
empty, nested, and multi-element arrays, and under an unindexed filter; sort
without a limit across getMore batches;
sort with limit on a compound index whose leading field is bound by equality,
and by a range, where the keys are sorted before the fetch;
plan choice between a filter's index and the sort's, both outcomes;
aggregation `$match`, `$sort + $limit`, `$match + $sort + $limit`, and
`$match + $group`; a hint by index name and by key pattern overriding the
planner's choice, and a `$natural` hint forcing a collection scan; partial
indexes with equality and range conditions, a sparse index, and a hint onto a
sparse index; a range and a sorted limit on a collated index under the same
collation; ranges covering just under and just over half the collection; `$or` across
indexed fields; a projection covered by the index, which must read no
documents; null equality; `$exists` on a sparse index; an anchored regex;
updateOne, deleteOne, and findAndModify filtered on an index, and upserts
against a unique index, each run once before timing to check its reads;
count with equality, range, and dotted range; distinct, unfiltered and
filtered on the distinct field or another indexed field; unique inserts maintaining flat, dotted, and through-array indexes;
non-unique multikey inserts; updates moving a flat or dotted indexed value;
and deletes removing through-array index entries, each followed by an
index-only count proving the index reflects the change. A known
DumboDB gap is declared with its tracking issue: it is logged while it persists
and fails the benchmark once DumboDB starts using the index, so the expectation
gets promoted.

The same check runs before timing every `*_Indexed` benchmark above, against
the index that benchmark creates. Joins check the foreign index; `GraphLookup`
is unchecked because MongoDB's explain does not report its index use.

## Scope - deferred

These are not implemented yet:

- `$unwind + $group`
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
| `-test-timeout`    | `10m` | `-timeout` for each benchmark run. |
| `-dumbodb-bin`, `-mongod-bin` | `""` | Run both targets as local processes instead of containers (set both) |
| `-work-dir`        | temp dir | Server data and logs for `-dumbodb-bin`/`-mongod-bin`. On Linux the run fails if this is on tmpfs or ramfs. |
| `-dumbodb-version`, `-mongodb-version` | from `--version` or the image | Version labels for the report |
| `-email-from`, `-email-to`, `-email-region`, `-email-subject` | `""` | Email the report via SES |
| `-report-dir`      | `""` | Write the report (txt, html, csv) here |
| `-alert-cmd`       | `""` | Shell command given the text report on stdin |

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
on top can rely on them.

Each benchmark runs against fresh servers: the runner recreates both
containers (or, with `-dumbodb-bin`/`-mongod-bin`, restarts both processes on
empty data directories) before every benchmark, so no result depends on what
ran before it. With `-no-containers` the servers are not restarted.

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

## Regression gate (`cmd/perfgate`)

`perfgate` compares two DumboDB builds rather than DumboDB with MongoDB. The
`Performance` workflow in the dumbodb repository runs it on every PR (head vs
the PR's base on main) and every push to main (vs the previous tip), split
across four parallel jobs.

- Both builds run on the same machine, with server data on a memory
  filesystem so disk variance does not mask code changes.
- Iteration counts are calibrated once per benchmark (`-calibrate`, default
  300ms of timed work) and reused for every sample.
- Each round starts both servers on fresh data directories and runs every
  benchmark against each, alternating which build goes first.
- A benchmark regresses when head's median is more than `-threshold` (10%)
  slower and a one-sided Mann-Whitney test gives p < `-alpha` (0.05). With
  the default 5 samples per build that requires all head samples to be slower
  than nearly all base samples. Suspected regressions get `-confirm-samples`
  (5) more samples per build before the verdict.

```bash
go run ./benchmarks/cmd/perfgate \
    -base-bin /tmp/dumbodb-base -head-bin /tmp/dumbodb-head \
    -data-root /mnt/perf -shard 0 -shards 4 -v
```

To measure noise, dispatch the workflow with both `base` and `head` set to
`main`; any regression it reports is a false positive.

## Unattended runs: local binaries and reports

For a host cron (as dumbodb's `cmd/soak` is run), `compare` can run without
Docker and send a report:

- `-dumbodb-bin` / `-mongod-bin` start each server from a local binary on a
  free port with a fresh on-disk data directory (under `-work-dir`), one
  target at a time, and stop it afterwards. `-work-dir` must be disk-backed:
  on a RAM filesystem syncs are free and write latencies are meaningless.
  Set it explicitly where `/tmp` is tmpfs.
- `-email-from`/`-email-to`/`-email-region` (SES), `-report-dir`, and
  `-alert-cmd` send a report: the summary (median and geometric mean
  multiplier) and the 5 worst benchmarks, with the results CSV attached. A
  failed run still reports, with the error and whatever results it has.

```bash
go run ./benchmarks/cmd/compare \
    -dumbodb-bin /path/to/dumbodb -mongod-bin /usr/bin/mongod \
    -test-timeout 110m -csv results.csv \
    -email-from sender@example.com -email-to you@example.com -email-region us-west-2
```

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

- **Writes are journaled on both targets**: the client sets `j:true`, so
  MongoDB acknowledges a write only after its journal is on disk. DumboDB
  always does this; comparing it against MongoDB's default (unjournaled
  acknowledgment) would compare durable writes with non-durable ones.
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
