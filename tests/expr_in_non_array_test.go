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

// TestExprInNonArray: expression $in gives 40081 when its second argument is
// not an array, null and missing included, folded or evaluated per document.
func TestExprInNonArray(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}, {Key: "n", Value: nil}})
		return err
	}
	in := func(arg any) bson.D { return bson.D{{Key: "$in", Value: bson.A{1, arg}}} }
	cases := map[string]bson.A{
		"missing":     {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: in("$nope")}}}}},
		"null_field":  {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: in("$n")}}}}},
		"null_const":  {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: in(nil)}}}}},
		"int_const":   {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: in(5)}}}}},
		"group_field": {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: in("$v")}}}}},
		"array":       {bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: in(bson.A{1, 2})}}}}},
		"match_expr":  {bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: in("$n")}}}}},
	}
	for name, pipeline := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "ExprInNonArray_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
