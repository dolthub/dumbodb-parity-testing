package benchmarks

import (
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Aggregation benchmarks use the same 1K small-doc dataset as the CRUD suite.
// Each iteration drains the cursor so we measure end-to-end pipeline cost, not
// just the first-batch dispatch.

func BenchmarkAgg_MatchGroup(b *testing.B) {
	col, ctx := withSeededCollection(b, "agg_matchgroup", benchDatasetSize, benchDocSize)
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "grp", Value: bson.D{{Key: "$gte", Value: 0}}}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$grp"},
			{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkAgg_Sort(b *testing.B) {
	col, ctx := withSeededCollection(b, "agg_sort", benchDatasetSize, benchDocSize)
	pipeline := bson.A{
		bson.D{{Key: "$sort", Value: bson.D{{Key: "i", Value: -1}}}},
		bson.D{{Key: "$limit", Value: 100}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkAgg_Project(b *testing.B) {
	col, ctx := withSeededCollection(b, "agg_project", benchDatasetSize, benchDocSize)
	pipeline := bson.A{
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "grp", Value: 1},
			{Key: "tag", Value: 1},
			// Drop payload to exercise the project-projection path specifically.
			{Key: "payload", Value: 0},
		}}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}
