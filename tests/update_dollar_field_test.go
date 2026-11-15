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

func runUpdateCases(t *testing.T, prefix string, xfail map[string]bool, setup []interface{}, cases []struct {
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
					Support: supportFor(t, xfail),
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
	runUpdateCases(t, "UpdateDollarPrefixedField", nil, []interface{}{
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
