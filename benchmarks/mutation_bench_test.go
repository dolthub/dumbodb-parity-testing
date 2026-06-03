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
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// The mutation-kind benchmarks stress how the storage layer handles
// changes to container lengths -- arrays growing or shrinking, fields
// being added or removed -- at varying nesting depths. Per the design
// doc, this is where the bson-a (ancestor-patching) vs bson-b
// (envelope-less + read-time materialise) format choice diverges most.
//
// All four operations run only at OOB document sizes (10 KB, 100 KB,
// 1 MB). Inline-sized partial updates rewrite the whole tuple regardless
// of mutation kind and do not exercise the chunk-tree splice path.
//
// Each benchmark seeds n typed-realistic documents (n picked by
// datasetSizeFor for the requested size), then cycles _ids modulo n.
// Iteration cycling means each doc is touched roughly b.N/n times; the
// fixtures carry enough seed array entries and "removable" fields to
// absorb several cycles before the operation degenerates to a no-op.

// updateMutationBenchN picks a dataset size for the mutation benchmarks.
// Smaller than insert/find datasets because each seeded doc costs more
// (typed structure + larger payloads), and the benchmark hammers each
// doc many times.
func updateMutationBenchN(size docSize) int {
	switch size {
	case sizeLarge:
		return 200
	case size100KB:
		return 50
	case size1MB:
		return 20
	}
	return 50
}

// benchmarkArrayExtend runs $push iterations on an array at the requested
// depth. Each iteration appends one element to the array; over b.N
// iterations the targeted arrays accumulate elements.
func benchmarkArrayExtend(b *testing.B, size docSize, depth int) {
	n := updateMutationBenchN(size)
	col, ctx, ids := withSeededTypedRealistic(b, fmt.Sprintf("arrext_%s_d%d", size, depth), n, size)
	path := mutationPath(depth, "target_d"+depthSuffix(depth)+"_arr")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i%n]
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$push", Value: bson.D{{Key: path, Value: int32(i)}}}})
		if err != nil {
			b.Fatalf("UpdateOne $push: %v", err)
		}
	}
}

// benchmarkArrayShorten runs $pop iterations on an array at the requested
// depth. Each iteration removes one element from the array's tail. Once
// the array empties, $pop becomes a no-op; that is part of the cost
// model -- real workloads also do empty-array pops.
func benchmarkArrayShorten(b *testing.B, size docSize, depth int) {
	n := updateMutationBenchN(size)
	col, ctx, ids := withSeededTypedRealistic(b, fmt.Sprintf("arrshrt_%s_d%d", size, depth), n, size)
	path := mutationPath(depth, "target_d"+depthSuffix(depth)+"_arr")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i%n]
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$pop", Value: bson.D{{Key: path, Value: int32(1)}}}})
		if err != nil {
			b.Fatalf("UpdateOne $pop: %v", err)
		}
	}
}

// benchmarkFieldInsert runs $set iterations that add a previously-absent
// field to the document at the requested depth. The new field is uniquely
// named per iteration so each call genuinely inserts rather than updates
// an existing field. Over time the targeted parent document accumulates
// many siblings; this is the natural cost of repeated inserts.
func benchmarkFieldInsert(b *testing.B, size docSize, depth int) {
	n := updateMutationBenchN(size)
	col, ctx, ids := withSeededTypedRealistic(b, fmt.Sprintf("fldins_%s_d%d", size, depth), n, size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i%n]
		path := mutationPath(depth, fmt.Sprintf("ins_%d", i))
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$set", Value: bson.D{{Key: path, Value: int32(i)}}}})
		if err != nil {
			b.Fatalf("UpdateOne $set new field: %v", err)
		}
	}
}

// benchmarkFieldRemove runs $unset iterations against fields that were
// added in a previous benchmarkFieldInsert call OR against the seeded
// target_dN_removable field. To keep the operation non-trivial across
// iterations we use a freshly inserted, uniquely named field on the
// fly: insert then unset, with insertion off the clock.
func benchmarkFieldRemove(b *testing.B, size docSize, depth int) {
	n := updateMutationBenchN(size)
	col, ctx, ids := withSeededTypedRealistic(b, fmt.Sprintf("fldrm_%s_d%d", size, depth), n, size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := ids[i%n]
		fieldName := fmt.Sprintf("rm_%d", i)
		path := mutationPath(depth, fieldName)
		// Insert the field off the clock so the timed call is a pure $unset.
		b.StopTimer()
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$set", Value: bson.D{{Key: path, Value: int32(i)}}}})
		if err != nil {
			b.Fatalf("seed $set for unset: %v", err)
		}
		b.StartTimer()
		_, err = col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$unset", Value: bson.D{{Key: path, Value: ""}}}})
		if err != nil {
			b.Fatalf("UpdateOne $unset: %v", err)
		}
	}
}

// depthSuffix returns the depth marker embedded in the typed-realistic
// fixture's leaf field names (target_d1_arr, target_d3_arr, target_d5_arr).
func depthSuffix(depth int) string {
	return fmt.Sprintf("%d", depth)
}

// The wire benchmarks. Each combines a size (10 KB / 100 KB / 1 MB) with
// a depth (1 / 3 / 5). The bake-off matrix in the design doc enumerates
// the 9 cells per kind; the harness selects which subset to run via the
// standard -bench filter.

