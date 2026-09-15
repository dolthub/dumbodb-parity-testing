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

package harness

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func raw(t *testing.T, d bson.D) bson.Raw {
	t.Helper()
	b, err := bson.Marshal(d)
	if err != nil {
		t.Fatalf("marshal %v: %v", d, err)
	}
	return b
}

// Every case here is one the existing CompareResponses reports as Match. If any
// of these regress to "identical", the convergence suite silently stops testing
// anything.
func TestStateCompare_CatchesWhatCompareResponsesMisses(t *testing.T) {
	oidA := primitive.NewObjectID()
	oidB := primitive.NewObjectID()
	t1 := primitive.NewDateTimeFromTime(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	t2 := primitive.NewDateTimeFromTime(time.Date(2031, 6, 6, 0, 0, 0, 0, time.UTC))

	cases := []struct {
		name string
		a, b bson.D
	}{
		{"int32 vs int64", bson.D{{Key: "v", Value: int32(1)}}, bson.D{{Key: "v", Value: int64(1)}}},
		{"int32 vs double", bson.D{{Key: "v", Value: int32(1)}}, bson.D{{Key: "v", Value: float64(1)}}},
		{"int64 vs double", bson.D{{Key: "v", Value: int64(1)}}, bson.D{{Key: "v", Value: float64(1)}}},
		{"different _id", bson.D{{Key: "_id", Value: oidA}}, bson.D{{Key: "_id", Value: oidB}}},
		{"different dates", bson.D{{Key: "d", Value: t1}}, bson.D{{Key: "d", Value: t2}}},
		{"different timestamps",
			bson.D{{Key: "ts", Value: primitive.Timestamp{T: 1, I: 1}}},
			bson.D{{Key: "ts", Value: primitive.Timestamp{T: 99, I: 99}}}},
		{"different uuid binary",
			bson.D{{Key: "u", Value: primitive.Binary{Subtype: 4, Data: []byte("aaaaaaaaaaaaaaaa")}}},
			bson.D{{Key: "u", Value: primitive.Binary{Subtype: 4, Data: []byte("bbbbbbbbbbbbbbbb")}}}},
		{"different string", bson.D{{Key: "v", Value: "x"}}, bson.D{{Key: "v", Value: "y"}}},
		{"null vs missing", bson.D{{Key: "v", Value: nil}}, bson.D{}},
		{"null vs zero", bson.D{{Key: "v", Value: nil}}, bson.D{{Key: "v", Value: int32(0)}}},
		{"empty string vs missing", bson.D{{Key: "v", Value: ""}}, bson.D{}},
		{"bool vs int", bson.D{{Key: "v", Value: true}}, bson.D{{Key: "v", Value: int32(1)}}},
		{"nested type change",
			bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}}}},
			bson.D{{Key: "a", Value: bson.D{{Key: "b", Value: int64(1)}}}}},
		{"array order",
			bson.D{{Key: "a", Value: bson.A{int32(1), int32(2)}}},
			bson.D{{Key: "a", Value: bson.A{int32(2), int32(1)}}}},
		{"array length",
			bson.D{{Key: "a", Value: bson.A{int32(1)}}},
			bson.D{{Key: "a", Value: bson.A{int32(1), int32(2)}}}},
		{"array element type",
			bson.D{{Key: "a", Value: bson.A{int32(1)}}},
			bson.D{{Key: "a", Value: bson.A{int64(1)}}}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if d := compareRawDocuments("doc", raw(t, c.a), raw(t, c.b)); d == nil {
				t.Errorf("reported identical; these documents differ")
			}
		})
	}

	// Assert the contrast this test is named for. If someone ever "simplifies"
	// the convergence path back onto CompareResponses, these cases stop being
	// tested at all and the suite keeps reporting green.
	t.Run("CompareResponses would miss most of these", func(t *testing.T) {
		missed := 0
		for _, c := range cases {
			if CompareResponses(c.a, nil, c.b, nil).Result == Match {
				missed++
			}
		}
		if missed == 0 {
			t.Skip("CompareResponses now distinguishes every case; the separate comparator may no longer be needed")
		}
		t.Logf("CompareResponses reports Match on %d of %d differing documents; "+
			"the convergence comparator must not be built on it", missed, len(cases))
	})
}

