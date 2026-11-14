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

// TestElemMatchExpr: $expr anywhere inside $elemMatch, also beside field
// conditions, inside $all or in a logical operator, and $where or $text in
// a logical operator there, fail with BadValue whatever the data.
func TestElemMatchExpr(t *testing.T) {
	shapes := map[string]bson.A{
		"array":  {bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(1)}}}},
		"scalar": {bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(5)}}},
		"empty":  {},
	}
	matches := map[string]string{
		"value":       `{"a":{"$elemMatch":{"$expr":{"$eq":[1,1]}}}}`,
		"object":      `{"a":{"$elemMatch":{"b":1,"$expr":{"$eq":[1,1]}}}}`,
		"nested":      `{"a":{"$elemMatch":{"b":{"$elemMatch":{"$expr":1}}}}}`,
		"inside_all":  `{"a":{"$all":[{"$elemMatch":{"$expr":{"$eq":[1,1]}}}]}}`,
		"object_last": `{"a":{"$elemMatch":{"b":{"$gt":0},"c":1,"$expr":true}}}`,
		"in_and":      `{"a":{"$elemMatch":{"$and":[{"$expr":true}]}}}`,
		"in_or":       `{"a":{"$elemMatch":{"$or":[{"b":1},{"$expr":true}]}}}`,
		"in_nor":      `{"a":{"$elemMatch":{"$nor":[{"$expr":false}]}}}`,
		"deep_mixed":  `{"a":{"$elemMatch":{"$and":[{"b":1},{"$or":[{"$expr":true}]}]}}}`,
		"under_and":   `{"$and":[{"a":{"$elemMatch":{"$or":[{"$expr":true}]}}}]}`,
		"where_in_or": `{"a":{"$elemMatch":{"$or":[{"$where":"true"}]}}}`,
		"text_in_or":  `{"a":{"$elemMatch":{"$or":[{"$text":{"$search":"x"}}]}}}`,
	}
	for shapeName, docs := range shapes {
		docs := docs
		setup := func(ctx context.Context, col *mongo.Collection) error {
			if len(docs) == 0 {
				return col.Database().CreateCollection(ctx, col.Name())
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		}
		for matchName, match := range matches {
			var m bson.D
			if err := bson.UnmarshalExtJSON([]byte(match), false, &m); err != nil {
				t.Fatal(err)
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "ElemMatchExpr_" + shapeName + "_" + matchName,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(bson.A{bson.D{{Key: "$match", Value: m}}}),
			})
		}
	}
}
