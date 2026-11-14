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

// TestBitwisePositionalPath: a bitwise query operator on a path ending in an
// array index tests a nested array found there as a whole, so it does not
// match, while an array reached by a field name is tested element by
// element. Checked through find, updateMany and deleteMany, and under $not
// and $nor.
func TestBitwisePositionalPath(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		var docs []interface{}
		err := bson.UnmarshalExtJSON([]byte(`{"d":[
			{"_id":1,"a":[[1],[3]]},
			{"_id":2,"a":[[7],7]},
			{"_id":3,"a":{"b":[[1]]}},
			{"_id":4,"a":{"0":[1]}},
			{"_id":5,"a":[{"b":[[1]]}]},
			{"_id":7,"a":[[[1]]]},
			{"_id":8,"a":[{"b":[1]}]},
			{"_id":9,"a":[1]},
			{"_id":10,"a":[2,[1]]},
			{"_id":12,"a":{"b":{"0":[3]}}}
		]}`), false, &struct{ D *[]interface{} }{&docs})
		if err != nil {
			return err
		}
		_, err = col.InsertMany(ctx, docs)
		return err
	}
	var filters []string
	for _, op := range []string{
		`{"$bitsAllSet":1}`, `{"$bitsAnySet":[0]}`, `{"$bitsAllClear":2}`, `{"$bitsAnyClear":[1]}`,
	} {
		for _, path := range []string{"a.0", "a.0.0", "a.b.0", "a.1"} {
			filters = append(filters,
				`{"`+path+`":`+op+`}`,
				`{"`+path+`":{"$not":`+op+`}}`,
				`{"$nor":[{"`+path+`":`+op+`}]}`,
			)
		}
	}
	filters = append(filters, `{"a.0":{"$bitsAllSet":1,"$exists":true}}`, `{"a.b":{"$bitsAnySet":1}}`)
	for _, f := range filters {
		var filter bson.D
		if err := bson.UnmarshalExtJSON([]byte(f), false, &filter); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "BitwisePositionalPath_" + f,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				ids, err := findIDs(ctx, col, filter)
				if err != nil {
					return errOutcome(err), nil
				}
				upd, err := col.UpdateMany(ctx, filter, bson.D{{Key: "$set", Value: bson.D{{Key: "u", Value: 1}}}})
				if err != nil {
					return errOutcome(err), nil
				}
				del, err := col.DeleteMany(ctx, filter)
				if err != nil {
					return errOutcome(err), nil
				}
				cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
				left, err := drainCursor(ctx, cur, err)
				if err != nil {
					return nil, err
				}
				return bson.D{
					{Key: "find", Value: ids},
					{Key: "updated", Value: upd.ModifiedCount},
					{Key: "deleted", Value: del.DeletedCount},
					{Key: "left", Value: left},
				}, nil
			},
		})
	}
}
