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
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestPipeline_LengthLimit: no pipeline, sub-pipeline or view may hold more
// than 1000 stages, and a view's stages count toward a query on it.
func TestPipeline_LengthLimit(t *testing.T) {
	matches := func(n int) bson.A {
		out := make(bson.A, n)
		for i := range out {
			out[i] = bson.D{{Key: "$match", Value: bson.D{{Key: "a", Value: 1}}}}
		}
		return out
	}
	// outcome is "ok" or the command error's code and message.
	outcome := func(err error) interface{} {
		var ce mongo.CommandError
		if errors.As(err, &ce) {
			return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: ce.Message}}
		}
		if err != nil {
			return fmt.Sprint(err)
		}
		return "ok"
	}
	run := func(cmd func(col *mongo.Collection) bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return outcome(col.Database().RunCommand(ctx, cmd(col)).Err()), nil
		}
	}
	aggregate := func(pipeline bson.A) func(col *mongo.Collection) bson.D {
		return func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}
		}
	}
	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"aggregate_1000", run(aggregate(matches(1000)))},
		{"aggregate_1001", run(aggregate(matches(1001)))},
		{"explain_1001", run(func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "explain", Value: aggregate(matches(1001))(col)}}
		})},
		{"facet_1001", run(aggregate(bson.A{bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: matches(1001)}}}}}))},
		{"lookup_pipeline_1001", run(aggregate(bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "x"}, {Key: "pipeline", Value: matches(1001)}, {Key: "as", Value: "j"}}}}}))},
		{"union_with_1001", run(aggregate(bson.A{bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: "x"}, {Key: "pipeline", Value: matches(1001)}}}}}))},
		{"create_view_1001", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			err := col.Database().RunCommand(ctx, bson.D{{Key: "create", Value: col.Name() + "_v"}, {Key: "viewOn", Value: col.Name()}, {Key: "pipeline", Value: matches(1001)}}).Err()
			return outcome(err), nil
		}},
		{"view_and_pipeline_over_1000", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := col.Database().RunCommand(ctx, bson.D{{Key: "create", Value: col.Name() + "_v"}, {Key: "viewOn", Value: col.Name()}, {Key: "pipeline", Value: matches(600)}}).Err(); err != nil {
				return nil, err
			}
			err := col.Database().RunCommand(ctx, aggregate(matches(500))(col.Database().Collection(col.Name()+"_v"))).Err()
			return outcome(err), nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Pipeline_LengthLimit_" + tc.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "a", Value: 1}})
					return err
				},
				Run: tc.run,
			})
		})
	}
}

// TestExplain_LongLookupMatchPipeline: explaining 500 $lookups followed by
// 500 $matches, the longest pipeline allowed, finishes promptly.
func TestExplain_LongLookupMatchPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Explain_LongLookupMatchPipeline",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "a", Value: 1}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			pipeline := bson.A{}
			for i := 0; i < 500; i++ {
				pipeline = append(pipeline, bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "x"}, {Key: "localField", Value: "a"}, {Key: "foreignField", Value: "b"}, {Key: "as", Value: "j"}}}})
			}
			for i := 0; i < 500; i++ {
				pipeline = append(pipeline, bson.D{{Key: "$match", Value: bson.D{{Key: "z", Value: 1}}}})
			}
			start := time.Now()
			err := col.Database().RunCommand(ctx, bson.D{{Key: "explain", Value: bson.D{{Key: "aggregate", Value: col.Name()}, {Key: "pipeline", Value: pipeline}, {Key: "cursor", Value: bson.D{}}}}, {Key: "verbosity", Value: "queryPlanner"}}).Err()
			if err != nil {
				return nil, err
			}
			return time.Since(start) < 5*time.Second, nil
		},
	})
}

