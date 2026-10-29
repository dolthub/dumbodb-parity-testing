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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $regexFind, $regexFindAll and $regexMatch share MongoDB's ExpressionRegex,
// which compiles with PCRE2 in UTF mode.

func insertRegexDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "s", Value: "blue fish, red fish"},
			{Key: "re", Value: "(\\w+) fish"},
			{Key: "o", Value: "i"},
			{Key: "n", Value: nil},
			{Key: "i", Value: int32(1)},
		},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "One Fish"}, {Key: "re", Value: "fish"}, {Key: "o", Value: "i"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "café été"}, {Key: "re", Value: "é"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "s", Value: nil}, {Key: "re", Value: "x"}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "re", Value: primitive.Regex{Pattern: "F", Options: "i"}}, {Key: "s", Value: "ff"}},
	})
	return err
}

func TestRegexOperators(t *testing.T) {
	for _, op := range []string{"$regexFindAll", "$regexFind", "$regexMatch"} {
		rx := func(input, regex interface{}, options ...interface{}) bson.D {
			spec := bson.D{part("input", input), part("regex", regex)}
			if len(options) > 0 {
				spec = append(spec, part("options", options[0]))
			}
			return bson.D{{Key: op, Value: spec}}
		}

		cases := []mathCase{
			{"Basic", rx("blue fish, red fish", "fish")},
			{"NoMatch", rx("blue fish", "bird")},
			{"Captures", rx("blue fish, red fish", "(\\w+) (fish)")},
			{"UnmatchedCapture", rx("b a", "(a)|(b)")},
			{"NestedCaptures", rx("abcabc", "((a)(b))c")},
			{"NamedCapture", rx("x=1, y=2", "(?<k>\\w)=(?<v>\\d)")},
			{"NonCapturing", rx("abab", "(?:a)(b)")},
			{"EmptyPattern", rx("abc", "")},
			{"EmptyPatternEmptyInput", rx("", "")},
			{"EmptyInput", rx("", "a")},
			{"StarPattern", rx("baaac", "a*")},
			{"Overlapping", rx("aaaa", "aa")},
			{"Lazy", rx("aaa", "a+?")},
			{"Alternation", rx("abab", "a|ab")},

			{"UnicodeIdx", rx("café café", "é")},
			{"EmojiIdx", rx("\U0001F600x\U0001F600x", "x")},
			{"EmptyPatternUnicode", rx("é\U0001F600", "")},
			{"DotUnicode", rx("é\U0001F600", ".")},
			{"CaptureAfterUnicode", rx("ééab", "(a)(b)")},

			{"AnchorStart", rx("a\na", "^a")},
			{"AnchorStartMultiline", rx("a\na", "^a", "m")},
			{"AnchorEnd", rx("a\na", "a$")},
			{"AnchorEndBeforeNewline", rx("a\n", "a$")},
			{"AnchorEndMultiline", rx("a\na\n", "a$", "m")},
			{"AbsoluteEnd", rx("a\n", "a\\z")},
			{"AbsoluteEndZ", rx("a\n", "a\\Z")},
			{"StartOfMatch", rx("aab", "\\Ga")},
			{"DotNewline", rx("a\nb", "a.b")},
			{"DotNewlineDotall", rx("a\nb", "a.b", "s")},
			{"CaseInsensitive", rx("Fish fISH", "fish", "i")},
			{"CaseInsensitiveUnicode", rx("Été", "é", "i")},
			{"Extended", rx("ab", "a b # comment", "x")},
			{"ExtendedEscapedSpace", rx("a b", "a\\ b", "x")},
			{"OptionU", rx("ab", "b", "u")},
			{"AllOptions", rx("A\nB", "a.b", "imsux")},
			{"InlineOption", rx("FISH", "(?i)fish")},

			{"Lookahead", rx("fish fishy", "fish(?=y)")},
			{"NegativeLookahead", rx("fish fishy", "fish(?!y)")},
			{"Lookbehind", rx("red fish blue fish", "(?<=blue )fish")},
			{"LookbehindAcrossMatches", rx("abab", "(?<=a)b")},
			{"Backreference", rx("aa ab bb", "(\\w)\\1")},
			{"AtomicGroup", rx("aaab", "(?>a+)b")},
			{"Possessive", rx("aaab", "a++b")},
			{"WordBoundary", rx("fish fishy catfish", "\\bfish\\b")},
			{"WordBoundaryAfterMatch", rx("ab b", "\\bb")},
			{"DigitASCII", rx("12٣٤", "\\d+")},
			{"WordASCII", rx("café", "\\w+")},
			{"SpaceVerticalTab", rx("a\vb", "\\s")},
			{"POSIXClass", rx("a1b2", "[[:digit:]]")},
			{"UnicodeProperty", rx("aéB", "\\p{Lu}")},
			{"HexEscape", rx("A", "\\x41")},
			{"QuantifierRange", rx("aaaa", "a{2,3}")},

			{"RegexObject", rx("Fish fish", primitive.Regex{Pattern: "fish", Options: "i"})},
			{"RegexObjectNoFlags", rx("Fish fish", primitive.Regex{Pattern: "fish"}, "i")},
			{"RegexObjectFlagsAndOptions", rx("Fish", primitive.Regex{Pattern: "fish", Options: "i"}, "i")},
			{"RegexObjectFlagsNullOptions", rx("Fish", primitive.Regex{Pattern: "fish", Options: "i"}, nil)},
			{"RegexObjectMultiline", rx("a\na", primitive.Regex{Pattern: "^a", Options: "m"})},

			{"Fields", rx("$s", "$re", "$o")},
			{"RegexFromField", rx("$s", "$re")},
			{"InputExpression", rx(bson.D{{Key: "$concat", Value: bson.A{"a", "b"}}}, "b")},

			{"InputNull", rx(nil, "a")},
			{"InputMissing", rx("$missing", "a")},
			{"InputNullField", rx("$n", "a")},
			{"RegexNull", rx("abc", nil)},
			{"RegexMissing", rx("abc", "$missing")},
			{"OptionsNull", rx("abc", "b", nil)},
			{"OptionsMissing", rx("abc", "b", "$missing")},
			{"OptionsEmpty", rx("abc", "b", "")},

			{"InputNumber", rx(int32(1), "a")},
			{"InputNumberField", rx("$i", "a")},
			{"InputArray", rx(bson.A{"a"}, "a")},
			{"RegexNumber", rx("abc", int32(1))},
			{"OptionsNumber", rx("abc", "a", int32(1))},
			{"InputNullRegexNumber", rx(nil, int32(1))},
			{"InputNumberRegexNumber", rx(int32(1), int32(1))},
			{"InputNumberRegexNull", rx(int32(1), nil)},
			{"InputNumberBadRegex", rx(int32(1), "(")},
			{"InputNumberRegexField", rx(int32(1), "$re")},
			{"RegexNullBadOptions", rx("abc", nil, int32(1))},
			{"RegexNullInvalidFlag", rx("abc", nil, "g")},
			{"InvalidFlag", rx("abc", "a", "g")},
			{"InvalidFlagField", rx("$s", "$re", "q")},
			{"EmbeddedNullPattern", rx("abc", "a\x00")},
			{"EmbeddedNullOptions", rx("abc", "a", "i\x00")},
			{"EmbeddedNullInput", rx("a\x00b", "b")},
			{"InvalidRegexParen", rx("abc", "(")},
			{"InvalidRegexBracket", rx("abc", "[a")},
			{"InvalidRegexRepeat", rx("abc", "*a")},
			{"InvalidRegexRange", rx("abc", "a{2,1}")},
			{"InvalidRegexField", rx("abc", bson.D{{Key: "$concat", Value: bson.A{"(", ""}}})},

			{"NoInput", bson.D{{Key: op, Value: bson.D{part("regex", "a")}}}},
			{"NoRegex", bson.D{{Key: op, Value: bson.D{part("input", "a")}}}},
			{"UnknownField", bson.D{{Key: op, Value: bson.D{part("input", "a"), part("regex", "a"), part("flags", "i")}}}},
			{"NonObjectArgument", bson.D{{Key: op, Value: "abc"}}},
			{"ArrayArgument", bson.D{{Key: op, Value: bson.A{bson.D{part("input", "a"), part("regex", "a")}}}}},
		}

		name := op[1:]
		for _, tc := range cases {
			t.Run(name+"_"+tc.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Regex_" + name + "_" + tc.name,
					Support: harness.DumboDBFull,
					Setup:   insertRegexDocs,
					Run:     projectTyped(tc.expr),
				})
			})
		}

		pipelines := []struct {
			name     string
			pipeline bson.A
		}{
			{"AllDocs", bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: rx("$s", "$re")}}}},
			}},
			{"AllDocsConstantRegex", bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: rx("$s", "f", "i")}}}},
			}},
			{"MatchExpr", bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: rx("$s", "fish", "i")}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}},
			{"UnwindMatches", bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(0)}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "m", Value: rx("$s", "(\\w+) fish")}}}},
				bson.D{{Key: "$unwind", Value: "$m"}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "m", Value: 1}}}},
			}},
		}
		for _, p := range pipelines {
			t.Run(name+"_"+p.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Regex_" + name + "_" + p.name,
					Support: harness.DumboDBFull,
					Setup:   insertRegexDocs,
					Run:     pipelineRunExtJSON(p.pipeline),
				})
			})
		}
	}
}
