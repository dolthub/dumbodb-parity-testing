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

func dateAdd(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateAdd", Value: bson.D(parts)}}
}

func add(start interface{}, unit string, amount interface{}, extra ...bson.E) bson.D {
	return dateAdd(append([]bson.E{part("startDate", start), part("unit", unit), part("amount", amount)}, extra...)...)
}

func TestDateAdd(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi, s, ms int) time.Time {
		return time.Date(y, m, d, h, mi, s, ms*1e6, time.UTC)
	}
	base := utc(2021, 1, 31, 10, 20, 30, 456)
	ny := part("timezone", "America/New_York")

	cases := []mathCase{
		{"Year", add(base, "year", int32(1))},
		{"Quarter", add(base, "quarter", int32(1))},
		{"Month", add(base, "month", int32(1))},
		{"Week", add(base, "week", int32(1))},
		{"Day", add(base, "day", int32(1))},
		{"Hour", add(base, "hour", int32(1))},
		{"Minute", add(base, "minute", int32(1))},
		{"Second", add(base, "second", int32(1))},
		{"Millisecond", add(base, "millisecond", int32(1))},
		{"NegativeAmount", add(base, "month", int32(-1))},
		{"ZeroAmount", add(base, "day", int32(0))},
		{"LargeDays", add(base, "day", int64(1000000))},
		{"LargeMilliseconds", add(base, "millisecond", int64(1e15))},

		{"MonthEndToFeb", add(utc(2021, 1, 31, 0, 0, 0, 0), "month", int32(1))},
		{"MonthEndToLeapFeb", add(utc(2020, 1, 31, 0, 0, 0, 0), "month", int32(1))},
		{"MonthEndTo30", add(utc(2021, 3, 31, 0, 0, 0, 0), "month", int32(1))},
		{"LeapDayPlusYear", add(utc(2020, 2, 29, 12, 0, 0, 0), "year", int32(1))},
		{"LeapDayPlusFourYears", add(utc(2020, 2, 29, 12, 0, 0, 0), "year", int32(4))},
		{"Day30PlusMonth", add(utc(2021, 1, 30, 0, 0, 0, 0), "month", int32(1))},
		{"Day28PlusMonth", add(utc(2021, 1, 28, 0, 0, 0, 0), "month", int32(1))},
		{"Day29PlusMonth", add(utc(2021, 1, 29, 0, 0, 0, 0), "month", int32(1))},
		{"MonthEndQuarter", add(utc(2021, 8, 31, 0, 0, 0, 0), "quarter", int32(1))},
		{"MonthEndManyMonths", add(utc(2021, 1, 31, 0, 0, 0, 0), "month", int32(13))},
		{"MonthEndNegativeMonths", add(utc(2021, 3, 31, 0, 0, 0, 0), "month", int32(-1))},
		{"YearsAcrossCenturies", add(utc(2000, 2, 29, 0, 0, 0, 0), "year", int32(100))},
		{"DecemberPlusMonth", add(utc(2021, 12, 31, 0, 0, 0, 0), "month", int32(2))},

		{"AmountLong", add(base, "day", int64(2))},
		{"AmountDouble", add(base, "day", 2.0)},
		{"AmountDecimal", add(base, "day", mustDecimal("2.000"))},
		{"AmountFractional", add(base, "day", 1.5)},
		{"AmountNegativeDouble", add(base, "hour", -3.0)},
		{"AmountString", add(base, "day", "1")},
		{"AmountBool", add(base, "day", true)},
		{"AmountNaN", add(base, "day", math.NaN())},
		{"AmountInfinity", add(base, "day", math.Inf(1))},
		{"AmountMinLong", add(base, "day", int64(math.MinInt64))},
		{"AmountMinLongMillis", add(base, "millisecond", int64(math.MinInt64))},
		{"AmountMaxLong", add(base, "millisecond", int64(math.MaxInt64))},
		{"AmountTooManyDays", add(base, "day", int64(1e15))},
		{"AmountTooManyYears", add(base, "year", int64(1e9))},
		{"AmountTooManyNegativeYears", add(base, "year", int64(-1e9))},
		{"ResultOverflow", add(base, "year", int64(300000000))},
		{"ResultUnderflow", add(base, "year", int64(-300000000))},
		{"ResultFarFuture", add(base, "year", int64(100000))},
		{"ResultFarPast", add(base, "year", int64(-100000))},
		{"AmountNull", add(base, "day", nil)},
		{"AmountMissing", add(base, "day", "$missing")},

		{"StartNull", add(nil, "day", int32(1))},
		{"StartMissing", add("$missing", "day", int32(1))},
		{"StartField", add("$date", "day", int32(1))},
		{"StartTimestamp", add(primitive.Timestamp{T: 1500000000, I: 1}, "day", int32(1))},
		{"StartObjectID", add(primitive.NewObjectIDFromTimestamp(base), "hour", int32(1))},
		{"StartString", add("2021-03-31", "day", int32(1))},
		{"StartNumber", add(int64(0), "day", int32(1))},
		{"StartPreEpochMillis", add(primitive.DateTime(-1), "second", int32(1))},
		{"StartPreEpochMillisDay", add(primitive.DateTime(-86400001), "day", int32(1))},
		{"StartPreEpochMillisMs", add(primitive.DateTime(-1), "millisecond", int32(0))},
		{"StartPreEpochZeroSeconds", add(primitive.DateTime(-1), "second", int32(0))},
		{"StartYearOne", add(utc(1, 1, 1, 0, 0, 0, 0), "day", int32(-1))},

		{"UnitNull", dateAdd(part("startDate", base), part("unit", nil), part("amount", int32(1)))},
		{"UnitInvalid", dateAdd(part("startDate", base), part("unit", "years"), part("amount", int32(1)))},
		{"UnitCapitalized", dateAdd(part("startDate", base), part("unit", "Day"), part("amount", int32(1)))},
		{"UnitNumber", dateAdd(part("startDate", base), part("unit", int32(1)), part("amount", int32(1)))},
		{"UnitField", dateAdd(part("startDate", base), part("unit", "$unit"), part("amount", int32(1)))},
		{"UnitInvalidStartNull", dateAdd(part("startDate", nil), part("unit", "years"), part("amount", int32(1)))},
		{"UnitInvalidTimezoneNull", dateAdd(part("startDate", base), part("unit", "years"), part("amount", int32(1)), part("timezone", nil))},
		{"UnitNullBadStart", dateAdd(part("startDate", "x"), part("unit", nil), part("amount", int32(1)))},
		{"TimezoneNullBadStart", add("x", "day", int32(1), part("timezone", nil))},
		{"BadStartBadAmount", add("x", "day", "y")},

		{"TzDay", add(base, "day", int32(1), ny)},
		{"TzDayAcrossDSTEnd", add(utc(2020, 10, 31, 5, 0, 0, 0), "day", int32(1), ny)},
		{"TzDayAcrossDSTStart", add(utc(2020, 3, 7, 5, 0, 0, 0), "day", int32(1), ny)},
		{"TzDayIntoDSTGap", add(utc(2020, 3, 7, 7, 30, 0, 0), "day", int32(1), ny)},
		{"TzHourAcrossDSTEnd", add(utc(2020, 11, 1, 5, 50, 2, 0), "hour", int32(1), ny)},
		{"TzMinuteAcrossDSTStart", add(utc(2020, 3, 8, 6, 50, 0, 0), "minute", int32(30), ny)},
		{"TzMonthEndLocal", add(utc(2021, 2, 1, 2, 0, 0, 0), "month", int32(1), ny)},
		{"TzMonthAcrossDST", add(utc(2021, 2, 15, 12, 0, 0, 0), "month", int32(1), ny)},
		{"TzYearLocalLeapDay", add(utc(2020, 3, 1, 3, 0, 0, 0), "year", int32(1), ny)},
		{"TzWeekAcrossDST", add(utc(2021, 3, 10, 12, 0, 0, 0), "week", int32(1), ny)},
		{"TzLondonWinterMonthEnd", add(utc(2021, 1, 31, 12, 0, 0, 0), "month", int32(1), part("timezone", "Europe/London"))},
		{"TzLondonSummerMonthEnd", add(utc(2021, 5, 31, 12, 0, 0, 0), "month", int32(1), part("timezone", "Europe/London"))},
		{"TzOffsetHour", add(base, "hour", int32(1), part("timezone", "+05:30"))},
		{"TzOffsetMonthEnd", add(utc(2021, 1, 30, 20, 0, 0, 0), "month", int32(1), part("timezone", "+05:30"))},
		{"TzNegativeOffsetMonthEnd", add(utc(2021, 2, 1, 2, 0, 0, 0), "month", int32(1), part("timezone", "-04:00"))},
		{"TzZeroOffsetMonthEnd", add(utc(2021, 1, 31, 0, 0, 0, 0), "month", int32(1), part("timezone", "+00:00"))},
		{"TzUTC", add(base, "month", int32(1), part("timezone", "UTC"))},
		{"TzField", add("$date", "month", int32(1), part("timezone", "$tz"))},
		{"TzInvalid", add(base, "day", int32(1), part("timezone", "Not/AZone"))},
		{"TzNumber", add(base, "day", int32(1), part("timezone", int32(1)))},
		{"TzNull", add(base, "day", int32(1), part("timezone", nil))},
		{"TzMissing", add(base, "day", int32(1), part("timezone", "$missing"))},
		{"TzMillisecondDST", add(utc(2020, 11, 1, 5, 59, 59, 999), "millisecond", int32(1), ny)},

		{"NoStartDate", dateAdd(part("unit", "day"), part("amount", int32(1)))},
		{"NoUnit", dateAdd(part("startDate", base), part("amount", int32(1)))},
		{"NoAmount", dateAdd(part("startDate", base), part("unit", "day"))},
		{"UnknownField", add(base, "day", int32(1), part("format", "x"))},
		{"NonObjectArgument", bson.D{{Key: "$dateAdd", Value: base}}},
		{"ArrayArgument", bson.D{{Key: "$dateAdd", Value: bson.A{bson.D{{Key: "startDate", Value: base}}}}}},
	}

	setup := insertDatePartsDocs
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateAdd_" + tc.name,
				Support: harness.DumboDBFull,
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
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: add(dateFromParts(part("year", "$y"), part("month", "$m")), "month", "$m", part("timezone", "$tz"))}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
				add("$date", "year", int32(20)),
				utc(2030, 1, 1, 0, 0, 0, 0),
			}}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupKey", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: add(dateFromParts(part("year", "$y")), "quarter", int32(1))}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"AddThenSubtract", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: subtract(add("$date", "month", int32(1), part("timezone", "$tz")), "month", int32(1), part("timezone", "$tz"))}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateAdd_" + p.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
