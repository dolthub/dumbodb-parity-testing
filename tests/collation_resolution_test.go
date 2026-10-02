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

// Collation resolution parity (collation-resolution.md section 4): how the
// collection default collation propagates to operations and indexes. The
// persistence keystone is in place, but resolution wiring (op/index inherit the
// default, _id pinning) is not, so most of these are XFail; they document the
// contract and flip to Full as the resolution work lands.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

var enS2 = &options.Collation{Locale: "en", Strength: 2}

// collatedColl creates a collection whose default collation is en/strength-2 and
// returns it.
func collatedColl(ctx context.Context, col *mongo.Collection) (*mongo.Collection, error) {
	db := col.Database()
	if err := db.CreateCollection(ctx, "cdef", options.CreateCollection().SetCollation(enS2)); err != nil {
		return nil, err
	}
	return db.Collection("cdef"), nil
}

func indexCollationByName(ctx context.Context, c *mongo.Collection, name string) (interface{}, error) {
	cur, err := c.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	var idxs []bson.M
	if err := cur.All(ctx, &idxs); err != nil {
		return nil, err
	}
	for _, idx := range idxs {
		if idx["name"] == name {
			return idx["collation"], nil
		}
	}
	return nil, nil
}

// A5: an operation collation of "simple" overrides the collection default down
// to binary. Both servers agree (binary miss), so this is Full today.
func TestCollation_CollectionDefault_SimpleOverride(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_CollectionDefault_SimpleOverride",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			c, err := collatedColl(ctx, col)
			if err != nil {
				return nil, err
			}
			if _, err := c.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: "Alice"}}); err != nil {
				return nil, err
			}
			n, err := c.CountDocuments(ctx, bson.D{{Key: "u", Value: "alice"}},
				options.Count().SetCollation(&options.Collation{Locale: "simple"}))
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "n", Value: n}}, nil
		},
	})
}

// A2: an index created with no collation inherits the collection default.
func TestCollation_Index_InheritsCollectionDefault(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Index_InheritsCollectionDefault",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			c, err := collatedColl(ctx, col)
			if err != nil {
				return nil, err
			}
			if _, err := c.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "u", Value: 1}}, Options: options.Index().SetName("u_idx"),
			}); err != nil {
				return nil, err
			}
			locale := ""
			coll, _ := indexCollationByName(ctx, c, "u_idx")
			if m, ok := coll.(bson.M); ok {
				locale, _ = m["locale"].(string)
			}
			return bson.D{{Key: "locale", Value: locale}}, nil
		},
	})
}

// D1: the _id index inherits the collection default (not simple).
func TestCollation_IdIndex_InheritsDefault(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_IdIndex_InheritsDefault",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			c, err := collatedColl(ctx, col)
			if err != nil {
				return nil, err
			}
			if _, err := c.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}}); err != nil {
				return nil, err
			}
			locale := ""
			coll, _ := indexCollationByName(ctx, c, "_id_")
			if m, ok := coll.(bson.M); ok {
				locale, _ = m["locale"].(string)
			}
			return bson.D{{Key: "locale", Value: locale}}, nil
		},
	})
}

// D2: _id uniqueness is enforced under the collection default; inserting _id "a"
// then "A" in an en/2 collection is a duplicate-key error.
func TestCollation_IdIndex_UniqueUnderDefault(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_IdIndex_UniqueUnderDefault",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			c, err := collatedColl(ctx, col)
			if err != nil {
				return nil, err
			}
			if _, err := c.InsertOne(ctx, bson.D{{Key: "_id", Value: "a"}}); err != nil {
				return nil, err
			}
			_, dupErr := c.InsertOne(ctx, bson.D{{Key: "_id", Value: "A"}})
			n, err := c.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "dup_rejected", Value: dupErr != nil}, {Key: "count", Value: n}}, nil
		},
	})
}

// D3: in a simple collection, _id "a" and "A" coexist (binary _id). Full today.
func TestCollation_IdIndex_SimpleCoexist(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_IdIndex_SimpleCoexist",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "a"}}); err != nil {
				return nil, err
			}
			_, dupErr := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "A"}})
			n, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "dup_rejected", Value: dupErr != nil}, {Key: "count", Value: n}}, nil
		},
	})
}

// D4: the _id index collation is pinned to the collection default; createIndex
// on _id with a different collation is BadValue (2), and with the same (or no)
// collation is a no-op. Only the code is compared: MongoDB's message embeds its
// ICU version.
func TestCollation_IdIndex_CollationPinnedToDefault(t *testing.T) {
	cases := []struct {
		name      string
		collated  bool
		collation bson.D
		support   harness.DumboDBSupport
	}{
		{"collated-fr", true, bson.D{{Key: "locale", Value: "fr"}}, harness.DumboDBFull},
		{"collated-en-strength3", true, bson.D{{Key: "locale", Value: "en"}, {Key: "strength", Value: 3}}, harness.DumboDBFull},
		{"collated-simple", true, bson.D{{Key: "locale", Value: "simple"}}, harness.DumboDBFull},
		{"collated-same", true, bson.D{{Key: "locale", Value: "en"}, {Key: "strength", Value: 2}}, harness.DumboDBFull},
		{"collated-none", true, nil, harness.DumboDBFull},
		{"simple-en", false, bson.D{{Key: "locale", Value: "en"}}, harness.DumboDBFull},
		{"simple-simple", false, bson.D{{Key: "locale", Value: "simple"}}, harness.DumboDBFull},
	}
	for _, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "Collation_IdIndex_CollationPinned_" + tc.name,
			Support: tc.support,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				target := col
				if tc.collated {
					c, err := collatedColl(ctx, col)
					if err != nil {
						return nil, err
					}
					target = c
				} else if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}}); err != nil {
					return nil, err
				}
				spec := bson.D{{Key: "key", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "name", Value: "_id_"}}
				if tc.collation != nil {
					spec = append(spec, bson.E{Key: "collation", Value: tc.collation})
				}
				err := target.Database().RunCommand(ctx, bson.D{
					{Key: "createIndexes", Value: target.Name()},
					{Key: "indexes", Value: bson.A{spec}},
				}).Err()
				code, _, _ := harness.CommandErrorCode(err)
				idCollation, listErr := indexCollationByName(ctx, target, "_id_")
				if listErr != nil {
					return nil, listErr
				}
				m, _ := idCollation.(bson.M)
				return bson.M{"code": code, "idLocale": m["locale"], "idStrength": m["strength"]}, nil
			},
		})
	}
}

// F3: createCollection with an invalid locale is rejected. Full today (DumboDB
// validates the locale against the accepted set).
func TestCollation_CreateInvalidLocale(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_CreateInvalidLocale",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().CreateCollection(ctx, "bad",
				options.CreateCollection().SetCollation(&options.Collation{Locale: "zz-nonsense"}))
			return bson.D{{Key: "rejected", Value: err != nil}}, nil
		},
	})
}
