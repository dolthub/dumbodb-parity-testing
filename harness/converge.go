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
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// OpTime is a replication position: a BSON timestamp plus the election term.
type OpTime struct {
	Seconds   uint32
	Increment uint32
	Term      int64
}

// Compare orders by term first, then timestamp, matching MongoDB's OpTime
// ordering. A higher term is always later in the replication log regardless of
// timestamp, so comparing timestamps first would accept a stale position after
// a failover.
func (o OpTime) Compare(other OpTime) int {
	switch {
	case o.Term != other.Term:
		if o.Term < other.Term {
			return -1
		}
		return 1
	case o.Seconds != other.Seconds:
		if o.Seconds < other.Seconds {
			return -1
		}
		return 1
	case o.Increment != other.Increment:
		if o.Increment < other.Increment {
			return -1
		}
		return 1
	}
	return 0
}

func (o OpTime) IsZero() bool { return o.Seconds == 0 && o.Increment == 0 }

func (o OpTime) String() string {
	return fmt.Sprintf("{ts:%d.%d t:%d}", o.Seconds, o.Increment, o.Term)
}

// MemberProgress is what one member reports about its own position. These are
// the member's claims, not a peer's view of it, because the honesty tier needs
// to compare what a member says against what it actually holds.
type MemberProgress struct {
	Addr        string
	State       string
	Applied     OpTime
	Durable     OpTime
	Written     OpTime
	AppliedWall time.Time
}

func (p MemberProgress) String() string {
	return fmt.Sprintf("%s state=%s applied=%s durable=%s", p.Addr, p.State, p.Applied, p.Durable)
}

// Progress asks a member for its own reported position.
func (rs *ReplicaSet) Progress(ctx context.Context, addr string) (MemberProgress, error) {
	out := MemberProgress{Addr: addr, State: "<unreachable>"}

	cli, err := rs.client(ctx, addr)
	if err != nil {
		return out, err
	}
	status, err := replSetGetStatus(ctx, cli)
	if err != nil {
		return out, fmt.Errorf("%s: %w", addr, err)
	}

	out.State = asString(status["myState"])
	if out.State == "" {
		out.State = stateName(asInt64(status["myState"]))
	}
	if optimes, ok := status["optimes"].(bson.M); ok {
		out.Applied = readOpTime(optimes["appliedOpTime"])
		out.Durable = readOpTime(optimes["durableOpTime"])
		out.Written = readOpTime(optimes["writtenOpTime"])
		if wall, ok := optimes["lastAppliedWallTime"].(primitive.DateTime); ok {
			out.AppliedWall = wall.Time()
		}
	}

	// Fall back to the self entry in members[]. DumboDB does not emit the
	// top-level optimes document that MongoDB provides, so without this the
	// harness reads zero and misreports a replicating member as stalled.
	if out.Applied.IsZero() {
		if self := selfMember(status); self != nil {
			out.Applied = readOpTime(self["optime"])
			if wall, ok := self["optimeDate"].(primitive.DateTime); ok {
				out.AppliedWall = wall.Time()
			}
		}
	}
	if out.Written.IsZero() {
		out.Written = out.Applied
	}
	return out, nil
}

func selfMember(status bson.M) bson.M {
	for _, raw := range asArray(status["members"]) {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		if self, _ := m["self"].(bool); self {
			return m
		}
	}
	return nil
}

func readOpTime(v interface{}) OpTime {
	doc, ok := v.(bson.M)
	if !ok {
		return OpTime{}
	}
	out := OpTime{Term: asInt64(doc["t"])}
	if ts, ok := doc["ts"].(primitive.Timestamp); ok {
		out.Seconds = ts.T
		out.Increment = ts.I
	}
	return out
}

// stateName maps the numeric replica set member state to its name, for the
// cases where myState arrives as a number.
func stateName(state int64) string {
	switch state {
	case 0:
		return "STARTUP"
	case 1:
		return StatePrimary
	case 2:
		return StateSecondary
	case 3:
		return StateRecovering
	case 5:
		return StateStartup2
	case 6:
		return "UNKNOWN"
	case 7:
		return "ARBITER"
	case 8:
		return "DOWN"
	case 9:
		return "ROLLBACK"
	case 10:
		return "REMOVED"
	}
	return fmt.Sprintf("STATE(%d)", state)
}

// maxPollBudget caps how long one progress read may take. Long enough for a
// healthy member under load, short enough that an unreachable one does not
// consume the whole convergence timeout.
const maxPollBudget = 2 * time.Second

func pollBudget(deadline time.Time) time.Duration {
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return time.Millisecond
	}
	if remaining < maxPollBudget {
		return remaining
	}
	return maxPollBudget
}

func (rs *ReplicaSet) progressBounded(ctx context.Context, addr string, budget time.Duration) (MemberProgress, error) {
	bounded, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return rs.Progress(bounded, addr)
}

// Watermark returns the primary's current applied optime, the position members
// must reach to be considered caught up.
func (rs *ReplicaSet) Watermark(ctx context.Context) (OpTime, error) {
	primary, err := rs.Primary(ctx)
	if err != nil {
		return OpTime{}, err
	}
	progress, err := rs.Progress(ctx, primary.Addr)
	if err != nil {
		return OpTime{}, err
	}
	if progress.Applied.IsZero() {
		return OpTime{}, fmt.Errorf("primary %s reports no applied optime", primary.Addr)
	}
	return progress.Applied, nil
}

