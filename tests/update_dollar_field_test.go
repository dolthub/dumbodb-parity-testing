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

// Update operators writing a top-level field named with a '$' are refused when
// they modify a stored document, though not when inserting an upserted one
// (dolthub/dumbodb#254).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// updateWriters applies update to filter's documents through update,
// updateMany, findAndModify and bulkWrite.
func updateWriters(filter, update interface{}, upsert bool) []struct {
	name  string
	write func(ctx context.Context, col *mongo.Collection) error
} {
	return []struct {
		name  string
		write func(ctx context.Context, col *mongo.Collection) error
	}{
		{"updateOne", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(upsert))
			return err
		}},
		{"updateMany", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, filter, update, options.Update().SetUpsert(upsert))
			return err
		}},
		{"findAndModify", func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, filter, update, options.FindOneAndUpdate().SetUpsert(upsert)).Err()
		}},
		{"bulkWrite", func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update).SetUpsert(upsert),
			})
			return err
		}},
	}
}

func runUpdateCases(t *testing.T, prefix string, setup []interface{}, cases []struct {
	name   string
	filter bson.D
	update interface{}
	upsert bool
}) {
	for _, c := range cases {
		for _, w := range updateWriters(c.filter, c.update, c.upsert) {
			t.Run(c.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    prefix + "_" + c.name + "_" + w.name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(setup...),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error { return w.write(ctx, col) })
					},
				})
			})
		}
	}
}

func TestUpdateDollarPrefixedField(t *testing.T) {
	one := bson.D{{Key: "_id", Value: 1}}
	none := bson.D{{Key: "_id", Value: 9}}
	set := func(k string) bson.D { return bson.D{{Key: "$set", Value: bson.D{{Key: k, Value: 1}}}} }
	runUpdateCases(t, "UpdateDollarPrefixedField", []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: "x"}, {Key: "a", Value: bson.D{{Key: "b", Value: 1}}}},
		bson.D{{Key: "_id", Value: 2}, {Key: "s", Value: "y"}},
	}, []struct {
		name   string
		filter bson.D
		update interface{}
		upsert bool
	}{
		{"set_top", one, set("$x"), false},
		{"set_top_dotted", one, set("$x.y"), false},
		{"set_nested", one, set("a.$x"), false},
		{"set_many_docs", bson.D{}, set("$x"), false},
		{"set_no_match", none, set("$x"), false},
		{"set_upsert_insert", none, set("$x"), true},
		{"inc_top", one, bson.D{{Key: "$inc", Value: bson.D{{Key: "$x", Value: 1}}}}, false},
		{"push_top", one, bson.D{{Key: "$push", Value: bson.D{{Key: "$x", Value: 1}}}}, false},
		{"unset_top", one, bson.D{{Key: "$unset", Value: bson.D{{Key: "$x", Value: 1}}}}, false},
		{"rename_to_top", one, bson.D{{Key: "$rename", Value: bson.D{{Key: "s", Value: "$x"}}}}, false},
		{"setOnInsert_upsert", none, bson.D{{Key: "$setOnInsert", Value: bson.D{{Key: "$x", Value: 1}}}}, true},
	})
}

// mongod refuses only an update that creates a top-level '$' field or
// replaces one with a different value; a positional element or array filter
// in the first position is refused while parsing.
func TestUpdateDollarPrefixedFieldStored(t *testing.T) {
	x := func(v interface{}) bson.D { return bson.D{{Key: "_id", Value: 1}, {Key: "$x", Value: v}} }
	p := bson.D{{Key: "_id", Value: 1}, {Key: "$p", Value: bson.A{1, 2}}}
	none := bson.D{{Key: "_id", Value: 1}}
	op := func(name, field string, v interface{}) bson.D {
		return bson.D{{Key: name, Value: bson.D{{Key: field, Value: v}}}}
	}
	for _, tc := range []struct {
		name   string
		stored bson.D
		update bson.D
		opts   *options.UpdateOptions
	}{
		{"setOnInsert_without_insert", none, bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 2}}}, {Key: "$setOnInsert", Value: bson.D{{Key: "$x", Value: 1}}}}, nil},
		{"setOnInsert_upsert_match", none, bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 2}}}, {Key: "$setOnInsert", Value: bson.D{{Key: "$x", Value: 1}}}}, options.Update().SetUpsert(true)},
		{"set_unchanged", x(1), bson.D{{Key: "$set", Value: bson.D{{Key: "$x", Value: 1}, {Key: "b", Value: 2}}}}, nil},
		{"set_changed", x(1), op("$set", "$x", 2), nil},
		{"set_changed_document", x(bson.D{{Key: "y", Value: 1}}), op("$set", "$x", bson.D{{Key: "y", Value: 2}}), nil},
		{"set_below_existing", x(bson.D{{Key: "y", Value: 1}}), op("$set", "$x.y", 2), nil},
		{"set_new_below_existing", x(bson.D{{Key: "y", Value: 1}}), op("$set", "$x.z", 2), nil},
		{"set_below_missing", none, op("$set", "$x.y", 2), nil},
		{"max_unchanged", x(5), bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 2}}}, {Key: "$max", Value: bson.D{{Key: "$x", Value: 1}}}}, nil},
		{"max_changed", x(5), op("$max", "$x", 9), nil},
		{"min_new", none, op("$min", "$x", 1), nil},
		{"mul_existing", x(2), op("$mul", "$x", 2), nil},
		{"inc_existing", x(1), op("$inc", "$x", 1), nil},
		{"currentDate_new", none, op("$currentDate", "$x", true), nil},
		{"bit_existing", x(1), op("$bit", "$x", bson.D{{Key: "or", Value: 2}}), nil},
		{"push_existing", p, op("$push", "$p", 3), nil},
		{"addToSet_existing", p, op("$addToSet", "$p", 3), nil},
		{"addToSet_new", none, op("$addToSet", "$p", 3), nil},
		{"pop_existing", p, op("$pop", "$p", 1), nil},
		{"pull_existing", p, op("$pull", "$p", 1), nil},
		{"pullAll_existing", p, op("$pullAll", "$p", bson.A{1}), nil},
		{"rename_onto_existing", bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: 1}, {Key: "$x", Value: 1}}, op("$rename", "s", "$x"), nil},
		{"rename_from_dollar", x(1), op("$rename", "$x", "y"), nil},
		{"unset_existing", x(1), op("$unset", "$x", 1), nil},
		{"positional_first", none, op("$set", "$", 1), nil},
		{"positional_first_dotted", none, op("$set", "$.b", 1), nil},
		{"all_positional_first", none, op("$set", "$[]", 1), nil},
		{"filtered_positional_first", none, op("$set", "$[e]", 1),
			options.Update().SetArrayFilters(options.ArrayFilters{Filters: []interface{}{bson.D{{Key: "e", Value: 1}}}})},
	} {
		for _, target := range []struct {
			name string
			id   int
		}{{"match", 1}, {"no_match", 9}} {
			t.Run(tc.name+"_"+target.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "UpdateDollarPrefixedFieldStored_" + tc.name + "_" + target.name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(tc.stored),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						opts := tc.opts
						if opts == nil {
							opts = options.Update()
						}
						return writeOutcome(ctx, col, func() error {
							_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: target.id}}, tc.update, opts)
							return err
						})
					},
				})
			})
		}
	}
}
