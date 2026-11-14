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

// TestViewListSearchIndexes: a view definition may hold $listSearchIndexes,
// which parses without mongot, with mongod's checks of its specification
// and place; reading the view then fails as the stage does.
func TestViewListSearchIndexes(t *testing.T) {
	cases := map[string]string{
		"first":             `[{"$listSearchIndexes":{}}]`,
		"name":              `[{"$listSearchIndexes":{"name":"x"}}]`,
		"name_null":         `[{"$listSearchIndexes":{"name":null}}]`,
		"not_first":         `[{"$match":{}},{"$listSearchIndexes":{}}]`,
		"not_object":        `[{"$listSearchIndexes":5}]`,
		"not_object_later":  `[{"$match":{}},{"$listSearchIndexes":5}]`,
		"unknown":           `[{"$listSearchIndexes":{"zz":1}}]`,
		"unknown_after":     `[{"$listSearchIndexes":{"name":"x","zz":1}}]`,
		"id_int":            `[{"$listSearchIndexes":{"id":5}}]`,
		"name_and_id":       `[{"$listSearchIndexes":{"id":"a","name":"b"}}]`,
		"in_facet":          `[{"$facet":{"a":[{"$listSearchIndexes":{}}]}}]`,
		"in_lookup":         `[{"$lookup":{"from":"c","as":"r","pipeline":[{"$listSearchIndexes":{}}]}}]`,
		"in_lookup_later":   `[{"$lookup":{"from":"c","as":"r","pipeline":[{"$match":{}},{"$listSearchIndexes":{}}]}}]`,
		"in_lookup_bad":     `[{"$lookup":{"from":"c","as":"r","pipeline":[{"$listSearchIndexes":{"name":1}}]}}]`,
		"lookup_in_facet":   `[{"$facet":{"a":[{"$lookup":{"from":"c","as":"r","pipeline":[{"$listSearchIndexes":{}}]}}]}}]`,
		"in_union":          `[{"$unionWith":{"coll":"c","pipeline":[{"$listSearchIndexes":{}}]}}]`,
		"then_out":          `[{"$listSearchIndexes":{}},{"$out":"x"}]`,
		"then_search_stage": `[{"$listSearchIndexes":{}},{"$search":{}}]`,
	}
	for name, pipeline := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ViewListSearchIndexes_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				view := "v_" + col.Name()
				out := bson.M{"create": errOutcome(col.Database().CreateView(ctx, view, col.Name(), p))}
				if out["create"].(bson.M)["ok"] == true {
					cursor, err := col.Database().Collection(view).Aggregate(ctx, bson.A{})
					out["aggregate"], _ = drainCursor(ctx, cursor, err)
					_, err = col.Database().Collection(view).CountDocuments(ctx, bson.D{})
					out["count"] = errOutcome(err)
				}
				return out, nil
			},
		})
	}
}
