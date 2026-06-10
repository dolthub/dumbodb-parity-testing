package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestCapped_CreateCollection_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_CreateCollection_Basic",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_basic"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			defer db.Collection(cappedName).Drop(ctx)
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestCapped_CreateCollection_WithMax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_CreateCollection_WithMax",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_withmax"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(65536).SetMaxDocuments(10)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			defer db.Collection(cappedName).Drop(ctx)
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestCapped_InsertAndEviction(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_InsertAndEviction",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_eviction"
			// max 3 documents, 4096 bytes
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096).SetMaxDocuments(3)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			// Insert 5 documents; oldest 2 should be evicted
			for i := 1; i <= 5; i++ {
				if _, err := capped.InsertOne(ctx, bson.D{{Key: "seq", Value: int32(i)}}); err != nil {
					return nil, err
				}
			}

			count, err := capped.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCapped_NaturalOrderCursor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_NaturalOrderCursor",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_natural"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			for i := 1; i <= 3; i++ {
				if _, err := capped.InsertOne(ctx, bson.D{{Key: "seq", Value: int32(i)}}); err != nil {
					return nil, err
				}
			}

			// Natural order: $natural sort
			findOpts := options.Find().SetSort(bson.D{{Key: "$natural", Value: 1}})
			cur, err := capped.Find(ctx, bson.D{}, findOpts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			seqs := make([]int32, len(results))
			for i, r := range results {
				for _, e := range r {
					if e.Key == "seq" {
						if v, ok := e.Value.(int32); ok {
							seqs[i] = v
						}
					}
				}
			}
			return bson.D{{Key: "seqs", Value: seqs}}, nil
		},
	})
}

func TestCapped_NaturalOrderReverse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_NaturalOrderReverse",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_natural_rev"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			for i := 1; i <= 3; i++ {
				if _, err := capped.InsertOne(ctx, bson.D{{Key: "seq", Value: int32(i)}}); err != nil {
					return nil, err
				}
			}

			findOpts := options.Find().SetSort(bson.D{{Key: "$natural", Value: -1}})
			cur, err := capped.Find(ctx, bson.D{}, findOpts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCapped_TailableCursor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_TailableCursor",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_tailable"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			if _, err := capped.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}}); err != nil {
				return nil, err
			}

			cursorType := options.Tailable
			findOpts := options.Find().SetCursorType(cursorType)
			cur, err := capped.Find(ctx, bson.D{}, findOpts)
			if err != nil {
				return nil, err
			}
			defer cur.Close(ctx)

			var docs []bson.D
			// Read available docs (don't block waiting for new ones)
			for cur.TryNext(ctx) {
				var d bson.D
				if err := cur.Decode(&d); err != nil {
					return nil, err
				}
				docs = append(docs, d)
			}
			return bson.D{{Key: "count", Value: int32(len(docs))}}, nil
		},
	})
}

func TestCapped_DeleteFails(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_DeleteFails",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_delete"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			if _, err := capped.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}}); err != nil {
				return nil, err
			}

			_, err := capped.DeleteOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			if err != nil {
				return bson.D{{Key: "error", Value: err.Error()}}, nil
			}
			return bson.D{{Key: "error", Value: "none"}}, nil
		},
	})
}

func TestCapped_UpdateGrowthFails(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_UpdateGrowthFails",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_grow"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			if _, err := capped.InsertOne(ctx, bson.D{{Key: "x", Value: "small"}}); err != nil {
				return nil, err
			}

			// Attempt to grow the document significantly
			_, err := capped.UpdateOne(ctx,
				bson.D{{Key: "x", Value: "small"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "this is a much larger string that should fail"}}}},
			)
			if err != nil {
				return bson.D{{Key: "error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "error", Value: "none"}}, nil
		},
	})
}

