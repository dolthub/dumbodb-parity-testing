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
	}
	// Collectionless sub-pipelines are not supported yet (workspace-yuv).
	pending := map[string]bool{"InLookupNoFrom": true, "InUnionWithNoColl": true}
	for name, p := range pipelines {
		pipeline := p
		n := "DocumentsStageContexts_" + name
		support := harness.DumboDBXFail
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
