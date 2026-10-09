package benchmarks

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Projection benchmarks return a few small fields of each document. With 10KB
// documents almost all of each document is the payload, which the projection
// discards.

func benchmarkFindProjection(b *testing.B, label string, size docSize, n int, filter func(i int) bson.D) {
	col, ctx := withSeededCollection(b, label, n, size)
	projection := bson.D{{Key: "i", Value: 1}, {Key: "tag", Value: 1}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, filter(i), options.Find().SetProjection(projection))
		if err != nil {
			b.Fatalf("Find: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func noFilter(int) bson.D { return bson.D{} }

// grpFilter matches one of ten grp values, ~10% of the collection.
func grpFilter(i int) bson.D { return bson.D{{Key: "grp", Value: i % 10}} }

func BenchmarkFind_Projection(b *testing.B) {
	benchmarkFindProjection(b, "findproj", benchDocSize, benchDatasetSize, noFilter)
}

func BenchmarkFind_Projection_10KB(b *testing.B) {
	benchmarkFindProjection(b, "findproj_10kb", sizeLarge, datasetSizeFor(sizeLarge), noFilter)
}

func BenchmarkFind_Projection_FilterEq_10KB(b *testing.B) {
	benchmarkFindProjection(b, "findproj_eq_10kb", sizeLarge, datasetSizeFor(sizeLarge), grpFilter)
}

func BenchmarkAgg_Project_10KB(b *testing.B) {
	col, ctx := withSeededCollection(b, "aggproj_10kb", datasetSizeFor(sizeLarge), sizeLarge)
	pipeline := bson.A{
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 1},
			{Key: "grp", Value: 1},
			{Key: "tag", Value: 1},
		}}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}
