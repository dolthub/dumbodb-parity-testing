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

// $collStats can start a $unionWith or $lookup sub-pipeline, and only start
// it (dolthub/dumbodb#239).

func TestCollStatsSubPipeline(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: int32(1)}}, bson.D{{Key: "_id", Value: int32(2)}}})
		return err
	}
	count := bson.D{{Key: "$collStats", Value: bson.D{{Key: "count", Value: bson.D{}}}}}
	keep := bson.D{{Key: "$project", Value: bson.D{{Key: "count", Value: 1}, {Key: "ns", Value: 1}}}}
	cases := []struct {
		name   string
		stages func(col *mongo.Collection) []bson.D
	}{
		{"UnionWith", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{count, keep}}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}
		}},
		{"Lookup", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{count, keep}}, {Key: "as", Value: "j"}}}},
				{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}
		}},
		{"UnionWithNotFirst", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, count}}}}}}
		}},
		{"LookupNotFirst", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$limit", Value: int32(1)}}, count}}, {Key: "as", Value: "j"}}}}}
		}},
		{"MissingCollection", func(col *mongo.Collection) []bson.D {
			return []bson.D{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name() + "_none"}, {Key: "pipeline", Value: bson.A{count}}}}}}
		}},
	}
	for _, c := range cases {
		build := c.stages
		harness.PairTest(t, harness.TestCase{
			Name:    "CollStatsSubPipeline_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, build(col)...)
			},
		})
	}
}

// A sub-pipeline over a view reads $collStats through a view without stages
// to its base collection; a view's stages come first, so $collStats is no
// longer first (40602). A top-level $collStats on a view is refused (166).
// find, count and distinct on a view report the database in ns.
func TestCollStatsViews(t *testing.T) {
	count := bson.D{{Key: "$collStats", Value: bson.D{{Key: "count", Value: bson.D{}}}}}
	storage := bson.D{{Key: "$collStats", Value: bson.D{{Key: "storageStats", Value: bson.D{}}}}}
	keep := bson.D{{Key: "$project", Value: bson.D{{Key: "count", Value: 1}, {Key: "ns", Value: 1}, {Key: "c", Value: "$storageStats.count"}}}}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		db := col.Database()
		base := col.Name() + "_base"
		if _, err := db.Collection(base).InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: int32(1)}}, bson.D{{Key: "_id", Value: int32(2)}}}); err != nil {
			return err
		}
		if err := db.CreateView(ctx, col.Name()+"_staged", base, mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}}}); err != nil {
			return err
		}
		if err := db.CreateView(ctx, col.Name()+"_plain", base, mongo.Pipeline{}); err != nil {
			return err
		}
		if err := db.CreateView(ctx, col.Name()+"_plain2", col.Name()+"_plain", mongo.Pipeline{}); err != nil {
			return err
		}
		if err := db.CreateView(ctx, col.Name()+"_stats", col.Name(), mongo.Pipeline{{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: base}, {Key: "pipeline", Value: bson.A{count}}}}}}); err != nil {
			return err
		}
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(9)}})
		return err
	}
	cleanup := func(ctx context.Context, col *mongo.Collection) {
		for _, s := range []string{"_stats", "_plain2", "_plain", "_staged", "_base"} {
			_ = col.Database().Collection(col.Name() + s).Drop(ctx)
		}
	}
	union := func(view string, stats bson.D) func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			defer cleanup(ctx, col)
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name() + view}, {Key: "pipeline", Value: bson.A{stats, keep}}}}})
		}
	}
	cases := map[string]func(ctx context.Context, col *mongo.Collection) (interface{}, error){
		"UnionWithStagedView": union("_staged", count),
		"UnionWithPlainView":  union("_plain", count),
		"UnionWithViewChain":  union("_plain2", storage),
		"LookupStagedView": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			defer cleanup(ctx, col)
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name() + "_staged"}, {Key: "pipeline", Value: bson.A{count}}, {Key: "as", Value: "j"}}}})
		},
		"TopLevelPlainView": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			defer cleanup(ctx, col)
			return fieldOpsAgg(ctx, col.Database().Collection(col.Name()+"_plain"), count)
		},
		"TopLevelStagedView": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			defer cleanup(ctx, col)
			return fieldOpsAgg(ctx, col.Database().Collection(col.Name()+"_staged"), count)
		},
		"FindOnView": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			defer cleanup(ctx, col)
			return findWithProjection(ctx, col.Database().Collection(col.Name()+"_stats"), bson.D{}, bson.D{{Key: "ns", Value: 1}, {Key: "count", Value: 1}})
		},
	}
	for name, run := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "CollStatsViews_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     run,
		})
	}
}
