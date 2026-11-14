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

// TestBitwiseArrayElements: the bitwise query operators match an array field
// when any element matches, without descending into nested arrays, and an
// empty array still has its mask validated.
func TestBitwiseArrayElements(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		var docs []interface{}
		err := bson.UnmarshalExtJSON([]byte(`{"d":[
			{"_id":1,"a":[1,2]},
			{"_id":2,"a":[2]},
			{"_id":3,"a":[[1]]},
			{"_id":4,"a":5},
			{"_id":5,"a":[{"b":1}]},
			{"_id":6,"a":[]},
			{"_id":7,"a":{"b":[1,[3]]}},
			{"_id":8,"a":[1.5,"x"]}
		]}`), false, &struct{ D *[]interface{} }{&docs})
		if err != nil {
			return err
		}
		_, err = col.InsertMany(ctx, docs)
		return err
	}
	matches := map[string]string{
		"any_set":             `{"a":{"$bitsAnySet":1}}`,
		"all_clear":           `{"a":{"$bitsAllClear":1}}`,
		"all_set":             `{"a":{"$bitsAllSet":[0]}}`,
		"any_clear":           `{"a":{"$bitsAnyClear":[0]}}`,
		"dotted":              `{"a.b":{"$bitsAnySet":1}}`,
		"dotted_nested":       `{"a.b":{"$bitsAnySet":2}}`,
		"not_any_set":         `{"a":{"$not":{"$bitsAnySet":1}}}`,
		"bad_mask":            `{"a":{"$bitsAnySet":-1}}`,
		"elem_match":          `{"a":{"$elemMatch":{"$bitsAllClear":1}}}`,
		"elem_match_bad_mask": `{"a":{"$elemMatch":{"$bitsAnySet":-1}}}`,
		"or_any_set":          `{"$or":[{"a":{"$bitsAnySet":2}},{"_id":4}]}`,
		"all_elem_match":      `{"a":{"$all":[{"$elemMatch":{"$bitsAnySet":1}}]}}`,
		"any_set_many":        `{"a":{"$bitsAnySet":3}}`,
		"all_set_binary":      `{"a":{"$bitsAllSet":{"$binary":{"base64":"Aw==","subType":"00"}}}}`,
		"any_clear_large":     `{"a":{"$bitsAnyClear":[0,1,40]}}`,
	}
	for name, match := range matches {
		var m bson.D
		if err := bson.UnmarshalExtJSON([]byte(match), false, &m); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "BitwiseArrayElements_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$match", Value: m}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}),
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "BitwiseArrayElements_find_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, m)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}

// TestBitwiseDecimalValues: the bitwise query operators match a Decimal128
// value, alone or as an array element, only when it is an integer that a
// 64-bit integer holds.
func TestBitwiseDecimalValues(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		var docs []interface{}
		err := bson.UnmarshalExtJSON([]byte(`{"d":[
			{"_id":1,"a":{"$numberDecimal":"1.5"}},
			{"_id":2,"a":[{"$numberDecimal":"1.5"}]},
			{"_id":3,"a":{"$numberDecimal":"3.0"}},
			{"_id":4,"a":[{"$numberDecimal":"-1.5"},{"$numberDecimal":"2"}]},
			{"_id":5,"a":[{"$numberDecimal":"1.0000000000000000000000000000001"}]},
			{"_id":6,"a":{"$numberDecimal":"1E+30"}},
			{"_id":7,"a":[{"$numberDecimal":"5E-1"}]},
			{"_id":8,"a":{"$numberDecimal":"-3"}}
		]}`), false, &struct{ D *[]interface{} }{&docs})
		if err != nil {
			return err
		}
		_, err = col.InsertMany(ctx, docs)
		return err
	}
	matches := map[string]string{
		"all_set":    `{"a":{"$bitsAllSet":1}}`,
		"any_set":    `{"a":{"$bitsAnySet":1}}`,
		"all_clear":  `{"a":{"$bitsAllClear":1}}`,
		"any_clear":  `{"a":{"$bitsAnyClear":1}}`,
		"not_set":    `{"a":{"$not":{"$bitsAnySet":1}}}`,
		"elem_match": `{"a":{"$elemMatch":{"$bitsAnyClear":[0]}}}`,
		"sign_bit":   `{"a":{"$bitsAllSet":[63]}}`,
	}
	for name, match := range matches {
		var m bson.D
		if err := bson.UnmarshalExtJSON([]byte(match), false, &m); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "BitwiseDecimalValues_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				ids, err := findIDs(ctx, col, m)
				if err != nil {
					return errOutcome(err), nil
				}
				del, err := col.DeleteMany(ctx, m)
				if err != nil {
					return errOutcome(err), nil
				}
				return bson.D{{Key: "find", Value: ids}, {Key: "deleted", Value: del.DeletedCount}}, nil
			},
		})
	}
}
