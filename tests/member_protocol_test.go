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

// Tier 7: DumboDB as a well-behaved member of the set, rather than as a
// correct replica of its data.
//
// Convergence proves the data is right. It cannot see any of this: whether the
// other members can interrogate the subject the way they interrogate each
// other, and whether the subject refuses what it does not implement promptly
// and legibly instead of hanging.
//
//go:build replication

package tests

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"github.com/dolthub/dumbodb-parity-testing/wire"
)

// memberFixture is a converged subject beside a reference mongod secondary, so
// every shape assertion is against a real member rather than against an
// opinion about what the reply should look like.
type memberFixture struct {
	rs        *harness.ReplicaSet
	subject   *harness.DumboMember
	commit    string
	reference *harness.Member
}

func startMemberProtocol(t *testing.T, ctx context.Context) *memberFixture {
	t.Helper()

	rs := harness.StartReplicaSet(t, 2)
	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("AnySecondary: %v", err)
	}
	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)
	if commit == "" || commit == "unknown" {
		t.Fatalf("subject reports gitVersion %q; a failure that cannot name its build is not reproducible", commit)
	}
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 150*time.Second); err != nil {
		t.Fatalf("dumbodb %s did not reach SECONDARY: %v", commit, err)
	}
	return &memberFixture{rs: rs, subject: subject, commit: commit, reference: reference}
}

// runOn sends cmd to addr over OP_MSG and returns the reply, whether or not
// the reply reports an error. A command the server refuses is a result here,
// not a failure: several of these cases are about the refusal.
func runOn(addr string, cmd bson.D) (bson.M, error) {
	conn, err := wire.Dial(addr)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return nil, err
	}
	return conn.RunCommand(cmd)
}

func ok(reply bson.M) bool {
	switch v := reply["ok"].(type) {
	case float64:
		return v == 1
	case int32:
		return v == 1
	case int64:
		return v == 1
	}
	return false
}

// inventedFieldBead tracks the fields below.
const inventedFieldBead = "yiv"

// knownInventedFields are reply fields DumboDB sends that a real mongod member
// does not, graded as known deviations rather than a red build.
//
// Established against mongod 8.0.28 with a live probe, because "mongod never
// sends this" is a claim about a server rather than about documentation:
//
//	replSetGetConfig   a plain call returns ok, config, $clusterTime,
//	                   operationTime and nothing else; asking a secondary for
//	                   commitmentStatus is refused outright with ok: 0.
//	replSetHeartbeat   config appears ONLY when the sender's configVersion is
//	                   stale, which is the optimisation of not reshipping a
//	                   configuration the sender already has. A current
//	                   configVersion gets a reply without it. time never
//	                   appears at all.
//
// Inverted: if a field stops being invented the case fails, demanding it be
// removed from here rather than left to rot.
var knownInventedFields = map[string]map[string]bool{
	"replSetGetConfig": {"commitmentStatus": true},
	"replSetHeartbeat": {"config": true, "time": true},
}

