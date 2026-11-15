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

// mongod parses arrayFilters with the update before reading a document and
// refuses an identifier the update does not use (dolthub/dumbodb#244), along
// with a malformed filter or a missing one.

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// arrayFilterWriters applies update with arrayFilters to filter's documents
// through update, updateMany, findAndModify and bulkWrite.
func arrayFilterWriters(filter, update interface{}, filters []interface{}) []struct {
	name  string
	write func(ctx context.Context, col *mongo.Collection) error
} {
	af := options.ArrayFilters{Filters: filters}
	return []struct {
		name  string
		write func(ctx context.Context, col *mongo.Collection) error
	}{
		{"updateOne", func(ctx context.Context, col *mongo.Collection) error {
			opts := options.Update()
			if filters != nil {
				opts.SetArrayFilters(af)
			}
			_, err := col.UpdateOne(ctx, filter, update, opts)
			return err
		}},
		{"updateMany", func(ctx context.Context, col *mongo.Collection) error {
			opts := options.Update()
			if filters != nil {
				opts.SetArrayFilters(af)
			}
			_, err := col.UpdateMany(ctx, filter, update, opts)
			return err
		}},
		{"findAndModify", func(ctx context.Context, col *mongo.Collection) error {
			opts := options.FindOneAndUpdate().SetSort(bson.D{{Key: "_id", Value: 1}})
			if filters != nil {
				opts.SetArrayFilters(af)
			}
			return col.FindOneAndUpdate(ctx, filter, update, opts).Err()
		}},
		{"bulkWrite", func(ctx context.Context, col *mongo.Collection) error {
			model := mongo.NewUpdateManyModel().SetFilter(filter).SetUpdate(update)
			if filters != nil {
				model.SetArrayFilters(af)
			}
			_, err := col.BulkWrite(ctx, []mongo.WriteModel{model})
			return err
		}},
	}
}

func runArrayFilterCases(t *testing.T, prefix string, xfail map[string]bool, setup []interface{}, cases []struct {
	name    string
	filter  bson.D
	update  interface{}
	filters []interface{}
}) {
	for _, c := range cases {
		for _, w := range arrayFilterWriters(c.filter, c.update, c.filters) {
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

func TestArrayFiltersValidation(t *testing.T) {
	one := bson.D{{Key: "_id", Value: 1}}
	none := bson.D{{Key: "_id", Value: 9}}
	setB := bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 5}}}}
	setE := bson.D{{Key: "$set", Value: bson.D{{Key: "a.$[e]", Value: 5}}}}
	gt1 := func(id string) bson.D { return bson.D{{Key: id, Value: bson.D{{Key: "$gt", Value: 1}}}} }
	runArrayFilterCases(t, "ArrayFiltersValidation", nil, []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: 0}},
	}, []struct {
		name    string
		filter  bson.D
		update  interface{}
		filters []interface{}
	}{
		{"unused", one, setB, []interface{}{gt1("x")}},
		{"unused_no_match", none, setB, []interface{}{gt1("x")}},
		{"one_of_two_unused", one, setE, []interface{}{gt1("e"), gt1("x")}},
		{"used_by_other_operator", one, bson.D{
			{Key: "$set", Value: bson.D{{Key: "b", Value: 5}}},
			{Key: "$inc", Value: bson.D{{Key: "a.$[x]", Value: 1}}},
		}, []interface{}{gt1("x")}},
		{"missing", one, setE, nil},
		{"missing_no_match", none, setE, nil},
		{"duplicate", none, setE, []interface{}{gt1("e"), gt1("e")}},
		{"two_identifiers", none, setE, []interface{}{bson.D{{Key: "e", Value: 1}, {Key: "f", Value: 2}}}},
		{"uppercase_identifier", none, bson.D{{Key: "$set", Value: bson.D{{Key: "a.$[E]", Value: 5}}}}, []interface{}{gt1("E")}},
		{"empty_filter", none, setE, []interface{}{bson.D{}}},
		{"pipeline_update", none, bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}}, []interface{}{gt1("e")}},
	})
}
