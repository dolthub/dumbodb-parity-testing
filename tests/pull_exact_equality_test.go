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

// $pull with a non-document value and every $pullAll operand remove only the
// elements exactly equal to it, not an array element containing it; and
// mongod refuses $near, $nearSphere, $geoNear and $jsonSchema in a $pull
// condition, and $expr inside $elemMatch, before reading a document
// (dolthub/dumbodb#234).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// pullWriters applies update to the document with _id id through update,
// findAndModify and bulkWrite.
func pullWriters(id interface{}, update bson.D) []struct {
	name  string
	write func(ctx context.Context, col *mongo.Collection) error
} {
	filter := bson.D{{Key: "_id", Value: id}}
	return []struct {
		name  string
		write func(ctx context.Context, col *mongo.Collection) error
	}{
		{"update", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateOne(ctx, filter, update)
			return err
		}},
		{"findAndModify", func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetReturnDocument(options.After)).Err()
		}},
		{"bulkWrite", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update)})
			return err
		}},
	}
}

func TestPullExactEquality(t *testing.T) {
	ab := bson.D{{Key: "a", Value: 2}, {Key: "b", Value: 1}}
	doc := bson.D{{Key: "_id", Value: 1}, {Key: "arr", Value: bson.A{ab, bson.A{ab}, 3, bson.A{3, 4}, 3.0, int64(3)}}}
	updates := []struct {
		name   string
		update bson.D
	}{
		{"pull_scalar", bson.D{{Key: "$pull", Value: bson.D{{Key: "arr", Value: 3}}}}},
		{"pull_array", bson.D{{Key: "$pull", Value: bson.D{{Key: "arr", Value: bson.A{3, 4}}}}}},
		{"pullAll_scalar", bson.D{{Key: "$pullAll", Value: bson.D{{Key: "arr", Value: bson.A{3}}}}}},
		{"pullAll_document", bson.D{{Key: "$pullAll", Value: bson.D{{Key: "arr", Value: bson.A{ab}}}}}},
		{"pullAll_array", bson.D{{Key: "$pullAll", Value: bson.D{{Key: "arr", Value: bson.A{bson.A{ab}}}}}}},
		{"pull_document_condition", bson.D{{Key: "$pull", Value: bson.D{{Key: "arr", Value: bson.D{{Key: "a", Value: 2}}}}}}},
	}
	for _, u := range updates {
		for _, w := range pullWriters(1, u.update) {
			t.Run(u.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "PullExactEquality_" + u.name + "_" + w.name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(doc),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error { return w.write(ctx, col) })
					},
				})
			})
		}
	}
}

func TestPullConditionValidation(t *testing.T) {
	pull := func(cond interface{}) bson.D {
		return bson.D{{Key: "$pull", Value: bson.D{{Key: "arr", Value: cond}}}}
	}
	conditions := []struct {
		name string
		cond interface{}
	}{
		{"near", bson.D{{Key: "$near", Value: bson.A{0, 0}}}},
		{"nearSphere", bson.D{{Key: "$nearSphere", Value: bson.A{0, 0}}}},
		{"near_on_field", bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.A{0, 0}}}}}},
		{"geoNear_on_field", bson.D{{Key: "loc", Value: bson.D{{Key: "$geoNear", Value: bson.A{0, 0}}}}}},
		{"near_in_and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.A{0, 0}}}}}}}}},
		{"near_bad_operand", bson.D{{Key: "$near", Value: "x"}}},
		{"jsonSchema", bson.D{{Key: "$jsonSchema", Value: bson.D{}}}},
		{"jsonSchema_in_or", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "$jsonSchema", Value: bson.D{}}}}}}},
		{"expr", bson.D{{Key: "$expr", Value: true}}},
		{"expr_in_elemMatch_on_field", bson.D{{Key: "c", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$expr", Value: true}}}}}}},
		{"expr_in_elemMatch", bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$expr", Value: true}}}}},
	}
	doc := bson.D{{Key: "_id", Value: 1}, {Key: "arr", Value: bson.A{bson.A{1, 2}, bson.D{{Key: "c", Value: bson.A{1}}}}}}
	for _, c := range conditions {
		for _, target := range []struct {
			name string
			id   interface{}
		}{{"match", 1}, {"no_match", 99}} {
			for _, w := range pullWriters(target.id, pull(c.cond)) {
				t.Run(c.name+"_"+target.name+"_"+w.name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    "PullConditionValidation_" + c.name + "_" + target.name + "_" + w.name,
						Support: harness.DumboDBFull,
						Setup:   insertDocs(doc),
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							return writeOutcome(ctx, col, func() error { return w.write(ctx, col) })
						},
					})
				})
			}
		}
	}
}

