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

package fuzz

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

func caseText(c Case) string {
	var b strings.Builder
	for _, cmd := range setupCommands(c.Indexes, c.Docs) {
		b.WriteString(extJSON(cmd, true) + "\n")
	}
	for _, s := range c.Steps {
		b.WriteString(extJSON(s.Command, true) + "\n")
	}
	return b.String()
}

func TestGenerateIsDeterministic(t *testing.T) {
	for _, mode := range []Mode{Reads, Writes} {
		a, b := caseText(Generate(42, mode, 30)), caseText(Generate(42, mode, 30))
		if a != b {
			t.Fatalf("%s: seed 42 generated two different cases", mode)
		}
		if a == caseText(Generate(43, mode, 30)) {
			t.Fatalf("%s: seeds 42 and 43 generated the same case", mode)
		}
	}
}

func TestGenerateShapesSteps(t *testing.T) {
	reads := Generate(7, Reads, 10)
	if len(reads.Steps) != 50 || len(reads.Docs) != 40 {
		t.Fatalf("reads: %d steps, %d docs; want 50, 40", len(reads.Steps), len(reads.Docs))
	}
	writes := Generate(7, Writes, 10)
	if len(writes.Steps) != 10 {
		t.Fatalf("writes: %d steps, want 10", len(writes.Steps))
	}
	names := map[string]bool{}
	for _, ix := range writes.Indexes {
		names[ix.Map()["name"].(string)] = true
	}
	for _, s := range append(reads.Steps, writes.Steps...) {
		if !names[s.Hint] {
			t.Fatalf("step hints %q, which is not a generated index", s.Hint)
		}
		if (s.Check != nil) != (s.Kind != "find" && s.Kind != "find-limit" && s.Kind != "count" && s.Kind != "aggregate" && s.Kind != "distinct") {
			t.Fatalf("%s step: a state check belongs to writes only", s.Kind)
		}
	}
}

func ids(xs ...int32) bson.A {
	out := bson.A{}
	for _, x := range xs {
		out = append(out, bson.D{{Key: "_id", Value: x}})
	}
	return out
}

func TestCompareClassifies(t *testing.T) {
	cases := []struct {
		name         string
		mongo, dumbo Outcome
		want         Class
	}{
		{"same", Outcome{Value: ids(1, 2)}, Outcome{Value: ids(1, 2)}, Match},
		{"order", Outcome{Value: ids(1, 2)}, Outcome{Value: ids(2, 1)}, OrderOnly},
		{"missing", Outcome{Value: ids(1, 2)}, Outcome{Value: ids(1)}, Mismatch},
		{"duplicate", Outcome{Value: ids(1, 2)}, Outcome{Value: ids(1, 1)}, Mismatch},
		{"count", Outcome{Value: bson.D{{Key: "n", Value: int32(2)}}}, Outcome{Value: bson.D{{Key: "n", Value: int32(3)}}}, Mismatch},
		{"numeric type", Outcome{Value: bson.D{{Key: "n", Value: int32(2)}}}, Outcome{Value: bson.D{{Key: "n", Value: int64(2)}}}, Mismatch},
		{"same error", Outcome{Err: mongo.CommandError{Code: 2, Name: "BadValue", Message: "a"}},
			Outcome{Err: mongo.CommandError{Code: 2, Name: "BadValue", Message: "b"}}, Match},
		{"different error", Outcome{Err: mongo.CommandError{Code: 2, Name: "BadValue"}},
			Outcome{Err: mongo.CommandError{Code: 14, Name: "TypeMismatch"}}, Mismatch},
		{"error vs result", Outcome{Value: ids(1)}, Outcome{Err: mongo.CommandError{Code: 2, Name: "BadValue"}}, Mismatch},
		{"unreachable", Outcome{Value: ids(1)}, Outcome{Unreachable: true}, ServerFailure},
	}
	for _, tc := range cases {
		if got, diff := Compare(tc.mongo, tc.dumbo); got != tc.want {
			t.Errorf("%s: class %q (%s), want %q", tc.name, got, diff, tc.want)
		}
	}
}

func TestCompareNamesMissingAndExtraDocuments(t *testing.T) {
	_, diff := Compare(Outcome{Value: ids(1, 2, 2)}, Outcome{Value: ids(2, 3)})
	for _, want := range []string{`lacks {"_id":1}, {"_id":2}`, `adds {"_id":3}`} {
		if !strings.Contains(diff, want) {
			t.Fatalf("diff %q lacks %q", diff, want)
		}
	}
}

