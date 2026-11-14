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
	"strconv"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Counts over 1,000 documents whose filter compares a field with a document
// of many fields: parsing the filter must stay linear in its size, and each
// document's comparison must stop at the first differing field rather than
// walk the whole operand.

const wideFilterFields = 5000

func runWideFilter(b *testing.B, label string, collation *options.Collation, filter func(wide bson.D) bson.D) {
	col, ctx := withEmptyCollection(b, label)
	if collation != nil {
		if err := col.Database().CreateCollection(ctx, col.Name(), options.CreateCollection().SetCollation(collation)); err != nil {
			b.Fatalf("create collection: %v", err)
		}
	}
	docs := make([]any, 1000)
	for i := range docs {
		docs[i] = bson.D{{Key: "i", Value: int32(i)}, {Key: "d", Value: bson.D{{Key: "k", Value: "x"}, {Key: "n", Value: int32(i)}}}}
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		b.Fatalf("seed: %v", err)
	}
	wide := make(bson.D, wideFilterFields)
	for i := range wide {
		wide[i] = bson.E{Key: "f" + strconv.Itoa(i), Value: "v"}
	}
	f := filter(wide)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := col.CountDocuments(ctx, f); err != nil {
			b.Fatalf("CountDocuments: %v", err)
		}
	}
}

var collatedEnStrength2 = &options.Collation{Locale: "en", Strength: 2}

func BenchmarkWideFilter_Equality(b *testing.B) {
	runWideFilter(b, "wide_eq", nil, func(wide bson.D) bson.D { return bson.D{{Key: "d", Value: wide}} })
}

func BenchmarkWideFilter_In(b *testing.B) {
	runWideFilter(b, "wide_in", nil, func(wide bson.D) bson.D {
		return bson.D{{Key: "d", Value: bson.D{{Key: "$in", Value: bson.A{wide}}}}}
	})
}

func BenchmarkCollated_WideFilter_Equality(b *testing.B) {
	runWideFilter(b, "coll_wide_eq", collatedEnStrength2, func(wide bson.D) bson.D { return bson.D{{Key: "d", Value: wide}} })
}

func BenchmarkCollated_WideFilter_Ne(b *testing.B) {
	runWideFilter(b, "coll_wide_ne", collatedEnStrength2, func(wide bson.D) bson.D {
		return bson.D{{Key: "d", Value: bson.D{{Key: "$ne", Value: wide}}}}
	})
}

func BenchmarkCollated_WideFilter_In(b *testing.B) {
	runWideFilter(b, "coll_wide_in", collatedEnStrength2, func(wide bson.D) bson.D {
		return bson.D{{Key: "d", Value: bson.D{{Key: "$in", Value: bson.A{wide}}}}}
	})
}
