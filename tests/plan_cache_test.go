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
	"regexp"
	"slices"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// The plan cache stores the winner of a multi-plan race per query shape. A
// filter on p (index p_1, a third of the documents) sorted on k (index k_1)
// with a small limit races the two indexes; walking k_1 wins. Hashes and
// timings differ between servers, so the tests compare their presence and
// the cached plan's shape.

func planCacheSetup(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 0, 3000)
	for i := 0; i < 3000; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "k", Value: int32((i * 7) % 3000)}, {Key: "p", Value: int32(i % 3)}, {Key: "q", Value: int32(i)}})
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	_, err := col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "k", Value: 1}}},
		{Keys: bson.D{{Key: "p", Value: 1}}},
	})
	return err
}

func runPlanCacheRace(ctx context.Context, col *mongo.Collection) error {
	for i := 0; i < 3; i++ {
		cur, err := col.Find(ctx, bson.D{{Key: "p", Value: int32(i % 3)}}, options.Find().SetSort(bson.D{{Key: "k", Value: 1}}).SetLimit(5))
		if err != nil {
			return err
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return err
		}
	}
	return nil
}

func planCacheStats(ctx context.Context, col *mongo.Collection) ([]bson.M, error) {
	cur, err := col.Aggregate(ctx, mongo.Pipeline{{{Key: "$planCacheStats", Value: bson.D{}}}})
	if err != nil {
		return nil, err
	}
	var out []bson.M
	err = cur.All(ctx, &out)
	return out, err
}

func planCacheClear(ctx context.Context, col *mongo.Collection, shape bson.D) (bson.M, error) {
	cmd := append(bson.D{{Key: "planCacheClear", Value: col.Name()}}, shape...)
	var res bson.M
	err := col.Database().RunCommand(ctx, cmd).Decode(&res)
	return res, err
}

var planCacheHash = regexp.MustCompile(`^[0-9A-F]{8}$`)

func isPlanCacheHash(v interface{}) bool {
	s, ok := v.(string)
	return ok && planCacheHash.MatchString(s)
}

// planStages lists a plan's stages and index names from the root down.
func planStages(plan interface{}) []string {
	var out []string
	for plan != nil {
		var get func(string) (interface{}, bool)
		switch p := plan.(type) {
		case bson.M:
			get = func(k string) (interface{}, bool) { v, ok := p[k]; return v, ok }
		case bson.D:
			get = func(k string) (interface{}, bool) { return lookupBSONField(p, k) }
		default:
			return out
		}
		stage, _ := get("stage")
		name := fmt.Sprint(stage)
		if idx, ok := get("indexName"); ok {
			name += ":" + fmt.Sprint(idx)
		}
		out = append(out, name)
		plan = nil
		for _, k := range []string{"inputStage", "outerStage"} {
			if v, ok := get(k); ok {
				plan = v
				break
			}
		}
	}
	return out
}

func sortedKeys(v interface{}) []string {
	m, _ := v.(bson.M)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func TestPlanCache_ExplainReportsHashesAndCachedWinner(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PlanCache_ExplainReportsHashesAndCachedWinner",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := planCacheSetup(ctx, col); err != nil {
				return err
			}
			return runPlanCacheRace(ctx, col)
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var res bson.M
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "explain", Value: bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: bson.D{{Key: "p", Value: int32(1)}}}, {Key: "sort", Value: bson.D{{Key: "k", Value: 1}}}, {Key: "limit", Value: 5}}},
				{Key: "verbosity", Value: "queryPlanner"},
			}).Decode(&res)
			if err != nil {
				return nil, err
			}
			qp, _ := res["queryPlanner"].(bson.M)
			winning, _ := qp["winningPlan"].(bson.M)
			return bson.M{
				"planCacheShapeHash": isPlanCacheHash(qp["planCacheShapeHash"]),
				"planCacheKey":       isPlanCacheHash(qp["planCacheKey"]),
				"queryHashIsShape":   qp["queryHash"] == qp["planCacheShapeHash"],
				"winningIsCached":    winning["isCached"],
				"winningStages":      planStages(winning),
			}, nil
		},
	})
}

func TestPlanCache_StatsListTheCachedWinner(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PlanCache_StatsListTheCachedWinner",
		Support: harness.DumboDBFull,
		Setup:   planCacheSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			before, err := planCacheStats(ctx, col)
			if err != nil {
				return nil, err
			}
			if err := runPlanCacheRace(ctx, col); err != nil {
				return nil, err
			}
			after, err := planCacheStats(ctx, col)
			if err != nil {
				return nil, err
			}
			entries := bson.A{}
			for _, e := range after {
				works, _ := e["works"].(int64)
				entries = append(entries, bson.M{
					"createdFromQuery":   sortedKeys(e["createdFromQuery"]),
					"isActive":           e["isActive"],
					"cachedPlan":         planStages(e["cachedPlan"]),
					"worksPositive":      works > 0,
					"planCacheShapeHash": isPlanCacheHash(e["planCacheShapeHash"]),
					"planCacheKey":       isPlanCacheHash(e["planCacheKey"]),
				})
			}
			return bson.M{"before": len(before), "after": entries}, nil
		},
	})
}

func TestPlanCache_ClearByShapeOrCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PlanCache_ClearByShapeOrCollection",
		Support: harness.DumboDBFull,
		Setup:   planCacheSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var steps bson.A
			step := func(name string, clear bson.D, rerun bool) error {
				if rerun {
					if err := runPlanCacheRace(ctx, col); err != nil {
						return err
					}
				}
				var reply interface{}
				if clear != nil {
					res, err := planCacheClear(ctx, col, clear)
					if err != nil {
						return err
					}
					reply = res
				}
				stats, err := planCacheStats(ctx, col)
				if err != nil {
					return err
				}
				steps = append(steps, bson.M{"step": name, "reply": reply, "entries": len(stats)})
				return nil
			}
			sameShape := bson.D{{Key: "query", Value: bson.D{{Key: "p", Value: int32(9)}}}, {Key: "sort", Value: bson.D{{Key: "k", Value: 1}}}, {Key: "projection", Value: bson.D{}}}
			otherShape := bson.D{{Key: "query", Value: bson.D{{Key: "q", Value: int32(5)}}}}
			for _, s := range []struct {
				name  string
				clear bson.D
				rerun bool
			}{
				{"cached", nil, true},
				{"clear its shape", sameShape, false},
				{"cached again", nil, true},
				{"clear another shape", otherShape, false},
				{"clear the collection", bson.D{}, false},
			} {
				if err := step(s.name, s.clear, s.rerun); err != nil {
					return nil, err
				}
			}
			return steps, nil
		},
	})
}

func TestPlanCache_MissingCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "PlanCache_ClearMissingCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return planCacheClear(ctx, col.Database().Collection("missing"), bson.D{})
		},
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "PlanCache_StatsMissingCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return planCacheStats(ctx, col.Database().Collection("missing"))
		},
	})
}
