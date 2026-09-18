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

// Tier 2: operations applied to a primary the subject is already following.
//
// Each case is one workload plus the default comparison, which is what makes a
// matrix this size affordable: adding coverage means adding operations, not
// writing assertion logic.
//go:build replication

package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const steadyDB = "steady"

// seedTargets gives the update, array and delete operations documents to match.
// Without it most of the vocabulary matches nothing.
func seedTargets(ctx context.Context, primary *mongo.Client, n int) error {
	r := harness.SeedRand(7)
	docs := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
	}
	_, err := primary.Database(steadyDB).Collection("workload").InsertMany(ctx, docs)
	return err
}

// steadyCase builds a tier 2 case from a slice of the vocabulary.
//
// Coverage is captured per case rather than in a package variable. Two cases
// running at once would otherwise each assert against whichever finished last,
// and the assertion exists precisely to catch a case that exercised nothing.
func steadyCase(name string, support harness.DumboDBSupport, ops []harness.Op, repeat int) harness.ReplicaCase {
	var coverage *harness.Coverage
	return harness.ReplicaCase{
		Name:    name,
		Support: support,
		Timeout: 8 * time.Minute,
		Setup: func(ctx context.Context, primary *mongo.Client) error {
			return seedTargets(ctx, primary, 200)
		},
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			w := harness.Workload{Name: name, Seed: 20260916, Ops: ops, Repeat: repeat}
			cov, err := w.Run(ctx, primary.Database(steadyDB))
			if cov != nil {
				coverage = cov
			}
			return err
		},
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			assertEveryOperationContributed(t, coverage, ops)
		},
	}
}

// A comparison passes trivially for an operation that never produced an oplog
// entry. Without this the case reports coverage it did not have: the array
// positional operators failed on every attempt for a while and the test still
// passed.
func assertEveryOperationContributed(t *testing.T, coverage *harness.Coverage, ops []harness.Op) {
	t.Helper()
	if coverage == nil {
		t.Fatal("workload reported no coverage")
	}
	t.Logf("coverage: %s", coverage)
	for _, op := range ops {
		ran := coverage.Ran[op.Name]
		failed := coverage.Failures[op.Name]
		if ran == 0 {
			t.Errorf("operation %q never ran, so this case proves nothing about it", op.Name)
			continue
		}
		if failed == ran {
			t.Errorf("operation %q failed on all %d attempts; it produced no oplog entry and was not actually compared", op.Name, ran)
		}
	}
}

// Every update operator the design document lists, applied to a primary the
// subject is already following.
func TestSteady_UpdateOperators(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_UpdateOperators", harness.DumboDBFull, harness.WriteOps(), 3))
}

// Array mutation carries the most intricate $v:2 diff encoding. A delta applied
// wrongly here leaves the document subtly different rather than failing, so only
// a byte-exact comparison against a reference notices.
func TestSteady_ArrayOperators(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_ArrayOperators", harness.DumboDBFull, harness.ArrayOps(), 3))
}

// Catalog operations travel the oplog as commands rather than document writes.
func TestSteady_CatalogOperations(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_CatalogOperations", harness.DumboDBFull, harness.CatalogOps(), 2))
}

// Transactions reach the oplog as applyOps entries that may span several
// records. An aborted transaction must leave no trace on either member.
func TestSteady_Transactions(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_Transactions", harness.DumboDBFull, harness.TransactionOps(), 3))
}

// The whole vocabulary at once, interleaved across four concurrent writers.
// Ordering defects need concurrency to surface; a serial workload cannot
// produce them.
func TestSteady_FullVocabularyConcurrent(t *testing.T) {
	t.Parallel()
	var coverage *harness.Coverage
	tc := harness.ReplicaCase{
		Name:    "Steady_FullVocabularyConcurrent",
		Support: harness.DumboDBFull,
		Timeout: 10 * time.Minute,
		Setup: func(ctx context.Context, primary *mongo.Client) error {
			return seedTargets(ctx, primary, 200)
		},
		Workload: func(ctx context.Context, primary *mongo.Client) error {
			w := harness.Workload{
				Name:        "full-concurrent",
				Seed:        20260916,
				Ops:         harness.StandardVocabulary(),
				Repeat:      2,
				Concurrency: 4,
			}
			cov, err := w.Run(ctx, primary.Database(steadyDB))
			if cov != nil {
				coverage = cov
			}
			return err
		},
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			assertEveryOperationContributed(t, coverage, harness.StandardVocabulary())
		},
	}
	harness.ReplicaTest(t, tc)
}
