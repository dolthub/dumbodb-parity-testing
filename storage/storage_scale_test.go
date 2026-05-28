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
	"testing"
)

// maxDumboOverDoltJSON is the maximum allowed ratio of DumboDB
// storage to Dolt-JSON storage for the same logical workload. This
// is the apples-to-apples comparison: both sides store the same JSON
// document payload with the same secondary-index coverage on email.
// Any overhead here is dumbo-side serialisation / chunking cost, not
// the JSON-vs-typed-columns story. 5% is the stake-in-the-ground
// budget; tighten or loosen after the first measurement pass.
const maxDumboOverDoltJSON = 1.05

// scaleSizes are the document counts the parity test sweeps over.
// 10M is the upper bound -- inserts at 500/batch are 20k batches,
// minutes of wall time. Use -run to target a single size if iterating
// quickly.
var scaleSizes = []int{10_000, 100_000, 1_000_000, 10_000_000}

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
	rows := make([]row, 0, len(scaleSizes))

	for _, n := range scaleSizes {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			doltBytes := measureStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDoltBackend(c) }, n)
			doltJSONBytes := measureStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDoltJSONBackend(c) }, n)
			dumboBytes := measureStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDumboDBBackend(c) }, n)

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
			r.withinBudget = r.dumboOverJSON <= maxDumboOverDoltJSON
			rows = append(rows, r)

			t.Logf("n=%d  dolt=%s  dolt-json=%s  dumbo=%s  "+
				"dumbo/dolt-json=%.4f (%+.2f%%)  "+
				"json/typed=%.4f (%+.2f%%)  "+
				"dumbo/typed=%.4f (%+.2f%%)",
				n,
				fmtBytes(doltBytes), fmtBytes(doltJSONBytes), fmtBytes(dumboBytes),
				r.dumboOverJSON, r.dumboOverJSONPct,
				r.jsonOverTyped, r.jsonOverTypedPct,
				r.dumboOverTyped, r.dumboOverTypedPct)

			if !r.withinBudget {
				t.Errorf("DumboDB %.2f%% over DoltJSON; budget is %.2f%%",
					r.dumboOverJSONPct, (maxDumboOverDoltJSON-1.0)*100.0)
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

