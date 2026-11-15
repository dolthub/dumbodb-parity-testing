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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Arithmetic errors and math operator results: MongoDB's codes for division
// and modulo by zero and non-numeric operands, and its results, types and
// errors for the math operators. Each case is its own pair test so a
// divergence can be marked XFail on its own.

func insertArithmeticDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 300)
	for i := range docs {
		d := 1
		if i == 0 || i == 250 {
			d = 0
		}
		docs[i] = bson.D{{Key: "_id", Value: i}, {Key: "v", Value: i + 7}, {Key: "d", Value: d}, {Key: "s", Value: "x"}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func drainCursor(ctx context.Context, cursor *mongo.Cursor, err error) (interface{}, error) {
	if err != nil {
		return errOutcome(err), nil
	}
	var docs []bson.M
	if err = cursor.All(ctx, &docs); err != nil {
		return errOutcome(err), nil
	}
	return docs, nil
}

// projectFirst evaluates expr against the first document only.
func projectFirst(expr interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: 0}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}},
		})
		return drainCursor(ctx, cursor, err)
	}
}

// projectTyped is projectFirst returning the result's BSON type with its value,
// so an int/long/double difference is not compared away.
func projectTyped(expr interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: 0}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}},
		})
		if err != nil {
			return errOutcome(err), nil
		}
		var docs []bson.Raw
		if err = cursor.All(ctx, &docs); err != nil {
			return errOutcome(err), nil
		}
		value := docs[0].Lookup("r")
		return bson.M{"type": value.Type.String(), "value": value.String()}, nil
	}
}

func pipelineRun(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, pipeline)
		return drainCursor(ctx, cursor, err)
	}
}

