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

func mustDecimal(s string) primitive.Decimal128 {
	d, err := primitive.ParseDecimal128(s)
	if err != nil {
		panic(err)
	}

	return d
}

func groupByGSumAmount(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	results, err := runPipeline(ctx, col, []bson.D{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$g"},
			{Key: "total", Value: bson.D{{Key: "$sum", Value: "$amount"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})

	return docsToSlice(results), err
}

func groupByGAvgAmount(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	results, err := runPipeline(ctx, col, []bson.D{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$g"},
			{Key: "avg", Value: bson.D{{Key: "$avg", Value: "$amount"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	})

	return docsToSlice(results), err
}

func insertGroupDecimalSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "x1"}, {Key: "g", Value: "a"}, {Key: "amount", Value: mustDecimal("10.5")}},
		bson.D{{Key: "_id", Value: "x2"}, {Key: "g", Value: "a"}, {Key: "amount", Value: mustDecimal("20.25")}},
		bson.D{{Key: "_id", Value: "x3"}, {Key: "g", Value: "a"}, {Key: "amount", Value: mustDecimal("0.05")}},
		bson.D{{Key: "_id", Value: "x4"}, {Key: "g", Value: "b"}, {Key: "amount", Value: mustDecimal("100")}},
		bson.D{{Key: "_id", Value: "x5"}, {Key: "g", Value: "b"}, {Key: "amount", Value: int32(50)}},
	})

	return err
}

func TestAggGroupNum_DecimalSum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupNum_DecimalSum",
		Support: harness.DumboDBFull,
		Setup:   insertGroupDecimalSeed,
		Run:     groupByGSumAmount,
	})
}

func TestAggGroupNum_DecimalAvg(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupNum_DecimalAvg",
		Support: harness.DumboDBFull,
		Setup:   insertGroupDecimalSeed,
		Run:     groupByGAvgAmount,
	})
}

func insertGroupDecimalFloatSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "y1"}, {Key: "g", Value: "c"}, {Key: "amount", Value: float64(1.5)}},
		bson.D{{Key: "_id", Value: "y2"}, {Key: "g", Value: "c"}, {Key: "amount", Value: mustDecimal("2.5")}},
		bson.D{{Key: "_id", Value: "y3"}, {Key: "g", Value: "d"}, {Key: "amount", Value: float64(0.1)}},
		bson.D{{Key: "_id", Value: "y4"}, {Key: "g", Value: "d"}, {Key: "amount", Value: float64(0.2)}},
		bson.D{{Key: "_id", Value: "y5"}, {Key: "g", Value: "d"}, {Key: "amount", Value: mustDecimal("0.3")}},
	})

	return err
}

func TestAggGroupNum_DecimalFloatMixSum(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupNum_DecimalFloatMixSum",
		Support: harness.DumboDBFull,
		Setup:   insertGroupDecimalFloatSeed,
		Run:     groupByGSumAmount,
	})
}

func insertGroupBigIntSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: "a1"}, {Key: "g", Value: "i32"}, {Key: "amount", Value: int32(2000000000)}},
		bson.D{{Key: "_id", Value: "a2"}, {Key: "g", Value: "i32"}, {Key: "amount", Value: int32(2000000000)}},
		bson.D{{Key: "_id", Value: "a3"}, {Key: "g", Value: "i32"}, {Key: "amount", Value: int32(1000000000)}},
		bson.D{{Key: "_id", Value: "b1"}, {Key: "g", Value: "i64"}, {Key: "amount", Value: int64(9000000000000000000)}},
		bson.D{{Key: "_id", Value: "b2"}, {Key: "g", Value: "i64"}, {Key: "amount", Value: int64(9000000000000000000)}},
		bson.D{{Key: "_id", Value: "c1"}, {Key: "g", Value: "mix"}, {Key: "amount", Value: int32(5)}},
		bson.D{{Key: "_id", Value: "c2"}, {Key: "g", Value: "mix"}, {Key: "amount", Value: float64(2.5)}},
		bson.D{{Key: "_id", Value: "c3"}, {Key: "g", Value: "mix"}, {Key: "amount", Value: int64(3)}},
	})

	return err
}

func TestAggGroupNum_Int32SumOverflow(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupNum_Int32SumOverflow",
		Support: harness.DumboDBFull,
		Setup:   insertGroupBigIntSeed,
		Run:     groupByGSumAmount,
	})
}

func TestAggGroupNum_IntPromotionAvg(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGroupNum_IntPromotionAvg",
		Support: harness.DumboDBFull,
		Setup:   insertGroupBigIntSeed,
		Run:     groupByGAvgAmount,
	})
}
