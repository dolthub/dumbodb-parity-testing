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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Explain-plan parity: explain must describe the plan the server runs, with
// the counts that plan really produces, stage by stage.

// explainStageWork flattens an executionStages tree into each stage's name,
// index, and the counters it reports.
func explainStageWork(plan interface{}) []bson.D {
	doc, ok := asDoc(plan)
	if !ok {
		return nil
	}
	entry := bson.D{}
	for _, k := range []string{"stage", "indexName", "nReturned", "keysExamined", "docsExamined"} {
		if v, ok := lookup(doc, k); ok {
			entry = append(entry, bson.E{Key: k, Value: v})
		}
	}
	out := []bson.D{entry}
	if v, ok := lookup(doc, "inputStage"); ok {
		out = append(out, explainStageWork(v)...)
	}
	if v, ok := lookup(doc, "inputStages"); ok {
		if arr, ok := v.(bson.A); ok {
			for _, child := range arr {
				out = append(out, explainStageWork(child)...)
			}
		}
	}
	return out
}

// explainWork reduces an executionStats explain to its winning plan, its
// totals, and the counters of each executed stage.
func explainWork(doc bson.D) bson.D {
	out := bson.D{}
	if qp := findQueryPlanner(doc); qp != nil {
		if wp, ok := lookup(qp, "winningPlan"); ok {
			out = append(out, bson.E{Key: "winningPlan", Value: explainExtractPlan(wp)})
		}
	}
	if es := findExecutionStats(doc); es != nil {
		for _, k := range []string{"nReturned", "totalKeysExamined", "totalDocsExamined"} {
			if v, ok := lookup(es, k); ok {
				out = append(out, bson.E{Key: k, Value: v})
			}
		}
		if stages, ok := lookup(es, "executionStages"); ok {
			out = append(out, bson.E{Key: "executionStages", Value: explainStageWork(stages)})
		}
	}
	return out
}

func explainWorkOf(inner bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		doc, err := explainRunExplain(ctx, col, inner, "executionStats")
		if err != nil {
			return nil, err
		}
		return explainWork(doc), nil
	}
}

func updateGrpCmd(col *mongo.Collection, filter bson.D, multi bool) bson.D {
	return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{bson.D{
		{Key: "q", Value: filter},
		{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: int32(1)}}}}},
		{Key: "multi", Value: multi},
	}}}}
}

func deleteGrpCmd(col *mongo.Collection, filter bson.D, limit int32) bson.D {
	return bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{bson.D{
		{Key: "q", Value: filter}, {Key: "limit", Value: limit},
	}}}}
}

// setupGrp10 seeds 1,000 documents with grp = _id % 10 and indexes grp.
func setupGrp10(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 0, 1000)
	for i := 0; i < 1000; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "grp", Value: int32(i % 10)}})
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}})
}

// TestExplainPlan_IdEquality: an _id equality alone is served by an EXPRESS
// stage on _id_; with another field it is a FETCH of an _id_ IXSCAN
// (dolthub/dumbodb#143).
func TestExplainPlan_IdEquality(t *testing.T) {
	id := bson.D{{Key: "_id", Value: int32(7)}}
	idAndGrp := bson.D{{Key: "_id", Value: int32(7)}, {Key: "grp", Value: int32(7)}}
	setT := bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: int32(1)}}}}
	cases := []struct {
		name  string
		inner func(*mongo.Collection) bson.D
	}{
		{"find", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: id}}
		}},
		{"update", func(col *mongo.Collection) bson.D { return updateGrpCmd(col, id, false) }},
		{"delete", func(col *mongo.Collection) bson.D { return deleteGrpCmd(col, id, 1) }},
		{"find_with_other_field", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: idAndGrp}}
		}},
		{"update_with_other_field", func(col *mongo.Collection) bson.D { return updateGrpCmd(col, idAndGrp, false) }},
		{"find_and_modify", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: id}, {Key: "update", Value: setT}}
		}},
		{"find_and_modify_remove", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: id}, {Key: "remove", Value: true}}
		}},
		{"find_and_modify_by_index", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{{Key: "grp", Value: int32(3)}}}, {Key: "update", Value: setT}}
		}},
	}
	for _, tc := range cases {
		support := harness.DumboDBFull
		if tc.name == "update_with_other_field" {
			// A single-document update over a clustered _id scan examines the
			// document after its match on MongoDB; DumboDB stops at the match.
			support = harness.DumboDBXFail
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_IdEquality_" + tc.name,
				Support:   support,
				Clustered: true,
				Setup:     setupGrp10,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.inner(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_WriteExecutionStats: a multi update or delete through an
// index reports the keys and documents its scan examined, stage by stage
// (dolthub/dumbodb#145).
func TestExplainPlan_WriteExecutionStats(t *testing.T) {
	grp := bson.D{{Key: "grp", Value: int32(3)}}
	cases := []struct {
		name  string
		inner func(*mongo.Collection) bson.D
	}{
		{"update_multi", func(col *mongo.Collection) bson.D { return updateGrpCmd(col, grp, true) }},
		{"delete_multi", func(col *mongo.Collection) bson.D { return deleteGrpCmd(col, grp, 0) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_WriteExecutionStats_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setupGrp10,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.inner(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_OrUnion: an $or with an indexed field in every branch is a
// union of index scans, each reporting its own counts (dolthub/dumbodb#132).
func TestExplainPlan_OrUnion(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if err := setupGrpK(ctx, col); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}})
	}
	or := func(branches ...bson.D) bson.D {
		arr := bson.A{}
		for _, b := range branches {
			arr = append(arr, b)
		}
		return bson.D{{Key: "$or", Value: arr}}
	}
	k := bson.D{{Key: "k", Value: int32(5000)}}
	grpK := bson.D{{Key: "grp", Value: int32(1)}, {Key: "k", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}
	cases := []struct {
		name    string
		support harness.DumboDBSupport
		filter  bson.D
	}{
		// MongoDB orders an $or's index scans by index creation order (k_1
		// before grp_1); DumboDB uses index name order.
		{"single_field_branches", harness.DumboDBXFail, or(k, bson.D{{Key: "grp", Value: int32(1)}})},
		// The branch is served by the compound index bounding both its fields.
		{"compound_branch", harness.DumboDBFull, or(k, grpK)},
		// Branches of one predicate on k_1 are one scan of their intervals.
		// XFail: index creation order, as above.
		{"nested_or", harness.DumboDBXFail, or(k, or(bson.D{{Key: "grp", Value: int32(1)}}, bson.D{{Key: "k", Value: int32(7)}}))},
		// A one-branch $or is planned as its branch, without SUBPLAN.
		{"single_branch", harness.DumboDBFull, or(grpK)},
		// Equalities on one field are planned as an $in, without SUBPLAN.
		{"equalities_on_one_field", harness.DumboDBFull, or(bson.D{{Key: "k", Value: int32(1)}}, bson.D{{Key: "k", Value: int32(2)}})},
		// Branches scan in index order, a branch of several predicates as
		// soon as it is reached, one of a single predicate once a branch on
		// another index follows. XFail: index creation order, as above.
		{"branch_order", harness.DumboDBXFail, or(bson.D{{Key: "grp", Value: int32(3)}}, k, grpK, bson.D{{Key: "k", Value: int32(5)}, {Key: "_id", Value: bson.D{{Key: "$gte", Value: int32(0)}}}})},
		{"overlapping_intervals", harness.DumboDBFull, or(bson.D{{Key: "k", Value: bson.D{{Key: "$gt", Value: int32(9995)}}}}, bson.D{{Key: "k", Value: int32(9997)}}, bson.D{{Key: "k", Value: int32(7)}})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_OrUnion_" + tc.name,
				Support: tc.support,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: tc.filter}})(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_InIntervals: an $in is one IXSCAN of an interval per value,
// examining the key past each interval it lands on outside them.
func TestExplainPlan_InIntervals(t *testing.T) {
	in := func(vs ...int32) bson.D {
		arr := bson.A{}
		for _, v := range vs {
			arr = append(arr, v)
		}
		return bson.D{{Key: "k", Value: bson.D{{Key: "$in", Value: arr}}}}
	}
	cases := []struct {
		name   string
		filter bson.D
	}{
		{"apart", in(7, 5000)},
		{"adjacent", in(7, 8, 5000)},
		{"past_the_last_key", in(7, 9999)},
		{"missing_value", in(-5, 7, 20000)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_InIntervals_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setupGrpK,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: tc.filter}})(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_OverHalf: a range over most of the collection. mongod reads
// the index; DumboDB deliberately scans the collection past half of it, and
// explains the scan it runs (dolthub/dumbodb#139).
func TestExplainPlan_OverHalf(t *testing.T) {
	expectRoot := func(stage string) func(*testing.T, interface{}, error) {
		return func(t *testing.T, res interface{}, err error) {
			if err != nil {
				t.Fatal(err)
			}
			plan, _ := lookup(res.(bson.D), "winningPlan")
			stages := plan.([]bson.D)
			if got, _ := lookup(stages[len(stages)-1], "stage"); got != stage {
				t.Errorf("winning plan %v, want a %s leaf", stages, stage)
			}
		}
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "ExplainPlan_OverHalf",
		Support: harness.DumboDBDeviates,
		Setup:   setupGrpK,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			wide := bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(1000)}}}}
			return explainWorkOf(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: wide}})(ctx, col)
		},
		MongoExpect: expectRoot("IXSCAN"),
		DumboExpect: expectRoot("COLLSCAN"),
	})
}

// TestExplainPlan_InSortLimit: $in on the leading field of a compound index,
// sorted on the next field with a limit, merges one sorted scan per $in value
// (SORT_MERGE) and stops at the limit (dolthub/dumbodb#146).
func TestExplainPlan_InSortLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ExplainPlan_InSortLimit",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 0, 10000)
			for i := 0; i < 10000; i++ {
				docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "bucket", Value: int32(i % 50)}, {Key: "k", Value: int32(i)}})
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "bucket", Value: 1}, {Key: "k", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return explainWorkOf(bson.D{
				{Key: "find", Value: col.Name()},
				{Key: "filter", Value: bson.D{{Key: "bucket", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(3)}}}}}},
				{Key: "sort", Value: bson.D{{Key: "k", Value: 1}}},
				{Key: "limit", Value: int32(5)},
			})(ctx, col)
		},
	})
}

// lookupJoins returns what explain reports for a pipeline's $lookup joins:
// each EQ_LOOKUP stage's strategy and index when the join runs in the query
// plan (SBE), and each classic $lookup stage's indexesUsed.
func lookupJoins(doc bson.D) bson.D {
	var strategies, indexes []interface{}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case bson.D:
			if st, _ := lookup(t, "stage"); st == "EQ_LOOKUP" {
				strategy, _ := lookup(t, "strategy")
				strategies = append(strategies, strategy)
				if ix, ok := lookup(t, "indexName"); ok {
					indexes = append(indexes, ix)
				}
			}
			if used, ok := lookup(t, "indexesUsed"); ok {
				if arr, ok := used.(bson.A); ok {
					indexes = append(indexes, arr...)
				}
			}
			for _, e := range t {
				walk(e.Value)
			}
		case bson.A:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(doc)
	return bson.D{{Key: "strategies", Value: strategies}, {Key: "foreignIndexes", Value: indexes}}
}

