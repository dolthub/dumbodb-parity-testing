package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// Doc-size scaling benchmarks. These measure how throughput degrades as
// documents grow — a pathology that's invisible in the 100-byte baseline but
// dominates real-world workloads with images, logs, or embedded documents.
//
// We seed fewer docs at larger sizes to keep setup bounded: 1000 × 1MB would
// be a full gigabyte of seed data that nobody benefits from. Per-op cost is
// what we measure, not pool size.

// datasetSizeFor picks a reasonable dataset size for a given document size.
// The product (size × count) stays within a few hundred MB so seed time is
// tolerable even on cold DumboDB.
func datasetSizeFor(size docSize) int {
	switch size {
	case sizeSmall, sizeMedium:
		return 1000
	case sizeLarge:
		return 500
	case size100KB:
		return 100
	case size1MB:
		return 50
	}
	return 100
}

func benchmarkInsertOneAt(b *testing.B, label string, size docSize) {
	col, ctx := withEmptyCollection(b, label)
	r := rand.New(rand.NewSource(*dataSeed))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		doc := makeDoc(r, i, size)
		doc[0].Value = fmt.Sprintf("iter-%d", i)
		if _, err := col.InsertOne(ctx, doc); err != nil {
			b.Fatalf("InsertOne: %v", err)
		}
	}
}

func BenchmarkInsertOne_1KB(b *testing.B)   { benchmarkInsertOneAt(b, "insertone_1kb", sizeMedium) }
func BenchmarkInsertOne_10KB(b *testing.B)  { benchmarkInsertOneAt(b, "insertone_10kb", sizeLarge) }
func BenchmarkInsertOne_100KB(b *testing.B) { benchmarkInsertOneAt(b, "insertone_100kb", size100KB) }
func BenchmarkInsertOne_1MB(b *testing.B)   { benchmarkInsertOneAt(b, "insertone_1mb", size1MB) }

func benchmarkFindOneAt(b *testing.B, label string, size docSize) {
	n := datasetSizeFor(size)
	col, ctx := withSeededCollection(b, label, n, size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % n
		var out bson.M
		if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&out); err != nil {
			b.Fatalf("FindOne: %v", err)
		}
	}
}

func BenchmarkFindOne_1KB(b *testing.B)   { benchmarkFindOneAt(b, "findone_1kb", sizeMedium) }
func BenchmarkFindOne_10KB(b *testing.B)  { benchmarkFindOneAt(b, "findone_10kb", sizeLarge) }
func BenchmarkFindOne_100KB(b *testing.B) { benchmarkFindOneAt(b, "findone_100kb", size100KB) }
func BenchmarkFindOne_1MB(b *testing.B)   { benchmarkFindOneAt(b, "findone_1mb", size1MB) }

// benchmarkUpdateOneSetFieldAt measures a single $set on one small field in
// a document of varying size. The interesting question is how much the
// server must rewrite vs. patch in place as docs grow.
func benchmarkUpdateOneSetFieldAt(b *testing.B, label string, size docSize) {
	n := datasetSizeFor(size)
	col, ctx := withSeededCollection(b, label, n, size)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id := i % n
		_, err := col.UpdateOne(ctx,
			bson.D{{Key: "_id", Value: id}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "touched", Value: i}}}})
		if err != nil {
			b.Fatalf("UpdateOne: %v", err)
		}
	}
}

func BenchmarkUpdateOne_SetField_1KB(b *testing.B) {
	benchmarkUpdateOneSetFieldAt(b, "updset_1kb", sizeMedium)
}
func BenchmarkUpdateOne_SetField_10KB(b *testing.B) {
	benchmarkUpdateOneSetFieldAt(b, "updset_10kb", sizeLarge)
}
func BenchmarkUpdateOne_SetField_100KB(b *testing.B) {
	benchmarkUpdateOneSetFieldAt(b, "updset_100kb", size100KB)
}
func BenchmarkUpdateOne_SetField_1MB(b *testing.B) {
	benchmarkUpdateOneSetFieldAt(b, "updset_1mb", size1MB)
}
