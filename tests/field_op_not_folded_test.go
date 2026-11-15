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

// TestFieldOpNotFolded: mongod never folds $getField, $setField and
// $unsetField to constants, even over constant arguments, so a $shift
// default holding one is not constant and a failing operator over one is not
// an error while parsing.
func TestFieldOpNotFolded(t *testing.T) {
	shift := func(def string) string {
		return `[{"$setWindowFields":{"sortBy":{"_id":1},"output":{"s":{"$shift":{"output":1,"by":1,"default":` + def + `}}}}}]`
	}
	cases := map[string]string{
		"shift_get_field":         shift(`{"$getField":{"field":"a","input":{"a":1}}}`),
		"shift_get_field_literal": shift(`{"$getField":{"field":{"$literal":"a"},"input":{"$literal":{"a":1}}}}`),
		"shift_set_field":         shift(`{"$setField":{"field":"a","input":{"$literal":{}},"value":1}}`),
		"shift_unset_field":       shift(`{"$unsetField":{"field":"a","input":{"$literal":{"a":1}}}}`),
		"shift_nested_get_field":  shift(`{"$add":[1,{"$getField":{"field":"a","input":{"a":1}}}]}`),
		"shift_in_object":         shift(`{"a":{"$getField":{"field":"a","input":{"a":1}}}}`),
		"shift_add":               shift(`{"$add":[1,2]}`),
		"shift_object":            shift(`{"a":{"$add":[1,2]}}`),
		"divide_get_field":        `[{"$match":{"_id":999}},{"$project":{"z":{"$cond":[false,{"$divide":[1,{"$getField":{"field":"a","input":{"a":0}}}]},1]}}}]`,
		"divide_set_field":        `[{"$match":{"_id":999}},{"$project":{"z":{"$cond":[false,{"$divide":[1,{"$setField":{"field":"a","input":{},"value":0}}]},1]}}}]`,
		"get_field_value":         `[{"$project":{"z":{"$getField":{"field":"a","input":{"a":{"$add":[1,2]}}}}}}]`,
	}
	for name, pipeline := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "FieldOpNotFolded_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
				return err
			},
			Run: pipelineRun(p),
		})
	}
}
