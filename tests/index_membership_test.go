// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

// Behaviors M1 and M2 of dumbodb
// docs/design/secondary-index-structural-sharing.md.

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
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "miss"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "bravo"}}}}); err != nil {
				return nil, err
			}
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

// A query that implies the partial condition uses the index (see the
// explain tests); this pins that the index-used path returns exactly the
// in-filter docs and nothing outside the partial set.
func TestIndex_Partial_ImpliesFilterReturnsSubset(t *testing.T) {
	partialOpts := options.Index().SetPartialFilterExpression(
		bson.D{{Key: "status", Value: "active"}})
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_Partial_ImpliesFilterReturnsSubset",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: "s1"}, {Key: "f", Value: "alpha"}, {Key: "status", Value: "active"}},
				bson.D{{Key: "_id", Value: "s2"}, {Key: "f", Value: "alpha"}, {Key: "status", Value: "inactive"}},
				bson.D{{Key: "_id", Value: "s3"}, {Key: "f", Value: "bravo"}, {Key: "status", Value: "active"}},
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
			cur, err := col.Find(ctx,
				bson.D{{Key: "f", Value: "alpha"}, {Key: "status", Value: "active"}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var got []bson.D
			if err := cur.All(ctx, &got); err != nil {
				return nil, err
			}
			return got, nil
		},
	})
}
