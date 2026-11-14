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

// sbeExplainSetup fills col with 20 documents indexed on g and its "_f"
// collection with 10 to join, indexed on k when foreignIndex is set.
func sbeExplainSetup(foreignIndex bool) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 20)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: bson.A{int32(i % 3), int32(i)}},
				{Key: "g", Value: int32(i % 3)}, {Key: "x", Value: int32(i)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		if err := createIndex(ctx, col, bson.D{{Key: "g", Value: 1}}); err != nil {
			return err
		}
		foreign := col.Database().Collection(col.Name() + "_f")
		_ = foreign.Drop(ctx)
		fdocs := make([]interface{}, 10)
		for i := range fdocs {
			fdocs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "k", Value: int32(i % 3)}}
		}
		if _, err := foreign.InsertMany(ctx, fdocs); err != nil {
			return err
		}
		if foreignIndex {
			return createIndex(ctx, foreign, bson.D{{Key: "k", Value: 1}})
		}
		return nil
	}
}

// sbeQueryPlanStages lists the GROUP and EQ_LOOKUP stages at the top of an
// explain's query plan, which SBE runs, with each join's strategy.
func sbeQueryPlanStages(doc bson.D) []string {
	qp, _ := lookup(doc, "queryPlanner")
	if stages, ok := lookup(doc, "stages"); ok {
		if arr, _ := stages.(bson.A); len(arr) > 0 {
			first, _ := arr[0].(bson.D)
			cursor, _ := lookup(first, "$cursor")
			if c, ok := cursor.(bson.D); ok {
				qp, _ = lookup(c, "queryPlanner")
			}
		}
	}
	qpDoc, _ := qp.(bson.D)
	wp, _ := lookup(qpDoc, "winningPlan")
	wpDoc, _ := wp.(bson.D)
	plan, ok := lookup(wpDoc, "queryPlan")
	if !ok {
		return nil
	}
	var out []string
	for {
		node, _ := plan.(bson.D)
		stage, _ := lookup(node, "stage")
		switch stage {
		case "GROUP":
			out = append(out, "GROUP")
		case "EQ_LOOKUP":
			strategy, _ := lookup(node, "strategy")
			out = append(out, "EQ_LOOKUP "+strategy.(string))
		default:
			return out
		}
		plan, _ = lookup(node, "inputStage")
	}
}

