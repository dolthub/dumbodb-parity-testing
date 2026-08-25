package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Collated point-lookup-at-scale: the collation counterpart to
// point_lookup_scaled_bench_test.go. The lookup key is the unique STRING field
// sk, and both the index and the query carry an fr_CA collation -- real CLDR
// tailoring (French/backwards accent ordering), so the index stores non-trivial
// ICU sort keys rather than raw UTF-8. Every lookup returns exactly one document,
// so latency growth across N is pure seek cost.
//
// The indexed variant proves end-to-end that a collation-matching query is
// served by the sort-key index and stays near-flat as N grows; the unindexed
// variant is a collated full scan whose latency grows with N. This is the
// black-box, sort-key-path companion to the white-box collated node-count test
// in dumbodb (TestSeekScaling_CollatedPointLookupIsLogarithmic).
//
// Scale ceiling 10K/50K matches the plain point-lookup and scaled-index suites.

func benchCollation() *options.Collation { return &options.Collation{Locale: "fr_CA"} }

// seedStringKeys fills col with n docs {_id:i, sk:"k<i>"} (unique string key).
// Untimed; the caller adds the collated index (or not) before ResetTimer.
func seedStringKeys(b *testing.B, ctx context.Context, col *mongo.Collection, n int) {
	b.Helper()
	const batch = 1000
	buf := make([]interface{}, 0, batch)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if _, err := col.InsertMany(ctx, buf, options.InsertMany().SetOrdered(false)); err != nil {
			b.Fatalf("seed: %v", err)
		}
		buf = buf[:0]
	}
	for i := 0; i < n; i++ {
		buf = append(buf, bson.D{{Key: "_id", Value: i}, {Key: "sk", Value: fmt.Sprintf("k%07d", i)}})
		if len(buf) == batch {
			flush()
		}
	}
	flush()
}

func createCollatedIndex(b *testing.B, ctx context.Context, col *mongo.Collection, coll *options.Collation) {
	b.Helper()
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "sk", Value: 1}},
		Options: options.Index().SetCollation(coll),
	}); err != nil {
		b.Fatalf("create collated index: %v", err)
	}
}

func collatedLookupFind(b *testing.B, ctx context.Context, col *mongo.Collection, coll *options.Collation, key string) int {
	cur, err := col.Find(ctx, bson.D{{Key: "sk", Value: key}}, options.Find().SetCollation(coll))
	if err != nil {
		b.Fatalf("Find: %v", err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		b.Fatalf("cursor.All: %v", err)
	}
	return len(docs)
}

func assertCollatedPointLookup(b *testing.B, ctx context.Context, col *mongo.Collection, coll *options.Collation, n int) {
	if got := collatedLookupFind(b, ctx, col, coll, fmt.Sprintf("k%07d", n/2)); got != 1 {
		b.Fatalf("collated point lookup returned %d docs, want 1", got)
	}
}

func benchmarkCollatedPointLookupScaled(b *testing.B, n int) {
	coll := benchCollation()
	col, ctx := withEmptyCollection(b, fmt.Sprintf("cpointlookup_%d", n))
	seedStringKeys(b, ctx, col, n)
	assertCollatedPointLookup(b, ctx, col, coll, n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collatedLookupFind(b, ctx, col, coll, fmt.Sprintf("k%07d", i%n))
	}
}

func benchmarkCollatedPointLookupScaledIndexed(b *testing.B, n int) {
	coll := benchCollation()
	col, ctx := withEmptyCollection(b, fmt.Sprintf("cpointlookup_%d_idx", n))
	seedStringKeys(b, ctx, col, n)
	createCollatedIndex(b, ctx, col, coll)
	assertCollatedPointLookup(b, ctx, col, coll, n)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collatedLookupFind(b, ctx, col, coll, fmt.Sprintf("k%07d", i%n))
	}
}

func BenchmarkCollatedPointLookup_10K(b *testing.B) { benchmarkCollatedPointLookupScaled(b, 10000) }
func BenchmarkCollatedPointLookup_10K_Indexed(b *testing.B) {
	benchmarkCollatedPointLookupScaledIndexed(b, 10000)
}
func BenchmarkCollatedPointLookup_50K(b *testing.B) { benchmarkCollatedPointLookupScaled(b, 50000) }
func BenchmarkCollatedPointLookup_50K_Indexed(b *testing.B) {
	benchmarkCollatedPointLookupScaledIndexed(b, 50000)
}
