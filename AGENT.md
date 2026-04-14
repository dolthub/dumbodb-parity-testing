# Integration Rig Agent Guide

## Purpose

This rig exists to debug and fix CI failures in `dolthub/dumbodb-parity-testing`.
You have **read access to both** `dolthub/dumbodb` and `dolthub/dumbodb-parity-testing`
via `GH_TOKEN`. For pushing fixes, use the write tokens on disk (see below).

You are the bridge between the two repos. When parity CI goes red with a panic or
unexpected failure, you investigate, find the root cause, and push the fix to
whichever repo owns it.

---

## Credentials

| Operation | Token |
|-----------|-------|
| `gh` CLI queries (either repo) | `GH_TOKEN` (read both — already set in your env) |
| `git push` to dolthub/dumbodb | `$(cat /home/ubuntu/.gh_gt_token_dumbodb)` |
| `git push` to dolthub/dumbodb-parity-testing | `$(cat /home/ubuntu/.gh_gt_token_parity)` |

Git push authentication is handled automatically by the global URL rewrites.
You do not need to set tokens manually for `git push`.

Never echo or print token values. Always reference via `$(cat <file>)`.

---

## Local Test Environment Setup

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
cd <your-worktree>
go test ./... -v 2>&1 | tee /tmp/parity-run.log
```

---

## Investigating Panics

When CI shows a panic:

```bash
# Check recent CI runs
GH_TOKEN=$(cat /home/ubuntu/.gh_gt_read_both) gh run list --repo dolthub/dumbodb-parity-testing --limit 5

# Download logs from a failing run
GH_TOKEN=$(cat /home/ubuntu/.gh_gt_read_both) gh run view <run-id> --log-failed --repo dolthub/dumbodb-parity-testing
```

Identify whether the panic is in:
- **The parity harness** (pair.go, setup.go, etc.) → fix in dumbodb-parity-testing
- **DumboDB itself** (server panic) → fix in dumbodb

File a bead in the correct rig and push the fix. Push to dumbodb using the dumbodb
write token; push to parity-testing using the parity write token.

---

## Pushing Fixes

**Fix in dumbodb-parity-testing** (this repo):
```bash
git push origin <branch>
# URL rewrite handles auth automatically using parity write token
GH_TOKEN=$(cat /home/ubuntu/.gh_gt_token_parity) gh pr create --repo dolthub/dumbodb-parity-testing ...
```

**Fix in dumbodb**:
```bash
# Clone or use /tmp/dumbodb-src
git -C /tmp/dumbodb-src push origin <branch>
# URL rewrite handles auth automatically using dumbodb write token
GH_TOKEN=$(cat /home/ubuntu/.gh_gt_token_dumbodb) gh pr create --repo dolthub/dumbodb ...
```

---

## Hard Rules

- **Never modify `known-divergences.txt`** — neil-only, no exceptions.
- **Do not push directly to main** on either repo. Always branch + PR.
- **Do not escalate panics as "unknown"** — reproduce locally first, identify the
  crashing line, then escalate with evidence if you cannot fix it.

---

## Workflow

1. `gt hook` — find your assignment
2. Check CI: `GH_TOKEN=$(cat /home/ubuntu/.gh_gt_read_both) gh run list --repo dolthub/dumbodb-parity-testing --limit 5`
3. Download failing logs and identify the panic/failure
4. Reproduce locally (setup above)
5. Fix in the correct repo, push branch, open PR
6. Verify CI goes green on your PR
7. Report to mayor with: root cause, which repo, PR link, CI status
