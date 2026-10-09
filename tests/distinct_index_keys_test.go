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

// An unfiltered distinct whose key leads a usable index is answered by MongoDB
// with DISTINCT_SCAN and returns index keys: null for a missing field or branch,
// undefined for an empty array, and array keys flattened. Any filter, or no
// usable index, returns document values instead.

func distinctKeyDocs() []interface{} {
	b := func(v interface{}) bson.D { return bson.D{{Key: "b", Value: v}} }
	return []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: b(int32(1))}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: b(bson.A{})}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: bson.D{{Key: "c", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{b(int32(2)), bson.D{{Key: "c", Value: int32(1)}}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: b(bson.A{int32(3), bson.A{int32(4), int32(5)}})}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: b(nil)}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "a", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "a", Value: int32(9)}},
		bson.D{{Key: "_id", Value: int32(9)}, {Key: "x", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: int32(10)}, {Key: "x", Value: bson.A{bson.A{int32(1), int32(2)}, int32(3)}}},
		bson.D{{Key: "_id", Value: int32(11)}, {Key: "y", Value: bson.D{{Key: "p", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(12)}, {Key: "y", Value: bson.D{{Key: "p", Value: int32(2)}}}},
		bson.D{{Key: "_id", Value: int32(13)}, {Key: "y", Value: bson.A{bson.A{int32(7)}, bson.A{int32(8)}}}},
	}
}

func TestDistinct_IndexKeys(t *testing.T) {
	ab := bson.D{{Key: "a.b", Value: 1}}
	idDollar := bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}
	for _, tc := range []struct {
		name    string
		field   string
		index   *mongo.IndexModel
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"DottedNoIndex", "a.b", nil, bson.D{}, harness.DumboDBFull},
		{"DottedIndex", "a.b", &mongo.IndexModel{Keys: ab}, bson.D{}, harness.DumboDBFull},
		{"DottedIndexDescending", "a.b", &mongo.IndexModel{Keys: bson.D{{Key: "a.b", Value: -1}}}, bson.D{}, harness.DumboDBFull},
		{"DottedIndexSparse", "a.b", &mongo.IndexModel{Keys: ab, Options: options.Index().SetSparse(true)}, bson.D{}, harness.DumboDBFull},
		{"DottedIndexPartial", "a.b", &mongo.IndexModel{Keys: ab, Options: options.Index().SetPartialFilterExpression(idDollar)}, bson.D{}, harness.DumboDBFull},
		{"DottedCompoundLeading", "a.b", &mongo.IndexModel{Keys: bson.D{{Key: "a.b", Value: 1}, {Key: "_id", Value: 1}}}, bson.D{}, harness.DumboDBFull},
		{"DottedCompoundSecond", "a.b", &mongo.IndexModel{Keys: bson.D{{Key: "_id", Value: 1}, {Key: "a.b", Value: 1}}}, bson.D{}, harness.DumboDBFull},
		{"DottedIndexFilterOnKey", "a.b", &mongo.IndexModel{Keys: ab}, bson.D{{Key: "a.b", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}, harness.DumboDBFull},
		{"DottedIndexFilterOther", "a.b", &mongo.IndexModel{Keys: ab}, idDollar, harness.DumboDBFull},
		{"TopLevelNoIndex", "x", nil, bson.D{}, harness.DumboDBFull},
		{"TopLevelIndex", "x", &mongo.IndexModel{Keys: bson.D{{Key: "x", Value: 1}}}, bson.D{}, harness.DumboDBFull},
		// Distinct subdocuments and nested arrays share an index key byte
		// string; every one must still be returned (workspace-4tl.2).
		{"TopLevelIndexSubdocsAndArrays", "y", &mongo.IndexModel{Keys: bson.D{{Key: "y", Value: 1}}}, bson.D{}, harness.DumboDBXFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Distinct_IndexKeys_" + tc.name,
				Support: tc.support,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertMany(ctx, distinctKeyDocs()); err != nil {
						return err
					}
					if tc.index == nil {
						return nil
					}
					_, err := col.Indexes().CreateOne(ctx, *tc.index)
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					vals, err := col.Distinct(ctx, tc.field, tc.filter)
					if err != nil {
						return nil, err
					}
					return bson.A(vals), nil
				},
			})
		})
	}
}
