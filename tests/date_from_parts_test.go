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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Date part operators. Each case is its own pair test, compared by BSON type
// and exact value. The harness masks dates inside documents, so pipelines
// compare their results as extended JSON.

func insertDatePartsDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "y", Value: int32(2021)},
			{Key: "m", Value: int64(7)},
			{Key: "d", Value: 15.0},
			{Key: "dec", Value: mustDecimal("30")},
			{Key: "tz", Value: "Asia/Kolkata"},
			{Key: "n", Value: nil},
			{Key: "s", Value: "x"},
			{Key: "date", Value: time.Date(2021, 7, 15, 13, 45, 30, 123e6, time.UTC)},
		},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "y", Value: int32(1999)}, {Key: "m", Value: int32(12)}, {Key: "tz", Value: "+02:00"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "y", Value: int32(2000)}, {Key: "m", Value: int32(2)}, {Key: "tz", Value: "America/New_York"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "y", Value: nil}},
		bson.D{{Key: "_id", Value: int32(4)}},
	})
	return err
}

func runDateCases(t *testing.T, prefix string, support harness.DumboDBSupport, cases []mathCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    prefix + "_" + tc.name,
				Support: support,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}

func dateFromParts(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateFromParts", Value: bson.D(parts)}}
}

func part(key string, value interface{}) bson.E { return bson.E{Key: key, Value: value} }

