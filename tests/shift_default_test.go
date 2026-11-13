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

// TestShiftDefaultConstant: $shift's default is folded at parse time; one
// that reads a field or a variable, $$NOW included, fails with FailedToParse,
// a constant expression is evaluated, and $$REMOVE leaves the field out.
func TestShiftDefaultConstant(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "y", Value: int32(1)}, {Key: "s", Value: "old"}})
		return err
	}
	defaults := map[string]interface{}{
		"field":       "$y",
		"current":     "$$CURRENT",
		"now":         "$$NOW",
		"constant":    bson.D{{Key: "$add", Value: bson.A{int32(1), int32(2)}}},
		"reads_field": bson.D{{Key: "$add", Value: bson.A{"$y", int32(1)}}},
		"literal":     bson.D{{Key: "$literal", Value: "$y"}},
		"let":         bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "a", Value: int32(1)}}}, {Key: "in", Value: "$$a"}}}},
		"divide_zero": bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}},
		"null":        nil,
		"remove":      "$$REMOVE",
		"document":    bson.D{{Key: "a", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}}},
	}
	for name, def := range defaults {
		harness.PairTest(t, harness.TestCase{
			Name:    "ShiftDefaultConstant_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: pipelineRun(bson.A{bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
				{Key: "output", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$shift", Value: bson.D{
					{Key: "output", Value: "$y"}, {Key: "by", Value: int32(5)}, {Key: "default", Value: def}}}}}}}}}}}),
		})
	}
}
