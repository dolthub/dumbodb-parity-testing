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

// TestExplainPlan_IdEquality: mongod serves an _id equality with an EXPRESS
// stage on _id_. DumboDB does a primary-key point read but explains a
// COLLSCAN (dolthub/dumbodb#143), with zero counts for writes (#145).
func TestExplainPlan_IdEquality(t *testing.T) {
	id := bson.D{{Key: "_id", Value: int32(7)}}
	cases := []struct {
		name  string
		inner func(*mongo.Collection) bson.D
	}{
		{"find", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: id}}
		}},
		{"update", func(col *mongo.Collection) bson.D { return updateGrpCmd(col, id, false) }},
		{"delete", func(col *mongo.Collection) bson.D { return deleteGrpCmd(col, id, 1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "ExplainPlan_IdEquality_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   setupGrp10,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.inner(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_WriteExecutionStats: a multi update or delete through an
// index reports the keys and documents its scan examined. DumboDB reports
// zeros (dolthub/dumbodb#145), and names a batched delete DELETE.
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
				Support: harness.DumboDBXFail,
				Setup:   setupGrp10,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return explainWorkOf(tc.inner(col))(ctx, col)
				},
			})
		})
	}
}

// TestExplainPlan_OrUnion: an $or with an indexed field in every branch is a
// union of index scans. DumboDB runs the union and reports its totals, but
// every stage below the root reports zero (dolthub/dumbodb#132).
func TestExplainPlan_OrUnion(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ExplainPlan_OrUnion",
		Support: harness.DumboDBXFail,
		Setup:   setupGrpK,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			or := bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "k", Value: int32(5000)}}, bson.D{{Key: "grp", Value: int32(1)}}}}}
			return explainWorkOf(bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: or}})(ctx, col)
		},
	})
}

// TestExplainPlan_InSortLimit: $in on the leading field of a compound index,
// sorted on the next field with a limit. mongod merges one sorted scan per
// $in value (SORT_MERGE) and stops at the limit; DumboDB reads both ranges
// whole and sorts them (dolthub/dumbodb#146).
func TestExplainPlan_InSortLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ExplainPlan_InSortLimit",
		Support: harness.DumboDBXFail,
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

// lookupForeignIndex returns the index explain reports for a $lookup's
// foreign side: an EQ_LOOKUP stage's indexName when the pipeline runs in SBE,
// or the $lookup stage's indexesUsed in the classic engine.
func lookupForeignIndex(doc bson.D) []interface{} {
	var out []interface{}
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch t := v.(type) {
		case bson.D:
			if st, _ := lookup(t, "stage"); st == "EQ_LOOKUP" {
				if ix, ok := lookup(t, "indexName"); ok {
					out = append(out, ix)
				}
			}
			if used, ok := lookup(t, "indexesUsed"); ok {
				if arr, ok := used.(bson.A); ok {
					out = append(out, arr...)
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
	return out
}

// TestExplainPlan_LookupForeignIndex: a $lookup on an indexed foreignField
// reads the foreign side through its index, and explain says so. DumboDB
// uses the index but explains only the local collection scan
// (dolthub/dumbodb#144).
func TestExplainPlan_LookupForeignIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ExplainPlan_LookupForeignIndex",
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
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
				docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "ref", Value: int32(i % 100)}})
			}
			if _, err := foreign.InsertMany(ctx, docs); err != nil {
				return err
			}
			return createIndex(ctx, foreign, bson.D{{Key: "ref", Value: 1}})
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			doc, err := explainRunExplain(ctx, col, bson.D{
				{Key: "aggregate", Value: col.Name()},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: col.Name() + "_f"},
					{Key: "localField", Value: "fk"},
					{Key: "foreignField", Value: "ref"},
					{Key: "as", Value: "j"},
				}}}}},
				{Key: "cursor", Value: bson.D{}},
			}, "queryPlanner")
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "foreignIndex", Value: lookupForeignIndex(doc)}}, nil
		},
	})
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
// the serverStatus counters the same command moves when run. DumboDB's explain
// plans separately from execution, so the two disagree whenever the plans
// differ (dolthub/dumbodb#139, #141, #143, #145).
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
	update := func(col *mongo.Collection) bson.D { return updateGrpCmd(col, bson.D{{Key: "grp", Value: int32(3)}}, true) }
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
		{"find_id", harness.DumboDBXFail, find(bson.D{{Key: "_id", Value: int32(7)}}), read(find(bson.D{{Key: "_id", Value: int32(7)}})), true},
		{"find_over_half", harness.DumboDBXFail, find(wideK), read(find(wideK)), true},
		{"aggregate_match_group", harness.DumboDBXFail, matchGroup, read(matchGroup), true},
		{"distinct_filtered", harness.DumboDBXFail, distinct, command(distinct), true},
		{"update_multi", harness.DumboDBXFail, update, command(update), false},
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
