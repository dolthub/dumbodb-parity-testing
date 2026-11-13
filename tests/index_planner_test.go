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
	"math"
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
	// work, when set, measures a read other than a find of filter.
	work func(ctx context.Context, col *mongo.Collection) (bson.D, error)
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
	ids, keys, docs, err := readWork(ctx, col, func() ([]string, error) {
		return findValues(ctx, col, filter, opts, "_id")
	})
	if err != nil {
		return nil, err
	}
	var returned interface{} = len(ids)
	if opts == nil || opts.Limit == nil || opts.Sort != nil {
		returned = ids
	}
	return bson.D{
		{Key: "returned", Value: returned},
		{Key: "docsExamined", Value: docs},
		{Key: "readsIndex", Value: keys > 0},
		{Key: "excessKeys", Value: keys > docs+excessKeySlack},
	}, nil
}

// readWork runs read under scannedBy and returns what it returned, sorted,
// with the index keys and documents it examined.
func readWork(ctx context.Context, col *mongo.Collection, read func() ([]string, error)) (returned []string, keys, docs int64, err error) {
	op := func() error {
		returned, err = read()
		return err
	}
	scanned, err := scannedBy(ctx, col, op, true)
	if err != nil {
		return nil, 0, 0, err
	}
	keys, _ = scanned[0].Value.(int64)
	docs, _ = scanned[1].Value.(int64)
	sort.Strings(returned)
	return returned, keys, docs, nil
}

// findValues runs a find and returns field of each document it returns.
func findValues(ctx context.Context, col *mongo.Collection, filter bson.D, opts *options.FindOptions, field string) ([]string, error) {
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, fmt.Sprint(d[field]))
	}
	return out, nil
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
					if tc.work != nil {
						return tc.work(ctx, col)
					}
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
		// The two ranges merge in k order, reading about the limit's keys
		// (dolthub/dumbodb#146).
		{name: "find_sort_limit", support: harness.DumboDBFull, filter: in, opts: options.Find().SetSort(sortK).SetLimit(5)},
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

// distinctWork measures a distinct of field under filter: the values, the
// documents examined, and whether it read about one index key per value, as a
// DISTINCT_SCAN does, rather than every key.
func distinctWork(field string, filter bson.D) func(context.Context, *mongo.Collection) (bson.D, error) {
	return func(ctx context.Context, col *mongo.Collection) (bson.D, error) {
		values, keys, docs, err := readWork(ctx, col, func() ([]string, error) {
			vals, err := col.Distinct(ctx, field, filter)
			out := make([]string, 0, len(vals))
			for _, v := range vals {
				out = append(out, fmt.Sprint(v))
			}
			return out, err
		})
		if err != nil {
			return nil, err
		}
		return bson.D{
			{Key: "returned", Value: values},
			{Key: "docsExamined", Value: docs},
			{Key: "keysPerValue", Value: keys <= int64(len(values))+excessKeySlack},
		}, nil
	}
}

// coveredFindWork measures a find projected to fields alone, comparing each
// returned document with its values' types.
func coveredFindWork(filter bson.D, opts *options.FindOptions, fields ...string) func(context.Context, *mongo.Collection) (bson.D, error) {
	return func(ctx context.Context, col *mongo.Collection) (bson.D, error) {
		projection := bson.D{{Key: "_id", Value: 0}}
		for _, f := range fields {
			projection = append(projection, bson.E{Key: f, Value: 1})
		}
		if opts == nil {
			opts = options.Find()
		}
		opts.SetProjection(projection)
		values, keys, docs, err := readWork(ctx, col, func() ([]string, error) {
			cur, err := col.Find(ctx, filter, opts)
			if err != nil {
				return nil, err
			}
			var found []bson.D
			if err := cur.All(ctx, &found); err != nil {
				return nil, err
			}
			out := make([]string, 0, len(found))
			for _, d := range found {
				entry := ""
				for _, e := range d {
					entry += fmt.Sprintf("%s=%T(%v) ", e.Key, e.Value, e.Value)
				}
				out = append(out, entry)
			}
			return out, nil
		})
		if err != nil {
			return nil, err
		}
		return bson.D{{Key: "returned", Value: values}, {Key: "docsExamined", Value: docs}, {Key: "readsIndex", Value: keys > 0}}, nil
	}
}

