package tests

// write_concern_test.go covers writeConcern acks on CRUD ops and the deprecated
// getLastError command (pa-jkc). getLastError was removed in MongoDB 5.1, so
// both servers answer CommandNotFound. The writeConcern and getLastError cases
// each run on a dedicated client (freshCol) so their per-connection session
// state neither leaks onto nor is polluted by the shared pool -- without that
// isolation they flake under the full test order.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// wcCollection returns a new collection handle on the same DB+name with the
// given write concern attached. The Go driver attaches write concerns at the
// collection/database level, not per-operation.
func wcCollection(col *mongo.Collection, wc *writeconcern.WriteConcern) *mongo.Collection {
	return col.Database().Collection(col.Name(), options.Collection().SetWriteConcern(wc))
}

// freshCol dials a dedicated client to the same server and returns a handle on
// the same DB+collection. Running on its own connection pool keeps a case's
// session state off the shared pool -- required so the writeConcern and
// getLastError cases stay order-independent.
func freshCol(ctx context.Context, col *mongo.Collection) (*mongo.Collection, func(), error) {
	c, closeFn, err := secondClient(ctx)
	if err != nil {
		return nil, nil, err
	}
	return c.Database(col.Database().Name()).Collection(col.Name()), closeFn, nil
}

// insertAndReadBack inserts `doc` via colWC and reads the result back using
// the original unrestricted handle so the read is always acknowledged. The
// returned result captures whether the write landed, regardless of the write
// concern's ack semantics.
func insertAndReadBack(ctx context.Context, colWC, readCol *mongo.Collection, doc bson.D, filter bson.D) (interface{}, error) {
	if _, err := colWC.InsertOne(ctx, doc); err != nil {
		return nil, err
	}
	var back bson.D
	if err := readCol.FindOne(ctx, filter).Decode(&back); err != nil {
		return nil, err
	}
	return back, nil
}

func TestWriteConcern_insert_w1_nojournal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "WriteConcern_insert_w1_nojournal",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			wc := writeconcern.New(writeconcern.W(1), writeconcern.J(false))
			return insertAndReadBack(ctx, wcCollection(col, wc), col,
				bson.D{{Key: "_id", Value: "wc-w1-nj"}, {Key: "v", Value: "hello"}},
				bson.D{{Key: "_id", Value: "wc-w1-nj"}})
		},
	})
}

func TestWriteConcern_insert_w1_journal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "WriteConcern_insert_w1_journal",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			wc := writeconcern.New(writeconcern.W(1), writeconcern.J(true))
			return insertAndReadBack(ctx, wcCollection(col, wc), col,
				bson.D{{Key: "_id", Value: "wc-w1-j"}, {Key: "v", Value: "hello"}},
				bson.D{{Key: "_id", Value: "wc-w1-j"}})
		},
	})
}

// w:0 is fire-and-forget: the driver issues an unacknowledged write op that
// returns immediately. We still expect the document to land eventually, so the
// read-back confirms persistence.
func TestWriteConcern_insert_w0_fire_and_forget(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "WriteConcern_insert_w0_fire_and_forget",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			wc := writeconcern.New(writeconcern.W(0))
			colWC := wcCollection(col, wc)
			if _, err := colWC.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "wc-w0"},
				{Key: "v", Value: "hello"},
			}); err != nil {
				return nil, err
			}
			// Read-back with the ack'd handle so we don't race the insert.
			var back bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "wc-w0"}}).Decode(&back)
			return back, err
		},
	})
}

