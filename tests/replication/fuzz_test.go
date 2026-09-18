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

// Tier 6: differential fuzzing over the workload vocabulary.
//
// The target is the $v:2 delta update format. Those defects are silent by
// construction: a delta that applies slightly wrong leaves a document that is
// merely incorrect rather than an apply that fails, so nothing surfaces an
// error and only a byte-exact comparison against a reference notices. The
// nested and array diff space is too large to enumerate by hand.
//
// Every divergence is reported with the seed and with a MINIMAL operation
// sequence, because a hundred-operation repro of a delta bug is not actionable.
//
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

// fuzzWeight decides how often an operation is drawn.
//
// Weighting by name rather than by a hand-kept list means a new operation in
// the vocabulary is drawn automatically at the right rate, instead of being
// silently excluded until someone remembers to add it here.
func fuzzWeight(name string) int {
	switch {
	case strings.HasPrefix(name, "array-"):
		return 4
	case strings.HasPrefix(name, "update-"), strings.HasPrefix(name, "findOneAndUpdate"),
		name == "updateMany", name == "replaceOne", name == "bulkWrite-mixed":
		return 4
	case strings.HasPrefix(name, "insert"):
		return 2
	// Catalog operations drop and recreate the collection. They belong in the
	// stream, but a high weight spends the run rebuilding rather than
	// mutating documents, which is where the delta defects are.
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

// fuzzSequence draws a reproducible operation sequence for a seed.
func fuzzSequence(seed int64, length int) []harness.Op {
	pool := fuzzPool()
	r := rand.New(rand.NewSource(seed))
	sequence := make([]harness.Op, 0, length)
	for i := 0; i < length; i++ {
		sequence = append(sequence, pool[r.Intn(len(pool))])
	}
	return sequence
}

// nonDestructiveSequence draws only operations that mutate documents, never
// ones that drop or rename the collection.
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

// fuzzTargetCount is the seeded range the vocabulary's update, array and
// delete operations address through existingID, which picks an id in
// [doc-000000000, doc-000000199].
//
// Seeding it is not optional. Without it every update and array operation
// matches nothing, so the operators this tier exists to exercise, the $v:2
// delta and array mutation paths, never run at all. Measured before this was
// added: three 40-operation trials left 14, 6 and 3 documents to compare, and
// five of the eleven negative-control corruptions could not even be applied
// for want of a document.
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

// fuzzHarness owns one replica set for the whole run. Starting a set costs
// close to a minute, and shrinking replays a sequence many times, so a set per
// trial would make shrinking unaffordable and the feature would go unused.
type fuzzHarness struct {
	rs      *harness.ReplicaSet
	subject *harness.DumboMember
	commit  string
	primary *mongo.Client
	trial   int
	// lastCompared is how many documents the last trial actually put in front
	// of the comparator, so a passing run states its own weight.
	lastCompared int
}

// run executes one sequence in its own database and reports the divergences
// between the subject and the reference secondary for it.
//
// It returns an error only when the trial could not be carried out. A workload
// operation that fails is not an error: the vocabulary deliberately contains
// operations that fail sometimes, and those still produce oplog activity.
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
		// Non-convergence is graded as a divergence rather than as an
		// apparatus error: a subject that stops keeping up with a workload is
		// exactly as broken as one that stores the wrong bytes.
		return []harness.Divergence{{
			Path:   dbName,
			Detail: fmt.Sprintf("subject did not converge: %v", err),
		}}, nil
	}

	subjectState, referenceState, err := f.capture(ctx, dbName)
	if err != nil {
		return nil, err
	}

	// A trial that compared nothing against nothing would report convergence,
	// and the whole tier would pass forever while testing nothing at all. The
	// way that happens here is narrowTo finding no database under dbName, so
	// assert the reference actually holds what the workload wrote before
	// believing any comparison of it.
	documents := countDocuments(referenceState)
	f.lastCompared = documents
	if documents == 0 {
		return nil, fmt.Errorf("the reference secondary holds no documents for %s after %d operations; the trial compared nothing",
			dbName, len(ops))
	}

	divergences := harness.DiffServerState(referenceState, subjectState)

	// Drop the trial database so the next capture stays cheap and a later
	// trial cannot be blamed for an earlier one's state.
	if err := f.primary.Database(dbName).Drop(ctx); err != nil {
		return divergences, nil
	}
	_, _ = f.rs.WaitConverged(ctx, fuzzConvergeWait, f.subject.Addr)
	return divergences, nil
}

