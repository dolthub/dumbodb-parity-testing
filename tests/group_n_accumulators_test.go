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

// $firstN, $lastN, $minN, $maxN, $top, $bottom, $topN, $bottomN, $median and
// $percentile as $group, $bucket and $setWindowFields accumulators
// (dolthub/dumbodb#188). Input is sorted by _id first, since $firstN, $lastN
// and the order of ties follow input order.
//
// An empty sortBy is left out: with a $sort ahead of the $group, mongod
// 8.0.28 crashes on it.

func nAccSeed(ctx context.Context, col *mongo.Collection) error {
	dec, err := primitive.ParseDecimal128("4.5")
	if err != nil {
		return err
	}
	_, err = col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "g", Value: int32(1)}, {Key: "v", Value: int32(5)}, {Key: "s", Value: "b"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "g", Value: int32(1)}, {Key: "v", Value: nil}, {Key: "s", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "g", Value: int32(1)}, {Key: "s", Value: "c"}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "g", Value: int32(1)}, {Key: "v", Value: int64(3)}, {Key: "s", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "g", Value: int32(2)}, {Key: "v", Value: "x"}, {Key: "s", Value: "z"}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "g", Value: int32(2)}, {Key: "v", Value: 1.5}, {Key: "s", Value: "y"}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "g", Value: int32(1)}, {Key: "v", Value: 9.0}, {Key: "s", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "g", Value: int32(2)}, {Key: "v", Value: dec}, {Key: "s", Value: "y"}},
	})
	return err
}

func nAccGroup(acc bson.D) bson.A {
	return bson.A{
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "r", Value: acc}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
}

func TestGroupNAccumulators(t *testing.T) {
	op := func(name string, spec bson.D) bson.D { return bson.D{{Key: name, Value: spec}} }
	nSpec := func(input, n interface{}) bson.D { return bson.D{{Key: "input", Value: input}, {Key: "n", Value: n}} }
	sortS := bson.D{{Key: "s", Value: 1}}
	approx := bson.E{Key: "method", Value: "approximate"}
	cases := []struct {
		name string
		acc  bson.D
	}{
		{"FirstN", op("$firstN", nSpec("$v", 2))},
		{"FirstNLargerThanGroup", op("$firstN", nSpec("$v", 10))},
		{"FirstNMissingInput", op("$firstN", nSpec("$nope", 2))},
		{"LastN", op("$lastN", nSpec("$v", 3))},
		{"MinN", op("$minN", nSpec("$v", 2))},
		{"MaxN", op("$maxN", nSpec("$v", 3))},
		{"MinNStrings", op("$minN", nSpec("$s", 2))},
		{"MaxNArrays", op("$maxN", nSpec(bson.A{"$v", "$s"}, 2))},
		{"MinNMissingInput", op("$minN", nSpec("$nope", 2))},
		{"NFolded", op("$firstN", nSpec("$v", bson.D{{Key: "$add", Value: bson.A{1, 1}}}))},
		{"NDecimal", op("$firstN", nSpec("$v", primitive.NewDecimal128(0, 2)))},
		{"NLong", op("$firstN", nSpec("$v", int64(9999999999999)))},
		{"NDouble", op("$firstN", nSpec("$v", 2.0))},
		{"NZero", op("$firstN", nSpec("$v", 0))},
		{"NNegative", op("$firstN", nSpec("$v", -1))},
		{"NFraction", op("$firstN", nSpec("$v", 1.5))},
		{"NString", op("$firstN", nSpec("$v", "a"))},
		{"NFieldPath", op("$firstN", nSpec("$v", "$v"))},
		{"NGroupKey", op("$firstN", nSpec("$v", "$_id"))},
		{"NMissing", op("$firstN", bson.D{{Key: "input", Value: "$v"}})},
		{"NInputMissing", op("$firstN", bson.D{{Key: "n", Value: 2}})},
		{"NUnknownArgument", op("$firstN", bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: 2}, {Key: "x", Value: 1}})},
		{"NNotObject", bson.D{{Key: "$lastN", Value: 5}}},
		{"Top", op("$top", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: sortS}})},
		{"Bottom", op("$bottom", bson.D{{Key: "output", Value: bson.A{"$v", "$s"}}, {Key: "sortBy", Value: sortS}})},
		{"TopNCompoundSort", op("$topN", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}, {Key: "_id", Value: -1}}}, {Key: "n", Value: 3}})},
		{"TopNTies", op("$topN", bson.D{{Key: "output", Value: "$_id"}, {Key: "sortBy", Value: sortS}, {Key: "n", Value: 2}})},
		{"BottomNTiesDescending", op("$bottomN", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: -1}}}, {Key: "n", Value: 2}})},
		{"BottomNTiesAscending", op("$bottomN", bson.D{{Key: "output", Value: "$_id"}, {Key: "sortBy", Value: sortS}, {Key: "n", Value: 2}})},
		{"TopMissingOutput", op("$top", bson.D{{Key: "output", Value: "$nope"}, {Key: "sortBy", Value: sortS}})},
		{"TopNMissingOutput", op("$topN", bson.D{{Key: "output", Value: "$nope"}, {Key: "sortBy", Value: sortS}, {Key: "n", Value: 2}})},
		{"TopSortByMissingField", op("$top", bson.D{{Key: "output", Value: "$_id"}, {Key: "sortBy", Value: bson.D{{Key: "nope", Value: 1}}}})},
		{"TopNoSortBy", op("$top", bson.D{{Key: "output", Value: "$v"}})},
		{"TopNoOutput", op("$top", bson.D{{Key: "sortBy", Value: sortS}})},
		{"TopNNoN", op("$topN", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: sortS}})},
		{"TopNEmpty", op("$topN", bson.D{})},
		{"TopWithN", op("$top", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: sortS}, {Key: "n", Value: 2}})},
		{"TopSortByNotObject", op("$top", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: 1}})},
		{"TopSortByBadOrder", op("$top", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 2}}}})},
		{"TopSortByString", op("$top", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: "a"}}}})},
		{"TopSortByNatural", op("$top", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "$natural", Value: 1}}}})},
		{"TopNotObject", bson.D{{Key: "$top", Value: 5}}},
		{"TopNNFieldPath", op("$topN", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: sortS}, {Key: "n", Value: "$v"}})},
		{"TopNNZero", op("$topN", bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: sortS}, {Key: "n", Value: 0}})},
		{"Median", op("$median", bson.D{{Key: "input", Value: "$v"}, approx})},
		{"MedianMissingInput", op("$median", bson.D{{Key: "input", Value: "$nope"}, approx})},
		{"Percentile", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.1, 0.5, 1}}, approx})},
		{"PercentileFoldedP", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{bson.D{{Key: "$add", Value: bson.A{0.1, 0.1}}}}}, approx})},
		{"PercentileMissingInput", op("$percentile", bson.D{{Key: "input", Value: "$nope"}, {Key: "p", Value: bson.A{0.5, 0.9}}, approx})},
		{"PercentilePOutOfRange", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{1.5}}, approx})},
		{"PercentilePNegative", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{-0.1}}, approx})},
		{"PercentilePEmpty", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{}}, approx})},
		{"PercentilePScalar", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: 0.5}, approx})},
		{"PercentilePString", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{"a"}}, approx})},
		{"PercentilePFieldPath", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: "$v"}, approx})},
		{"PercentileNoP", op("$percentile", bson.D{{Key: "input", Value: "$v"}, approx})},
		{"PercentileNoInput", op("$percentile", bson.D{{Key: "p", Value: bson.A{0.5}}, approx})},
		{"PercentileNoMethod", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.5}}})},
		{"PercentileMethodNotString", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.5}}, {Key: "method", Value: 5}})},
		{"PercentileUnknownField", op("$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.5}}, approx, {Key: "x", Value: 1}})},
		{"PercentileNotObject", bson.D{{Key: "$percentile", Value: 5}}},
		{"MedianContinuous", op("$median", bson.D{{Key: "input", Value: "$v"}, {Key: "method", Value: "continuous"}})},
		{"MedianWithP", op("$median", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.5}}, approx})},
		{"MedianNotObject", bson.D{{Key: "$median", Value: 5}}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "GroupNAcc_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   nAccSeed,
			Run:     pipelineRun(nAccGroup(c.acc)),
		})
	}
}

