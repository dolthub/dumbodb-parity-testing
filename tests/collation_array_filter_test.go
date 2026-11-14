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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// collationArrayFilterSetup creates a collection with a case-insensitive
// default collation holding arrays of strings, numbers, nested arrays and
// documents, plus a few long arrays.
func collationArrayFilterSetup(ctx context.Context, col *mongo.Collection) error {
	if err := col.Drop(ctx); err != nil {
		return err
	}
	opts := options.CreateCollection().SetCollation(&options.Collation{Locale: "en", Strength: 2})
	if err := col.Database().CreateCollection(ctx, col.Name(), opts); err != nil {
		return err
	}
	docs := []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{"Abc", "def", 5}}},
		bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: bson.A{"ABC", bson.A{"X", "y"}}}},
		bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: bson.A{bson.D{{Key: "k", Value: "Q"}}, "zz", 7}}},
		bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: "abc"}},
		bson.D{{Key: "_id", Value: 6}, {Key: "a", Value: bson.A{1, 2, 3}}},
		bson.D{{Key: "_id", Value: 7}, {Key: "a", Value: bson.A{bson.A{"x", "Y"}, "m"}}},
	}
	for id := 10; id < 14; id++ {
		long := bson.A{}
		for i := 0; i < 300; i++ {
			long = append(long, fmt.Sprintf("s%04d", i*(id-9)))
		}
		long = append(long, id)
		docs = append(docs, bson.D{{Key: "_id", Value: id}, {Key: "a", Value: long}})
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

// Filters on array fields under a collection's case-insensitive default
// collation compare each string element under the collation, through find,
// count, update and delete.
func TestCollation_ArrayFieldFilters(t *testing.T) {
	nums := bson.A{}
	upper := bson.A{}
	for i := 0; i < 100; i++ {
		nums = append(nums, i+20)
		upper = append(upper, fmt.Sprintf("S%04d", i*2))
	}
	f := func(op string, v interface{}) bson.D {
		return bson.D{{Key: "a", Value: bson.D{{Key: op, Value: v}}}}
	}
	filters := map[string]bson.D{
		"eq_string":          {{Key: "a", Value: "abc"}},
		"eq_nested_array":    {{Key: "a", Value: bson.A{"x", "y"}}},
		"eq_document":        {{Key: "a", Value: bson.D{{Key: "k", Value: "q"}}}},
		"ne_string":          f("$ne", "ABC"),
		"in_numbers":         f("$in", nums),
		"in_mixed":           f("$in", bson.A{"DEF", 7, bson.A{"X", "Y"}}),
		"in_upper":           f("$in", upper),
		"nin_mixed":          f("$nin", bson.A{"zZ", 2}),
		"nin_numbers":        f("$nin", nums),
		"all_strings":        f("$all", bson.A{"abc", "DEF"}),
		"all_upper":          f("$all", upper[:20]),
		"all_mixed":          f("$all", bson.A{"ZZ", 7}),
		"all_nested":         f("$all", bson.A{bson.A{"X", "Y"}}),
		"all_nested_strings": f("$all", bson.A{bson.A{"x", "Y"}, "M"}),
		"all_document":       f("$all", bson.A{bson.D{{Key: "k", Value: "Q"}}, "ZZ"}),
		"all_long_mixed":     f("$all", append(bson.A{10}, upper[:30]...)),
		"gt_string":          f("$gt", "S0500"),
		"lte_string":         f("$lte", "ABC"),
		"gte_number":         f("$gte", 7),
		"lt_array":           f("$lt", bson.A{"X", "Z"}),
		"elem_match_in":      f("$elemMatch", bson.D{{Key: "$in", Value: upper}}),
		"elem_match_range":   f("$elemMatch", bson.D{{Key: "$gte", Value: "ABC"}, {Key: "$lt", Value: "DEG"}}),
		"elem_match_eq_doc":  f("$elemMatch", bson.D{{Key: "k", Value: "q"}}),
	}
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "CollationArrayFilter_" + name,
			Support: harness.DumboDBFull,
			Setup:   collationArrayFilterSetup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				byID := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
				cur, err := col.Find(ctx, filter, byID)
				if err != nil {
					return errOutcome(err), nil
				}
				found, err := collectIDs(ctx, cur)
				if err != nil {
					return nil, err
				}
				n, err := col.CountDocuments(ctx, filter)
				if err != nil {
					return errOutcome(err), nil
				}
				upd, err := col.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "u", Value: 1}}}})
				if err != nil {
					return errOutcome(err), nil
				}
				del, err := col.DeleteMany(ctx, filter)
				if err != nil {
					return errOutcome(err), nil
				}
				cur, err = col.Find(ctx, bson.D{}, byID)
				if err != nil {
					return nil, err
				}
				left, err := collectIDs(ctx, cur)
				if err != nil {
					return nil, err
				}
				return bson.D{
					{Key: "find", Value: found},
					{Key: "count", Value: n},
					{Key: "updated", Value: upd.ModifiedCount},
					{Key: "deleted", Value: del.DeletedCount},
					{Key: "left", Value: left},
				}, nil
			},
		})
	}
}
