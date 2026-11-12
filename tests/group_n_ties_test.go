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
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod keeps the n best values of $topN, $bottomN, $minN and $maxN in an
// ordered set: a value that only ties the worst kept one is refused, and a
// better one evicts the latest of the tied worst values ($topN, $minN) or the
// earliest ($bottomN, $maxN). $maxN lists ties in reverse arrival order.
// Results carry $type so int, long, double and decimal ties are told apart.
// $documents runs in mongod's classic engine and a $group on a collection in
// SBE, which report an invalid n under different codes.

func nTiesRun(docs bson.A, acc bson.D) func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Database().Aggregate(ctx, mongo.Pipeline{
			{{Key: "$documents", Value: docs}},
			{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: acc}}}},
			{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}, {Key: "t", Value: bson.D{{Key: "$map", Value: bson.D{
				{Key: "input", Value: bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$isArray", Value: "$r"}}, "$r", bson.A{"$r"}}}}},
				{Key: "in", Value: bson.D{{Key: "$type", Value: "$$this"}}},
			}}}}}}},
		})
		if err != nil {
			return nil, err
		}
		var out []bson.D
		err = cursor.All(ctx, &out)
		return out, err
	}
}

func nTiesDocs(field string, values ...interface{}) bson.A {
	docs := bson.A{}
	for i, v := range values {
		docs = append(docs, bson.D{{Key: "i", Value: int32(i + 1)}, {Key: field, Value: v}})
	}
	return docs
}

func TestGroupNTies(t *testing.T) {
	s := func(vals ...int32) bson.A {
		out := make([]interface{}, len(vals))
		for i, v := range vals {
			out[i] = v
		}
		return nTiesDocs("s", out...)
	}
	mixed := nTiesDocs("v", int32(2), int64(2), 2.0, dz("2"), int32(1), int32(3))
	allTwos := nTiesDocs("v", int32(2), int64(2), 2.0, dz("2"))
	top := func(op string, n int32, dir int32) bson.D {
		return bson.D{{Key: op, Value: bson.D{{Key: "output", Value: "$i"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: dir}}}, {Key: "n", Value: n}}}}
	}
	minMax := func(op string, n int32) bson.D {
		return bson.D{{Key: op, Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: n}}}}
	}
	cases := []struct {
		name string
		docs bson.A
		acc  bson.D
	}{
		{"TopN2", s(2, 2, 2, 1, 3), top("$topN", 2, 1)},
		{"TopN3", s(2, 2, 2, 1, 3), top("$topN", 3, 1)},
		{"TopNCutoffTie", s(2, 2, 1, 3), top("$topN", 2, 1)},
		{"BottomN2", s(2, 2, 2, 3, 1), top("$bottomN", 2, 1)},
		{"BottomN3", s(2, 2, 2, 3, 1), top("$bottomN", 3, 1)},
		{"BottomNDescending", s(2, 2, 2, 1, 3), top("$bottomN", 2, -1)},
		{"BottomNAllEqual", s(1, 1, 1, 1), top("$bottomN", 3, 1)},
		{"BottomAllEqual", s(1, 1, 1, 1), bson.D{{Key: "$bottom", Value: bson.D{{Key: "output", Value: "$i"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}}}}},
		{"MinN2", mixed, minMax("$minN", 2)},
		{"MinN4", mixed, minMax("$minN", 4)},
		{"MinNCutoffTie", nTiesDocs("v", int32(2), int64(2), int32(1)), minMax("$minN", 2)},
		{"MaxN2", mixed, minMax("$maxN", 2)},
		{"MaxN3", mixed, minMax("$maxN", 3)},
		{"MaxN5", mixed, minMax("$maxN", 5)},
		{"MaxNAllEqual", allTwos, minMax("$maxN", 3)},
		{"MinNAllEqual", allTwos, minMax("$minN", 3)},
		{"ClassicNZero", mixed, minMax("$minN", 0)},
		{"ClassicNField", mixed, bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: "$v"}}}}},
		{"ClassicNFraction", mixed, bson.D{{Key: "$lastN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: 1.5}}}}},
		{"ClassicNString", mixed, bson.D{{Key: "$topN", Value: bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "v", Value: 1}}}, {Key: "n", Value: "x"}}}}},
		{"ClassicNRoot", mixed, bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: "$$ROOT"}}}}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "GroupNTies_" + c.name,
			Support: harness.DumboDBFull,
			Run:     nTiesRun(c.docs, c.acc),
		})
	}
}

func groupNArraysSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(1), int32(10)}}, {Key: "s", Value: int32(1)}, {Key: "z", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{int32(5)}}, {Key: "s", Value: int32(2)}, {Key: "z", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: int32(3)}, {Key: "s", Value: int32(3)}, {Key: "z", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{}}, {Key: "s", Value: int32(4)}, {Key: "z", Value: int32(0)}},
	})
	return err
}

