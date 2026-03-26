# dongo-parity-testing

A/B test harness that runs operations against **MongoDB 8** and **Dongo** side-by-side and compares responses. MongoDB 8 is the oracle — there are no hardcoded expected values. If MongoDB and Dongo agree, the test passes.

---

## What this is

Each test in the suite runs the same operation against both servers, then compares the results using a fuzzy comparator that normalizes non-deterministic values (ObjectIds, timestamps) before diffing. The harness supports three modes:

| Mode | Meaning |
|---|---|
| `DongoFull` | Run on both servers; divergence fails CI |
| `DongoMongoOnly` | Run on MongoDB only; Dongo skipped (deprecated/unsupported feature) |
| `DongoXFail` | Run on both; Dongo divergence recorded but not a CI failure |

---

## Prerequisites

- **Go 1.21+**
- **Docker** (for the MongoDB 8 container)
- **Dongo binary** — built from [dolthub/dongo](https://github.com/dolthub/dongo)

---

## Running locally

### 1. Start MongoDB 8

```bash
docker run -d --name mongo8 -p 27017:27017 mongo:8.0
```

### 2. Build and start Dongo

```bash
git clone https://github.com/dolthub/dongo.git
cd dongo
go build -o dongo ./cmd/dongo
./dongo --port 27018 &
```

### 3. Run the full suite

```bash
go test ./...
```

To run a single test file or specific test:

```bash
go test ./tests/ -run TestInsertOne
```

### Environment variables

| Variable | Default | Description |
|---|---|---|
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection URI |
| `DONGO_URI` | `mongodb://localhost:27018` | Dongo connection URI |

Example — non-default ports:

```bash
MONGO_URI=mongodb://localhost:27099 DONGO_URI=mongodb://localhost:27098 go test ./...
```

---

## Test result legend

| Result | Meaning |
|---|---|
| **PASS** | MongoDB and Dongo returned identical responses |
| **FAIL** | MongoDB and Dongo diverged (unexpected — breaks CI for `DongoFull` tests) |
| **SKIP** | Test ran on MongoDB only (`DongoMongoOnly` mode); Dongo was not exercised |
| **XFAIL** | Dongo diverged, but this was expected (`DongoXFail` mode); not a CI failure |

At the end of a run, the harness prints a summary:

```
Parity Summary
  Matching:    42
  Diverging:    0
  Mongo-only:   5
  XFail:        3
  Total:       50
```

CI exits non-zero only when `Diverging > 0`.

---

## Known divergences

See [`known-divergences.txt`](known-divergences.txt) for documented cases where Dongo intentionally differs from MongoDB.

> **Note:** `known-divergences.txt` is managed exclusively by the project owner (neil). Do not edit it directly — see [`AGENT.md`](AGENT.md) for the governance rule.

---

## Repo layout

```
dongo-parity-tesing/
├── AGENT.md                        Agent/contributor rules (read before writing tests)
├── README.md                       This file
├── go.mod                          Module: github.com/dolthub/dongo-parity-testing
├── harness/
│   ├── pair.go                     PairTest() — dual-client runner
│   ├── compare.go                  Fuzzy comparator (ObjectId/timestamp normalization)
│   ├── setup.go                    Per-test DB isolation, shared client singleton
│   └── report.go                   TestResult, Summary, PASS/FAIL/SKIP/XFAIL types
├── tests/
│   ├── crud_test.go                InsertOne/Many, Find/FindOne, Update/Delete, BulkWrite
│   └── query_test.go               Comparison, logical, element, array operators
├── known-divergences.txt           Neil-only managed list of expected divergences
└── .github/workflows/
    └── parity.yml                  CI: mongo:8.0 service + Dongo build from source
```

---

## Adding tests

1. Add a `*_test.go` file under `tests/`.
2. Use `harness.PairTest(t, harness.TestCase{...})` — see existing tests for examples.
3. Choose the right support level (`DongoFull`, `DongoMongoOnly`, `DongoXFail`).
4. Run `go test ./...` locally before pushing — do not regress the parity score.

See [`AGENT.md`](AGENT.md) for the full contributor guide.
