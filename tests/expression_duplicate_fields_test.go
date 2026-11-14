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

// TestExpressionDuplicateFields: a key repeated in an expression gives
// mongod's parse error, in an aggregate or an update pipeline: 16406 in an
// object literal, 15983 in an operator object, 40181 for an operator object
// that is a $set field's value, 16410 in $project and 40238 for a $group
// field naming two accumulators.
func TestExpressionDuplicateFields(t *testing.T) {
	cases := map[string]string{
		"ReplaceWithLiteral":       `[{"$replaceWith":{"y":1,"y":2}}]`,
		"ReplaceWithNested":        `[{"$replaceWith":{"y":{"z":1,"z":2}}}]`,
		"ReplaceWithOperator":      `[{"$replaceWith":{"$add":[1,2],"$add":[1,2]}}]`,
		"ReplaceWithInnerOperator": `[{"$replaceWith":{"y":[{"$add":1,"$add":2}]}}]`,
		"ReplaceWithMergeObjects":  `[{"$replaceWith":{"$mergeObjects":[{"y":1,"y":2}]}}]`,
		"ReplaceRootLiteral":       `[{"$replaceRoot":{"newRoot":{"y":1,"y":2}}}]`,
		"SetOperator":              `[{"$set":{"y":{"$add":[1,2],"$add":[1,2]}}}]`,
		"SetLiteralOperator":       `[{"$set":{"y":{"$literal":1,"$literal":2}}}]`,
		"SetNestedFieldOperator":   `[{"$set":{"y":{"w":{"$add":1,"$add":2}}}}]`,
		"SetDottedFieldOperator":   `[{"$set":{"y.w":{"$add":1,"$add":2}}}]`,
		"SetArrayLiteral":          `[{"$set":{"y":[{"z":1,"z":2}]}}]`,
		"SetOperandLiteral":        `[{"$set":{"y":{"$abs":{"z":1,"z":2}}}}]`,
		"SetOperandOperator":       `[{"$set":{"y":{"$abs":{"$add":1,"$add":2}}}}]`,
		"SetArgumentLiteral":       `[{"$set":{"y":{"$add":[1,{"z":1,"z":2}]}}}]`,
		"AddFieldsOperator":        `[{"$addFields":{"y":{"$add":[1,2],"$add":[1,2]}}}]`,
		"ProjectOperator":          `[{"$project":{"y":{"$add":[1,2],"$add":[1,2]}}}]`,
		"ProjectNestedOperator":    `[{"$project":{"y":{"a":{"$add":[1,2],"$add":[1,2]}}}}]`,
		"ProjectArrayLiteral":      `[{"$project":{"y":[{"z":1,"z":2}]}}]`,
		"SetThenReplaceWith":       `[{"$set":{"y":1}},{"$replaceWith":{"y":1,"y":2}}]`,
	}
	aggregateOnly := map[string]string{
		"GroupTwoAccumulators": `[{"$group":{"_id":null,"s":{"$sum":1,"$sum":2}}}]`,
		"GroupOperandLiteral":  `[{"$group":{"_id":null,"s":{"$sum":{"z":1,"z":2}}}}]`,
		"LookupPipeline":       `[{"$lookup":{"from":"x","pipeline":[{"$replaceWith":{"y":1,"y":2}}],"as":"r"}}]`,
		"FacetReplaceWith":     `[{"$facet":{"f":[{"$replaceWith":{"y":1,"y":2}}]}}]`,
		"FacetSetOperator":     `[{"$facet":{"f":[{"$set":{"y":{"$add":1,"$add":2}}}]}}]`,
	}
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}})
		return err
	}
	parse := func(name, js string) bson.A {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		return pipeline
	}
	for name, js := range cases {
		pipeline := parse(name, js)
		harness.PairTest(t, harness.TestCase{
			Name:    "ExpressionDuplicateFields_Update" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, pipeline)
				return errOutcome(err), nil
			},
		})
		aggregateOnly[name] = js
	}
	for name, js := range aggregateOnly {
		pipeline := parse(name, js)
		harness.PairTest(t, harness.TestCase{
			Name:    "ExpressionDuplicateFields_Aggregate" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().RunCommandCursor(ctx, bson.D{
					{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}},
				})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
