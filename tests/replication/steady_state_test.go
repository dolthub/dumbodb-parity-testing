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

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const steadyDB = "steady"

func seedTargets(ctx context.Context, primary *mongo.Client, n int) error {
	r := harness.SeedRand(7)
	docs := make([]interface{}, 0, n)
	for i := 0; i < n; i++ {
		docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
	}
	_, err := primary.Database(steadyDB).Collection("workload").InsertMany(ctx, docs)
	return err
}

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
		noops := coverage.NoOps[op.Name]
		if failed+noops == ran {
			t.Errorf("operation %q contributed nothing across %d attempts (%d failed, %d changed nothing); it produced no oplog entry and was not actually compared",
				op.Name, ran, failed, noops)
		}
	}
}

func TestSteady_UpdateOperators(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_UpdateOperators", harness.DumboDBFull, harness.WriteOps(), 3))
}

func TestSteady_ArrayOperators(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_ArrayOperators", harness.DumboDBFull, harness.ArrayOps(), 3))
}

func TestSteady_CatalogOperations(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_CatalogOperations", harness.DumboDBFull, harness.CatalogOps(), 2))
}

func TestSteady_Transactions(t *testing.T) {
	t.Parallel()
	harness.ReplicaTest(t, steadyCase("Steady_Transactions", harness.DumboDBFull, harness.TransactionOps(), 3))
}

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
