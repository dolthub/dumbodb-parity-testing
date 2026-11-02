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

func dateToParts(parts ...bson.E) bson.D {
	return bson.D{{Key: "$dateToParts", Value: bson.D(parts)}}
}

func TestDateToParts(t *testing.T) {
	utc := func(y int, m time.Month, d, h, mi, s, ms int) time.Time {
		return time.Date(y, m, d, h, mi, s, ms*1e6, time.UTC)
	}
	sample := utc(2017, 1, 1, 1, 29, 9, 123)
	cases := []mathCase{
		{"Basic", dateToParts(part("date", sample))},
		{"ISO", dateToParts(part("date", sample), part("iso8601", true))},
		{"ISOFalse", dateToParts(part("date", sample), part("iso8601", false))},
		{"FieldDate", dateToParts(part("date", "$date"))},
		{"FieldDateISO", dateToParts(part("date", "$date"), part("iso8601", true))},
		{"Epoch", dateToParts(part("date", time.Unix(0, 0)))},
		{"BeforeEpoch", dateToParts(part("date", utc(1969, 12, 31, 23, 59, 59, 999)))},
		{"BeforeEpochMillis", dateToParts(part("date", primitive.DateTime(-1)))},
		{"YearOne", dateToParts(part("date", utc(1, 1, 1, 0, 0, 0, 0)))},
		{"Year9999", dateToParts(part("date", utc(9999, 12, 31, 23, 59, 59, 999)))},
		{"BeforeYearOne", dateToParts(part("date", primitive.DateTime(-62135596800001)))},
		{"AfterYear9999", dateToParts(part("date", primitive.DateTime(253402300800000)))},
		{"FarPast", dateToParts(part("date", primitive.DateTime(-1e15)))},
		{"FarFuture", dateToParts(part("date", primitive.DateTime(1e15)))},
		{"FarPastISO", dateToParts(part("date", primitive.DateTime(-1e15)), part("iso8601", true))},
		{"LeapDay", dateToParts(part("date", utc(2020, 2, 29, 12, 0, 0, 0)))},

		{"ISOYearBoundaryJan1", dateToParts(part("date", utc(2021, 1, 1, 0, 0, 0, 0)), part("iso8601", true))},
		{"ISOYearBoundaryDec31", dateToParts(part("date", utc(2019, 12, 31, 0, 0, 0, 0)), part("iso8601", true))},
		{"ISOWeek53", dateToParts(part("date", utc(2020, 12, 31, 0, 0, 0, 0)), part("iso8601", true))},
		{"ISOSunday", dateToParts(part("date", utc(2017, 1, 1, 0, 0, 0, 0)), part("iso8601", true))},
		{"ISOYearOne", dateToParts(part("date", utc(1, 1, 1, 0, 0, 0, 0)), part("iso8601", true))},

		{"TimezoneOlson", dateToParts(part("date", sample), part("timezone", "America/New_York"))},
		{"TimezoneOlsonISO", dateToParts(part("date", sample), part("timezone", "America/New_York"), part("iso8601", true))},
		{"TimezoneOffset", dateToParts(part("date", sample), part("timezone", "+05:30"))},
		{"TimezoneOffsetNegative", dateToParts(part("date", sample), part("timezone", "-0230"))},
		{"TimezoneOffsetLarge", dateToParts(part("date", sample), part("timezone", "+25:00"))},
		{"TimezoneUTC", dateToParts(part("date", sample), part("timezone", "UTC"))},
		{"TimezoneField", dateToParts(part("date", "$date"), part("timezone", "$tz"))},
		{"TimezoneHalfHour", dateToParts(part("date", sample), part("timezone", "Asia/Kathmandu"))},
		{"TimezoneDSTSummer", dateToParts(part("date", utc(2020, 7, 1, 3, 0, 0, 0)), part("timezone", "Europe/London"))},
		{"TimezoneDSTTransition", dateToParts(part("date", utc(2020, 3, 8, 7, 0, 0, 0)), part("timezone", "America/New_York"))},
		{"TimezoneHistorical", dateToParts(part("date", utc(1900, 1, 1, 0, 0, 0, 0)), part("timezone", "Europe/Amsterdam"))},
		{"TimezoneBeforeEpoch", dateToParts(part("date", primitive.DateTime(-1)), part("timezone", "+01:00"))},
		{"TimezoneInvalid", dateToParts(part("date", sample), part("timezone", "Not/AZone"))},
		{"TimezoneNumber", dateToParts(part("date", sample), part("timezone", int32(1)))},
		{"TimezoneNull", dateToParts(part("date", sample), part("timezone", nil))},
		{"TimezoneMissingField", dateToParts(part("date", sample), part("timezone", "$missing"))},

		{"Timestamp", dateToParts(part("date", primitive.Timestamp{T: 1500000000, I: 7}))},
		{"ObjectID", dateToParts(part("date", primitive.NewObjectIDFromTimestamp(sample)))},
		{"DateNull", dateToParts(part("date", nil))},
		{"DateMissingField", dateToParts(part("date", "$missing"))},
		{"DateString", dateToParts(part("date", "2017-01-01"))},
		{"DateNumber", dateToParts(part("date", int64(0)))},
		{"DateDouble", dateToParts(part("date", 1.5))},
		{"DateNullBadTimezone", dateToParts(part("date", nil), part("timezone", "Not/AZone"))},
		{"DateNullBadISO", dateToParts(part("date", nil), part("iso8601", "x"))},
		{"BadDateNullTimezone", dateToParts(part("date", "x"), part("timezone", nil))},

		{"ISOString", dateToParts(part("date", sample), part("iso8601", "true"))},
		{"ISONumber", dateToParts(part("date", sample), part("iso8601", int32(1)))},
		{"ISONull", dateToParts(part("date", sample), part("iso8601", nil))},
		{"ISOMissingField", dateToParts(part("date", sample), part("iso8601", "$missing"))},
		{"ISOExpression", dateToParts(part("date", sample), part("iso8601", bson.D{{Key: "$eq", Value: bson.A{1, 1}}}))},

		{"NoDate", dateToParts(part("timezone", "UTC"))},
		{"Empty", bson.D{{Key: "$dateToParts", Value: bson.D{}}}},
		{"UnknownField", dateToParts(part("date", sample), part("format", "x"))},
		{"NonObjectArgument", bson.D{{Key: "$dateToParts", Value: sample}}},
		{"ArrayArgument", bson.D{{Key: "$dateToParts", Value: bson.A{bson.D{{Key: "date", Value: sample}}}}}},
		{"NullArgument", bson.D{{Key: "$dateToParts", Value: nil}}},
	}
	runDateCases(t, "DateToParts", harness.DumboDBFull, cases)

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AddFieldsAllDocs", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: dateToParts(part("date", dateFromParts(part("year", "$y"), part("month", "$m"))), part("timezone", "$tz"))}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{
				bson.D{{Key: "$getField", Value: bson.D{{Key: "field", Value: "hour"}, {Key: "input", Value: dateToParts(part("date", "$date"), part("timezone", "$tz"))}}}},
				int32(19),
			}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"GroupKey", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: dateToParts(part("date", dateFromParts(part("year", "$y"))))}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id.year", Value: 1}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateToParts_" + p.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
