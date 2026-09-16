# Shared configuration and helpers for the concurrency test scripts.
# Sourced by server.sh, run.sh, and the repro scripts. Not meant to be run directly.
#
# Override any of these with environment variables, e.g.:
#   DUMBODB_DIR=/path/to/dumbodb PORT=27099 ./server.sh start

# Repo locations.
HARNESS_DIR=${HARNESS_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}
DUMBODB_DIR=${DUMBODB_DIR:-/workspace/dumbodb}

# Server endpoint.
HOST=${HOST:-127.0.0.1}
PORT=${PORT:-27018}
URI=${URI:-mongodb://${HOST}:${PORT}}

# Runtime working area: server data dir, logs, pidfile, and result JSON.
# Kept out of the repos so nothing is accidentally committed.
RUN_DIR=${RUN_DIR:-/tmp/dumbo-concurrency}
DATA_DIR=${DATA_DIR:-${RUN_DIR}/data}
RESULTS_DIR=${RESULTS_DIR:-${RUN_DIR}/results}
SERVER_LOG=${SERVER_LOG:-${RUN_DIR}/server.log}
SERVER_PID_FILE=${SERVER_PID_FILE:-${RUN_DIR}/server.pid}
HARNESS_BIN=${HARNESS_BIN:-${RUN_DIR}/concurrency}

log()  { printf '[%s] %s\n' "$(date +%H:%M:%S)" "$*" >&2; }
die()  { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

ensure_dirs() { mkdir -p "$RUN_DIR" "$RESULTS_DIR"; }

# Wait until a line appears in a file, or time out. Used to detect server readiness
# without needing a mongo client, nc, or /dev/tcp (none are guaranteed present).
wait_for_line() {
  local file=$1 needle=$2 timeout=${3:-60} waited=0
  while [ "$waited" -lt "$timeout" ]; do
    [ -f "$file" ] && grep -q "$needle" "$file" && return 0
    sleep 1
    waited=$((waited + 1))
  done
  return 1
}

server_pid() {
  [ -f "$SERVER_PID_FILE" ] || return 1
  local pid
  pid=$(cat "$SERVER_PID_FILE")
  kill -0 "$pid" 2>/dev/null && { echo "$pid"; return 0; }
  return 1
}
