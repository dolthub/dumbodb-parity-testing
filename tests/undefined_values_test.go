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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Commands may carry the deprecated BSON undefined type anywhere a value
// goes, except as a filter's comparison operand.

func undefinedSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}},
	})
	return err
}

func undefinedFind(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return nil, err
		}
		var results []bson.D
		return results, cursor.All(ctx, &results)
	}
}

func TestUndefinedValues(t *testing.T) {
	undef := primitive.Undefined{}
	for _, tc := range []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"ExistsOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$exists", Value: undef}}}})},
		{"Equality", undefinedFind(bson.D{{Key: "f", Value: undef}})},
		{"InOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$in", Value: bson.A{undef, int32(1)}}}}})},
		{"EqOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$eq", Value: undef}}}})},
		{"NeOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$ne", Value: undef}}}})},
		{"GtOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: undef}}}})},
		{"LteOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$lte", Value: undef}}}})},
		{"NinOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$nin", Value: bson.A{undef}}}}})},
		{"AllOperand", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$all", Value: bson.A{undef}}}}})},
		{"DottedEquality", undefinedFind(bson.D{{Key: "g.h", Value: undef}})},
		{"InsideArrayEquality", undefinedFind(bson.D{{Key: "f", Value: bson.A{undef}}})},
		{"InsideDocumentEquality", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "x", Value: undef}}}})},
		{"NotEq", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$eq", Value: undef}}}}}})},
		{"AfterFailingOperator", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: int32(100)}, {Key: "$eq", Value: undef}}}})},
		{"ElemMatchValue", undefinedFind(bson.D{{Key: "f", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: undef}}}}}})},
		{"Insert", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(10)}, {Key: "u", Value: undef}, {Key: "a", Value: bson.A{undef}}}); err != nil {
				return nil, err
			}
			return undefinedFind(bson.D{{Key: "_id", Value: int32(10)}})(ctx, col)
		}},
		{"SetUpdate", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(1)}}, bson.D{{Key: "$set", Value: bson.D{{Key: "u", Value: undef}}}}); err != nil {
				return nil, err
			}
			return undefinedFind(bson.D{{Key: "_id", Value: int32(1)}})(ctx, col)
		}},
		{"ProjectLiteral", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$literal", Value: undef}}}}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		}},
		{"Comment", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.D
			err := col.Database().RunCommand(ctx, bson.D{{Key: "count", Value: col.Name()}, {Key: "comment", Value: undef}}).Decode(&res)
			return res.Map()["n"], err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "UndefinedValues_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   undefinedSetup,
				Run:     tc.run,
			})
		})
	}
}
