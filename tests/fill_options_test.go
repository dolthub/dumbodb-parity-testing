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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $linearFill computes with mongod's expression arithmetic, so decimal inputs
// and sort keys give decimal results. $fill and $setWindowFields set a dotted
// output as $addFields does, and $fill parses its options as mongod's IDL
// parser does.

func TestFillOptions_LinearFillTypes(t *testing.T) {
	cases := []struct {
		name string
		docs []interface{}
	}{
		{"DecimalValues", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: int32(1)}, {Key: "x", Value: dz("10")}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: int32(2)}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: int32(4)}, {Key: "x", Value: dz("40")}},
		}},
		{"DecimalSortKeys", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: dz("1")}, {Key: "x", Value: int32(10)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: dz("2")}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: dz("4")}, {Key: "x", Value: int32(40)}},
		}},
		{"MixedSortKeys", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: int32(1)}, {Key: "x", Value: int32(10)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: dz("2")}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: int32(4)}, {Key: "x", Value: int64(40)}},
		}},
		{"DoubleSortDecimalValue", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: 1.5}, {Key: "x", Value: dz("10.1")}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: int32(2)}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: int32(4)}, {Key: "x", Value: int32(3)}},
		}},
		{"DateSortLongValue", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, {Key: "x", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)}, {Key: "x", Value: int64(7)}},
		}},
		{"EqualMixedSortKeys", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: int32(1)}, {Key: "x", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: dz("1.0")}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: int32(4)}, {Key: "x", Value: int32(2)}},
		}},
	}
	for _, c := range cases {
		c := c
		for _, stage := range []bson.D{
			{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
				{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$linearFill", Value: "$x"}}}}}}}},
			{{Key: "$fill", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
				{Key: "output", Value: bson.D{{Key: "x", Value: bson.D{{Key: "method", Value: "linear"}}}}}}}},
		} {
			stage := stage
			harness.PairTest(t, harness.TestCase{
				Name:    "FillOptions_LinearFill" + stage[0].Key[1:2] + "_" + c.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertMany(ctx, c.docs)
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return fieldOpsAgg(ctx, col, stage, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
				},
			})
		}
	}
}

func fillOptionsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: int32(1)}, {Key: "y", Value: bson.D{{Key: "z", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: int32(2)}, {Key: "y", Value: bson.D{{Key: "w", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: int32(3)}, {Key: "y", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "t", Value: int32(4)}, {Key: "y", Value: bson.A{int32(1), bson.D{{Key: "q", Value: int32(1)}}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "t", Value: int32(5)}},
	})
	return err
}

func TestFillOptions_Specs(t *testing.T) {
	byT := bson.E{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}}
	out := func(field string, spec bson.D) bson.E {
		return bson.E{Key: "output", Value: bson.D{{Key: field, Value: spec}}}
	}
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"WindowDottedSum", bson.D{{Key: "$setWindowFields", Value: bson.D{byT,
			out("y.z", bson.D{{Key: "$sum", Value: int32(1)}})}}}},
		{"WindowDottedPush", bson.D{{Key: "$setWindowFields", Value: bson.D{byT,
			out("y.z", bson.D{{Key: "$push", Value: "$t"}})}}}},
		{"FillDottedLocf", bson.D{{Key: "$fill", Value: bson.D{byT, out("y.z", bson.D{{Key: "method", Value: "locf"}})}}}},
		{"FillDottedValue", bson.D{{Key: "$fill", Value: bson.D{byT, out("y.z", bson.D{{Key: "value", Value: int32(7)}})}}}},
		{"MethodUnknownKey", bson.D{{Key: "$fill", Value: bson.D{byT, out("x", bson.D{{Key: "method", Value: "locf"}, {Key: "extra", Value: int32(1)}})}}}},
		{"ValueUnknownKey", bson.D{{Key: "$fill", Value: bson.D{byT, out("x", bson.D{{Key: "value", Value: int32(1)}, {Key: "extra", Value: int32(1)}})}}}},
		{"OnlyUnknownKey", bson.D{{Key: "$fill", Value: bson.D{byT, out("x", bson.D{{Key: "extra", Value: int32(1)}})}}}},
		{"EmptyFieldSpec", bson.D{{Key: "$fill", Value: bson.D{byT, out("x", bson.D{})}}}},
		{"PartitionByNull", bson.D{{Key: "$fill", Value: bson.D{byT, {Key: "partitionBy", Value: nil}, out("x", bson.D{{Key: "value", Value: int32(1)}})}}}},
		{"SortByNull", bson.D{{Key: "$fill", Value: bson.D{{Key: "sortBy", Value: nil}, out("x", bson.D{{Key: "value", Value: int32(1)}})}}}},
		{"UnknownNullOption", bson.D{{Key: "$fill", Value: bson.D{out("x", bson.D{{Key: "method", Value: "locf"}}), {Key: "extra", Value: nil}}}}},
		{"MethodNull", bson.D{{Key: "$fill", Value: bson.D{out("x", bson.D{{Key: "method", Value: nil}})}}}},
		{"MethodNullWithValue", bson.D{{Key: "$fill", Value: bson.D{out("x", bson.D{{Key: "method", Value: nil}, {Key: "value", Value: int32(5)}})}}}},
		{"MethodNotString", bson.D{{Key: "$fill", Value: bson.D{out("x", bson.D{{Key: "method", Value: int32(1)}})}}}},
		{"ValueNull", bson.D{{Key: "$fill", Value: bson.D{out("x", bson.D{{Key: "value", Value: nil}})}}}},
		{"ValueNullWithMethod", bson.D{{Key: "$fill", Value: bson.D{out("x", bson.D{{Key: "value", Value: nil}, {Key: "method", Value: "locf"}})}}}},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    "FillOptions_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   fillOptionsSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
			},
		})
	}
}

