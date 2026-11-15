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
				Support: harness.DumboDBFull,
				Setup:   insertDocs(arrayIndexPathDocs()...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return findIDs(ctx, col, tc.filter)
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
				Support: harness.DumboDBFull,
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
					Support: harness.DumboDBFull,
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
						ids, err := findIDs(ctx, col, tc.filter)
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
				Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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

func arrayIndexPathReviewDocs() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{bson.A{1}, bson.A{3}}}},
		bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: bson.A{bson.A{bson.A{1}}}}},
		bson.D{{Key: "_id", Value: 6}, {Key: "a", Value: bson.A{bson.D{{Key: "0", Value: 1}}}}},
		bson.D{{Key: "_id", Value: 7}, {Key: "a", Value: bson.D{{Key: "0", Value: bson.A{1}}}}},
		bson.D{{Key: "_id", Value: 8}, {Key: "a", Value: bson.A{bson.D{{Key: "0", Value: bson.A{5}}}, 7}}},
		bson.D{{Key: "_id", Value: 9}, {Key: "a", Value: bson.A{6}}},
		bson.D{{Key: "_id", Value: 17}, {Key: "a", Value: bson.A{bson.A{1, 2}, bson.A{3}}}},
		bson.D{{Key: "_id", Value: 20}, {Key: "a", Value: bson.A{bson.A{2, 3}}}},
	}
}

// $all of $elemMatch documents on an array reached by index, and a numeric
// path element that also names a field of the array's documents.
func arrayIndexPathReviewFilters() []struct {
	name   string
	filter bson.D
} {
	elemMatch := func(op string, v interface{}) bson.D {
		return bson.D{{Key: "$elemMatch", Value: bson.D{{Key: op, Value: v}}}}
	}
	all := func(path string, elems ...interface{}) bson.D {
		return bson.D{{Key: path, Value: bson.D{{Key: "$all", Value: bson.A(elems)}}}}
	}
	op := func(path, name string, v interface{}) bson.D {
		return bson.D{{Key: path, Value: bson.D{{Key: name, Value: v}}}}
	}
	return []struct {
		name   string
		filter bson.D
	}{
		{"all_elemMatch", all("a.0", elemMatch("$eq", 1))},
		{"all_two_elemMatch", all("a.0", elemMatch("$gt", 0), elemMatch("$lt", 2))},
		{"all_elemMatch_nested_index", all("a.0.0", elemMatch("$eq", 1))},
		{"not_all_elemMatch", op("a.0", "$not", bson.D{{Key: "$all", Value: bson.A{elemMatch("$eq", 1)}}})},
		{"nor_all_elemMatch", bson.D{{Key: "$nor", Value: bson.A{all("a.0", elemMatch("$gt", 2))}}}},
		{"elemMatch_all_elemMatch", op("a", "$elemMatch", bson.D{{Key: "$all", Value: bson.A{elemMatch("$eq", 1)}}})},
		{"field_zero_eq", bson.D{{Key: "a.0", Value: 1}}},
		{"field_zero_array", bson.D{{Key: "a.0", Value: 5}}},
		{"field_zero_ne", op("a.0", "$ne", 1)},
		{"field_zero_in", op("a.0", "$in", bson.A{1, 5})},
		{"field_zero_gt", op("a.0", "$gt", 4)},
		{"field_zero_nin", op("a.0", "$nin", bson.A{5})},
		{"field_zero_not", op("a.0", "$not", bson.D{{Key: "$eq", Value: 1}})},
		{"field_zero_nor", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "a.0", Value: 5}}}}}},
		{"field_zero_mod", op("a.0", "$mod", bson.A{5, 0})},
		{"field_zero_type", op("a.0", "$type", "number")},
		{"field_zero_exists", op("a.0", "$exists", false)},
	}
}

func TestArrayIndexPath_Review(t *testing.T) {
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: true}}}}
	for _, tc := range arrayIndexPathReviewFilters() {
		for _, w := range []struct {
			name string
			run  func(ctx context.Context, col *mongo.Collection) (interface{}, error)
		}{
			{"find", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return findIDs(ctx, col, tc.filter)
			}},
			{"count", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return col.CountDocuments(ctx, tc.filter)
			}},
			{"hinted_find", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a.0", Value: 1}}}); err != nil {
					return nil, err
				}
				cur, err := col.Find(ctx, tc.filter, options.Find().SetHint(bson.D{{Key: "a.0", Value: 1}}).
					SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var docs []bson.D
				err = cur.All(ctx, &docs)
				return docs, err
			}},
			{"updateMany", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return writeOutcome(ctx, col, func() error {
					_, err := col.UpdateMany(ctx, tc.filter, set)
					return err
				})
			}},
			{"deleteMany", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return writeOutcome(ctx, col, func() error {
					_, err := col.DeleteMany(ctx, tc.filter)
					return err
				})
			}},
			{"findAndModify_remove", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return writeOutcome(ctx, col, func() error {
					return col.FindOneAndDelete(ctx, tc.filter, options.FindOneAndDelete().SetSort(bson.D{{Key: "_id", Value: 1}})).Err()
				})
			}},
		} {
			t.Run(tc.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "ArrayIndexPath_Review_" + tc.name + "_" + w.name,
					Support: supportFor(t, arrayIndexPathReviewXFail),
					Setup:   insertDocs(arrayIndexPathReviewDocs()...),
					Run:     w.run,
				})
			})
		}
	}
}

