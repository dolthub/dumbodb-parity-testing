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

// The two claims docs/verify/replication.md makes about commit history that a
// reader cannot check by looking at the data: that an idle primary adds no
// commits, and that a removed member keeps everything it replicated.
//
// These are the automated analog of that document. It is a manual guide the
// owner will walk personally, so a claim in it that nobody tests is a claim
// that will be found wrong in front of him.
//
//go:build replication

package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const historyDB = "history"

// commitIDs returns the replicated history of db on the subject, newest first.
func commitIDs(ctx context.Context, cli *mongo.Client, db string) ([]string, error) {
	var res bson.M
	err := cli.Database(db).RunCommand(ctx, bson.D{{Key: "dumboLog", Value: 1}}).Decode(&res)
	if err != nil {
		return nil, err
	}
	raw, _ := res["commits"].(bson.A)
	ids := make([]string, 0, len(raw))
	for _, entry := range raw {
		doc, ok := entry.(bson.M)
		if !ok {
			continue
		}
		id, _ := doc["commitId"].(string)
		ids = append(ids, id)
	}
	return ids, nil
}

// TestHistory_IdlePrimaryAddsNoCommits covers the verify document's idle
// history check.
//
// MongoDB writes periodic no-op oplog entries to an idle primary. Committing
// one per arrival would fill the history with empty commits, so the checkpoint
// has to advance without one. The interesting half of this test is the proof
// that the no-ops actually arrived: without it, a subject that had stopped
// replicating entirely would pass.
func TestHistory_IdlePrimaryAddsNoCommits(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	if _, err := f.client.Database(historyDB).Collection("idle").InsertOne(ctx,
		bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: "seed"}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", f.commit, err)
	}

	subjectClient, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	before, err := commitIDs(ctx, subjectClient, historyDB)
	if err != nil {
		t.Fatalf("dumboLog: %v", err)
	}
	if len(before) < 2 {
		t.Fatalf("dumbodb %s: %s has %d commits after replicating an insert; expected the initialization commit and at least one replicated commit",
			f.commit, historyDB, len(before))
	}
	beforeProgress, err := f.rs.Progress(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("Progress: %v", err)
	}

	// mongod's periodic no-op writer runs every ten seconds by default. Give
	// it two intervals plus room for the subject to apply them.
	idle := 25 * time.Second
	t.Logf("holding the primary idle for %s", idle)
	select {
	case <-time.After(idle):
	case <-ctx.Done():
		t.Fatalf("context expired while idling: %v", ctx.Err())
	}

	afterProgress, err := f.rs.Progress(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if afterProgress.Applied.Compare(beforeProgress.Applied) <= 0 {
		t.Fatalf("premise failed: the subject's applied optime stayed at %s across %s of idling, so no no-op entries reached it and this test proves nothing about them",
			beforeProgress.Applied, idle)
	}

	after, err := commitIDs(ctx, subjectClient, historyDB)
	if err != nil {
		t.Fatalf("dumboLog: %v", err)
	}
	if len(after) != len(before) {
		t.Errorf("dumbodb %s: an idle primary added %d commits to %s (applied advanced %s -> %s); no-op entries must advance the checkpoint without committing",
			f.commit, len(after)-len(before), historyDB, beforeProgress.Applied, afterProgress.Applied)
	}
	if len(after) > 0 && len(before) > 0 && after[0] != before[0] {
		t.Errorf("dumbodb %s: HEAD of %s moved from %s to %s while the primary was idle", f.commit, historyDB, before[0], after[0])
	}
	t.Logf("dumbodb %s: %d commits held across %s idle while applied advanced %s -> %s",
		f.commit, len(after), idle, beforeProgress.Applied, afterProgress.Applied)
}

