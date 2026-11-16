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

// $all takes values, regexes and, when its first element is one, $elemMatch
// documents; mongod refuses any other operator document in it, and a mix of
// $elemMatch and plain elements, before reading a document
// (dolthub/dumbodb#289).

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestAllOperatorDocuments(t *testing.T) {
	all := func(elems ...interface{}) bson.D {
		return bson.D{{Key: "x", Value: bson.D{{Key: "$all", Value: bson.A(elems)}}}}
	}
	elemMatch := bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: "q"}}}}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"regex_document", all(bson.D{{Key: "$regex", Value: "q"}})},
		{"eq_document", all(bson.D{{Key: "$eq", Value: "q"}})},
		{"gt_document", all("q", bson.D{{Key: "$gt", Value: 1}})},
		{"elemMatch_after_value", all("q", elemMatch)},
		{"value_after_elemMatch", all(elemMatch, "q")},
		{"elemMatch_only", all(elemMatch)},
		{"regex_literal", all(primitive.Regex{Pattern: "q"})},
		{"plain_document", all(bson.D{{Key: "a", Value: 1}})},
		{"empty", all()},
		{"nested_under_not", bson.D{{Key: "x", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$all", Value: bson.A{bson.D{{Key: "$regex", Value: "q"}}}}}}}}}},
		{"nested_under_or", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "y", Value: 1}}, all(bson.D{{Key: "$regex", Value: "q"}})}}}},
	}
	docs := []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "x", Value: bson.A{"q"}}},
		bson.D{{Key: "_id", Value: 2}, {Key: "x", Value: bson.A{bson.D{{Key: "a", Value: 1}}}}},
	}

	for _, tc := range filters {
		t.Run("find_"+tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AllOperatorDocuments_find_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDocs(docs...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return findIDs(ctx, col, tc.filter)
				},
			})
		})
		t.Run("empty_collection_"+tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AllOperatorDocuments_empty_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return col.CountDocuments(ctx, tc.filter)
				},
			})
		})
	}

	bad := all(bson.D{{Key: "$regex", Value: "q"}})
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "y", Value: 1}}}}
	for _, tc := range []struct {
		name  string
		write func(ctx context.Context, col *mongo.Collection) error
	}{
		{"updateOne", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateOne(ctx, bad, set)
			return err
		}},
		{"deleteMany", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteMany(ctx, bad)
			return err
		}},
		{"findAndModify", func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, bad, set).Err()
		}},
		{"bulkWrite", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{mongo.NewUpdateOneModel().SetFilter(bad).SetUpdate(set)})
			return err
		}},
	} {
		t.Run("write_"+tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AllOperatorDocuments_write_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDocs(docs...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error { return tc.write(ctx, col) })
				},
			})
		})
	}
}

// mongod validates the $elemMatch operands of $all, and classifies each $all
// element by its first key as sent, before any document is read.
func TestAllOperatorDocumentsNested(t *testing.T) {
	elemMatch := func(v bson.D) bson.D { return bson.D{{Key: "$elemMatch", Value: v}} }
	all := func(elems ...interface{}) bson.D { return bson.D{{Key: "$all", Value: bson.A(elems)}} }
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"not_all_bad_elemMatch", bson.D{{Key: "a", Value: bson.D{{Key: "$not", Value: all(elemMatch(bson.D{{Key: "$foo", Value: 1}}))}}}}},
		{"nor_all_nested_operator", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "a", Value: all(elemMatch(bson.D{{Key: "b", Value: all(bson.D{{Key: "$lt", Value: 1}})}}))}}}}}},
		{"literal_before_elemMatch_key", bson.D{{Key: "a", Value: all(bson.D{{Key: "z", Value: 1}, {Key: "$elemMatch", Value: bson.D{{Key: "$gt", Value: 0}}}})}}},
		{"operator_before_elemMatch_key", bson.D{{Key: "a", Value: bson.D{{Key: "$not", Value: all(bson.D{{Key: "$gt", Value: 1}, {Key: "$elemMatch", Value: bson.D{{Key: "$gt", Value: 1}}}})}}}}},
	}
	docs := []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 5}},
		bson.D{{Key: "_id", Value: 2}, {Key: "x", Value: 1}},
		bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: bson.D{{Key: "b", Value: 1}}}},
		bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: bson.A{bson.D{{Key: "z", Value: 1}}}}},
		bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: bson.A{5}}},
	}
	// The malformed $elemMatch cases need documents without arrays at a,
	// where nothing evaluates the operand.
	noArrays := docs[:3]
	for _, tc := range filters {
		tcDocs := docs
		if strings.HasPrefix(tc.name, "not_all") || strings.HasPrefix(tc.name, "nor_all") {
			tcDocs = noArrays
		}
		t.Run(tc.name+"_find", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AllOperatorDocumentsNested_" + tc.name + "_find",
				Support: harness.DumboDBFull,
				Setup:   insertDocs(tcDocs...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return findIDs(ctx, col, tc.filter)
				},
			})
		})
		t.Run(tc.name+"_deleteMany", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AllOperatorDocumentsNested_" + tc.name + "_deleteMany",
				Support: harness.DumboDBFull,
				Setup:   insertDocs(tcDocs...),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.DeleteMany(ctx, tc.filter)
						return err
					})
				},
			})
		})
	}
}

