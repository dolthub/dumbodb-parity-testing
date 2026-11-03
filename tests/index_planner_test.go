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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Index-planner parity: which documents a query returns, whether it reads
// its index at runtime, how many documents it fetches, and whether it reads
// index keys well beyond them, measured with serverStatus counters
// (scannedBy). Exact key counts are not compared: mongod examines one extra
// key per range it scans, which DumboDB does not count.

type plannerCase struct {
	name    string
	support harness.DumboDBSupport
	filter  bson.D
	opts    *options.FindOptions
	// mongoExpect and dumboExpect assert each server's own plannerWork result
	// for a DumboDBDeviates case.
	mongoExpect, dumboExpect func(t *testing.T, work bson.D)
}

// excessKeySlack is how many more index keys than documents examined a query
// may read before plannerWork reports excessKeys: mongod's extra key per range
// for the few ranges these cases scan.
const excessKeySlack = 16

// plannerWork runs the query under scannedBy and reports its result and work.
// Which documents an unsorted limit returns depends on the order of equal
// index keys, by record id in mongod and by a hash of _id in DumboDB, so for
// those queries only the number returned is compared.
func plannerWork(ctx context.Context, col *mongo.Collection, filter bson.D, opts *options.FindOptions) (bson.D, error) {
	var ids []string
	op := func() error {
		cur, err := col.Find(ctx, filter, opts)
		if err != nil {
			return err
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return err
		}
		ids = ids[:0]
		for _, d := range docs {
			ids = append(ids, fmt.Sprint(d["_id"]))
		}
		return nil
	}
	scanned, err := scannedBy(ctx, col, op, true)
	if err != nil {
		return nil, err
	}
	keys, _ := scanned[0].Value.(int64)
	docs, _ := scanned[1].Value.(int64)
	var returned interface{} = len(ids)
	if opts == nil || opts.Limit == nil || opts.Sort != nil {
		sort.Strings(ids)
		returned = ids
	}
	return bson.D{
		{Key: "returned", Value: returned},
		{Key: "docsExamined", Value: docs},
		{Key: "readsIndex", Value: keys > 0},
		{Key: "excessKeys", Value: keys > docs+excessKeySlack},
	}, nil
}

func runPlannerCases(t *testing.T, prefix string, setup func(context.Context, *mongo.Collection) error, cases []plannerCase) {
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			expect := func(check func(*testing.T, bson.D)) func(*testing.T, interface{}, error) {
				if check == nil {
					return nil
				}
				return func(t *testing.T, res interface{}, err error) {
					if err != nil {
						t.Fatalf("%s: %v", tc.name, err)
					}
					check(t, res.(bson.D))
				}
			}
			harness.PairTest(t, harness.TestCase{
				Name:    fmt.Sprintf("%s_%s", prefix, tc.name),
				Support: tc.support,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return plannerWork(ctx, col, tc.filter, tc.opts)
				},
				MongoExpect: expect(tc.mongoExpect),
				DumboExpect: expect(tc.dumboExpect),
			})
		})
	}
}

// expectWork returns a check that plannerWork reported want for each key in
// want.
func expectWork(want bson.D) func(*testing.T, bson.D) {
	return func(t *testing.T, work bson.D) {
		t.Helper()
		got := work.Map()
		for _, e := range want {
			if fmt.Sprint(got[e.Key]) != fmt.Sprint(e.Value) {
				t.Errorf("%s = %v, want %v", e.Key, got[e.Key], e.Value)
			}
		}
	}
}

// TestIndexPlanner_CompoundIn: $in on the leading field of a compound index
// (dolthub/dumbodb#131).
func TestIndexPlanner_CompoundIn(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "bucket", Value: int32(i % 50)}, {Key: "k", Value: int32(i)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "bucket", Value: 1}, {Key: "k", Value: 1}}})
		return err
	}
	in := bson.D{{Key: "bucket", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(3)}}}}}
	sortK := bson.D{{Key: "k", Value: 1}}
	runPlannerCases(t, "IndexPlanner_CompoundIn", setup, []plannerCase{
		{name: "find", support: harness.DumboDBFull, filter: in},
		{name: "find_sort", support: harness.DumboDBFull, filter: in, opts: options.Find().SetSort(sortK)},
		{name: "find_limit", support: harness.DumboDBFull, filter: in, opts: options.Find().SetLimit(5)},
		// mongod merges the two ranges in k order and reads about 5 keys;
		// DumboDB reads both ranges whole to sort them (dolthub/dumbodb#146).
		{name: "find_sort_limit", support: harness.DumboDBXFail, filter: in, opts: options.Find().SetSort(sortK).SetLimit(5)},
	})
}

