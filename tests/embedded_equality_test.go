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
)

// Equality on embedded documents. MongoDB matches a document only in the
// field order it was written; DumboDB stores keys sorted (workspace-7ob), so
// it matches any order of the same fields. Array element equality is
// workspace-9ew.

func insertEmbeddedEqualityDocs(ctx context.Context, col *mongo.Collection) error {
	ba := bson.D{{Key: "b", Value: int32(1)}, {Key: "a", Value: int32(2)}}
	ab := bson.D{{Key: "a", Value: int32(2)}, {Key: "b", Value: int32(1)}}
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "addr", Value: ba}, {Key: "items", Value: bson.A{ba, int32(5)}}, {Key: "o", Value: bson.D{{Key: "p", Value: ba}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "addr", Value: ab}, {Key: "items", Value: bson.A{ab}}, {Key: "o", Value: bson.D{{Key: "p", Value: ab}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "addr", Value: bson.D{{Key: "a", Value: int32(2)}, {Key: "b", Value: int32(1)}, {Key: "c", Value: int32(3)}}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "addr", Value: "x"}},
	})
	return err
}

func embeddedEqualityIDs(filter interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return nil, err
		}
		return collectIDs(ctx, cursor)
	}
}

func TestEmbeddedDocumentEquality(t *testing.T) {
	ba := bson.D{{Key: "b", Value: int32(1)}, {Key: "a", Value: int32(2)}}
	ab := bson.D{{Key: "a", Value: int32(2)}, {Key: "b", Value: int32(1)}}
	op := func(name string, v interface{}) bson.D { return bson.D{{Key: name, Value: v}} }

	cases := []struct {
		name      string
		setup     func(context.Context, *mongo.Collection) error
		run       func(context.Context, *mongo.Collection) (interface{}, error)
		mongoWant string
		dumboWant string
	}{
		{"Implicit", nil, embeddedEqualityIDs(bson.D{{Key: "addr", Value: ba}}), "[1]", "[]"},
		{"ImplicitSortedOrder", nil, embeddedEqualityIDs(bson.D{{Key: "addr", Value: ab}}), "[2]", "[1 2]"},
		{"Eq", nil, embeddedEqualityIDs(bson.D{{Key: "addr", Value: op("$eq", ba)}}), "[1]", "[]"},
		{"Ne", nil, embeddedEqualityIDs(bson.D{{Key: "addr", Value: op("$ne", ba)}}), "[2 3 4]", "[1 2 3 4]"},
		{"In", nil, embeddedEqualityIDs(bson.D{{Key: "addr", Value: op("$in", bson.A{ba, "y"})}}), "[1]", "[]"},
		{"Nin", nil, embeddedEqualityIDs(bson.D{{Key: "addr", Value: op("$nin", bson.A{ba})}}), "[2 3 4]", "[1 2 3 4]"},
		{"ArrayElement", nil, embeddedEqualityIDs(bson.D{{Key: "items", Value: ba}}), "[1]", "[]"},
		{"EqArrayElement", nil, embeddedEqualityIDs(bson.D{{Key: "items", Value: op("$eq", ba)}}), "[1]", "[]"},
		{"NeArrayElement", nil, embeddedEqualityIDs(bson.D{{Key: "items", Value: op("$ne", ba)}}), "[2 3 4]", "[1 2 3 4]"},
		{"All", nil, embeddedEqualityIDs(bson.D{{Key: "items", Value: op("$all", bson.A{ba})}}), "[1]", "[]"},
		{"ElemMatchEq", nil, embeddedEqualityIDs(bson.D{{Key: "items", Value: op("$elemMatch", op("$eq", ba))}}), "[1]", "[]"},
		{"NestedPath", nil, embeddedEqualityIDs(bson.D{{Key: "o.p", Value: ba}}), "[1]", "[]"},
		{"NestedLiteral", nil, embeddedEqualityIDs(bson.D{{Key: "o", Value: bson.D{{Key: "p", Value: ba}}}}), "[1]", "[]"},
		{"Or", nil, embeddedEqualityIDs(bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "addr", Value: ba}}, bson.D{{Key: "_id", Value: int32(4)}}}}}), "[1 4]", "[4]"},
		{"Indexed", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "addr", Value: 1}}})
			return err
		}, embeddedEqualityIDs(bson.D{{Key: "addr", Value: ba}}), "[1]", "[]"},
		{"Expr", nil, embeddedEqualityIDs(bson.D{{Key: "$expr", Value: op("$eq", bson.A{"$addr", op("$literal", ba)})}}), "[1]", "[]"},
		{"Aggregate", nil, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$match", Value: bson.D{{Key: "addr", Value: ba}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			})
			if err != nil {
				return nil, err
			}
			return collectIDs(ctx, cursor)
		}, "[1]", "[]"},
		{"Count", nil, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, bson.D{{Key: "addr", Value: ba}})
		}, "1", "0"},
		{"DistinctFilter", nil, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.Distinct(ctx, "_id", bson.D{{Key: "addr", Value: ba}})
		}, "[1]", "[]"},
		{"Update", nil, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx, bson.D{{Key: "addr", Value: ba}}, bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: true}}}})
			if err != nil {
				return nil, err
			}
			return res.MatchedCount, nil
		}, "1", "0"},
		{"Delete", nil, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteMany(ctx, bson.D{{Key: "addr", Value: ba}})
			if err != nil {
				return nil, err
			}
			return res.DeletedCount, nil
		}, "1", "0"},
		{"IDStaysExact", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: ba}})
			return err
		}, embeddedEqualityIDs(bson.D{{Key: "_id", Value: ab}}), "[]", "[]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setup := func(ctx context.Context, col *mongo.Collection) error {
				if err := insertEmbeddedEqualityDocs(ctx, col); err != nil {
					return err
				}
				if tc.setup != nil {
					return tc.setup(ctx, col)
				}
				return nil
			}
			fieldOrderDeviation(t, "EmbeddedEquality_"+tc.name, setup, tc.run, tc.mongoWant, tc.dumboWant)
		})
	}
}
