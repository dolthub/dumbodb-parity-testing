package tests

// Parity family for unique-index enforcement on the update path
// (behavior U2 of dumbodb docs/design/secondary-index-structural-sharing.md):
// an update that would change a doc's unique key to collide with
// another doc fails with a duplicate-key error and leaves both docs
// unchanged.

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func idxmUniqueSetup(ctx context.Context, col *mongo.Collection) error {
	docs := []interface{}{
		bson.D{{Key: "_id", Value: "u1"}, {Key: "f", Value: "alpha"}},
		bson.D{{Key: "_id", Value: "u2"}, {Key: "f", Value: "bravo"}},
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "f", Value: int32(1)}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

// idxmDupOutcome reduces an update error to a comparable shape: did it
// fail, and was it a duplicate-key failure (MongoDB error 11000 /
// "duplicate key" text).
func idxmDupOutcome(err error) bson.D {
	if err == nil {
		return bson.D{{Key: "failed", Value: false}, {Key: "dup", Value: false}}
	}
	dup := mongo.IsDuplicateKeyError(err) ||
		strings.Contains(strings.ToLower(err.Error()), "duplicate")
	return bson.D{{Key: "failed", Value: true}, {Key: "dup", Value: dup}}
}

func TestIndex_UniqueUpdate_SetCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UniqueUpdate_SetCollision",
		Support: harness.DumboDBFull,
		Setup:   idxmUniqueSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, updErr := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "bravo"}}}})
			out := idxmDupOutcome(updErr)

			// Both docs must be unchanged after the failed update.
			probes, err := idxmProbe(ctx, col, "alpha", bson.D{{Key: "f", Value: "alpha"}})
			if err != nil {
				return nil, err
			}
			out = append(out, probes...)
			probes, err = idxmProbe(ctx, col, "bravo", bson.D{{Key: "f", Value: "bravo"}})
			if err != nil {
				return nil, err
			}
			out = append(out, probes...)
			return out, nil
		},
	})
}

func TestIndex_UniqueUpdate_ReplaceCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UniqueUpdate_ReplaceCollision",
		Support: harness.DumboDBFull,
		Setup:   idxmUniqueSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, updErr := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "_id", Value: "u1"}, {Key: "f", Value: "bravo"}})
			out := idxmDupOutcome(updErr)
			probes, err := idxmProbe(ctx, col, "bravo", bson.D{{Key: "f", Value: "bravo"}})
			if err != nil {
				return nil, err
			}
			out = append(out, probes...)
			return out, nil
		},
	})
}

func TestIndex_UniqueUpdate_NonCollidingSucceeds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UniqueUpdate_NonCollidingSucceeds",
		Support: harness.DumboDBFull,
		Setup:   idxmUniqueSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Changing to a fresh value is fine; so is a same-value
			// rewrite of the doc that already owns the key.
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "charlie"}}}}); err != nil {
				return nil, err
			}
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u2"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "bravo"}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"charlie", bson.D{{Key: "f", Value: "charlie"}}},
				{"bravo", bson.D{{Key: "f", Value: "bravo"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}
