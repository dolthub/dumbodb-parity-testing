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

// $lookup let variables are not constants, so mongod does not fold
// expressions that use them; an error in a branch never taken is not raised.

func lookupLetSetup(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(0)}}); err != nil {
		return err
	}
	_, err := col.Database().Collection("emp").InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}})
	return err
}

func TestLookupLetNotFolded(t *testing.T) {
	divide := bson.D{{Key: "$divide", Value: bson.A{int32(1), "$$x"}}}
	untaken := bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$eq", Value: bson.A{"$_id", int32(99)}}}, divide, true}}}
	stages := map[string]bson.D{
		"Match":       {{Key: "$match", Value: bson.D{{Key: "$expr", Value: untaken}}}},
		"Project":     {{Key: "$project", Value: bson.D{{Key: "v", Value: untaken}}}},
		"AddFields":   {{Key: "$addFields", Value: bson.D{{Key: "v", Value: untaken}}}},
		"Set":         {{Key: "$set", Value: bson.D{{Key: "v", Value: untaken}}}},
		"ReplaceWith": {{Key: "$replaceWith", Value: bson.D{{Key: "v", Value: untaken}}}},
		"Taken":       {{Key: "$project", Value: bson.D{{Key: "v", Value: divide}}}},
	}
	for name, stage := range stages {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLetNotFolded_" + name,
			Support: harness.DumboDBFull,
			Setup:   lookupLetSetup,
			Run: round6Aggregate(bson.A{bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: "emp"},
				{Key: "let", Value: bson.D{{Key: "x", Value: "$x"}}},
				{Key: "pipeline", Value: bson.A{stage}},
				{Key: "as", Value: "j"},
			}}}}),
		})
	}
}

// A let value substituted into $project must stay a value: a bare 0 or true
// there is an exclusion or inclusion flag (review O17).
func TestLookupLetProjectFlags(t *testing.T) {
	for name, v := range map[string]interface{}{"Zero": int32(0), "One": int32(1), "Seven": int32(7), "True": true, "False": false} {
		for stageName, stage := range map[string]bson.D{
			"Project":   {{Key: "$project", Value: bson.D{{Key: "y", Value: "$$v"}}}},
			"AddFields": {{Key: "$addFields", Value: bson.D{{Key: "y", Value: "$$v"}}}},
		} {
			harness.PairTest(t, harness.TestCase{
				Name:    "LookupLetProjectFlags_" + stageName + "_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if _, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: v}}); err != nil {
						return err
					}
					_, err := col.Database().Collection("emp").InsertOne(ctx, bson.D{{Key: "_id", Value: int32(9)}, {Key: "y", Value: "FOREIGN-Y"}, {Key: "secret", Value: "S"}})
					return err
				},
				Run: round6Aggregate(bson.A{bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: "emp"},
					{Key: "let", Value: bson.D{{Key: "v", Value: "$v"}}},
					{Key: "pipeline", Value: bson.A{stage}},
					{Key: "as", Value: "j"},
				}}}}),
			})
		}
	}
}
