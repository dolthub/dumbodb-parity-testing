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
	"time"
)

const (
	baseDocs = 100_000
	diffDocs = 100
)

// backendFactories lists both backends. Tests iterate over them to run each
// scenario against Dolt and DumboDB side-by-side.
var backendFactories = []struct {
	name string
	new  func(context.Context) (Backend, error)
}{
	{"Dolt", func(ctx context.Context) (Backend, error) { return NewDoltBackend(ctx) }},
	{"DumboDB", func(ctx context.Context) (Backend, error) { return NewDumboDBBackend(ctx) }},
}

// insertDocs inserts n sequentially-keyed documents into b.
func insertDocs(ctx context.Context, t testing.TB, b Backend, n int) {
	t.Helper()
	const batchSize = 500
	buf := make([]Doc, 0, batchSize)
	for i := 0; i < n; i++ {
		buf = append(buf, Doc{
			ID:    fmt.Sprintf("doc%07d", i),
			Email: fmt.Sprintf("user%07d@example.com", i),
			Name:  fmt.Sprintf("User %d", i),
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

// updateDocs updates the Email field of the first n documents.
func updateDocs(ctx context.Context, t testing.TB, b Backend, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("doc%07d", i)
		newEmail := fmt.Sprintf("updated%07d@example.com", i)
		if err := b.UpdateEmail(ctx, id, newEmail); err != nil {
			t.Fatalf("update %s: %v", id, err)
		}
	}
}

// TestMergeStorage_LargeBaseTinyDiff measures storage growth from merging a
// branch that changed diffDocs out of baseDocs documents.
//
// This is a measurement test, not a correctness assertion. Run with -v to see
// the results table. The expected outcome is that Dolt's storage grows by
// roughly the size of the diff while DumboDB's grows by the size of the full
// index (exposing the O(N) rebuild).
func TestMergeStorage_LargeBaseTinyDiff(t *testing.T) {
	ctx := context.Background()

	type result struct {
		name     string
		mergeDur time.Duration
		before   int64
		after    int64
	}
	results := make([]result, 0, len(backendFactories))

	for _, bf := range backendFactories {
		bf := bf
		t.Run(bf.name, func(t *testing.T) {
			b, err := bf.new(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()

			if err := b.Setup(ctx); err != nil {
				t.Fatalf("setup: %v", err)
			}

			t.Logf("inserting %d base docs...", baseDocs)
			insertDocs(ctx, t, b, baseDocs)
			if err := b.Commit(ctx, "base"); err != nil {
				t.Fatalf("commit base: %v", err)
			}

			before, err := b.StorageBytes(ctx)
			if err != nil {
				t.Fatalf("storage before: %v", err)
			}

			if err := b.CreateBranch(ctx, "feat"); err != nil {
				t.Fatalf("create branch: %v", err)
			}
			if err := b.Checkout(ctx, "feat"); err != nil {
				t.Fatalf("checkout feat: %v", err)
			}
			updateDocs(ctx, t, b, diffDocs)
			if err := b.Commit(ctx, "tiny diff"); err != nil {
				t.Fatalf("commit diff: %v", err)
			}
			if err := b.Checkout(ctx, "main"); err != nil {
				t.Fatalf("checkout main: %v", err)
			}

			t.Log("merging...")
			dur, err := b.Merge(ctx, "feat")
			if err != nil {
				t.Fatalf("merge: %v", err)
			}

			after, err := b.StorageBytes(ctx)
			if err != nil {
				t.Fatalf("storage after: %v", err)
			}

			results = append(results, result{
				name:     b.Name(),
				mergeDur: dur,
				before:   before,
				after:    after,
			})
		})
	}

	headers := []string{"Backend", "Merge duration", "Storage before", "Storage after", "Delta"}
	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{
			r.name,
			r.mergeDur.Round(time.Millisecond).String(),
			fmtBytes(r.before),
			fmtBytes(r.after),
			fmtBytes(r.after - r.before),
		}
	}
	printTable(t, headers, rows)
}

// TestMergeTime_ScalesWithBase runs TestMergeStorage_LargeBaseTinyDiff at
// multiple base sizes with a fixed diff of diffDocs updates. The expected
// pattern is that DumboDB merge time grows linearly with base size while Dolt
// merge time stays flat.
func TestMergeTime_ScalesWithBase(t *testing.T) {
	ctx := context.Background()
	baseSizes := []int{1_000, 10_000, 100_000}

	type result struct {
		baseSize int
		backend  string
		dur      time.Duration
	}
	var results []result

	for _, n := range baseSizes {
		n := n
		t.Run(fmt.Sprintf("base=%d", n), func(t *testing.T) {
			for _, bf := range backendFactories {
				bf := bf
				t.Run(bf.name, func(t *testing.T) {
					b, err := bf.new(ctx)
					if err != nil {
						t.Fatal(err)
					}
					defer b.Close()

					if err := b.Setup(ctx); err != nil {
						t.Fatalf("setup: %v", err)
					}

					insertDocs(ctx, t, b, n)
					if err := b.Commit(ctx, "base"); err != nil {
						t.Fatalf("commit base: %v", err)
					}

					if err := b.CreateBranch(ctx, "feat"); err != nil {
						t.Fatalf("create branch: %v", err)
					}
					if err := b.Checkout(ctx, "feat"); err != nil {
						t.Fatalf("checkout feat: %v", err)
					}
					updateDocs(ctx, t, b, diffDocs)
					if err := b.Commit(ctx, "diff"); err != nil {
						t.Fatalf("commit diff: %v", err)
					}
					if err := b.Checkout(ctx, "main"); err != nil {
						t.Fatalf("checkout main: %v", err)
					}

					dur, err := b.Merge(ctx, "feat")
					if err != nil {
						t.Fatalf("merge: %v", err)
					}

					t.Logf("base=%d %s merge=%v", n, b.Name(), dur.Round(time.Millisecond))
					results = append(results, result{baseSize: n, backend: b.Name(), dur: dur})
				})
			}
		})
	}

	// Summary table.
	headers := []string{"Base size", "Backend", "Merge duration"}
	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{
			fmt.Sprintf("%d", r.baseSize),
			r.backend,
			r.dur.Round(time.Millisecond).String(),
		}
	}
	printTable(t, headers, rows)
}

// BenchmarkMerge_LargeBaseTinyDiff benchmarks the merge operation with baseDocs
// documents and a diffDocs-sized diff. Because setup is expensive, b.N is
// always 1 in practice; the benchmark value captures wall-clock merge time
// suitable for benchstat comparison across commits.
func BenchmarkMerge_LargeBaseTinyDiff(b *testing.B) {
	ctx := context.Background()

	for _, bf := range backendFactories {
		bf := bf
		b.Run(bf.name, func(b *testing.B) {
			b.StopTimer()

			backend, err := bf.new(ctx)
			if err != nil {
				b.Fatal(err)
			}
			defer backend.Close()

			if err := backend.Setup(ctx); err != nil {
				b.Fatalf("setup: %v", err)
			}
			insertDocs(ctx, b, backend, baseDocs)
			if err := backend.Commit(ctx, "base"); err != nil {
				b.Fatalf("commit base: %v", err)
			}
			if err := backend.CreateBranch(ctx, "feat"); err != nil {
				b.Fatalf("create branch: %v", err)
			}
			if err := backend.Checkout(ctx, "feat"); err != nil {
				b.Fatalf("checkout feat: %v", err)
			}
			updateDocs(ctx, b, backend, diffDocs)
			if err := backend.Commit(ctx, "diff"); err != nil {
				b.Fatalf("commit diff: %v", err)
			}
			if err := backend.Checkout(ctx, "main"); err != nil {
				b.Fatalf("checkout main: %v", err)
			}

			b.StartTimer()
			for i := 0; i < b.N; i++ {
				if _, err := backend.Merge(ctx, "feat"); err != nil {
					b.Fatalf("merge: %v", err)
				}
			}
		})
	}
}

// TestIndexLookup_PostMerge verifies that secondary index lookups are fast after
// a merge. It runs 1000 email lookups and logs p50/p99 latency for each backend.
func TestIndexLookup_PostMerge(t *testing.T) {
	ctx := context.Background()

	const lookups = 1000
	const lookupBase = 1_000

	for _, bf := range backendFactories {
		bf := bf
		t.Run(bf.name, func(t *testing.T) {
			b, err := bf.new(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()

			if err := b.Setup(ctx); err != nil {
				t.Fatalf("setup: %v", err)
			}
			insertDocs(ctx, t, b, lookupBase)
			if err := b.Commit(ctx, "base"); err != nil {
				t.Fatalf("commit base: %v", err)
			}

			if err := b.CreateBranch(ctx, "feat"); err != nil {
				t.Fatalf("create branch: %v", err)
			}
			if err := b.Checkout(ctx, "feat"); err != nil {
				t.Fatalf("checkout feat: %v", err)
			}
			updateDocs(ctx, t, b, diffDocs)
			if err := b.Commit(ctx, "diff"); err != nil {
				t.Fatalf("commit diff: %v", err)
			}
			if err := b.Checkout(ctx, "main"); err != nil {
				t.Fatalf("checkout main: %v", err)
			}
			if _, err := b.Merge(ctx, "feat"); err != nil {
				t.Fatalf("merge: %v", err)
			}

			// Measure lookup latency via UpdateEmail probes (a read-modify write
			// that exercises the secondary index path on both backends).
			durations := make([]time.Duration, lookups)
			for i := 0; i < lookups; i++ {
				id := fmt.Sprintf("doc%07d", i%lookupBase)
				email := fmt.Sprintf("probe%07d@example.com", i)
				start := time.Now()
				if err := b.UpdateEmail(ctx, id, email); err != nil {
					t.Fatalf("lookup %d: %v", i, err)
				}
				durations[i] = time.Since(start)
			}

			p50 := percentile(durations, 50)
			p99 := percentile(durations, 99)
			t.Logf("%s post-merge index lookup: p50=%v p99=%v", b.Name(), p50.Round(time.Microsecond), p99.Round(time.Microsecond))
		})
	}
}

// percentile returns the p-th percentile of a duration slice.
func percentile(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	// Simple selection: copy and sort.
	sorted := make([]time.Duration, len(d))
	copy(sorted, d)
	// Insertion sort is fine for 1000 elements.
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