func TestWriteConcern_update_w1(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "WriteConcern_update_w1",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "u1"}, {Key: "v", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			base, closeFn, err := freshCol(ctx, col)
			if err != nil {
				return nil, err
			}
			defer closeFn()
			wc := writeconcern.New(writeconcern.W(1), writeconcern.J(true))
			colWC := wcCollection(base, wc)
			res, err := colWC.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: int32(2)}}}})
			if err != nil {
				return nil, err
			}
			var back bson.D
			if err := base.FindOne(ctx, bson.D{{Key: "_id", Value: "u1"}}).Decode(&back); err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "matchedCount", Value: res.MatchedCount},
				{Key: "modifiedCount", Value: res.ModifiedCount},
				{Key: "doc", Value: back},
			}, nil
		},
	})
}

func TestWriteConcern_delete_w1(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "WriteConcern_delete_w1",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "d1"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			base, closeFn, err := freshCol(ctx, col)
			if err != nil {
				return nil, err
			}
			defer closeFn()
			wc := writeconcern.New(writeconcern.W(1), writeconcern.J(true))
			colWC := wcCollection(base, wc)
			res, err := colWC.DeleteOne(ctx, bson.D{{Key: "_id", Value: "d1"}})
			if err != nil {
				return nil, err
			}
			cnt, err := base.CountDocuments(ctx, bson.D{{Key: "_id", Value: "d1"}})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "deletedCount", Value: res.DeletedCount},
				{Key: "remaining", Value: cnt},
			}, nil
		},
	})
}

func TestWriteConcern_bulkWrite_w1(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "WriteConcern_bulkWrite_w1",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			base, closeFn, err := freshCol(ctx, col)
			if err != nil {
				return nil, err
			}
			defer closeFn()
			wc := writeconcern.New(writeconcern.W(1), writeconcern.J(false))
			colWC := wcCollection(base, wc)
			models := []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "bw-wc-1"}}),
				mongo.NewInsertOneModel().SetDocument(bson.D{{Key: "_id", Value: "bw-wc-2"}}),
			}
			res, err := colWC.BulkWrite(ctx, models)
			if err != nil {
				return nil, err
			}
			cnt, err := base.CountDocuments(ctx, bson.D{})
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

//
// getLastError was removed from MongoDB in 5.1. We still test it because the
// dongo rig is tracking wire-protocol coverage; on modern MongoDB the driver
// returns a "no such command" error, and the parity expectation is that
// DumboDB returns the same shape of error.

func TestGetLastError_after_insert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "GetLastError_after_insert",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			base, closeFn, err := freshCol(ctx, col)
			if err != nil {
				return nil, err
			}
			defer closeFn()
			if _, err := base.InsertOne(ctx, bson.D{{Key: "_id", Value: "gle-1"}}); err != nil {
				return nil, err
			}
			return runCommandDoc(ctx, base, bson.D{{Key: "getLastError", Value: 1}})
		},
	})
}

func TestGetLastError_with_j_true(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "GetLastError_with_j_true",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			base, closeFn, err := freshCol(ctx, col)
			if err != nil {
				return nil, err
			}
			defer closeFn()
			if _, err := base.InsertOne(ctx, bson.D{{Key: "_id", Value: "gle-j"}}); err != nil {
				return nil, err
			}
			return runCommandDoc(ctx, base, bson.D{
				{Key: "getLastError", Value: 1},
				{Key: "j", Value: true},
			})
		},
	})
}

// TestGetLastError_return_value picks out the stable fields of the response
// shape so we exercise the command's reply contract rather than
// implementation-specific trailers.
func TestGetLastError_return_value(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name: "GetLastError_return_value",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			base, closeFn, err := freshCol(ctx, col)
			if err != nil {
				return nil, err
			}
			defer closeFn()
			if _, err := base.InsertOne(ctx, bson.D{{Key: "_id", Value: "gle-ret"}}); err != nil {
				return nil, err
			}
			raw, err := runCommandDoc(ctx, base, bson.D{{Key: "getLastError", Value: 1}})
			if err != nil {
				return nil, err
			}
			doc, ok := raw.(bson.D)
			if !ok {
				return raw, nil
			}
			return pickFields(doc, "ok", "err", "n"), nil
		},
	})
}
