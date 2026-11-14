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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $setWindowFields evaluates partitionBy as an expression of any shape, and
// refuses a partition key that is an array.

func windowPartitionSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "q", Value: int32(1)}, {Key: "t", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "q", Value: int32(2)}, {Key: "t", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "b"}, {Key: "q", Value: int32(1)}, {Key: "t", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "q", Value: int32(1)}, {Key: "t", Value: int32(4)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "p", Value: nil}, {Key: "q", Value: int32(1)}, {Key: "t", Value: int32(5)}},
	})
	return err
}

func TestWindowPartitionExpression(t *testing.T) {
	cases := []struct {
		name        string
		partitionBy interface{}
	}{
		{"Object", bson.D{{Key: "x", Value: "$p"}}},
		{"ObjectTwoFields", bson.D{{Key: "x", Value: "$p"}, {Key: "y", Value: "$q"}}},
		{"Operator", bson.D{{Key: "$concat", Value: bson.A{"$p", "-"}}}},
		{"FieldPath", "$p"},
		{"Constant", int32(5)},
		{"ArrayLiteral", bson.A{"$p", "$q"}},
		{"Error", bson.D{{Key: "$divide", Value: bson.A{"$q", 0}}}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "WindowPartition_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   windowPartitionSeed,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: c.partitionBy},
					{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
					{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$push", Value: "$_id"}}}}},
				}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}),
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "WindowPartition_FieldIsArray",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: bson.A{1}}})
			return err
		},
		Run: pipelineRun(bson.A{bson.D{{Key: "$setWindowFields", Value: bson.D{
			{Key: "partitionBy", Value: "$p"},
			{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$count", Value: bson.D{}}}}}},
		}}}}),
	})
}