func TestArithmeticErrors(t *testing.T) {
	dec0, err := primitive.ParseDecimal128("0")
	if err != nil {
		t.Fatal(err)
	}
	decNegOne, err := primitive.ParseDecimal128("-1")
	if err != nil {
		t.Fatal(err)
	}
	op := func(name string, arg interface{}) bson.D { return bson.D{{Key: name, Value: arg}} }
	dec := func(s string) primitive.Decimal128 {
		d, err := primitive.ParseDecimal128(s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	divide := func(a, b interface{}) bson.D { return bson.D{{Key: "$divide", Value: bson.A{a, b}}} }
	mod := func(a, b interface{}) bson.D { return bson.D{{Key: "$mod", Value: bson.A{a, b}}} }

	cases := []struct {
		name    string
		support harness.DumboDBSupport
		run     func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"Divide_ZeroField", harness.DumboDBFull, projectFirst(divide("$v", "$d"))},
		{"Divide_ZeroInt", harness.DumboDBFull, projectFirst(divide("$v", 0))},
		{"Divide_ZeroLong", harness.DumboDBFull, projectFirst(divide("$v", int64(0)))},
		{"Divide_ZeroDouble", harness.DumboDBFull, projectFirst(divide("$v", 0.0))},
		{"Divide_NegativeZeroDouble", harness.DumboDBFull, projectFirst(divide("$v", -0.0))},
		{"Divide_ZeroDecimal", harness.DumboDBFull, projectFirst(divide("$v", dec0))},
		{"Divide_ZeroByZeroDouble", harness.DumboDBFull, projectFirst(divide(0.0, 0.0))},
		{"Divide_StringDivisor", harness.DumboDBFull, projectFirst(divide("$v", "$s"))},
		{"Divide_NullDivisor", harness.DumboDBFull, projectFirst(divide("$v", nil))},
		{"Divide_MissingDivisor", harness.DumboDBFull, projectFirst(divide("$v", "$missing"))},
		{"Mod_ZeroInt", harness.DumboDBFull, projectFirst(mod("$v", 0))},
		{"Mod_ZeroLong", harness.DumboDBFull, projectFirst(mod("$v", int64(0)))},
		{"Mod_ZeroDouble", harness.DumboDBFull, projectFirst(mod("$v", 0.0))},
		{"Mod_NegativeZeroDouble", harness.DumboDBFull, projectFirst(mod("$v", -0.0))},
		{"Mod_ZeroDecimal", harness.DumboDBFull, projectFirst(mod("$v", dec0))},
		{"Mod_StringDivisor", harness.DumboDBFull, projectFirst(mod("$v", "$s"))},
		{"Mod_NullDivisor", harness.DumboDBFull, projectFirst(mod("$v", nil))},
		{"Divide_ZeroAfterFirstBatch", harness.DumboDBFull, pipelineRun(bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: divide("$v", "$d")}}}},
		})},
		{"Divide_InAddFields", harness.DumboDBFull, pipelineRun(bson.A{
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: divide("$v", 0)}}}},
		})},
		// MongoDB evaluates $group expressions in its SBE engine, whose error
		// codes differ (dolthub/dumbodb#125).
		{"Divide_InGroupAccumulator", harness.DumboDBFull, pipelineRun(bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$sum", Value: divide("$v", 0)}}}}}},
		})},
		{"Divide_InFindExpr", harness.DumboDBFull, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{divide("$v", "$d"), 0}}}}})
			return drainCursor(ctx, cursor, err)
		}},
		{"Divide_InUpdatePipeline", harness.DumboDBFull, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: 0}}, bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "r", Value: divide("$v", 0)}}}}})
			return errOutcome(err), nil
		}},
		{"Mod_QueryOperatorZeroDivisor", harness.DumboDBFull, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "v", Value: bson.D{{Key: "$mod", Value: bson.A{0, 0}}}}})
			return drainCursor(ctx, cursor, err)
		}},

		{"Math_SubtractLongOverflow", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{int64(math.MinInt64), int64(1)}))},
		{"Math_AbsMinInt", harness.DumboDBFull, projectTyped(op("$abs", int32(math.MinInt32)))},
		{"Math_AbsMinLong", harness.DumboDBFull, projectTyped(op("$abs", int64(math.MinInt64)))},
		{"Math_SqrtString", harness.DumboDBFull, projectTyped(op("$sqrt", "x"))},
		{"Math_PowString", harness.DumboDBFull, projectTyped(op("$pow", bson.A{"x", 2}))},
		{"Math_TruncString", harness.DumboDBFull, projectTyped(op("$trunc", "x"))},
		{"Math_LogString", harness.DumboDBFull, projectTyped(op("$log", bson.A{"x", 10}))},
		{"Math_SqrtNegative", harness.DumboDBFull, projectTyped(op("$sqrt", -1))},
		{"Math_SqrtNegativeDecimal", harness.DumboDBFull, projectTyped(op("$sqrt", decNegOne))},
		{"Math_LnZero", harness.DumboDBFull, projectTyped(op("$ln", 0))},
		{"Math_LnNegative", harness.DumboDBFull, projectTyped(op("$ln", -1))},
		{"Math_LnNegativeDecimal", harness.DumboDBFull, projectTyped(op("$ln", decNegOne))},
		{"Math_LogZero", harness.DumboDBFull, projectTyped(op("$log", bson.A{0, 10}))},
		{"Math_LogNegative", harness.DumboDBFull, projectTyped(op("$log", bson.A{-1, 10}))},
		{"Math_LogBaseOne", harness.DumboDBFull, projectTyped(op("$log", bson.A{10, 1}))},
		{"Math_LogBaseNegative", harness.DumboDBFull, projectTyped(op("$log", bson.A{10, -2}))},
		{"Math_Log10Zero", harness.DumboDBFull, projectTyped(op("$log10", 0))},
		{"Math_Log10Negative", harness.DumboDBFull, projectTyped(op("$log10", -1))},
		{"Math_PowZeroNegativeExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{0, -1}))},
		{"Math_PowZeroDoubleNegativeExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{0.0, -1}))},
		{"Math_PowIntBeyondInt32", harness.DumboDBFull, projectTyped(op("$pow", bson.A{2, 40}))},
		{"Math_RoundPlaceOutOfRange", harness.DumboDBFull, projectTyped(op("$round", bson.A{1.5, 200}))},
		{"Math_AddIntOverflow", harness.DumboDBFull, projectTyped(op("$add", bson.A{int32(math.MaxInt32), int32(1)}))},
		{"Math_AddLongOverflow", harness.DumboDBFull, projectTyped(op("$add", bson.A{int64(math.MaxInt64), int64(1)}))},
		{"Math_MultiplyIntOverflow", harness.DumboDBFull, projectTyped(op("$multiply", bson.A{int32(math.MaxInt32), int32(2)}))},
		{"Math_MultiplyLongOverflow", harness.DumboDBFull, projectTyped(op("$multiply", bson.A{int64(math.MaxInt64), int64(2)}))},
		{"Math_PowLongOverflow", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int64(2), 70}))},
		{"Math_ExpOverflow", harness.DumboDBFull, projectTyped(op("$exp", 1000))},

		{"Math_AbsDecimal", harness.DumboDBFull, projectTyped(op("$abs", dec("-2.5")))},
		{"Math_AbsDouble", harness.DumboDBFull, projectTyped(op("$abs", -2.5))},
		{"Math_AbsString", harness.DumboDBFull, projectTyped(op("$abs", "x"))},
		{"Math_CeilString", harness.DumboDBFull, projectTyped(op("$ceil", "x"))},
		{"Math_FloorString", harness.DumboDBFull, projectTyped(op("$floor", "x"))},
		{"Math_CeilDecimal", harness.DumboDBFull, projectTyped(op("$ceil", dec("2.1")))},
		{"Math_FloorDecimal", harness.DumboDBFull, projectTyped(op("$floor", dec("-2.1")))},
		{"Math_SqrtInt", harness.DumboDBFull, projectTyped(op("$sqrt", int32(4)))},
		{"Math_SqrtDecimal", harness.DumboDBFull, projectTyped(op("$sqrt", dec("4")))},
		{"Math_SqrtDecimalInexact", harness.DumboDBFull, projectTyped(op("$sqrt", dec("2")))},
		{"Math_ExpDecimal", harness.DumboDBFull, projectTyped(op("$exp", dec("1")))},
		{"Math_ExpString", harness.DumboDBFull, projectTyped(op("$exp", "x"))},
		{"Math_LnDecimal", harness.DumboDBFull, projectTyped(op("$ln", dec("1")))},
		{"Math_LnDecimalInexact", harness.DumboDBFull, projectTyped(op("$ln", dec("10")))},
		{"Math_Log10Decimal", harness.DumboDBFull, projectTyped(op("$log10", dec("100")))},
		{"Math_Log10String", harness.DumboDBFull, projectTyped(op("$log10", "x"))},
		{"Math_LnString", harness.DumboDBFull, projectTyped(op("$ln", "x"))},
		{"Math_LogDecimal", harness.DumboDBFull, projectTyped(op("$log", bson.A{dec("100"), 10}))},
		{"Math_LogBaseZero", harness.DumboDBFull, projectTyped(op("$log", bson.A{10, 0}))},
		{"Math_LogStringBase", harness.DumboDBFull, projectTyped(op("$log", bson.A{10, "x"}))},
		{"Math_PowStringExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{2, "x"}))},
		{"Math_PowIntFitsInt", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int32(2), int32(10)}))},
		{"Math_PowIntNegativeExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int32(2), int32(-1)}))},
		{"Math_PowOneNegativeExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int32(1), int32(-5)}))},
		{"Math_PowMinusOneNegativeExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int32(-1), int32(-5)}))},
		{"Math_PowLongToMinLong", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int64(-2), int32(63)}))},
		{"Math_PowFractionalExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{int32(2), 0.5}))},
		{"Math_PowDecimal", harness.DumboDBFull, projectTyped(op("$pow", bson.A{dec("2"), int32(3)}))},
		{"Math_PowDecimalZeroNegativeExponent", harness.DumboDBFull, projectTyped(op("$pow", bson.A{dec("0"), int32(-1)}))},
		{"Math_TruncPlaceOutOfRange", harness.DumboDBFull, projectTyped(op("$trunc", bson.A{1.5, 200}))},
		{"Math_TruncPlaceBelowRange", harness.DumboDBFull, projectTyped(op("$trunc", bson.A{1.5, -21}))},
		{"Math_TruncPlaceString", harness.DumboDBFull, projectTyped(op("$trunc", bson.A{1.5, "x"}))},
		{"Math_TruncDecimal", harness.DumboDBFull, projectTyped(op("$trunc", dec("2.7")))},
		{"Math_TruncIntNegativePlace", harness.DumboDBFull, projectTyped(op("$trunc", bson.A{int32(1299), -2}))},
		{"Math_RoundString", harness.DumboDBFull, projectTyped(op("$round", "x"))},
		{"Math_RoundPlaceAtLimit", harness.DumboDBFull, projectTyped(op("$round", bson.A{1.5, 100}))},
		{"Math_RoundPlaceLowerLimit", harness.DumboDBFull, projectTyped(op("$round", bson.A{1234.5, -20}))},
		{"Math_RoundPlaceFractional", harness.DumboDBFull, projectTyped(op("$round", bson.A{1.55, 1.5}))},
		{"Math_RoundDecimalHalfEven", harness.DumboDBFull, projectTyped(op("$round", dec("2.5")))},
		{"Math_RoundIntNegativePlace", harness.DumboDBFull, projectTyped(op("$round", bson.A{int32(1250), -2}))},
		{"Math_RoundLongNegativePlace", harness.DumboDBFull, projectTyped(op("$round", bson.A{int64(1350), -2}))},
		{"Math_SubtractIntOverflow", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{int32(math.MinInt32), int32(1)}))},
		{"Math_SubtractLongOverflowPositive", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{int64(math.MaxInt64), int64(-1)}))},
		{"Math_SubtractDecimal", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{dec("5.5"), int32(1)}))},
		{"Math_SubtractDates", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{
			primitive.NewDateTimeFromTime(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)),
			primitive.NewDateTimeFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)),
		}))},
		{"Math_SubtractNumberFromDate", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{
			primitive.NewDateTimeFromTime(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)), int64(1000),
		}))},
		{"Math_SubtractString", harness.DumboDBFull, projectTyped(op("$subtract", bson.A{"x", 1}))},
	}

	for _, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "Arithmetic_" + tc.name,
			Support: tc.support,
			Setup:   insertArithmeticDocs,
			Run:     tc.run,
		})
	}
}
