package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/dolthub/dongo-parity-testing/harness"
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

// ─── maxTimeMS ────────────────────────────────────────────────────────────────

func TestCursor_find_maxTimeMS(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_maxTimeMS",
		Support: harness.DongoFull,
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

// ─── batchSize ────────────────────────────────────────────────────────────────

func TestCursor_find_batchSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_batchSize",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── Cursor exhaustion ────────────────────────────────────────────────────────

func TestCursor_exhaustion_noDocsAfterAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_exhaustion_noDocsAfterAll",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── hint ─────────────────────────────────────────────────────────────────────

func TestCursor_find_hint_naturalOrder(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_hint_naturalOrder",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── comment ──────────────────────────────────────────────────────────────────

func TestCursor_find_comment(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_comment",
		Support: harness.DongoFull,
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

// ─── allowDiskUse ─────────────────────────────────────────────────────────────

func TestCursor_find_allowDiskUse(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_allowDiskUse",
		Support: harness.DongoFull,
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

// ─── readPreference ───────────────────────────────────────────────────────────

func TestCursor_find_readPreference_primary(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_readPreference_primary",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── Sort on multiple fields ──────────────────────────────────────────────────

func TestCursor_sort_multiField_groupThenVal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_sort_multiField_groupThenVal",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── Skip + Limit ─────────────────────────────────────────────────────────────

func TestCursor_skipLimit_page1(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_skipLimit_page1",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── Aggregate cursor options ─────────────────────────────────────────────────

func TestCursor_aggregate_batchSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_aggregate_batchSize",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// ─── FindOne options ──────────────────────────────────────────────────────────

func TestCursor_findOne_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_findOne_sort",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
