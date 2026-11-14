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

// TestGroupNValidatedWithoutInput: mongod rejects $firstN with n:0 (7548606)
// after an _id match that finds nothing, but returns no documents without
// error when the $group reads a missing collection directly, through a view
// or through $unionWith.
func TestGroupNValidatedWithoutInput(t *testing.T) {
	group := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: int32(0)}}}}}}}}
	matchID := func(id interface{}) bson.D { return bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: id}}}} }
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(2)}},
		})
		return err
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"MatchNullID", pipelineRun(bson.A{matchID(nil), group})},
		{"MatchMissingIDThenMatch", pipelineRun(bson.A{matchID(int32(99)), bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: int32(1)}}}}, group})},
		{"ViewOverMissingCollection", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: col.Name() + "_v"}, {Key: "viewOn", Value: col.Name() + "_missing"}, {Key: "pipeline", Value: bson.A{}}}).Err(); err != nil {
				return nil, err
			}
			cursor, err := db.Collection(col.Name()+"_v").Aggregate(ctx, bson.A{group})
			return drainCursor(ctx, cursor, err)
		}},
		{"UnionWithMissingCollection", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				matchID(int32(99)),
				bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name() + "_missing"}, {Key: "pipeline", Value: bson.A{group}}}}},
			})
			return drainCursor(ctx, cursor, err)
		}},
		{"AggregateMissingCollection", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Database().Collection(col.Name()+"_missing").Aggregate(ctx, bson.A{group})
			return drainCursor(ctx, cursor, err)
		}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "GroupNValidatedWithoutInput_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     c.run,
		})
	}
}