// TestIndexPlanner_BoundShapes: null equality (dolthub/dumbodb#133), $exists
// on a sparse index (#134) and an anchored regex prefix (#135) can each use an
// index. Null on a sparse index cannot: the index omits documents missing the
// field, which {field: null} matches.
func TestIndexPlanner_BoundShapes(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			doc := bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: fmt.Sprintf("s%04d", i)}}
			if i%10 == 0 {
				doc = append(doc, bson.E{Key: "n", Value: nil}, bson.E{Key: "sp", Value: int32(i)})
			} else {
				doc = append(doc, bson.E{Key: "n", Value: int32(i)})
			}
			docs = append(docs, doc)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "n", Value: 1}}},
			{Keys: bson.D{{Key: "sp", Value: 1}}, Options: options.Index().SetSparse(true)},
			{Keys: bson.D{{Key: "s", Value: 1}}},
		})
		return err
	}
	regex := bson.D{{Key: "s", Value: primitive.Regex{Pattern: "^s050"}}}
	runPlannerCases(t, "IndexPlanner_BoundShapes", setup, []plannerCase{
		{name: "null_eq", support: harness.DumboDBFull, filter: bson.D{{Key: "n", Value: nil}}},
		{name: "null_eq_operator", support: harness.DumboDBFull, filter: bson.D{{Key: "n", Value: bson.D{{Key: "$eq", Value: nil}}}}},
		{name: "null_on_sparse_index", support: harness.DumboDBFull, filter: bson.D{{Key: "sp", Value: nil}}},
		{name: "exists_on_sparse_index", support: harness.DumboDBFull, filter: bson.D{{Key: "sp", Value: bson.D{{Key: "$exists", Value: true}}}}},
		{name: "regex_prefix", support: harness.DumboDBFull, filter: regex},
		{name: "regex_prefix_operator", support: harness.DumboDBFull, filter: bson.D{{Key: "s", Value: bson.D{{Key: "$regex", Value: "^s050"}}}}},
	})
}

