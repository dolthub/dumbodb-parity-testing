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

// Trigonometric and angle conversion operators. Each case is its own pair
// test, compared by BSON type and exact value, so a divergence of a single
// ULP or an int/double/decimal difference is visible.

type mathCase struct {
	name string
	expr interface{}
}

func insertTrigDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "d", Value: 0.5},
			{Key: "i", Value: int32(1)},
			{Key: "l", Value: int64(-1)},
			{Key: "dec", Value: mustDecimal("0.25")},
			{Key: "n", Value: nil},
			{Key: "s", Value: "x"},
		},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "d", Value: -0.75}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "d", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "d", Value: nil}},
		bson.D{{Key: "_id", Value: int32(4)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "d", Value: mustDecimal("0.1")}},
	})
	return err
}

// unaryMathInputs are the inputs every single-argument math operator is
// checked against, in addition to the operator's own cases.
func unaryMathInputs(op string) []mathCase {
	un := func(arg interface{}) bson.D { return bson.D{{Key: op, Value: arg}} }
	return []mathCase{
		{"IntZero", un(int32(0))},
		{"IntOne", un(int32(1))},
		{"IntNegative", un(int32(-1))},
		{"Long", un(int64(1))},
		{"LongLarge", un(int64(1) << 53)},
		{"DoubleHalf", un(0.5)},
		{"DoubleNegHalf", un(-0.5)},
		{"DoubleNegativeZero", un(math.Copysign(0, -1))},
		{"DoubleTiny", un(1e-300)},
		{"DoubleLarge", un(1e300)},
		{"DoubleNaN", un(math.NaN())},
		{"DoublePosInf", un(math.Inf(1))},
		{"DoubleNegInf", un(math.Inf(-1))},
		{"DecimalHalf", un(mustDecimal("0.5"))},
		{"DecimalNegHalf", un(mustDecimal("-0.5"))},
		{"DecimalOne", un(mustDecimal("1"))},
		{"DecimalZero", un(mustDecimal("0"))},
		{"DecimalNegZero", un(mustDecimal("-0"))},
		{"DecimalPrecise", un(mustDecimal("0.1234567890123456789012345678901234"))},
		{"DecimalLarge", un(mustDecimal("1E+30"))},
		{"DecimalTiny", un(mustDecimal("1E-40"))},
		{"DecimalNegTiny", un(mustDecimal("-7E-60"))},
		{"DecimalSmall", un(mustDecimal("1.234E-37"))},
		{"DecimalZeroSmallExponent", un(mustDecimal("0E-50"))},
		{"DecimalNaN", un(mustDecimal("NaN"))},
		{"DecimalPosInf", un(mustDecimal("Infinity"))},
		{"DecimalNegInf", un(mustDecimal("-Infinity"))},
		{"FieldDouble", un("$d")},
		{"FieldInt", un("$i")},
		{"FieldLong", un("$l")},
		{"FieldDecimal", un("$dec")},
		{"FieldNull", un("$n")},
		{"FieldMissing", un("$missing")},
		{"LiteralNull", un(nil)},
		{"SingleElementArgList", un(bson.A{0.5})},
		{"NullInArgList", un(bson.A{nil})},
		{"NestedExpression", un(bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(4)}}})},
		{"String", un("x")},
		{"FieldString", un("$s")},
		{"Bool", un(true)},
		{"Date", un(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))},
		{"ObjectID", un(primitive.NewObjectIDFromTimestamp(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)))},
		{"Object", un(bson.D{{Key: "a", Value: int32(1)}})},
		{"ArrayValue", un(bson.A{bson.A{0.5}})},
		{"NoArgs", un(bson.A{})},
		{"TwoArgs", un(bson.A{0.5, 0.5})},
	}
}

func runMathCases(t *testing.T, prefix string, support harness.DumboDBSupport, cases []mathCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    prefix + "_" + tc.name,
				Support: support,
				Setup:   insertTrigDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}

// runMathContexts checks op over every document in $addFields, inside a
// $match $expr, and under a $group accumulator.
// sbeGroupDivergences are cases where MongoDB evaluates $group in its SBE
// engine, whose out-of-bounds error code differs (workspace-tfj.16).
var sbeGroupDivergences = map[string]bool{
	"Trig_acosh_GroupSum": true,
}

