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

// $dateFromString without a format parses with timelib's free-form parser;
// the cases cover each of its grammar rules, with and without a full date.

func insertDateStringDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "ds", Value: "2017-02-08T12:10:40.787"},
			{Key: "fmt", Value: "%d/%m/%Y"},
			{Key: "dmy", Value: "08/02/2017"},
			{Key: "tz", Value: "America/New_York"},
			{Key: "n", Value: nil},
			{Key: "num", Value: int32(5)},
			{Key: "date", Value: time.Date(2021, 7, 15, 13, 45, 30, 123e6, time.UTC)},
		},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "ds", Value: "2000-01-01"}, {Key: "tz", Value: "+02:00"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "ds", Value: "not a date"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "ds", Value: nil}},
		bson.D{{Key: "_id", Value: int32(4)}},
	})
	return err
}

func dateFromString(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateFromString", Value: bson.D(parts)}}
}

func dateString(s string, parts ...bson.E) bson.D {
	return dateFromString(append([]bson.E{part("dateString", s)}, parts...)...)
}

func TestDateFromString(t *testing.T) {
	strings := []struct{ name, s string }{
		// ISO 8601 and close relatives.
		{"ISODate", "2017-02-08"},
		{"ISODateTime", "2017-02-08T12:10:40"},
		{"ISODateTimeMillis", "2017-02-08T12:10:40.787"},
		{"ISODateTimeZ", "2017-02-08T12:10:40.787Z"},
		{"ISOLowercaseT", "2017-02-08t12:10:40"},
		{"ISOOffsetCompact", "2017-02-08T12:10:40.787+0500"},
		{"ISOOffsetColon", "2017-02-08T12:10:40.787+05:00"},
		{"ISOOffsetNegative", "2017-02-08T12:10:40-0700"},
		{"ISOOffsetHoursOnly", "2017-02-08T12:10:40+05"},
		{"ISOOffsetSeconds", "2017-02-08T12:10:40+01:30:15"},
		{"ISOMicros", "2017-02-08T12:10:40.7871234"},
		{"ISOLongFraction", "2017-02-08T12:10:40.12345678901"},
		{"ISOFractionRoundsDown", "2017-02-08T12:10:40.9999"},
		{"ISOSpaceSeparator", "2017-02-08 12:10:40"},
		{"ISOHourMinute", "2017-02-08 12:10"},
		{"ISOTHour", "2017-02-08T12"},
		{"ISOSignedYear", "+2017-02-08"},
		{"ISONegativeYear", "-0044-03-15"},
		{"ISOExtendedYear", "+012017-02-08"},
		{"ISOTwoDigitYear", "17-02-08"},
		{"ISOTwoDigitYear69", "69-02-08"},
		{"ISOTwoDigitYear70", "70-02-08"},
		{"YearOne", "0001-01-01"},
		{"Year9999", "9999-12-31T23:59:59.999"},
		{"BeforeEpoch", "1969-12-31T23:59:59.9995"},
		{"SlashYMD", "2017/02/08"},
		{"SlashYMDShort", "2017/2/8"},
		{"SlashYMDTrailing", "2017/02/08/"},
		{"GnuDateShorter", "2017-2"},
		{"GnuDateShort", "2017-2-8"},
		{"GnuDateShortTwoDigit", "17-2-8"},
		{"DateNoColon", "20170208"},
		{"XMLRPC", "20170208T12:10:40"},
		{"XMLRPCNoColon", "20170208t121040"},
		{"SOAP", "2017-02-08T12:10:40.787-07:00"},
		{"WDDX", "2017-2-8T12:10:40"},
		{"EXIF", "2017:02:08 12:10:40"},
		{"PgDayOfYear", "2017.039"},
		{"PgDayOfYearNoSep", "2017039"},
		{"ISOWeekDay", "2017-W06-3"},
		{"ISOWeekDayCompact", "2017W063"},
		{"ISOWeekDaySunday", "2017-W06-7"},
		{"ISOWeekDayZero", "2017-W06-0"},
		{"ISOWeek", "2017-W06"},
		{"ISOWeek53", "2015-W53-5"},

		// American and other numeric orders.
		{"American", "02/08/2017"},
		{"AmericanShortYear", "2/8/17"},
		{"AmericanWithTime", "02/08/2017 12:10:40"},
		{"AmericanShort", "2/8"},
		{"PointedDate4", "08.02.2017"},
		{"PointedDateDash", "8-2-2017"},
		{"PointedDate2", "8.2.17"},

		// Textual months.
		{"MonthDayYear", "February 8, 2017"},
		{"MonthAbbrDayYear", "Feb 8 2017"},
		{"MonthDaySuffix", "feb 8th 2017"},
		{"MonthUppercase", "FEBRUARY 8 2017"},
		{"MonthSept", "Sept 8 2017"},
		{"DayMonthYear", "8 February 2017"},
		{"DayMonthYearDashes", "8-Feb-2017"},
		{"DayMonthYearDots", "8.Feb.2017"},
		{"DayRomanMonthYear", "8 II 2017"},
		{"MonthYear", "Feb 2017"},
		{"YearMonth", "2017 Feb"},
		{"PgTextShort", "Feb-08-2017"},
		{"PgTextReverse", "2017-Feb-08"},
		{"WeekdayMonthDayYear", "Wednesday, February 8, 2017"},
		{"WeekdayMismatch", "Monday, February 8, 2017"},
		{"JavaScriptToString", "Wed Feb 08 2017 12:10:40 GMT+0000"},
		{"JavaScriptWithZoneName", "Wed Feb 08 2017 12:10:40 GMT-0500 (EST)"},
		{"RFC2822", "Wed, 08 Feb 2017 12:10:40 +0000"},
		{"RFC2822GMT", "Wed, 08 Feb 2017 12:10:40 GMT"},
		{"RFC850", "Wednesday, 08-Feb-17 12:10:40 GMT"},
		{"ANSIC", "Wed Feb  8 12:10:40 2017"},
		{"CommonLogFormat", "08/Feb/2017:12:10:40 -0700"},
		{"MonthDayNoYear", "Feb 8"},
		{"MonthDayTimeNoYear", "Feb 8 12:10"},
		{"MonthDayYearTime12", "February 8 2017 12:10 pm"},
		{"MonthDayYearTime12Dots", "February 8 2017 12:10 p.m."},
		{"DateTextualTime24", "Feb 8 2017 12:10:40"},

		// Clock forms.
		{"Time12Long", "2017-02-08 12:10:40 PM"},
		{"Time12Short", "2017-02-08 1:30am"},
		{"Time12Tiny", "2017-02-08 1pm"},
		{"Time12Midnight", "2017-02-08 12am"},
		{"Time12Noon", "2017-02-08 12pm"},
		{"Time12DotSeparator", "2017-02-08 1.30pm"},
		{"MSSQLTime", "2017-02-08 12:30:00:123AM"},
		{"MSSQLTimeDot", "2017-02-08 1:30:00.5PM"},
		{"Time24Tiny", "2017-02-08 t13"},
		{"Time24DotSeparator", "2017-02-08 13.10.40"},
		{"Hour24", "2017-02-08 24:00"},
		{"LeapSecond", "2017-02-08 23:59:60"},
		{"GnuNoColon", "2017-02-08 1210"},
		{"GnuNoColonT", "2017-02-08T1210"},
		{"ISONoColon", "2017-02-08T121040"},
		{"TimeFirst", "12:10:40 2017-02-08"},

		// Zones inside the string.
		{"ZoneAbbrEST", "2017-02-08T12:10:40 EST"},
		{"ZoneAbbrEDT", "2017-02-08T12:10:40 EDT"},
		{"ZoneAbbrCEST", "2017-02-08 12:10:40 CEST"},
		{"ZoneAbbrLowercase", "2017-02-08 12:10:40 pst"},
		{"ZoneAbbrParens", "2017-02-08 12:10:40 (UTC)"},
		{"ZoneUT", "2017-02-08 12:10:40 UT"},
		{"ZoneMilitary", "2017-02-08 12:10:40 A"},
		{"ZoneMilitaryZ", "2017-02-08 12:10:40 z"},
		{"ZoneGMTOffset", "2017-02-08 12:10:40 GMT+1"},
		{"ZoneGMTOffsetMinutes", "2017-02-08 12:10:40 GMT-03:30"},
		{"ZoneIdentifier", "2017-02-08 12:10:40 Europe/London"},
		{"ZoneUnknownAbbr", "2017-02-08 12:10:40 XYZ"},
		{"ZoneOffsetTooLarge", "2017-02-08 12:10:40 +2500"},

		// Relative phrases applied to a full date.
		{"RelPlusDay", "2017-02-08 +1 day"},
		{"RelMinusWeeks", "2017-02-08 -2 weeks"},
		{"RelPlusMonthOverflow", "2017-01-31 +1 month"},
		{"RelPlusYear", "2016-02-29 +1 year"},
		{"RelMinutes", "2017-02-08 12:00 +90 min"},
		{"RelSeconds", "2017-02-08 +3600 secs"},
		{"RelFortnight", "2017-02-08 +1 fortnight"},
		{"RelMilliseconds", "2017-02-08 +1500 ms"},
		{"RelAgo", "2017-02-08 3 days ago"},
		{"RelCompound", "2017-02-08 +1 week 2 days 4 hours 2 seconds"},
		{"RelTextNext", "2017-02-08 next day"},
		{"RelTextLast", "2017-02-08 last year"},
		{"RelTextThird", "2017-02-08 third day"},
		{"RelWeekday", "2017-02-08 +2 weekdays"},
		{"RelWeekdayBackwards", "2017-02-11 -1 weekday"},
		{"NextWeekday", "2017-02-08 next monday"},
		{"LastWeekday", "2017-02-08 last friday"},
		{"WeekdayAlone", "2017-02-08 monday"},
		{"WeekdaySame", "2017-02-08 wednesday"},
		{"WeekdayPlural", "2017-02-08 mondays"},
		{"NextWeek", "2017-02-08 next week"},
		{"ThisWeek", "2017-02-08 this week"},
		{"FirstDayOf", "first day of February 2017"},
		{"LastDayOf", "last day of feb 2017"},
		{"LastDayOfNextMonth", "2017-01-31 last day of next month"},
		{"WeekdayOf", "second wednesday of February 2017"},
		{"LastWeekdayOf", "last friday of February 2017"},
		{"BackOf", "2017-02-08 back of 7pm"},
		{"FrontOf", "2017-02-08 front of 7"},
		{"Noon", "2017-02-08 noon"},
		{"Midnight", "2017-02-08 15:00 midnight"},
		{"Today", "2017-02-08 15:00 today"},
		{"Tomorrow", "2017-02-08 tomorrow"},
		{"YesterdayNoon", "2017-02-08 yesterday noon"},
		{"Now", "2017-02-08 now"},

		// Timestamps.
		{"Timestamp", "@1500000000"},
		{"TimestampNegative", "@-1"},
		{"TimestampMillis", "@1500000000.123"},
		{"TimestampWithZone", "@1500000000 EST"},

		// Rejected or incomplete strings.
		{"Empty", ""},
		{"Whitespace", "   "},
		{"Garbage", "foo"},
		{"TrimmedWhitespace", "  2017-02-08  "},
		{"TabSeparator", "2017-02-08\t12:10"},
		{"NonBreakingSpace", "2017-02-08\xc2\xa012:10"},
		{"InvalidDay", "2017-02-30"},
		{"InvalidLeapDay", "2017-02-29"},
		{"LeapDay", "2016-02-29"},
		{"Century1900LeapDay", "1900-02-29"},
		{"InvalidMonth", "2017-13-01"},
		{"InvalidHour", "2017-02-08T25:00:00"},
		{"InvalidMinute", "2017-02-08 12:60"},
		{"DoubleDate", "2017-02-08 2017-02-09"},
		{"DoubleTime", "2017-02-08 12:10 13:10"},
		{"DoubleZone", "2017-02-08 12:10 EST PST"},
		{"TimeOnly", "12:10:40"},
		{"NowOnly", "now"},
		{"TodayOnly", "today"},
		{"RelativeOnly", "+1 day"},
		{"TrailingGarbage", "2017-02-08 xyzzy"},
		{"TrailingDigits", "2017-02-08 12:10:40 99"},
		{"Unicode", "2017-02-08T12:10:40\xc3\xa9"},
	}
	cases := make([]mathCase, 0, len(strings)+64)
	for _, s := range strings {
		cases = append(cases, mathCase{s.name, dateString(s.s)})
	}

	cases = append(cases,
		mathCase{"TzOlson", dateString("2017-02-08T12:10:40", part("timezone", "America/New_York"))},
		mathCase{"TzOffset", dateString("2017-02-08T12:10:40", part("timezone", "+05:30"))},
		mathCase{"TzDSTGap", dateString("2020-03-08 02:30", part("timezone", "America/New_York"))},
		mathCase{"TzDSTOverlap", dateString("2020-11-01 01:30", part("timezone", "America/New_York"))},
		mathCase{"TzWithStringZ", dateString("2017-02-08T12:10:40Z", part("timezone", "America/New_York"))},
		mathCase{"TzWithStringOffset", dateString("2017-02-08T12:10:40+0500", part("timezone", "Europe/Paris"))},
		mathCase{"TzWithStringAbbr", dateString("2017-02-08T12:10:40 EST", part("timezone", "+01:00"))},
		mathCase{"TzUTCWithStringOffset", dateString("2017-02-08T12:10:40+0500", part("timezone", "UTC"))},
		mathCase{"TzGMTWithStringOffset", dateString("2017-02-08T12:10:40+0500", part("timezone", "GMT"))},
		mathCase{"TzZeroOffsetWithStringOffset", dateString("2017-02-08T12:10:40+0500", part("timezone", "+00:00"))},
		mathCase{"TzWithTimestamp", dateString("@1500000000", part("timezone", "Asia/Tokyo"))},
		mathCase{"TzWithRelative", dateString("2017-03-11 22:00 +1 day", part("timezone", "America/New_York"))},
		mathCase{"TzInvalid", dateString("2017-02-08", part("timezone", "Not/AZone"))},
		mathCase{"TzNumber", dateString("2017-02-08", part("timezone", int32(5)))},
		mathCase{"TzNull", dateString("2017-02-08", part("timezone", nil))},
		mathCase{"TzField", dateFromString(part("dateString", "$ds"), part("timezone", "$tz"))},

		mathCase{"DateStringNull", dateFromString(part("dateString", nil))},
		mathCase{"DateStringMissing", dateFromString(part("dateString", "$missing"))},
		mathCase{"DateStringField", dateFromString(part("dateString", "$ds"))},
		mathCase{"DateStringNumber", dateFromString(part("dateString", int32(5)))},
		mathCase{"DateStringDate", dateFromString(part("dateString", "$date"))},
		mathCase{"OnNullValue", dateFromString(part("dateString", nil), part("onNull", "none"))},
		mathCase{"OnNullExpression", dateFromString(part("dateString", "$missing"), part("onNull", "$date"))},
		mathCase{"OnNullNull", dateFromString(part("dateString", nil), part("onNull", nil))},
		mathCase{"OnNullUnused", dateString("2017-02-08", part("onNull", "none"))},
		mathCase{"OnNullBadTimezone", dateFromString(part("dateString", nil), part("timezone", "Not/AZone"), part("onNull", "none"))},
		mathCase{"OnNullNullTimezone", dateFromString(part("dateString", nil), part("timezone", nil), part("onNull", "none"))},
		mathCase{"OnErrorValue", dateString("foo", part("onError", "bad"))},
		mathCase{"OnErrorExpression", dateString("foo", part("onError", "$date"))},
		mathCase{"OnErrorNull", dateString("foo", part("onError", nil))},
		mathCase{"OnErrorNonString", dateFromString(part("dateString", int32(5)), part("onError", "bad"))},
		mathCase{"OnErrorTimezoneConflict", dateString("2017-02-08Z", part("timezone", "Europe/Paris"), part("onError", "bad"))},
		mathCase{"OnErrorBadTimezoneNotCaught", dateString("foo", part("timezone", "Not/AZone"), part("onError", "bad"))},
		mathCase{"OnErrorAndOnNull", dateFromString(part("dateString", nil), part("onError", "bad"), part("onNull", "none"))},
		mathCase{"OnErrorNullTimezone", dateString("foo", part("timezone", nil), part("onError", "bad"))},

		mathCase{"FormatYMD", dateString("2017-02-08", part("format", "%Y-%m-%d"))},
		mathCase{"FormatDMY", dateString("08/02/2017", part("format", "%d/%m/%Y"))},
		mathCase{"FormatFull", dateString("2017-02-08T12:10:40.787", part("format", "%Y-%m-%dT%H:%M:%S.%L"))},
		mathCase{"FormatCompact", dateString("20170208", part("format", "%Y%m%d"))},
		mathCase{"FormatISOWeek", dateString("2017-W06-3", part("format", "%G-W%V-%u"))},
		mathCase{"FormatDayOfYear", dateString("2017-039", part("format", "%Y-%j"))},
		mathCase{"FormatMonthName", dateString("8 February 2017", part("format", "%d %B %Y"))},
		mathCase{"FormatMonthAbbr", dateString("8 Feb 2017", part("format", "%d %b %Y"))},
		mathCase{"FormatOffset", dateString("2017-02-08T12:10:40+0530", part("format", "%Y-%m-%dT%H:%M:%S%z"))},
		mathCase{"FormatOffsetMinutes", dateString("2017-02-08T12:10:40+330", part("format", "%Y-%m-%dT%H:%M:%S%Z"))},
		mathCase{"FormatLiteralPercent", dateString("2017%02%08", part("format", "%Y%%%m%%%d"))},
		mathCase{"FormatNoTime", dateString("2017-02-08", part("format", "%Y-%m-%d"), part("timezone", "Asia/Tokyo"))},
		mathCase{"FormatHourMinuteOnly", dateString("2017-02-08 12:10", part("format", "%Y-%m-%d %H:%M"))},
		mathCase{"FormatNoDay", dateString("2017-02", part("format", "%Y-%m"))},
		mathCase{"FormatMismatch", dateString("2017-02-08", part("format", "%d/%m/%Y"))},
		mathCase{"FormatTrailingData", dateString("2017-02-08 extra", part("format", "%Y-%m-%d"))},
		mathCase{"FormatShortString", dateString("2017-02", part("format", "%Y-%m-%d"))},
		mathCase{"FormatInvalidDay", dateString("2017-02-30", part("format", "%Y-%m-%d"))},
		mathCase{"FormatOneDigitMonth", dateString("2017-2-8", part("format", "%Y-%m-%d"))},
		mathCase{"FormatUnknownSpecifier", dateString("2017", part("format", "%Y %x"))},
		mathCase{"FormatTrailingPercent", dateString("2017", part("format", "%Y%"))},
		mathCase{"FormatNumber", dateString("2017-02-08", part("format", int32(5)))},
		mathCase{"FormatNull", dateString("2017-02-08", part("format", nil))},
		mathCase{"FormatNullDateString", dateFromString(part("dateString", nil), part("format", "%Y"))},
		mathCase{"FormatBadWithNullDateString", dateFromString(part("dateString", nil), part("format", "%x"))},
		mathCase{"FormatField", dateFromString(part("dateString", "$dmy"), part("format", "$fmt"))},
		mathCase{"FormatMismatchOnError", dateString("2017-02-08", part("format", "%d/%m/%Y"), part("onError", "bad"))},
		mathCase{"FormatWithTimezone", dateString("2017-02-08 12:10", part("format", "%Y-%m-%d %H:%M"), part("timezone", "-03:00"))},

		mathCase{"NoDateString", dateFromString(part("format", "%Y"))},
		mathCase{"UnknownField", dateString("2017-02-08", part("date", "x"))},
		mathCase{"NonObjectArgument", bson.D{{Key: "$dateFromString", Value: "2017-02-08"}}},
		mathCase{"ArrayArgument", bson.D{{Key: "$dateFromString", Value: bson.A{bson.D{{Key: "dateString", Value: "2017-02-08"}}}}}},
	)

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateFromString_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   insertDateStringDocs,
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
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: dateFromString(part("dateString", "$ds"), part("onError", "bad"))}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
				dateFromString(part("dateString", "$ds"), part("onError", nil)),
				dateString("2010-01-01"),
			}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupKey", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: dateFromString(part("dateString", "$ds"), part("onError", "bad"))}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"ErrorWithoutOnError", bson.A{
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: dateFromString(part("dateString", "$ds"))}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateFromString_" + p.name,
				Support: harness.DumboDBXFail,
				Setup:   insertDateStringDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