func TestCapped_InsertMany_ExceedMax(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_InsertMany_ExceedMax",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_insertmany"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096).SetMaxDocuments(5)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			docs := make([]interface{}, 10)
			for i := 0; i < 10; i++ {
				docs[i] = bson.D{{Key: "seq", Value: int32(i)}}
			}
			if _, err := capped.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			count, err := capped.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCapped_IsCapped_CollStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_IsCapped_CollStats",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_stats"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			defer db.Collection(cappedName).Drop(ctx)

			var result bson.M
			if err := db.RunCommand(ctx, bson.D{
				{Key: "collStats", Value: cappedName},
			}).Decode(&result); err != nil {
				return nil, err
			}

			capped, _ := result["capped"].(bool)
			return bson.D{{Key: "capped", Value: capped}}, nil
		},
	})
}

func TestView_CreateFromCollection_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_CreateFromCollection_Basic",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "name", Value: "Alice"}, {Key: "age", Value: int32(30)}},
				bson.D{{Key: "name", Value: "Bob"}, {Key: "age", Value: int32(25)}},
				bson.D{{Key: "name", Value: "Carol"}, {Key: "age", Value: int32(35)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_basic"
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "age", Value: bson.D{{Key: "$gte", Value: int32(30)}}}}}},
			}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			count, err := db.Collection(viewName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestView_CreateWithProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_CreateWithProjection",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "name", Value: "Alice"}, {Key: "secret", Value: "s1"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "name", Value: "Bob"}, {Key: "secret", Value: "s2"}, {Key: "score", Value: int32(20)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_proj"
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$project", Value: bson.D{{Key: "name", Value: int32(1)}, {Key: "score", Value: int32(1)}, {Key: "_id", Value: int32(0)}}}},
			}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			// Return each projected doc's name plus whether the projected-out
			// fields are present, so the parity check verifies the view's
			// projection is actually applied (secret and _id dropped) rather
			// than only that two docs came back.
			cur, err := db.Collection(viewName).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			out := bson.A{}
			for _, d := range results {
				m := d.Map()
				_, hasSecret := m["secret"]
				_, hasID := m["_id"]
				out = append(out, bson.D{
					{Key: "name", Value: m["name"]},
					{Key: "hasSecret", Value: hasSecret},
					{Key: "hasID", Value: hasID},
				})
			}
			return bson.D{{Key: "docs", Value: out}}, nil
		},
	})
}

func TestView_QueryIsReadOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_QueryIsReadOnly",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_readonly"
			pipeline := mongo.Pipeline{}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			view := db.Collection(viewName)
			_, err := view.InsertOne(ctx, bson.D{{Key: "y", Value: int32(2)}})
			if err != nil {
				return bson.D{{Key: "write_error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "write_error", Value: "none"}}, nil
		},
	})
}

func TestView_WriteUpdateFails(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_WriteUpdateFails",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_updfail"
			pipeline := mongo.Pipeline{}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			_, err := db.Collection(viewName).UpdateOne(ctx,
				bson.D{{Key: "x", Value: int32(1)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(2)}}}},
			)
			if err != nil {
				return bson.D{{Key: "write_error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "write_error", Value: "none"}}, nil
		},
	})
}

func TestView_WriteDeleteFails(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_WriteDeleteFails",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_delfail"
			pipeline := mongo.Pipeline{}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			_, err := db.Collection(viewName).DeleteOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			if err != nil {
				return bson.D{{Key: "write_error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "write_error", Value: "none"}}, nil
		},
	})
}