// TestExplainPlan_LookupForeignIndex: a $lookup reads its foreign collection
// through an index on the joined field, and explain says so: an equality join
// as an EQ_LOOKUP stage of the query plan, a join with a sub-pipeline in its
// $lookup stage's indexesUsed (dolthub/dumbodb#144).
func TestExplainPlan_LookupForeignIndex(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		local := make([]interface{}, 0, 100)
		for i := 0; i < 100; i++ {
			local = append(local, bson.D{{Key: "_id", Value: int32(i)}, {Key: "fk", Value: int32(i)}})
		}
		if _, err := col.InsertMany(ctx, local); err != nil {
			return err
		}
		foreign := col.Database().Collection(col.Name() + "_f")
		docs := make([]interface{}, 0, 1000)
		for i := 0; i < 1000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "ref", Value: int32(i % 100)}, {Key: "grp", Value: int32(i % 10)}})
		}
		if _, err := foreign.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, foreign, bson.D{{Key: "ref", Value: 1}})
	}
	equality := func(from, local, foreign string, extra ...bson.E) bson.D {
		spec := append(bson.D{{Key: "from", Value: from}, {Key: "localField", Value: local}, {Key: "foreignField", Value: foreign}}, extra...)
		return bson.D{{Key: "$lookup", Value: append(spec, bson.E{Key: "as", Value: "j"})}}
	}
	cases := []struct {
		name      string
		verbosity string
		pipeline  func(from string) bson.A
	}{
		{"equality", "queryPlanner", func(from string) bson.A { return bson.A{equality(from, "fk", "ref")} }},
		{"equality_unindexed", "queryPlanner", func(from string) bson.A { return bson.A{equality(from, "fk", "grp")} }},
		{"let_pipeline", "executionStats", func(from string) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: from},
				{Key: "let", Value: bson.D{{Key: "r", Value: "$fk"}}},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$ref", "$$r"}}}}}}}}},
				{Key: "as", Value: "j"},
			}}}}
		}},
		{"nested", "executionStats", func(from string) bson.A {
			return bson.A{equality(from, "fk", "ref", bson.E{Key: "pipeline", Value: bson.A{equality(from, "grp", "ref")}})}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_LookupForeignIndex_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					doc, err := explainRunExplain(ctx, col, bson.D{
						{Key: "aggregate", Value: col.Name()},
						{Key: "pipeline", Value: tc.pipeline(col.Name() + "_f")},
						{Key: "cursor", Value: bson.D{}},
					}, tc.verbosity)
					if err != nil {
						return nil, err
					}
					return lookupJoins(doc), nil
				},
			})
		})
	}
}

// TestExplainPlan_CountingGroup: a pipeline that only counts the documents its
// $match selects runs in the query plan, as a GROUP over a scan that reads no
// document when the index bounds the whole filter.
func TestExplainPlan_CountingGroup(t *testing.T) {
	count := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: int32(1)}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}}
	match := func(filter bson.D) bson.D { return bson.D{{Key: "$match", Value: filter}} }
	grp := func(v interface{}) bson.D { return bson.D{{Key: "grp", Value: v}} }
	cases := []struct {
		name     string
		pipeline bson.A
	}{
		{"count_scan", bson.A{match(grp(bson.D{{Key: "$gte", Value: int32(500)}})), count}},
		{"compound_count_scan", bson.A{match(bson.D{{Key: "grp", Value: int32(5)}, {Key: "k", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}), count}},
		{"in_intervals", bson.A{match(grp(bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2)}}})), count}},
		{"filter_outside_index", bson.A{match(bson.D{{Key: "grp", Value: int32(5)}, {Key: "t", Value: int32(1)}}), count}},
		{"collscan", bson.A{match(bson.D{{Key: "t", Value: int32(5)}}), count}},
		{"no_match", bson.A{count}},
		{"count_stage", bson.A{match(grp(int32(5))), bson.D{{Key: "$count", Value: "n"}}}},
		{"stage_after_count", bson.A{match(grp(int32(5))), count, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: int32(0)}}}}}},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if err := setupGrpK(ctx, col); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_CountingGroup_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					doc, err := explainRunExplain(ctx, col, bson.D{
						{Key: "aggregate", Value: col.Name()},
						{Key: "pipeline", Value: tc.pipeline},
						{Key: "cursor", Value: bson.D{}},
					}, "executionStats")
					if err != nil {
						return nil, err
					}
					return countingGroupWork(doc), nil
				},
			})
		})
	}
}

// countingGroupWork reduces the explain of a plan MongoDB's SBE engine runs to
// its explain version, the stages after the query, the query plan and the
// totals. SBE's executionStages are its own stages, so they are left out.
func countingGroupWork(doc bson.D) bson.D {
	out := bson.D{}
	if v, ok := lookup(doc, "explainVersion"); ok {
		out = append(out, bson.E{Key: "explainVersion", Value: v})
	}
	if stages, ok := lookup(doc, "stages"); ok {
		var names bson.A
		for _, s := range stages.(bson.A) {
			if d, ok := s.(bson.D); ok && len(d) > 0 {
				names = append(names, d[0].Key)
			}
		}
		out = append(out, bson.E{Key: "stages", Value: names})
	}
	if qp := findQueryPlanner(doc); qp != nil {
		if wp, ok := lookup(qp, "winningPlan"); ok {
			if plan, ok := lookup(wp.(bson.D), "queryPlan"); ok {
				out = append(out, bson.E{Key: "queryPlan", Value: explainExtractPlan(plan)})
			}
		}
	}
	if es := findExecutionStats(doc); es != nil {
		for _, k := range []string{"nReturned", "totalKeysExamined", "totalDocsExamined"} {
			if v, ok := lookup(es, k); ok {
				out = append(out, bson.E{Key: k, Value: v})
			}
		}
	}
	return out
}

// explainAgreement explains inner, then runs it with run under scannedBy, and
// reports whether explain's totals match the keys and documents the command
// read. Only the agreement is compared: mongod and DumboDB may choose
// different plans, but each must explain the one it runs.
func explainAgreement(inner bson.D, run func(context.Context, *mongo.Collection) error, repeatable bool) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		doc, err := explainRunExplain(ctx, col, inner, "executionStats")
		if err != nil {
			return nil, err
		}
		es := findExecutionStats(doc)
		explainKeys, _ := lookup(es, "totalKeysExamined")
		explainDocs, _ := lookup(es, "totalDocsExamined")
		scanned, err := scannedBy(ctx, col, func() error { return run(ctx, col) }, repeatable)
		if err != nil {
			return nil, err
		}
		return bson.D{
			{Key: "keysAgree", Value: toInt(explainKeys) == int(scanned[0].Value.(int64))},
			{Key: "docsAgree", Value: toInt(explainDocs) == int(scanned[1].Value.(int64))},
		}, nil
	}
}

// TestExplainPlan_AgreesWithExecution: explain's executionStats totals equal
// the serverStatus counters the same command moves when run
// (dolthub/dumbodb#145).
func TestExplainPlan_AgreesWithExecution(t *testing.T) {
	read := func(cmd func(*mongo.Collection) bson.D) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			cur, err := col.Database().RunCommandCursor(ctx, cmd(col))
			if err != nil {
				return err
			}
			var docs []bson.Raw
			return cur.All(ctx, &docs)
		}
	}
	command := func(cmd func(*mongo.Collection) bson.D) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			return col.Database().RunCommand(ctx, cmd(col)).Err()
		}
	}
	find := func(filter bson.D, extra ...bson.E) func(*mongo.Collection) bson.D {
		return func(col *mongo.Collection) bson.D {
			return append(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}}, extra...)
		}
	}
	rangeK := bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(5000)}, {Key: "$lt", Value: int32(5050)}}}}
	wideK := bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(1000)}}}}
	or := bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "k", Value: int32(5000)}}, bson.D{{Key: "grp", Value: int32(1)}}}}}
	matchGroup := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "grp", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$grp"}}}},
		}}, {Key: "cursor", Value: bson.D{}}}
	}
	distinct := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "grp"},
			{Key: "query", Value: bson.D{{Key: "grp", Value: bson.D{{Key: "$lt", Value: int32(100)}}}}}}
	}
	count := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: rangeK}}
	}
	update := func(col *mongo.Collection) bson.D {
		return updateGrpCmd(col, bson.D{{Key: "grp", Value: int32(3)}}, true)
	}
	cases := []struct {
		name       string
		support    harness.DumboDBSupport
		inner      func(*mongo.Collection) bson.D
		run        func(context.Context, *mongo.Collection) error
		repeatable bool
	}{
		{"find_range", harness.DumboDBFull, find(rangeK), read(find(rangeK)), true},
		{"find_or_union", harness.DumboDBFull, find(or), read(find(or)), true},
		{"find_covered", harness.DumboDBFull, find(rangeK, bson.E{Key: "projection", Value: bson.D{{Key: "k", Value: 1}, {Key: "_id", Value: 0}}}),
			read(find(rangeK, bson.E{Key: "projection", Value: bson.D{{Key: "k", Value: 1}, {Key: "_id", Value: 0}}})), true},
		{"find_id", harness.DumboDBFull, find(bson.D{{Key: "_id", Value: int32(7)}}), read(find(bson.D{{Key: "_id", Value: int32(7)}})), true},
		{"find_over_half", harness.DumboDBFull, find(wideK), read(find(wideK)), true},
		{"aggregate_match_group", harness.DumboDBFull, matchGroup, read(matchGroup), true},
		{"distinct_filtered", harness.DumboDBFull, distinct, command(distinct), true},
		{"count_range", harness.DumboDBFull, count, command(count), true},
		{"update_multi", harness.DumboDBFull, update, command(update), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_AgreesWithExecution_" + tc.name,
				Support: tc.support,
				Setup:   setupGrpK,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainAgreement(tc.inner(col), tc.run, tc.repeatable)(ctx, col)
				},
			})
		})
	}
}

// TestOr_EmptyIn: an $or branch with an empty $in on an indexed field matches
// nothing, alone or beside another indexed branch.
func TestOr_EmptyIn(t *testing.T) {
	emptyIn := bson.D{{Key: "grp", Value: bson.D{{Key: "$in", Value: bson.A{}}}}}
	cases := []struct {
		name   string
		filter bson.D
	}{
		{"alone", bson.D{{Key: "$or", Value: bson.A{emptyIn}}}},
		{"beside_id", bson.D{{Key: "$or", Value: bson.A{emptyIn, bson.D{{Key: "_id", Value: int32(7)}}}}}},
		{"beside_indexed", bson.D{{Key: "$or", Value: bson.A{emptyIn, bson.D{{Key: "grp", Value: int32(3)}, {Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(40)}}}}}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Or_EmptyIn_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setupGrp10,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Find(ctx, tc.filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					err = cur.All(ctx, &docs)
					return docs, err
				},
			})
		})
	}
}

