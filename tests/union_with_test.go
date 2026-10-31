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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $unionWith was not implemented (workspace-ed9).
func TestUnionWith(t *testing.T) {
	other := "other"
	docs := bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "d", Value: int32(1)}}}}}
	pipelines := map[string]bson.A{
		"CollName":       {bson.D{{Key: "$unionWith", Value: other}}},
		"CollSpec":       {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: other}}}}},
		"CollPipeline":   {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: other}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "o", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}}}}}}}},
		"Missing":        {bson.D{{Key: "$unionWith", Value: "nope"}}},
		"Self":           {bson.D{{Key: "$unionWith", Value: "col"}}},
		"Documents":      {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "pipeline", Value: bson.A{docs}}}}}},
		"ThenSort":       {bson.D{{Key: "$unionWith", Value: other}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "o", Value: -1}, {Key: "_id", Value: 1}}}}, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "order", Value: bson.D{{Key: "$push", Value: "$_id"}}}}}}},
		"AfterMatch":     {bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}}, bson.D{{Key: "$unionWith", Value: other}}},
		"Nested":         {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: other}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$unionWith", Value: "col"}}}}}}}},
		"InFacet":        {bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{bson.D{{Key: "$unionWith", Value: other}}}}}}}},
		"NotString":      {bson.D{{Key: "$unionWith", Value: int32(1)}}},
		"UnknownField":   {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: other}, {Key: "x", Value: int32(1)}}}}},
		"NoCollNoDocs":   {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "pipeline", Value: bson.A{}}}}}},
		"PipelineNotArr": {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: other}, {Key: "pipeline", Value: int32(1)}}}}},
		"PipelineOut":    {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: other}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$out", Value: "x"}}}}}}}},
		"CollNotString":  {bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: int32(1)}}}}},
	}
	for name, p := range pipelines {
		pipeline := p
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "UnionWith_" + name,
				Support: harness.DumboDBXFail,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: int32(1)}, {Key: "c", Value: "x"}}, bson.D{{Key: "_id", Value: int32(2)}}}); err != nil {
						return err
					}
					_, err := col.Database().Collection(other).InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: int32(10)}, {Key: "o", Value: int32(1)}},
						bson.D{{Key: "_id", Value: int32(11)}, {Key: "o", Value: int32(2)}},
					})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Aggregate(ctx, pipeline)
					if err != nil {
						return errOutcome(err), nil
					}
					var out []bson.Raw
					if err := cur.All(ctx, &out); err != nil {
						return errOutcome(err), nil
					}
					// Natural order is unspecified; compare as a multiset.
					strs := make([]string, len(out))
					for i, d := range out {
						strs[i] = d.String()
					}
					sort.Strings(strs)
					return strs, nil
				},
			})
		})
	}
}
