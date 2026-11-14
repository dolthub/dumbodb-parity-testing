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

// TestAlwaysTrueFalse: $alwaysTrue and $alwaysFalse match every document or
// none, and require the number 1.
func TestAlwaysTrueFalse(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(1)}})
		return err
	}
	filters := map[string]string{
		"true":          `{"$alwaysTrue":1}`,
		"false":         `{"$alwaysFalse":1}`,
		"double_one":    `{"$alwaysTrue":1.0}`,
		"zero":          `{"$alwaysTrue":0}`,
		"fraction":      `{"$alwaysTrue":1.5}`,
		"string":        `{"$alwaysTrue":"x"}`,
		"in_or":         `{"$or":[{"$alwaysFalse":1},{"v":1}]}`,
		"with_field":    `{"$alwaysFalse":1,"v":1}`,
		"in_elem_match": `{"v":{"$elemMatch":{"$alwaysTrue":1}}}`,
	}
	for name, f := range filters {
		var filter bson.D
		if err := bson.UnmarshalExtJSON([]byte(f), false, &filter); err != nil {
			t.Fatal(err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "AlwaysTrueFalse_find_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, filter)
				return drainCursor(ctx, cursor, err)
			},
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "AlwaysTrueFalse_count_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return col.CountDocuments(ctx, filter)
			},
		})
	}
}
