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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// The date part operators ($year ... $isoDayOfWeek) and $dateToString over
// their argument forms, time zones and the edges of the date range.

// datePartAlreadyMatching are the cases DumboDB matched before the date part
// operators took time zones and MongoDB's argument forms (workspace-bl0).
var datePartAlreadyMatching = map[string]bool{
	"DateToString_BeforeEpoch":           true,
	"DateToString_BeforeYearOne":         true,
	"DateToString_DateMissing":           true,
	"DateToString_DateNull":              true,
	"DateToString_Default":               true,
	"DateToString_DefaultUTC":            true,
	"DateToString_DefaultZeroOffset":     true,
	"DateToString_Literal":               true,
	"DateToString_OnNull":                true,
	"DateToString_OnNullUnused":          true,
	"DateToString_TimezoneNullDateNull":  true,
	"DateToString_YearOne":               true,
	"dayOfMonth_AfterYear9999":           true,
	"dayOfMonth_BeforeEpoch":             true,
	"dayOfMonth_BeforeYearOne":           true,
	"dayOfMonth_Date":                    true,
	"dayOfMonth_Expression":              true,
	"dayOfMonth_FarFuture":               true,
	"dayOfMonth_FieldPath":               true,
	"dayOfMonth_ISOWeek53":               true,
	"dayOfMonth_ISOYearBoundary":         true,
	"dayOfMonth_LeapDay":                 true,
	"dayOfMonth_MissingField":            true,
	"dayOfMonth_Null":                    true,
	"dayOfMonth_SingleElementArray":      true,
	"dayOfMonth_WeekBeforeFirstSunday":   true,
	"dayOfMonth_WeekLastDay":             true,
	"dayOfMonth_YearOne":                 true,
	"dayOfWeek_AfterYear9999":            true,
	"dayOfWeek_BeforeEpoch":              true,
	"dayOfWeek_BeforeYearOne":            true,
	"dayOfWeek_Date":                     true,
	"dayOfWeek_FarFuture":                true,
	"dayOfWeek_FieldPath":                true,
	"dayOfWeek_ISOWeek53":                true,
	"dayOfWeek_ISOYearBoundary":          true,
	"dayOfWeek_LeapDay":                  true,
	"dayOfWeek_MissingField":             true,
	"dayOfWeek_Null":                     true,
	"dayOfWeek_SingleElementArray":       true,
	"dayOfWeek_WeekBeforeFirstSunday":    true,
	"dayOfWeek_WeekLastDay":              true,
	"dayOfWeek_YearOne":                  true,
	"dayOfYear_AfterYear9999":            true,
	"dayOfYear_BeforeEpoch":              true,
	"dayOfYear_BeforeYearOne":            true,
	"dayOfYear_Date":                     true,
	"dayOfYear_Expression":               true,
	"dayOfYear_FarFuture":                true,
	"dayOfYear_FieldPath":                true,
	"dayOfYear_ISOWeek53":                true,
	"dayOfYear_ISOYearBoundary":          true,
	"dayOfYear_LeapDay":                  true,
	"dayOfYear_MissingField":             true,
	"dayOfYear_Null":                     true,
	"dayOfYear_SingleElementArray":       true,
	"dayOfYear_WeekBeforeFirstSunday":    true,
	"dayOfYear_WeekLastDay":              true,
	"dayOfYear_YearOne":                  true,
	"hour_AfterYear9999":                 true,
	"hour_BeforeEpoch":                   true,
	"hour_BeforeYearOne":                 true,
	"hour_Date":                          true,
	"hour_FarFuture":                     true,
	"hour_FieldPath":                     true,
	"hour_ISOWeek53":                     true,
	"hour_ISOYearBoundary":               true,
	"hour_LeapDay":                       true,
	"hour_MissingField":                  true,
	"hour_Null":                          true,
	"hour_SingleElementArray":            true,
	"hour_WeekBeforeFirstSunday":         true,
	"hour_WeekLastDay":                   true,
	"hour_YearOne":                       true,
	"isoDayOfWeek_AfterYear9999":         true,
	"isoDayOfWeek_BeforeEpoch":           true,
	"isoDayOfWeek_BeforeYearOne":         true,
	"isoDayOfWeek_Date":                  true,
	"isoDayOfWeek_FarFuture":             true,
	"isoDayOfWeek_FieldPath":             true,
	"isoDayOfWeek_ISOWeek53":             true,
	"isoDayOfWeek_ISOYearBoundary":       true,
	"isoDayOfWeek_LeapDay":               true,
	"isoDayOfWeek_MissingField":          true,
	"isoDayOfWeek_Null":                  true,
	"isoDayOfWeek_SingleElementArray":    true,
	"isoDayOfWeek_WeekBeforeFirstSunday": true,
	"isoDayOfWeek_WeekLastDay":           true,
	"isoDayOfWeek_YearOne":               true,
	"isoWeekYear_MissingField":           true,
	"isoWeekYear_Null":                   true,
	"isoWeek_AfterYear9999":              true,
	"isoWeek_BeforeEpoch":                true,
	"isoWeek_BeforeYearOne":              true,
	"isoWeek_Date":                       true,
	"isoWeek_Expression":                 true,
	"isoWeek_FarFuture":                  true,
	"isoWeek_FieldPath":                  true,
	"isoWeek_ISOWeek53":                  true,
	"isoWeek_ISOYearBoundary":            true,
	"isoWeek_LeapDay":                    true,
	"isoWeek_MissingField":               true,
	"isoWeek_Null":                       true,
	"isoWeek_SingleElementArray":         true,
	"isoWeek_WeekBeforeFirstSunday":      true,
	"isoWeek_WeekLastDay":                true,
	"isoWeek_YearOne":                    true,
	"millisecond_AfterYear9999":          true,
	"millisecond_BeforeEpoch":            true,
	"millisecond_BeforeYearOne":          true,
	"millisecond_Date":                   true,
	"millisecond_FarFuture":              true,
	"millisecond_FieldPath":              true,
	"millisecond_ISOWeek53":              true,
	"millisecond_ISOYearBoundary":        true,
	"millisecond_LeapDay":                true,
	"millisecond_MissingField":           true,
	"millisecond_Null":                   true,
	"millisecond_SingleElementArray":     true,
	"millisecond_WeekBeforeFirstSunday":  true,
	"millisecond_WeekLastDay":            true,
	"millisecond_YearOne":                true,
	"minute_AfterYear9999":               true,
	"minute_BeforeEpoch":                 true,
	"minute_BeforeYearOne":               true,
	"minute_Date":                        true,
	"minute_FarFuture":                   true,
	"minute_FieldPath":                   true,
	"minute_ISOWeek53":                   true,
	"minute_ISOYearBoundary":             true,
	"minute_LeapDay":                     true,
	"minute_MissingField":                true,
	"minute_Null":                        true,
	"minute_SingleElementArray":          true,
	"minute_WeekBeforeFirstSunday":       true,
	"minute_WeekLastDay":                 true,
	"minute_YearOne":                     true,
	"month_AfterYear9999":                true,
	"month_BeforeEpoch":                  true,
	"month_BeforeYearOne":                true,
	"month_Date":                         true,
	"month_Expression":                   true,
	"month_FarFuture":                    true,
	"month_FieldPath":                    true,
	"month_ISOWeek53":                    true,
	"month_ISOYearBoundary":              true,
	"month_LeapDay":                      true,
	"month_MissingField":                 true,
	"month_Null":                         true,
	"month_SingleElementArray":           true,
	"month_WeekBeforeFirstSunday":        true,
	"month_WeekLastDay":                  true,
	"month_YearOne":                      true,
	"second_AfterYear9999":               true,
	"second_BeforeEpoch":                 true,
	"second_BeforeYearOne":               true,
	"second_Date":                        true,
	"second_FarFuture":                   true,
	"second_FieldPath":                   true,
	"second_ISOWeek53":                   true,
	"second_ISOYearBoundary":             true,
	"second_LeapDay":                     true,
	"second_MissingField":                true,
	"second_Null":                        true,
	"second_SingleElementArray":          true,
	"second_WeekBeforeFirstSunday":       true,
	"second_WeekLastDay":                 true,
	"second_YearOne":                     true,
	"week_AfterYear9999":                 true,
	"week_BeforeEpoch":                   true,
	"week_BeforeYearOne":                 true,
	"week_FarFuture":                     true,
	"week_FieldPath":                     true,
	"week_ISOWeek53":                     true,
	"week_ISOYearBoundary":               true,
	"week_LeapDay":                       true,
	"week_MissingField":                  true,
	"week_Null":                          true,
	"week_WeekBeforeFirstSunday":         true,
	"week_WeekLastDay":                   true,
	"week_YearOne":                       true,
	"year_AfterYear9999":                 true,
	"year_BeforeEpoch":                   true,
	"year_BeforeYearOne":                 true,
	"year_Date":                          true,
	"year_FarFuture":                     true,
	"year_FieldPath":                     true,
	"year_ISOWeek53":                     true,
	"year_ISOYearBoundary":               true,
	"year_LeapDay":                       true,
	"year_MissingField":                  true,
	"year_Null":                          true,
	"year_SingleElementArray":            true,
	"year_WeekBeforeFirstSunday":         true,
	"year_WeekLastDay":                   true,
	"year_YearOne":                       true,
}

