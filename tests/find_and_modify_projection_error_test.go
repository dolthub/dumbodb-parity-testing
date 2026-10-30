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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// findAndModifyProjectionPending: mongod commits the write and then fails
// the projection; DumboDB discards the write (decision: workspace-cn8).
var findAndModifyProjectionPending = map[string]bool{
	"UpdateBadConvert": true,
	"UpdateBadPath":    true,
}

// DumboDB wrote the document and then failed applying 'fields', so the
// client saw an error for a committed write (workspace-8tj.18). Each case
// reports the command outcome and the collection afterwards.
func TestFindAndModifyProjectionError(t *testing.T) {
	divideByZero := bson.D{{Key: "x", Value: bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}}}
	badConvert := bson.D{{Key: "x", Value: bson.D{{Key: "$toInt", Value: "$s"}}}}
	badPath := bson.D{{Key: "x", Value: bson.D{{Key: "$add", Value: bson.A{"$s", int32(1)}}}}}
	fine := bson.D{{Key: "x", Value: bson.D{{Key: "$add", Value: bson.A{"$n", int32(1)}}}}}
	cases := []struct {
		name string
		cmd  bson.D
	}{
		{"UpdateDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "fields", Value: divideByZero}}},
		{"UpdateNewDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "new", Value: true}, {Key: "fields", Value: divideByZero}}},
		{"UpdateBadConvert", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "s", Value: "y"}}}}}, {Key: "fields", Value: badConvert}}},
		{"UpdateBadPath", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "fields", Value: badPath}}},
		{"ReplaceDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "n", Value: int32(9)}}}, {Key: "fields", Value: divideByZero}}},
		{"RemoveDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "remove", Value: true}, {Key: "fields", Value: divideByZero}}},
		{"UpsertNewDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(7)}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "upsert", Value: true}, {Key: "new", Value: true}, {Key: "fields", Value: divideByZero}}},
		{"UpsertOldDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(7)}}}, {Key: "update", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "upsert", Value: true}, {Key: "fields", Value: divideByZero}}},
		{"NoMatchDivideByZero", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(99)}}}, {Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "fields", Value: divideByZero}}},
		{"UpdateFine", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "new", Value: true}, {Key: "fields", Value: fine}}},
		{"InvalidProjectionShape", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: int32(1)}}}, {Key: "update", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "n", Value: int32(1)}}}}}, {Key: "fields", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(0)}}}}},
	}
	for _, tc := range cases {
		cmd := tc.cmd
		t.Run(tc.name, func(t *testing.T) {
			support := harness.DumboDBFull
			if findAndModifyProjectionPending[tc.name] {
				support = harness.DumboDBXFail
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "FindAndModifyProjectionError_" + tc.name,
				Support: support,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "n", Value: int32(1)}, {Key: "s", Value: "x"}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					full := append(bson.D{{Key: "findAndModify", Value: col.Name()}}, cmd...)
					var reply bson.M
					err := col.Database().RunCommand(ctx, full).Decode(&reply)
					outcome := errOutcome(err)
					if err == nil {
						outcome["value"] = reply["value"]
						outcome["lastErrorObject"] = reply["lastErrorObject"]
					}
					cur, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
					if err != nil {
						return nil, err
					}
					var docs []bson.M
					if err := cur.All(ctx, &docs); err != nil {
						return nil, err
					}
					return bson.M{"outcome": outcome, "after": docs}, nil
				},
			})
		})
	}
}

// mongod folds constant subexpressions while parsing, so a constant that
// fails is an error even when no document is processed.
func TestConstantSubexpressionErrors(t *testing.T) {
	div0 := bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}
	exprs := []struct {
		name string
		expr interface{}
	}{
		{"Top", div0},
		{"InCondBranch", bson.D{{Key: "$cond", Value: bson.A{"$a", int32(1), div0}}}},
		{"InConstantCondUntaken", bson.D{{Key: "$cond", Value: bson.A{true, int32(1), div0}}}},
		{"InLetVars", bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "v", Value: div0}}}, {Key: "in", Value: "$a"}}}}},
		{"InArrayLiteral", bson.A{"$a", div0}},
		{"InDocumentLiteral", bson.D{{Key: "p", Value: "$a"}, {Key: "q", Value: div0}}},
		{"InLiteral", bson.D{{Key: "$literal", Value: div0}}},
		{"WithRand", bson.D{{Key: "$add", Value: bson.A{bson.D{{Key: "$rand", Value: bson.D{}}}, div0}}}},
		{"WithNow", bson.D{{Key: "$add", Value: bson.A{"$$NOW", div0}}}},
		{"NestedConstant", bson.D{{Key: "$add", Value: bson.A{"$a", bson.D{{Key: "$toInt", Value: "x"}}}}}},
		{"ConstantOk", bson.D{{Key: "$add", Value: bson.A{"$a", bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(2)}}}}}}},
		{"InSwitchDefault", bson.D{{Key: "$switch", Value: bson.D{{Key: "branches", Value: bson.A{bson.D{{Key: "case", Value: "$a"}, {Key: "then", Value: int32(1)}}}}, {Key: "default", Value: div0}}}}},
		{"InMapIn", bson.D{{Key: "$map", Value: bson.D{{Key: "input", Value: "$arr"}, {Key: "in", Value: div0}}}}},
	}
	for _, e := range exprs {
		for _, empty := range []bool{true, false} {
			for _, stage := range []string{"$project", "$addFields", "find"} {
				name := "ConstantSubexpression_" + stage[len(stage)-4:] + "_" + e.name
				if empty {
					name += "_Empty"
				}
				expr, isEmpty, st := e.expr, empty, stage
				t.Run(name, func(t *testing.T) {
					support := harness.DumboDBFull
					if name == "ConstantSubexpression_elds_NestedConstant_Empty" {
						support = harness.DumboDBXFail // workspace-2vz
					}
					harness.PairTest(t, harness.TestCase{
						Name:    name,
						Support: support,
						Setup: func(ctx context.Context, col *mongo.Collection) error {
							if isEmpty {
								return col.Database().CreateCollection(ctx, col.Name())
							}
							_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}, {Key: "arr", Value: bson.A{int32(1)}}})
							return err
						},
						Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
							var cur *mongo.Cursor
							var err error
							if st == "find" {
								cur, err = col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "r", Value: expr}}))
							} else {
								cur, err = col.Aggregate(ctx, mongo.Pipeline{{{Key: st, Value: bson.D{{Key: "r", Value: expr}}}}})
							}
							if err != nil {
								return errOutcome(err), nil
							}
							var docs []bson.M
							if err := cur.All(ctx, &docs); err != nil {
								return errOutcome(err), nil
							}
							return docs, nil
						},
					})
				})
			}
		}
	}
}