// TestMaxTimeMS_LargeIDIn: a count, distinct or find whose _id $in holds many
// slow-to-encode decimals either finishes or stops at maxTimeMS with
// MaxTimeMSExpired, promptly; which one depends on the server's speed, so both
// count as the same outcome.
func TestMaxTimeMS_LargeIDIn(t *testing.T) {
	values := make(bson.A, 300_000)
	for i := range values {
		values[i] = mustDecimal(fmt.Sprintf("1.%033dE-6065", i))
	}
	filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: values}}}}
	cases := []struct {
		name string
		cmd  func(col *mongo.Collection) bson.D
	}{
		{"count", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "count", Value: col.Name()}, {Key: "query", Value: filter}, {Key: "maxTimeMS", Value: 5}}
		}},
		{"distinct", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "distinct", Value: col.Name()}, {Key: "key", Value: "a"}, {Key: "query", Value: filter}, {Key: "maxTimeMS", Value: 5}}
		}},
		{"find", func(col *mongo.Collection) bson.D {
			return bson.D{{Key: "find", Value: col.Name()}, {Key: "filter", Value: filter}, {Key: "maxTimeMS", Value: 5}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:      "MaxTimeMS_LargeIDIn_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					start := time.Now()
					err := col.Database().RunCommand(ctx, tc.cmd(col)).Err()
					var ce mongo.CommandError
					outcome := "finished or timed out"
					if errors.As(err, &ce) && ce.Code != 50 {
						outcome = fmt.Sprintf("error %d", ce.Code)
					} else if err != nil && !errors.As(err, &ce) {
						outcome = err.Error()
					}
					return bson.D{{Key: "outcome", Value: outcome}, {Key: "prompt", Value: time.Since(start) < 5*time.Second}}, nil
				},
			})
		})
	}
}

