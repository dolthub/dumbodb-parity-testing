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
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestBucketEngine: mongod runs a $bucket of fewer than 50 buckets in SBE and
// one of 50 or more in classic, which changes constant sums, $min/$max ties
// and error codes. Each document is accumulated as its bucket is found, so an
// accumulator error comes before a later document's missing bucket.
func TestBucketEngine(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 125)
		for i := range docs {
			var tie interface{} = int32(i % 3)
			if i%2 == 1 {
				tie = float64(i % 3)
			}
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: int32(i % 60)}, {Key: "t", Value: tie}, {Key: "z", Value: int32(1 - btoi(i == 7))}}
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	for _, buckets := range []int{49, 50, 51} {
		boundaries := bson.A{}
		shifted := bson.A{}
		for i := 0; i <= buckets; i++ {
			boundaries = append(boundaries, int32(i))
			shifted = append(shifted, int32(100+i))
		}
		cases := map[string]bson.A{
			"outputs": {bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: boundaries}, {Key: "default", Value: "o"},
					{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$sum", Value: 0.1}}},
						{Key: "mn", Value: bson.D{{Key: "$min", Value: "$t"}}}, {Key: "mx", Value: bson.D{{Key: "$max", Value: "$t"}}}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: 1}, {Key: "mn", Value: bson.D{{Key: "$type", Value: "$mn"}}}, {Key: "mx", Value: bson.D{{Key: "$type", Value: "$mx"}}}}}}},
			"no_branch": {bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: shifted}}}}},
			"error_order": {bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: boundaries},
					{Key: "output", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$first", Value: bson.D{{Key: "$divide", Value: bson.A{int32(1), "$z"}}}}}}}}}}}},
		}
		for name, pipeline := range cases {
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("BucketEngine_%d_%s", buckets, name),
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(pipeline),
			})
		}
	}
}

// TestAvgSBEIntegers: SBE's $avg divides the exact integer total, rounded
// once to a double, by the count; the classic engine divides the
// double-double's leading double.
func TestAvgSBEIntegers(t *testing.T) {
	groups := [][]interface{}{
		{int64(9007199254740993), int32(1), int64(1), int64(math.MinInt64), int32(math.MinInt32), int32(math.MinInt32), int64(math.MaxInt64)},
		{int32(-1), int64(-9007199254740993), int64(math.MinInt64), int32(0), int64(-9007199254740993), int64(math.MaxInt64)},
		{int64(math.MaxInt64), int64(math.MaxInt64), int32(3)},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		var docs []interface{}
		for g, values := range groups {
			for _, v := range values {
				docs = append(docs, bson.D{{Key: "_id", Value: int32(len(docs))}, {Key: "g", Value: int32(g)}, {Key: "v", Value: v}})
			}
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	group := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "a", Value: bson.D{{Key: "$avg", Value: "$v"}}}}}}
	pipelines := map[string]bson.A{
		"sbe":     {bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, group, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}},
		"classic": {bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, bson.D{{Key: "$_internalInhibitOptimization", Value: bson.D{}}}, group, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}},
		"bucket": {bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$g"},
			{Key: "boundaries", Value: bson.A{int32(0), int32(1), int32(2), int32(3)}}, {Key: "output", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$avg", Value: "$v"}}}}}}}}},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "AvgSBEIntegers_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRunExtJSON(pipeline),
		})
	}
}

// TestConstantSumEngines: thirty documents per group tell the classic
// engine's plain addition of a constant 0.1 from SBE's double-double, in
// $group and in windows.
func TestConstantSumEngines(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 60)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "g", Value: int32(i % 2)}}
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	group := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "s", Value: bson.D{{Key: "$sum", Value: 0.1}}}}}}
	byID := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}
	pipelines := map[string]bson.A{
		"group_sbe":     {group, byID},
		"group_classic": {bson.D{{Key: "$_internalInhibitOptimization", Value: bson.D{}}}, group, byID},
		"window_unbounded": {bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "partitionBy", Value: "$g"}, {Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$sum", Value: 0.1}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "current"}}}}}}}}}}}, byID},
		"window_bounded": {bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "partitionBy", Value: "$g"}, {Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$sum", Value: 0.1}, {Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{int32(-29), int32(0)}}}}}}}}}}}, byID},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "ConstantSumEngines_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRunExtJSON(pipeline),
		})
	}
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
