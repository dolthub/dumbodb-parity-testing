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

// count, distinct and delete ignored maxTimeMS, or rejected it as an unknown
// field.
func TestMaxTimeMSCommands(t *testing.T) {
	commands := map[string]func(coll string) bson.D{
		"Count": func(coll string) bson.D {
			return bson.D{{Key: "count", Value: coll}, {Key: "query", Value: bson.D{{Key: "a", Value: bson.D{{Key: "$regex", Value: "^x"}}}}}}
		},
		"Distinct": func(coll string) bson.D {
			return bson.D{{Key: "distinct", Value: coll}, {Key: "key", Value: "a"}}
		},
		"Delete": func(coll string) bson.D {
			return bson.D{{Key: "delete", Value: coll}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: "x1"}}}, {Key: "limit", Value: int32(0)}}}}}
		},
		"Update": func(coll string) bson.D {
			return bson.D{{Key: "update", Value: coll}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "a", Value: "x1"}}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: int32(1)}}}}}}}}}
		},
		"FindAndModify": func(coll string) bson.D {
			return bson.D{{Key: "findAndModify", Value: coll}, {Key: "query", Value: bson.D{{Key: "a", Value: "x1"}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "b", Value: int32(1)}}}}}}
		},
	}
	values := map[string]interface{}{
		"Int":      int32(60000),
		"Zero":     int32(0),
		"Long":     int64(60000),
		"Double":   60000.0,
		"Fraction": 1.5,
		"Negative": int32(-1),
		"String":   "x",
		"Huge":     int64(1) << 40,
	}
	for cmdName, c := range commands {
		build := c
		for valName, v := range values {
			value := v
			name := "MaxTimeMSCommands_" + cmdName + "_" + valName
			t.Run(cmdName+"_"+valName, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name,
					Support: harness.DumboDBFull,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						_, err := col.InsertMany(ctx, []interface{}{
							bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: "x1"}},
							bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: "y"}},
						})
						return err
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						cmd := append(build(col.Name()), bson.E{Key: "maxTimeMS", Value: value})
						var res bson.M
						err := col.Database().RunCommand(ctx, cmd).Decode(&res)
						if err != nil {
							return errOutcome(err), nil
						}
						delete(res, "$clusterTime")
						delete(res, "operationTime")
						delete(res, "lastErrorObject")
						return res, nil
					},
				})
			})
		}
	}
}
