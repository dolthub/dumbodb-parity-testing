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

package benchmarks

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Index-use benchmarks. A query that silently falls back to a collection scan
// returns correct results, so only latency would show it. Before timing, each
// benchmark asserts through explain (executionStats) that the winning plan uses
// the expected index and, for find, that the documents examined stay a small
// fraction of the collection. The assertion runs against MongoDB too, which
// validates it.

const indexUseDocs = 10000

// indexUseDoc is the fixture: flat, compound, dotted, deep dotted, and
// through-array keys, each selective.
func indexUseDoc(i int) bson.D {
	return bson.D{
		{Key: "_id", Value: int32(i)},
		{Key: "k", Value: int32(i)},
		{Key: "grp", Value: int32(i % 1000)},
		{Key: "a", Value: bson.D{
			{Key: "b", Value: int32(i)},
			{Key: "c", Value: bson.D{{Key: "d", Value: int32(i % 1000)}}},
		}},
		{Key: "items", Value: bson.A{
			bson.D{{Key: "sku", Value: fmt.Sprintf("sku-%d-0", i)}},
			bson.D{{Key: "sku", Value: fmt.Sprintf("sku-%d-1", i)}},
		}},
		{Key: "pad", Value: fmt.Sprintf("%0200d", i)},
	}
}

func withIndexUseCollection(b *testing.B, label string, keys bson.D) (*mongo.Collection, context.Context, bool) {
	b.Helper()
	ctx := context.Background()
	client, err := connect(ctx)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	col, cleanup := freshCollection(ctx, client, label)
	b.Cleanup(func() {
		cleanup()
		_ = client.Disconnect(context.Background())
	})
	const batch = 1000
	buf := make([]interface{}, 0, batch)
	for i := 0; i < indexUseDocs; i++ {
		buf = append(buf, indexUseDoc(i))
		if len(buf) == batch || i == indexUseDocs-1 {
			if _, err := col.InsertMany(ctx, buf, options.InsertMany().SetOrdered(false)); err != nil {
				b.Fatalf("seed: %v", err)
			}
			buf = buf[:0]
		}
	}
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: options.Index().SetName("idx")}); err != nil {
		b.Fatalf("create index %v: %v", keys, err)
	}
	return col, ctx, isDumboDB(ctx, client)
}

// isDumboDB reports whether the target is DumboDB, which names its storage
// engine "dolt" in buildInfo.
func isDumboDB(ctx context.Context, client *mongo.Client) bool {
	var info struct {
		StorageEngines []string `bson:"storageEngines"`
	}
	if err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&info); err != nil {
		return false
	}
	return slices.Contains(info.StorageEngines, "dolt")
}

// indexUseExpectation is either required, or a known DumboDB gap named by its
// tracking issue: the gap is logged while it persists and fails the benchmark
// once DumboDB starts using the index, so the expectation gets promoted.
type indexUseExpectation struct {
	knownDumboGap string
}

var mustUseIndex = indexUseExpectation{}

type explainSummary struct {
	stages       []string
	indexNames   []string
	nReturned    int64
	docsExamined int64
	hasExecStats bool
}

func collectPlan(v interface{}, out *explainSummary) {
	m, ok := v.(bson.M)
	if !ok {
		return
	}
	if s, ok := m["stage"].(string); ok {
		out.stages = append(out.stages, s)
	}
	if n, ok := m["indexName"].(string); ok {
		out.indexNames = append(out.indexNames, n)
	}
	collectPlan(m["queryPlan"], out)
	collectPlan(m["inputStage"], out)
	if arr, ok := m["inputStages"].(bson.A); ok {
		for _, c := range arr {
			collectPlan(c, out)
		}
	}
}

func asInt64(v interface{}) (int64, bool) {
	switch n := v.(type) {
	case int32:
		return int64(n), true
	case int64:
		return n, true
	case float64:
		return int64(n), true
	}
	return 0, false
}

func explainExecution(ctx context.Context, col *mongo.Collection, inner bson.D) (explainSummary, error) {
	var res bson.M
	err := col.Database().RunCommand(ctx, bson.D{
		{Key: "explain", Value: inner},
		{Key: "verbosity", Value: "executionStats"},
	}).Decode(&res)
	if err != nil {
		return explainSummary{}, err
	}
	var out explainSummary
	if qp, ok := res["queryPlanner"].(bson.M); ok {
		collectPlan(qp["winningPlan"], &out)
	}
	if es, ok := res["executionStats"].(bson.M); ok {
		n, nOK := asInt64(es["nReturned"])
		d, dOK := asInt64(es["totalDocsExamined"])
		out.nReturned, out.docsExamined, out.hasExecStats = n, d, nOK && dOK
	}
	return out, nil
}

