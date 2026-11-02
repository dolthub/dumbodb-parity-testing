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

// Every stage that reads another collection, inside every stage that runs a
// sub-pipeline. $lookup built its sub-pipeline stages separately from $facet
// and left out $graphLookup (workspace-7v6).

func subPipelineSetup(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "name", Value: "c"}}); err != nil {
		return err
	}
	_, err := col.Database().Collection("emp").InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "name", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "name", Value: "b"}, {Key: "boss", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "name", Value: "c"}, {Key: "boss", Value: "b"}},
	})
	return err
}

func TestSubPipelineForeignStages(t *testing.T) {
	size := func(field string) bson.D {
		return bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$" + field}}}}}}
	}
	inner := map[string]bson.A{
		"GraphLookup": {
			bson.D{{Key: "$graphLookup", Value: bson.D{
				{Key: "from", Value: "emp"}, {Key: "startWith", Value: "$boss"},
				{Key: "connectFromField", Value: "boss"}, {Key: "connectToField", Value: "name"}, {Key: "as", Value: "chain"},
			}}},
			size("chain"),
		},
		"Lookup": {
			bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "emp"}, {Key: "localField", Value: "boss"}, {Key: "foreignField", Value: "name"}, {Key: "as", Value: "b"}}}},
			size("b"),
		},
		"UnionWith": {
			bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: "emp"}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "name", Value: 1}}}},
		},
		"Facet": {
			bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{bson.D{{Key: "$count", Value: "n"}}}}}}},
		},
	}
	matchC := bson.D{{Key: "$match", Value: bson.D{{Key: "name", Value: "c"}}}}
	for innerName, stages := range inner {
		sub := append(bson.A{matchC}, stages...)
		outer := map[string]bson.A{
			"Lookup":    {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "emp"}, {Key: "pipeline", Value: sub}, {Key: "as", Value: "j"}}}}},
			"UnionWith": {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: "emp"}, {Key: "pipeline", Value: sub}}}}},
			"Facet": {
				bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "emp"}, {Key: "localField", Value: "name"}, {Key: "foreignField", Value: "name"}, {Key: "as", Value: "e"}}}},
				bson.D{{Key: "$unwind", Value: "$e"}},
				bson.D{{Key: "$replaceWith", Value: "$e"}},
				bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: sub}}}},
			},
		}
		for outerName, pipeline := range outer {
			harness.PairTest(t, harness.TestCase{
				Name:    "SubPipelineForeignStages_" + outerName + "_" + innerName,
				Support: harness.DumboDBFull,
				Setup:   subPipelineSetup,
				Run:     round6Aggregate(pipeline),
			})
		}
	}
}

// Stages mongod refuses inside $facet (workspace-vc6).
func TestFacetDisallowedStages(t *testing.T) {
	stages := map[string]bson.D{
		"Facet":      {{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{}}}}},
		"Out":        {{Key: "$out", Value: "o"}},
		"Merge":      {{Key: "$merge", Value: "o"}},
		"CollStats":  {{Key: "$collStats", Value: bson.D{}}},
		"IndexStats": {{Key: "$indexStats", Value: bson.D{}}},
		"GeoNear":    {{Key: "$geoNear", Value: bson.D{{Key: "near", Value: bson.A{0, 0}}, {Key: "distanceField", Value: "d"}}}},
		"Documents":  {{Key: "$documents", Value: bson.A{}}},
	}
	for name, stage := range stages {
		harness.PairTest(t, harness.TestCase{
			Name:    "FacetDisallowedStages_" + name,
			Support: harness.DumboDBFull,
			Setup:   subPipelineSetup,
			Run:     round6Aggregate(bson.A{bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{stage}}}}}}),
		})
	}
}
