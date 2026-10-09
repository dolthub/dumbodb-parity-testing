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
		{"AscLimit", asc, bson.D{}, asc, harness.DumboDBXFail},
		{"DescLimit", asc, bson.D{}, desc, harness.DumboDBXFail},
		{"DescIndexAscSort", desc, bson.D{}, asc, harness.DumboDBXFail},
		{"CompoundLeading", bson.D{{Key: "k", Value: 1}, {Key: "p", Value: 1}}, bson.D{}, asc, harness.DumboDBXFail},
		{"UnindexedFilter", asc, bson.D{{Key: "p", Value: "p1"}}, asc, harness.DumboDBXFail},
		{"NoIndexControl", nil, bson.D{}, asc, harness.DumboDBXFail},
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
