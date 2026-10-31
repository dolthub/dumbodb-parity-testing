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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A find projection accepts any aggregation expression as a field value, with
// the same meaning as in $project (workspace-mkf).

func insertFindExprDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(1)}, {Key: "name", Value: "Alpha"}, {Key: "a", Value: "x"}, {Key: "b", Value: "y"},
			{Key: "x", Value: int32(3)}, {Key: "neg", Value: int32(-4)}, {Key: "arr", Value: bson.A{int32(1), int32(2), int32(3)}},
			{Key: "dt", Value: time.Date(2021, 3, 4, 5, 6, 7, 0, time.UTC)}, {Key: "s", Value: "banana"},
			{Key: "sub", Value: bson.D{{Key: "p", Value: int32(1)}, {Key: "q", Value: "w"}}},
			{Key: "items", Value: bson.A{bson.D{{Key: "k", Value: int32(1)}}, bson.D{{Key: "k", Value: int32(5)}}}},
		},
		bson.D{
			{Key: "_id", Value: int32(2)}, {Key: "name", Value: "beta"}, {Key: "x", Value: 1.5},
			{Key: "arr", Value: bson.A{}}, {Key: "s", Value: ""},
		},
		bson.D{{Key: "_id", Value: int32(3)}},
	})
	return err
}

func findExprCase(t *testing.T, name string, filter, projection bson.D) {
	t.Helper()
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_" + name,
		Support: harness.DumboDBFull,
		Setup:   insertFindExprDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if filter == nil {
				filter = bson.D{}
			}
			cur, err := col.Find(ctx, filter, options.Find().SetProjection(projection).SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var out []bson.D
			return docsToSlice(out), cur.All(ctx, &out)
		},
	})
}

