package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// seedDocs are shared seed documents with deterministic string _ids.
var seedDocs = []interface{}{
	bson.D{
		{Key: "_id", Value: "c1"},
		{Key: "name", Value: "Alice"},
		{Key: "score", Value: int32(10)},
		{Key: "tags", Value: bson.A{"go", "db"}},
	},
	bson.D{
		{Key: "_id", Value: "c2"},
		{Key: "name", Value: "Bob"},
		{Key: "score", Value: int32(20)},
		{Key: "tags", Value: bson.A{"go"}},
	},
	bson.D{
		{Key: "_id", Value: "c3"},
		{Key: "name", Value: "Carol"},
		{Key: "score", Value: int32(30)},
		{Key: "tags", Value: bson.A{"db", "sql"}},
	},
}

func insertSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, seedDocs)
	return err
}

func docsToSlice(docs []bson.D) []interface{} {
	out := make([]interface{}, len(docs))
	for i, d := range docs {
		out[i] = d
	}
	return out
}

func TestInsertOne_acknowledged(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertOne_acknowledged",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "name", Value: "Dave"},
				{Key: "score", Value: int32(42)},
			})
			if err != nil {
				return nil, err
			}
			// InsertedID is a non-deterministic ObjectID; signal success structurally.
			return bson.D{{Key: "ok", Value: int32(1)}}, nil
		},
	})
}

func TestInsertOne_explicit_id(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertOne_explicit_id",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "explicit-1"},
				{Key: "value", Value: "hello"},
			})
			if err != nil {
				return nil, err
			}
			// Read back to verify the document landed with the correct shape.
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "explicit-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestInsertOne_duplicate_key_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertOne_duplicate_key_error",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "dup-1"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "dup-1"}})
			return nil, err
		},
	})
}

func TestInsertOne_nested_doc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertOne_nested_doc",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "nested-1"},
				{Key: "address", Value: bson.D{
					{Key: "city", Value: "Seattle"},
					{Key: "zip", Value: "98101"},
				}},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "nested-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestInsertOne_array_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertOne_array_field",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "arr-1"},
				{Key: "nums", Value: bson.A{int32(1), int32(2), int32(3)}},
			})
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "arr-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestInsertMany_ordered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertMany_ordered",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "m3"}, {Key: "v", Value: int32(3)}},
			}
			res, err := col.InsertMany(ctx, docs)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(res.InsertedIDs))}}, nil
		},
	})
}

func TestInsertMany_unordered_partial_failure(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertMany_unordered_partial_failure",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "dup"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "new1"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "dup"}, {Key: "v", Value: int32(2)}},  // duplicate
				bson.D{{Key: "_id", Value: "new2"}, {Key: "v", Value: int32(3)}},
			}
			opts := options.InsertMany().SetOrdered(false)
			_, err := col.InsertMany(ctx, docs, opts)
			// Expect BulkWriteException; return error for comparison.
			return nil, err
		},
	})
}

func TestFindOne_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOne_match",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "name", Value: "Alice"}}).Decode(&result)
			return result, err
		},
	})
}

func TestFindOne_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOne_no_match",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "name", Value: "Zed"}}).Decode(&result)
			// mongo.ErrNoDocuments is the expected error from both servers.
			return nil, err
		},
	})
}

func TestFindOne_projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOne_projection",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().SetProjection(bson.D{
				{Key: "name", Value: 1},
				{Key: "_id", Value: 0},
			})
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}}, opts).Decode(&result)
			return result, err
		},
	})
}

func TestFindOne_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOne_sort",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOne().
				SetSort(bson.D{{Key: "score", Value: -1}}).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOne(ctx, bson.D{}, opts).Decode(&result)
			return result, err
		},
	})
}

