package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// cursorSeedDocs: small dataset for cursor option tests.
var cursorSeedDocs = []interface{}{
	bson.D{{Key: "_id", Value: "c1"}, {Key: "val", Value: int32(10)}, {Key: "group", Value: "a"}},
	bson.D{{Key: "_id", Value: "c2"}, {Key: "val", Value: int32(20)}, {Key: "group", Value: "b"}},
	bson.D{{Key: "_id", Value: "c3"}, {Key: "val", Value: int32(30)}, {Key: "group", Value: "a"}},
	bson.D{{Key: "_id", Value: "c4"}, {Key: "val", Value: int32(40)}, {Key: "group", Value: "b"}},
	bson.D{{Key: "_id", Value: "c5"}, {Key: "val", Value: int32(50)}, {Key: "group", Value: "a"}},
	bson.D{{Key: "_id", Value: "c6"}, {Key: "val", Value: int32(60)}, {Key: "group", Value: "b"}},
	bson.D{{Key: "_id", Value: "c7"}, {Key: "val", Value: int32(70)}, {Key: "group", Value: "a"}},
	bson.D{{Key: "_id", Value: "c8"}, {Key: "val", Value: int32(80)}, {Key: "group", Value: "b"}},
}

func insertCursorSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, cursorSeedDocs)
	return err
}

func TestCursor_find_maxTimeMS(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_maxTimeMS",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			dur := 5 * time.Second
			opts := options.Find().SetMaxTime(dur).SetSort(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_batchSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_batchSize",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// batchSize=2 forces multiple round trips; final result is identical.
			opts := options.Find().SetBatchSize(2).SetSort(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_batchSize_one(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_batchSize_one",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetBatchSize(1).SetSort(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_exhaustion_noDocsAfterAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_exhaustion_noDocsAfterAll",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var first []bson.D
			if err := cursor.All(ctx, &first); err != nil {
				return nil, err
			}
			// After All(), Next() must return false.
			hasMore := cursor.Next(ctx)
			return bson.D{
				{Key: "count", Value: int32(len(first))},
				{Key: "hasMore", Value: hasMore},
			}, nil
		},
	})
}

func TestCursor_exhaustion_emptyCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_exhaustion_emptyCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "count", Value: int32(len(results))},
				{Key: "hasMore", Value: cursor.Next(ctx)},
			}, nil
		},
	})
}

func TestCursor_iterate_manually(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_iterate_manually",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(3)
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			defer cursor.Close(ctx)
			var ids []interface{}
			for cursor.Next(ctx) {
				var doc bson.D
				if err := cursor.Decode(&doc); err != nil {
					return nil, err
				}
				for _, e := range doc {
					if e.Key == "_id" {
						ids = append(ids, e.Value)
					}
				}
			}
			return bson.D{{Key: "ids", Value: ids}}, cursor.Err()
		},
	})
}

