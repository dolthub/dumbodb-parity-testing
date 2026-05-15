package tests

import (
	"context"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestTransactionSmoke verifies basic startTransaction / insert / commitTransaction.
func TestTransactionSmoke(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "transaction-smoke",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			session, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer session.EndSession(ctx)

			var insertedID interface{}
			err = mongo.WithSession(ctx, session, func(sc mongo.SessionContext) error {
				if err := session.StartTransaction(); err != nil {
					return err
				}
				res, err := col.InsertOne(sc, bson.D{{Key: "smoke", Value: true}})
				if err != nil {
					_ = session.AbortTransaction(sc)
					return err
				}
				insertedID = res.InsertedID
				return session.CommitTransaction(sc)
			})
			return insertedID, err
		},
	})
}
