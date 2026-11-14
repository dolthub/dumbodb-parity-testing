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

// TestFacetQueryStatsOrder: on {aggregate: 1} against admin, a $facet's
// branches parse in order before a $queryStats in one of them is refused.
func TestFacetQueryStatsOrder(t *testing.T) {
	cases := map[string]string{
		"query_stats_first":    `[{"$documents":[{}]},{"$facet":{"a":[{"$queryStats":{}}],"b":[{"$limit":-1}]}}]`,
		"earlier_branch_bad":   `[{"$documents":[{}]},{"$facet":{"b":[{"$limit":-1}],"a":[{"$queryStats":{}}]}}]`,
		"earlier_stage_bad":    `[{"$documents":[{}]},{"$facet":{"a":[{"$limit":-1},{"$queryStats":{}}]}}]`,
		"earlier_group_bad":    `[{"$documents":[{}]},{"$facet":{"a":[{"$group":{"_id":1,"x":5}}],"b":[{"$queryStats":{}}]}}]`,
		"earlier_lookup_bad":   `[{"$documents":[{}]},{"$facet":{"a":[{"$lookup":{"from":"c","as":5}}],"b":[{"$queryStats":{}}]}}]`,
		"query_stats_spec_bad": `[{"$documents":[{}]},{"$facet":{"a":[{"$queryStats":5},{"$limit":-1}]}}]`,
		"earlier_branch_ok":    `[{"$documents":[{}]},{"$facet":{"b":[{"$count":"n"}],"a":[{"$queryStats":{}}]}}]`,
	}
	for name, pipeline := range cases {
		var p bson.A
		if err := bson.UnmarshalExtJSON([]byte(pipeline), false, &p); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "FacetQueryStatsOrder_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Database().Client().Database("admin").Aggregate(ctx, p)
				return drainCursor(ctx, cursor, err)
			},
		})
	}
}