func TestFind_all_sorted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Find_all_sorted",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestFind_filter_and_projection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Find_filter_and_projection",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "score", Value: 1}}).
				SetProjection(bson.D{
					{Key: "name", Value: 1},
					{Key: "score", Value: 1},
					{Key: "_id", Value: 0},
				})
			filter := bson.D{{Key: "score", Value: bson.D{{Key: "$gt", Value: int32(10)}}}}
			cursor, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestFind_limit_skip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Find_limit_skip",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetSkip(1).
				SetLimit(1).
				SetProjection(bson.D{
					{Key: "_id", Value: 1},
					{Key: "name", Value: 1},
				})
			cursor, err := col.Find(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			if err := cursor.All(ctx, &results); err != nil {
				return nil, err
			}
			return docsToSlice(results), nil
		},
	})
}

func TestFind_count_via_cursor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Find_count_via_cursor",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{})
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

func TestUpdateOne_set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_set",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(99)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_set_new_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_set_new_field",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "active", Value: true}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "active", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_set_nested(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_set_nested",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "nested-upd"},
				{Key: "meta", Value: bson.D{{Key: "rank", Value: int32(1)}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "nested-upd"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "meta.rank", Value: int32(42)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "nested-upd"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_upsert",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "new-doc"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "value", Value: "upserted"}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
				{Key: "upsertedCount", Value: res.UpsertedCount},
			}, nil
		},
	})
}

