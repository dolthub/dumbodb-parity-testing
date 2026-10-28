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

// Case and accent variants, whose collated order differs from binary order.
var collatedOperatorValues = []interface{}{
	"a", "A", "ab", "aB", "Ab", "b", "B", "c", "C", "cote", "cot\u00e9", "c\u00f4te", "c\u00f4t\u00e9",
	"e", "E", "\u00e9", "z", "Z", "", int32(5), int32(50), true,
}

// Operator filters on a collated index field (dumbodb #130) through find,
// aggregate $match and count, under the index's collation.
func TestCollation_OperatorFilterOnCollatedIndex(t *testing.T) {
	collations := []struct {
		name string
		coll *options.Collation
	}{
		{"enS1", &options.Collation{Locale: "en", Strength: 1}},
		{"enS2", &options.Collation{Locale: "en", Strength: 2}},
		{"frCA", &options.Collation{Locale: "fr_CA"}},
	}
	op := func(kv ...interface{}) bson.D {
		inner := bson.D{}
		for i := 0; i < len(kv); i += 2 {
			inner = append(inner, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return bson.D{{Key: "s", Value: inner}}
	}
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"eq_B", op("$eq", "B")},
		{"gt_b", op("$gt", "b")},
		{"gte_B", op("$gte", "B")},
		{"lt_c", op("$lt", "c")},
		{"lte_C", op("$lte", "C")},
		{"range_b_d", op("$gte", "b", "$lt", "d")},
		{"range_cote", op("$gt", "cote", "$lte", "c\u00f4t\u00e9")},
		{"in_B_z", op("$in", bson.A{"B", "z"})},
		{"ne_B", op("$ne", "B")},
		{"mixed_lower", op("$gte", int32(5), "$gt", "b")},
		{"eq_and_gte", op("$eq", "b", "$gte", "c")},
	}

	for _, c := range collations {
		for _, f := range filters {
			name := "CollOp_" + c.name + "_" + f.name
			coll, filter := c.coll, f.filter
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name,
					Support: harness.DumboDBFull,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
							Keys:    bson.D{{Key: "s", Value: 1}},
							Options: options.Index().SetCollation(coll),
						}); err != nil {
							return err
						}
						docs := make([]interface{}, 0, len(collatedOperatorValues))
						for i, v := range collatedOperatorValues {
							docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: v}})
						}
						_, err := col.InsertMany(ctx, docs)
						return err
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						byID := bson.D{{Key: "_id", Value: 1}}
						cur, err := col.Find(ctx, filter, options.Find().SetCollation(coll).SetSort(byID))
						if err != nil {
							return nil, err
						}
						found, err := collectIDs(ctx, cur)
						if err != nil {
							return nil, err
						}
						cur, err = col.Aggregate(ctx, mongo.Pipeline{
							{{Key: "$match", Value: filter}},
							{{Key: "$sort", Value: byID}},
						}, options.Aggregate().SetCollation(coll))
						if err != nil {
							return nil, err
						}
						matched, err := collectIDs(ctx, cur)
						if err != nil {
							return nil, err
						}
						n, err := col.CountDocuments(ctx, filter, options.Count().SetCollation(coll))
						if err != nil {
							return nil, err
						}
						return bson.D{
							{Key: "find", Value: found},
							{Key: "aggregate", Value: matched},
							{Key: "count", Value: n},
						}, nil
					},
				})
			})
		}
	}
}
