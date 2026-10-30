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

// mongod orders regex values by their raw bytes, pattern then options, and
// never compiles them. DumboDB compiled both sides and panicked on a pattern
// that does not compile (workspace-8tj.8).

func re(pattern, options string) primitive.Regex {
	return primitive.Regex{Pattern: pattern, Options: options}
}

func insertRegexCompareDocs(ctx context.Context, col *mongo.Collection) error {
	docs := []interface{}{}
	for i, r := range []primitive.Regex{
		re("(", ""), re("[", "i"), re("a", ""), re("a", "i"), re("a", "m"), re("ab", ""),
		re("a(", "x"), re("A", ""), re("", ""), re("*", "s"), re("a", "im"),
	} {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "r", Value: r}})
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func TestRegexCompare(t *testing.T) {
	lit := func(r primitive.Regex) bson.D { return bson.D{{Key: "$literal", Value: r}} }
	cases := []mathCase{
		{"EqInvalidSame", bson.D{{Key: "$eq", Value: bson.A{lit(re("(", "")), lit(re("(", ""))}}}},
		{"EqInvalidOptions", bson.D{{Key: "$eq", Value: bson.A{lit(re("(", "")), lit(re("(", "i"))}}}},
		{"CmpInvalid", bson.D{{Key: "$cmp", Value: bson.A{lit(re("(", "")), lit(re("[", ""))}}}},
		{"CmpPrefix", bson.D{{Key: "$cmp", Value: bson.A{lit(re("a", "z")), lit(re("ab", ""))}}}},
		{"CmpOptions", bson.D{{Key: "$cmp", Value: bson.A{lit(re("a", "i")), lit(re("a", "m"))}}}},
		{"CmpOptionsOrder", bson.D{{Key: "$cmp", Value: bson.A{lit(re("a", "im")), lit(re("a", "mi"))}}}},
		{"CmpCase", bson.D{{Key: "$cmp", Value: bson.A{lit(re("A", "")), lit(re("a", ""))}}}},
		{"LtInvalidVsString", bson.D{{Key: "$lt", Value: bson.A{lit(re("(", "")), "("}}}},
		{"SortArrayInvalid", bson.D{{Key: "$sortArray", Value: bson.D{
			{Key: "input", Value: bson.A{lit(re("[", "")), lit(re("(", "")), lit(re("a", ""))}},
			{Key: "sortBy", Value: 1},
		}}}},
		{"MaxInvalid", bson.D{{Key: "$max", Value: bson.A{lit(re("(", "")), lit(re("[", ""))}}}},
		{"InInvalid", bson.D{{Key: "$in", Value: bson.A{lit(re("(", "")), bson.A{lit(re("(", ""))}}}}},
		{"SetUnionInvalid", bson.D{{Key: "$setUnion", Value: bson.A{bson.A{lit(re("(", ""))}, bson.A{lit(re("(", ""))}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "RegexCompare_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   insertRegexCompareDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"SortStage", bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "r", Value: 1}, {Key: "_id", Value: 1}}}}}},
		{"SortStageDescending", bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "r", Value: -1}, {Key: "_id", Value: 1}}}}}},
		{"GroupMinMax", bson.A{bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "min", Value: bson.D{{Key: "$min", Value: "$r"}}},
			{Key: "max", Value: bson.D{{Key: "$max", Value: "$r"}}},
		}}}}},
		{"GroupByRegex", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$r"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
		{"SortArrayField", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "all", Value: bson.D{{Key: "$push", Value: "$r"}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$sortArray", Value: bson.D{{Key: "input", Value: "$all"}, {Key: "sortBy", Value: -1}}}}}}}},
		}},
		{"MatchExprEq", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$r", lit(re("(", ""))}}}}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "RegexCompare_" + p.name,
				Support: harness.DumboDBXFail,
				Setup:   insertRegexCompareDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
