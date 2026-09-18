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
	"fmt"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

const (
	defaultReplicaMembers = 2
	defaultReplicaTimeout = 4 * time.Minute
	// defaultConvergeWait is generous on purpose.
	//
	// It was 90 seconds, chosen when five of the vocabulary's operators were
	// silently writing nothing. Once they were fixed to actually mutate, the
	// concurrent full-vocabulary case stopped finishing in time: the subject
	// was reported SECONDARY, advancing steadily, and simply short of the
	// watermark when the budget expired.
	//
	// A budget too short produces false failures against a working member,
	// and its cost is paid on every failing run rather than every run. The
	// convergence timeout now reports whether a member advanced while
	// waiting, so a genuinely stuck one still says so rather than hiding
	// behind the larger number.
	defaultConvergeWait = 3 * time.Minute
)

// ReplicaCase is one replication parity test: a workload run against a real
// MongoDB primary, then a comparison of what the members ended up holding.
//
// PairTest does not fit this shape. It drives one Run per server and compares
// two return values; a replication case has one server to write to and two to
// compare, and the interesting assertion is about stored state rather than a
// command response.
type ReplicaCase struct {
	Name    string
	Support DumboDBSupport

	// Members is the number of mongod members: one becomes primary, the rest
	// are reference secondaries. Zero means two.
	Members int

	// Timeout bounds the whole case. Zero means four minutes.
	Timeout time.Duration

	// SeedBeforeJoin runs against the primary BEFORE the subject joins, so its
	// data must arrive through initial sync rather than the oplog. That is the
	// only way to test the clone path: anything written after the join arrives
	// as ordinary steady-state replication.
	SeedBeforeJoin func(ctx context.Context, primary *mongo.Client) error

	// DuringClone runs concurrently with the subject's initial sync, starting
	// immediately after the join and without waiting for SECONDARY.
	//
	// This is the case the buffered-oplog design exists for: the clone is not a
	// single instant, and these writes must be reconciled by the oplog rather
	// than lost or double-applied.
	DuringClone func(ctx context.Context, primary *mongo.Client) error

	// Setup prepares the primary before the subject is expected to be caught
	// up. Data written here still replicates; it is separated from Workload
	// only for readability.
	Setup func(ctx context.Context, primary *mongo.Client) error

	// Workload performs the operations under test against the primary.
	Workload func(ctx context.Context, primary *mongo.Client) error

	// Assert adds case-specific checks. The default comparison always runs;
	// this is for anything beyond it.
	Assert func(t *testing.T, res ReplicaResult)
}

// ReplicaResult is what a case produced, for custom assertions.
type ReplicaResult struct {
	Watermark     OpTime
	Primary       *ServerState
	Reference     *ServerState
	Subject       *ServerState
	SubjectCommit string
	// Divergences between the subject and the reference secondary.
	Divergences []Divergence
}

