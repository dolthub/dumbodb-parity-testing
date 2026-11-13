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

// TestReverseArrayCodes: a non-array $reverseArray argument fails with 34435
// in the classic engine, at parse when it is constant, and 7158002 in SBE.
func TestReverseArrayCodes(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "x"}, {Key: "n", Value: nil}})
		return err
	}
	pipelines := map[string]bson.A{
		"field":    {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$reverseArray", Value: "$s"}}}}}}},
		"constant": {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$reverseArray", Value: "x"}}}}}}},
		"null":     {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$reverseArray", Value: "$n"}}}}}}},
		"sbe":      {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$first", Value: bson.D{{Key: "$reverseArray", Value: "$s"}}}}}}}}},
	}
	for name, pipeline := range pipelines {
		harness.PairTest(t, harness.TestCase{
			Name:    "ReverseArrayCodes_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