// Array sort keys, output errors from documents that are not kept, $firstN
// stopping once it holds n values, and SBE's codes for an invalid n.
func TestGroupNCollection(t *testing.T) {
	topBy := func(op string, dir int32, extra ...bson.E) bson.D {
		return bson.D{{Key: op, Value: append(bson.D{{Key: "output", Value: "$_id"}, {Key: "sortBy", Value: bson.D{{Key: "a", Value: dir}}}}, extra...)}}
	}
	n2 := bson.E{Key: "n", Value: int32(2)}
	inv := bson.D{{Key: "$divide", Value: bson.A{int32(1), "$z"}}}
	firstN := func(n interface{}) bson.D {
		return bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$_id"}, {Key: "n", Value: n}}}}
	}
	group := func(acc bson.D) bson.D {
		return bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: acc}}}}
	}
	window := func(acc bson.D) bson.D {
		return bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "output", Value: bson.D{{Key: "r", Value: acc}}}}}}
	}
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"Bottom", group(topBy("$bottom", 1))},
		{"BottomN", group(topBy("$bottomN", 1, n2))},
		{"BottomNDescending", group(topBy("$bottomN", -1, n2))},
		{"TopNDescending", group(topBy("$topN", -1, n2))},
		{"WindowBottom", window(topBy("$bottom", 1))},
		{"WindowBottomN", window(topBy("$bottomN", 1, n2))},
		{"WindowTopDescending", window(topBy("$top", -1))},
		{"TopNOutputError", group(bson.D{{Key: "$topN", Value: bson.D{{Key: "output", Value: inv}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, n2}}})},
		{"TopOutputError", group(bson.D{{Key: "$top", Value: bson.D{{Key: "output", Value: inv}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}}}})},
		{"BottomNOutputError", group(bson.D{{Key: "$bottomN", Value: bson.D{{Key: "output", Value: inv}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: -1}}}, n2}}})},
		{"FirstNStopsEvaluating", group(bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: inv}, {Key: "n", Value: int32(1)}}}})},
		{"LastNEvaluatesAll", group(bson.D{{Key: "$lastN", Value: bson.D{{Key: "input", Value: inv}, {Key: "n", Value: int32(1)}}}})},
		{"SBENZero", group(firstN(int32(0)))},
		{"SBENRoot", group(firstN("$$ROOT"))},
		{"SBENNow", group(firstN("$$NOW"))},
		{"SBENField", group(firstN("$s"))},
		{"WindowNMissing", window(firstN("$v"))},
		{"WindowNZero", window(firstN(int32(0)))},
		{"WindowNNegative", window(bson.D{{Key: "$minN", Value: bson.D{{Key: "input", Value: "$_id"}, {Key: "n", Value: int32(-1)}}}})},
		{"WindowNFraction", window(bson.D{{Key: "$lastN", Value: bson.D{{Key: "input", Value: "$_id"}, {Key: "n", Value: 1.5}}}})},
		{"WindowNNull", window(firstN(nil))},
		{"WindowTopNString", window(bson.D{{Key: "$topN", Value: bson.D{{Key: "output", Value: "$_id"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, {Key: "n", Value: "x"}}}})},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    "GroupNCollection_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   groupNArraysSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
			},
		})
	}
}

// $group and $bucket stop evaluating $firstN's input once they hold n
// values; windows and $bucketAuto evaluate it for every document.
func TestFirstNEvaluation(t *testing.T) {
	firstN := bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: bson.D{{Key: "$divide", Value: bson.A{int32(1), "$z"}}}}, {Key: "n", Value: int32(1)}}}}
	cases := map[string][]bson.D{
		"Group":      {{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, {{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: firstN}}}}},
		"Bucket":     {{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$_id"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "output", Value: bson.D{{Key: "r", Value: firstN}}}}}}},
		"BucketAuto": {{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$_id"}, {Key: "buckets", Value: int32(1)}, {Key: "output", Value: bson.D{{Key: "r", Value: firstN}}}}}}},
		"Window":     {{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "output", Value: bson.D{{Key: "r", Value: firstN}}}}}}},
	}
	for name, stages := range cases {
		stages := stages
		harness.PairTest(t, harness.TestCase{
			Name:    "FirstNEvaluation_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "z", Value: int32(1)}},
					bson.D{{Key: "_id", Value: int32(2)}, {Key: "z", Value: int32(1)}},
					bson.D{{Key: "_id", Value: int32(3)}, {Key: "z", Value: int32(0)}},
				})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
}
