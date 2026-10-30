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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Limits mongod applies to expressions that a client can make arbitrarily
// expensive (security review of #184: workspace-8tj).

func TestResourceLimits(t *testing.T) {
	op := func(name string, v interface{}) bson.D { return bson.D{{Key: name, Value: v}} }
	regexMatch := func(input, regex string) bson.D {
		return op("$regexMatch", bson.D{{Key: "input", Value: input}, {Key: "regex", Value: regex}})
	}
	countAll := func(input, regex string) bson.D {
		return op("$size", op("$regexFindAll", bson.D{{Key: "input", Value: input}, {Key: "regex", Value: regex}}))
	}
	replaceAll := func(input, find, replacement string) bson.D {
		return op("$strLenBytes", op("$replaceAll", bson.D{{Key: "input", Value: input}, {Key: "find", Value: find}, {Key: "replacement", Value: replacement}}))
	}
	fromString := func(s string) bson.D {
		return op("$dateFromString", bson.D{{Key: "dateString", Value: s}})
	}
	nested := func(depth int) string { return strings.Repeat("(", depth) + "a" + strings.Repeat(")", depth) }

	cases := []mathCase{
		{"RegexNesting249", regexMatch("a", nested(249))},
		{"RegexNesting250", regexMatch("a", nested(250))},
		{"RegexNesting251", regexMatch("a", nested(251))},
		{"RegexNesting2000", regexMatch("a", nested(2000))},
		{"RegexNonCapturingNesting300", regexMatch("a", strings.Repeat("(?:", 300)+"a"+strings.Repeat(")", 300))},
		{"RegexLongFlat20k", regexMatch("a", strings.Repeat("(?:a)", 20000))},
		{"RegexLongLiteral100k", regexMatch("a", strings.Repeat("a", 100000))},
		{"RegexManyBraces", regexMatch("a", strings.Repeat("{", 20000))},
		{"RegexManyPossessive", regexMatch("a", strings.Repeat("a++", 5000))},
		{"RegexCodePointTooLarge", regexMatch("a", `\x{110000}`)},
		{"RegexSurrogate", regexMatch("a", `\x{D800}`)},
		{"RegexMaxCodePoint", regexMatch("a", `\x{10FFFF}`)},
		{"RegexPatternLimit", regexMatch("a", strings.Repeat("a", 32764))},
		{"RegexPatternOverLimit", regexMatch("a", strings.Repeat("a", 32765))},
		{"RegexInvalidUTF8Pattern", regexMatch("a\xff", "a\xfe")},
		{"RegexInvalidUTF8Input", regexMatch("a\xff", "a")},
		{"RegexInvalidUTF8InputNoMatch", regexMatch("\xff", "b")},
		{"FindAllInvalidUTF8Input", countAll("ab\xffab", "ab")},

		{"FindAllOutputUnder64MB", countAll(strings.Repeat("a", 100000), ".")},
		{"FindAllOutputOver64MB", countAll(strings.Repeat("a", 1000000), ".")},
		{"FindAllLargeMatches", countAll(strings.Repeat("ab", 2000000), "(a)(b)")},

		{"ReplaceAllUnder16MB", replaceAll(strings.Repeat("a", 1000), "", strings.Repeat("b", 1000))},
		{"ReplaceAllOver16MB", replaceAll(strings.Repeat("a", 4096), "", strings.Repeat("b", 4096))},
		{"ReplaceAllManyMatches", replaceAll(strings.Repeat("a", 100000), "a", strings.Repeat("b", 200))},
		{"ReplaceAllOver512MB", replaceAll(strings.Repeat("a", 100000), "", strings.Repeat("b", 6000))},
		{"ReplaceOneLarge", op("$strLenBytes", op("$replaceOne", bson.D{{Key: "input", Value: strings.Repeat("a", 4096)}, {Key: "find", Value: ""}, {Key: "replacement", Value: strings.Repeat("b", 4096)}}))},

		{"DateFromStringDashes", fromString(strings.Repeat("-", 20000))},
		{"DateFromStringPluses", fromString(strings.Repeat("+", 20000))},
		// A valid string over 4096 bytes is a deliberate deviation; see
		// date_string_length_test.go.
		{"DateFromStringLongDigits", fromString(strings.Repeat("1", 20000))},
		{"DateFromStringLongWords", fromString(strings.Repeat("monday ", 3000))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ResourceLimits_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}