// mongod accepts $near, $nearSphere and $geoNear only at the top level of a
// filter, in $and, and in a one-clause $or, and refuses them elsewhere before
// reading a document.
func TestNearPlacement(t *testing.T) {
	near := bson.D{{Key: "$near", Value: bson.A{0, 0}}}
	k := bson.D{{Key: "k", Value: near}}
	elemMatch := func(v interface{}) bson.D { return bson.D{{Key: "$elemMatch", Value: v}} }
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"all_elemMatch_elemMatch", bson.D{{Key: "arr", Value: bson.D{{Key: "$all", Value: bson.A{elemMatch(elemMatch(k))}}}}}},
		{"elemMatch_field", bson.D{{Key: "arr", Value: elemMatch(k)}}},
		{"elemMatch_operator", bson.D{{Key: "arr", Value: elemMatch(near)}}},
		{"elemMatch_nearSphere", bson.D{{Key: "arr", Value: elemMatch(bson.D{{Key: "k", Value: bson.D{{Key: "$nearSphere", Value: bson.A{0, 0}}}}})}}},
		{"elemMatch_or", bson.D{{Key: "arr", Value: elemMatch(bson.D{{Key: "$or", Value: bson.A{k}}})}}},
		{"not", bson.D{{Key: "loc", Value: bson.D{{Key: "$not", Value: near}}}}},
		{"nor", bson.D{{Key: "$nor", Value: bson.A{bson.D{{Key: "loc", Value: near}}}}}},
		{"or_two_clauses", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "loc", Value: near}}, bson.D{{Key: "_id", Value: 2}}}}}},
		{"or_one_clause", bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "loc", Value: near}}}}}},
		{"and", bson.D{{Key: "$and", Value: bson.A{bson.D{{Key: "loc", Value: near}}, bson.D{{Key: "_id", Value: 1}}}}}},
		{"top_level", bson.D{{Key: "loc", Value: near}}},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2d"}}}); err != nil {
			return err
		}
		return insertDocs(
			bson.D{{Key: "_id", Value: 1}, {Key: "loc", Value: bson.A{0, 0}}, {Key: "arr", Value: bson.A{bson.A{bson.D{{Key: "k", Value: bson.A{0, 0}}}}}}},
			bson.D{{Key: "_id", Value: 2}, {Key: "loc", Value: bson.A{5, 5}}, {Key: "arr", Value: 5}},
		)(ctx, col)
	}
	for _, tc := range filters {
		t.Run(tc.name+"_find", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "NearPlacement_" + tc.name + "_find",
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return findIDs(ctx, col, tc.filter)
				},
			})
		})
		t.Run(tc.name+"_deleteMany", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "NearPlacement_" + tc.name + "_deleteMany",
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.DeleteMany(ctx, tc.filter)
						return err
					})
				},
			})
		})
	}
}

