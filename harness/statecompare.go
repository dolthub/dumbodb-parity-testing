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
	"context"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var systemDatabases = map[string]bool{
	"admin":  true,
	"config": true,
	"local":  true,
}

type Divergence struct {
	Path   string
	Detail string
}

func (d Divergence) String() string { return d.Path + ": " + d.Detail }

type ServerState struct {
	Source    string
	Databases map[string]*DatabaseState
}

type DatabaseState struct {
	Collections map[string]*CollectionState
}

type CollectionState struct {
	Options   bson.Raw
	Indexes   map[string]bson.Raw
	Documents map[string]bson.Raw
}

func CaptureServerState(ctx context.Context, cli *mongo.Client, source string) (*ServerState, error) {
	state := &ServerState{Source: source, Databases: map[string]*DatabaseState{}}

	names, err := cli.ListDatabaseNames(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("%s: listDatabases: %w", source, err)
	}
	for _, dbName := range names {
		if systemDatabases[dbName] {
			continue
		}
		db, err := captureDatabase(ctx, cli.Database(dbName), source, dbName)
		if err != nil {
			return nil, err
		}
		state.Databases[dbName] = db
	}
	return state, nil
}

func captureDatabase(ctx context.Context, db *mongo.Database, source, dbName string) (*DatabaseState, error) {
	out := &DatabaseState{Collections: map[string]*CollectionState{}}

	specs, err := db.ListCollectionSpecifications(ctx, bson.D{})
	if err != nil {
		return nil, fmt.Errorf("%s: listCollections %s: %w", source, dbName, err)
	}
	for _, spec := range specs {
		if strings.HasPrefix(spec.Name, "system.") {
			continue
		}
		coll, err := captureCollection(ctx, db.Collection(spec.Name), spec.Options, source, dbName)
		if err != nil {
			return nil, err
		}
		out.Collections[spec.Name] = coll
	}
	return out, nil
}

func captureCollection(ctx context.Context, coll *mongo.Collection, opts bson.Raw, source, dbName string) (*CollectionState, error) {
	ns := dbName + "." + coll.Name()
	out := &CollectionState{
		Options:   opts,
		Indexes:   map[string]bson.Raw{},
		Documents: map[string]bson.Raw{},
	}

	idxCursor, err := coll.Indexes().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: listIndexes %s: %w", source, ns, err)
	}
	var indexes []bson.Raw
	if err := idxCursor.All(ctx, &indexes); err != nil {
		return nil, fmt.Errorf("%s: listIndexes %s: %w", source, ns, err)
	}
	for _, idx := range indexes {
		name, err := idx.LookupErr("name")
		if err != nil {
			return nil, fmt.Errorf("%s: index on %s has no name", source, ns)
		}
		out.Indexes[name.StringValue()] = idx
	}

	docCursor, err := coll.Find(ctx, bson.D{}, options.Find().SetHint(bson.D{{Key: "$natural", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("%s: find %s: %w", source, ns, err)
	}
	var docs []bson.Raw
	if err := docCursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("%s: find %s: %w", source, ns, err)
	}
	for _, doc := range docs {
		id, err := doc.LookupErr("_id")
		if err != nil {
			return nil, fmt.Errorf("%s: document in %s has no _id", source, ns)
		}
		out.Documents[documentKey(id)] = doc
	}
	return out, nil
}

func documentKey(id bson.RawValue) string {
	return fmt.Sprintf("%02x:%s", byte(id.Type), hex.EncodeToString(id.Value))
}

func DiffServerState(want, got *ServerState) []Divergence {
	var out []Divergence

	for _, dbName := range sortedKeys(want.Databases, got.Databases) {
		w, inWant := want.Databases[dbName]
		g, inGot := got.Databases[dbName]
		switch {
		case !inGot:
			out = append(out, Divergence{dbName, fmt.Sprintf("database present on %s, absent on %s", want.Source, got.Source)})
		case !inWant:
			out = append(out, Divergence{dbName, fmt.Sprintf("database present on %s, absent on %s", got.Source, want.Source)})
		default:
			out = append(out, diffDatabase(dbName, w, g, want.Source, got.Source)...)
		}
	}
	return out
}

func diffDatabase(dbName string, want, got *DatabaseState, wantSrc, gotSrc string) []Divergence {
	var out []Divergence
	for _, name := range sortedKeys(want.Collections, got.Collections) {
		ns := dbName + "." + name
		w, inWant := want.Collections[name]
		g, inGot := got.Collections[name]
		switch {
		case !inGot:
			out = append(out, Divergence{ns, fmt.Sprintf("collection present on %s, absent on %s", wantSrc, gotSrc)})
		case !inWant:
			out = append(out, Divergence{ns, fmt.Sprintf("collection present on %s, absent on %s", gotSrc, wantSrc)})
		default:
			out = append(out, diffCollection(ns, w, g, wantSrc, gotSrc)...)
		}
	}
	return out
}

func diffCollection(ns string, want, got *CollectionState, wantSrc, gotSrc string) []Divergence {
	var out []Divergence

	if d := compareRawDocuments(ns+" options", want.Options, got.Options); d != nil {
		out = append(out, *d)
	}

	for _, name := range sortedKeys(want.Indexes, got.Indexes) {
		path := ns + " index " + name
		w, inWant := want.Indexes[name]
		g, inGot := got.Indexes[name]
		switch {
		case !inGot:
			out = append(out, Divergence{path, fmt.Sprintf("index present on %s, absent on %s", wantSrc, gotSrc)})
		case !inWant:
			out = append(out, Divergence{path, fmt.Sprintf("index present on %s, absent on %s", gotSrc, wantSrc)})
		default:
			if d := compareRawDocuments(path, stripIndexVersion(w), stripIndexVersion(g)); d != nil {
				out = append(out, *d)
			}
		}
	}

	for _, key := range sortedKeys(want.Documents, got.Documents) {
		w, inWant := want.Documents[key]
		g, inGot := got.Documents[key]
		path := fmt.Sprintf("%s _id=%s", ns, describeID(w, g))
		switch {
		case !inGot:
			out = append(out, Divergence{path, fmt.Sprintf("document present on %s, absent on %s", wantSrc, gotSrc)})
		case !inWant:
			out = append(out, Divergence{path, fmt.Sprintf("document present on %s, absent on %s", gotSrc, wantSrc)})
		default:
			if d := compareRawDocuments(path, w, g); d != nil {
				out = append(out, *d)
			}
		}
	}
	return out
}

func stripIndexVersion(idx bson.Raw) bson.Raw {
	elems, err := idx.Elements()
	if err != nil {
		return idx
	}
	out := bson.D{}
	for _, e := range elems {
		if e.Key() == "v" {
			continue
		}
		var v interface{}
		if err := e.Value().Unmarshal(&v); err != nil {
			return idx
		}
		out = append(out, bson.E{Key: e.Key(), Value: v})
	}
	raw, err := bson.Marshal(out)
	if err != nil {
		return idx
	}
	return raw
}

func describeID(a, b bson.Raw) string {
	for _, r := range []bson.Raw{a, b} {
		if r == nil {
			continue
		}
		if id, err := r.LookupErr("_id"); err == nil {
			return id.String()
		}
	}
	return "<unknown>"
}

func compareRawDocuments(path string, a, b bson.Raw) *Divergence {
	return compareRawValue(path, bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: a},
		bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: b}, true)
}

