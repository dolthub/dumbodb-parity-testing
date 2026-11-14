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

// TestFilterExprPushedMatch: mongod pushes the leading $match of a $lookup or
// $unionWith sub-pipeline, and a $match after a leading $sort, into the
// query, so it checks $expr in that query's plan order: last in a collection
// scan, first when an index (the joined foreign field's included) serves it.
func TestFilterExprPushedMatch(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(9)}, {Key: "w", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{1, 2}}, {Key: "v", Value: int32(9)}, {Key: "w", Value: int32(1)}},
		}); err != nil {
			return err
		}
		if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "w", Value: 1}}}); err != nil {
			return err
		}
		other := col.Database().Collection(col.Name() + "_local")
		if err := other.Drop(ctx); err != nil {
			return err
		}
		_, err := other.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(9)}, {Key: "w", Value: int32(1)}})
		return err
	}
	v8 := bson.E{Key: "v", Value: 8}
	match := func(conds ...bson.E) bson.D { return bson.D{{Key: "$match", Value: bson.D(conds)}} }
	sortBy := func(field string) bson.D { return bson.D{{Key: "$sort", Value: bson.D{{Key: field, Value: 1}}}} }
	subPipelines := map[string]bson.A{
		"match":            {match(exprPlanSize, v8)},
		"sort_match":       {sortBy("_id"), match(exprPlanSize, v8)},
		"sort_index_match": {sortBy("w"), match(exprPlanSize, v8)},
		"two_matches":      {match(v8), match(exprPlanSize)},
		"indexed_eq":       {match(exprPlanSize, v8, bson.E{Key: "w", Value: 1})},
		"indexed_range":    {match(exprPlanSize, v8, bson.E{Key: "_id", Value: bson.D{{Key: "$gte", Value: 0}}})},
		"or_expr_second":   {match(bson.E{Key: "$or", Value: bson.A{bson.D{{Key: "v", Value: 9}}, bson.D{exprPlanSize}}})},
		"limit_match":      {bson.D{{Key: "$limit", Value: 9}}, match(exprPlanSize, v8)},
	}
	for name, sub := range subPipelines {
		stages := map[string]bson.D{
			"lookup": {{Key: "$lookup", Value: bson.D{{Key: "from", Value: ""}, {Key: "pipeline", Value: sub}, {Key: "as", Value: "j"}}}},
			"lookup_let": {{Key: "$lookup", Value: bson.D{{Key: "from", Value: ""}, {Key: "let", Value: bson.D{{Key: "x", Value: "$v"}}},
				{Key: "pipeline", Value: sub}, {Key: "as", Value: "j"}}}},
			"union_with": {{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: ""}, {Key: "pipeline", Value: sub}}}},
		}
		for kind, stage := range stages {
			harness.PairTest(t, harness.TestCase{
				Name:    "FilterExprPushedMatch_" + kind + "_" + name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					spec := stage[0].Value.(bson.D)
					named := make(bson.D, len(spec))
					copy(named, spec)
					for i := range named {
						if named[i].Key == "from" || named[i].Key == "coll" {
							named[i].Value = col.Name()
						}
					}
					local := col.Database().Collection(col.Name() + "_local")
					cursor, err := local.Aggregate(ctx, bson.A{bson.D{{Key: stage[0].Key, Value: named}}, bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 0}}}}})
					return drainCursor(ctx, cursor, err)
				},
			})
		}
	}
	joins := map[string]string{"unindexed_field": "v", "indexed_field": "w", "id": "_id"}
	for name, field := range joins {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterExprPushedMatch_lookup_join_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				local := col.Database().Collection(col.Name() + "_local")
				cursor, err := local.Aggregate(ctx, bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()},
					{Key: "localField", Value: field}, {Key: "foreignField", Value: field},
					{Key: "pipeline", Value: bson.A{match(exprPlanSize, v8)}}, {Key: "as", Value: "j"}}}}})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
	tops := map[string]bson.A{
		"sort_match":         {sortBy("_id"), match(exprPlanSize, v8)},
		"sort_field_match":   {sortBy("v"), match(exprPlanSize, v8)},
		"match_sort_match":   {match(bson.E{Key: "v", Value: 9}), sortBy("v"), match(exprPlanSize, v8)},
		"sort_indexed_match": {sortBy("v"), match(exprPlanSize, v8, bson.E{Key: "w", Value: 1})},
	}
	for name, pipeline := range tops {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterExprPushedMatch_aggregate_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