// Output paths that conflict, one within another, are refused: 40176 in $set
// and $addFields (and $fill by value), 6307900 in $setWindowFields (and $fill
// by method). An output name component cannot start with '$'.
func TestOutputPathConflicts(t *testing.T) {
	pipelines := map[string]string{
		"AddFieldsPathThenSub":  `[{"$addFields":{"y":1,"y.z":2}}]`,
		"AddFieldsSubThenPath":  `[{"$addFields":{"y.z":2,"y":1}}]`,
		"SetObjectThenSub":      `[{"$set":{"y":{"z":1},"y.z":2}}]`,
		"SetSiblings":           `[{"$set":{"y.a":1,"y.b":2}}]`,
		"SetOperatorThenSub":    `[{"$set":{"y":{"$add":[1,1]},"y.b":2}}]`,
		"SetEmptyObjectThenSub": `[{"$set":{"y":{},"y.z":2}}]`,
		"WindowPathThenSub":     `[{"$setWindowFields":{"output":{"y":{"$sum":1},"y.z":{"$sum":1}}}}]`,
		"WindowSubThenPath":     `[{"$setWindowFields":{"output":{"y.z":{"$sum":1},"y":{"$sum":1}}}}]`,
		"WindowDollarComponent": `[{"$setWindowFields":{"output":{"y.$z":{"$sum":1}}}}]`,
		"WindowSiblings":        `[{"$setWindowFields":{"output":{"y.a":{"$sum":1},"y.b":{"$sum":1}}}}]`,
		"FillMethodPathThenSub": `[{"$fill":{"sortBy":{"x":1},"output":{"y":{"method":"locf"},"y.z":{"method":"locf"}}}}]`,
		"FillValuePathThenSub":  `[{"$fill":{"output":{"y":{"value":1},"y.z":{"value":2}}}}]`,
	}
	for name, js := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"s":`+js+`}`), false, &struct{ S *[]bson.D }{&stages}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "OutputPathConflicts_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "OutputPathConflicts_UpdatePipelineSet",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx, bson.D{}, mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: int32(1)}, {Key: "y.z", Value: int32(2)}}}}})
			return nil, err
		},
	})
}

// An empty object beside paths within it adds nothing: the paths apply to
// the current value, which may be an object or an array. A dotted name
// inside an object literal is refused (40183), and a repeated output name
// conflicts with itself.
func TestOutputPathEmptyObjectAndDuplicates(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "y", Value: bson.D{{Key: "q", Value: int32(1)}}}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "y", Value: bson.A{bson.D{{Key: "a", Value: int32(1)}}, int32(2)}}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "y", Value: int32(5)}},
			bson.D{{Key: "_id", Value: int32(4)}},
		})
		return err
	}
	pipelines := map[string]string{
		"EmptyThenSub":       `[{"$set":{"y":{},"y.z":2}},{"$sort":{"_id":1}}]`,
		"SubThenEmpty":       `[{"$addFields":{"y.z":2,"y":{}}},{"$sort":{"_id":1}}]`,
		"DottedInSubObject":  `[{"$set":{"y":{"a.b":1}}}]`,
		"DottedDeeper":       `[{"$set":{"y":{"c":{"a.b":1}}}}]`,
		"DuplicateSet":       `[{"$set":{"y":1,"y":2}}]`,
		"DuplicateAddFields": `[{"$addFields":{"y":1,"y":2}}]`,
		"DuplicateWindow":    `[{"$setWindowFields":{"output":{"y":{"$sum":1},"y":{"$sum":2}}}}]`,
		"DuplicateProject":   `[{"$project":{"y":1,"y":0}}]`,
		"DuplicateInFacet":   `[{"$facet":{"f":[{"$set":{"y":1,"y":2}}]}}]`,
	}
	for name, js := range pipelines {
		var stages bson.A
		if err := bson.UnmarshalExtJSON([]byte(js), false, &stages); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "OutputPathEmptyObjectAndDuplicates_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var res bson.M
				if err := col.Database().RunCommand(ctx, bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: stages}, {Key: "cursor", Value: bson.D{}}}).Decode(&res); err != nil {
					return nil, err
				}
				return res["cursor"].(bson.M)["firstBatch"], nil
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "OutputPathEmptyObjectAndDuplicates_UpdatePipelineKeepsY",
		Support: harness.DumboDBFull,
		Setup:   seed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: bson.D{}}, {Key: "y.z", Value: int32(3)}}}}}); err != nil {
				return nil, err
			}
			return findWithProjection(ctx, col, bson.D{{Key: "_id", Value: int32(1)}}, nil)
		},
	})
}