func TestCursor_find_hint_naturalOrder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_hint_naturalOrder",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetHint(bson.D{{Key: "$natural", Value: 1}}).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_hint_idIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_hint_idIndex",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetHint("_id_").
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_comment(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_comment",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetComment("cursor_comment_test").
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_allowDiskUse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_allowDiskUse",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetAllowDiskUse(true).
				SetSort(bson.D{{Key: "val", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_readPreference_primary(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_readPreference_primary",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			col2 := col.Database().Collection(col.Name(), options.Collection().SetReadPreference(readpref.Primary()))
			cursor, err := col2.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_find_readPreference_primaryPreferred(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_readPreference_primaryPreferred",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			col2 := col.Database().Collection(col.Name(), options.Collection().SetReadPreference(readpref.PrimaryPreferred()))
			cursor, err := col2.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_sort_multiField_groupThenVal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_sort_multiField_groupThenVal",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "group", Value: 1}, {Key: "val", Value: -1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}, {Key: "group", Value: 1}, {Key: "val", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_sort_multiField_valThenId(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_sort_multiField_valThenId",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "val", Value: -1}, {Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_skipLimit_page1(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_skipLimit_page1",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(0).SetLimit(3).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_skipLimit_page2(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_skipLimit_page2",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(3).SetLimit(3).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_skipLimit_beyondEnd(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_skipLimit_beyondEnd",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(100).SetLimit(10).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCursor_skipOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_skipOnly",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(5).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_limitOnly(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_limitOnly",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetLimit(4).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCursor_aggregate_batchSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_aggregate_batchSize",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Aggregate().SetBatchSize(2)
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_aggregate_allowDiskUse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_aggregate_allowDiskUse",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Aggregate().SetAllowDiskUse(true)
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "val", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_aggregate_maxTimeMS(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_aggregate_maxTimeMS",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			dur := 5 * time.Second
			opts := options.Aggregate().SetMaxTime(dur)
			cursor, err := col.Aggregate(ctx, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_findOne_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_findOne_sort",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().SetSort(bson.D{{Key: "val", Value: -1}})
			var result bson.D
			err := col.FindOne(ctx, bson.D{}, opts).Decode(&result)
			if err != nil {
				return nil, err
			}
			var id interface{}
			for _, e := range result {
				if e.Key == "_id" {
					id = e.Value
				}
			}
			return bson.D{{Key: "_id", Value: id}}, nil
		},
	})
}

func TestCursor_findOne_skip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_findOne_skip",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(2)
			var result bson.D
			err := col.FindOne(ctx, bson.D{}, opts).Decode(&result)
			if err != nil {
				return nil, err
			}
			var id interface{}
			for _, e := range result {
				if e.Key == "_id" {
					id = e.Value
				}
			}
			return bson.D{{Key: "_id", Value: id}}, nil
		},
	})
}

func TestCursor_LimitZero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_LimitZero",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 5)
			for i := 0; i < 5; i++ {
				docs[i] = bson.D{{Key: "_id", Value: fmt.Sprintf("doc%d", i+1)}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetLimit(0).SetSort(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCursor_LimitNegative(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_LimitNegative",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 10)
			for i := 0; i < 10; i++ {
				docs[i] = bson.D{{Key: "_id", Value: fmt.Sprintf("doc%02d", i+1)}, {Key: "v", Value: int32(i + 1)}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "v", Value: 1}}).SetLimit(-3)
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCursor_HintByDocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_HintByDocument",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "score", Value: int32(20)}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "score", Value: int32(30)}},
			})
			if err != nil {
				return err
			}
			_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("score_1"),
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetHint(bson.D{{Key: "score", Value: 1}}).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_HintByName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_HintByName",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "score", Value: int32(10)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "score", Value: int32(20)}},
			})
			if err != nil {
				return err
			}
			_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("score_1"),
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetHint("score_1").
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_ReturnKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_ReturnKey",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "score", Value: int32(10)}, {Key: "name", Value: "alice"}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "score", Value: int32(20)}, {Key: "name", Value: "bob"}},
			})
			if err != nil {
				return err
			}
			_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("score_1"),
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetHint(bson.D{{Key: "score", Value: 1}}).
				SetReturnKey(true).
				SetSort(bson.D{{Key: "score", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_MinMaxBounds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_MinMaxBounds",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 10)
			for i := 0; i < 10; i++ {
				docs[i] = bson.D{{Key: "_id", Value: fmt.Sprintf("doc%02d", i+1)}, {Key: "score", Value: int32((i + 1) * 10)}}
			}
			_, err := col.InsertMany(ctx, docs)
			if err != nil {
				return err
			}
			_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: 1}},
				Options: options.Index().SetName("score_1"),
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetHint(bson.D{{Key: "score", Value: 1}}).
				SetMin(bson.D{{Key: "score", Value: int32(30)}}).
				SetMax(bson.D{{Key: "score", Value: int32(60)}}).
				SetSort(bson.D{{Key: "score", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "score", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_CollationCaseInsensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_CollationCaseInsensitive",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "name", Value: "Alice"}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "name", Value: "bob"}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "name", Value: "CHARLIE"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			caseInsensitive := &options.Collation{Locale: "en", Strength: 2}
			opts := options.Find().
				SetCollation(caseInsensitive).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{{Key: "name", Value: "alice"}}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_CollationSort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_CollationSort",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "word", Value: "banana"}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "word", Value: "Apple"}},
				bson.D{{Key: "_id", Value: "c"}, {Key: "word", Value: "cherry"}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			enCollation := &options.Collation{Locale: "en", Strength: 1}
			opts := options.Find().
				SetCollation(enCollation).
				SetSort(bson.D{{Key: "word", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "word", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_MultiBatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_MultiBatch",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 25)
			for i := 0; i < 25; i++ {
				docs[i] = bson.D{{Key: "_id", Value: fmt.Sprintf("doc%02d", i+1)}, {Key: "seq", Value: int32(i + 1)}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetBatchSize(4)
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			defer cursor.Close(ctx)
			var count int32
			for cursor.Next(ctx) {
				count++
			}
			if err := cursor.Err(); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCursor_MultiBatchAllHelper(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_MultiBatchAllHelper",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 15)
			for i := 0; i < 15; i++ {
				docs[i] = bson.D{{Key: "_id", Value: fmt.Sprintf("doc%02d", i+1)}, {Key: "seq", Value: int32(i + 1)}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetBatchSize(3).SetSort(bson.D{{Key: "seq", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCursor_TailableCappedCollectionRequired(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_TailableCappedCollectionRequired",
		Support: harness.DumboDBMongoOnly,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "a"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// On a regular (non-capped) collection, tailable cursors return an error.
			_, err := col.Find(ctx, bson.D{}, options.Find().SetCursorType(options.Tailable))
			if err != nil {
				return bson.D{{Key: "error", Value: true}}, nil
			}
			return bson.D{{Key: "error", Value: false}}, nil
		},
	})
}

func TestCursor_NextIteration(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_NextIteration",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			defer cursor.Close(ctx)
			var ids []interface{}
			for cursor.Next(ctx) {
				var doc bson.D
				if err := cursor.Decode(&doc); err != nil {
					return nil, err
				}
				for _, e := range doc {
					if e.Key == "_id" {
						ids = append(ids, e.Value)
					}
				}
			}
			return bson.D{{Key: "ids", Value: ids}}, cursor.Err()
		},
	})
}

func TestCursor_CloseReleasesResources(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_CloseReleasesResources",
		Support: harness.DumboDBFull,
		Setup:   insertCursorSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			// Read first doc then close early.
			_ = cursor.Next(ctx)
			closeErr := cursor.Close(ctx)
			// After close, Next() must return false.
			hasMore := cursor.Next(ctx)
			return bson.D{
				{Key: "closeErr", Value: closeErr == nil},
				{Key: "hasMoreAfterClose", Value: hasMore},
			}, nil
		},
	})
}

func TestCursor_AllowPartialResultsTrue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_AllowPartialResultsTrue",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "a"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "b"}, {Key: "v", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetAllowPartialResults(true).
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestCursor_AllowPartialResultsFalse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_AllowPartialResultsFalse",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "x"}, {Key: "v", Value: int32(42)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetAllowPartialResults(false).
				SetProjection(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestCursor_SortLimitSkipCombined(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_SortLimitSkipCombined",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 20)
			for i := 0; i < 20; i++ {
				docs[i] = bson.D{{Key: "_id", Value: fmt.Sprintf("doc%02d", i+1)}, {Key: "seq", Value: int32(i + 1)}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Page 2: items 6-10 when sorted ascending.
			opts := options.Find().
				SetSort(bson.D{{Key: "seq", Value: 1}}).
				SetSkip(5).
				SetLimit(5).
				SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "seq", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}
