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

// mongod refuses a non-document array filter for the whole command before
// any statement runs, and parses array filter bodies before the update itself,
// for operator, pipeline and replacement updates alike.
func TestArrayFiltersParseOrder(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	badBody := bson.A{d("x", d("$foo", 1))}
	setB := d("$set", d("b", 1))
	setX := d("$set", d("a.$[x]", 5))
	type stmt struct {
		u       interface{}
		filters bson.A
	}
	for _, c := range []struct {
		name  string
		stmts []stmt
	}{
		{"non_document_after_valid_statement", []stmt{{setB, nil}, {setX, bson.A{5}}}},
		{"non_document_after_valid_filter", []stmt{{setB, nil}, {setX, bson.A{d("x", 1), 1}}}},
		{"non_document_pipeline", []stmt{{bson.A{setB}, bson.A{1}}}},
		{"non_document_replacement", []stmt{{d("b", 1), bson.A{1}}}},
		{"body_before_unknown_modifier", []stmt{{d("$foo", d("b", 1), "$set", d("a.$[x]", 5)), badBody}}},
		{"body_before_pipeline_refusal", []stmt{{bson.A{setB}, badBody}}},
		{"pipeline_refusal", []stmt{{bson.A{setB}, bson.A{d("x", 1)}}}},
		{"body_before_unused", []stmt{{setB, badBody}}},
		{"replacement_bad_body", []stmt{{d("b", 1), badBody}}},
		{"replacement_duplicate_identifier", []stmt{{d("b", 1), bson.A{d("x", 1), d("x", 2)}}}},
		{"replacement_valid_body", []stmt{{d("b", 1), bson.A{d("x", 1)}}}},
	} {
		for _, w := range []string{"update", "bulkWrite", "findAndModify"} {
			if w == "findAndModify" && len(c.stmts) > 1 {
				continue
			}
			name := c.name + "_" + w
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "ArrayFiltersParseOrder_" + name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(d("_id", 1, "a", bson.A{1, 2})),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error {
							var cmd bson.D
							switch w {
							case "update":
								var updates bson.A
								for _, s := range c.stmts {
									u := d("q", d("_id", 1), "u", s.u)
									if s.filters != nil {
										u = append(u, bson.E{Key: "arrayFilters", Value: s.filters})
									}
									updates = append(updates, u)
								}
								cmd = d("update", col.Name(), "updates", updates, "ordered", false)
							case "bulkWrite":
								var ops bson.A
								for _, s := range c.stmts {
									op := d("update", 0, "filter", d("_id", 1), "updateMods", s.u)
									if s.filters != nil {
										op = append(op, bson.E{Key: "arrayFilters", Value: s.filters})
									}
									ops = append(ops, op)
								}
								cmd = d("bulkWrite", 1, "ops", ops, "nsInfo", bson.A{d("ns", col.Database().Name()+"."+col.Name())}, "ordered", false)
								var res bson.M
								err := col.Database().Client().Database("admin").RunCommand(ctx, cmd).Decode(&res)
								if err != nil {
									return err
								}
								for _, e := range res["cursor"].(bson.M)["firstBatch"].(bson.A) {
									if code, ok := e.(bson.M)["code"].(int32); ok {
										return mongo.CommandError{Code: code}
									}
								}
								return nil
							case "findAndModify":
								cmd = d("findAndModify", col.Name(), "query", d("_id", 1), "update", c.stmts[0].u, "arrayFilters", c.stmts[0].filters)
							}
							return col.Database().RunCommand(ctx, cmd).Err()
						})
					},
				})
			})
		}
	}
}
