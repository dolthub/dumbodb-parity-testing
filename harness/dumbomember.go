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
	"os"
	"os/exec"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// dumboMemberGrace bounds a graceful shutdown before escalating to SIGKILL.
const dumboMemberGrace = 15 * time.Second

// DumboMember is a DumboDB process joined to a ReplicaSet as the subject under
// test: hidden, priority 0, votes 0.
//
// Its data directory survives Stop and Kill so the process can be relaunched
// onto the same state, which is what the resume and durability cases need.
type DumboMember struct {
	*Member
	DataDir string

	bin  string
	rs   *ReplicaSet
	proc *serverProc
	t    *testing.T
}

// JoinDumboDB launches DumboDB, adds it to rs as a hidden non-voting member,
// and waits for the configuration to install. It does NOT wait for the member
// to reach SECONDARY: reaching it is the subject's job and the thing under
// test, not a harness precondition.
func (rs *ReplicaSet) JoinDumboDB(t *testing.T) *DumboMember {
	t.Helper()

	bin := findDumboDBBinary()
	if bin == "" {
		t.Fatalf("dumbodb binary not found (set DUMBODB_BIN)")
	}

	port, err := freePort()
	if err != nil {
		t.Fatalf("JoinDumboDB: %v", err)
	}
	dir, err := os.MkdirTemp("", "dumbodb-member-")
	if err != nil {
		t.Fatalf("JoinDumboDB: %v", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	d := &DumboMember{
		Member:  &Member{ID: rs.nextMemberID(), Addr: addr, URI: "mongodb://" + addr},
		DataDir: dir,
		bin:     bin,
		rs:      rs,
		t:       t,
	}
	t.Cleanup(d.teardown)

	d.launch()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := rs.addHiddenMember(ctx, d.Member); err != nil {
		t.Fatalf("JoinDumboDB: %v", err)
	}
	rs.Members = append(rs.Members, d.Member)
	return d
}

func (d *DumboMember) launch() {
	d.t.Helper()
	cmd := exec.Command(d.bin, "--replSet", d.rs.Name, "--addr", d.Addr, "--data-dir", d.DataDir)
	// Pass an empty dir so serverProc never removes it; teardown owns removal
	// after the final shutdown, so a relaunch finds its state intact.
	proc, err := startProc(cmd, "dumbodb-member", "")
	if err != nil {
		d.t.Fatalf("launch dumbodb: %v", err)
	}
	d.proc = proc
	if !waitPort(d.Addr, 60*time.Second) {
		d.t.Fatalf("dumbodb did not listen on %s (log %s)", d.Addr, proc.log)
	}
	// Same reason the mongod spawn waits for a real response: the reconfig
	// that adds this member runs a quorum check against it, and an open port
	// is not an answer.
	if err := waitServerReady(d.Addr, 60*time.Second); err != nil {
		d.t.Fatalf("dumbodb on %s never became ready (log %s): %v", d.Addr, proc.log, err)
	}
}

// Stop shuts the member down gracefully, leaving its data directory intact.
func (d *DumboMember) Stop() {
	if d.proc != nil {
		d.proc.shutdownGraceful(dumboMemberGrace)
		d.proc = nil
	}
}

// Kill terminates the member without a clean shutdown, for crash-recovery
// cases. The data directory is left as the process left it.
func (d *DumboMember) Kill() {
	if d.proc == nil {
		return
	}
	if d.proc.cmd != nil && d.proc.cmd.Process != nil {
		_ = d.proc.cmd.Process.Kill()
		_, _ = d.proc.cmd.Process.Wait()
	}
	if d.proc.logf != nil {
		_ = d.proc.logf.Close()
	}
	d.proc = nil
}

// Start relaunches a stopped member on the same address and data directory.
func (d *DumboMember) Start() {
	d.t.Helper()
	if d.proc != nil {
		d.t.Fatal("DumboMember.Start: already running")
	}
	d.launch()
}

// Restart is a graceful stop followed by a relaunch onto the same state.
func (d *DumboMember) Restart() {
	d.Stop()
	d.Start()
}

// Commit returns the subject's build commit from buildInfo.gitVersion.
//
// BSONnet changes DumboDB continuously, so a failure that does not name the
// commit it was produced against is not reproducible. Asking the running server
// avoids trusting an environment variable to describe the binary.
func (d *DumboMember) Commit(ctx context.Context) (string, error) {
	cli, err := directClient(ctx, d.Addr)
	if err != nil {
		return "", err
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	var res bson.M
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&res); err != nil {
		return "", fmt.Errorf("buildInfo: %w", err)
	}
	return asString(res["gitVersion"]), nil
}

// Client returns a client pinned to the member.
func (d *DumboMember) Client(ctx context.Context) (*mongo.Client, error) {
	return directClient(ctx, d.Addr)
}

// ReadLog returns the current process log.
func (d *DumboMember) ReadLog() (string, error) {
	if d.proc == nil {
		return "", fmt.Errorf("dumbodb member %s is not running", d.Addr)
	}
	contents, err := os.ReadFile(d.proc.log)
	if err != nil {
		return "", fmt.Errorf("read dumbodb member log: %w", err)
	}
	return string(contents), nil
}

// AssertHiddenNonVoting fails unless the installed configuration carries all
// three properties.
//
// The design document states that relying on only hidden or only priority 0 is
// unsafe, so all three are checked on every join rather than assumed from a
// reconfig that returned ok.
func (d *DumboMember) AssertHiddenNonVoting(ctx context.Context) {
	d.t.Helper()
	cfg, err := d.rs.Config(ctx)
	if err != nil {
		d.t.Fatalf("AssertHiddenNonVoting: %v", err)
	}
	for _, raw := range asArray(cfg["members"]) {
		m, ok := raw.(bson.M)
		if !ok || asString(m["host"]) != d.Addr {
			continue
		}
		if m["hidden"] != true {
			d.t.Errorf("member %s: hidden is %v, want true", d.Addr, m["hidden"])
		}
		if got := asInt64(m["priority"]); got != 0 {
			d.t.Errorf("member %s: priority is %d, want 0", d.Addr, got)
		}
		if got := asInt64(m["votes"]); got != 0 {
			d.t.Errorf("member %s: votes is %d, want 0", d.Addr, got)
		}
		return
	}
	d.t.Fatalf("member %s is not in the configuration of %s", d.Addr, d.rs.Name)
}

// teardown ends the member the fast way: the set is disposable and about to be
// killed with it, so nothing is gained by leaving it tidy.
//
// It used to reconfigure the member out of the set first, so the survivors
// would not heartbeat a corpse, and then stop the process with SIGTERM and a
// fifteen second grace. Both are pointless at the end of a test: the remaining
// members are killed moments later, and no assertion can run after this. A
// graceful stop is still available as Stop, for the restart and durability
// cases that deliberately exercise clean shutdown, which is test content
// rather than cleanup.
func (d *DumboMember) teardown() {
	// A set this harness spawned dies moments from now, so removing the member
	// from its configuration first is wasted work. An adopted set does not:
	// MONGO_REPL_SET_URI points at something that outlives the test, and
	// leaving a dead member behind means later tests adopt a corpse and wait
	// out their convergence deadline against a member that will never report.
	if d.rs.external {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := d.rs.removeMember(ctx, d.Addr); err != nil {
			d.t.Logf("removing %s from the adopted set %s: %v", d.Addr, d.rs.Name, err)
		}
	}
	d.Kill()
	if d.DataDir != "" {
		_ = os.RemoveAll(d.DataDir)
	}
}

// nextMemberID returns an unused member _id.
func (rs *ReplicaSet) nextMemberID() int {
	highest := -1
	for _, m := range rs.Members {
		if m.ID > highest {
			highest = m.ID
		}
	}
	return highest + 1
}

func (rs *ReplicaSet) addHiddenMember(ctx context.Context, m *Member) error {
	err := rs.Reconfig(ctx, func(cfg bson.M) error {
		members := asArray(cfg["members"])
		for _, raw := range members {
			if existing, ok := raw.(bson.M); ok && asString(existing["host"]) == m.Addr {
				return fmt.Errorf("member %s is already configured", m.Addr)
			}
		}
		cfg["members"] = append(members, bson.M{
			"_id":      m.ID,
			"host":     m.Addr,
			"hidden":   true,
			"priority": 0,
			"votes":    0,
		})
		return nil
	})
	if err != nil {
		return fmt.Errorf("adding %s as a hidden non-voting member: %w", m.Addr, err)
	}
	return nil
}

func (rs *ReplicaSet) removeMember(ctx context.Context, addr string) error {
	err := rs.Reconfig(ctx, func(cfg bson.M) error {
		kept := bson.A{}
		for _, raw := range asArray(cfg["members"]) {
			if m, ok := raw.(bson.M); ok && asString(m["host"]) == addr {
				continue
			}
			kept = append(kept, raw)
		}
		cfg["members"] = kept
		return nil
	})
	if err != nil {
		return err
	}
	for i, m := range rs.Members {
		if m.Addr == addr {
			rs.Members = append(rs.Members[:i], rs.Members[i+1:]...)
			break
		}
	}
	return nil
}

// RemoveMember drops a member from the replica set configuration. This is
// MongoDB's way to stop a member replicating, and since dumbodb 3f3bc13 it is
// the only way: the dumboReplicationDetach command was removed in favour of it.
func (rs *ReplicaSet) RemoveMember(ctx context.Context, addr string) error {
	return rs.removeMember(ctx, addr)
}

// AddMember re-adds a member to the configuration as hidden, priority 0,
// votes 0, which is how a member removed with RemoveMember rejoins.
func (rs *ReplicaSet) AddMember(ctx context.Context, m *Member) error {
	if err := rs.addHiddenMember(ctx, m); err != nil {
		return err
	}
	rs.Members = append(rs.Members, m)
	return nil
}