// usesIndex reports whether s shows an index scan on idx and, when the command
// reports document counts for a find, examined at most a tenth of the
// collection.
func (s explainSummary) usesIndex(idx string, checkExamined bool) (bool, string) {
	scan := false
	for _, st := range s.stages {
		if st == "IXSCAN" || st == "COUNT_SCAN" || st == "DISTINCT_SCAN" {
			scan = true
		}
	}
	if !scan || !slices.Contains(s.indexNames, idx) {
		return false, fmt.Sprintf("plan %v on %v, want an index scan on %q", s.stages, s.indexNames, idx)
	}
	if checkExamined && s.hasExecStats && s.docsExamined*10 > indexUseDocs {
		return false, fmt.Sprintf("examined %d of %d docs for %d results", s.docsExamined, indexUseDocs, s.nReturned)
	}
	return true, ""
}

// requireIndexUse fails b unless inner's plan uses idx, honoring exp on DumboDB.
func requireIndexUse(b *testing.B, ctx context.Context, col *mongo.Collection, dumbo bool, inner bson.D, exp indexUseExpectation) {
	b.Helper()
	s, err := explainExecution(ctx, col, inner)
	if err != nil {
		b.Fatalf("explain %v: %v", inner, err)
	}
	ok, why := s.usesIndex("idx", inner[0].Key == "find")
	b.ReportMetric(float64(s.docsExamined), "docs-examined")
	switch {
	case ok && dumbo && exp.knownDumboGap != "":
		b.Fatalf("DumboDB now uses the index (%s); promote this benchmark to mustUseIndex", exp.knownDumboGap)
	case !ok && dumbo && exp.knownDumboGap != "":
		b.Logf("known DumboDB gap (%s): %s", exp.knownDumboGap, why)
	case !ok:
		b.Fatalf("index not used for %v: %s", inner, why)
	}
}

func findCmd(col *mongo.Collection, filter, sort bson.D, limit int64) bson.D {
	cmd := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}}
	if sort != nil {
		cmd = append(cmd, bson.E{Key: "sort", Value: sort})
	}
	if limit > 0 {
		cmd = append(cmd, bson.E{Key: "limit", Value: limit})
	}
	return cmd
}

func runFind(b *testing.B, ctx context.Context, col *mongo.Collection, filter, sort bson.D, limit int64) {
	opts := options.Find()
	if sort != nil {
		opts.SetSort(sort)
	}
	if limit > 0 {
		opts.SetLimit(limit)
	}
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		b.Fatalf("Find: %v", err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		b.Fatalf("cursor.All: %v", err)
	}
}

// benchmarkIndexedFind times filter(i) after asserting filter(probe) uses the index.
func benchmarkIndexedFind(b *testing.B, label string, keys bson.D, filter func(i int) bson.D, sort bson.D, limit int64, exp indexUseExpectation) {
	col, ctx, dumbo := withIndexUseCollection(b, label, keys)
	requireIndexUse(b, ctx, col, dumbo, findCmd(col, filter(indexUseDocs/2), sort, limit), exp)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runFind(b, ctx, col, filter(i%indexUseDocs), sort, limit)
	}
}

func eqOn(field string) func(int) bson.D {
	return func(i int) bson.D { return bson.D{{Key: field, Value: int32(i)}} }
}

func rangeOn(field string, width int) func(int) bson.D {
	return func(i int) bson.D {
		return bson.D{{Key: field, Value: bson.D{{Key: "$gte", Value: int32(i)}, {Key: "$lt", Value: int32(i + width)}}}}
	}
}

func inOn(field string) func(int) bson.D {
	return func(i int) bson.D {
		return bson.D{{Key: field, Value: bson.D{{Key: "$in", Value: bson.A{int32(i), int32(i + 7), int32(i + 13)}}}}}
	}
}

var (
	flatKey     = bson.D{{Key: "k", Value: 1}}
	compoundKey = bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}}
	dottedKey   = bson.D{{Key: "a.b", Value: 1}}
	deepKey     = bson.D{{Key: "a.c.d", Value: 1}}
	skuKey      = bson.D{{Key: "items.sku", Value: 1}}
)