func TestGroupNAccumulators_BucketAndWindow(t *testing.T) {
	approx := bson.E{Key: "method", Value: "approximate"}
	cases := []struct {
		name     string
		pipeline bson.A
	}{
		{"Bucket", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$g"}, {Key: "boundaries", Value: bson.A{0, 5}}, {Key: "output", Value: bson.D{
				{Key: "t", Value: bson.D{{Key: "$topN", Value: bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, {Key: "n", Value: 2}}}}},
				{Key: "m", Value: bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$v"}, approx}}}},
				{Key: "l", Value: bson.D{{Key: "$lastN", Value: bson.D{{Key: "input", Value: "$_id"}, {Key: "n", Value: 2}}}}},
			}}}}},
		}},
		{"Window", bson.A{
			bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "partitionBy", Value: "$g"}, {Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "output", Value: bson.D{
				{Key: "f", Value: bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: 2}}}}},
				{Key: "t", Value: bson.D{{Key: "$topN", Value: bson.D{{Key: "output", Value: "$v"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, {Key: "n", Value: 2}}}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{-1, 1}}}}}},
				{Key: "m", Value: bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$v"}, approx}}}},
				{Key: "mx", Value: bson.D{{Key: "$maxN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: 2}}}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{0, 1}}}}}},
				{Key: "b", Value: bson.D{{Key: "$bottomN", Value: bson.D{{Key: "output", Value: "$_id"}, {Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, {Key: "n", Value: 2}}}}},
				{Key: "p", Value: bson.D{{Key: "$percentile", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.9}}, approx}}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{-1, 0}}}}}},
				{Key: "ln", Value: bson.D{{Key: "$lastN", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "n", Value: 2}}}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", 0}}}}}},
				{Key: "mn", Value: bson.D{{Key: "$minN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: 1}}}}},
			}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "GroupNAcc_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   nAccSeed,
			Run:     pipelineRun(c.pipeline),
		})
	}
}
