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

// TestArrayInputNotArray: an input that is not an array gives $map 16883,
// $reduce 40080, $in 40081 and $zip 34468, whether it is a field, a constant
// or a $lookup let variable.
func TestArrayInputNotArray(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(5)}})
		return err
	}
	mapOf := func(input any) bson.D {
		return bson.D{{Key: "$map", Value: bson.D{{Key: "input", Value: input}, {Key: "in", Value: 1}}}}
	}
	reduceOf := func(input any) bson.D {
		return bson.D{{Key: "$reduce", Value: bson.D{{Key: "input", Value: input}, {Key: "initialValue", Value: 0}, {Key: "in", Value: 1}}}}
	}
	inOf := func(input any) bson.D { return bson.D{{Key: "$in", Value: bson.A{1, input}}} }
	zipOf := func(input any) bson.D {
		return bson.D{{Key: "$zip", Value: bson.D{{Key: "inputs", Value: bson.A{input}}}}}
	}
	project := func(expr bson.D) bson.D { return bson.D{{Key: "$project", Value: bson.D{{Key: "x", Value: expr}}}} }
	lookupLet := func(let any, expr bson.D) bson.A {
		return bson.A{bson.D{{Key: "$lookup", Value: bson.D{
			{Key: "let", Value: bson.D{{Key: "v", Value: let}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{bson.D{}}}}, project(expr)}},
			{Key: "as", Value: "r"},
		}}}}
	}
	ops := map[string]func(any) bson.D{"map": mapOf, "reduce": reduceOf, "in": inOf, "zip": zipOf}
	cases := map[string]bson.A{}
	for name, op := range ops {
		cases[name+"_field"] = bson.A{project(op("$v"))}
		cases[name+"_const"] = bson.A{project(op(5))}
		cases[name+"_literal"] = bson.A{project(op(bson.D{{Key: "$literal", Value: 5}}))}
		cases[name+"_group"] = bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: op("$v")}}}}}
		cases[name+"_let_const"] = lookupLet(5, op("$$v"))
		cases[name+"_let_field"] = lookupLet("$v", op("$$v"))
	}
	for name, pipeline := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "ArrayInputNotArray_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
