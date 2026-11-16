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
