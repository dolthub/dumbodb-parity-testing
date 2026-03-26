package tests

import (
	"context"
	"testing"

	"github.com/dolthub/dongo-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TestTransactionSmoke verifies the harness support-level contract.
//
// MongoDB 8 supports multi-document transactions. Dongo does not yet.
// This test is intentionally marked DongoFull so CI fails red — proving
// the harness correctly surfaces a real divergence.
//
// To verify the other direction: change DongoFull → DongoXFail and CI
// should go green (divergence recorded but not fatal).
func TestTransactionSmoke(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "transaction-smoke",
		Support: harness.DongoXFail, // DongoFull confirmed red; XFail = divergence recorded, CI green
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
