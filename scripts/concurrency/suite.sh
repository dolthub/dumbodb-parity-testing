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
#     group          concurrent (auto-commit server) or matrix (bare server)
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

case "$PROFILE" in
  smoke) SCALE=( --operations "$SMOKE_OPS" ) ;;
  soak)  SCALE=( --duration "$SOAK_DURATION" ) ;;
  *) die "unknown profile '$PROFILE' (use smoke or soak)" ;;
esac

# Declarative matrix. Expected values reflect DumboDB merge-mode-cas behavior;
# confirm on first green run and adjust as the server changes.
# Race-sensitive CAS cases (see investigate-cas-race.sh) share one open defect,
# workspace-1bk.4.5: a residual double-accept on one observed generation. They
# are flaky at smoke scale (0 or 1 duplicate) so they cannot gate a per-commit
# run -- marked skip in smoke, xfail in soak where they reliably fail. cas-ft is
# the exception: convergent CAS is ~6x rarer, so it passes reliably at smoke
# scale and only fails in a soak.
CASES=(
  # group      | name         | scenario       | mode           | payload | smoke | soak
  "concurrent  | cas-ft       | cas            | fieldTouched   | 0       | pass  | xfail"  # 4.5 convergent (soak-only)
  "concurrent  | cas-fd       | cas            | fieldDivergent | 0       | pass  | pass"   # convergent coalescing allowed
  "concurrent  | uuidcas-ft   | uuid-cas       | fieldTouched   | 0       | skip  | xfail"  # 4.5 divergent (flaky at smoke)
  "concurrent  | uuidcas-fd   | uuid-cas       | fieldDivergent | 0       | skip  | xfail"  # 4.5 divergent (flaky at smoke)
  "concurrent  | divcas-fd    | divergent-cas  | fieldDivergent | 0       | skip  | xfail"  # 4.5 divergent (flaky at smoke)
  "concurrent  | disjoint-fd  | disjoint-set   | fieldDivergent | 0       | pass  | pass"
  "concurrent  | blindinc-ft  | blind-inc      | fieldTouched   | 0       | pass  | pass"
  "concurrent  | identical-fd | identical-set  | fieldDivergent | 0       | pass  | pass"
  "concurrent  | sameset-fd   | same-set       | fieldDivergent | 0       | pass  | pass"
)
# Deterministic merge matrix: one case per row (payload 0 = inline).
MATRIX_ROWS=(one-sided disjoint-fields same-field-same-value same-value-plus-disjoint
             same-field-different-values modify-delete add-add-identical add-add-different both-delete)
for row in "${MATRIX_ROWS[@]}"; do
  CASES+=("matrix | matrix-$row | field-divergent-matrix-$row | fieldDivergent | 0 | pass | pass")
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
  if [ "$want_server" != "$current_server" ]; then
    log "=== switching server to $want_server for $group cases ==="
    SKIP_BUILD=${SUITE_SERVER_BUILT:-0} ./server.sh start "$want_server" >/dev/null 2>&1 \
      || { ./server.sh start "$want_server"; die "server failed to start ($want_server)"; }
    SUITE_SERVER_BUILT=1   # build once; reuse binary for later group switches
    current_server="$want_server"
  fi

  # Matrix cases are a single deterministic merge; concurrent cases use profile scale.
  if [ "$group" = matrix ]; then scale=( --operations 1 ); else scale=( "${SCALE[@]}" ); fi

  log "[$total/${#CASES[@]}] $name ($scenario / $mode, expect $expect)"
  SKIP_BUILD=1 ./run.sh --scenario "$scenario" --mode "$mode" --workers "$WORKERS" \
    "${scale[@]}" --payload "$payload" --name "$name" >/dev/null 2>&1
  json="${RESULTS_DIR}/${name}.json"
  verdict=$(verdict_of "$json"); dups=$(duplicates_of "$json")

  # Classify against expectation.
  if [ "$expect" = pass ]; then
    if [ "$verdict" = conclusivePass ]; then result=PASS; else result=FAIL; fails=$((fails + 1)); fi
  else # xfail
    if [ "$verdict" = conclusivePass ]; then result=XPASS; xpasses=$((xpasses + 1)); else result=xfail; fi
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
