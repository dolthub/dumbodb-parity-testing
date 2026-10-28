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
		name  string
		stage string
		spec  bson.D
	}{
		{"Computed", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$literal", Value: int32(5)}}}, {Key: "b", Value: "$f"}}}}},
		{"Constant", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: "x"}}}}},
		{"Deep", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: "$f"}}}}}}},
		{"Inclusion", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}}}}},
		{"InclusionAndComputed", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: true}, {Key: "c", Value: "$f"}}}}},
		{"Exclusion", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "z", Value: int32(0)}}}}},
		{"DottedComputed", "$project", bson.D{{Key: "v.a", Value: bson.D{{Key: "$literal", Value: int32(5)}}}, {Key: "v.b", Value: "$f"}}},
		{"DottedDeep", "$project", bson.D{{Key: "v.a.b", Value: "$f"}}},
		{"DottedInclusionAndComputed", "$project", bson.D{{Key: "v.a", Value: true}, {Key: "v.c", Value: "$f"}}},
		{"DottedAndNested", "$project", bson.D{{Key: "v.a", Value: int32(1)}, {Key: "w", Value: bson.D{{Key: "x", Value: "$_id"}}}}},
		{"WithTopLevelFields", "$project", bson.D{{Key: "f", Value: int32(1)}, {Key: "v", Value: bson.D{{Key: "b", Value: "$f"}}}, {Key: "t", Value: "$_id"}}},
		{"ExcludeID", "$project", bson.D{{Key: "_id", Value: int32(0)}, {Key: "v", Value: bson.D{{Key: "b", Value: "$f"}}}}},
		{"ComputedID", "$project", bson.D{{Key: "_id", Value: "$f"}, {Key: "v", Value: bson.D{{Key: "b", Value: "$_id"}}}}},
		{"NestedExclusionDeep", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: false}, {Key: "z", Value: int32(0)}}}}},
		{"DottedAddFields", "$addFields", bson.D{{Key: "v.b", Value: "$f"}}},
		{"DottedSet", "$set", bson.D{{Key: "v.a", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}}}},
		{"AddFieldsArray", "$addFields", bson.D{{Key: "arr", Value: bson.A{"$_id", "$f", "lit"}}}},
		{"AddFieldsNestedDeep", "$addFields", bson.D{{Key: "v", Value: bson.D{{Key: "n", Value: bson.D{{Key: "m", Value: "$_id"}}}}}}},
		{"AddFieldsNestedLiteralNumber", "$addFields", bson.D{{Key: "v", Value: bson.D{{Key: "one", Value: int32(1)}, {Key: "no", Value: false}}}}},
		{"AddFieldsNestedMissing", "$addFields", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: "$nope"}}}}},
		{"PathCollisionPrefixFirst", "$project", bson.D{{Key: "v", Value: int32(1)}, {Key: "v.a", Value: int32(1)}}},
		{"PathCollisionPrefixLast", "$project", bson.D{{Key: "v.a", Value: int32(1)}, {Key: "v", Value: int32(1)}}},
		{"PathCollisionNestedAndDotted", "$project", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}}}, {Key: "v.a", Value: int32(1)}}},
		{"PathCollisionComputed", "$project", bson.D{{Key: "v", Value: "$f"}, {Key: "v.a", Value: int32(1)}}},
		{"Empty", "$project", bson.D{{Key: "v", Value: bson.D{}}}},
		{"AddFields", "$addFields", bson.D{{Key: "v", Value: bson.D{{Key: "b", Value: "$f"}}}}},
		{"Set", "$set", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Project_ObjectLiteral_" + tc.name,
				Support: harness.DumboDBFull,
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

func TestFind_ProjectionObjectLiteral(t *testing.T) {
	for _, tc := range []struct {
		name string
		proj bson.D
	}{
		{"Nested", bson.D{{Key: "v", Value: bson.D{{Key: "b", Value: "$f"}}}}},
		{"Dotted", bson.D{{Key: "v.b", Value: "$f"}}},
		{"NestedInclusion", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}}}}},
		{"NestedInclusionAndComputed", bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: true}, {Key: "c", Value: "$f"}}}}},
		{"NestedExclusion", bson.D{{Key: "v", Value: bson.D{{Key: "z", Value: int32(0)}}}}},
		{"PathCollision", bson.D{{Key: "v", Value: int32(1)}, {Key: "v.a", Value: int32(1)}}},
		{"WithSlice", bson.D{{Key: "v", Value: bson.D{{Key: "$slice", Value: int32(1)}}}, {Key: "f", Value: int32(1)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Find_ProjectionObjectLiteral_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   projectObjectSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(tc.proj))
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
