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
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const historyDB = "history"

func commitIDs(ctx context.Context, cli *mongo.Client, db string) ([]string, error) {
	var res bson.M
	err := cli.Database(db).RunCommand(ctx, bson.D{{Key: "dumboLog", Value: 1}}).Decode(&res)
	if err != nil {
		return nil, err
	}
	raw, isArray := res["commits"].(bson.A)
	if !isArray {
		return nil, fmt.Errorf("dumboLog on %s returned commits of type %T, want an array", db, res["commits"])
	}
	ids := make([]string, 0, len(raw))
	for i, entry := range raw {
		doc, isDoc := entry.(bson.M)
		if !isDoc {
			return nil, fmt.Errorf("dumboLog on %s: commit %d is %T, want a document", db, i, entry)
		}
		id, isString := doc["commitId"].(string)
		if !isString || id == "" {
			return nil, fmt.Errorf("dumboLog on %s: commit %d has commitId %v of type %T, want a non-empty string", db, i, doc["commitId"], doc["commitId"])
		}
		ids = append(ids, id)
	}
	return ids, nil
}

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

func TestHistory_WriteDoesNotMoveUnrelatedDatabase(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)
	activeDB := historyDB + "_active"
	unrelatedDB := historyDB + "_unrelated"
	if _, err := f.client.Database(activeDB).Collection("items").InsertOne(ctx,
		bson.D{{Key: "_id", Value: 1}, {Key: "value", Value: "before"}}); err != nil {
		t.Fatalf("seeding active database: %v", err)
	}
	if _, err := f.client.Database(unrelatedDB).Collection("items").InsertOne(ctx,
		bson.D{{Key: "_id", Value: 1}, {Key: "value", Value: "unchanged"}}); err != nil {
		t.Fatalf("seeding unrelated database: %v", err)
	}
	if _, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge after seeding: %v", f.commit, err)
	}
	subjectClient, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	activeBefore, err := commitIDs(ctx, subjectClient, activeDB)
	if err != nil {
		t.Fatalf("active dumboLog before update: %v", err)
	}
	unrelatedBefore, err := commitIDs(ctx, subjectClient, unrelatedDB)
	if err != nil {
		t.Fatalf("unrelated dumboLog before update: %v", err)
	}

	for _, db := range []struct {
		name    string
		commits []string
	}{{activeDB, activeBefore}, {unrelatedDB, unrelatedBefore}} {
		if len(db.commits) < 2 {
			t.Fatalf("premise failed: %s has %d commits on dumbodb %s after seeding and converging, so its history is not being watched and this case proves nothing",
				db.name, len(db.commits), f.commit)
		}
	}

	unrelatedDocs, err := subjectClient.Database(unrelatedDB).Collection("items").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("counting unrelated documents: %v", err)
	}

	if _, err := f.client.Database(activeDB).Collection("items").UpdateOne(ctx,
		bson.D{{Key: "_id", Value: 1}}, bson.D{{Key: "$set", Value: bson.D{{Key: "value", Value: "after"}}}}); err != nil {
		t.Fatalf("updating active database: %v", err)
	}
	if _, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge after update: %v", f.commit, err)
	}
	activeAfter, err := commitIDs(ctx, subjectClient, activeDB)
	if err != nil {
		t.Fatalf("active dumboLog after update: %v", err)
	}
	unrelatedAfter, err := commitIDs(ctx, subjectClient, unrelatedDB)
	if err != nil {
		t.Fatalf("unrelated dumboLog after update: %v", err)
	}
	if len(activeAfter) != len(activeBefore)+1 {
		t.Errorf("dumbodb %s: active database history grew from %d to %d commits, want exactly one new commit",
			f.commit, len(activeBefore), len(activeAfter))
	}
	if len(unrelatedAfter) != len(unrelatedBefore) {
		t.Fatalf("dumbodb %s: write to %s added %d commits to unrelated database %s",
			f.commit, activeDB, len(unrelatedAfter)-len(unrelatedBefore), unrelatedDB)
	}
	for index := range unrelatedBefore {
		if unrelatedAfter[index] != unrelatedBefore[index] {
			t.Fatalf("dumbodb %s: unrelated database history changed at commit %d from %s to %s",
				f.commit, index, unrelatedBefore[index], unrelatedAfter[index])
		}
	}
	after, err := subjectClient.Database(unrelatedDB).Collection("items").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("counting unrelated documents after the update: %v", err)
	}
	if after != unrelatedDocs {
		t.Errorf("dumbodb %s: writing to %s changed %s from %d to %d documents",
			f.commit, activeDB, unrelatedDB, unrelatedDocs, after)
	}
}

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