// TestExplainPlan_SortMergeScanLimit: an $in on a compound index's leading
// field, sorted on its second, merges up to 200 scans; more sort in memory.
func TestExplainPlan_SortMergeScanLimit(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 2000)
		for i := 0; i < 2000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "grp", Value: int32(i % 1000)}, {Key: "k", Value: int32(i)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}})
	}
	for _, n := range []int{200, 201} {
		vals := bson.A{}
		for i := 0; i < n; i++ {
			vals = append(vals, int32(i))
		}
		find := bson.D{{Key: "filter", Value: bson.D{{Key: "grp", Value: bson.D{{Key: "$in", Value: vals}}}}},
			{Key: "sort", Value: bson.D{{Key: "k", Value: 1}}}, {Key: "limit", Value: int32(5)}}
		t.Run(fmt.Sprintf("in_%d", n), func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      fmt.Sprintf("ExplainPlan_SortMergeScanLimit_%d", n),
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					doc, err := explainRunExplain(ctx, col, append(bson.D{{Key: "find", Value: col.Name()}}, find...), "queryPlanner")
					if err != nil {
						return nil, err
					}
					cur, err := col.Find(ctx, find[0].Value, options.Find().SetSort(find[1].Value).SetLimit(5))
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					if err := cur.All(ctx, &docs); err != nil {
						return nil, err
					}
					return bson.D{{Key: "plan", Value: explainExtractCritical(doc)}, {Key: "docs", Value: docs}}, nil
				},
			})
		})
	}
}

// TestSortMerge_DuplicateInValues: $in values equal as keys, or a multikey
// document in two $in ranges, return each document once.
func TestSortMerge_DuplicateInValues(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}, {Key: "b", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 2}, {Key: "b", Value: 2}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: int64(2)}, {Key: "b", Value: 3}},
			bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: 3}, {Key: "b", Value: 0}},
			bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: bson.A{1, 3}}, {Key: "b", Value: 4}},
		}); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}})
	}
	in := func(vals ...interface{}) bson.D {
		return bson.D{{Key: "a", Value: bson.D{{Key: "$in", Value: bson.A(vals)}}}}
	}
	byB := bson.D{{Key: "b", Value: 1}}
	find := func(filter bson.D, opts *options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"repeated", find(in(2, 1, 2), options.Find().SetSort(byB))},
		{"equal_numbers", find(in(2, int64(2)), options.Find().SetSort(byB))},
		{"covered", find(in(2, 2), options.Find().SetSort(byB).SetProjection(bson.D{{Key: "b", Value: 1}, {Key: "_id", Value: 0}}))},
		{"skip_limit", find(in(2, 2), options.Find().SetSort(byB).SetSkip(1).SetLimit(1))},
		{"multikey", find(in(1, 3), options.Find().SetSort(byB))},
		{"aggregate", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Aggregate(ctx, mongo.Pipeline{{{Key: "$match", Value: in(2, 2)}}, {{Key: "$sort", Value: byB}}})
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "SortMerge_DuplicateInValues_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run:       tc.run,
			})
		})
	}
}

// TestExplainPlan_InSortPicksMerge: with single-field and compound indexes on
// the $in and sort fields, an $in sorted on the second compound key merges the
// compound index's ranges.
func TestExplainPlan_InSortPicksMerge(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 1000)
		for i := 0; i < 1000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "grp", Value: int32(i % 10)}, {Key: "k", Value: int32(i)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		for _, keys := range []bson.D{{{Key: "grp", Value: 1}}, {{Key: "k", Value: 1}}, {{Key: "grp", Value: 1}, {Key: "k", Value: 1}}} {
			if err := createIndex(ctx, col, keys); err != nil {
				return err
			}
		}
		return nil
	}
	harness.PairTest(t, harness.TestCase{
		Name:      "ExplainPlan_InSortPicksMerge",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup:     setup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			find := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "grp", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2)}}}}}},
				{Key: "sort", Value: bson.D{{Key: "k", Value: 1}}}, {Key: "limit", Value: int32(5)}}
			doc, err := explainRunExplain(ctx, col, find, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return explainExtractCritical(doc), nil
		},
	})
}

// TestTieOrder: documents tied on the sort key across $in branches, and
// documents equally far from a $near point, come back in MongoDB's order, so a
// limit keeps the same ones.
func TestTieOrder(t *testing.T) {
	inSetup := func(ctx context.Context, col *mongo.Collection) error {
		docs := []interface{}{}
		for i := 0; i < 12; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(100 - i)}, {Key: "grp", Value: int32(i % 3)}, {Key: "k", Value: int32(i / 3)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}})
	}
	geoSetup := func(ctx context.Context, col *mongo.Collection) error {
		docs := []interface{}{}
		for i, p := range [][2]float64{{1, 0}, {0, 1}, {-1, 0}, {0, -1}, {2, 0}, {0, 2}} {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(50 - i)}, {Key: "pos", Value: bson.A{p[0], p[1]}}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "pos", Value: "2d"}}})
		return err
	}
	find := func(filter, sort bson.D, limit int64) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			opts := options.Find().SetLimit(limit)
			if sort != nil {
				opts.SetSort(sort)
			}
			cur, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			err = cur.All(ctx, &docs)
			return docs, err
		}
	}
	in := bson.D{{Key: "grp", Value: bson.D{{Key: "$in", Value: bson.A{int32(2), int32(0), int32(1)}}}}}
	cases := []struct {
		name  string
		setup func(context.Context, *mongo.Collection) error
		run   func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"in_branches_ascending", inSetup, find(in, bson.D{{Key: "k", Value: 1}}, 4)},
		{"in_branches_descending", inSetup, find(in, bson.D{{Key: "k", Value: -1}}, 4)},
		{"near_equidistant", geoSetup, find(bson.D{{Key: "pos", Value: bson.D{{Key: "$near", Value: bson.A{0.0, 0.0}}}}}, nil, 3)},
	}
	for _, tc := range cases {
		support := harness.DumboDBFull
		if tc.name == "near_equidistant" {
			// mongod returns points at equal distance in the order its
			// distance heap yields them, which no stable order reproduces.
			support = harness.DumboDBXFail
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "TieOrder_" + tc.name,
				Support:   support,
				Clustered: true,
				Setup:     tc.setup,
				Run:       tc.run,
			})
		})
	}
}

