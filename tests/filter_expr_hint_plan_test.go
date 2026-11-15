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

// exprPlanSetup seeds a document whose failing $expr is reached only when
// $expr is checked before v: 8, with indexes on b, {b, c} and v.
func exprPlanSetup(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(9)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{1, 2}}, {Key: "v", Value: int32(9)}},
	}); err != nil {
		return err
	}
	_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "b", Value: 1}}},
		{Keys: bson.D{{Key: "b", Value: 1}, {Key: "c", Value: 1}}},
		{Keys: bson.D{{Key: "v", Value: 1}}},
	})
	return err
}

var exprPlanSize = bson.E{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$size", Value: "$a"}}, 2}}}}

// TestFilterExprHintPlan: a hinted index whose leading field the filter does
// not constrain is read in full and filtered as a collection scan, $expr
// last; one whose leading field is constrained drops documents by its bounds
// before $expr, then checks $expr before the other predicates. An index read
// only for a sort is filtered as a collection scan too.
func TestFilterExprHintPlan(t *testing.T) {
	cond := func(field string, v interface{}) bson.E { return bson.E{Key: field, Value: v} }
	op := func(name string, v interface{}) bson.D { return bson.D{{Key: name, Value: v}} }
	v8 := cond("v", 8)
	finds := map[string]struct {
		filter bson.D
		hint   interface{}
		sort   bson.D
	}{
		"hint_id_unconstrained":    {bson.D{exprPlanSize, v8}, bson.D{{Key: "_id", Value: 1}}, nil},
		"hint_b_unconstrained":     {bson.D{exprPlanSize, v8}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_name_unconstrained":  {bson.D{exprPlanSize, v8}, "b_1", nil},
		"hint_b_other_range":       {bson.D{exprPlanSize, v8, cond("_id", op("$gte", 1))}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_compound_second_key": {bson.D{exprPlanSize, v8, cond("c", nil)}, "b_1_c_1", nil},
		"hint_id_range":            {bson.D{exprPlanSize, v8, cond("_id", op("$gte", 1))}, bson.D{{Key: "_id", Value: 1}}, nil},
		"hint_b_null":              {bson.D{exprPlanSize, v8, cond("b", nil)}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_b_ne":                {bson.D{exprPlanSize, v8, cond("b", op("$ne", 5))}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_b_nor":               {bson.D{exprPlanSize, v8, cond("$nor", bson.A{bson.D{cond("b", 5)}})}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_b_or_bounds_empty":   {bson.D{exprPlanSize, v8, cond("$or", bson.A{bson.D{cond("b", 1)}, bson.D{cond("b", 2)}})}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_b_type_bounds_empty": {bson.D{exprPlanSize, v8, cond("b", op("$type", "string"))}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_b_size":              {bson.D{exprPlanSize, v8, cond("b", op("$size", 1))}, bson.D{{Key: "b", Value: 1}}, nil},
		"hint_v_bounds_empty":      {bson.D{exprPlanSize, v8}, "v_1", nil},
		"hint_v_range":             {bson.D{exprPlanSize, cond("v", op("$gt", 8))}, "v_1", nil},
		"sort_on_index":            {bson.D{exprPlanSize, v8}, nil, bson.D{{Key: "b", Value: 1}}},
		"sort_on_id":               {bson.D{exprPlanSize, v8}, nil, bson.D{{Key: "_id", Value: 1}}},
		"two_candidate_indexes":    {bson.D{exprPlanSize, v8, cond("b", op("$ne", 5))}, nil, nil},
	}
	for name, f := range finds {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterExprHintPlan_find_" + name,
			Support: harness.DumboDBFull,
			Setup:   exprPlanSetup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cmd := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: f.filter}}
				if f.hint != nil {
					cmd = append(cmd, bson.E{Key: "hint", Value: f.hint})
				}
				if f.sort != nil {
					cmd = append(cmd, bson.E{Key: "sort", Value: f.sort})
				}
				return runCommandDoc(ctx, col, cmd)
			},
		})
	}
	hintID := bson.D{{Key: "_id", Value: 1}}
	filter := bson.D{exprPlanSize, v8}
	commands := map[string]func(name string) bson.D{
		"aggregate": func(name string) bson.D {
			return bson.D{{Key: "aggregate", Value: name}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: filter}}}},
				{Key: "hint", Value: hintID}, {Key: "cursor", Value: bson.D{}}}
		},
		"update": func(name string) bson.D {
			return bson.D{{Key: "update", Value: name}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: filter},
				{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: 1}}}}}, {Key: "multi", Value: true}, {Key: "hint", Value: hintID}}}}}
		},
		"delete": func(name string) bson.D {
			return bson.D{{Key: "delete", Value: name}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: filter},
				{Key: "limit", Value: 0}, {Key: "hint", Value: hintID}}}}}
		},
		"count": func(name string) bson.D {
			return bson.D{{Key: "count", Value: name}, {Key: "query", Value: filter}, {Key: "hint", Value: hintID}}
		},
	}
	for name, cmd := range commands {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterExprHintPlan_" + name + "_hint_id",
			Support: harness.DumboDBFull,
			Setup:   exprPlanSetup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return runCommandDoc(ctx, col, cmd(col.Name()))
			},
		})
	}
}
