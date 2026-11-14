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

// TestAggCollectionCollation: an aggregate without a collation uses the
// collection's default, so on a case-insensitive collection $match (indexed
// or not, with ranges beside $eq), $group, $addToSet and $sort compare
// strings case-insensitively; an explicit simple collation opts out.
func TestAggCollectionCollation(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		db := col.Database()
		name := col.Name()
		if err := col.Drop(ctx); err != nil {
			return err
		}
		opts := options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})
		if err := db.CreateCollection(ctx, name, opts); err != nil {
			return err
		}
		if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}}); err != nil {
			return err
		}
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: "B"}, {Key: "s", Value: "abc"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: "c"}, {Key: "s", Value: "ABC"}},
			bson.D{{Key: "_id", Value: "A"}, {Key: "a", Value: "x"}, {Key: "s", Value: "x"}},
		})
		return err
	}
	pipelines := map[string]bson.A{
		"match_eq_gt": {bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$eq", Value: "B"}, {Key: "$gt", Value: "a"}}}}}},
			bson.D{{Key: "$count", Value: "n"}}},
		"match_eq_gte": {bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$eq", Value: "b"}, {Key: "$gte", Value: "B"}}}}}},
			bson.D{{Key: "$count", Value: "n"}}},
		"match_id":   {bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: "a"}}}}},
		"match_in":   {bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A{"b", "C"}}}}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}},
		"group":      {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$s"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "n", Value: 1}}}}, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "n", Value: 1}}}}},
		"add_to_set": {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "u", Value: bson.D{{Key: "$addToSet", Value: "$s"}}}}}}, bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$u"}}}}}}},
		"sort":       {bson.D{{Key: "$sort", Value: bson.D{{Key: "a", Value: -1}}}}},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "AggCollectionCollation_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "AggCollectionCollation_simple_override",
		Support: harness.DumboDBFull,
		Setup:   setup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: "b"}}}}},
				options.Aggregate().SetCollation(&options.Collation{Locale: "simple"}))
			return drainCursor(ctx, cursor, err)
		},
	})
}
