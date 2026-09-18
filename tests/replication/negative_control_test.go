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

//go:build replication

package replication

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func TestControl_ReferenceSecondaryMatchesPrimary(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "Control_ReferenceSecondaryMatchesPrimary",
		Support: harness.DumboDBMongoOnly,
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			return harness.SeedCorruptionCorpus(ctx, primary, "control_corpus")
		},
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			coll := collectionOrFail(t, res.Reference, "control_corpus", "items")
			if len(coll.Documents) != 5 {
				t.Errorf("reference holds %d documents, want 5", len(coll.Documents))
			}
			if len(coll.Indexes) < 2 {
				t.Errorf("reference holds %d indexes, want the _id index plus one more", len(coll.Indexes))
			}
			if len(coll.Options) == 0 {
				t.Error("reference carries no collection options, so the validator corruption would test nothing")
			}
		},
	})
}

func TestControl_ComparatorCatchesSeededCorruption(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "Control_ComparatorCatchesSeededCorruption",
		Support: harness.DumboDBMongoOnly,
		Timeout: 6 * time.Minute,
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			return harness.SeedCorruptionCorpus(ctx, primary, "corruption_corpus")
		},
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			if res.Primary == nil || res.Reference == nil {
				t.Fatal("no captured state")
			}
			if d := harness.DiffServerState(res.Primary, res.Reference); len(d) != 0 {
				t.Fatalf("baseline capture already diverges, so corruption results mean nothing: %v", d)
			}

			for _, c := range harness.Corruptions() {
				t.Run(c.Name, func(t *testing.T) {
					corrupted := captureCopy(t, res.Reference)
					if err := c.Apply(corrupted); err != nil {
						t.Fatalf("could not apply corruption: %v", err)
					}
					d := harness.DiffServerState(res.Primary, corrupted)
					if len(d) == 0 {
						t.Errorf("comparator reported IDENTICAL after injecting %q. %s", c.Name, c.Rationale)
						return
					}
					t.Logf("caught: %s", d[0])
				})
			}
		},
	})
}

func captureCopy(t *testing.T, in *harness.ServerState) *harness.ServerState {
	t.Helper()
	out := &harness.ServerState{Source: in.Source, Databases: map[string]*harness.DatabaseState{}}
	for dbName, db := range in.Databases {
		outDB := &harness.DatabaseState{Collections: map[string]*harness.CollectionState{}}
		for collName, coll := range db.Collections {
			outColl := &harness.CollectionState{
				Options:   append(bson.Raw(nil), coll.Options...),
				Indexes:   map[string]bson.Raw{},
				Documents: map[string]bson.Raw{},
			}
			for k, v := range coll.Indexes {
				outColl.Indexes[k] = append(bson.Raw(nil), v...)
			}
			for k, v := range coll.Documents {
				outColl.Documents[k] = append(bson.Raw(nil), v...)
			}
			outDB.Collections[collName] = outColl
		}
		out.Databases[dbName] = outDB
	}
	return out
}

func collectionOrFail(t *testing.T, state *harness.ServerState, dbName, collName string) *harness.CollectionState {
	t.Helper()
	if state == nil {
		t.Fatal("no captured state")
	}
	db, ok := state.Databases[dbName]
	if !ok {
		t.Fatalf("database %q missing from captured state", dbName)
	}
	coll, ok := db.Collections[collName]
	if !ok {
		t.Fatalf("collection %q missing from captured state", collName)
	}
	return coll
}
