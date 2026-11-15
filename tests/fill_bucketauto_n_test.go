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

// TestFillPartitionByAndBucketAutoN: $fill's partitionBy must be an object or
// a string (TypeMismatch); a $bucketAuto N accumulator's n must be constant,
// and a $let with variables or $$NOW is not (4544714); $$NOW is not a
// constant $shift default either.
func TestFillPartitionByAndBucketAutoN(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}})
		return err
	}
	fill := func(partitionBy interface{}) bson.A {
		return bson.A{bson.D{{Key: "$fill", Value: bson.D{{Key: "partitionBy", Value: partitionBy},
			{Key: "output", Value: bson.D{{Key: "v", Value: bson.D{{Key: "value", Value: int32(1)}}}}}}}}}
	}
	bucketAuto := func(n interface{}) bson.A {
		return bson.A{bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "buckets", Value: int32(1)},
			{Key: "output", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$firstN", Value: bson.D{{Key: "input", Value: "$v"}, {Key: "n", Value: n}}}}}}}}}}}
	}
	pipelines := map[string]bson.A{
		"fill_int":           fill(int32(5)),
		"fill_bool":          fill(true),
		"fill_array":         fill(bson.A{int32(1)}),
		"fill_string":        fill("$v"),
		"bucket_auto_let":    bucketAuto(bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "x", Value: int32(2)}}}, {Key: "in", Value: "$$x"}}}}),
		"bucket_auto_now":    bucketAuto(bson.D{{Key: "$cond", Value: bson.A{"$$NOW", int32(1), int32(2)}}}),
		"bucket_auto_folded": bucketAuto(bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}),
		"shift_default_now": {bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$shift", Value: bson.D{{Key: "output", Value: "$v"}, {Key: "by", Value: int32(5)}, {Key: "default", Value: "$$NOW"}}}}}}}}}}},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "FillPartitionByAndBucketAutoN_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
