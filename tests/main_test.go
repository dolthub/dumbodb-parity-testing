package tests

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/dolthub/docudolt-parity-testing/harness"
)

// TestMain starts docudolt automatically if it is not already running, so that
// "go test ./..." works in a local dev environment without manual setup.
//
// Server resolution order:
//  1. If DOCUDOLT_URI / MONGO_URI are set, those are used as-is.
//  2. If docudolt is already listening on :27018, nothing is started.
//  3. If DOCUDOLT_BIN points to a binary (or a binary is found at well-known
//     paths), docudolt is started automatically in a temporary data directory.
//  4. If neither server can be reached after the above, all tests are skipped
//     with a helpful message rather than failing with "connection refused".
func TestMain(m *testing.M) {
	var docuDoltCmd *exec.Cmd

	// Auto-start docudolt only when DOCUDOLT_URI is not explicitly set (i.e. we
	// are using the default :27018).
	if os.Getenv("DOCUDOLT_URI") == "" && !portOpen("127.0.0.1:27018") {
		docuDoltCmd = tryStartDocuDolt()
	}

	// Verify connectivity; skip gracefully if either server is absent.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	_, err := harness.GetClients(ctx)
	cancel()

	if err != nil {
		fmt.Fprintf(os.Stderr,
			"\nSKIP: parity tests require MongoDB on :27017 and DocuDolt on :27018\n"+
				"  Error: %v\n\n"+
				"  Quick setup:\n"+
				"    docker run -d -p 27017:27017 mongo:8.0\n"+
				"    DOCUDOLT_BIN=/path/to/docudolt go test ./...\n\n",
			err,
		)
		if docuDoltCmd != nil {
			_ = docuDoltCmd.Process.Kill()
		}
		os.Exit(0) // exit 0 — skipping is not a failure
	}

	code := m.Run()

	if docuDoltCmd != nil {
		_ = docuDoltCmd.Process.Kill()
		_ = docuDoltCmd.Wait()
	}

	os.Exit(code)
}

// tryStartDocuDolt locates a docudolt binary and starts it on :27018 in a temp
// directory. Returns the running *exec.Cmd, or nil if startup fails.
func tryStartDocuDolt() *exec.Cmd {
	bin := findDocuDoltBin()
	if bin == "" {
		return nil
	}

	dataDir, err := os.MkdirTemp("", "docudolt-parity-*")
	if err != nil {
		return nil
	}

	cmd := exec.Command(bin, "-addr", "127.0.0.1:27018", "-data-dir", dataDir)
	cmd.Stdout = os.Stderr // route docudolt logs to stderr so they're visible under -v
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil
	}

	// Wait up to 10 s for docudolt to accept connections.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if portOpen("127.0.0.1:27018") {
			fmt.Fprintf(os.Stderr, "docudolt started automatically (pid %d, data %s)\n", cmd.Process.Pid, dataDir)
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

// findDocuDoltBin returns the path to a docudolt binary, checking (in order):
//  1. DOCUDOLT_BIN environment variable
//  2. /tmp/docudolt-bin   (conventional location used by AGENT.md setup)
//  3. docudolt on PATH
func findDocuDoltBin() string {
	if v := os.Getenv("DOCUDOLT_BIN"); v != "" {
		return v
	}
	for _, p := range []string{"/tmp/docudolt-bin", "/usr/local/bin/docudolt"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if path, err := exec.LookPath("docudolt"); err == nil {
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
