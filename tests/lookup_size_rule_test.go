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

// An equality $lookup that mongod runs in SBE refuses a match that would take
// one document's matches past 100MB (146). Elsewhere mongod runs it in the
// classic engine, which refuses a match only once the matches so far pass
// 100MB (4568), so the seed's 26 matches of 4.1MB pass.

func TestLookupSizeRule(t *testing.T) {
	sizeOfJ := bson.D{{Key: "$project", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$size", Value: "$j"}}}}}}
	byID := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}
	cases := []struct {
		name string
		run  func(ctx context.Context, col *mongo.Collection) (interface{}, error)
	}{
		{"AfterAddFields", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$addFields", Value: bson.D{{Key: "z", Value: int32(1)}}}},
				lookupLimitStage(col, nil), sizeOfJ, byID)
		}},
		{"AfterBucketAuto", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$bucketAuto", Value: bson.D{
				{Key: "groupBy", Value: "$_id"}, {Key: "buckets", Value: int32(5)},
				{Key: "output", Value: bson.D{{Key: "k", Value: bson.D{{Key: "$first", Value: "$k"}}}}},
			}}}, lookupLimitStage(col, nil), sizeOfJ, byID)
		}},
		{"InFacet", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{
				lookupLimitStage(col, nil), sizeOfJ, byID,
			}}}}})
		}},
		{"FromView", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			view := col.Name() + "_v"
			if err := db.CreateView(ctx, view, col.Name(), mongo.Pipeline{lookupLimitStage(col, nil)}); err != nil {
				return nil, err
			}
			defer db.Collection(view).Drop(ctx)
			return fieldOpsAgg(ctx, db.Collection(view), sizeOfJ, byID)
		}},
		{"AfterMatch", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$match", Value: bson.D{{Key: "k", Value: int32(1)}}}},
				lookupLimitStage(col, nil), sizeOfJ)
		}},
		{"BeforeFacet", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, lookupLimitStage(col, nil),
				bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{sizeOfJ}}}}})
		}},
		{"InUnionWith", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(-1)}}}},
				bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{
					lookupLimitStage(col, nil), sizeOfJ,
				}}}}})
		}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupSizeRule_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   lookupLimitSeed,
			Run:     c.run,
		})
	}
}
