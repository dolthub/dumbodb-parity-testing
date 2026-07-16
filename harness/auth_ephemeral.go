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
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

// EphemeralServers is a throwaway, access-control-enabled MongoDB + DumboDB
// pair with fresh data directories and NO users. It exists for the
// localhost-exception matrix (area A), whose tests depend on a server that
// starts with --auth and zero users, where creating the first user permanently
// changes global state and so cannot share the isolated-DB model of the shared
// servers. Each test gets its own pair, torn down via t.Cleanup.
type EphemeralServers struct {
	MongoURI   string
	DumboDBURI string

	mongo *serverProc
	dumbo *serverProc
}

type serverProc struct {
	cmd *exec.Cmd
	dir string
}

// StartEphemeralServers launches a fresh mongod --auth and dumbodb --auth on
// free ports with empty data directories, returning once both accept
// connections. It skips the test if the auth suite is disabled or the server
// binaries cannot be located. Teardown is registered with t.Cleanup.
func StartEphemeralServers(t *testing.T) *EphemeralServers {
	t.Helper()
	RequireAuth(t)

	mongod := findMongodBin()
	if mongod == "" {
		t.Skip("mongod binary not found; set MONGOD_BIN to run localhost-exception tests")
	}
	dumbodb := findDumboDBBinary()
	if dumbodb == "" {
		t.Skip("dumbodb binary not found; set DUMBODB_BIN to run localhost-exception tests")
	}

	es := &EphemeralServers{}
	t.Cleanup(es.Stop)

	mp := freePort(t)
	mdir := tempDir(t, "eph-mongo-")
	mongoCmd := exec.Command(mongod,
		"--auth",
		"--port", fmt.Sprintf("%d", mp),
		"--dbpath", mdir,
		"--bind_ip", "127.0.0.1",
		"--nounixsocket",
	)
	startProc(t, mongoCmd, "mongod")
	es.mongo = &serverProc{cmd: mongoCmd, dir: mdir}
	es.MongoURI = fmt.Sprintf("mongodb://127.0.0.1:%d", mp)

	dp := freePort(t)
	ddir := tempDir(t, "eph-dumbo-")
	dumboCmd := exec.Command(dumbodb,
		"--auth",
		"--addr", fmt.Sprintf("127.0.0.1:%d", dp),
		"--data-dir", ddir,
	)
	startProc(t, dumboCmd, "dumbodb")
	es.dumbo = &serverProc{cmd: dumboCmd, dir: ddir}
	es.DumboDBURI = fmt.Sprintf("mongodb://127.0.0.1:%d", dp)

	if !waitPort(fmt.Sprintf("127.0.0.1:%d", mp), 20*time.Second) {
		t.Fatalf("ephemeral mongod did not become ready on port %d", mp)
	}
	if !waitPort(fmt.Sprintf("127.0.0.1:%d", dp), 20*time.Second) {
		t.Fatalf("ephemeral dumbodb did not become ready on port %d", dp)
	}
	return es
}

// Stop kills both servers and removes their data directories. Safe to call more
// than once.
func (e *EphemeralServers) Stop() {
	for _, p := range []*serverProc{e.mongo, e.dumbo} {
		if p == nil || p.cmd == nil || p.cmd.Process == nil {
			continue
		}
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
		if p.dir != "" {
			_ = os.RemoveAll(p.dir)
		}
	}
	e.mongo, e.dumbo = nil, nil
}

func startProc(t *testing.T, cmd *exec.Cmd, name string) {
	t.Helper()
	// Route server logs to a file (kept for post-mortem) instead of the test's
	// stderr, which the verbose mongod output would otherwise flood.
	logf, err := os.CreateTemp("", name+"-*.log")
	if err != nil {
		t.Fatalf("create %s log: %v", name, err)
	}
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		t.Fatalf("start ephemeral %s: %v", name, err)
	}
	t.Logf("ephemeral %s started (pid %d, log %s)", name, cmd.Process.Pid, logf.Name())
}

func findMongodBin() string {
	if v := os.Getenv("MONGOD_BIN"); v != "" {
		return v
	}
	if p, err := exec.LookPath("mongod"); err == nil {
		return p
	}
	return ""
}

// findDumboDBBinary mirrors the discovery order used by TestMain's
// findDumboDBBin, but lives in the harness so ephemeral servers do not depend
// on the tests package.
func findDumboDBBinary() string {
	if v := os.Getenv("DUMBODB_BIN"); v != "" {
		return v
	}
	for _, p := range []string{"/tmp/dumbodb-bin", "/usr/local/bin/dumbodb"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("dumbodb"); err == nil {
		return p
	}
	return ""
}

// freePort returns a currently-free TCP port on the loopback interface. There
// is an inherent race between closing the probe listener and the server
// binding, but it is acceptable for local test orchestration.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port
}

func tempDir(t *testing.T, prefix string) string {
	t.Helper()
	d, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	return d
}

func waitPort(addr string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}
