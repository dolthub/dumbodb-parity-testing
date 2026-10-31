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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Date edge cases from the review of #184 (workspace-m8c.9): extreme dates
// with zones that move them past the int64 range of milliseconds, onNull
// and onError that evaluate to missing, and constant onError/onNull
// expressions, which mongod evaluates when it parses them.

func TestDateEdgesReview(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		out := bson.D{}
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	maxDate, minDate := primitive.DateTime(math.MaxInt64), primitive.DateTime(math.MinInt64)
	part := func(op string, date interface{}, tz string) bson.D { return d(op, d("date", date, "timezone", tz)) }
	fromString := func(kv ...interface{}) bson.D { return d("$dateFromString", d(kv...)) }
	failing := d("$add", bson.A{int32(1), "a"})

	cases := []mathCase{
		{"YearMaxPositiveOffset", part("$year", maxDate, "+0530")},
		{"HourMaxPositiveOffset", part("$hour", maxDate, "+0530")},
		{"YearMaxNamedZone", part("$year", maxDate, "Asia/Kolkata")},
		{"DayOfYearMaxNamedZone", part("$dayOfYear", maxDate, "Asia/Kolkata")},
		{"YearMinNegativeOffset", part("$year", minDate, "-0530")},
		{"MinuteMinNegativeOffset", part("$minute", minDate, "-0530")},
		{"YearMinNamedZone", part("$year", minDate, "America/New_York")},
		{"YearMaxUTC", part("$year", maxDate, "UTC")},
		{"DateToPartsMaxOffset", d("$dateToParts", d("date", maxDate, "timezone", "+0530"))},
		{"IsoWeekYearMaxOffset", part("$isoWeekYear", maxDate, "+1400")},

		{"FromStringOnNullMissing", d("x", fromString("dateString", nil, "onNull", "$nope"), "y", int32(1))},
		{"FromStringOnErrorMissing", d("x", fromString("dateString", "bad", "onError", "$nope"), "y", int32(1))},
		{"FromStringOnNullMissingTop", fromString("dateString", nil, "onNull", "$nope")},
		{"FromStringOnErrorRemove", fromString("dateString", "bad", "onError", "$$REMOVE")},
		{"ToStringOnNullMissing", d("x", d("$dateToString", d("date", nil, "onNull", "$nope")), "y", int32(1))},
		{"ToStringOnNullMissingTop", d("$dateToString", d("date", "$nope", "onNull", "$nope"))},

		{"FromStringConstantOnErrorFails", fromString("dateString", "2021-01-01", "onError", failing)},
		{"FromStringConstantOnNullFails", fromString("dateString", "2021-01-01", "onNull", failing)},
		{"FromStringConstantOnErrorFailsBadInput", fromString("dateString", "bad", "onError", failing)},
		{"FromStringFieldOnErrorFails", fromString("dateString", "2021-01-01", "onError", d("$add", bson.A{int32(1), "$s"}))},
		{"ToStringConstantOnNullFails", d("$dateToString", d("date", primitive.DateTime(0), "onNull", failing))},
		{"ToStringFieldOnNullFails", d("$dateToString", d("date", primitive.DateTime(0), "onNull", d("$add", bson.A{int32(1), "$s"})))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DateEdgesReview_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}
