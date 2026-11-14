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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $elemMatch filters naming the elements' _id, alone and inside $and, $or,
// $nor, a nested $elemMatch and $all, over many array elements, through
// find, count, update and delete.
func TestElemMatch_IDOverManyElements(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 30)
		for d := 0; d < 30; d++ {
			arr := bson.A{}
			for j := 0; j < 200; j++ {
				arr = append(arr, bson.D{
					{Key: "_id", Value: j},
					{Key: "v", Value: (j + d) % 50},
					{Key: "sub", Value: bson.A{bson.D{{Key: "_id", Value: d}, {Key: "w", Value: j % 7}}}},
				})
			}
			docs = append(docs, bson.D{{Key: "_id", Value: d}, {Key: "arr", Value: arr}})
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	wide := bson.D{{Key: "_id", Value: 199}, {Key: "v", Value: bson.D{{Key: "$lt", Value: 10}}}}
	for k := 0; k < 20; k++ {
		wide = append(wide, bson.E{Key: fmt.Sprintf("k%d", k), Value: nil})
	}
	elemMatch := func(cond interface{}) bson.D {
		return bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: cond}}}}
	}
	filters := map[string]bson.D{
		"id_and_value": elemMatch(bson.D{{Key: "_id", Value: 5}, {Key: "v", Value: 9}}),
		"wide":         elemMatch(wide),
		"missing_id":   elemMatch(bson.D{{Key: "_id", Value: -1}, {Key: "v", Value: 0}}),
		"and": elemMatch(bson.D{{Key: "$and", Value: bson.A{
			bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: 150}}}},
			bson.D{{Key: "v", Value: 3}},
		}}}),
		"or": elemMatch(bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "v", Value: 40}},
		}}}),
		"nor": elemMatch(bson.D{{Key: "$nor", Value: bson.A{
			bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: 199}}}},
			bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: 20}}}},
		}}}),
		"nested": elemMatch(bson.D{{Key: "_id", Value: 3}, {Key: "sub", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
			{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{4, 7}}}}, {Key: "w", Value: 3},
		}}}}}),
		"all": {{Key: "arr", Value: bson.D{{Key: "$all", Value: bson.A{
			bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "_id", Value: 0}, {Key: "v", Value: bson.D{{Key: "$lt", Value: 5}}}}}},
			bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "_id", Value: 100}}}},
		}}}}},
	}
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "ElemMatchIDOverManyElements_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				byID := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
				cur, err := col.Find(ctx, filter, byID)
				if err != nil {
					return errOutcome(err), nil
				}
				found, err := collectIDs(ctx, cur)
				if err != nil {
					return nil, err
				}
				n, err := col.CountDocuments(ctx, filter)
				if err != nil {
					return errOutcome(err), nil
				}
				upd, err := col.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: true}}}})
				if err != nil {
					return errOutcome(err), nil
				}
				cur, err = col.Find(ctx, bson.D{{Key: "hit", Value: true}}, byID)
				if err != nil {
					return nil, err
				}
				hits, err := collectIDs(ctx, cur)
				if err != nil {
					return nil, err
				}
				del, err := col.DeleteMany(ctx, filter)
				if err != nil {
					return errOutcome(err), nil
				}
				return bson.D{
					{Key: "find", Value: found},
					{Key: "count", Value: n},
					{Key: "updated", Value: upd.ModifiedCount},
					{Key: "hits", Value: hits},
					{Key: "deleted", Value: del.DeletedCount},
				}, nil
			},
		})
	}
}
