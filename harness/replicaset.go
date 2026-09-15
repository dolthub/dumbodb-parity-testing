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
	"strings"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// replSetURIEnv names a pre-provisioned multi-member set. Deliberately not
// MONGO_RS_URI: that one is the single-node set backing TopologyReplicaSet
// transaction tests, and pointing replication tests at it would silently give
// them a set with no secondary to use as a reference.
const replSetURIEnv = "MONGO_REPL_SET_URI"

// Member states as reported in replSetGetStatus members[].stateStr.
const (
	StatePrimary    = "PRIMARY"
	StateSecondary  = "SECONDARY"
	StateStartup2   = "STARTUP2"
	StateRecovering = "RECOVERING"
)

// ReplicaSet is a running MongoDB replica set owned by one test. It is the
// apparatus replication parity tests are built on: a primary to drive, a stock
// mongod secondary to use as the reference, and room to join DumboDB as a
// third member.
type ReplicaSet struct {
	Name    string
	Members []*Member

	t        *testing.T
	external bool

	mu   sync.Mutex
	pool map[string]*mongo.Client
}

// Member is one participant in a ReplicaSet. Members this harness spawned carry
// a proc; members joined from outside (a pre-provisioned set, or DumboDB) do
// not, and are never stopped by ReplicaSet teardown.
type Member struct {
	ID   int
	Addr string
	URI  string

	proc *serverProc
}

