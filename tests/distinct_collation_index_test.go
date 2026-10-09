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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// An unfiltered distinct whose collation matches a collated index led by the
// key is answered by MongoDB with DISTINCT_SCAN + FETCH: one fetched document
// per collation-equal index key, returning that document's value. A group whose
// fetched document lacks the field (or holds an empty array) contributes
// nothing. MongoDB fetches the group's first-inserted document; dumbodb has no
// insertion order, so only cases where every group has a single document are
// compared exactly (workspace-16g).

var distinctCollationEn2 = &options.Collation{Locale: "en", Strength: 2}

func distinctCollatedIndexSetup(docs ...interface{}) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if len(docs) > 0 {
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "s", Value: 1}},
			Options: options.Index().SetCollation(distinctCollationEn2),
		})
		return err
	}
}

func distinctCollatedRun(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	vals, err := col.Distinct(ctx, "s", bson.D{}, options.Distinct().SetCollation(distinctCollationEn2))
	if err != nil {
		return nil, err
	}
	return bson.A(vals), nil
}

func TestDistinct_CollatedIndex_SingleDocGroups(t *testing.T) {
	for _, tc := range []struct {
		name string
		docs []interface{}
	}{
		{"MissingAndEmptyArrayContributeNothing", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "A"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "b"}},
			bson.D{{Key: "_id", Value: int32(3)}},
			bson.D{{Key: "_id", Value: int32(4)}, {Key: "s", Value: bson.A{}}},
		}},
		{"ExplicitNullContributesNull", []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "A"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: nil}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Distinct_CollatedIndex_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   distinctCollatedIndexSetup(tc.docs...),
				Run:     distinctCollatedRun,
			})
		})
	}
}

func TestDistinct_CollectionCollation_IndexInherits(t *testing.T) {
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "Distinct_CollectionCollation_IndexInherits",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := tgt.Admin.Database(tgt.NS)
			defer func() { _ = db.Drop(ctx) }()
			tgt.Setup(db.CreateCollection(ctx, "c", options.CreateCollection().SetCollation(distinctCollationEn2)))
			col := db.Collection("c")
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "A"}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: "b"}},
				bson.D{{Key: "_id", Value: int32(3)}},
			})
			tgt.Setup(err)
			_, err = col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "s", Value: 1}}})
			tgt.Setup(err)
			vals, err := col.Distinct(ctx, "s", bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.A(vals), nil
		},
	})
}

// MongoDB returns the first-inserted member of a collation-equal group; dumbodb
// returns one member without an insertion-order guarantee.
func TestDistinct_CollatedIndex_GroupRepresentative(t *testing.T) {
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "Distinct_CollatedIndex_GroupRepresentative",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := tgt.Admin.Database(tgt.NS)
			defer func() { _ = db.Drop(ctx) }()
			col := db.Collection("c")
			for _, d := range []bson.D{
				{{Key: "_id", Value: int32(9)}, {Key: "s", Value: "a"}},
				{{Key: "_id", Value: int32(1)}, {Key: "s", Value: "A"}},
			} {
				_, err := col.InsertOne(ctx, d)
				tgt.Setup(err)
			}
			tgt.Setup(distinctCollatedIndexSetup()(ctx, col))
			return distinctCollatedRun(ctx, col)
		},
		MongoExpect: func(t *testing.T, res interface{}, err error) {
			if err != nil || len(res.(bson.A)) != 1 || res.(bson.A)[0] != "a" {
				t.Errorf("mongo: want first-inserted [a], got %v %v", res, err)
			}
		},
		DumboExpect: func(t *testing.T, res interface{}, err error) {
			if err != nil || len(res.(bson.A)) != 1 || !strings.EqualFold(res.(bson.A)[0].(string), "a") {
				t.Errorf("dumbodb: want one member of {a, A}, got %v %v", res, err)
			}
		},
	})
}
