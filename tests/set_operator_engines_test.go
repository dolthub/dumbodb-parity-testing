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

// TestSetOperatorEngines: with null, missing and non-array operands, the
// classic engine checks set operator operands in order ($setUnion and
// $setIntersection return null at the first nullish one and fail at the first
// non-array; $setEquals and $setIsSubset refuse a nullish operand), while SBE
// returns null when any operand is nullish.
func TestSetOperatorEngines(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "x"}, {Key: "n", Value: nil}, {Key: "a", Value: bson.A{int32(1)}}})
		return err
	}
	operands := [][2]string{{"$s", "$n"}, {"$n", "$s"}, {"$n", "$a"}, {"$a", "$n"}, {"$m", "$a"}, {"$a", "$m"}, {"$s", "$a"}, {"$a", "$s"}}
	for _, op := range []string{"$setUnion", "$setIntersection", "$setEquals", "$setDifference", "$setIsSubset"} {
		for _, args := range operands {
			expr := bson.D{{Key: op, Value: bson.A{args[0], args[1]}}}
			name := fmt.Sprintf("%s_%s_%s", op[1:], args[0][1:], args[1][1:])
			harness.PairTest(t, harness.TestCase{
				Name:    "SetOperatorEngines_classic_" + name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}}}),
			})
			harness.PairTest(t, harness.TestCase{
				Name:    "SetOperatorEngines_sbe_" + name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: pipelineRun(bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
					{Key: "r", Value: bson.D{{Key: "$first", Value: expr}}}}}}}),
			})
		}
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "SetOperatorEngines_classic_isSubset_constant_second",
		Support: harness.DumboDBFull,
		Setup:   setup,
		Run: pipelineRun(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0},
			{Key: "r", Value: bson.D{{Key: "$setIsSubset", Value: bson.A{"$s", bson.A{int32(1)}}}}}}}}}),
	})
}
