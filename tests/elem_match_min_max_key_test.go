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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// MinKey and MaxKey compare with every type, so inside $elemMatch a
// comparison to them matches an element that is itself an array, though a
// comparison to any other non-array value does not.

func elemMatchMinMaxSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{bson.A{"x", int32(-1)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{bson.A{}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: bson.A{bson.D{{Key: "x", Value: int32(1)}}}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{int32(5)}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.A{bson.A{primitive.MaxKey{}}}}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: bson.A{nil}}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "a", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "a", Value: bson.A{bson.A{bson.A{int32(1)}}}}},
	})
	return err
}

func elemMatchMinMaxRun(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		ids, err := cluFindIDs(filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		if err != nil {
			return nil, err
		}
		n, err := col.CountDocuments(ctx, filter)
		if err != nil {
			return nil, err
		}
		return bson.D{{Key: "find", Value: ids}, {Key: "count", Value: n}}, nil
	}
}

func TestElemMatch_MinMaxKey_ArrayElement(t *testing.T) {
	elemMatch := func(ops ...bson.E) bson.D {
		return bson.D{{Key: "a", Value: bson.D{{Key: "$elemMatch", Value: bson.D(ops)}}}}
	}
	for _, tc := range []struct {
		name    string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"GtMinKey", elemMatch(bson.E{Key: "$gt", Value: primitive.MinKey{}}), harness.DumboDBFull},
		{"GteMinKey", elemMatch(bson.E{Key: "$gte", Value: primitive.MinKey{}}), harness.DumboDBFull},
		{"LtMaxKey", elemMatch(bson.E{Key: "$lt", Value: primitive.MaxKey{}}), harness.DumboDBFull},
		{"LteMaxKey", elemMatch(bson.E{Key: "$lte", Value: primitive.MaxKey{}}), harness.DumboDBFull},
		{"GtMinKeyLtMaxKey", elemMatch(bson.E{Key: "$gt", Value: primitive.MinKey{}}, bson.E{Key: "$lt", Value: primitive.MaxKey{}}), harness.DumboDBFull},
		{"GtMaxKey", elemMatch(bson.E{Key: "$gt", Value: primitive.MaxKey{}}), harness.DumboDBFull},
		{"LtMinKey", elemMatch(bson.E{Key: "$lt", Value: primitive.MinKey{}}), harness.DumboDBFull},
		{"LteMinKey", elemMatch(bson.E{Key: "$lte", Value: primitive.MinKey{}}), harness.DumboDBFull},
		{"GteMaxKey", elemMatch(bson.E{Key: "$gte", Value: primitive.MaxKey{}}), harness.DumboDBFull},
		{"NotGtMinKey", elemMatch(bson.E{Key: "$not", Value: bson.D{{Key: "$gt", Value: primitive.MinKey{}}}}), harness.DumboDBFull},
		{"GtEmptyArray", elemMatch(bson.E{Key: "$gt", Value: bson.A{}}), harness.DumboDBFull},
		{"LtNumber", elemMatch(bson.E{Key: "$lt", Value: int32(7)}), harness.DumboDBFull},
		{"TopLevelGtMinKey", bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: primitive.MinKey{}}}}}, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ElemMatch_MinMaxKey_ArrayElement_" + tc.name,
				Support: tc.support,
				Setup:   elemMatchMinMaxSetup,
				Run:     elemMatchMinMaxRun(tc.filter),
			})
		})
	}
}
