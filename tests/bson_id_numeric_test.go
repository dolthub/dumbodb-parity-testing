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
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// MongoDB compares numeric values across BSON subtypes: a query for {_id: 42}
// matches a stored _id regardless of whether it was written as int32, int64,
// or double, including through the _id index. TestBSON_int32_vs_int64_equality
// asserts this on a regular field. These tests assert the same equivalence on
// _id, which DumboDB matches by value once numeric _id values are normalised
// to a canonical form before hashing. Full mode: divergence fails CI.

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
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(int32(42), int64(42)),
	})
}

func TestBSON_id_int32_stored_query_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int32_stored_query_double",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(int32(42), float64(42)),
	})
}

func TestBSON_id_int64_stored_query_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int64_stored_query_int32",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(int64(42), int32(42)),
	})
}

func TestBSON_id_int64_stored_query_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int64_stored_query_double",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(int64(42), float64(42)),
	})
}

func TestBSON_id_double_stored_query_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_int32",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(float64(42), int32(42)),
	})
}

func TestBSON_id_double_stored_query_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_int64",
		Support: harness.DumboDBFull,
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
		Support: harness.DumboDBFull,
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

func decID(t *testing.T, s string) primitive.Decimal128 {
	t.Helper()
	d, err := primitive.ParseDecimal128(s)
	if err != nil {
		t.Fatalf("parse decimal %q: %v", s, err)
	}
	return d
}

func TestBSON_id_decimal_stored_query_int32(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_decimal_stored_query_int32",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(decID(t, "42"), int32(42)),
	})
}

func TestBSON_id_decimal_stored_query_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_decimal_stored_query_int64",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(decID(t, "42"), int64(42)),
	})
}

func TestBSON_id_decimal_stored_query_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_decimal_stored_query_double",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(decID(t, "42"), float64(42)),
	})
}

func TestBSON_id_int64_stored_query_decimal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_int64_stored_query_decimal",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(int64(42), decID(t, "42")),
	})
}

func TestBSON_id_double_stored_query_decimal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_decimal",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(float64(42), decID(t, "42")),
	})
}

// Non-integer numeric _id equality. MongoDB compares by exact value, so a
// double and a decimal that represent the same value (0.5, 42.5 - both exactly
// representable in binary and decimal) are equal, and decimals differing only
// in scale (0.10 vs 0.1) are equal. double 0.1 and decimal 0.1 are NOT equal
// (0.1 is inexact in binary), which both servers already agree on.

func TestBSON_id_double_stored_query_decimal_half(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_decimal_half",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(float64(0.5), decID(t, "0.5")),
	})
}

func TestBSON_id_decimal_stored_query_double_half(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_decimal_stored_query_double_half",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(decID(t, "0.5"), float64(0.5)),
	})
}

func TestBSON_id_double_stored_query_decimal_425(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_stored_query_decimal_425",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(float64(42.5), decID(t, "42.5")),
	})
}

func TestBSON_id_decimal_scale_tenths(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_decimal_scale_tenths",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(decID(t, "0.10"), decID(t, "0.1")),
	})
}

func TestBSON_id_decimal_scale_25(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_decimal_scale_25",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(decID(t, "2.50"), decID(t, "2.5")),
	})
}

func TestBSON_id_double_inf_vs_decimal_inf(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_inf_vs_decimal_inf",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(math.Inf(1), decID(t, "Infinity")),
	})
}

func TestBSON_id_double_nan_vs_decimal_nan(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_double_nan_vs_decimal_nan",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(math.NaN(), decID(t, "NaN")),
	})
}

// Composite _id: MongoDB matches embedded-document _ids field by field with
// value-based numeric equality, so {x: NumberLong(42)} equals {x: 42.0}.
func TestBSON_id_doc_nested_long_vs_double(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_nested_long_vs_double",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			bson.D{{Key: "x", Value: int64(42)}},
			bson.D{{Key: "x", Value: float64(42)}},
		),
	})
}

func TestBSON_id_doc_nested_int32_vs_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_nested_int32_vs_int64",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			bson.D{{Key: "a", Value: int32(42)}},
			bson.D{{Key: "a", Value: int64(42)}},
		),
	})
}

func TestBSON_id_doc_nested_int32_vs_decimal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_nested_int32_vs_decimal",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			bson.D{{Key: "a", Value: int32(42)}},
			bson.D{{Key: "a", Value: decID(t, "42")}},
		),
	})
}

func TestBSON_id_doc_deep_nested_int32_vs_int64(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_deep_nested_int32_vs_int64",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: int32(42)}}}},
			bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: int64(42)}}}},
		),
	})
}

// MongoDB embedded-document _id equality is field-order sensitive: {a,b} and
// {b,a} are distinct _ids, so a query for one must not match the other.
func TestBSON_id_doc_field_order_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_field_order_distinct",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}},
			bson.D{{Key: "b", Value: int32(2)}, {Key: "a", Value: int32(1)}},
		),
	})
}

// Non-integer numeric values nested in a composite _id must also match by
// exact value, as they do at the top level.
func TestBSON_id_doc_nested_double_half_vs_decimal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_nested_double_half_vs_decimal",
		Support: harness.DumboDBXFail,
		Run: idCrossTypeCount(
			bson.D{{Key: "a", Value: float64(0.5)}},
			bson.D{{Key: "a", Value: decID(t, "0.5")}},
		),
	})
}

func TestBSON_id_doc_nested_decimal_scale(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_nested_decimal_scale",
		Support: harness.DumboDBXFail,
		Run: idCrossTypeCount(
			bson.D{{Key: "a", Value: decID(t, "0.10")}},
			bson.D{{Key: "a", Value: decID(t, "0.1")}},
		),
	})
}