func runMathContexts(t *testing.T, prefix, op string, support harness.DumboDBSupport) {
	expr := bson.D{{Key: op, Value: "$d"}}
	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AddFieldsAllDocs", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: 0}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: expr}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{1, 2, 3, 4, 5}}}}}}},
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{expr, 0.6}}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupSum", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{1, 2, 3, 4}}}}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "s", Value: bson.D{{Key: "$sum", Value: expr}}}}}},
		}},
	}
	for _, p := range pipelines {
		name := prefix + "_" + p.name
		caseSupport := support
		if sbeGroupDivergences[name] {
			caseSupport = harness.DumboDBXFail
		}
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    name,
				Support: caseSupport,
				Setup:   insertTrigDocs,
				Run:     pipelineRun(p.pipeline),
			})
		})
	}
}

func runUnaryMathOperator(t *testing.T, op string, support harness.DumboDBSupport, extra ...mathCase) {
	prefix := "Trig_" + op[1:]
	runMathCases(t, prefix, support, append(unaryMathInputs(op), extra...))
	runMathContexts(t, prefix, op, support)
}

func TestTrig_SinCosTan(t *testing.T) {
	for _, op := range []string{"$sin", "$cos", "$tan"} {
		un := func(arg interface{}) bson.D { return bson.D{{Key: op, Value: arg}} }
		t.Run(op[1:], func(t *testing.T) {
			runUnaryMathOperator(t, op, harness.DumboDBFull,
				mathCase{"Pi", un(math.Pi)},
				mathCase{"HalfPi", un(math.Pi / 2)},
				mathCase{"TwoPi", un(2 * math.Pi)},
				mathCase{"NegThreeQuarterPi", un(-3 * math.Pi / 4)},
				mathCase{"IntLarge", un(int32(math.MaxInt32))},
				mathCase{"LongMax", un(int64(math.MaxInt64))},
				mathCase{"DoubleHuge", un(1e22)},
				mathCase{"DoubleMax", un(math.MaxFloat64)},
				mathCase{"DecimalPi", un(mustDecimal("3.141592653589793238462643383279503"))},
				mathCase{"DecimalHalfPi", un(mustDecimal("1.570796326794896619231321691639751"))},
				mathCase{"DecimalHuge", un(mustDecimal("1E+6000"))},
			)
		})
	}
}

func TestTrig_InverseSinCosTan(t *testing.T) {
	for _, op := range []string{"$asin", "$acos", "$atan"} {
		un := func(arg interface{}) bson.D { return bson.D{{Key: op, Value: arg}} }
		t.Run(op[1:], func(t *testing.T) {
			runUnaryMathOperator(t, op, harness.DumboDBFull,
				mathCase{"DoubleOne", un(1.0)},
				mathCase{"DoubleNegOne", un(-1.0)},
				mathCase{"DoubleJustBelowOne", un(math.Nextafter(1, 0))},
				mathCase{"DoubleJustAboveOne", un(math.Nextafter(1, 2))},
				mathCase{"DoubleJustBelowNegOne", un(math.Nextafter(-1, -2))},
				mathCase{"IntTwo", un(int32(2))},
				mathCase{"LongNegTwo", un(int64(-2))},
				mathCase{"DoubleSqrtHalf", un(math.Sqrt2 / 2)},
				mathCase{"DecimalNinesBelowOne", un(mustDecimal("0.9999999999999999999999999999999999"))},
				mathCase{"DecimalJustAboveOne", un(mustDecimal("1.000000000000000000000000000000001"))},
				mathCase{"DecimalNegOne", un(mustDecimal("-1"))},
				mathCase{"DecimalOneTrailingZeros", un(mustDecimal("1.000"))},
				mathCase{"DecimalThird", un(mustDecimal("0.3333333333333333333333333333333333"))},
				mathCase{"DecimalTwo", un(mustDecimal("2"))},
				mathCase{"DoubleHuge", un(1e22)},
			)
		})
	}
}

