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
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A range filter with an array operand matches an array field when the
// whole array or one of its array elements compares so.
func TestRangeArrayOperand(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{bson.A{"A", "B"}, "m"}}},
			bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: bson.A{"A", "C"}}},
			bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: bson.A{bson.A{"A", "D"}}}},
			bson.D{{Key: "_id", Value: 4}, {Key: "a", Value: bson.A{bson.A{"a", "b"}}}},
			bson.D{{Key: "_id", Value: 5}, {Key: "a", Value: bson.A{}}},
			bson.D{{Key: "_id", Value: 6}, {Key: "a", Value: bson.A{bson.A{}}}},
			bson.D{{Key: "_id", Value: 7}, {Key: "a", Value: bson.A{1, bson.A{2, 3}}}},
			bson.D{{Key: "_id", Value: 8}, {Key: "a", Value: bson.A{bson.A{1}, bson.A{5}}}},
			bson.D{{Key: "_id", Value: 9}, {Key: "a", Value: 5}},
			bson.D{{Key: "_id", Value: 10}, {Key: "a", Value: bson.A{bson.A{bson.A{1}}}}},
			bson.D{{Key: "_id", Value: 11}, {Key: "a", Value: bson.A{bson.D{{Key: "x", Value: 1}}, bson.A{0}}}},
		})
		return err
	}
	operands := map[string]bson.A{
		"strings": {"A", "C"},
		"empty":   {},
		"one":     {2},
		"nested":  {bson.A{1}},
		"pair":    {1, 5},
	}
	for _, op := range []string{"$lt", "$lte", "$gt", "$gte"} {
		for name, operand := range operands {
			filter := bson.D{{Key: "a", Value: bson.D{{Key: op, Value: operand}}}}
			for _, coll := range []string{"simple", "en2"} {
				findOpts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
				if coll == "en2" {
					findOpts.SetCollation(&options.Collation{Locale: "en", Strength: 2})
				}
				harness.PairTest(t, harness.TestCase{
					Name:    fmt.Sprintf("RangeArrayOperand_%s_%s_%s", op[1:], name, coll),
					Support: harness.DumboDBFull,
					Setup:   setup,
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						cur, err := col.Find(ctx, filter, findOpts)
						if err != nil {
							return errOutcome(err), nil
						}
						return collectIDs(ctx, cur)
					},
				})
			}
		}
	}
}
