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
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// These exercise real mongod processes. Unlike StartEphemeralServers, a missing
// binary skips rather than fails: this is harness self-test infrastructure and
// no CI job provisions mongod for it yet. Once the replication CI job exists,
// mongod is present and these run.
func requireMongod(t *testing.T) {
	t.Helper()
	if findMongodBin() == "" && os.Getenv(replSetURIEnv) == "" {
		t.Skipf("no mongod (set MONGOD_BIN or %s)", replSetURIEnv)
	}
}

func TestReplicaSet_ProvisionsPrimaryAndReference(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	secondaries, err := rs.Secondaries(ctx)
	if err != nil {
		t.Fatalf("Secondaries: %v", err)
	}
	if len(secondaries) != 1 {
		t.Fatalf("want exactly 1 reference secondary, got %d", len(secondaries))
	}
	if secondaries[0].Addr == primary.Addr {
		t.Fatalf("primary %s also reported as secondary", primary.Addr)
	}
	t.Logf("set %s: primary=%s reference=%s", rs.Name, primary.Addr, secondaries[0].Addr)

	cfg, err := rs.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	if got := len(asArray(cfg["members"])); got != 2 {
		t.Fatalf("config reports %d members, want 2", got)
	}
}

// A write on the primary must be readable from the reference secondary. This is
// the property every convergence test depends on, so the harness proves it
// before anything is built on top.
func TestReplicaSet_ReferenceSecondaryReceivesWrites(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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

	coll := pc.Database("harness_rs").Collection("seed")
	if _, err := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: "written"}}); err != nil {
		t.Fatalf("insert on primary: %v", err)
	}

	secondaries, err := rs.Secondaries(ctx)
	if err != nil || len(secondaries) == 0 {
		t.Fatalf("Secondaries: %v (n=%d)", err, len(secondaries))
	}
	sc, err := rs.DirectClient(ctx, secondaries[0])
	if err != nil {
		t.Fatalf("secondary client: %v", err)
	}
	defer func() { _ = sc.Disconnect(context.Background()) }()

	scoll := sc.Database("harness_rs").Collection("seed")
	deadline := time.Now().Add(30 * time.Second)
	for {
		var got bson.M
		err := scoll.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&got)
		if err == nil {
			if got["v"] != "written" {
				t.Fatalf("secondary holds %v, want v=written", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("write never reached the reference secondary: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Reconfig is how DumboDB gets added to the set, so the version bump and
// install path are proven here rather than discovered during the join.
func TestReplicaSet_ReconfigBumpsVersionAndInstalls(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	before, err := rs.Config(ctx)
	if err != nil {
		t.Fatalf("Config: %v", err)
	}
	beforeVersion := asInt64(before["version"])

	// Must target a secondary: making the current primary non-electable is
	// rejected with NodeNotElectable, and config array position says nothing
	// about role.
	target, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("AnySecondary: %v", err)
	}

	err = rs.ReconfigMember(ctx, target.Addr, func(member bson.M) error {
		member["priority"] = 0
		member["votes"] = 0
		member["hidden"] = true
		return nil
	})
	if err != nil {
		t.Fatalf("ReconfigMember(%s): %v", target.Addr, err)
	}

	after, err := rs.Config(ctx)
	if err != nil {
		t.Fatalf("Config after reconfig: %v", err)
	}
	if got := asInt64(after["version"]); got <= beforeVersion {
		t.Fatalf("config version did not advance: before=%d after=%d", beforeVersion, got)
	}

	var installed bson.M
	for _, raw := range asArray(after["members"]) {
		m, _ := raw.(bson.M)
		if m != nil && asString(m["host"]) == target.Addr {
			installed = m
		}
	}
	if installed == nil {
		t.Fatalf("member %s vanished from the configuration", target.Addr)
	}
	if asInt64(installed["priority"]) != 0 || asInt64(installed["votes"]) != 0 || installed["hidden"] != true {
		t.Fatalf("hidden/priority/votes did not install on %s: %v", target.Addr, installed)
	}
}

// Making the primary non-electable must be refused. e8a.2 joins DumboDB as
// priority:0/votes:0, so a harness that silently reconfigured the wrong member
// would produce a set with no valid subject and no error.
func TestReplicaSet_ReconfigMemberRejectsUnknownHost(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	err := rs.ReconfigMember(ctx, "127.0.0.1:1", func(bson.M) error { return nil })
	if err == nil {
		t.Fatal("expected an error reconfiguring a host that is not a member")
	}
	if !contains(err.Error(), "not in the configuration") {
		t.Fatalf("error should name the problem; got %q", err)
	}
}

func TestReplicaSet_StepDownElectsANewPrimary(t *testing.T) {
	requireMongod(t)
	if testing.Short() {
		t.Skip("step-down election is slow")
	}
	rs := StartReplicaSet(t, 3)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	old, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	fresh, err := rs.StepDownPrimary(ctx, 30)
	if err != nil {
		t.Fatalf("StepDownPrimary: %v", err)
	}
	if fresh.Addr == old.Addr {
		t.Fatalf("primary did not move: still %s", old.Addr)
	}
	t.Logf("primary moved %s -> %s", old.Addr, fresh.Addr)
}

// WaitForState must report the state actually reached, not just time out.
func TestReplicaSet_WaitForStateReportsActualState(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	err = rs.WaitForState(ctx, primary, StateRecovering, 2*time.Second)
	if err == nil {
		t.Fatal("expected a timeout waiting for RECOVERING on the primary")
	}
	if want := "last state PRIMARY"; !contains(err.Error(), want) {
		t.Fatalf("error must name the state reached; got %q, want it to contain %q", err, want)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
