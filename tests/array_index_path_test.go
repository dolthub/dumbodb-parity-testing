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

// A path whose last element is an array index (a.0, a.0.0, a.b.0) compares the
// value there as a whole: mongod does not expand a nested array reached by
// index, only one reached by a field name (dolthub/dumbodb#296).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// arrayIndexPathXFail lists the cases DumboDB fails until dolthub/dumbodb#296 is fixed.
var arrayIndexPathXFail = map[string]bool{
	"TestArrayIndexPath_AggregateMatch":                      true,
	"TestArrayIndexPath_Count/eq":                            true,
	"TestArrayIndexPath_Count/eq_array":                      true,
	"TestArrayIndexPath_Count/field_then_index":              true,
	"TestArrayIndexPath_Count/gt":                            true,
	"TestArrayIndexPath_Count/gte_lte":                       true,
	"TestArrayIndexPath_Count/in":                            true,
	"TestArrayIndexPath_Count/lt_array":                      true,
	"TestArrayIndexPath_Count/mod":                           true,
	"TestArrayIndexPath_Count/ne":                            true,
	"TestArrayIndexPath_Count/nested_index":                  true,
	"TestArrayIndexPath_Count/nested_index_gt":               true,
	"TestArrayIndexPath_Count/nin":                           true,
	"TestArrayIndexPath_Count/nor":                           true,
	"TestArrayIndexPath_Count/not_eq":                        true,
	"TestArrayIndexPath_Count/not_gt":                        true,
	"TestArrayIndexPath_Count/type_int":                      true,
	"TestArrayIndexPath_Find/eq":                             true,
	"TestArrayIndexPath_Find/eq_array":                       true,
	"TestArrayIndexPath_Find/field_then_index":               true,
	"TestArrayIndexPath_Find/gt":                             true,
	"TestArrayIndexPath_Find/gte_lte":                        true,
	"TestArrayIndexPath_Find/in":                             true,
	"TestArrayIndexPath_Find/lt_array":                       true,
	"TestArrayIndexPath_Find/mod":                            true,
	"TestArrayIndexPath_Find/ne":                             true,
	"TestArrayIndexPath_Find/nested_index":                   true,
	"TestArrayIndexPath_Find/nested_index_gt":                true,
	"TestArrayIndexPath_Find/nin":                            true,
	"TestArrayIndexPath_Find/nor":                            true,
	"TestArrayIndexPath_Find/not_eq":                         true,
	"TestArrayIndexPath_Find/not_gt":                         true,
	"TestArrayIndexPath_Find/type_int":                       true,
	"TestArrayIndexPath_IndexedCount/a_0_0_eq":               true,
	"TestArrayIndexPath_IndexedCount/a_0_0_field_then_index": true,
	"TestArrayIndexPath_IndexedCount/a_0_0_gt":               true,
	"TestArrayIndexPath_IndexedCount/a_0_0_nested_index":     true,
	"TestArrayIndexPath_IndexedCount/a_0_eq":                 true,
	"TestArrayIndexPath_IndexedCount/a_0_field_then_index":   true,
	"TestArrayIndexPath_IndexedCount/a_0_gt":                 true,
	"TestArrayIndexPath_IndexedCount/a_0_nested_index":       true,
	"TestArrayIndexPath_IndexedCount/a_b_0_eq":               true,
	"TestArrayIndexPath_IndexedCount/a_b_0_field_then_index": true,
	"TestArrayIndexPath_IndexedCount/a_b_0_gt":               true,
	"TestArrayIndexPath_IndexedCount/a_b_0_nested_index":     true,
	"TestArrayIndexPath_IndexedCount/a_eq":                   true,
	"TestArrayIndexPath_IndexedCount/a_field_then_index":     true,
	"TestArrayIndexPath_IndexedCount/a_gt":                   true,
	"TestArrayIndexPath_IndexedCount/a_nested_index":         true,
	"TestArrayIndexPath_Writes/bulkWrite_updateMany_eq":      true,
	"TestArrayIndexPath_Writes/deleteMany_eq":                true,
	"TestArrayIndexPath_Writes/deleteMany_mod":               true,
	"TestArrayIndexPath_Writes/deleteMany_nor":               true,
	"TestArrayIndexPath_Writes/findAndModify_gt":             true,
	"TestArrayIndexPath_Writes/findAndModify_remove_gt":      true,
	"TestArrayIndexPath_Writes/updateMany_eq":                true,
	"TestArrayIndexPath_Writes/updateMany_nor":               true,
	"TestArrayIndexPath_Writes/updateMany_not":               true,
}

