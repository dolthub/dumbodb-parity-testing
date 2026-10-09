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

// Non-unique indexes on dotted paths. Each query runs as a forced collection
// scan, as planned, and with the index hinted; MongoDB returns the same result
// all three ways, so a dumbodb planner that reads a wrong index diverges.

const dottedIndexName = "dotted_idx"

// dottedShallowDocs probes how a.b resolves across document shapes.
func dottedShallowDocs() []interface{} {
	d := func(id int32, k int32, a interface{}) bson.D {
		doc := bson.D{{Key: "_id", Value: id}, {Key: "k", Value: k}}
		if a != nil {
			doc = append(doc, bson.E{Key: "a", Value: a})
		}
		return doc
	}
	b := func(v interface{}) bson.D { return bson.D{{Key: "b", Value: v}} }
	return []interface{}{
		d(1, 1, b(int32(1))),
		d(2, 2, b(int32(2))),
		d(3, 1, b(bson.A{int32(1), int32(3)})),
		d(4, 2, bson.A{b(int32(1)), b(int32(4))}),
		d(5, 1, bson.A{b(int32(5)), bson.D{{Key: "c", Value: int32(1)}}}),
		d(6, 2, bson.D{{Key: "c", Value: int32(1)}}),
		d(7, 1, int32(5)),
		d(8, 2, nil),
		d(9, 1, b(nil)),
		d(10, 2, bson.A{}),
		d(11, 1, b(bson.A{})),
		d(12, 2, bson.A{bson.A{b(int32(6))}}),
		d(13, 1, b(bson.D{{Key: "c", Value: int32(1)}})),
		d(14, 2, b("s")),
		d(15, 1, b(int32(2))),
	}
}

// dottedDeepDocs probes a.b.c.d with arrays and early stops at each level.
func dottedDeepDocs() []interface{} {
	cd := func(v interface{}) bson.D { return bson.D{{Key: "c", Value: bson.D{{Key: "d", Value: v}}}} }
	return []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "b", Value: cd(int32(1))}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.D{{Key: "b", Value: bson.A{cd(int32(1)), cd(int32(2))}}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: bson.D{{Key: "b", Value: bson.D{{Key: "c", Value: bson.A{bson.D{{Key: "d", Value: int32(3)}}}}}}}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{bson.D{{Key: "b", Value: cd(int32(4))}}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.D{{Key: "b", Value: bson.D{{Key: "c", Value: int32(5)}}}}}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: bson.D{{Key: "b", Value: bson.D{}}}}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "a", Value: bson.D{{Key: "b", Value: cd(bson.A{int32(2), int32(7)})}}}},
		bson.D{{Key: "_id", Value: int32(8)}},
	}
}

func dottedSetup(docs []interface{}, keys bson.D) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    keys,
			Options: options.Index().SetName(dottedIndexName),
		})
		return err
	}
}

// dottedFindIDs returns _ids in result order. A nil sort orders by _id.
func dottedFindIDs(ctx context.Context, col *mongo.Collection, filter, sort bson.D, hint interface{}) (bson.A, error) {
	if sort == nil {
		sort = bson.D{{Key: "_id", Value: 1}}
	}
	opts := options.Find().SetSort(sort).SetProjection(bson.D{{Key: "_id", Value: 1}})
	if hint != nil {
		opts.SetHint(hint)
	}
	cur, err := col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	var docs []bson.D
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	out := bson.A{}
	for _, d := range docs {
		out = append(out, d[0].Value)
	}
	return out, nil
}

// dottedFindAllWays runs the query as a collection scan, as planned, and
// hinted onto the dotted index.
func dottedFindAllWays(filter, sort bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		out := bson.D{}
		for _, v := range []struct {
			name string
			hint interface{}
		}{
			{"collscan", bson.D{{Key: "$natural", Value: 1}}},
			{"planned", nil},
			{"hinted", dottedIndexName},
		} {
			ids, err := dottedFindIDs(ctx, col, filter, sort, v.hint)
			if err != nil {
				out = append(out, bson.E{Key: v.name, Value: comparableErrorInfo(err)})
				continue
			}
			out = append(out, bson.E{Key: v.name, Value: ids})
		}
		return out, nil
	}
}

