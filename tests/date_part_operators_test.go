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
		// Not $add, which does not yet accept dates (workspace-bb0).
		{"Expression", bson.D{{Key: "$dateSubtract", Value: bson.D{
			{Key: "startDate", Value: sample}, {Key: "unit", Value: "day"}, {Key: "amount", Value: int32(-1)},
		}}}},
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

	runDateCases(t, "DatePart", harness.DumboDBFull, cases)
}