// groupWork measures an aggregate that returns one document per group, keyed
// by _id.
func groupWork(pipeline bson.A) func(context.Context, *mongo.Collection) (bson.D, error) {
	return func(ctx context.Context, col *mongo.Collection) (bson.D, error) {
		groups, keys, docs, err := readWork(ctx, col, func() ([]string, error) {
			cur, err := col.Aggregate(ctx, pipeline)
			if err != nil {
				return nil, err
			}
			var out []bson.M
			if err := cur.All(ctx, &out); err != nil {
				return nil, err
			}
			ids := make([]string, 0, len(out))
			for _, d := range out {
				ids = append(ids, fmt.Sprint(d["_id"]))
			}
			return ids, nil
		})
		if err != nil {
			return nil, err
		}
		return bson.D{{Key: "returned", Value: groups}, {Key: "docsExamined", Value: docs}, {Key: "readsIndex", Value: keys > 0}}, nil
	}
}

func setupGrpK(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 0, 10000)
	for i := 0; i < 10000; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "k", Value: int32(i)}, {Key: "grp", Value: int32(i % 1000)}})
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "k", Value: 1}}},
		{Keys: bson.D{{Key: "grp", Value: 1}}},
	})
	return err
}

// TestIndexPlanner_Distinct: distinct on an indexed field is a DISTINCT_SCAN,
// one key per value and no documents (dolthub/dumbodb#141).
func TestIndexPlanner_Distinct(t *testing.T) {
	runPlannerCases(t, "IndexPlanner_Distinct", setupGrpK, []plannerCase{
		{name: "all", support: harness.DumboDBFull, work: distinctWork("grp", bson.D{})},
		{name: "filtered_on_key", support: harness.DumboDBFull,
			work: distinctWork("grp", bson.D{{Key: "grp", Value: bson.D{{Key: "$lt", Value: int32(100)}}}})},
	})
}

// TestIndexPlanner_Covered: a read that needs only indexed fields, a find's
// projection or a $group's key, is answered from the index keys without
// fetching documents (dolthub/dumbodb#142, #139).
func TestIndexPlanner_Covered(t *testing.T) {
	rangeK := bson.D{{Key: "k", Value: bson.D{{Key: "$gte", Value: int32(5000)}, {Key: "$lt", Value: int32(5050)}}}}
	groupGrp := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "grp", Value: bson.D{{Key: "$gte", Value: int32(0)}}}}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$grp"}}}},
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if err := setupGrpK(ctx, col); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "grp", Value: 1}, {Key: "k", Value: 1}})
	}
	runPlannerCases(t, "IndexPlanner_Covered", setup, []plannerCase{
		{name: "projection", support: harness.DumboDBFull, work: coveredFindWork(rangeK, nil, "k")},
		// grp_1_k_1 covers k, so it serves grp rather than grp_1.
		{name: "compound", support: harness.DumboDBFull, work: coveredFindWork(bson.D{{Key: "grp", Value: int32(7)}}, nil, "grp", "k")},
		{name: "sort_limit", support: harness.DumboDBFull, work: coveredFindWork(rangeK, options.Find().SetSort(bson.D{{Key: "k", Value: -1}}).SetLimit(5), "k")},
		{name: "match_group", support: harness.DumboDBFull, work: groupWork(groupGrp)},
	})

	// Numbers of every type share one index encoding; a covered read must
	// still return each as the type stored.
	typed := func(ctx context.Context, col *mongo.Collection) error {
		values := []interface{}{int32(5), int64(6), 7.0, 7.5, -0.25, math.Copysign(0, -1), int64(1) << 40, "s", true, nil}
		docs := make([]interface{}, 0, len(values))
		for i, v := range values {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: v}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "v", Value: 1}})
	}
	runPlannerCases(t, "IndexPlanner_Covered", typed, []plannerCase{
		{name: "typed_values", support: harness.DumboDBFull, work: coveredFindWork(bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: int32(-10)}}}}, nil, "v")},
	})
}

