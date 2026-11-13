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

var caseInsensitive = &options.Collation{Locale: "en", Strength: 2}

// TestCollationArrayEquality: equality, $in, $ne, $nin and $all compare the
// elements of an array field under the collation.
func TestCollationArrayEquality(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "t", Value: bson.A{"A", "b"}}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "t", Value: "A"}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "t", Value: bson.A{"x"}}},
		})
		return err
	}
	filters := map[string]bson.D{
		"eq":  {{Key: "t", Value: "a"}},
		"in":  {{Key: "t", Value: bson.D{{Key: "$in", Value: bson.A{"a"}}}}},
		"ne":  {{Key: "t", Value: bson.D{{Key: "$ne", Value: "a"}}}},
		"nin": {{Key: "t", Value: bson.D{{Key: "$nin", Value: bson.A{"a"}}}}},
		"all": {{Key: "t", Value: bson.D{{Key: "$all", Value: bson.A{"a", "B"}}}}},
	}
	for name, filter := range filters {
		harness.PairTest(t, harness.TestCase{
			Name:    "CollationArrayEquality_find_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, filter, options.Find().SetCollation(caseInsensitive).SetSort(bson.D{{Key: "_id", Value: 1}}))
				return drainCursor(ctx, cursor, err)
			},
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "CollationArrayEquality_count_" + name,
			Support: harness.DumboDBFull,
			Setup:   setup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return col.CountDocuments(ctx, filter, options.Count().SetCollation(caseInsensitive))
			},
		})
	}
}
