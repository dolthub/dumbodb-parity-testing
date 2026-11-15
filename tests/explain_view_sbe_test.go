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

// TestExplainPlan_ViewSBEStages: mongod runs an equality $lookup of a
// pipeline on a view in SBE, as on a collection. The $group or $lookup of the
// view's own pipeline runs there too, unless the pipeline run on the view
// holds an expression SBE cannot run outside a $match, which leaves it to the
// classic engine.
func TestExplainPlan_ViewSBEStages(t *testing.T) {
	join := func(coll string) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: coll + "_f"}, {Key: "localField", Value: "g"},
			{Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"}}}}
	}
	match := bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$gte", Value: 0}}}}}}
	group := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$x"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}
	setLiteral := bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}}
	setToInt := bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: bson.D{{Key: "$toInt", Value: "1"}}}}}}
	sizeOfJ := bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$size", Value: "$j"}}}}}}
	exprSize := bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gte", Value: bson.A{bson.D{{Key: "$size", Value: "$j"}}, 0}}}}}}}
	cases := []struct {
		name     string
		view     func(coll string) bson.A
		pipeline func(coll string) bson.A
	}{
		{"match_view_lookup", func(string) bson.A { return bson.A{match} }, func(c string) bson.A { return bson.A{join(c)} }},
		{"match_view_lookup_size", func(string) bson.A { return bson.A{match} }, func(c string) bson.A { return bson.A{join(c), sizeOfJ} }},
		{"lookup_view_set", func(c string) bson.A { return bson.A{join(c)} }, func(string) bson.A { return bson.A{setLiteral} }},
		{"lookup_view_set_to_int", func(c string) bson.A { return bson.A{join(c)} }, func(string) bson.A { return bson.A{setToInt} }},
		{"lookup_view_size", func(c string) bson.A { return bson.A{join(c)} }, func(string) bson.A { return bson.A{sizeOfJ} }},
		{"lookup_view_expr_size", func(c string) bson.A { return bson.A{join(c)} }, func(string) bson.A { return bson.A{exprSize} }},
		{"group_view_set", func(string) bson.A { return bson.A{group} }, func(string) bson.A { return bson.A{setLiteral} }},
		{"group_view_set_to_int", func(string) bson.A { return bson.A{group} }, func(string) bson.A { return bson.A{setToInt} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_ViewSBEStages_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   sbeExplainSetup(false),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					db := col.Database()
					view := col.Name() + "_v"
					if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: view}, {Key: "viewOn", Value: col.Name()},
						{Key: "pipeline", Value: tc.view(col.Name())}}).Err(); err != nil {
						return nil, err
					}
					defer db.Collection(view).Drop(ctx)
					cmd := bson.D{{Key: "aggregate", Value: view}, {Key: "pipeline", Value: tc.pipeline(col.Name())}, {Key: "cursor", Value: bson.D{}}}
					doc, err := explainRunExplain(ctx, col, cmd, "queryPlanner")
					if err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					return bson.D{{Key: "explainVersion", Value: ver}, {Key: "sbe", Value: sbeQueryPlanStages(doc)},
						{Key: "stages", Value: topLevelStages(doc)}}, nil
				},
			})
		})
	}
}

// TestAggregate_ViewSBEStages: the $group of a view's pipeline reports SBE's
// expression errors, unless the pipeline run on the view holds an expression
// SBE cannot run, which leaves the $group to the classic engine.
func TestAggregate_ViewSBEStages(t *testing.T) {
	group := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
		{Key: "s", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$add", Value: bson.A{"$x", "s"}}}}}}}}}
	cases := []struct {
		name string
		set  bson.D
	}{
		{"literal", bson.D{{Key: "z", Value: 1}}},
		{"to_int", bson.D{{Key: "z", Value: bson.D{{Key: "$toInt", Value: "1"}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Aggregate_ViewSBEStages_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   sbeExplainSetup(false),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					db := col.Database()
					view := col.Name() + "_v"
					if err := db.CreateView(ctx, view, col.Name(), mongo.Pipeline{group}); err != nil {
						return nil, err
					}
					defer db.Collection(view).Drop(ctx)
					cursor, err := db.Collection(view).Aggregate(ctx, bson.A{bson.D{{Key: "$set", Value: tc.set}}})
					return drainCursor(ctx, cursor, err)
				},
			})
		})
	}
}
