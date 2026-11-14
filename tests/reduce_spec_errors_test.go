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

// TestReduceSpecErrors: $reduce takes one object (40075), refuses a field
// other than input, initialValue and in (40076, 'as' included), and then
// requires input, initialValue and in (40077, 40078, 40079).
func TestReduceSpecErrors(t *testing.T) {
	cases := map[string]string{
		"as_int":            `{"input":[1],"initialValue":0,"in":1,"as":1}`,
		"as_string":         `{"input":[1],"initialValue":0,"in":1,"as":"x"}`,
		"unknown":           `{"input":[1],"initialValue":0,"in":1,"bogus":1}`,
		"unknown_first":     `{"bogus":1}`,
		"unknown_before_in": `{"input":[1],"bogus":1}`,
		"empty":             `{}`,
		"no_input":          `{"initialValue":0,"in":1}`,
		"no_initial":        `{"input":[1],"in":1}`,
		"no_in":             `{"input":[1],"initialValue":0}`,
		"not_object":        `5`,
		"null":              `null`,
		"array":             `[{"input":[1],"initialValue":0,"in":1}]`,
		"ok":                `{"input":[1,2,3],"initialValue":0,"in":{"$add":["$$value","$$this"]}}`,
	}
	for name, spec := range cases {
		var s bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"$project":{"r":{"$reduce":`+spec+`}}}`), false, &s); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ReduceSpecErrors_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
				return err
			},
			Run: pipelineRun(bson.A{s}),
		})
	}
}
