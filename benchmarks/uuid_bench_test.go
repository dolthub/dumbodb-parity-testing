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
	"math/rand"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// uuidSubtype is BSON binary subtype 4, the standard UUID representation.
const uuidSubtype = 0x04

const uuidSeededDocs = 50000

func setUUIDVersion(u *[16]byte, version byte) {
	u[6] = u[6]&0x0F | version<<4
	u[8] = u[8]&0x3F | 0x80
}

func uuidString(u [16]byte) string {
	return fmt.Sprintf("%x-%x-%x-%x-%x", u[0:4], u[4:6], u[6:8], u[8:10], u[10:16])
}

func randomUUIDv4(r *rand.Rand) [16]byte {
	var u [16]byte
	r.Read(u[:])
	setUUIDVersion(&u, 4)
	return u
}

// uuidV7Generator yields RFC 9562 version 7 UUIDs that strictly increase: a
// 12-bit counter in rand_a orders UUIDs made in the same millisecond.
type uuidV7Generator struct {
	r      *rand.Rand
	lastMS uint64
	seq    uint16
}

func (g *uuidV7Generator) next() [16]byte {
	ms := uint64(time.Now().UnixMilli())
	if ms > g.lastMS {
		g.lastMS, g.seq = ms, 0
	} else if g.seq++; g.seq > 0x0FFF {
		g.lastMS, g.seq = g.lastMS+1, 0
	}
	var u [16]byte
	g.r.Read(u[8:])
	binary.BigEndian.PutUint64(u[0:8], g.lastMS<<16|uint64(g.seq))
	setUUIDVersion(&u, 7)
	return u
}

// sequentialUUIDOf builds a version 7 UUID whose timestamp is i milliseconds
// past a fixed epoch, so the UUIDs sort in i order and are stable across runs.
func sequentialUUIDOf(i int) interface{} {
	var u [16]byte
	binary.BigEndian.PutUint64(u[0:8], uint64(1700000000000+i)<<16)
	setUUIDVersion(&u, 7)
	return primitive.Binary{Subtype: uuidSubtype, Data: u[:]}
}

func BenchmarkFind_IdRange_UUID_10K(b *testing.B) {
	benchmarkIDRangeScaled(b, "uuid", 10000, sequentialUUIDOf)
}
func BenchmarkFind_IdRange_UUID_50K(b *testing.B) {
	benchmarkIDRangeScaled(b, "uuid", 50000, sequentialUUIDOf)
}

type uuidKind int

const (
	uuidV4Binary uuidKind = iota
	uuidV7Binary
	uuidV4String
)

// uuidSource returns a generator of fresh _ids of kind. v4 is random, so
// inserts land throughout the _id order; v7 increases, so they append.
func uuidSource(kind uuidKind) func() interface{} {
	r := rand.New(rand.NewSource(*dataSeed))
	switch kind {
	case uuidV4Binary:
		return func() interface{} {
			u := randomUUIDv4(r)
			return primitive.Binary{Subtype: uuidSubtype, Data: u[:]}
		}
	case uuidV7Binary:
		g := &uuidV7Generator{r: r}
		return func() interface{} {
			u := g.next()
			return primitive.Binary{Subtype: uuidSubtype, Data: u[:]}
		}
	default:
		return func() interface{} { return uuidString(randomUUIDv4(r)) }
	}
}

func requireIDOfKind(b *testing.B, ctx context.Context, col *mongo.Collection, kind uuidKind) {
	b.Helper()
	var doc bson.Raw
	if err := col.FindOne(ctx, bson.D{}).Decode(&doc); err != nil {
		b.Fatalf("FindOne: %v", err)
	}
	id := doc.Lookup("_id")
	if kind == uuidV4String {
		if s, ok := id.StringValueOK(); !ok || len(s) != 36 {
			b.Fatalf("_id is %v, want a 36-character UUID string", id)
		}
		return
	}
	if subtype, data, ok := id.BinaryOK(); !ok || subtype != uuidSubtype || len(data) != 16 {
		b.Fatalf("_id is %v, want a 16-byte binary subtype 4 UUID", id)
	}
}

// benchmarkInsertManyUUID inserts batches of documents with fresh UUID _ids
// into a collection seeded with uuidSeededDocs documents of the same kind, so
// each insert lands among existing keys. Only the inserted documents are
// cleared, keeping the collection between 50K and 60K documents.
func benchmarkInsertManyUUID(b *testing.B, label string, kind uuidKind, batch int) {
	col, ctx := withEmptyCollection(b, label)
	nextID := uuidSource(kind)
	if err := seedWithIds(ctx, col, uuidSeededDocs, sizeSmall, func(int) interface{} { return nextID() }); err != nil {
		b.Fatalf("seed: %v", err)
	}
	requireIDOfKind(b, ctx, col, kind)
	r := rand.New(rand.NewSource(*dataSeed))
	inserted := bson.D{{Key: "inserted", Value: true}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clearEvery(b, ctx, col, i, insertResetDocs/batch, inserted)
		b.StopTimer()
		docs := make([]interface{}, batch)
		for j := range docs {
			d := makeDoc(r, uuidSeededDocs+i*batch+j, sizeSmall)
			d[0].Value = nextID()
			docs[j] = append(d, inserted[0])
		}
		b.StartTimer()
		if _, err := col.InsertMany(ctx, docs, options.InsertMany().SetOrdered(false)); err != nil {
			b.Fatalf("InsertMany: %v", err)
		}
	}
}

func BenchmarkInsertMany_UUIDv4_50K_100(b *testing.B) {
	benchmarkInsertManyUUID(b, "ins_uuidv4_100", uuidV4Binary, 100)
}
func BenchmarkInsertMany_UUIDv4_50K_1000(b *testing.B) {
	benchmarkInsertManyUUID(b, "ins_uuidv4_1000", uuidV4Binary, 1000)
}
func BenchmarkInsertMany_UUIDv7_50K_100(b *testing.B) {
	benchmarkInsertManyUUID(b, "ins_uuidv7_100", uuidV7Binary, 100)
}
func BenchmarkInsertMany_UUIDv7_50K_1000(b *testing.B) {
	benchmarkInsertManyUUID(b, "ins_uuidv7_1000", uuidV7Binary, 1000)
}
func BenchmarkInsertMany_UUIDv4String_50K_100(b *testing.B) {
	benchmarkInsertManyUUID(b, "ins_uuidv4str_100", uuidV4String, 100)
}
func BenchmarkInsertMany_UUIDv4String_50K_1000(b *testing.B) {
	benchmarkInsertManyUUID(b, "ins_uuidv4str_1000", uuidV4String, 1000)
}
