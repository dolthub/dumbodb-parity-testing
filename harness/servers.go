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
	"sync"
	"time"
)

// The parity suite needs two server pairs: a plain pair for the non-auth suites
// and an access-control-enabled pair for the auth suite. ProvisionServers starts
// whichever are not supplied by environment overrides. A missing binary or a
// server that will not start is a hard error the caller must surface as a test
// failure -- never a skip, so that a run with no servers cannot report green
// without having compared anything.

var (
	provisioned    serverSet
	provisionOnce  sync.Once
	provisionErr   error
	provisionalMu  sync.Mutex
	spawnedServers []*serverProc
)

type serverSet struct {
	mongoURI     string
	dumboURI     string
	authMongoURI string
	authDumboURI string
}

// ProvisionServers starts every server the suite needs that is not already
// supplied via an environment override, recording each URI. It is safe to call
// repeatedly; provisioning happens once. A non-nil error means a required
// binary was missing or a server failed to start.
func ProvisionServers() error {
	provisionOnce.Do(func() { provisionErr = provisioned.start() })
	return provisionErr
}

// TeardownServers stops every server ProvisionServers started and removes their
// data directories.
func TeardownServers() {
	provisionalMu.Lock()
	defer provisionalMu.Unlock()
	for _, p := range spawnedServers {
		p.stop()
	}
	spawnedServers = nil
}

func (s *serverSet) start() error {
	var err error

	if s.mongoURI, err = resolveServer("MONGO_URI", "mongod-nonauth-", mongodArgs(false)); err != nil {
		return fmt.Errorf("non-auth mongod: %w", err)
	}
	if s.dumboURI, err = resolveServer("DUMBODB_URI", "dumbodb-nonauth-", dumbodbArgs(false)); err != nil {
		return fmt.Errorf("non-auth dumbodb: %w", err)
	}
	if s.authMongoURI, err = resolveServer("MONGO_AUTH_URI", "mongod-auth-", mongodArgs(true)); err != nil {
		return fmt.Errorf("auth mongod: %w", err)
	}
	if s.authDumboURI, err = resolveServer("DUMBODB_AUTH_URI", "dumbodb-auth-", dumbodbArgs(true)); err != nil {
		return fmt.Errorf("auth dumbodb: %w", err)
	}

	return nil
}

// serverArgs describes how to launch one server and which binary it needs.
type serverArgs struct {
	binary  func() string
	binName string
	flags   func(port int, dir string) []string
	mongod  bool
}

func mongodArgs(auth bool) serverArgs {
	return serverArgs{
		binary:  findMongodBin,
		binName: "mongod",
		mongod:  true,
		flags: func(port int, dir string) []string {
			f := []string{"--port", fmt.Sprintf("%d", port), "--dbpath", dir, "--bind_ip", "127.0.0.1", "--nounixsocket"}
			if auth {
				f = append([]string{"--auth"}, f...)
			}
			return f
		},
	}
}

func dumbodbArgs(auth bool) serverArgs {
	return serverArgs{
		binary:  findDumboDBBinary,
		binName: "dumbodb",
		flags: func(port int, dir string) []string {
			f := []string{"--addr", fmt.Sprintf("127.0.0.1:%d", port), "--data-dir", dir}
			if auth {
				f = append([]string{"--auth"}, f...)
			}
			return f
		},
	}
}

// resolveServer returns the URI for a server: the environment override if set
// (verifying it is reachable), otherwise a freshly spawned instance.
func resolveServer(envKey, dirPrefix string, args serverArgs) (string, error) {
	if uri := os.Getenv(envKey); uri != "" {
		if !waitPort(hostPort(uri), 10*time.Second) {
			return "", fmt.Errorf("%s=%s is not reachable", envKey, uri)
		}
		return uri, nil
	}

	bin := args.binary()
	if bin == "" {
		return "", fmt.Errorf("%s binary not found (set %s or the *_BIN env var)", args.binName, envKey)
	}

	port, err := freePort()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", dirPrefix)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(bin, args.flags(port, dir)...)
	proc, err := startProc(cmd, args.binName, dir)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}

	provisionalMu.Lock()
	spawnedServers = append(spawnedServers, proc)
	provisionalMu.Unlock()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if !waitPort(addr, 25*time.Second) {
		return "", fmt.Errorf("%s did not become ready on %s", args.binName, addr)
	}
	return "mongodb://" + addr, nil
}

type serverProc struct {
	cmd *exec.Cmd
	dir string
	log string
}

func (p *serverProc) stop() {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Kill()
	_, _ = p.cmd.Process.Wait()
	if p.dir != "" {
		_ = os.RemoveAll(p.dir)
	}
}

// startProc launches cmd with its output routed to a temp log file for
// post-mortem inspection.
func startProc(cmd *exec.Cmd, name, dir string) (*serverProc, error) {
	logf, err := os.CreateTemp("", name+"-*.log")
	if err != nil {
		return nil, fmt.Errorf("create %s log: %w", name, err)
	}
	cmd.Stdout = logf
	cmd.Stderr = logf
	if err := cmd.Start(); err != nil {
		_ = logf.Close()
		return nil, fmt.Errorf("start %s: %w", name, err)
	}
	return &serverProc{cmd: cmd, dir: dir, log: logf.Name()}, nil
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

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("allocate free port: %w", err)
	}
	defer func() { _ = l.Close() }()
	return l.Addr().(*net.TCPAddr).Port, nil
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

// hostPort strips a mongodb:// scheme to the host:port waitPort dials.
func hostPort(uri string) string {
	s := uri
	if i := len("mongodb://"); len(s) > i && s[:i] == "mongodb://" {
		s = s[i:]
	}
	if i := indexByte(s, '/'); i >= 0 {
		s = s[:i]
	}
	return s
}

func indexByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}
