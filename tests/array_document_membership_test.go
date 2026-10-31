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

// Membership and equality of documents in arrays, from the review of #184
// (workspace-m8c.11): $in and $nin with document members against arrays
// holding them, and an array filter value against nested arrays.

func insertArrayMembershipDocs(ctx context.Context, col *mongo.Collection) error {
	ab := bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: bson.A{ab}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "arr", Value: bson.A{bson.A{ab}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "arr", Value: bson.A{int32(5), ab}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "arr", Value: ab}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "arr", Value: bson.A{int32(5)}}},
		bson.D{{Key: "_id", Value: int32(6)}},
	})
	return err
}

func TestArrayDocumentMembership(t *testing.T) {
	ab := bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}
	op := func(name string, v interface{}) bson.D { return bson.D{{Key: name, Value: v}} }
	ids := func(filter interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return errOutcome(err), nil
			}
			return collectIDs(ctx, cursor)
		}
	}

	cases := []struct {
		name   string
		filter interface{}
	}{
		{"InDocument", bson.D{{Key: "arr", Value: op("$in", bson.A{ab})}}},
		{"InDocumentAndScalar", bson.D{{Key: "arr", Value: op("$in", bson.A{int32(7), ab})}}},
		{"NinDocument", bson.D{{Key: "arr", Value: op("$nin", bson.A{ab})}}},
		{"NinDocumentAndScalar", bson.D{{Key: "arr", Value: op("$nin", bson.A{int32(5), ab})}}},
		{"InArrayOfDocument", bson.D{{Key: "arr", Value: op("$in", bson.A{bson.A{ab}})}}},
		{"NinArrayOfDocument", bson.D{{Key: "arr", Value: op("$nin", bson.A{bson.A{ab}})}}},
		{"EqArrayOfDocument", bson.D{{Key: "arr", Value: bson.A{ab}}}},
		{"EqOpArrayOfDocument", bson.D{{Key: "arr", Value: op("$eq", bson.A{ab})}}},
		{"NeArrayOfDocument", bson.D{{Key: "arr", Value: op("$ne", bson.A{ab})}}},
		{"EqNestedArray", bson.D{{Key: "arr", Value: bson.A{bson.A{ab}}}}},
		{"InPositional", bson.D{{Key: "arr.0", Value: op("$in", bson.A{ab})}}},
		{"NinPositional", bson.D{{Key: "arr.0", Value: op("$nin", bson.A{ab})}}},
		{"ElemMatchIn", bson.D{{Key: "arr", Value: op("$elemMatch", op("$in", bson.A{ab}))}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ArrayMembership_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertArrayMembershipDocs,
				Run:     ids(tc.filter),
			})
		})
	}
}
