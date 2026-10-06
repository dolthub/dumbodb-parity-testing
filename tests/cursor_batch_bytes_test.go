package tests

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A batch stops before the document that would take it past 16MiB, so 600KB
// documents come back 27 to a batch regardless of the requested batch size.
const (
	largeCursorDocCount = 120
	largeCursorDocBytes = 600 * 1024
)

func insertLargeCursorDocs(ctx context.Context, col *mongo.Collection) error {
	pad := strings.Repeat("x", largeCursorDocBytes)
	docs := make([]interface{}, largeCursorDocCount)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "pad", Value: pad}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func insertSmallCursorDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 1000)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

// batchShape runs open against a monitored client, drains the cursor, and
// reports the size of every batch.
func batchShape(ctx context.Context, col *mongo.Collection, firstCommand string,
	open func(*mongo.Collection) (*mongo.Cursor, error)) (interface{}, error) {
	recorder := newStartedCommandRecorder()
	client, err := monitoredClient(ctx, harness.ServerURI(ctx), recorder)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Disconnect(ctx) }()
	cursor, err := open(client.Database(col.Database().Name()).Collection(col.Name()))
	if err != nil {
		return nil, err
	}
	documents := 0
	for cursor.Next(ctx) {
		documents++
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return bson.D{
		{Key: "firstBatchCount", Value: cursorBatchLength(recorder.lastReply(firstCommand), "firstBatch")},
		{Key: "nextBatchCounts", Value: cursorBatchLengths(recorder.repliesFor("getMore"), "nextBatch")},
		{Key: "documentCount", Value: int32(documents)},
	}, nil
}

func TestCursor_find_largeDocs_batchesCappedAt16MiB(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_large_docs_batches_capped_at_16MiB",
		Support: harness.DumboDBFull,
		Setup:   insertLargeCursorDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return batchShape(ctx, col, "find", func(c *mongo.Collection) (*mongo.Cursor, error) {
				return c.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			})
		},
	})
}

func TestCursor_aggregate_largeDocs_batchesCappedAt16MiB(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_aggregate_large_docs_batches_capped_at_16MiB",
		Support: harness.DumboDBFull,
		Setup:   insertLargeCursorDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return batchShape(ctx, col, "aggregate", func(c *mongo.Collection) (*mongo.Cursor, error) {
				return c.Aggregate(ctx, mongo.Pipeline{{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}})
			})
		},
	})
}

func TestCursor_find_largeDocs_sizeCapBeatsBatchSize(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_find_large_docs_size_cap_beats_batchSize",
		Support: harness.DumboDBFull,
		Setup:   insertLargeCursorDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return batchShape(ctx, col, "find", func(c *mongo.Collection) (*mongo.Cursor, error) {
				return c.Find(ctx, bson.D{}, options.Find().SetBatchSize(50).SetSort(bson.D{{Key: "_id", Value: 1}}))
			})
		},
	})
}

func TestCursor_getMore_noBatchSize_hasNoCountLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Cursor_getMore_no_batchSize_has_no_count_limit",
		Support: harness.DumboDBFull,
		Setup:   insertSmallCursorDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			var found bson.Raw
			if err := db.RunCommand(ctx, bson.D{
				{Key: "find", Value: col.Name()}, {Key: "sort", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "batchSize", Value: int32(1)},
			}).Decode(&found); err != nil {
				return nil, err
			}
			var more bson.Raw
			if err := db.RunCommand(ctx, bson.D{{Key: "getMore", Value: found.Lookup("cursor", "id").Int64()}, {Key: "collection", Value: col.Name()}}).Decode(&more); err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "firstBatchCount", Value: cursorBatchLength(found, "firstBatch")},
				{Key: "nextBatchCount", Value: cursorBatchLength(more, "nextBatch")},
				{Key: "exhausted", Value: more.Lookup("cursor", "id").Int64() == 0},
			}, nil
		},
	})
}