func BenchmarkUpdateOne_ArrayExtend_10KB_depth1(b *testing.B) {
	benchmarkArrayExtend(b, sizeLarge, 1)
}
func BenchmarkUpdateOne_ArrayExtend_10KB_depth3(b *testing.B) {
	benchmarkArrayExtend(b, sizeLarge, 3)
}
func BenchmarkUpdateOne_ArrayExtend_10KB_depth5(b *testing.B) {
	benchmarkArrayExtend(b, sizeLarge, 5)
}
func BenchmarkUpdateOne_ArrayExtend_100KB_depth1(b *testing.B) {
	benchmarkArrayExtend(b, size100KB, 1)
}
func BenchmarkUpdateOne_ArrayExtend_100KB_depth3(b *testing.B) {
	benchmarkArrayExtend(b, size100KB, 3)
}
func BenchmarkUpdateOne_ArrayExtend_100KB_depth5(b *testing.B) {
	benchmarkArrayExtend(b, size100KB, 5)
}
func BenchmarkUpdateOne_ArrayExtend_1MB_depth1(b *testing.B) {
	benchmarkArrayExtend(b, size1MB, 1)
}
func BenchmarkUpdateOne_ArrayExtend_1MB_depth3(b *testing.B) {
	benchmarkArrayExtend(b, size1MB, 3)
}
func BenchmarkUpdateOne_ArrayExtend_1MB_depth5(b *testing.B) {
	benchmarkArrayExtend(b, size1MB, 5)
}

func BenchmarkUpdateOne_ArrayShorten_10KB_depth1(b *testing.B) {
	benchmarkArrayShorten(b, sizeLarge, 1)
}
func BenchmarkUpdateOne_ArrayShorten_10KB_depth3(b *testing.B) {
	benchmarkArrayShorten(b, sizeLarge, 3)
}
func BenchmarkUpdateOne_ArrayShorten_10KB_depth5(b *testing.B) {
	benchmarkArrayShorten(b, sizeLarge, 5)
}
func BenchmarkUpdateOne_ArrayShorten_100KB_depth1(b *testing.B) {
	benchmarkArrayShorten(b, size100KB, 1)
}
func BenchmarkUpdateOne_ArrayShorten_100KB_depth3(b *testing.B) {
	benchmarkArrayShorten(b, size100KB, 3)
}
func BenchmarkUpdateOne_ArrayShorten_100KB_depth5(b *testing.B) {
	benchmarkArrayShorten(b, size100KB, 5)
}
func BenchmarkUpdateOne_ArrayShorten_1MB_depth1(b *testing.B) {
	benchmarkArrayShorten(b, size1MB, 1)
}
func BenchmarkUpdateOne_ArrayShorten_1MB_depth3(b *testing.B) {
	benchmarkArrayShorten(b, size1MB, 3)
}
func BenchmarkUpdateOne_ArrayShorten_1MB_depth5(b *testing.B) {
	benchmarkArrayShorten(b, size1MB, 5)
}

func BenchmarkUpdateOne_FieldInsert_10KB_depth1(b *testing.B) {
	benchmarkFieldInsert(b, sizeLarge, 1)
}
func BenchmarkUpdateOne_FieldInsert_10KB_depth3(b *testing.B) {
	benchmarkFieldInsert(b, sizeLarge, 3)
}
func BenchmarkUpdateOne_FieldInsert_10KB_depth5(b *testing.B) {
	benchmarkFieldInsert(b, sizeLarge, 5)
}
func BenchmarkUpdateOne_FieldInsert_100KB_depth1(b *testing.B) {
	benchmarkFieldInsert(b, size100KB, 1)
}
func BenchmarkUpdateOne_FieldInsert_100KB_depth3(b *testing.B) {
	benchmarkFieldInsert(b, size100KB, 3)
}
func BenchmarkUpdateOne_FieldInsert_100KB_depth5(b *testing.B) {
	benchmarkFieldInsert(b, size100KB, 5)
}
func BenchmarkUpdateOne_FieldInsert_1MB_depth1(b *testing.B) {
	benchmarkFieldInsert(b, size1MB, 1)
}
func BenchmarkUpdateOne_FieldInsert_1MB_depth3(b *testing.B) {
	benchmarkFieldInsert(b, size1MB, 3)
}
func BenchmarkUpdateOne_FieldInsert_1MB_depth5(b *testing.B) {
	benchmarkFieldInsert(b, size1MB, 5)
}

func BenchmarkUpdateOne_FieldRemove_10KB_depth1(b *testing.B) {
	benchmarkFieldRemove(b, sizeLarge, 1)
}
func BenchmarkUpdateOne_FieldRemove_10KB_depth3(b *testing.B) {
	benchmarkFieldRemove(b, sizeLarge, 3)
}
func BenchmarkUpdateOne_FieldRemove_10KB_depth5(b *testing.B) {
	benchmarkFieldRemove(b, sizeLarge, 5)
}
func BenchmarkUpdateOne_FieldRemove_100KB_depth1(b *testing.B) {
	benchmarkFieldRemove(b, size100KB, 1)
}
func BenchmarkUpdateOne_FieldRemove_100KB_depth3(b *testing.B) {
	benchmarkFieldRemove(b, size100KB, 3)
}
func BenchmarkUpdateOne_FieldRemove_100KB_depth5(b *testing.B) {
	benchmarkFieldRemove(b, size100KB, 5)
}
func BenchmarkUpdateOne_FieldRemove_1MB_depth1(b *testing.B) {
	benchmarkFieldRemove(b, size1MB, 1)
}
func BenchmarkUpdateOne_FieldRemove_1MB_depth3(b *testing.B) {
	benchmarkFieldRemove(b, size1MB, 3)
}
func BenchmarkUpdateOne_FieldRemove_1MB_depth5(b *testing.B) {
	benchmarkFieldRemove(b, size1MB, 5)
}
