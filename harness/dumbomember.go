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
	"go.mongodb.org/mongo-driver/mongo/options"
)

const dumboMemberGrace = 15 * time.Second

type DumboMember struct {
	*Member
	DataDir string

	bin  string
	rs   *ReplicaSet
	proc *serverProc
	t    *testing.T
	opts DumboMemberOptions
}

// JoinDumboDB adds a DumboDB member configured to match the set.
func (rs *ReplicaSet) JoinDumboDB(t *testing.T) *DumboMember {
	t.Helper()
	return rs.JoinDumboDBWith(t, rs.MatchingDumboOptions())
}

// JoinDumboDBWith adds a DumboDB member with an explicit configuration, which
// need not match the set's. A deliberate mismatch is the subject of several
// cases rather than a mistake.
func (rs *ReplicaSet) JoinDumboDBWith(t *testing.T, opts DumboMemberOptions) *DumboMember {
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
		opts:    opts,
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
	args := []string{"--replSet", d.rs.Name, "--addr", d.Addr, "--data-dir", d.DataDir}
	cmd := exec.Command(d.bin, append(args, d.opts.memberArgs()...)...)
	proc, err := startProc(cmd, "dumbodb-member", "")
	if err != nil {
		d.t.Fatalf("launch dumbodb: %v", err)
	}
	d.proc = proc
	if !waitPort(d.Addr, 60*time.Second) {
		d.t.Fatalf("dumbodb did not listen on %s (log %s)", d.Addr, proc.log)
	}
	if err := d.waitReady(60 * time.Second); err != nil {
		d.t.Fatalf("dumbodb on %s never became ready (log %s): %v", d.Addr, proc.log, err)
	}
}

func (d *DumboMember) Stop() {
	if d.proc != nil {
		d.proc.shutdownGraceful(dumboMemberGrace)
		d.proc = nil
	}
}

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

func (d *DumboMember) Start() {
	d.t.Helper()
	if d.proc != nil {
		d.t.Fatal("DumboMember.Start: already running")
	}
	d.launch()
}

func (d *DumboMember) Restart() {
	d.Stop()
	d.Start()
}

func (d *DumboMember) Commit(ctx context.Context) (string, error) {
	cli, err := d.dial(ctx)
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

func (d *DumboMember) Client(ctx context.Context) (*mongo.Client, error) {
	return d.dial(ctx)
}

// dial connects to the member using its own TLS material and the set's
// credentials.
//
// The member's configuration decides TLS, because a member may deliberately
// differ from the set; the credentials come from the set, because the users a
// client authenticates as arrive by replication.
func (d *DumboMember) dial(ctx context.Context) (*mongo.Client, error) {
	return d.dialWith(ctx, d.rs.cred)
}

func (d *DumboMember) dialWith(ctx context.Context, cred *options.Credential) (*mongo.Client, error) {
	uri := "mongodb://" + d.Addr + "/?directConnection=true"
	opts := options.Client().ApplyURI(uri)
	if f := d.opts.TLS; f != nil {
		config, err := clientTLSConfig(f)
		if err != nil {
			return nil, err
		}
		opts = opts.SetTLSConfig(config)
	}
	if cred != nil {
		opts = opts.SetAuth(*cred)
	}
	cli, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", d.Addr, err)
	}
	return cli, nil
}

// waitReady polls the member until it answers ping, deliberately without
// credentials.
//
// A member that has just started has replicated nothing, so the set's users do
// not exist on it yet and authenticating would fail for as long as the timeout
// allows. ping is exempt from access control, which is what makes an
// unauthenticated readiness check possible at all.
func (d *DumboMember) waitReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cli, err := d.dialWith(ctx, nil)
		if err == nil {
			err = cli.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err()
			_ = cli.Disconnect(context.Background())
		}
		cancel()
		if err == nil {
			return nil
		}
		last = err
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("no response to ping on %s within %s: %w", d.Addr, timeout, last)
}

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

func (d *DumboMember) teardown() {
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

func (rs *ReplicaSet) RemoveMember(ctx context.Context, addr string) error {
	return rs.removeMember(ctx, addr)
}

func (rs *ReplicaSet) AddMember(ctx context.Context, m *Member) error {
	if err := rs.addHiddenMember(ctx, m); err != nil {
		return err
	}
	rs.Members = append(rs.Members, m)
	return nil
}
