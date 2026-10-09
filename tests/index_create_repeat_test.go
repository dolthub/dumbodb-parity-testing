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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Repeating createIndexes with definitions that already exist is a no-op in
// MongoDB, for one index or several. Reported in dolthub/dumbodb#113.

func createIndexesCmd(col *mongo.Collection, specs ...bson.D) bson.D {
	indexes := bson.A{}
	for _, s := range specs {
		indexes = append(indexes, s)
	}
	return bson.D{{Key: "createIndexes", Value: col.Name()}, {Key: "indexes", Value: indexes}}
}

func indexSpec(name string, keys bson.D) bson.D {
	return bson.D{{Key: "key", Value: keys}, {Key: "name", Value: name}}
}

// createIndexesResult keeps the fields both servers report, dropping
// server-specific ones such as $clusterTime.
func createIndexesResult(ctx context.Context, col *mongo.Collection, cmd bson.D) bson.D {
	var res bson.M
	if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
		return bson.D{{Key: "error", Value: comparableErrorInfo(err)}}
	}
	out := bson.D{}
	for _, k := range []string{"numIndexesBefore", "numIndexesAfter", "note", "ok"} {
		if v, ok := res[k]; ok {
			out = append(out, bson.E{Key: k, Value: v})
		}
	}
	return out
}

func indexNames(ctx context.Context, col *mongo.Collection) (bson.A, error) {
	cur, err := col.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	var specs []bson.M
	if err := cur.All(ctx, &specs); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s["name"].(string))
	}
	sort.Strings(names)
	out := bson.A{}
	for _, n := range names {
		out = append(out, n)
	}
	return out, nil
}

func TestIndex_CreateIndexes_Repeat(t *testing.T) {
	method := indexSpec("method", bson.D{{Key: "methodKey", Value: 1}})
	name := indexSpec("name", bson.D{{Key: "fullyQualifiedName", Value: 1}})
	nested := indexSpec("domain_method", bson.D{{Key: "discoveryDomain.ID", Value: 1}, {Key: "methodKey", Value: 1}})
	for _, tc := range []struct {
		name    string
		first   []bson.D
		second  []bson.D
		support harness.DumboDBSupport
	}{
		{"Single", []bson.D{method}, []bson.D{method}, harness.DumboDBFull},
		{"TwoTopLevel", []bson.D{method, name}, []bson.D{method, name}, harness.DumboDBFull},
		{"CompoundNested", []bson.D{nested, name}, []bson.D{nested, name}, harness.DumboDBFull},
		{"OneExistingOneNew", []bson.D{method}, []bson.D{method, name}, harness.DumboDBFull},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Index_CreateIndexes_Repeat_" + tc.name,
				Support: tc.support,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "methodKey", Value: "m"}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					first := createIndexesResult(ctx, col, createIndexesCmd(col, tc.first...))
					second := createIndexesResult(ctx, col, createIndexesCmd(col, tc.second...))
					names, err := indexNames(ctx, col)
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "first", Value: first}, {Key: "second", Value: second}, {Key: "indexes", Value: names}}, nil
				},
			})
		})
	}
}