func TestDateFromParts(t *testing.T) {
	cases := []mathCase{
		{"YearOnly", dateFromParts(part("year", int32(2017)))},
		{"AllCalendarParts", dateFromParts(part("year", int32(2017)), part("month", int32(2)), part("day", int32(8)),
			part("hour", int32(12)), part("minute", int32(30)), part("second", int32(15)), part("millisecond", int32(123)))},
		{"YearOne", dateFromParts(part("year", int32(1)))},
		{"Year9999End", dateFromParts(part("year", int32(9999)), part("month", int32(12)), part("day", int32(31)),
			part("hour", int32(23)), part("minute", int32(59)), part("second", int32(59)), part("millisecond", int32(999)))},
		{"Year1969", dateFromParts(part("year", int32(1969)), part("month", int32(12)), part("day", int32(31)), part("millisecond", int32(1)))},
		{"LeapDay", dateFromParts(part("year", int32(2020)), part("month", int32(2)), part("day", int32(29)))},
		{"NonLeapDayCarries", dateFromParts(part("year", int32(2021)), part("month", int32(2)), part("day", int32(29)))},
		{"Century1900Feb29", dateFromParts(part("year", int32(1900)), part("month", int32(2)), part("day", int32(29)))},

		{"MonthThirteen", dateFromParts(part("year", int32(2017)), part("month", int32(13)))},
		{"MonthZero", dateFromParts(part("year", int32(2017)), part("month", int32(0)))},
		{"MonthNegative", dateFromParts(part("year", int32(2017)), part("month", int32(-13)))},
		{"DayZero", dateFromParts(part("year", int32(2017)), part("month", int32(3)), part("day", int32(0)))},
		{"DayLarge", dateFromParts(part("year", int32(2017)), part("day", int32(400)))},
		{"HourCarries", dateFromParts(part("year", int32(2017)), part("hour", int32(25)))},
		{"MinuteNegative", dateFromParts(part("year", int32(2017)), part("minute", int32(-1)))},
		{"SecondLarge", dateFromParts(part("year", int32(2017)), part("second", int32(32767)))},
		{"MillisecondCarries", dateFromParts(part("year", int32(2017)), part("millisecond", int32(1000)))},
		{"MillisecondNegative", dateFromParts(part("year", int32(2017)), part("millisecond", int32(-32768)))},
		{"CarryBelowYearOne", dateFromParts(part("year", int32(1)), part("month", int32(0)))},
		{"CarryAboveYear9999", dateFromParts(part("year", int32(9999)), part("month", int32(13)))},
		{"MonthOutOfRange", dateFromParts(part("year", int32(2017)), part("month", int32(32768)))},
		{"DayOutOfRange", dateFromParts(part("year", int32(2017)), part("day", int32(-32769)))},
		{"HourOutOfRange", dateFromParts(part("year", int32(2017)), part("hour", int32(40000)))},
		{"MillisecondOutOfRange", dateFromParts(part("year", int32(2017)), part("millisecond", int64(1)<<40))},
		{"YearZero", dateFromParts(part("year", int32(0)))},
		{"YearTenThousand", dateFromParts(part("year", int32(10000)))},
		{"YearNegative", dateFromParts(part("year", int32(-1)))},

		{"YearLong", dateFromParts(part("year", int64(2017)))},
		{"YearDouble", dateFromParts(part("year", 2017.0))},
		{"YearDecimal", dateFromParts(part("year", mustDecimal("2017.000")))},
		{"YearFractional", dateFromParts(part("year", 2017.5))},
		{"MonthFractional", dateFromParts(part("year", int32(2017)), part("month", 1.5))},
		{"MillisecondFractional", dateFromParts(part("year", int32(2017)), part("millisecond", 0.5))},
		{"MonthDecimalFractional", dateFromParts(part("year", int32(2017)), part("month", mustDecimal("1.1")))},
		{"YearNaN", dateFromParts(part("year", mustDecimal("NaN")))},
		{"YearString", dateFromParts(part("year", "2017"))},
		{"MonthString", dateFromParts(part("year", int32(2017)), part("month", "x"))},
		{"MonthBool", dateFromParts(part("year", int32(2017)), part("month", true))},
		{"YearDate", dateFromParts(part("year", "$date"))},

		{"YearNull", dateFromParts(part("year", nil))},
		{"YearMissingField", dateFromParts(part("year", "$missing"))},
		{"MonthNull", dateFromParts(part("year", int32(2017)), part("month", nil))},
		{"MillisecondMissingField", dateFromParts(part("year", int32(2017)), part("millisecond", "$missing"))},
		{"TimezoneNull", dateFromParts(part("year", int32(2017)), part("timezone", nil))},
		{"TimezoneMissingField", dateFromParts(part("year", int32(2017)), part("timezone", "$missing"))},
		{"NullYearBadMonth", dateFromParts(part("year", nil), part("month", "x"))},
		{"BadYearNullMonth", dateFromParts(part("year", "x"), part("month", nil))},
		{"NullYearBadTimezone", dateFromParts(part("year", nil), part("timezone", "Not/AZone"))},

		{"FieldParts", dateFromParts(part("year", "$y"), part("month", "$m"), part("day", "$d"), part("hour", "$dec"))},
		{"ExpressionParts", dateFromParts(part("year", bson.D{{Key: "$add", Value: bson.A{int32(2000), int32(17)}}}),
			part("month", bson.D{{Key: "$size", Value: bson.A{bson.A{1, 2, 3}}}}))},
		{"FieldTimezone", dateFromParts(part("year", int32(2017)), part("hour", int32(12)), part("timezone", "$tz"))},

		{"TimezoneOlson", dateFromParts(part("year", int32(2017)), part("month", int32(6)), part("timezone", "America/New_York"))},
		{"TimezoneOlsonWinter", dateFromParts(part("year", int32(2017)), part("month", int32(1)), part("timezone", "America/New_York"))},
		{"TimezoneUTC", dateFromParts(part("year", int32(2017)), part("timezone", "UTC"))},
		{"TimezoneGMT", dateFromParts(part("year", int32(2017)), part("timezone", "GMT"))},
		{"TimezoneZ", dateFromParts(part("year", int32(2017)), part("timezone", "Z"))},
		{"TimezoneOffsetColon", dateFromParts(part("year", int32(2017)), part("timezone", "+05:30"))},
		{"TimezoneOffsetCompact", dateFromParts(part("year", int32(2017)), part("timezone", "-0800"))},
		{"TimezoneOffsetHours", dateFromParts(part("year", int32(2017)), part("timezone", "+05"))},
		{"TimezoneOffsetLarge", dateFromParts(part("year", int32(2017)), part("timezone", "+14:00"))},
		{"TimezoneOffsetTooLarge", dateFromParts(part("year", int32(2017)), part("timezone", "+25:00"))},
		{"TimezoneInvalid", dateFromParts(part("year", int32(2017)), part("timezone", "Not/AZone"))},
		{"TimezoneLowercase", dateFromParts(part("year", int32(2017)), part("timezone", "america/new_york"))},
		{"TimezoneEmpty", dateFromParts(part("year", int32(2017)), part("timezone", ""))},
		{"TimezoneNumber", dateFromParts(part("year", int32(2017)), part("timezone", int32(5)))},
		{"TimezoneHalfHourZone", dateFromParts(part("year", int32(2017)), part("timezone", "Asia/Kathmandu"))},
		{"DSTGap", dateFromParts(part("year", int32(2020)), part("month", int32(3)), part("day", int32(8)),
			part("hour", int32(2)), part("minute", int32(30)), part("timezone", "America/New_York"))},
		{"DSTOverlap", dateFromParts(part("year", int32(2020)), part("month", int32(11)), part("day", int32(1)),
			part("hour", int32(1)), part("minute", int32(30)), part("timezone", "America/New_York"))},
		{"DSTOverlapEurope", dateFromParts(part("year", int32(2020)), part("month", int32(10)), part("day", int32(25)),
			part("hour", int32(2)), part("minute", int32(30)), part("timezone", "Europe/Berlin"))},
		{"HistoricalOffset", dateFromParts(part("year", int32(1900)), part("timezone", "Europe/Amsterdam"))},
		{"TimezoneCarry", dateFromParts(part("year", int32(2017)), part("month", int32(12)), part("day", int32(31)),
			part("hour", int32(23)), part("timezone", "-05:00"))},

		{"IsoWeekYearOnly", dateFromParts(part("isoWeekYear", int32(2017)))},
		{"IsoAllParts", dateFromParts(part("isoWeekYear", int32(2017)), part("isoWeek", int32(6)), part("isoDayOfWeek", int32(3)),
			part("hour", int32(12)), part("minute", int32(30)), part("second", int32(15)), part("millisecond", int32(123)))},
		{"IsoWeekYear2021", dateFromParts(part("isoWeekYear", int32(2021)))},
		{"IsoWeek53", dateFromParts(part("isoWeekYear", int32(2020)), part("isoWeek", int32(53)), part("isoDayOfWeek", int32(7)))},
		{"IsoWeekCarries", dateFromParts(part("isoWeekYear", int32(2017)), part("isoWeek", int32(54)))},
		{"IsoWeekZero", dateFromParts(part("isoWeekYear", int32(2017)), part("isoWeek", int32(0)))},
		{"IsoDayOfWeekZero", dateFromParts(part("isoWeekYear", int32(2017)), part("isoDayOfWeek", int32(0)))},
		{"IsoDayOfWeekLarge", dateFromParts(part("isoWeekYear", int32(2017)), part("isoDayOfWeek", int32(100)))},
		{"IsoWeekOutOfRange", dateFromParts(part("isoWeekYear", int32(2017)), part("isoWeek", int32(32768)))},
		{"IsoWeekYearZero", dateFromParts(part("isoWeekYear", int32(0)))},
		{"IsoWeekYearTenThousand", dateFromParts(part("isoWeekYear", int32(10000)))},
		{"IsoWeekYearOne", dateFromParts(part("isoWeekYear", int32(1)))},
		{"IsoTimezone", dateFromParts(part("isoWeekYear", int32(2017)), part("isoWeek", int32(10)), part("timezone", "Europe/London"))},
		{"IsoNullWeek", dateFromParts(part("isoWeekYear", int32(2017)), part("isoWeek", nil))},
		{"IsoFractionalDay", dateFromParts(part("isoWeekYear", int32(2017)), part("isoDayOfWeek", 2.5))},

		{"MixedYearAndIsoWeekYear", dateFromParts(part("year", int32(2017)), part("isoWeekYear", int32(2017)))},
		{"MixedMonthAndIsoWeek", dateFromParts(part("year", int32(2017)), part("isoWeek", int32(2)))},
		{"MixedIsoAndDay", dateFromParts(part("isoWeekYear", int32(2017)), part("day", int32(2)))},
		{"NoYear", dateFromParts(part("month", int32(2)))},
		{"NoYearOrIsoWeekYear", dateFromParts(part("isoWeek", int32(2)))},
		{"Empty", dateFromParts()},
		{"UnknownField", dateFromParts(part("year", int32(2017)), part("weekday", int32(2)))},
		{"NonObjectArgument", bson.D{{Key: "$dateFromParts", Value: int32(2017)}}},
		{"ArrayArgument", bson.D{{Key: "$dateFromParts", Value: bson.A{bson.D{{Key: "year", Value: int32(2017)}}}}}},
		{"StringArgument", bson.D{{Key: "$dateFromParts", Value: "2017"}}},
	}
	runDateCases(t, "DateFromParts", harness.DumboDBFull, cases)

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AddFieldsAllDocs", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: dateFromParts(part("year", "$y"), part("month", "$m"), part("timezone", "$tz"))}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{
				dateFromParts(part("year", "$y"), part("month", "$m")),
				dateFromParts(part("year", int32(2000)), part("month", int32(1))),
			}}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupKey", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: dateFromParts(part("year", "$y"), part("month", "$m"))}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateFromParts_" + p.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}

// pipelineRunExtJSON returns each result document as extended JSON, so dates
// are compared by value rather than masked.
func pipelineRunExtJSON(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			return errOutcome(err), nil
		}
		var docs []bson.Raw
		if err = cursor.All(ctx, &docs); err != nil {
			return errOutcome(err), nil
		}
		out := make([]string, len(docs))
		for i, doc := range docs {
			out[i] = doc.String()
		}
		return out, nil
	}
}
