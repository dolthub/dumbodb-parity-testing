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
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestExplain_ListClusterCatalog: mongod explains {aggregate: 1} led by
// $listClusterCatalog as the stages it expands into, $_internalListCollections
// first, with a leading $match moved back past the stages that do not set
// its fields, then the later stages, each with the documents it returned at
// executionStats verbosity; a $limit through stages that keep one output per
// input stops the listing early.
func TestExplain_ListClusterCatalog(t *testing.T) {
	lcc := func(spec bson.D, later ...bson.D) bson.A {
		pipeline := bson.A{bson.D{{Key: "$listClusterCatalog", Value: spec}}}
		for _, s := range later {
			pipeline = append(pipeline, s)
		}
		return pipeline
	}
	cases := []struct {
		name      string
		pipeline  bson.A
		verbosity string
	}{
		{"plain", lcc(bson.D{}), "executionStats"},
		{"query_planner", lcc(bson.D{}), "queryPlanner"},
		{"shards_balancing", lcc(bson.D{{Key: "shards", Value: true}, {Key: "balancingConfiguration", Value: true}}), "executionStats"},
		{"match_type_limit", lcc(bson.D{}, bson.D{{Key: "$match", Value: bson.D{{Key: "type", Value: "collection"}}}},
			bson.D{{Key: "$limit", Value: 2}}), "executionStats"},
		{"match_sharded", lcc(bson.D{}, bson.D{{Key: "$match", Value: bson.D{{Key: "sharded", Value: false}}}}), "executionStats"},
		{"limit", lcc(bson.D{}, bson.D{{Key: "$limit", Value: 2}}), "executionStats"},
		{"count", lcc(bson.D{}, bson.D{{Key: "$count", Value: "n"}}), "executionStats"},
		{"sort_limit", lcc(bson.D{}, bson.D{{Key: "$sort", Value: bson.D{{Key: "ns", Value: 1}}}}, bson.D{{Key: "$limit", Value: 1}}), "executionStats"},
		{"unknown_field", lcc(bson.D{{Key: "bogus", Value: true}}), "executionStats"},
		{"bad_match", lcc(bson.D{}, bson.D{{Key: "$match", Value: bson.D{{Key: "$bogus", Value: 1}}}}), "executionStats"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Explain_ListClusterCatalog_" + tc.name,
				Support: harness.DumboDBFull,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					db := col.Database().Client().Database(fmt.Sprintf("lcc_explain_%d", time.Now().UnixNano()))
					defer db.Drop(ctx)
					for _, name := range []string{"x", "y", "z"} {
						if err := db.CreateCollection(ctx, name); err != nil {
							return nil, err
						}
					}
					cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "aggregate", Value: 1}, {Key: "pipeline", Value: tc.pipeline},
						{Key: "cursor", Value: bson.D{}}}}, {Key: "verbosity", Value: tc.verbosity}}
					var doc bson.D
					if err := db.RunCommand(ctx, cmd).Decode(&doc); err != nil {
						return errOutcome(err), nil
					}
					ver, _ := lookup(doc, "explainVersion")
					return bson.D{{Key: "explainVersion", Value: ver}, {Key: "stages", Value: topLevelStages(doc)}}, nil
				},
			})
		})
	}
}