func BenchmarkIndexUse_FlatEq_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_flat_eq", flatKey, eqOn("k"), nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_FlatRange_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_flat_range", flatKey, rangeOn("k", 50), nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_FlatIn_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_flat_in", flatKey, inOn("k"), nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_CompoundPrefix_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_cmp_prefix", compoundKey, func(i int) bson.D {
		return bson.D{{Key: "grp", Value: int32(i % 1000)}}
	}, nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_CompoundFull_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_cmp_full", compoundKey, func(i int) bson.D {
		return bson.D{{Key: "grp", Value: int32(i % 1000)}, {Key: "k", Value: int32(i)}}
	}, nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_DottedEq_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_dot_eq", dottedKey, eqOn("a.b"), nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_DottedRange_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_dot_range", dottedKey, rangeOn("a.b", 50), nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_DottedIn_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_dot_in", dottedKey, inOn("a.b"), nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_DeepDottedEq_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_deep_eq", deepKey, func(i int) bson.D {
		return bson.D{{Key: "a.c.d", Value: int32(i % 1000)}}
	}, nil, 0, mustUseIndex)
}

func BenchmarkIndexUse_ThroughArrayEq_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_arr_eq", skuKey, func(i int) bson.D {
		return bson.D{{Key: "items.sku", Value: fmt.Sprintf("sku-%d-1", i)}}
	}, nil, 0, mustUseIndex)
}

// DumboDB's explain claims the index provides the order, but the backend scans
// and sorts in memory.
func BenchmarkIndexUse_FlatSortLimit_10K(b *testing.B) {
	benchmarkIndexedFind(b, "iu_flat_sort", flatKey, func(int) bson.D { return bson.D{} },
		bson.D{{Key: "k", Value: 1}}, 10, indexUseExpectation{knownDumboGap: "workspace-rdu"})
}

func benchmarkIndexedCount(b *testing.B, label string, keys bson.D, filter func(i int) bson.D) {
	col, ctx, dumbo := withIndexUseCollection(b, label, keys)
	cmd := func(i int) bson.D {
		return bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: filter(i)}}
	}
	requireIndexUse(b, ctx, col, dumbo, cmd(indexUseDocs/2), mustUseIndex)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := col.Database().RunCommand(ctx, cmd(i%indexUseDocs)).Err(); err != nil {
			b.Fatalf("count: %v", err)
		}
	}
}

func BenchmarkIndexUse_FlatCount_10K(b *testing.B) {
	benchmarkIndexedCount(b, "iu_flat_count", flatKey, eqOn("k"))
}

func BenchmarkIndexUse_DottedCount_10K(b *testing.B) {
	benchmarkIndexedCount(b, "iu_dot_count", dottedKey, eqOn("a.b"))
}

func benchmarkIndexedDistinct(b *testing.B, label string, keys bson.D, field string) {
	col, ctx, dumbo := withIndexUseCollection(b, label, keys)
	cmd := bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: field}, {Key: "query", Value: bson.D{}}}
	requireIndexUse(b, ctx, col, dumbo, cmd, mustUseIndex)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.Distinct(ctx, field, bson.D{}); err != nil {
			b.Fatalf("Distinct: %v", err)
		}
	}
}

func BenchmarkIndexUse_FlatDistinct_10K(b *testing.B) {
	benchmarkIndexedDistinct(b, "iu_flat_distinct", bson.D{{Key: "grp", Value: 1}}, "grp")
}

func BenchmarkIndexUse_DottedDistinct_10K(b *testing.B) {
	benchmarkIndexedDistinct(b, "iu_dot_distinct", deepKey, "a.c.d")
}

// Write paths: inserts maintain each index kind; dotted and through-array keys
// resolve their paths through every document.
func benchmarkIndexedInsert(b *testing.B, label string, keys bson.D, unique bool) {
	ctx := context.Background()
	client, err := connect(ctx)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	col, cleanup := freshCollection(ctx, client, label)
	b.Cleanup(func() {
		cleanup()
		_ = client.Disconnect(context.Background())
	})
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: keys, Options: options.Index().SetUnique(unique)}); err != nil {
		b.Fatalf("create index: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.InsertOne(ctx, indexUseDoc(i)); err != nil {
			b.Fatalf("InsertOne: %v", err)
		}
	}
}

func BenchmarkIndexUse_InsertUniqueFlat(b *testing.B) {
	benchmarkIndexedInsert(b, "iu_ins_flat", flatKey, true)
}

func BenchmarkIndexUse_InsertUniqueDotted(b *testing.B) {
	benchmarkIndexedInsert(b, "iu_ins_dot", dottedKey, true)
}

func BenchmarkIndexUse_InsertUniqueThroughArray(b *testing.B) {
	benchmarkIndexedInsert(b, "iu_ins_arr", skuKey, true)
}
