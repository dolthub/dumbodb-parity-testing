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

// TestFilterLimit: $filter's limit is an expression that stops after that
// many matches; null or missing means no limit, anything but a positive
// 32-bit integer fails (327391, 327392), and an empty or null input returns
// before the limit is read. A non-array input fails with 28651 and an unknown
// parameter with 28647.
func TestFilterLimit(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: bson.A{int32(1), int32(2), int32(3)}},
			{Key: "e", Value: bson.A{}}, {Key: "n", Value: nil}, {Key: "s", Value: "x"}, {Key: "two", Value: int32(2)}})
		return err
	}
	limits := []interface{}{int32(0), int32(-1), 1.5, "a", nil, 2.0, int64(2), mustDecimal("2"), "$missing", "$n", "$two",
		int64(4294967298), 2147483648.0, mustDecimal("2.5"), true}
	for i, limit := range limits {
		for _, input := range []string{"$arr", "$e", "$n"} {
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("FilterLimit_%d_%s", i, input[1:]),
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: pipelineRun(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: bson.D{{Key: "$filter", Value: bson.D{
					{Key: "input", Value: input}, {Key: "cond", Value: true}, {Key: "limit", Value: limit},
				}}}}}}}}),
			})
		}
	}
	others := map[string]bson.D{
		"cond_and_limit": {{Key: "input", Value: "$arr"}, {Key: "cond", Value: bson.D{{Key: "$gt", Value: bson.A{"$$this", int32(1)}}}}, {Key: "limit", Value: int32(1)}},
		"unknown":        {{Key: "input", Value: "$arr"}, {Key: "cond", Value: true}, {Key: "bogus", Value: int32(1)}},
		"string_input":   {{Key: "input", Value: "$s"}, {Key: "cond", Value: true}, {Key: "limit", Value: int32(0)}},
		"missing_input":  {{Key: "input", Value: "$nope"}, {Key: "cond", Value: true}, {Key: "limit", Value: int32(0)}},
	}
	for name, spec := range others {
		harness.PairTest(t, harness.TestCase{
			Name:    "FilterLimit_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: pipelineRun(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0},
				{Key: "r", Value: bson.D{{Key: "$filter", Value: spec}}}}}}}),
		})
	}
}
