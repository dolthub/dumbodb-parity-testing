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

// TestMaxTimeMS_LargeIDIn: a count or distinct whose _id $in holds many slow-to-
// encode decimals stops at maxTimeMS with MaxTimeMSExpired.
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
					code := int32(0)
					if errors.As(err, &ce) {
						code = ce.Code
					}
					return bson.D{{Key: "code", Value: code}, {Key: "prompt", Value: time.Since(start) < 5*time.Second}}, nil
				},
			})
		})
	}
}