// TestExplainPlan_SBEStages: an aggregate explain shows the stages mongod
// runs in SBE ($group, $bucket, $sortByCount, the $group of a $count, and
// equality $lookups) inside the query plan, under explainVersion "2", and
// the stages its classic engine runs after the query: a $group with a
// $median, a pipeline led by an express _id match, a $group by _id keeping
// one value, a $lookup on an array index path, a $lookup joining across
// collations, a $match reading a join after its $lookup and every stage
// after those. A $skip
// or $limit moves ahead of the stages it cannot affect, into the query.
func TestExplainPlan_SBEStages(t *testing.T) {
	join := func(localField string) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "_f"}, {Key: "localField", Value: localField},
			{Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"}}}}
	}
	group := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}}
	median := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"},
		{Key: "m", Value: bson.D{{Key: "$median", Value: bson.D{{Key: "input", Value: "$x"}, {Key: "method", Value: "approximate"}}}}}}}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}
	bucket := bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: bson.A{0, 10, 20}}}}}
	collation := bson.D{{Key: "locale", Value: "en"}, {Key: "strength", Value: 2}}
	cases := []struct {
		name         string
		pipeline     bson.A
		foreignIndex bool
		collation    bson.D
		// stageNames compares the names of the stages after the query,
		// which a classic pipeline mongod rewrites can show otherwise.
		stageNames bool
	}{
		{name: "group", pipeline: bson.A{group}, stageNames: true},
		{name: "group_median", pipeline: bson.A{median}, stageNames: true},
		{name: "express_match_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: 3}}}}, group}, stageNames: true},
		// mongod runs it as a $groupByDistinctScan over the _id index.
		{name: "group_id_first", pipeline: bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"},
			{Key: "f", Value: bson.D{{Key: "$first", Value: "$x"}}}}}}}},
		{name: "lookup_array_index", pipeline: bson.A{join("a.0")}, stageNames: true},
		{name: "lookup_array_index_group", pipeline: bson.A{join("a.0"), group}, stageNames: true},
		{name: "lookup", pipeline: bson.A{join("g")}, stageNames: true},
		{name: "match_lookup_group", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "g", Value: 1}}}}, join("g"), group}, stageNames: true},
		{name: "group_lookup", pipeline: bson.A{group, join("_id")}, stageNames: true},
		{name: "lookup_count", pipeline: bson.A{join("g"), bson.D{{Key: "$count", Value: "c"}}}, stageNames: true},
		{name: "lookup_match_join_group", pipeline: bson.A{join("g"), bson.D{{Key: "$match", Value: bson.D{{Key: "j.k", Value: 1}}}}, group}, stageNames: true},
		{name: "lookup_match_group", pipeline: bson.A{join("g"), bson.D{{Key: "$match", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$gt", Value: 3}}}}}}, group}, stageNames: true},
		{name: "lookup_group_median", pipeline: bson.A{join("g"), median}, stageNames: true},
		{name: "group_set", pipeline: bson.A{group, set}, stageNames: true},
		{name: "group_median_set", pipeline: bson.A{median, set}, stageNames: true},
		{name: "group_group", pipeline: bson.A{group, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "t", Value: bson.D{{Key: "$sum", Value: "$n"}}}}}}}, stageNames: true},
		{name: "sort_group", pipeline: bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "x", Value: 1}}}}, group}, stageNames: true},
		{name: "bucket_set", pipeline: bson.A{bucket, set}, stageNames: true},
		{name: "set_bucket", pipeline: bson.A{set, bucket}, stageNames: true},
		{name: "sort_by_count", pipeline: bson.A{bson.D{{Key: "$sortByCount", Value: "$g"}}}, stageNames: true},
		{name: "collated_lookup_group", pipeline: bson.A{join("g"), group}, collation: collation, stageNames: true},
		{name: "collated_indexed_lookup_group", pipeline: bson.A{join("g"), group}, foreignIndex: true, collation: collation, stageNames: true},
		{name: "set_skip", pipeline: bson.A{set, bson.D{{Key: "$skip", Value: 2}}}, stageNames: true},
		{name: "skip_set_limit", pipeline: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "g", Value: 1}}}}, bson.D{{Key: "$skip", Value: 1}}, set,
			bson.D{{Key: "$limit", Value: 2}}}, stageNames: true},
		{name: "set_limit_group", pipeline: bson.A{set, bson.D{{Key: "$limit", Value: 5}}, group}, stageNames: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_SBEStages_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   sbeExplainSetup(tc.foreignIndex),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					pipeline := make(bson.A, len(tc.pipeline))
					for i, s := range tc.pipeline {
						pipeline[i] = foreignNamed(s.(bson.D), col.Name())
					}
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}
					if tc.collation != nil {
						cmd = append(cmd, bson.E{Key: "collation", Value: tc.collation})
					}
					doc, err := explainRunExplain(ctx, col, cmd, "executionStats")
					if err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					out := bson.D{{Key: "explainVersion", Value: ver}, {Key: "sbe", Value: sbeQueryPlanStages(doc)}}
					if es, ok := lookup(doc, "executionStats"); ok {
						n, _ := lookup(es.(bson.D), "nReturned")
						out = append(out, bson.E{Key: "nReturned", Value: n})
					}
					stages := topLevelStages(doc)
					if !tc.stageNames {
						for i, s := range stages {
							n, _ := lookup(s.(bson.D), "nReturned")
							stages[i] = n
						}
					}
					return append(out, bson.E{Key: "stages", Value: stages}), nil
				},
			})
		})
	}
}

// foreignNamed is stage with a $lookup from "_f" reading coll's "_f"
// collection.
func foreignNamed(stage bson.D, coll string) bson.D {
	if len(stage) != 1 || stage[0].Key != "$lookup" {
		return stage
	}
	spec := append(bson.D{}, stage[0].Value.(bson.D)...)
	for i, e := range spec {
		if e.Key == "from" {
			spec[i].Value = coll + e.Value.(string)
		}
	}
	return bson.D{{Key: "$lookup", Value: spec}}
}

// TestAggregate_SBEAfterLookup: mongod runs a $group in its classic engine,
// reporting the classic engine's expression errors, when a $match reading
// the join comes between it and a $lookup, or when the $lookup joins across
// collations on a field a foreign index leads.
func TestAggregate_SBEAfterLookup(t *testing.T) {
	join := func(coll string) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: coll + "_f"}, {Key: "localField", Value: "g"},
			{Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"}}}}
	}
	concat := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil},
		{Key: "s", Value: bson.D{{Key: "$push", Value: bson.D{{Key: "$concat", Value: bson.A{"$x"}}}}}}}}}
	cases := []struct {
		name         string
		pipeline     func(coll string) bson.A
		foreignIndex bool
		collated     bool
	}{
		{"lookup_group", func(c string) bson.A { return bson.A{join(c), concat} }, false, false},
		{"lookup_match_join_group", func(c string) bson.A {
			return bson.A{join(c), bson.D{{Key: "$match", Value: bson.D{{Key: "j.k", Value: 1}}}}, concat}
		}, false, false},
		{"collated_lookup_group", func(c string) bson.A { return bson.A{join(c), concat} }, false, true},
		{"collated_indexed_lookup_group", func(c string) bson.A { return bson.A{join(c), concat} }, true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Aggregate_SBEAfterLookup_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   sbeExplainSetup(tc.foreignIndex),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Aggregate()
					if tc.collated {
						opts.SetCollation(&options.Collation{Locale: "en", Strength: 2})
					}
					cursor, err := col.Aggregate(ctx, tc.pipeline(col.Name()), opts)
					return drainCursor(ctx, cursor, err)
				},
			})
		})
	}
}
