// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package storage

import (
	"context"
	"fmt"
	"testing"
)

// TestStorageParity_CommitCadence sweeps the commit cadence parameter K
// across the partial-update workload. For each K it runs the same
// insert + mutation + GC + measure pipeline as
// TestStorageParity_PartialUpdates, but the mutation phase is split into
// K equal chunks with a commit after each chunk.
//
// Cadence matters because structural sharing -- the property that lets
// (b)'s envelope-less storage and (a)'s ancestor-patching storage both
// reuse chunks across history -- is only observable across commits.
// Without commits, every intermediate chunk produced by every mutation
// lingers in the working set and the on-disk byte count reflects
// garbage rather than steady state. The bake-off uses this test to see
// how (a) vs (b) react to commit density: more commits means more
// snapshot boundaries for chunks to be shared across.
//
// Per the design doc the sweep runs K = 1, 4, 8, 32. K=1 is the same
// shape as the existing TestStorageParity_PartialUpdates and serves as
// a sanity baseline.
func TestStorageParity_CommitCadence(t *testing.T) {
	if testing.Short() {
		t.Skip("commit-cadence sweep is long-running; omit -short to run")
	}
	ctx := context.Background()

	const n = 2000
	const padBytes = 2500 // out-of-band sized docs, matches large_out_of_band

	type row struct {
		k             int
		doltJSONBytes int64
		dumboBytes    int64
		dumboOverJSON float64
	}
	rows := make([]row, 0, 4)

	for _, k := range []int{1, 4, 8, 32} {
		k := k
		t.Run(fmt.Sprintf("K%d", k), func(t *testing.T) {
			doltJSONBytes := measureInsertUpdateCadenced(ctx, t,
				func(c context.Context) (Backend, error) { return NewDoltJSONBackend(c) },
				n, padBytes, k)
			dumboBytes := measureInsertUpdateCadenced(ctx, t,
				func(c context.Context) (Backend, error) { return NewDumboDBBackend(c) },
				n, padBytes, k)

			r := row{
				k:             k,
				doltJSONBytes: doltJSONBytes,
				dumboBytes:    dumboBytes,
				dumboOverJSON: float64(dumboBytes) / float64(doltJSONBytes),
			}
			rows = append(rows, r)
			t.Logf("K=%d n=%d pad=%d dolt-json=%s dumbo=%s dumbo/dolt-json=%.3fx",
				k, n, padBytes,
				fmtBytes(doltJSONBytes), fmtBytes(dumboBytes), r.dumboOverJSON)
		})
	}

	headers := []string{"K", "DoltJSON", "DumboDB", "Dumbo/DoltJSON"}
	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		tableRows[i] = []string{
			fmt.Sprintf("%d", r.k),
			fmtBytes(r.doltJSONBytes),
			fmtBytes(r.dumboBytes),
			fmt.Sprintf("%.3fx", r.dumboOverJSON),
		}
	}
	printTable(t, headers, tableRows)
}

// measureInsertUpdateCadenced runs the same insert+update+measure
// pipeline as measureInsertUpdate but commits k times across the
// mutation phase rather than once at the end.
func measureInsertUpdateCadenced(
	ctx context.Context,
	t *testing.T,
	factory func(context.Context) (Backend, error),
	n int,
	padBytes int,
	k int,
) int64 {
	t.Helper()
	if k < 1 {
		t.Fatalf("k must be >= 1, got %d", k)
	}
	b, err := factory(ctx)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	defer b.Close()

	if err := b.Setup(ctx); err != nil {
		t.Fatalf("setup: %v", err)
	}
	insertPaddedDocs(ctx, t, b, n, padBytes)
	if err := b.Commit(ctx, "cadence-base"); err != nil {
		t.Fatalf("commit insert phase: %v", err)
	}

	// Mutation phase split into k chunks. Each chunk touches roughly
	// n/k documents and ends with a commit. The final chunk may pick
	// up the rounding remainder so we always touch all n docs.
	chunkSize := n / k
	if chunkSize < 1 {
		chunkSize = 1
	}
	idx := 0
	for chunk := 0; chunk < k; chunk++ {
		end := idx + chunkSize
		if chunk == k-1 || end > n {
			end = n
		}
		for ; idx < end; idx++ {
			id := fmt.Sprintf("doc%07d", idx)
			newEmail := fmt.Sprintf("updated%07d@example.com", idx)
			if err := b.UpdateEmail(ctx, id, newEmail); err != nil {
				t.Fatalf("update %s: %v", id, err)
			}
		}
		if err := b.Commit(ctx, fmt.Sprintf("cadence-chunk-%d", chunk)); err != nil {
			t.Fatalf("commit chunk %d: %v", chunk, err)
		}
	}

	bytes, err := b.StorageBytes(ctx)
	if err != nil {
		t.Fatalf("storage bytes: %v", err)
	}
	return bytes
}
