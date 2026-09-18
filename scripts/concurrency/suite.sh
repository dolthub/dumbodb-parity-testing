#!/usr/bin/env bash
#
# Run the concurrency test matrix against DumboDB and report an aggregate
# pass/fail. This is the CI entry point.
#
# Usage:
#   ./suite.sh [profile]
#     smoke   (default) bounded, fast -- the per-commit gate
#     soak    long-running -- for nightly / release characterization
#
# Exit code: 0 if every case matched its expected result, non-zero otherwise.
# A case marked xfail (a known-open bug) is allowed to fail; if it unexpectedly
# passes, that is reported (the bug may be fixed) but does not fail the build
# unless STRICT_XPASS=1.
#
# The matrix is declared in CASES below -- add a line to add coverage. Fields:
#   group | name | scenario | mode | payload | expect-smoke | expect-soak
#     group          concurrent (auto-commit), concurrent-reap (auto-commit
#                    with accelerated session reap in soak), or matrix (bare)
#     expect-smoke   expected result in the smoke profile
#     expect-soak    expected result in the soak profile
#       pass  = must reach conclusivePass
#       xfail = known-open bug; expected NOT to pass (tracked, not gating)
#
# Expectations are per-profile because some bugs are scale-dependent. Example:
# the CAS double-match (workspace-1bk.4.5) is ~1 in 12,000 matches, so it PASSES
# at smoke scale (too few matches to hit it) and only reliably fails in a soak.
# That is why smoke cannot gate it -- a soak job is required to catch it.
#
# Scale comes from the profile, not the case: smoke uses SMOKE_OPS operations,
# soak uses SOAK_DURATION. Matrix cases are deterministic (one merge) regardless.
#
# Env: SMOKE_OPS (default 50000), SOAK_DURATION (default 30m), WORKERS (32),
#      STRICT_XPASS=1 to fail the build when an xfail case passes.
set -uo pipefail
cd "$(dirname "$0")"
. ./lib.sh

PROFILE=${1:-smoke}
WORKERS=${WORKERS:-32}
SMOKE_OPS=${SMOKE_OPS:-50000}
SOAK_DURATION=${SOAK_DURATION:-30m}
REAP_SESSION_TIMEOUT=3s
REAP_SESSION_SWEEP_PERIOD=1s

case "$PROFILE" in
  smoke) SCALE=( --operations "$SMOKE_OPS" ) ;;
  soak)  SCALE=( --duration "$SOAK_DURATION" ) ;;
  *) die "unknown profile '$PROFILE' (use smoke or soak)" ;;
esac

