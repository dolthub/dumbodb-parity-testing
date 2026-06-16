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

func insertGroupMixedNumKeySeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "m1"}, {Key: "k", Value: int32(1)}, {Key: "v", Value: int32(10)}},
		bson.D{{Key: "_id", Value: "m2"}, {Key: "k", Value: int64(1)}, {Key: "v", Value: int32(20)}},
		bson.D{{Key: "_id", Value: "m3"}, {Key: "k", Value: float64(1.0)}, {Key: "v", Value: int32(30)}},
		bson.D{{Key: "_id", Value: "m4"}, {Key: "k", Value: int32(2)}, {Key: "v", Value: int32(40)}},
		bson.D{{Key: "_id", Value: "m5"}, {Key: "k", Value: int64(2)}, {Key: "v", Value: int32(5)}},
	})

	return err
}

func TestAggGroupEdge_MixedNumericKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupEdge_MixedNumericKey",
		Support: harness.DumboDBFull,
		Setup:   insertGroupMixedNumKeySeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$k"},
					{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$v"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})

			return docsToSlice(results), err
		},
	})
}

func insertGroupMissingFieldSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "a1"}, {Key: "g", Value: "a"}, {Key: "v", Value: int32(2)}},
		bson.D{{Key: "_id", Value: "a2"}, {Key: "g", Value: "a"}, {Key: "v", Value: int32(4)}},
		bson.D{{Key: "_id", Value: "b1"}, {Key: "g", Value: "b"}},
		bson.D{{Key: "_id", Value: "b2"}, {Key: "g", Value: "b"}},
		bson.D{{Key: "_id", Value: "c1"}, {Key: "g", Value: "c"}, {Key: "v", Value: int32(10)}},
		bson.D{{Key: "_id", Value: "c2"}, {Key: "g", Value: "c"}},
		bson.D{{Key: "_id", Value: "c3"}, {Key: "g", Value: "c"}, {Key: "v", Value: "x"}},
	})

	return err
}

func TestAggGroupEdge_SumAvgMissingAndNonNumeric(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupEdge_SumAvgMissingAndNonNumeric",
		Support: harness.DumboDBFull,
		Setup:   insertGroupMissingFieldSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$g"},
					{Key: "s", Value: bson.D{{Key: "$sum", Value: "$v"}}},
					{Key: "a", Value: bson.D{{Key: "$avg", Value: "$v"}}},
				}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})

			return docsToSlice(results), err
		},
	})
}

func TestAggGroupEdge_EmptyInput(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupEdge_EmptyInput",
		Support: harness.DumboDBFull,
		Setup:   insertGroupMissingFieldSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "g", Value: "nonexistent"}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: nil},
					{Key: "total", Value: bson.D{{Key: "$sum", Value: "$v"}}},
					{Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}},
				}}},
			})

			return docsToSlice(results), err
		},
	})
}

func insertGroupStdDevSeed(ctx context.Context, col *mongo.Collection) error {
	docs := []interface{}{}
	add := func(id, g string, v int32) {
		docs = append(docs, bson.D{{Key: "_id", Value: id}, {Key: "g", Value: g}, {Key: "v", Value: v}})
	}
	for i, v := range []int32{2, 4, 4, 4, 5, 5, 7, 9} {
		add("a"+string(rune('0'+i)), "a", v)
	}
	for i, v := range []int32{1, 2, 3, 4, 5} {
		add("b"+string(rune('0'+i)), "b", v)
	}
	add("c0", "c", 42)

	_, err := col.InsertMany(ctx, docs)

	return err
}

// stdDevRounded groups by g, applies the given accumulator over the groups with
// more than one value, and rounds to 10 places so the comparison tolerates the
// last-ULP difference between the two-pass and online variance algorithms.
func stdDevRounded(op string) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		results, err := runPipeline(ctx, col, []bson.D{
			{{Key: "$match", Value: bson.D{{Key: "g", Value: bson.D{{Key: "$in", Value: bson.A{"a", "b"}}}}}}},
			{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: "$g"},
				{Key: "sd", Value: bson.D{{Key: op, Value: "$v"}}},
			}}},
			{{Key: "$project", Value: bson.D{
				{Key: "_id", Value: 1},
				{Key: "sd", Value: bson.D{{Key: "$round", Value: bson.A{"$sd", 10}}}},
			}}},
			{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		})

		return docsToSlice(results), err
	}
}

func TestAggGroupEdge_StdDevPop(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupEdge_StdDevPop",
		Support: harness.DumboDBFull,
		Setup:   insertGroupStdDevSeed,
		Run:     stdDevRounded("$stdDevPop"),
	})
}

func TestAggGroupEdge_StdDevSamp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupEdge_StdDevSamp",
		Support: harness.DumboDBFull,
		Setup:   insertGroupStdDevSeed,
		Run:     stdDevRounded("$stdDevSamp"),
	})
}

func TestAggGroupEdge_StdDevSingleValue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupEdge_StdDevSingleValue",
		Support: harness.DumboDBFull,
		Setup:   insertGroupStdDevSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$match", Value: bson.D{{Key: "g", Value: "c"}}}},
				{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: "$g"},
					{Key: "pop", Value: bson.D{{Key: "$stdDevPop", Value: "$v"}}},
					{Key: "samp", Value: bson.D{{Key: "$stdDevSamp", Value: "$v"}}},
				}}},
			})

			return docsToSlice(results), err
		},
	})
}
