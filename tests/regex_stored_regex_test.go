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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A regex query matches a stored regex identical to it, directly or as an
// array element, besides the strings the pattern matches.

func storedRegexSetup(ctx context.Context, col *mongo.Collection) error {
	re := primitive.Regex{Pattern: "^ab", Options: ""}
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: re}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "s", Value: bson.A{re}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "s", Value: bson.A{"x", re}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "s", Value: bson.A{bson.A{re}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "s", Value: bson.A{primitive.Regex{Pattern: "^ab", Options: "i"}}}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "s", Value: bson.A{"abc"}}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "s", Value: "abc"}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "d", Value: bson.A{bson.D{{Key: "s", Value: re}}}}},
		bson.D{{Key: "_id", Value: int32(9)}, {Key: "s", Value: bson.A{bson.A{"abc"}}}},
	})
	return err
}

func TestFilter_RegexMatchesStoredRegex(t *testing.T) {
	re := primitive.Regex{Pattern: "^ab", Options: ""}
	for _, tc := range []struct {
		name   string
		filter bson.D
	}{
		{"Bare", bson.D{{Key: "s", Value: re}}},
		{"Operator", bson.D{{Key: "s", Value: bson.D{{Key: "$regex", Value: "^ab"}}}}},
		{"OperatorRegexValue", bson.D{{Key: "s", Value: bson.D{{Key: "$regex", Value: re}}}}},
		{"OtherOptions", bson.D{{Key: "s", Value: primitive.Regex{Pattern: "^ab", Options: "i"}}}},
		{"In", bson.D{{Key: "s", Value: bson.D{{Key: "$in", Value: bson.A{re}}}}}},
		{"Nin", bson.D{{Key: "s", Value: bson.D{{Key: "$nin", Value: bson.A{re}}}}}},
		{"Not", bson.D{{Key: "s", Value: bson.D{{Key: "$not", Value: re}}}}},
		{"All", bson.D{{Key: "s", Value: bson.D{{Key: "$all", Value: bson.A{re}}}}}},
		{"Dotted", bson.D{{Key: "d.s", Value: re}}},
		{"ElemMatchInRegex", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$in", Value: bson.A{primitive.Regex{Pattern: "ab"}}}}}}}}},
		{"ElemMatchNotRegex", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$not", Value: primitive.Regex{Pattern: "ab"}}}}}}}},
		{"ElemMatchEqArray", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$eq", Value: bson.A{"abc"}}}}}}}},
		{"ElemMatchInPlain", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$in", Value: bson.A{"abc"}}}}}}}},
		{"ElemMatchNin", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$nin", Value: bson.A{"abc", "x"}}}}}}}},
		{"ElemMatchAll", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$all", Value: bson.A{"abc"}}}}}}}},
		{"ElemMatchNotIn", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$in", Value: bson.A{"abc"}}}}}}}}}},
		{"ElemMatchSize", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$size", Value: int32(1)}}}}}}},
		{"ElemMatchTypeArray", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$type", Value: "array"}}}}}}},
		{"ElemMatchGteArray", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$gte", Value: bson.A{"a"}}}}}}}},
		{"ElemMatch", bson.D{{Key: "s", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$regex", Value: "^ab"}}}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Filter_RegexMatchesStoredRegex_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   storedRegexSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
					cursor, err := col.Find(ctx, tc.filter, opts)
					if err != nil {
						return nil, err
					}
					var results []bson.D
					return results, cursor.All(ctx, &results)
				},
			})
		})
	}
}