// Field order is not significant for ordinary objects, because DumboDB
// canonicalizes key order on write.
func TestStateCompare_IgnoresOrdinaryFieldOrder(t *testing.T) {
	cases := []struct {
		name string
		a, b bson.D
	}{
		{"top level",
			bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}},
			bson.D{{Key: "b", Value: int32(2)}, {Key: "a", Value: int32(1)}}},
		{"nested",
			bson.D{{Key: "x", Value: bson.D{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}}},
			bson.D{{Key: "x", Value: bson.D{{Key: "b", Value: "2"}, {Key: "a", Value: "1"}}}}},
		{"inside an array element",
			bson.D{{Key: "arr", Value: bson.A{bson.D{{Key: "a", Value: "1"}, {Key: "b", Value: "2"}}}}},
			bson.D{{Key: "arr", Value: bson.A{bson.D{{Key: "b", Value: "2"}, {Key: "a", Value: "1"}}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if d := compareRawDocuments("doc", raw(t, c.a), raw(t, c.b)); d != nil {
				t.Errorf("reported a divergence for field order alone: %s", d)
			}
		})
	}
}

// Inside an _id value, field order IS identity: MongoDB treats {a:1,b:2} and
// {b:2,a:1} as two different documents, and DumboDB stores _id verbatim to
// preserve that.
func TestStateCompare_IdFieldOrderIsSignificant(t *testing.T) {
	a := raw(t, bson.D{{Key: "_id", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}}})
	b := raw(t, bson.D{{Key: "_id", Value: bson.D{{Key: "b", Value: int32(2)}, {Key: "a", Value: int32(1)}}}})

	d := compareRawDocuments("doc", a, b)
	if d == nil {
		t.Fatal("reported identical; _id field order distinguishes two genuinely different documents")
	}
	if !contains(d.Detail, "field order") {
		t.Errorf("divergence should name field order as the cause; got %q", d.Detail)
	}
}

// Identical input must compare clean, including the awkward types.
func TestStateCompare_IdenticalDocumentsMatch(t *testing.T) {
	oid := primitive.NewObjectID()
	doc := bson.D{
		{Key: "_id", Value: oid},
		{Key: "i32", Value: int32(7)},
		{Key: "i64", Value: int64(7)},
		{Key: "f", Value: 7.5},
		{Key: "s", Value: "seven"},
		{Key: "b", Value: true},
		{Key: "n", Value: nil},
		{Key: "d", Value: primitive.NewDateTimeFromTime(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))},
		{Key: "ts", Value: primitive.Timestamp{T: 3, I: 4}},
		{Key: "bin", Value: primitive.Binary{Subtype: 0, Data: []byte{1, 2, 3}}},
		{Key: "arr", Value: bson.A{int32(1), "two", bson.D{{Key: "k", Value: "v"}}}},
		{Key: "sub", Value: bson.D{{Key: "deep", Value: bson.D{{Key: "deeper", Value: int32(1)}}}}},
	}
	if d := compareRawDocuments("doc", raw(t, doc), raw(t, doc)); d != nil {
		t.Fatalf("identical documents reported as divergent: %s", d)
	}
}

// The divergence must locate itself; a bare "documents differ" would mean
// re-running everything by hand to find out where.
func TestStateCompare_DivergencePathLocatesTheField(t *testing.T) {
	a := raw(t, bson.D{{Key: "outer", Value: bson.D{{Key: "inner", Value: bson.A{int32(1), int32(2)}}}}})
	b := raw(t, bson.D{{Key: "outer", Value: bson.D{{Key: "inner", Value: bson.A{int32(1), int32(9)}}}}})

	d := compareRawDocuments("mydb.mycoll", a, b)
	if d == nil {
		t.Fatal("reported identical")
	}
	if want := "mydb.mycoll.outer.inner[1]"; d.Path != want {
		t.Errorf("path = %q, want %q", d.Path, want)
	}
}

