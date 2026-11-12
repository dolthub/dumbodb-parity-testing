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

// A reference to an undefined variable fails with 17276 in every stage
// (dolthub/dumbodb#246).

func TestUndefinedVariables(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}})
		return err
	}
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"ProjectField", bson.D{{Key: "$project", Value: bson.D{{Key: "c", Value: "$$nope"}}}}},
		{"ProjectID", bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: "$$nope"}}}}},
		{"Set", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$nope"}}}}},
		{"SetPath", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$nope.x"}}}}},
		{"SetNested", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: bson.D{{Key: "x", Value: "$$nope"}}}}}}},
		{"SetCapitalized", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$Nope"}}}}},
		{"SetEmptyName", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$"}}}}},
		{"SetSystemVariables", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: "$$ROOT.a"}, {Key: "d", Value: "$$CURRENT"}, {Key: "e", Value: "$$REMOVE"}}}}},
		{"SetBoundByLet", bson.D{{Key: "$set", Value: bson.D{{Key: "c", Value: bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "x", Value: int32(1)}}}, {Key: "in", Value: "$$x"}}}}}}}}},
		{"GroupID", bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$$nope"}}}}},
		{"GroupIDRoot", bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$$ROOT"}}}}},
		{"LookupUndefined", bson.D{{Key: "$lookup", Value: bson.D{{Key: "let", Value: bson.D{{Key: "v", Value: "$a"}}},
			{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{bson.D{{Key: "q", Value: int32(1)}}}}}, bson.D{{Key: "$project", Value: bson.D{{Key: "z", Value: "$$v"}, {Key: "y", Value: "$$w"}}}}}}, {Key: "as", Value: "j"}}}}},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    "UndefinedVariables_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage)
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "UndefinedVariables_FindProjection",
		Support: harness.DumboDBFull,
		Setup:   seed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findWithProjection(ctx, col, bson.D{}, bson.D{{Key: "c", Value: "$$nope"}})
		},
	})
}
