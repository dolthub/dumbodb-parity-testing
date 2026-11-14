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

// TestZipSpecErrors: $zip checks its fields in order, then that it has an
// input, that defaults need useLongestLength and match the inputs' length.
func TestZipSpecErrors(t *testing.T) {
	cases := map[string]string{
		"defaults_string":          `{"inputs":[[1],[2,3]],"defaults":"ab","useLongestLength":true}`,
		"defaults_before_inputs":   `{"defaults":"ab","inputs":5}`,
		"inputs_before_defaults":   `{"inputs":5,"defaults":"ab"}`,
		"defaults_null":            `{"inputs":[[1],[2,3]],"defaults":null,"useLongestLength":true}`,
		"defaults_path":            `{"inputs":[[1],[2,3]],"defaults":"$x","useLongestLength":true}`,
		"longest_int":              `{"inputs":[[1],[2,3]],"useLongestLength":1}`,
		"longest_null":             `{"inputs":[[1],[2,3]],"useLongestLength":null}`,
		"unknown":                  `{"inputs":[[1],[2,3]],"zz":1}`,
		"unknown_first":            `{"zz":1,"inputs":5}`,
		"empty":                    `{}`,
		"no_inputs":                `{"inputs":[]}`,
		"defaults_only":            `{"defaults":[1,2]}`,
		"defaults_without_longest": `{"inputs":[[1],[2,3]],"defaults":[1,2]}`,
		"defaults_longest_false":   `{"inputs":[[1],[2,3]],"defaults":[1,2],"useLongestLength":false}`,
		"defaults_short":           `{"inputs":[[1],[2,3]],"defaults":[1],"useLongestLength":true}`,
		"defaults_ok":              `{"inputs":[[1],[2,3]],"defaults":["$_id",2],"useLongestLength":true}`,
		"input_not_array":          `{"inputs":[[1],5]}`,
		"not_object":               `5`,
		"array":                    `[[1]]`,
	}
	for name, spec := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"$project":{"z":{"$zip":`+spec+`}}}`), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ZipSpecErrors_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
				return err
			},
			Run: pipelineRun(bson.A{s}),
		})
	}
}
