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

// $replaceOne and $replaceAll share MongoDB's ExpressionReplaceBase.

func insertReplaceDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "s", Value: "blue fish, red fish"},
			{Key: "f", Value: "fish"},
			{Key: "r", Value: "cat"},
			{Key: "n", Value: nil},
			{Key: "i", Value: int32(1)},
		},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "fishfish"}, {Key: "f", Value: "fish"}, {Key: "r", Value: ""}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "no match"}, {Key: "f", Value: "fish"}, {Key: "r", Value: "cat"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "s", Value: nil}, {Key: "f", Value: "fish"}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "s", Value: int32(5)}, {Key: "f", Value: "fish"}, {Key: "r", Value: "cat"}},
	})
	return err
}

func replaceOp(op string, parts ...bson.E) bson.D {
	return bson.D{{Key: op, Value: bson.D(parts)}}
}

func TestReplaceOperators(t *testing.T) {
	for _, op := range []string{"$replaceOne", "$replaceAll"} {
		replace := func(input, find, replacement interface{}) bson.D {
			return replaceOp(op, part("input", input), part("find", find), part("replacement", replacement))
		}

		cases := []mathCase{
			{"Basic", replace("blue fish, red fish", "fish", "cat")},
			{"NoMatch", replace("blue fish", "bird", "cat")},
			{"WholeString", replace("fish", "fish", "cat")},
			{"AtStart", replace("fish fish", "fish", "cat")},
			{"AtEnd", replace("red fish", "fish", "cat")},
			{"Adjacent", replace("fishfishfish", "fish", "x")},
			{"Overlapping", replace("aaaa", "aa", "b")},
			{"OverlappingOdd", replace("aaa", "aa", "b")},
			{"ReplacementContainsFind", replace("ab", "a", "aa")},
			{"EmptyReplacement", replace("a-b-c", "-", "")},
			{"EmptyFind", replace("abc", "", "-")},
			{"EmptyFindEmptyInput", replace("", "", "-")},
			{"EmptyInput", replace("", "a", "-")},
			{"EmptyAll", replace("", "", "")},
			{"FindLongerThanInput", replace("ab", "abc", "x")},
			{"CaseSensitive", replace("Fish fish", "fish", "cat")},
			{"RegexCharsLiteral", replace("a.b.c", ".", "!")},
			{"DollarInReplacement", replace("a-b", "-", "$1")},
			{"BackslashInReplacement", replace("a-b", "-", `\0`)},
			{"Unicode", replace("café café", "é", "e")},
			{"UnicodeReplacement", replace("a-b-c", "-", "→")},
			{"UnicodeEmptyFind", replace("éa", "", "-")},
			{"Emoji", replace("x\U0001F600y\U0001F600", "\U0001F600", "!")},
			{"EmbeddedNull", replace("a\x00b\x00c", "\x00", "-")},
			{"Newlines", replace("a\nb\nc", "\n", " ")},

			{"Fields", replace("$s", "$f", "$r")},
			{"Expressions", replace(bson.D{{Key: "$concat", Value: bson.A{"fish", "-", "fish"}}}, bson.D{{Key: "$toLower", Value: "FISH"}}, "cat")},

			{"InputNull", replace(nil, "a", "b")},
			{"FindNull", replace("abc", nil, "b")},
			{"ReplacementNull", replace("abc", "a", nil)},
			{"InputMissing", replace("$missing", "a", "b")},
			{"FindMissing", replace("abc", "$missing", "b")},
			{"ReplacementMissing", replace("abc", "a", "$missing")},
			{"NullField", replace("$n", "a", "b")},
			{"AllNull", replace(nil, nil, nil)},

			{"InputNumber", replace(int32(1), "a", "b")},
			{"FindNumber", replace("abc", int32(1), "b")},
			{"ReplacementNumber", replace("abc", "a", int32(1))},
			{"InputArray", replace(bson.A{"a"}, "a", "b")},
			{"InputObject", replace(bson.D{{Key: "a", Value: "b"}}, "a", "b")},
			{"InputRegex", replace(primitive.Regex{Pattern: "a"}, "a", "b")},
			{"InputBool", replace(true, "a", "b")},
			{"InputNumberField", replace("$i", "a", "b")},
			{"InputNullFindNumber", replace(nil, int32(1), "b")},
			{"InputNumberFindNull", replace(int32(1), nil, "b")},
			{"FindNumberReplacementNumber", replace("abc", int32(1), int32(2))},
			{"FindNullReplacementNumber", replace("abc", nil, int32(2))},
			{"InputNumberReplacementNumber", replace(int32(1), "a", int32(2))},

			{"MissingInput", replaceOp(op, part("find", "a"), part("replacement", "b"))},
			{"MissingFind", replaceOp(op, part("input", "a"), part("replacement", "b"))},
			{"MissingReplacement", replaceOp(op, part("input", "a"), part("find", "a"))},
			{"MissingAll", replaceOp(op)},
			{"UnknownField", replaceOp(op, part("input", "a"), part("find", "a"), part("replacement", "b"), part("count", int32(1)))},
			{"UnknownFieldFirst", replaceOp(op, part("count", int32(1)), part("input", "a"))},
			{"NonObjectArgument", bson.D{{Key: op, Value: "abc"}}},
			{"ArrayArgument", bson.D{{Key: op, Value: bson.A{"abc", "a", "b"}}}},
			{"NullArgument", bson.D{{Key: op, Value: nil}}},
		}

		name := op[1:]
		for _, tc := range cases {
			t.Run(name+"_"+tc.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Replace_" + name + "_" + tc.name,
					Support: harness.DumboDBFull,
					Setup:   insertReplaceDocs,
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
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: replace("$s", "$f", "$r")}}}},
			}},
			{"AllDocsTypeError", bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: replace("$s", "$f", "$r")}}}},
			}},
			{"MatchExpr", bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{
					replace("$s", "fish", "cat"), "catfish",
				}}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			}},
			{"GroupKey", bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(4)}}}}}},
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: replace("$f", "f", "d")}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			}},
			{"AddFields", bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(3)}}}}}},
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "s", Value: replace("$s", "", "|")}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: 1}}}},
			}},
		}
		for _, p := range pipelines {
			t.Run(name+"_"+p.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Replace_" + name + "_" + p.name,
					Support: harness.DumboDBFull,
					Setup:   insertReplaceDocs,
					Run:     pipelineRunExtJSON(p.pipeline),
				})
			})
		}
	}
}