func TestCanonicalIgnoresFieldOrderAndServerFields(t *testing.T) {
	a := canonical(bson.D{{Key: "ok", Value: 1.0}, {Key: "n", Value: int32(1)}, {Key: "value", Value: bson.D{{Key: "m", Value: 1}, {Key: "_id", Value: 2}}}})
	b := canonical(bson.D{{Key: "value", Value: bson.D{{Key: "_id", Value: 2}, {Key: "m", Value: 1}}}, {Key: "n", Value: int32(1)}, {Key: "operationTime", Value: primitive.Timestamp{T: 1}}})
	if extJSON(a, true) != extJSON(b, true) {
		t.Fatalf("%s != %s", extJSON(a, true), extJSON(b, true))
	}
}

func TestSignatureGroupsByShapeNotValue(t *testing.T) {
	step := func(filter bson.D) Step { return Step{Kind: "find", Hint: "a1", Filter: filter} }
	gt := func(v any) bson.D { return bson.D{{Key: "a", Value: bson.D{{Key: "$gt", Value: v}}}} }
	if Signature(Mismatch, step(gt(int32(1)))) != Signature(Mismatch, step(gt(2.5))) {
		t.Fatal("two numbers gave different signatures")
	}
	for _, other := range []any{"a", nil, primitive.MinKey{}, primitive.MaxKey{}} {
		if Signature(Mismatch, step(gt(int32(1)))) == Signature(Mismatch, step(gt(other))) {
			t.Fatalf("a number and %v gave the same signature", other)
		}
	}
	if Signature(Mismatch, step(gt(int32(1)))) == Signature(OrderOnly, step(gt(int32(1)))) {
		t.Fatal("classes share a signature")
	}
}

func TestReproRoundTrips(t *testing.T) {
	c := Generate(5, Writes, 3)
	f := Finding{Class: Mismatch, Kind: c.Steps[0].Kind, Hint: c.Steps[0].Hint, Repro: reproCommands(c.Indexes, c.Docs, c.Steps[0])}
	path := filepath.Join(t.TempDir(), "repro.txt")
	if err := os.WriteFile(path, []byte(ReproText(f)), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadRepro(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(f.Repro) {
		t.Fatalf("read %d commands, wrote %d", len(got), len(f.Repro))
	}
	for i := range got {
		if extJSON(got[i], true) != extJSON(f.Repro[i], true) {
			t.Fatalf("command %d changed:\n%s\n%s", i, extJSON(got[i], true), extJSON(f.Repro[i], true))
		}
	}
}

func TestRecorderWritesOneReproPerSignature(t *testing.T) {
	dir := t.TempDir()
	var log strings.Builder
	rec, err := NewRecorder(dir, &log)
	if err != nil {
		t.Fatal(err)
	}
	f := Finding{Signature: "s1", Class: Mismatch}
	if !rec.IsNew("s1") {
		t.Fatal("s1 is new")
	}
	rec.Record(f)
	rec.Record(f)
	rec.Record(Finding{Signature: "s2", Class: OrderOnly})
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	groups := rec.Groups()
	if len(groups) != 2 || groups[0].Signature != "s1" || groups[0].Count != 2 {
		t.Fatalf("groups %+v", groups)
	}
	repros, _ := filepath.Glob(filepath.Join(dir, "repro", "*.txt"))
	lines, _ := os.ReadFile(filepath.Join(dir, "findings.jsonl"))
	if len(repros) != 2 || strings.Count(string(lines), "\n") != 3 {
		t.Fatalf("%d repro files and %d findings lines; want 2 and 3", len(repros), strings.Count(string(lines), "\n"))
	}
}

func TestReportCountsOrderOnlyUnlessIgnored(t *testing.T) {
	groups := []Group{{Signature: "o", Class: OrderOnly, Count: 3}}
	if r := BuildReport(RunSummary{DumboDBVersion: "v"}, groups); !strings.Contains(r.Subject, "1 divergent shapes") {
		t.Fatalf("subject %q", r.Subject)
	}
	if r := BuildReport(RunSummary{DumboDBVersion: "v", IgnoreOrder: true}, groups); !strings.Contains(r.Subject, "no divergences") {
		t.Fatalf("subject with -ignore-order %q", r.Subject)
	}
}
