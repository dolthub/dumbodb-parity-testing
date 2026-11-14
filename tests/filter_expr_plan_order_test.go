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

// TestFilterExprPlanOrder: whether a failing $expr is reached depends on the
// plan. A collection scan checks $expr after the other predicates, an index
// scan (here on _id) checks negations, then $expr, then the rest, and a
// $match not pushed into the query checks the predicates in written order.
func TestFilterExprPlanOrder(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(9)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{1, 2}}, {Key: "v", Value: int32(9)}},
		})
		return err
	}
	sizeExpr := bson.E{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$size", Value: "$a"}}, 2}}}}
	exprClause := bson.D{sizeExpr}
	idRange := bson.E{Key: "_id", Value: bson.D{{Key: "$gte", Value: 1}}}
	orClause := bson.E{Key: "$or", Value: bson.A{bson.D{{Key: "v", Value: 9}}, exprClause}}
	finds := map[string]bson.D{
		"collscan_expr_last":       {sizeExpr, {Key: "v", Value: 8}},
		"ixscan_expr_before_eq":    {sizeExpr, {Key: "v", Value: 8}, idRange},
		"ixscan_ne_before_expr":    {sizeExpr, {Key: "v", Value: bson.D{{Key: "$ne", Value: 9}}}, idRange},
		"ixscan_not_before_expr":   {sizeExpr, {Key: "v", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$eq", Value: 9}}}}}, idRange},
		"ixscan_nin_before_expr":   {sizeExpr, {Key: "v", Value: bson.D{{Key: "$nin", Value: bson.A{9}}}}, idRange},
		"ixscan_exists_false":      {sizeExpr, {Key: "w", Value: bson.D{{Key: "$exists", Value: false}}}, {Key: "v", Value: 8}, idRange},
		"collscan_or_short_cuts":   {orClause},
		"ixscan_or_expr_first":     {orClause, idRange},
		"ixscan_and_or_expr_first": {{Key: "$and", Value: bson.A{bson.D{orClause}, bson.D{idRange}}}},
	}
	for name, filter := range finds {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterExprPlanOrder_find_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return runCommandDoc(ctx, col, bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}, {Key: "sort", Value: bson.D{{Key: "_id", Value: 1}}}})
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "FilterExprPlanOrder_find_natural_hint",
		Support: harness.DumboDBFull,
		Setup:   setup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runCommandDoc(ctx, col, bson.D{{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{orClause, idRange}}, {Key: "hint", Value: bson.D{{Key: "$natural", Value: 1}}}, {Key: "sort", Value: bson.D{{Key: "_id", Value: 1}}}})
		},
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "FilterExprPlanOrder_count_ixscan",
		Support: harness.DumboDBFull,
		Setup:   setup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runCommandDoc(ctx, col, bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: bson.D{orClause, idRange}}})
		},
	})
	matches := map[string]bson.A{
		"unpushed_written_order_fails": {bson.D{{Key: "$limit", Value: 9}}, bson.D{{Key: "$match", Value: bson.D{sizeExpr, {Key: "v", Value: 8}}}}},
		"unpushed_written_order_skips": {bson.D{{Key: "$limit", Value: 9}}, bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: 10}}}, sizeExpr}}}},
		"pushed_collscan":              {bson.D{{Key: "$match", Value: bson.D{sizeExpr, {Key: "v", Value: 8}}}}},
	}
	for name, pipeline := range matches {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterExprPlanOrder_aggregate_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, pipeline)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
