#!/usr/bin/env bash
#
# Build and manage a DumboDB server for concurrency tests.
#
#   ./server.sh start [mode]   build (via make) and start the server; mode is
#                              auto-commit (default), session-isolation, or bare
#   ./server.sh stop           stop the server
#   ./server.sh status         report whether the server is up, with its revision
#
# The default mode is auto-commit, which is the per-write reconcile path the CAS
# scenarios exercise. Do not use bare mode for CAS tests: it does not reconcile,
# and the numbers are meaningless.
#
# Env overrides (see lib.sh): DUMBODB_DIR, PORT, DATA_DIR, SKIP_BUILD=1.
set -euo pipefail
cd "$(dirname "$0")"
. ./lib.sh

DUMBODB_BIN="${DUMBODB_DIR}/.runtime/bin/dumbodb"

server_revision() { git -C "$DUMBODB_DIR" describe --tags --always --dirty 2>/dev/null || echo unknown; }

do_stop() {
  local pid
  if pid=$(server_pid); then
    log "stopping server pid $pid"
    kill "$pid" 2>/dev/null || true
    sleep 2
    kill -9 "$pid" 2>/dev/null || true
  fi
  # Fallback: any stray server on our address.
  pkill -9 -f "dumbodb -addr ${HOST}:${PORT}" 2>/dev/null || true
  rm -f "$SERVER_PID_FILE"
}

do_start() {
  local mode=${1:-auto-commit} flag=""
  case "$mode" in
    auto-commit)       flag="-auto-commit" ;;
    session-isolation) flag="-session-isolation" ;;
    bare)              flag="" ;;
    *) die "unknown mode '$mode' (use auto-commit, session-isolation, or bare)" ;;
  esac

  ensure_dirs
  do_stop
  sleep 1

  if [ "${SKIP_BUILD:-0}" != "1" ]; then
    log "building server in $DUMBODB_DIR (make build)"
    ( cd "$DUMBODB_DIR" && make build >/dev/null ) || die "server build failed"
  fi
  [ -x "$DUMBODB_BIN" ] || die "server binary not found at $DUMBODB_BIN (build it, or unset SKIP_BUILD)"

  rm -rf "$DATA_DIR"; mkdir -p "$DATA_DIR"
  : > "$SERVER_LOG"
  log "starting server ($mode) on ${HOST}:${PORT}, revision $(server_revision)"
  nohup "$DUMBODB_BIN" -addr "${HOST}:${PORT}" $flag \
    -data-dir "$DATA_DIR" -log-level info > "$SERVER_LOG" 2>&1 &
  echo $! > "$SERVER_PID_FILE"
  disown 2>/dev/null || true

  if wait_for_line "$SERVER_LOG" "Listening on TCP" 60; then
    log "server up (pid $(cat "$SERVER_PID_FILE"), revision $(server_revision))"
  else
    log "server did not report readiness; last log lines:"
    tail -n 15 "$SERVER_LOG" >&2 || true
    die "server failed to start"
  fi
}

do_status() {
  local pid
  if pid=$(server_pid); then
    echo "up: pid $pid on ${HOST}:${PORT}, revision $(server_revision)"
  else
    echo "down"
    return 1
  fi
}

case "${1:-}" in
  start)  shift; do_start "${1:-auto-commit}" ;;
  stop)   do_stop ;;
  status) do_status ;;
  *) die "usage: $0 {start [auto-commit|session-isolation|bare] | stop | status}" ;;
esac