func TestView_WithMatchPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_WithMatchPipeline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "status", Value: "active"}, {Key: "val", Value: int32(1)}},
				bson.D{{Key: "status", Value: "inactive"}, {Key: "val", Value: int32(2)}},
				bson.D{{Key: "status", Value: "active"}, {Key: "val", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_match"
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: "active"}}}},
			}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			count, err := db.Collection(viewName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

// TestView_Find_And_Count_ApplyMatchPipeline guards that the view's $match
// pipeline is applied on the find() path and the legacy count command, not
// only on aggregate/countDocuments. Before the fix find() returned all source
// rows (pipeline ignored) and the count command returned 0.
func TestView_Find_And_Count_ApplyMatchPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_Find_And_Count_ApplyMatchPipeline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "status", Value: "active"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "status", Value: "inactive"}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "status", Value: "active"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_find_match"
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "status", Value: "active"}}}},
			}
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			cur, err := db.Collection(viewName).Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			findIDs := bson.A{}
			for _, d := range results {
				findIDs = append(findIDs, d.Map()["_id"])
			}

			var countCmd bson.D
			if err := db.RunCommand(ctx, bson.D{{Key: "count", Value: viewName}}).Decode(&countCmd); err != nil {
				return nil, err
			}

			return bson.D{
				{Key: "findIDs", Value: findIDs},
				{Key: "countN", Value: countCmd.Map()["n"]},
			}, nil
		},
	})
}

func TestView_ListCollections_ShowsView(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_ListCollections_ShowsView",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_listcols"
			pipeline := mongo.Pipeline{}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			cur, err := db.ListCollections(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var cols []bson.M
			if err := cur.All(ctx, &cols); err != nil {
				return nil, err
			}
			var found bool
			for _, c := range cols {
				if c["name"] == viewName {
					found = true
					break
				}
			}
			return bson.D{{Key: "found", Value: found}}, nil
		},
	})
}

func TestView_WithLookupPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_WithLookupPipeline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "dept_id", Value: int32(1)}, {Key: "name", Value: "Alice"}},
				bson.D{{Key: "dept_id", Value: int32(2)}, {Key: "name", Value: "Bob"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			// Create a departments collection
			deptCol := db.Collection("depts_lookup")
			defer deptCol.Drop(ctx)
			_, err := deptCol.InsertMany(ctx, []interface{}{
				bson.D{{Key: "dept_id", Value: int32(1)}, {Key: "dept_name", Value: "Engineering"}},
				bson.D{{Key: "dept_id", Value: int32(2)}, {Key: "dept_name", Value: "Marketing"}},
			})
			if err != nil {
				return nil, err
			}

			viewName := "view_lookup"
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: deptCol.Name()},
					{Key: "localField", Value: "dept_id"},
					{Key: "foreignField", Value: "dept_id"},
					{Key: "as", Value: "dept"},
				}}},
			}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			count, err := db.Collection(viewName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestView_QueryWithFilter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_QueryWithFilter",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "score", Value: int32(10)}, {Key: "category", Value: "A"}},
				bson.D{{Key: "score", Value: int32(20)}, {Key: "category", Value: "B"}},
				bson.D{{Key: "score", Value: int32(30)}, {Key: "category", Value: "A"}},
				bson.D{{Key: "score", Value: int32(40)}, {Key: "category", Value: "B"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_queryfilt"
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "category", Value: "A"}}}},
			}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			count, err := db.Collection(viewName).CountDocuments(ctx, bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(15)}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestView_Empty_Pipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_Empty_Pipeline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "x", Value: int32(1)}},
				bson.D{{Key: "x", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_emptypipe"
			pipeline := mongo.Pipeline{}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			count, err := db.Collection(viewName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_CreateCollection_Basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_CreateCollection_Basic",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_basic"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			defer db.Collection(tsName).Drop(ctx)
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestTimeSeries_CreateCollection_WithMetaField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_CreateCollection_WithMetaField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_meta"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			defer db.Collection(tsName).Drop(ctx)
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestTimeSeries_CreateCollection_WithGranularity(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_CreateCollection_WithGranularity",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_granularity"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetGranularity("hours"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			defer db.Collection(tsName).Drop(ctx)
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestTimeSeries_InsertDocuments(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_InsertDocuments",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_insert"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: 22.5}},
				bson.D{{Key: "ts", Value: now.Add(time.Minute)}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: 23.0}},
				bson.D{{Key: "ts", Value: now.Add(2 * time.Minute)}, {Key: "sensor_id", Value: "s2"}, {Key: "temp", Value: 18.5}},
			}
			res, err := ts.InsertMany(ctx, docs)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "inserted", Value: int32(len(res.InsertedIDs))}}, nil
		},
	})
}

