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

// TestLiteParseOrder: mongod checks every stage name, in sub-pipelines too,
// before it builds any stage, and checks only the specifications of the
// stages with a lite parser in that pass, so an unknown later stage fails
// first. On {aggregate: 1}, each stage parses, then the first stage must run
// without a collection, then the stages that must come first are checked.
func TestLiteParseOrder(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
		return err
	}
	const bogus = `{"$bogus":1}`
	collection := map[string]string{
		"change_stream":         `[{"$changeStream":{}},` + bogus + `]`,
		"search":                `[{"$search":{}},` + bogus + `]`,
		"facet_nested":          `[{"$facet":{"a":[` + bogus + `,{"$changeStream":{}}]}}]`,
		"list_local_sessions":   `[{"$listLocalSessions":{}},` + bogus + `]`,
		"set_variable":          `[{"$setVariableFromSubPipeline":{}},` + bogus + `]`,
		"split_large_event":     `[{"$changeStreamSplitLargeEvent":{}},` + bogus + `]`,
		"split_large_event_bad": `[{"$changeStreamSplitLargeEvent":5},` + bogus + `]`,
		"query_stats":           `[{"$queryStats":{}},` + bogus + `]`,
		"lookup_out":            `[{"$lookup":{"from":"c","pipeline":[{"$out":"x"},` + bogus + `],"as":"j"}}]`,
		"group_bad":             `[{"$group":5},` + bogus + `]`,
		"limit_bad":             `[{"$limit":-1},` + bogus + `]`,
		"densify_bad":           `[{"$densify":{"field":5}},` + bogus + `]`,
		"documents":             `[{"$documents":[]},` + bogus + `]`,
		"facet_out":             `[{"$facet":{"a":[{"$out":"x"},` + bogus + `]}}]`,
		"inhibit_bad":           `[{"$_internalInhibitOptimization":5},` + bogus + `]`,
		"project_bad":           `[{"$project":5},` + bogus + `]`,
		"out_bad":               `[{"$out":5},` + bogus + `]`,
		"out_db_bad":            `[{"$out":{"db":5}},` + bogus + `]`,
		"merge_bad":             `[{"$merge":5},` + bogus + `]`,
		"merge_into_bad":        `[{"$merge":{"into":5}},` + bogus + `]`,
		"facet_bad":             `[{"$facet":5},` + bogus + `]`,
		"facet_empty":           `[{"$facet":{}},` + bogus + `]`,
		"facet_field_bad":       `[{"$facet":{"a":5}},` + bogus + `]`,
		"lookup_bad":            `[{"$lookup":5},` + bogus + `]`,
		"lookup_from_bad":       `[{"$lookup":{"from":5,"as":"j","pipeline":[]}},` + bogus + `]`,
		"lookup_pipeline_bad":   `[{"$lookup":{"from":"c","as":"j","pipeline":5}},` + bogus + `]`,
		"union_bad":             `[{"$unionWith":5},` + bogus + `]`,
		"union_coll_bad":        `[{"$unionWith":{"coll":5}},` + bogus + `]`,
		"union_pipeline_bad":    `[{"$unionWith":{"coll":"c","pipeline":5}},` + bogus + `]`,
		"change_stream_bad":     `[{"$changeStream":5},` + bogus + `]`,
	}
	for name, pipeline := range collection {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "LiteParseOrder_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(p),
		})
	}
	database := map[string]struct{ db, pipeline string }{
		"admin_match_then_query_settings": {"admin", `[{"$match":{}},{"$querySettings":{}}]`},
		"test_documents_query_settings":   {"test", `[{"$documents":[]},{"$querySettings":{}}]`},
		"test_match_then_query_stats":     {"test", `[{"$match":{}},{"$queryStats":{}}]`},
		"admin_group_bad":                 {"admin", `[{"$group":5}]`},
		"admin_change_stream_bogus":       {"admin", `[{"$changeStream":{}},` + bogus + `]`},
	}
	for name, tc := range database {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(tc.pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		db := tc.db
		harness.PairTest(t, harness.TestCase{
			Name:    "LiteParseOrder_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().Client().Database(db).Aggregate(ctx, p)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
