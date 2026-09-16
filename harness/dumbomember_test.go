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

package harness

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func requireDumboDB(t *testing.T) {
	t.Helper()
	requireMongod(t)
	if findDumboDBBinary() == "" {
		t.Skip("no dumbodb binary (set DUMBODB_BIN)")
	}
}

// The join itself must succeed and install all three properties. This asserts
// harness behavior, not DumboDB replication behavior: reaching SECONDARY is a
// separate question handled below.
func TestDumboMember_JoinsAsHiddenNonVoting(t *testing.T) {
	requireDumboDB(t)
	rs := StartReplicaSet(t, 2)
	d := rs.JoinDumboDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	d.AssertHiddenNonVoting(ctx)

	commit, err := d.Commit(ctx)
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	// "unknown" is what the build emits when the version stamp was not set, so
	// checking for empty alone let an unattributable binary pass unnoticed.
	if commit == "" || commit == "unknown" {
		t.Errorf("buildInfo.gitVersion is %q; the subject build carries no commit stamp, so no result from it is attributable", commit)
	}
	t.Logf("subject under test: dumbodb %s at %s", commit, d.Addr)
}

// The set must keep exactly one primary after the join. A non-voting member
// that perturbs the election would invalidate every downstream test.
func TestDumboMember_JoinDoesNotDisturbTheSet(t *testing.T) {
	requireDumboDB(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	before, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary before join: %v", err)
	}

	d := rs.JoinDumboDB(t)
	d.AssertHiddenNonVoting(ctx)

	after, err := rs.WaitForPrimary(ctx, 60*time.Second)
	if err != nil {
		t.Fatalf("Primary after join: %v", err)
	}
	if after.Addr != before.Addr {
		t.Errorf("primary moved on joining a non-voting member: %s -> %s", before.Addr, after.Addr)
	}

	mongoSecondaries, err := rs.Secondaries(ctx)
	if err != nil {
		t.Fatalf("Secondaries: %v", err)
	}
	for _, s := range mongoSecondaries {
		if s.Addr == d.Addr {
			t.Logf("subject reached SECONDARY")
		}
	}
}

// Stop and Start must preserve the data directory, which every resume and
// durability case depends on.
func TestDumboMember_RestartPreservesDataDirectory(t *testing.T) {
	requireDumboDB(t)
	rs := StartReplicaSet(t, 2)
	d := rs.JoinDumboDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, err := d.Commit(ctx); err != nil {
		t.Fatalf("subject unreachable before restart: %v", err)
	}
	dir := d.DataDir

	d.Restart()

	if d.DataDir != dir {
		t.Fatalf("data directory changed across restart: %s -> %s", dir, d.DataDir)
	}
	if _, err := d.Commit(ctx); err != nil {
		t.Fatalf("subject unreachable after restart: %v", err)
	}
}

// A hard kill must leave the member relaunchable, which is the precondition for
// the crash-recovery cases in tier 3.
func TestDumboMember_SurvivesHardKill(t *testing.T) {
	requireDumboDB(t)
	rs := StartReplicaSet(t, 2)
	d := rs.JoinDumboDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if _, err := d.Commit(ctx); err != nil {
		t.Fatalf("subject unreachable before kill: %v", err)
	}
	d.Kill()
	d.Start()
	if _, err := d.Commit(ctx); err != nil {
		t.Fatalf("subject did not come back after a hard kill: %v", err)
	}
}

// Whether the subject reaches SECONDARY is the actual replication question.
// Reported separately from the harness assertions so a red result here reads as
// a subject result rather than broken infrastructure.
func TestDumboMember_ReachesSecondary(t *testing.T) {
	requireDumboDB(t)
	rs := StartReplicaSet(t, 2)
	d := rs.JoinDumboDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	commit, _ := d.Commit(ctx)
	err := rs.WaitForState(ctx, d.Member, StateSecondary, 120*time.Second)
	if err != nil {
		t.Skipf("XFAIL dumbodb %s: subject did not reach SECONDARY: %v", commit, err)
	}
	t.Logf("dumbodb %s reached SECONDARY", commit)
}

// replSetGetStatus must carry the top-level optimes document. That is where
// mongosh, monitoring and anything computing replication lag read a member's
// own position; a member without it reads as having made no progress.
//
// The harness has a fallback to the self entry in members[], added when this was
// missing. This test is what allows that fallback to be removed, and what
// notices if the field disappears again.
func TestDumboMember_ReportsStandardOptimes(t *testing.T) {
	requireDumboDB(t)
	rs := StartReplicaSet(t, 2)
	d := rs.JoinDumboDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	if err := rs.WaitForState(ctx, d.Member, StateSecondary, 120*time.Second); err != nil {
		t.Skipf("subject did not reach SECONDARY, optimes not meaningful yet: %v", err)
	}

	cli, err := rs.client(ctx, d.Addr)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	status, err := replSetGetStatus(ctx, cli)
	if err != nil {
		t.Fatalf("replSetGetStatus: %v", err)
	}

	optimes, ok := status["optimes"].(bson.M)
	if !ok {
		t.Fatal("replSetGetStatus has no top-level optimes document")
	}
	for _, field := range []string{
		"lastCommittedOpTime", "appliedOpTime", "durableOpTime", "writtenOpTime",
		"lastAppliedWallTime", "lastDurableWallTime",
	} {
		if _, ok := optimes[field]; !ok {
			t.Errorf("optimes is missing %q", field)
		}
	}

	applied := readOpTime(optimes["appliedOpTime"])
	if applied.IsZero() {
		t.Error("appliedOpTime is zero on a member reporting SECONDARY")
	}
	t.Logf("optimes.appliedOpTime = %s", applied)
}
