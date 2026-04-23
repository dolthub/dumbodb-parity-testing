package benchmarks

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// These are indexed variants of the existing CRUD and aggregation benchmarks.
// Each pairs with an unindexed counterpart in crud_bench_test.go /
// agg_bench_test.go, and the two rows appear side-by-side in the compare
// table — the delta between them is the point of this file.
//
// Convention: create the secondary index AFTER seeding (so the seed path stays
// comparable across benchmarks) and BEFORE b.ResetTimer() (so index-build
// time doesn't count against the op under test).

func createIndex(b *testing.B, ctx context.Context, col *mongo.Collection, keys bson.D) {
	b.Helper()
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys}); err != nil {
		b.Fatalf("create index %v: %v", keys, err)
	}
}

// BenchmarkFindOne_ById_Indexed is the explicit-index control: _id already
// has a primary index, so this should match BenchmarkFindOne_ById closely.
// The gap between them is noise / index-lookup overhead variance.
func BenchmarkFindOne_ById_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "findone_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "_id", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

func BenchmarkFind_FilterEq_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "findeq_idx", benchDatasetSize, benchDocSize)
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

func BenchmarkFind_FilterRange_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "findrange_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "i", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "i", Value: bson.D{
			{Key: "$gte", Value: 100},
			{Key: "$lt", Value: 200},
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

// BenchmarkUpdateOne_Indexed is the _id control — should closely match the
// unindexed variant since _id is already the primary key.
func BenchmarkUpdateOne_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "updateone_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "_id", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "touched", Value: i}}}})
		if err != nil {
			b.Fatalf("UpdateOne: %v", err)
		}
	}
}

func BenchmarkUpdateMany_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "updatemany_idx", benchDatasetSize, benchDocSize)
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

// BenchmarkDeleteOne_Indexed is the _id control. It reseeds every
// benchDatasetSize iterations so the collection never runs dry —
// same pattern as its unindexed counterpart.
func BenchmarkDeleteOne_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "deleteone_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "_id", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % benchDatasetSize
		if i > 0 && i%benchDatasetSize == 0 {
			b.StopTimer()
			_, _ = col.DeleteMany(ctx, bson.D{})
			if err := seedCollection(ctx, col, benchDatasetSize, benchDocSize); err != nil {
				b.Fatalf("reseed: %v", err)
			}
			b.StartTimer()
		}
		if _, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}}); err != nil {
			b.Fatalf("DeleteOne: %v", err)
		}
	}
}

func BenchmarkDeleteMany_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "deletemany_idx", benchDatasetSize, benchDocSize)
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
		for j := 0; j < benchDatasetSize/10; j++ {
			d := makeDoc(r, i*benchDatasetSize+j, benchDocSize)
			d[0].Value = fmt.Sprintf("refill-%d-%d", i, j)
			d[2].Value = grp
			refill = append(refill, d)
		}
		if _, err := col.InsertMany(ctx, refill); err != nil {
			b.Fatalf("refill: %v", err)
		}
		b.StartTimer()
	}
}

// BenchmarkCountDocuments_Indexed counts with a filter so the index is
// actually useful. (Counting with no filter gets a collection-level
// shortcut and would not exercise the index.)
func BenchmarkCountDocuments_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "count_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.CountDocuments(ctx, bson.D{{Key: "grp", Value: 5}}); err != nil {
			b.Fatalf("CountDocuments: %v", err)
		}
	}
}

func BenchmarkDistinct_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "distinct_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.Distinct(ctx, "grp", bson.D{}); err != nil {
			b.Fatalf("Distinct: %v", err)
		}
	}
}

func BenchmarkAgg_MatchGroup_Indexed(b *testing.B) {
	col, ctx := withSeededCollection(b, "agg_matchgroup_idx", benchDatasetSize, benchDocSize)
	createIndex(b, ctx, col, bson.D{{Key: "grp", Value: 1}})
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