// TestPipeline_LengthLimitThroughViews: commands on a view of 1000 stages, a
// chain of views over 1000, collMod and a findAndModify pipeline are held to
// the 1000-stage limit, counting the stages mongod runs a command as.
func TestPipeline_LengthLimitThroughViews(t *testing.T) {
	sets := func(n int) bson.A {
		out := make(bson.A, n)
		for i := range out {
			out[i] = bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: 1}}}}
		}
		return out
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: 1}}); err != nil {
			return err
		}
		db := col.Database()
		for _, v := range []struct {
			name, on string
			n        int
		}{{"_v1000", col.Name(), 1000}, {"_v999", col.Name(), 999}, {"_v998", col.Name(), 998}, {"_v997", col.Name(), 997},
			{"_v600", col.Name(), 600}, {"_v600b", col.Name() + "_v600", 600}} {
			if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: col.Name() + v.name}, {Key: "viewOn", Value: v.on}, {Key: "pipeline", Value: sets(v.n)}}).Err(); err != nil {
				return err
			}
		}
		return nil
	}
	cmds := []struct {
		name string
		cmd  func(name string) bson.D
	}{
		{"find_filtered", func(n string) bson.D {
			return bson.D{{Key: "find", Value: n + "_v1000"}, {Key: "filter", Value: bson.D{{Key: "a", Value: 1}}}}
		}},
		{"find_unfiltered", func(n string) bson.D { return bson.D{{Key: "find", Value: n + "_v1000"}} }},
		{"find_chain", func(n string) bson.D { return bson.D{{Key: "find", Value: n + "_v600b"}} }},
		{"count", func(n string) bson.D { return bson.D{{Key: "count", Value: n + "_v1000"}} }},
		{"count_chain", func(n string) bson.D { return bson.D{{Key: "count", Value: n + "_v600b"}} }},
		{"distinct", func(n string) bson.D { return bson.D{{Key: "distinct", Value: n + "_v1000"}, {Key: "key", Value: "a"}} }},
		{"aggregate_chain", func(n string) bson.D {
			return bson.D{{Key: "aggregate", Value: n + "_v600b"}, {Key: "pipeline", Value: bson.A{}}, {Key: "cursor", Value: bson.D{}}}
		}},
		{"explain_find", func(n string) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "find", Value: n + "_v1000"}, {Key: "filter", Value: bson.D{{Key: "a", Value: 1}}}}}}
		}},
		{"coll_mod", func(n string) bson.D {
			return bson.D{{Key: "collMod", Value: n + "_v600"}, {Key: "viewOn", Value: n}, {Key: "pipeline", Value: sets(1001)}}
		}},
		{"find_and_modify_pipeline", func(n string) bson.D {
			return bson.D{{Key: "findAndModify", Value: n}, {Key: "query", Value: bson.D{}}, {Key: "update", Value: sets(1001)}}
		}},
		{"update_pipeline", func(n string) bson.D {
			return bson.D{{Key: "update", Value: n}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "u", Value: sets(1001)}}}}}
		}},
		{"lookup_from_view", func(n string) bson.D {
			return bson.D{{Key: "aggregate", Value: n}, {Key: "pipeline", Value: bson.A{bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: n + "_v1000"}, {Key: "localField", Value: "a"}, {Key: "foreignField", Value: "a"}, {Key: "as", Value: "j"}}}}}}, {Key: "cursor", Value: bson.D{}}}
		}},
	}
	aggregate := func(n string, stage bson.D) bson.D {
		return bson.D{{Key: "aggregate", Value: n}, {Key: "pipeline", Value: bson.A{stage}}, {Key: "cursor", Value: bson.D{}}}
	}
	union := func(view string, sub int) func(n string) bson.D {
		return func(n string) bson.D {
			return aggregate(n, bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: n + view}, {Key: "pipeline", Value: sets(sub)}}}})
		}
	}
	lookup := func(view string, sub int, local bool) func(n string) bson.D {
		return func(n string) bson.D {
			spec := bson.D{{Key: "from", Value: n + view}, {Key: "pipeline", Value: sets(sub)}, {Key: "as", Value: "j"}}
			if local {
				spec = append(spec, bson.E{Key: "localField", Value: "a"}, bson.E{Key: "foreignField", Value: "a"})
			}
			return aggregate(n, bson.D{{Key: "$lookup", Value: spec}})
		}
	}
	explainUpdate := func(stages int) func(n string) bson.D {
		return func(n string) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "update", Value: n}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "u", Value: sets(stages)}}}}}}}
		}
	}
	explainFindAndModify := func(stages int) func(n string) bson.D {
		return func(n string) bson.D {
			return bson.D{{Key: "explain", Value: bson.D{{Key: "findAndModify", Value: n}, {Key: "query", Value: bson.D{}}, {Key: "update", Value: sets(stages)}}}}
		}
	}
	cmds = append(cmds, []struct {
		name string
		cmd  func(name string) bson.D
	}{
		{"union_with_bare_v1000", func(n string) bson.D { return aggregate(n, bson.D{{Key: "$unionWith", Value: n + "_v1000"}}) }},
		{"union_with_v999_sub1", union("_v999", 1)},
		{"union_with_v999_sub2", union("_v999", 2)},
		{"union_with_v998_sub2", union("_v998", 2)},
		{"lookup_pipeline_v1000_sub0", lookup("_v1000", 0, false)},
		{"lookup_pipeline_v1000_sub1", lookup("_v1000", 1, false)},
		{"lookup_pipeline_v999_sub2", lookup("_v999", 2, false)},
		{"lookup_pipeline_v998_sub2", lookup("_v998", 2, false)},
		{"lookup_local_v999_sub0", lookup("_v999", 0, true)},
		{"lookup_local_v999_sub1", lookup("_v999", 1, true)},
		{"lookup_local_v998_sub1", lookup("_v998", 1, true)},
		{"count_v998", func(n string) bson.D { return bson.D{{Key: "count", Value: n + "_v998"}} }},
		{"count_v999", func(n string) bson.D { return bson.D{{Key: "count", Value: n + "_v999"}} }},
		{"count_v998_query", func(n string) bson.D {
			return bson.D{{Key: "count", Value: n + "_v998"}, {Key: "query", Value: bson.D{{Key: "a", Value: 1}}}}
		}},
		{"count_v997_skip_limit", func(n string) bson.D {
			return bson.D{{Key: "count", Value: n + "_v997"}, {Key: "skip", Value: 1}, {Key: "limit", Value: 5}}
		}},
		{"explain_update_1000", explainUpdate(1000)},
		{"explain_update_1001", explainUpdate(1001)},
		{"explain_find_and_modify_1000", explainFindAndModify(1000)},
		{"explain_find_and_modify_1001", explainFindAndModify(1001)},
	}...)
	for _, c := range cmds {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Pipeline_LengthLimitThroughViews_" + c.name,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					err := col.Database().RunCommand(ctx, c.cmd(col.Name())).Err()
					var ce mongo.CommandError
					if errors.As(err, &ce) {
						msg := strings.ReplaceAll(ce.Message, col.Database().Name()+"."+col.Name(), "<ns>")
						return bson.D{{Key: "code", Value: ce.Code}, {Key: "message", Value: msg}}, nil
					}
					return bson.D{{Key: "ok", Value: err == nil}}, nil
				},
			})
		})
	}
}
