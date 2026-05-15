#!/usr/bin/env bash
#
# Bring up the Dolt and DumboDB containers (if they're not already running),
# export the env vars the storage parity tests expect, and run the suite.
#
# WARNING: if either container is missing or not running on entry, the script
# wipes the contents of its data dir (via a throwaway busybox container, so
# root-owned files are deleted too) and creates the container fresh. Running
# containers are left alone, including their data.
#
# Any extra arguments are passed through to `go test`. For example:
#   ./run-storage-tests.sh -run TestMergeStorage_LargeBaseTinyDiff
#
# Overridable via env:
#   DATA_ROOT          parent dir for host-side data mounts
#                      (default: /home/ubuntu/dumbo_vs_dolt)
#   DOLT_DATA_DIR      bind-mounted to /var/lib/dolt inside the dolt container
#   DUMBODB_DATA_DIR   bind-mounted to /var/lib/dumbodb inside the dumbodb container
#   DOLT_ROOT_PASSWORD root password the dolt container is created with
#   DOLT_HOST_PORT     host port for dolt (default: 3306)
#   DUMBODB_HOST_PORT  host port for dumbodb (default: 27018)
#
# DOLT_URI and DUMBODB_URI are derived from the container settings above and
# always overwritten so a stale value in the caller's shell can't cause an
# auth mismatch.
set -euo pipefail

DATA_ROOT="${DATA_ROOT:-/home/ubuntu/dumbo_vs_dolt}"
export DOLT_DATA_DIR="${DOLT_DATA_DIR:-$DATA_ROOT/dolt_data_dir}"
export DUMBODB_DATA_DIR="${DUMBODB_DATA_DIR:-$DATA_ROOT/dumbodb_data_dir}"

DOLT_ROOT_PASSWORD="${DOLT_ROOT_PASSWORD:-dolt}"
DOLT_HOST_PORT="${DOLT_HOST_PORT:-3306}"
DUMBODB_HOST_PORT="${DUMBODB_HOST_PORT:-27018}"

export DOLT_URI="root:${DOLT_ROOT_PASSWORD}@tcp(localhost:${DOLT_HOST_PORT})/"
export DUMBODB_URI="mongodb://localhost:${DUMBODB_HOST_PORT}"

mkdir -p "$DOLT_DATA_DIR" "$DUMBODB_DATA_DIR"

# wipe_data_dir empties the contents of a data directory in place, leaving the
# directory itself (and its mount/permissions) intact. The wipe runs inside a
# throwaway busybox container so it works even when the data was written by
# container-root. Bails out on suspicious paths so a misconfigured env var
# can't trigger a wide-blast delete.
wipe_data_dir() {
  local dir="$1"
  case "$dir" in
    ""|"/"|"/home"|"/root"|"/var"|"/etc"|"/usr"|"/opt"|"/tmp")
      echo "!! refusing to wipe data dir '$dir' (looks unsafe)" >&2
      exit 1
      ;;
  esac
  echo ">> wiping $dir"
  docker run --rm -v "${dir}:/wipe" busybox \
    sh -c 'find /wipe -mindepth 1 -delete'
}

# ensure_container makes sure a container with the given name is running. If
# the container is already running, leave it alone. Otherwise (stopped or
# missing) remove it, wipe its data dir, and re-create it fresh. Args after
# the data-dir path are passed verbatim to `docker run`.
ensure_container() {
  local name="$1" data_dir="$2"
  shift 2
  local state
  if state=$(docker inspect --format '{{.State.Status}}' "$name" 2>/dev/null); then
    state="${state//[[:space:]]/}"
  else
    state=missing
  fi
  if [ "$state" = "running" ]; then
    echo ">> $name already running"
    return
  fi
  if [ "$state" != "missing" ]; then
    echo ">> removing $name (was $state)"
    docker rm -f "$name" >/dev/null
  fi
  wipe_data_dir "$data_dir"
  echo ">> creating $name"
  docker run -d --name "$name" "$@" >/dev/null
}

ensure_container dolt-server "$DOLT_DATA_DIR" \
  -p "${DOLT_HOST_PORT}:3306" \
  -e "DOLT_ROOT_PASSWORD=${DOLT_ROOT_PASSWORD}" \
  -e "DOLT_ROOT_HOST=%" \
  -v "${DOLT_DATA_DIR}:/var/lib/dolt" \
  dolthub/dolt-sql-server:latest

ensure_container dumbodb "$DUMBODB_DATA_DIR" \
  -p "${DUMBODB_HOST_PORT}:27017" \
  -v "${DUMBODB_DATA_DIR}:/var/lib/dumbodb" \
  dolthub/dumbodb:latest

wait_for_tcp() {
  local label="$1" host="$2" port="$3"
  local i
  for i in $(seq 1 30); do
    if (exec 3<>"/dev/tcp/${host}/${port}") 2>/dev/null; then
      exec 3<&- 3>&-
      echo ">> ${label} listening on ${host}:${port}"
      return
    fi
    sleep 1
  done
  echo "!! ${label} did not start listening on ${host}:${port}" >&2
  exit 1
}

wait_for_tcp dolt-server 127.0.0.1 "$DOLT_HOST_PORT"
wait_for_tcp dumbodb     127.0.0.1 "$DUMBODB_HOST_PORT"

echo ">> running storage tests"
exec env GOWORK=off go test ./storage/... -v -count=1 -timeout 30m "$@"
