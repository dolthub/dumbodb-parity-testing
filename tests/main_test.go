package tests

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestMain starts dumbodb automatically if it is not already running, so that
// "go test ./..." works in a local dev environment without manual setup.
//
// Server resolution order:
//  1. If DUMBODB_URI / MONGO_URI are set, those are used as-is.
//  2. If dumbodb is already listening on :27018, nothing is started.
//  3. If DUMBODB_BIN points to a binary (or a binary is found at well-known
//     paths), dumbodb is started automatically in a temporary data directory.
//  4. If neither server can be reached after the above, all tests are skipped
//     with a helpful message rather than failing with "connection refused".
func TestMain(m *testing.M) {
	var dumboDBCmd *exec.Cmd

	// Auto-start dumbodb only when DUMBODB_URI is not explicitly set (i.e. we
	// are using the default :27018).
	if os.Getenv("DUMBODB_URI") == "" && !portOpen("127.0.0.1:27018") {
		dumboDBCmd = tryStartDumboDB()
	}

	// Verify connectivity; skip gracefully if either server is absent.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := harness.GetClients(ctx)
	cancel()

	if err != nil {
		fmt.Fprintf(os.Stderr,
			"\nSKIP: parity tests require MongoDB on :27017 and DumboDB on :27018\n"+
				"  Error: %v\n\n"+
				"  Quick setup:\n"+
				"    docker run -d -p 27017:27017 mongo:8.0\n"+
				"    DUMBODB_BIN=/path/to/dumbodb go test ./...\n\n",
			err,
		)
		if dumboDBCmd != nil {
			_ = dumboDBCmd.Process.Kill()
		}
		os.Exit(0) // exit 0 — skipping is not a failure
	}

	code := m.Run()

	if dumboDBCmd != nil {
		_ = dumboDBCmd.Process.Kill()
		_ = dumboDBCmd.Wait()
	}

	os.Exit(code)
}

// tryStartDumboDB locates a dumbodb binary and starts it on :27018 in a temp
// directory. Returns the running *exec.Cmd, or nil if startup fails.
func tryStartDumboDB() *exec.Cmd {
	bin := findDumboDBBin()
	if bin == "" {
		return nil
	}

	dataDir, err := os.MkdirTemp("", "dumbodb-parity-*")
	if err != nil {
		return nil
	}

	cmd := exec.Command(bin, "-addr", "127.0.0.1:27018", "-data-dir", dataDir)
	cmd.Stdout = os.Stderr // route dumbodb logs to stderr so they're visible under -v
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil
	}

	// Wait up to 10 s for dumbodb to accept connections.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen("127.0.0.1:27018") {
			fmt.Fprintf(os.Stderr, "dumbodb started automatically (pid %d, data %s)\n", cmd.Process.Pid, dataDir)
			return cmd
		}
		// Bail early if the process already exited.
		if cmd.ProcessState != nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	_ = cmd.Process.Kill()
	return nil
}

// findDumboDBBin returns the path to a dumbodb binary, checking (in order):
//  1. DUMBODB_BIN environment variable
//  2. /tmp/dumbodb-bin   (conventional location used by AGENT.md setup)
//  3. dumbodb on PATH
func findDumboDBBin() string {
	if v := os.Getenv("DUMBODB_BIN"); v != "" {
		return v
	}
	for _, p := range []string{"/tmp/dumbodb-bin", "/usr/local/bin/dumbodb"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("dumbodb"); err == nil {
		return path
	}
	return ""
}

// portOpen reports whether a TCP listener is accepting on addr.
func portOpen(addr string) bool {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