// mongod refuses $expr inside $elemMatch in a find filter as well.
func TestElemMatchExprFilter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter bson.D
	}{
		{"top", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$expr", Value: true}}}}}}},
		{"nested", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "c", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$expr", Value: true}}}}}}}}}}},
		{"in_and", bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "$expr", Value: true}}}}}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ElemMatchExprFilter_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return findIDs(ctx, col, tc.filter)
				},
			})
		})
	}
}

// Operators inside a $pull condition's values are values, not operators; and
// $jsonSchema and $where are refused inside $elemMatch too.
func TestPullConditionOperands(t *testing.T) {
	near := bson.D{{Key: "$near", Value: bson.A{0, 0}}}
	conditions := []struct {
		name string
		cond interface{}
	}{
		{"eq_operand", bson.D{{Key: "l", Value: bson.D{{Key: "$eq", Value: near}}}}},
		{"literal_document", bson.D{{Key: "l", Value: bson.D{{Key: "x", Value: near}}}}},
		{"ne_operand", bson.D{{Key: "$ne", Value: near}}},
		{"in_document", bson.D{{Key: "$in", Value: bson.A{bson.D{{Key: "a", Value: near}}}}}},
		{"in_operator", bson.D{{Key: "$in", Value: bson.A{near}}}},
		{"near_in_not", bson.D{{Key: "c", Value: bson.D{{Key: "$not", Value: near}}}}},
		{"near_in_elemMatch", bson.D{{Key: "c", Value: bson.D{{Key: "$elemMatch", Value: near}}}}},
		{"near_in_elemMatch_query", bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "l", Value: near}}}}},
		{"near_number", bson.D{{Key: "$near", Value: 5}}},
		{"jsonSchema_in_field_elemMatch", bson.D{{Key: "c", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$jsonSchema", Value: bson.D{}}}}}}}},
		{"jsonSchema_in_elemMatch", bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$jsonSchema", Value: bson.D{}}}}}},
		{"where_in_elemMatch", bson.D{{Key: "c", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$where", Value: "1"}}}}}}},
		{"where", bson.D{{Key: "$where", Value: "1"}}},
		{"text_in_elemMatch", bson.D{{Key: "c", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$text", Value: bson.D{{Key: "$search", Value: "x"}}}}}}}}},
	}
	doc := bson.D{{Key: "_id", Value: 1}, {Key: "arr", Value: bson.A{bson.D{{Key: "l", Value: 1}}, bson.D{{Key: "c", Value: bson.A{1}}}}}}
	for _, c := range conditions {
		for _, target := range []struct {
			name string
			id   interface{}
		}{{"match", 1}, {"no_match", 99}} {
			for _, w := range pullWriters(target.id, bson.D{{Key: "$pull", Value: bson.D{{Key: "arr", Value: c.cond}}}}) {
				t.Run(c.name+"_"+target.name+"_"+w.name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    "PullConditionOperands_" + c.name + "_" + target.name + "_" + w.name,
						Support: supportFor(t, pullReviewXFail),
						Setup:   insertDocs(doc),
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							return writeOutcome(ctx, col, func() error { return w.write(ctx, col) })
						},
					})
				})
			}
		}
	}
}

// $pull and $pullAll compare strings under the update's collation, or the
// collection's default one, and $pull with a regex removes the strings it
// matches.
func TestPullCollationAndRegex(t *testing.T) {
	caseless := &options.Collation{Locale: "en", Strength: 2}
	pull := func(v interface{}) bson.D { return bson.D{{Key: "$pull", Value: bson.D{{Key: "arr", Value: v}}}} }
	pullAll := func(v ...interface{}) bson.D {
		return bson.D{{Key: "$pullAll", Value: bson.D{{Key: "arr", Value: bson.A(v)}}}}
	}
	doc := bson.D{{Key: "_id", Value: 1}, {Key: "arr", Value: bson.A{
		"A", "a", "b", "abc", "xyz", bson.A{"abc"}, bson.A{"A"}, bson.D{{Key: "k", Value: "A"}},
		primitive.Regex{Pattern: "a"},
	}}}
	for _, tc := range []struct {
		name      string
		update    bson.D
		collation *options.Collation
		defaulted bool
	}{
		{"pull_string", pull("a"), caseless, false},
		{"pull_eq", pull(bson.D{{Key: "$eq", Value: "a"}}), caseless, false},
		{"pull_in", pull(bson.D{{Key: "$in", Value: bson.A{"a"}}}), caseless, false},
		{"pullAll_nested", pullAll(bson.A{"a"}, bson.D{{Key: "k", Value: "a"}}), caseless, false},
		{"pull_string_collection_default", pull("a"), caseless, true},
		{"pullAll_collection_default", pullAll("a", bson.A{"a"}), caseless, true},
		{"pull_string_simple", pull("a"), nil, false},
		{"pull_regex", pull(primitive.Regex{Pattern: "a"}), nil, false},
		{"pull_regex_options", pull(primitive.Regex{Pattern: "^X", Options: "i"}), nil, false},
		{"pullAll_regex", pullAll(primitive.Regex{Pattern: "a"}), nil, false},
	} {
		for _, w := range []string{"update", "findAndModify"} {
			t.Run(tc.name+"_"+w, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "PullCollationAndRegex_" + tc.name + "_" + w,
					Support: supportFor(t, pullReviewXFail),
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if tc.defaulted {
							if err := col.Drop(ctx); err != nil {
								return err
							}
							if err := col.Database().CreateCollection(ctx, col.Name(), options.CreateCollection().SetCollation(tc.collation)); err != nil {
								return err
							}
						}
						_, err := col.InsertOne(ctx, doc)
						return err
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						filter := bson.D{{Key: "_id", Value: 1}}
						return writeOutcome(ctx, col, func() error {
							if w == "update" {
								opts := options.Update()
								if tc.collation != nil && !tc.defaulted {
									opts.SetCollation(tc.collation)
								}
								_, err := col.UpdateOne(ctx, filter, tc.update, opts)
								return err
							}
							opts := options.FindOneAndUpdate()
							if tc.collation != nil && !tc.defaulted {
								opts.SetCollation(tc.collation)
							}
							return col.FindOneAndUpdate(ctx, filter, tc.update, opts).Err()
						})
					},
				})
			})
		}
	}
}

