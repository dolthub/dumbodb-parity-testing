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

// TestCollStatsOptions: $collStats count, queryExecStats and storageStats
// must be objects (TypeMismatch otherwise), null means absent, and count and
// queryExecStats must be empty (31170), at the top level and in $unionWith.
func TestCollStatsOptions(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
		return err
	}
	keys := bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "k", Value: bson.D{{Key: "$map", Value: bson.D{
		{Key: "input", Value: bson.D{{Key: "$objectToArray", Value: "$$ROOT"}}}, {Key: "in", Value: "$$this.k"}}}}}}}}
	specs := map[string]bson.D{
		"storage_null":    {{Key: "storageStats", Value: nil}},
		"storage_int":     {{Key: "storageStats", Value: int32(1)}},
		"storage_string":  {{Key: "storageStats", Value: "x"}},
		"storage_array":   {{Key: "storageStats", Value: bson.A{}}},
		"count_int":       {{Key: "count", Value: int32(1)}},
		"count_string":    {{Key: "count", Value: "x"}},
		"count_null":      {{Key: "count", Value: nil}},
		"count_nonempty":  {{Key: "count", Value: bson.D{{Key: "zz", Value: int32(1)}}}},
		"exec_int":        {{Key: "queryExecStats", Value: int32(1)}},
		"exec_null":       {{Key: "queryExecStats", Value: nil}},
		"exec_nonempty":   {{Key: "queryExecStats", Value: bson.D{{Key: "zz", Value: int32(1)}}}},
		"storage_scale_0": {{Key: "storageStats", Value: bson.D{{Key: "scale", Value: nil}}}},
	}
	for name, spec := range specs {
		harness.PairTest(t, harness.TestCase{
			Name:    "CollStatsOptions_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(bson.A{bson.D{{Key: "$collStats", Value: spec}}, keys}),
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "CollStatsOptions_union_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()},
					{Key: "pipeline", Value: bson.A{bson.D{{Key: "$collStats", Value: spec}}, keys}}}}}})
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
