package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Point-lookup-at-scale: the black-box latency companion to dumbodb's white-box
// node-fetch proof (workspace-da6.1). The existing scaled reads return a growing
// slice of the collection -- Find_FilterEq ~10% (one of ten grp values),
// Find_FilterRange ~1% -- so their latency mixes seek cost with materializing an
// N-proportional result set. These filter on the UNIQUE field i, so every lookup
// returns exactly one document and the result set is constant across N. Latency
// growth from 10K to 50K is therefore pure seek cost.
//
// The indexed variant should stay near-flat (log-N seek); the unindexed variant
// is a full scan whose latency grows with N. Their side-by-side delta, and the
// indexed row's flatness across N, is the point -- an end-to-end confirmation
// that a served point lookup does sub-linear work.
//
// Scale ceiling matches scaled_indexed_bench_test.go: 10K and 50K. DumboDB's
// wire-protocol seed step gates larger N (~30 min at 50K; 100K exceeds a 60-min
// timeout). da6.1's in-process node-count test reaches 100K+ cheaply and carries
// the direct O(log N) growth assertion; this benchmark confirms it end-to-end
// within the seedable range and against MongoDB's shape.

// pointLookupFind runs one equality lookup on the unique field i and returns the
// number of matched documents.
func pointLookupFind(b *testing.B, ctx context.Context, col *mongo.Collection, key int) int {
	cur, err := col.Find(ctx, bson.D{{Key: "i", Value: key}})
	if err != nil {
		b.Fatalf("Find: %v", err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		b.Fatalf("cursor.All: %v", err)
	}
	return len(docs)
}

// assertPointLookup fails before the timer starts if a lookup on i does not
// return exactly one doc -- a guard that these really are point lookups (i is
// unique), not a mis-seeded fan-out that would flatter the seek.
func assertPointLookup(b *testing.B, ctx context.Context, col *mongo.Collection, n int) {
	if got := pointLookupFind(b, ctx, col, n/2); got != 1 {
		b.Fatalf("point lookup i=%d returned %d docs, want 1", n/2, got)
	}
}

func benchmarkPointLookupScaled(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("pointlookup_%d", n), n, sizeSmall)
	assertPointLookup(b, ctx, col, n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pointLookupFind(b, ctx, col, i%n)
	}
}

func benchmarkPointLookupScaledIndexed(b *testing.B, n int) {
	col, ctx := withSeededCollection(b, fmt.Sprintf("pointlookup_%d_idx", n), n, sizeSmall)
	createIndex(b, ctx, col, bson.D{{Key: "i", Value: 1}})
	assertPointLookup(b, ctx, col, n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pointLookupFind(b, ctx, col, i%n)
	}
}

func BenchmarkPointLookup_10K(b *testing.B)         { benchmarkPointLookupScaled(b, 10000) }
func BenchmarkPointLookup_10K_Indexed(b *testing.B) { benchmarkPointLookupScaledIndexed(b, 10000) }
func BenchmarkPointLookup_50K(b *testing.B)         { benchmarkPointLookupScaled(b, 50000) }
func BenchmarkPointLookup_50K_Indexed(b *testing.B) { benchmarkPointLookupScaledIndexed(b, 50000) }
