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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestUpsertEqualities: an upsert seeds a path from every equality of its
// filter value, as mongod extracts them: each $eq, the element of a
// one-element $in and each element of $all that is not a regex or
// $elemMatch, beside other operators too; two equalities on a path fail with
// 54. A replacement refuses any such _id sub-path with 111, and a replacement
// whose _id is an array fails with 54.
func TestUpsertEqualities(t *testing.T) {
	regex := primitive.Regex{Pattern: "q"}
	filters := []bson.D{
		{{Key: "_id.x", Value: bson.D{{Key: "$eq", Value: int32(1)}, {Key: "$gt", Value: int32(0)}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$gt", Value: int32(0)}, {Key: "$eq", Value: int32(1)}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$in", Value: bson.A{int32(1)}}, {Key: "$gt", Value: int32(0)}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$eq", Value: int32(1)}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$all", Value: bson.A{int32(1)}}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$all", Value: bson.A{int32(1), int32(2)}}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$all", Value: bson.A{bson.A{int32(1)}}}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$all", Value: bson.A{regex}}}}},
		{{Key: "_id.x", Value: bson.D{{Key: "$gt", Value: int32(0)}, {Key: "$lt", Value: int32(5)}}}},
		{{Key: "x", Value: bson.D{{Key: "$all", Value: bson.A{int32(1), int32(2)}}}}},
		{{Key: "x", Value: bson.D{{Key: "$eq", Value: int32(1)}, {Key: "$gt", Value: int32(0)}}}},
		{{Key: "x", Value: bson.D{{Key: "$all", Value: bson.A{bson.A{int32(1)}}}}}},
		{{Key: "x", Value: bson.D{{Key: "$all", Value: bson.A{bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "a", Value: int32(1)}}}}}}}}},
		{{Key: "x", Value: bson.D{{Key: "$eq", Value: int32(1)}, {Key: "$in", Value: bson.A{int32(2)}}}}},
		{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{bson.A{int32(5)}}}}}},
		{{Key: "_id", Value: bson.A{int32(5)}}},
		{{Key: "_id", Value: bson.D{{Key: "$eq", Value: bson.A{int32(5)}}}}},
	}
	updates := map[string]interface{}{
		"replacement": bson.D{{Key: "b", Value: int32(1)}},
		"modifier":    bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: int32(1)}}}},
	}
	for i, filter := range filters {
		for kind, update := range updates {
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("UpsertEqualities_%d_%s", i, kind),
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var err error
					if kind == "replacement" {
						_, err = col.ReplaceOne(ctx, filter, update, options.Replace().SetUpsert(true))
					} else {
						_, err = col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
					}
					if err != nil {
						return errOutcome(err), nil
					}
					cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$cond", Value: bson.A{
						bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$_id"}}, "objectId"}}}, "generated", "$_id"}}}}}}}})
					return drainCursor(ctx, cursor, err)
				},
			})
		}
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "UpsertEqualities_replacement_array_id",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.ReplaceOne(ctx, bson.D{{Key: "a", Value: int32(1)}}, bson.D{{Key: "_id", Value: bson.A{int32(5)}}}, options.Replace().SetUpsert(true))
			return errOutcome(err), nil
		},
	})
}
