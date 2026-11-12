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

// A string starting with "$" assigned to _id in a projection is a field path
// or variable, as it is for any other field.

func projectIDSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(10)}, {Key: "b", Value: "x"}, {Key: "s", Value: bson.D{{Key: "t", Value: int32(7)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: int32(20)}, {Key: "b", Value: "y"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "b", Value: "z"}},
	})
	return err
}

func TestProjectIDExpression(t *testing.T) {
	cases := []struct {
		name       string
		projection bson.D
	}{
		{"ProjectID_FieldPath", bson.D{{Key: "_id", Value: "$a"}}},
		{"ProjectID_FieldPathWithInclusion", bson.D{{Key: "_id", Value: "$b"}, {Key: "a", Value: int32(1)}}},
		{"ProjectID_DottedFieldPath", bson.D{{Key: "_id", Value: "$s.t"}, {Key: "b", Value: int32(1)}}},
		{"ProjectID_Variable", bson.D{{Key: "_id", Value: "$$ROOT.b"}}},
		{"ProjectID_LiteralString", bson.D{{Key: "_id", Value: "plain"}, {Key: "b", Value: int32(1)}}},
	}
	for _, c := range cases {
		projection := c.projection
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Setup:   projectIDSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col,
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					bson.D{{Key: "$project", Value: projection}},
				)
			},
		})
	}
}

func TestProjectIDExpression_Find(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProjectID_FindFieldPath",
		Support: harness.DumboDBFull,
		Setup:   projectIDSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{}, options.Find().
				SetSort(bson.D{{Key: "_id", Value: 1}}).
				SetProjection(bson.D{{Key: "_id", Value: "$b"}, {Key: "a", Value: int32(1)}}))
			if err != nil {
				return nil, err
			}
			var res []bson.D
			err = cur.All(ctx, &res)
			return res, err
		},
	})
}

func projectIDThenWrite(ctx context.Context, col *mongo.Collection, writeStage bson.D) (interface{}, error) {
	if _, err := fieldOpsAgg(ctx, col,
		bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: "$b"}, {Key: "a", Value: int32(1)}}}},
		writeStage,
	); err != nil {
		return nil, err
	}
	cur, err := col.Database().Collection(col.Name()+"_out").Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	if err != nil {
		return nil, err
	}
	var res []bson.D
	err = cur.All(ctx, &res)
	return res, err
}

func TestProjectIDExpression_BeforeOut(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProjectID_FieldPathBeforeOut",
		Support: harness.DumboDBFull,
		Setup:   projectIDSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return projectIDThenWrite(ctx, col, bson.D{{Key: "$out", Value: col.Name() + "_out"}})
		},
	})
}

func TestProjectIDExpression_BeforeMerge(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ProjectID_FieldPathBeforeMerge",
		Support: harness.DumboDBFull,
		Setup:   projectIDSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return projectIDThenWrite(ctx, col, bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: col.Name() + "_out"}}}})
		},
	})
}