func TestTimeSeries_QueryByTimeRange(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_QueryByTimeRange",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_timerange"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			docs := make([]interface{}, 5)
			for i := 0; i < 5; i++ {
				docs[i] = bson.D{
					{Key: "ts", Value: base.Add(time.Duration(i) * time.Hour)},
					{Key: "sensor_id", Value: "s1"},
					{Key: "value", Value: int32(i * 10)},
				}
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			// Query for docs in first 3 hours
			filter := bson.D{{Key: "ts", Value: bson.D{
				{Key: "$gte", Value: base},
				{Key: "$lt", Value: base.Add(3 * time.Hour)},
			}}}
			count, err := ts.CountDocuments(ctx, filter)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_QueryBySensorID(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_QueryBySensorID",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_bysensor"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "val", Value: int32(1)}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "sensor_id", Value: "s2"}, {Key: "val", Value: int32(2)}},
				bson.D{{Key: "ts", Value: now.Add(2 * time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "val", Value: int32(3)}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			count, err := ts.CountDocuments(ctx, bson.D{{Key: "sensor_id", Value: "s1"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_GroupAggregation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_GroupAggregation",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_groupagg"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: 20.0}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: 22.0}},
				bson.D{{Key: "ts", Value: now.Add(2 * time.Second)}, {Key: "sensor_id", Value: "s2"}, {Key: "temp", Value: 15.0}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$sensor_id"},
					{Key: "avg_temp", Value: bson.D{{Key: "$avg", Value: "$temp"}}},
					{Key: "count", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}
			cur, err := ts.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "group_count", Value: int32(len(results))}}, nil
		},
	})
}

func TestTimeSeries_CollStats(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_CollStats",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_collstats"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			defer db.Collection(tsName).Drop(ctx)

			var result bson.M
			if err := db.RunCommand(ctx, bson.D{
				{Key: "collStats", Value: tsName},
			}).Decode(&result); err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestTimeSeries_ListCollections_ShowsTimeSeries(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_ListCollections_ShowsTimeSeries",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_listcols"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			defer db.Collection(tsName).Drop(ctx)

			cur, err := db.ListCollections(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var cols []bson.M
			if err := cur.All(ctx, &cols); err != nil {
				return nil, err
			}
			var found bool
			for _, c := range cols {
				if c["name"] == tsName {
					found = true
					break
				}
			}
			return bson.D{{Key: "found", Value: found}}, nil
		},
	})
}

func TestTimeSeries_AggregateMatch_TimeRange(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_AggregateMatch_TimeRange",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_aggmatch"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			base := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
			docs := make([]interface{}, 10)
			for i := 0; i < 10; i++ {
				docs[i] = bson.D{
					{Key: "ts", Value: base.Add(time.Duration(i) * time.Hour)},
					{Key: "sensor_id", Value: fmt.Sprintf("s%d", i%3)},
					{Key: "value", Value: int32(i)},
				}
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "ts", Value: bson.D{
					{Key: "$gte", Value: base},
					{Key: "$lt", Value: base.Add(5 * time.Hour)},
				}}}}},
				bson.D{{Key: "$count", Value: "total"}},
			}
			cur, err := ts.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestTimeSeries_InsertOne_MissingTimeField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_InsertOne_MissingTimeField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_missing_ts"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			// Missing "ts" field — should fail
			_, err := ts.InsertOne(ctx, bson.D{{Key: "value", Value: int32(42)}})
			if err != nil {
				return bson.D{{Key: "error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "error", Value: "none"}}, nil
		},
	})
}

func TestTimeSeries_InsertOne_NonDateTimeField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_InsertOne_NonDateTimeField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_nondate_ts"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			// "ts" field is a string, not a Date — should fail
			_, err := ts.InsertOne(ctx, bson.D{{Key: "ts", Value: "not-a-date"}, {Key: "value", Value: int32(1)}})
			if err != nil {
				return bson.D{{Key: "error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "error", Value: "none"}}, nil
		},
	})
}