func TestTrig_Atan2(t *testing.T) {
	atan2 := func(y, x interface{}) bson.D { return bson.D{{Key: "$atan2", Value: bson.A{y, x}}} }
	negZero := math.Copysign(0, -1)
	cases := []mathCase{
		{"IntInt", atan2(int32(1), int32(1))},
		{"IntLong", atan2(int32(-1), int64(2))},
		{"LongLong", atan2(int64(3), int64(-4))},
		{"DoubleQuadrant1", atan2(0.5, 2.0)},
		{"DoubleQuadrant2", atan2(0.5, -2.0)},
		{"DoubleQuadrant3", atan2(-0.5, -2.0)},
		{"DoubleQuadrant4", atan2(-0.5, 2.0)},
		{"PosZeroPosZero", atan2(0.0, 0.0)},
		{"PosZeroNegZero", atan2(0.0, negZero)},
		{"NegZeroPosZero", atan2(negZero, 0.0)},
		{"NegZeroNegZero", atan2(negZero, negZero)},
		{"ZeroNegative", atan2(0.0, -1.0)},
		{"PositiveZero", atan2(1.0, 0.0)},
		{"NegativeZero", atan2(-1.0, 0.0)},
		{"IntZeroIntZero", atan2(int32(0), int32(0))},
		{"PosInfPosInf", atan2(math.Inf(1), math.Inf(1))},
		{"PosInfNegInf", atan2(math.Inf(1), math.Inf(-1))},
		{"NegInfFinite", atan2(math.Inf(-1), 1.0)},
		{"FinitePosInf", atan2(1.0, math.Inf(1))},
		{"FiniteNegInf", atan2(-1.0, math.Inf(-1))},
		{"NaNY", atan2(math.NaN(), 1.0)},
		{"NaNX", atan2(1.0, math.NaN())},
		{"Tiny", atan2(1e-300, 1e300)},
		{"DecimalDecimal", atan2(mustDecimal("1"), mustDecimal("3"))},
		{"DecimalDouble", atan2(mustDecimal("0.5"), 2.0)},
		{"DoubleDecimal", atan2(0.5, mustDecimal("-2"))},
		{"DecimalInt", atan2(mustDecimal("-1"), int32(-1))},
		{"DecimalZeros", atan2(mustDecimal("0"), mustDecimal("-0"))},
		{"DecimalNegZeros", atan2(mustDecimal("-0"), mustDecimal("-0"))},
		{"DecimalZeroOne", atan2(mustDecimal("0E-10"), mustDecimal("1"))},
		{"DecimalInf", atan2(mustDecimal("Infinity"), mustDecimal("-Infinity"))},
		{"DecimalNaN", atan2(mustDecimal("NaN"), mustDecimal("1"))},
		{"DecimalTiny", atan2(mustDecimal("1E-40"), mustDecimal("1"))},
		{"DecimalPrecise", atan2(mustDecimal("0.1234567890123456789012345678901234"), mustDecimal("0.9876543210987654321098765432109876"))},
		{"DecimalHuge", atan2(mustDecimal("1E+6000"), mustDecimal("1E-6000"))},
		{"Fields", atan2("$d", "$i")},
		{"FieldDecimal", atan2("$dec", "$l")},
		{"NullY", atan2(nil, 1.0)},
		{"NullX", atan2(1.0, nil)},
		{"MissingY", atan2("$missing", 1.0)},
		{"MissingX", atan2(1.0, "$missing")},
		{"NullBoth", atan2("$n", "$missing")},
		{"StringY", atan2("x", 1.0)},
		{"StringX", atan2(1.0, "x")},
		{"StringBoth", atan2("$s", "$s")},
		{"NullYStringX", atan2(nil, "x")},
		{"StringYNullX", atan2("x", nil)},
		{"BoolX", atan2(1.0, true)},
		{"ArrayY", atan2(bson.A{1.0}, 1.0)},
		{"NestedExpression", atan2(bson.D{{Key: "$add", Value: bson.A{int32(1), int32(2)}}}, bson.D{{Key: "$sqrt", Value: int32(2)}})},
		{"OneArg", bson.D{{Key: "$atan2", Value: bson.A{1.0}}}},
		{"ThreeArgs", bson.D{{Key: "$atan2", Value: bson.A{1.0, 1.0, 1.0}}}},
		{"NonArrayArg", bson.D{{Key: "$atan2", Value: 1.0}}},
	}
	runMathCases(t, "Trig_atan2", harness.DumboDBFull, cases)
}