func TestIndex_DottedPath_Find(t *testing.T) {
	shallow := bson.D{{Key: "a.b", Value: 1}}
	deep := bson.D{{Key: "a.b.c.d", Value: 1}}
	compound := bson.D{{Key: "a.b", Value: 1}, {Key: "k", Value: 1}}
	cases := []struct {
		name    string
		docs    []interface{}
		keys    bson.D
		filter  bson.D
		sort    bson.D
		support harness.DumboDBSupport
	}{
		{"Eq", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: int32(1)}}, nil, harness.DumboDBFull},
		{"EqNoMatch", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: int32(99)}}, nil, harness.DumboDBFull},
		{"EqString", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: "s"}}, nil, harness.DumboDBFull},
		{"EqNull", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: nil}}, nil, harness.DumboDBFull},
		{"EqEmptyArray", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.A{}}}, nil, harness.DumboDBFull},
		{"EqWholeArray", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.A{int32(1), int32(3)}}}, nil, harness.DumboDBFull},
		{"EqSubdoc", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "c", Value: int32(1)}}}}, nil, harness.DumboDBFull},
		{"Gt", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}, nil, harness.DumboDBFull},
		{"Range", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$gte", Value: int32(2)}, {Key: "$lt", Value: int32(5)}}}}, nil, harness.DumboDBFull},
		{"In", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$in", Value: bson.A{int32(2), int32(5)}}}}}, nil, harness.DumboDBFull},
		{"InWithNull", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$in", Value: bson.A{int32(4), nil}}}}}, nil, harness.DumboDBFull},
		{"Ne", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$ne", Value: int32(1)}}}}, nil, harness.DumboDBFull},
		{"Nin", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$nin", Value: bson.A{int32(1), int32(5)}}}}}, nil, harness.DumboDBFull},
		{"NotGt", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$gt", Value: int32(2)}}}}}}, nil, harness.DumboDBFull},
		{"OpsEvaluatedSeparately", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$gt", Value: int32(2)}, {Key: "$lt", Value: int32(3)}}}}, nil, harness.DumboDBFull},
		{"ExistsTrue", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$exists", Value: true}}}}, nil, harness.DumboDBFull},
		{"ExistsFalse", dottedShallowDocs(), shallow, bson.D{{Key: "a.b", Value: bson.D{{Key: "$exists", Value: false}}}}, nil, harness.DumboDBFull},
		{"Positional", dottedShallowDocs(), shallow, bson.D{{Key: "a.b.0", Value: int32(1)}}, nil, harness.DumboDBFull},
		{"SortAsc", dottedShallowDocs(), shallow, bson.D{}, bson.D{{Key: "a.b", Value: 1}, {Key: "_id", Value: 1}}, harness.DumboDBFull},
		{"SortDesc", dottedShallowDocs(), shallow, bson.D{}, bson.D{{Key: "a.b", Value: -1}, {Key: "_id", Value: 1}}, harness.DumboDBFull},
		{"CompoundPrefix", dottedShallowDocs(), compound, bson.D{{Key: "a.b", Value: int32(1)}}, nil, harness.DumboDBFull},
		{"CompoundFull", dottedShallowDocs(), compound, bson.D{{Key: "a.b", Value: int32(1)}, {Key: "k", Value: int32(2)}}, nil, harness.DumboDBFull},
		{"DeepEq", dottedDeepDocs(), deep, bson.D{{Key: "a.b.c.d", Value: int32(1)}}, nil, harness.DumboDBFull},
		{"DeepEqArrayElem", dottedDeepDocs(), deep, bson.D{{Key: "a.b.c.d", Value: int32(2)}}, nil, harness.DumboDBFull},
		{"DeepEqNull", dottedDeepDocs(), deep, bson.D{{Key: "a.b.c.d", Value: nil}}, nil, harness.DumboDBFull},
		{"DeepRange", dottedDeepDocs(), deep, bson.D{{Key: "a.b.c.d", Value: bson.D{{Key: "$gte", Value: int32(3)}, {Key: "$lte", Value: int32(5)}}}}, nil, harness.DumboDBFull},
		{"DeepSortAsc", dottedDeepDocs(), deep, bson.D{}, bson.D{{Key: "a.b.c.d", Value: 1}, {Key: "_id", Value: 1}}, harness.DumboDBFull},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_DottedPath_Find_" + tc.name,
				Support: tc.support,
				Setup:   dottedSetup(tc.docs, tc.keys),
				Run:     dottedFindAllWays(tc.filter, tc.sort),
			})
		})
	}
}

func TestIndex_DottedPath_Count(t *testing.T) {
	for _, tc := range []struct {
		name    string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"Eq", bson.D{{Key: "a.b", Value: int32(1)}}, harness.DumboDBFull},
		{"Range", bson.D{{Key: "a.b", Value: bson.D{{Key: "$gte", Value: int32(2)}, {Key: "$lt", Value: int32(5)}}}}, harness.DumboDBFull},
		{"Null", bson.D{{Key: "a.b", Value: nil}}, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_DottedPath_Count_" + tc.name,
				Support: tc.support,
				Setup:   dottedSetup(dottedShallowDocs(), bson.D{{Key: "a.b", Value: 1}}),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					planned, err := col.CountDocuments(ctx, tc.filter)
					if err != nil {
						return nil, err
					}
					hinted, err := col.CountDocuments(ctx, tc.filter, options.Count().SetHint(dottedIndexName))
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "planned", Value: planned}, {Key: "hinted", Value: hinted}}, nil
				},
			})
		})
	}
}

