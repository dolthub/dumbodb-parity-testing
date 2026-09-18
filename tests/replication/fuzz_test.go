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
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const (
	defaultFuzzSeeds   = 3
	defaultFuzzLength  = 40
	defaultFuzzMinutes = 20
	fuzzConvergeWait   = 120 * time.Second
)

func fuzzWeight(name string) int {
	switch {
	case strings.HasPrefix(name, "array-"):
		return 4
	case strings.HasPrefix(name, "update-"), strings.HasPrefix(name, "findOneAndUpdate"),
		name == "updateMany", name == "replaceOne", name == "bulkWrite-mixed":
		return 4
	case strings.HasPrefix(name, "insert"):
		return 2
	case strings.HasPrefix(name, "drop"), strings.HasPrefix(name, "create"),
		strings.HasPrefix(name, "rename"), strings.HasPrefix(name, "collMod"):
		return 1
	default:
		return 2
	}
}

func fuzzPool() []harness.Op {
	pool := make([]harness.Op, 0, 128)
	for _, op := range harness.StandardVocabulary() {
		for i := 0; i < fuzzWeight(op.Name); i++ {
			pool = append(pool, op)
		}
	}
	return pool
}

func fuzzSequence(seed int64, length int) []harness.Op {
	pool := fuzzPool()
	r := rand.New(rand.NewSource(seed))
	sequence := make([]harness.Op, 0, length)
	for i := 0; i < length; i++ {
		sequence = append(sequence, pool[r.Intn(len(pool))])
	}
	return sequence
}

func nonDestructiveSequence(seed int64, length int) []harness.Op {
	pool := make([]harness.Op, 0, 64)
	for _, op := range harness.StandardVocabulary() {
		switch {
		case strings.HasPrefix(op.Name, "drop"), strings.HasPrefix(op.Name, "rename"),
			strings.HasPrefix(op.Name, "create"), strings.HasPrefix(op.Name, "collMod"),
			op.Name == "deleteMany":
			continue
		}
		pool = append(pool, op)
	}
	r := rand.New(rand.NewSource(seed))
	sequence := make([]harness.Op, 0, length)
	for i := 0; i < length; i++ {
		sequence = append(sequence, pool[r.Intn(len(pool))])
	}
	return sequence
}

func opNames(ops []harness.Op) []string {
	names := make([]string, 0, len(ops))
	for _, op := range ops {
		names = append(names, op.Name)
	}
	return names
}

const fuzzTargetCount = 200

func seedFuzzTargets(ctx context.Context, db *mongo.Database) error {
	r := harness.SeedRand(11)
	docs := make([]interface{}, 0, fuzzTargetCount)
	for i := 0; i < fuzzTargetCount; i++ {
		docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
	}
	_, err := db.Collection("workload").InsertMany(ctx, docs)
	return err
}

type fuzzHarness struct {
	rs           *harness.ReplicaSet
	subject      *harness.DumboMember
	commit       string
	primary      *mongo.Client
	trial        int
	lastCompared int
}

func (f *fuzzHarness) run(ctx context.Context, seed int64, ops []harness.Op) ([]harness.Divergence, error) {
	f.trial++
	dbName := fmt.Sprintf("fuzz_%d_%d", seed, f.trial)

	if err := seedFuzzTargets(ctx, f.primary.Database(dbName)); err != nil {
		return nil, fmt.Errorf("seeding the trial: %w", err)
	}
	workload := harness.Workload{Name: dbName, Seed: seed, Ops: ops}
	if _, err := workload.Run(ctx, f.primary.Database(dbName)); err != nil {
		return nil, fmt.Errorf("running the workload: %w", err)
	}
	if _, err := f.rs.WaitConverged(ctx, fuzzConvergeWait, f.subject.Addr); err != nil {
		return []harness.Divergence{{
			Path:   dbName,
			Detail: fmt.Sprintf("subject did not converge: %v", err),
		}}, nil
	}

	subjectState, referenceState, err := f.capture(ctx, dbName)
	if err != nil {
		return nil, err
	}

	documents := countDocuments(referenceState)
	f.lastCompared = documents
	if documents == 0 {
		return nil, fmt.Errorf("the reference secondary holds no documents for %s after %d operations; the trial compared nothing",
			dbName, len(ops))
	}

	divergences := harness.DiffServerState(referenceState, subjectState)

	if err := f.primary.Database(dbName).Drop(ctx); err != nil {
		return divergences, nil
	}
	_, _ = f.rs.WaitConverged(ctx, fuzzConvergeWait, f.subject.Addr)
	return divergences, nil
}

