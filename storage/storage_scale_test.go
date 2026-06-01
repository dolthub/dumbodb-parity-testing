// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
)

// maxDumboOverDoltJSON is the per-size budget for the ratio of
// DumboDB storage to Dolt-JSON storage. Both sides store the same
// JSON document payload with the same secondary-index coverage on
// email; the ratio is dumbo-side serialisation + chunking + GC-leftover
// overhead. Today's measurements (CI numbers, with workstation
// numbers in parentheses):
//
//   10k:   2.29x (2.23x)
//   100k:  8.16x (6.49x)
//   1M:    --    (8.18x)
//   10M:   --    (--)
//
// The thresholds below carry ~15% headroom over today's worst case
// per size so a meaningful regression flags fast. As dumbo storage
// improves, tighten the matching entry. Add entries for new sizes
// added to defaultScaleSizes.
var maxDumboOverDoltJSON = map[int]float64{
	10_000:     2.6,
	100_000:    9.5,
	1_000_000:  9.5,
	10_000_000: 11.0,
}

// dumboBudgetFor returns the budget for n, defaulting to the highest
// configured entry if n is unknown (a missing entry means the test
// matrix grew without a calibration pass).
func dumboBudgetFor(n int) float64 {
	if v, ok := maxDumboOverDoltJSON[n]; ok {
		return v
	}
	var fallback float64
	for _, v := range maxDumboOverDoltJSON {
		if v > fallback {
			fallback = v
		}
	}
	return fallback
}

// scaleSizes are the document counts the parity test sweeps over by
// default -- the manual run does the whole matrix. CI overrides via
// STORAGE_PARITY_MAX_DOCS to cap the sweep within its time budget.
//
// 10M is the upper bound. Inserts at 500/batch are 20k batches and
// the GC pass on a multi-GB store takes minutes; the full sweep is
// hours of wall time on a workstation.
var defaultScaleSizes = []int{10_000, 100_000, 1_000_000, 10_000_000}

// effectiveScaleSizes filters defaultScaleSizes against the optional
// STORAGE_PARITY_MAX_DOCS env var (any size above the cap is
// dropped). An unset or malformed env var returns the full list.
func effectiveScaleSizes() []int {
	cap := os.Getenv("STORAGE_PARITY_MAX_DOCS")
	if cap == "" {
		return defaultScaleSizes
	}
	capN, err := strconv.Atoi(cap)
	if err != nil || capN <= 0 {
		return defaultScaleSizes
	}
	out := make([]int, 0, len(defaultScaleSizes))
	for _, n := range defaultScaleSizes {
		if n <= capN {
			out = append(out, n)
		}
	}
	return out
}

