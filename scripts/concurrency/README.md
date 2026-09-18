# Concurrency test scripts

Consistent, repeatable runs of the concurrency harness against a DumboDB server.
These encode the settings that are easy to get wrong (server mode, merge mode,
scale) so results are comparable across people and machines.

## Quick start

```sh
cd scripts/concurrency

# Run the CI matrix:
./suite.sh smoke          # bounded, fast -- the per-commit gate
./suite.sh soak           # 30-min runs -- nightly / release

# Reproduce the filed CAS failure (workspace-1bk.4.5) on its own:
./repro-4.5.sh            # full 30-minute soak (the race needs the volume)

# Or drive one scenario yourself:
./server.sh start                                          # build + start (auto-commit)
./run.sh --scenario uuid-cas --mode fieldTouched --operations 50000
./server.sh stop
```

## The two things that decide whether you see the CAS bug

1. **Merge mode.** `fieldTouched` requires a convergent compare-and-swap to
   conflict, so a double-match is a hard failure. `fieldDivergent` *allows*
   convergent edits to coalesce, so the same scenario passes by design. If you
   test `fieldDivergent`, you will not see the CAS failure -- that is expected,
   not a fix.

2. **Scale.** The residual CAS race is roughly 1 in 12,000 matches (measured).
   Detection is volume-limited: you need ~70,000 matches (~1.3M attempts,
   ~25-30 min at DumboDB's throughput) to reliably see one. A bounded run shows
   zero and proves nothing. There is no known accelerator -- `cas-delay` does
   not amplify it, so the smoke profile cannot gate this bug; a soak job is
   required. That is why `cas-ft` expects `pass` in smoke and `xfail` in soak.

The server must run with `-auto-commit` (the per-write reconcile path).
`server.sh start` uses it by default. Do not use `bare` mode for CAS tests.

The soak profile runs the documentTouched group with a checked-in 3-second
session timeout and 1-second sweep period. This repeatedly reaps pooled idle
sessions and makes every documentTouched CAS-family case a regression guard for
the reconnect behavior fixed by DumboDB f76ab32. Pre-fix fcc433c fails this
configuration with code 251; f76ab32 and later must return matched or no-match,
never a session-timeout rejection. Smoke runs retain the server defaults. These
settings are part of `suite.sh`, not a manual reproduction knob.

## Scripts

- `server.sh {start [mode] | stop | status}` -- build (via `make`) and manage the
  server. Modes: `auto-commit` (default), `session-isolation`, `bare`.
- `run.sh --scenario NAME [flags]` -- run one scenario, print a PASS/FAIL
  summary and any collision findings, exit with the harness code (0/1/3).
- `repro-4.5.sh [--fast]` -- the canned CAS reproduction.
- `lib.sh` -- shared config; override paths/ports via environment variables.

## Configuration (environment variables)

| var | default | meaning |
|-----|---------|---------|
| `DUMBODB_DIR` | `/workspace/dumbodb` | server repo (checked out to the revision under test) |
| `PORT` | `27018` | server listen port |
| `RUN_DIR` | `/tmp/dumbo-concurrency` | data dir, logs, and result JSON |
| `SKIP_BUILD` | unset | set to `1` to reuse existing binaries |
| `SESSION_TIMEOUT` | server default | optional `server.sh` idle-session timeout |
| `SESSION_SWEEP_PERIOD` | server default | optional `server.sh` idle-session sweep cadence |

Result JSON and the server log land under `RUN_DIR` (default
`/tmp/dumbo-concurrency`), outside the repos.
