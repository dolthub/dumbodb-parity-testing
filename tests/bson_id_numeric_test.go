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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// MongoDB compares numeric values across BSON subtypes: a query for {_id: 42}
// matches a stored _id regardless of whether it was written as int32, int64,
// or double, including through the _id index. TestBSON_int32_vs_int64_equality
// asserts this on a regular field. These tests assert the same equivalence on
// _id, where DumboDB currently requires an exact numeric-subtype match and so
// misses the document. They are XFail until DumboDB matches by value on _id.

// idCrossTypeCount stores one document whose _id is stored, then counts
// documents matching {_id: query}. MongoDB returns 1 for any value-equal
// numeric query; the returned count is what the harness compares.
func idCrossTypeCount(stored, query interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: stored}, {Key: "v", Value: "x"}}); err != nil {
			return nil, err
		}
		count, err := col.CountDocuments(ctx, bson.D{{Key: "_id", Value: query}})
		if err != nil {
			return nil, err
		}
		return bson.D{{Key: "count", Value: count}}, nil
	}
}

func TestBSON_id_int32_stored_query_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int32_stored_query_int64",
		Support: harness.DumboDBXFail,
		Run:     idCrossTypeCount(int32(42), int64(42)),
	})
}

func TestBSON_id_int32_stored_query_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int32_stored_query_double",
		Support: harness.DumboDBXFail,
		Run:     idCrossTypeCount(int32(42), float64(42)),
	})
}

func TestBSON_id_int64_stored_query_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int64_stored_query_int32",
		Support: harness.DumboDBXFail,
		Run:     idCrossTypeCount(int64(42), int32(42)),
	})
}

func TestBSON_id_int64_stored_query_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int64_stored_query_double",
		Support: harness.DumboDBXFail,
		Run:     idCrossTypeCount(int64(42), float64(42)),
	})
}

func TestBSON_id_double_stored_query_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_int32",
		Support: harness.DumboDBXFail,
		Run:     idCrossTypeCount(float64(42), int32(42)),
	})
}

func TestBSON_id_double_stored_query_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_int64",
		Support: harness.DumboDBXFail,
		Run:     idCrossTypeCount(float64(42), int64(42)),
	})
}

// TestBSON_id_int64_stored_findone_double reproduces the mongo-express document
// lookup that surfaced this gap: a document is inserted with an int64 _id, then
// fetched with a value-equal double _id. MongoDB returns the document; DumboDB
// returns ErrNoDocuments. Uses _id 0 to mirror the mongo-express Long test.
func TestBSON_id_int64_stored_findone_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int64_stored_findone_double",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int64(0)}, {Key: "label", Value: "zero"}}); err != nil {
				return nil, err
			}
			var result bson.D
			err := col.FindOne(ctx, bson.D{{Key: "_id", Value: float64(0)}}).Decode(&result)
			return result, err
		},
	})
}
