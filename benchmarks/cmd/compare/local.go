package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// localServer is a target process started from a binary on this host, used
// when -dumbodb-bin and -mongod-bin are set.
type localServer struct {
	label   string
	port    int
	cmd     *exec.Cmd
	exited  chan struct{}
	dataDir string
	logPath string
}

func runLocalTargets(workDir string) (dumbo, mongo []result, err error) {
	dumbo, dumboErr := runLocalTarget("dumbodb", filepath.Join(workDir, "dumbodb-data"), func(port int, dataDir string) *exec.Cmd {
		cmd := exec.Command(*dumboBin, "--port", fmt.Sprint(port), "--data-dir", dataDir)
		cmd.Env = append(os.Environ(), "DUMBODB_NO_METRICS=1")
		return cmd
	})
	mongo, mongoErr := runLocalTarget("mongodb", filepath.Join(workDir, "mongodb-data"), func(port int, dataDir string) *exec.Cmd {
		return exec.Command(*mongodBin, "--dbpath", dataDir, "--port", fmt.Sprint(port), "--bind_ip", "127.0.0.1")
	})
	return dumbo, mongo, errors.Join(dumboErr, mongoErr)
}

func runLocalTarget(label, dataDir string, command func(port int, dataDir string) *exec.Cmd) ([]result, error) {
	srv, err := startLocalServer(label, dataDir, command)
	if err != nil {
		return nil, err
	}
	defer srv.stop()
	results, err := runBench(label, srv.uri())
	if !srv.alive() {
		err = errors.Join(err, fmt.Errorf("%s exited during the run:\n%s", label, logTail(srv.logPath, 20)))
	}
	return results, err
}

func startLocalServer(label, dataDir string, command func(port int, dataDir string) *exec.Cmd) (*localServer, error) {
	if err := os.RemoveAll(dataDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	port, err := freePort()
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
	s := &localServer{label: label, port: port, cmd: cmd, exited: make(chan struct{}), dataDir: dataDir, logPath: logPath}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()

	deadline := time.Now().Add(*healthTimeout)
	for time.Now().Before(deadline) {
		if !s.alive() {
			return nil, fmt.Errorf("%s exited during startup:\n%s", label, logTail(logPath, 20))
		}
		if conn, err := net.DialTimeout("tcp", s.addr(), 500*time.Millisecond); err == nil {
			conn.Close()
			fmt.Fprintf(os.Stderr, "==> %s started (%s)\n", label, s.uri())
			return s, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	s.stop()
	return nil, fmt.Errorf("%s did not accept connections within %s", label, *healthTimeout)
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (s *localServer) addr() string { return fmt.Sprintf("127.0.0.1:%d", s.port) }

func (s *localServer) uri() string { return "mongodb://" + s.addr() }

func (s *localServer) alive() bool {
	select {
	case <-s.exited:
		return false
	default:
		return true
	}
}

func (s *localServer) stop() {
	if s.alive() {
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

func logTail(path string, lines int) string {
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

func binaryVersion(bin, prefix string) string {
	out, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return "unknown"
	}
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return strings.TrimPrefix(first, prefix)
}
