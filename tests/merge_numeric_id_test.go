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

// TestMergeNumericID: $merge matches an 'on' value equal by value across
// numeric types, inside documents too, but not a string of the same digits.
// replace writes the incoming _id's type, merge refuses a different numeric
// type with 66, and a pipeline leaves the stored _id alone.
func TestMergeNumericID(t *testing.T) {
	cases := map[string]string{
		"ReplaceDecimal":      `[{"$documents":[{"_id":{"$numberDecimal":"1"},"z":1},{"_id":{"$numberDecimal":"2.0"},"z":2}]},{"$merge":{"into":"w","whenMatched":"replace"}}]`,
		"ReplaceDouble":       `[{"$documents":[{"_id":1.0,"z":1},{"_id":{"$numberDecimal":"0.50"},"z":5}]},{"$merge":{"into":"w","whenMatched":"replace"}}]`,
		"ReplaceNestedDouble": `[{"$documents":[{"_id":{"a":1.0},"z":4}]},{"$merge":{"into":"w","whenMatched":"replace"}}]`,
		"ReplaceStringDigits": `[{"$documents":[{"_id":"2","z":6}]},{"$merge":{"into":"w","whenMatched":"replace"}}]`,
		"MergeDecimal":        `[{"$documents":[{"_id":{"$numberDecimal":"1"},"m":1}]},{"$merge":{"into":"w"}}]`,
		"MergeSameType":       `[{"$documents":[{"_id":{"$numberLong":"2"},"m":2}]},{"$merge":{"into":"w"}}]`,
		"KeepExistingDouble":  `[{"$documents":[{"_id":1.0,"k":1}]},{"$merge":{"into":"w","whenMatched":"keepExisting"}}]`,
		"FailDouble":          `[{"$documents":[{"_id":1.0,"k":1}]},{"$merge":{"into":"w","whenMatched":"fail"}}]`,
		"PipelineLong":        `[{"$documents":[{"_id":{"$numberLong":"1"},"p":1}]},{"$merge":{"into":"w","whenMatched":[{"$set":{"p":"$$new.p"}}]}}]`,
		"DiscardDecimal":      `[{"$documents":[{"_id":{"$numberDecimal":"1.000"},"d":1},{"_id":9,"d":9}]},{"$merge":{"into":"w","whenMatched":"replace","whenNotMatched":"discard"}}]`,
	}
	for name, js := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeNumericID_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.Database().Collection("w").InsertMany(ctx, []interface{}{
					bson.D{{Key: "_id", Value: int32(1)}, {Key: "y", Value: "a"}},
					bson.D{{Key: "_id", Value: int64(2)}, {Key: "y", Value: "b"}},
					bson.D{{Key: "_id", Value: bson.D{{Key: "a", Value: int32(1)}}}, {Key: "y", Value: "c"}},
					bson.D{{Key: "_id", Value: 0.5}, {Key: "y", Value: "d"}},
				})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().Aggregate(ctx, pipeline)
				res, _ := drainCursor(ctx, cursor, err)
				cur, err := col.Database().Collection("w").Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "y", Value: 1}, {Key: "z", Value: 1}, {Key: "d", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var raw []bson.Raw
				if err = cur.All(ctx, &raw); err != nil {
					return nil, err
				}
				// Documents compare as maps, so the _id's BSON type is kept
				// beside each one.
				docs := make([]bson.M, len(raw))
				for i, r := range raw {
					var m bson.M
					if err = bson.Unmarshal(r, &m); err != nil {
						return nil, err
					}
					m["idType"] = r.Lookup("_id").Type.String()
					docs[i] = m
				}
				return bson.M{"result": res, "target": docs}, nil
			},
		})
	}
}
