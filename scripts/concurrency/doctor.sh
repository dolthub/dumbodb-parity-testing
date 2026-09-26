#!/usr/bin/env bash
#
# Preflight for a host that will run the concurrency load jobs. Reports what is
# present/missing so a new machine can be brought up without discovering gaps
# 8 hours into a soak. Exits non-zero if anything required is missing.
#
# Usage: ./doctor.sh [--build]
#   --build  also do a full server build (slow; fetches deps) as the final check
#
# Does not source lib.sh (that hard-exits when dumbodb is absent, which is one of
# the things we are here to check); it resolves paths itself.
set -uo pipefail
cd "$(dirname "$0")"
HARNESS_DIR=$(cd ../.. && pwd)
DUMBODB_DIR=${DUMBODB_DIR:-$(cd "${HARNESS_DIR}/../dumbodb" 2>/dev/null && pwd || echo "${HARNESS_DIR}/../dumbodb")}
RUN_DIR=${RUN_DIR:-/tmp/dumbo-concurrency}

fail=0
ok()   { printf '  ok    %s\n' "$*"; }
bad()  { printf '  MISS  %s\n' "$*"; fail=1; }
warn() { printf '  warn  %s\n' "$*"; }

echo "== tools =="
command -v bash    >/dev/null && ok "bash    $(bash --version | head -1 | grep -oE '[0-9]+\.[0-9]+' | head -1)" || bad "bash"
command -v git     >/dev/null && ok "git" || bad "git"
command -v make    >/dev/null && ok "make" || bad "make (needed to build the server)"
command -v python3 >/dev/null && ok "python3 (result JSON parsing)" || bad "python3 (run.sh/suite.sh parse JSON with it)"
command -v timeout >/dev/null && ok "timeout (weekend-loop per-iteration cap)" || bad "timeout (coreutils)"
command -v column  >/dev/null && ok "column (summary table)" || warn "column absent -- summary prints unaligned, not fatal"
if command -v go >/dev/null; then
  gv=$(go env GOVERSION 2>/dev/null | sed 's/^go//')
  maj=${gv%%.*}; rest=${gv#*.}; min=${rest%%.*}
  if [ "${maj:-0}" -gt 1 ] || { [ "${maj:-0}" -eq 1 ] && [ "${min:-0}" -ge 24 ]; }; then
    ok "go $gv (>=1.24)"
  else
    bad "go $gv -- go.mod needs 1.24+"
  fi
else
  bad "go (1.24+)"
fi

echo "== repos =="
[ -d "${HARNESS_DIR}/.git" ] && ok "parity repo: $HARNESS_DIR ($(git -C "$HARNESS_DIR" rev-parse --short HEAD 2>/dev/null))" || warn "parity repo has no .git at $HARNESS_DIR"
if [ -e "${DUMBODB_DIR}/cmd/dumbodb" ]; then
  ok "dumbodb repo: $DUMBODB_DIR ($(git -C "$DUMBODB_DIR" describe --tags --always 2>/dev/null || echo '?'))"
else
  bad "dumbodb repo not at DUMBODB_DIR=$DUMBODB_DIR (no cmd/dumbodb) -- set DUMBODB_DIR or check out ../dumbodb"
fi

echo "== disk (RUN_DIR=$RUN_DIR) =="
mkdir -p "$RUN_DIR" 2>/dev/null || true
if [ -w "$RUN_DIR" ]; then
  free_kb=$(df -Pk "$RUN_DIR" 2>/dev/null | awk 'NR==2{print $4}')
  free_gb=$(( ${free_kb:-0} / 1024 / 1024 ))
  [ "$free_gb" -ge 20 ] && ok "writable, ${free_gb}G free" || warn "writable but only ${free_gb}G free -- a soak writes a lot; give it headroom"
  # rough write-throughput check; the weekend died on IOPS starvation, so surface a slow disk here.
  t0=$(date +%s%N); dd if=/dev/zero of="$RUN_DIR/.doctor-io" bs=1M count=256 conv=fdatasync 2>/dev/null; t1=$(date +%s%N); rm -f "$RUN_DIR/.doctor-io"
  ms=$(( (t1 - t0) / 1000000 )); mbps=$(( 256000 / (ms==0?1:ms) ))
  [ "$mbps" -ge 50 ] && ok "sequential write ~${mbps} MB/s" || warn "sequential write only ~${mbps} MB/s -- slow disk; the weekend run died on AWS IOPS starvation, prefer NVMe/instance-store or provisioned-IOPS for RUN_DIR"
else
  bad "RUN_DIR not writable: $RUN_DIR (set RUN_DIR to a fast, roomy path)"
fi

echo "== build =="
if [ "${1:-}" = "--build" ]; then
  ( cd "$DUMBODB_DIR" && make build >/dev/null 2>&1 ) && ok "server builds (make build)" || bad "server build failed -- run 'cd $DUMBODB_DIR && make build' to see why (deps/network/toolchain)"
  ( cd "$HARNESS_DIR" && GOWORK=off go build -o /dev/null ./cmd/concurrency >/dev/null 2>&1 ) && ok "harness builds" || bad "harness build failed -- 'cd $HARNESS_DIR && GOWORK=off go build ./cmd/concurrency'"
else
  echo "  (skipped; pass --build to compile server+harness -- slow, fetches deps)"
fi

echo
[ "$fail" -eq 0 ] && echo "PREFLIGHT OK -- host looks ready. Validate with: ./suite.sh smoke" \
                  || { echo "PREFLIGHT FAILED -- fix the MISS items above before running the load jobs."; exit 1; }
