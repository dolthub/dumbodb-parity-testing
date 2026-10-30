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
	"fmt"
	"math"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func dateDiff(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateDiff", Value: bson.D(parts)}}
}

func diff(start, end, unit interface{}, extra ...bson.E) bson.D {
	return dateDiff(append([]bson.E{part("startDate", start), part("endDate", end), part("unit", unit)}, extra...)...)
}

// $dateDiff ignored timezone and startOfWeek and validated loosely
// (workspace-0dd).
func TestDateDiff(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi, s, ms int) time.Time {
		return time.Date(y, m, d, h, mi, s, ms*1e6, time.UTC)
	}
	a := utc(2020, 12, 31, 23, 59, 59, 999)
	b := utc(2021, 1, 1, 0, 0, 0, 0)
	ny := part("timezone", "America/New_York")

	cases := []mathCase{
		{"YearBoundary", diff(a, b, "year")},
		{"QuarterBoundary", diff(a, b, "quarter")},
		{"MonthBoundary", diff(a, b, "month")},
		{"WeekBoundary", diff(a, b, "week")},
		{"DayBoundary", diff(a, b, "day")},
		{"HourBoundary", diff(a, b, "hour")},
		{"MinuteBoundary", diff(a, b, "minute")},
		{"SecondBoundary", diff(a, b, "second")},
		{"Millisecond", diff(a, b, "millisecond")},
		{"Negative", diff(b, a, "day")},
		{"ManyYears", diff(utc(1900, 6, 1, 0, 0, 0, 0), utc(2100, 1, 1, 0, 0, 0, 0), "year")},
		{"ManyDays", diff(utc(1, 1, 1, 0, 0, 0, 0), utc(9999, 12, 31, 0, 0, 0, 0), "day")},
		{"LeapDay", diff(utc(2020, 2, 28, 12, 0, 0, 0), utc(2020, 3, 1, 12, 0, 0, 0), "day")},
		{"PreEpoch", diff(utc(1969, 12, 31, 23, 0, 0, 0), utc(1970, 1, 1, 1, 0, 0, 0), "hour")},
		{"PreEpochMillis", diff(primitive.DateTime(-1500), primitive.DateTime(-1), "second")},
		{"MillisOverflow", diff(primitive.DateTime(math.MinInt64), primitive.DateTime(math.MaxInt64), "millisecond")},
		{"MillisUnderflow", diff(primitive.DateTime(math.MaxInt64), primitive.DateTime(math.MinInt64), "millisecond")},
		{"ExtremeDays", diff(primitive.DateTime(math.MinInt64), primitive.DateTime(math.MaxInt64), "day")},
		{"ExtremeSeconds", diff(primitive.DateTime(math.MinInt64), primitive.DateTime(math.MaxInt64), "second")},

		{"WeekDefaultSunday", diff(utc(2021, 7, 10, 0, 0, 0, 0), utc(2021, 7, 11, 0, 0, 0, 0), "week")},
		{"WeekMonday", diff(utc(2021, 7, 11, 0, 0, 0, 0), utc(2021, 7, 12, 0, 0, 0, 0), "week", part("startOfWeek", "monday"))},
		{"WeekAbbrev", diff(utc(2021, 7, 1, 0, 0, 0, 0), utc(2021, 7, 30, 0, 0, 0, 0), "week", part("startOfWeek", "FRI"))},
		{"WeekNegative", diff(utc(2021, 7, 30, 0, 0, 0, 0), utc(2021, 7, 1, 0, 0, 0, 0), "week", part("startOfWeek", "wed"))},
		{"StartOfWeekInvalid", diff(a, b, "week", part("startOfWeek", "someday"))},
		{"StartOfWeekInvalidDay", diff(a, b, "day", part("startOfWeek", "someday"))},
		{"StartOfWeekInvalidDayField", diff("$date", b, "day", part("startOfWeek", "someday"))},
		{"StartOfWeekNumber", diff(a, b, "week", part("startOfWeek", int32(1)))},
		{"StartOfWeekNull", diff(a, b, "week", part("startOfWeek", nil))},
		{"StartOfWeekNullDay", diff(a, b, "day", part("startOfWeek", nil))},
		{"StartOfWeekField", diff("$date", b, "week", part("startOfWeek", "$s"))},

		{"TzDay", diff(utc(2021, 1, 1, 3, 0, 0, 0), utc(2021, 1, 1, 6, 0, 0, 0), "day", ny)},
		{"TzYear", diff(utc(2021, 1, 1, 3, 0, 0, 0), utc(2021, 1, 1, 6, 0, 0, 0), "year", ny)},
		{"TzHourAcrossDSTStart", diff(utc(2021, 3, 14, 6, 30, 0, 0), utc(2021, 3, 14, 7, 30, 0, 0), "hour", ny)},
		{"TzHourAcrossDSTEnd", diff(utc(2021, 11, 7, 5, 30, 0, 0), utc(2021, 11, 7, 6, 30, 0, 0), "hour", ny)},
		{"TzMinuteAcrossDSTEnd", diff(utc(2021, 11, 7, 5, 59, 0, 0), utc(2021, 11, 7, 6, 1, 0, 0), "minute", ny)},
		{"TzSecondAcrossDSTEnd", diff(utc(2021, 11, 7, 5, 59, 59, 0), utc(2021, 11, 7, 6, 0, 1, 0), "second", ny)},
		{"TzDayAcrossDST", diff(utc(2021, 3, 13, 12, 0, 0, 0), utc(2021, 3, 15, 12, 0, 0, 0), "day", ny)},
		{"TzOffsetHour", diff(utc(2021, 1, 1, 0, 0, 0, 0), utc(2021, 1, 1, 1, 0, 0, 0), "hour", part("timezone", "+05:45"))},
		{"TzOffsetDay", diff(utc(2021, 1, 1, 18, 0, 0, 0), utc(2021, 1, 1, 19, 0, 0, 0), "day", part("timezone", "+05:30"))},
		{"TzLordHowe", diff(utc(2021, 4, 3, 14, 0, 0, 0), utc(2021, 4, 3, 16, 0, 0, 0), "minute", part("timezone", "Australia/Lord_Howe"))},
		{"TzMillisecond", diff(a, b, "millisecond", ny)},
		{"TzInvalid", diff(a, b, "day", part("timezone", "Not/AZone"))},
		{"TzNumber", diff(a, b, "day", part("timezone", int32(1)))},
		{"TzNull", diff(a, b, "day", part("timezone", nil))},
		{"TzMissing", diff(a, b, "day", part("timezone", "$missing"))},
		{"TzField", diff("$date", b, "day", part("timezone", "$tz"))},
		{"TzInvalidField", diff("$date", b, "day", part("timezone", "$s"))},

		{"StartNull", diff(nil, b, "day")},
		{"EndNull", diff(a, nil, "day")},
		{"StartMissing", diff("$missing", b, "day")},
		{"StartString", diff("2021-01-01", b, "day")},
		{"EndNumber", diff(a, int64(0), "day")},
		{"StartTimestamp", diff(primitive.Timestamp{T: 1500000000, I: 1}, b, "day")},
		{"StartObjectID", diff(primitive.NewObjectIDFromTimestamp(a), b, "second")},
		{"StartStringEndNull", diff("x", nil, "day")},
		{"StartStringBadUnit", diff("x", b, "days")},
		{"StartStringBadUnitField", diff("$s", b, "$s")},

		{"UnitNull", diff(a, b, nil)},
		{"UnitMissing", diff(a, b, "$missing")},
		{"UnitInvalid", diff(a, b, "days")},
		{"UnitNumber", diff(a, b, int32(1))},
		{"UnitField", diff("$date", b, "$s")},
		{"UnitNullTzInvalid", diff(a, b, nil, part("timezone", "Not/AZone"))},
		{"UnitInvalidTzNull", diff(a, b, "days", part("timezone", nil))},

		{"NoStartDate", dateDiff(part("endDate", b), part("unit", "day"))},
		{"NoEndDate", dateDiff(part("startDate", a), part("unit", "day"))},
		{"NoUnit", dateDiff(part("startDate", a), part("endDate", b))},
		{"UnknownField", diff(a, b, "day", part("format", "x"))},
		{"NonObjectArgument", bson.D{{Key: "$dateDiff", Value: a}}},
		{"ArrayArgument", bson.D{{Key: "$dateDiff", Value: bson.A{bson.D{{Key: "startDate", Value: a}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateDiff_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	// Every unit across a DST change, for each document's zone.
	for _, unit := range []string{"year", "quarter", "month", "week", "day", "hour", "minute", "second", "millisecond"} {
		name := fmt.Sprintf("Matrix_%s", unit)
		tz := part("timezone", bson.D{{Key: "$ifNull", Value: bson.A{"$tz", "UTC"}}})
		pipeline := bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.A{
				diff(utc(2020, 3, 8, 6, 30, 15, 500), utc(2020, 11, 1, 6, 59, 59, 999), unit, tz),
				diff(utc(2020, 11, 1, 5, 59, 59, 999), utc(2020, 3, 8, 7, 30, 15, 500), unit, tz, part("startOfWeek", "sat")),
				diff(utc(1987, 2, 28, 23, 0, 0, 0), utc(2024, 2, 29, 1, 0, 0, 0), unit, tz),
			}}}}},
		}
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateDiff_" + name,
				Support: harness.DumboDBXFail,
				Setup:   insertDatePartsDocs,
				Run:     pipelineRunExtJSON(pipeline),
			})
		})
	}
}
