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

var ci2 = &options.Collation{Locale: "en", Strength: 2}

func seedNames(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: "Alice"}},
		bson.D{{Key: "_id", Value: 2}, {Key: "u", Value: "BOB"}},
		bson.D{{Key: "_id", Value: 3}, {Key: "u", Value: "alice"}},
		bson.D{{Key: "_id", Value: 4}, {Key: "u", Value: "carol"}},
	})
	return err
}

func TestCollation_Find_CaseInsensitiveEquality(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Find_CaseInsensitiveEquality",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{{Key: "u", Value: "alice"}},
				options.Find().SetCollation(ci2).SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			ids := bson.A{}
			for _, d := range docs {
				ids = append(ids, d.Map()["_id"])
			}
			return bson.D{{Key: "ids", Value: ids}}, nil
		},
	})
}

func TestCollation_Find_In(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Find_In",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{{Key: "u", Value: bson.D{{Key: "$in", Value: bson.A{"ALICE", "bob"}}}}},
				options.Find().SetCollation(ci2).SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			ids := bson.A{}
			for _, d := range docs {
				ids = append(ids, d.Map()["_id"])
			}
			return bson.D{{Key: "ids", Value: ids}}, nil
		},
	})
}

func TestCollation_Find_SortCaseInsensitive(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Find_SortCaseInsensitive",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{},
				options.Find().SetCollation(ci2).SetSort(bson.D{{Key: "u", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			order := bson.A{}
			for _, d := range docs {
				order = append(order, d.Map()["u"])
			}
			return bson.D{{Key: "order", Value: order}}, nil
		},
	})
}

func TestCollation_Find_AccentInsensitiveStrength1(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Find_AccentInsensitiveStrength1",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: "cafe"}},
				bson.D{{Key: "_id", Value: 2}, {Key: "u", Value: "caf\u00e9"}}, // cafe with acute e
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{{Key: "u", Value: "cafe"}},
				options.Find().SetCollation(&options.Collation{Locale: "en", Strength: 1}).SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			ids := bson.A{}
			for _, d := range docs {
				ids = append(ids, d.Map()["_id"])
			}
			return bson.D{{Key: "ids", Value: ids}}, nil
		},
	})
}

func TestCollation_Count(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Count",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			n, err := col.CountDocuments(ctx, bson.D{{Key: "u", Value: "alice"}},
				options.Count().SetCollation(ci2))
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "n", Value: n}}, nil
		},
	})
}

func TestCollation_Distinct(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Distinct",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			vals, err := col.Distinct(ctx, "_id", bson.D{{Key: "u", Value: "alice"}},
				options.Distinct().SetCollation(ci2))
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "vals", Value: vals}}, nil
		},
	})
}

func TestCollation_DeleteMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_DeleteMany",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.DeleteMany(ctx, bson.D{{Key: "u", Value: "alice"}},
				options.Delete().SetCollation(ci2))
			if err != nil {
				return nil, err
			}
			remaining, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "deleted", Value: res.DeletedCount}, {Key: "remaining", Value: remaining}}, nil
		},
	})
}

func TestCollation_UpdateMany(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_UpdateMany",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx, bson.D{{Key: "u", Value: "alice"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "x"}}}},
				options.Update().SetCollation(ci2))
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "matched", Value: res.MatchedCount}, {Key: "modified", Value: res.ModifiedCount}}, nil
		},
	})
}

func TestCollation_FindOneAndUpdate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_FindOneAndUpdate",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var updated bson.D
			err := col.FindOneAndUpdate(ctx, bson.D{{Key: "u", Value: "BOB"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "tag", Value: "y"}}}},
				options.FindOneAndUpdate().SetCollation(ci2).SetReturnDocument(options.After)).Decode(&updated)
			if err != nil {
				return nil, err
			}
			m := updated.Map()
			return bson.D{{Key: "id", Value: m["_id"]}, {Key: "tag", Value: m["tag"]}}, nil
		},
	})
}

// The tests below exercise collation actually GOVERNING search behavior, not
// just being stored/echoed. They currently diverge (XFail): DumboDB has no
// collection-default collation, and its operation-collation path is an
// equality-only case-fold approximation that does not cover ranges.

// A collection created with a default collation must make queries collated
// even when the operation supplies no collation of its own.
func TestCollation_CollectionDefault_Find(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_CollectionDefault_Find",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			if err := db.CreateCollection(ctx, "cdef", options.CreateCollection().SetCollation(ci2)); err != nil {
				return nil, err
			}
			c := db.Collection("cdef")
			if _, err := c.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: "Alice"}},
				bson.D{{Key: "_id", Value: 2}, {Key: "u", Value: "BOB"}},
				bson.D{{Key: "_id", Value: 3}, {Key: "u", Value: "alice"}},
			}); err != nil {
				return nil, err
			}
			// No operation collation: the collection default must govern.
			cur, err := c.Find(ctx, bson.D{{Key: "u", Value: "alice"}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			ids := bson.A{}
			for _, d := range docs {
				ids = append(ids, d.Map()["_id"])
			}
			return bson.D{{Key: "ids", Value: ids}}, nil
		},
	})
}

// A collection default collation must also govern sort order with no
// per-operation collation.
func TestCollation_CollectionDefault_Sort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_CollectionDefault_Sort",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			if err := db.CreateCollection(ctx, "cdef", options.CreateCollection().SetCollation(ci2)); err != nil {
				return nil, err
			}
			c := db.Collection("cdef")
			if _, err := c.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: "Alice"}},
				bson.D{{Key: "_id", Value: 2}, {Key: "u", Value: "BOB"}},
				bson.D{{Key: "_id", Value: 3}, {Key: "u", Value: "alice"}},
				bson.D{{Key: "_id", Value: 4}, {Key: "u", Value: "carol"}},
			}); err != nil {
				return nil, err
			}
			cur, err := c.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "u", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			order := bson.A{}
			for _, d := range docs {
				order = append(order, d.Map()["u"])
			}
			return bson.D{{Key: "order", Value: order}}, nil
		},
	})
}

// Collation must apply to range bounds, not only equality. The equality-only
// case-fold approximation leaves $gte comparing bytewise.
func TestCollation_Find_RangeGte(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Find_RangeGte",
		Support: harness.DumboDBFull,
		Setup:   seedNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, bson.D{{Key: "u", Value: bson.D{{Key: "$gte", Value: "BOB"}}}},
				options.Find().SetCollation(ci2).SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			ids := bson.A{}
			for _, d := range docs {
				ids = append(ids, d.Map()["_id"])
			}
			return bson.D{{Key: "ids", Value: ids}}, nil
		},
	})
}