// ReplicaTest runs tc and grades it according to tc.Support.
//
// The reference mongod secondary is a control, not decoration. It is compared
// against the primary first, and a mismatch there fails the test outright
// regardless of support level: if a stock secondary cannot converge, the
// apparatus is broken and nothing the subject does means anything.
func ReplicaTest(t *testing.T, tc ReplicaCase) TestResult {
	t.Helper()

	timeout := tc.Timeout
	if timeout == 0 {
		timeout = defaultReplicaTimeout
	}
	members := tc.Members
	if members == 0 {
		members = defaultReplicaMembers
	}
	if members < 2 {
		t.Fatalf("%s: need at least 2 mongod members for a primary and a reference secondary, got %d", tc.Name, members)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	rs := StartReplicaSet(t, members)

	// Seeding must happen before the join so the data travels through initial
	// sync. Resolve the primary first for that reason.
	seedPrimary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("%s: %v", tc.Name, err)
	}
	seedClient, err := rs.DirectClient(ctx, seedPrimary)
	if err != nil {
		t.Fatalf("%s: primary client: %v", tc.Name, err)
	}
	if tc.SeedBeforeJoin != nil {
		if err := tc.SeedBeforeJoin(ctx, seedClient); err != nil {
			_ = seedClient.Disconnect(context.Background())
			t.Fatalf("%s: seeding the primary before the join failed: %v", tc.Name, err)
		}
		// Let the reference catch up, so the seed is genuinely source state
		// rather than still in flight when the subject starts cloning.
		if _, err := rs.WaitConverged(ctx, defaultConvergeWait, seedPrimary.Addr); err != nil {
			_ = seedClient.Disconnect(context.Background())
			t.Fatalf("%s: primary did not settle after seeding: %v", tc.Name, err)
		}
	}
	_ = seedClient.Disconnect(context.Background())

	var subject *DumboMember
	if tc.Support != DumboDBMongoOnly {
		subject = rs.JoinDumboDB(t)
	}

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("%s: %v", tc.Name, err)
	}
	primaryClient, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("%s: primary client: %v", tc.Name, err)
	}
	defer func() { _ = primaryClient.Disconnect(context.Background()) }()

	// Synchronize before dispatching, because "during the clone" and "after
	// the subject is caught up" are different tests and neither happens by
	// accident.
	//
	// DuringClone waits for STARTUP2 so the writes genuinely overlap cloning.
	// Firing it the instant the member joins raced the clone starting, and on
	// a small fixture the clone could finish first, quietly turning a
	// concurrent-clone case into an ordinary steady-state one.
	//
	// Everything else waits for SECONDARY. Without that, tier 2 ran its
	// workload while the subject was still cloning, so operations meant to
	// exercise steady-state application were exercising initial sync instead.
	// The comparison at the end still held, which is why this was invisible.
	cloneDone := make(chan error, 1)
	if tc.DuringClone != nil {
		if subject != nil {
			if err := rs.WaitForState(ctx, subject.Member, StateStartup2, defaultConvergeWait); err != nil {
				t.Logf("%s: subject did not report STARTUP2 before DuringClone (%v); the clone may already have finished", tc.Name, err)
			}
		}
		go func() { cloneDone <- tc.DuringClone(ctx, primaryClient) }()
	} else {
		cloneDone <- nil
		if subject != nil {
			if err := rs.WaitForState(ctx, subject.Member, StateSecondary, defaultConvergeWait); err != nil {
				t.Fatalf("%s: subject did not reach SECONDARY before the workload, so this case would have exercised initial sync rather than steady-state application: %v", tc.Name, err)
			}
		}
	}

	if tc.Setup != nil {
		if err := tc.Setup(ctx, primaryClient); err != nil {
			t.Fatalf("%s: setup on the primary failed: %v", tc.Name, err)
		}
	}
	if tc.Workload == nil {
		t.Fatalf("%s: no workload", tc.Name)
	}
	if err := tc.Workload(ctx, primaryClient); err != nil {
		t.Fatalf("%s: workload on the primary failed: %v", tc.Name, err)
	}
	if err := <-cloneDone; err != nil {
		t.Fatalf("%s: concurrent-with-clone workload failed: %v", tc.Name, err)
	}

	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("%s: no reference secondary: %v", tc.Name, err)
	}

	res := ReplicaResult{}
	res.Watermark, res.Primary, res.Reference = runControl(t, ctx, rs, tc.Name, primary, reference)

	if subject == nil {
		t.Logf("MONGO_ONLY %s: control converged on %s (subject skipped)", tc.Name, res.Watermark)
		return finish(t, tc, res, TestResult{Name: tc.Name, Status: StatusSkip})
	}

	// Attribution is the point of pinning the subject build; losing it silently
	// makes every result unreproducible, so surface the reason rather than
	// printing "unknown".
	res.SubjectCommit, err = subject.Commit(ctx)
	// "unknown" is what a binary built without the version stamp reports, and
	// it is exactly as unattributable as an empty string. Every other entry
	// point rejects it; this one used to accept it and label the result as
	// though it named a build.
	if err != nil || res.SubjectCommit == "" || res.SubjectCommit == "unknown" {
		t.Errorf("%s: could not read the subject's build commit (buildInfo.gitVersion): %v", tc.Name, err)
		res.SubjectCommit = "UNATTRIBUTED"
	}
	subjectFailure := gradeSubject(t, ctx, rs, tc, subject, reference, &res)
	return finish(t, tc, res, subjectFailure)
}

