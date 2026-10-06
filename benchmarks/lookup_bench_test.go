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
)

// Join benchmarks. A fixed set of lookupOrders input documents joins into the
// seeded collection of n documents; every order matches exactly one foreign
// document on the unique field i, so the result is the same at every n and any
// growth with n is the cost of reaching the foreign side.

const lookupOrders = 100

// withLookupCollections seeds the foreign collection (n documents of the
// standard shape) and an "orders" collection in the same database whose ref
// values are spread evenly over the foreign i values.
func withLookupCollections(b *testing.B, label string, n int) (orders, foreign *mongo.Collection, ctx context.Context) {
	b.Helper()
	foreign, ctx = withSeededCollection(b, label, n, sizeSmall)
	orders = foreign.Database().Collection("orders")
	docs := make([]interface{}, 0, lookupOrders)
	for k := 0; k < lookupOrders; k++ {
		docs = append(docs, bson.D{{Key: "_id", Value: k}, {Key: "ref", Value: k * (n / lookupOrders)}})
	}
	if _, err := orders.InsertMany(ctx, docs); err != nil {
		b.Fatalf("seed orders: %v", err)
	}
	return orders, foreign, ctx
}

// runJoin runs pipeline on orders and returns, per order, the size of the
// joined array named field.
func runJoin(b *testing.B, ctx context.Context, orders *mongo.Collection, pipeline bson.A, field string) []int {
	cur, err := orders.Aggregate(ctx, pipeline)
	if err != nil {
		b.Fatalf("Aggregate: %v", err)
	}
	var out []bson.M
	if err := cur.All(ctx, &out); err != nil {
		b.Fatalf("cursor.All: %v", err)
	}
	sizes := make([]int, len(out))
	for i, doc := range out {
		arr, _ := doc[field].(bson.A)
		sizes[i] = len(arr)
	}
	return sizes
}

// assertJoinSizes checks the join once before timing: one output per order,
// each with want joined documents.
func assertJoinSizes(b *testing.B, ctx context.Context, orders *mongo.Collection, pipeline bson.A, field string, want int) {
	b.Helper()
	sizes := runJoin(b, ctx, orders, pipeline, field)
	if len(sizes) != lookupOrders {
		b.Fatalf("join returned %d documents, want %d", len(sizes), lookupOrders)
	}
	for i, got := range sizes {
		if got != want {
			b.Fatalf("order %d joined %d documents, want %d", i, got, want)
		}
	}
}

func benchmarkJoin(b *testing.B, ctx context.Context, orders *mongo.Collection, pipeline bson.A, field string, want int) {
	assertJoinSizes(b, ctx, orders, pipeline, field, want)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJoin(b, ctx, orders, pipeline, field)
	}
}

func equalityLookup(from string) bson.D {
	return bson.D{{Key: "$lookup", Value: bson.D{
		{Key: "from", Value: from}, {Key: "localField", Value: "ref"},
		{Key: "foreignField", Value: "i"}, {Key: "as", Value: "joined"},
	}}}
}

func benchmarkLookupEquality(b *testing.B, n int, indexed bool) {
	label := fmt.Sprintf("lookup_eq_%d", n)
	if indexed {
		label += "_idx"
	}
	orders, foreign, ctx := withLookupCollections(b, label, n)
	if indexed {
		createIndex(b, ctx, foreign, bson.D{{Key: "i", Value: 1}})
	}
	benchmarkJoin(b, ctx, orders, bson.A{equalityLookup(foreign.Name())}, "joined", 1)
}

func BenchmarkLookup_Equality_1K(b *testing.B)          { benchmarkLookupEquality(b, 1000, false) }
func BenchmarkLookup_Equality_1K_Indexed(b *testing.B)  { benchmarkLookupEquality(b, 1000, true) }
func BenchmarkLookup_Equality_10K(b *testing.B)         { benchmarkLookupEquality(b, 10000, false) }
func BenchmarkLookup_Equality_10K_Indexed(b *testing.B) { benchmarkLookupEquality(b, 10000, true) }
func BenchmarkLookup_Equality_50K(b *testing.B)         { benchmarkLookupEquality(b, 50000, false) }
func BenchmarkLookup_Equality_50K_Indexed(b *testing.B) { benchmarkLookupEquality(b, 50000, true) }

