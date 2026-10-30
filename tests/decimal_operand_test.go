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
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// operators.toFloat64 read a Decimal128 as 0, so operators that convert
// operands with it could compute with 0 (workspace-ptx).
func TestDecimalOperands(t *testing.T) {
	d := mustDecimal
	op := func(name string, args ...interface{}) bson.D { return bson.D{{Key: name, Value: bson.A(args)}} }
	one := func(name string, arg interface{}) bson.D { return bson.D{{Key: name, Value: arg}} }
	arr := bson.D{{Key: "$literal", Value: bson.A{int32(10), int32(20), int32(30), int32(40)}}}
	cases := []mathCase{
		{"Subtract", op("$subtract", d("7.5"), int32(2))},
		{"SubtractDouble", op("$subtract", 7.5, d("2"))},
		{"Divide", op("$divide", int32(7), d("2"))},
		{"DivideByDecimalZero", op("$divide", int32(7), d("0"))},
		{"Mod", op("$mod", d("7.5"), int32(2))},
		{"ModDecimalDivisor", op("$mod", int32(7), d("2.5"))},
		{"Pow", op("$pow", d("2"), int32(10))},
		{"PowDecimalExponent", op("$pow", int32(2), d("0.5"))},
		{"Exp", one("$exp", d("1"))},
		{"Sqrt", one("$sqrt", d("2"))},
		{"SqrtNegative", one("$sqrt", d("-4"))},
		{"Ln", one("$ln", d("10"))},
		{"LnZero", one("$ln", d("0"))},
		{"Log", op("$log", d("100"), int32(10))},
		{"LogDecimalBase", op("$log", int32(100), d("10"))},
		{"LogBaseOne", op("$log", int32(100), d("1"))},
		{"Log10", one("$log10", d("1000"))},
		{"Log10Negative", one("$log10", d("-1"))},
		{"Round", op("$round", d("2.567"), int32(2))},
		{"RoundDecimalPlaces", op("$round", 2.567, d("2"))},
		{"Trunc", op("$trunc", d("-2.567"), d("1"))},
		{"Floor", one("$floor", d("-2.5"))},
		{"Ceil", one("$ceil", d("2.1"))},
		{"Abs", one("$abs", d("-2.5"))},
		{"Sin", one("$sin", d("1"))},
		{"Atan2", op("$atan2", d("1"), int32(1))},
		{"Atan2DecimalX", op("$atan2", int32(1), d("1"))},
		{"ArrayElemAt", op("$arrayElemAt", arr, d("2"))},
		{"ArrayElemAtFraction", op("$arrayElemAt", arr, d("1.5"))},
		{"Slice", op("$slice", arr, d("2"))},
		{"SliceStart", op("$slice", arr, d("1"), d("2"))},
		{"Range", op("$range", d("0"), int32(3))},
		{"RangeStep", op("$range", int32(0), int32(6), d("2"))},
		{"Substr", op("$substrBytes", "abcdef", d("1"), d("3"))},
		{"SubstrCP", op("$substrCP", "abcdef", d("2"), int32(2))},
		{"IndexOfArray", op("$indexOfArray", arr, int32(30), d("1"))},
		{"IndexOfBytes", op("$indexOfBytes", "abcabc", "c", d("3"))},
		{"Multiply", op("$multiply", d("1.5"), int32(2), 2.0)},
		{"Add", op("$add", d("1.5"), int32(2), 2.0)},
		{"FirstN", bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: arr}, {Key: "n", Value: d("2")}}}}},
		{"SortArrayLimitless", bson.D{{Key: "$sortArray", Value: bson.D{{Key: "input", Value: bson.A{d("2"), int32(1), 1.5}}, {Key: "sortBy", Value: 1}}}}},
		{"ToBool", one("$toBool", d("0"))},
		{"Cond", op("$cond", d("0"), "t", "f")},
		{"Not", one("$not", d("0.0"))},
	}
	pending := map[string]bool{"ArrayElemAtFraction": true} // index validation: workspace-ycs
	var full, xfail []mathCase
	for _, tc := range cases {
		if pending[tc.name] {
			xfail = append(xfail, tc)
		} else {
			full = append(full, tc)
		}
	}
	runMathCases(t, "DecimalOperands", harness.DumboDBXFail, full)
	runMathCases(t, "DecimalOperands", harness.DumboDBXFail, xfail)
}
