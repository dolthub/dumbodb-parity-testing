package tests

// Parity families for sparse / partial index membership (behaviors M1
// and M2 of dumbodb docs/design/secondary-index-structural-sharing.md):
// query results and unique coexistence are MongoDB-defined; the
// stored-content halves are dumbodb-only tests.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestIndex_Sparse_QueryAfterFieldTransitions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_QueryAfterFieldTransitions",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "has"}, {Key: "f", Value: "alpha"}},
				bson.D{{Key: "_id", Value: "miss"}, {Key: "other", Value: int32(1)}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "f", Value: int32(1)}},
				Options: options.Index().SetSparse(true),
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Field appears on the previously-missing doc...
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "miss"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "bravo"}}}}); err != nil {
				return nil, err
			}
			// ...and disappears from the doc that had it.
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "has"}},
				bson.D{{Key: "$unset", Value: bson.D{{Key: "f", Value: ""}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"bravo", bson.D{{Key: "f", Value: "bravo"}}},
				{"alpha", bson.D{{Key: "f", Value: "alpha"}}},
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

func TestIndex_Sparse_UniqueCoexistence(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Sparse_UniqueCoexistence",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "f", Value: int32(1)}},
				Options: options.Index().SetSparse(true).SetUnique(true),
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// Two docs both missing the sparse-unique field must coexist.
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "m1"}, {Key: "other", Value: int32(1)}},
				bson.D{{Key: "_id", Value: "m2"}, {Key: "other", Value: int32(2)}},
				bson.D{{Key: "_id", Value: "v1"}, {Key: "f", Value: "x"}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return nil, err
			}
			n, err := idxmCount(ctx, col, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "total", Value: n}}, nil
		},
	})
}

func TestIndex_Partial_MembershipTransition(t *testing.T) {
	partialOpts := options.Index().SetPartialFilterExpression(
		bson.D{{Key: "status", Value: "active"}})
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_MembershipTransition",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "a1"}, {Key: "f", Value: "alpha"}, {Key: "status", Value: "active"}},
				bson.D{{Key: "_id", Value: "i1"}, {Key: "f", Value: "bravo"}, {Key: "status", Value: "inactive"}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "f", Value: int32(1)}},
				Options: partialOpts,
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			// The design doc's motivating example: a member leaves the
			// filter; a non-member enters it.
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "a1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "inactive"}}}}); err != nil {
				return nil, err
			}
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "i1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "status", Value: "active"}}}}); err != nil {
				return nil, err
			}
			// Queries on f must return all matching docs regardless of
			// index membership (the partial index covers a subset; the
			// query planner must not lose the rest).
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"alpha", bson.D{{Key: "f", Value: "alpha"}}},
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
