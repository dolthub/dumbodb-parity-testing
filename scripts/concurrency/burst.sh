#!/usr/bin/env bash
#
# Series of short burst tests: hammer a scenario with many short runs to catch a
# write that was rejected to the client but saved on the server.
#
# WHY THIS EXISTS (the "final write" blind spot)
# disjoint-set is the one scenario that can see a saved-but-rejected write: each
# worker $sets its own field to a monotonically increasing sequence, so if the
# stored field is HIGHER than that worker's last acknowledged sequence, a write
# it was told failed was actually persisted. But that check only sees each
# worker's FINAL write -- a saved-reject that a later write overwrote is
# invisible. A long run therefore gives just one final-write check per worker;
# many short runs give one per worker per BURST, multiplying detection by the
# number of bursts. That is what caught workspace-1bk.9.8.8.1 (server rejects a
# committed write with "dataset head is not ancestor of commit").
#
# Usage: ./burst.sh [scenario] [mode] [ops-per-burst] [max-bursts]
#   defaults: disjoint-set fieldDivergent 5000 1000   (0 max = until Ctrl-C)
# Stops on the first failing burst (that is the catch) and keeps its evidence.
set -uo pipefail
cd "$(dirname "$0")"
. ./lib.sh

SCEN=${1:-disjoint-set}
MODE=${2:-fieldDivergent}
OPS=${3:-5000}
MAX=${4:-1000}
WORKERS=${WORKERS:-32}
ARCHIVE=${ARCHIVE:-${RUN_DIR}/burst-archive}
mkdir -p "$ARCHIVE"

stop=0
trap 'stop=1' INT TERM
cleanup() { ./server.sh stop >/dev/null 2>&1 || true; }
trap cleanup EXIT

./server.sh start auto-commit >/dev/null 2>&1 || { ./server.sh start auto-commit; die "server failed to start"; }
log "building harness (GOWORK=off)"
( cd "$HARNESS_DIR" && GOWORK=off go build -o "$HARNESS_BIN" ./cmd/concurrency ) || die "harness build failed"

log "burst: $SCEN / $MODE, ${OPS} ops x up to ${MAX:-inf} bursts, $WORKERS workers, rev $(git -C "$DUMBODB_DIR" describe --tags --always 2>/dev/null || echo '?')"
out="${RESULTS_DIR}/burst-${SCEN}-${MODE}.json"
i=0
while [ "$stop" -eq 0 ]; do
  i=$((i + 1))
  { [ "$MAX" -gt 0 ] && [ "$i" -gt "$MAX" ]; } && { i=$((i - 1)); break; }

  SKIP_BUILD=1 ./run.sh --scenario "$SCEN" --mode "$MODE" --workers "$WORKERS" \
    --operations "$OPS" --name "burst-${SCEN}-${MODE}" > "${ARCHIVE}/last.out" 2>&1
  rc=$?
  verdict=$(python3 -c "import json,sys; print(json.load(open('$out')).get('Verdict','?'))" 2>/dev/null || echo "?")

  if [ "$rc" -eq 0 ] && [ "$verdict" = conclusivePass ]; then
    [ $((i % 25)) -eq 0 ] && printf '\r  %d bursts clean...' "$i"
  else
    dst="${ARCHIVE}/catch-${i}-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$dst"
    cp -f "$out" "$dst/" 2>/dev/null || true
    cp -f "${ARCHIVE}/last.out" "$dst/run.out" 2>/dev/null || true
    cp -f "$SERVER_LOG" "$dst/server.log" 2>/dev/null || true
    printf '\n\nCAUGHT on burst %d (verdict=%s rc=%s). Evidence: %s\n' "$i" "$verdict" "$rc" "$dst"
    grep -E "FAILED|rejected|error" "${ARCHIVE}/last.out" | grep -vi "setlocale" | head
    echo
    exit 1
  fi
done

printf '\n%d bursts, all clean.\n' "$i"
exit 0
