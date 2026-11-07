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
	"go.mongodb.org/mongo-driver/mongo/options"

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

// TestAggGroupNum_SumOrder: $sum and $avg give one result whatever the order
// of doubles, integers and decimals, in $group (input sorted on _id first)
// and over an array. Doubles alone keep the double-double sum's precision.
func TestAggGroupNum_SumOrder(t *testing.T) {
	dec := mustDecimal
	cases := []struct {
		name   string
		values bson.A
	}{
		{"overflow_then_decimal", bson.A{1e308, 1e308, dec("1")}},
		{"decimal_then_overflow", bson.A{dec("1"), 1e308, 1e308}},
		{"decimal_then_overflow_back", bson.A{dec("1"), 1e308, 1e308, -1e308}},
		{"tiny_decimal_after", bson.A{1e20, dec("1E-20")}},
		{"tiny_decimal_before", bson.A{dec("1E-20"), 1e20}},
		{"cancel_then_decimal", bson.A{1e300, 1.0, -1e300, dec("2")}},
		{"decimal_then_cancel", bson.A{dec("2"), 1e300, 1.0, -1e300}},
		{"ints_doubles_decimal", bson.A{int32(1), 0.1, int64(2), dec("0.3"), 0.2}},
		{"doubles_small_after_large", bson.A{1e16, 1.0, 1.0, 1.0, 1.0}},
		{"doubles_tenths", bson.A{0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1, 0.1}},
		{"long_and_doubles", bson.A{int64(1 << 60), 0.5, 0.25}},
	}
	for _, tc := range cases {
		support := harness.DumboDBFull
		if tc.name == "ints_doubles_decimal" {
			// The last of 34 digits differs: mongod rounds its double-double's
			// conversion to Decimal128 in a way DumboDB does not reproduce.
			support = harness.DumboDBXFail
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "AggGroupNum_SumOrder_" + tc.name,
				Support:   support,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					docs := make([]interface{}, len(tc.values))
					for i, v := range tc.values {
						docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: v}}
					}
					_, err := col.InsertMany(ctx, docs)
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Aggregate(ctx, mongo.Pipeline{
						{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
						{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
							{Key: "sum", Value: bson.D{{Key: "$sum", Value: "$v"}}},
							{Key: "avg", Value: bson.D{{Key: "$avg", Value: "$v"}}}}}},
						{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "sum", Value: 1}, {Key: "avg", Value: 1},
							{Key: "arraySum", Value: bson.D{{Key: "$sum", Value: bson.A{tc.values}}}},
							{Key: "arrayAvg", Value: bson.D{{Key: "$avg", Value: bson.A{tc.values}}}}}}},
					})
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					err = cur.All(ctx, &docs)
					return docs, err
				},
			})
		})
	}
}

// TestAggGroup_Collation: under a case-insensitive collation $group puts
// strings equal under it in one group, at top level and inside an _id
// document, and $addToSet and distinct counts follow it.
func TestAggGroup_Collation(t *testing.T) {
	ci := &options.Collation{Locale: "en", Strength: 2}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: "abc"}, {Key: "n", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "s", Value: "ABC"}, {Key: "n", Value: 2}},
			bson.D{{Key: "_id", Value: 3}, {Key: "s", Value: "Abc"}, {Key: "n", Value: 3}},
			bson.D{{Key: "_id", Value: 4}, {Key: "s", Value: "xyz"}, {Key: "n", Value: 4}},
		})
		return err
	}
	cases := []struct {
		name  string
		group bson.D
	}{
		{"by_field", bson.D{{Key: "_id", Value: "$s"}, {Key: "total", Value: bson.D{{Key: "$sum", Value: "$n"}}}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}},
		{"by_document", bson.D{{Key: "_id", Value: bson.D{{Key: "k", Value: "$s"}}}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}},
		{"add_to_set", bson.D{{Key: "_id", Value: nil}, {Key: "set", Value: bson.D{{Key: "$addToSet", Value: "$s"}}}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "AggGroup_Collation_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Aggregate(ctx, mongo.Pipeline{
						{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
						{{Key: "$group", Value: tc.group}},
						{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}, {Key: "_id", Value: 1}}}},
						// $addToSet's order is unspecified.
						{{Key: "$addFields", Value: bson.D{{Key: "set", Value: bson.D{{Key: "$size", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$set", bson.A{}}}}}}}}}},
					}, options.Aggregate().SetCollation(ci))
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					err = cur.All(ctx, &docs)
					return docs, err
				},
			})
		})
	}
}
