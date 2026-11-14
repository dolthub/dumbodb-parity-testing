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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A query's collation applies to strings inside arrays as it does to a
// top-level string, for every operator that reaches array elements.

func collationArraySetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: "N"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{"N"}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: bson.A{int32(1), "N"}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{math.NaN(), "N"}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.A{bson.A{"N"}}}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "a", Value: bson.A{"Z", "b"}}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "a", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(9)}, {Key: "a", Value: bson.A{}}},
	})
	return err
}

func collationArrayRun(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	en2 := &options.Collation{Locale: "en", Strength: 2}
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		ids, err := cluFindIDs(filter, options.Find().SetCollation(en2).SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		if err != nil {
			return nil, err
		}
		n, err := col.CountDocuments(ctx, filter, options.Count().SetCollation(en2))
		if err != nil {
			return nil, err
		}
		return bson.D{{Key: "find", Value: ids}, {Key: "count", Value: n}}, nil
	}
}

func TestCollation_ArrayElements(t *testing.T) {
	a := func(v interface{}) bson.D { return bson.D{{Key: "a", Value: v}} }
	op := func(name string, v interface{}) bson.D { return a(bson.D{{Key: name, Value: v}}) }
	for _, tc := range []struct {
		name    string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"Equality", a("n"), harness.DumboDBFull},
		{"Eq", op("$eq", "n"), harness.DumboDBFull},
		{"Ne", op("$ne", "n"), harness.DumboDBFull},
		{"Gt", op("$gt", "m"), harness.DumboDBFull},
		{"Gte", op("$gte", "n"), harness.DumboDBFull},
		{"Lt", op("$lt", "o"), harness.DumboDBFull},
		{"Lte", op("$lte", "n"), harness.DumboDBFull},
		{"In", op("$in", bson.A{"n", "q"}), harness.DumboDBFull},
		{"Nin", op("$nin", bson.A{"n"}), harness.DumboDBFull},
		{"All", op("$all", bson.A{"n"}), harness.DumboDBFull},
		{"AllTwo", op("$all", bson.A{"z", "B"}), harness.DumboDBFull},
		{"ElemMatchEq", op("$elemMatch", bson.D{{Key: "$eq", Value: "n"}}), harness.DumboDBFull},
		{"ElemMatchRange", op("$elemMatch", bson.D{{Key: "$gte", Value: "n"}, {Key: "$lt", Value: "o"}}), harness.DumboDBFull},
		{"ElemMatchNestedArray", op("$elemMatch", bson.D{{Key: "$eq", Value: bson.A{"n"}}}), harness.DumboDBFull},
		{"NestedArrayEquality", a(bson.A{"n"}), harness.DumboDBFull},
		{"InNestedArray", op("$in", bson.A{bson.A{"n"}}), harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Collation_ArrayElements_" + tc.name,
				Support: tc.support,
				Setup:   collationArraySetup,
				Run:     collationArrayRun(tc.filter),
			})
		})
	}
}

// The $in case of dolthub/dumbodb#227.
func TestCollation_ArrayElements_InIssue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_ArrayElements_InIssue",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "m", Value: bson.A{"X", "z"}}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "m", Value: bson.A{"a", "Y"}}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "m", Value: bson.A{"q"}}},
			})
			return err
		},
		Run: collationArrayRun(bson.D{{Key: "m", Value: bson.D{{Key: "$in", Value: bson.A{"x", "y"}}}}}),
	})
}
