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

// TestOutMergeFinalStage: $out and $merge before the last stage fail with
// 40601 once every stage parses, and write nothing.
func TestOutMergeFinalStage(t *testing.T) {
	cases := map[string]string{
		"OutThenMatch":      `[{"$out":"o"},{"$match":{}}]`,
		"OutThenSkip":       `[{"$out":"o"},{"$skip":1}]`,
		"OutThenLimit":      `[{"$out":"o"},{"$limit":1}]`,
		"MergeThenMatch":    `[{"$merge":"o"},{"$match":{}}]`,
		"MergePipelineThen": `[{"$merge":{"into":"o","whenMatched":[{"$set":{"a":1}}]}},{"$project":{"a":1}}]`,
		"OutThenOut":        `[{"$out":"o"},{"$out":"o"}]`,
		"OutThenBadMatch":   `[{"$out":"o"},{"$match":{"$bad":1}}]`,
		"OutThenBadMerge":   `[{"$out":"o"},{"$match":{}},{"$merge":{"into":"o","bogus":1}}]`,
		"MergeBadThenMatch": `[{"$merge":{"into":"o","bogus":1}},{"$match":{}}]`,
		"MatchOutMatch":     `[{"$match":{}},{"$out":"o"},{"$match":{}}]`,
	}
	for name, js := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "OutMergeFinalStage_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Aggregate(ctx, pipeline)
				res, _ := drainCursor(ctx, cursor, err)
				names, err := col.Database().ListCollectionNames(ctx, bson.D{{Key: "name", Value: "o"}})
				if err != nil {
					return nil, err
				}
				return bson.M{"result": res, "written": len(names)}, nil
			},
		})
	}
}
