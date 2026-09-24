#!/usr/bin/env bash
#
# Run the concurrency suite in a loop until you stop it (Ctrl-C). Built for
# unattended multi-day soak hunting: it keeps going after a failing iteration
# (that is the point -- catch rare intermittent races over many passes),
# archives full evidence for every failure, and prints a summary on exit.
#
# Usage:
#   ./weekend-loop.sh [profile] [per-iteration-timeout]
#     profile   soak (default) | smoke
#     timeout   max wall-clock per iteration (default 16h), e.g. 16h, 900m
#
#   Stop with Ctrl-C. A running tally is printed after each pass and written to
#   the loop log; failing iterations keep their full evidence.
#
# Rough pass durations (defaults): soak ~= 12-13h (25 concurrent cases x 30m +
# matrices); smoke ~= 25-30m. Over a 48h weekend that is ~3-4 soak passes or
# ~100 smoke passes.
#
# Where things land (under RUN_DIR, default /tmp/dumbo-concurrency):
#   weekend-archive/loop.log            one line per iteration, plus start/stop
#   weekend-archive/iter-<n>-<ts>/      full evidence for failing iteration n:
#                                         suite.out, results/ (per-case JSON+TSV),
#                                         server.log
#
# Env overrides (see lib.sh): DUMBODB_DIR, PORT, SOAK_DURATION, SMOKE_OPS,
# WORKERS. SUITE_CMD overrides the suite entry point (testing hook).
set -uo pipefail
cd "$(dirname "$0")"
. ./lib.sh

PROFILE=${1:-soak}
ITER_TIMEOUT=${2:-16h}
SUITE_CMD=${SUITE_CMD:-./suite.sh}
ARCHIVE=${ARCHIVE:-${RUN_DIR}/weekend-archive}

# Each iteration also runs a burst phase after the suite: many short runs of the
# saved-rejected detector (disjoint-set), one final-write check per worker per
# burst -- the high-rate way to catch a write the server rejected but saved
# (workspace-1bk.9.8.8.1). BURST_COUNT=0 skips it.
BURST_COUNT=${BURST_COUNT:-150}
BURST_OPS=${BURST_OPS:-5000}
BURST_SCEN=${BURST_SCEN:-disjoint-set}
BURST_MODE=${BURST_MODE:-fieldDivergent}
BURST_MAXTIME=${BURST_MAXTIME:-90m}   # hang guard; normal phase finishes well under this
LOOP_LOG="${ARCHIVE}/loop.log"

case "$PROFILE" in smoke|soak) ;; *) die "unknown profile '$PROFILE' (use soak or smoke)" ;; esac
ensure_dirs
mkdir -p "$ARCHIVE"

iter=0 passes=0 fails=0
declare -a failed_iters=()
stop=0

kill_stray_servers() {
  # Anything left bound to our port from a killed/hung iteration.
  ps aux 2>/dev/null | grep "dumbodb -addr ${HOST}:${PORT}" | grep -vE 'grep|zsh|bash' \
    | awk '{print $2}' | xargs -r kill -9 2>/dev/null || true
}

tally() { echo "iterations=$iter passes=$passes fails=$fails failed=[${failed_iters[*]:-}]"; }

summary_and_exit() {
  echo
  echo "=== weekend loop stopped $(date) ==="
  tally | tee -a "$LOOP_LOG"
  if [ "$fails" -gt 0 ]; then
    echo "failing-iteration evidence under: $ARCHIVE/iter-*" | tee -a "$LOOP_LOG"
  fi
  kill_stray_servers
  exit 0
}
trap 'stop=1' INT TERM

echo "=== weekend loop start $(date) profile=$PROFILE iter-timeout=$ITER_TIMEOUT ===" | tee -a "$LOOP_LOG"
echo "stop with Ctrl-C; loop log: $LOOP_LOG" | tee -a "$LOOP_LOG"

while [ "$stop" -eq 0 ]; do
  iter=$((iter + 1))
  started=$(date +%s)
  kill_stray_servers
  echo "[iter $iter] $(date) starting $PROFILE" | tee -a "$LOOP_LOG"

  # timeout so a hung/deadlocked pass cannot stall the whole weekend.
  timeout "$ITER_TIMEOUT" $SUITE_CMD "$PROFILE" > "${ARCHIVE}/last-run.out" 2>&1
  rc_suite=$?

  # Burst phase: high-rate saved-rejected hunt (count-bounded so the hang guard
  # can't kill a burst mid-run and read as a false catch). rc 0 = clean,
  # 1 = caught, anything else (incl. 124 hang) = failure.
  rc_burst=0
  if [ "$BURST_COUNT" != "0" ] && [ "$stop" -eq 0 ]; then
    kill_stray_servers
    timeout "$BURST_MAXTIME" ${BURST_CMD:-./burst.sh} "$BURST_SCEN" "$BURST_MODE" "$BURST_OPS" "$BURST_COUNT" \
      > "${ARCHIVE}/last-burst.out" 2>&1
    rc_burst=$?
  fi

  elapsed=$(( $(date +%s) - started ))
  if [ "$rc_suite" -eq 0 ] && [ "$rc_burst" -eq 0 ]; then
    passes=$((passes + 1))
    echo "[iter $iter] PASS in ${elapsed}s | $(tally)" | tee -a "$LOOP_LOG"
  else
    fails=$((fails + 1)); failed_iters+=("$iter")
    dst="${ARCHIVE}/iter-${iter}-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$dst"
    cp -f "${ARCHIVE}/last-run.out" "${dst}/suite.out" 2>/dev/null || true
    cp -rf "$RESULTS_DIR" "${dst}/results" 2>/dev/null || true
    cp -f "$SERVER_LOG" "${dst}/server.log" 2>/dev/null || true
    if [ "$rc_burst" -ne 0 ]; then
      cp -f "${ARCHIVE}/last-burst.out" "${dst}/burst.out" 2>/dev/null || true
      cp -rf "${RUN_DIR}/burst-archive" "${dst}/burst-archive" 2>/dev/null || true
    fi
    what=""
    [ "$rc_suite" -ne 0 ] && what="suite($([ "$rc_suite" -eq 124 ] && echo TIMEOUT || echo "rc=$rc_suite"))"
    [ "$rc_burst" -ne 0 ] && what="$what burst($([ "$rc_burst" -eq 1 ] && echo CAUGHT || echo "rc=$rc_burst"))"
    echo "[iter $iter] FAIL$what in ${elapsed}s -- evidence: $dst | $(tally)" | tee -a "$LOOP_LOG"
  fi

  # Honor a Ctrl-C that arrived during the pass before starting the next one.
  [ "$stop" -eq 0 ] || break
done

summary_and_exit