// StartReplicaSet provisions an n-member mongod replica set and waits for a
// primary. Teardown is registered with t.Cleanup. When replSetURIEnv is set the
// set is assumed to already exist and n is ignored.
func StartReplicaSet(t *testing.T, n int) *ReplicaSet {
	t.Helper()
	if n < 1 {
		t.Fatalf("StartReplicaSet: need at least 1 member, got %d", n)
	}

	if uri := os.Getenv(replSetURIEnv); uri != "" {
		return adoptReplicaSet(t, uri)
	}

	bin := findMongodBin()
	if bin == "" {
		t.Fatalf("mongod binary not found (set MONGOD_BIN or %s)", replSetURIEnv)
	}

	name := fmt.Sprintf("repl%d", time.Now().UnixNano()%100000)
	rs := &ReplicaSet{Name: name, t: t}
	t.Cleanup(rs.stop)

	for i := 0; i < n; i++ {
		m, err := rs.spawnMongod(bin, i)
		if err != nil {
			t.Fatalf("StartReplicaSet: member %d: %v", i, err)
		}
		rs.Members = append(rs.Members, m)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := rs.initiate(ctx); err != nil {
		t.Fatalf("StartReplicaSet: %v", err)
	}
	return rs
}

func (rs *ReplicaSet) spawnMongod(bin string, id int) (*Member, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", fmt.Sprintf("mongod-%s-%d-", rs.Name, id))
	if err != nil {
		return nil, err
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	cmd := exec.Command(bin,
		"--replSet", rs.Name,
		"--port", fmt.Sprintf("%d", port),
		"--dbpath", dir,
		"--bind_ip", "127.0.0.1",
		"--nounixsocket",
	)
	proc, err := startProc(cmd, fmt.Sprintf("mongod-%s-%d", rs.Name, id), dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if !waitPort(addr, 40*time.Second) {
		return nil, fmt.Errorf("mongod %s did not listen on %s (log %s)", rs.Name, addr, proc.log)
	}
	return &Member{ID: id, Addr: addr, URI: "mongodb://" + addr, proc: proc}, nil
}

// initiate configures the set with every spawned member and waits for an
// elected primary.
func (rs *ReplicaSet) initiate(ctx context.Context) error {
	seed := rs.Members[0]
	cli, err := directClient(ctx, seed.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	members := bson.A{}
	for _, m := range rs.Members {
		members = append(members, bson.D{{Key: "_id", Value: m.ID}, {Key: "host", Value: m.Addr}})
	}
	cmd := bson.D{{Key: "replSetInitiate", Value: bson.D{
		{Key: "_id", Value: rs.Name},
		{Key: "members", Value: members},
	}}}
	if err := cli.Database("admin").RunCommand(ctx, cmd).Err(); err != nil &&
		!strings.Contains(err.Error(), "already initialized") {
		return fmt.Errorf("replSetInitiate %s: %w", rs.Name, err)
	}
	if _, err := rs.WaitForPrimary(ctx, 60*time.Second); err != nil {
		return err
	}
	return rs.WaitForSteadyState(ctx, 90*time.Second)
}

// WaitForSteadyState blocks until exactly one member reports PRIMARY and every
// other member reports SECONDARY.
//
// Which member wins the election is not predictable, so this must not assume
// Members[0] is the primary: doing so produces a test that passes or fails
// depending on who won, which is the worst kind of flake because it looks like
// a subject bug.
func (rs *ReplicaSet) WaitForSteadyState(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last map[string]string
	for time.Now().Before(deadline) {
		byAddr, err := rs.memberStates(ctx)
		if err == nil {
			last = byAddr
			primaries, secondaries := 0, 0
			for _, m := range rs.Members {
				switch byAddr[m.Addr] {
				case StatePrimary:
					primaries++
				case StateSecondary:
					secondaries++
				}
			}
			if primaries == 1 && primaries+secondaries == len(rs.Members) {
				return nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("replica set %s did not reach steady state within %s (states: %v)", rs.Name, timeout, last)
}

// adoptReplicaSet takes over a set that already exists, discovering its members
// from replSetGetStatus. Adopted members are never stopped on teardown.
func adoptReplicaSet(t *testing.T, uri string) *ReplicaSet {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	addr := hostPort(uri)
	if !waitPort(addr, 15*time.Second) {
		t.Fatalf("%s=%s is not reachable", replSetURIEnv, uri)
	}
	cli, err := directClient(ctx, addr)
	if err != nil {
		t.Fatalf("adopt %s: %v", uri, err)
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	status, err := replSetGetStatus(ctx, cli)
	if err != nil {
		t.Fatalf("adopt %s: %v", uri, err)
	}
	rs := &ReplicaSet{Name: asString(status["set"]), t: t, external: true}
	for _, raw := range asArray(status["members"]) {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		host := asString(m["name"])
		rs.Members = append(rs.Members, &Member{
			ID:   int(asInt64(m["_id"])),
			Addr: host,
			URI:  "mongodb://" + host,
		})
	}
	if len(rs.Members) == 0 {
		t.Fatalf("adopt %s: replSetGetStatus reported no members", uri)
	}
	return rs
}

func (rs *ReplicaSet) stop() {
	rs.closePool()
	for _, m := range rs.Members {
		if m.proc != nil {
			m.proc.stop()
		}
	}
}

// URI returns a replica-set-aware connection string naming every member.
func (rs *ReplicaSet) URI() string {
	hosts := make([]string, 0, len(rs.Members))
	for _, m := range rs.Members {
		hosts = append(hosts, m.Addr)
	}
	return fmt.Sprintf("mongodb://%s/?replicaSet=%s", strings.Join(hosts, ","), rs.Name)
}

// Primary returns the member that is accepting writes.
//
// The candidate comes from replSetGetStatus, but stateStr reports PRIMARY
// during step-up before the node will accept a write, so the candidate is
// confirmed with hello.isWritablePrimary. Trusting stateStr alone yields a
// NotWritablePrimary error on the first insert of a freshly elected set.
func (rs *ReplicaSet) Primary(ctx context.Context) (*Member, error) {
	byAddr, err := rs.memberStates(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range rs.Members {
		if byAddr[m.Addr] != StatePrimary {
			continue
		}
		writable, err := rs.isWritablePrimary(ctx, m.Addr)
		if err != nil {
			return nil, err
		}
		if writable {
			return m, nil
		}
		return nil, fmt.Errorf("member %s reports PRIMARY but is not yet accepting writes", m.Addr)
	}
	return nil, fmt.Errorf("replica set %s has no primary (states: %v)", rs.Name, byAddr)
}

func (rs *ReplicaSet) isWritablePrimary(ctx context.Context, addr string) (bool, error) {
	cli, err := rs.client(ctx, addr)
	if err != nil {
		return false, err
	}
	var res bson.M
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&res); err != nil {
		return false, fmt.Errorf("hello %s: %w", addr, err)
	}
	w, _ := res["isWritablePrimary"].(bool)
	return w, nil
}

// Secondaries returns every member currently reporting SECONDARY.
func (rs *ReplicaSet) Secondaries(ctx context.Context) ([]*Member, error) {
	byAddr, err := rs.memberStates(ctx)
	if err != nil {
		return nil, err
	}
	var out []*Member
	for _, m := range rs.Members {
		if byAddr[m.Addr] == StateSecondary {
			out = append(out, m)
		}
	}
	return out, nil
}

// WaitForPrimary blocks until some member reports PRIMARY.
func (rs *ReplicaSet) WaitForPrimary(ctx context.Context, timeout time.Duration) (*Member, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		m, err := rs.Primary(ctx)
		if err == nil {
			return m, nil
		}
		last = err
		time.Sleep(300 * time.Millisecond)
	}
	return nil, fmt.Errorf("replica set %s elected no primary within %s: %w", rs.Name, timeout, last)
}

// WaitForState blocks until the set reports want for m. The error names the
// state actually reached, because "timed out" alone is not diagnosable.
func (rs *ReplicaSet) WaitForState(ctx context.Context, m *Member, want string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	got := "<unknown>"
	for time.Now().Before(deadline) {
		byAddr, err := rs.memberStates(ctx)
		if err == nil {
			if s, ok := byAddr[m.Addr]; ok {
				if s == want {
					return nil
				}
				got = s
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("member %s did not reach %s within %s (last state %s)", m.Addr, want, timeout, got)
}

// memberStates maps member address to stateStr, asked of whichever member
// answers. replSetGetStatus reports the whole set from any member's view.
func (rs *ReplicaSet) memberStates(ctx context.Context) (map[string]string, error) {
	var lastErr error
	for _, m := range rs.Members {
		cli, err := rs.client(ctx, m.Addr)
		if err != nil {
			lastErr = err
			continue
		}
		status, err := replSetGetStatus(ctx, cli)
		if err != nil {
			lastErr = err
			continue
		}
		out := map[string]string{}
		for _, raw := range asArray(status["members"]) {
			mm, ok := raw.(bson.M)
			if !ok {
				continue
			}
			out[asString(mm["name"])] = asString(mm["stateStr"])
		}
		return out, nil
	}
	return nil, fmt.Errorf("no member of %s answered replSetGetStatus: %w", rs.Name, lastErr)
}

// Config returns the installed replica set configuration document.
func (rs *ReplicaSet) Config(ctx context.Context) (bson.M, error) {
	p, err := rs.Primary(ctx)
	if err != nil {
		return nil, err
	}
	cli, err := rs.client(ctx, p.Addr)
	if err != nil {
		return nil, err
	}
	var res bson.M
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetConfig", Value: 1}}).Decode(&res); err != nil {
		return nil, fmt.Errorf("replSetGetConfig: %w", err)
	}
	cfg, ok := res["config"].(bson.M)
	if !ok {
		return nil, fmt.Errorf("replSetGetConfig returned no config: %v", res)
	}
	return cfg, nil
}

// Reconfig reads the current configuration, hands it to mutate, bumps the
// version, and installs the result. Callers that add or remove members go
// through here so the version bump is never forgotten.
func (rs *ReplicaSet) Reconfig(ctx context.Context, mutate func(cfg bson.M) error) error {
	cfg, err := rs.Config(ctx)
	if err != nil {
		return err
	}
	if err := mutate(cfg); err != nil {
		return err
	}
	cfg["version"] = int32(asInt64(cfg["version"])) + 1

	p, err := rs.Primary(ctx)
	if err != nil {
		return err
	}
	cli, err := rs.client(ctx, p.Addr)
	if err != nil {
		return err
	}
	cmd := bson.D{{Key: "replSetReconfig", Value: cfg}}
	if err := cli.Database("admin").RunCommand(ctx, cmd).Err(); err != nil {
		return fmt.Errorf("replSetReconfig: %w", err)
	}
	return nil
}

// ReconfigMember applies mutate to the configuration entry for addr.
//
// Prefer this over indexing into cfg["members"]: position in that array has no
// relationship to role, so "the last member" is sometimes the primary, and a
// reconfig that makes the primary non-electable is rejected with
// NodeNotElectable. Addressing by host removes the guess.
func (rs *ReplicaSet) ReconfigMember(ctx context.Context, addr string, mutate func(member bson.M) error) error {
	return rs.Reconfig(ctx, func(cfg bson.M) error {
		for _, raw := range asArray(cfg["members"]) {
			m, ok := raw.(bson.M)
			if !ok {
				continue
			}
			if asString(m["host"]) == addr {
				return mutate(m)
			}
		}
		return fmt.Errorf("member %s is not in the configuration of %s", addr, rs.Name)
	})
}

// AnySecondary returns a member that is currently SECONDARY. Tests that need "a
// member that is not the primary" use this rather than picking by index.
func (rs *ReplicaSet) AnySecondary(ctx context.Context) (*Member, error) {
	secondaries, err := rs.Secondaries(ctx)
	if err != nil {
		return nil, err
	}
	if len(secondaries) == 0 {
		return nil, fmt.Errorf("replica set %s has no secondary", rs.Name)
	}
	return secondaries[0], nil
}

// StepDownPrimary forces the current primary to step down for at least secs and
// waits for a new primary to be elected. The old primary is excluded from the
// wait so a set that re-elects the same node is reported rather than hidden.
func (rs *ReplicaSet) StepDownPrimary(ctx context.Context, secs int) (*Member, error) {
	old, err := rs.Primary(ctx)
	if err != nil {
		return nil, err
	}
	cli, err := rs.client(ctx, old.Addr)
	if err != nil {
		return nil, err
	}
	cmd := bson.D{{Key: "replSetStepDown", Value: secs}, {Key: "force", Value: true}}
	// A successful step-down closes the connection, which the driver surfaces
	// as an error; only an explicit command failure matters here.
	if err := cli.Database("admin").RunCommand(ctx, cmd).Err(); err != nil &&
		!isStepDownDisconnect(err) {
		return nil, fmt.Errorf("replSetStepDown: %w", err)
	}

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		p, err := rs.Primary(ctx)
		if err == nil && p.Addr != old.Addr {
			return p, nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil, fmt.Errorf("no new primary after stepping down %s", old.Addr)
}

func isStepDownDisconnect(err error) bool {
	s := err.Error()
	return strings.Contains(s, "socket was unexpectedly closed") ||
		strings.Contains(s, "connection(") ||
		strings.Contains(s, "EOF")
}

// Client returns a replica-set-aware client for the whole set.
func (rs *ReplicaSet) Client(ctx context.Context) (*mongo.Client, error) {
	return mongo.Connect(ctx, options.Client().ApplyURI(rs.URI()))
}

// DirectClient returns a client pinned to one member, bypassing topology
// discovery. Reading a specific member's own view requires this.
func (rs *ReplicaSet) DirectClient(ctx context.Context, m *Member) (*mongo.Client, error) {
	return directClient(ctx, m.Addr)
}

// client returns a pooled client pinned to addr. The wait loops call this
// several times a second; a fresh mongo.Client per poll spins up a topology
// monitor each time and roughly doubled the harness runtime.
func (rs *ReplicaSet) client(ctx context.Context, addr string) (*mongo.Client, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if c, ok := rs.pool[addr]; ok {
		return c, nil
	}
	c, err := directClient(ctx, addr)
	if err != nil {
		return nil, err
	}
	if rs.pool == nil {
		rs.pool = map[string]*mongo.Client{}
	}
	rs.pool[addr] = c
	return c, nil
}

func (rs *ReplicaSet) closePool() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	for _, c := range rs.pool {
		_ = c.Disconnect(context.Background())
	}
	rs.pool = nil
}

func directClient(ctx context.Context, addr string) (*mongo.Client, error) {
	cli, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://"+addr+"/?directConnection=true"))
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	return cli, nil
}

func replSetGetStatus(ctx context.Context, cli *mongo.Client) (bson.M, error) {
	var res bson.M
	err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&res)
	if err != nil {
		return nil, fmt.Errorf("replSetGetStatus: %w", err)
	}
	return res, nil
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asArray(v interface{}) bson.A {
	a, _ := v.(bson.A)
	return a
}

func asInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	}
	return 0
}