func compareRawValue(path string, a, b bson.RawValue, root bool) *Divergence {
	if a.Type != b.Type {
		return &Divergence{path, fmt.Sprintf("type %s != %s (values %s, %s)", a.Type, b.Type, a.String(), b.String())}
	}

	switch a.Type {
	case bson.TypeEmbeddedDocument:
		return compareDocumentValue(path, a, b, root)
	case bson.TypeArray:
		return compareArrayValue(path, a, b)
	default:
		if !a.Equal(b) {
			return &Divergence{path, fmt.Sprintf("%s != %s", a.String(), b.String())}
		}
		return nil
	}
}

func compareDocumentValue(path string, a, b bson.RawValue, root bool) *Divergence {
	da, errA := bson.Raw(a.Value).Elements()
	db, errB := bson.Raw(b.Value).Elements()
	if errA != nil || errB != nil {
		return &Divergence{path, "malformed embedded document"}
	}

	fieldsA := map[string]bson.RawValue{}
	for _, e := range da {
		fieldsA[e.Key()] = e.Value()
	}
	fieldsB := map[string]bson.RawValue{}
	for _, e := range db {
		fieldsB[e.Key()] = e.Value()
	}

	for _, key := range sortedKeys(fieldsA, fieldsB) {
		va, inA := fieldsA[key]
		vb, inB := fieldsB[key]
		child := path + "." + key
		switch {
		case !inB:
			return &Divergence{child, "field missing on the second server"}
		case !inA:
			return &Divergence{child, "field missing on the first server"}
		}
		if root && key == "_id" && va.Type == bson.TypeEmbeddedDocument {
			if !va.Equal(vb) {
				return &Divergence{child, fmt.Sprintf("_id documents differ including field order: %s != %s", va.String(), vb.String())}
			}
			continue
		}
		if d := compareRawValue(child, va, vb, false); d != nil {
			return d
		}
	}
	return nil
}

func compareArrayValue(path string, a, b bson.RawValue) *Divergence {
	ea, errA := bson.Raw(a.Value).Elements()
	eb, errB := bson.Raw(b.Value).Elements()
	if errA != nil || errB != nil {
		return &Divergence{path, "malformed array"}
	}
	if len(ea) != len(eb) {
		return &Divergence{path, fmt.Sprintf("array length %d != %d", len(ea), len(eb))}
	}
	for i := range ea {
		if d := compareRawValue(fmt.Sprintf("%s[%d]", path, i), ea[i].Value(), eb[i].Value(), false); d != nil {
			return d
		}
	}
	return nil
}

func sortedKeys[V any](maps ...map[string]V) []string {
	seen := map[string]struct{}{}
	for _, m := range maps {
		for k := range m {
			seen[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
