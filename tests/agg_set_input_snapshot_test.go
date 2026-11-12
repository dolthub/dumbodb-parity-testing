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

// mongod evaluates every field of $set and $addFields against the stage's
// input document, so no field sees another field's new value.

func setSnapshotSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(1)},
			{Key: "a", Value: int32(1)},
			{Key: "b", Value: int32(2)},
			{Key: "x", Value: bson.D{{Key: "y", Value: int32(10)}, {Key: "z", Value: int32(20)}}},
		},
	})
	return err
}

func TestSetInputSnapshot(t *testing.T) {
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"Set_SwapFields", bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: "$b"}, {Key: "b", Value: "$a"}}}}},
		{"AddFields_SwapFields", bson.D{{Key: "$addFields", Value: bson.D{{Key: "a", Value: "$b"}, {Key: "b", Value: "$a"}}}}},
		{"Set_LaterFieldReadsInput", bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: int32(99)}, {Key: "c", Value: "$a"}}}}},
		{"Set_OperatorSwap", bson.D{{Key: "$set", Value: bson.D{
			{Key: "a", Value: bson.D{{Key: "$add", Value: bson.A{"$b", int32(100)}}}},
			{Key: "b", Value: bson.D{{Key: "$add", Value: bson.A{"$a", int32(100)}}}},
		}}}},
		{"Set_RootAfterAssign", bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: int32(5)}, {Key: "r", Value: "$$ROOT"}}}}},
		{"Set_MissingRemovesThenRead", bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: "$nope"}, {Key: "c", Value: "$a"}}}}},
		{"Set_DottedThenRead", bson.D{{Key: "$set", Value: bson.D{{Key: "x.y", Value: "$a"}, {Key: "a", Value: "$x.y"}}}}},
		{"Set_NestedSiblingRead", bson.D{{Key: "$set", Value: bson.D{{Key: "x.y", Value: "$x.z"}, {Key: "x.z", Value: "$x.y"}}}}},
		{"Set_ArrayLiteralThenRead", bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: bson.A{"$b"}}, {Key: "b", Value: "$a"}}}}},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Setup:   setSnapshotSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage)
			},
		})
	}
}

func TestSetInputSnapshot_UpdatePipelineSwap(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Set_UpdatePipelineSwap",
		Support: harness.DumboDBFull,
		Setup:   setSnapshotSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, mongo.Pipeline{
				bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: "$b"}, {Key: "b", Value: "$a"}}}},
			})
			if err != nil {
				return nil, err
			}
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$match", Value: bson.D{}}})
		},
	})
}
