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

// distinctScanSetup fills col with 20 documents, indexed on b and on c then
// a.
func distinctScanSetup(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 20)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i)}, {Key: "b", Value: int32(i % 4)}, {Key: "c", Value: int32(i % 5)}}
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	if err := createIndex(ctx, col, bson.D{{Key: "b", Value: 1}}); err != nil {
		return err
	}
	return createIndex(ctx, col, bson.D{{Key: "c", Value: 1}, {Key: "a", Value: 1}})
}

// winningPlanStages lists the stages of an explain's winning plan, the
// query's under $cursor when the explain has stages, each with its index.
func winningPlanStages(doc bson.D) []string {
	qp, _ := lookup(doc, "queryPlanner")
	if stages, ok := lookup(doc, "stages"); ok {
		if arr, _ := stages.(bson.A); len(arr) > 0 {
			first, _ := arr[0].(bson.D)
			if cursor, ok := lookup(first, "$cursor"); ok {
				qp, _ = lookup(cursor.(bson.D), "queryPlanner")
			}
		}
	}
	qpDoc, _ := qp.(bson.D)
	plan, _ := lookup(qpDoc, "winningPlan")
	if wp, ok := plan.(bson.D); ok {
		if inner, ok := lookup(wp, "queryPlan"); ok {
			plan = inner
		}
	}
	var out []string
	for plan != nil {
		node, _ := plan.(bson.D)
		stage, _ := lookup(node, "stage")
		name, _ := stage.(string)
		if index, ok := lookup(node, "indexName"); ok {
			name += " " + index.(string)
		}
		out = append(out, name)
		plan, _ = lookup(node, "inputStage")
	}
	return out
}

