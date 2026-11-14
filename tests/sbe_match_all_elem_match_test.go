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

// TestSBEMatchAllElemMatch: a bitwise operator inside an $elemMatch, also
// one that is an element of $all, makes mongod run a following $group in
// its classic engine, which reports a different code for $firstN's n and
// evaluates n only when a document reaches the $group.
func TestSBEMatchAllElemMatch(t *testing.T) {
	shapes := map[string]bson.A{
		"missing": {bson.D{{Key: "_id", Value: int32(1)}}},
		"scalar":  {bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(5)}}},
		"array": {bson.D{
			{Key: "_id", Value: int32(1)},
			{Key: "a", Value: bson.A{int32(1), int32(2)}},
		}},
		"empty": {},
	}
	matches := map[string]string{
		"all_elem_match_bits":     `{"a":{"$all":[{"$elemMatch":{"$bitsAnySet":1}}]}}`,
		"all_plain":               `{"a":{"$all":[1,2]}}`,
		"elem_match_bits":         `{"a":{"$elemMatch":{"$bitsAnySet":1}}}`,
		"all_elem_match_gt":       `{"a":{"$all":[{"$elemMatch":{"$gt":0}}]}}`,
		"all_second_elem_bits":    `{"a":{"$all":[{"$elemMatch":{"$gt":0}},{"$elemMatch":{"$bitsAllClear":4}}]}}`,
		"all_elem_match_not_bits": `{"a":{"$all":[{"$elemMatch":{"$not":{"$bitsAnySet":4}}}]}}`,
		"not_all_elem_match_bits": `{"a":{"$not":{"$all":[{"$elemMatch":{"$bitsAnySet":4}}]}}}`,
		"or_all_elem_match_bits":  `{"$or":[{"a":{"$all":[{"$elemMatch":{"$bitsAnySet":1}}]}},{"b":1}]}`,
		"elem_match_object_all":   `{"a":{"$elemMatch":{"b":{"$all":[{"$elemMatch":{"$bitsAnySet":1}}]}}}}`,
	}
	const group = `{"$group":{"_id":null,"f":{"$firstN":{"input":"$y","n":0}}}}`
	for shapeName, docs := range shapes {
		docs := docs
		setup := func(ctx context.Context, col *mongo.Collection) error {
			if len(docs) == 0 {
				return col.Database().CreateCollection(ctx, col.Name())
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		}
		for matchName, match := range matches {
			var p bson.A
			if err := bson.UnmarshalExtJSON([]byte(`[{"$match":`+match+`},`+group+`]`), false, &p); err != nil {
				t.Fatal(err)
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "SBEMatchAllElemMatch_" + shapeName + "_" + matchName,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run:     pipelineRun(p),
			})
		}
	}
}
