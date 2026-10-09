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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A single-field sort led by an index is answered by walking the index in
// order, so a limit stops the scan early: executionStats show only the
// documents the walk read. Without a usable index the sort is blocking and
// examines every document.

const sortWalkDocs = 300

func sortWalkSetup(keys bson.D) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 0, sortWalkDocs)
		for i := 0; i < sortWalkDocs; i++ {
			docs = append(docs, bson.D{
				{Key: "_id", Value: int32(i)},
				{Key: "k", Value: int32((i * 7) % sortWalkDocs)},
				{Key: "p", Value: fmt.Sprintf("p%d", i%3)},
			})
		}
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		if keys == nil {
			return nil
		}
		return createIndex(ctx, col, keys)
	}
}

func TestIndex_SortWalk(t *testing.T) {
	asc := bson.D{{Key: "k", Value: 1}}
	desc := bson.D{{Key: "k", Value: -1}}
	for _, tc := range []struct {
		name    string
		keys    bson.D
		filter  bson.D
		sort    bson.D
		support harness.DumboDBSupport
	}{
		{"AscLimit", asc, bson.D{}, asc, harness.DumboDBFull},
		{"DescLimit", asc, bson.D{}, desc, harness.DumboDBFull},
		{"DescIndexAscSort", desc, bson.D{}, asc, harness.DumboDBFull},
		{"CompoundLeading", bson.D{{Key: "k", Value: 1}, {Key: "p", Value: 1}}, bson.D{}, asc, harness.DumboDBFull},
		{"UnindexedFilter", asc, bson.D{{Key: "p", Value: "p1"}}, asc, harness.DumboDBFull},
		{"NoIndexControl", nil, bson.D{}, asc, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_SortWalk_" + tc.name,
				Support: tc.support,
				Setup:   sortWalkSetup(tc.keys),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cur, err := col.Find(ctx, tc.filter, options.Find().SetSort(tc.sort).SetLimit(5))
					if err != nil {
						return nil, err
					}
					var docs []bson.D
					if err := cur.All(ctx, &docs); err != nil {
						return nil, err
					}
					ids := bson.A{}
					for _, d := range docs {
						ids = append(ids, d[0].Value)
					}
					explain, err := explainRunExplain(ctx, col, bson.D{
						{Key: "find", Value: col.Name()},
						{Key: "filter", Value: tc.filter},
						{Key: "sort", Value: tc.sort},
						{Key: "limit", Value: int32(5)},
					}, "executionStats")
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "ids", Value: ids}, {Key: "explain", Value: explainExtractCritical(explain)}}, nil
				},
			})
		})
	}
}

func sortWalkOrderDocs(multikey, emptyArray bool) []interface{} {
	d := func(id int32, k interface{}) bson.D {
		doc := bson.D{{Key: "_id", Value: id}}
		if k != nil {
			doc = append(doc, bson.E{Key: "k", Value: k})
		}
		return doc
	}
	docs := []interface{}{
		d(1, nil),
		d(2, int32(1)),
		d(3, 1.5),
		d(4, int64(2)),
		d(5, "a"),
		d(6, "b"),
		d(7, bson.D{{Key: "x", Value: int32(2)}}),
		d(8, bson.D{{Key: "x", Value: int32(1)}}),
		d(9, bson.D{{Key: "y", Value: int32(0)}}),
		d(10, false),
		d(11, true),
		d(12, primitive.NewDateTimeFromTime(time.Unix(1700000000, 0))),
	}
	if multikey {
		docs = append(docs, d(13, bson.A{int32(7), int32(-1)}), d(14, bson.A{"c", int32(3)}))
	}
	if emptyArray {
		docs = append(docs, d(15, bson.A{}))
	}
	return docs
}

// The index walk must order values like MongoDB's sort: across types, among
// subdocuments (which share one index key), and for multikey documents by their
// minimum (ascending) or maximum (descending) element.
func TestIndex_SortWalk_Order(t *testing.T) {
	for _, data := range []struct {
		name                 string
		multikey, emptyArray bool
	}{
		{"Scalars", false, false},
		{"Multikey", true, false},
		{"MultikeyEmptyArray", true, true},
	} {
		for _, dir := range []int32{1, -1} {
			for _, limit := range []int64{0, 4} {
				name := fmt.Sprintf("%s_Dir%d_Limit%d", data.name, dir, limit)
				t.Run(name, func(t *testing.T) {
					harness.PairTest(t, harness.TestCase{
						Name:    "Index_SortWalk_Order_" + name,
						Support: harness.DumboDBFull,
						Setup: func(ctx context.Context, col *mongo.Collection) error {
							if _, err := col.InsertMany(ctx, sortWalkOrderDocs(data.multikey, data.emptyArray)); err != nil {
								return err
							}
							return createIndex(ctx, col, bson.D{{Key: "k", Value: 1}})
						},
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							out := bson.D{}
							for _, v := range []struct {
								name string
								hint interface{}
							}{
								{"collscan", bson.D{{Key: "$natural", Value: 1}}},
								{"planned", nil},
								{"hinted", bson.D{{Key: "k", Value: 1}}},
							} {
								opts := options.Find().SetSort(bson.D{{Key: "k", Value: dir}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
								if limit > 0 {
									opts.SetLimit(limit)
								}
								if v.hint != nil {
									opts.SetHint(v.hint)
								}
								cur, err := col.Find(ctx, bson.D{}, opts)
								if err != nil {
									return nil, err
								}
								var docs []bson.D
								if err := cur.All(ctx, &docs); err != nil {
									return nil, err
								}
								ids := bson.A{}
								for _, d := range docs {
									ids = append(ids, d[0].Value)
								}
								out = append(out, bson.E{Key: v.name, Value: ids})
							}
							return out, nil
						},
					})
				})
			}
		}
	}
}
