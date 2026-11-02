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

// $rank, $denseRank and range windows read a dotted sortBy path; every
// document read as missing and tied at rank 1 (workspace-utt). Range bounds
// need an ascending sortBy (workspace-nak).
func TestWindowDottedSortBy(t *testing.T) {
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "items", Value: bson.A{
			bson.D{{Key: "qty", Value: int32(5)}}, bson.D{{Key: "qty", Value: int32(3)}}, bson.D{{Key: "qty", Value: int32(9)}},
			bson.D{{Key: "qty", Value: int32(1)}}, bson.D{{Key: "qty", Value: int32(1)}},
		}}})
		return err
	}
	outputs := map[string]bson.D{
		"Rank":      {{Key: "r", Value: bson.D{{Key: "$rank", Value: bson.D{}}}}},
		"DenseRank": {{Key: "d", Value: bson.D{{Key: "$denseRank", Value: bson.D{}}}}},
		"RangeSum": {{Key: "s", Value: bson.D{
			{Key: "$sum", Value: "$items.qty"},
			{Key: "window", Value: bson.D{{Key: "range", Value: bson.A{int32(-2), int32(0)}}}},
		}}},
	}
	for name, output := range outputs {
		for orderName, order := range map[string]int32{"Asc": 1, "Desc": -1} {
			harness.PairTest(t, harness.TestCase{
				Name:    "WindowDottedSortBy_" + name + "_" + orderName,
				Support: harness.DumboDBFull,
				Setup:   setup,
				Run: round6Aggregate(bson.A{
					bson.D{{Key: "$unwind", Value: "$items"}},
					bson.D{{Key: "$setWindowFields", Value: bson.D{
						{Key: "sortBy", Value: bson.D{{Key: "items.qty", Value: order}}},
						{Key: "output", Value: output},
					}}},
					bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}}}},
				}),
			})
		}
	}
}