func TestFindProjExpr(t *testing.T) {
	op := func(name string, v interface{}) bson.D { return bson.D{{Key: name, Value: v}} }
	m := func(v interface{}) bson.D { return bson.D{{Key: "m", Value: v}} }

	for _, tc := range []struct {
		name       string
		filter     bson.D
		projection bson.D
	}{
		{"RegexFindAllSize", nil, m(op("$size", op("$regexFindAll", bson.D{{Key: "input", Value: "$s"}, {Key: "regex", Value: "a"}})))},
		{"ToUpper", nil, m(op("$toUpper", "$name"))},
		{"Concat", nil, m(op("$concat", bson.A{"$a", "-", "$b"}))},
		{"CondArray", nil, m(op("$cond", bson.A{op("$gt", bson.A{"$x", int32(1)}), "big", "small"}))},
		{"Switch", nil, m(op("$switch", bson.D{
			{Key: "branches", Value: bson.A{bson.D{{Key: "case", Value: op("$eq", bson.A{"$x", int32(3)})}, {Key: "then", Value: "three"}}}},
			{Key: "default", Value: "other"},
		}))},
		{"Map", nil, m(op("$map", bson.D{{Key: "input", Value: "$arr"}, {Key: "in", Value: op("$multiply", bson.A{"$$this", int32(2)})}}))},
		{"Filter", nil, m(op("$filter", bson.D{{Key: "input", Value: "$arr"}, {Key: "cond", Value: op("$gt", bson.A{"$$this", int32(1)})}}))},
		{"Reduce", nil, m(op("$reduce", bson.D{{Key: "input", Value: "$arr"}, {Key: "initialValue", Value: int32(0)}, {Key: "in", Value: op("$add", bson.A{"$$value", "$$this"})}}))},
		{"DateToString", nil, m(op("$dateToString", bson.D{{Key: "date", Value: "$dt"}, {Key: "format", Value: "%Y-%m-%d"}}))},
		{"Let", nil, m(op("$let", bson.D{{Key: "vars", Value: bson.D{{Key: "v", Value: "$x"}}}, {Key: "in", Value: op("$add", bson.A{"$$v", int32(1)})}}))},
		{"Remove", nil, m(op("$ifNull", bson.A{"$nope", "$$REMOVE"}))},
		{"AddMissing", nil, m(op("$add", bson.A{"$nope", int32(1)}))},
		{"TypeMissing", nil, m(op("$type", "$nope"))},
		{"SumArray", nil, m(op("$sum", "$arr"))},
		{"ObjectToArray", nil, m(op("$objectToArray", "$sub"))},
		{"MergeObjects", nil, m(op("$mergeObjects", bson.A{"$sub", bson.D{{Key: "z", Value: int32(1)}}}))},
		{"GetField", nil, m(op("$getField", "name"))},
		{"RegexMatch", nil, m(op("$regexMatch", bson.D{{Key: "input", Value: "$name"}, {Key: "regex", Value: "^a"}, {Key: "options", Value: "i"}}))},
		{"LiteralDocument", nil, m(op("$literal", bson.D{{Key: "$x", Value: int32(1)}}))},
		{"RandBounded", nil, m(op("$lt", bson.A{op("$rand", bson.D{}), int32(1)}))},
		{"ArrayOfExpressions", nil, m(bson.A{"$x", op("$add", bson.A{int32(1), int32(2)}), "lit"})},
		{"WithInclusion", nil, bson.D{{Key: "x", Value: int32(1)}, {Key: "m", Value: op("$abs", "$neg")}}},
		{"WithoutID", nil, bson.D{{Key: "_id", Value: int32(0)}, {Key: "m", Value: op("$toLower", "$name")}}},
		{"ComputedID", nil, bson.D{{Key: "_id", Value: op("$toUpper", "$name")}}},
		{"DottedPath", nil, bson.D{{Key: "a.b", Value: op("$add", bson.A{"$x", int32(1)})}}},
		{"NestedDocument", nil, bson.D{{Key: "n", Value: bson.D{{Key: "p", Value: op("$add", bson.A{"$x", int32(1)})}, {Key: "q", Value: "$name"}}}}},
		{"NestedOverExisting", nil, bson.D{{Key: "sub", Value: bson.D{{Key: "p", Value: int32(1)}, {Key: "r", Value: op("$concat", bson.A{"$sub.q", "!"})}}}}},
		{"WithSlice", nil, bson.D{{Key: "arr", Value: op("$slice", int32(1))}, {Key: "m", Value: op("$size", "$arr")}}},
		{"WithElemMatch", nil, bson.D{{Key: "items", Value: op("$elemMatch", bson.D{{Key: "k", Value: bson.D{{Key: "$gt", Value: int32(2)}}}})}, {Key: "m", Value: op("$size", "$items")}}},
		{"WithPositional", bson.D{{Key: "arr", Value: int32(2)}}, bson.D{{Key: "arr.$", Value: int32(1)}, {Key: "m", Value: op("$size", "$arr")}}},
		{"ExclusionWithExpression", nil, bson.D{{Key: "x", Value: int32(0)}, {Key: "m", Value: op("$abs", "$neg")}}},
		{"ExpressionThenExclusion", nil, bson.D{{Key: "m", Value: op("$abs", "$neg")}, {Key: "x", Value: int32(0)}}},
		{"ExpressionWithoutIDExclusion", nil, bson.D{{Key: "m", Value: op("$abs", "$neg")}, {Key: "_id", Value: false}}},
		{"ComputedIDMissing", nil, bson.D{{Key: "_id", Value: "$nope"}, {Key: "x", Value: int32(1)}}},
		{"ComputedIDArray", nil, bson.D{{Key: "_id", Value: bson.A{"$x", int32(1)}}}},
		{"ArrayWithSlice", nil, bson.D{{Key: "arr", Value: op("$slice", int32(1))}, {Key: "m", Value: bson.A{"$x", "$nope"}}}},
		{"RemoveWithSlice", nil, bson.D{{Key: "arr", Value: op("$slice", int32(1))}, {Key: "m", Value: op("$ifNull", bson.A{"$nope", "$$REMOVE"})}}},
		{"DottedWithSlice", nil, bson.D{{Key: "arr", Value: op("$slice", int32(1))}, {Key: "sub.r", Value: op("$toUpper", "$name")}}},
		{"EmptyObject", nil, m(bson.D{})},
		{"SizeMissing", bson.D{{Key: "_id", Value: int32(1)}}, m(bson.D{{Key: "$size", Value: "$nope"}})},
		{"SizeOfString", bson.D{{Key: "_id", Value: int32(1)}}, m(bson.D{{Key: "$size", Value: "$name"}})},
		{"UnknownOperator", nil, m(op("$notAnOperator", int32(1)))},
		{"TwoOperators", nil, m(bson.D{{Key: "$add", Value: bson.A{int32(1)}}, {Key: "$abs", Value: int32(1)}})},
		{"OperatorAndField", nil, m(bson.D{{Key: "$add", Value: bson.A{int32(1)}}, {Key: "f", Value: int32(1)}})},
		{"WrongArity", nil, m(op("$abs", bson.A{int32(1), int32(2)}))},
		{"RuntimeError", bson.D{{Key: "_id", Value: int32(1)}}, m(op("$divide", bson.A{int32(1), int32(0)}))},
		{"ToLongOfDate", bson.D{{Key: "_id", Value: int32(1)}}, m(op("$toLong", "$dt"))},
		{"Now", nil, m(op("$lte", bson.A{"$$NOW", op("$dateAdd", bson.D{{Key: "startDate", Value: "$$NOW"}, {Key: "unit", Value: "day"}, {Key: "amount", Value: int32(1)}})}))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			findExprCase(t, tc.name, tc.filter, tc.projection)
		})
	}
}

// $size fails unless its argument is an array, including null and missing.
func TestSizeOperand(t *testing.T) {
	for _, tc := range []struct {
		name string
		arg  interface{}
	}{
		{"Array", "$arr"},
		{"Missing", "$nope"},
		{"Null", nil},
		{"String", "$name"},
		{"Literal", bson.A{bson.A{int32(1), int32(2)}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "SizeOperand_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertFindExprDocs,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					results, err := runPipeline(ctx, col, []bson.D{
						{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
						{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: tc.arg}}}}}},
					})
					return docsToSlice(results), err
				},
			})
		})
	}
}
