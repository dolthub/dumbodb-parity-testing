package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Point lookup on the unique field i, at 10K and 50K, indexed and unindexed.
// One result regardless of N, so latency across scale is pure seek cost: the
// indexed variant stays near-flat (log-N seek), the unindexed one grows with N.

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