func arrayIndexPathDocs() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{bson.A{1}, bson.A{3}}}},
		bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: bson.A{bson.A{7}, 7}}},
		bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: bson.A{bson.D{{Key: "b", Value: bson.A{bson.A{1}}}}}}},
		bson.D{{Key: "_id", Value: 7}, {Key: "a", Value: bson.A{bson.A{bson.A{1}}}}},
		bson.D{{Key: "_id", Value: 9}, {Key: "a", Value: bson.A{1}}},
	}
}

func arrayIndexPathFilters() []struct {
	name   string
	filter bson.D
} {
	op := func(path, name string, v interface{}) bson.D {
		return bson.D{{Key: path, Value: bson.D{{Key: name, Value: v}}}}
	}
	return []struct {
		name   string
		filter bson.D
	}{
		{"eq", bson.D{{Key: "a.0", Value: 1}}},
		{"eq_array", bson.D{{Key: "a.0", Value: bson.A{1}}}},
		{"gt", op("a.0", "$gt", 0)},
		{"gte_lte", bson.D{{Key: "a.0", Value: bson.D{{Key: "$gte", Value: 1}, {Key: "$lte", Value: 1}}}}},
		{"lt_array", op("a.0", "$lt", bson.A{2})},
		{"in", op("a.0", "$in", bson.A{1, bson.A{7}})},
		{"ne", op("a.0", "$ne", 1)},
		{"nin", op("a.0", "$nin", bson.A{1})},
		{"mod", op("a.0", "$mod", bson.A{1, 0})},
		{"type_int", op("a.0", "$type", "int")},
		{"type_array", op("a.0", "$type", "array")},
		{"size", op("a.0", "$size", 1)},
		{"exists", op("a.0", "$exists", true)},
		{"bits", op("a.0", "$bitsAllSet", 1)},
		{"elemMatch", op("a.0", "$elemMatch", bson.D{{Key: "$eq", Value: 1}})},
		{"regex", op("a.0", "$regex", "1")},
		{"not_eq", op("a.0", "$not", bson.D{{Key: "$eq", Value: 1}})},
		{"not_gt", op("a.0", "$not", bson.D{{Key: "$gt", Value: 0}})},
		{"nor", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "a.0", Value: 1}}}}}},
		{"nested_index", bson.D{{Key: "a.0.0", Value: 1}}},
		{"nested_index_gt", op("a.0.0", "$gt", 0)},
		{"field_then_index", bson.D{{Key: "a.b.0", Value: 1}}},
		{"field_then_index_array", bson.D{{Key: "a.b.0", Value: bson.A{1}}}},
		{"index_then_field", bson.D{{Key: "a.0.b", Value: 1}}},
	}
}

func TestArrayIndexPath_Find(t *testing.T) {
	for _, tc := range arrayIndexPathFilters() {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ArrayIndexPath_Find_" + tc.name,
				Support: supportFor(t, arrayIndexPathXFail),
				Setup:   insertDocs(arrayIndexPathDocs()...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return sortedIDs(ctx, col, tc.filter)
				},
			})
		})
	}
}

func TestArrayIndexPath_Count(t *testing.T) {
	for _, tc := range arrayIndexPathFilters() {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ArrayIndexPath_Count_" + tc.name,
				Support: supportFor(t, arrayIndexPathXFail),
				Setup:   insertDocs(arrayIndexPathDocs()...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return col.CountDocuments(ctx, tc.filter)
				},
			})
		})
	}
}

