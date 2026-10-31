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

func dateTrunc(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateTrunc", Value: bson.D(parts)}}
}

func trunc(date interface{}, unit interface{}, extra ...bson.E) bson.D {
	return dateTrunc(append([]bson.E{part("date", date), part("unit", unit)}, extra...)...)
}

// $dateTrunc with binSize 0 divided by zero and crashed DumboDB
// (workspace-8tj.7); it also ignored timezone and startOfWeek (workspace-0dd).
func TestDateTrunc(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi, s, ms int) time.Time {
		return time.Date(y, m, d, h, mi, s, ms*1e6, time.UTC)
	}
	base := utc(2021, 7, 15, 13, 45, 30, 123)
	ny := part("timezone", "America/New_York")
	bin := func(v interface{}) bson.E { return part("binSize", v) }

	cases := []mathCase{
		{"BinSizeZero", trunc(base, "day", bin(int32(0)))},
		{"BinSizeZeroHour", trunc(base, "hour", bin(int32(0)))},
		{"BinSizeZeroMillisecond", trunc(base, "millisecond", bin(int64(0)))},
		{"BinSizeNegative", trunc(base, "year", bin(int32(-1)))},
		{"BinSizeZeroField", trunc(base, "day", bin(bson.D{{Key: "$subtract", Value: bson.A{"$y", "$y"}}}))},
		{"BinSizeFractional", trunc(base, "day", bin(1.5))},
		{"BinSizeWholeDouble", trunc(base, "day", bin(2.0))},
		{"BinSizeDecimal", trunc(base, "day", bin(mustDecimal("3")))},
		{"BinSizeNaN", trunc(base, "day", bin(math.NaN()))},
		{"BinSizeInfinity", trunc(base, "day", bin(math.Inf(1)))},
		{"BinSizeString", trunc(base, "day", bin("2"))},
		{"BinSizeBool", trunc(base, "day", bin(true))},
		{"BinSizeNull", trunc(base, "day", bin(nil))},
		{"BinSizeMissing", trunc(base, "day", bin("$missing"))},
		{"BinSizeHugeDay", trunc(base, "day", bin(int64(100000000001)))},
		{"BinSizeMaxDay", trunc(base, "day", bin(int64(100000000000)))},
		{"BinSizeHugeHour", trunc(base, "hour", bin(int64(math.MaxInt64/1000)))},
		{"BinSizeMaxLongMillis", trunc(base, "millisecond", bin(int64(math.MaxInt64)))},
		{"BinSizeOverflowSecond", trunc(base, "second", bin(int64(math.MaxInt64/100)))},

		{"Year", trunc(base, "year")},
		{"Year3", trunc(base, "year", bin(int32(3)))},
		{"Quarter", trunc(base, "quarter")},
		{"Quarter3", trunc(base, "quarter", bin(int32(3)))},
		{"Month", trunc(base, "month")},
		{"Month5", trunc(base, "month", bin(int32(5)))},
		{"Week", trunc(base, "week")},
		{"Week2", trunc(base, "week", bin(int32(2)))},
		{"Day", trunc(base, "day")},
		{"Day7", trunc(base, "day", bin(int32(7)))},
		{"Hour", trunc(base, "hour")},
		{"Hour5", trunc(base, "hour", bin(int32(5)))},
		{"Minute", trunc(base, "minute")},
		{"Minute15", trunc(base, "minute", bin(int32(15)))},
		{"Second", trunc(base, "second")},
		{"Second7", trunc(base, "second", bin(int32(7)))},
		{"Millisecond", trunc(base, "millisecond")},
		{"Millisecond100", trunc(base, "millisecond", bin(int32(100)))},
		{"PreEpochDay", trunc(utc(1969, 12, 31, 23, 0, 0, 0), "day")},
		{"PreEpochMillisecond", trunc(primitive.DateTime(-1), "millisecond", bin(int32(1000)))},
		{"BeforeReferenceYear", trunc(utc(1997, 6, 1, 0, 0, 0, 0), "year", bin(int32(4)))},
		{"BeforeReferenceMonth", trunc(utc(1999, 11, 1, 0, 0, 0, 0), "month", bin(int32(5)))},
		{"YearOne", trunc(utc(1, 1, 1, 0, 0, 0, 0), "week")},
		{"FarFuture", trunc(utc(9999, 12, 31, 23, 59, 59, 999), "year", bin(int32(7)))},
		{"MinDateMillis", trunc(primitive.DateTime(math.MinInt64), "millisecond", bin(int32(1000)))},
		{"MinDateHour", trunc(primitive.DateTime(math.MinInt64), "hour")},
		{"MinDateDay", trunc(primitive.DateTime(math.MinInt64), "day")},
		{"MaxDateMillis", trunc(primitive.DateTime(math.MaxInt64), "millisecond", bin(int32(7)))},
		{"MaxDateYear", trunc(primitive.DateTime(math.MaxInt64), "year")},

		{"StartOfWeekMonday", trunc(base, "week", part("startOfWeek", "monday"))},
		{"StartOfWeekSaturday", trunc(base, "week", part("startOfWeek", "Saturday"))},
		{"StartOfWeekAbbrev", trunc(base, "week", part("startOfWeek", "THU"))},
		{"StartOfWeekInvalid", trunc(base, "week", part("startOfWeek", "someday"))},
		{"StartOfWeekNumber", trunc(base, "week", part("startOfWeek", int32(1)))},
		{"StartOfWeekNull", trunc(base, "week", part("startOfWeek", nil))},
		{"StartOfWeekNullDay", trunc(base, "day", part("startOfWeek", nil))},
		{"StartOfWeekInvalidDay", trunc(base, "day", part("startOfWeek", "someday"))},
		{"StartOfWeekInvalidDayField", trunc("$date", "day", part("startOfWeek", "someday"))},
		{"StartOfWeekField", trunc("$date", "week", part("startOfWeek", "$s"))},

		{"TzDay", trunc(base, "day", ny)},
		{"TzHour", trunc(base, "hour", part("timezone", "+05:30"))},
		{"TzHour5", trunc(base, "hour", bin(int32(5)), part("timezone", "Asia/Kolkata"))},
		{"TzMonth", trunc(utc(2021, 8, 1, 2, 0, 0, 0), "month", ny)},
		{"TzWeek", trunc(base, "week", ny, part("startOfWeek", "mon"))},
		{"TzYear", trunc(utc(2021, 1, 1, 3, 0, 0, 0), "year", ny)},
		{"TzDSTDay", trunc(utc(2020, 11, 1, 12, 0, 0, 0), "day", ny)},
		{"TzDSTHour", trunc(utc(2020, 11, 1, 6, 30, 0, 0), "hour", ny)},
		{"TzAsuncion", trunc(utc(2025, 3, 31, 12, 0, 0, 0), "day", part("timezone", "America/Asuncion"))},
		{"TzMillisecond", trunc(base, "millisecond", bin(int32(3600000)), part("timezone", "+05:45"))},
		{"TzInvalid", trunc(base, "day", part("timezone", "Not/AZone"))},
		{"TzNumber", trunc(base, "day", part("timezone", int32(1)))},
		{"TzNull", trunc(base, "day", part("timezone", nil))},
		{"TzMissing", trunc(base, "day", part("timezone", "$missing"))},
		{"TzField", trunc("$date", "day", part("timezone", "$tz"))},
		{"TzInvalidField", trunc("$date", "day", part("timezone", "$s"))},

		{"DateNull", trunc(nil, "day")},
		{"DateMissing", trunc("$missing", "day")},
		{"DateField", trunc("$date", "month")},
		{"DateString", trunc("2021-01-01", "day")},
		{"DateNumber", trunc(int64(0), "day")},
		{"DateTimestamp", trunc(primitive.Timestamp{T: 1500000000, I: 1}, "day")},
		{"DateObjectID", trunc(primitive.NewObjectIDFromTimestamp(base), "hour")},
		{"DateStringField", trunc("$s", "day")},
		{"DateNullBadUnit", trunc(nil, "days")},
		{"DateStringBadUnitField", trunc("$s", "$s")},
		{"DateStringBinSizeZeroField", trunc("$s", "day", bin("$n"))},

		{"UnitNull", trunc(base, nil)},
		{"UnitMissing", trunc(base, "$missing")},
		{"UnitInvalid", trunc(base, "days")},
		{"UnitCapitalized", trunc(base, "Day")},
		{"UnitNumber", trunc(base, int32(1))},
		{"UnitField", trunc("$date", "$s")},
		{"UnitInvalidBinSizeZero", trunc(base, "days", bin(int32(0)))},
		{"UnitNullBinSizeZero", trunc(base, nil, bin(int32(0)))},
		{"UnitNullTzInvalid", trunc(base, nil, part("timezone", "Not/AZone"))},
		{"BinSizeZeroTzInvalid", trunc(base, "day", bin(int32(0)), part("timezone", "Not/AZone"))},
		{"UnitInvalidTzNull", trunc(base, "days", part("timezone", nil))},

		{"NoDate", dateTrunc(part("unit", "day"))},
		{"NoUnit", dateTrunc(part("date", base))},
		{"UnknownField", trunc(base, "day", part("format", "x"))},
		{"NonObjectArgument", bson.D{{Key: "$dateTrunc", Value: base}}},
		{"ArrayArgument", bson.D{{Key: "$dateTrunc", Value: bson.A{bson.D{{Key: "date", Value: base}}}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateTrunc_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	// Every unit and bin size across a DST change, for each document's zone.
	for _, unit := range []string{"year", "quarter", "month", "week", "day", "hour", "minute", "second", "millisecond"} {
		for _, binSize := range []int32{1, 2, 3, 7, 13} {
			name := fmt.Sprintf("Matrix_%s_%d", unit, binSize)
			pipeline := bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.A{
					trunc(utc(2020, 3, 8, 7, 30, 15, 500), unit, bin(binSize), part("timezone", bson.D{{Key: "$ifNull", Value: bson.A{"$tz", "UTC"}}})),
					trunc(utc(2020, 11, 1, 5, 59, 59, 999), unit, bin(binSize), part("timezone", bson.D{{Key: "$ifNull", Value: bson.A{"$tz", "UTC"}}}), part("startOfWeek", "fri")),
					trunc(utc(1987, 2, 28, 23, 0, 0, 0), unit, bin(binSize), part("timezone", bson.D{{Key: "$ifNull", Value: bson.A{"$tz", "UTC"}}})),
				}}}}},
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "DateTrunc_" + name,
					Support: harness.DumboDBFull,
					Setup:   insertDatePartsDocs,
					Run:     pipelineRunExtJSON(pipeline),
				})
			})
		}
	}
}