# Declarative matrix. Expected values reflect DumboDB merge-mode-cas behavior;
# confirm on first green run and adjust as the server changes.
# The CAS-family cases (see investigate-cas-race.sh) guard against
# workspace-1bk.4.5, the residual double-accept fixed at dumbodb fcc433c. They
# expect pass at both scales now; if 4.5 regresses, the soak profile reliably
# reproduces it and these fail. Run soak against fcc433c or later.
CASES=(
  # group      | name         | scenario       | mode           | payload | smoke | soak
  "concurrent  | cas-ft       | cas            | fieldTouched   | 0       | pass  | pass"   # 4.5 regression guard (fixed fcc433c)
  "concurrent  | cas-fd       | cas            | fieldDivergent | 0       | pass  | pass"   # convergent coalescing allowed
  "concurrent  | uuidcas-ft   | uuid-cas       | fieldTouched   | 0       | pass  | pass"   # 4.5 regression guard
  "concurrent  | uuidcas-fd   | uuid-cas       | fieldDivergent | 0       | pass  | pass"   # 4.5 regression guard
  "concurrent  | divcas-fd    | divergent-cas  | fieldDivergent | 0       | pass  | pass"   # 4.5 regression guard
  "concurrent  | disjoint-fd  | disjoint-set   | fieldDivergent | 0       | pass  | pass"
  "concurrent  | blindinc-ft  | blind-inc      | fieldTouched   | 0       | pass  | pass"
  "concurrent  | identical-fd | identical-set  | fieldDivergent | 0       | pass  | pass"
  "concurrent  | sameset-fd   | same-set       | fieldDivergent | 0       | pass  | pass"

  # documentTouched ordinary writes. CAS soak is xfail until 1bk.9.8.14 is fixed.
  "concurrent-reap | cas-dt       | cas            | documentTouched | 0   | pass  | xfail"
  "concurrent-reap | uuidcas-dt   | uuid-cas       | documentTouched | 0   | pass  | pass"
  "concurrent-reap | divcas-dt    | divergent-cas  | documentTouched | 0   | pass  | pass"
  "concurrent-reap | blindinc-dt  | blind-inc      | documentTouched | 0   | pass  | pass"
  "concurrent-reap | disjoint-dt  | disjoint-set   | documentTouched | 0   | pass  | pass"
  "concurrent-reap | identical-dt | identical-set  | documentTouched | 0   | pass  | pass"
  "concurrent-reap | sameset-dt   | same-set       | documentTouched | 0   | pass  | pass"

  # documentDivergent ordinary writes and full-document discriminators.
  # Numeric CAS and convergent full-document writes are xfail until the
  # dataset-head ancestry race in 1bk.9.8.8.1 is fixed.
  "concurrent  | cas-dd       | cas                       | documentDivergent | 0 | xfail | xfail"
  "concurrent  | uuidcas-dd   | uuid-cas                  | documentDivergent | 0 | pass  | pass"
  "concurrent  | divcas-dd    | divergent-cas             | documentDivergent | 0 | pass  | pass"
  "concurrent  | blindinc-dd  | blind-inc                 | documentDivergent | 0 | pass  | pass"
  "concurrent  | disjoint-dd  | disjoint-set              | documentDivergent | 0 | pass  | pass"
  "concurrent  | identical-dd | identical-set             | documentDivergent | 0 | pass  | pass"
  "concurrent  | sameset-dd   | same-set                  | documentDivergent | 0 | pass  | pass"
  "concurrent  | wholeconv-dd | whole-document-convergent | documentDivergent | 8192 | xfail | xfail"
  "concurrent  | wholediv-dd  | whole-document-divergent  | documentDivergent | 8192 | pass  | pass"
)
# Deterministic merge matrices run on the bare branch-merge path. Document
# modes cover both inline and out-of-band storage because canonical whole-
# document equality is storage-sensitive.
MATRIX_ROWS=(one-sided disjoint-fields same-field-same-value same-value-plus-disjoint
             same-field-different-values modify-delete add-add-identical add-add-different both-delete)
for row in "${MATRIX_ROWS[@]}"; do
  CASES+=("matrix | matrix-fd-$row | field-divergent-matrix-$row | fieldDivergent | 0 | pass | pass")
  CASES+=("matrix | matrix-dt-inline-$row | document-touched-matrix-$row | documentTouched | 0 | pass | pass")
  CASES+=("matrix | matrix-dt-oob-$row | document-touched-matrix-$row | documentTouched | 8192 | pass | pass")
  CASES+=("matrix | matrix-dd-inline-$row | document-divergent-matrix-$row | documentDivergent | 0 | pass | pass")
  CASES+=("matrix | matrix-dd-oob-$row | document-divergent-matrix-$row | documentDivergent | 8192 | pass | pass")
done

ensure_dirs
log "building harness (GOWORK=off)"
( cd "$HARNESS_DIR" && GOWORK=off go build -o "$HARNESS_BIN" ./cmd/concurrency ) || die "harness build failed"

SUMMARY_TSV="${RESULTS_DIR}/suite-${PROFILE}.tsv"
: > "$SUMMARY_TSV"
printf 'name\tscenario\tmode\texpect\tverdict\tresult\tduplicates\n' >> "$SUMMARY_TSV"

fails=0 xpasses=0 total=0
current_server=""

field() { echo "$1" | awk -F'|' -v n="$2" '{gsub(/^ +| +$/,"",$n); print $n}'; }

verdict_of() {  # extract Verdict from a result JSON
  python3 -c "import json,sys; print(json.load(open(sys.argv[1])).get('Verdict','?'))" "$1" 2>/dev/null || echo "?"
}
duplicates_of() {
  python3 -c "import json,sys; print(json.load(open(sys.argv[1])).get('Ledger',{}).get('CAS',{}).get('DuplicateMatches',0))" "$1" 2>/dev/null || echo "?"
}