// pullReviewXFail lists the review cases DumboDB fails until the follow-up fix for
// dolthub/dumbodb#234.
var pullReviewXFail = map[string]bool{
	"TestPullCollationAndRegex/pullAll_collection_default_findAndModify":             true,
	"TestPullCollationAndRegex/pullAll_collection_default_update":                    true,
	"TestPullCollationAndRegex/pullAll_nested_findAndModify":                         true,
	"TestPullCollationAndRegex/pullAll_nested_update":                                true,
	"TestPullCollationAndRegex/pull_eq_findAndModify":                                true,
	"TestPullCollationAndRegex/pull_eq_update":                                       true,
	"TestPullCollationAndRegex/pull_in_findAndModify":                                true,
	"TestPullCollationAndRegex/pull_in_update":                                       true,
	"TestPullCollationAndRegex/pull_regex_findAndModify":                             true,
	"TestPullCollationAndRegex/pull_regex_options_findAndModify":                     true,
	"TestPullCollationAndRegex/pull_regex_options_update":                            true,
	"TestPullCollationAndRegex/pull_regex_update":                                    true,
	"TestPullCollationAndRegex/pull_string_collection_default_findAndModify":         true,
	"TestPullCollationAndRegex/pull_string_collection_default_update":                true,
	"TestPullCollationAndRegex/pull_string_findAndModify":                            true,
	"TestPullCollationAndRegex/pull_string_update":                                   true,
	"TestPullConditionOperands/eq_operand_match_bulkWrite":                           true,
	"TestPullConditionOperands/eq_operand_match_findAndModify":                       true,
	"TestPullConditionOperands/eq_operand_match_update":                              true,
	"TestPullConditionOperands/eq_operand_no_match_bulkWrite":                        true,
	"TestPullConditionOperands/eq_operand_no_match_findAndModify":                    true,
	"TestPullConditionOperands/eq_operand_no_match_update":                           true,
	"TestPullConditionOperands/in_document_match_bulkWrite":                          true,
	"TestPullConditionOperands/in_document_match_findAndModify":                      true,
	"TestPullConditionOperands/in_document_match_update":                             true,
	"TestPullConditionOperands/in_document_no_match_bulkWrite":                       true,
	"TestPullConditionOperands/in_document_no_match_findAndModify":                   true,
	"TestPullConditionOperands/in_document_no_match_update":                          true,
	"TestPullConditionOperands/in_operator_match_bulkWrite":                          true,
	"TestPullConditionOperands/in_operator_match_findAndModify":                      true,
	"TestPullConditionOperands/in_operator_match_update":                             true,
	"TestPullConditionOperands/in_operator_no_match_bulkWrite":                       true,
	"TestPullConditionOperands/in_operator_no_match_findAndModify":                   true,
	"TestPullConditionOperands/in_operator_no_match_update":                          true,
	"TestPullConditionOperands/jsonSchema_in_elemMatch_match_bulkWrite":              true,
	"TestPullConditionOperands/jsonSchema_in_elemMatch_match_findAndModify":          true,
	"TestPullConditionOperands/jsonSchema_in_elemMatch_match_update":                 true,
	"TestPullConditionOperands/jsonSchema_in_elemMatch_no_match_bulkWrite":           true,
	"TestPullConditionOperands/jsonSchema_in_elemMatch_no_match_findAndModify":       true,
	"TestPullConditionOperands/jsonSchema_in_elemMatch_no_match_update":              true,
	"TestPullConditionOperands/jsonSchema_in_field_elemMatch_match_bulkWrite":        true,
	"TestPullConditionOperands/jsonSchema_in_field_elemMatch_match_findAndModify":    true,
	"TestPullConditionOperands/jsonSchema_in_field_elemMatch_match_update":           true,
	"TestPullConditionOperands/jsonSchema_in_field_elemMatch_no_match_bulkWrite":     true,
	"TestPullConditionOperands/jsonSchema_in_field_elemMatch_no_match_findAndModify": true,
	"TestPullConditionOperands/jsonSchema_in_field_elemMatch_no_match_update":        true,
	"TestPullConditionOperands/literal_document_match_bulkWrite":                     true,
	"TestPullConditionOperands/literal_document_match_findAndModify":                 true,
	"TestPullConditionOperands/literal_document_match_update":                        true,
	"TestPullConditionOperands/literal_document_no_match_bulkWrite":                  true,
	"TestPullConditionOperands/literal_document_no_match_findAndModify":              true,
	"TestPullConditionOperands/literal_document_no_match_update":                     true,
	"TestPullConditionOperands/ne_operand_match_bulkWrite":                           true,
	"TestPullConditionOperands/ne_operand_match_findAndModify":                       true,
	"TestPullConditionOperands/ne_operand_match_update":                              true,
	"TestPullConditionOperands/ne_operand_no_match_bulkWrite":                        true,
	"TestPullConditionOperands/ne_operand_no_match_findAndModify":                    true,
	"TestPullConditionOperands/ne_operand_no_match_update":                           true,
}