// WaitConverged snapshots the primary's applied optime and waits until every
// named member reports having applied at least that position. It returns the
// watermark so callers can assert against a known point in history.
//
// This is what separates "the subject is wrong" from "the subject is slow". A
// comparison run without it produces a divergence report whenever the reader
// simply got there first, and those look identical to real defects.
//
// It trusts what each member reports about itself. That trust is the reason the
// honesty checks exist: a member that claims a position it has not durably
// applied turns this gate into a rubber stamp.
func (rs *ReplicaSet) WaitConverged(ctx context.Context, timeout time.Duration, addrs ...string) (OpTime, error) {
	if len(addrs) == 0 {
		for _, m := range rs.Members {
			addrs = append(addrs, m.Addr)
		}
	}

	watermark, err := rs.Watermark(ctx)
	if err != nil {
		return OpTime{}, err
	}

	deadline := time.Now().Add(timeout)
	last := map[string]memberSample{}
	for time.Now().Before(deadline) {
		caughtUp := 0
		for _, addr := range addrs {
			// Bound each read. An unreachable member otherwise blocks on driver
			// server selection for ~30s, so the caller's timeout is ignored and
			// every failure involving a down member costs half a minute.
			progress, err := rs.progressBounded(ctx, addr, pollBudget(deadline))
			last[addr] = memberSample{progress: progress, err: err, at: time.Now()}
			if err == nil && progress.Applied.Compare(watermark) >= 0 {
				caughtUp++
			}
		}
		if caughtUp == len(addrs) {
			return watermark, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return watermark, convergenceTimeout(watermark, timeout, addrs, last)
}

// convergenceTimeout explains which members are behind and by how much. A bare
// timeout here would cost an hour of manual re-derivation every time.
func convergenceTimeout(watermark OpTime, timeout time.Duration, addrs []string, last map[string]memberSample) error {
	var lines []string
	sorted := append([]string(nil), addrs...)
	sort.Strings(sorted)

	for _, addr := range sorted {
		sample, seen := last[addr]
		switch {
		case !seen:
			lines = append(lines, fmt.Sprintf("  %s was never sampled", addr))
		case sample.err != nil:
			// The member stopped answering. Its last known position is still
			// worth printing: it says whether it died caught up or behind.
			lines = append(lines, fmt.Sprintf(
				"  %s is not answering replSetGetStatus (%v); last known position %s, state %s",
				addr, sample.err, sample.progress.Applied, sample.progress.State))
		case sample.progress.Applied.IsZero():
			lines = append(lines, fmt.Sprintf(
				"  %s reports NO progress at all (applied optime is zero), state %s -- it is not replicating, not merely lagging",
				addr, sample.progress.State))
		case sample.progress.Applied.Compare(watermark) >= 0:
			// Reaching here means every member looked caught up on its own last
			// sample yet the run still timed out, so the reads were not all
			// succeeding in the same pass. Saying nothing would leave an empty
			// report, which is what this function exists to prevent.
			lines = append(lines, fmt.Sprintf(
				"  %s reported reaching %s, but not in the same pass as the others",
				addr, sample.progress.Applied))
		default:
			lines = append(lines, fmt.Sprintf(
				"  %s applied %s, %d seconds behind the watermark, state %s",
				addr, sample.progress.Applied,
				int64(watermark.Seconds)-int64(sample.progress.Applied.Seconds), sample.progress.State))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "  (no members were named, which is itself a harness bug)")
	}
	return fmt.Errorf("members did not converge on %s within %s:\n%s",
		watermark, timeout, strings.Join(lines, "\n"))
}

// memberSample is one poll of a member: what it said, or why it could not be
// asked, and when. Keeping the error means a member that stopped answering is
// reported as such rather than silently retaining a stale caught-up reading.
type memberSample struct {
	progress MemberProgress
	err      error
	at       time.Time
}

// MustConverge fails the test if the members do not converge.
func (rs *ReplicaSet) MustConverge(t *testing.T, ctx context.Context, timeout time.Duration, addrs ...string) OpTime {
	t.Helper()
	watermark, err := rs.WaitConverged(ctx, timeout, addrs...)
	if err != nil {
		t.Fatalf("%v", err)
	}
	return watermark
}

// Status returns a member's raw replSetGetStatus response.
func (rs *ReplicaSet) Status(ctx context.Context, addr string) (bson.M, error) {
	cli, err := rs.client(ctx, addr)
	if err != nil {
		return nil, err
	}
	return replSetGetStatus(ctx, cli)
}

// Hello returns a member's raw hello response. The honesty checks compare it
// against replSetGetStatus: a member whose two self-descriptions disagree is
// misreporting to somebody.
func (rs *ReplicaSet) Hello(ctx context.Context, addr string) (bson.M, error) {
	cli, err := rs.client(ctx, addr)
	if err != nil {
		return nil, err
	}
	var res bson.M
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&res); err != nil {
		return nil, fmt.Errorf("hello %s: %w", addr, err)
	}
	return res, nil
}

// ReadOpTime extracts an OpTime from a replSetGetStatus optimes field.
func ReadOpTime(v interface{}) OpTime { return readOpTime(v) }

// ClientFor returns a caller-owned client pinned to an address, for members
// addressed by string rather than by *Member.
func (rs *ReplicaSet) ClientFor(ctx context.Context, addr string) (*mongo.Client, error) {
	return directClient(ctx, addr)
}
