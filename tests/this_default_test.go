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

// Parity coverage for $$this as the *default* variable name in $filter and
// $map when the `as` option is omitted. Pre-existing $filter/$map tests in
// this suite all set an explicit `as` (e.g. "$$n", "$$t"); none exercised
// the implicit-$$this path that MongoDB uses when `as` is absent.

package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func thisDefaultSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "td1"},
		{Key: "nums", Value: bson.A{int32(1), int32(2), int32(3), int32(4), int32(5)}},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "n", Value: int32(1)}, {Key: "v", Value: "a"}},
			bson.D{{Key: "n", Value: int32(2)}, {Key: "v", Value: "b"}},
		}},
	})
	return err
}

func runThisDefaultCase(t *testing.T, name string, projection bson.D) {
	t.Helper()
	harness.PairTest(t, harness.TestCase{
		Name:    name,
		Support: harness.DumboDBFull,
		Setup:   thisDefaultSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: projection}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestThisDefault_FilterNoAs(t *testing.T) {
	runThisDefaultCase(t, "ThisDefault_FilterNoAs", bson.D{
		{Key: "result", Value: bson.D{{Key: "$filter", Value: bson.D{
			{Key: "input", Value: "$nums"},
			{Key: "cond", Value: bson.D{{Key: "$gt", Value: bson.A{"$$this", int32(2)}}}},
		}}}},
		{Key: "_id", Value: int32(0)},
	})
}

func TestThisDefault_MapNoAs(t *testing.T) {
	runThisDefaultCase(t, "ThisDefault_MapNoAs", bson.D{
		{Key: "result", Value: bson.D{{Key: "$map", Value: bson.D{
			{Key: "input", Value: "$nums"},
			{Key: "in", Value: bson.D{{Key: "$multiply", Value: bson.A{"$$this", int32(10)}}}},
		}}}},
		{Key: "_id", Value: int32(0)},
	})
}

// $$this can appear as the entire `in` expression (no transformation).
func TestThisDefault_MapInIsThisAlone(t *testing.T) {
	runThisDefaultCase(t, "ThisDefault_MapInIsThisAlone", bson.D{
		{Key: "result", Value: bson.D{{Key: "$map", Value: bson.D{
			{Key: "input", Value: "$nums"},
			{Key: "in", Value: "$$this"},
		}}}},
		{Key: "_id", Value: int32(0)},
	})
}

// $$this.field traverses into the array element when it's a sub-document.
func TestThisDefault_MapThisDotField(t *testing.T) {
	runThisDefaultCase(t, "ThisDefault_MapThisDotField", bson.D{
		{Key: "result", Value: bson.D{{Key: "$map", Value: bson.D{
			{Key: "input", Value: "$items"},
			{Key: "in", Value: "$$this.v"},
		}}}},
		{Key: "_id", Value: int32(0)},
	})
}

func TestThisDefault_FilterThisDotField(t *testing.T) {
	runThisDefaultCase(t, "ThisDefault_FilterThisDotField", bson.D{
		{Key: "result", Value: bson.D{{Key: "$filter", Value: bson.D{
			{Key: "input", Value: "$items"},
			{Key: "cond", Value: bson.D{{Key: "$gt", Value: bson.A{"$$this.n", int32(1)}}}},
		}}}},
		{Key: "_id", Value: int32(0)},
	})
}

// Nested $map: the inner $$this must shadow the outer one. Verifies the
// variable-binding stack works correctly for the default name.
func TestThisDefault_NestedMapShadow(t *testing.T) {
	runThisDefaultCase(t, "ThisDefault_NestedMapShadow", bson.D{
		{Key: "result", Value: bson.D{{Key: "$map", Value: bson.D{
			{Key: "input", Value: "$nums"},
			{Key: "in", Value: bson.D{{Key: "$map", Value: bson.D{
				{Key: "input", Value: bson.A{int32(10), int32(20)}},
				{Key: "in", Value: bson.D{{Key: "$add", Value: bson.A{"$$this", int32(0)}}}},
			}}}},
		}}}},
		{Key: "_id", Value: int32(0)},
	})
}
