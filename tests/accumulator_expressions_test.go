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
	for _, tc := range []struct {
		name string
		expr interface{}
	}{
		{"MaxArray", bson.D{{Key: "$max", Value: "$a"}}},
		{"MaxArgs", bson.D{{Key: "$max", Value: bson.A{"$x", "$y", int32(3)}}}},
		{"MaxMixedTypes", bson.D{{Key: "$max", Value: bson.A{"$x", int32(1), bson.A{int32(1)}}}}},
		{"MaxMissing", bson.D{{Key: "$max", Value: "$nope"}}},
		{"MinArray", bson.D{{Key: "$min", Value: "$a"}}},
		{"MinArgs", bson.D{{Key: "$min", Value: bson.A{"$x", "$y", int32(3)}}}},
		{"MinNullOnly", bson.D{{Key: "$min", Value: bson.A{nil, "$nope"}}}},
		{"StdDevPopArray", bson.D{{Key: "$stdDevPop", Value: "$a"}}},
		{"StdDevPopArgs", bson.D{{Key: "$stdDevPop", Value: bson.A{"$x", "$y"}}}},
		{"StdDevSampArray", bson.D{{Key: "$stdDevSamp", Value: "$a"}}},
		{"StdDevSampSingle", bson.D{{Key: "$stdDevSamp", Value: bson.A{int32(3)}}}},
		{"MaxN", bson.D{{Key: "$maxN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$a"}}}}},
		{"MinN", bson.D{{Key: "$minN", Value: bson.D{{Key: "n", Value: int32(3)}, {Key: "input", Value: "$a"}}}}},
		{"FirstN", bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$a"}}}}},
		{"LastN", bson.D{{Key: "$lastN", Value: bson.D{{Key: "n", Value: int32(2)}, {Key: "input", Value: "$a"}}}}},
		{"Median", bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$a"}, {Key: "method", Value: "approximate"}}}}},
		{"Percentile", bson.D{{Key: "$percentile", Value: bson.D{{Key: "input", Value: "$a"}, {Key: "p", Value: bson.A{0.5, 0.9}}, {Key: "method", Value: "approximate"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Expr_Accumulators_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   accumulatorExprSetup,
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
