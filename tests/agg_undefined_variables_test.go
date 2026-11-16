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

// mongod refuses a reference to an undefined variable with 17276, and a
// malformed '$' string expression with its FieldPath error, when it parses the
// pipeline, so even an empty collection fails; $set and $addFields never store
// such a string as a literal (dolthub/dumbodb#246, fixed by
// dolthub/dumbodb#245).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestAggUndefinedVariables(t *testing.T) {
	stage := func(name string, spec interface{}) bson.A { return bson.A{bson.D{{Key: name, Value: spec}}} }
	field := func(v interface{}) bson.D { return bson.D{{Key: "c", Value: v}} }
	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"project", stage("$project", field("$$nope"))},
		{"project_id", stage("$project", bson.D{{Key: "_id", Value: "$$nope"}})},
		{"project_nested", stage("$project", field(bson.D{{Key: "d", Value: "$$nope"}}))},
		{"project_empty_path_element", stage("$project", field("$a..b"))},
		{"set", stage("$set", field("$$nope"))},
		{"set_dotted", stage("$set", field("$$nope.x"))},
		{"set_nested", stage("$set", field(bson.D{{Key: "d", Value: "$$nope"}}))},
		{"set_in_array", stage("$set", field(bson.A{"$$nope"}))},
		{"addFields", stage("$addFields", field("$$nope"))},
		{"set_uppercase", stage("$set", field("$$Upper"))},
		{"set_empty_path_element", stage("$set", field("$a..b"))},
		{"set_trailing_dot", stage("$set", field("$a."))},
		{"set_dollar_element", stage("$set", field("$a.$b"))},
		{"set_dollar", stage("$set", field("$"))},
		{"set_empty_variable", stage("$set", field("$$"))},
		{"set_invalid_variable", stage("$set", field("$$$x"))},
		{"group_id", stage("$group", bson.D{{Key: "_id", Value: "$$nope"}})},
		{"group_accumulator", stage("$group", bson.D{{Key: "_id", Value: nil}, {Key: "s", Value: bson.D{{Key: "$sum", Value: "$$nope"}}}})},
		{"sortByCount", stage("$sortByCount", "$$nope")},
		{"bucket", stage("$bucket", bson.D{{Key: "groupBy", Value: "$$nope"}, {Key: "boundaries", Value: bson.A{0, 5}}})},
		{"bucketAuto", stage("$bucketAuto", bson.D{{Key: "groupBy", Value: "$$nope"}, {Key: "buckets", Value: 2}})},
		{"redact", stage("$redact", "$$nope")},
		{"replaceWith", stage("$replaceWith", bson.D{{Key: "x", Value: "$$nope"}})},
		{"facet", stage("$facet", bson.D{{Key: "x", Value: stage("$set", field("$$nope"))}})},
		{"set_defined_root", stage("$set", field("$$ROOT.a"))},
		{"set_defined_remove", stage("$set", bson.D{{Key: "a", Value: "$$REMOVE"}})},
		{"set_map_this", stage("$set", field(bson.D{{Key: "$map", Value: bson.D{{Key: "input", Value: "$arr"}, {Key: "in", Value: "$$this"}}}}))},
		{"set_let", stage("$set", field(bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "q", Value: 2}}}, {Key: "in", Value: "$$q"}}}}))},
		{"redact_descend", stage("$redact", "$$DESCEND")},
	}
	run := func(ctx context.Context, col *mongo.Collection, pipeline bson.A) (interface{}, error) {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var docs []bson.D
		err = cur.All(ctx, &docs)
		return docs, err
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AggUndefinedVariables_" + p.name,
				Support: harness.DumboDBFull,
				Setup:   insertDocs(bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}, {Key: "arr", Value: bson.A{1, 2}}}),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return run(ctx, col, p.pipeline)
				},
			})
		})
		t.Run(p.name+"_empty_collection", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "AggUndefinedVariables_" + p.name + "_empty",
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return run(ctx, col, p.pipeline)
				},
			})
		})
	}
}

func TestAggUndefinedVariablesInUpdates(t *testing.T) {
	pipeline := bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$nope"}}}}}
	for _, target := range []struct {
		name string
		id   int
	}{{"match", 1}, {"no_match", 9}} {
		for _, w := range []struct {
			name  string
			write func(ctx context.Context, col *mongo.Collection, filter bson.D) error
		}{
			{"updateOne", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
				_, err := col.UpdateOne(ctx, filter, pipeline)
				return err
			}},
			{"findAndModify", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
				return col.FindOneAndUpdate(ctx, filter, pipeline, options.FindOneAndUpdate()).Err()
			}},
			{"bulkWrite", func(ctx context.Context, col *mongo.Collection, filter bson.D) error {
				_, err := col.BulkWrite(ctx, []mongo.WriteModel{mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(pipeline)})
				return err
			}},
		} {
			t.Run(target.name+"_"+w.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "AggUndefinedVariablesInUpdates_" + target.name + "_" + w.name,
					Support: harness.DumboDBFull,
					Setup:   insertDocs(bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}}),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return writeOutcome(ctx, col, func() error {
							return w.write(ctx, col, bson.D{{Key: "_id", Value: target.id}})
						})
					},
				})
			})
		}
	}
}
