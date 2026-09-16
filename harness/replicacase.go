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
	defaultConvergeWait   = 90 * time.Second
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
	if err != nil || res.SubjectCommit == "" {
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
