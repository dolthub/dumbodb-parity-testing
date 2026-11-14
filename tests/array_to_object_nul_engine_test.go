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

// TestArrayToObjectNulKeyEngine: a NUL byte in an $arrayToObject key gives
// 4940400/4940401 when the expression folds or runs in the classic engine,
// and 5153207/5153214 when it runs in SBE.
func TestArrayToObjectNulKeyEngine(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}, {Key: "k", Value: "a"}})
		return err
	}
	pair := func(v any) bson.D {
		return bson.D{{Key: "$arrayToObject", Value: bson.A{bson.A{bson.A{"p\x00q", v}}}}}
	}
	kv := func(v any) bson.D {
		return bson.D{{Key: "$arrayToObject", Value: bson.A{bson.A{bson.D{{Key: "k", Value: "p\x00q"}, {Key: "v", Value: v}}}}}}
	}
	cases := map[string]bson.A{
		"group_folded_pair":   {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "o", Value: bson.D{{Key: "$first", Value: pair(1)}}}}}}},
		"group_folded_kv":     {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "o", Value: bson.D{{Key: "$first", Value: kv(1)}}}}}}},
		"project_pair":        {bson.D{{Key: "$project", Value: bson.D{{Key: "o", Value: pair("$k")}}}}},
		"group_id_pair":       {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: pair("$k")}}}}},
		"group_id_kv":         {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: kv("$k")}}}}},
		"group_push_pair":     {bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "o", Value: bson.D{{Key: "$push", Value: pair("$k")}}}}}}},
		"bucket_auto_classic": {bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$x"}, {Key: "buckets", Value: 1}, {Key: "output", Value: bson.D{{Key: "o", Value: bson.D{{Key: "$push", Value: pair("$k")}}}}}}}}},
	}
	for name, pipeline := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "ArrayToObjectNulKeyEngine_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run:     pipelineRun(pipeline),
		})
	}
}
