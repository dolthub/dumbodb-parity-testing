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

// TestFacetBranchIsolation: each $facet branch sees the input documents as
// they were, whatever another branch's stages change, and the branches'
// outputs do not share documents a later stage changes.
func TestFacetBranchIsolation(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: bson.D{{Key: "q", Value: int32(1)}}}, {Key: "x", Value: int32(1)}, {Key: "t", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: bson.D{{Key: "q", Value: int32(2)}}}, {Key: "x", Value: int32(2)}, {Key: "t", Value: int32(3)}},
		})
		return err
	}
	byID := bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}
	pipelines := map[string]bson.A{
		"set_nested": {byID, bson.D{{Key: "$facet", Value: bson.D{
			{Key: "a", Value: bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "p.r", Value: "$x"}}}}}},
			{Key: "b", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}}}}}},
		}}}},
		"densify": {byID, bson.D{{Key: "$facet", Value: bson.D{
			{Key: "a", Value: bson.A{bson.D{{Key: "$densify", Value: bson.D{{Key: "field", Value: "t"}, {Key: "range", Value: bson.D{{Key: "step", Value: int32(1)}, {Key: "bounds", Value: "full"}}}}}}}},
			{Key: "b", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "t", Value: 1}}}}}},
		}}}},
		"three_branches": {byID, bson.D{{Key: "$facet", Value: bson.D{
			{Key: "a", Value: bson.A{bson.D{{Key: "$unset", Value: "p.q"}}}},
			{Key: "b", Value: bson.A{bson.D{{Key: "$addFields", Value: bson.D{{Key: "p.z", Value: int32(1)}}}}}},
			{Key: "c", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "p", Value: 1}}}}}},
		}}}},
		"outputs_then_set": {byID, bson.D{{Key: "$facet", Value: bson.D{
			{Key: "a", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}},
			{Key: "b", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}},
		}}}, bson.D{{Key: "$set", Value: bson.D{{Key: "a.p.w", Value: int32(5)}}}}},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "FacetBranchIsolation_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
