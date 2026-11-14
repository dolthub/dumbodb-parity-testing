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

// TestVariablePathArrays: a path read from a variable traverses arrays as a
// field path reads the document does.
func TestVariablePathArrays(t *testing.T) {
	cases := map[string]string{
		"map_this":        `{"$map":{"input":"$a","in":"$$this.b"}}`,
		"map_as":          `{"$map":{"input":"$a","as":"x","in":"$$x.b"}}`,
		"map_as_deep":     `{"$map":{"input":"$a","as":"x","in":"$$x.b.b"}}`,
		"reduce":          `{"$reduce":{"input":"$a","initialValue":[],"in":{"$concatArrays":["$$value",[{"$ifNull":["$$this.b","none"]}]]}}}`,
		"filter":          `{"$filter":{"input":"$a","cond":{"$eq":["$$this.b",[9]]}}}`,
		"let_array":       `{"$let":{"vars":{"v":"$a"},"in":"$$v.b"}}`,
		"let_scalar":      `{"$type":{"$let":{"vars":{"v":5},"in":"$$v.b"}}}`,
		"let_scalars":     `{"$let":{"vars":{"v":[1,2]},"in":"$$v.b"}}`,
		"let_index":       `{"$let":{"vars":{"v":[1,2]},"in":"$$v.0"}}`,
		"let_missing":     `{"$type":{"$let":{"vars":{"v":[{"c":1}]},"in":"$$v.b"}}}`,
		"root_array":      `"$$ROOT.a.b"`,
		"current_array":   `"$$CURRENT.a.b.b"`,
		"root_missing":    `{"$type":"$$ROOT.nope"}`,
		"let_doc_numeric": `{"$let":{"vars":{"v":{"0":3}},"in":"$$v.0"}}`,
	}
	for name, expr := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"$project":{"m":`+expr+`}}`), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "VariablePathArrays_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				var docs []interface{}
				for _, d := range []string{
					`{"_id":1,"a":[[{"b":9}],{"b":2},[[{"b":3}]],5,{"c":1},[{"b":[7,8]},{"b":{"b":1}}]]}`,
					`{"_id":2,"a":[]}`,
				} {
					var doc bson.D
					if err := bson.UnmarshalExtJSON([]byte(d), false, &doc); err != nil {
						return err
					}
					docs = append(docs, doc)
				}
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: pipelineRun(bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, s}),
		})
	}
}
