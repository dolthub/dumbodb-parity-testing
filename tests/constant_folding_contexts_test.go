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

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod folds constant subexpressions wherever it parses an expression, so a
// failing constant in a branch never taken is still an error.
func TestConstantFoldingContexts(t *testing.T) {
	untaken := bson.D{{Key: "$cond", Value: bson.A{"$a", int32(1), bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}}}}
	pipelines := map[string]bson.A{
		"MatchExpr":      {bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: untaken}}}}},
		"GroupID":        {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: untaken}}}}},
		"GroupSum":       {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "n", Value: bson.D{{Key: "$sum", Value: untaken}}}}}}},
		"ReplaceRoot":    {bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: bson.D{{Key: "r", Value: untaken}}}}}}},
		"ReplaceWith":    {bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "r", Value: untaken}}}}},
		"Redact":         {bson.D{{Key: "$redact", Value: bson.D{{Key: "$cond", Value: bson.A{untaken, "$$KEEP", "$$PRUNE"}}}}}},
		"BucketGroupBy":  {bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: untaken}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "default", Value: "x"}}}}},
		"SortByCount":    {bson.D{{Key: "$sortByCount", Value: untaken}}},
		"LookupLet":      {bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: "other"}, {Key: "let", Value: bson.D{{Key: "v", Value: untaken}}}, {Key: "pipeline", Value: bson.A{}}, {Key: "as", Value: "j"}}}}},
		"FacetProject":   {bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: untaken}}}}}}}}}},
		"SetWindowField": {bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "output", Value: bson.D{{Key: "w", Value: bson.D{{Key: "$sum", Value: untaken}}}}}}}}},
		"UnwindAfterSet": {bson.D{{Key: "$set", Value: bson.D{{Key: "r", Value: untaken}}}}, bson.D{{Key: "$unwind", Value: "$r"}}},
		// Fails only where a is missing, which no document is.
		"MatchExprMissingOnly": {bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{
			bson.D{{Key: "$divide", Value: bson.A{int32(1), bson.D{{Key: "$ifNull", Value: bson.A{"$a", int32(0)}}}}}}, int32(0),
		}}}}}}}},
	}
	for _, empty := range []bool{true, false} {
		for name, pipeline := range pipelines {
			n := "ConstantFoldingContexts_" + name
			if empty {
				n += "_Empty"
			}
			p, isEmpty := pipeline, empty
			t.Run(n, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    n,
					Support: harness.DumboDBFull,
					Setup:   constantFoldingSetup(isEmpty),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						cur, err := col.Aggregate(ctx, p)
						if err != nil {
							return errOutcome(err), nil
						}
						var docs []bson.M
						if err := cur.All(ctx, &docs); err != nil {
							return errOutcome(err), nil
						}
						return len(docs), nil
					},
				})
			})
		}
	}

	others := map[string]func(ctx context.Context, col *mongo.Collection) error{
		"FindExpr": func(ctx context.Context, col *mongo.Collection) error {
			cur, err := col.Find(ctx, bson.D{{Key: "$expr", Value: untaken}})
			if err == nil {
				err = cur.All(ctx, &[]bson.M{})
			}
			return err
		},
		"CountExpr": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.CountDocuments(ctx, bson.D{{Key: "$expr", Value: untaken}})
			return err
		},
		"UpdatePipeline": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{}, mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "r", Value: untaken}}}}})
			return err
		},
		"UpdateFilterExpr": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{{Key: "$expr", Value: untaken}}, bson.D{{Key: "$set", Value: bson.D{{Key: "z", Value: 1}}}})
			return err
		},
		"DeleteFilterExpr": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.DeleteMany(ctx, bson.D{{Key: "$expr", Value: untaken}})
			return err
		},
		"FindAndModifyPipeline": func(ctx context.Context, col *mongo.Collection) error {
			return col.FindOneAndUpdate(ctx, bson.D{}, mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "r", Value: untaken}}}}}, options.FindOneAndUpdate()).Err()
		},
		"UpdatePipelineAddFields": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{}, mongo.Pipeline{{{Key: "$addFields", Value: bson.D{{Key: "r", Value: untaken}}}}})
			return err
		},
		"UpdatePipelineProject": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{}, mongo.Pipeline{{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}, {Key: "r", Value: untaken}}}}})
			return err
		},
		"UpdatePipelineReplaceWith": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{}, mongo.Pipeline{{{Key: "$replaceWith", Value: bson.D{{Key: "a", Value: "$a"}, {Key: "r", Value: untaken}}}}})
			return err
		},
		"UpdatePipelineReplaceRoot": func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.UpdateMany(ctx, bson.D{}, mongo.Pipeline{{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: bson.D{{Key: "a", Value: "$a"}, {Key: "r", Value: untaken}}}}}}})
			return err
		},
		"BulkWritePipeline": func(ctx context.Context, col *mongo.Collection) error {
			var reply struct {
				NErrors int32 `bson:"nErrors"`
			}
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "bulkWrite", Value: int32(1)},
				{Key: "ops", Value: bson.A{bson.D{{Key: "update", Value: int32(0)}, {Key: "filter", Value: bson.D{}}, {Key: "multi", Value: true}, {Key: "updateMods", Value: bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "r", Value: untaken}}}}}}}}},
				{Key: "nsInfo", Value: bson.A{bson.D{{Key: "ns", Value: col.Database().Name() + "." + col.Name()}}}},
			}).Decode(&reply)
			if err == nil && reply.NErrors > 0 {
				err = fmt.Errorf("nErrors %d", reply.NErrors)
			}
			return err
		},
	}
	for _, empty := range []bool{true, false} {
		for name, run := range others {
			n := "ConstantFoldingContexts_" + name
			if empty {
				n += "_Empty"
			}
			r, isEmpty := run, empty
			support := harness.DumboDBFull
			t.Run(n, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    n,
					Support: support,
					Setup:   constantFoldingSetup(isEmpty),
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						return errOutcome(r(ctx, col)), nil
					},
				})
			})
		}
	}
}

func constantFoldingSetup(empty bool) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if empty {
			return col.Database().CreateCollection(ctx, col.Name())
		}
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}})
		return err
	}
}