// TestExplainPlan_SingleWriteOptions: explain of a single-document write
// honours its sort, hint and collation, as the write itself does.
func TestExplainPlan_SingleWriteOptions(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "u", Value: 1}, {Key: "a", Value: 3}, {Key: "b", Value: 1}, {Key: "n", Value: "abc"}},
			bson.D{{Key: "_id", Value: 2}, {Key: "u", Value: 2}, {Key: "a", Value: 3}, {Key: "b", Value: 2}, {Key: "n", Value: "ABC"}},
			bson.D{{Key: "_id", Value: 3}, {Key: "u", Value: 3}, {Key: "a", Value: 3}, {Key: "b", Value: 3}, {Key: "n", Value: "Abc"}},
			bson.D{{Key: "_id", Value: 4}, {Key: "u", Value: 4}, {Key: "a", Value: 4}, {Key: "b", Value: 4}, {Key: "n", Value: "x"}},
		}); err != nil {
			return err
		}
		for _, k := range []string{"a", "b", "n"} {
			if err := createIndex(ctx, col, bson.D{{Key: k, Value: 1}}); err != nil {
				return err
			}
		}
		return nil
	}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "t", Value: 1}}}}
	ci := bson.D{{Key: "locale", Value: "en"}, {Key: "strength", Value: 2}}
	cases := []struct {
		name  string
		inner func(*mongo.Collection) bson.D
	}{
		{"find_and_modify_sort_remove", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{{Key: "u", Value: bson.D{{Key: "$gt", Value: 0}}}}},
				{Key: "sort", Value: bson.D{{Key: "u", Value: -1}}}, {Key: "remove", Value: true}}
		}},
		{"update_one_sort", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{bson.D{
				{Key: "q", Value: bson.D{{Key: "u", Value: bson.D{{Key: "$gt", Value: 0}}}}}, {Key: "u", Value: set}, {Key: "sort", Value: bson.D{{Key: "u", Value: -1}}}}}}}
		}},
		{"find_and_modify_hint", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "findAndModify", Value: col.Name()}, {Key: "query", Value: bson.D{{Key: "a", Value: 3}}}, {Key: "update", Value: set}, {Key: "hint", Value: "b_1"}}
		}},
		{"delete_statement_collation", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{bson.D{
				{Key: "q", Value: bson.D{{Key: "n", Value: "abc"}}}, {Key: "limit", Value: 0}, {Key: "collation", Value: ci}}}}}
		}},
		{"delete_statement_hint", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "delete", Value: col.Name()}, {Key: "deletes", Value: bson.A{bson.D{
				{Key: "q", Value: bson.D{{Key: "a", Value: 3}}}, {Key: "limit", Value: 0}, {Key: "hint", Value: "b_1"}}}}}
		}},
		{"update_statement_collation", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "update", Value: col.Name()}, {Key: "updates", Value: bson.A{bson.D{
				{Key: "q", Value: bson.D{{Key: "n", Value: "abc"}}}, {Key: "u", Value: set}, {Key: "multi", Value: true}, {Key: "collation", Value: ci}}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_SingleWriteOptions_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.inner(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_IxscanReturns: an IXSCAN reports the entries it returned,
// before the FETCH above it applies the rest of the filter.
func TestExplainPlan_IxscanReturns(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 300)
		for i := 0; i < 300; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "b", Value: int32(i % 10)}, {Key: "u", Value: int32(i % 20)}, {Key: "a", Value: int32(i % 13)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		for _, k := range []string{"b", "a"} {
			if err := createIndex(ctx, col, bson.D{{Key: k, Value: 1}}); err != nil {
				return err
			}
		}
		return nil
	}
	cases := []struct {
		name string
		cmd  func(*mongo.Collection) bson.D
	}{
		{"residual_filter", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "b", Value: int32(3)}, {Key: "u", Value: bson.D{{Key: "$lt", Value: int32(5)}}}}}}
		}},
		{"hinted_residual", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "a", Value: int32(3)}, {Key: "u", Value: bson.D{{Key: "$gte", Value: int32(10)}}}}}, {Key: "hint", Value: "a_1"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_IxscanReturns_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.cmd(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_CountOptions: count explains its skip and limit, a multikey
// index and a hint as mongod does.
func TestExplainPlan_CountOptions(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 100)
		for i := 0; i < 100; i++ {
			d := bson.D{{Key: "_id", Value: int32(i)}, {Key: "b", Value: int32(i % 10)}, {Key: "a", Value: int32(i % 7)}, {Key: "m", Value: bson.A{int32(i), int32(i + 1)}}}
			docs = append(docs, d)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		for _, k := range []string{"b", "a", "m"} {
			if err := createIndex(ctx, col, bson.D{{Key: k, Value: 1}}); err != nil {
				return err
			}
		}
		return nil
	}
	count := func(extra ...bson.E) func(*mongo.Collection) bson.D {
		return func(col *mongo.Collection) bson.D { return append(bson.D{{Key: "count", Value: col.Name()}}, extra...) }
	}
	cases := []struct {
		name string
		cmd  func(*mongo.Collection) bson.D
	}{
		{"skip_limit", count(bson.E{Key: "query", Value: bson.D{{Key: "b", Value: int32(2)}}}, bson.E{Key: "skip", Value: int32(1)}, bson.E{Key: "limit", Value: int32(3)})},
		{"multikey", count(bson.E{Key: "query", Value: bson.D{{Key: "m", Value: bson.D{{Key: "$gt", Value: int32(50)}}}}})},
		{"hint_other_index", count(bson.E{Key: "query", Value: bson.D{{Key: "a", Value: int32(3)}}}, bson.E{Key: "hint", Value: "b_1"})},
		{"multikey_limit", count(bson.E{Key: "query", Value: bson.D{{Key: "m", Value: bson.D{{Key: "$gt", Value: int32(5)}}}}}, bson.E{Key: "limit", Value: int32(5)})},
		{"multikey_hinted", count(bson.E{Key: "query", Value: bson.D{{Key: "m", Value: bson.D{{Key: "$gt", Value: int32(50)}}}}}, bson.E{Key: "hint", Value: "m_1"})},
		{"multikey_in", count(bson.E{Key: "query", Value: bson.D{{Key: "m", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2), int32(8)}}}}}})},
		{"in", count(bson.E{Key: "query", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2), int32(8)}}}}}})},
		{"in_limit", count(bson.E{Key: "query", Value: bson.D{{Key: "b", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2), int32(8)}}}}}}, bson.E{Key: "limit", Value: int32(4)})},
		{"hint_other_index_skip_limit", count(bson.E{Key: "query", Value: bson.D{{Key: "a", Value: int32(3)}}}, bson.E{Key: "hint", Value: "b_1"},
			bson.E{Key: "skip", Value: int32(2)}, bson.E{Key: "limit", Value: int32(3)})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_CountOptions_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.cmd(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_ViewFilter: explain of a find or count on a view applies
// both the view's $match and the command's filter.
func TestExplainPlan_ViewFilter(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 2}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 3}},
		}); err != nil {
			return err
		}
		db := col.Database()
		_ = db.Collection(col.Name() + "_v").Drop(ctx)
		return db.CreateView(ctx, col.Name()+"_v", col.Name(), mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 2}}}}}}})
	}
	cases := []struct {
		name string
		cmd  func(view string) bson.D
	}{
		{"find", func(view string) bson.D {
			return bson.D{{Key: "find", Value: view}, {Key: "filter", Value: bson.D{{Key: "a", Value: 3}}}}
		}},
		{"count", func(view string) bson.D {
			return bson.D{{Key: "count", Value: view}, {Key: "query", Value: bson.D{{Key: "a", Value: 3}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_ViewFilter_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var doc bson.D
					err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: tc.cmd(col.Name() + "_v")}, {Key: "verbosity", Value: "executionStats"}}).Decode(&doc)
					if err != nil {
						return nil, err
					}
					es := findExecutionStats(doc)
					var n interface{}
					if es != nil {
						n, _ = lookup(es, "nReturned")
					}
					ver, _ := lookup(doc, "explainVersion")
					return bson.D{{Key: "nReturned", Value: n}, {Key: "explainVersion", Value: ver}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_LookupStrategy: whether an equality $lookup joins in the
// query plan (EQ_LOOKUP) or as a classic $lookup stage, its strategy and the
// foreign index it uses, across foreign index shapes, a missing foreign
// collection, an absorbed $unwind, a collation and a foreign view.
func TestExplainPlan_LookupStrategy(t *testing.T) {
	setup := func(index bson.D, view bool) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			local := make([]interface{}, 0, 50)
			for i := 0; i < 50; i++ {
				local = append(local, bson.D{{Key: "_id", Value: int32(i)}, {Key: "fk", Value: int32(i % 10)}})
			}
			if _, err := col.InsertMany(ctx, local); err != nil {
				return err
			}
			if index == nil {
				return nil
			}
			foreign := col.Database().Collection(col.Name() + "_f")
			docs := make([]interface{}, 0, 100)
			for i := 0; i < 100; i++ {
				docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "x", Value: int32(i % 10)}, {Key: "y", Value: int32(i)}})
			}
			if _, err := foreign.InsertMany(ctx, docs); err != nil {
				return err
			}
			if err := createIndex(ctx, foreign, index); err != nil {
				return err
			}
			if view {
				_ = col.Database().Collection(col.Name() + "_fv").Drop(ctx)
				return col.Database().CreateView(ctx, col.Name()+"_fv", col.Name()+"_f", mongo.Pipeline{})
			}
			return nil
		}
	}
	join := func(from string) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: from}, {Key: "localField", Value: "fk"}, {Key: "foreignField", Value: "x"}, {Key: "as", Value: "j"}}}}
	}
	cases := []struct {
		name      string
		setup     func(context.Context, *mongo.Collection) error
		from      string
		pipeline  func(from string) bson.A
		collation *options.Collation
	}{
		{"compound_leading_index", setup(bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 1}}, false), "_f", func(f string) bson.A { return bson.A{join(f)} }, nil},
		{"missing_foreign", setup(nil, false), "_missing", func(f string) bson.A { return bson.A{join(f)} }, nil},
		{"unwind_absorbed", setup(bson.D{{Key: "x", Value: 1}}, false), "_f", func(f string) bson.A {
			return bson.A{join(f), bson.D{{Key: "$unwind", Value: "$j"}}}
		}, nil},
		{"collation", setup(bson.D{{Key: "x", Value: 1}}, false), "_f", func(f string) bson.A { return bson.A{join(f)} }, &options.Collation{Locale: "en", Strength: 2}},
		{"foreign_view", setup(bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 1}}, true), "_fv", func(f string) bson.A { return bson.A{join(f)} }, nil},
		{"foreign_view_stats", setup(bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 1}}, true), "_fv", func(f string) bson.A { return bson.A{join(f)} }, nil},
		{"match_between_unwind", setup(bson.D{{Key: "x", Value: 1}}, false), "_f", func(f string) bson.A {
			return bson.A{join(f), bson.D{{Key: "$match", Value: bson.D{{Key: "fk", Value: 1}}}}, bson.D{{Key: "$unwind", Value: "$j"}}}
		}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_LookupStrategy_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   tc.setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: tc.pipeline(col.Name() + tc.from)}, {Key: "cursor", Value: bson.D{}}}
					if tc.collation != nil {
						cmd = append(cmd, bson.E{Key: "collation", Value: bson.D{{Key: "locale", Value: tc.collation.Locale}, {Key: "strength", Value: tc.collation.Strength}}})
					}
					verbosity := "queryPlanner"
					if strings.HasSuffix(tc.name, "_stats") {
						verbosity = "executionStats"
					}
					doc, err := explainRunExplain(ctx, col, cmd, verbosity)
					if err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					classic := false
					var names []string
					unwinding := false
					if stages, ok := lookup(doc, "stages"); ok {
						if arr, ok := stages.(bson.A); ok {
							for _, s := range arr {
								d, ok := s.(bson.D)
								if !ok || len(d) == 0 {
									continue
								}
								names = append(names, d[0].Key)
								if d[0].Key == "$lookup" {
									classic = true
									if spec, ok := d[0].Value.(bson.D); ok {
										_, unwinding = lookup(spec, "unwinding")
									}
								}
							}
						}
					}
					return bson.D{{Key: "explainVersion", Value: ver}, {Key: "classic", Value: classic}, {Key: "stages", Value: names},
						{Key: "unwinding", Value: unwinding}, {Key: "joins", Value: lookupJoins(doc)}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_SkipAndOrWalk: a find reads skip plus limit index keys, a
// blocking sort keeps skip plus limit documents, and an $or answered by
// walking a sort index is planned under SUBPLAN.
func TestExplainPlan_SkipAndOrWalk(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 50)
		for i := 0; i < 50; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "u", Value: int32(i)}, {Key: "a", Value: int32(i % 7)}, {Key: "b", Value: int32(i % 5)}, {Key: "grp", Value: int32(i % 3)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		for _, k := range []string{"u", "a", "b", "grp"} {
			if err := createIndex(ctx, col, bson.D{{Key: k, Value: 1}}); err != nil {
				return err
			}
		}
		return nil
	}
	cases := []struct {
		name string
		cmd  func(*mongo.Collection) bson.D
	}{
		{"skip_limit", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "u", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}},
				{Key: "skip", Value: int32(2)}, {Key: "limit", Value: int32(1)}}
		}},
		{"sort_skip_limit", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}},
				{Key: "sort", Value: bson.D{{Key: "b", Value: -1}, {Key: "u", Value: 1}}}, {Key: "skip", Value: int32(1)}, {Key: "limit", Value: int32(1)}}
		}},
		{"or_sort_walk", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(3)}}}}, bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}}}},
				{Key: "sort", Value: bson.D{{Key: "grp", Value: 1}}}, {Key: "limit", Value: int32(2)}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_SkipAndOrWalk_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.cmd(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_SortWalkCounts: a walk of a multikey sort index reports each
// document once in its IXSCAN, and a lone $or answered by a walk stays under
// SUBPLAN once its plan is cached.
func TestExplainPlan_SortWalkCounts(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 50)
		for i := 0; i < 50; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "u", Value: int32(i % 20)},
				{Key: "arr", Value: bson.A{int32(i), int32(i + 1), int32(i + 2)}},
				{Key: "a", Value: int32(i % 7)}, {Key: "b", Value: int32(i % 5)}, {Key: "grp", Value: int32(i % 3)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		for _, k := range []string{"arr", "a", "b", "grp"} {
			if err := createIndex(ctx, col, bson.D{{Key: k, Value: 1}}); err != nil {
				return err
			}
		}
		return nil
	}
	orFind := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(3)}}}}, bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}}}},
			{Key: "sort", Value: bson.D{{Key: "grp", Value: 1}}}, {Key: "limit", Value: int32(2)}}
	}
	cases := []struct {
		name   string
		cmd    func(*mongo.Collection) bson.D
		warmup int
	}{
		{"multikey", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "u", Value: int32(15)}}},
				{Key: "sort", Value: bson.D{{Key: "arr", Value: 1}}}, {Key: "limit", Value: int32(1)}}
		}, 0},
		{"or_walk", orFind, 0},
		{"or_walk_cached", orFind, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_SortWalkCounts_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					for i := 0; i < tc.warmup; i++ {
						if err := col.Database().RunCommand(ctx, tc.cmd(col)).Err(); err != nil {
							return nil, err
						}
					}
					return explainWorkOf(tc.cmd(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_ReshapingView: explain of a find, count, distinct or
// aggregate on a view whose pipeline reshapes documents runs the view's stages
// before the command's, so the command's filter sees the reshaped documents,
// and reports each stage the query does not absorb.
func TestExplainPlan_ReshapingView(t *testing.T) {
	views := []struct {
		name     string
		pipeline mongo.Pipeline
	}{
		{"project", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "z", Value: "$a"}}}}}},
		{"match_set", mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 2}}}}}}, {{Key: "$set", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", 10}}}}}}}}},
		{"sort_limit", mongo.Pipeline{{{Key: "$sort", Value: bson.D{{Key: "a", Value: -1}}}}, {{Key: "$limit", Value: 2}}}},
		{"match", mongo.Pipeline{{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 2}}}}}}}},
	}
	cmds := []struct {
		name string
		cmd  func(view string, filter bson.D) bson.D
	}{
		{"find", func(view string, filter bson.D) bson.D {
			return bson.D{{Key: "find", Value: view}, {Key: "filter", Value: filter}}
		}},
		{"count", func(view string, filter bson.D) bson.D {
			return bson.D{{Key: "count", Value: view}, {Key: "query", Value: filter}}
		}},
		{"aggregate", func(view string, filter bson.D) bson.D {
			return bson.D{{Key: "aggregate", Value: view}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: filter}}}}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"distinct", func(view string, filter bson.D) bson.D {
			return bson.D{{Key: "distinct", Value: view}, {Key: "key", Value: "_id"}, {Key: "query", Value: filter}}
		}},
	}
	filters := map[string]bson.D{
		"project":    {{Key: "z", Value: 3}},
		"match_set":  {{Key: "a", Value: 30}},
		"sort_limit": {{Key: "a", Value: bson.D{{Key: "$lt", Value: 3}}}},
		"match":      {{Key: "a", Value: 3}},
	}
	for _, v := range views {
		for _, c := range cmds {
			t.Run(v.name+"/"+c.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "ExplainPlan_ReshapingView_" + v.name + "_" + c.name,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						docs := []interface{}{}
						for i := 1; i <= 5; i++ {
							docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "a", Value: (i % 3) + 1}})
						}
						if _, err := col.InsertMany(ctx, docs); err != nil {
							return err
						}
						return col.Database().CreateView(ctx, col.Name()+"_v", col.Name(), v.pipeline)
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						var doc bson.D
						err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: c.cmd(col.Name()+"_v", filters[v.name])}, {Key: "verbosity", Value: "executionStats"}}).Decode(&doc)
						if err != nil {
							return nil, err
						}
						es := findExecutionStats(doc)
						var n interface{}
						if es != nil {
							n, _ = lookup(es, "nReturned")
						}
						ver, _ := lookup(doc, "explainVersion")
						return bson.D{{Key: "nReturned", Value: n}, {Key: "explainVersion", Value: ver}, {Key: "stages", Value: topLevelStages(doc)}}, nil
					},
				})
			})
		}
	}
}

