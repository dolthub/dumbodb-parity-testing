#!/usr/bin/env bash
#
# Codified reproduction of the residual CAS double-accept in DumboDB.
# Hand this to whoever is investigating the server-side race.
#
# THE BUG (workspace-1bk.4.5 and its siblings)
# --------------------------------------------
# Under concurrent compare-and-swap, DumboDB occasionally lets TWO writers that
# observed the same generation both receive n:1 -- so two updates "win" one
# optimistic-lock slot and one acknowledged write is silently lost. The harness
# detects this with a zero-tolerance invariant: at most one match per observed
# generation. Each violation is reported as a collision finding naming the two
# writers (worker + sequence) that both won the same generation.
#
# It manifests across scenarios and modes, but is ONE underlying defect:
#   cas / fieldTouched         two equal increments on one generation both commit
#   divergent-cas / fieldDivergent  two distinct values on one generation both commit
#   uuid-cas / fieldDivergent  two distinct UUID tokens on one generation both accepted
#
# WHY IT NEEDS SCALE
# ------------------
# The race is rare (order 1 in a few thousand to ~12,000 matches, depending on
# scenario) and DumboDB throughput here is ~825 ops/s, so reproduction is
# volume-limited. Each config runs for --duration (default 30m) to accumulate
# enough matches. A short run may show zero and prove nothing. There is no known
# accelerator: cas-delay does not amplify it.
#
# USAGE
#   ./investigate-cas-race.sh [--duration D] [--only NAME]
#     --duration D   per-config run length (default 30m; longer = more reliable)
#     --only NAME    run a single config: cas-ft | divcas-fd | uuidcas-fd
#
# EXIT CODE (regression contract)
#   0  no double-accepts observed in any config  -> the bug appears fixed
#   1  at least one config reproduced a double-accept -> bug still present
#
# The goal for the server implementor: make this script exit 0.
set -uo pipefail
cd "$(dirname "$0")"
. ./lib.sh

DURATION=30m ONLY=""
while [ $# -gt 0 ]; do
  case "$1" in
    --duration) DURATION=$2; shift 2 ;;
    --only)     ONLY=$2; shift 2 ;;
    *) die "unknown flag '$1' (see header)" ;;
  esac
done

# name | scenario | mode | what a duplicate here means
CONFIGS=(
  "cas-ft     | cas           | fieldTouched   | convergent CAS: two equal increments on one generation both commit"
  "divcas-fd  | divergent-cas | fieldDivergent | divergent CAS: two distinct values on one generation both commit"
  "uuidcas-fd | uuid-cas      | fieldDivergent | divergent UUID: two distinct tokens on one generation both accepted"
)

log "starting DumboDB (auto-commit), revision $(git -C "$DUMBODB_DIR" describe --tags --always 2>/dev/null || echo unknown)"
./server.sh start auto-commit >/dev/null 2>&1 || { ./server.sh start auto-commit; die "server failed to start"; }

# Build the harness here, because the runs below pass SKIP_BUILD=1 to run.sh to
# avoid rebuilding the SERVER -- and run.sh reads that same variable to skip
# building the HARNESS. Without this the harness binary never exists, every run
# dies unseen, and this script reports "no double-accepts" having executed
# nothing at all.
ensure_dirs
log "building harness (GOWORK=off)"
( cd "$HARNESS_DIR" && GOWORK=off go build -o "$HARNESS_BIN" ./cmd/concurrency ) \
  || die "harness build failed"

total_dupes=0
printf '\n%-12s %-14s %-15s %-9s %-11s %s\n' NAME SCENARIO MODE MATCHES DUPLICATES RESULT

for cfg in "${CONFIGS[@]}"; do
  name=$(echo "$cfg" | awk -F'|' '{gsub(/^ +| +$/,"",$1);print $1}')
  scen=$(echo "$cfg" | awk -F'|' '{gsub(/^ +| +$/,"",$2);print $2}')
  mode=$(echo "$cfg" | awk -F'|' '{gsub(/^ +| +$/,"",$3);print $3}')
  [ -n "$ONLY" ] && [ "$ONLY" != "$name" ] && continue

  log "running $name ($scen / $mode) for $DURATION ..."
  SKIP_BUILD=1 ./run.sh --scenario "$scen" --mode "$mode" --workers 32 \
    --duration "$DURATION" --name "investigate-$name" > "${RESULTS_DIR}/investigate-${name}.out" 2>&1
  run_code=$?
  json="${RESULTS_DIR}/investigate-${name}.json"

  # A run that did not happen is not a clean run. Exit code 1 is a real harness
  # failure verdict and is expected here; anything else, or a missing report,
  # means this config produced no evidence and must not be counted as one.
  if [ "$run_code" != 0 ] && [ "$run_code" != 1 ] && [ "$run_code" != 3 ]; then
    tail -n 5 "${RESULTS_DIR}/investigate-${name}.out" >&2 || true
    die "$name did not run (exit $run_code); see ${RESULTS_DIR}/investigate-${name}.out"
  fi
  [ -f "$json" ] || die "$name produced no report at $json"

  read -r matches dupes < <(python3 -c "
import json,sys
r=json.load(open(sys.argv[1])); L=r['Ledger']
print(L.get('Matched',0), L.get('CAS',{}).get('DuplicateMatches',0))
" "$json") || die "$name wrote an unreadable report at $json"
  total_dupes=$((total_dupes + dupes))
  result=$([ "$dupes" -gt 0 ] && echo REPRODUCED || echo "clean(inconclusive)")
  printf '%-12s %-14s %-15s %-9s %-11s %s\n' "$name" "$scen" "$mode" "$matches" "$dupes" "$result"

  # Show the interleavings for debugging.
  python3 -c "
import json,sys
C=json.load(open(sys.argv[1]))['Ledger'].get('CAS',{})
for f in (C.get('Findings') or [])[:5]:
    print('    gen %s: worker %s (seq %s) and worker %s (seq %s) both won'
          % (f.get('ObservedGeneration'), f.get('FirstWorker'), f.get('FirstSequence'),
             f.get('CompetingWorker'), f.get('CompetingSequence')))
" "$json" 2>/dev/null || true
done

./server.sh stop >/dev/null 2>&1 || true

echo
if [ "$total_dupes" -gt 0 ]; then
  echo "RESULT: reproduced -- $total_dupes double-accept(s). The bug is present."
  echo "Each 'gen N: worker A and worker B both won' is a lost update to debug."
  exit 1
fi
echo "RESULT: no double-accepts observed. Either the bug is fixed, or the runs were"
echo "too short to hit it -- rerun with a longer --duration to raise confidence."
echo "Match counts above are the evidence: a config showing 0 matches did not"
echo "exercise the race and says nothing either way."
exit 0
