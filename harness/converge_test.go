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

package harness

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestOpTime_OrdersByTermThenTimestamp(t *testing.T) {
	cases := []struct {
		name string
		a, b OpTime
		want int
	}{
		{"equal", OpTime{10, 1, 1}, OpTime{10, 1, 1}, 0},
		{"later increment", OpTime{10, 2, 1}, OpTime{10, 1, 1}, 1},
		{"later seconds", OpTime{11, 1, 1}, OpTime{10, 9, 1}, 1},
		// A higher term wins even with an earlier timestamp: after a failover
		// the new term's log supersedes whatever the old primary had.
		{"higher term beats later timestamp", OpTime{1, 1, 2}, OpTime{999, 999, 1}, 1},
		{"lower term loses despite later timestamp", OpTime{999, 999, 1}, OpTime{1, 1, 2}, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.a.Compare(c.b); got != c.want {
				t.Errorf("%s.Compare(%s) = %d, want %d", c.a, c.b, got, c.want)
			}
		})
	}
}

func TestConvergenceTimeout_ExplainsWhoIsBehind(t *testing.T) {
	watermark := OpTime{Seconds: 100, Increment: 1, Term: 1}
	addrs := []string{"127.0.0.1:1", "127.0.0.1:2", "127.0.0.1:3"}
	last := map[string]memberSample{
		"127.0.0.1:1": {progress: MemberProgress{Addr: "127.0.0.1:1", State: StateSecondary, Applied: watermark}},
		"127.0.0.1:2": {progress: MemberProgress{Addr: "127.0.0.1:2", State: StateSecondary, Applied: OpTime{Seconds: 70, Increment: 1, Term: 1}}},
		"127.0.0.1:3": {progress: MemberProgress{Addr: "127.0.0.1:3", State: StateStartup2}},
	}

	err := convergenceTimeout(watermark, 30*time.Second, addrs, last)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()

	// A member that reached the watermark is still named, but as having got
	// there out of step rather than as lagging. An empty report is the failure
	// mode this function exists to prevent.
	if !contains(msg, "not in the same pass") {
		t.Errorf("a member that reached the watermark should be explained, not omitted:\n%s", msg)
	}
	if !contains(msg, "30 seconds behind") {
		t.Errorf("a lagging member should report how far behind it is:\n%s", msg)
	}
	// A member reporting nothing is a different failure from a slow one, and
	// conflating them sends the reader looking for a performance problem.
	if !contains(msg, "NO progress at all") {
		t.Errorf("a member reporting no progress should be distinguished from a lagging one:\n%s", msg)
	}
	if !contains(msg, StateStartup2) {
		t.Errorf("the stalled member's state should be named:\n%s", msg)
	}
}

func TestConvergenceTimeout_ReportsUnreachableMembers(t *testing.T) {
	err := convergenceTimeout(OpTime{Seconds: 5, Term: 1}, time.Second,
		[]string{"127.0.0.1:9"}, map[string]memberSample{})
	if err == nil {
		t.Fatal("expected an error")
	}
	if !contains(err.Error(), "never sampled") {
		t.Errorf("an unsampled member should be reported as such: %v", err)
	}
}

// Against two stock mongod members the gate must actually pass, and the
// watermark it returns must cover a write made before the call.
func TestConverge_ReferenceMembersConverge(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	pc, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	defer func() { _ = pc.Disconnect(context.Background()) }()

	coll := pc.Database("harness_converge").Collection("docs")
	for i := 0; i < 50; i++ {
		if _, err := coll.InsertOne(ctx, bson.D{{Key: "i", Value: i}}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	watermark := rs.MustConverge(t, ctx, 60*time.Second)
	if watermark.IsZero() {
		t.Fatal("watermark is zero")
	}
	t.Logf("converged on %s", watermark)

	// Every member must now hold all 50 documents; a gate that returns before
	// the data has landed is worse than no gate.
	for _, m := range rs.Members {
		cli, err := rs.DirectClient(ctx, m)
		if err != nil {
			t.Fatalf("client %s: %v", m.Addr, err)
		}
		n, err := cli.Database("harness_converge").Collection("docs").
			CountDocuments(ctx, bson.D{})
		_ = cli.Disconnect(context.Background())
		if err != nil {
			t.Fatalf("count on %s: %v", m.Addr, err)
		}
		if n != 50 {
			t.Errorf("%s holds %d documents after convergence, want 50", m.Addr, n)
		}
	}
}

// Progress must report a member's own claims, and a primary must look like one.
func TestConverge_ProgressReportsMemberState(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	progress, err := rs.Progress(ctx, primary.Addr)
	if err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if progress.State != StatePrimary {
		t.Errorf("primary reports state %q, want %s", progress.State, StatePrimary)
	}
	if progress.Applied.IsZero() {
		t.Error("primary reports a zero applied optime")
	}
	if progress.Durable.IsZero() {
		t.Error("primary reports a zero durable optime")
	}
	t.Logf("%s", progress)
}

// A member that cannot possibly reach the watermark must time out with a
// legible explanation rather than hanging or reporting success.
func TestConverge_UnreachableMemberTimesOutLegibly(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	_, err := rs.WaitConverged(ctx, 3*time.Second, "127.0.0.1:9")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected convergence to fail for an address that is not a member")
	}
	if !contains(err.Error(), "127.0.0.1:9") {
		t.Errorf("the failing address should be named: %v", err)
	}
	// The timeout must be honored. Driver server selection blocks ~30s on an
	// unreachable host, which silently multiplies every CI failure involving a
	// down member.
	if elapsed > 15*time.Second {
		t.Errorf("a 3s convergence timeout took %s; the deadline is not bounding the progress reads", elapsed)
	}
}

// A member that was caught up and then died must still be named, with the
// reason it stopped answering. The empty report this prevents is what an
// adversarial kill produced: every member skipped, nothing printed.
func TestConvergenceTimeout_NamesAMemberThatStoppedAnswering(t *testing.T) {
	watermark := OpTime{Seconds: 100, Increment: 1, Term: 1}
	last := map[string]memberSample{
		"127.0.0.1:1": {
			progress: MemberProgress{Addr: "127.0.0.1:1", State: StateSecondary, Applied: watermark},
			err:      context.DeadlineExceeded,
		},
	}
	err := convergenceTimeout(watermark, 30*time.Second, []string{"127.0.0.1:1"}, last)
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	if !contains(msg, "not answering") {
		t.Errorf("should report that the member stopped answering:\n%s", msg)
	}
	if !contains(msg, "last known position") {
		t.Errorf("should report the last known position:\n%s", msg)
	}
	if contains(msg, "harness bug") {
		t.Errorf("report should not have fallen through to the empty-list guard:\n%s", msg)
	}
}
