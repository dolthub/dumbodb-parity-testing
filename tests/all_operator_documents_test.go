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
