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

// A distinct filtered only on its own key reads the key's index range.

func distinctOnKeySetup(arrays bool) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		values := []interface{}{int32(1), 1.0, int64(1), int32(2), 2.5, int32(150), "a", "b", nil, bson.D{{Key: "x", Value: int32(1)}}, true, int32(-3)}
		if arrays {
			values = append(values, bson.A{int32(5), int32(500)}, bson.A{})
		}
		docs := make([]interface{}, 0, len(values)*3+1)
		for i := 0; i < len(values)*3; i++ {
			docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: values[i%len(values)]}})
		}
		docs = append(docs, bson.D{{Key: "_id", Value: int32(1000)}})
		if _, err := col.InsertMany(ctx, docs); err != nil {
			return err
		}
		return createIndex(ctx, col, bson.D{{Key: "v", Value: 1}})
	}
}

func TestDistinct_FilteredOnKey(t *testing.T) {
	for _, arrays := range []bool{false, true} {
		for _, tc := range []struct {
			name    string
			filter  bson.D
			numeric bool
		}{
			{"Lt", bson.D{{Key: "v", Value: bson.D{{Key: "$lt", Value: int32(100)}}}}, true},
			{"Range", bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: int32(1)}, {Key: "$lte", Value: int32(2)}}}}, true},
			{"Eq", bson.D{{Key: "v", Value: int32(1)}}, true},
			{"StringRange", bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: ""}}}}, false},
			{"Empty", bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: int32(1000)}}}}, false},
		} {
			name := tc.name
			support := harness.DumboDBFull
			if arrays {
				name += "_Multikey"
				// XFail workspace-3so: keeps double 1.0 where mongod returns int32 1.
				if tc.numeric {
					support = harness.DumboDBXFail
				}
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "Distinct_FilteredOnKey_" + name,
					Support: support,
					Setup:   distinctOnKeySetup(arrays),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						var res bson.D
						err := col.Database().RunCommand(ctx, bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "v"}, {Key: "query", Value: tc.filter}}).Decode(&res)
						return res.Map()["values"], err
					},
				})
			})
		}
	}
}