func TestTimeSeries_Granularity_Seconds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_Granularity_Seconds",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_gran_sec"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetGranularity("seconds"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			if _, err := ts.InsertOne(ctx, bson.D{
				{Key: "ts", Value: now},
				{Key: "value", Value: int32(100)},
			}); err != nil {
				return nil, err
			}
			count, err := ts.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_Granularity_Minutes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_Granularity_Minutes",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_gran_min"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetGranularity("minutes"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			if _, err := ts.InsertOne(ctx, bson.D{
				{Key: "ts", Value: now},
				{Key: "value", Value: int32(200)},
			}); err != nil {
				return nil, err
			}
			count, err := ts.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_MaxTimeField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_MaxTimeField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_maxtime"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
			docs := make([]interface{}, 5)
			for i := 0; i < 5; i++ {
				docs[i] = bson.D{
					{Key: "ts", Value: base.Add(time.Duration(i) * 24 * time.Hour)},
					{Key: "sensor_id", Value: "s1"},
					{Key: "value", Value: int32(i)},
				}
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			// Aggregate to find max ts
			pipeline := mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "maxTs", Value: bson.D{{Key: "$max", Value: "$ts"}}},
				}}},
			}
			cur, err := ts.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "has_result", Value: len(results) > 0}}, nil
		},
	})
}

func TestTimeSeries_SumAggregation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_SumAggregation",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_sumagg"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "value", Value: int32(10)}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "value", Value: int32(20)}},
				bson.D{{Key: "ts", Value: now.Add(2 * time.Second)}, {Key: "sensor_id", Value: "s2"}, {Key: "value", Value: int32(5)}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$sensor_id"},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$value"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}
			cur, err := ts.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "group_count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCapped_SmallSize_ManyInserts(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_SmallSize_ManyInserts",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_small"
			// Very small: 1024 bytes
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(1024)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			for i := 0; i < 20; i++ {
				if _, err := capped.InsertOne(ctx, bson.D{
					{Key: "seq", Value: int32(i)},
					{Key: "data", Value: "padding"},
				}); err != nil {
					return nil, err
				}
			}
			count, err := capped.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			// Should be less than 20 — oldest evicted
			return bson.D{{Key: "count_under_20", Value: count < 20}}, nil
		},
	})
}

func TestCapped_Find_AfterEviction(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_Find_AfterEviction",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_find_evict"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096).SetMaxDocuments(3)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			// Insert 5, only 3 survive
			for i := 1; i <= 5; i++ {
				if _, err := capped.InsertOne(ctx, bson.D{{Key: "n", Value: int32(i)}}); err != nil {
					return nil, err
				}
			}

			// seq 1 and 2 should be evicted; 3, 4, 5 remain
			count1, err := capped.CountDocuments(ctx, bson.D{{Key: "n", Value: int32(1)}})
			if err != nil {
				return nil, err
			}
			count5, err := capped.CountDocuments(ctx, bson.D{{Key: "n", Value: int32(5)}})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "n1_gone", Value: count1 == 0},
				{Key: "n5_present", Value: count5 == 1},
			}, nil
		},
	})
}

func TestView_OnCappedCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_OnCappedCollection",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_for_view"
			viewName := "view_on_capped"

			cappedOpts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, cappedOpts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)
			defer db.Collection(viewName).Drop(ctx)

			if _, err := capped.InsertMany(ctx, []interface{}{
				bson.D{{Key: "val", Value: int32(1)}},
				bson.D{{Key: "val", Value: int32(2)}},
				bson.D{{Key: "val", Value: int32(3)}},
			}); err != nil {
				return nil, err
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "val", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}},
			}
			// pipeline passed directly to CreateView
			if err := db.CreateView(ctx, viewName, cappedName, pipeline); err != nil {
				return nil, err
			}

			count, err := db.Collection(viewName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_WithExpireAfterSeconds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_WithExpireAfterSeconds",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_expire"
			expireAfter := int64(3600) // 1 hour
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			).SetExpireAfterSeconds(expireAfter)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			if _, err := ts.InsertOne(ctx, bson.D{
				{Key: "ts", Value: now},
				{Key: "value", Value: int32(1)},
			}); err != nil {
				return nil, err
			}
			count, err := ts.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestTimeSeries_AggregateAvg(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_AggregateAvg",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_avgagg"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "value", Value: int32(10)}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "value", Value: int32(30)}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "avg_val", Value: bson.D{{Key: "$avg", Value: "$value"}}},
				}}},
			}
			cur, err := ts.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "has_avg", Value: len(results) == 1}}, nil
		},
	})
}