// TestAggregate_MatchAfterShaping: a $match after a $project or $set returns
// what it would without being moved ahead, whatever the optimizer does.
func TestAggregate_MatchAfterShaping(t *testing.T) {
	pipelines := []struct {
		name     string
		pipeline mongo.Pipeline
	}{
		{"rename", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "z", Value: "$a"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: 3}}}}}},
		{"rename_missing", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "z", Value: "$q"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: nil}}}}}},
		{"included", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}, {{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 2}}}}}}}},
		{"dropped", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}, {{Key: "$match", Value: bson.D{{Key: "b", Value: 1}}}}}},
		{"set_overwrite", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", 10}}}}}}}, {{Key: "$match", Value: bson.D{{Key: "a", Value: 30}}}}}},
		{"set_rename_or", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$a"}}}}, {{Key: "$match", Value: bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "z", Value: 1}}, bson.D{{Key: "b", Value: 2}}}}}}}}},
		{"array_rename", mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "z", Value: "$arr.k"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: 2}}}}}},
		{"unset", mongo.Pipeline{{{Key: "$unset", Value: "b"}}, {{Key: "$match", Value: bson.D{{Key: "a", Value: 2}}}}}},
		{"get_field_after_set", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: "$a"}}}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: "y"}}, 2}}}}}}}}},
		{"get_field_after_unset", mongo.Pipeline{{{Key: "$unset", Value: "a"}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: "a"}}, 2}}}}}}}}},
		{"current_rename", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$$CURRENT.a"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: 2}}}}}},
		{"get_field_literal_after_set", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: "$a"}}}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: bson.D{{Key: "$literal", Value: "y"}}}}, 2}}}}}}}}},
		{"get_field_literal_after_unset", mongo.Pipeline{{{Key: "$unset", Value: "a"}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: bson.D{{Key: "$literal", Value: "a"}}}}, 2}}}}}}}}},
		{"get_field_plus_field_after_set", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: "$a"}}}}, {{Key: "$match", Value: append(bson.D{{Key: "b", Value: 1}}, bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: "y"}}, 2}}}}}...)}}}},
		{"set_type_error_then_get_field", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: bson.D{{Key: "$divide", Value: bson.A{1, 0}}}}}}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: "q"}}, 5}}}}}}}}},
		{"expr_renamed", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$a"}}}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$z", 2}}}}}}}}},
		{"expr_overwritten", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", 10}}}}}}}, {{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$a", 20}}}}}}}}},
		{"comment", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "q", Value: 1}}}}, {{Key: "$match", Value: bson.D{{Key: "a", Value: 2}, {Key: "$comment", Value: "x"}}}}}},
		{"json_schema", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "q", Value: 1}}}}, {{Key: "$match", Value: bson.D{{Key: "$jsonSchema", Value: bson.D{{Key: "required", Value: bson.A{"b"}}}}}}}}},
		{"array_index_source", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$n.0"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: 2}}}}}},
		{"dotted_source_array_equality", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$arr.k"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: bson.A{1, 2}}}}}}},
		{"dotted_source_size", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$arr.k"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: bson.D{{Key: "$size", Value: 2}}}}}}}},
		{"dotted_source_type", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$arr.k"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: bson.D{{Key: "$type", Value: "array"}}}}}}}},
		{"dotted_source_all", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$arr.k"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: bson.D{{Key: "$all", Value: bson.A{1, 2}}}}}}}}},
		{"dotted_source_elem_match", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$arr.k"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$gte", Value: 6}}}}}}}}}},
		{"dotted_source_exists", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "z", Value: "$arr.q"}}}}, {{Key: "$match", Value: bson.D{{Key: "z", Value: bson.D{{Key: "$exists", Value: true}}}}}}}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Aggregate_MatchAfterShaping_" + p.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					docs := []interface{}{}
					for i := 1; i <= 6; i++ {
						docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "a", Value: (i % 3) + 1}, {Key: "b", Value: i % 2},
							{Key: "arr", Value: bson.A{bson.D{{Key: "k", Value: i}}, bson.D{{Key: "k", Value: i + 1}}}},
							{Key: "n", Value: bson.A{i, i + 1}}})
					}
					if _, err := col.InsertMany(ctx, docs); err != nil {
						return err
					}
					return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}})
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Aggregate(ctx, append(p.pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}))
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					err = cur.All(ctx, &docs)
					return docs, err
				},
			})
		})
	}
}

// topLevelStages returns each top-level stage of an aggregate explain as its
// name and nReturned, or nil for an explain without stages.
func topLevelStages(doc bson.D) bson.A {
	v, ok := lookupBSONField(doc, "stages")
	if !ok {
		return nil
	}
	arr, _ := v.(bson.A)
	out := bson.A{}
	for _, s := range arr {
		stage, _ := s.(bson.D)
		entry := bson.D{}
		for _, e := range stage {
			if strings.HasPrefix(e.Key, "$") {
				entry = append(entry, bson.E{Key: "stage", Value: e.Key})
			}
		}
		if n, ok := lookupBSONField(stage, "nReturned"); ok {
			entry = append(entry, bson.E{Key: "nReturned", Value: n})
		}
		out = append(out, entry)
	}
	return out
}

// TestExplainPlan_PipelineStages: an aggregate explain reports, after the
// query under $cursor, each stage the query does not absorb, with the number
// of documents it returned. A $count shows as its $group and $project after
// a $set, $addFields or $unset, but not after a $project.
func TestExplainPlan_PipelineStages(t *testing.T) {
	match := bson.D{{Key: "$match", Value: bson.D{{Key: "g", Value: 1}}}}
	set := bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: 1}}}}
	count := bson.D{{Key: "$count", Value: "n"}}
	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"set", bson.A{match, set}},
		{"project", bson.A{match, bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}}},
		{"unset", bson.A{bson.D{{Key: "$unset", Value: "a"}}}},
		{"unwind", bson.A{match, bson.D{{Key: "$unwind", Value: "$arr"}}}},
		{"sort_limit_set", bson.A{match, bson.D{{Key: "$sort", Value: bson.D{{Key: "a", Value: 1}}}}, bson.D{{Key: "$limit", Value: 3}}, set}},
		{"set_only", bson.A{set}},
		{"skip", bson.A{match, bson.D{{Key: "$skip", Value: 2}}}},
		{"limit_then_match", bson.A{bson.D{{Key: "$limit", Value: 6}}, match}},
		{"add_fields_sort", bson.A{match, bson.D{{Key: "$addFields", Value: bson.D{{Key: "b", Value: 1}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "b", Value: 1}}}}}},
		{"replace_root", bson.A{bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: bson.D{{Key: "x", Value: "$a"}}}}}}}},
		{"set_then_match", bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$multiply", Value: bson.A{"$a", 10}}}}}}}, bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: 10}}}}}},
		{"unset_then_match", bson.A{bson.D{{Key: "$unset", Value: "arr"}}, match}},
		{"set_then_expr", bson.A{set, bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$g", 1}}}}}}}}},
		{"set_then_comment", bson.A{set, bson.D{{Key: "$match", Value: bson.D{{Key: "g", Value: 1}, {Key: "$comment", Value: "x"}}}}}},
		{"unwind_then_limit", bson.A{match, bson.D{{Key: "$unwind", Value: "$arr"}}, bson.D{{Key: "$limit", Value: 2}}}},
		{"set_then_count", bson.A{set, count}},
		{"add_fields_then_count", bson.A{bson.D{{Key: "$addFields", Value: bson.D{{Key: "b", Value: 1}}}}, count}},
		{"unset_then_count", bson.A{bson.D{{Key: "$unset", Value: "a"}}, count}},
		{"project_then_count", bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}, count}},
		{"match_set_count", bson.A{match, set, count}},
		{"project_set_count", bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}}}}, set, count}},
		{"current_rename", bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: "$$CURRENT.g"}}}}, bson.D{{Key: "$match", Value: bson.D{{Key: "z", Value: 1}}}}}},
		{"root_rename", bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: "$$ROOT.g"}}}}, bson.D{{Key: "$match", Value: bson.D{{Key: "z", Value: 1}}}}}},
		{"json_schema", bson.A{set, bson.D{{Key: "$match", Value: bson.D{{Key: "$jsonSchema", Value: bson.D{{Key: "required", Value: bson.A{"g"}}}}}}}}},
		{"renamed_plus_other_field", bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: "$g"}}}}, bson.D{{Key: "$match", Value: bson.D{{Key: "z", Value: 1}, {Key: "a", Value: 2}}}}}},
		{"lookup_set_match", bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "explain_plan_foreign"}, {Key: "localField", Value: "a"},
			{Key: "foreignField", Value: "a"}, {Key: "as", Value: "j"}}}}, set, match}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_PipelineStages_" + p.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					docs := make([]interface{}, 20)
					for i := range docs {
						docs[i] = bson.D{{Key: "_id", Value: i}, {Key: "g", Value: i % 4}, {Key: "a", Value: i % 3}, {Key: "arr", Value: bson.A{i, i + 1}}}
					}
					if _, err := col.InsertMany(ctx, docs); err != nil {
						return err
					}
					return createIndex(ctx, col, bson.D{{Key: "g", Value: 1}})
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var doc bson.D
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: p.pipeline}, {Key: "cursor", Value: bson.D{}}}
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Decode(&doc); err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					return bson.D{{Key: "explainVersion", Value: ver}, {Key: "stages", Value: topLevelStages(doc)}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_CountHintedFetch: a count's explain reads what mongod's
// does: a hinted FETCH stops at the limit, its IXSCAN returning every key it
// reads; an unfiltered hinted count reads only keys; and an $elemMatch, $size
// or empty-array equality fetches each document its bounds hold.
func TestExplainPlan_CountHintedFetch(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 60)
		for i := 0; i < 60; i++ {
			arr := bson.A{int32(i % 4), int32(i % 3)}
			if i%5 == 0 {
				arr = bson.A{}
			}
			d := bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 10)}, {Key: "b", Value: int32(i % 7)}, {Key: "arr", Value: arr}}
			if i%5 != 0 {
				d = append(d, bson.E{Key: "s", Value: int32(i)})
			}
			docs = append(docs, d)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "s", Value: 1}}, Options: options.Index().SetName("s_sp").SetSparse(true)},
			{Keys: bson.D{{Key: "b", Value: 1}}},
			{Keys: bson.D{{Key: "arr", Value: 1}}},
			{Keys: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}}},
		})
		return err
	}
	count := func(query bson.D, hint string, limit int32) func(col *mongo.Collection) bson.D {
		return func(col *mongo.Collection) bson.D {
			cmd := bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: query}}
			if hint != "" {
				cmd = append(cmd, bson.E{Key: "hint", Value: hint})
			}
			if limit > 0 {
				cmd = append(cmd, bson.E{Key: "limit", Value: limit})
			}
			return cmd
		}
	}
	cases := []struct {
		name string
		cmd  func(col *mongo.Collection) bson.D
	}{
		{"sparse_off_filter_limit", count(bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(5)}}}}, "s_sp", 5)},
		{"unfiltered_limit", count(bson.D{}, "b_1", 5)},
		{"elem_match", count(bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}}, "arr_1", 0)},
		{"size_zero", count(bson.D{{Key: "arr", Value: bson.D{{Key: "$size", Value: int32(0)}}}}, "arr_1", 0)},
		{"empty_array", count(bson.D{{Key: "arr", Value: bson.A{}}}, "arr_1", 0)},
		{"unhinted_elem_match", count(bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$gt", Value: int32(1)}}}}}}, "", 0)},
		{"unhinted_empty_array", count(bson.D{{Key: "arr", Value: bson.A{}}}, "", 0)},
		{"compound_leading_equality_limit", count(bson.D{{Key: "b", Value: int32(3)}}, "b_1_a_1", 2)},
		{"compound_leading_range_limit", count(bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: int32(3)}}}}, "b_1_a_1", 4)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_CountHintedFetch_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.cmd(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_SourceStages: explain at executionStats verbosity answers a
// pipeline led by a stage that makes its own documents, on a collection of
// several documents.
func TestExplainPlan_SourceStages(t *testing.T) {
	addFields := bson.D{{Key: "$addFields", Value: bson.D{{Key: "x", Value: 1}}}}
	pipelines := []struct {
		name  string
		first bson.D
	}{
		{"coll_stats_count", bson.D{{Key: "$collStats", Value: bson.D{{Key: "count", Value: bson.D{}}}}}},
		{"coll_stats_storage", bson.D{{Key: "$collStats", Value: bson.D{{Key: "storageStats", Value: bson.D{}}}}}},
		{"coll_stats_latency", bson.D{{Key: "$collStats", Value: bson.D{{Key: "latencyStats", Value: bson.D{}}}}}},
		{"index_stats", bson.D{{Key: "$indexStats", Value: bson.D{}}}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_SourceStages_" + p.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 1}}, bson.D{{Key: "_id", Value: 2}}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: bson.A{p.first, addFields}}, {Key: "cursor", Value: bson.D{}}}
					err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Err()
					return bson.D{{Key: "ok", Value: err == nil}}, nil
				},
			})
		})
	}
}

