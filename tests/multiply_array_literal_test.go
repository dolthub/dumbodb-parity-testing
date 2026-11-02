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
	"math"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $multiply's null, type and widening rules, and array literals evaluated as
// expressions in every operator that takes them.

func insertMultiplyDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "i", Value: int32(3)},
			{Key: "l", Value: int64(5)},
			{Key: "d", Value: 0.1},
			{Key: "dec", Value: mustDecimal("1.5")},
			{Key: "n", Value: nil},
			{Key: "s", Value: "x"},
			{Key: "arr", Value: bson.A{int32(1), int32(2)}},
			{Key: "date", Value: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
	})
	return err
}

func TestMultiplyAndArrayLiterals(t *testing.T) {
	mul := func(args ...interface{}) bson.D { return bson.D{{Key: "$multiply", Value: bson.A(args)}} }
	add := func(args ...interface{}) bson.D { return bson.D{{Key: "$add", Value: bson.A(args)}} }
	concat := func(args ...interface{}) bson.D { return bson.D{{Key: "$concatArrays", Value: bson.A(args)}} }
	cases := []mathCase{
		{"MultiplyInts", mul(int32(2), "$i")},
		{"MultiplyIntOverflowToLong", mul(int32(math.MaxInt32), int32(2))},
		{"MultiplyLongOverflowToDouble", mul(int64(math.MaxInt64), int32(2))},
		{"MultiplyLongs", mul("$l", int64(3))},
		{"MultiplyDouble", mul("$i", "$d")},
		{"MultiplyDecimal", mul("$i", "$dec")},
		{"MultiplyDoubleThenDecimal", mul("$d", int32(3), "$dec")},
		{"MultiplyDecimalThenDouble", mul("$dec", "$d")},
		{"MultiplyNaN", mul("$i", math.NaN())},
		{"MultiplyInfinity", mul(int64(2), math.Inf(1))},
		{"MultiplyNegativeZero", mul(int32(0), -1.0)},
		{"MultiplyEmpty", bson.D{{Key: "$multiply", Value: bson.A{}}}},
		{"MultiplySingleNonArray", bson.D{{Key: "$multiply", Value: "$i"}}},
		{"MultiplyNullLiteral", mul(int32(2), nil)},
		{"MultiplyNullField", mul(int32(2), "$n")},
		{"MultiplyMissingField", mul("$missing", int32(2))},
		{"MultiplyNullBeforeString", mul(nil, "x")},
		{"MultiplyStringBeforeNull", mul("$s", nil)},
		{"MultiplyString", mul(int32(2), "$s")},
		{"MultiplyBool", mul(int32(2), true)},
		{"MultiplyDate", mul(int32(2), "$date")},
		{"MultiplyArrayField", mul(int32(2), "$arr")},
		{"MultiplyArrayLiteral", mul(int32(2), bson.A{int32(1)})},
		{"MultiplyNestedNull", mul(add("$missing", int32(1)), 2.5)},
		{"MultiplyNestedValue", mul(add("$i", int32(1)), 2.5)},
		{"MultiplyNestedRemove", mul(bson.D{{Key: "$cond", Value: bson.A{false, int32(1), "$$REMOVE"}}}, int32(2))},
		{"MultiplyNestedMultiply", mul(mul("$i", int32(2)), "$l")},

		{"ConcatLiteralFieldPaths", concat(bson.A{"$i"}, bson.A{"$s", "$l"})},
		{"ConcatLiteralNestedOperator", concat(bson.A{add("$i", int32(1))}, "$arr")},
		{"ConcatLiteralMissingElement", concat(bson.A{"$missing", int32(1)})},
		{"ConcatLiteralNestedArray", concat(bson.A{bson.A{"$i"}}, bson.A{int32(2)})},
		{"ConcatFields", concat("$arr", "$arr")},
		{"ConcatEmpty", bson.D{{Key: "$concatArrays", Value: bson.A{}}}},
		{"ConcatSingleField", bson.D{{Key: "$concatArrays", Value: "$arr"}}},
		{"ConcatNullArgument", concat("$arr", nil)},
		{"ConcatMissingArgument", concat("$arr", "$missing")},
		{"ConcatNonArray", concat("$arr", "$s")},
		{"ConcatNonArrayBeforeNull", concat("$s", nil)},
		{"ConcatNullBeforeNonArray", concat(nil, "$s")},

		{"LiteralInProject", bson.A{"$i", add(int32(1), int32(2)), "$missing", bson.A{"$s"}}},
		{"LiteralArrayElemAt", bson.D{{Key: "$arrayElemAt", Value: bson.A{bson.A{"$i", "$s"}, int32(1)}}}},
		{"LiteralIn", bson.D{{Key: "$in", Value: bson.A{"$i", bson.A{"$l", "$i"}}}}},
		{"LiteralReverseArray", bson.D{{Key: "$reverseArray", Value: bson.A{bson.A{"$i", "$s", "$missing"}}}}},
		{"LiteralSize", bson.D{{Key: "$size", Value: bson.A{bson.A{"$i", "$missing"}}}}},
		{"LiteralSetUnion", bson.D{{Key: "$setUnion", Value: bson.A{bson.A{"$i"}, bson.A{"$i", int32(9)}}}}},
		{"LiteralFirst", bson.D{{Key: "$first", Value: bson.A{bson.A{"$s", "$i"}}}}},
		{"LiteralInsideLiteralOperator", bson.D{{Key: "$literal", Value: bson.A{"$i"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "MultiplyArrayLiteral_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertMultiplyDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}
