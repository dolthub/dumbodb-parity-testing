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

// TestFacetReadOnlyThenModifying: a $facet branch that changes documents
// does not change what an earlier read-only branch returned.
func TestFacetReadOnlyThenModifying(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}, {Key: "o", Value: bson.D{{Key: "x", Value: int32(1)}}}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(2)}, {Key: "o", Value: bson.D{{Key: "x", Value: int32(2)}}}},
		})
		return err
	}
	cases := map[string]string{
		"limit_then_set":         `{"a":[{"$limit":2}],"b":[{"$set":{"v":99,"o.x":7}}]}`,
		"limit_then_add_fields":  `{"a":[{"$limit":2}],"b":[{"$addFields":{"o.w":5}}]}`,
		"match_unwind_then_set":  `{"a":[{"$match":{}}],"b":[{"$unwind":"$o"}],"c":[{"$set":{"v":0}}]}`,
		"sort_then_replace_with": `{"a":[{"$sort":{"_id":-1}}],"b":[{"$replaceWith":{"$mergeObjects":["$$ROOT",{"v":5}]}}]}`,
	}
	for name, facet := range cases {
		var f bson.D
		if err := bson.UnmarshalExtJSON([]byte(facet), false, &f); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "FacetReadOnlyThenModifying_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$facet", Value: f}},
			}),
		})
	}
}
