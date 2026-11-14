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

// TestViewSearchNesting: a view may hold Atlas Search stages at any depth of
// $lookup and $unionWith sub-pipelines. Once the whole pipeline parses, a
// search stage must be first in its pipeline (40602), may not be in a $facet
// (40600, naming the outer stage when a sub-pipeline holds one), and a
// $vectorSearch may not be under a $lookup (51047). A search stage's
// specification must be an object.
func TestViewSearchNesting(t *testing.T) {
	cases := map[string]string{
		"union_union_search":       `[{"$unionWith":{"coll":"c","pipeline":[{"$unionWith":{"coll":"c","pipeline":[{"$search":{}}]}}]}}]`,
		"lookup_lookup_search":     `[{"$lookup":{"from":"c","pipeline":[{"$lookup":{"from":"c","pipeline":[{"$search":{}}],"as":"k"}}],"as":"j"}}]`,
		"union_search":             `[{"$unionWith":{"coll":"c","pipeline":[{"$search":{}}]}}]`,
		"union_search_meta":        `[{"$unionWith":{"coll":"c","pipeline":[{"$searchMeta":{}}]}}]`,
		"union_vector_search":      `[{"$unionWith":{"coll":"c","pipeline":[{"$vectorSearch":{}}]}}]`,
		"union_search_not_first":   `[{"$unionWith":{"coll":"c","pipeline":[{"$match":{}},{"$search":{}}]}}]`,
		"facet_search":             `[{"$facet":{"a":[{"$search":{}}]}}]`,
		"facet_union_search":       `[{"$facet":{"a":[{"$unionWith":{"coll":"c","pipeline":[{"$search":{}}]}}]}}]`,
		"facet_lookup_plain":       `[{"$facet":{"a":[{"$lookup":{"from":"c","pipeline":[],"as":"k"}}]}}]`,
		"union_facet_search":       `[{"$unionWith":{"coll":"c","pipeline":[{"$facet":{"a":[{"$search":{}}]}}]}}]`,
		"union_lookup_vector":      `[{"$unionWith":{"coll":"c","pipeline":[{"$lookup":{"from":"c","pipeline":[{"$vectorSearch":{}}],"as":"k"}}]}}]`,
		"lookup_union_vector":      `[{"$lookup":{"from":"c","pipeline":[{"$unionWith":{"coll":"c","pipeline":[{"$vectorSearch":{}}]}}],"as":"k"}}]`,
		"lookup_facet_vector":      `[{"$lookup":{"from":"c","pipeline":[{"$facet":{"a":[{"$vectorSearch":{}}]}}],"as":"k"}}]`,
		"union_search_spec_bad":    `[{"$unionWith":{"coll":"c","pipeline":[{"$search":5}]}}]`,
		"search_spec_bad":          `[{"$search":5}]`,
		"union_search_then_limit":  `[{"$unionWith":{"coll":"c","pipeline":[{"$search":{}},{"$limit":-1}]}}]`,
		"search_not_first_and_out": `[{"$match":{}},{"$search":{}},{"$out":"z"}]`,
	}
	for name, pipeline := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ViewSearchNesting_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				view := col.Name() + "_v"
				_ = col.Database().Collection(view).Drop(ctx)
				return runCommandDoc(ctx, col, bson.D{{Key: "create", Value: view}, {Key: "viewOn", Value: col.Name()}, {Key: "pipeline", Value: p}})
			},
		})
	}
}
