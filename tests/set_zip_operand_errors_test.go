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

// TestSetZipOperandErrors: constant $setEquals operands must be arrays when
// mongod optimizes them; $setIsSubset's code depends on which operands are
// constant; $zip refuses inputs that are not arrays.
func TestSetZipOperandErrors(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{1}}, {Key: "s", Value: int32(2)}, {Key: "n", Value: nil}})
		return err
	}
	exprs := map[string]string{
		"set_equals_const_second":   `{"$setEquals":[[1],2]}`,
		"set_equals_field_const":    `{"$setEquals":["$a",2]}`,
		"set_equals_const_first":    `{"$setEquals":[2,[1]]}`,
		"set_equals_fields":         `{"$setEquals":["$a","$s"]}`,
		"set_equals_field_array":    `{"$setEquals":["$s",[1]]}`,
		"set_is_subset_consts":      `{"$setIsSubset":[[1],2]}`,
		"set_is_subset_const_first": `{"$setIsSubset":[2,[1]]}`,
		"set_is_subset_field_const": `{"$setIsSubset":["$a",2]}`,
		"set_is_subset_field_arr":   `{"$setIsSubset":["$s",[1]]}`,
		"set_is_subset_fields":      `{"$setIsSubset":["$s","$a"]}`,
		"zip_const_input":           `{"$zip":{"inputs":[[1],2]}}`,
		"zip_field_input":           `{"$zip":{"inputs":["$a","$s"]}}`,
		"zip_null_input":            `{"$zip":{"inputs":["$a","$n"]}}`,
		"zip_inputs_int":            `{"$zip":{"inputs":5}}`,
		"zip_inputs_field":          `{"$zip":{"inputs":"$a"}}`,
	}
	for name, e := range exprs {
		var expr bson.D
		if err := bson.UnmarshalExtJSON([]byte(e), false, &expr); err != nil {
			t.Fatal(err)
		}
		for stage, pipeline := range map[string]bson.A{
			"project": {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: expr}}}}},
			"group":   {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: expr}}}}},
		} {
			harness.PairTest(t, harness.TestCase{
				Name:    "SetZipOperandErrors_" + stage + "_" + name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(pipeline),
			})
		}
	}
}
