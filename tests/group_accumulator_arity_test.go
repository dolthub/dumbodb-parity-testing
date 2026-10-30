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

// $group accumulators are unary: mongod rejects any array operand with
// 40237. $mergeObjects with zero or 2+ operands dereferenced a nil
// accumulator in DumboDB (workspace-8tj.10).
func TestGroupAccumulatorArity(t *testing.T) {
	accumulators := []string{
		"$mergeObjects", "$sum", "$avg", "$min", "$max", "$push", "$addToSet",
		"$first", "$last", "$stdDevPop", "$stdDevSamp", "$count",
	}
	operands := []struct {
		name    string
		operand bson.A
	}{
		{"Empty", bson.A{}},
		{"One", bson.A{"$d"}},
		{"Two", bson.A{"$d", "$e"}},
		{"OneLiteral", bson.A{bson.D{{Key: "x", Value: int32(1)}}}},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(0)}, {Key: "d", Value: bson.D{{Key: "a", Value: int32(1)}}}, {Key: "e", Value: bson.D{{Key: "b", Value: int32(2)}}}},
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "d", Value: bson.D{{Key: "c", Value: int32(3)}}}},
		})
		return err
	}
	for _, acc := range accumulators {
		for _, o := range operands {
			name := "GroupAccumulatorArity_" + acc[1:] + "_" + o.name
			pipeline := bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: acc, Value: o.operand}}}}}},
			}
			t.Run(acc[1:]+"_"+o.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name,
					Support: harness.DumboDBXFail,
					Setup:   setup,
					Run:     pipelineRunExtJSON(pipeline),
				})
			})
		}
	}
}
