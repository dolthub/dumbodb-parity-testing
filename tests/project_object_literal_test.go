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

// An object in a projection or $addFields is a nested specification: its
// fields are expressions, inclusions or exclusions applied under that field.

func projectObjectSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: int32(5)}, {Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "z", Value: int32(9)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: bson.A{bson.D{{Key: "a", Value: int32(2)}, {Key: "z", Value: int32(8)}}, int32(3)}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "f", Value: "s"}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "v", Value: int32(7)}},
	})
	return err
}

func TestProject_ObjectLiteral(t *testing.T) {
	for _, tc := range []struct {
		name    string
		stage   string
		spec    bson.D
		support harness.DumboDBSupport
	}{
		{"Computed", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$literal", Value: int32(5)}}}, {Key: "b", Value: "$f"}}}}, harness.DumboDBXFail},
		{"Constant", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: "x"}}}}, harness.DumboDBXFail},
		{"Deep", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: "$f"}}}}}}, harness.DumboDBXFail},
		{"Inclusion", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}}}}, harness.DumboDBXFail},
		{"InclusionAndComputed", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: true}, {Key: "c", Value: "$f"}}}}, harness.DumboDBXFail},
		{"Exclusion", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "z", Value: int32(0)}}}}, harness.DumboDBXFail},
		{"Empty", "$project", bson.D{{Key: "v", Value: bson.D{}}}, harness.DumboDBFull},
		{"AddFields", "$addFields", bson.D{{Key: "v", Value: bson.D{{Key: "b", Value: "$f"}}}}, harness.DumboDBXFail},
		{"Set", "$set", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}}}}}, harness.DumboDBXFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Project_ObjectLiteral_" + tc.name,
				Support: tc.support,
				Setup:   projectObjectSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cursor, err := col.Aggregate(ctx, bson.A{
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
						bson.D{{Key: tc.stage, Value: tc.spec}},
					})
					if err != nil {
						return nil, err
					}
					var results []bson.D
					return results, cursor.All(ctx, &results)
				},
			})
		})
	}
}