// TestAggregate_ViewMatchPushdown: an aggregate on a view filters the view's
// output, so a $match the user leads with, or one moved ahead of a rename,
// sees the documents after the view's own stages.
func TestAggregate_ViewMatchPushdown(t *testing.T) {
	views := map[string]mongo.Pipeline{
		"rename": {{{Key: "$set", Value: bson.D{{Key: "a", Value: "$b"}}}}},
		"match":  {{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 4}}}}}}},
		"limit":  {{{Key: "$sort", Value: bson.D{{Key: "_id", Value: -1}}}}, {{Key: "$limit", Value: 2}}},
	}
	match := func(f bson.D) bson.D { return bson.D{{Key: "$match", Value: f}} }
	cases := []struct {
		name, view string
		pipeline   mongo.Pipeline
	}{
		{"moved_past_rename", "rename", mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "y", Value: "$a"}}}}, match(bson.D{{Key: "y", Value: 9}})}},
		{"leading_on_renamed", "rename", mongo.Pipeline{match(bson.D{{Key: "a", Value: 9}})}},
		{"on_match_view", "match", mongo.Pipeline{match(bson.D{{Key: "b", Value: bson.D{{Key: "$gte", Value: 4}}}})}},
		{"limit_after_view_limit", "limit", mongo.Pipeline{match(bson.D{{Key: "a", Value: bson.D{{Key: "$gte", Value: 1}}}}), {{Key: "$limit", Value: 1}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Aggregate_ViewMatchPushdown_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: 4}, {Key: "b", Value: 9}},
						bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: 9}, {Key: "b", Value: 4}},
						bson.D{{Key: "_id", Value: 6}, {Key: "a", Value: 1}, {Key: "b", Value: 1}},
					}); err != nil {
						return err
					}
					return col.Database().CreateView(ctx, col.Name()+"_v", col.Name(), views[tc.view])
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Database().Collection(col.Name()+"_v").Aggregate(ctx, tc.pipeline)
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					err = cur.All(ctx, &docs)
					return docs, err
				},
			})
		})
	}
}

// TestExplainPlan_CompoundSortWithoutSortIndex: a sort on several fields that
// no index yields is sorted in memory over the filter's index.
func TestExplainPlan_CompoundSortWithoutSortIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:      "ExplainPlan_CompoundSortWithoutSortIndex",
		Support:   harness.DumboDBFull,
		Clustered: true,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 240)
			for i := range docs {
				docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 7)}, {Key: "b", Value: int32(i * 13 % 11)}, {Key: "m", Value: int32(i % 5)}}
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}, {Key: "m", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return explainWorkOf(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}},
				{Key: "sort", Value: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}}, {Key: "limit", Value: 3}})(ctx, col)
		},
	})
}

// TestCount_MultikeyTwoSidedRange: a count of a two-sided range on a multikey
// field counts documents whose elements meet each side separately.
func TestCount_MultikeyTwoSidedRange(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{0, 6}}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 3}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: bson.A{2}}},
		}); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}})
	}
	query := bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$lt", Value: 5}}}}
	for _, hinted := range []bool{false, true} {
		name := "unhinted"
		if hinted {
			name = "hinted"
		}
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "Count_MultikeyTwoSidedRange_" + name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cmd := bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: query}}
					if hinted {
						cmd = append(cmd, bson.E{Key: "hint", Value: "a_1"})
					}
					var res bson.M
					if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
						return nil, err
					}
					work, err := explainWorkOf(cmd)(ctx, col)
					return bson.D{{Key: "n", Value: res["n"]}, {Key: "explain", Value: work}}, err
				},
			})
		})
	}
}

// TestExplainPlan_StagesAfterEqLookup: after a $lookup the query plan
// absorbs, a $sort, $limit or $skip that keeps its order moves into the plan
// and any other stage is reported on its own, with its count.
func TestExplainPlan_StagesAfterEqLookup(t *testing.T) {
	lookupStage := func(foreign string) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: foreign}, {Key: "localField", Value: "a"}, {Key: "foreignField", Value: "b"}, {Key: "as", Value: "j"}}}}
	}
	pipelines := []struct {
		name  string
		after []bson.D
	}{
		{"match_on_join", []bson.D{{{Key: "$match", Value: bson.D{{Key: "j.v", Value: "x"}}}}}},
		{"sort_local", []bson.D{{{Key: "$sort", Value: bson.D{{Key: "a", Value: -1}}}}}},
		{"sort_joined", []bson.D{{{Key: "$sort", Value: bson.D{{Key: "j.v", Value: 1}}}}}},
		{"limit", []bson.D{{{Key: "$limit", Value: 1}}}},
		{"sort_limit", []bson.D{{{Key: "$sort", Value: bson.D{{Key: "a", Value: -1}}}}, {{Key: "$limit", Value: 2}}}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_StagesAfterEqLookup_" + p.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}}, bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: 2}}, bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 3}}}); err != nil {
						return err
					}
					_, err := col.Database().Collection(col.Name()+"_j").InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: 1}, {Key: "b", Value: 1}, {Key: "v", Value: "x"}},
						bson.D{{Key: "_id", Value: 2}, {Key: "b", Value: 2}, {Key: "v", Value: "y"}},
						bson.D{{Key: "_id", Value: 3}, {Key: "b", Value: 3}, {Key: "v", Value: "y"}},
					})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					pipeline := bson.A{lookupStage(col.Name() + "_j")}
					for _, s := range p.after {
						pipeline = append(pipeline, s)
					}
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}
					var doc bson.D
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Decode(&doc); err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					var plan []string
					if qp := findQueryPlanner(doc); qp != nil {
						if wp, ok := lookupBSONField(qp, "winningPlan"); ok {
							if w, ok := wp.(bson.D); ok {
								if inner, ok := lookupBSONField(w, "queryPlan"); ok {
									w, _ = inner.(bson.D)
								}
								plan = planStages(w)
							}
						}
					}
					return bson.D{{Key: "explainVersion", Value: ver}, {Key: "stages", Value: topLevelStages(doc)}, {Key: "plan", Value: plan}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_CountScanEndKey: a COUNT_SCAN that reaches the end of its
// range examines one key more than it counts, the key past the end; one its
// limit stops examines only what it counted.
func TestExplainPlan_CountScanEndKey(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 40)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 10)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}})
	}
	count := func(query bson.D, extra ...bson.E) bson.D {
		return append(bson.D{{Key: "count", Value: "_"}, {Key: "query", Value: query}}, extra...)
	}
	cases := []struct {
		name string
		cmd  bson.D
	}{
		{"equality", count(bson.D{{Key: "a", Value: int32(3)}})},
		{"miss", count(bson.D{{Key: "a", Value: int32(100)}})},
		{"to_index_end", count(bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(7)}}}})},
		{"limit_reached", count(bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}, bson.E{Key: "limit", Value: int32(5)})},
		{"limit_not_reached", count(bson.D{{Key: "a", Value: int32(3)}}, bson.E{Key: "limit", Value: int32(10)})},
		{"skip", count(bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}, bson.E{Key: "skip", Value: int32(3)})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_CountScanEndKey_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cmd := append(bson.D{{Key: "count", Value: col.Name()}}, tc.cmd[1:]...)
					return explainWorkOf(cmd)(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_CountTrailingMultikey: on a compound index whose trailing
// field holds arrays, a count bounded on the leading field reads only keys,
// counting each document once.
func TestExplainPlan_CountTrailingMultikey(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 82)
		for i := 0; i < 80; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "b", Value: int32(i % 7)}, {Key: "m", Value: bson.A{int32(i % 4), int32(i % 3), int32(i % 5)}}})
		}
		docs = append(docs, bson.D{{Key: "_id", Value: int32(100)}, {Key: "m", Value: bson.A{1, 2}}}, bson.D{{Key: "_id", Value: int32(101)}, {Key: "b", Value: nil}})
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "b", Value: 1}, {Key: "m", Value: 1}})
	}
	cases := []struct {
		name  string
		query bson.D
	}{
		{"equality", bson.D{{Key: "b", Value: int32(3)}}},
		{"range", bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: int32(3)}}}}},
		{"two_sided", bson.D{{Key: "b", Value: bson.D{{Key: "$gt", Value: int32(1)}, {Key: "$lt", Value: int32(5)}}}}},
		{"null", bson.D{{Key: "b", Value: nil}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_CountTrailingMultikey_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup:     setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cmd := bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: tc.query}}
					var res bson.M
					if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
						return nil, err
					}
					work, err := explainWorkOf(cmd)(ctx, col)
					return bson.D{{Key: "n", Value: res["n"]}, {Key: "explain", Value: work}}, err
				},
			})
		})
	}
}

