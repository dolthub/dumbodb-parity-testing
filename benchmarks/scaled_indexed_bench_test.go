package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Scaled-up variants of the indexed/unindexed comparison benchmarks. The 1K
// baselines in indexed_bench_test.go don't show MongoDB's index advantage
// cleanly: at 1K small documents a full-collection scan finishes faster than
// the indexed lookup's overhead, so the indexed/unindexed ratio sits near 1.
// At larger N the unindexed scan grows linearly while the indexed path stays
// near-constant, and the comparison becomes meaningful.
//
// Each scaled benchmark has an indexed and unindexed counterpart at matching
// N. They appear side-by-side in the compare table; the row delta (indexed
// vs unindexed) is the point of this file.
//
// Per-N functions (rather than sub-benchmarks via b.Run) keep the names
// directly searchable in `-bench` regex and visible as one row each in the
// compare table — matching the convention used by the existing 1K benchmarks.
//
// Sizes: we run at 10K and 50K. 10K is where read-side indexes (FilterRange,
// CountDocuments) cleanly cross the 2x threshold on MongoDB; 50K confirms the
// trend. 100K was tried but produced no actionable signal beyond what 50K
// shows: read benchmarks scale similarly, and DumboDB's seed step at that
// size routinely exceeded a 60-minute test timeout. If you want 100K data,
// add the benchmarks back and run with `-test-timeout=2h`.

func benchmarkFindFilterEqScaled(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("findeq_%d", n), n, sizeSmall)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "grp", Value: i % 10}})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func benchmarkFindFilterEqScaledIndexed(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("findeq_%d_idx", n), n, sizeSmall)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "grp", Value: i % 10}})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkFind_FilterEq_10K(b *testing.B)         { benchmarkFindFilterEqScaled(b, 10000) }
func BenchmarkFind_FilterEq_10K_Indexed(b *testing.B) { benchmarkFindFilterEqScaledIndexed(b, 10000) }
func BenchmarkFind_FilterEq_50K(b *testing.B)         { benchmarkFindFilterEqScaled(b, 50000) }
func BenchmarkFind_FilterEq_50K_Indexed(b *testing.B) { benchmarkFindFilterEqScaledIndexed(b, 50000) }

// Range filter bounds scale with n so the result set stays roughly 1% of the
// collection across sizes. Without scaling, a fixed [100, 200) range returns
// 100 docs at n=1000 (10% of the collection — index gives ~10x speedup) and
// the same 100 docs at n=100000 (0.1% — index gives ~1000x). Keeping the
// selectivity constant isolates the index effect from the result-set effect.
func benchmarkFindFilterRangeScaled(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("findrange_%d", n), n, sizeSmall)
	lo, hi := n/10, n/10+n/100
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "i", Value: bson.D{
			{Key: "$gte", Value: lo},
			{Key: "$lt", Value: hi},
		}}})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func benchmarkFindFilterRangeScaledIndexed(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("findrange_%d_idx", n), n, sizeSmall)
	createIndex(b, ctx, col, bson.D{{Key: "i", Value: 1}})
	lo, hi := n/10, n/10+n/100
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "i", Value: bson.D{
			{Key: "$gte", Value: lo},
			{Key: "$lt", Value: hi},
		}}})
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkFind_FilterRange_10K(b *testing.B) { benchmarkFindFilterRangeScaled(b, 10000) }
func BenchmarkFind_FilterRange_10K_Indexed(b *testing.B) {
	benchmarkFindFilterRangeScaledIndexed(b, 10000)
}
func BenchmarkFind_FilterRange_50K(b *testing.B) { benchmarkFindFilterRangeScaled(b, 50000) }
func BenchmarkFind_FilterRange_50K_Indexed(b *testing.B) {
	benchmarkFindFilterRangeScaledIndexed(b, 50000)
}

func benchmarkUpdateManyScaled(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("updatemany_%d", n), n, sizeSmall)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := col.UpdateMany(ctx,
			bson.D{{Key: "grp", Value: i % 10}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "touched", Value: i}}}})
		if err != nil {
			b.Fatalf("UpdateMany: %v", err)
		}
	}
}

func benchmarkUpdateManyScaledIndexed(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("updatemany_%d_idx", n), n, sizeSmall)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := col.UpdateMany(ctx,
			bson.D{{Key: "grp", Value: i % 10}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "touched", Value: i}}}})
		if err != nil {
			b.Fatalf("UpdateMany: %v", err)
		}
	}
}