func (f *fuzzHarness) capture(ctx context.Context, dbName string) (*harness.ServerState, *harness.ServerState, error) {
	reference, err := f.rs.AnySecondary(ctx)
	if err != nil {
		return nil, nil, err
	}
	subjectClient, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		return nil, nil, err
	}
	referenceClient, err := f.rs.ClientFor(ctx, reference.Addr)
	if err != nil {
		return nil, nil, err
	}
	subjectState, err := harness.CaptureServerState(ctx, subjectClient, "dumbodb "+f.commit)
	if err != nil {
		return nil, nil, err
	}
	referenceState, err := harness.CaptureServerState(ctx, referenceClient, "reference secondary")
	if err != nil {
		return nil, nil, err
	}
	return narrowTo(subjectState, dbName), narrowTo(referenceState, dbName), nil
}

func countDocuments(state *harness.ServerState) int {
	total := 0
	for _, db := range state.Databases {
		for _, collection := range db.Collections {
			total += len(collection.Documents)
		}
	}
	return total
}

func narrowTo(state *harness.ServerState, dbName string) *harness.ServerState {
	narrowed := &harness.ServerState{Source: state.Source, Databases: map[string]*harness.DatabaseState{}}
	if db, present := state.Databases[dbName]; present {
		narrowed.Databases["trial"] = db
	}
	return narrowed
}

func (f *fuzzHarness) shrink(ctx context.Context, t *testing.T, seed int64, ops []harness.Op, budget int, until time.Time) []harness.Op {
	best := ops
	granularity := 2
	for len(best) > 1 && budget > 0 && time.Now().Before(until) {
		chunk := len(best) / granularity
		if chunk == 0 {
			break
		}
		reduced := false
		for start := 0; start+chunk <= len(best) && budget > 0 && time.Now().Before(until); start += chunk {
			candidate := make([]harness.Op, 0, len(best)-chunk)
			candidate = append(candidate, best[:start]...)
			candidate = append(candidate, best[start+chunk:]...)
			if len(candidate) == 0 {
				continue
			}
			budget--
			divergences, err := f.run(ctx, seed, candidate)
			if err != nil {
				t.Logf("shrink: trial could not be carried out, keeping the larger sequence: %v", err)
				continue
			}
			if len(divergences) > 0 {
				best = candidate
				granularity = 2
				reduced = true
				break
			}
		}
		if !reduced {
			if granularity >= len(best) {
				break
			}
			granularity *= 2
		}
	}
	return best
}

func fuzzEnvInt(name string, fallback int) int {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func TestFuzz_DifferentialConvergence(t *testing.T) {
	t.Parallel()
	seeds := fuzzEnvInt("REPLICATION_FUZZ_SEEDS", defaultFuzzSeeds)
	length := fuzzEnvInt("REPLICATION_FUZZ_LENGTH", defaultFuzzLength)
	base := int64(fuzzEnvInt("REPLICATION_FUZZ_BASE_SEED", 20260917))

	budget := time.Duration(fuzzEnvInt("REPLICATION_FUZZ_BUDGET_MINUTES", defaultFuzzMinutes)) * time.Minute
	deadline := time.Now().Add(budget)
	ctx, cancel := context.WithTimeout(context.Background(), budget+5*time.Minute)
	defer cancel()

	rs := harness.StartReplicaSet(t, 2)
	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	primaryClient, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)
	if commit == "" || commit == "unknown" {
		t.Fatalf("subject reports gitVersion %q; a fuzz failure that cannot name its build is not reproducible", commit)
	}
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 180*time.Second); err != nil {
		t.Fatalf("dumbodb %s did not reach SECONDARY: %v", commit, err)
	}

	f := &fuzzHarness{rs: rs, subject: subject, commit: commit, primary: primaryClient}
	bestCompared := 0
	trialsRun := 0

	for i := 0; i < seeds; i++ {
		if time.Now().After(deadline) {
			t.Logf("budget of %s spent after %d of %d seeds", budget, i, seeds)
			break
		}
		seed := base + int64(i)
		sequence := fuzzSequence(seed, length)

		divergences, err := f.run(ctx, seed, sequence)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		trialsRun++
		if f.lastCompared > bestCompared {
			bestCompared = f.lastCompared
		}
		if len(divergences) == 0 {
			t.Logf("seed %d: %d operations converged (%d documents compared)", seed, length, f.lastCompared)
			continue
		}

		t.Logf("seed %d diverged in %d place(s); shrinking", seed, len(divergences))
		shrinkUntil := deadline.Add(-2 * time.Minute)
		minimal := f.shrink(ctx, t, seed, sequence, 24, shrinkUntil)
		final, runErr := f.run(ctx, seed, minimal)
		if runErr != nil {
			final = divergences
		}

		t.Errorf("dumbodb %s diverged from the reference secondary.\n"+
			"  seed:      %d\n"+
			"  reproduce: REPLICATION_FUZZ_SEEDS=1 REPLICATION_FUZZ_BASE_SEED=%d REPLICATION_FUZZ_LENGTH=%d\n"+
			"  minimal:   %d of %d operations: %s\n"+
			"  divergences:\n    %s",
			commit, seed, seed, length, len(minimal), length,
			strings.Join(opNames(minimal), ", "),
			strings.Join(divergenceStrings(final), "\n    "))
	}

	if trialsRun == 0 {
		t.Fatal("no fuzz trial ran")
	}
	if bestCompared < fuzzCoverageFloor {
		t.Errorf("the best of %d trial(s) compared only %d documents, below the floor of %d. The run reported convergence without exercising anything; check that the trial seeding still matches the ids the vocabulary addresses",
			trialsRun, bestCompared, fuzzCoverageFloor)
	}
}

