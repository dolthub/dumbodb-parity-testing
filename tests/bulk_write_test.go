package tests

// bulk_write_test.go covers the MongoDB bulkWrite command (pa-jkc).
// These start as DumboDBXFail and will be flipped to DumboDBFull once the
// dongo rig implements the wire-level bulkWrite command.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestBulkWriteCmd_mixed_ops exercises a bulkWrite with one of each core op
// (insertOne, updateOne, deleteOne) and compares the returned counts.
func TestBulkWriteCmd_mixed_ops(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteCmd_mixed_ops",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "bwc1"}, {Key: "x", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "bwc2"}, {Key: "x", Value: int32(2)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(
					bson.D{{Key: "_id", Value: "bwc3"}, {Key: "x", Value: int32(3)}}),
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "bwc1"}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: int32(10)}}}}),
				mongo.NewDeleteOneModel().SetFilter(bson.D{{Key: "_id", Value: "bwc2"}}),
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

// TestBulkWriteCmd_ordered_stops_on_error verifies that an ordered bulkWrite
// halts at the first failing op: the subsequent insert must not land.
func TestBulkWriteCmd_ordered_stops_on_error(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteCmd_ordered_stops_on_error",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "existing"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "ord-1"}}),
				// Duplicate key — ordered:true should halt here.
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "existing"}}),
				// Should NOT land because ordered:true halted above.
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "ord-3"}}),
			}
			_, _ = col.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(true))
			// Ignore the bulk write error and report what actually landed.
			cnt, err := col.CountDocuments(ctx, bson.D{
				{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{"ord-1", "ord-3"}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "landedAfterError", Value: cnt}}, nil
		},
	})
}

// TestBulkWriteCmd_unordered_continues_past_errors verifies that an unordered
// bulkWrite runs every op, so both surrounding inserts land despite the middle
// op failing with a duplicate-key error.
func TestBulkWriteCmd_unordered_continues_past_errors(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteCmd_unordered_continues_past_errors",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "existing"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "un-1"}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "existing"}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "un-3"}}),
			}
			_, _ = col.BulkWrite(ctx, models, options.BulkWrite().SetOrdered(false))
			cnt, err := col.CountDocuments(ctx, bson.D{
				{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{"un-1", "un-3"}}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "landedAfterError", Value: cnt}}, nil
		},
	})
}

// TestBulkWriteCmd_upsert exercises upsert inside bulkWrite and checks that
// upsertedCount reflects the creation.
func TestBulkWriteCmd_upsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteCmd_upsert",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "up-1"}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: "created"}}}}).
					SetUpsert(true),
			}
			res, err := col.BulkWrite(ctx, models)
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

// TestBulkWriteCmd_return_counts exercises the full return shape — every
// counter field that bulkWrite exposes, in one run.
func TestBulkWriteCmd_return_counts(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteCmd_return_counts",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "rc1"}, {Key: "v", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "rc2"}, {Key: "v", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "rc3"}, {Key: "v", Value: int32(3)}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			models := []mongo.WriteModel{
				// +1 insertedCount
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "rc4"}}),
				// +1 matched, +1 modified
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "rc1"}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(11)}}}}),
				// +1 deletedCount
				mongo.NewDeleteOneModel().SetFilter(bson.D{{Key: "_id", Value: "rc2"}}),
				// +1 upsertedCount
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: "rc-upsert"}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: "new"}}}}).
					SetUpsert(true),
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
				{Key: "upsertedCount", Value: res.UpsertedCount},
			}, nil
		},
	})
}

// TestBulkWriteCmd_nonexistent_collection verifies that bulkWrite auto-creates
// the target collection when its first op is an insert.
func TestBulkWriteCmd_nonexistent_collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BulkWriteCmd_nonexistent_collection",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Use a fresh sibling collection name that has not been created.
			fresh := col.Database().Collection("bwc_fresh_" + col.Name())
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "fresh-1"}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "fresh-2"}}),
			}
			res, err := fresh.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			cnt, err := fresh.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "insertedCount", Value: res.InsertedCount},
				{Key: "docsInCollection", Value: cnt},
			}, nil
		},
	})
}
