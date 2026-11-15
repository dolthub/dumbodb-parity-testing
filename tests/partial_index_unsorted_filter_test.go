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

// A unique partial index whose filter compares an embedded document with
// unsorted keys covers the documents existing when it is built
// (dolthub/dumbodb#208). mongod compares embedded documents in field order,
// so these cases keep each document in the filter's order.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestPartialIndexUnsortedFilter(t *testing.T) {
	yx := bson.D{{Key: "y", Value: 1}, {Key: "x", Value: 1}}
	for _, filter := range []struct {
		name string
		pfe  bson.D
	}{
		{"literal", bson.D{{Key: "a", Value: yx}}},
		{"eq", bson.D{{Key: "a", Value: bson.D{{Key: "$eq", Value: yx}}}}},
		{"in", bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A{yx}}}}}},
		{"and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: yx}}}}}},
	} {
		opts := func() *options.IndexOptions {
			return options.Index().SetName("k_1u").SetUnique(true).SetPartialFilterExpression(filter.pfe)
		}
		t.Run(filter.name+"_insert_after_build", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "PartialIndexUnsortedFilter_" + filter.name + "_insert_after_build",
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "k", Value: 1}, {Key: "a", Value: yx}}); err != nil {
						return err
					}
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}}, Options: opts()})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 2}, {Key: "k", Value: 1}, {Key: "a", Value: yx}})
						return err
					})
				},
			})
		})
		t.Run(filter.name+"_build_over_duplicates", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "PartialIndexUnsortedFilter_" + filter.name + "_build_over_duplicates",
				Support: harness.DumboDBFull,
				Setup: insertDocs(
					bson.D{{Key: "_id", Value: 1}, {Key: "k", Value: 1}, {Key: "a", Value: yx}},
					bson.D{{Key: "_id", Value: 2}, {Key: "k", Value: 1}, {Key: "a", Value: yx}},
				),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}}, Options: opts()})
					return errCode(err), nil
				},
			})
		})
		t.Run(filter.name+"_update_into_filter", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "PartialIndexUnsortedFilter_" + filter.name + "_update_into_filter",
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: 1}, {Key: "k", Value: 1}, {Key: "a", Value: yx}},
						bson.D{{Key: "_id", Value: 2}, {Key: "k", Value: 1}},
					}); err != nil {
						return err
					}
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}}, Options: opts()})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: 2}}, bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: yx}}}})
						return err
					})
				},
			})
		})
	}
}