for spec in "${CASES[@]}"; do
  group=$(field "$spec" 1); name=$(field "$spec" 2); scenario=$(field "$spec" 3)
  mode=$(field "$spec" 4); payload=$(field "$spec" 5)
  expect_smoke=$(field "$spec" 6); expect_soak=$(field "$spec" 7)
  expect=$([ "$PROFILE" = soak ] && echo "$expect_soak" || echo "$expect_smoke")
  total=$((total + 1))

  # skip: not gated in this profile (e.g. a flaky-until-fixed race case).
  if [ "$expect" = skip ]; then
    log "[$total/${#CASES[@]}] $name -- SKIP (not gated in $PROFILE)"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$scenario" "$mode" skip - SKIP - >> "$SUMMARY_TSV"
    continue
  fi

  # Start the right server for this group (only when it changes).
  want_server=$([ "$group" = matrix ] && echo bare || echo auto-commit)
  session_timeout=""
  session_sweep_period=""
  if [ "$PROFILE" = soak ] && [ "$group" = concurrent-reap ]; then
    session_timeout=$REAP_SESSION_TIMEOUT
    session_sweep_period=$REAP_SESSION_SWEEP_PERIOD
  fi
  server_key="${want_server}:${session_timeout:-default}:${session_sweep_period:-default}"
  if [ "$server_key" != "$current_server" ]; then
    log "=== switching server to $server_key for $group cases ==="
    SESSION_TIMEOUT=$session_timeout SESSION_SWEEP_PERIOD=$session_sweep_period \
      SKIP_BUILD=${SUITE_SERVER_BUILT:-0} ./server.sh start "$want_server" >/dev/null 2>&1 \
      || { SESSION_TIMEOUT=$session_timeout SESSION_SWEEP_PERIOD=$session_sweep_period \
           ./server.sh start "$want_server"; die "server failed to start ($server_key)"; }
    SUITE_SERVER_BUILT=1   # build once; reuse binary for later group switches
    current_server=$server_key
  fi

  # Matrix cases are a single deterministic merge; concurrent cases use profile scale.
  if [ "$group" = matrix ]; then scale=( --operations 1 ); else scale=( "${SCALE[@]}" ); fi

  log "[$total/${#CASES[@]}] $name ($scenario / $mode, expect $expect)"
  SKIP_BUILD=1 ./run.sh --scenario "$scenario" --mode "$mode" --workers "$WORKERS" \
    "${scale[@]}" --payload "$payload" --name "$name" > "${RESULTS_DIR}/${name}.out" 2>&1
  run_code=$?
  json="${RESULTS_DIR}/${name}.json"

  # A run that did not produce a report produced no evidence. It must never be
  # classified -- least of all as an "expected" xfail -- or a crash reads as the
  # known bug. Harness exit codes 0/1/3 are real verdicts; anything else, or a
  # missing report, is an ERROR that fails the suite.
  if { [ "$run_code" != 0 ] && [ "$run_code" != 1 ] && [ "$run_code" != 3 ]; } || [ ! -f "$json" ]; then
    result=ERROR; fails=$((fails + 1))
    tail -n 5 "${RESULTS_DIR}/${name}.out" >&2 || true
    verdict="no-report"; dups=-
  else
    verdict=$(verdict_of "$json"); dups=$(duplicates_of "$json")
    if [ "$expect" = pass ]; then
      if [ "$verdict" = conclusivePass ]; then result=PASS; else result=FAIL; fails=$((fails + 1)); fi
    else # xfail: only a genuine failing verdict counts as the known bug
      case "$verdict" in
        conclusivePass) result=XPASS; xpasses=$((xpasses + 1)) ;;
        failed)         result=xfail ;;
        *)              result=ERROR; fails=$((fails + 1)) ;;  # inconclusive/unknown is not evidence
      esac
    fi
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$name" "$scenario" "$mode" "$expect" "$verdict" "$result" "$dups" >> "$SUMMARY_TSV"
done

./server.sh stop >/dev/null 2>&1 || true

echo
echo "==================== suite: $PROFILE ===================="
column -t -s $'\t' "$SUMMARY_TSV"
echo "========================================================"
echo "cases=$total  unexpected-failures=$fails  unexpected-passes(xpass)=$xpasses"
echo "results: $SUMMARY_TSV  (per-case JSON in $RESULTS_DIR)"

rc=0
[ "$fails" -gt 0 ] && rc=1
[ "${STRICT_XPASS:-0}" = 1 ] && [ "$xpasses" -gt 0 ] && rc=1
[ "$xpasses" -gt 0 ] && echo "note: $xpasses xfail case(s) now PASS -- a known-open bug may be fixed; review and update expect."
exit $rc
