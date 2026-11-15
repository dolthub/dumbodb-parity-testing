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

// TestSwitchConstantFalseFieldThen: mongod drops a $switch branch whose case
// is the constant false even when its then reads a field, and fails with
// 40069 when no branch or default remains, with or without documents.
func TestSwitchConstantFalseFieldThen(t *testing.T) {
	sw := bson.D{{Key: "$switch", Value: bson.D{{Key: "branches", Value: bson.A{bson.D{{Key: "case", Value: false}, {Key: "then", Value: "$v"}}}}}}}
	seeded := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: true}},
		})
		return err
	}
	empty := func(ctx context.Context, col *mongo.Collection) error {
		return col.Database().CreateCollection(ctx, col.Name())
	}
	project := bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: sw}}}}}
	pushGroup := bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: sw}}}}}}}
	matchExpr := bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: sw}}}}}
	find := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, bson.D{{Key: "$expr", Value: sw}})
		return drainCursor(ctx, cursor, err)
	}
	cases := []struct {
		name  string
		setup func(context.Context, *mongo.Collection) error
		run   func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"GroupPush", seeded, pipelineRun(pushGroup)},
		{"MatchExpr", seeded, pipelineRun(matchExpr)},
		{"FindExpr", seeded, find},
		{"EmptyProject", empty, pipelineRun(project)},
		{"EmptyGroupPush", empty, pipelineRun(pushGroup)},
		{"EmptyMatchExpr", empty, pipelineRun(matchExpr)},
		{"EmptyFindExpr", empty, find},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "SwitchConstantFalseFieldThen_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   c.setup,
			Run:     c.run,
		})
	}
}