func TestStateCompare_StripIndexVersion(t *testing.T) {
	withV := raw(t, bson.D{{Key: "v", Value: int32(2)}, {Key: "name", Value: "a_1"}, {Key: "key", Value: bson.D{{Key: "a", Value: int32(1)}}}})
	otherV := raw(t, bson.D{{Key: "v", Value: int32(9)}, {Key: "name", Value: "a_1"}, {Key: "key", Value: bson.D{{Key: "a", Value: int32(1)}}}})

	if d := compareRawDocuments("idx", stripIndexVersion(withV), stripIndexVersion(otherV)); d != nil {
		t.Errorf("index version should not count as a divergence: %s", d)
	}

	differentKey := raw(t, bson.D{{Key: "v", Value: int32(2)}, {Key: "name", Value: "a_1"}, {Key: "key", Value: bson.D{{Key: "b", Value: int32(1)}}}})
	if d := compareRawDocuments("idx", stripIndexVersion(withV), stripIndexVersion(differentKey)); d == nil {
		t.Error("a different index key must be a divergence")
	}
}

// Structural differences above the document level.
func TestStateCompare_StructuralDifferences(t *testing.T) {
	doc := raw(t, bson.D{{Key: "_id", Value: int32(1)}})
	key := documentKey(bson.RawValue{Type: bson.TypeInt32, Value: doc.Lookup("_id").Value})

	base := func() *ServerState {
		return &ServerState{
			Source: "reference",
			Databases: map[string]*DatabaseState{
				"app": {Collections: map[string]*CollectionState{
					"items": {
						Options:   raw(t, bson.D{}),
						Indexes:   map[string]bson.Raw{"_id_": raw(t, bson.D{{Key: "name", Value: "_id_"}})},
						Documents: map[string]bson.Raw{key: doc},
					},
				}},
			},
		}
	}

	t.Run("missing database", func(t *testing.T) {
		got := base()
		got.Source = "subject"
		delete(got.Databases, "app")
		if len(DiffServerState(base(), got)) == 0 {
			t.Error("a missing database must be a divergence")
		}
	})

	t.Run("missing collection", func(t *testing.T) {
		got := base()
		got.Source = "subject"
		delete(got.Databases["app"].Collections, "items")
		if len(DiffServerState(base(), got)) == 0 {
			t.Error("a missing collection must be a divergence")
		}
	})

	t.Run("missing index", func(t *testing.T) {
		got := base()
		got.Source = "subject"
		delete(got.Databases["app"].Collections["items"].Indexes, "_id_")
		if len(DiffServerState(base(), got)) == 0 {
			t.Error("a missing index must be a divergence")
		}
	})

	t.Run("missing document", func(t *testing.T) {
		got := base()
		got.Source = "subject"
		delete(got.Databases["app"].Collections["items"].Documents, key)
		if len(DiffServerState(base(), got)) == 0 {
			t.Error("a missing document must be a divergence")
		}
	})

	t.Run("extra document", func(t *testing.T) {
		got := base()
		got.Source = "subject"
		extra := raw(t, bson.D{{Key: "_id", Value: int32(2)}})
		got.Databases["app"].Collections["items"].Documents["extra"] = extra
		if len(DiffServerState(base(), got)) == 0 {
			t.Error("an extra document must be a divergence")
		}
	})

	t.Run("identical states match", func(t *testing.T) {
		if d := DiffServerState(base(), base()); len(d) != 0 {
			t.Errorf("identical states reported %d divergences: %v", len(d), d)
		}
	})
}
