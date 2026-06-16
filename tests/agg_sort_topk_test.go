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

func insertSortTopKSeed(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, 250)
	for i := 0; i < 250; i++ {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: int32(i % 17)}}
	}

	_, err := col.InsertMany(ctx, docs)

	return err
}

func TestAggSortTopK_SortLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggSortTopK_SortLimit",
		Support: harness.DumboDBFull,
		Setup:   insertSortTopKSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}}},
				{{Key: "$limit", Value: 10}},
			})

			return docsToSlice(results), err
		},
	})
}

func TestAggSortTopK_SortSkipLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggSortTopK_SortSkipLimit",
		Support: harness.DumboDBFull,
		Setup:   insertSortTopKSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			results, err := runPipeline(ctx, col, []bson.D{
				{{Key: "$sort", Value: bson.D{{Key: "v", Value: -1}, {Key: "_id", Value: 1}}}},
				{{Key: "$skip", Value: 5}},
				{{Key: "$limit", Value: 10}},
			})

			return docsToSlice(results), err
		},
	})
}
