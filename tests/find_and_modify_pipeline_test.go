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

// findAndModify accepts a pipeline-style update; DumboDB returned
// NotImplemented (workspace-d40, dolthub/dumbodb#120).
func TestFindAndModifyPipeline(t *testing.T) {
	set := func(fields bson.D) bson.D { return bson.D{{Key: "$set", Value: fields}} }
	byID := func(id int32) bson.E { return bson.E{Key: "query", Value: bson.D{{Key: "_id", Value: id}}} }
	incN := set(bson.D{{Key: "n", Value: bson.D{{Key: "$add", Value: bson.A{"$n", int32(1)}}}}})
	cases := []struct {
		name string
		cmd  bson.D
	}{
		{"SetOld", bson.D{byID(1), {Key: "update", Value: bson.A{incN}}}},
		{"SetNew", bson.D{byID(1), {Key: "update", Value: bson.A{incN}}, {Key: "new", Value: true}}},
		{"TwoStages", bson.D{byID(1), {Key: "update", Value: bson.A{incN, set(bson.D{{Key: "m", Value: bson.D{{Key: "$multiply", Value: bson.A{"$n", int32(10)}}}}})}}, {Key: "new", Value: true}}},
		{"Unset", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$unset", Value: "s"}}}}, {Key: "new", Value: true}}},
		{"ReplaceWith", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "only", Value: "$s"}}}}}}, {Key: "new", Value: true}}},
		{"Project", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: int32(1)}}}}}}, {Key: "new", Value: true}}},
		{"UpsertNew", bson.D{byID(7), {Key: "update", Value: bson.A{set(bson.D{{Key: "n", Value: int32(5)}})}}, {Key: "upsert", Value: true}, {Key: "new", Value: true}}},
		{"UpsertOld", bson.D{byID(7), {Key: "update", Value: bson.A{set(bson.D{{Key: "n", Value: int32(5)}})}}, {Key: "upsert", Value: true}}},
		{"NoMatch", bson.D{byID(99), {Key: "update", Value: bson.A{incN}}}},
		{"Fields", bson.D{byID(1), {Key: "update", Value: bson.A{incN}}, {Key: "new", Value: true}, {Key: "fields", Value: bson.D{{Key: "n", Value: int32(1)}}}}},
		{"Empty", bson.D{byID(1), {Key: "update", Value: bson.A{}}, {Key: "new", Value: true}}},
		{"UnknownStage", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}}}},
		{"NonObjectStage", bson.D{byID(1), {Key: "update", Value: bson.A{int32(1)}}}},
		{"UnknownOperator", bson.D{byID(1), {Key: "update", Value: bson.A{set(bson.D{{Key: "x", Value: bson.D{{Key: "$bogus", Value: int32(1)}}}})}}}},
		{"WithRemove", bson.D{byID(1), {Key: "update", Value: bson.A{incN}}, {Key: "remove", Value: true}}},
		{"ReplaceWithNewID", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "_id", Value: int32(2)}}}}}}, {Key: "new", Value: true}}},
		{"UnsetID", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$unset", Value: "_id"}}}}, {Key: "new", Value: true}}},
		{"SetID", bson.D{byID(1), {Key: "update", Value: bson.A{set(bson.D{{Key: "_id", Value: int32(3)}})}}, {Key: "new", Value: true}}},
		{"ProjectWithoutID", bson.D{byID(1), {Key: "update", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "n", Value: int32(1)}}}}}}, {Key: "new", Value: true}}},
	}
	for _, tc := range cases {
		cmd := tc.cmd
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "FindAndModifyPipeline_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertFindAndModifyDoc,
				Run:     findAndModifyRun(cmd),
			})
		})
	}
}

// The update command shares the pipeline code; _id is kept when a stage
// drops it (workspace-2ph).
func TestUpdatePipelineKeepsID(t *testing.T) {
	stages := map[string]bson.A{
		"ReplaceWith":      {bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "only", Value: "$s"}}}}},
		"ReplaceWithNewID": {bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "_id", Value: int32(2)}}}}},
		"UnsetID":          {bson.D{{Key: "$unset", Value: "_id"}}},
		"ProjectWithoutID": {bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}, {Key: "n", Value: int32(1)}}}}},
	}
	for name, p := range stages {
		pipeline := p
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "UpdatePipelineKeepsID_" + name,
				Support: harness.DumboDBFull,
				Setup:   insertFindAndModifyDoc,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, pipeline)
					var docs []bson.M
					cur, ferr := col.Find(ctx, bson.D{})
					if ferr != nil {
						return nil, ferr
					}
					if ferr = cur.All(ctx, &docs); ferr != nil {
						return nil, ferr
					}
					return bson.M{"outcome": errOutcome(err), "after": docs}, nil
				},
			})
		})
	}
}

