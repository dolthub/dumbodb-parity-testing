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

// Parity coverage for $replaceRoot and $replaceWith when used with the
// $$ROOT system variable, the most common Compass-shaped pattern.

package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func replaceRootRootSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "rr1"},
		{Key: "x", Value: int32(1)},
		{Key: "nested", Value: bson.D{{Key: "n", Value: int32(42)}}},
	})
	return err
}

func runReplaceRootCase(t *testing.T, name string, pipeline bson.A) {
	t.Helper()
	harness.PairTest(t, harness.TestCase{
		Name:    name,
		Support: harness.DumboDBFull,
		Setup:   replaceRootRootSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestReplaceRoot_NewRootIsRoot(t *testing.T) {
	runReplaceRootCase(t, "ReplaceRoot_NewRootIsRoot", bson.A{
		bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$$ROOT"}}}},
	})
}

func TestReplaceWith_Root(t *testing.T) {
	runReplaceRootCase(t, "ReplaceWith_Root", bson.A{
		bson.D{{Key: "$replaceWith", Value: "$$ROOT"}},
	})
}

func TestReplaceRoot_NewRootIsRootDotNested(t *testing.T) {
	runReplaceRootCase(t, "ReplaceRoot_NewRootIsRootDotNested", bson.A{
		bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$$ROOT.nested"}}}},
	})
}

func TestReplaceWith_RootDotNested(t *testing.T) {
	runReplaceRootCase(t, "ReplaceWith_RootDotNested", bson.A{
		bson.D{{Key: "$replaceWith", Value: "$$ROOT.nested"}},
	})
}

func TestReplaceRoot_MergeObjectsWithRoot(t *testing.T) {
	runReplaceRootCase(t, "ReplaceRoot_MergeObjectsWithRoot", bson.A{
		bson.D{{Key: "$replaceRoot", Value: bson.D{
			{Key: "newRoot", Value: bson.D{
				{Key: "$mergeObjects", Value: bson.A{"$$ROOT", bson.D{{Key: "extra", Value: int32(1)}}}},
			}},
		}}},
	})
}

func TestReplaceWith_MergeObjectsWithRoot(t *testing.T) {
	runReplaceRootCase(t, "ReplaceWith_MergeObjectsWithRoot", bson.A{
		bson.D{{Key: "$replaceWith", Value: bson.D{
			{Key: "$mergeObjects", Value: bson.A{"$$ROOT", bson.D{{Key: "extra", Value: int32(1)}}}},
		}}},
	})
}
