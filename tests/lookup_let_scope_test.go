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

// TestLookupLetScope: $lookup let variables are scoped as mongod scopes them.
// A nested $lookup, $let, $map, $filter or $reduce that defines a name again
// hides the outer value, $literal keeps "$$v" as a string, and a variable
// where a constant is required fails to parse.
func TestLookupLetScope(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(2)}},
		}); err != nil {
			return err
		}
		foreign := col.Database().Collection(col.Name() + "_f")
		if err := foreign.Drop(ctx); err != nil {
			return err
		}
		_, err := foreign.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "y", Value: int32(1)}, {Key: "arr", Value: bson.A{int32(1), int32(2), int32(3)}}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "y", Value: int32(2)}, {Key: "arr", Value: bson.A{int32(1), int32(2), int32(3)}}},
		})
		return err
	}
	project := func(fields bson.D) bson.A {
		return bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, bson.D{{Key: "$project", Value: append(bson.D{{Key: "_id", Value: 0}}, fields...)}}}
	}
	vx := bson.D{{Key: "v", Value: "$x"}}
	cases := []struct {
		name string
		let  bson.D
		sub  func(foreign string) bson.A
	}{
		{"nested_lookup_redefines", vx, func(f string) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: f},
				{Key: "let", Value: bson.D{{Key: "v", Value: bson.D{{Key: "$add", Value: bson.A{"$y", int32(100)}}}}}},
				{Key: "pipeline", Value: project(bson.D{{Key: "r", Value: "$$v"}})}, {Key: "as", Value: "k"}}}}}
		}},
		{"nested_lookup_reads_outer", vx, func(f string) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: f},
				{Key: "let", Value: bson.D{{Key: "w", Value: bson.D{{Key: "$add", Value: bson.A{"$$v", int32(10)}}}}}},
				{Key: "pipeline", Value: project(bson.D{{Key: "r", Value: bson.A{"$$v", "$$w"}}})}, {Key: "as", Value: "k"}}}}}
		}},
		{"let_redefines", vx, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$let", Value: bson.D{
				{Key: "vars", Value: bson.D{{Key: "v", Value: bson.D{{Key: "$add", Value: bson.A{"$$v", int32(100)}}}}}}, {Key: "in", Value: "$$v"}}}}}})
		}},
		{"map_as", vx, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$map", Value: bson.D{
				{Key: "input", Value: bson.A{int32(7)}}, {Key: "as", Value: "v"}, {Key: "in", Value: "$$v"}}}}}})
		}},
		{"map_this", bson.D{{Key: "this", Value: "$x"}}, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$map", Value: bson.D{
				{Key: "input", Value: bson.A{int32(7)}}, {Key: "in", Value: "$$this"}}}}}, {Key: "t", Value: "$$this"}})
		}},
		{"reduce_value", bson.D{{Key: "value", Value: "$x"}}, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$reduce", Value: bson.D{
				{Key: "input", Value: bson.A{int32(5)}}, {Key: "initialValue", Value: "$$value"},
				{Key: "in", Value: bson.D{{Key: "$add", Value: bson.A{"$$this", "$$value"}}}}}}}}})
		}},
		{"filter_as_and_limit", vx, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$filter", Value: bson.D{{Key: "input", Value: "$arr"}, {Key: "as", Value: "v"},
				{Key: "cond", Value: bson.D{{Key: "$gte", Value: bson.A{"$$v", int32(2)}}}}, {Key: "limit", Value: "$$v"}}}}}})
		}},
		{"filter_limit", vx, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$filter", Value: bson.D{{Key: "input", Value: "$arr"},
				{Key: "cond", Value: true}, {Key: "limit", Value: "$$v"}}}}}})
		}},
		{"literal", vx, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$literal", Value: "$$v"}}}})
		}},
		{"shift_default", vx, func(string) bson.A {
			return bson.A{bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
				{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$shift", Value: bson.D{
					{Key: "output", Value: "$y"}, {Key: "by", Value: int32(5)}, {Key: "default", Value: "$$v"}}}}}}}}}}}
		}},
		{"set_field_field", vx, func(string) bson.A {
			return project(bson.D{{Key: "r", Value: bson.D{{Key: "$setField", Value: bson.D{
				{Key: "field", Value: "$$v"}, {Key: "input", Value: bson.D{}}, {Key: "value", Value: int32(1)}}}}}})
		}},
		{"facet_and_union", vx, func(f string) bson.A {
			return bson.A{bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: f}, {Key: "pipeline", Value: project(bson.D{{Key: "u", Value: "$$v"}})}}}},
				bson.D{{Key: "$facet", Value: bson.D{{Key: "a", Value: project(bson.D{{Key: "u", Value: "$$v"}})}}}}}
		}},
	}
	for _, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLetScope_" + tc.name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				foreign := col.Name() + "_f"
				cursor, err := col.Aggregate(ctx, bson.A{
					bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: foreign}, {Key: "let", Value: tc.let},
						{Key: "pipeline", Value: tc.sub(foreign)}, {Key: "as", Value: "j"}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
