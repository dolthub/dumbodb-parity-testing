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

// TestUpdatePipelineStageErrors: a stage an update pipeline does not allow is
// parsed first, so its own error comes before 72; lite parse errors fail the
// whole command.
func TestUpdatePipelineStageErrors(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
		return err
	}
	cases := map[string]string{
		"search_not_first":       `[{"$set":{"z":1}},{"$search":{}}]`,
		"search_first":           `[{"$search":{}}]`,
		"list_local_sessions":    `[{"$listLocalSessions":{}}]`,
		"documents":              `[{"$documents":[]}]`,
		"local_sessions_unknown": `[{"$listLocalSessions":{"zz":1}}]`,
		"local_sessions_bool":    `[{"$listLocalSessions":{"allUsers":1}}]`,
		"set_variable_empty":     `[{"$setVariableFromSubPipeline":{}}]`,
		"set_variable_valid":     `[{"$setVariableFromSubPipeline":{"setVariable":"$$SEARCH_META","pipeline":[]}}]`,
		"split_large_event":      `[{"$changeStreamSplitLargeEvent":{}}]`,
		"coll_stats_unknown":     `[{"$collStats":{"zz":1}}]`,
		"coll_stats_not_object":  `[{"$collStats":5}]`,
		"coll_stats_valid":       `[{"$collStats":{}}]`,
		"group_then_bogus":       `[{"$group":{"_id":1}},{"$bogus":1}]`,
	}
	for name, pipeline := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "UpdatePipelineStageErrors_update_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return runCommandDoc(ctx, col, bson.D{{Key: "update", Value: col.Name()},
					{Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "u", Value: p}}}}})
			},
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "UpdatePipelineStageErrors_find_and_modify_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				res := col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: 1}}, p, options.FindOneAndUpdate())
				var doc bson.M
				if err := res.Decode(&doc); err != nil {
					return errOutcome(err), nil
				}
				return doc, nil
			},
		})
	}
}