// A positional update and arrayFilters whose matches an array index path
// decides.
func TestArrayIndexPath_PositionalUpdates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter bson.D
		update bson.D
		opts   *options.UpdateOptions
	}{
		{"positional_by_index_filter", bson.D{{Key: "a.0", Value: 1}}, bson.D{{Key: "$set", Value: bson.D{{Key: "a.$", Value: 0}}}}, options.Update()},
		{"positional_by_elemMatch", bson.D{{Key: "a", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "0", Value: 3}}}}}}, bson.D{{Key: "$set", Value: bson.D{{Key: "a.$", Value: 0}}}}, options.Update()},
		{"array_filter_on_index", bson.D{}, bson.D{{Key: "$set", Value: bson.D{{Key: "a.$[e]", Value: 0}}}},
			options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{bson.D{{Key: "e.0", Value: 1}}}})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ArrayIndexPath_PositionalUpdates_" + tc.name,
				Support: supportFor(t, arrayIndexPathReviewXFail),
				Setup: insertDocs(
					bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{bson.A{1}, bson.A{3}}}},
					bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: bson.A{bson.A{3, 1}, bson.A{1}}}},
					bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: bson.A{1, 3}}},
				),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.UpdateMany(ctx, tc.filter, tc.update, tc.opts)
						return err
					})
				},
			})
		})
	}
}

