#!/usr/bin/env bash
#
# Reproduce workspace-1bk.4.5: residual CAS double-match under fieldTouched.
#
# This is the exact configuration that fails. Two things matter:
#   * mode fieldTouched  (fieldDivergent ALLOWS convergent coalescing -> no failure)
#   * soak scale         (the race is ~1 in 12,000 matches -- measured)
# The server must run with -auto-commit (server.sh start does this).
#
# The race is genuinely rare and DumboDB's throughput here is ~825 ops/s, so
# detection is volume-limited: you need ~70,000 matches (~1.3M attempts, ~25-30
# min) to reliably see a duplicate. A shorter run may show zero and prove
# nothing -- 300k attempts / 15k matches came back clean in testing. There is no
# known accelerator (cas-delay does NOT amplify it -- it only lowers throughput).
#
# Usage:
#   ./repro-4.5.sh              full 30-minute soak (reliable)
#   ./repro-4.5.sh <duration>   custom duration, e.g. 45m (longer = more reliable)
#
# Expected result: verdict=failed, DuplicateMatches > 0, storedVersionEqualsMatched
# and oneMatchPerObservedGeneration failing. The printed collision findings give
# the concrete interleavings to debug.
set -euo pipefail
cd "$(dirname "$0")"

DURATION=${1:-30m}
./server.sh start auto-commit
./run.sh --scenario cas --mode fieldTouched --workers 32 \
         --duration "$DURATION" --name repro-4.5
