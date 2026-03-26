package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dongo-parity-testing/harness"
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

// --- InsertOne ---

func TestInsertOne_acknowledged(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertOne_acknowledged",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// --- InsertMany ---

func TestInsertMany_ordered(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "InsertMany_ordered",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// --- FindOne ---

func TestFindOne_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindOne_match",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// --- Find ---

func TestFind_all_sorted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Find_all_sorted",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// --- UpdateOne ---

func TestUpdateOne_set(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_set",
		Support: harness.DongoFull,
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

func TestUpdateOne_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateOne_upsert",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
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

// --- UpdateMany ---

func TestUpdateMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UpdateMany",
		Support: harness.DongoFull,
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

// --- DeleteOne ---

func TestDeleteOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteOne",
		Support: harness.DongoFull,
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
		Support: harness.DongoFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteOne(ctx, bson.D{{Key: "_id", Value: "ghost"}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deletedCount", Value: res.DeletedCount}}, nil
		},
	})
}

// --- DeleteMany ---

func TestDeleteMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "DeleteMany",
		Support: harness.DongoFull,
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

// --- ReplaceOne ---

func TestReplaceOne(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ReplaceOne",
		Support: harness.DongoFull,
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

// --- BulkWrite (collection-level) ---

func TestBulkWrite_mixed_ops(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWrite_mixed_ops",
		Support: harness.DongoFull,
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
