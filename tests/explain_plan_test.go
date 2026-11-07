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
	}
	for _, tc := range cases {
		support := harness.DumboDBFull
		if tc.name == "multikey" {
			// mongod's multikey COUNT_SCAN examines one key more than the
			// range holds; DumboDB counts the range's entries.
			support = harness.DumboDBXFail
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "ExplainPlan_CountOptions_" + tc.name,
				Support:   support,
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
					doc, err := explainRunExplain(ctx, col, cmd, "queryPlanner")
					if err != nil {
						return nil, err
					}
					ver, _ := lookup(doc, "explainVersion")
					classic := false
					if stages, ok := lookup(doc, "stages"); ok {
						if arr, ok := stages.(bson.A); ok {
							for _, s := range arr {
								if d, ok := s.(bson.D); ok && len(d) > 0 && d[0].Key == "$lookup" {
									classic = true
								}
							}
						}
					}
					return bson.D{{Key: "explainVersion", Value: ver}, {Key: "classic", Value: classic}, {Key: "joins", Value: lookupJoins(doc)}}, nil
				},
			})
		})
	}
}