func BenchmarkUpdateMany_10K(b *testing.B)         { benchmarkUpdateManyScaled(b, 10000) }
func BenchmarkUpdateMany_10K_Indexed(b *testing.B) { benchmarkUpdateManyScaledIndexed(b, 10000) }
func BenchmarkUpdateMany_50K(b *testing.B)         { benchmarkUpdateManyScaled(b, 50000) }
func BenchmarkUpdateMany_50K_Indexed(b *testing.B) { benchmarkUpdateManyScaledIndexed(b, 50000) }

// DeleteMany scales the per-iteration refill cost: each iteration deletes one
// group (n/10 docs) and refills it off the clock. At n=100K that's 10K
// deletes + 10K inserts per timed iteration — slow but bounded.
func benchmarkDeleteManyScaled(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("deletemany_%d", n), n, sizeSmall)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		grp := i % 10
		if _, err := col.DeleteMany(ctx, bson.D{{Key: "grp", Value: grp}}); err != nil {
			b.Fatalf("DeleteMany: %v", err)
		}
		b.StopTimer()
		r := rand.New(rand.NewSource(*dataSeed + int64(i)))
		var refill []interface{}
		for j := 0; j < n/10; j++ {
			d := makeDoc(r, i*n+j, sizeSmall)
			d[0].Value = fmt.Sprintf("refill-%d-%d", i, j)
			d[2].Value = grp
			refill = append(refill, d)
		}
		if _, err := col.InsertMany(ctx, refill, options.InsertMany().SetOrdered(false)); err != nil {
			b.Fatalf("refill: %v", err)
		}
		b.StartTimer()
	}
}

func benchmarkDeleteManyScaledIndexed(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("deletemany_%d_idx", n), n, sizeSmall)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		grp := i % 10
		if _, err := col.DeleteMany(ctx, bson.D{{Key: "grp", Value: grp}}); err != nil {
			b.Fatalf("DeleteMany: %v", err)
		}
		b.StopTimer()
		r := rand.New(rand.NewSource(*dataSeed + int64(i)))
		var refill []interface{}
		for j := 0; j < n/10; j++ {
			d := makeDoc(r, i*n+j, sizeSmall)
			d[0].Value = fmt.Sprintf("refill-%d-%d", i, j)
			d[2].Value = grp
			refill = append(refill, d)
		}
		if _, err := col.InsertMany(ctx, refill, options.InsertMany().SetOrdered(false)); err != nil {
			b.Fatalf("refill: %v", err)
		}
		b.StartTimer()
	}
}

func BenchmarkDeleteMany_10K(b *testing.B)         { benchmarkDeleteManyScaled(b, 10000) }
func BenchmarkDeleteMany_10K_Indexed(b *testing.B) { benchmarkDeleteManyScaledIndexed(b, 10000) }
func BenchmarkDeleteMany_50K(b *testing.B)         { benchmarkDeleteManyScaled(b, 50000) }
func BenchmarkDeleteMany_50K_Indexed(b *testing.B) { benchmarkDeleteManyScaledIndexed(b, 50000) }

func benchmarkCountDocumentsScaled(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("count_%d", n), n, sizeSmall)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.CountDocuments(ctx, bson.D{{Key: "grp", Value: 5}}); err != nil {
			b.Fatalf("CountDocuments: %v", err)
		}
	}
}

func benchmarkCountDocumentsScaledIndexed(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("count_%d_idx", n), n, sizeSmall)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.CountDocuments(ctx, bson.D{{Key: "grp", Value: 5}}); err != nil {
			b.Fatalf("CountDocuments: %v", err)
		}
	}
}

func BenchmarkCountDocuments_10K(b *testing.B) { benchmarkCountDocumentsScaled(b, 10000) }
func BenchmarkCountDocuments_10K_Indexed(b *testing.B) {
	benchmarkCountDocumentsScaledIndexed(b, 10000)
}
func BenchmarkCountDocuments_50K(b *testing.B) { benchmarkCountDocumentsScaled(b, 50000) }
func BenchmarkCountDocuments_50K_Indexed(b *testing.B) {
	benchmarkCountDocumentsScaledIndexed(b, 50000)
}
