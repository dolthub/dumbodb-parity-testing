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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestExpressionPathNul: an expression field path whose field name holds a
// null byte is refused, with the field names checked in order.
func TestExpressionPathNul(t *testing.T) {
	pipelines := map[string]string{
		"project":         `[{"$project":{"a":"$p\u0000q"}}]`,
		"project_current": `[{"$project":{"a":"$$CURRENT.p\u0000q"}}]`,
		"before_dollar":   `[{"$project":{"a":"$p\u0000q.$x"}}]`,
		"after_dollar":    `[{"$project":{"a":"$x.$p\u0000q"}}]`,
		"after_empty":     `[{"$project":{"a":"$x..p\u0000q"}}]`,
		"group_id":        `[{"$group":{"_id":"$p\u0000q"}}]`,
		"match_expr":      `[{"$match":{"$expr":{"$eq":["$p\u0000q",1]}}}]`,
	}
	for name, pipeline := range pipelines {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ExpressionPathNul_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
				return err
			},
			Run: pipelineRun(p),
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "ExpressionPathNul_find_projection",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "a", Value: "$p\x00q"}}))
			return drainCursor(ctx, cursor, err)
		},
	})
}