// capture reads both members and narrows each to dbName, so a trial is judged
// only on what it produced.
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
		// Address the database by a fixed key so the comparison does not
		// report every trial's database as added and removed.
		narrowed.Databases["trial"] = db
	}
	return narrowed
}

// shrink reduces a failing sequence to a minimal one that still fails, by
// delta debugging: try ever finer partitions, keep any subsequence that still
// reproduces, stop when removing any single operation makes it pass.
//
// Replaying is not free, so the budget is bounded and the best sequence found
// so far is returned when it runs out. A large repro is worth less than a
// small one but far more than none.
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

// TestFuzz_DifferentialConvergence runs seeded operation streams and requires
// the subject to hold byte-identical state to the reference secondary.
//
// Seeds and length are bounded by default and raised with REPLICATION_FUZZ_SEEDS
// and REPLICATION_FUZZ_LENGTH. The bounded default is deliberate: the owner
// asked for a green check before merging rather than a nightly job against
// main, so this runs in the gated CI at a size that fits, and explores further
// on demand.
func TestFuzz_DifferentialConvergence(t *testing.T) {
	t.Parallel()
	seeds := fuzzEnvInt("REPLICATION_FUZZ_SEEDS", defaultFuzzSeeds)
	length := fuzzEnvInt("REPLICATION_FUZZ_LENGTH", defaultFuzzLength)
	base := int64(fuzzEnvInt("REPLICATION_FUZZ_BASE_SEED", 20260917))

	// A wall-clock budget rather than a per-seed estimate. Shrinking replays a
	// sequence many times, so a single divergence can cost far more than the
	// run that found it. Overrunning the CI job would lose the repro, which is
	// the whole deliverable, so shrinking gives up its remaining budget rather
	// than the test giving up its report.
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
		// Leave a slice of the budget for the final replay and the report.
		shrinkUntil := deadline.Add(-2 * time.Minute)
		minimal := f.shrink(ctx, t, seed, sequence, 24, shrinkUntil)
		final, runErr := f.run(ctx, seed, minimal)
		if runErr != nil {
			final = divergences
		}

		// The repro is the deliverable. A divergence reported without one is
		// a bug nobody can act on.
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

// fuzzCoverageFloor is the smallest number of documents a run may have put in
// front of the comparator and still be believed.
//
// A run that reports convergence having compared almost nothing is the failure
// mode of this tier, and it is silent: the sequences are random, so a
// regression that emptied every trial would look exactly like a clean run. The
// floor is checked against the best trial rather than every trial, because a
// sequence that legitimately ends in dropCollection leaves little behind and
// failing that would be flaky.
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

// TestFuzz_SequenceIsReproducible guards the property the whole tier rests on.
// If a seed did not produce the same sequence twice, every repro line this
// suite prints would be a lie.
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

// TestFuzz_DetectsInjectedDivergence is the negative control for this tier.
//
// A fuzzer that reports convergence on every seed is indistinguishable from a
// fuzzer that compares nothing, and the specific way that happens here is
// narrowTo selecting no database, after which both sides are empty and every
// trial passes forever. So this drives a real trial through the same capture
// and narrowing path the fuzzer uses, then corrupts the result and requires
// each corruption to be caught.
//
// Running it against a real trial rather than a synthetic state is the point:
// it exercises narrowTo, which the existing negative control does not.
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

	// Run a sequence but do not let run() drop the database, so the capture
	// below has something to narrow to. A short sequence is enough; the
	// corruptions supply the differences.
	const seed = 424242
	f.trial++
	dbName := fmt.Sprintf("fuzz_control_%d", f.trial)
	// Seed the corruption corpus rather than the fuzz targets. The corruptions
	// address specific field names, and GenerateDocument produces none of
	// them: retypeInt looks for "n", finds nothing, and returns the document
	// unchanged, after which the comparator correctly reports identical and
	// the control blames it for a difference that was never injected.
	if err := harness.SeedCorruptionCorpus(ctx, primaryClient, dbName); err != nil {
		t.Fatalf("seeding the corruption corpus: %v", err)
	}
	// A non-destructive sequence on top, so the control still exercises a
	// replicated workload rather than a static corpus. A drawn sequence can
	// drop the collection, and then most of the corpus has nothing to corrupt.
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
			// A corruption that silently changed nothing would look exactly
			// like a comparator that missed a real difference, and the second
			// reading is far more alarming than the first. Separate them here
			// rather than leave the reader to guess which one happened.
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

// statesDiffer reports whether two captures hold any different bytes, without
// going through DiffServerState, which is the thing under test here.
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
