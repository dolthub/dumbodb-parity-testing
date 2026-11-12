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

// An accumulator that keeps values fails with ExceededMemoryLimit once they
// pass 100MB, however small the input documents are.
func TestAccumulatorMemoryLimit(t *testing.T) {
	big := bson.D{{Key: "$range", Value: bson.A{int32(0), int32(1000000)}}}
	bigFrom := bson.D{{Key: "$range", Value: bson.A{"$_id", int32(1000000)}}}
	group := func(acc bson.D) []bson.D {
		return []bson.D{{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "f", Value: acc}}}}}
	}
	cases := map[string][]bson.D{
		"Push":     group(bson.D{{Key: "$push", Value: big}}),
		"AddToSet": group(bson.D{{Key: "$addToSet", Value: bigFrom}}),
		"FirstN":   group(bson.D{{Key: "$firstN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "input", Value: big}}}}),
		"LastNFew": group(bson.D{{Key: "$lastN", Value: bson.D{{Key: "n", Value: int32(3)}, {Key: "input", Value: big}}}}),
		"TopN":     group(bson.D{{Key: "$topN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "output", Value: big}, {Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}}}}),
		"MaxN":     group(bson.D{{Key: "$maxN", Value: bson.D{{Key: "n", Value: int32(100)}, {Key: "input", Value: bigFrom}}}}),
		"BucketAutoPush": {{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$_id"}, {Key: "buckets", Value: int32(1)},
			{Key: "output", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$push", Value: big}}}}}}}}},
		"WindowPush": {{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}},
			{Key: "output", Value: bson.D{{Key: "f", Value: bson.D{{Key: "$push", Value: big},
				{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{"unbounded", "current"}}}}}}}}}}}},
	}
	for name, stages := range cases {
		stages := append(stages, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}})
		harness.PairTest(t, harness.TestCase{
			Name:    "AccumulatorMemoryLimit_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				var docs []interface{}
				for i := int32(1); i <= 14; i++ {
					docs = append(docs, bson.D{{Key: "_id", Value: i}})
				}
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
}