// An index on the path or its array must not change the result: its keys
// expand the nested arrays the filter compares whole.
func TestArrayIndexPath_IndexedCount(t *testing.T) {
	for _, idx := range []struct {
		name string
		key  bson.D
	}{
		{"a_0", bson.D{{Key: "a.0", Value: 1}}},
		{"a", bson.D{{Key: "a", Value: 1}}},
		{"a_0_0", bson.D{{Key: "a.0.0", Value: 1}}},
		{"a_b_0", bson.D{{Key: "a.b.0", Value: 1}}},
	} {
		for _, tc := range []struct {
			name   string
			filter bson.D
		}{
			{"eq", bson.D{{Key: "a.0", Value: 1}}},
			{"gt", bson.D{{Key: "a.0", Value: bson.D{{Key: "$gt", Value: 0}}}}},
			{"nested_index", bson.D{{Key: "a.0.0", Value: 1}}},
			{"field_then_index", bson.D{{Key: "a.b.0", Value: 1}}},
		} {
			t.Run(idx.name+"_"+tc.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "ArrayIndexPath_IndexedCount_" + idx.name + "_" + tc.name,
					Support: supportFor(t, arrayIndexPathXFail),
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: idx.key}); err != nil {
							return err
						}
						return insertDocs(arrayIndexPathDocs()...)(ctx, col)
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						n, err := col.CountDocuments(ctx, tc.filter)
						if err != nil {
							return nil, err
						}
						ids, err := sortedIDs(ctx, col, tc.filter)
						return bson.A{n, ids}, err
					},
				})
			})
		}
	}
}

func TestArrayIndexPath_Writes(t *testing.T) {
	eq := bson.D{{Key: "a.0", Value: 1}}
	gt := bson.D{{Key: "a.0", Value: bson.D{{Key: "$gt", Value: 0}}}}
	notEq := bson.D{{Key: "a.0", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$eq", Value: 1}}}}}}
	nor := bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "a.0", Value: 1}}}}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: true}}}}
	for _, tc := range []struct {
		name  string
		write func(ctx context.Context, col *mongo.Collection) error
	}{
		{"updateMany_eq", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, eq, set)
			return err
		}},
		{"updateMany_not", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, notEq, set)
			return err
		}},
		{"updateMany_nor", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, nor, set)
			return err
		}},
		{"deleteMany_eq", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteMany(ctx, eq)
			return err
		}},
		{"deleteMany_nor", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteMany(ctx, nor)
			return err
		}},
		{"deleteMany_mod", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteMany(ctx, bson.D{{Key: "a.0", Value: bson.D{{Key: "$mod", Value: bson.A{1, 0}}}}})
			return err
		}},
		{"findAndModify_gt", func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, gt, set, options.FindOneAndUpdate().SetSort(bson.D{{Key: "_id", Value: 1}})).Err()
		}},
		{"findAndModify_remove_gt", func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndDelete(ctx, gt, options.FindOneAndDelete().SetSort(bson.D{{Key: "_id", Value: 1}})).Err()
		}},
		{"bulkWrite_updateMany_eq", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{mongo.NewUpdateManyModel().SetFilter(eq).SetUpdate(set)})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ArrayIndexPath_Writes_" + tc.name,
				Support: supportFor(t, arrayIndexPathXFail),
				Setup:   insertDocs(arrayIndexPathDocs()...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error { return tc.write(ctx, col) })
				},
			})
		})
	}
}

func TestArrayIndexPath_AggregateMatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayIndexPath_AggregateMatch",
		Support: supportFor(t, arrayIndexPathXFail),
		Setup:   insertDocs(arrayIndexPathDocs()...),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "a.0", Value: bson.D{{Key: "$gte", Value: 1}}}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		},
	})
}
