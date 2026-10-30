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

// mongod parses a validator as a query when create or collMod sets it, so
// invalid or disallowed validators are rejected up front (workspace-1cb).
func TestValidatorParse(t *testing.T) {
	div0 := bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}
	cond := func(c, a, b interface{}) bson.D { return bson.D{{Key: "$cond", Value: bson.A{c, a, b}}} }
	lt := func(a, b interface{}) bson.D { return bson.D{{Key: "$lt", Value: bson.A{a, b}}} }
	validators := map[string]bson.D{
		"UnknownTopLevel":   {{Key: "$foo", Value: int32(1)}},
		"UnknownFieldOp":    {{Key: "a", Value: bson.D{{Key: "$bogus", Value: int32(1)}}}},
		"InNotArray":        {{Key: "a", Value: bson.D{{Key: "$in", Value: int32(5)}}}},
		"AndNotArray":       {{Key: "$and", Value: int32(5)}},
		"SizeString":        {{Key: "a", Value: bson.D{{Key: "$size", Value: "x"}}}},
		"BadRegex":          {{Key: "a", Value: bson.D{{Key: "$regex", Value: "("}}}},
		"Where":             {{Key: "$where", Value: "true"}},
		"Text":              {{Key: "$text", Value: bson.D{{Key: "$search", Value: "x"}}}},
		"Near":              {{Key: "loc", Value: bson.D{{Key: "$near", Value: bson.A{0, 0}}}}},
		"NearSphere":        {{Key: "loc", Value: bson.D{{Key: "$nearSphere", Value: bson.A{0, 0}}}}},
		"ExprUnknownOp":     {{Key: "$expr", Value: bson.D{{Key: "$bogusOp", Value: int32(1)}}}},
		"ExprConstantError": {{Key: "$expr", Value: cond("$a", true, div0)}},
		"ExprRand":          {{Key: "$expr", Value: lt(bson.D{{Key: "$rand", Value: bson.D{}}}, 0.5)}},
		"ExprNow":           {{Key: "$expr", Value: lt("$d", "$$NOW")}},
		"ElemMatchBad":      {{Key: "a", Value: bson.D{{Key: "$elemMatch", Value: int32(1)}}}},
		"TypeUnknown":       {{Key: "a", Value: bson.D{{Key: "$type", Value: "nope"}}}},
		"Valid":             {{Key: "a", Value: bson.D{{Key: "$gt", Value: int32(1)}}}, {Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{"$a", int32(10)}}}}},
		"JSONSchema":        {{Key: "$jsonSchema", Value: bson.D{{Key: "bsonType", Value: "object"}}}},
		"Comment":           {{Key: "$comment", Value: "x"}, {Key: "a", Value: int32(1)}},
		"SampleRate":        {{Key: "$sampleRate", Value: 0.5}},
	}
	for name, v := range validators {
		validator := v
		for _, cmd := range []string{"Create", "CollMod"} {
			n := "ValidatorParse_" + cmd + "_" + name
			c := cmd
			t.Run(cmd+"_"+name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    n,
					Support: harness.DumboDBXFail,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if c == "CollMod" {
							return col.Database().CreateCollection(ctx, col.Name())
						}
						return nil
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						var err error
						if c == "Create" {
							err = col.Database().RunCommand(ctx, bson.D{{Key: "create", Value: col.Name()}, {Key: "validator", Value: validator}}).Err()
						} else {
							err = col.Database().RunCommand(ctx, bson.D{{Key: "collMod", Value: col.Name()}, {Key: "validator", Value: validator}}).Err()
						}
						return errOutcome(err), nil
					},
				})
			})
		}
	}
}