// The let/pipeline form of the same join.
func benchmarkLookupPipeline(b *testing.B, n int, indexed bool) {
	label := fmt.Sprintf("lookup_pipe_%d", n)
	if indexed {
		label += "_idx"
	}
	orders, foreign, ctx := withLookupCollections(b, label, n)
	if indexed {
		createIndex(b, ctx, foreign, bson.D{{Key: "i", Value: 1}})
	}
	pipeline := bson.A{bson.D{{Key: "$lookup", Value: bson.D{
		{Key: "from", Value: foreign.Name()},
		{Key: "let", Value: bson.D{{Key: "ref", Value: "$ref"}}},
		{Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$i", "$$ref"}}}}}}},
		}},
		{Key: "as", Value: "joined"},
	}}}}
	benchmarkJoin(b, ctx, orders, pipeline, "joined", 1)
}

func BenchmarkLookup_Pipeline_10K(b *testing.B)         { benchmarkLookupPipeline(b, 10000, false) }
func BenchmarkLookup_Pipeline_10K_Indexed(b *testing.B) { benchmarkLookupPipeline(b, 10000, true) }

// A $lookup inside a $lookup sub-pipeline: each matched foreign document joins
// again into the foreign collection (its grp value names another i). The inner
// stage runs once per outer order.
func benchmarkLookupNested(b *testing.B, n int, indexed bool) {
	label := fmt.Sprintf("lookup_nested_%d", n)
	if indexed {
		label += "_idx"
	}
	orders, foreign, ctx := withLookupCollections(b, label, n)
	if indexed {
		createIndex(b, ctx, foreign, bson.D{{Key: "i", Value: 1}})
	}
	pipeline := bson.A{bson.D{{Key: "$lookup", Value: bson.D{
		{Key: "from", Value: foreign.Name()},
		{Key: "localField", Value: "ref"},
		{Key: "foreignField", Value: "i"},
		{Key: "pipeline", Value: bson.A{
			bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: foreign.Name()}, {Key: "localField", Value: "grp"},
				{Key: "foreignField", Value: "i"}, {Key: "as", Value: "group"},
			}}},
		}},
		{Key: "as", Value: "joined"},
	}}}}
	benchmarkJoin(b, ctx, orders, pipeline, "joined", 1)
}

func BenchmarkLookup_Nested_10K(b *testing.B)         { benchmarkLookupNested(b, 10000, false) }
func BenchmarkLookup_Nested_10K_Indexed(b *testing.B) { benchmarkLookupNested(b, 10000, true) }

// $graphLookup from each order: start at ref, follow grp -> i. Every document
// with i >= 10 reaches its grp document, which links to itself, so each
// traversal visits two documents.
func benchmarkGraphLookup(b *testing.B, n int, indexed bool) {
	label := fmt.Sprintf("graphlookup_%d", n)
	if indexed {
		label += "_idx"
	}
	orders, foreign, ctx := withLookupCollections(b, label, n)
	if indexed {
		createIndex(b, ctx, foreign, bson.D{{Key: "i", Value: 1}})
	}
	pipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "ref", Value: bson.D{{Key: "$gte", Value: 10}}}}}},
		bson.D{{Key: "$graphLookup", Value: bson.D{
			{Key: "from", Value: foreign.Name()}, {Key: "startWith", Value: "$ref"},
			{Key: "connectFromField", Value: "grp"}, {Key: "connectToField", Value: "i"},
			{Key: "as", Value: "chain"},
		}}},
	}
	// The $match drops order 0 (ref 0), so assert on the remaining orders.
	sizes := runJoin(b, ctx, orders, pipeline, "chain")
	if len(sizes) != lookupOrders-1 {
		b.Fatalf("graphLookup returned %d documents, want %d", len(sizes), lookupOrders-1)
	}
	for i, got := range sizes {
		if got != 2 {
			b.Fatalf("order %d chain has %d documents, want 2", i, got)
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runJoin(b, ctx, orders, pipeline, "chain")
	}
}

func BenchmarkGraphLookup_10K(b *testing.B)         { benchmarkGraphLookup(b, 10000, false) }
func BenchmarkGraphLookup_10K_Indexed(b *testing.B) { benchmarkGraphLookup(b, 10000, true) }
