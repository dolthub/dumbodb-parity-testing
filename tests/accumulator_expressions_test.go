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

// Accumulators that MongoDB also evaluates as expressions over an array or a
// list of arguments.

func accumulatorExprSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(3), 1.5, int64(7), nil, "s", bson.A{int32(100)}}}, {Key: "x", Value: int32(4)}, {Key: "y", Value: 2.5}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{}}, {Key: "x", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: int32(9)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{int32(5), int32(1), int32(4), int32(2), int32(3)}}, {Key: "x", Value: "z"}, {Key: "y", Value: int32(1)}},
	})
	return err
}

func TestExpr_Accumulators(t *testing.T) {
	nOp := func(op string, n, input interface{}) bson.D {
		return bson.D{{Key: op, Value: bson.D{{Key: "n", Value: n}, {Key: "input", Value: input}}}}
	}
	arraysOnly := bson.D{{Key: "a", Value: bson.D{{Key: "$type", Value: "array"}}}}
	for _, tc := range []struct {
		name  string
		expr  interface{}
		match bson.D
	}{
		{"MaxArray", bson.D{{Key: "$max", Value: "$a"}}, nil},
		{"MaxArgs", bson.D{{Key: "$max", Value: bson.A{"$x", "$y", int32(3)}}}, nil},
		{"MaxMixedTypes", bson.D{{Key: "$max", Value: bson.A{"$x", int32(1), bson.A{int32(1)}}}}, nil},
		{"MaxMissing", bson.D{{Key: "$max", Value: "$nope"}}, nil},
		{"MinArray", bson.D{{Key: "$min", Value: "$a"}}, nil},
		{"MinArgs", bson.D{{Key: "$min", Value: bson.A{"$x", "$y", int32(3)}}}, nil},
		{"MinNullOnly", bson.D{{Key: "$min", Value: bson.A{nil, "$nope"}}}, nil},
		{"StdDevPopArray", bson.D{{Key: "$stdDevPop", Value: "$a"}}, nil},
		{"StdDevPopArgs", bson.D{{Key: "$stdDevPop", Value: bson.A{"$x", "$y"}}}, nil},
		{"StdDevSampArray", bson.D{{Key: "$stdDevSamp", Value: "$a"}}, nil},
		{"StdDevSampSingle", bson.D{{Key: "$stdDevSamp", Value: bson.A{int32(3)}}}, nil},
		{"MaxN", bson.D{{Key: "$maxN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$a"}}}}, nil},
		{"MinN", bson.D{{Key: "$minN", Value: bson.D{{Key: "n", Value: int32(3)}, {Key: "input", Value: "$a"}}}}, nil},
		{"FirstN", bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$a"}}}}, nil},
		{"LastN", bson.D{{Key: "$lastN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$a"}}}}, nil},
		{"Median", bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$a"}, {Key: "method", Value: "approximate"}}}}, nil},
		{"Percentile", bson.D{{Key: "$percentile", Value: bson.D{{Key: "input", Value: "$a"}, {Key: "p", Value: bson.A{0.5, 0.9}}, {Key: "method", Value: "approximate"}}}}, nil},
		{"MaxSingleArrayOperand", bson.D{{Key: "$max", Value: bson.A{"$a"}}}, nil},
		{"MaxNArrays", nOp("$maxN", int32(2), "$a"), arraysOnly},
		{"MinNArrays", nOp("$minN", int32(3), "$a"), arraysOnly},
		{"FirstNArrays", nOp("$firstN", int32(2), "$a"), arraysOnly},
		{"LastNArrays", nOp("$lastN", int32(2), "$a"), arraysOnly},
		{"MaxNLargeN", nOp("$maxN", int32(10), "$a"), arraysOnly},
		{"MaxNLongN", nOp("$maxN", int64(1), "$a"), arraysOnly},
		{"MaxNZeroN", nOp("$maxN", int32(0), "$a"), arraysOnly},
		{"MaxNFractionalN", nOp("$maxN", 1.5, "$a"), arraysOnly},
		{"MaxNNullN", nOp("$maxN", "$x", "$a"), arraysOnly},
		{"FirstNMissingInput", nOp("$firstN", int32(1), "$nope"), arraysOnly},
		{"PercentileOfArgs", bson.D{{Key: "$percentile", Value: bson.D{{Key: "input", Value: bson.A{"$x", "$y", int32(10)}}, {Key: "p", Value: bson.A{0.0, 1.0}}, {Key: "method", Value: "approximate"}}}}, nil},
		{"MedianExactMethod", bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$a"}, {Key: "method", Value: "exact"}}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Expr_Accumulators_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   accumulatorExprSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					pipeline := bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}
					if tc.match != nil {
						pipeline = append(pipeline, bson.D{{Key: "$match", Value: tc.match}})
					}
					pipeline = append(pipeline, bson.D{{Key: "$project", Value: bson.D{{Key: "v", Value: tc.expr}}}})
					cursor, err := col.Aggregate(ctx, pipeline)
					if err != nil {
						return nil, err
					}
					var results []bson.D
					return results, cursor.All(ctx, &results)
				},
			})
		})
	}
}

func TestExpr_AccumulatorsDecimalAndEmpty(t *testing.T) {
	four, _ := primitive.ParseDecimal128("4")
	one, _ := primitive.ParseDecimal128("1")
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{four, int32(2)}}, {Key: "s", Value: bson.A{"x", bson.A{"y"}}}, {Key: "n", Value: nil}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{"x"}}, {Key: "s", Value: bson.A{}}},
		})
		return err
	}
	for _, tc := range []struct {
		name string
		expr interface{}
	}{
		{"StdDevPopDecimal", bson.D{{Key: "$stdDevPop", Value: "$a"}}},
		{"MedianDecimal", bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$a"}, {Key: "method", Value: "approximate"}}}}},
		{"MaxDecimal", bson.D{{Key: "$max", Value: "$a"}}},
		{"MaxNDecimalN", bson.D{{Key: "$maxN", Value: bson.D{{Key: "n", Value: one}, {Key: "input", Value: "$a"}}}}},
		{"PercentileNoNumericInput", bson.D{{Key: "$percentile", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "p", Value: bson.A{0.5, 0.1}}, {Key: "method", Value: "approximate"}}}}},
		{"MedianNoNumericInput", bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$s"}, {Key: "method", Value: "approximate"}}}}},
		{"EqMissingNull", bson.D{{Key: "$eq", Value: bson.A{"$missing", nil}}}},
		{"LtMissingNull", bson.D{{Key: "$lt", Value: bson.A{"$missing", nil}}}},
		{"CmpMissingNull", bson.D{{Key: "$cmp", Value: bson.A{"$missing", nil}}}},
		{"EqNullFieldNull", bson.D{{Key: "$eq", Value: bson.A{"$n", nil}}}},
		{"LtMissingMinKey", bson.D{{Key: "$lt", Value: bson.A{"$missing", primitive.MinKey{}}}}},
		{"EqMissingMissing", bson.D{{Key: "$eq", Value: bson.A{"$missing", "$other"}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Expr_AccumulatorsDecimalAndEmpty_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cursor, err := col.Aggregate(ctx, bson.A{
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
						bson.D{{Key: "$project", Value: bson.D{{Key: "v", Value: tc.expr}}}},
					})
					if err != nil {
						return nil, err
					}
					var results []bson.D
					return results, cursor.All(ctx, &results)
				},
			})
		})
	}
}
