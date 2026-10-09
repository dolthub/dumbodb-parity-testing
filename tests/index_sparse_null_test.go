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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A sparse index skips only documents missing the indexed field. A document
// whose field is explicitly null is indexed: it collides on a sparse unique
// index, appears in distinct, and is found through a hinted sparse index.

const sparseNullIndexName = "sparse_null_idx"

func sparseNullSetup(field string, unique bool, docs ...interface{}) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if len(docs) > 0 {
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: field, Value: 1}},
			Options: options.Index().SetSparse(true).SetUnique(unique).SetName(sparseNullIndexName),
		})
		return err
	}
}

// sparseNullInsertEach inserts docs one at a time and records each outcome.
func sparseNullInsertEach(ctx context.Context, col *mongo.Collection, docs ...bson.D) (bson.D, error) {
	out := bson.D{}
	for i, d := range docs {
		_, err := col.InsertOne(ctx, d)
		out = append(out, bson.E{Key: fmt.Sprintf("insert%d", i+1), Value: comparableErrorInfo(err)})
	}
	ids, err := uniqIDs(ctx, col)
	if err != nil {
		return nil, err
	}
	return append(out, bson.E{Key: "present", Value: ids}), nil
}

func TestIndex_SparseNull_Unique(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_SparseNull_Unique",
		Support: harness.DumboDBFull,
		Setup:   sparseNullSetup("x", true),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sparseNullInsertEach(ctx, col,
				bson.D{{Key: "_id", Value: int32(1)}},
				bson.D{{Key: "_id", Value: int32(2)}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: nil}},
				bson.D{{Key: "_id", Value: int32(4)}, {Key: "x", Value: nil}},
				bson.D{{Key: "_id", Value: int32(5)}, {Key: "x", Value: int32(1)}},
			)
		},
	})
}

func TestIndex_SparseNull_UniqueDotted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_SparseNull_UniqueDotted",
		Support: harness.DumboDBFull,
		Setup:   sparseNullSetup("a.b", true),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sparseNullInsertEach(ctx, col,
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "b", Value: nil}}}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.D{{Key: "b", Value: nil}}}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: int32(5)}},
				bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: int32(5)}},
				bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.D{{Key: "c", Value: int32(1)}}}},
				bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: bson.D{{Key: "c", Value: int32(1)}}}},
			)
		},
	})
}

func TestIndex_SparseNull_UniqueUpdateToNull(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_SparseNull_UniqueUpdateToNull",
		Support: harness.DumboDBFull,
		Setup: sparseNullSetup("x", true,
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: nil}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: int32(1)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: int32(2)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: nil}}}})
			return bson.D{{Key: "update", Value: comparableErrorInfo(err)}}, nil
		},
	})
}

func TestIndex_SparseNull_Distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_SparseNull_Distinct",
		Support: harness.DumboDBFull,
		Setup: sparseNullSetup("x", false,
			bson.D{{Key: "_id", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: nil}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: int32(1)}},
		),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vals, err := col.Distinct(ctx, "x", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.A(vals), nil
		},
	})
}

func TestIndex_SparseNull_HintedQuery(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		// MongoDB answers from the hinted sparse index alone, so docs missing x
		// drop out even though {x: null} matches them.
		{"EqNull", bson.D{{Key: "x", Value: nil}}, harness.DumboDBFull},
		{"ExistsTrue", bson.D{{Key: "x", Value: bson.D{{Key: "$exists", Value: true}}}}, harness.DumboDBFull},
		{"TypeNull", bson.D{{Key: "x", Value: bson.D{{Key: "$type", Value: "null"}}}}, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_SparseNull_HintedQuery_" + tc.name,
				Support: tc.support,
				Setup: sparseNullSetup("x", false,
					bson.D{{Key: "_id", Value: int32(1)}},
					bson.D{{Key: "_id", Value: int32(2)}},
					bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: nil}},
					bson.D{{Key: "_id", Value: int32(4)}, {Key: "x", Value: int32(1)}},
				),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					ids, err := dottedFindIDs(ctx, col, tc.filter, nil, sparseNullIndexName)
					if err != nil {
						return nil, err
					}
					n, err := col.CountDocuments(ctx, tc.filter, options.Count().SetHint(sparseNullIndexName))
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "find", Value: ids}, {Key: "count", Value: n}}, nil
				},
			})
		})
	}
}