// allDollarKeyLiteralsXFail lists the cases DumboDB fails until $all
// classifies elements by their first key as written.
var allDollarKeyLiteralsXFail = map[string]bool{
	"TestAllDollarKeyLiterals/dbref_count":                             true,
	"TestAllDollarKeyLiterals/dbref_find":                              true,
	"TestAllDollarKeyLiterals/dbref_updateMany":                        true,
	"TestAllDollarKeyLiterals/later_dollar_key_after_value_count":      true,
	"TestAllDollarKeyLiterals/later_dollar_key_after_value_find":       true,
	"TestAllDollarKeyLiterals/later_dollar_key_after_value_updateMany": true,
	"TestAllDollarKeyLiterals/later_dollar_key_count":                  true,
	"TestAllDollarKeyLiterals/later_dollar_key_find":                   true,
	"TestAllDollarKeyLiterals/later_dollar_key_updateMany":             true,
	"TestAllDollarKeyLiterals/later_elemMatch_key_count":               true,
	"TestAllDollarKeyLiterals/later_elemMatch_key_find":                true,
	"TestAllDollarKeyLiterals/later_elemMatch_key_updateMany":          true,
	"TestAllDollarKeyLiterals/later_gt_key_count":                      true,
	"TestAllDollarKeyLiterals/later_gt_key_find":                       true,
	"TestAllDollarKeyLiterals/later_gt_key_updateMany":                 true,
	"TestAllDollarKeyLiterals/maxDistance_first_key_count":             true,
	"TestAllDollarKeyLiterals/maxDistance_first_key_find":              true,
	"TestAllDollarKeyLiterals/maxDistance_first_key_updateMany":        true,
	"TestAllDollarKeyLiterals/not_first_key_count":                     true,
	"TestAllDollarKeyLiterals/not_first_key_find":                      true,
	"TestAllDollarKeyLiterals/not_first_key_updateMany":                true,
	"TestAllDollarKeyLiterals/pull":                                    true,
	"TestAllDollarKeyLiterals/unknown_first_key_count":                 true,
	"TestAllDollarKeyLiterals/unknown_first_key_find":                  true,
	"TestAllDollarKeyLiterals/unknown_first_key_stored_count":          true,
	"TestAllDollarKeyLiterals/unknown_first_key_stored_find":           true,
	"TestAllDollarKeyLiterals/unknown_first_key_stored_updateMany":     true,
	"TestAllDollarKeyLiterals/unknown_first_key_updateMany":            true,
}

// mongod classifies an $all element by its first key as written and refuses
// only the operators it knows; any other '$' name is a literal value.
func TestAllDollarKeyLiterals(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		var out bson.D
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	all := func(elems ...interface{}) bson.D {
		return d("x", d("$all", bson.A(elems)))
	}
	docs := []interface{}{
		d("_id", 1, "x", bson.A{d("a", 1, "$b", 2)}),
		d("_id", 2, "x", bson.A{5, d("a", d("d", 1, "c", 2), "$b", 2)}),
		d("_id", 3, "x", bson.A{d("$b", 2)}),
		d("_id", 4, "x", bson.A{d("$ref", "c", "$id", 1)}),
		d("_id", 5, "x", bson.A{d("z", 1, "$elemMatch", d("q", 1))}),
		d("_id", 6, "x", bson.A{d("a", 1, "$gt", 1)}),
		d("_id", 7, "x", bson.A{d("$not", 1)}),
	}
	for _, c := range []struct {
		name   string
		filter bson.D
	}{
		{"later_dollar_key", all(d("a", 1, "$b", 2))},
		{"later_dollar_key_after_value", all(5, d("a", d("d", 1, "c", 2), "$b", 2))},
		{"unknown_first_key", all(d("$foo", 1))},
		{"unknown_first_key_stored", all(d("$b", 2))},
		{"dbref", all(d("$ref", "c", "$id", 1))},
		{"later_elemMatch_key", all(d("z", 1, "$elemMatch", d("q", 1)))},
		{"later_gt_key", all(d("a", 1, "$gt", 1))},
		{"not_first_key", all(d("$not", 1))},
		{"maxDistance_first_key", all(d("$maxDistance", 1))},
		{"eq_first_key", all(d("$eq", 1))},
		{"regex_first_key", all(d("$regex", "a"))},
		{"options_first_key", all(d("$options", "i"))},
		{"in_first_key_after_value", all(5, d("$in", bson.A{1}))},
	} {
		for _, w := range []struct {
			name string
			run  func(ctx context.Context, col *mongo.Collection) (interface{}, error)
		}{
			{"find", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return findIDs(ctx, col, c.filter)
			}},
			{"count", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return col.CountDocuments(ctx, c.filter)
			}},
			{"updateMany", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return writeOutcome(ctx, col, func() error {
					_, err := col.UpdateMany(ctx, c.filter, d("$set", d("hit", true)))
					return err
				})
			}},
		} {
			name := c.name + "_" + w.name
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "AllDollarKeyLiterals_" + name,
					Support: supportFor(t, allDollarKeyLiteralsXFail),
					Setup:   insertDocs(docs...),
					Run:     w.run,
				})
			})
		}
	}
	t.Run("pull", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "AllDollarKeyLiterals_pull",
			Support: supportFor(t, allDollarKeyLiteralsXFail),
			Setup:   insertDocs(d("_id", 1, "arr", bson.A{d("c", bson.A{d("a", 1, "$b", 2)}), d("c", bson.A{1})})),
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return writeOutcome(ctx, col, func() error {
					_, err := col.UpdateOne(ctx, d("_id", 1),
						d("$pull", d("arr", d("c", d("$all", bson.A{d("a", 1, "$b", 2)})))))
					return err
				})
			},
		})
	})
}
