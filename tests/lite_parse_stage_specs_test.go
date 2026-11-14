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

// TestLiteParseStageSpecs: mongod's lite parse checks the specifications of
// $collStats and $listLocalSessions, so their errors come before a later
// unknown stage's.
func TestLiteParseStageSpecs(t *testing.T) {
	const bogus = `,{"$bogus":1}`
	cases := map[string]struct{ db, pipeline string }{
		"coll_stats_unknown":         {"", `[{"$collStats":{"zz":1}}` + bogus + `]`},
		"coll_stats_count_type":      {"", `[{"$collStats":{"count":1}}` + bogus + `]`},
		"coll_stats_not_object":      {"", `[{"$collStats":5}` + bogus + `]`},
		"coll_stats_empty_first":     {"", `[{"$collStats":{"count":{"a":1},"zz":1}}]`},
		"coll_stats_unknown_alone":   {"", `[{"$collStats":{"zz":1}}]`},
		"local_sessions_bool":        {"admin", `[{"$listLocalSessions":{"allUsers":1}}` + bogus + `]`},
		"local_sessions_unknown":     {"admin", `[{"$listLocalSessions":{"zz":1}}` + bogus + `]`},
		"local_sessions_not_object":  {"", `[{"$listLocalSessions":5}]`},
		"local_sessions_users_type":  {"admin", `[{"$listLocalSessions":{"users":5}}]`},
		"local_sessions_user_type":   {"admin", `[{"$listLocalSessions":{"users":[5]}}]`},
		"local_sessions_missing_db":  {"admin", `[{"$listLocalSessions":{"users":[{"user":"a"}]}}]`},
		"local_sessions_coll_unknwn": {"", `[{"$listLocalSessions":{"zz":1}}` + bogus + `]`},
	}
	for name, tc := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(tc.pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		db := tc.db
		harness.PairTest(t, harness.TestCase{
			Name:    "LiteParseStageSpecs_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if db == "" {
					cursor, err := col.Aggregate(ctx, p)
					return drainCursor(ctx, cursor, err)
				}
				cursor, err := col.Database().Client().Database(db).Aggregate(ctx, p)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
