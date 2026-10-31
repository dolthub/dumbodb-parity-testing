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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod resolves time zone names with timelib's built-in database (version
// 2024.1 in 8.0.28), not the host's tzdata: it accepts exactly the names in
// that database and applies its rules. (review of #184: workspace-m8c.6)

// timeZoneDatabasePending diverge for reasons outside the database.
var timeZoneDatabasePending = map[string]bool{}

func TestTimeZoneDatabase(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		out := bson.D{}
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	at := func(y int, m time.Month, day, h int) time.Time { return time.Date(y, m, day, h, 0, 0, 0, time.UTC) }
	offsetAt := func(date time.Time, tz string) bson.D {
		return d("$dateToString", d("date", date, "format", "%z", "timezone", tz))
	}

	cases := []mathCase{
		// Names outside timelib's database.
		{"Posixrules", offsetAt(at(2021, 1, 1, 0), "posixrules")},
		{"Localtime", offsetAt(at(2021, 1, 1, 0), "localtime")},
		{"RightUTC", offsetAt(at(2021, 1, 1, 0), "right/UTC")},
		{"PosixLondon", offsetAt(at(2021, 1, 1, 0), "posix/Europe/London")},
		{"DoubleSlash", offsetAt(at(2021, 1, 1, 0), "Europe//London")},
		{"Lowercase", offsetAt(at(2021, 1, 1, 0), "europe/london")},
		{"Uppercase", offsetAt(at(2021, 1, 1, 0), "EUROPE/LONDON")},
		{"TrailingSlash", offsetAt(at(2021, 1, 1, 0), "Europe/London/")},
		{"Local", offsetAt(at(2021, 1, 1, 0), "Local")},
		{"Factory", offsetAt(at(2021, 1, 1, 0), "Factory")},
		{"ZoneTab", offsetAt(at(2021, 1, 1, 0), "zone.tab")},

		// Names in it.
		{"UTC", offsetAt(at(2021, 1, 1, 0), "UTC")},
		{"GMT", offsetAt(at(2021, 7, 1, 0), "GMT")},
		{"EtcGMTPlus5", offsetAt(at(2021, 7, 1, 0), "Etc/GMT+5")},
		{"EST5EDT", offsetAt(at(2021, 7, 1, 0), "EST5EDT")},
		{"USEastern", offsetAt(at(2021, 7, 1, 0), "US/Eastern")},
		{"Kiev", offsetAt(at(2021, 7, 1, 0), "Europe/Kiev")},
		{"Kyiv", offsetAt(at(2021, 7, 1, 0), "Europe/Kyiv")},

		// Rules that changed after tzdata 2024a.
		{"AsuncionMarch2025", offsetAt(at(2025, 3, 31, 12), "America/Asuncion")},
		{"AsuncionJuly2025", offsetAt(at(2025, 7, 1, 12), "America/Asuncion")},
		{"AsuncionJan2026", offsetAt(at(2026, 1, 15, 12), "America/Asuncion")},

		// Far-future dates follow the database's rules past its transitions.
		{"LondonSummer2050", offsetAt(at(2050, 7, 1, 0), "Europe/London")},
		{"NewYorkWinter2100", offsetAt(at(2100, 1, 1, 0), "America/New_York")},
		{"SydneySummer2200", offsetAt(at(2200, 1, 1, 0), "Australia/Sydney")},
		{"LondonPre1900", offsetAt(at(1800, 7, 1, 0), "Europe/London")},
		{"DSTEdgeNewYork", offsetAt(time.Date(2021, 3, 14, 6, 59, 59, 0, time.UTC), "America/New_York")},
		{"DSTEdgeNewYorkAfter", offsetAt(time.Date(2021, 3, 14, 7, 0, 0, 0, time.UTC), "America/New_York")},

		// Other operators taking a timezone.
		{"YearInvalidZone", d("$year", d("date", at(2021, 1, 1, 0), "timezone", "posixrules"))},
		{"HourAsuncion", d("$hour", d("date", at(2025, 3, 31, 12), "timezone", "America/Asuncion"))},
		{"DateFromPartsAsuncion", d("$dateToString", d("date", d("$dateFromParts", d("year", int32(2025), "month", int32(3), "day", int32(31), "hour", int32(12), "timezone", "America/Asuncion")), "format", "%H"))},
		{"DateFromStringZoneName", d("$dateToString", d("date", d("$dateFromString", d("dateString", "2025-03-31T12:00:00", "timezone", "America/Asuncion")), "format", "%H"))},
		{"DateFromStringInvalidZone", d("$dateFromString", d("dateString", "2021-01-01", "timezone", "localtime"))},
		{"DateTruncAsuncion", d("$dateToString", d("date", d("$dateTrunc", d("date", at(2025, 3, 31, 12), "unit", "day", "timezone", "America/Asuncion")), "format", "%H"))},
	}
	for _, tc := range cases {
		support := harness.DumboDBFull
		if timeZoneDatabasePending[tc.name] {
			support = harness.DumboDBXFail
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "TimeZoneDB_" + tc.name,
				Support: support,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}