const fuzzCoverageFloor = 50

func divergenceStrings(divergences []harness.Divergence) []string {
	out := make([]string, 0, len(divergences))
	for i, d := range divergences {
		if i == 12 {
			out = append(out, fmt.Sprintf("... and %d more", len(divergences)-i))
			break
		}
		out = append(out, d.String())
	}
	return out
}

func TestFuzz_SequenceIsReproducible(t *testing.T) {
	t.Parallel()
	first := opNames(fuzzSequence(99, 50))
	second := opNames(fuzzSequence(99, 50))
	if strings.Join(first, ",") != strings.Join(second, ",") {
		t.Fatalf("the same seed produced different sequences:\n  %v\n  %v", first, second)
	}
	other := opNames(fuzzSequence(100, 50))
	if strings.Join(first, ",") == strings.Join(other, ",") {
		t.Fatal("different seeds produced identical sequences; the seed is not reaching the generator")
	}
}

func TestFuzz_DetectsInjectedDivergence(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	rs := harness.StartReplicaSet(t, 2)
	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	primaryClient, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 180*time.Second); err != nil {
		t.Fatalf("dumbodb %s did not reach SECONDARY: %v", commit, err)
	}

	f := &fuzzHarness{rs: rs, subject: subject, commit: commit, primary: primaryClient}

	const seed = 424242
	f.trial++
	dbName := fmt.Sprintf("fuzz_control_%d", f.trial)
	if err := harness.SeedCorruptionCorpus(ctx, primaryClient, dbName); err != nil {
		t.Fatalf("seeding the corruption corpus: %v", err)
	}
	workload := harness.Workload{Name: dbName, Seed: seed, Ops: nonDestructiveSequence(seed, 20)}
	if _, err := workload.Run(ctx, primaryClient.Database(dbName)); err != nil {
		t.Fatalf("running the control workload: %v", err)
	}
	if _, err := rs.WaitConverged(ctx, fuzzConvergeWait, subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", commit, err)
	}

	subjectState, referenceState, err := f.capture(ctx, dbName)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if documents := countDocuments(referenceState); documents == 0 {
		t.Fatalf("the control workload left no documents under %s, so nothing here tests anything", dbName)
	}
	if d := harness.DiffServerState(referenceState, subjectState); len(d) != 0 {
		t.Fatalf("the clean capture already diverges, so corruption results mean nothing: %v", d)
	}

	for _, c := range harness.Corruptions() {
		t.Run(c.Name, func(t *testing.T) {
			corrupted := captureCopy(t, subjectState)
			if err := c.Apply(corrupted); err != nil {
				t.Skipf("corruption %q does not apply to this trial's shape: %v", c.Name, err)
			}
			if !statesDiffer(subjectState, corrupted) {
				t.Fatalf("corruption %q left the captured state byte-identical, so it injected nothing and this case tests nothing. %s",
					c.Name, c.Rationale)
			}
			if d := harness.DiffServerState(referenceState, corrupted); len(d) == 0 {
				t.Errorf("the fuzz comparison reported IDENTICAL after injecting %q, which did change the captured state. %s",
					c.Name, c.Rationale)
				return
			}
		})
	}
}

func statesDiffer(a, b *harness.ServerState) bool {
	if len(a.Databases) != len(b.Databases) {
		return true
	}
	for name, dbA := range a.Databases {
		dbB, present := b.Databases[name]
		if !present || len(dbA.Collections) != len(dbB.Collections) {
			return true
		}
		for collName, collA := range dbA.Collections {
			collB, present := dbB.Collections[collName]
			if !present {
				return true
			}
			if !bytes.Equal(collA.Options, collB.Options) {
				return true
			}
			if mapsDiffer(collA.Documents, collB.Documents) || mapsDiffer(collA.Indexes, collB.Indexes) {
				return true
			}
		}
	}
	return false
}

func mapsDiffer(a, b map[string]bson.Raw) bool {
	if len(a) != len(b) {
		return true
	}
	for key, valueA := range a {
		valueB, present := b[key]
		if !present || !bytes.Equal(valueA, valueB) {
			return true
		}
	}
	return false
}
