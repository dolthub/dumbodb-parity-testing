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
		{"SharedDataDistribution", stage("$sharedDataDistribution", bson.D{})},
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
	for _, name := range []string{"$sharedDataDistribution", "$bogus"} {
		name := name
		harness.PairTest(t, harness.TestCase{
			Name:    "UnsupportedStage_DatabaseLevel_" + name[1:],
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				err := col.Database().RunCommand(ctx, bson.D{
					{Key: "aggregate", Value: 1},
					{Key: "pipeline", Value: bson.A{bson.D{{Key: name, Value: bson.D{}}}}},
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
