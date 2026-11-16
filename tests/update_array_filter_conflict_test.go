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

// mongod refuses, while parsing an update, a $[] or $[id] element next to any
// other element under the same path (40), even when nothing matches.
func TestUpdateArrayFilterPathConflicts(t *testing.T) {
	set := func(kv ...interface{}) bson.D {
		var fields bson.D
		for i := 0; i < len(kv); i += 2 {
			fields = append(fields, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return bson.D{{Key: "$set", Value: fields}}
	}
	eq := func(id string) []interface{} { return []interface{}{bson.D{{Key: id, Value: 1}}} }
	for _, c := range []struct {
		name    string
		update  bson.D
		filters []interface{}
	}{
		{"all_then_index_other_operator", bson.D{
			{Key: "$set", Value: bson.D{{Key: "a.$[]", Value: 5}}},
			{Key: "$inc", Value: bson.D{{Key: "a.0", Value: 1}}},
		}, nil},
		{"index_then_all_other_operator", bson.D{
			{Key: "$inc", Value: bson.D{{Key: "a.0", Value: 1}}},
			{Key: "$set", Value: bson.D{{Key: "a.$[]", Value: 5}}},
		}, nil},
		{"all_then_index", set("a.$[]", 5, "a.0", 1), nil},
		{"identifier_then_index", set("a.$[e]", 5, "a.0", 1), eq("e")},
		{"all_then_field", set("a.$[]", 5, "a.foo", 1), nil},
		{"all_nested_then_index_nested", set("b.$[].x", 5, "b.0.y", 1), nil},
		{"index_nested_then_all_nested", set("b.0.x", 5, "b.$[].y", 1), nil},
		{"all_nested_then_index", set("b.$[].x", 5, "b.1", 1), nil},
		{"positional_then_all", set("a.$", 5, "a.$[]", 1), nil},
		{"all_twice_nested", set("b.$[].x", 5, "b.$[].y", 1), nil},
		{"identifiers_distinct", set("a.$[e]", 5, "a.$[f]", 1), []interface{}{
			bson.D{{Key: "e", Value: 1}}, bson.D{{Key: "f", Value: 2}},
		}},
	} {
		for _, target := range []struct {
			name   string
			filter bson.D
		}{{"match", bson.D{{Key: "a", Value: 1}}}, {"no_match", bson.D{{Key: "_id", Value: 9}}}} {
			name := c.name + "_" + target.name
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "UpdateArrayFilterPathConflicts_" + name,
					Support: harness.DumboDBFull,
					Setup: insertDocs(bson.D{
						{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{1, 2}},
						{Key: "b", Value: bson.A{bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 1}}}},
					}),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error {
							opts := options.Update()
							if c.filters != nil {
								opts.SetArrayFilters(options.ArrayFilters{Filters: c.filters})
							}
							_, err := col.UpdateOne(ctx, target.filter, c.update, opts)
							return err
						})
					},
				})
			})
		}
	}
}
