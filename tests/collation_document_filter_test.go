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

// collationDocumentFilterDocs hold documents and arrays of documents whose
// strings differ only in case. Every document keeps its fields in byte order,
// the order DumboDB stores them in.
func collationDocumentFilterDocs() []interface{} {
	xy := func(x, y string) bson.D { return bson.D{{Key: "x", Value: x}, {Key: "y", Value: y}} }
	return []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "d", Value: xy("A", "b")}},
		bson.D{{Key: "_id", Value: 2}, {Key: "d", Value: xy("a", "B")}},
		bson.D{{Key: "_id", Value: 3}, {Key: "d", Value: bson.A{xy("A", "B")}}},
		bson.D{{Key: "_id", Value: 4}, {Key: "d", Value: bson.D{
			{Key: "a", Value: bson.A{"X", "y"}}, {Key: "b", Value: bson.D{{Key: "c", Value: "Q"}}},
		}}},
		bson.D{{Key: "_id", Value: 5}, {Key: "d", Value: bson.A{bson.A{bson.D{{Key: "a", Value: "P"}}}}}},
		bson.D{{Key: "_id", Value: 6}, {Key: "d", Value: xy("a", "c")}},
		bson.D{{Key: "_id", Value: 7}, {Key: "d", Value: "ab"}},
	}
}

// Equality, $in, $ne, $nin, range, $all and $elemMatch filters with document
// operands compare strings inside documents, at any depth, under the
// collation; field names stay binary. Checked through find, count,
// aggregate $match, updateMany and deleteMany, under a collection default
// collation and under a command collation.
func TestCollation_DocumentFilters(t *testing.T) {
	xy := func(x, y string) bson.D { return bson.D{{Key: "x", Value: x}, {Key: "y", Value: y}} }
	f := func(field, op string, v interface{}) bson.D {
		return bson.D{{Key: field, Value: bson.D{{Key: op, Value: v}}}}
	}
	nested := bson.D{{Key: "a", Value: bson.A{"x", "Y"}}, {Key: "b", Value: bson.D{{Key: "c", Value: "q"}}}}
	filters := map[string]bson.D{
		"implicit":         {{Key: "d", Value: xy("a", "b")}},
		"implicit_nested":  {{Key: "d", Value: nested}},
		"implicit_array":   {{Key: "d", Value: bson.A{xy("a", "b")}}},
		"eq":               f("d", "$eq", xy("a", "b")),
		"in":               f("d", "$in", bson.A{xy("a", "b")}),
		"in_nested_array":  f("d", "$in", bson.A{bson.A{bson.D{{Key: "a", Value: "p"}}}, nested}),
		"ne":               f("d", "$ne", xy("a", "b")),
		"nin":              f("d", "$nin", bson.A{xy("a", "b"), nested}),
		"gte":              f("d", "$gte", xy("a", "b")),
		"gt":               f("d", "$gt", xy("a", "b")),
		"lt":               f("d", "$lt", xy("a", "C")),
		"lte":              f("d", "$lte", xy("a", "b")),
		"gt_nested":        f("d", "$gt", bson.D{{Key: "a", Value: bson.A{"x", "Y"}}, {Key: "b", Value: bson.D{{Key: "c", Value: "p"}}}}),
		"all":              f("d", "$all", bson.A{xy("a", "b")}),
		"all_two":          f("d", "$all", bson.A{xy("A", "b"), xy("a", "B")}),
		"elem_match_eq":    f("d", "$elemMatch", bson.D{{Key: "$eq", Value: xy("a", "b")}}),
		"elem_match_array": f("d", "$elemMatch", bson.D{{Key: "$gte", Value: bson.A{bson.D{{Key: "a", Value: "p"}}}}}),
		"positional":       {{Key: "d.0", Value: xy("a", "b")}},
		"positional_in":    f("d.0", "$in", bson.A{xy("a", "b")}),
		"positional_ne":    f("d.0", "$ne", xy("a", "b")),
		"dotted_document":  {{Key: "d.b", Value: bson.D{{Key: "c", Value: "q"}}}},
		"not_eq":           f("d", "$not", bson.D{{Key: "$eq", Value: xy("a", "b")}}),
		"nor":              {{Key: "$nor", Value: bson.A{bson.D{{Key: "d", Value: xy("a", "b")}}}}},
	}
	collation := &options.Collation{Locale: "en", Strength: 2}
	for _, mode := range []string{"collection", "command"} {
		for name, filter := range filters {
			harness.PairTest(t, harness.TestCase{
				Name:    "CollationDocumentFilter_" + mode + "_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if err := col.Drop(ctx); err != nil {
						return err
					}
					opts := options.CreateCollection()
					if mode == "collection" {
						opts.SetCollation(collation)
					}
					if err := col.Database().CreateCollection(ctx, col.Name(), opts); err != nil {
						return err
					}
					_, err := col.InsertMany(ctx, collationDocumentFilterDocs())
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var coll *options.Collation
					if mode == "command" {
						coll = collation
					}
					byID := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).
						SetProjection(bson.D{{Key: "_id", Value: 1}}).SetCollation(coll)
					cur, err := col.Find(ctx, filter, byID)
					if err != nil {
						return errOutcome(err), nil
					}
					found, err := collectIDs(ctx, cur)
					if err != nil {
						return nil, err
					}
					n, err := col.CountDocuments(ctx, filter, options.Count().SetCollation(coll))
					if err != nil {
						return errOutcome(err), nil
					}
					cur, err = col.Aggregate(ctx, bson.A{
						bson.D{{Key: "$match", Value: filter}},
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					}, options.Aggregate().SetCollation(coll))
					if err != nil {
						return errOutcome(err), nil
					}
					matched, err := collectIDs(ctx, cur)
					if err != nil {
						return nil, err
					}
					upd, err := col.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "u", Value: 1}}}},
						options.Update().SetCollation(coll))
					if err != nil {
						return errOutcome(err), nil
					}
					del, err := col.DeleteMany(ctx, filter, options.Delete().SetCollation(coll))
					if err != nil {
						return errOutcome(err), nil
					}
					return bson.D{
						{Key: "find", Value: found},
						{Key: "count", Value: n},
						{Key: "match", Value: matched},
						{Key: "updated", Value: upd.ModifiedCount},
						{Key: "deleted", Value: del.DeletedCount},
					}, nil
				},
			})
		}
	}
}
