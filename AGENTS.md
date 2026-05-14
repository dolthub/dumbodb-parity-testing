# Agent Instructions

## Purpose

The dumbodb-parity-testing repository tests parity between DumboDB and its two
inspirations: MongoDB and Dolt.

- **MongoDB parity**: DumboDB speaks the MongoDB wire protocol, so the primary test suite
  (`tests/`) runs every operation against both MongoDB 8 and DumboDB side-by-side and
  compares responses. Tests use `harness.PairTest` with one of three support levels:
  `DumboDBFull` (divergence fails CI), `DumboDBMongoOnly` (DumboDB skipped), or
  `DumboDBXFail` (divergence expected, not fatal).

- **Load and latency benchmarks**: The `benchmarks/` package measures per-operation
  latency (ns/op) against a single target. The `cmd/compare` runner orchestrates dual
  runs against both servers and produces a side-by-side comparison table.

- **Storage and merge behaviour vs Dolt**: The `storage/` package compares DumboDB and
  Dolt directly on branch + merge workloads, measuring on-disk storage growth and merge
  time scaling. This exposes the difference between DumboDB's current O(N) index rebuild
  and Dolt's O(diff) incremental merge.

---

## Rules

- Do not commit unless explicitly asked.
- Do not push directly to main on either repo. Always branch + PR.
- All new Go files must include the DoltHub Apache 2.0 copyright header (see any file in `storage/` for the exact text).
- reproduce locally first, identify the crashing line, then escalate with evidence if you cannot fix it.
- All go code must contain 7 bit ASCII only.

---

## Repo layout

```
harness/        PairTest harness for MongoDB vs DumboDB parity tests
tests/          Parity test suite (31 files, DumboDB vs MongoDB 8)
benchmarks/     Per-operation latency benchmarks (single target, ns/op)
storage/        Dolt vs DumboDB storage/merge comparison tests
```

---

## Building

The repo is not part of the workspace `go.work` at `/workspace/go.work`. Always
disable the workspace when building or running tests here:

```bash
GOWORK=off go build ./...
GOWORK=off go vet ./...
GOWORK=off go test ./tests/...
```

---

## Local test environment setup

To reproduce CI failures locally, you need both repos and a running MongoDB:

```bash
# 1. Your primary worktree is dumbodb-parity-testing (already cloned here)
# 2. Clone dumbodb alongside it
git clone https://github.com/dolthub/dumbodb /tmp/dumbodb-src

# 3. Build dumbodb binary
cd /tmp/dumbodb-src && go build -o /tmp/dumbodb-bin ./cmd/dumbodb/

# 4. Start MongoDB (Docker)
docker run -d --name mongo8 -p 27017:27017 mongo:8.0

# 5. Start dumbodb
DUMBODB_BIN=/tmp/dumbodb-bin $DUMBODB_BIN --listen 127.0.0.1:27018 &

# 6. Run the parity suite
GOWORK=off go test ./... -v 2>&1 | tee /tmp/parity-run.log
```

---

## Running storage tests

Requires both servers running as Docker containers with data directories
mounted. Set all four env vars before running:

```
DUMBODB_URI      MongoDB URI for DumboDB  (default: mongodb://localhost:27018)
DUMBODB_DATA_DIR host path to DumboDB data directory (required)
DOLT_URI         MySQL DSN for dolt sql-server (default: root:@tcp(localhost:3306)/)
DOLT_DATA_DIR    host path to Dolt data directory (required)
```

```bash
GOWORK=off go test ./storage/... -v -run TestMergeStorage_LargeBaseTinyDiff -timeout 30m
```

---

## Investigating panics

When CI shows a panic:

```bash
# Check recent CI runs
gh run list --repo dolthub/dumbodb-parity-testing --limit 5

# Download logs from a failing run
gh run view <run-id> --log-failed --repo dolthub/dumbodb-parity-testing
```

Identify whether the panic is in:
- **The parity harness** (pair.go, setup.go, etc.) -- fix in dumbodb-parity-testing
- **DumboDB itself** (server panic) -- fix in dumbodb

---

## Pushing fixes

**Fix in dumbodb-parity-testing** (this repo):
```bash
git push origin <branch>
gh pr create --repo dolthub/dumbodb-parity-testing ...
```

**Fix in dumbodb**:
```bash
git -C /tmp/dumbodb-src push origin <branch>
gh pr create --repo dolthub/dumbodb ...
```

---

## Adding parity tests

1. Add a `*_test.go` file under `tests/`.
2. Use `harness.PairTest(t, harness.TestCase{...})`.
3. Choose `DumboDBFull`, `DumboDBMongoOnly`, or `DumboDBXFail`.
4. Do not regress the parity score.

---

## Workflow

1. `gt hook` -- find your assignment
2. Check CI: `gh run list --repo dolthub/dumbodb-parity-testing --limit 5` (authentication provided as needed)
3. Download failing logs and identify the panic/failure
4. Reproduce locally (setup above)
5. Fix in the correct repo, push branch, open PR
6. Verify CI goes green on your PR
7. Report to mayor with: root cause, which repo, PR link, CI status