// runControl proves the apparatus before the subject is judged by it.
func runControl(t *testing.T, ctx context.Context, rs *ReplicaSet, name string, primary, reference *Member) (OpTime, *ServerState, *ServerState) {
	t.Helper()

	watermark, err := rs.WaitConverged(ctx, defaultConvergeWait, primary.Addr, reference.Addr)
	if err != nil {
		t.Fatalf("%s: CONTROL FAILED, a stock mongod secondary did not converge. The apparatus is broken, not the subject.\n%v", name, err)
	}

	primaryState := captureOrFail(t, ctx, rs, primary, "primary")
	referenceState := captureOrFail(t, ctx, rs, reference, "reference")

	if d := DiffServerState(primaryState, referenceState); len(d) > 0 {
		t.Fatalf("%s: CONTROL FAILED, the reference secondary does not match the primary. The apparatus is broken, not the subject.\n%s",
			name, formatDivergences(d))
	}
	return watermark, primaryState, referenceState
}

// gradeSubject compares the subject against the reference and applies the
// support level. A subject that never converges is graded the same as one that
// converged to the wrong data: both mean it failed to reproduce the primary.
func gradeSubject(t *testing.T, ctx context.Context, rs *ReplicaSet, tc ReplicaCase, subject *DumboMember, reference *Member, res *ReplicaResult) TestResult {
	t.Helper()

	failure := ""
	if _, err := rs.WaitConverged(ctx, defaultConvergeWait, subject.Addr); err != nil {
		failure = err.Error()
	} else {
		res.Subject = captureOrFail(t, ctx, rs, subject.Member, "subject")
		res.Divergences = DiffServerState(res.Reference, res.Subject)
		if len(res.Divergences) > 0 {
			failure = formatDivergences(res.Divergences)
		}
	}

	label := fmt.Sprintf("%s (dumbodb %s)", tc.Name, res.SubjectCommit)
	switch tc.Support {
	case DumboDBFull:
		if failure == "" {
			t.Logf("FULL %s: PASS", label)
			return TestResult{Name: tc.Name, Status: StatusPass}
		}
		t.Errorf("FULL %s: DIVERGE\n%s", label, failure)
		return TestResult{Name: tc.Name, Status: StatusFail, Diff: failure}

	case DumboDBXFail:
		if failure == "" {
			// The ratchet. A case that starts passing must be promoted, or the
			// suite quietly stops noticing when it breaks again.
			t.Errorf("XPASS %s: the subject now matches the reference -- promote this case to DumboDBFull", label)
			return TestResult{Name: tc.Name, Status: StatusXPass}
		}
		t.Logf("XFAIL %s: diverged as expected\n%s", label, failure)
		return TestResult{Name: tc.Name, Status: StatusXFail, Diff: failure}

	default:
		t.Fatalf("%s: unsupported DumboDBSupport level %d for a replication case", tc.Name, tc.Support)
		return TestResult{Name: tc.Name, Status: StatusFail}
	}
}

func finish(t *testing.T, tc ReplicaCase, res ReplicaResult, result TestResult) TestResult {
	t.Helper()
	if tc.Assert != nil {
		tc.Assert(t, res)
	}
	return result
}

func captureOrFail(t *testing.T, ctx context.Context, rs *ReplicaSet, m *Member, role string) *ServerState {
	t.Helper()
	cli, err := rs.DirectClient(ctx, m)
	if err != nil {
		t.Fatalf("%s client %s: %v", role, m.Addr, err)
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	state, err := CaptureServerState(ctx, cli, role)
	if err != nil {
		t.Fatalf("capturing %s state: %v", role, err)
	}
	return state
}

// formatDivergences caps the report: a whole-collection mismatch produces one
// divergence per document, and a thousand identical lines hide the shape of the
// failure rather than explaining it.
func formatDivergences(d []Divergence) string {
	const maxShown = 20
	var b strings.Builder
	fmt.Fprintf(&b, "%d divergence(s):\n", len(d))
	for i, div := range d {
		if i == maxShown {
			fmt.Fprintf(&b, "  ... and %d more\n", len(d)-maxShown)
			break
		}
		fmt.Fprintf(&b, "  %s\n", div)
	}
	return b.String()
}
