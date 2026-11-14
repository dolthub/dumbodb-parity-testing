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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestReplacementUpsertIDSubPaths: a replacement upsert through findAndModify
// fails with 111 when its filter has an equality on an _id sub-path and with
// 54 for an _id taken from {$in:[[5]]}; through findAndModify or update it
// inserts with a generated _id for a regex on an _id sub-path.
func TestReplacementUpsertIDSubPaths(t *testing.T) {
	filters := map[string]bson.D{
		"EqAndGt": {{Key: "_id.x", Value: bson.D{{Key: "$eq", Value: int32(1)}, {Key: "$gt", Value: int32(0)}}}},
		"AllOne":  {{Key: "_id.x", Value: bson.D{{Key: "$all", Value: bson.A{int32(1)}}}}},
		"Regex":   {{Key: "_id.x", Value: primitive.Regex{Pattern: "q"}}},
		"InArray": {{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{bson.A{int32(5)}}}}}},
	}
	replacement := bson.D{{Key: "b", Value: int32(1)}}
	contents := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "_id", Value: bson.D{{Key: "$type", Value: "$_id"}}}, {Key: "b", Value: 1}}))
		return drainCursor(ctx, cursor, err)
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "ReplacementUpsertIDSubPaths_Update_Regex",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.ReplaceOne(ctx, filters["Regex"], replacement, options.Replace().SetUpsert(true)); err != nil {
				return errOutcome(err), nil
			}
			return contents(ctx, col)
		},
	})
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "ReplacementUpsertIDSubPaths_FindAndModify_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				err := col.Database().RunCommand(ctx, bson.D{
					{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: filter},
					{Key: "update", Value: replacement}, {Key: "upsert", Value: true},
				}).Err()
				if err != nil {
					return errOutcome(err), nil
				}
				return contents(ctx, col)
			},
		})
	}
}
