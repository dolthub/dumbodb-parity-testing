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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Window functions that are only valid inside $setWindowFields, used
// anywhere else.

func insertWindowOutsideDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(0)}, {Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(3)}, {Key: "y", Value: int32(5)}},
	})
	return err
}

// errOutcomeWithMessage is errOutcome with the server's error message, which
// these rejections are about.
func errOutcomeWithMessage(err error) bson.M {
	out := errOutcome(err)
	var cmdErr mongo.CommandError
	var writeErr mongo.WriteException
	switch {
	case errors.As(err, &cmdErr):
		out["errmsg"] = cmdErr.Message
	case errors.As(err, &writeErr) && writeErr.WriteConcernError == nil && len(writeErr.WriteErrors) > 0:
		out["errmsg"] = writeErr.WriteErrors[0].Message
	}
	return out
}

func windowRunPipeline(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			return errOutcomeWithMessage(err), nil
		}
		var docs []bson.Raw
		if err = cursor.All(ctx, &docs); err != nil {
			return errOutcomeWithMessage(err), nil
		}
		return len(docs), nil
	}
}

func TestWindowOperatorsOutsideSetWindowFields(t *testing.T) {
	windowOps := []struct {
		name string
		op   string
		arg  interface{}
	}{
		{"rank", "rank", bson.D{}},
		{"denseRank", "denseRank", bson.D{}},
		{"documentNumber", "documentNumber", bson.D{}},
		{"shift", "shift", bson.D{{Key: "output", Value: "$x"}, {Key: "by", Value: int32(1)}}},
		{"expMovingAvg", "expMovingAvg", bson.D{{Key: "input", Value: "$x"}, {Key: "N", Value: int32(2)}}},
		{"derivative", "derivative", bson.D{{Key: "input", Value: "$x"}}},
		{"integral", "integral", bson.D{{Key: "input", Value: "$x"}}},
		{"locf", "locf", "$x"},
		{"linearFill", "linearFill", "$x"},
		{"covariancePop", "covariancePop", bson.A{"$x", "$y"}},
		{"covarianceSamp", "covarianceSamp", bson.A{"$x", "$y"}},
		{"covariancePopScalar", "covariancePop", "$x"},
		{"covarianceSampEmptyDoc", "covarianceSamp", bson.D{}},
	}

	for _, w := range windowOps {
		expr := bson.D{{Key: "$" + w.op, Value: w.arg}}
		contexts := []struct {
			name string
			run  func(context.Context, *mongo.Collection) (interface{}, error)
		}{
			{"Project", windowRunPipeline(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: expr}}}}})},
			{"AddFields", windowRunPipeline(bson.A{bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: expr}}}}})},
			{"Set", windowRunPipeline(bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "r", Value: expr}}}}})},
			{"MatchExpr", windowRunPipeline(bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: expr}}}}})},
			{"Nested", windowRunPipeline(bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), expr}}}}}}}})},
			{"ReplaceRoot", windowRunPipeline(bson.A{bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: bson.D{{Key: "r", Value: expr}}}}}}})},
			{"GroupAccumulator", windowRunPipeline(bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: expr}}}}})},
			{"GroupKey", windowRunPipeline(bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: expr}}}}})},
			{"BucketOutput", windowRunPipeline(bson.A{bson.D{{Key: "$bucket", Value: bson.D{
				{Key: "groupBy", Value: "$x"}, {Key: "boundaries", Value: bson.A{int32(0), int32(10)}}, {Key: "output", Value: bson.D{{Key: "r", Value: expr}}},
			}}}})},
			{"BucketAutoOutput", windowRunPipeline(bson.A{bson.D{{Key: "$bucketAuto", Value: bson.D{
				{Key: "groupBy", Value: "$x"}, {Key: "buckets", Value: int32(1)}, {Key: "output", Value: bson.D{{Key: "r", Value: expr}}},
			}}}})},
			{"FindFilterExpr", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, bson.D{{Key: "$expr", Value: expr}})
				if err != nil {
					return errOutcomeWithMessage(err), nil
				}
				var docs []bson.D
				if err = cursor.All(ctx, &docs); err != nil {
					return errOutcomeWithMessage(err), nil
				}
				return len(docs), nil
			}},
			{"FindProjection", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "r", Value: expr}}))
				if err != nil {
					return errOutcomeWithMessage(err), nil
				}
				var docs []bson.D
				if err = cursor.All(ctx, &docs); err != nil {
					return errOutcomeWithMessage(err), nil
				}
				return len(docs), nil
			}},
			{"UpdatePipeline", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				_, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: int32(0)}}, mongo.Pipeline{bson.D{{Key: "$set", Value: bson.D{{Key: "r", Value: expr}}}}})
				if err != nil {
					return errOutcomeWithMessage(err), nil
				}
				return "updated", nil
			}},
		}
		for _, c := range contexts {
			t.Run(w.name+"_"+c.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "WindowOutside_" + w.name + "_" + c.name,
					Support: harness.DumboDBXFail,
					Setup:   insertWindowOutsideDocs,
					Run:     c.run,
				})
			})
		}
	}
}