// TestIndexPlanner_DottedNull: {"a.b": null} on an index of a dotted path.
// A null key is indexed for a missing field, including for each array element
// that lacks it, so the index serves the query (dolthub/dumbodb#181).
func TestIndexPlanner_DottedNull(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			arr := bson.A{bson.D{{Key: "b", Value: int32(i)}}}
			if i%10 == 0 {
				arr = append(arr, bson.D{{Key: "c", Value: int32(i)}})
			}
			doc := bson.D{{Key: "_id", Value: int32(i)}, {Key: "a", Value: arr}}
			if i%10 != 3 {
				doc = append(doc, bson.E{Key: "s", Value: bson.D{{Key: "b", Value: int32(i)}}})
			}
			// e holds the shapes a null equality through an array reaches:
			// an empty array, a non-document element, an explicit null.
			if i%100 < 4 {
				e := []interface{}{bson.A{}, bson.A{int32(3), bson.D{{Key: "b", Value: int32(i)}}}, bson.D{{Key: "b", Value: nil}}, bson.A{bson.D{{Key: "b", Value: int32(i)}}}}[i%100]
				doc = append(doc, bson.E{Key: "e", Value: e})
			} else {
				doc = append(doc, bson.E{Key: "e", Value: bson.D{{Key: "b", Value: int32(i)}}})
			}
			docs = append(docs, doc)
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
			{Keys: bson.D{{Key: "a.b", Value: 1}}},
			{Keys: bson.D{{Key: "s.b", Value: 1}}},
			{Keys: bson.D{{Key: "e.b", Value: 1}}},
		})
		return err
	}
	runPlannerCases(t, "IndexPlanner_DottedNull", setup, []plannerCase{
		{name: "array_element_missing_field", support: harness.DumboDBFull, filter: bson.D{{Key: "a.b", Value: nil}}},
		{name: "subdocument_missing_field", support: harness.DumboDBFull, filter: bson.D{{Key: "s.b", Value: nil}}},
		{name: "array_shapes", support: harness.DumboDBFull, filter: bson.D{{Key: "e.b", Value: nil}}},
		{name: "array_shapes_eq_operator", support: harness.DumboDBFull, filter: bson.D{{Key: "e.b", Value: bson.D{{Key: "$eq", Value: nil}}}}},
	})
}

// nearWork measures a read nearest first: the ids it returns and whether it
// fetched at most a tenth of the collection, as a read from a geo index does.
func nearWork(read func(ctx context.Context, col *mongo.Collection) ([]bson.M, error)) func(context.Context, *mongo.Collection) (bson.D, error) {
	return func(ctx context.Context, col *mongo.Collection) (bson.D, error) {
		var ids []string
		scanned, err := scannedBy(ctx, col, func() error {
			docs, err := read(ctx, col)
			ids = ids[:0]
			for _, d := range docs {
				ids = append(ids, fmt.Sprint(d["_id"]))
			}
			return err
		}, true)
		if err != nil {
			return nil, err
		}
		docs, _ := scanned[1].Value.(int64)
		return bson.D{{Key: "returned", Value: ids}, {Key: "fewDocs", Value: docs <= 1000}}, nil
	}
}

// TestIndexPlanner_GeoNear: a $near find and a $geoNear pipeline with a limit
// read a 2dsphere index nearest first and stop, fetching a small part of the
// collection (dolthub/dumbodb#124). The query point is off the grid, so no
// two points tie.
func TestIndexPlanner_GeoNear(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, 10000)
		for i := 0; i < 10000; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "loc", Value: bson.D{
				{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{float64(i%100) * 0.01, float64(i/100) * 0.01}},
			}}})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}})
		return err
	}
	point := bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{0.5031, 0.4127}}}
	all := func(cur *mongo.Cursor, err error) ([]bson.M, error) {
		if err != nil {
			return nil, err
		}
		var docs []bson.M
		err = cur.All(context.Background(), &docs)
		return docs, err
	}
	runPlannerCases(t, "IndexPlanner_GeoNear", setup, []plannerCase{
		{name: "near_limit", support: harness.DumboDBFull, work: nearWork(func(ctx context.Context, col *mongo.Collection) ([]bson.M, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{{Key: "$geometry", Value: point}}}}}}
			return all(col.Find(ctx, filter, options.Find().SetLimit(10)))
		})},
		{name: "near_max_distance", support: harness.DumboDBFull, work: nearWork(func(ctx context.Context, col *mongo.Collection) ([]bson.M, error) {
			filter := bson.D{{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.D{{Key: "$geometry", Value: point}, {Key: "$maxDistance", Value: 3000}}}}}}
			return all(col.Find(ctx, filter))
		})},
		{name: "geo_near_limit", support: harness.DumboDBFull, work: nearWork(func(ctx context.Context, col *mongo.Collection) ([]bson.M, error) {
			return all(col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$geoNear", Value: bson.D{{Key: "near", Value: point}, {Key: "distanceField", Value: "d"}, {Key: "spherical", Value: true}}}},
				bson.D{{Key: "$limit", Value: 10}},
			}))
		})},
	})
}