func TestTimeSeries_MultipleMetaValues(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_MultipleMetaValues",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_multimeta"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("tags"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "tags", Value: bson.D{{Key: "region", Value: "us"}, {Key: "device", Value: "A"}}}, {Key: "val", Value: int32(1)}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "tags", Value: bson.D{{Key: "region", Value: "eu"}, {Key: "device", Value: "B"}}}, {Key: "val", Value: int32(2)}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}
			count, err := ts.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestView_DropAndRecreate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_DropAndRecreate",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "x", Value: int32(1)}},
				bson.D{{Key: "x", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_droprecreate"
			pipeline := mongo.Pipeline{}
			// pipeline passed directly to CreateView

			// Create view
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			view := db.Collection(viewName)

			count1, err := view.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}

			// Drop and recreate with filter
			if err := view.Drop(ctx); err != nil {
				return nil, err
			}
			filteredPipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}},
			}
			// pipeline passed directly
			if err := db.CreateView(ctx, viewName, col.Name(), filteredPipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			count2, err := db.Collection(viewName).CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "count_before", Value: count1},
				{Key: "count_after", Value: count2},
			}, nil
		},
	})
}

func TestCapped_CreateIndex_Fails(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_CreateIndex_Fails",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_idx"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			// Creating a unique index on a capped collection is not allowed in MongoDB
			model := mongo.IndexModel{
				Keys:    bson.D{{Key: "name", Value: 1}},
				Options: options.Index().SetUnique(true),
			}
			_, err := capped.Indexes().CreateOne(ctx, model)
			if err != nil {
				return bson.D{{Key: "error", Value: "got_error"}}, nil
			}
			return bson.D{{Key: "error", Value: "none"}}, nil
		},
	})
}

func TestTimeSeries_DistinctOnMetaField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_DistinctOnMetaField",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_distinct"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "val", Value: int32(1)}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "sensor_id", Value: "s2"}, {Key: "val", Value: int32(2)}},
				bson.D{{Key: "ts", Value: now.Add(2 * time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "val", Value: int32(3)}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			results, err := ts.Distinct(ctx, "sensor_id", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "distinct_count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCapped_CountDocuments_Empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_CountDocuments_Empty",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_count_empty"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			count, err := capped.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCapped_FindOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_FindOne",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_findone"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			if _, err := capped.InsertOne(ctx, bson.D{{Key: "x", Value: int32(42)}}); err != nil {
				return nil, err
			}
			var result bson.D
			if err := capped.FindOne(ctx, bson.D{{Key: "x", Value: int32(42)}}).Decode(&result); err != nil {
				return nil, err
			}
			for _, e := range result {
				if e.Key == "x" {
					return bson.D{{Key: "x", Value: e.Value}}, nil
				}
			}
			return bson.D{{Key: "x", Value: nil}}, nil
		},
	})
}

