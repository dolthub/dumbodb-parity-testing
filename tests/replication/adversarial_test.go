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

// Tier 3: correctness under interruption.
//
// Everything proven so far assumes nothing goes wrong mid-flight. These cases
// break the subject on purpose and then require it to reach exactly the state a
// stock secondary reached, with no gap and no double-apply.
//go:build replication

package replication

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const advDB = "adversarial"

// advApparatus provisions a set, seeds it, and joins a caught-up subject.
func advApparatus(t *testing.T, seed int) (*harness.ReplicaSet, *harness.DumboMember, context.Context, context.CancelFunc) {
	t.Helper()
	rs := harness.StartReplicaSet(t, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)

	primary, err := rs.Primary(ctx)
	if err != nil {
		cancel()
		t.Fatalf("Primary: %v", err)
	}
	cli, err := rs.DirectClient(ctx, primary)
	if err != nil {
		cancel()
		t.Fatalf("primary client: %v", err)
	}
	r := harness.SeedRand(int64(seed))
	docs := make([]interface{}, 0, 200)
	for i := 0; i < 200; i++ {
		docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
	}
	if _, err := cli.Database(advDB).Collection("workload").InsertMany(ctx, docs); err != nil {
		_ = cli.Disconnect(context.Background())
		cancel()
		t.Fatalf("seeding: %v", err)
	}
	_ = cli.Disconnect(context.Background())

	subject := rs.JoinDumboDB(t)
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 150*time.Second); err != nil {
		cancel()
		t.Fatalf("subject never reached SECONDARY: %v", err)
	}
	if _, err := rs.WaitConverged(ctx, 120*time.Second, subject.Addr); err != nil {
		cancel()
		t.Fatalf("subject did not converge before the interruption: %v", err)
	}
	return rs, subject, ctx, cancel
}

// compareWithReference requires the subject to hold exactly what the reference
// secondary holds.
func compareWithReference(t *testing.T, ctx context.Context, rs *harness.ReplicaSet, subject *harness.DumboMember, stage string) {
	t.Helper()

	if _, err := rs.WaitConverged(ctx, 150*time.Second, subject.Addr); err != nil {
		t.Fatalf("%s: subject did not converge: %v", stage, err)
	}
	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("%s: no reference secondary: %v", stage, err)
	}
	if reference.Addr == subject.Addr {
		t.Fatalf("%s: the subject was selected as its own reference", stage)
	}

	refState := captureMember(t, ctx, rs, reference.Addr, "reference")
	subState := captureMember(t, ctx, rs, subject.Addr, "subject")

	if d := harness.DiffServerState(refState, subState); len(d) > 0 {
		t.Errorf("%s: subject diverged from the reference secondary in %d place(s); first: %s",
			stage, len(d), d[0])
	}
}

func captureMember(t *testing.T, ctx context.Context, rs *harness.ReplicaSet, addr, role string) *harness.ServerState {
	t.Helper()
	cli, err := rs.ClientFor(ctx, addr)
	if err != nil {
		t.Fatalf("%s client: %v", role, err)
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()
	state, err := harness.CaptureServerState(ctx, cli, role)
	if err != nil {
		t.Fatalf("capturing %s: %v", role, err)
	}
	return state
}

func drive(ctx context.Context, rs *harness.ReplicaSet, seed int64, repeat int) error {
	primary, err := rs.Primary(ctx)
	if err != nil {
		return err
	}
	cli, err := rs.DirectClient(ctx, primary)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	w := harness.Workload{Name: "adversarial", Seed: seed, Ops: harness.StandardVocabulary(), Repeat: repeat}
	_, err = w.Run(ctx, cli.Database(advDB))
	return err
}

// A hard kill mid-stream must not lose or duplicate anything. The subject
// resumes from its durable checkpoint and ends where the reference ended.
func TestAdversarial_HardKillMidStream(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 101)
	defer cancel()

	// Write while the subject is running, kill it part-way, keep writing, then
	// bring it back. It must catch up on both halves.
	if err := drive(ctx, rs, 1, 1); err != nil {
		t.Fatalf("first workload: %v", err)
	}
	subject.Kill()
	if err := drive(ctx, rs, 2, 1); err != nil {
		t.Fatalf("workload while the subject was down: %v", err)
	}
	subject.Start()
	if err := drive(ctx, rs, 3, 1); err != nil {
		t.Fatalf("workload after restart: %v", err)
	}

	compareWithReference(t, ctx, rs, subject, "after a hard kill mid-stream")
}

// A graceful restart exercises the durable-shutdown path rather than crash
// recovery, and must be equally lossless.
func TestAdversarial_GracefulRestartMidStream(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 102)
	defer cancel()

	if err := drive(ctx, rs, 11, 1); err != nil {
		t.Fatalf("first workload: %v", err)
	}
	subject.Stop()
	if err := drive(ctx, rs, 12, 1); err != nil {
		t.Fatalf("workload while the subject was down: %v", err)
	}
	subject.Start()
	if err := drive(ctx, rs, 13, 1); err != nil {
		t.Fatalf("workload after restart: %v", err)
	}

	compareWithReference(t, ctx, rs, subject, "after a graceful restart mid-stream")
}

