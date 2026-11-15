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

// Applying $[] or $[identifier] to a missing path or a non-array is a write
// error that stops a multi update at that document, keeping the ones updated
// before it (dolthub/dumbodb#253).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// arrayFiltersNonArrayXFail lists the cases DumboDB fails until dolthub/dumbodb#253 is fixed.
var arrayFiltersNonArrayXFail = map[string]bool{
	"TestArrayFiltersNonArray/all_positional_all_docs_bulkWrite":      true,
	"TestArrayFiltersNonArray/all_positional_all_docs_updateMany":     true,
	"TestArrayFiltersNonArray/all_positional_missing_bulkWrite":       true,
	"TestArrayFiltersNonArray/all_positional_missing_findAndModify":   true,
	"TestArrayFiltersNonArray/all_positional_missing_updateMany":      true,
	"TestArrayFiltersNonArray/all_positional_missing_updateOne":       true,
	"TestArrayFiltersNonArray/all_positional_non_array_bulkWrite":     true,
	"TestArrayFiltersNonArray/all_positional_non_array_findAndModify": true,
	"TestArrayFiltersNonArray/all_positional_non_array_updateMany":    true,
	"TestArrayFiltersNonArray/all_positional_non_array_updateOne":     true,
	"TestArrayFiltersNonArray/filtered_all_docs_bulkWrite":            true,
	"TestArrayFiltersNonArray/filtered_all_docs_updateMany":           true,
	"TestArrayFiltersNonArray/filtered_missing_bulkWrite":             true,
	"TestArrayFiltersNonArray/filtered_missing_findAndModify":         true,
	"TestArrayFiltersNonArray/filtered_missing_updateMany":            true,
	"TestArrayFiltersNonArray/filtered_missing_updateOne":             true,
	"TestArrayFiltersNonArray/filtered_non_array_bulkWrite":           true,
	"TestArrayFiltersNonArray/filtered_non_array_findAndModify":       true,
	"TestArrayFiltersNonArray/filtered_non_array_updateMany":          true,
	"TestArrayFiltersNonArray/filtered_non_array_updateOne":           true,
	"TestArrayFiltersNonArray/inc_non_array_bulkWrite":                true,
	"TestArrayFiltersNonArray/inc_non_array_findAndModify":            true,
	"TestArrayFiltersNonArray/inc_non_array_updateMany":               true,
	"TestArrayFiltersNonArray/inc_non_array_updateOne":                true,
	"TestArrayFiltersNonArray/nested_missing_path_bulkWrite":          true,
	"TestArrayFiltersNonArray/nested_missing_path_findAndModify":      true,
	"TestArrayFiltersNonArray/nested_missing_path_updateMany":         true,
	"TestArrayFiltersNonArray/nested_missing_path_updateOne":          true,
	"TestArrayFiltersNonArray/unset_non_array_bulkWrite":              true,
	"TestArrayFiltersNonArray/unset_non_array_findAndModify":          true,
	"TestArrayFiltersNonArray/unset_non_array_updateMany":             true,
	"TestArrayFiltersNonArray/unset_non_array_updateOne":              true,
	"TestArrayFiltersNonArrayUpsert":                                  true,
}

func TestArrayFiltersNonArray(t *testing.T) {
	setE := bson.D{{Key: "$set", Value: bson.D{{Key: "x.$[e]", Value: 9}}}}
	setAll := bson.D{{Key: "$set", Value: bson.D{{Key: "x.$[]", Value: 9}}}}
	positive := []interface{}{bson.D{{Key: "e", Value: bson.D{{Key: "$gt", Value: 0}}}}}
	runArrayFilterCases(t, "ArrayFiltersNonArray", arrayFiltersNonArrayXFail, []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "x", Value: bson.A{1, 2}}},
		bson.D{{Key: "_id", Value: 2}, {Key: "x", Value: 5}},
		bson.D{{Key: "_id", Value: 3}, {Key: "x", Value: bson.A{3}}},
		bson.D{{Key: "_id", Value: 4}, {Key: "y", Value: 1}},
	}, []struct {
		name    string
		filter  bson.D
		update  interface{}
		filters []interface{}
	}{
		{"filtered_all_docs", bson.D{}, setE, positive},
		{"filtered_non_array", bson.D{{Key: "_id", Value: 2}}, setE, positive},
		{"filtered_missing", bson.D{{Key: "_id", Value: 4}}, setE, positive},
		{"all_positional_all_docs", bson.D{}, setAll, nil},
		{"all_positional_non_array", bson.D{{Key: "_id", Value: 2}}, setAll, nil},
		{"all_positional_missing", bson.D{{Key: "_id", Value: 4}}, setAll, nil},
		{"inc_non_array", bson.D{{Key: "_id", Value: 2}}, bson.D{{Key: "$inc", Value: bson.D{{Key: "x.$[]", Value: 1}}}}, nil},
		{"unset_non_array", bson.D{{Key: "_id", Value: 2}}, bson.D{{Key: "$unset", Value: bson.D{{Key: "x.$[]", Value: 1}}}}, nil},
		{"nested_missing_path", bson.D{{Key: "_id", Value: 4}}, bson.D{{Key: "$set", Value: bson.D{{Key: "y.z.$[]", Value: 1}}}}, nil},
		{"arrays_only", bson.D{{Key: "x", Value: bson.D{{Key: "$type", Value: "array"}}}}, setE, positive},
	})
}

func TestArrayFiltersNonArrayUpsert(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayFiltersNonArrayUpsert",
		Support: supportFor(t, arrayFiltersNonArrayXFail),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return writeOutcome(ctx, col, func() error {
				_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: 7}},
					bson.D{{Key: "$set", Value: bson.D{{Key: "x.$[]", Value: 1}}}}, options.Update().SetUpsert(true))
				return err
			})
		},
	})
}