func TestTrig_Hyperbolic(t *testing.T) {
	for _, op := range []string{"$sinh", "$cosh", "$tanh"} {
		un := func(arg interface{}) bson.D { return bson.D{{Key: op, Value: arg}} }
		t.Run(op[1:], func(t *testing.T) {
			runUnaryMathOperator(t, op, harness.DumboDBFull,
				mathCase{"IntTwenty", un(int32(20))},
				mathCase{"IntNegTwenty", un(int32(-20))},
				mathCase{"DoubleSmall", un(1e-9)},
				mathCase{"DoubleNearOverflow", un(710.0)},
				mathCase{"DoubleOverflow", un(711.0)},
				mathCase{"DoubleNegOverflow", un(-711.0)},
				mathCase{"LongMax", un(int64(math.MaxInt64))},
				mathCase{"DoubleThirtyThree", un(33.0)},
				mathCase{"DecimalTwenty", un(mustDecimal("20"))},
				mathCase{"DecimalNegForty", un(mustDecimal("-40"))},
				mathCase{"DecimalNearOverflow", un(mustDecimal("14000"))},
				mathCase{"DecimalOverflow", un(mustDecimal("15000"))},
				mathCase{"DecimalNegOverflow", un(mustDecimal("-15000"))},
				mathCase{"DecimalBinaryOverflow", un(mustDecimal("11400"))},
				mathCase{"DecimalThirtyEightPointFive", un(mustDecimal("38.5"))},
				mathCase{"DecimalSmall", un(mustDecimal("1E-20"))},
				mathCase{"DecimalOneTrailingZeros", un(mustDecimal("1.0000"))},
				mathCase{"DecimalNegNaN", un(mustDecimal("-NaN"))},
			)
		})
	}
}

func TestTrig_InverseHyperbolic(t *testing.T) {
	for _, op := range []string{"$asinh", "$acosh", "$atanh"} {
		un := func(arg interface{}) bson.D { return bson.D{{Key: op, Value: arg}} }
		t.Run(op[1:], func(t *testing.T) {
			runUnaryMathOperator(t, op, harness.DumboDBFull,
				mathCase{"DoubleOne", un(1.0)},
				mathCase{"DoubleNegOne", un(-1.0)},
				mathCase{"DoubleJustBelowOne", un(math.Nextafter(1, 0))},
				mathCase{"DoubleJustAboveOne", un(math.Nextafter(1, 2))},
				mathCase{"DoubleJustAboveNegOne", un(math.Nextafter(-1, 0))},
				mathCase{"IntTwo", un(int32(2))},
				mathCase{"LongNegTwo", un(int64(-2))},
				mathCase{"DoubleOnePointFive", un(1.5)},
				mathCase{"DoubleHuge", un(1e22)},
				mathCase{"DoubleMax", un(math.MaxFloat64)},
				mathCase{"DoubleNineTenths", un(0.9)},
				mathCase{"DecimalNinesBelowOne", un(mustDecimal("0.9999999999999999999999999999999999"))},
				mathCase{"DecimalJustAboveOne", un(mustDecimal("1.000000000000000000000000000000001"))},
				mathCase{"DecimalNegOne", un(mustDecimal("-1"))},
				mathCase{"DecimalOneTrailingZeros", un(mustDecimal("1.000"))},
				mathCase{"DecimalTwo", un(mustDecimal("2"))},
				mathCase{"DecimalHuge", un(mustDecimal("1E+6000"))},
				mathCase{"DecimalNineTenths", un(mustDecimal("0.9"))},
				mathCase{"DecimalThird", un(mustDecimal("0.3333333333333333333333333333333333"))},
			)
		})
	}
}

func TestTrig_AngleConversion(t *testing.T) {
	for _, op := range []string{"$degreesToRadians", "$radiansToDegrees"} {
		un := func(arg interface{}) bson.D { return bson.D{{Key: op, Value: arg}} }
		t.Run(op[1:], func(t *testing.T) {
			runUnaryMathOperator(t, op, harness.DumboDBXFail,
				mathCase{"Int180", un(int32(180))},
				mathCase{"IntNeg90", un(int32(-90))},
				mathCase{"Long360", un(int64(360))},
				mathCase{"DoubleNeg45", un(-45.0)},
				mathCase{"DoublePi", un(math.Pi)},
				mathCase{"DoubleHalfPi", un(math.Pi / 2)},
				mathCase{"DoubleMax", un(math.MaxFloat64)},
				mathCase{"DoubleNegMax", un(-math.MaxFloat64)},
				mathCase{"DoubleSmallest", un(math.SmallestNonzeroFloat64)},
				mathCase{"IntMax", un(int32(math.MaxInt32))},
				mathCase{"LongMax", un(int64(math.MaxInt64))},
				mathCase{"Decimal180", un(mustDecimal("180"))},
				mathCase{"Decimal180TrailingZeros", un(mustDecimal("180.000"))},
				mathCase{"DecimalPi", un(mustDecimal("3.141592653589793238462643383279503"))},
				mathCase{"DecimalMax", un(mustDecimal("9.999999999999999999999999999999999E+6144"))},
				mathCase{"DecimalMin", un(mustDecimal("1E-6176"))},
				mathCase{"DecimalThird", un(mustDecimal("0.3333333333333333333333333333333333"))},
			)
		})
	}
}