// TestExplainPlan_ViewHiddenFieldGetField: explain on a view that hides a
// field reports the same counts whatever value a $getField probe of that field
// guesses, because the probe runs after the view hides it.
func TestExplainPlan_ViewHiddenFieldGetField(t *testing.T) {
	for _, guess := range []string{"s1", "zz"} {
		for _, shape := range []string{"field", "doc"} {
			t.Run(guess+"/"+shape, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:      "ExplainPlan_ViewHiddenFieldGetField_" + guess + "_" + shape,
					Support:   harness.DumboDBFull,
					Clustered: true,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if _, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 1}, {Key: "secret", Value: "s1"}}, bson.D{{Key: "_id", Value: 2}, {Key: "secret", Value: "s2"}}}); err != nil {
							return err
						}
						return col.Database().CreateView(ctx, col.Name()+"_v", col.Name(), mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "secret", Value: 0}}}}})
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						var probe interface{} = "secret"
						if shape == "doc" {
							probe = bson.D{{Key: "field", Value: "secret"}, {Key: "input", Value: "$$CURRENT"}}
						}
						match := bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$getField", Value: probe}}, guess}}}}}}}
						cmd := bson.D{{Key: "aggregate", Value: col.Name() + "_v"}, {Key: "pipeline", Value: bson.A{match}}, {Key: "cursor", Value: bson.D{}}}
						var doc bson.D
						if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Decode(&doc); err != nil {
							return nil, err
						}
						return bson.D{{Key: "stages", Value: topLevelStages(doc)}}, nil
					},
				})
			})
		}
	}
}

// TestExplainPlan_SkipLimitWindow: a leading $skip and $limit, or ones after
// an equality $lookup, run in the query plan as SKIP and LIMIT with their
// amounts, under any EQ_LOOKUP, and nReturned counts what they pass.
func TestExplainPlan_SkipLimitWindow(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 5)
		foreign := make([]interface{}, 5)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i + 1)}, {Key: "a", Value: int32(i + 1)}}
			foreign[i] = bson.D{{Key: "_id", Value: int32(i + 1)}, {Key: "a", Value: int32(i + 1)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, foreign)
		return err
	}
	skip := bson.D{{Key: "$skip", Value: 1}}
	limit := bson.D{{Key: "$limit", Value: 2}}
	match := bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 1}}}}}}
	lookup := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name() + "_f"}, {Key: "localField", Value: "a"},
			{Key: "foreignField", Value: "a"}, {Key: "as", Value: "j"}}}}
	}
	cases := []struct {
		name     string
		pipeline func(col *mongo.Collection) bson.A
	}{
		{"skip", func(*mongo.Collection) bson.A { return bson.A{skip} }},
		{"skip_limit", func(*mongo.Collection) bson.A { return bson.A{skip, limit} }},
		{"match_skip", func(*mongo.Collection) bson.A { return bson.A{match, skip} }},
		{"lookup_skip", func(c *mongo.Collection) bson.A { return bson.A{lookup(c), skip} }},
		{"lookup_skip_limit", func(c *mongo.Collection) bson.A { return bson.A{lookup(c), skip, limit} }},
		{"skip_limit_lookup", func(c *mongo.Collection) bson.A { return bson.A{skip, limit, lookup(c)} }},
		{"match_limit_skip", func(*mongo.Collection) bson.A { return bson.A{match, bson.D{{Key: "$limit", Value: 3}}, skip} }},
		{"skip_skip", func(*mongo.Collection) bson.A { return bson.A{skip, skip} }},
		{"limit_limit", func(*mongo.Collection) bson.A { return bson.A{bson.D{{Key: "$limit", Value: 4}}, limit} }},
		{"skip_limit_skip", func(*mongo.Collection) bson.A { return bson.A{skip, bson.D{{Key: "$limit", Value: 3}}, skip} }},
		{"limit_skip_limit", func(*mongo.Collection) bson.A { return bson.A{bson.D{{Key: "$limit", Value: 4}}, skip, limit} }},
		{"lookup_limit_skip", func(c *mongo.Collection) bson.A { return bson.A{lookup(c), limit, skip} }},
		{"lookup_skip_skip", func(c *mongo.Collection) bson.A { return bson.A{lookup(c), skip, skip} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_SkipLimitWindow_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.D
					cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: tc.pipeline(col)},
						{Key: "cursor", Value: bson.D{}}}}, {Key: "verbosity", Value: "executionStats"}}
					if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
						return nil, err
					}
					qp, _ := lookupBSONField(res, "queryPlanner")
					wp, _ := lookupBSONField(qp.(bson.D), "winningPlan")
					plan := wp.(bson.D)
					if inner, ok := lookupBSONField(plan, "queryPlan"); ok {
						plan = inner.(bson.D)
					}
					var window bson.A
					for plan != nil {
						name, _ := lookupBSONField(plan, "stage")
						entry := bson.D{{Key: "stage", Value: name}}
						for _, k := range []string{"skipAmount", "limitAmount"} {
							if v, ok := lookupBSONField(plan, k); ok {
								entry = append(entry, bson.E{Key: k, Value: v})
							}
						}
						window = append(window, entry)
						next, _ := lookupBSONField(plan, "inputStage")
						plan, _ = next.(bson.D)
					}
					stats, _ := lookupBSONField(res, "executionStats")
					n, _ := lookupBSONField(stats.(bson.D), "nReturned")
					return bson.D{{Key: "plan", Value: window}, {Key: "nReturned", Value: n}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_CountFromCompoundAndSparse: a count binding a compound
// index's leading fields by equality and its next by a range, or an
// $exists: true on a sparse index, counts from the index without fetching.
func TestExplainPlan_CountFromCompoundAndSparse(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 20)
		for i := range docs {
			d := bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i % 7)}, {Key: "b", Value: int32(i % 3)}, {Key: "c", Value: int32(i % 2)}}
			if i%4 != 0 {
				d = append(d, bson.E{Key: "s", Value: int32(i)})
			}
			if i == 19 {
				d = append(d, bson.E{Key: "m", Value: bson.A{1, 2}})
			}
			docs[i] = d
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "b", Value: 1}, {Key: "a", Value: 1}}, Options: options.Index().SetName("b_a")},
			{Keys: bson.D{{Key: "s", Value: 1}}, Options: options.Index().SetName("s_sp").SetSparse(true)},
			{Keys: bson.D{{Key: "c", Value: 1}, {Key: "m", Value: 1}}, Options: options.Index().SetName("c_m")},
		})
		return err
	}
	cases := []struct {
		name   string
		filter bson.D
		hint   interface{}
	}{
		{"eq_range_hinted", bson.D{{Key: "b", Value: 2}, {Key: "a", Value: bson.D{{Key: "$gt", Value: 3}}}}, "b_a"},
		{"eq_range", bson.D{{Key: "b", Value: 2}, {Key: "a", Value: bson.D{{Key: "$gt", Value: 3}}}}, nil},
		{"eq_eq", bson.D{{Key: "b", Value: 2}, {Key: "a", Value: 5}}, nil},
		{"extra_field", bson.D{{Key: "b", Value: 2}, {Key: "a", Value: 5}, {Key: "c", Value: 1}}, "b_a"},
		{"multikey_trailing", bson.D{{Key: "c", Value: 1}, {Key: "m", Value: 2}}, "c_m"},
		{"multikey_trailing_range", bson.D{{Key: "c", Value: 1}, {Key: "m", Value: bson.D{{Key: "$gte", Value: 2}}}}, "c_m"},
		{"multikey_trailing_two_sided", bson.D{{Key: "c", Value: 1}, {Key: "m", Value: bson.D{{Key: "$gt", Value: 1}, {Key: "$lt", Value: 2}}}}, "c_m"},
		{"sparse_exists_hinted", bson.D{{Key: "s", Value: bson.D{{Key: "$exists", Value: true}}}}, "s_sp"},
		{"sparse_exists", bson.D{{Key: "s", Value: bson.D{{Key: "$exists", Value: true}}}}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_CountFromCompoundAndSparse_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					count := bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: tc.filter}}
					if tc.hint != nil {
						count = append(count, bson.E{Key: "hint", Value: tc.hint})
					}
					var counted bson.D
					if err := col.Database().RunCommand(ctx, count).Decode(&counted); err != nil {
						return nil, err
					}
					n, _ := lookupBSONField(counted, "n")
					var res bson.D
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: count}, {Key: "verbosity", Value: "executionStats"}}).Decode(&res); err != nil {
						return nil, err
					}
					qp, _ := lookupBSONField(res, "queryPlanner")
					wp, _ := lookupBSONField(qp.(bson.D), "winningPlan")
					stats, _ := lookupBSONField(res, "executionStats")
					keys, _ := lookupBSONField(stats.(bson.D), "totalKeysExamined")
					docs, _ := lookupBSONField(stats.(bson.D), "totalDocsExamined")
					return bson.D{{Key: "n", Value: n}, {Key: "plan", Value: planStages(wp)}, {Key: "keys", Value: keys}, {Key: "docs", Value: docs}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_StageEntryShapes: a $sort and the $limit after it explain
// as one $sort of sortKey and limit with its sort counters, and a pipeline
// led by $collStats or $indexStats explains only its stages.
func TestExplainPlan_StageEntryShapes(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 5)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i + 1)}, {Key: "a", Value: int32(i + 1)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, docs[:3])
		return err
	}
	lookup := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name() + "_f"}, {Key: "localField", Value: "a"},
			{Key: "foreignField", Value: "a"}, {Key: "as", Value: "j"}}}}
	}
	sort := bson.D{{Key: "$sort", Value: bson.D{{Key: "j.a", Value: 1}}}}
	limit := bson.D{{Key: "$limit", Value: 2}}
	cases := []struct {
		name     string
		pipeline func(col *mongo.Collection) bson.A
	}{
		{"lookup_sort_limit", func(c *mongo.Collection) bson.A { return bson.A{lookup(c), sort, limit} }},
		{"set_sort_limit", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "q", Value: "$a"}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "q", Value: -1}}}}, limit}
		}},
		{"set_sort", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "q", Value: "$a"}}}}, bson.D{{Key: "$sort", Value: bson.D{{Key: "q", Value: -1}}}}}
		}},
		{"coll_stats", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$collStats", Value: bson.D{{Key: "count", Value: bson.D{}}}}}}
		}},
		{"index_stats_match", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$indexStats", Value: bson.D{}}}, bson.D{{Key: "$match", Value: bson.D{{Key: "name", Value: "_id_"}}}}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_StageEntryShapes_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.D
					cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: tc.pipeline(col)},
						{Key: "cursor", Value: bson.D{}}}}, {Key: "verbosity", Value: "executionStats"}}
					if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
						return nil, err
					}
					_, hasPlanner := lookupBSONField(res, "queryPlanner")
					stages, _ := lookupBSONField(res, "stages")
					var shapes bson.A
					arr, _ := stages.(bson.A)
					for _, s := range arr {
						entry := s.(bson.D)
						shape := bson.D{}
						for _, e := range entry {
							switch e.Key {
							case "executionTimeMillisEstimate", "totalDataSizeSortedBytesEstimate":
								shape = append(shape, bson.E{Key: e.Key, Value: "present"})
							case "$sort", "nReturned", "usedDisk", "spills", "spilledDataStorageSize":
								shape = append(shape, e)
							default:
								shape = append(shape, bson.E{Key: e.Key, Value: "present"})
							}
						}
						shapes = append(shapes, shape)
					}
					return bson.D{{Key: "queryPlanner", Value: hasPlanner}, {Key: "stages", Value: shapes}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_SkipUnderFetch: a skip over an index scan that answers the
// whole filter drops keys before their documents are fetched; one with a
// filter the index cannot decide skips fetched documents.
func TestExplainPlan_SkipUnderFetch(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 20)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "g", Value: int32(i % 4)}, {Key: "h", Value: int32(i)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "g", Value: 1}})
	}
	g := bson.D{{Key: "g", Value: 1}}
	gh := bson.D{{Key: "g", Value: 1}, {Key: "h", Value: 5}}
	cases := []struct {
		name string
		cmd  func(coll string) bson.D
	}{
		{"aggregate_skip_limit", func(c string) bson.D {
			return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: g}}, bson.D{{Key: "$skip", Value: 1}}, bson.D{{Key: "$limit", Value: 1}}}}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"aggregate_skip", func(c string) bson.D {
			return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: g}}, bson.D{{Key: "$skip", Value: 1}}}}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"aggregate_skip_residual", func(c string) bson.D {
			return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: gh}}, bson.D{{Key: "$skip", Value: 1}}}}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"find_skip_limit", func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "filter", Value: g}, {Key: "skip", Value: 1}, {Key: "limit", Value: 1}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_SkipUnderFetch_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.D
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: tc.cmd(col.Name())}, {Key: "verbosity", Value: "executionStats"}}).Decode(&res); err != nil {
						return nil, err
					}
					qp, _ := lookupBSONField(res, "queryPlanner")
					wp, _ := lookupBSONField(qp.(bson.D), "winningPlan")
					stats, _ := lookupBSONField(res, "executionStats")
					n, _ := lookupBSONField(stats.(bson.D), "nReturned")
					var exec bson.A
					es, _ := lookupBSONField(stats.(bson.D), "executionStages")
					for node, _ := es.(bson.D); node != nil; {
						entry := bson.D{}
						for _, k := range []string{"stage", "nReturned", "keysExamined", "docsExamined"} {
							if v, ok := lookupBSONField(node, k); ok {
								entry = append(entry, bson.E{Key: k, Value: v})
							}
						}
						exec = append(exec, entry)
						next, _ := lookupBSONField(node, "inputStage")
						node, _ = next.(bson.D)
					}
					return bson.D{{Key: "plan", Value: planStages(wp)}, {Key: "nReturned", Value: n}, {Key: "executionStages", Value: exec}}, nil
				},
			})
		})
	}
}