// Repeated kills while writes continue. One restart can succeed by luck; a
// sequence is what exposes an off-by-one in checkpoint resumption.
func TestAdversarial_RepeatedKillsUnderLoad(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 103)
	defer cancel()

	for round := 0; round < 4; round++ {
		if err := drive(ctx, rs, int64(20+round), 1); err != nil {
			t.Fatalf("round %d workload: %v", round, err)
		}
		subject.Kill()
		if err := drive(ctx, rs, int64(30+round), 1); err != nil {
			t.Fatalf("round %d workload while down: %v", round, err)
		}
		subject.Start()
	}

	compareWithReference(t, ctx, rs, subject, "after four kill/restart rounds")
}

// Stepping down the primary moves the subject to a new sync source. Continuity
// must be preserved: no lost writes, no forked history.
func TestAdversarial_PrimaryStepDown(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 104)
	defer cancel()

	if err := drive(ctx, rs, 41, 1); err != nil {
		t.Fatalf("workload before step-down: %v", err)
	}

	old, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	fresh, err := rs.StepDownPrimary(ctx, 30)
	if err != nil {
		t.Fatalf("StepDownPrimary: %v", err)
	}
	t.Logf("primary moved %s -> %s", old.Addr, fresh.Addr)

	if err := drive(ctx, rs, 42, 1); err != nil {
		t.Fatalf("workload after step-down: %v", err)
	}

	compareWithReference(t, ctx, rs, subject, "after a primary step-down")
}

// A step-down while the subject is down: it must pick up from a different
// source than the one it left, without gaps.
func TestAdversarial_StepDownWhileSubjectIsDown(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 105)
	defer cancel()

	if err := drive(ctx, rs, 51, 1); err != nil {
		t.Fatalf("workload before: %v", err)
	}

	subject.Kill()
	if _, err := rs.StepDownPrimary(ctx, 30); err != nil {
		t.Fatalf("StepDownPrimary: %v", err)
	}
	if err := drive(ctx, rs, 52, 1); err != nil {
		t.Fatalf("workload under the new primary: %v", err)
	}
	subject.Start()
	if err := drive(ctx, rs, 53, 1); err != nil {
		t.Fatalf("workload after restart: %v", err)
	}

	compareWithReference(t, ctx, rs, subject, "after a step-down while the subject was down")
}

// Restarting with no new writes must be a no-op. A subject that re-applies its
// last batch on startup would corrupt counters that $inc has already advanced,
// and nothing else in the suite would notice.
func TestAdversarial_RestartIsIdempotent(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 106)
	defer cancel()

	if err := drive(ctx, rs, 61, 2); err != nil {
		t.Fatalf("workload: %v", err)
	}
	if _, err := rs.WaitConverged(ctx, 150*time.Second, subject.Addr); err != nil {
		t.Fatalf("subject did not converge: %v", err)
	}

	before := captureMember(t, ctx, rs, subject.Addr, "subject")

	for i := 0; i < 3; i++ {
		subject.Restart()
		if _, err := rs.WaitConverged(ctx, 150*time.Second, subject.Addr); err != nil {
			t.Fatalf("restart %d: subject did not converge: %v", i, err)
		}
	}

	after := captureMember(t, ctx, rs, subject.Addr, "subject")
	if d := harness.DiffServerState(before, after); len(d) > 0 {
		t.Errorf("three restarts with no intervening writes changed %d thing(s); first: %s", len(d), d[0])
	}
	compareWithReference(t, ctx, rs, subject, "after three no-op restarts")
}

// Killing the subject repeatedly at short random intervals while a concurrent
// workload runs, so kills land at unpredictable points including between an
// oplog fetch and its commit.
func TestAdversarial_KillsDuringConcurrentLoad(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 107)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- drive(ctx, rs, 71, 6) }()

	for i := 0; i < 5; i++ {
		time.Sleep(time.Duration(700+i*250) * time.Millisecond)
		subject.Kill()
		time.Sleep(300 * time.Millisecond)
		subject.Start()
	}
	if err := <-done; err != nil {
		t.Fatalf("concurrent workload: %v", err)
	}

	compareWithReference(t, ctx, rs, subject, "after kills during concurrent load")
}

// Sanity: the documents the subject holds must match the reference exactly,
// checked by count as well as by content, so a comparator that silently skipped
// a collection could not hide a gap.
func TestAdversarial_NoGapAfterInterruption(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := advApparatus(t, 108)
	defer cancel()

	if err := drive(ctx, rs, 81, 1); err != nil {
		t.Fatalf("workload: %v", err)
	}
	subject.Kill()
	if err := drive(ctx, rs, 82, 2); err != nil {
		t.Fatalf("workload while down: %v", err)
	}
	subject.Start()

	if _, err := rs.WaitConverged(ctx, 150*time.Second, subject.Addr); err != nil {
		t.Fatalf("subject did not converge: %v", err)
	}

	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("no reference: %v", err)
	}
	refCli, err := rs.ClientFor(ctx, reference.Addr)
	if err != nil {
		t.Fatalf("reference client: %v", err)
	}
	defer func() { _ = refCli.Disconnect(context.Background()) }()
	subCli, err := subject.Client(ctx)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	defer func() { _ = subCli.Disconnect(context.Background()) }()

	refCount, err := refCli.Database(advDB).Collection("workload").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("reference count: %v", err)
	}
	subCount, err := subCli.Database(advDB).Collection("workload").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("subject count: %v", err)
	}
	if refCount != subCount {
		t.Errorf("subject holds %d documents, reference holds %d", subCount, refCount)
	}
	compareWithReference(t, ctx, rs, subject, "no-gap check")
}