// TestIndexPlanner_PartialImplication: a partial index whose condition is a
// range or $exists is usable when the query implies that condition
// (dolthub/dumbodb#136). A query that does not imply it must not use it.
func TestIndexPlanner_PartialImplication(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			doc := bson.D{{Key: "_id", Value: int32(i)}, {Key: "k", Value: int32(i)}, {Key: "grp", Value: int32(i % 1000)}, {Key: "m", Value: int32(i)}}
			if i%10 == 0 {
				doc = append(doc, bson.E{Key: "opt", Value: int32(i)})
			}
			docs = append(docs, doc)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "k", Value: 1}}, Options: options.Index().SetPartialFilterExpression(
				bson.D{{Key: "grp", Value: bson.D{{Key: "$lt", Value: int32(100)}}}})},
			{Keys: bson.D{{Key: "m", Value: 1}}, Options: options.Index().SetPartialFilterExpression(
				bson.D{{Key: "opt", Value: bson.D{{Key: "$exists", Value: true}}}})},
		})
		return err
	}
	k := bson.E{Key: "k", Value: int32(5055)}
	m := bson.E{Key: "m", Value: int32(5050)}
	noOpt := bson.E{Key: "m", Value: int32(5051)}
	runPlannerCases(t, "IndexPlanner_PartialImplication", setup, []plannerCase{
		{name: "range_repeated", support: harness.DumboDBFull, filter: bson.D{k, {Key: "grp", Value: bson.D{{Key: "$lt", Value: int32(100)}}}}},
		{name: "range_tightened", support: harness.DumboDBFull, filter: bson.D{k, {Key: "grp", Value: bson.D{{Key: "$gte", Value: int32(10)}, {Key: "$lt", Value: int32(60)}}}}},
		{name: "range_by_equality", support: harness.DumboDBFull, filter: bson.D{k, {Key: "grp", Value: int32(55)}}},
		{name: "range_not_implied", support: harness.DumboDBFull, filter: bson.D{k, {Key: "grp", Value: bson.D{{Key: "$lt", Value: int32(200)}}}}},
		{name: "exists_repeated", support: harness.DumboDBFull, filter: bson.D{m, {Key: "opt", Value: bson.D{{Key: "$exists", Value: true}}}}},
		{name: "exists_by_equality", support: harness.DumboDBFull, filter: bson.D{m, {Key: "opt", Value: int32(5050)}}},
		{name: "exists_not_implied", support: harness.DumboDBFull, filter: bson.D{m}},
		// Each condition on opt below also matches a document missing opt, as
		// the one with m 5051 is, so the partial index on m must not serve it.
		{name: "exists_null_matches_missing", support: harness.DumboDBFull, filter: bson.D{noOpt, {Key: "opt", Value: nil}}},
		{name: "exists_in_null_matches_missing", support: harness.DumboDBFull, filter: bson.D{noOpt, {Key: "opt", Value: bson.D{{Key: "$in", Value: bson.A{nil, int32(3)}}}}}},
		{name: "exists_ne_matches_missing", support: harness.DumboDBFull, filter: bson.D{noOpt, {Key: "opt", Value: bson.D{{Key: "$ne", Value: int32(3)}}}}},
		{name: "exists_false_matches_missing", support: harness.DumboDBFull, filter: bson.D{noOpt, {Key: "opt", Value: bson.D{{Key: "$exists", Value: false}}}}},
	})
}

// TestIndexPlanner_WideRange: a range matching most of the collection
// (dolthub/dumbodb#139). A hint or a limit uses the index as mongod does.
// Without either, DumboDB deliberately scans the collection where mongod
// reads the index: past half the collection, one point lookup per match
// costs more than a sequential scan.
func TestIndexPlanner_WideRange(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "g", Value: int32(i % 10)}, {Key: "b", Value: int32(i % 7)}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "g", Value: 1}}})
		return err
	}
	wide := bson.D{{Key: "g", Value: bson.D{{Key: "$gte", Value: int32(1)}}}}
	runPlannerCases(t, "IndexPlanner_WideRange", setup, []plannerCase{
		{name: "narrow", support: harness.DumboDBFull, filter: bson.D{{Key: "g", Value: int32(3)}}},
		{name: "hinted", support: harness.DumboDBFull, filter: wide, opts: options.Find().SetHint(bson.D{{Key: "g", Value: 1}})},
		{name: "limit", support: harness.DumboDBFull, filter: wide, opts: options.Find().SetLimit(5)},
		{name: "in_limit", support: harness.DumboDBFull, filter: bson.D{{Key: "g", Value: bson.D{{Key: "$in", Value: bson.A{int32(2), int32(4), int32(6), int32(8)}}}}}, opts: options.Find().SetLimit(5)},
		{name: "nested_or", support: harness.DumboDBFull, filter: bson.D{{Key: "$or", Value: bson.A{
			bson.D{{Key: "g", Value: int32(1)}},
			bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "g", Value: int32(2)}}}}},
		}}}},
		// An $or beside an indexed field leaves the field's index usable.
		{name: "or_beside_indexed_field", support: harness.DumboDBFull, filter: bson.D{
			{Key: "g", Value: int32(3)},
			{Key: "$or", Value: bson.A{bson.D{{Key: "b", Value: int32(1)}}, bson.D{{Key: "b", Value: int32(5)}}}},
		}},
		{name: "unhinted", support: harness.DumboDBDeviates, filter: wide,
			mongoExpect: expectWork(bson.D{{Key: "readsIndex", Value: true}, {Key: "docsExamined", Value: 9000}}),
			dumboExpect: expectWork(bson.D{{Key: "readsIndex", Value: false}, {Key: "docsExamined", Value: 10000}})},
	})
}
