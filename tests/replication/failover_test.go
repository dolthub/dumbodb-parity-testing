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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const failoverDB = "failover"

// TestFailover_SubjectFollowsANewPrimary kills the mongod DumboDB is reading
// from and requires it to carry on replicating from whichever member wins the
// election.
//
// Nothing else in this package kills a mongod. The adversarial cases kill the
// SUBJECT and restart it; the sync source is always there to come back to.
// This is the opposite failure, and it is the one a real deployment has: the
// upstream goes away and never returns.
//
// Three data-bearing members, not two. An election needs a majority of
// configured votes, so with two members the majority is two, the survivor sees
// one vote of two and refuses to promote itself, and the set goes read-only
// with no failover to observe. DumboDB cannot make up the difference because
// it joins with votes 0 by design. Three members make the majority two, which
// the survivors still reach.
func TestFailover_SubjectFollowsANewPrimary(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	rs := harness.StartReplicaSet(t, 3)
	subject := rs.JoinDumboDB(t)
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 150*time.Second); err != nil {
		t.Fatalf("subject never reached SECONDARY: %v", err)
	}

	writeFailoverBatch(t, ctx, rs, 0, 50)
	if _, err := rs.WaitConverged(ctx, 120*time.Second, subject.Addr); err != nil {
		t.Fatalf("subject did not converge before the kill: %v", err)
	}

	// Kill what the subject is actually reading from, which is not necessarily
	// the primary: a secondary is an eligible sync source and DumboDB chooses
	// for itself. Killing the primary when the subject had chained off a
	// secondary would leave the subject's upstream untouched and prove nothing.
	doomed, err := rs.SyncSourceOf(ctx, subject.Addr)
	if err != nil {
		t.Fatalf("reading the subject's sync source: %v", err)
	}
	if doomed == "" {
		t.Fatal("the subject reports no sync source, so there is nothing to fail over from")
	}
	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	t.Logf("subject is syncing from %s; the primary is %s", doomed, primary.Addr)

	if err := rs.KillMember(doomed); err != nil {
		t.Fatalf("killing %s: %v", doomed, err)
	}

	// An election only happens if what died was the primary. When the subject
	// was reading from a secondary the set keeps its primary and only the
	// subject has to re-target, which is still the behaviour under test.
	if doomed == primary.Addr {
		elected, err := rs.WaitForNewPrimary(ctx, doomed, 120*time.Second)
		if err != nil {
			t.Fatalf("no new primary after killing %s: %v", doomed, err)
		}
		t.Logf("%s was elected primary", elected.Addr)
	}

	source, err := rs.WaitForSyncSourceChange(ctx, subject.Addr, doomed, 150*time.Second)
	if err != nil {
		t.Fatalf("subject did not find a new sync source: %v", err)
	}
	t.Logf("subject re-targeted to %s", source)

	// Writes made only after the kill. These are the ones that can reach the
	// subject exclusively through its new upstream, so they are what proves
	// replication resumed rather than merely survived.
	writeFailoverBatch(t, ctx, rs, 50, 50)

	if _, err := rs.WaitConverged(ctx, 150*time.Second, subject.Addr); err != nil {
		t.Fatalf("subject did not converge after the failover: %v", err)
	}
	assertSubjectMatchesSurvivingSecondary(t, ctx, rs, subject, doomed)
}

// writeFailoverBatch inserts ids [from, from+count) on whichever member is
// primary at the time, which is a different one before and after an election.
func writeFailoverBatch(t *testing.T, ctx context.Context, rs *harness.ReplicaSet, from, count int) {
	t.Helper()
	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	cli, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	docs := make([]interface{}, 0, count)
	for i := from; i < from+count; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "batch", Value: from}})
	}
	if _, err := cli.Database(failoverDB).Collection("events").InsertMany(ctx, docs); err != nil {
		t.Fatalf("writing ids %d..%d on %s: %v", from, from+count-1, primary.Addr, err)
	}
}

// assertSubjectMatchesSurvivingSecondary compares the subject against a stock
// mongod secondary that lived through the same failover, and separately counts
// the documents written after the kill.
//
// The count is not redundant. Two empty servers agree, and so do two servers
// that both stopped at the same moment, so a state diff alone cannot tell
// "replicated everything" from "both stopped early".
func assertSubjectMatchesSurvivingSecondary(t *testing.T, ctx context.Context, rs *harness.ReplicaSet, subject *harness.DumboMember, doomed string) {
	t.Helper()

	var reference *harness.Member
	secondaries, err := rs.Secondaries(ctx)
	if err != nil {
		t.Fatalf("Secondaries: %v", err)
	}
	for _, m := range secondaries {
		if m.Addr != subject.Addr && m.Addr != doomed {
			reference = m
			break
		}
	}
	if reference == nil {
		t.Fatal("no surviving mongod secondary to compare against")
	}

	refState := captureMember(t, ctx, rs, reference.Addr, "reference")
	subState := captureMember(t, ctx, rs, subject.Addr, "subject")
	if d := harness.DiffServerState(refState, subState); len(d) > 0 {
		t.Errorf("after the failover the subject diverged from %s in %d place(s); first: %s",
			reference.Addr, len(d), d[0])
	}

	after, err := rs.WritableCount(ctx, subject.Addr, failoverDB, "events")
	if err != nil {
		t.Fatalf("counting on the subject: %v", err)
	}
	if after != 100 {
		t.Errorf("the subject holds %d documents, want 100; it did not receive everything written across the failover", after)
	}
}