// TestHistory_RemovedMemberKeepsHistory covers the verify document's final
// scenario: reconfiguring the member out of the set is how you stop
// replication now that dumboReplicationDetach is gone.
//
// A removed member is not a discarded stale node. It must stop taking writes
// and keep every commit it made, because that history is the reason to run
// DumboDB as the secondary rather than a mongod.
func TestHistory_RemovedMemberKeepsHistory(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	coll := f.client.Database(historyDB).Collection("removal")
	if _, err := coll.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: 1}},
		bson.D{{Key: "_id", Value: 2}},
	}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", f.commit, err)
	}

	subjectClient, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	before, err := commitIDs(ctx, subjectClient, historyDB)
	if err != nil {
		t.Fatalf("dumboLog: %v", err)
	}

	if err := f.rs.RemoveMember(ctx, f.subject.Addr); err != nil {
		t.Fatalf("removing %s from %s: %v", f.subject.Addr, f.rs.Name, err)
	}

	// Write after the removal. This must not arrive.
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 3}}); err != nil {
		t.Fatalf("post-removal insert: %v", err)
	}
	select {
	case <-time.After(20 * time.Second):
	case <-ctx.Done():
		t.Fatalf("context expired: %v", ctx.Err())
	}

	count, err := subjectClient.Database(historyDB).Collection("removal").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("counting on the removed member: %v", err)
	}
	if count != 2 {
		t.Errorf("dumbodb %s: removed member holds %d documents, want the 2 it had at removal; it is still applying the primary's writes",
			f.commit, count)
	}

	// Everything it replicated is still there, and still readable. Comparing
	// the full list rather than the length catches a history that was
	// truncated or rewritten rather than merely frozen.
	after, err := commitIDs(ctx, subjectClient, historyDB)
	if err != nil {
		t.Fatalf("dumboLog on the removed member: %v", err)
	}
	if len(after) < len(before) {
		t.Fatalf("dumbodb %s: removal dropped history, %d commits before and %d after", f.commit, len(before), len(after))
	}
	frozen := after[len(after)-len(before):]
	for i := range before {
		if frozen[i] != before[i] {
			t.Errorf("dumbodb %s: commit %d of the replicated history changed from %s to %s across removal",
				f.commit, i, before[i], frozen[i])
		}
	}
	if len(after) != len(before) {
		t.Errorf("dumbodb %s: removed member added %d commits after leaving the set", f.commit, len(after)-len(before))
	}

	// A removed member that stopped serving would also pass the checks above.
	docs, err := subjectClient.Database(historyDB).Collection("removal").Find(ctx, bson.D{})
	if err != nil {
		t.Fatalf("reading from the removed member: %v", err)
	}
	var held []bson.M
	if err := docs.All(ctx, &held); err != nil {
		t.Fatalf("decoding from the removed member: %v", err)
	}
	if len(held) != 2 {
		t.Errorf("dumbodb %s: removed member returned %d documents on a find, want 2", f.commit, len(held))
	}
	t.Logf("dumbodb %s: %d commits preserved after removal from %s", f.commit, len(after), f.rs.Name)
}

// TestHistory_RejoinResumesWithoutRewriting covers the claim in
// docs/COMMANDS.md that "re-adding the same member identity to the same
// replica set activates it again".
//
// Activating again is not the same as syncing again. A member that threw its
// history away and re-cloned would also end up holding the right documents,
// so the assertion that matters is that the commits made before the removal
// are still the same commits afterwards.
func TestHistory_RejoinResumesWithoutRewriting(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	coll := f.client.Database(historyDB).Collection("rejoin")
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: "before"}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if _, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", f.commit, err)
	}

	subjectClient, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	before, err := commitIDs(ctx, subjectClient, historyDB)
	if err != nil {
		t.Fatalf("dumboLog: %v", err)
	}

	if err := f.rs.RemoveMember(ctx, f.subject.Addr); err != nil {
		t.Fatalf("removing %s: %v", f.subject.Addr, err)
	}
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 2}, {Key: "v", Value: "while-out"}}); err != nil {
		t.Fatalf("insert while removed: %v", err)
	}
	select {
	case <-time.After(15 * time.Second):
	case <-ctx.Done():
		t.Fatalf("context expired: %v", ctx.Err())
	}

	if err := f.rs.AddMember(ctx, f.subject.Member); err != nil {
		t.Fatalf("re-adding %s: %v", f.subject.Addr, err)
	}
	if err := f.rs.WaitForState(ctx, f.subject.Member, harness.StateSecondary, 150*time.Second); err != nil {
		t.Fatalf("dumbodb %s did not return to SECONDARY after rejoining: %v", f.commit, err)
	}
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 3}, {Key: "v", Value: "after"}}); err != nil {
		t.Fatalf("insert after rejoin: %v", err)
	}
	if _, err := f.rs.WaitConverged(ctx, 120*time.Second, f.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge after rejoining: %v", f.commit, err)
	}

	// The write made while it was out of the set has to be caught up, not
	// skipped: a resumed member starts from its checkpoint, not from now.
	count, err := subjectClient.Database(historyDB).Collection("rejoin").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("counting after rejoin: %v", err)
	}
	if count != 3 {
		t.Errorf("dumbodb %s: holds %d documents after rejoining, want 3; the write made while it was out of the set was skipped",
			f.commit, count)
	}

	after, err := commitIDs(ctx, subjectClient, historyDB)
	if err != nil {
		t.Fatalf("dumboLog after rejoin: %v", err)
	}
	if len(after) <= len(before) {
		t.Fatalf("dumbodb %s: %d commits after rejoining, %d before; nothing was committed for the catch-up",
			f.commit, len(after), len(before))
	}
	preserved := after[len(after)-len(before):]
	for i := range before {
		if preserved[i] != before[i] {
			t.Fatalf("dumbodb %s: rejoining rewrote history; commit %d was %s and is now %s. The member re-synced rather than resuming",
				f.commit, i, before[i], preserved[i])
		}
	}
	t.Logf("dumbodb %s: resumed on %d preserved commits, %d total after catch-up", f.commit, len(before), len(after))
}