func TestUpdateOne_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_no_match",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "nonexistent"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: 1}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_inc_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_inc_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(5)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_inc_negative(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_inc_negative",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c3"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(-10)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c3"}},
				options.FindOne().SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_inc_and_set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_inc_and_set",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c2"}},
				bson.D{
					{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(1)}}},
					{Key: "$set", Value: bson.D{{Key: "name", Value: "Bobby"}}},
				},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c2"}},
				options.FindOne().SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_unset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_unset",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "tags", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_unset_nonexistent_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_unset_nonexistent_field",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "doesNotExist", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_multi_field_inc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_multi_field_inc",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "mf-1"},
				{Key: "a", Value: int32(1)},
				{Key: "b", Value: int32(2)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "mf-1"}},
				bson.D{{Key: "$inc", Value: bson.D{
					{Key: "a", Value: int32(10)},
					{Key: "b", Value: int32(20)},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "mf-1"}},
				options.FindOne().SetProjection(bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_mul_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_mul_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "score", Value: int32(3)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_mul_zero(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_mul_zero",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c2"}},
				bson.D{{Key: "$mul", Value: bson.D{{Key: "score", Value: int32(0)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c2"}},
				options.FindOne().SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_rename_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_rename_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$rename", Value: bson.D{{Key: "name", Value: "fullName"}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_rename_nonexistent(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_rename_nonexistent",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$rename", Value: bson.D{{Key: "ghost", Value: "spirit"}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_min_reduces(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_min_reduces",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c3"}},
				bson.D{{Key: "$min", Value: bson.D{{Key: "score", Value: int32(5)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c3"}},
				options.FindOne().SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_min_no_change(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_min_no_change",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				// 10 is not greater than existing 10, so no change
				bson.D{{Key: "$min", Value: bson.D{{Key: "score", Value: int32(100)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_max_increases(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_max_increases",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$max", Value: bson.D{{Key: "score", Value: int32(999)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_max_no_change(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_max_no_change",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c3"}},
				// 1 is less than existing 30, so no change
				bson.D{{Key: "$max", Value: bson.D{{Key: "score", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_setOnInsert_on_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_setOnInsert_on_upsert",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "soi-1"}},
				bson.D{
					{Key: "$set", Value: bson.D{{Key: "active", Value: true}}},
					{Key: "$setOnInsert", Value: bson.D{{Key: "createdAt", Value: "2026-01-01"}}},
				},
				opts,
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "soi-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_setOnInsert_no_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_setOnInsert_no_upsert",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// When matched (not inserted), $setOnInsert should be a no-op
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{
					{Key: "$set", Value: bson.D{{Key: "active", Value: true}}},
					{Key: "$setOnInsert", Value: bson.D{{Key: "shouldNotAppear", Value: "never"}}},
				},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_push_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_push_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: "new-tag"}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_push_each(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_push_each",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$push", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "$each", Value: bson.A{"rust", "python"}},
					}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_push_each_slice(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_push_each_slice",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Keep only last 2 elements after push
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$push", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "$each", Value: bson.A{"rust", "python"}},
						{Key: "$slice", Value: int32(-2)},
					}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_push_each_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_push_each_sort",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "sort-1"},
				{Key: "scores", Value: bson.A{int32(3), int32(1), int32(4)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "sort-1"}},
				bson.D{{Key: "$push", Value: bson.D{
					{Key: "scores", Value: bson.D{
						{Key: "$each", Value: bson.A{int32(2), int32(5)}},
						{Key: "$sort", Value: int32(1)},
					}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "sort-1"}},
				options.FindOne().SetProjection(bson.D{{Key: "scores", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_push_each_position(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_push_each_position",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Insert at position 0 (front)
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$push", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "$each", Value: bson.A{"first"}},
						{Key: "$position", Value: int32(0)},
					}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pop_last(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pop_last",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$pop", Value: bson.D{{Key: "tags", Value: int32(1)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pop_first(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pop_first",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$pop", Value: bson.D{{Key: "tags", Value: int32(-1)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pull_value(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pull_value",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$pull", Value: bson.D{{Key: "tags", Value: "go"}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pull_with_condition(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pull_with_condition",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "pull-cond"},
				{Key: "nums", Value: bson.A{int32(1), int32(5), int32(3), int32(8), int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "pull-cond"}},
				bson.D{{Key: "$pull", Value: bson.D{
					{Key: "nums", Value: bson.D{{Key: "$gt", Value: int32(4)}}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "pull-cond"}},
				options.FindOne().SetProjection(bson.D{{Key: "nums", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pullAll(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pullAll",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c3"}},
				bson.D{{Key: "$pullAll", Value: bson.D{{Key: "tags", Value: bson.A{"db", "sql"}}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c3"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_addToSet_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_addToSet_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: "newval"}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_addToSet_no_duplicate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_addToSet_no_duplicate",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// "go" already exists in c1.tags — should not be added again
			res, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: "go"}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_addToSet_each(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_addToSet_each",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$addToSet", Value: bson.D{
					{Key: "tags", Value: bson.D{
						{Key: "$each", Value: bson.A{"go", "rust", "python"}}, // "go" is duplicate
					}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "tags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_positional_first_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_positional_first_match",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "pos-1"},
				{Key: "scores", Value: bson.A{
					bson.D{{Key: "val", Value: int32(10)}},
					bson.D{{Key: "val", Value: int32(20)}},
					bson.D{{Key: "val", Value: int32(30)}},
				}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{
					{Key: "_id", Value: "pos-1"},
					{Key: "scores.val", Value: int32(20)},
				},
				bson.D{{Key: "$set", Value: bson.D{{Key: "scores.$.val", Value: int32(99)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "pos-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateMany_positional_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_positional_all",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "all-1"},
				{Key: "vals", Value: bson.A{int32(1), int32(2), int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $[] updates all elements of the array
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "all-1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "vals.$[]", Value: int32(10)}}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "all-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_arrayFilters(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_arrayFilters",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "af-1"},
				{Key: "vals", Value: bson.A{int32(1), int32(5), int32(8), int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetArrayFilters(options.ArrayFilters{
				Filters: []interface{}{
					bson.D{{Key: "elem", Value: bson.D{{Key: "$gte", Value: int32(5)}}}},
				},
			})
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "af-1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "vals.$[elem]", Value: int32(0)}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "af-1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pipeline_set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pipeline_set",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := []bson.D{
				{{Key: "$set", Value: bson.D{{Key: "doubled", Value: bson.D{{Key: "$multiply", Value: bson.A{"$score", 2}}}}}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}},
				options.FindOne().SetProjection(bson.D{{Key: "doubled", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pipeline_unset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pipeline_unset",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := []bson.D{
				{{Key: "$unset", Value: bson.A{"tags"}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c1"}}).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_pipeline_addFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_pipeline_addFields",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := []bson.D{
				{{Key: "$addFields", Value: bson.D{{Key: "level", Value: "standard"}}}},
			}
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c2"}},
				pipeline,
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "c2"}},
				options.FindOne().SetProjection(bson.D{{Key: "level", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateMany_pipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_pipeline",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := []bson.D{
				{{Key: "$set", Value: bson.D{{Key: "tier", Value: "all"}}}},
			}
			res, err := col.UpdateMany(ctx, bson.D{}, pipeline)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateOne_bit_and(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_bit_and",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit-1"},
				{Key: "flags", Value: int32(0b1111)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit-1"}},
				bson.D{{Key: "$bit", Value: bson.D{
					{Key: "flags", Value: bson.D{{Key: "and", Value: int32(0b1010)}}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "bit-1"}},
				options.FindOne().SetProjection(bson.D{{Key: "flags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_bit_or(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_bit_or",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit-2"},
				{Key: "flags", Value: int32(0b0101)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit-2"}},
				bson.D{{Key: "$bit", Value: bson.D{
					{Key: "flags", Value: bson.D{{Key: "or", Value: int32(0b1010)}}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "bit-2"}},
				options.FindOne().SetProjection(bson.D{{Key: "flags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateOne_bit_xor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_bit_xor",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "bit-3"},
				{Key: "flags", Value: int32(0b1100)},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "bit-3"}},
				bson.D{{Key: "$bit", Value: bson.D{
					{Key: "flags", Value: bson.D{{Key: "xor", Value: int32(0b1010)}}},
				}}},
			)
			if err != nil {
				return nil, err
			}
			var result bson.D
			err = col.FindOne(ctx, bson.D{{Key: "_id", Value: "bit-3"}},
				options.FindOne().SetProjection(bson.D{{Key: "flags", Value: 1}, {Key: "_id", Value: 0}})).Decode(&result)
			return result, err
		},
	})
}

func TestUpdateMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(20)}}}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(5)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateMany_unset(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_unset",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx,
				bson.D{},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "tags", Value: ""}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestUpdateMany_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany_upsert",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Update().SetUpsert(true)
			res, err := col.UpdateMany(ctx,
				bson.D{{Key: "_id", Value: "upsert-many"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(1)}}}},
				opts,
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "upsertedCount", Value: res.UpsertedCount},
			}, nil
		},
	})
}

func TestFindOneAndUpdate_returnBefore(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_returnBefore",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndUpdate().
				SetReturnDocument(options.Before).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "score", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "score", Value: int32(99)}}}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndUpdate_returnAfter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndUpdate_returnAfter",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndUpdate().
				SetReturnDocument(options.After).
				SetProjection(bson.D{{Key: "score", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "c2"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(100)}}}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndReplace_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndReplace_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndReplace().
				SetReturnDocument(options.Before).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndReplace(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "_id", Value: "c1"}, {Key: "name", Value: "Replaced"}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndReplace_returnAfter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndReplace_returnAfter",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndReplace().
				SetReturnDocument(options.After).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndReplace(ctx,
				bson.D{{Key: "_id", Value: "c2"}},
				bson.D{{Key: "_id", Value: "c2"}, {Key: "name", Value: "Swapped"}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndReplace_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndReplace_upsert",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndReplace().
				SetUpsert(true).
				SetReturnDocument(options.After)
			var result bson.D
			err := col.FindOneAndReplace(ctx,
				bson.D{{Key: "_id", Value: "for-upsert"}},
				bson.D{{Key: "_id", Value: "for-upsert"}, {Key: "val", Value: "inserted"}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndReplace_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndReplace_no_match",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOneAndReplace(ctx,
				bson.D{{Key: "_id", Value: "ghost"}},
				bson.D{{Key: "_id", Value: "ghost"}, {Key: "x", Value: 1}},
			).Decode(&result)
			return nil, err // expect ErrNoDocuments
		},
	})
}

func TestFindOneAndDelete_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndDelete_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.FindOneAndDelete().
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndDelete(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				opts,
			).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndDelete_sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndDelete_sort",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Delete the doc with the highest score
			opts := options.FindOneAndDelete().
				SetSort(bson.D{{Key: "score", Value: -1}}).
				SetProjection(bson.D{{Key: "name", Value: 1}, {Key: "_id", Value: 0}})
			var result bson.D
			err := col.FindOneAndDelete(ctx, bson.D{}, opts).Decode(&result)
			return result, err
		},
	})
}

func TestFindOneAndDelete_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOneAndDelete_no_match",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var result bson.D
			err := col.FindOneAndDelete(ctx,
				bson.D{{Key: "_id", Value: "ghost"}},
			).Decode(&result)
			return nil, err // expect ErrNoDocuments
		},
	})
}

func TestCountDocuments_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CountDocuments_all",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCountDocuments_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CountDocuments_filter",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx,
				bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(20)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCountDocuments_empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CountDocuments_empty",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCountDocuments_skip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CountDocuments_skip",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Count().SetSkip(1)
			count, err := col.CountDocuments(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCountDocuments_limit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CountDocuments_limit",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Count().SetLimit(2)
			count, err := col.CountDocuments(ctx, bson.D{}, opts)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestCountDocuments_nested_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "CountDocuments_nested_filter",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "nf-1"}, {Key: "info", Value: bson.D{{Key: "active", Value: true}}}},
				bson.D{{Key: "_id", Value: "nf-2"}, {Key: "info", Value: bson.D{{Key: "active", Value: false}}}},
				bson.D{{Key: "_id", Value: "nf-3"}, {Key: "info", Value: bson.D{{Key: "active", Value: true}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.CountDocuments(ctx,
				bson.D{{Key: "info.active", Value: true}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestEstimatedDocumentCount_basic(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "EstimatedDocumentCount_basic",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.EstimatedDocumentCount(ctx)
			if err != nil {
				return nil, err
			}
			// Exact count may vary by implementation; just verify it's non-zero
			if count > 0 {
				return bson.D{{Key: "nonzero", Value: true}}, nil
			}
			return bson.D{{Key: "nonzero", Value: false}}, nil
		},
	})
}

func TestEstimatedDocumentCount_empty(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "EstimatedDocumentCount_empty",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			count, err := col.EstimatedDocumentCount(ctx)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: count}}, nil
		},
	})
}

func TestDistinct_string_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_string_field",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := col.Distinct(ctx, "name", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestDistinct_nested_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_nested_field",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "d1"}, {Key: "addr", Value: bson.D{{Key: "city", Value: "NYC"}}}},
				bson.D{{Key: "_id", Value: "d2"}, {Key: "addr", Value: bson.D{{Key: "city", Value: "LA"}}}},
				bson.D{{Key: "_id", Value: "d3"}, {Key: "addr", Value: bson.D{{Key: "city", Value: "NYC"}}}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := col.Distinct(ctx, "addr.city", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestDistinct_array_field(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_array_field",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Distinct on an array field returns individual elements
			results, err := col.Distinct(ctx, "tags", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestDistinct_with_filter(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_with_filter",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := col.Distinct(ctx, "name",
				bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(20)}}}},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestDistinct_no_results(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Distinct_no_results",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := col.Distinct(ctx, "name", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "count", Value: int32(len(results))}}, nil
		},
	})
}

func TestDeleteOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteOne",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: "c1"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deletedCount", Value: res.DeletedCount}}, nil
		},
	})
}

func TestDeleteOne_no_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteOne_no_match",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: "ghost"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deletedCount", Value: res.DeletedCount}}, nil
		},
	})
}

func TestDeleteMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteMany",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			filter := bson.D{{Key: "score", Value: bson.D{{Key: "$lte", Value: int32(20)}}}}
			res, err := col.DeleteMany(ctx, filter)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deletedCount", Value: res.DeletedCount}}, nil
		},
	})
}

func TestDeleteMany_all(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteMany_all",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteMany(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deletedCount", Value: res.DeletedCount}}, nil
		},
	})
}

func TestReplaceOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ReplaceOne",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "c2"}},
				bson.D{
					{Key: "_id", Value: "c2"},
					{Key: "name", Value: "Robert"},
					{Key: "score", Value: int32(99)},
				},
			)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestBulkWrite_mixed_ops(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_mixed_ops",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bw1"}, {Key: "x", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "bw2"}, {Key: "x", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(
					bson.D{{Key: "_id", Value: "bw3"}, {Key: "x", Value: int32(3)}},
				),
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "bw1"}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(10)}}}}),
				mongo.NewDeleteOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "bw2"}}),
			}
			res, err := col.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "insertedCount", Value: res.InsertedCount},
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
				{Key: "deletedCount", Value: res.DeletedCount},
			}, nil
		},
	})
}

