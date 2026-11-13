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

// TestWindowSum_RemovableWindows: window $sum and $avg match mongod, which
// sums a window with a lower bound in one running total per partition, adding
// each value as it enters and its negation as it leaves, so an overflow or a
// rounding stays in later windows. The result narrows to the types still in
// the window, and NaN and infinities are counted rather than summed.
func TestWindowSum_RemovableWindows(t *testing.T) {
	dec := mustDecimal
	datasets := map[string]bson.A{
		"decimal_and_overflow": {dec("1"), 1e308, 1e308, 0.1, 0.2, dec("0.3"), int64(9007199254740993),
			int32(1), int32(math.MaxInt32), 1e16, int32(1)},
		"nan_and_infinity": {0.1, 0.2, int64(9007199254740993), int32(1), int32(math.MaxInt32), 1e16, int32(1),
			int32(1), dec("NaN"), int32(3), math.Inf(1), int32(3), int32(3)},
		"integer_limits": {int32(math.MinInt32), int32(math.MaxInt32), int64(math.MinInt64), int64(math.MaxInt64),
			int32(math.MinInt32), int32(1), int64(math.MinInt64), 2.5, dec("1.000000000000000000000000000000001"),
			int32(-1), "text", nil},
		"long_then_tenths": {int64(9007199254740993), int32(math.MaxInt32), 0.1, 0.1, int32(1),
			int32(math.MinInt32), -1e308, int32(math.MinInt32), int64(9007199254740993), dec("0.3"), int32(1)},
	}
	windows := map[string]bson.D{
		"docs_prev":            {{Key: "documents", Value: bson.A{int32(-1), int32(0)}}},
		"docs_around":          {{Key: "documents", Value: bson.A{int32(-2), int32(1)}}},
		"docs_ahead":           {{Key: "documents", Value: bson.A{int32(1), int32(3)}}},
		"docs_behind":          {{Key: "documents", Value: bson.A{int32(-3), int32(-1)}}},
		"docs_current_on":      {{Key: "documents", Value: bson.A{"current", "unbounded"}}},
		"docs_through_current": {{Key: "documents", Value: bson.A{"unbounded", "current"}}},
		"range_behind":         {{Key: "range", Value: bson.A{int32(-2), int32(0)}}},
		"range_around":         {{Key: "range", Value: bson.A{int32(-3), int32(3)}}},
		"range_on":             {{Key: "range", Value: bson.A{int32(0), "unbounded"}}},
	}
	for dataName, values := range datasets {
		setup := func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, len(values))
			for i, v := range values {
				docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "p", Value: int32(i % 2)}, {Key: "v", Value: v}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		}
		for windowName, window := range windows {
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("WindowSum_RemovableWindows_%s_%s", dataName, windowName),
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: pipelineRunExtJSON(bson.A{
					bson.D{{Key: "$setWindowFields", Value: bson.D{
						{Key: "partitionBy", Value: "$p"},
						{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
						{Key: "output", Value: bson.D{
							{Key: "s", Value: bson.D{{Key: "$sum", Value: "$v"}, {Key: "window", Value: window}}},
							{Key: "a", Value: bson.D{{Key: "$avg", Value: "$v"}, {Key: "window", Value: window}}},
						}},
					}}},
					bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: 1}, {Key: "a", Value: 1}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				}),
			})
		}
	}
}

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

// TestAggGroupNum_ConstantSum: the classic engine sums a literal int, long or
// double $sum argument with plain typed arithmetic (an int overflows to a
// long, a long to a double, doubles add uncompensated); SBE uses the
// double-double. A decimal literal takes the usual path in both.
func TestAggGroupNum_ConstantSum(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 10)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "g", Value: int32(i % 2)}}
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	args := map[string]interface{}{
		"double":         0.1,
		"int_max":        int32(math.MaxInt32),
		"long_max":       int64(math.MaxInt64),
		"literal_double": bson.D{{Key: "$literal", Value: 0.1}},
		"decimal":        mustDecimal("0.1"),
		"expression":     bson.D{{Key: "$add", Value: bson.A{0.1, int32(0)}}},
	}
	for name, arg := range args {
		pipelines := map[string]bson.A{
			"group_sbe": {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "s", Value: bson.D{{Key: "$sum", Value: arg}}}}}}},
			"group_classic": {bson.D{{Key: "$_internalInhibitOptimization", Value: bson.D{}}},
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "s", Value: bson.D{{Key: "$sum", Value: arg}}}}}}},
			"bucket": {bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$_id"},
				{Key: "boundaries", Value: bson.A{int32(0), int32(5), int32(10)}},
				{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$sum", Value: arg}}}}}}}}},
			"bucket_auto": {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$_id"}, {Key: "buckets", Value: int32(2)},
				{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$sum", Value: arg}}}}}}}}},
		}
		for pipeName, pipeline := range pipelines {
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("AggGroupNum_ConstantSum_%s_%s", name, pipeName),
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRunExtJSON(pipeline),
			})
		}
	}
}
