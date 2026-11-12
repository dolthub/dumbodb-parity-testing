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

// Stages a standalone mongod without Atlas Search refuses, and the errors it
// gives for them (dolthub/dumbodb#186).

func TestUnsupportedStageErrors(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
		return err
	}
	stage := func(name string, spec interface{}) bson.A { return bson.A{bson.D{{Key: name, Value: spec}}} }
	cases := []struct {
		name     string
		pipeline bson.A
	}{
		{"Search", stage("$search", bson.D{{Key: "text", Value: bson.D{{Key: "query", Value: "a"}, {Key: "path", Value: "x"}}}})},
		{"SearchMeta", stage("$searchMeta", bson.D{{Key: "text", Value: bson.D{{Key: "query", Value: "a"}, {Key: "path", Value: "x"}}}})},
		{"VectorSearch", stage("$vectorSearch", bson.D{{Key: "index", Value: "x"}, {Key: "path", Value: "v"}, {Key: "queryVector", Value: bson.A{1}}, {Key: "numCandidates", Value: 1}, {Key: "limit", Value: 1}})},
		{"ListSearchIndexes", stage("$listSearchIndexes", bson.D{})},
		{"SearchMetaAfterMatch", bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, bson.D{{Key: "$searchMeta", Value: bson.D{}}}}},
		{"SearchMetaInFacet", stage("$facet", bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "$searchMeta", Value: bson.D{}}}}}})},
		{"ShardedDataDistribution", stage("$shardedDataDistribution", bson.D{})},
		{"ShardedDataDistributionAfterMatch", bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, bson.D{{Key: "$shardedDataDistribution", Value: bson.D{}}}}},
		{"ShardedDataDistributionInFacet", stage("$facet", bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "$shardedDataDistribution", Value: bson.D{}}}}}})},
		{"QueryStatsOnCollection", stage("$queryStats", bson.D{})},
		{"QueryStatsInUnionWith", stage("$unionWith", bson.D{{Key: "coll", Value: "x"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$queryStats", Value: bson.D{}}}}}})},
		{"ListCatalogNotFirst", bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, bson.D{{Key: "$listCatalog", Value: bson.D{}}}}},
		{"ListCatalogInFacet", stage("$facet", bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "$listCatalog", Value: bson.D{}}}}}})},
		{"ChangeStreamInFacet", stage("$facet", bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "$changeStream", Value: bson.D{}}}}}})},
		{"ChangeStreamInLookupNoInput", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(-1)}}}},
			bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "x"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$changeStream", Value: bson.D{}}}}}, {Key: "as", Value: "j"}}}},
		}},
		{"CurrentOpOnCollection", stage("$currentOp", bson.D{})},
		{"ListSessionsOnCollection", stage("$listSessions", bson.D{})},
		{"ListLocalSessionsOnCollection", stage("$listLocalSessions", bson.D{})},
		{"ChangeStream", stage("$changeStream", bson.D{})},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "UnsupportedStage_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run:     pipelineRun(c.pipeline),
		})
	}
}

func TestUnsupportedStageErrors_DatabaseLevel(t *testing.T) {
	documents := bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "a", Value: int32(1)}}}}}
	cases := []struct {
		name     string
		pipeline bson.A
	}{
		{"ShardedDataDistribution", bson.A{bson.D{{Key: "$shardedDataDistribution", Value: bson.D{}}}}},
		{"Bogus", bson.A{bson.D{{Key: "$bogus", Value: bson.D{}}}}},
		{"ChangeStream", bson.A{bson.D{{Key: "$changeStream", Value: bson.D{}}}}},
		{"ListSessions", bson.A{bson.D{{Key: "$listSessions", Value: bson.D{}}}}},
		{"Search", bson.A{bson.D{{Key: "$search", Value: bson.D{}}}}},
		{"SearchInFacet", bson.A{bson.D{{Key: "$facet", Value: bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "$search", Value: bson.D{}}}}}}}}}},
		{"QueryStats", bson.A{bson.D{{Key: "$queryStats", Value: bson.D{}}}}},
		{"ListCatalog", bson.A{bson.D{{Key: "$listCatalog", Value: bson.D{}}}}},
		{"DocumentsThenShardedDataDistribution", bson.A{documents, bson.D{{Key: "$shardedDataDistribution", Value: bson.D{}}}}},
		{"DocumentsThenQueryStats", bson.A{documents, bson.D{{Key: "$queryStats", Value: bson.D{}}}}},
		{"DocumentsThenListCatalog", bson.A{documents, bson.D{{Key: "$listCatalog", Value: bson.D{}}}}},
	}
	for _, c := range cases {
		pipeline := c.pipeline
		harness.PairTest(t, harness.TestCase{
			Name:    "UnsupportedStage_DatabaseLevel_" + c.name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				err := col.Database().RunCommand(ctx, bson.D{
					{Key: "aggregate", Value: 1},
					{Key: "pipeline", Value: pipeline},
					{Key: "cursor", Value: bson.D{}},
				}).Err()
				return nil, err
			},
		})
	}
}

