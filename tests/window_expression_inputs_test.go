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

// A window function's input is any aggregation expression, evaluated against
// each document of the window, not only a field path.

func windowExprSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(5)}, {Key: "z", Value: int32(0)}, {Key: "s", Value: "x"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(7)}, {Key: "z", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "v", Value: int32(9)}, {Key: "z", Value: int32(2)}, {Key: "s", Value: "y"}},
	})
	return err
}

func TestWindowExpressionInputs(t *testing.T) {
	addOne := bson.D{{Key: "$add", Value: bson.A{"$v", 1}}}
	divZero := bson.D{{Key: "$divide", Value: bson.A{"$v", "$z"}}}
	prevAndCurrent := bson.D{{Key: "documents", Value: bson.A{-1, 0}}}
	cases := []struct {
		name string
		fn   bson.D
	}{
		{"Push", bson.D{{Key: "$push", Value: addOne}}},
		{"Sum", bson.D{{Key: "$sum", Value: bson.D{{Key: "$multiply", Value: bson.A{"$v", 2}}}}}},
		{"Avg", bson.D{{Key: "$avg", Value: addOne}}},
		{"MinWindowed", bson.D{{Key: "$min", Value: addOne}, {Key: "window", Value: prevAndCurrent}}},
		{"Max", bson.D{{Key: "$max", Value: addOne}}},
		{"First", bson.D{{Key: "$first", Value: addOne}}},
		{"Last", bson.D{{Key: "$last", Value: addOne}}},
		{"AddToSet", bson.D{{Key: "$addToSet", Value: bson.D{{Key: "$mod", Value: bson.A{"$v", 2}}}}}},
		{"StdDevPop", bson.D{{Key: "$stdDevPop", Value: addOne}}},
		{"PushObjectLiteral", bson.D{{Key: "$push", Value: bson.D{{Key: "a", Value: "$v"}, {Key: "b", Value: "$s"}}}}},
		{"PushArrayLiteral", bson.D{{Key: "$push", Value: bson.A{"$v", "$s"}}}},
		{"PushRemove", bson.D{{Key: "$push", Value: "$$REMOVE"}}},
		{"PushDivideByZero", bson.D{{Key: "$push", Value: divZero}}},
		{"SumDivideByZero", bson.D{{Key: "$sum", Value: divZero}}},
		{"Shift", bson.D{{Key: "$shift", Value: bson.D{{Key: "output", Value: addOne}, {Key: "by", Value: 1}}}}},
		{"ShiftMissingUsesDefault", bson.D{{Key: "$shift", Value: bson.D{{Key: "output", Value: "$s"}, {Key: "by", Value: 1}, {Key: "default", Value: "d"}}}}},
		{"Derivative", bson.D{{Key: "$derivative", Value: bson.D{{Key: "input", Value: addOne}}}, {Key: "window", Value: prevAndCurrent}}},
		{"DerivativeDivideByZero", bson.D{{Key: "$derivative", Value: bson.D{{Key: "input", Value: divZero}}}, {Key: "window", Value: prevAndCurrent}}},
		{"ExpMovingAvg", bson.D{{Key: "$expMovingAvg", Value: bson.D{{Key: "input", Value: addOne}, {Key: "N", Value: 2}}}}},
		{"CovariancePop", bson.D{{Key: "$covariancePop", Value: bson.A{addOne, "$z"}}}},
		{"CovarianceSamp", bson.D{{Key: "$covarianceSamp", Value: bson.A{"$v", bson.D{{Key: "$multiply", Value: bson.A{"$z", 2}}}}}}},
		{"Locf", bson.D{{Key: "$locf", Value: addOne}}},
		{"LinearFill", bson.D{{Key: "$linearFill", Value: addOne}}},
		{"TopOperatorOutput", bson.D{{Key: "$top", Value: bson.D{{Key: "output", Value: addOne}, {Key: "sortBy", Value: bson.D{{Key: "v", Value: -1}}}}}}},
		{"BottomObjectOutput", bson.D{{Key: "$bottom", Value: bson.D{{Key: "output", Value: bson.D{{Key: "a", Value: "$v"}, {Key: "b", Value: "$s"}}}, {Key: "sortBy", Value: bson.D{{Key: "v", Value: -1}}}}}}},
		{"TopArrayOutput", bson.D{{Key: "$top", Value: bson.D{{Key: "output", Value: bson.A{"$v", "$s"}}, {Key: "sortBy", Value: bson.D{{Key: "v", Value: 1}}}}}}},
	}
	for _, c := range cases {
		fn := c.fn
		harness.PairTest(t, harness.TestCase{
			Name:    "WindowExpr_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   windowExprSeed,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "output", Value: bson.D{{Key: "r", Value: fn}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}),
		})
	}
}
