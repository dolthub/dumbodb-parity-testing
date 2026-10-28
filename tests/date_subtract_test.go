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

func dateSubtract(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateSubtract", Value: bson.D(parts)}}
}

func subtract(start interface{}, unit string, amount interface{}, extra ...bson.E) bson.D {
	return dateSubtract(append([]bson.E{part("startDate", start), part("unit", unit), part("amount", amount)}, extra...)...)
}

func TestDateSubtract(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi, s, ms int) time.Time {
		return time.Date(y, m, d, h, mi, s, ms*1e6, time.UTC)
	}
	base := utc(2021, 3, 31, 10, 20, 30, 456)
	ny := part("timezone", "America/New_York")

	cases := []mathCase{
		{"Year", subtract(base, "year", int32(1))},
		{"Quarter", subtract(base, "quarter", int32(1))},
		{"Month", subtract(base, "month", int32(1))},
		{"Week", subtract(base, "week", int32(1))},
		{"Day", subtract(base, "day", int32(1))},
		{"Hour", subtract(base, "hour", int32(1))},
		{"Minute", subtract(base, "minute", int32(1))},
		{"Second", subtract(base, "second", int32(1))},
		{"Millisecond", subtract(base, "millisecond", int32(1))},
		{"NegativeAmount", subtract(base, "month", int32(-1))},
		{"ZeroAmount", subtract(base, "day", int32(0))},
		{"LargeDays", subtract(base, "day", int64(1000000))},
		{"LargeMilliseconds", subtract(base, "millisecond", int64(1e15))},

		{"MonthEndToFeb", subtract(utc(2021, 3, 31, 0, 0, 0, 0), "month", int32(1))},
		{"MonthEndToLeapFeb", subtract(utc(2020, 3, 31, 0, 0, 0, 0), "month", int32(1))},
		{"LeapDayMinusYear", subtract(utc(2020, 2, 29, 12, 0, 0, 0), "year", int32(1))},
		{"LeapDayMinusFourYears", subtract(utc(2020, 2, 29, 12, 0, 0, 0), "year", int32(4))},
		{"Day30MinusMonth", subtract(utc(2021, 5, 30, 0, 0, 0, 0), "month", int32(3))},
		{"Day28MinusMonth", subtract(utc(2021, 3, 28, 0, 0, 0, 0), "month", int32(1))},
		{"Day29MinusMonth", subtract(utc(2021, 3, 29, 0, 0, 0, 0), "month", int32(1))},
		{"MonthEndQuarter", subtract(utc(2021, 5, 31, 0, 0, 0, 0), "quarter", int32(1))},
		{"MonthEndManyMonths", subtract(utc(2021, 1, 31, 0, 0, 0, 0), "month", int32(-13))},
		{"YearsAcrossCenturies", subtract(utc(2000, 2, 29, 0, 0, 0, 0), "year", int32(100))},

		{"AmountLong", subtract(base, "day", int64(2))},
		{"AmountDouble", subtract(base, "day", 2.0)},
		{"AmountDecimal", subtract(base, "day", mustDecimal("2.000"))},
		{"AmountFractional", subtract(base, "day", 1.5)},
		{"AmountString", subtract(base, "day", "1")},
		{"AmountNaN", subtract(base, "day", math.NaN())},
		{"AmountMinLong", subtract(base, "day", int64(math.MinInt64))},
		{"AmountMaxLong", subtract(base, "millisecond", int64(math.MaxInt64))},
		{"AmountTooManyDays", subtract(base, "day", int64(1e15))},
		{"AmountTooManyYears", subtract(base, "year", int64(1e9))},
		{"ResultOverflow", subtract(base, "year", int64(300000000))},
		{"ResultFarPast", subtract(base, "year", int64(100000))},
		{"AmountNull", subtract(base, "day", nil)},
		{"AmountMissing", subtract(base, "day", "$missing")},

		{"StartNull", subtract(nil, "day", int32(1))},
		{"StartMissing", subtract("$missing", "day", int32(1))},
		{"StartField", subtract("$date", "day", int32(1))},
		{"StartTimestamp", subtract(primitive.Timestamp{T: 1500000000, I: 1}, "day", int32(1))},
		{"StartObjectID", subtract(primitive.NewObjectIDFromTimestamp(base), "hour", int32(1))},
		{"StartString", subtract("2021-03-31", "day", int32(1))},
		{"StartNumber", subtract(int64(0), "day", int32(1))},
		{"StartPreEpochMillis", subtract(primitive.DateTime(-1), "second", int32(1))},
		{"StartPreEpochMillisDay", subtract(primitive.DateTime(-86400001), "day", int32(1))},
		{"StartPreEpochMillisMs", subtract(primitive.DateTime(-1), "millisecond", int32(0))},
		{"StartPreEpochZeroSeconds", subtract(primitive.DateTime(-1), "second", int32(0))},
		{"StartYearOne", subtract(utc(1, 1, 1, 0, 0, 0, 0), "day", int32(1))},

		{"UnitNull", dateSubtract(part("startDate", base), part("unit", nil), part("amount", int32(1)))},
		{"UnitInvalid", dateSubtract(part("startDate", base), part("unit", "years"), part("amount", int32(1)))},
		{"UnitCapitalized", dateSubtract(part("startDate", base), part("unit", "Day"), part("amount", int32(1)))},
		{"UnitNumber", dateSubtract(part("startDate", base), part("unit", int32(1)), part("amount", int32(1)))},
		{"UnitField", dateSubtract(part("startDate", base), part("unit", "$unit"), part("amount", int32(1)))},
		{"UnitInvalidStartNull", dateSubtract(part("startDate", nil), part("unit", "years"), part("amount", int32(1)))},
		{"UnitInvalidTimezoneNull", dateSubtract(part("startDate", base), part("unit", "years"), part("amount", int32(1)), part("timezone", nil))},
		{"UnitNullBadStart", dateSubtract(part("startDate", "x"), part("unit", nil), part("amount", int32(1)))},
		{"TimezoneNullBadStart", subtract("x", "day", int32(1), part("timezone", nil))},
		{"BadStartBadAmount", subtract("x", "day", "y")},

		{"TzDay", subtract(base, "day", int32(1), ny)},
		{"TzDayAcrossDSTEnd", subtract(utc(2020, 11, 2, 5, 0, 0, 0), "day", int32(1), ny)},
		{"TzDayAcrossDSTStart", subtract(utc(2020, 3, 9, 4, 0, 0, 0), "day", int32(1), ny)},
		{"TzHourAcrossDSTEnd", subtract(utc(2020, 11, 1, 6, 50, 2, 0), "hour", int32(1), ny)},
		{"TzMinuteAcrossDSTStart", subtract(utc(2020, 3, 8, 7, 10, 0, 0), "minute", int32(30), ny)},
		{"TzMonthEndLocal", subtract(utc(2021, 4, 1, 2, 0, 0, 0), "month", int32(1), ny)},
		{"TzMonthAcrossDST", subtract(utc(2021, 4, 15, 12, 0, 0, 0), "month", int32(1), ny)},
		{"TzYearLocalLeapDay", subtract(utc(2020, 3, 1, 3, 0, 0, 0), "year", int32(1), ny)},
		{"TzLondonWinterMonthEnd", subtract(utc(2021, 3, 31, 12, 0, 0, 0), "month", int32(1), part("timezone", "Europe/London"))},
		{"TzLondonSummerMonthEnd", subtract(utc(2021, 5, 31, 12, 0, 0, 0), "month", int32(1), part("timezone", "Europe/London"))},
		{"TzOffsetHour", subtract(base, "hour", int32(1), part("timezone", "+05:30"))},
		{"TzOffsetMonthEnd", subtract(utc(2021, 3, 31, 20, 0, 0, 0), "month", int32(1), part("timezone", "+05:30"))},
		{"TzNegativeOffsetMonthEnd", subtract(utc(2021, 4, 1, 2, 0, 0, 0), "month", int32(1), part("timezone", "-04:00"))},
		{"TzZeroOffsetMonthEnd", subtract(utc(2021, 3, 31, 0, 0, 0, 0), "month", int32(1), part("timezone", "+00:00"))},
		{"TzUTC", subtract(base, "month", int32(1), part("timezone", "UTC"))},
		{"TzField", subtract("$date", "month", int32(1), part("timezone", "$tz"))},
		{"TzInvalid", subtract(base, "day", int32(1), part("timezone", "Not/AZone"))},
		{"TzNumber", subtract(base, "day", int32(1), part("timezone", int32(1)))},
		{"TzNull", subtract(base, "day", int32(1), part("timezone", nil))},
		{"TzMissing", subtract(base, "day", int32(1), part("timezone", "$missing"))},
		{"TzMillisecondDST", subtract(utc(2020, 11, 1, 6, 0, 0, 0), "millisecond", int32(1), ny)},

		{"NoStartDate", dateSubtract(part("unit", "day"), part("amount", int32(1)))},
		{"NoUnit", dateSubtract(part("startDate", base), part("amount", int32(1)))},
		{"NoAmount", dateSubtract(part("startDate", base), part("unit", "day"))},
		{"UnknownField", subtract(base, "day", int32(1), part("format", "x"))},
		{"NonObjectArgument", bson.D{{Key: "$dateSubtract", Value: base}}},
		{"ArrayArgument", bson.D{{Key: "$dateSubtract", Value: bson.A{bson.D{{Key: "startDate", Value: base}}}}}},
	}

	setup := insertDatePartsDocs
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateSubtract_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   setup,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AddFieldsAllDocs", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: subtract(dateFromParts(part("year", "$y"), part("month", "$m")), "month", "$m", part("timezone", "$tz"))}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{
				subtract("$date", "year", int32(20)),
				utc(2005, 1, 1, 0, 0, 0, 0),
			}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupKey", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: subtract(dateFromParts(part("year", "$y")), "quarter", int32(1))}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateSubtract_" + p.name,
				Support: harness.DumboDBXFail,
				Setup:   setup,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