// TestCount_EqualityWithContradictingRange: a count of an equality beside a
// range or $in on an indexed field counts what find returns, on a single and
// a compound index.
func TestCount_EqualityWithContradictingRange(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 20)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(1)}, {Key: "b", Value: int32(i)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		if err := createIndex(ctx, col, bson.D{{Key: "b", Value: 1}}); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}})
	}
	eq := func(more ...bson.E) bson.D { return append(bson.D{{Key: "$eq", Value: 5}}, more...) }
	cases := []struct {
		name   string
		filter bson.D
		hint   string
	}{
		{"single_eq_gt", bson.D{{Key: "b", Value: eq(bson.E{Key: "$gt", Value: 10})}}, "b_1"},
		{"single_eq_lt", bson.D{{Key: "b", Value: eq(bson.E{Key: "$lt", Value: 3})}}, "b_1"},
		{"single_eq_in", bson.D{{Key: "b", Value: eq(bson.E{Key: "$in", Value: bson.A{5, 6}})}}, "b_1"},
		{"single_eq_ne", bson.D{{Key: "b", Value: eq(bson.E{Key: "$ne", Value: 5})}}, "b_1"},
		{"compound_eq_gt", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: eq(bson.E{Key: "$gt", Value: 10})}}, "a_1_b_1"},
		{"compound_eq_lt", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: eq(bson.E{Key: "$lt", Value: 3})}}, "a_1_b_1"},
		{"compound_eq_in", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: eq(bson.E{Key: "$in", Value: bson.A{5, 6}})}}, "a_1_b_1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Count_EqualityWithContradictingRange_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					docs, err := col.CountDocuments(ctx, tc.filter, options.Count().SetHint(tc.hint))
					if err != nil {
						return nil, err
					}
					var res bson.D
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: tc.filter}, {Key: "hint", Value: tc.hint}}).Decode(&res); err != nil {
						return nil, err
					}
					n, _ := lookupBSONField(res, "n")
					return bson.D{{Key: "countDocuments", Value: docs}, {Key: "count", Value: n}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_HintWithCollation: under a non-simple collation an index
// of the simple collation serves bounds that hold no string, hinted or not,
// and a hinted one is scanned whole for string bounds.
func TestExplainPlan_HintWithCollation(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 10)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i)}, {Key: "s", Value: fmt.Sprintf("x%d", i)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		if err := createIndex(ctx, col, bson.D{{Key: "a", Value: 1}}); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "s", Value: 1}})
	}
	coll := bson.D{{Key: "locale", Value: "en"}, {Key: "strength", Value: 2}}
	gt := func(field string, v interface{}) bson.D {
		return bson.D{{Key: field, Value: bson.D{{Key: "$gt", Value: v}}}}
	}
	cases := []struct {
		name string
		cmd  func(c string) bson.D
	}{
		{"find_hinted", func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "filter", Value: gt("a", 0)}, {Key: "hint", Value: "a_1"}, {Key: "collation", Value: coll}}
		}},
		{"find_unhinted", func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "filter", Value: gt("a", 0)}, {Key: "collation", Value: coll}}
		}},
		{"find_hinted_string", func(c string) bson.D {
			return bson.D{{Key: "find", Value: c}, {Key: "filter", Value: gt("s", "x")}, {Key: "hint", Value: "s_1"}, {Key: "collation", Value: coll}}
		}},
		{"aggregate_hinted", func(c string) bson.D {
			return bson.D{{Key: "aggregate", Value: c}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: gt("a", 0)}}}},
				{Key: "hint", Value: "a_1"}, {Key: "collation", Value: coll}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"count_hinted", func(c string) bson.D {
			return bson.D{{Key: "count", Value: c}, {Key: "query", Value: gt("a", 0)}, {Key: "hint", Value: "a_1"}, {Key: "collation", Value: coll}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_HintWithCollation_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.D
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: tc.cmd(col.Name())}, {Key: "verbosity", Value: "executionStats"}}).Decode(&res); err != nil {
						return nil, err
					}
					if stages, ok := lookupBSONField(res, "stages"); ok {
						cursor, _ := lookupBSONField(stages.(bson.A)[0].(bson.D), "$cursor")
						res = cursor.(bson.D)
					}
					qp, _ := lookupBSONField(res, "queryPlanner")
					wp, _ := lookupBSONField(qp.(bson.D), "winningPlan")
					stats, _ := lookupBSONField(res, "executionStats")
					keys, _ := lookupBSONField(stats.(bson.D), "totalKeysExamined")
					docs, _ := lookupBSONField(stats.(bson.D), "totalDocsExamined")
					return bson.D{{Key: "plan", Value: planStages(wp)}, {Key: "keys", Value: keys}, {Key: "docs", Value: docs}}, nil
				},
			})
		})
	}
}

// TestExplainPlan_CursorPulledBeforeLimit: a $limit after a $lookup and the
// $unwind of its join pulls from the query only until it has its documents,
// so $cursor reports how many it pulled, more when matches come late.
func TestExplainPlan_CursorPulledBeforeLimit(t *testing.T) {
	for _, late := range []bool{false, true} {
		name := "early_matches"
		if late {
			name = "late_matches"
		}
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_CursorPulledBeforeLimit_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					docs := make([]interface{}, 5)
					for i := range docs {
						docs[i] = bson.D{{Key: "_id", Value: int32(i + 1)}, {Key: "a", Value: int32(i + 1)}}
					}
					if _, err := col.InsertMany(ctx, docs); err != nil {
						return err
					}
					foreign := docs
					if late {
						foreign = docs[3:]
					}
					_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, foreign)
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					pipeline := bson.A{
						bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name() + "_f"}, {Key: "localField", Value: "a"}, {Key: "foreignField", Value: "a"}, {Key: "as", Value: "j"}}}},
						bson.D{{Key: "$unwind", Value: "$j"}},
						bson.D{{Key: "$limit", Value: 2}},
					}
					var doc bson.D
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Decode(&doc); err != nil {
						return nil, err
					}
					return topLevelStages(doc), nil
				},
			})
		})
	}
}

// TestExplainPlan_AggregateProjection: a pipeline's trailing $project or
// $unset that only includes or excludes fields runs in the query plan, under
// its skip and limit, under a sort with no limit, and over one with a limit,
// covered when the index holds every field it returns.
func TestExplainPlan_AggregateProjection(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 10)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: int32(i)}, {Key: "b", Value: int32(i % 3)}}
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "a", Value: 1}})
	}
	match := bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: 2}}}}}}
	sortB := bson.D{{Key: "$sort", Value: bson.D{{Key: "b", Value: 1}}}}
	limit := func(n int) bson.D { return bson.D{{Key: "$limit", Value: n}} }
	project := func(spec bson.D) bson.D { return bson.D{{Key: "$project", Value: spec}} }
	cases := []struct {
		name     string
		pipeline bson.A
	}{
		{"sort_limit_project", bson.A{sortB, limit(3), project(bson.D{{Key: "b", Value: 1}})}},
		{"sort_project", bson.A{sortB, project(bson.D{{Key: "b", Value: 1}})}},
		{"match_project", bson.A{match, project(bson.D{{Key: "a", Value: 1}})}},
		{"project_only", bson.A{project(bson.D{{Key: "b", Value: 1}, {Key: "_id", Value: 0}})}},
		{"sort_limit_exclusion", bson.A{sortB, limit(3), project(bson.D{{Key: "b", Value: 0}})}},
		{"match_skip_project", bson.A{match, bson.D{{Key: "$skip", Value: 2}}, project(bson.D{{Key: "a", Value: 1}})}},
		{"match_limit_covered", bson.A{match, limit(2), project(bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 0}})}},
		{"unset_only", bson.A{bson.D{{Key: "$unset", Value: "b"}}}},
		{"match_sort_limit_covered", bson.A{match, bson.D{{Key: "$sort", Value: bson.D{{Key: "a", Value: 1}}}}, limit(3), project(bson.D{{Key: "a", Value: 1}, {Key: "_id", Value: 0}})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_AggregateProjection_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var res bson.D
					cmd := bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: tc.pipeline}, {Key: "cursor", Value: bson.D{}}}
					if err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: cmd}, {Key: "verbosity", Value: "executionStats"}}).Decode(&res); err != nil {
						return nil, err
					}
					if stages, ok := lookupBSONField(res, "stages"); ok {
						return bson.D{{Key: "stages", Value: topLevelStages(res)}, {Key: "count", Value: len(stages.(bson.A))}}, nil
					}
					qp, _ := lookupBSONField(res, "queryPlanner")
					wp, _ := lookupBSONField(qp.(bson.D), "winningPlan")
					stats, _ := lookupBSONField(res, "executionStats")
					n, _ := lookupBSONField(stats.(bson.D), "nReturned")
					return bson.D{{Key: "plan", Value: planStages(wp)}, {Key: "nReturned", Value: n}}, nil
				},
			})
		})
	}
}