// TestExplainPlan_DistinctScanGroup: with an index led by its key, mongod
// runs a $group by a field path, keeping no value or one value of each group
// ($first, $last, or $top or $bottom sorted by the key), after only $match
// and $sort stages its index serves, as a $groupByDistinctScan over a
// DISTINCT_SCAN, under explainVersion "1"; it fetches unless the index covers
// what the group keeps. Mixed accumulators, a filter on another field, an
// unindexed key, or a clustered collection's _id keep the $group in SBE.
func TestExplainPlan_DistinctScanGroup(t *testing.T) {
	group := func(fields ...bson.E) bson.D {
		return bson.D{{Key: "$group", Value: append(bson.D{{Key: "_id", Value: "$b"}}, fields...)}}
	}
	acc := func(name, op string, arg interface{}) bson.E {
		return bson.E{Key: name, Value: bson.D{{Key: op, Value: arg}}}
	}
	sortB := func(dir int32) bson.D { return bson.D{{Key: "$sort", Value: bson.D{{Key: "b", Value: dir}}}} }
	top := func(op string, dir int32) bson.E {
		return acc("t", op, bson.D{{Key: "output", Value: "$a"}, {Key: "sortBy", Value: bson.D{{Key: "b", Value: dir}}}})
	}
	cases := []struct {
		name      string
		pipeline  bson.A
		clustered bool
	}{
		{name: "group", pipeline: bson.A{group()}},
		{name: "group_first", pipeline: bson.A{group(acc("f", "$first", "$a"))}},
		{name: "group_first_expression", pipeline: bson.A{group(acc("f", "$first", bson.D{{Key: "$add", Value: bson.A{"$a", 1}}}))}},
		{name: "group_first_root", pipeline: bson.A{group(acc("f", "$first", "$$ROOT"))}},
		{name: "group_root_path", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$$ROOT.b"}}}}}},
		{name: "group_object_id", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "k", Value: "$b"}}}}}}}},
		{name: "match_range_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$gte", Value: 1}}}}}}, group()}},
		{name: "match_eq_group_first", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "b", Value: 1}}}}, group(acc("f", "$first", "$a"))}},
		{name: "match_in_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$in", Value: bson.A{0, 2}}}}}}}, group()}},
		{name: "match_ne_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$ne", Value: 0}}}}}}, group()}},
		{name: "two_matches_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: 0}}}}}},
			bson.D{{Key: "$match", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$lt", Value: 3}}}}}}, group()}},
		{name: "sort_group_first", pipeline: bson.A{sortB(1), group(acc("f", "$first", "$a"))}},
		{name: "sort_group_last", pipeline: bson.A{sortB(1), group(acc("l", "$last", "$a"))}},
		{name: "sort_desc_group_first", pipeline: bson.A{sortB(-1), group(acc("f", "$first", "$a"))}},
		{name: "group_top", pipeline: bson.A{group(top("$top", 1))}},
		{name: "group_bottom", pipeline: bson.A{group(top("$bottom", 1))}},
		{name: "compound_covered", pipeline: bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "c", Value: 1}, {Key: "a", Value: 1}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$c"}, {Key: "f", Value: bson.D{{Key: "$first", Value: "$a"}}}}}}}},
		{name: "group_id", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"}}}}}},
		{name: "group_then_stages", pipeline: bson.A{group(acc("f", "$first", "$a")), bson.D{{Key: "$match", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$gt", Value: 0}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}},
		{name: "group_group", pipeline: bson.A{group(acc("f", "$first", "$a")), bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}}},
		// These run in SBE.
		{name: "group_first_last", pipeline: bson.A{group(acc("f", "$first", "$a"), acc("l", "$last", "$a"))}},
		{name: "group_sum", pipeline: bson.A{group(acc("n", "$sum", 1))}},
		{name: "group_top_other_sort", pipeline: bson.A{group(acc("t", "$top", bson.D{{Key: "output", Value: "$a"}, {Key: "sortBy", Value: bson.D{{Key: "a", Value: 1}}}}))}},
		{name: "sort_group_top_reversed", pipeline: bson.A{sortB(1), group(top("$top", -1))}},
		{name: "match_other_field_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 3}}}}}}, group()}},
		{name: "unindexed_key", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$a"}}}}}},
		{name: "two_field_id", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "c", Value: "$c"}, {Key: "a", Value: "$a"}}}}}}}},
		{name: "id_first_last", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"},
			{Key: "f", Value: bson.D{{Key: "$first", Value: "$a"}}}, {Key: "l", Value: bson.D{{Key: "$last", Value: "$a"}}}}}}}},
		{name: "clustered_group_id_first", clustered: true, pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"},
			{Key: "f", Value: bson.D{{Key: "$first", Value: "$a"}}}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_DistinctScanGroup_" + tc.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if tc.clustered {
						if err := col.Drop(ctx); err != nil {
							return err
						}
						opts := options.CreateCollection().SetClusteredIndex(harness.ClusteredIndexOption)
						if err := col.Database().CreateCollection(ctx, col.Name(), opts); err != nil {
							return err
						}
					}
					return distinctScanSetup(ctx, col)
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: tc.pipeline}, {Key: "cursor", Value: bson.D{}}}
					doc, err := explainRunExplain(ctx, col, cmd, "executionStats")
					if err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					out := bson.D{{Key: "explainVersion", Value: ver}, {Key: "plan", Value: winningPlanStages(doc)}}
					if es := findExecutionStats(doc); es != nil {
						for _, k := range []string{"nReturned", "totalKeysExamined", "totalDocsExamined"} {
							v, _ := lookup(es, k)
							out = append(out, bson.E{Key: k, Value: v})
						}
					}
					return append(out, bson.E{Key: "stages", Value: topLevelStages(doc)}), nil
				},
			})
		})
	}
}

// TestAggregate_DistinctScanGroup: mongod runs a distinct scan $group in its
// classic engine, which evaluates each kept document's expressions with the
// classic engine's errors.
func TestAggregate_DistinctScanGroup(t *testing.T) {
	group := func(arg interface{}) bson.A {
		return bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "b", Value: 1}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$b"}, {Key: "f", Value: bson.D{{Key: "$first", Value: arg}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		}
	}
	cases := []struct {
		name     string
		pipeline bson.A
	}{
		{"first", group("$a")},
		{"add_string", group(bson.D{{Key: "$add", Value: bson.A{"$a", "s"}}})},
		{"divide_zero", group(bson.D{{Key: "$divide", Value: bson.A{"$a", 0}}})},
		{"keys", bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$b"}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Aggregate_DistinctScanGroup_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   distinctScanSetup,
				Run:     pipelineRun(tc.pipeline),
			})
		})
	}
}
