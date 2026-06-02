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

// TestStorageParity_DocSize sweeps the storage-parity workload across
// several document sizes at a fixed N. The base canonical Doc is only
// ~80 bytes; the SHA-512[:20] primary key dumbo uses is a fixed 20
// bytes per row, which is a meaningful fraction of small documents
// but a much smaller share of larger ones. This test isolates how
// much the per-row fixed overhead (key + tuple framing + per-doc
// chunk-store metadata) actually skews the parity ratio.
//
// Tracked under workspace-bow.
func TestStorageParity_DocSize(t *testing.T) {
	if testing.Short() {
		t.Skip("doc-size parity is moderate-running; omit -short to run")
	}
	ctx := context.Background()

	const n = 10000
	padSizes := []int{0, 300, 1000}

	// DoltTyped is omitted here because its `name VARCHAR(255)` schema
	// can't hold the padded Name field. The comparison that matters
	// for this investigation is DumboDB vs DoltJSON anyway, since
	// both store the document as JSON.
	type row struct {
		padBytes       int
		approxDocBytes int
		doltJSONBytes  int64
		dumboBytes     int64
		dumboOverJSON  float64
	}

	rows := make([]row, 0, len(padSizes))
	for _, pad := range padSizes {
		pad := pad
		t.Run(fmt.Sprintf("pad=%d", pad), func(t *testing.T) {
			doltJSONBytes := measurePaddedStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDoltJSONBackend(c) }, n, pad)
			dumboBytes := measurePaddedStraightInsert(ctx, t,
				func(c context.Context) (Backend, error) { return NewDumboDBBackend(c) }, n, pad)

			approx := approxDocJSONBytes(pad)
			r := row{
				padBytes:       pad,
				approxDocBytes: approx,
				doltJSONBytes:  doltJSONBytes,
				dumboBytes:     dumboBytes,
				dumboOverJSON:  float64(dumboBytes) / float64(doltJSONBytes),
			}
			rows = append(rows, r)
			t.Logf("pad=%d approxDocBytes=%d dolt-json=%s dumbo=%s dumbo/dolt-json=%.3fx",
				pad, approx,
				fmtBytes(doltJSONBytes), fmtBytes(dumboBytes),
				r.dumboOverJSON)
		})
	}

	headers := []string{
		"Pad bytes", "Approx doc bytes",
		"DoltJSON", "DumboDB",
		"Dumbo/DoltJSON",
	}
	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{
			fmt.Sprintf("%d", r.padBytes),
			fmt.Sprintf("%d", r.approxDocBytes),
			fmtBytes(r.doltJSONBytes),
			fmtBytes(r.dumboBytes),
			fmt.Sprintf("%.3fx", r.dumboOverJSON),
		}
	}
	printTable(t, headers, tableRows)
}

// measurePaddedStraightInsert is measureStraightInsert with a knob
// for inflating the Name field so the canonical Doc reaches a target
// total size.
func measurePaddedStraightInsert(
	ctx context.Context,
	t *testing.T,
	factory func(context.Context) (Backend, error),
	n int,
	padBytes int,
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
	insertPaddedDocs(ctx, t, b, n, padBytes)
	if err := b.Commit(ctx, "docsize-base"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	bytes, err := b.StorageBytes(ctx)
	if err != nil {
		t.Fatalf("storage bytes: %v", err)
	}
	return bytes
}

// insertPaddedDocs inserts n documents whose Name field is padded to
// add padBytes of pseudorandom (compression-resistant) characters.
// A repeating padding character would snappy-compress to nearly zero
// and hide the true effect of larger documents on storage.
func insertPaddedDocs(ctx context.Context, t testing.TB, b Backend, n int, padBytes int) {
	t.Helper()
	const batchSize = 500
	buf := make([]Doc, 0, batchSize)
	for i := 0; i < n; i++ {
		buf = append(buf, Doc{
			ID:    fmt.Sprintf("doc%07d", i),
			Email: fmt.Sprintf("user%07d@example.com", i),
			Name:  fmt.Sprintf("User %d %s", i, pseudoRandomPad(i, padBytes)),
			Age:   20 + i%50,
		})
		if len(buf) == batchSize || i == n-1 {
			if err := b.InsertBatch(ctx, buf); err != nil {
				t.Fatalf("insert batch at %d: %v", i, err)
			}
			buf = buf[:0]
		}
	}
}

// pseudoRandomPad returns a deterministic, snappy-resistant string of
// n printable ASCII characters seeded by i. Used so adjacent documents
// don't share long compressible substrings.
func pseudoRandomPad(i, n int) string {
	if n <= 0 {
		return ""
	}
	b := make([]byte, n)
	seed := uint64(i)*0x9E3779B97F4A7C15 + 0xBF58476D1CE4E5B9
	for j := 0; j < n; j++ {
		seed = seed*0x100000001B3 + 0xCBF29CE484222325
		b[j] = byte(33 + (seed%94)) // printable ASCII excluding space/quote noise
	}
	return string(b)
}

// approxDocJSONBytes returns the approximate JSON byte size of a
// padded canonical Doc. Used only for the test's logging table.
func approxDocJSONBytes(padBytes int) int {
	// Baseline (unpadded canonical Doc as JSON):
	//   {"_id":"docNNNNNNN","email":"userNNNNNNN@example.com","name":"User N","age":NN}
	// ~85 bytes for the smallest, ~95 for User indices that need more digits.
	const baseline = 90
	return baseline + padBytes
}
