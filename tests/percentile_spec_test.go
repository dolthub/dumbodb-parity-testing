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

// $percentile's p must fold to a constant array: mongod folds $range, $cond,
// $slice and the like, but not $map, $filter, $sortArray, $zip, $reduce or
// $let. The accumulator, window and expression forms share these rules.

func percentileSeed(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 20)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: int32(i % 7)}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func percentileSpec(p interface{}) bson.D {
	return bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: p}, {Key: "method", Value: "approximate"}}
}

func TestPercentileSpec_ConstantP(t *testing.T) {
	ps := []struct {
		name string
		p    interface{}
	}{
		{"Map", bson.D{{Key: "$map", Value: bson.D{{Key: "input", Value: bson.A{int32(1)}}, {Key: "in", Value: 0.5}}}}},
		{"Filter", bson.D{{Key: "$filter", Value: bson.D{{Key: "input", Value: bson.A{0.5}}, {Key: "cond", Value: true}}}}},
		{"SortArray", bson.D{{Key: "$sortArray", Value: bson.D{{Key: "input", Value: bson.A{0.5}}, {Key: "sortBy", Value: int32(1)}}}}},
		{"Zip", bson.D{{Key: "$zip", Value: bson.D{{Key: "inputs", Value: bson.A{bson.A{0.5}}}}}}},
		{"LetInArray", bson.A{bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "a", Value: 0.5}}}, {Key: "in", Value: "$$a"}}}}}},
		{"Range", bson.D{{Key: "$range", Value: bson.A{int32(0), int32(1)}}}},
		{"Cond", bson.D{{Key: "$cond", Value: bson.A{true, bson.A{0.5}, bson.A{0.1}}}}},
		{"Slice", bson.D{{Key: "$slice", Value: bson.A{bson.A{0.5, 0.2}, int32(1)}}}},
		{"FieldPath", "$v"},
	}
	forms := []struct {
		name  string
		stage func(p interface{}) bson.D
	}{
		{"Group", func(p interface{}) bson.D {
			return bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "p", Value: bson.D{{Key: "$percentile", Value: percentileSpec(p)}}}}}}
		}},
		{"Window", func(p interface{}) bson.D {
			return bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
				{Key: "output", Value: bson.D{{Key: "p", Value: bson.D{{Key: "$percentile", Value: percentileSpec(p)}}}}}}}}
		}},
		{"Expression", func(p interface{}) bson.D {
			return bson.D{{Key: "$project", Value: bson.D{{Key: "p", Value: bson.D{{Key: "$percentile", Value: percentileSpec(p)}}}}}}
		}},
	}
	for _, f := range forms {
		for _, c := range ps {
			stage := f.stage(c.p)
			harness.PairTest(t, harness.TestCase{
				Name:    "PercentileConstantP_" + f.name + "_" + c.name,
				Support: harness.DumboDBFull,
				Setup:   percentileSeed,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return fieldOpsAgg(ctx, col, stage, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
				},
			})
		}
	}
}

func TestPercentileSpec_ExpressionValidation(t *testing.T) {
	cases := []struct {
		name string
		op   string
		spec interface{}
	}{
		{"NotObject", "$percentile", int32(5)},
		{"MedianNotObject", "$median", int32(5)},
		{"PNotArray", "$percentile", percentileSpec("x")},
		{"PEmpty", "$percentile", percentileSpec(bson.A{})},
		{"PNotNumber", "$percentile", percentileSpec(bson.A{"a"})},
		{"POutOfRange", "$percentile", percentileSpec(bson.A{int32(2)})},
		{"MissingMethod", "$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.5}}}},
		{"MissingP", "$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "method", Value: "approximate"}}},
		{"UnknownField", "$percentile", append(percentileSpec(bson.A{0.5}), bson.E{Key: "z", Value: int32(1)})},
		{"MethodNotString", "$percentile", bson.D{{Key: "input", Value: "$v"}, {Key: "p", Value: bson.A{0.5}}, {Key: "method", Value: int32(1)}}},
		{"MedianP", "$median", percentileSpec(bson.A{0.5})},
		{"MedianMissingMethod", "$median", bson.D{{Key: "input", Value: "$v"}}},
		{"Valid", "$percentile", percentileSpec(bson.A{0.25, 0.75})},
	}
	for _, c := range cases {
		op, spec := c.op, c.spec
		harness.PairTest(t, harness.TestCase{
			Name:    "PercentileExpression_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   percentileSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col,
					bson.D{{Key: "$project", Value: bson.D{{Key: "p", Value: bson.D{{Key: op, Value: spec}}}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
			},
		})
	}
}

// Many groups and window documents, each with a long array of percentiles.
func TestPercentileSpec_LongOutputs(t *testing.T) {
	p := make(bson.A, 50000)
	for i := range p {
		p[i] = float64(i%11) / 10
	}
	stages := map[string]bson.D{
		"Group": {{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"},
			{Key: "p", Value: bson.D{{Key: "$percentile", Value: percentileSpec(p)}}}}}},
		"Window": {{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "p", Value: bson.D{{Key: "$percentile", Value: percentileSpec(p)}}}}}}}},
	}
	for name, stage := range stages {
		stage := stage
		harness.PairTest(t, harness.TestCase{
			Name:    "PercentileLongOutputs_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				docs := make([]interface{}, 300)
				for i := range docs {
					docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: int32(i)}}
				}
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage,
					bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$p"}}}, {Key: "last", Value: bson.D{{Key: "$last", Value: "$p"}}}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
			},
		})
	}
}
