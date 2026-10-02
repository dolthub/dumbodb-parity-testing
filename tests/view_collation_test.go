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

// View collation parity (VCOLL-01..10): a read on a view uses the view's
// collation, and an operation collation that differs from it is
// OptionNotSupportedOnView (167).

var viewCollationDocs = []interface{}{
	bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: "Alice"}},
	bson.D{{Key: "_id", Value: 2}, {Key: "u", Value: "alice"}},
	bson.D{{Key: "_id", Value: 3}, {Key: "u", Value: "BOB"}},
	bson.D{{Key: "_id", Value: 4}, {Key: "u", Value: "bob"}},
	bson.D{{Key: "_id", Value: 5}, {Key: "u", Value: "carol"}},
}

// viewCollationCase seeds the base collection, creates view "v" over it with
// |viewCollation| (nil = simple), and returns |run|'s result. An error from
// |run| is reported as its command error code.
func viewCollationCase(name string, viewCollation *options.Collation, run func(ctx context.Context, v *mongo.Collection) (interface{}, error)) harness.TestCase {
	return harness.TestCase{
		Name:    name,
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertMany(ctx, viewCollationDocs); err != nil {
				return nil, err
			}
			db := col.Database()
			opts := options.CreateView()
			if viewCollation != nil {
				opts.SetCollation(viewCollation)
			}
			if err := db.CreateView(ctx, "v", col.Name(), mongo.Pipeline{}, opts); err != nil {
				return nil, err
			}
			defer func() { _ = db.Collection("v").Drop(ctx) }()

			res, err := run(ctx, db.Collection("v"))
			if err != nil {
				code, _, _ := harness.CommandErrorCode(err)
				return bson.M{"code": code}, nil
			}
			return bson.M{"result": res}, nil
		},
	}
}

func viewFindIDs(ctx context.Context, v *mongo.Collection, filter bson.D, opts *options.FindOptions) (interface{}, error) {
	cur, err := v.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	ids := bson.A{}
	for _, d := range docs {
		ids = append(ids, d["_id"])
	}
	return ids, nil
}

func TestViewCollation(t *testing.T) {
	sortByID := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}})

	harness.PairTest(t, viewCollationCase("VCOLL-01-find-uses-view-collation", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return viewFindIDs(ctx, v, bson.D{{Key: "u", Value: "alice"}}, sortByID)
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-02-count-uses-view-collation", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return v.CountDocuments(ctx, bson.D{{Key: "u", Value: "alice"}})
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-03-distinct-uses-view-collation", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		values, err := v.Distinct(ctx, "u", bson.D{})
		return len(values), err
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-04-aggregate-uses-view-collation", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		cur, err := v.Aggregate(ctx, mongo.Pipeline{
			bson.D{{Key: "$match", Value: bson.D{{Key: "u", Value: "bob"}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		})
		if err != nil {
			return nil, err
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		return len(docs), nil
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-05-sort-uses-view-collation", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return viewFindIDs(ctx, v, bson.D{}, options.Find().SetSort(bson.D{{Key: "u", Value: 1}, {Key: "_id", Value: 1}}))
	}))

	sameCollation := viewCollationCase("VCOLL-06-find-same-collation-allowed", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return viewFindIDs(ctx, v, bson.D{{Key: "u", Value: "alice"}}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(enS2))
	})
	sameCollation.Support = harness.DumboDBFull
	harness.PairTest(t, sameCollation)

	harness.PairTest(t, viewCollationCase("VCOLL-07-find-simple-override-rejected", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return viewFindIDs(ctx, v, bson.D{}, options.Find().SetCollation(&options.Collation{Locale: "simple"}))
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-08-find-collation-on-simple-view-rejected", nil, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return viewFindIDs(ctx, v, bson.D{}, options.Find().SetCollation(enS2))
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-09-aggregate-override-rejected", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		_, err := v.Aggregate(ctx, mongo.Pipeline{}, options.Aggregate().SetCollation(&options.Collation{Locale: "fr"}))
		return nil, err
	}))

	harness.PairTest(t, viewCollationCase("VCOLL-10-count-override-rejected", enS2, func(ctx context.Context, v *mongo.Collection) (interface{}, error) {
		return v.CountDocuments(ctx, bson.D{}, options.Count().SetCollation(&options.Collation{Locale: "fr"}))
	}))
}
