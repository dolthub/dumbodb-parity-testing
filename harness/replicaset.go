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

var externalSetMu sync.Mutex

const replSetURIEnv = "MONGO_REPL_SET_URI"

const (
	StatePrimary    = "PRIMARY"
	StateSecondary  = "SECONDARY"
	StateStartup2   = "STARTUP2"
	StateRecovering = "RECOVERING"
)

type ReplicaSet struct {
	Name    string
	Members []*Member

	t        *testing.T
	external bool

	mu   sync.Mutex
	pool map[string]*mongo.Client
}

type Member struct {
	ID   int
	Addr string
	URI  string

	proc *serverProc
}

func StartReplicaSet(t *testing.T, n int) *ReplicaSet {
	t.Helper()
	if n < 1 {
		t.Fatalf("StartReplicaSet: need at least 1 member, got %d", n)
	}

	if uri := os.Getenv(replSetURIEnv); uri != "" {
		externalSetMu.Lock()
		t.Cleanup(externalSetMu.Unlock)
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
		"--wiredTigerCacheSizeGB", "0.25",
	)
	proc, err := startProc(cmd, fmt.Sprintf("mongod-%s-%d", rs.Name, id), dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if !waitPort(addr, 40*time.Second) {
		return nil, fmt.Errorf("mongod %s did not listen on %s (log %s)", rs.Name, addr, proc.log)
	}
	if err := waitServerReady(addr, 60*time.Second); err != nil {
		return nil, fmt.Errorf("mongod %s on %s never became ready (log %s): %w", rs.Name, addr, proc.log, err)
	}
	return &Member{ID: id, Addr: addr, URI: "mongodb://" + addr, proc: proc}, nil
}

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

func (rs *ReplicaSet) URI() string {
	hosts := make([]string, 0, len(rs.Members))
	for _, m := range rs.Members {
		hosts = append(hosts, m.Addr)
	}
	return fmt.Sprintf("mongodb://%s/?replicaSet=%s", strings.Join(hosts, ","), rs.Name)
}

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
	return fmt.Errorf("member %s did not reach %s within %s (last state %s)%s",
		m.Addr, want, timeout, got, rs.heartbeatDiagnosis(ctx, m.Addr))
}

func (rs *ReplicaSet) heartbeatDiagnosis(ctx context.Context, addr string) string {
	for _, peer := range rs.Members {
		if peer.Addr == addr {
			continue
		}
		cli, err := rs.client(ctx, peer.Addr)
		if err != nil {
			continue
		}
		status, err := replSetGetStatus(ctx, cli)
		if err != nil {
			continue
		}
		for _, raw := range asArray(status["members"]) {
			m, ok := raw.(bson.M)
			if !ok || asString(m["name"]) != addr {
				continue
			}
			msg := asString(m["lastHeartbeatMessage"])
			if msg == "" {
				continue
			}
			return fmt.Sprintf("; %s reports health=%d lastHeartbeatMessage=%q",
				peer.Addr, asInt64(m["health"]), msg)
		}
	}
	return ""
}

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

func (rs *ReplicaSet) Client(ctx context.Context) (*mongo.Client, error) {
	return mongo.Connect(ctx, options.Client().ApplyURI(rs.URI()))
}

func (rs *ReplicaSet) DirectClient(ctx context.Context, m *Member) (*mongo.Client, error) {
	return directClient(ctx, m.Addr)
}

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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for _, c := range rs.pool {
		wg.Add(1)
		go func(c *mongo.Client) {
			defer wg.Done()
			_ = c.Disconnect(ctx)
		}(c)
	}
	wg.Wait()
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