func TestBulkWrite_all_inserts(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_all_inserts",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "ai-1"}, {Key: "v", Value: int32(1)}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "ai-2"}, {Key: "v", Value: int32(2)}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "ai-3"}, {Key: "v", Value: int32(3)}}),
			}
			res, err := col.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "insertedCount", Value: res.InsertedCount}}, nil
		},
	})
}

func TestBulkWrite_unordered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_unordered",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.BulkWrite().SetOrdered(false)
			models := []mongo.WriteModel{
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "c1"}}).
					SetUpdate(bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(5)}}}}),
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "c2"}}).
					SetUpdate(bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(5)}}}}),
				mongo.NewDeleteOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "c3"}}),
			}
			res, err := col.BulkWrite(ctx, models, opts)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
				{Key: "deletedCount", Value: res.DeletedCount},
			}, nil
		},
	})
}

func TestBulkWrite_unordered_error_handling(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_unordered_error_handling",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "existing"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.BulkWrite().SetOrdered(false)
			models := []mongo.WriteModel{
				// This will succeed
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "new-doc"}}),
				// This will fail (duplicate key)
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "existing"}}),
				// With unordered, this should still run despite the error above
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "another-new"}}),
			}
			_, err := col.BulkWrite(ctx, models, opts)
			// Return the error type info for comparison
			return nil, err
		},
	})
}

