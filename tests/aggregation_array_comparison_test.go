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

// Aggregation comparison operators compare whole values in BSON order, so an
// array is not matched element by element (dolthub/dumbodb#236).

func arrayComparisonSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "b", Value: bson.A{int32(1), int32(9)}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "b", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "b", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "b", Value: bson.A{bson.A{int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "b", Value: bson.D{{Key: "x", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "b", Value: nil}},
		bson.D{{Key: "_id", Value: int32(7)}},
	})
	return err
}

func TestAggregationArrayComparison(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayComparison_FindExpr",
		Support: harness.DumboDBFull,
		Setup:   arrayComparisonSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{"$b", int32(1)}}}}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var out []bson.D
			err = cursor.All(ctx, &out)
			return out, err
		},
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayComparison_Operators",
		Support: harness.DumboDBFull,
		Setup:   arrayComparisonSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "cmp", Value: bson.D{{Key: "$cmp", Value: bson.A{"$b", int32(1)}}}},
					{Key: "eq", Value: bson.D{{Key: "$eq", Value: bson.A{"$b", bson.A{int32(1), int32(9)}}}}},
					{Key: "lt", Value: bson.D{{Key: "$lt", Value: bson.A{"$b", bson.A{int32(2)}}}}},
					{Key: "ne", Value: bson.D{{Key: "$ne", Value: bson.A{"$b", int32(5)}}}},
					{Key: "gte", Value: bson.D{{Key: "$gte", Value: bson.A{"$b", bson.A{}}}}},
					{Key: "lte", Value: bson.D{{Key: "$lte", Value: bson.A{"$b", bson.D{{Key: "x", Value: int32(1)}}}}}},
					{Key: "prefix", Value: bson.D{{Key: "$cmp", Value: bson.A{bson.A{int32(1), int32(2)}, bson.A{int32(1), int32(2), int32(0)}}}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			)
		},
	})
}
