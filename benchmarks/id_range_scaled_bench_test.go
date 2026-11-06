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
	"encoding/binary"
	"fmt"
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var idOrderGap = indexUseExpectation{knownDumboGap: "dumbodb#207: _id ranges and sorts scan the whole collection"}

const idPageSize = 100

func intID(i int) interface{} { return i }

// stringID zero-pads so lexical order matches numeric order.
func stringID(i int) interface{} { return fmt.Sprintf("doc-%08d", i) }

// objectIDOf builds ObjectIds that sort in i order and are stable across runs.
func objectIDOf(i int) interface{} {
	var oid primitive.ObjectID
	binary.BigEndian.PutUint32(oid[0:4], 1700000000)
	binary.BigEndian.PutUint64(oid[4:12], uint64(i))
	return oid
}

func withSeededIDCollection(b *testing.B, label string, n int, idOf func(int) interface{}) (*mongo.Collection, context.Context) {
	b.Helper()
	col, ctx := withEmptyCollection(b, label)
	if err := seedWithIds(ctx, col, n, sizeSmall, idOf); err != nil {
		b.Fatalf("seed: %v", err)
	}
	return col, ctx
}

func idRangeFilter(idOf func(int) interface{}, lo, hi int) bson.D {
	return bson.D{{Key: "_id", Value: bson.D{{Key: "$gte", Value: idOf(lo)}, {Key: "$lt", Value: idOf(hi)}}}}
}

func findAll(b *testing.B, ctx context.Context, col *mongo.Collection, filter bson.D, opts ...*options.FindOptions) []bson.M {
	cur, err := col.Find(ctx, filter, opts...)
	if err != nil {
		b.Fatalf("Find: %v", err)
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		b.Fatalf("cursor.All: %v", err)
	}
	return docs
}

// benchmarkIDRangeScaled finds a window of n/100 consecutive _ids, sliding the
// window each iteration so no single range stays hot.
func benchmarkIDRangeScaled(b *testing.B, label string, n int, idOf func(int) interface{}) {
	col, ctx := withSeededIDCollection(b, fmt.Sprintf("idrange_%s_%d", label, n), n, idOf)
	width := n / 100
	probe := idRangeFilter(idOf, n/2, n/2+width)
	requireIndexed(b, ctx, col, findCmd(col, probe, nil, 0), idIndex, n, idOrderGap)
	if got := len(findAll(b, ctx, col, probe)); got != width {
		b.Fatalf("_id range returned %d docs, want %d", got, width)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lo := (i * width) % (n - width)
		findAll(b, ctx, col, idRangeFilter(idOf, lo, lo+width))
	}
}

func BenchmarkFind_IdRange_Int_10K(b *testing.B) { benchmarkIDRangeScaled(b, "int", 10000, intID) }
func BenchmarkFind_IdRange_Int_50K(b *testing.B) { benchmarkIDRangeScaled(b, "int", 50000, intID) }
func BenchmarkFind_IdRange_String_10K(b *testing.B) {
	benchmarkIDRangeScaled(b, "str", 10000, stringID)
}
func BenchmarkFind_IdRange_String_50K(b *testing.B) {
	benchmarkIDRangeScaled(b, "str", 50000, stringID)
}
func BenchmarkFind_IdRange_ObjectId_10K(b *testing.B) {
	benchmarkIDRangeScaled(b, "oid", 10000, objectIDOf)
}
func BenchmarkFind_IdRange_ObjectId_50K(b *testing.B) {
	benchmarkIDRangeScaled(b, "oid", 50000, objectIDOf)
}

func idPage(b *testing.B, ctx context.Context, col *mongo.Collection, after int) []bson.M {
	opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetLimit(idPageSize)
	return findAll(b, ctx, col, bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: after}}}}, opts)
}

// benchmarkIDPaginateScaled times one page of keyset pagination,
// {_id: {$gt: last}} sorted on _id with a limit, walking the collection page by
// page and restarting at the end.
func benchmarkIDPaginateScaled(b *testing.B, n int) {
	col, ctx := withSeededIDCollection(b, fmt.Sprintf("idpage_%d", n), n, intID)
	probe := bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: n / 2}}}}
	requireIndexed(b, ctx, col, findCmd(col, probe, bson.D{{Key: "_id", Value: 1}}, idPageSize), idIndex, n, idOrderGap)
	page := idPage(b, ctx, col, n/2)
	got := make([]interface{}, len(page))
	want := make([]interface{}, idPageSize)
	for j := range page {
		got[j] = page[j]["_id"]
	}
	for j := range want {
		want[j] = int32(n/2 + 1 + j)
	}
	if !reflect.DeepEqual(got, want) {
		b.Fatalf("page after %d returned _ids %v, want %v", n/2, got, want)
	}
	b.ResetTimer()
	last := -1
	for i := 0; i < b.N; i++ {
		page := idPage(b, ctx, col, last)
		if len(page) < idPageSize {
			last = -1
			continue
		}
		last = int(page[len(page)-1]["_id"].(int32))
	}
}

func BenchmarkFind_IdPaginate_10K(b *testing.B) { benchmarkIDPaginateScaled(b, 10000) }
func BenchmarkFind_IdPaginate_50K(b *testing.B) { benchmarkIDPaginateScaled(b, 50000) }
