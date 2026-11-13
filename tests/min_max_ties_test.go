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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMinMaxTies: of values $min or $max finds equal, such as 1, 1.0 and
// NumberLong(1), SBE keeps the last and the classic engine the first.
func TestMinMaxTies(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "n", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "n", Value: 1.0}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "n", Value: int64(1)}},
		})
		return err
	}
	for _, order := range []int32{1, -1} {
		for engine, inhibit := range map[string]bool{"sbe": false, "classic": true} {
			pipeline := bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: order}}}}}
			if inhibit {
				pipeline = append(pipeline, bson.D{{Key: "$_internalInhibitOptimization", Value: bson.D{}}})
			}
			pipeline = append(pipeline,
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
					{Key: "mn", Value: bson.D{{Key: "$min", Value: "$n"}}}, {Key: "mx", Value: bson.D{{Key: "$max", Value: "$n"}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "mn", Value: bson.D{{Key: "$type", Value: "$mn"}}}, {Key: "mx", Value: bson.D{{Key: "$type", Value: "$mx"}}}}}},
			)
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("MinMaxTies_%s_%d", engine, order),
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(pipeline),
			})
		}
	}
}