// All XFail (workspace-3cb): unfiltered, MongoDB answers from the multikey
// index with DISTINCT_SCAN and returns raw index keys, so an empty array yields
// undefined and a missing branch yields null. Filtered, it reads documents and
// returns neither; dumbodb always reads documents.
func TestIndex_DottedPath_Distinct(t *testing.T) {
	filtered := bson.D{{Key: "_id", Value: bson.D{{Key: "$lte", Value: int32(4)}}}}
	for _, tc := range []struct {
		name    string
		docs    []interface{}
		field   string
		filter  bson.D
		support harness.DumboDBSupport
	}{
		{"ShallowAll", dottedShallowDocs(), "a.b", bson.D{}, harness.DumboDBXFail},
		{"ShallowFiltered", dottedShallowDocs(), "a.b", filtered, harness.DumboDBFull},
		{"DeepAll", dottedDeepDocs(), "a.b.c.d", bson.D{}, harness.DumboDBXFail},
		{"DeepFiltered", dottedDeepDocs(), "a.b.c.d", filtered, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_DottedPath_Distinct_" + tc.name,
				Support: tc.support,
				Setup:   dottedSetup(tc.docs, bson.D{{Key: tc.field, Value: 1}}),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					vals, err := col.Distinct(ctx, tc.field, tc.filter)
					if err != nil {
						return nil, err
					}
					return bson.A(vals), nil
				},
			})
		})
	}
}

// TestIndex_DottedPath_ExplainUsesIndex compares the winning plan's stage chain
// and index for selective filters on a dotted-path index.
// XFail: dumbodb's planner skips dotted filter fields (workspace-4tl.2) and
// reports COLLSCAN where MongoDB reports FETCH over IXSCAN.
func TestIndex_DottedPath_ExplainUsesIndex(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter bson.D
	}{
		{"Eq", bson.D{{Key: "a.b", Value: int32(2)}}},
		{"Range", bson.D{{Key: "a.b", Value: bson.D{{Key: "$gte", Value: int32(2)}, {Key: "$lt", Value: int32(5)}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_DottedPath_ExplainUsesIndex_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   dottedSetup(dottedShallowDocs(), bson.D{{Key: "a.b", Value: 1}}),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					doc, err := explainRunExplain(ctx, col, bson.D{
						{Key: "find", Value: col.Name()},
						{Key: "filter", Value: tc.filter},
					}, "queryPlanner")
					if err != nil {
						return nil, err
					}
					return explainExtractCritical(doc), nil
				},
			})
		})
	}
}
