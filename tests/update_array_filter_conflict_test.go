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

// mongod resolves every positional path against the document as stored, then
// refuses two resolved paths that reach the same element (40), and applies
// none of the update.
func TestUpdatePositionalApplyConflicts(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	filters := func(fs ...bson.D) []interface{} {
		out := make([]interface{}, len(fs))
		for i, f := range fs {
			out[i] = f
		}
		return out
	}
	for _, c := range []struct {
		name    string
		update  bson.D
		filters []interface{}
	}{
		{"all_and_identifier", d("$set", d("a.$[]", 5), "$inc", d("a.$[x]", 1)), filters(d("x", 1))},
		{"all_and_identifier_disjoint", d("$set", d("a.$[]", 5), "$inc", d("a.$[x]", 1)), filters(d("x", 99))},
		{"identifiers_overlap", d("$set", d("a.$[x]", 5), "$inc", d("a.$[y]", 1)), filters(d("x", 1), d("y", d("$gt", 0)))},
		{"identifiers_overlap_second", d("$set", d("a.$[x]", 5), "$inc", d("a.$[y]", 1)), filters(d("x", 2), d("y", d("$gt", 0)))},
		{"identifiers_disjoint", d("$set", d("a.$[x]", 5), "$inc", d("a.$[y]", 1)), filters(d("x", 1), d("y", 2))},
		{"identifiers_one_operator", d("$set", d("a.$[x]", 5, "a.$[y]", 6)), filters(d("x", 1), d("y", 1))},
		{"subfields", d("$set", d("b.$[].x", 5, "b.$[e].y", 7)), filters(d("e.x", 1))},
		{"same_subfield", d("$set", d("b.$[].x", 5, "b.$[e].x", 7)), filters(d("e.x", 1))},
		{"element_and_subfield", d("$set", d("b.$[e]", 5, "b.$[f].x", 7)), filters(d("e.x", 1), d("f.y", 1))},
		{"subfield_and_element", d("$set", d("b.$[f].x", 7, "b.$[e]", 5)), filters(d("e.x", 1), d("f.y", 1))},
	} {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "UpdatePositionalApplyConflicts_" + c.name,
				Support: harness.DumboDBFull,
				Setup: insertDocs(d("_id", 1, "a", bson.A{1, 2},
					"b", bson.A{d("x", 1, "y", 1), d("x", 2, "y", 2)})),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						opts := options.Update().SetArrayFilters(options.ArrayFilters{Filters: c.filters})
						_, err := col.UpdateOne(ctx, d("_id", 1), c.update, opts)
						return err
					})
				},
			})
		})
	}
}
