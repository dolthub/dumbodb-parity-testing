#!/usr/bin/env bash
#
# Run one concurrency scenario against a running DumboDB server and print a
# clear PASS/FAIL summary. Exits with the harness exit code (0 pass, 1 fail,
# 3 inconclusive), so it composes in CI.
#
# Usage:
#   ./run.sh --scenario cas --mode fieldTouched --workers 32 --duration 30m
#
# Flags (all optional except --scenario):
#   --scenario NAME   cas, uuid-cas, blind-inc, disjoint-set, same-set,
#                     identical-set, divergent-cas, field-divergent-matrix-*
#   --mode MODE       fieldTouched (default), fieldDivergent, documentTouched,
#                     documentDivergent, or "" for the server default
#   --workers N       default 32
#   --duration D      default 30m (ignored if --operations is set)
#   --operations N    cap by operation count instead of duration
#   --cas-delay X     deterministic read->update delay, e.g. 2ms (widens the
#                     contention window; makes rare races surface faster)
#   --payload N       retained payload bytes (0 inline, e.g. 8192 out-of-band)
#   --seed N          default 1
#   --name LABEL      output file label (default: scenario_mode_timestamp)
#
# Env overrides (see lib.sh): URI, SKIP_BUILD=1.
set -euo pipefail
cd "$(dirname "$0")"
. ./lib.sh

SCENARIO="" MODE="fieldTouched" WORKERS=32 DURATION=30m OPERATIONS=0
CAS_DELAY=0 PAYLOAD=0 SEED=1 NAME=""

while [ $# -gt 0 ]; do
  case "$1" in
    --scenario)   SCENARIO=$2; shift 2 ;;
    --mode)       MODE=$2; shift 2 ;;
    --workers)    WORKERS=$2; shift 2 ;;
    --duration)   DURATION=$2; shift 2 ;;
    --operations) OPERATIONS=$2; shift 2 ;;
    --cas-delay)  CAS_DELAY=$2; shift 2 ;;
    --payload)    PAYLOAD=$2; shift 2 ;;
    --seed)       SEED=$2; shift 2 ;;
    --name)       NAME=$2; shift 2 ;;
    *) die "unknown flag '$1'" ;;
  esac
done
[ -n "$SCENARIO" ] || die "--scenario is required"

ensure_dirs
if [ "${SKIP_BUILD:-0}" != "1" ]; then
  log "building harness (GOWORK=off)"
  ( cd "$HARNESS_DIR" && GOWORK=off go build -o "$HARNESS_BIN" ./cmd/concurrency ) \
    || die "harness build failed"
fi
[ -x "$HARNESS_BIN" ] || die "harness binary not found at $HARNESS_BIN"

label=${NAME:-"${SCENARIO}_${MODE:-default}_$(date +%Y%m%d-%H%M%S)"}
out="${RESULTS_DIR}/${label}.json"
db="conc_${label//[^A-Za-z0-9_]/_}"

# Duration vs operations: --operations wins when set.
limit=( -duration "$DURATION" )
[ "$OPERATIONS" -gt 0 ] && limit=( -operations "$OPERATIONS" )

log "scenario=$SCENARIO mode=${MODE:-default} workers=$WORKERS ${limit[*]} cas-delay=$CAS_DELAY payload=$PAYLOAD"
log "output -> $out"

set +e
"$HARNESS_BIN" \
  -uri="$URI" \
  -scenario="$SCENARIO" \
  -merge-mode="$MODE" \
  -workers="$WORKERS" \
  "${limit[@]}" \
  -cas-delay="$CAS_DELAY" \
  -payload-bytes="$PAYLOAD" \
  -seed="$SEED" \
  -database="$db" \
  -collection=documents \
  -output="$out"
code=$?
set -e

# Human-readable summary from the JSON, including the CAS collision findings
# (the interleavings to debug) when duplicates were recorded.
python3 - "$out" <<'PY' || true
import json, sys
try:
    r = json.load(open(sys.argv[1]))
except Exception as e:
    print("could not read result:", e); sys.exit(0)
L = r.get("Ledger", {}); C = L.get("CAS", {})
print("-" * 60)
print(f"verdict   : {r.get('Verdict')}   revision {r.get('Revision')}")
print(f"scenario  : {r.get('Scenario')}  mode {r.get('Config',{}).get('MergeMode') or 'default'}")
print(f"attempts  : {L.get('Attempts')}  matched {L.get('Matched')}  noMatch {L.get('NoMatch')}")
print(f"rejected  : {L.get('Rejected')}  indeterminate {L.get('Indeterminate')}")
print(f"duplicates: {C.get('DuplicateMatches')}   invalidEdges {C.get('InvalidEdges')}")
for c in r.get("Checks") or []:
    if not c.get("Passed") and not c.get("Skipped"):
        print(f"  FAILED  {c.get('Name')}: {c.get('Detail')}")
finds = C.get("Findings") or []
if finds:
    print("collision findings (each = two writers that both won one generation):")
    for f in finds[:10]:
        print(f"  gen {f.get('ObservedGeneration')}: "
              f"w{f.get('FirstWorker')}(seq {f.get('FirstSequence')}) vs "
              f"w{f.get('CompetingWorker')}(seq {f.get('CompetingSequence')})")
print("-" * 60)
PY

exit $code