// TestMemberProtocol_InboundCommandShapes compares each inbound command's
// reply against the reference member's.
//
// DumboDB is allowed to omit a field a real member sends; the other members
// tolerate absence. It is not allowed to invent one, because a field a real
// mongod never sends is a field no MongoDB tooling expects, and it is the kind
// of divergence that looks harmless until something keys off it.
func TestMemberProtocol_InboundCommandShapes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	config, err := f.rs.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	setVersion := config["version"]

	cases := []struct {
		name     string
		cmd      bson.D
		required []string
	}{
		{
			name: "replSetGetStatus",
			cmd:  bson.D{{Key: "replSetGetStatus", Value: 1}, {Key: "$db", Value: "admin"}},
			// The other members read state and progress from here. Without
			// these the subject is invisible to an operator and to tooling.
			required: []string{"set", "myState", "members", "optimes"},
		},
		{
			name:     "replSetGetConfig",
			cmd:      bson.D{{Key: "replSetGetConfig", Value: 1}, {Key: "$db", Value: "admin"}},
			required: []string{"config"},
		},
		{
			name:     "replSetGetRBID",
			cmd:      bson.D{{Key: "replSetGetRBID", Value: 1}, {Key: "$db", Value: "admin"}},
			required: []string{"rbid"},
		},
		{
			name: "replSetHeartbeat",
			cmd: bson.D{
				{Key: "replSetHeartbeat", Value: f.rs.Name},
				{Key: "configVersion", Value: setVersion},
				{Key: "from", Value: ""},
				{Key: "term", Value: int64(1)},
				{Key: "$db", Value: "admin"},
			},
			required: []string{"set", "state"},
		},
		{
			name:     "hello",
			cmd:      bson.D{{Key: "hello", Value: 1}, {Key: "$db", Value: "admin"}},
			required: []string{"isWritablePrimary", "setName", "me", "secondary", "hosts", "topologyVersion", "maxWireVersion"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			subjectReply, err := runOn(f.subject.Addr, c.cmd)
			if err != nil {
				t.Fatalf("%s against the subject: %v", c.name, err)
			}
			referenceReply, err := runOn(f.reference.Addr, c.cmd)
			if err != nil {
				t.Fatalf("%s against the reference: %v", c.name, err)
			}
			if !ok(referenceReply) {
				t.Skipf("the reference member refused %s (%v); nothing to compare against",
					c.name, referenceReply["errmsg"])
			}
			if !ok(subjectReply) {
				t.Fatalf("dumbodb %s refused %s, which a real member answers: %v",
					f.commit, c.name, subjectReply["errmsg"])
			}
			for _, field := range c.required {
				if _, present := subjectReply[field]; !present {
					t.Errorf("dumbodb %s: %s reply has no %q; a real member sends it and the other members read it",
						f.commit, c.name, field)
				}
			}
			for field := range subjectReply {
				if _, present := referenceReply[field]; !present {
					if knownInventedFields[c.name][field] {
						t.Logf("XFAIL %s.%s (workspace-%s): carried by dumbodb %s, never sent by a real member",
							c.name, field, inventedFieldBead, f.commit)
						continue
					}
					t.Errorf("dumbodb %s: %s reply carries %q, which a real mongod member never sends",
						f.commit, c.name, field)
				}
			}
			for field := range knownInventedFields[c.name] {
				_, onSubject := subjectReply[field]
				_, onReference := referenceReply[field]
				if !onSubject || onReference {
					t.Errorf("XPASS %s.%s: dumbodb %s no longer invents this field, so remove it from knownInventedFields (workspace-%s)",
						c.name, field, f.commit, inventedFieldBead)
				}
			}
			// Omissions are tolerated rather than failed: the other members
			// cope with a field being absent. They are reported because an
			// unremarked omission is how a field nobody noticed was missing
			// turns into a defect later.
			var omitted []string
			for field := range referenceReply {
				if _, present := subjectReply[field]; !present {
					omitted = append(omitted, field)
				}
			}
			if len(omitted) > 0 {
				sort.Strings(omitted)
				t.Logf("dumbodb %s: %s omits %d field(s) a real member sends: %s",
					f.commit, c.name, len(omitted), strings.Join(omitted, ", "))
			}
		})
	}
}

// TestMemberProtocol_IsSelfIdentifiesTheMember covers _isSelf, which members
// use to work out which configuration entry is their own. A member that
// answers this wrongly can conclude it is not in its own set.
func TestMemberProtocol_IsSelfIdentifiesTheMember(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	cmd := bson.D{{Key: "_isSelf", Value: 1}, {Key: "$db", Value: "admin"}}
	subjectReply, err := runOn(f.subject.Addr, cmd)
	if err != nil {
		t.Fatalf("_isSelf against the subject: %v", err)
	}
	referenceReply, err := runOn(f.reference.Addr, cmd)
	if err != nil {
		t.Fatalf("_isSelf against the reference: %v", err)
	}
	if !ok(referenceReply) {
		t.Skipf("the reference member refused _isSelf (%v)", referenceReply["errmsg"])
	}
	if !ok(subjectReply) {
		t.Fatalf("dumbodb %s refused _isSelf, which a real member answers: %v", f.commit, subjectReply["errmsg"])
	}
	if subjectReply["id"] == nil {
		t.Errorf("dumbodb %s: _isSelf reply has no id", f.commit)
	}
	for field := range subjectReply {
		if _, present := referenceReply[field]; !present {
			t.Errorf("dumbodb %s: _isSelf reply carries %q, which a real mongod member never sends", f.commit, field)
		}
	}
}

