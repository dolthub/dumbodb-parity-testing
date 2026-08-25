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

// Query-plan parity for collated indexes: a collated query is index-served
// (IXSCAN, O(1) docs examined) and a binary query is not, matching MongoDB.

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

// setupCollatedRowCount is large so a lost-index regression examines ~this many
// docs and fails clearly, rather than squeaking under a small threshold.
const setupCollatedRowCount = 2000

// setupCollatedEmailIndex seeds setupCollatedRowCount rows plus
// Alice@example.com under a unique en/strength-2 index on email.
func setupCollatedEmailIndex(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetCollation(&options.Collation{Locale: "en", Strength: 2}),
	}); err != nil {
		return err
	}
	docs := make([]interface{}, 0, setupCollatedRowCount+1)
	for i := 0; i < setupCollatedRowCount; i++ {
		docs = append(docs, bson.D{{Key: "email", Value: fmt.Sprintf("User%d@example.com", i)}})
	}
	docs = append(docs, bson.D{{Key: "email", Value: "Alice@example.com"}})
	_, err := col.InsertMany(ctx, docs)
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

func TestCollation_Plan_CollatedQuery_UsesIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Plan_CollatedQuery_UsesIndex",
		Support: harness.DumboDBFull,
		Setup:   setupCollatedEmailIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			r := explainFind(ctx, col, bson.D{{Key: "email", Value: "alice@example.com"}}, en2, "queryPlanner")
			return bson.D{{Key: "stage", Value: winningStage(r)}}, nil
		},
	})
}

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

func TestCollation_Plan_CollatedQuery_IndexServed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_Plan_CollatedQuery_IndexServed",
		Support: harness.DumboDBFull,
		Setup:   setupCollatedEmailIndex,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			r := explainFind(ctx, col, bson.D{{Key: "email", Value: "alice@example.com"}}, en2, "executionStats")
			return bson.D{{Key: "totalDocsExamined", Value: docsExamined(r)}}, nil
		},
	})
}
