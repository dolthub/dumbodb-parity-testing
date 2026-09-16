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

package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// The driver must actually exercise what its operation list claims. An
// operation that always errors contributes nothing to the oplog and would
// otherwise be invisible: coverage would count it as "ran".
func TestWorkload_VocabularyExercisesEveryOperation(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "Workload_VocabularyExercisesEveryOperation",
		Support: harness.DumboDBMongoOnly,
		Timeout: 8 * time.Minute,
		Setup: func(ctx context.Context, primary *mongo.Client) error {
			// Seed documents the update, array and delete operations can match.
			// Without this most of the vocabulary matches nothing and the run
			// reports full coverage having replicated almost nothing.
			docs := make([]interface{}, 0, 200)
			r := harness.SeedRand(1)
			for i := 0; i < 200; i++ {
				docs = append(docs, harness.GenerateDocument(r, existingDocID(i)))
			}
			_, err := primary.Database("wl").Collection("workload").InsertMany(ctx, docs)
			return err
		},
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			w := harness.Workload{
				Name:   "standard",
				Seed:   20260916,
				Ops:    harness.StandardVocabulary(),
				Repeat: 3,
			}
			cov, err := w.Run(ctx, primary.Database("wl"))
			if err != nil {
				return err
			}
			workloadCoverage = cov
			return nil
		},
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			if workloadCoverage == nil {
				t.Fatal("workload reported no coverage")
			}
			t.Logf("coverage: %s", workloadCoverage)

			expected := harness.StandardVocabulary()
			if got, want := len(workloadCoverage.Names()), len(expected); got != want {
				t.Errorf("coverage lists %d operations, vocabulary has %d", got, want)
			}

			// An operation that failed every single time exercised nothing.
			for _, op := range expected {
				ran := workloadCoverage.Ran[op.Name]
				failed := workloadCoverage.Failures[op.Name]
				if ran == 0 {
					t.Errorf("operation %q never ran", op.Name)
					continue
				}
				if failed == ran {
					t.Errorf("operation %q failed on all %d attempts; it contributes nothing to the oplog", op.Name, ran)
				}
			}

			// The workload must have actually moved data onto the reference.
			db, ok := res.Reference.Databases["wl"]
			if !ok {
				t.Fatal("the reference secondary received no workload database")
			}
			if len(db.Collections) < 2 {
				t.Errorf("reference holds %d collections; the catalog operations did not take effect", len(db.Collections))
			}
			coll, ok := db.Collections["workload"]
			if !ok {
				t.Fatal("reference is missing the main workload collection")
			}
			if len(coll.Documents) == 0 {
				t.Error("reference holds no workload documents")
			}
			t.Logf("reference holds %d collections, %d documents in workload",
				len(db.Collections), len(coll.Documents))
		},
	})
}

var workloadCoverage *harness.Coverage

func existingDocID(i int) string {
	return harness.DocIDFor(i)
}

// A workload must replay identically from its seed, or a failing run cannot be
// reproduced.
func TestWorkload_DeterministicFromSeed(t *testing.T) {
	first := generateSequence(20260916)
	second := generateSequence(20260916)
	third := generateSequence(20260917)

	if len(first) != len(second) {
		t.Fatalf("same seed produced %d and %d documents", len(first), len(second))
	}
	for i := range first {
		a, _ := bson.Marshal(first[i])
		b, _ := bson.Marshal(second[i])
		if string(a) != string(b) {
			t.Fatalf("same seed diverged at document %d", i)
		}
	}

	same := 0
	for i := range first {
		a, _ := bson.Marshal(first[i])
		b, _ := bson.Marshal(third[i])
		if string(a) == string(b) {
			same++
		}
	}
	if same == len(first) {
		t.Error("different seeds produced identical documents; the seed is not driving generation")
	}
}

func generateSequence(seed int64) []bson.D {
	r := harness.SeedRand(seed)
	out := make([]bson.D, 0, 50)
	for i := 0; i < 50; i++ {
		out = append(out, harness.GenerateDocument(r, harness.DocIDFor(i)))
	}
	return out
}
