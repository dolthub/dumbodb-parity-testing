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

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Inline x mode ends with its group; the space after the group still
// matches literally (workspace-qc4).
func TestRegexInlineExtendedScope(t *testing.T) {
	patterns := []string{
		"(?x: a ) b", "(?x:a) b", "(?:(?x) a ) b", "((?x) a ) b", "(?x) a (?-x) b",
		"(?x: a (?-x: b ) c ) d", "(?i:A) a", "(?x:(?-x) a ) b",
	}
	inputs := []string{"a b", "aab", "ab", "a  b", "a b c d", "A a"}
	for _, p := range patterns {
		for _, in := range inputs {
			name := "RegexInlineExtended_" + p + "_" + in
			expr := bson.D{{Key: "$regexMatch", Value: bson.D{{Key: "input", Value: in}, {Key: "regex", Value: p}}}}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name,
					Support: harness.DumboDBXFail,
					Setup:   insertOneDoc,
					Run:     projectTyped(expr),
				})
			})
		}
	}
}
