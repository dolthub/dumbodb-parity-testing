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

// $documents runs only in a database-level aggregate or a sub-pipeline with
// no collection; on a collection mongod fails with InvalidNamespace where
// DumboDB reported NotImplemented (workspace-9m6).
func TestDocumentsStageContexts(t *testing.T) {
	docs := bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "x", Value: int32(1)}}, bson.D{{Key: "x", Value: int32(2)}}}}}
	pipelines := map[string]bson.A{
		"First":             {docs},
		"FirstThenMatch":    {docs, bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: int32(2)}}}}},
		"AfterMatch":        {bson.D{{Key: "$match", Value: bson.D{}}}, docs},
		"InFacet":           {bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{docs}}}}}},
		"InLookupNoFrom":    {bson.D{{Key: "$lookup", Value: bson.D{{Key: "pipeline", Value: bson.A{docs}}, {Key: "as", Value: "j"}}}}},
		"InLookupWithFrom":  {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "other"}, {Key: "pipeline", Value: bson.A{docs}}, {Key: "as", Value: "j"}}}}},
		"InUnionWithNoColl": {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "pipeline", Value: bson.A{docs}}}}}},
		"NotArray":          {bson.D{{Key: "$documents", Value: int32(1)}}},
		"LookupNoFromLet": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "let", Value: bson.D{{Key: "v", Value: "$_id"}}},
			{Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "y", Value: "$$v"}}, bson.D{{Key: "y", Value: int32(5)}}}}},
				bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$y", "$$v"}}}}}}},
			}},
			{Key: "as", Value: "j"},
		}}}},
		"LookupNoFromExprDocs": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.D{{Key: "$map", Value: bson.D{
				{Key: "input", Value: bson.A{int32(1), int32(2)}}, {Key: "in", Value: bson.D{{Key: "k", Value: "$$this"}}},
			}}}}}}},
			{Key: "as", Value: "j"},
		}}}},
		"LookupNoFromNotFirst":     {bson.D{{Key: "$lookup", Value: bson.D{{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}, docs}}, {Key: "as", Value: "j"}}}}},
		"LookupNoFromNoDocs":       {bson.D{{Key: "$lookup", Value: bson.D{{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}}, {Key: "as", Value: "j"}}}}},
		"LookupNoFromNoPipeline":   {bson.D{{Key: "$lookup", Value: bson.D{{Key: "localField", Value: "a"}, {Key: "foreignField", Value: "b"}, {Key: "as", Value: "j"}}}}},
		"LookupNoFromDocsNotArray": {bson.D{{Key: "$lookup", Value: bson.D{{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: int32(1)}}}}, {Key: "as", Value: "j"}}}}},
		"LookupNoFromLetExpression": {bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "let", Value: bson.D{{Key: "v", Value: bson.D{{Key: "$add", Value: bson.A{"$_id", int32(10)}}}}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "y", Value: "$$v"}}}}}}},
			{Key: "as", Value: "j"},
		}}}},
	}
	// $unionWith is not implemented yet (workspace-ed9).
	pending := map[string]bool{"InLookupNoFrom": true, "InUnionWithNoColl": true, "LookupNoFromLet": true, "LookupNoFromExprDocs": true, "LookupNoFromNotFirst": true, "LookupNoFromNoDocs": true, "LookupNoFromNoPipeline": true, "LookupNoFromDocsNotArray": true, "LookupNoFromLetExpression": true}
	for name, p := range pipelines {
		pipeline := p
		n := "DocumentsStageContexts_" + name
		support := harness.DumboDBFull
		if pending[name] {
			support = harness.DumboDBXFail
		}
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    n,
				Support: support,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Aggregate(ctx, pipeline)
					if err != nil {
						return errOutcome(err), nil
					}
					var out []bson.M
					if err := cur.All(ctx, &out); err != nil {
						return errOutcome(err), nil
					}
					for _, d := range out {
						delete(d, "_id")
					}
					return out, nil
				},
			})
		})
	}
}