func TestBulkWrite_replace_model(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_replace_model",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewReplaceOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "c1"}}).
					SetReplacement(bson.D{{Key: "_id", Value: "c1"}, {Key: "name", Value: "Replaced"}}),
			}
			res, err := col.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestBulkWrite_update_many_model(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_update_many_model",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewUpdateManyModel().
					SetFilter(bson.D{{Key: "score", Value: bson.D{{Key: "$gte", Value: int32(20)}}}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "tier", Value: "high"}}}}),
			}
			res, err := col.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
			}, nil
		},
	})
}

func TestBulkWrite_delete_many_model(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_delete_many_model",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewDeleteManyModel().
					SetFilter(bson.D{{Key: "score", Value: bson.D{{Key: "$lt", Value: int32(25)}}}}),
			}
			res, err := col.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deletedCount", Value: res.DeletedCount}}, nil
		},
	})
}

func TestBulkWrite_upsert_model(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_upsert_model",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "bw-upsert"}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "val", Value: "created"}}}}).
					SetUpsert(true),
			}
			res, err := col.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "upsertedCount", Value: res.UpsertedCount}}, nil
		},
	})
}

func TestUpdateOne_inc_on_string_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_inc_on_string_error",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $inc on a string field should error
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "name", Value: int32(1)}}}},
			)
			return nil, err // expect error
		},
	})
}

func TestUpdateOne_set_on_id_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_set_on_id_error",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// $set on _id to change its value should error
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "_id", Value: "new-id"}}}},
			)
			return nil, err // expect error
		},
	})
}

func TestUpdateOne_invalid_operator_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_invalid_operator_error",
		Support: harness.DumboDBFull,
		Setup:   insertSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "c1"}},
				bson.D{{Key: "$unknownOp", Value: bson.D{{Key: "score", Value: int32(1)}}}},
			)
			return nil, err // expect error
		},
	})
}
