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

// TestDatabaseSourceStageLaterFirstOnly: after the source stage of an
// {aggregate: 1} pipeline, $collStats and $listLocalSessions are refused as
// stages that must be first, once their specifications parse.
func TestDatabaseSourceStageLaterFirstOnly(t *testing.T) {
	cases := map[string]string{
		"DocumentsCollStats":           `[{"$documents":[{"a":1}]},{"$collStats":{}}]`,
		"DocumentsCollStatsBadSpec":    `[{"$documents":[{"a":1}]},{"$collStats":{"bogus":1}}]`,
		"DocumentsLocalSessions":       `[{"$documents":[{"a":1}]},{"$listLocalSessions":{}}]`,
		"DocumentsLocalSessionsBad":    `[{"$documents":[{"a":1}]},{"$listLocalSessions":1}]`,
		"CatalogCollStats":             `[{"$listClusterCatalog":{}},{"$collStats":{}}]`,
		"CatalogLocalSessions":         `[{"$listClusterCatalog":{}},{"$listLocalSessions":{}}]`,
		"DocumentsCollStatsThenDocs":   `[{"$documents":[]},{"$collStats":{}},{"$documents":[]}]`,
		"DocumentsCollStatsAfterMatch": `[{"$documents":[{"a":1}]},{"$match":{}},{"$collStats":{}}]`,
	}
	for name, js := range cases {
		var pipeline bson.A
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *bson.A }{&pipeline}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "DatabaseSourceStageLaterFirstOnly_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().Aggregate(ctx, pipeline)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