func TestTimeSeries_FindAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_FindAll",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_findall"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := make([]interface{}, 4)
			for i := 0; i < 4; i++ {
				docs[i] = bson.D{
					{Key: "ts", Value: now.Add(time.Duration(i) * time.Second)},
					{Key: "sensor_id", Value: "s1"},
					{Key: "val", Value: int32(i)},
				}
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			cur, err := ts.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestView_Sort_OnView(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_Sort_OnView",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "val", Value: int32(3)}},
				bson.D{{Key: "val", Value: int32(1)}},
				bson.D{{Key: "val", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_sort"
			pipeline := mongo.Pipeline{}
			if err := db.CreateView(ctx, viewName, col.Name(), pipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			findOpts := options.Find().SetSort(bson.D{{Key: "val", Value: 1}})
			cur, err := db.Collection(viewName).Find(ctx, bson.D{}, findOpts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			// Return the ordered val sequence so the parity check verifies sort
			// order on the view, not just the row count.
			vals := bson.A{}
			for _, d := range results {
				vals = append(vals, d.Map()["val"])
			}
			return bson.D{{Key: "vals", Value: vals}}, nil
		},
	})
}

func TestCapped_UpdateSameSize_Succeeds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Capped_UpdateSameSize_Succeeds",
		Support: harness.DumboDBMongoOnly,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			cappedName := "capped_upd_same"
			opts := options.CreateCollection().SetCapped(true).SetSizeInBytes(4096)
			if err := db.CreateCollection(ctx, cappedName, opts); err != nil {
				return nil, err
			}
			capped := db.Collection(cappedName)
			defer capped.Drop(ctx)

			if _, err := capped.InsertOne(ctx, bson.D{{Key: "x", Value: int32(1)}}); err != nil {
				return nil, err
			}

			// Same-size update (int32 -> int32) should be allowed
			res, err := capped.UpdateOne(ctx,
				bson.D{{Key: "x", Value: int32(1)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(2)}}}},
			)
			if err != nil {
				return bson.D{{Key: "error", Value: err.Error()}}, nil
			}
			return bson.D{{Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

func TestTimeSeries_Empty_Collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_Empty_Collection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_empty"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			count, err := ts.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestView_Aggregate_OnView(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_Aggregate_OnView",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "cat", Value: "A"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "cat", Value: "B"}, {Key: "score", Value: int32(20)}},
				bson.D{{Key: "cat", Value: "A"}, {Key: "score", Value: int32(30)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			viewName := "view_agg"
			viewPipeline := mongo.Pipeline{
				bson.D{{Key: "$match", Value: bson.D{{Key: "cat", Value: "A"}}}},
			}
			if err := db.CreateView(ctx, viewName, col.Name(), viewPipeline); err != nil {
				return nil, err
			}
			defer db.Collection(viewName).Drop(ctx)

			aggPipeline := mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$score"}}},
				}}},
			}
			cur, err := db.Collection(viewName).Aggregate(ctx, aggPipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			if len(results) == 0 {
				return bson.D{{Key: "total", Value: int32(0)}}, nil
			}
			return results[0], nil
		},
	})
}

func TestTimeSeries_MinAggregation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "TimeSeries_MinAggregation",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			tsName := "ts_minagg"
			tsOpts := options.CreateCollection().SetTimeSeriesOptions(
				options.TimeSeries().SetTimeField("ts").SetMetaField("sensor_id"),
			)
			if err := db.CreateCollection(ctx, tsName, tsOpts); err != nil {
				return nil, err
			}
			ts := db.Collection(tsName)
			defer ts.Drop(ctx)

			now := time.Now()
			docs := []interface{}{
				bson.D{{Key: "ts", Value: now}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: int32(5)}},
				bson.D{{Key: "ts", Value: now.Add(time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: int32(15)}},
				bson.D{{Key: "ts", Value: now.Add(2 * time.Second)}, {Key: "sensor_id", Value: "s1"}, {Key: "temp", Value: int32(10)}},
			}
			if _, err := ts.InsertMany(ctx, docs); err != nil {
				return nil, err
			}

			pipeline := mongo.Pipeline{
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "min_temp", Value: bson.D{{Key: "$min", Value: "$temp"}}},
					{Key: "max_temp", Value: bson.D{{Key: "$max", Value: "$temp"}}},
				}}},
			}
			cur, err := ts.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cur.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "has_result", Value: len(results) == 1}}, nil
		},
	})
}
