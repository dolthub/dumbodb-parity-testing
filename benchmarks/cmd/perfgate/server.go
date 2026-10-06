package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

type server struct {
	label   string
	port    int
	cmd     *exec.Cmd
	exited  chan struct{}
	dataDir string
	logPath string
}

func startServer(label, bin string, port int, dataDir string) (*server, error) {
	if err := os.RemoveAll(dataDir); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	logPath := dataDir + ".log"
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, err
	}
	defer logFile.Close()

	cmd := exec.Command(bin, "--port", fmt.Sprint(port), "--data-dir", dataDir)
	cmd.Env = append(os.Environ(), "DUMBODB_NO_METRICS=1")
	cmd.Stdout, cmd.Stderr = logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", label, err)
	}
	s := &server{label: label, port: port, cmd: cmd, exited: make(chan struct{}), dataDir: dataDir, logPath: logPath}
	go func() {
		_ = cmd.Wait()
		close(s.exited)
	}()

	deadline := time.Now().Add(*healthTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-s.exited:
			return nil, fmt.Errorf("%s exited during startup; log: %s", label, logPath)
		default:
		}
		conn, err := net.DialTimeout("tcp", s.addr(), 500*time.Millisecond)
		if err == nil {
			conn.Close()
			return s, nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	s.stop()
	return nil, fmt.Errorf("%s did not accept connections within %s; log: %s", label, *healthTimeout, logPath)
}

func (s *server) addr() string { return fmt.Sprintf("127.0.0.1:%d", s.port) }

func (s *server) uri() string { return "mongodb://" + s.addr() }

func (s *server) alive() bool {
	select {
	case <-s.exited:
		return false
	default:
		return true
	}
}

func (s *server) stop() {
	if s.alive() {
		_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-s.exited:
		case <-time.After(15 * time.Second):
			_ = syscall.Kill(-s.cmd.Process.Pid, syscall.SIGKILL)
			<-s.exited
		}
	}
	_ = os.RemoveAll(s.dataDir)
}

func roundDataDir(root, label string, round int) string {
	return filepath.Join(root, fmt.Sprintf("%s-round%02d", label, round))
}
