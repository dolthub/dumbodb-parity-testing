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

// TestAggGroupNum_SumNearestDouble: $sum and $avg of doubles report the
// double-double's leading double, as mongod does, so a total that cancels a
// large value keeps that value's rounding; a Decimal128 total gets the
// non-decimal total added even when that is zero. Both engines agree.
func TestAggGroupNum_SumNearestDouble(t *testing.T) {
	dec := mustDecimal
	cases := map[string]bson.A{
		"cancelled_long": {int64(9007199254740993), int32(math.MaxInt32), 0.1, 0.1, int64(-9007199254740993)},
		"decimal_huge":   {dec("1E+6000")},
		"decimal_scaled": {dec("1E+5"), dec("2.50")},
		"decimal_series": {dec("1.000000000000000000000000000000001"), int64(math.MinInt64), dec("1")},
	}
	for name, values := range cases {
		for engine, prefix := range map[string]bson.A{"sbe": {}, "classic": {bson.D{{Key: "$_internalInhibitOptimization", Value: bson.D{}}}}} {
			pipeline := append(bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}, prefix...)
			pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
				{Key: "s", Value: bson.D{{Key: "$sum", Value: "$v"}}},
				{Key: "a", Value: bson.D{{Key: "$avg", Value: "$v"}}}}}})
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("AggGroupNum_SumNearestDouble_%s_%s", name, engine),
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					docs := make([]interface{}, len(values))
					for i, v := range values {
						docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: v}}
					}
					_, err := col.InsertMany(ctx, docs)
					return err
				},
				Run: pipelineRunExtJSON(pipeline),
			})
		}
	}
}
