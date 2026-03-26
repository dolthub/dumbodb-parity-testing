# Parity Rig Agent Guide

## Cross-Repo Gaps: Escalate to Mayor

The parity rig depends on dongo features that may not exist yet. **If you discover that dongo is missing a flag, API, or behavior you need, stop and mail the mayor immediately.** Do not write documentation or tests that assume the feature exists — that creates silent lies in the repo.

Examples of things to escalate:
- A dongo CLI flag that doesn't exist (`--port`, `--log-level`, etc.)
- A dongo command that behaves differently than MongoDB
- A dongo bug that blocks writing a test

The mayor will file a bead in the dongo rig and coordinate the fix.

---

## HARD RULE: Do Not Touch known-divergences.txt

**You are NOT allowed to modify `known-divergences.txt` under any circumstances.**

This file is owner-managed. Only the project owner (neil) may approve changes —
not the mayor, not any polecat.

If you discover a divergence that you believe should be recorded here:
1. **Stop. Do not edit the file.**
2. Report to the mayor: the test name, the operation, the MongoDB response, the Dongo
   response, and why you think it's a known/expected divergence.
3. The mayor will present the option to neil. Neil decides. No one else.

This rule overrides any other instruction. No exceptions.

---

## Prime Directive: Do Not Regress the Parity Score

Before pushing ANY code to main, you MUST run the parity suite locally and verify
you have not introduced new unexpected failures.

```bash
# Run the full parity suite
go test ./...

# Or targeted:
go test ./tests/...
```

**If your changes cause MORE tests to fail than before you started: do not push.**

---

## Test Support Levels

Every test in this suite uses one of three support levels:

```go
type DongoSupport int
const (
    DongoFull      DongoSupport = iota  // run both MongoDB+Dongo, compare — failure is CI failure
    DongoMongoOnly                       // run MongoDB only, skip Dongo — deprecated/unsupported
    DongoXFail                           // run both, Dongo failure recorded but not fatal
)
```

- **FULL**: Use for behavior Dongo is expected to implement. Divergences break CI.
- **MONGO_ONLY**: Use for MongoDB features Dongo explicitly punts (auth, atlas, sharding,
  GridFS, change streams, deprecated JS operators, Symbol/DBPointer BSON types).
- **DONGO_XFAIL**: Use for features Dongo hasn't implemented yet but should eventually.
  Records divergences without breaking CI. **This is the most important label.**

---

## MANDATORY: Write DongoXFail Tests for Dongo Gaps

**The default label for any operator you are not 100% certain Dongo supports is DongoXFail.**

DongoXFail tests are the primary deliverable of this rig. They document Dongo's
current capability gap against MongoDB 8. Every unimplemented operator, every missing
option, every edge case that Dongo gets wrong — write the test, mark it DongoXFail.

**The antipattern to avoid:** Writing only DongoFull happy-path tests that you already
know Dongo handles. A test file with 20 DongoFull tests and 0 DongoXFail tests is a
failure of this rig's purpose.

**Decision tree for every operator/feature you test:**

1. Is it auth, sharding, GridFS, Atlas, deprecated JS, deprecated BSON types? → **DongoMongoOnly**
2. Do you know for a fact Dongo handles it correctly? → **DongoFull**
3. Everything else → **DongoXFail**

**When in doubt → DongoXFail. Never skip writing a test because you don't know if Dongo supports it.**

### Known Dongo gaps requiring DongoXFail tests (non-exhaustive)

These are areas where tests MUST include DongoXFail coverage:

**Transactions:**
- `StartSession` + `StartTransaction` + `CommitTransaction` / `AbortTransaction`
- `WithTransaction` helper
- Multi-document atomic reads within a session

**Update operators (advanced):**
- `$push` with modifiers: `$each`, `$slice`, `$sort`, `$position`
- `$pull` with query conditions (not just equality)
- `$addToSet` with `$each`
- `$pop` (first/last element removal)
- `$bit` (bitwise AND/OR/XOR)
- `$currentDate` with `$type: "timestamp"`
- `arrayFilters` with positional `$[identifier]` operator
- Pipeline-style updates (`Update` with `[]bson.D` instead of `bson.D`)