// arrayIndexPathReviewXFail lists the review cases DumboDB fails until the follow-up fix for
// dolthub/dumbodb#296.
var arrayIndexPathReviewXFail = map[string]bool{
	"TestArrayIndexPath_PositionalUpdates/positional_by_index_filter":           true,
	"TestArrayIndexPath_Review/all_elemMatch_count":                             true,
	"TestArrayIndexPath_Review/all_elemMatch_deleteMany":                        true,
	"TestArrayIndexPath_Review/all_elemMatch_find":                              true,
	"TestArrayIndexPath_Review/all_elemMatch_findAndModify_remove":              true,
	"TestArrayIndexPath_Review/all_elemMatch_hinted_find":                       true,
	"TestArrayIndexPath_Review/all_elemMatch_nested_index_count":                true,
	"TestArrayIndexPath_Review/all_elemMatch_nested_index_deleteMany":           true,
	"TestArrayIndexPath_Review/all_elemMatch_nested_index_find":                 true,
	"TestArrayIndexPath_Review/all_elemMatch_nested_index_findAndModify_remove": true,
	"TestArrayIndexPath_Review/all_elemMatch_nested_index_hinted_find":          true,
	"TestArrayIndexPath_Review/all_elemMatch_nested_index_updateMany":           true,
	"TestArrayIndexPath_Review/all_elemMatch_updateMany":                        true,
	"TestArrayIndexPath_Review/all_two_elemMatch_count":                         true,
	"TestArrayIndexPath_Review/all_two_elemMatch_deleteMany":                    true,
	"TestArrayIndexPath_Review/all_two_elemMatch_find":                          true,
	"TestArrayIndexPath_Review/all_two_elemMatch_findAndModify_remove":          true,
	"TestArrayIndexPath_Review/all_two_elemMatch_hinted_find":                   true,
	"TestArrayIndexPath_Review/all_two_elemMatch_updateMany":                    true,
	"TestArrayIndexPath_Review/elemMatch_all_elemMatch_count":                   true,
	"TestArrayIndexPath_Review/elemMatch_all_elemMatch_deleteMany":              true,
	"TestArrayIndexPath_Review/elemMatch_all_elemMatch_find":                    true,
	"TestArrayIndexPath_Review/elemMatch_all_elemMatch_findAndModify_remove":    true,
	"TestArrayIndexPath_Review/elemMatch_all_elemMatch_hinted_find":             true,
	"TestArrayIndexPath_Review/elemMatch_all_elemMatch_updateMany":              true,
	"TestArrayIndexPath_Review/field_zero_array_count":                          true,
	"TestArrayIndexPath_Review/field_zero_array_deleteMany":                     true,
	"TestArrayIndexPath_Review/field_zero_array_find":                           true,
	"TestArrayIndexPath_Review/field_zero_array_findAndModify_remove":           true,
	"TestArrayIndexPath_Review/field_zero_array_hinted_find":                    true,
	"TestArrayIndexPath_Review/field_zero_array_updateMany":                     true,
	"TestArrayIndexPath_Review/field_zero_eq_count":                             true,
	"TestArrayIndexPath_Review/field_zero_eq_deleteMany":                        true,
	"TestArrayIndexPath_Review/field_zero_eq_find":                              true,
	"TestArrayIndexPath_Review/field_zero_eq_findAndModify_remove":              true,
	"TestArrayIndexPath_Review/field_zero_eq_hinted_find":                       true,
	"TestArrayIndexPath_Review/field_zero_eq_updateMany":                        true,
	"TestArrayIndexPath_Review/field_zero_exists_hinted_find":                   true,
	"TestArrayIndexPath_Review/field_zero_gt_count":                             true,
	"TestArrayIndexPath_Review/field_zero_gt_deleteMany":                        true,
	"TestArrayIndexPath_Review/field_zero_gt_find":                              true,
	"TestArrayIndexPath_Review/field_zero_gt_findAndModify_remove":              true,
	"TestArrayIndexPath_Review/field_zero_gt_hinted_find":                       true,
	"TestArrayIndexPath_Review/field_zero_gt_updateMany":                        true,
	"TestArrayIndexPath_Review/field_zero_in_count":                             true,
	"TestArrayIndexPath_Review/field_zero_in_deleteMany":                        true,
	"TestArrayIndexPath_Review/field_zero_in_find":                              true,
	"TestArrayIndexPath_Review/field_zero_in_findAndModify_remove":              true,
	"TestArrayIndexPath_Review/field_zero_in_hinted_find":                       true,
	"TestArrayIndexPath_Review/field_zero_in_updateMany":                        true,
	"TestArrayIndexPath_Review/field_zero_mod_count":                            true,
	"TestArrayIndexPath_Review/field_zero_mod_deleteMany":                       true,
	"TestArrayIndexPath_Review/field_zero_mod_find":                             true,
	"TestArrayIndexPath_Review/field_zero_mod_findAndModify_remove":             true,
	"TestArrayIndexPath_Review/field_zero_mod_hinted_find":                      true,
	"TestArrayIndexPath_Review/field_zero_mod_updateMany":                       true,
	"TestArrayIndexPath_Review/field_zero_ne_count":                             true,
	"TestArrayIndexPath_Review/field_zero_ne_deleteMany":                        true,
	"TestArrayIndexPath_Review/field_zero_ne_find":                              true,
	"TestArrayIndexPath_Review/field_zero_ne_hinted_find":                       true,
	"TestArrayIndexPath_Review/field_zero_ne_updateMany":                        true,
	"TestArrayIndexPath_Review/field_zero_nin_count":                            true,
	"TestArrayIndexPath_Review/field_zero_nin_deleteMany":                       true,
	"TestArrayIndexPath_Review/field_zero_nin_find":                             true,
	"TestArrayIndexPath_Review/field_zero_nin_hinted_find":                      true,
	"TestArrayIndexPath_Review/field_zero_nin_updateMany":                       true,
	"TestArrayIndexPath_Review/field_zero_nor_count":                            true,
	"TestArrayIndexPath_Review/field_zero_nor_deleteMany":                       true,
	"TestArrayIndexPath_Review/field_zero_nor_find":                             true,
	"TestArrayIndexPath_Review/field_zero_nor_hinted_find":                      true,
	"TestArrayIndexPath_Review/field_zero_nor_updateMany":                       true,
	"TestArrayIndexPath_Review/field_zero_not_count":                            true,
	"TestArrayIndexPath_Review/field_zero_not_deleteMany":                       true,
	"TestArrayIndexPath_Review/field_zero_not_find":                             true,
	"TestArrayIndexPath_Review/field_zero_not_hinted_find":                      true,
	"TestArrayIndexPath_Review/field_zero_not_updateMany":                       true,
	"TestArrayIndexPath_Review/field_zero_type_count":                           true,
	"TestArrayIndexPath_Review/field_zero_type_deleteMany":                      true,
	"TestArrayIndexPath_Review/field_zero_type_find":                            true,
	"TestArrayIndexPath_Review/field_zero_type_findAndModify_remove":            true,
	"TestArrayIndexPath_Review/field_zero_type_hinted_find":                     true,
	"TestArrayIndexPath_Review/field_zero_type_updateMany":                      true,
	"TestArrayIndexPath_Review/nor_all_elemMatch_count":                         true,
	"TestArrayIndexPath_Review/nor_all_elemMatch_deleteMany":                    true,
	"TestArrayIndexPath_Review/nor_all_elemMatch_find":                          true,
	"TestArrayIndexPath_Review/nor_all_elemMatch_hinted_find":                   true,
	"TestArrayIndexPath_Review/nor_all_elemMatch_updateMany":                    true,
	"TestArrayIndexPath_Review/not_all_elemMatch_count":                         true,
	"TestArrayIndexPath_Review/not_all_elemMatch_deleteMany":                    true,
	"TestArrayIndexPath_Review/not_all_elemMatch_find":                          true,
	"TestArrayIndexPath_Review/not_all_elemMatch_findAndModify_remove":          true,
	"TestArrayIndexPath_Review/not_all_elemMatch_hinted_find":                   true,
	"TestArrayIndexPath_Review/not_all_elemMatch_updateMany":                    true,
}
