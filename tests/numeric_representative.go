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

// numericRepresentativeTest covers results in which numerically equal values
// of different BSON types (NumberInt(1), NumberLong(1), 1.0) collapse to one
// value, such as a $group _id or a distinct value. Which type survives is
// the first one the scan meets. mongod scans in natural order, which its
// documentation calls an internal implementation feature not to rely on;
// DumboDB scans in _id-hash order to stay history independent. So mongod
// must return mongoWant exactly, and DumboDB the same values with any
// numeric types (workspace-77m, workspace-3so).
func numericRepresentativeTest(
	t *testing.T,
	name string,
	setup func(context.Context, *mongo.Collection) error,
	run func(context.Context, *mongo.Collection) (interface{}, error),
	mongoWant interface{},
) {
	t.Helper()

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    name,
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := tgt.Admin.Database(tgt.NS)
			defer func() { _ = db.Drop(ctx) }()
			col := db.Collection("c")
			if setup != nil {
				tgt.Setup(setup(ctx, col))
			}
			return run(ctx, col)
		},
		MongoExpect: func(t *testing.T, res interface{}, err error) {
			if cmp := harness.CompareResponses(mongoWant, nil, res, err); cmp.Result != harness.Match {
				t.Errorf("mongo: %s", cmp.Diff)
			}
		},
		DumboExpect: func(t *testing.T, res interface{}, err error) {
			cmp := harness.CompareResponses(anyNumericType(mongoWant), nil, anyNumericType(res), err)
			if cmp.Result != harness.Match {
				t.Errorf("dumbodb: %s", cmp.Diff)
			}
		},
	})
}

// anyNumericType converts every int32 and int64 in v to a double, so that
// values compare equal whatever numeric type they were returned as.
func anyNumericType(v interface{}) interface{} {
	switch v := v.(type) {
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case bson.D:
		out := make(bson.D, len(v))
		for i, e := range v {
			out[i] = bson.E{Key: e.Key, Value: anyNumericType(e.Value)}
		}
		return out
	case bson.A:
		out := make(bson.A, len(v))
		for i, e := range v {
			out[i] = anyNumericType(e)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(v))
		for i, e := range v {
			out[i] = anyNumericType(e)
		}
		return out
	default:
		return v
	}
}
