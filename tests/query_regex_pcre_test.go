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
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Query filters compiled regexes separately from the expression operators,
// so PCRE2 behavior fixed for $regexMatch still differed in find
// (workspace-6u7). The Expr form runs each pattern through $regexMatch.

var queryRegexInputs = []string{
	"a b", "aab", "(aa", "a\n", "\nb", "AB", "a\nb", "foo bar", "é", "aaa", "ab", "",
	"123", "ab\r\nc", "x y", "_", "a b", "A a",
}

var queryRegexPatterns = []string{
	`\Q(\E(?<x>a)\k<x>`, `a+ +b`, `^$`, `(?x: a ) b`, `[\W]`, `[[:^alpha:]]`, `(?<=a)b`, `a(?=b)`,
	`\x{41}`, `(a)\1`, `(?i)A`, `a.b`, `^b`, `\d+`, `[\D]`, `\bfoo\b`, "é", `(?#c)a`, `a{2,}`,
	`a{,2}`, `\h`, `\R`, `a++b`, `(?>a+)b`, `\p{L}`, `A`, `(?<n>a)(?<n>b)`, `(`, `[a`, `a**`,
	`a\`, `x{2,1}`, `(?P<n>a)(?P=n)`, `a # comment`, `[ ]b`, `\Aa`, `b\z`, `b\Z`, `(?m)^b`, `.`,
	`(?s).`, `\N`, `[^\n]`, `\x{110000}`, `\cA`, `^(?=.*b)a`, `^(?:a)+`, `(^a)+`, `$(?=a*)`,
	`\b(?!x)a`, `^(?<=)a`, `(?=a)+a`,
}

func insertQueryRegexDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, len(queryRegexInputs))
	for i, s := range queryRegexInputs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: s}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func regexFindIDs(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cur, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return errOutcome(err), nil
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return errOutcome(err), nil
		}
		ids := make([]interface{}, len(docs))
		for i, d := range docs {
			ids[i] = d["_id"]
		}
		return ids, nil
	}
}

func TestQueryRegexPCRE(t *testing.T) {
	for pi, p := range queryRegexPatterns {
		for _, o := range []string{"", "i", "m", "s", "x", "imsx"} {
			forms := map[string]bson.D{
				"Regex":   {{Key: "s", Value: bson.D{{Key: "$regex", Value: p}, {Key: "$options", Value: o}}}},
				"Literal": {{Key: "s", Value: primitive.Regex{Pattern: p, Options: o}}},
				"In":      {{Key: "s", Value: bson.D{{Key: "$in", Value: bson.A{primitive.Regex{Pattern: p, Options: o}}}}}},
				"Expr": {{Key: "$expr", Value: bson.D{{Key: "$regexMatch", Value: bson.D{
					{Key: "input", Value: "$s"}, {Key: "regex", Value: p}, {Key: "options", Value: o},
				}}}}},
			}
			for form, filter := range forms {
				name := fmt.Sprintf("QueryRegexPCRE_%s_%02d_%s", form, pi, strings.ToUpper(o))
				f := filter
				t.Run(name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    name,
						Support: harness.DumboDBXFail,
						Setup:   insertQueryRegexDocs,
						Run:     regexFindIDs(f),
					})
				})
			}
		}
	}
}

// Patterns at mongod's limits.
func TestQueryRegexPCRELimits(t *testing.T) {
	cases := map[string]string{
		"MaxLength":  strings.Repeat("a", 32764),
		"PastLength": strings.Repeat("a", 32765),
		"Nesting250": strings.Repeat("(", 250) + "a" + strings.Repeat(")", 250),
		"Nesting251": strings.Repeat("(", 251) + "a" + strings.Repeat(")", 251),
	}
	for name, p := range cases {
		n := "QueryRegexPCRELimits_" + name
		f := bson.D{{Key: "s", Value: bson.D{{Key: "$regex", Value: p}}}}
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    n,
				Support: harness.DumboDBXFail,
				Setup:   insertQueryRegexDocs,
				Run:     regexFindIDs(f),
			})
		})
	}
}
