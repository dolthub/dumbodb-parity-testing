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

// PCRE2 constructs from the review of #184 (workspace-m8c.7), each checked
// with $regexFindAll so every match and capture is compared.

// regexReviewAlreadyMatching are the cases DumboDB matched before
// workspace-m8c.7 was fixed.
var regexReviewAlreadyMatching = map[string]bool{
	"ExtendedCommentParenNamedRef": true,
	"ClassParenNamedRef":           true,
	"ExtendedSpaceBeforeLazy":      true,
	"ExtendedGroupScoped":          true,
	"ExtendedInlineInGroup":        true,
	"ExtendedTurnedOff":            true,
	"ExtendedOption":               true,
	"MultilineCaretInternal":       true,
	"CaretNotMultiline":            true,
	"BackslashU":                   true,
	"QuantifiedDollar":             true,
}

func TestRegexTranslatorReview(t *testing.T) {
	findAll := func(input, regex string, options ...string) bson.D {
		spec := bson.D{{Key: "input", Value: input}, {Key: "regex", Value: regex}}
		if len(options) > 0 {
			spec = append(spec, bson.E{Key: "options", Value: options[0]})
		}
		return bson.D{{Key: "$regexFindAll", Value: spec}}
	}

	cases := []mathCase{
		// Group numbering ignores parentheses that are not groups.
		{"QuotedParenNamedRef", findAll("(aa", `\Q(\E(?<x>a)\k<x>`)},
		{"CommentParenNamedRef", findAll("aa", `(?#(x)(?<x>a)\k<x>`)},
		{"ExtendedCommentParenNamedRef", findAll("aa", "(?<x>a) # (\n\\k<x>", "x")},
		{"ClassParenNamedRef", findAll("(aa", `[(](?<x>a)\k<x>`)},

		// Extended mode.
		{"ExtendedSpaceBetweenQuantifiers", findAll("aab", "a+ +b", "x")},
		{"ExtendedSpaceBeforeLazy", findAll("aab", "a+ ?b", "x")},
		{"ExtendedGroupScoped", findAll("a bab", "(?x: a b ) a b")},
		{"ExtendedInlineInGroup", findAll("a bab", "((?x) a b ) a b")},
		{"ExtendedTurnedOff", findAll("a bab", "(?x) a b (?-x) a b")},
		{"ExtendedOption", findAll("ab", "a b", "x")},

		// Negated classes inside brackets are ASCII.
		{"ClassNotWord", findAll("caf\u00E9-x", `[\W]+`)},
		{"ClassNotDigit", findAll("1\u0663a", `[\D]+`)},
		{"ClassNotSpace", findAll("a\u00A0b c", `[\S]+`)},
		{"ClassNotWordAndDigit", findAll("\u00E9-1", `[\W\d]+`)},
		{"ClassNotHorizontal", findAll("a\tb", `[\H]+`)},
		{"ClassNotVertical", findAll("a\nb", `[\V]+`)},
		{"ClassNegatedPOSIX", findAll("ab12", `[[:^alpha:]]+`)},
		{"ClassNegatedPOSIXDigit", findAll("ab12", `[[:^digit:]]+`)},

		// Multiline ^ does not match after a trailing newline.
		{"MultilineCaretAfterTrailingNewline", findAll("a\n", "^", "m")},
		{"MultilineCaretDollarEmpty", findAll("a\n", "^$", "m")},
		{"MultilineCaretInternal", findAll("a\nb\n", "^.", "m")},
		{"CaretNotMultiline", findAll("a\n", "^")},

		// Escapes PCRE2 accepts.
		{"EscapedUnderscore", findAll("a_b", `\_`)},
		{"HexNoDigits", findAll("a\x00b", `\x`)},
		{"HexOneDigit", findAll("a\x06b", `\x6`)},
		{"ClassEscapedEight", findAll("a8", `[\8]`)},
		{"ClassEscapedG", findAll("ag", `[\g]`)},
		{"OctalBraces", findAll("xa", `\o{141}`)},
		{"NamedCodePoint", findAll("xa", `\N{U+0061}`)},
		{"ResetFlagsCaret", findAll("Aa", "(?^)a", "i")},
		{"ResetFlagsCaretX", findAll("ab", "(?^x) a b")},

		// Patterns PCRE2 rejects.
		{"BackslashU", findAll("A", `A`)},
		{"DuplicateNames", findAll("aa", `(?<n>a)(?<n>a)`)},
		{"NameStartsWithDigit", findAll("a", `(?<1a>a)`)},
		{"POSIXCollating", findAll("a", `[[.a.]]`)},
		{"QuantifiedWordBoundary", findAll("a", `\B{0,2}`)},
		{"QuantifiedCaret", findAll("a", `^*`)},
		{"QuantifiedDollar", findAll("a", `$+`)},

		// Constructs regexp2 cannot express.
		{"MatchReset", findAll("ab", `a\Kb`)},
		{"Recursion", findAll("(())", `\((?R)?\)`)},
		{"BranchReset", findAll("ab", `(?|(a)|(b))`)},
		{"Ungreedy", findAll("aaa", `(?U)a+`)},
		{"CatastrophicNoMatch", bson.D{{Key: "$regexMatch", Value: bson.D{{Key: "input", Value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, {Key: "regex", Value: "^(a*)*b"}}}}},
		{"CaseFoldLongS", findAll("\u017F", "s", "i")},
		{"CaseFoldFinalSigma", findAll("\u03C2", "\u03C3", "i")},
		{"CaseFoldMicro", findAll("\u00B5", "\u03BC", "i")},
		{"CaseFoldDottedI", findAll("\u0130", "i", "i")},
	}
	for _, tc := range cases {
		support := harness.DumboDBXFail
		if regexReviewAlreadyMatching[tc.name] {
			support = harness.DumboDBFull
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "RegexReview_" + tc.name,
				Support: support,
				Setup:   insertRegexDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}
