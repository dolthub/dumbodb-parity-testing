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

// $reduce can build values nested far deeper than BSON input allows.
// mongod checks the accumulated depth periodically (cap 400) and its size
// (100MB), and refuses to return values nested 200 levels or more.
// Unbounded nesting crashed DumboDB with a stack overflow (workspace-8tj.6).

// deepReduce nests $$value n times inside an array or a document.
func deepReduce(n int, asDocument bool) bson.D {
	initial, in := interface{}(bson.A{}), interface{}(bson.A{"$$value"})
	if asDocument {
		initial, in = bson.D{}, bson.D{{Key: "a", Value: "$$value"}}
	}
	return bson.D{{Key: "$reduce", Value: bson.D{
		{Key: "input", Value: bson.D{{Key: "$range", Value: bson.A{0, n}}}},
		{Key: "initialValue", Value: initial},
		{Key: "in", Value: in},
	}}}
}

var reduceDepthUses = []struct {
	name string
	use  func(v bson.D) interface{}
}{
	{"Return", func(v bson.D) interface{} { return v }},
	{"Type", func(v bson.D) interface{} { return bson.D{{Key: "$type", Value: v}} }},
	{"Eq", func(v bson.D) interface{} { return bson.D{{Key: "$eq", Value: bson.A{v, v}}} }},
	{"Cmp", func(v bson.D) interface{} { return bson.D{{Key: "$cmp", Value: bson.A{v, bson.A{v}}}} }},
	{"SortArray", func(v bson.D) interface{} {
		return bson.D{{Key: "$size", Value: bson.D{{Key: "$sortArray", Value: bson.D{
			{Key: "input", Value: bson.A{v, v, 1}},
			{Key: "sortBy", Value: 1},
		}}}}}
	}},
}

func TestReduce_DeepNesting(t *testing.T) {
	for _, depth := range []int{150, 198, 199, 200, 399, 400, 401, 1000, 100000} {
		for _, shape := range []string{"Array", "Document"} {
			for _, u := range reduceDepthUses {
				name := fmt.Sprintf("Reduce_DeepNesting_%s_%d_%s", shape, depth, u.name)
				expr := u.use(deepReduce(depth, shape == "Document"))
				t.Run(name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    name,
						Support: harness.DumboDBFull,
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							return projectOne(ctx, col, expr)
						},
					})
				})
			}
		}
	}
}

// Doubling an array each iteration exceeds the 100MB accumulator limit.
func TestReduce_MemoryLimit(t *testing.T) {
	expr := bson.D{{Key: "$size", Value: bson.D{{Key: "$reduce", Value: bson.D{
		{Key: "input", Value: bson.D{{Key: "$range", Value: bson.A{0, 40}}}},
		{Key: "initialValue", Value: bson.A{1}},
		{Key: "in", Value: bson.D{{Key: "$concatArrays", Value: bson.A{"$$value", "$$value"}}}},
	}}}}}
	harness.PairTest(t, harness.TestCase{
		Name:    "Reduce_MemoryLimit",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return projectOne(ctx, col, expr)
		},
	})
}

func projectOne(ctx context.Context, col *mongo.Collection, expr interface{}) (interface{}, error) {
	if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}); err != nil {
		return nil, err
	}
	cur, err := col.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}},
	})
	if err != nil {
		return nil, err
	}
	var out []bson.Raw
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
