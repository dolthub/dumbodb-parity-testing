// Copyright 2026 Dolthub, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0

package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Typed-doc benchmarks exercise the wire paths with documents whose
// fields are typed (ObjectId, Date, int32, nested objects, arrays) -- the
// shape real Mongo workloads tend to have. The baseline ExtJSON encoder
// wraps each typed value ({"$oid": "..."}, {"$date": ...}, etc.), and
// these benchmarks make that wrapping cost visible. The bake-off compares
// the same benchmarks against bson-a and bson-b where the wrappers are
// gone.

func benchmarkInsertOneTypedRealisticAt(b *testing.B, size docSize) {
	col, ctx := withEmptyCollection(b, fmt.Sprintf("ins_typedreal_%s", size))
	r := rand.New(rand.NewSource(*dataSeed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := makeTypedRealisticDoc(r, i, size)
		if _, err := col.InsertOne(ctx, doc); err != nil {
			b.Fatalf("InsertOne: %v", err)
		}
	}
}

func BenchmarkInsertOne_TypedRealistic_Small(b *testing.B) {
	benchmarkInsertOneTypedRealisticAt(b, sizeSmall)
}
func BenchmarkInsertOne_TypedRealistic_1KB(b *testing.B) {
	benchmarkInsertOneTypedRealisticAt(b, sizeMedium)
}
func BenchmarkInsertOne_TypedRealistic_10KB(b *testing.B) {
	benchmarkInsertOneTypedRealisticAt(b, sizeLarge)
}
func BenchmarkInsertOne_TypedRealistic_100KB(b *testing.B) {
	benchmarkInsertOneTypedRealisticAt(b, size100KB)
}
func BenchmarkInsertOne_TypedRealistic_1MB(b *testing.B) {
	benchmarkInsertOneTypedRealisticAt(b, size1MB)
}

func benchmarkFindOneTypedRealisticAt(b *testing.B, size docSize) {
	n := datasetSizeFor(size)
	col, ctx, ids := withSeededTypedRealistic(b, fmt.Sprintf("find_typedreal_%s", size), n, size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i%n]
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

func BenchmarkFindOne_TypedRealistic_Small(b *testing.B) {
	benchmarkFindOneTypedRealisticAt(b, sizeSmall)
}
func BenchmarkFindOne_TypedRealistic_1KB(b *testing.B) {
	benchmarkFindOneTypedRealisticAt(b, sizeMedium)
}
func BenchmarkFindOne_TypedRealistic_10KB(b *testing.B) {
	benchmarkFindOneTypedRealisticAt(b, sizeLarge)
}
func BenchmarkFindOne_TypedRealistic_100KB(b *testing.B) {
	benchmarkFindOneTypedRealisticAt(b, size100KB)
}
func BenchmarkFindOne_TypedRealistic_1MB(b *testing.B) {
	benchmarkFindOneTypedRealisticAt(b, size1MB)
}

// Aggregate-filter benchmark: full-collection scan + equality filter on
// a typed field. This exercises the prefilter pushdown path
// (extJSONFieldPatterns on main; the new BSON-element prefilter on
// bson-a and bson-b).
func BenchmarkAggregateFilter_TypedRealistic_10KB(b *testing.B) {
	n := datasetSizeFor(sizeLarge)
	col, ctx, _ := withSeededTypedRealistic(b, "aggfilter_typedreal_10kb", n, sizeLarge)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Filter on version (a top-level int32). Matches all docs since
		// seed sets version=1 uniformly, so the prefilter has to scan
		// every doc and the result set size is constant.
		cur, err := col.Aggregate(ctx, bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "version", Value: int32(1)}}}},
		})
		if err != nil {
			b.Fatalf("Aggregate: %v", err)
		}
		var out []bson.M
		if err := cur.All(ctx, &out); err != nil {
			b.Fatalf("cursor.All: %v", err)
		}
	}
}

// Insert / find benchmarks against the typed-extreme fixture. Used to
// stress the ExtJSON wrapping tax on the baseline (ten ObjectIds and
// ten Dates at the top level), which the bson-a / bson-b branches drop
// entirely.
func benchmarkInsertOneTypedExtremeAt(b *testing.B, size docSize) {
	col, ctx := withEmptyCollection(b, fmt.Sprintf("ins_typedxtm_%s", size))
	r := rand.New(rand.NewSource(*dataSeed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := makeTypedExtremeDoc(r, i, size)
		if _, err := col.InsertOne(ctx, doc); err != nil {
			b.Fatalf("InsertOne: %v", err)
		}
	}
}

func BenchmarkInsertOne_TypedExtreme_Small(b *testing.B) {
	benchmarkInsertOneTypedExtremeAt(b, sizeSmall)
}
func BenchmarkInsertOne_TypedExtreme_10KB(b *testing.B) {
	benchmarkInsertOneTypedExtremeAt(b, sizeLarge)
}
