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

// Package targets starts and stops the DumboDB and MongoDB servers that the
// comparison runners (benchmarks/cmd/compare, cmd/fuzz) drive, either from
// local binaries or as Docker containers, and delivers their reports.
package targets

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// ServerCommand builds the command that serves on port with its data in dataDir.
type ServerCommand func(port int, dataDir string) *exec.Cmd

// DumboDBCommand runs the dumbodb binary bin.
func DumboDBCommand(bin string) ServerCommand {
	return func(port int, dataDir string) *exec.Cmd {
		cmd := exec.Command(bin, "--port", fmt.Sprint(port), "--data-dir", dataDir)
		cmd.Env = append(os.Environ(), "DUMBODB_NO_METRICS=1")
		return cmd
	}
}

// MongodCommand runs the mongod binary bin.
func MongodCommand(bin string) ServerCommand {
	return func(port int, dataDir string) *exec.Cmd {
		return exec.Command(bin, "--dbpath", dataDir, "--port", fmt.Sprint(port), "--bind_ip", "127.0.0.1")
	}
}

// LocalServer is a server process started from a binary on this host.
type LocalServer struct {
	Label   string
	LogPath string
	port    int
	cmd     *exec.Cmd
	exited  chan struct{}
	dataDir string
}

// StartLocal starts a server on a free port with an empty dataDir (removing
// what was there) and waits up to healthTimeout for it to accept connections.
// Its output goes to dataDir + ".log".
func StartLocal(label, dataDir string, command ServerCommand, healthTimeout time.Duration) (*LocalServer, error) {
	if err := os.RemoveAll(dataDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	port, err := FreePort()
	if err != nil {
		return nil, err
	}
	logPath := dataDir + ".log"
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()

	cmd := command(port, dataDir)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", label, err)
	}
	s := &LocalServer{Label: label, LogPath: logPath, port: port, cmd: cmd, exited: make(chan struct{}), dataDir: dataDir}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()

	deadline := time.Now().Add(healthTimeout)
	for time.Now().Before(deadline) {
		if !s.Alive() {
			return nil, fmt.Errorf("%s exited during startup:\n%s", label, LogTail(logPath, 20))
		}
		if conn, err := net.DialTimeout("tcp", s.Addr(), 500*time.Millisecond); err == nil {
			conn.Close()
			fmt.Fprintf(os.Stderr, "==> %s started (%s)\n", label, s.URI())
			return s, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	s.Stop()
	return nil, fmt.Errorf("%s did not accept connections within %s", label, healthTimeout)
}

// FreePort returns a TCP port on 127.0.0.1 that was free when asked.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (s *LocalServer) Addr() string { return fmt.Sprintf("127.0.0.1:%d", s.port) }

func (s *LocalServer) URI() string { return "mongodb://" + s.Addr() }

func (s *LocalServer) Alive() bool {
	select {
	case <-s.exited:
		return false
	default:
		return true
	}
}

// Stop sends SIGTERM to the server's process group, escalating to SIGKILL
// after 30s, and removes its data directory. The log is kept.
func (s *LocalServer) Stop() {
	if s.Alive() {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-s.exited:
		case <-time.After(30 * time.Second):
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			<-s.exited
		}
	}
	_ = os.RemoveAll(s.dataDir)
}

// LogTail returns the last lines of the file at path, or the open error.
func LogTail(path string, lines int) string {
	f, err := os.Open(path)
	if err != nil {
		return err.Error()
	}
	defer f.Close()
	var ring []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		ring = append(ring, scanner.Text())
		if len(ring) > lines {
			ring = ring[1:]
		}
	}
	return strings.Join(ring, "\n")
}

// BinaryVersion returns the first line of `bin --version` without prefix,
// or "unknown".
func BinaryVersion(bin, prefix string) string {
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimPrefix(first, prefix)
}
