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
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// insertIDCode inserts {_id: id} and returns the write-error code (0 on
// success), so acceptance parity is compared on the code, not error wording.
func insertIDCode(id interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}})
		return bson.D{{Key: "code", Value: writeErrCode(err)}}, nil
	}
}

func writeErrCode(err error) int32 {
	if err == nil {
		return 0
	}
	var we mongo.WriteException
	if errors.As(err, &we) && len(we.WriteErrors) > 0 {
		return int32(we.WriteErrors[0].Code)
	}
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return -1
}

// Non-numeric _id equality edge cases. _id equality is by exact type and value:
// no cross-type coercion outside the numeric family.

func TestBSON_id_binary_subtype_distinct(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_binary_subtype_distinct",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			primitive.Binary{Subtype: 0x00, Data: data},
			primitive.Binary{Subtype: 0x04, Data: data},
		),
	})
}

func TestBSON_id_binary_same_subtype_match(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_binary_same_subtype_match",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			primitive.Binary{Subtype: 0x00, Data: data},
			primitive.Binary{Subtype: 0x00, Data: data},
		),
	})
}

func TestBSON_id_date_vs_timestamp_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_date_vs_timestamp_distinct",
		Support: harness.DumboDBFull,
		Run: idCrossTypeCount(
			primitive.NewDateTimeFromTime(time.Unix(1, 0).UTC()),
			primitive.Timestamp{T: 1, I: 0},
		),
	})
}

func TestBSON_id_bool_vs_int_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_bool_vs_int_distinct",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(true, int32(1)),
	})
}

func TestBSON_id_minkey_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_minkey_match",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.MinKey{}, primitive.MinKey{}),
	})
}

func TestBSON_id_minkey_vs_maxkey_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_minkey_vs_maxkey_distinct",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.MinKey{}, primitive.MaxKey{}),
	})
}

func TestBSON_id_timestamp_match(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_timestamp_match",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.Timestamp{T: 5, I: 1}, primitive.Timestamp{T: 5, I: 1}),
	})
}

func TestBSON_id_timestamp_distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_timestamp_distinct",
		Support: harness.DumboDBFull,
		Run:     idCrossTypeCount(primitive.Timestamp{T: 5, I: 1}, primitive.Timestamp{T: 5, I: 2}),
	})
}

// A document _id may not contain $-prefixed field names (MongoDB code 52),
// except a valid DBRef ({$ref, $id[, $db]}). Dotted keys and non-leading $
// are allowed.

func TestBSON_id_doc_dollar_key_rejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_dollar_key_rejected",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "$a", Value: int32(1)}}),
	})
}

func TestBSON_id_doc_nested_dollar_key_rejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_nested_dollar_key_rejected",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "a", Value: bson.D{{Key: "$b", Value: int32(1)}}}}),
	})
}

func TestBSON_id_doc_dbref_allowed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_dbref_allowed",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "$ref", Value: "coll"}, {Key: "$id", Value: int32(1)}}),
	})
}

func TestBSON_id_doc_dbref_with_db_allowed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_dbref_with_db_allowed",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "$ref", Value: "coll"}, {Key: "$id", Value: int32(1)}, {Key: "$db", Value: "d"}}),
	})
}

func TestBSON_id_doc_dbref_extra_dollar_rejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_dbref_extra_dollar_rejected",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "$ref", Value: "coll"}, {Key: "$id", Value: int32(1)}, {Key: "$db", Value: "d"}, {Key: "$x", Value: int32(9)}}),
	})
}

func TestBSON_id_doc_dbref_reversed_rejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_dbref_reversed_rejected",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "$id", Value: int32(1)}, {Key: "$ref", Value: "coll"}}),
	})
}

func TestBSON_id_doc_dotted_key_allowed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_doc_dotted_key_allowed",
		Support: harness.DumboDBFull,
		Run:     insertIDCode(bson.D{{Key: "a.b", Value: int32(1)}}),
	})
}

// A null _id is a valid, single-valued _id: MongoDB stores it, finds it by
// {_id: null}, and rejects a second null _id as a duplicate key. A raw insert
// command is used because the driver would replace a null _id with a generated
// ObjectId.
func TestBSON_id_null_accepted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BSON_id_null_accepted",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			insert := bson.D{
				{Key: "insert", Value: col.Name()},
				{Key: "documents", Value: bson.A{bson.D{{Key: "_id", Value: nil}, {Key: "v", Value: int32(1)}}}},
			}
			var res bson.D
			if err := col.Database().RunCommand(ctx, insert).Decode(&res); err != nil {
				return nil, err
			}
			found := col.FindOne(ctx, bson.D{{Key: "_id", Value: nil}}).Err() == nil
			return bson.D{{Key: "n", Value: res.Map()["n"]}, {Key: "found", Value: found}}, nil
		},
	})
}
