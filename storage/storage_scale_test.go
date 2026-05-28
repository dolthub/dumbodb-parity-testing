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

// maxDumboOverhead is the maximum allowed ratio of DumboDB storage to
// Dolt storage for the same logical workload. JSON documents include
// field names per row where Dolt's row format does not, so a small
// overhead is expected. The 5% start is a stake-in-the-ground that
// almost certainly fails on the first run; tighten or loosen after
// the first measurement pass.
const maxDumboOverhead = 1.05

// scaleSizes are the document counts the parity test sweeps over.
// 10M is the upper bound -- inserts at 500/batch are 20k batches,
// minutes of wall time. Use -run to target a single size if iterating
// quickly.
var scaleSizes = []int{10_000, 100_000, 1_000_000, 10_000_000}

// TestStorageParity_Scale measures post-GC on-disk storage for the
// same straight-insert workload across both backends and asserts
// DumboDB stays within maxDumboOverhead of Dolt.
//
// Workload per size: insert N canonical Doc records (the same
// {_id, email, name, age} shape used by the merge tests), commit,
// run GC via Backend.StorageBytes, walk the data dir.
//
// Not a benchmark: each size runs once and the assertion catches
// regressions / scaling-coefficient drift. The expected first-pass
// failure mode is DumboDB's BSON-with-field-names overhead; once the
// real ratio is measured we calibrate maxDumboOverhead.
func TestStorageParity_Scale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale parity is long-running; omit -short to run")
	}
	ctx := context.Background()

	type row struct {
		n            int
		doltBytes    int64
		dumboBytes   int64
		ratio        float64
		overheadPct  float64
		withinBudget bool
	}
	rows := make([]row, 0, len(scaleSizes))

	// Resolve once so the ordering Dolt-then-DumboDB is fixed even
	// if backendFactories order changes; we want the same backend on
	// each side of every ratio in the summary table.
	var doltFactory, dumboFactory func(context.Context) (Backend, error)
	for _, bf := range backendFactories {
		switch bf.name {
		case "Dolt":
			doltFactory = bf.new
		case "DumboDB":
			dumboFactory = bf.new
		}
	}
	if doltFactory == nil || dumboFactory == nil {
		t.Fatalf("backendFactories must include Dolt and DumboDB; got %+v", backendFactories)
	}

	for _, n := range scaleSizes {
		n := n
		t.Run(fmt.Sprintf("n=%d", n), func(t *testing.T) {
			doltBytes := measureStraightInsert(ctx, t, doltFactory, n)
			dumboBytes := measureStraightInsert(ctx, t, dumboFactory, n)
			ratio := float64(dumboBytes) / float64(doltBytes)
			overheadPct := (ratio - 1.0) * 100.0

			r := row{
				n:            n,
				doltBytes:    doltBytes,
				dumboBytes:   dumboBytes,
				ratio:        ratio,
				overheadPct:  overheadPct,
				withinBudget: ratio <= maxDumboOverhead,
			}
			rows = append(rows, r)

			t.Logf("n=%d dolt=%s dumbo=%s ratio=%.4f overhead=%.2f%%",
				n, fmtBytes(doltBytes), fmtBytes(dumboBytes), ratio, overheadPct)

			if !r.withinBudget {
				t.Errorf("DumboDB storage %.2f%% over Dolt; budget is %.2f%%",
					overheadPct, (maxDumboOverhead-1.0)*100.0)
			}
		})
	}

	// Summary table, printed even if individual sub-tests failed so
	// the calibration story is visible in one place.
	headers := []string{"Docs", "Dolt", "DumboDB", "Ratio", "Overhead", "Within budget"}
	tableRows := make([][]string, len(rows))
	for i, r := range rows {
		within := "yes"
		if !r.withinBudget {
			within = "NO"
		}
		tableRows[i] = []string{
			fmt.Sprintf("%d", r.n),
			fmtBytes(r.doltBytes),
			fmtBytes(r.dumboBytes),
			fmt.Sprintf("%.4f", r.ratio),
			fmt.Sprintf("%+.2f%%", r.overheadPct),
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

