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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// MongoDB rejects an invalid filter when it parses the command, whether or
// not any document reaches the invalid predicate.

func filterValidationSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: int32(3)}, {Key: "f", Value: bson.A{int32(1)}}},
	})
	return err
}

// filterValidationMatches lists the cases DumboDB already answers like MongoDB.
var filterValidationMatches = map[string]bool{
	"Find_InNotArray":           true,
	"Find_InsideOr":             true,
	"Find_Valid":                true,
	"Find_Valid_Empty":          true,
	"Count_InNotArray":          true,
	"Count_InsideOr":            true,
	"Count_Valid":               true,
	"Count_Valid_Empty":         true,
	"Distinct_InNotArray":       true,
	"Distinct_InsideOr":         true,
	"Distinct_Valid":            true,
	"Distinct_Valid_Empty":      true,
	"Aggregate_InNotArray":      true,
	"Aggregate_InsideOr":        true,
	"Aggregate_Valid":           true,
	"Aggregate_Valid_Empty":     true,
	"Update_Valid":              true,
	"Update_Valid_Empty":        true,
	"Delete_InNotArray":         true,
	"Delete_InsideOr":           true,
	"Delete_Valid":              true,
	"Delete_Valid_Empty":        true,
	"FindAndModify_InNotArray":  true,
	"FindAndModify_InsideOr":    true,
	"FindAndModify_Valid":       true,
	"FindAndModify_Valid_Empty": true,
}

func TestFilter_Validation(t *testing.T) {
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"InNotArray", bson.D{{Key: "f", Value: bson.D{{Key: "$in", Value: int32(1)}}}}},
		{"AfterUnmatchedField", bson.D{{Key: "a", Value: int32(1)}, {Key: "f", Value: bson.D{{Key: "$in", Value: int32(1)}}}}},
		{"AfterUnmatchedOperator", bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: int32(100)}, {Key: "$mod", Value: bson.A{int32(0), int32(0)}}}}}},
		{"InsideAnd", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "a", Value: int32(1)}}, bson.D{{Key: "f", Value: bson.D{{Key: "$size", Value: "x"}}}}}}}},
		{"InsideOr", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: int32(2)}}, bson.D{{Key: "f", Value: bson.D{{Key: "$type", Value: "nope"}}}}}}}},
		{"InsideNot", bson.D{{Key: "a", Value: int32(1)}, {Key: "f", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$in", Value: int32(1)}}}}}}},
		{"InsideElemMatch", bson.D{{Key: "a", Value: int32(1)}, {Key: "f", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$in", Value: int32(1)}}}}}}},
		{"InvalidRegex", bson.D{{Key: "a", Value: int32(1)}, {Key: "f", Value: bson.D{{Key: "$regex", Value: "("}}}}},
		{"UnknownOperator", bson.D{{Key: "a", Value: int32(1)}, {Key: "f", Value: bson.D{{Key: "$nope", Value: int32(1)}}}}},
		{"Valid", bson.D{{Key: "a", Value: int32(1)}, {Key: "f", Value: bson.D{{Key: "$in", Value: bson.A{int32(1)}}}}}},
	}
	commands := []struct {
		name string
		run  func(context.Context, *mongo.Collection, bson.D) (interface{}, error)
	}{
		{"Find", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			cursor, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		}},
		{"Count", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			var res bson.D
			err := col.Database().RunCommand(ctx, bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: filter}}).Decode(&res)
			return res.Map()["n"], err
		}},
		{"Distinct", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			return col.Distinct(ctx, "a", filter)
		}},
		{"Aggregate", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, bson.D{{Key: "$match", Value: filter}}})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		}},
		{"Update", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			res, err := col.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "u", Value: int32(1)}}}})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "matched", Value: res.MatchedCount}}, nil
		}},
		{"Delete", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			res, err := col.DeleteMany(ctx, filter)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deleted", Value: res.DeletedCount}}, nil
		}},
		{"FindAndModify", func(ctx context.Context, col *mongo.Collection, filter bson.D) (interface{}, error) {
			var doc bson.D
			err := col.FindOneAndUpdate(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "u", Value: int32(1)}}}}).Decode(&doc)
			if err == mongo.ErrNoDocuments {
				return "none", nil
			}
			return doc, err
		}},
	}
	for _, cmd := range commands {
		for _, f := range filters {
			for _, empty := range []bool{false, true} {
				name := cmd.name + "_" + f.name
				if empty {
					name += "_Empty"
				}
				t.Run(name, func(t *testing.T) {
					setup := filterValidationSetup
					if empty {
						setup = func(context.Context, *mongo.Collection) error { return nil }
					}
					support := harness.DumboDBXFail
					if filterValidationMatches[name] {
						support = harness.DumboDBFull
					}
					harness.PairTest(t, harness.TestCase{
						Name:    "Filter_Validation_" + name,
						Support: support,
						Setup:   setup,
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							return cmd.run(ctx, col, f.filter)
						},
					})
				})
			}
		}
	}
}
