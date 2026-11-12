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

// $$NOW has one value for a whole command, however long it runs. In a
// filter it is a variable only inside $expr.
func TestNowIsConstant(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		docs := make([]interface{}, 3000)
		for i := range docs {
			docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "s", Value: "$$NOW"}}
		}
		_, err := col.InsertMany(ctx, docs)
		return err
	}
	countN := bson.D{{Key: "$count", Value: "n"}}
	cases := map[string]func(ctx context.Context, col *mongo.Collection) (interface{}, error){
		"GroupKey": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$$NOW"}}}}, countN)
		},
		"ProjectThenGroup": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$project", Value: bson.D{{Key: "t", Value: "$$NOW"}}}},
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$t"}}}}, countN)
		},
		"LookupSubPipeline": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{
					bson.D{{Key: "$limit", Value: int32(2)}}, bson.D{{Key: "$project", Value: bson.D{{Key: "t", Value: "$$NOW"}}}},
				}}, {Key: "as", Value: "j"}}}},
				bson.D{{Key: "$unwind", Value: "$j"}}, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$j.t"}}}}, countN)
		},
		"MatchLiteralString": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$match", Value: bson.D{{Key: "s", Value: "$$NOW"}}}}, countN)
		},
		"LiteralString": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(0)}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "t", Value: bson.D{{Key: "$literal", Value: "$$NOW"}}}}}})
		},
		"UpdatePipeline": func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateMany(ctx, bson.D{}, mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "t", Value: "$$NOW"}}}}}); err != nil {
				return nil, err
			}
			return fieldOpsAgg(ctx, col, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$t"}}}}, countN)
		},
	}
	for name, run := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "NowIsConstant_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run:     run,
		})
	}
}