func TestDatePartOperators(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi, s, ms int) time.Time {
		return time.Date(y, m, d, h, mi, s, ms*1e6, time.UTC)
	}
	sample := utc(2017, 1, 1, 1, 29, 9, 123)
	withTZ := func(date, tz interface{}) bson.D {
		return bson.D{{Key: "date", Value: date}, {Key: "timezone", Value: tz}}
	}
	inputs := []struct {
		name string
		arg  interface{}
	}{
		{"Date", sample},
		{"FieldPath", "$date"},
		{"SingleElementArray", bson.A{sample}},
		{"ObjectForm", bson.D{{Key: "date", Value: sample}}},
		{"Expression", bson.D{{Key: "$add", Value: bson.A{sample, int64(86400000)}}}},
		{"TimezoneOlson", withTZ(sample, "America/New_York")},
		{"TimezoneOffset", withTZ(sample, "+05:30")},
		{"TimezoneNegativeOffset", withTZ(sample, "-0230")},
		{"TimezoneHalfHourZone", withTZ(sample, "Asia/Kathmandu")},
		{"TimezoneUTC", withTZ(sample, "UTC")},
		{"TimezoneField", withTZ("$date", "$tz")},
		{"TimezoneDSTSummer", withTZ(utc(2020, 7, 1, 3, 0, 0, 0), "Europe/London")},
		{"TimezoneHistorical", withTZ(utc(1900, 1, 1, 0, 0, 0, 0), "Europe/Amsterdam")},
		{"TimezoneInvalid", withTZ(sample, "Not/AZone")},
		{"TimezoneNumber", withTZ(sample, int32(1))},
		{"TimezoneNull", withTZ(sample, nil)},
		{"TimezoneMissingField", withTZ(sample, "$missing")},
		{"BeforeEpoch", primitive.DateTime(-1)},
		{"BeforeEpochOffset", withTZ(primitive.DateTime(-1), "+01:00")},
		{"YearOne", utc(1, 1, 1, 0, 0, 0, 0)},
		{"BeforeYearOne", primitive.DateTime(-62135596800001)},
		{"AfterYear9999", primitive.DateTime(253402300800000)},
		{"FarFuture", primitive.DateTime(1e15)},
		{"ISOYearBoundary", utc(2021, 1, 1, 0, 0, 0, 0)},
		{"ISOWeek53", utc(2020, 12, 31, 0, 0, 0, 0)},
		{"WeekBeforeFirstSunday", utc(2022, 1, 1, 0, 0, 0, 0)},
		{"WeekLastDay", utc(2022, 12, 31, 0, 0, 0, 0)},
		{"LeapDay", utc(2020, 2, 29, 23, 59, 59, 999)},
		{"Timestamp", primitive.Timestamp{T: 1500000000, I: 7}},
		{"ObjectID", primitive.NewObjectIDFromTimestamp(sample)},
		{"Null", nil},
		{"MissingField", "$missing"},
		{"NullInObjectForm", bson.D{{Key: "date", Value: nil}}},
		{"NullDateBadTimezone", withTZ(nil, "Not/AZone")},
		{"String", "2017-01-01"},
		{"Number", int64(0)},
		{"Double", 1.5},
		{"TwoElementArray", bson.A{sample, sample}},
		{"EmptyArray", bson.A{}},
		{"ObjectFormNoDate", bson.D{{Key: "timezone", Value: "UTC"}}},
		{"ObjectFormUnknownField", bson.D{{Key: "date", Value: sample}, {Key: "format", Value: "x"}}},
		{"ArrayOfObjectForm", bson.A{bson.D{{Key: "date", Value: sample}}}},
	}
	ops := []string{"$year", "$month", "$dayOfMonth", "$hour", "$minute", "$second", "$millisecond",
		"$dayOfWeek", "$dayOfYear", "$week", "$isoWeek", "$isoWeekYear", "$isoDayOfWeek"}

	var cases []mathCase
	for _, op := range ops {
		for _, in := range inputs {
			cases = append(cases, mathCase{op[1:] + "_" + in.name, bson.D{{Key: op, Value: in.arg}}})
		}
	}

	toString := func(parts ...bson.E) bson.D { return bson.D{{Key: "$dateToString", Value: bson.D(parts)}} }
	allSpecifiers := "%Y-%m-%d %H:%M:%S.%L|%j|%w|%U|%G|%V|%u|%z|%Z|%B|%b|%%"
	cases = append(cases,
		mathCase{"DateToString_Default", toString(part("date", sample))},
		mathCase{"DateToString_DefaultTimezone", toString(part("date", sample), part("timezone", "America/New_York"))},
		mathCase{"DateToString_DefaultUTC", toString(part("date", sample), part("timezone", "UTC"))},
		mathCase{"DateToString_DefaultZeroOffset", toString(part("date", sample), part("timezone", "+00:00"))},
		mathCase{"DateToString_AllSpecifiers", toString(part("date", sample), part("format", allSpecifiers))},
		mathCase{"DateToString_AllSpecifiersOlson", toString(part("date", sample), part("format", allSpecifiers), part("timezone", "America/New_York"))},
		mathCase{"DateToString_AllSpecifiersHalfHour", toString(part("date", sample), part("format", allSpecifiers), part("timezone", "Asia/Kathmandu"))},
		mathCase{"DateToString_NegativeOffset", toString(part("date", sample), part("format", "%z %Z"), part("timezone", "-0230"))},
		mathCase{"DateToString_FieldDate", toString(part("date", "$date"), part("timezone", "$tz"))},
		mathCase{"DateToString_Literal", toString(part("date", sample), part("format", "date: %Y, done"))},
		mathCase{"DateToString_BeforeEpoch", toString(part("date", primitive.DateTime(-1)))},
		mathCase{"DateToString_YearOne", toString(part("date", utc(1, 1, 1, 0, 0, 0, 0)))},
		mathCase{"DateToString_YearOutOfRange", toString(part("date", primitive.DateTime(253402300800000)))},
		mathCase{"DateToString_BeforeYearOne", toString(part("date", primitive.DateTime(-62135596800001)))},
		mathCase{"DateToString_Timestamp", toString(part("date", primitive.Timestamp{T: 1500000000, I: 7}))},
		mathCase{"DateToString_ObjectID", toString(part("date", primitive.NewObjectIDFromTimestamp(sample)))},
		mathCase{"DateToString_String", toString(part("date", "2017-01-01"))},
		mathCase{"DateToString_Number", toString(part("date", int64(0)))},
		mathCase{"DateToString_DateNull", toString(part("date", nil))},
		mathCase{"DateToString_DateMissing", toString(part("date", "$missing"))},
		mathCase{"DateToString_OnNull", toString(part("date", nil), part("onNull", "none"))},
		mathCase{"DateToString_OnNullUnused", toString(part("date", sample), part("onNull", "none"))},
		mathCase{"DateToString_FormatNull", toString(part("date", sample), part("format", nil))},
		mathCase{"DateToString_FormatNumber", toString(part("date", sample), part("format", int32(1)))},
		mathCase{"DateToString_FormatInvalidSpecifier", toString(part("date", sample), part("format", "%Q"))},
		mathCase{"DateToString_FormatTrailingPercent", toString(part("date", sample), part("format", "%Y%"))},
		mathCase{"DateToString_FormatBadDateNull", toString(part("date", nil), part("format", "%Q"))},
		mathCase{"DateToString_TimezoneNull", toString(part("date", sample), part("timezone", nil))},
		mathCase{"DateToString_TimezoneNullDateNull", toString(part("date", nil), part("timezone", nil), part("onNull", "none"))},
		mathCase{"DateToString_TimezoneInvalid", toString(part("date", sample), part("timezone", "Not/AZone"))},
		mathCase{"DateToString_NoDate", toString(part("format", "%Y"))},
		mathCase{"DateToString_UnknownField", toString(part("date", sample), part("zone", "UTC"))},
		mathCase{"DateToString_NonObject", bson.D{{Key: "$dateToString", Value: sample}}},
	)

	var pending, matching []mathCase
	for _, c := range cases {
		if datePartAlreadyMatching[c.name] {
			matching = append(matching, c)
		} else {
			pending = append(pending, c)
		}
	}
	runDateCases(t, "DatePart", harness.DumboDBFull, matching)
	runDateCases(t, "DatePart", harness.DumboDBXFail, pending)
}
