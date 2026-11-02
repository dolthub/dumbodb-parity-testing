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
	"math"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func addOf(args ...interface{}) bson.D {
	// bson.A(nil) would encode as null, not an empty array.
	return bson.D{{Key: "$add", Value: append(bson.A{}, args...)}}
}

func TestAddOperator(t *testing.T) {
	date := time.Date(2021, 7, 15, 13, 45, 30, 123e6, time.UTC)
	preEpoch := primitive.DateTime(-1500)

	cases := []mathCase{
		{"Empty", addOf()},
		{"SingleInt", addOf(int32(5))},
		{"SingleNonArray", bson.D{{Key: "$add", Value: int32(5)}}},
		{"IntInt", addOf(int32(1), int32(2))},
		{"IntOverflowToLong", addOf(int32(math.MaxInt32), int32(1))},
		{"IntLong", addOf(int32(1), int64(2))},
		{"LongOverflowToDouble", addOf(int64(math.MaxInt64), int64(1))},
		{"LongUnderflowToDouble", addOf(int64(math.MinInt64), int64(-1))},
		{"OverflowThenBack", addOf(int64(math.MaxInt64), int64(1), int64(-1))},
		{"IntDouble", addOf(int32(1), 0.5)},
		{"DoubleInt", addOf(0.5, int32(1))},
		{"LongDouble", addOf(int64(1)<<53, 1.0)},
		{"DoubleDecimal", addOf(0.1, mustDecimal("0.2"))},
		{"LongDecimal", addOf(int64(math.MaxInt64), mustDecimal("1"))},
		{"DecimalPrecision", addOf(mustDecimal("0.1"), mustDecimal("0.2"))},
		{"NaN", addOf(math.NaN(), int32(1))},
		{"Infinity", addOf(math.Inf(1), math.Inf(-1))},
		{"NegativeZero", addOf(math.Copysign(0, -1), math.Copysign(0, -1))},
		{"Nested", addOf(int32(1), addOf(int32(2), 0.5))},
		{"Fields", addOf("$y", "$m", "$d", "$dec")},

		{"NullFirst", addOf(nil, int32(1))},
		{"NullLast", addOf(int32(1), nil)},
		{"Missing", addOf("$missing", int32(1))},
		{"NullField", addOf("$n", int32(1))},
		{"NullBeforeString", addOf(nil, "x")},
		{"StringBeforeNull", addOf("x", nil)},
		{"String", addOf(int32(1), "x")},
		{"StringField", addOf(int32(1), "$s")},
		{"Bool", addOf(int32(1), true)},
		{"Object", addOf(int32(1), bson.D{{Key: "a", Value: int32(1)}})},
		{"Array", addOf(int32(1), bson.A{int32(1)})},
		{"Timestamp", addOf(primitive.Timestamp{T: 1, I: 1}, int32(1))},
		{"ObjectID", addOf(primitive.NewObjectIDFromTimestamp(date), int32(1))},

		{"DateInt", addOf(date, int32(1000))},
		{"IntDate", addOf(int32(1000), date)},
		{"DateLong", addOf(date, int64(86400000))},
		{"DateField", addOf("$date", int32(1))},
		{"DateOnly", addOf(date)},
		{"DateNegative", addOf(date, int32(-1000))},
		{"PreEpochDate", addOf(preEpoch, int32(1))},
		{"DateDoubleHalf", addOf(date, 1.5)},
		{"DateDoubleNegativeHalf", addOf(date, -1.5)},
		{"DateDoubleTwoAndHalf", addOf(date, 2.5)},
		{"DateDoubleSmall", addOf(date, 0.4)},
		{"DateDoublesAfter", addOf(date, 1.5, 1.5)},
		{"DoublesBeforeDate", addOf(1.5, 1.5, date)},
		{"DoublesBeforeDateField", addOf(1.5, 1.5, "$date")},
		{"MixedBeforeDate", addOf(int32(1), 0.5, "$date")},
		{"DateDecimal", addOf(date, mustDecimal("1000"))},
		{"DateDecimalTrailingZeros", addOf(date, mustDecimal("1.000"))},
		{"DateDecimalFraction", addOf(date, mustDecimal("1.5"))},
		{"DecimalFractionBeforeDate", addOf(mustDecimal("1.5"), "$date")},
		{"DecimalBeforeDate", addOf(mustDecimal("2"), "$date")},
		{"DateDecimalNaN", addOf(date, mustDecimal("NaN"))},
		{"DateDecimalHuge", addOf(date, mustDecimal("1E20"))},
		{"TwoDates", addOf(date, date)},
		{"TwoDatesWithNumber", addOf(date, int32(1), "$date")},
		{"DateNull", addOf(date, nil)},
		{"DateString", addOf(date, "x")},
		{"DateTimestamp", addOf(date, primitive.Timestamp{T: 1, I: 1})},
		{"DateNaN", addOf(date, math.NaN())},
		{"DateInfinity", addOf(date, math.Inf(1))},
		{"DateMaxLong", addOf(date, int64(math.MaxInt64))},
		{"DateMinLong", addOf(preEpoch, int64(math.MinInt64))},
		{"DateHugeDouble", addOf(date, 1e19)},
		{"DateMaxLongAsDouble", addOf(date, float64(math.MaxInt64))},
		{"DateMinLongAsDouble", addOf(primitive.DateTime(0), float64(math.MinInt64))},
		{"OverflowedSumBeforeDate", addOf(int64(math.MaxInt64), int64(1), "$date")},
		{"LargeDoubleBeforeDate", addOf(9e15, "$date")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Add_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AllDocsDatePlusField", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: addOf("$date", "$m")}}}},
		}},
		{"MatchExprDate", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
				addOf("$date", int64(86400000)), date,
			}}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupSumOfDatePlus", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: addOf("$date", int32(1))}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Add_" + p.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
