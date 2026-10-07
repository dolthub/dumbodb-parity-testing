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
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ReplicaSetOptions configures a replica set beyond the plaintext default.
//
// The two fields are independent on purpose. A set can run TLS with no
// internal authentication, which is the configuration workspace-3y6.26 is
// about, and the only way to test that is to be able to ask for one without
// the other.
type ReplicaSetOptions struct {
	// TLS makes every member serve requireTLS using the fixture's material.
	// Nil leaves the set plaintext.
	TLS *TLSFixture

	// KeyFile is the shared secret for internal membership authentication,
	// as produced by NewKeyFile. Empty means the set has none, so members
	// never authenticate to each other. mongod turns access control on when
	// a keyfile is present, so a set with one is also an authenticated set.
	KeyFile string
}

// Root credentials for a set whose keyfile forced access control on.
//
// The localhost exception covers replSetInitiate and replSetGetStatus while no
// user exists, which is how the set gets as far as electing a primary. It
// closes the moment this user is created, so everything after bootstrapRoot
// authenticates.
const (
	rootUser     = "root"
	rootPassword = "root"
)

// NewKeyFile writes a shared key for internal membership authentication.
//
// mongod demands 6 to 1024 base64 characters and refuses a file any other
// account can read. DumboDB applies the same two rules
// (internal/replication/membership/credentials.go), so a file either side
// rejects is a harness bug rather than a finding.
func NewKeyFile(t *testing.T, dir string) string {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("NewKeyFile: %v", err)
	}
	path := filepath.Join(dir, "keyfile")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(secret)), 0o600); err != nil {
		t.Fatalf("NewKeyFile: %v", err)
	}
	return path
}

// StartReplicaSetWith starts a replica set with TLS, a keyfile, both or
// neither, and leaves it in steady state.
//
// Unlike StartReplicaSet this never adopts the set named by MONGO_REPL_SET_URI.
// That set is plaintext and unauthenticated; running a TLS case against it
// would report on a configuration nobody asked for.
func StartReplicaSetWith(t *testing.T, n int, opts ReplicaSetOptions) *ReplicaSet {
	t.Helper()
	if n < 1 {
		t.Fatalf("StartReplicaSetWith: need at least 1 member, got %d", n)
	}
	bin := findMongodBin()
	if bin == "" {
		t.Fatalf("mongod binary not found (set MONGOD_BIN)")
	}

	name := fmt.Sprintf("repl%d", time.Now().UnixNano()%100000)
	rs := &ReplicaSet{Name: name, t: t, opts: opts}
	t.Cleanup(rs.stop)

	for i := 0; i < n; i++ {
		m, err := rs.spawnMongod(bin, i)
		if err != nil {
			t.Fatalf("StartReplicaSetWith: member %d: %v", i, err)
		}
		rs.Members = append(rs.Members, m)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := rs.initiate(ctx); err != nil {
		t.Fatalf("StartReplicaSetWith: %v", err)
	}
	return rs
}

// memberArgs renders the TLS and keyfile flags every mongod in the set takes.
func (rs *ReplicaSet) memberArgs() []string {
	var args []string
	if f := rs.opts.TLS; f != nil {
		args = append(args,
			"--tlsMode", "requireTLS",
			"--tlsCertificateKeyFile", f.ServerPEMFile,
			"--tlsCAFile", f.CAFile,
		)
	}
	if rs.opts.KeyFile != "" {
		args = append(args, "--keyFile", rs.opts.KeyFile)
	}
	return args
}

// bootstrapRoot creates the first user, which closes the localhost exception.
//
// Only a set with a keyfile needs this: mongod turns access control on with
// the keyfile, and without a user nothing could authenticate once the
// exception lapses. The connection pool is dropped afterwards so that every
// later dial carries the credentials.
func (rs *ReplicaSet) bootstrapRoot(ctx context.Context) error {
	if rs.opts.KeyFile == "" {
		return nil
	}
	primary, err := rs.Primary(ctx)
	if err != nil {
		return err
	}
	cli, err := rs.client(ctx, primary.Addr)
	if err != nil {
		return err
	}
	cmd := bson.D{
		{Key: "createUser", Value: rootUser},
		{Key: "pwd", Value: rootPassword},
		{Key: "roles", Value: bson.A{"root"}},
	}
	if err := cli.Database("admin").RunCommand(ctx, cmd).Err(); err != nil {
		return fmt.Errorf("creating the bootstrap user on %s: %w", primary.Addr, err)
	}
	rs.closePool()
	rs.cred = &options.Credential{
		AuthMechanism: "SCRAM-SHA-256",
		AuthSource:    "admin",
		Username:      rootUser,
		Password:      rootPassword,
	}
	return nil
}

// clientOptions builds the dial options for one address, applying whatever
// combination of TLS and credentials the set is running.
func (rs *ReplicaSet) clientOptions(uri string) (*options.ClientOptions, error) {
	// No server-selection timeout is set, deliberately: this must behave
	// exactly as the plaintext directClient it replaced, so that adding TLS
	// support did not quietly retime thirteen existing replication suites.
	opts := options.Client().ApplyURI(uri)
	if f := rs.opts.TLS; f != nil {
		config, err := clientTLSConfig(f)
		if err != nil {
			return nil, err
		}
		opts = opts.SetTLSConfig(config)
	}
	if rs.cred != nil {
		opts = opts.SetAuth(*rs.cred)
	}
	return opts, nil
}

// clientTLSConfig verifies the server against the fixture CA and presents the
// fixture's client certificate, which a member serving requireTLS with a CA
// demands.
//
// The client certificate deliberately is not the member certificate. Giving a
// test client the same subject a member uses would make it indistinguishable
// from a peer, and an accidental cluster member is not what any of these cases
// mean to measure.
func clientTLSConfig(f *TLSFixture) (*tls.Config, error) {
	caPEM, err := os.ReadFile(f.CAFile)
	if err != nil {
		return nil, fmt.Errorf("reading CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("CA %s held no certificate", f.CAFile)
	}
	cert, err := tls.LoadX509KeyPair(f.ClientCertFile, f.ClientKeyFile)
	if err != nil {
		return nil, fmt.Errorf("loading client certificate: %w", err)
	}
	return &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", Certificates: []tls.Certificate{cert}}, nil
}

// dial opens a direct connection to one member, however the set is configured.
func (rs *ReplicaSet) dial(ctx context.Context, addr string) (*mongo.Client, error) {
	opts, err := rs.clientOptions("mongodb://" + addr + "/?directConnection=true")
	if err != nil {
		return nil, err
	}
	cli, err := mongo.Connect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", addr, err)
	}
	return cli, nil
}

// waitReady polls one address until it answers, using the set's own dial
// settings. ping is exempt from access control, so this works before the
// bootstrap user exists as well as after.
func (rs *ReplicaSet) waitReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cli, err := rs.dial(ctx, addr)
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
	return fmt.Errorf("no response to ping on %s within %s: %w", addr, timeout, last)
}