// TestStorageParity_Scale measures post-GC on-disk storage for the
// same straight-insert workload across three storage shapes:
//
//   - Dolt (typed columns):   the {_id, email, name, age} schema with
//                              INDEX idx_email -- baseline.
//   - DoltJSON:                same data stored as {_id, doc JSON}
//                              with a generated `email` column +
//                              INDEX idx_email. Apples-to-apples
//                              JSON storage with an equivalent
//                              secondary index.
//   - DumboDB:                 stored as BSON via the wire protocol
//                              with an `email_1` index.
//
// The primary assertion is DumboDB <= maxDumboOverDoltJSON x DoltJSON
// -- both sides store JSON, so any overhead is dumbo-side chunking /
// serialisation cost. The DumboDB-vs-Dolt-typed ratio is logged for
// context (it includes the JSON-vs-typed-columns penalty too).
//
// Workload per size: insert N canonical Doc records, commit, run GC
// via Backend.StorageBytes, walk the data dir.
//
// Not a benchmark: each size runs once and the assertion catches
// regressions / scaling-coefficient drift. The expected first-pass
// failure mode is DumboDB's BSON-with-field-names overhead vs
// DoltJSON's MySQL-JSON-binary encoding; once the real ratio is
// measured we calibrate maxDumboOverDoltJSON.
func TestStorageParity_Scale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale parity is long-running; omit -short to run")
	}
	ctx := context.Background()

	type row struct {
		n             int
		doltBytes     int64
		doltJSONBytes int64
		dumboBytes    int64
		// Apples-to-apples: both store JSON with idx_email coverage.
		dumboOverJSON     float64
		dumboOverJSONPct  float64
		// Context: dolt's JSON penalty over typed columns.
		jsonOverTyped     float64
		jsonOverTypedPct  float64
		// Context: dumbo's overhead vs the typed baseline.
		dumboOverTyped    float64
		dumboOverTypedPct float64
		withinBudget      bool
	}
	sizes := effectiveScaleSizes()
	if len(sizes) == 0 {
		t.Skipf("STORAGE_PARITY_MAX_DOCS=%q excludes every defaultScaleSizes entry", os.Getenv("STORAGE_PARITY_MAX_DOCS"))
	}
	rows := make([]row, 0, len(sizes))

	for _, n := range sizes {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			doltBytes := measureStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDoltBackend(c) }, n)
			doltJSONBytes := measureStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDoltJSONBackend(c) }, n)
			dumboBytes := measureStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDumboDBBackend(c) }, n)

			budget := dumboBudgetFor(n)
			r := row{
				n:                 n,
				doltBytes:         doltBytes,
				doltJSONBytes:     doltJSONBytes,
				dumboBytes:        dumboBytes,
				dumboOverJSON:     float64(dumboBytes) / float64(doltJSONBytes),
				jsonOverTyped:     float64(doltJSONBytes) / float64(doltBytes),
				dumboOverTyped:    float64(dumboBytes) / float64(doltBytes),
			}
			r.dumboOverJSONPct = (r.dumboOverJSON - 1.0) * 100.0
			r.jsonOverTypedPct = (r.jsonOverTyped - 1.0) * 100.0
			r.dumboOverTypedPct = (r.dumboOverTyped - 1.0) * 100.0
			r.withinBudget = r.dumboOverJSON <= budget
			rows = append(rows, r)

			t.Logf("n=%d  dolt=%s  dolt-json=%s  dumbo=%s  "+
				"dumbo/dolt-json=%.4f (%+.2f%%)  budget=%.2fx  "+
				"json/typed=%.4f (%+.2f%%)  "+
				"dumbo/typed=%.4f (%+.2f%%)",
				n,
				fmtBytes(doltBytes), fmtBytes(doltJSONBytes), fmtBytes(dumboBytes),
				r.dumboOverJSON, r.dumboOverJSONPct, budget,
				r.jsonOverTyped, r.jsonOverTypedPct,
				r.dumboOverTyped, r.dumboOverTypedPct)

			if !r.withinBudget {
				t.Errorf("DumboDB %.4fx over DoltJSON; budget for n=%d is %.2fx",
					r.dumboOverJSON, n, budget)
			}
		})
	}

	// Summary table, printed even if individual sub-tests failed so
	// the calibration story is visible in one place.
	headers := []string{
		"Docs", "DoltTyped", "DoltJSON", "DumboDB",
		"Dumbo/DoltJSON", "JSON/Typed", "Dumbo/Typed", "Within budget",
	}
	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		within := "yes"
		if !r.withinBudget {
			within = "NO"
		}
		tableRows[i] = []string{
			fmt.Sprintf("%d", r.n),
			fmtBytes(r.doltBytes),
			fmtBytes(r.doltJSONBytes),
			fmtBytes(r.dumboBytes),
			fmt.Sprintf("%.3fx (%+.2f%%)", r.dumboOverJSON, r.dumboOverJSONPct),
			fmt.Sprintf("%.3fx (%+.2f%%)", r.jsonOverTyped, r.jsonOverTypedPct),
			fmt.Sprintf("%.3fx (%+.2f%%)", r.dumboOverTyped, r.dumboOverTypedPct),
			within,
		}
	}
	printTable(t, headers, tableRows)
}

// measureStraightInsert is the common per-size routine: spin up a
// fresh backend, setup, insert n docs in batches, commit, then ask
// Backend.StorageBytes (which runs GC) for the post-GC byte count.
func measureStraightInsert(
	ctx context.Context,
	t *testing.T,
	factory func(context.Context) (Backend, error),
	n int,
) int64 {
	t.Helper()
	b, err := factory(ctx)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	defer b.Close()

	if err := b.Setup(ctx); err != nil {
		t.Fatalf("setup: %v", err)
	}
	insertDocs(ctx, t, b, n)
	if err := b.Commit(ctx, "scale-base"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	bytes, err := b.StorageBytes(ctx)
	if err != nil {
		t.Fatalf("storage bytes: %v", err)
	}
	return bytes
}