// TestMemberProtocol_HelloReportsSecondaryState checks that hello agrees with
// the subject's actual state, since that is what the driver's topology
// monitor and the other members act on.
func TestMemberProtocol_HelloReportsSecondaryState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	reply, err := runOn(f.subject.Addr, bson.D{{Key: "hello", Value: 1}, {Key: "$db", Value: "admin"}})
	if err != nil {
		t.Fatalf("hello: %v", err)
	}
	if reply["isWritablePrimary"] != false {
		t.Errorf("dumbodb %s: hello.isWritablePrimary is %v, want false for a secondary", f.commit, reply["isWritablePrimary"])
	}
	if reply["secondary"] != true {
		t.Errorf("dumbodb %s: hello.secondary is %v while the member is SECONDARY", f.commit, reply["secondary"])
	}
	if got, _ := reply["setName"].(string); got != f.rs.Name {
		t.Errorf("dumbodb %s: hello.setName is %q, want %q", f.commit, got, f.rs.Name)
	}
	if got, _ := reply["me"].(string); got != f.subject.Addr {
		t.Errorf("dumbodb %s: hello.me is %q, want %q", f.commit, got, f.subject.Addr)
	}

	// The subject is hidden, so it must not advertise itself in hosts: a
	// client that saw it there would try to read from a member the set
	// deliberately keeps out of client-visible topology.
	if hosts, isArray := reply["hosts"].(bson.A); isArray {
		for _, host := range hosts {
			if host == f.subject.Addr {
				t.Errorf("dumbodb %s: hello.hosts advertises the hidden member %s", f.commit, f.subject.Addr)
			}
		}
	}

	version, isDoc := reply["topologyVersion"].(bson.M)
	if !isDoc {
		t.Fatalf("dumbodb %s: hello.topologyVersion is %T, want a document", f.commit, reply["topologyVersion"])
	}
	for _, field := range []string{"processId", "counter"} {
		if _, present := version[field]; !present {
			t.Errorf("dumbodb %s: hello.topologyVersion has no %q", f.commit, field)
		}
	}
}

// TestMemberProtocol_AwaitableHelloBlocks covers the awaitable form. A server
// advertising topologyVersion obliges it: the driver sends back the version it
// last saw with maxAwaitTimeMS and expects the call to BLOCK until the
// topology changes or the timeout expires.
//
// Returning immediately is the failure that matters. A driver would then spin,
// re-issuing hello as fast as the network allows against every monitored
// server, which is why this is asserted as a lower bound on elapsed time
// rather than as a reply shape.
func TestMemberProtocol_AwaitableHelloBlocks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	const awaitMS = 2000
	// The reference is measured first and its result gates the assertion. An
	// earlier version dialled a fresh connection for the seed hello and
	// another for the await, and measured the reference returning in about a
	// millisecond: its topologyVersion had moved between the two calls,
	// because the subject had just joined. Returning immediately on a STALE
	// version is correct behavior, so that measurement held the subject to a
	// standard the control was not being asked to meet.
	referenceElapsed, referenceOK := awaitHello(t, f.reference.Addr, awaitMS)
	if !referenceOK || referenceElapsed < time.Duration(awaitMS)*time.Millisecond/2 {
		t.Skipf("premise failed: the reference member returned after %s (ok=%v) for maxAwaitTimeMS=%d, so it is not demonstrating the awaitable contract and there is nothing to hold the subject to",
			referenceElapsed, referenceOK, awaitMS)
	}

	subjectElapsed, subjectOK := awaitHello(t, f.subject.Addr, awaitMS)
	if !subjectOK {
		t.Fatalf("dumbodb %s refused an awaitable hello that the reference member answered", f.commit)
	}
	if subjectElapsed < time.Duration(awaitMS)*time.Millisecond/2 {
		t.Errorf("dumbodb %s: awaitable hello returned after %s for maxAwaitTimeMS=%d, where the reference blocked for %s. An unchanged topology must block, or a driver monitoring this server spins, re-issuing hello as fast as the network allows",
			f.commit, subjectElapsed, awaitMS, referenceElapsed)
	}
	t.Logf("dumbodb %s blocked %s, reference blocked %s", f.commit, subjectElapsed, referenceElapsed)
}

