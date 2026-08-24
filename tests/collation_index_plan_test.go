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

// Collated-index query-plan parity (workspace-33x / nah / u6x). These assert
// what the order/equality suites never did: that a collated index is actually
// USED (index-served, O(1) docs examined), and that the planner picks the same
// stage MongoDB does. They XFail until collation-ordered index keys land; the
// point is they now fail HONESTLY -- before the explain fix (u6x) they would
// have false-passed on a synthesized IXSCAN.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// setupCollatedEmailIndex creates a unique en/strength-2 index on email (no
// collection default) and seeds 50 rows plus Alice@example.com.
func setupCollatedEmailIndex(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetCollation(&options.Collation{Locale: "en", Strength: 2}),
	}); err != nil {
		return err
	}
	for i := 0; i < 50; i++ {
		if _, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: fmt.Sprintf("User%d@example.com", i)}}); err != nil {
			return err
		}
	}
	_, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: "Alice@example.com"}})
	return err
}

func explainFind(ctx context.Context, col *mongo.Collection, filter bson.D, coll bson.D, verbosity string) bson.M {
	inner := bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}}
	if coll != nil {
		inner = append(inner, bson.E{Key: "collation", Value: coll})
	}
	var r bson.M
	_ = col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: inner}, {Key: "verbosity", Value: verbosity}}).Decode(&r)
	return r
}

func winningStage(r bson.M) string {
	b, _ := json.Marshal(r["queryPlanner"])
	s := string(b)
	if strings.Contains(s, "IXSCAN") {
		return "IXSCAN"
	}
	if strings.Contains(s, "COLLSCAN") {
		return "COLLSCAN"
	}
	return "?"
}

func docsExamined(r bson.M) int64 {
	if es, ok := r["executionStats"].(bson.M); ok {
		switch v := es["totalDocsExamined"].(type) {
		case int32:
			return int64(v)
		case int64:
			return v
		case float64:
			return int64(v)
		}
	}
	return -1
}

var en2 = bson.D{{Key: "locale", Value: "en"}, {Key: "strength", Value: 2}}

// A collated query on a matching collated index must be served by the index
// (MongoDB: IXSCAN).
func TestCollation_Plan_CollatedQuery_UsesIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Plan_CollatedQuery_UsesIndex",
		Support: harness.DumboDBXFail,
		Setup:   setupCollatedEmailIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			r := explainFind(ctx, col, bson.D{{Key: "email", Value: "alice@example.com"}}, en2, "queryPlanner")
			return bson.D{{Key: "stage", Value: winningStage(r)}}, nil
		},
	})
}

// A simple (binary) query must NOT use a collated index (MongoDB: COLLSCAN).
func TestCollation_Plan_SimpleQuery_NotCollatedIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Plan_SimpleQuery_NotCollatedIndex",
		Support: harness.DumboDBFull,
		Setup:   setupCollatedEmailIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			r := explainFind(ctx, col, bson.D{{Key: "email", Value: "alice@example.com"}}, nil, "queryPlanner")
			return bson.D{{Key: "stage", Value: winningStage(r)}}, nil
		},
	})
}

// A collated point query must examine O(1) docs (index-served), not the whole
// collection.
func TestCollation_Plan_CollatedQuery_IndexServed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Plan_CollatedQuery_IndexServed",
		Support: harness.DumboDBXFail,
		Setup:   setupCollatedEmailIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			r := explainFind(ctx, col, bson.D{{Key: "email", Value: "alice@example.com"}}, en2, "executionStats")
			return bson.D{{Key: "index_served", Value: docsExamined(r) >= 0 && docsExamined(r) <= 5}}, nil
		},
	})
}
