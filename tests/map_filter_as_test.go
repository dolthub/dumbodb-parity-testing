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

// TestMapFilterAs: a non-string 'as' of $map or $filter is an empty
// variable name, checked after unknown and missing fields and the input
// expression; $map reports its own argument errors with mongod's codes.
func TestMapFilterAs(t *testing.T) {
	cases := map[string]string{
		"filter_as_int":          `{"$filter":{"input":[1],"as":1,"cond":true}}`,
		"filter_as_object":       `{"$filter":{"input":[1],"as":{"a":1},"cond":"$$this"}}`,
		"filter_as_int_bad_in":   `{"$filter":{"input":"$$bad","as":1,"cond":true}}`,
		"filter_as_int_bad_cond": `{"$filter":{"input":[1],"as":1,"cond":"$$bad"}}`,
		"filter_as_int_input":    `{"$filter":{"input":5,"as":1,"cond":true}}`,
		"filter_as_int_no_input": `{"$filter":{"as":1,"cond":true}}`,
		"filter_as_int_no_cond":  `{"$filter":{"input":[1],"as":1}}`,
		"filter_as_int_limit":    `{"$filter":{"input":[1],"as":1,"cond":true,"limit":"x"}}`,
		"filter_as_empty_zz":     `{"$filter":{"input":[1],"as":"","cond":true,"zz":1}}`,
		"filter_as_ok":           `{"$filter":{"input":[1,2],"as":"v","cond":{"$gt":["$$v",1]}}}`,
		"map_as_int":             `{"$map":{"input":[1],"as":1,"in":1}}`,
		"map_as_null":            `{"$map":{"input":[1],"as":null,"in":1}}`,
		"map_as_empty_zz":        `{"$map":{"input":[1],"as":"","in":1,"zz":1}}`,
		"map_no_in":              `{"$map":{"input":[1],"as":"A"}}`,
		"map_no_input":           `{"$map":{"in":1}}`,
		"map_not_object":         `{"$map":5}`,
		"map_array":              `{"$map":[1,2]}`,
		"map_unknown":            `{"$map":{"zz":1}}`,
	}
	for name, expr := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"$project":{"z":`+expr+`}}`), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "MapFilterAs_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
				return err
			},
			Run: pipelineRun(bson.A{s}),
		})
	}
}