// awaitHello performs the seed hello and the awaiting hello on ONE connection,
// so the topologyVersion sent back is the one the server just issued.
func awaitHello(t *testing.T, addr string, awaitMS int64) (time.Duration, bool) {
	t.Helper()
	conn, err := wire.Dial(addr)
	if err != nil {
		t.Fatalf("dial %s: %v", addr, err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		t.Fatalf("deadline: %v", err)
	}
	seed, err := conn.RunCommand(bson.D{{Key: "hello", Value: 1}, {Key: "$db", Value: "admin"}})
	if err != nil {
		t.Fatalf("hello against %s: %v", addr, err)
	}
	version, isDoc := seed["topologyVersion"].(bson.M)
	if !isDoc {
		t.Fatalf("%s: hello carries no topologyVersion, so the awaitable form cannot be requested", addr)
	}
	start := time.Now()
	reply, err := conn.RunCommand(bson.D{
		{Key: "hello", Value: 1},
		{Key: "topologyVersion", Value: bson.D{
			{Key: "processId", Value: version["processId"]},
			{Key: "counter", Value: version["counter"]},
		}},
		{Key: "maxAwaitTimeMS", Value: awaitMS},
		{Key: "$db", Value: "admin"},
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("awaitable hello against %s: %v", addr, err)
	}
	return elapsed, ok(reply)
}

// TestMemberProtocol_ExhaustHelloSetsMoreToCome covers the exhaust form, which
// is invisible to the Go driver and to a plain OP_MSG send: it lives entirely
// in the flag bits.
func TestMemberProtocol_ExhaustHelloSetsMoreToCome(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	probe := func(addr string) (bson.M, uint32) {
		conn, err := wire.Dial(addr)
		if err != nil {
			t.Fatalf("dial %s: %v", addr, err)
		}
		defer func() { _ = conn.Close() }()
		if err := conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
			t.Fatalf("deadline: %v", err)
		}
		seed, _, err := conn.RunCommandFlags(bson.D{{Key: "hello", Value: 1}, {Key: "$db", Value: "admin"}}, 0)
		if err != nil {
			t.Fatalf("hello against %s: %v", addr, err)
		}
		version, isDoc := seed["topologyVersion"].(bson.M)
		if !isDoc {
			t.Fatalf("%s: no topologyVersion, cannot request an exhaust stream", addr)
		}
		reply, flags, err := conn.RunCommandFlags(bson.D{
			{Key: "hello", Value: 1},
			{Key: "topologyVersion", Value: bson.D{
				{Key: "processId", Value: version["processId"]},
				{Key: "counter", Value: version["counter"]},
			}},
			{Key: "maxAwaitTimeMS", Value: int64(1000)},
			{Key: "$db", Value: "admin"},
		}, wire.FlagExhaustAllowed)
		if err != nil {
			t.Fatalf("exhaust hello against %s: %v", addr, err)
		}
		return reply, flags
	}

	referenceReply, referenceFlags := probe(f.reference.Addr)
	if !ok(referenceReply) || referenceFlags&wire.FlagMoreToCome == 0 {
		t.Skipf("the reference member did not open an exhaust stream (ok=%v flags=%#x); nothing to hold the subject to",
			ok(referenceReply), referenceFlags)
	}

	subjectReply, subjectFlags := probe(f.subject.Addr)
	if !ok(subjectReply) {
		t.Fatalf("dumbodb %s refused an exhaust hello that the reference member accepted: %v",
			f.commit, subjectReply["errmsg"])
	}
	if subjectFlags&wire.FlagMoreToCome == 0 {
		t.Errorf("dumbodb %s: exhaust hello replied with flags %#x, moreToCome clear, where the reference set it. A driver that asked for a stream is left waiting for frames that never come",
			f.commit, subjectFlags)
	}
}

// TestMemberProtocol_RefusesDownstreamSyncPromptly covers the deliberate
// deviation: DumboDB does not serve its oplog to downstream members.
//
// This path is reachable in practice rather than theoretical. A voting
// secondary will try to chain from a hidden non-voter on its second sync
// source selection pass, so the refusal has to arrive quickly and say what it
// is. A hang here would stall the member that tried to chain, which is a worse
// outcome than the refusal itself.
func TestMemberProtocol_RefusesDownstreamSyncPromptly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	const promptly = 5 * time.Second

	cases := []struct {
		name       string
		cmd        bson.D
		anyRefusal bool
	}{
		{
			name: "find on local.oplog.rs",
			cmd: bson.D{
				{Key: "find", Value: "oplog.rs"},
				{Key: "filter", Value: bson.D{}},
				{Key: "limit", Value: int32(1)},
				{Key: "$db", Value: "local"},
			},
		},
		{
			name: "getMore on local.oplog.rs",
			cmd: bson.D{
				{Key: "getMore", Value: int64(1)},
				{Key: "collection", Value: "oplog.rs"},
				{Key: "$db", Value: "local"},
			},
			// The cursor id is fabricated, because there is no way to obtain a
			// real one: find on local.oplog.rs is refused, so a downstream
			// member never gets far enough to call getMore. The refusal in
			// msg_getmore.go is defense in depth that no external test can
			// reach, and the reachable outcome is a cursor-not-found refusal.
			// Requiring the downstream wording here would be asserting against
			// a path the caller cannot take.
			anyRefusal: true,
		},
		{
			name: "replSetUpdatePosition",
			cmd: bson.D{
				{Key: "replSetUpdatePosition", Value: 1},
				{Key: "optimes", Value: bson.A{}},
				{Key: "$db", Value: "admin"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			start := time.Now()
			reply, err := runOn(f.subject.Addr, c.cmd)
			elapsed := time.Since(start)
			if err != nil {
				t.Fatalf("dumbodb %s: %s did not answer at all: %v. A refusal has to be a reply, not a dropped connection",
					f.commit, c.name, err)
			}
			if elapsed > promptly {
				t.Errorf("dumbodb %s: %s took %s to refuse; a member chaining from this one is stalled for that long",
					f.commit, c.name, elapsed)
			}
			if ok(reply) {
				t.Fatalf("dumbodb %s: %s succeeded. DumboDB does not serve downstream oplog replication, so a success here means a downstream member will start following a stream this server cannot honour",
					f.commit, c.name)
			}
			message, _ := reply["errmsg"].(string)
			if c.anyRefusal {
				t.Logf("%s refused in %s: code=%v %q", c.name, elapsed, reply["code"], message)
				return
			}
			if !strings.Contains(message, "downstream") {
				t.Errorf("dumbodb %s: %s was refused with %q, which does not say what was refused. An operator reading this in a log of the member that tried to chain needs to learn that downstream sync is unsupported",
					f.commit, c.name, message)
			}
			t.Logf("%s refused in %s: code=%v %q", c.name, elapsed, reply["code"], message)
		})
	}
}

// TestMemberProtocol_ReferenceServesItsOwnOplog establishes that the refusal
// above is a deliberate DumboDB deviation rather than something the apparatus
// blocks. Without this the refusal tests would pass against a broken harness.
func TestMemberProtocol_ReferenceServesItsOwnOplog(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startMemberProtocol(t, ctx)

	reply, err := runOn(f.reference.Addr, bson.D{
		{Key: "find", Value: "oplog.rs"},
		{Key: "filter", Value: bson.D{}},
		{Key: "limit", Value: int32(1)},
		{Key: "$db", Value: "local"},
	})
	if err != nil {
		t.Fatalf("find on the reference oplog: %v", err)
	}
	if !ok(reply) {
		t.Fatalf("premise failed: the reference mongod secondary refused to serve its own oplog (%v), so the subject's refusal proves nothing about DumboDB",
			reply["errmsg"])
	}
}
