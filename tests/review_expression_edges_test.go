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

// TestArrayOperandScalarAfterFirst: mongod rejects a set or concat operand
// list whose first element is an array and a later one is a scalar, whether
// the operands are literals or fields.
func TestArrayOperandScalarAfterFirst(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(1)}}, {Key: "s", Value: int32(2)}})
		return err
	}
	inBoth := map[string]string{
		"SetUnionLiteral":        `{"$setUnion":[[1],2]}`,
		"SetUnionThreeLiteral":   `{"$setUnion":[[1],[2],3]}`,
		"SetUnionField":          `{"$setUnion":["$a","$s"]}`,
		"SetIntersectionLiteral": `{"$setIntersection":[[1],2]}`,
		"SetIntersectionField":   `{"$setIntersection":["$a","$s"]}`,
		"SetDifferenceLiteral":   `{"$setDifference":[[1],2]}`,
		"SetDifferenceField":     `{"$setDifference":["$a","$s"]}`,
		"SetEqualsField":         `{"$setEquals":["$a","$s"]}`,
		"SetIsSubsetField":       `{"$setIsSubset":["$a","$s"]}`,
	}
	groupOnly := map[string]string{
		"ConcatField":        `{"$concatArrays":["$a","$s"]}`,
		"ConcatFieldLiteral": `{"$concatArrays":["$a",2]}`,
	}
	parse := func(name, js string) bson.D {
		var expr bson.D
		if err := bson.UnmarshalExtJSON([]byte(js), false, &expr); err != nil {
			t.Fatal(name, err)
		}
		return expr
	}
	addGroup := func(name string, expr bson.D) {
		harness.PairTest(t, harness.TestCase{
			Name:    "ArrayOperandScalarAfterFirst_Group" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: expr}}}}}}}),
		})
	}
	for name, js := range inBoth {
		expr := parse(name, js)
		harness.PairTest(t, harness.TestCase{
			Name:    "ArrayOperandScalarAfterFirst_Project" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: expr}}}}}),
		})
		addGroup(name, expr)
	}
	for name, js := range groupOnly {
		addGroup(name, parse(name, js))
	}
}

// TestReplaceWithArrayToObjectNulKey: mongod fails with 4940400 when
// $replaceWith builds a document from a stored array holding a NUL key.
func TestReplaceWithArrayToObjectNulKey(t *testing.T) {
	seeds := map[string]bson.A{
		"Pair":     {bson.A{"p\x00q", int32(1)}},
		"KV":       {bson.D{{Key: "k", Value: "p\x00q"}, {Key: "v", Value: int32(1)}}},
		"PairOnly": {bson.A{"\x00", int32(1)}},
	}
	for name, a := range seeds {
		harness.PairTest(t, harness.TestCase{
			Name:    "ReplaceWithArrayToObjectNulKey_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: a}})
				return err
			},
			Run: pipelineRun(bson.A{bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$arrayToObject", Value: "$a"}}}}}),
		})
	}
}

// TestLargeLegalAccumulatorGroup: mongod's SBE $firstN keeps a constant
// million-element range from each of 9 documents within its 100MB limit.
func TestLargeLegalAccumulatorGroup(t *testing.T) {
	big := bson.D{{Key: "$range", Value: bson.A{int32(0), int32(1000000)}}}
	cases := []struct {
		name string
		docs int32
		acc  bson.D
	}{
		{"FirstN9", 9, bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: big}, {Key: "n", Value: int32(100)}}}}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LargeLegalAccumulatorGroup_" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				var docs []interface{}
				for i := int32(0); i < c.docs; i++ {
					docs = append(docs, bson.D{{Key: "_id", Value: i}})
				}
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "f", Value: c.acc}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$f"}}}}}},
			}),
		})
	}
}
