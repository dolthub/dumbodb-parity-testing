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

// planUsesIndex reports whether a queryPlanner explain response's winning plan
// tree contains an index scan (IXSCAN/COUNT_SCAN/DISTINCT_SCAN) anywhere,
// walking inputStage/inputStages. Works on both MongoDB's nested FETCH->IXSCAN
// shape and DumboDB's flat winningPlan.
func planUsesIndex(explain bson.M) bool {
	qp, ok := explain["queryPlanner"].(bson.M)
	if !ok {
		return false
	}
	wp, ok := qp["winningPlan"].(bson.M)
	if !ok {
		return false
	}
	return stageUsesIndex(wp)
}

func stageUsesIndex(stage bson.M) bool {
	switch s, _ := stage["stage"].(string); s {
	case "IXSCAN", "COUNT_SCAN", "DISTINCT_SCAN":
		return true
	}
	if child, ok := stage["inputStage"].(bson.M); ok && stageUsesIndex(child) {
		return true
	}
	if children, ok := stage["inputStages"].(bson.A); ok {
		for _, c := range children {
			if cm, ok := c.(bson.M); ok && stageUsesIndex(cm) {
				return true
			}
		}
	}
	return false
}

// pushdownSetup seeds an indexed base collection and a view whose pipeline leads
// with an equality $match on the indexed field.
func pushdownSetup(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "status", Value: "active"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "status", Value: "inactive"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "status", Value: "active"}},
	}); err != nil {
		return err
	}
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "status", Value: 1}}}); err != nil {
		return err
	}
	return col.Database().CreateView(ctx, "vw_active", col.Name(),
		mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "status", Value: "active"}}}}})
}

func explainFindUsesIndex(ctx context.Context, col *mongo.Collection, target string, filter bson.D) (interface{}, error) {
	var res bson.M
	err := col.Database().RunCommand(ctx, bson.D{
		{Key: "explain", Value: bson.D{{Key: "find", Value: target}, {Key: "filter", Value: filter}}},
		{Key: "verbosity", Value: "queryPlanner"},
	}).Decode(&res)
	if err != nil {
		return nil, err
	}
	return bson.D{{Key: "usesIndex", Value: planUsesIndex(res)}}, nil
}

// TestView_IndexPushdown_DirectBaseControl is the control: a direct find on the
// base collection with an eq predicate on the indexed field uses the index on
// both servers. This proves DumboDB's index pushdown works for direct reads --
// isolating the view-read gap below.
func TestView_IndexPushdown_DirectBaseControl(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_IndexPushdown_DirectBaseControl",
		Support: harness.DumboDBFull,
		Setup:   pushdownSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return explainFindUsesIndex(ctx, col, col.Name(), bson.D{{Key: "status", Value: "active"}})
		},
	})
}

// TestView_IndexPushdown_ViewRead verifies reading through a view whose pipeline
// leads with an eq $match on an indexed base field uses the index (IXSCAN) on
// both servers: MongoDB resolves the view to an aggregation over the base, and
// DumboDB pushes the view's leading $match to the base collection (workspace-jw1).
func TestView_IndexPushdown_ViewRead(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_IndexPushdown_ViewRead",
		Support: harness.DumboDBFull,
		Setup:   pushdownSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return explainFindUsesIndex(ctx, col, "vw_active", bson.D{})
		},
	})
}

// TestView_IndexPushdown_NestedView verifies the pushdown survives view chain
// flattening: a view over a view whose innermost leading stage is the indexed
// $match still uses the base index on both servers.
func TestView_IndexPushdown_NestedView(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "View_IndexPushdown_NestedView",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := pushdownSetup(ctx, col); err != nil {
				return err
			}
			// vw_active_sorted reads vw_active (whose leading stage is the indexed
			// $match) and appends a $sort; the resolved chain still leads with the
			// indexed $match.
			return col.Database().CreateView(ctx, "vw_active_sorted", "vw_active",
				mongo.Pipeline{{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return explainFindUsesIndex(ctx, col, "vw_active_sorted", bson.D{})
		},
	})
}
