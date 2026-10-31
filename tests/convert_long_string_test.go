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

// Parsing a long numeric string took quadratic CPU in $toInt/$toLong
// (workspace-8tj.16) and $toDecimal (workspace-8tj.17). Long decimal and
// double strings also failed where mongod parses them (workspace-fvo), and
// decimal strings rounded half-even where mongod truncates (workspace-76c).
func TestConvertLongNumericStrings(t *testing.T) {
	const n = 200_000
	zeros := strings.Repeat("0", n)
	sevens := strings.Repeat("7", n)
	inputs := []struct {
		name  string
		value string
	}{
		{"Sevens", sevens},
		{"NegativeSevens", "-" + sevens},
		{"LeadingZerosOne", zeros + "1"},
		{"LeadingZerosMaxLong", "+" + zeros + "9223372036854775807"},
		{"LeadingZerosMinLong", "-" + zeros + "9223372036854775808"},
		{"LeadingZerosPastLong", zeros + "9223372036854775808"},
		{"LeadingZerosMaxInt", zeros + "2147483647"},
		{"LeadingZerosPastInt", zeros + "2147483648"},
		{"Zeros", zeros},
		{"SevensThenLetter", sevens + "x"},
		{"LetterThenSevens", "x" + sevens},
		{"SevensFraction", sevens + ".5"},
		{"FractionSevens", "0." + sevens},
		{"ZerosFractionSevens", zeros + "." + sevens},
		{"SevensExponent", sevens + "e-200000"},
		{"OneExponentZeros", "1e" + zeros + "5"},
		{"SevensBigExponent", sevens + "e7000"},
		{"ZerosThenSevensFraction", "0." + zeros + sevens},
		{"Spaces", " " + sevens},
		{"Digits35", "0.77777777777777777777777777777777777"},
		{"NegativeDigits35", "-0.77777777777777777777777777777777777"},
		{"Digits35Tie", "1.0000000000000000000000000000000005"},
		{"Digits35Odd", "1.0000000000000000000000000000000015"},
		{"Nines35", "99999999999999999999999999999999999"},
		{"NinesPastMaxDecimal", "9.99999999999999999999999999999999999e6144"},
		{"SubnormalDigits", "1.23456789e-6170"},
		{"BelowSubnormal", "1e-6177"},
	}
	for _, in := range inputs {
		for _, to := range []string{"int", "long", "decimal", "double"} {
			name := "ConvertLongString_" + to + "_" + in.name
			expr := bson.D{{Key: "$convert", Value: bson.D{{Key: "input", Value: in.value}, {Key: "to", Value: to}}}}
			t.Run(to+"_"+in.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name,
					Support: harness.DumboDBFull,
					Setup:   insertOneDoc,
					Run:     projectTyped(expr),
				})
			})
		}
	}
}