// DumboMemberOptions configures the DumboDB member independently of the set.
//
// Independently, because the cases worth running are the ones where the two
// disagree: a member brought up without the keyfile the set uses, or without
// the TLS material it demands.
type DumboMemberOptions struct {
	// TLS makes DumboDB serve requireTLS and dial its peers over TLS. Nil
	// leaves it plaintext, which a requireTLS set will not talk to.
	TLS *TLSFixture

	// KeyFile is the shared secret for internal membership authentication.
	// Empty means the member never authenticates as __system.
	KeyFile string
}

// memberArgs renders the flags for the DumboDB member.
//
// --tlsClusterFile and --tlsClusterCAFile are the outbound half: the
// certificate DumboDB presents when it dials a peer, as distinct from the one
// it serves. They are set explicitly even though DumboDB falls back to the
// serving material, so that a change to that fallback shows up as a failure
// here rather than silently altering what a member presents.
func (o DumboMemberOptions) memberArgs() []string {
	var args []string
	if f := o.TLS; f != nil {
		args = append(args,
			"--tlsMode", "requireTLS",
			"--tlsCertificateKeyFile", f.ServerPEMFile,
			"--tlsCAFile", f.CAFile,
			"--tlsClusterFile", f.ServerPEMFile,
			"--tlsClusterCAFile", f.CAFile,
		)
	}
	if o.KeyFile != "" {
		args = append(args, "--keyFile", o.KeyFile)
	}
	return args
}

// MatchingDumboOptions returns the member configuration that mirrors the set:
// the same TLS material and the same keyfile. It is the configuration a real
// deployment would use, and the baseline the mismatched cases deviate from.
func (rs *ReplicaSet) MatchingDumboOptions() DumboMemberOptions {
	return DumboMemberOptions{TLS: rs.opts.TLS, KeyFile: rs.opts.KeyFile}
}
