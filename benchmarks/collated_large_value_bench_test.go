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

package benchmarks

import (
	"context"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Aggregations over a collection with a default collation, where one document
// holds a large array of strings: the collation key of a large kept or sorted
// value must be computed once, not on every comparison.

const collatedLargeArrayLen = 20000

func withCollatedLargeValue(b *testing.B, label string) (*mongo.Collection, context.Context) {
	b.Helper()
	col, ctx := withEmptyCollection(b, label)
	opts := options.CreateCollection().SetCollation(&options.Collation{Locale: "en"})
	if err := col.Database().CreateCollection(ctx, col.Name(), opts); err != nil {
		b.Fatalf("create collection: %v", err)
	}
	big := make(bson.A, collatedLargeArrayLen)
	for i := range big {
		big[i] = fmt.Sprintf("abcdefgh%d", i)
	}
	docs := []any{bson.D{{Key: "i", Value: int32(0)}, {Key: "v", Value: big}}}
	for i := 1; i <= 2000; i++ {
		docs = append(docs, bson.D{{Key: "i", Value: int32(i)}, {Key: "v", Value: "x"}})
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		b.Fatalf("seed: %v", err)
	}
	return col, ctx
}

func runCollatedLargeValue(b *testing.B, label string, pipeline bson.A) {
	col, ctx := withCollatedLargeValue(b, label)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cur, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

func BenchmarkCollated_GroupMaxLargeValue(b *testing.B) {
	runCollatedLargeValue(b, "coll_group_max", bson.A{
		bson.D{{Key: "$sort", Value: bson.D{{Key: "i", Value: 1}}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "m", Value: bson.D{{Key: "$max", Value: "$v"}}}}}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "n", Value: bson.D{{Key: "$size", Value: "$m"}}}}}},
	})
}

func BenchmarkCollated_SortLimitLargeValue(b *testing.B) {
	runCollatedLargeValue(b, "coll_sort_limit", bson.A{
		bson.D{{Key: "$sort", Value: bson.D{{Key: "v", Value: 1}}}},
		bson.D{{Key: "$limit", Value: 1}},
		bson.D{{Key: "$project", Value: bson.D{{Key: "i", Value: 1}}}},
	})
}
