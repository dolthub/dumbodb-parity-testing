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

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Multi-condition filters applied per array element: an $elemMatch
// projection, array filters, a positional update and a multi-condition
// filter beside $expr, each over several documents.
func TestFilterMultiConditionPaths(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 6)
		for d := 0; d < 6; d++ {
			arr := bson.A{}
			for j := 0; j < 5; j++ {
				arr = append(arr, bson.D{{Key: "a", Value: (j + d) % 3}, {Key: "b", Value: j - 2}, {Key: "c", Value: bson.A{j, d}}})
			}
			docs = append(docs, bson.D{{Key: "_id", Value: d}, {Key: "arr", Value: arr}, {Key: "v", Value: d % 4}})
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	byID := bson.D{{Key: "_id", Value: 1}}
	findAll := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(byID))
		return drainCursor(ctx, cur, err)
	}
	cases := map[string]func(ctx context.Context, col *mongo.Collection) (interface{}, error){
		"elem_match_projection": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(byID).SetProjection(bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch",
				Value: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: bson.D{{Key: "$gt", Value: -1}}}}}}}}))
			return drainCursor(ctx, cur, err)
		},
		"array_filters": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateMany(ctx, bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "arr.$[e].hit", Value: true}}}},
				options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{
					bson.D{{Key: "e.a", Value: 2}, {Key: "e.b", Value: bson.D{{Key: "$gte", Value: 0}}}},
				}})); err != nil {
				return errOutcome(err), nil
			}
			return findAll(ctx, col)
		},
		"positional": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateMany(ctx, bson.D{{Key: "arr.b", Value: bson.D{{Key: "$gt", Value: 0}, {Key: "$lt", Value: 2}}}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "arr.$.hit", Value: true}}}}); err != nil {
				return errOutcome(err), nil
			}
			return findAll(ctx, col)
		},
		"expr_beside_conditions": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{
				{Key: "v", Value: bson.D{{Key: "$lt", Value: 3}}},
				{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{bson.D{{Key: "$size", Value: "$arr"}}, "$v"}}}},
				{Key: "arr.b", Value: 2},
				{Key: "$or", Value: bson.A{bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: 2}}}}, bson.D{{Key: "v", Value: 0}}}},
			}, options.Find().SetSort(byID).SetProjection(bson.D{{Key: "_id", Value: 1}}))
			return drainCursor(ctx, cur, err)
		},
	}
	for name, run := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterMultiConditionPaths_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     run,
		})
	}
}