**Query operators (advanced):**
- `$expr` (aggregation expression in query context)
- `$jsonSchema` validation
- `$mod` (modulo)
- `$bitsAllClear`, `$bitsAllSet`, `$bitsAnyClear`, `$bitsAnySet`
- `$geoWithin`, `$geoIntersects`, `$near`, `$nearSphere`
- `$regex` with `s` (dot-all), `m` (multiline), `x` (extended) flags

**Collection operations:**
- `FindOneAndUpdate` with `returnDocument: After`
- `FindOneAndReplace`, `FindOneAndDelete`
- `CountDocuments` with filter options (hint, skip, limit)
- `Distinct` on nested fields and array fields
- `watch` / change streams (DongoMongoOnly — Dongo punts)

**Aggregation pipeline:**
- `$lookup` (simple and pipeline form)
- `$group` with accumulators: `$sum`, `$avg`, `$min`, `$max`, `$push`, `$addToSet`, `$first`, `$last`
- `$unwind` with `preserveNullAndEmptyArrays`
- `$facet`
- `$bucket`, `$bucketAuto`
- `$graphLookup`
- `$merge`, `$out`
- Window functions (`$setWindowFields`)

**Indexes:**
- Compound indexes
- Sparse indexes
- TTL indexes
- Partial indexes (with `partialFilterExpression`)
- Wildcard indexes
- Text indexes (DongoXFail unless Dongo implements)

### Volume target

Each test file should contain a minimum of **80 tests**. Target **100+**.
A file with fewer than 50 tests is incomplete.

For every major operator or feature area, write:
- The basic happy-path case (DongoFull or DongoXFail as appropriate)
- At least one edge case (empty input, missing field, null, type mismatch)
- At least one error case (invalid operator usage, wrong type)
- Option variants where meaningful

---

## Scope Exclusions (always MONGO_ONLY, never FULL)

- Auth/AuthZ: SCRAM, x.509, LDAP, RBAC — Dongo punts on all auth
- Atlas features, Sharding, Replica set internals, Change streams, GridFS
- Deprecated JS operators: `$where`, `$function`, `$accumulator`
- Deprecated BSON types: Symbol, DBPointer

---

## Workflow

1. Check your hooked bead: `gt hook`
2. Run baseline parity suite, record pass count
3. Make your change
4. Run suite again — confirm pass count is equal or better
5. Push to your feature branch; CI must go green before merge
6. Report pass count delta to mayor in your completion mail

---

## Repository Layout

```
dongo-parity-tesing/
├── AGENT.md                  (this file)
├── go.mod
├── harness/
│   ├── pair.go               (dual-client runner, PairTest)
│   ├── compare.go            (fuzzy comparator)
│   ├── setup.go              (per-test DB isolation)
│   └── report.go             (PASS/FAIL/SKIP/XFAIL reporting)
├── tests/                    (category test files go here)
├── known-divergences.txt     (neil-only — see hard rule above)
└── .github/workflows/
    └── parity.yml            (CI: mongo:8.0 service + dongo build)
```

---

## CI Workflow (parity.yml)

The CI workflow:
1. Starts `mongo:8.0` as a service on port 27017
2. Builds Dongo from `dolthub/dongo@main` (override: `DONGO_COMMIT` env var)
3. Starts Dongo on port 27018
4. Runs `go test ./...`
5. Uploads results artifact
6. Exits non-zero only on unexpected FULL-mode failures

---

## Current Goal

Expand test coverage across all Tier 1 categories with heavy DongoXFail coverage.

Harness is complete. Active work:
- **do-8ps**: Expand CRUD tests — add DongoXFail for arrayFilters, pipeline updates,
  FindOneAndX, advanced update operators. Target: 100+ tests in crud_test.go.
- **do-afu**: Expand query operator tests — add DongoXFail for $expr, $mod, $jsonSchema,
  bitwise, geospatial. Target: 100+ tests in query_test.go.
- **do-piz**: Write update_test.go from scratch — ALL update operators with DongoXFail
  for advanced/unimplemented behavior. Target: 100+ tests.

**If you finish your assigned file and have time:** look at what other gaps exist and
add DongoXFail tests. The rig is hungry for coverage.