// Stages DumboDB builds outside the generic stage table must still be named
// as not allowed in an update pipeline.
func TestUnsupportedStageErrors_UpdatePipeline(t *testing.T) {
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"Lookup", bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "c"}, {Key: "pipeline", Value: bson.A{}}, {Key: "as", Value: "j"}}}}},
		{"UnionWith", bson.D{{Key: "$unionWith", Value: "c"}}},
		{"GraphLookup", bson.D{{Key: "$graphLookup", Value: bson.D{{Key: "from", Value: "c"}, {Key: "startWith", Value: "$x"}, {Key: "connectFromField", Value: "a"}, {Key: "connectToField", Value: "b"}, {Key: "as", Value: "g"}}}}},
		{"Out", bson.D{{Key: "$out", Value: "x"}}},
		{"Merge", bson.D{{Key: "$merge", Value: "x"}}},
		{"IndexStats", bson.D{{Key: "$indexStats", Value: bson.D{}}}},
		{"Unknown", bson.D{{Key: "$bogus", Value: bson.D{}}}},
		{"ShardedDataDistribution", bson.D{{Key: "$shardedDataDistribution", Value: bson.D{}}}},
		{"Search", bson.D{{Key: "$search", Value: bson.D{}}}},
		{"SearchMeta", bson.D{{Key: "$searchMeta", Value: bson.D{}}}},
		{"VectorSearch", bson.D{{Key: "$vectorSearch", Value: bson.D{}}}},
		{"ChangeStream", bson.D{{Key: "$changeStream", Value: bson.D{}}}},
		{"CurrentOp", bson.D{{Key: "$currentOp", Value: bson.D{}}}},
		{"QueryStats", bson.D{{Key: "$queryStats", Value: bson.D{}}}},
		{"ListCatalog", bson.D{{Key: "$listCatalog", Value: bson.D{}}}},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    "UnsupportedStage_UpdatePipeline_" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				_, err := col.UpdateOne(ctx, bson.D{}, mongo.Pipeline{stage})
				return nil, err
			},
		})
	}
}

// mongod parses a view's pipeline when the view is defined, and refuses stages
// that write to disk.
func TestUnsupportedStageErrors_ViewDefinition(t *testing.T) {
	cases := []struct {
		name     string
		collMod  bool
		pipeline bson.A
	}{
		{"Out", false, bson.A{bson.D{{Key: "$out", Value: "x"}}}},
		{"MergeAfterMatch", false, bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, bson.D{{Key: "$merge", Value: "x"}}}},
		{"ChangeStream", false, bson.A{bson.D{{Key: "$changeStream", Value: bson.D{}}}}},
		{"ShardedDataDistribution", false, bson.A{bson.D{{Key: "$shardedDataDistribution", Value: bson.D{}}}}},
		{"CollModOut", true, bson.A{bson.D{{Key: "$out", Value: "x"}}}},
	}
	for _, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    "UnsupportedStage_ViewDefinition_" + c.name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				view := col.Name() + "_v"
				defer db.Collection(view).Drop(ctx)
				if c.collMod {
					if err := db.CreateView(ctx, view, col.Name(), mongo.Pipeline{}); err != nil {
						return nil, err
					}
					return nil, db.RunCommand(ctx, bson.D{{Key: "collMod", Value: view}, {Key: "pipeline", Value: c.pipeline}}).Err()
				}
				return nil, db.RunCommand(ctx, bson.D{{Key: "create", Value: view}, {Key: "viewOn", Value: col.Name()}, {Key: "pipeline", Value: c.pipeline}}).Err()
			},
		})
	}
}

// A view may contain $unionWith and $facet.
func TestUnsupportedStageErrors_ViewWithUnionWith(t *testing.T) {
	for name, stage := range map[string]bson.D{
		"UnionWith": {{Key: "$unionWith", Value: "other"}},
		"Facet":     {{Key: "$facet", Value: bson.D{{Key: "n", Value: bson.A{bson.D{{Key: "$count", Value: "c"}}}}}}},
	} {
		stage := stage
		harness.PairTest(t, harness.TestCase{
			Name:    "View_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
					return err
				}
				_, err := col.Database().Collection("other").InsertOne(ctx, bson.D{{Key: "_id", Value: int32(2)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				view := col.Name() + "_v"
				if err := db.CreateView(ctx, view, col.Name(), mongo.Pipeline{stage}); err != nil {
					return nil, err
				}
				defer db.Collection(view).Drop(ctx)
				defer db.Collection("other").Drop(ctx)
				agg, err := fieldOpsAgg(ctx, db.Collection(view), bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
				if err != nil {
					return nil, err
				}
				n, err := db.Collection(view).CountDocuments(ctx, bson.D{})
				return bson.D{{Key: "agg", Value: agg}, {Key: "count", Value: n}}, err
			},
		})
	}
}
