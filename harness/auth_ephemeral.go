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
	"os"
	"os/exec"
	"testing"
	"time"
)

// EphemeralServers is a throwaway, access-control-enabled MongoDB + DumboDB
// pair with fresh data directories and NO users. It exists for the
// localhost-exception cases (area A), whose tests depend on a server that
// starts with --auth and zero users, where creating the first user permanently
// changes global state and so cannot share the isolated-DB model of the shared
// servers. Each test gets its own pair, torn down via t.Cleanup.
type EphemeralServers struct {
	MongoURI   string
	DumboDBURI string

	mongo *serverProc
	dumbo *serverProc
}

// StartEphemeralServers launches a fresh mongod --auth and dumbodb --auth on
// free ports with empty data directories, returning once both accept
// connections. A missing server binary or a failed start is fatal -- never a
// skip -- so an environment that cannot run these tests fails loudly rather
// than passing silently. Teardown is registered with t.Cleanup.
func StartEphemeralServers(t *testing.T) *EphemeralServers {
	t.Helper()

	mongod := findMongodBin()
	if mongod == "" {
		t.Fatal("mongod binary not found; set MONGOD_BIN to run localhost-exception tests")
	}
	dumbodb := findDumboDBBinary()
	if dumbodb == "" {
		t.Fatal("dumbodb binary not found; set DUMBODB_BIN to run localhost-exception tests")
	}

	es := &EphemeralServers{}
	t.Cleanup(es.Stop)

	mp := mustFreePort(t)
	mdir := mustTempDir(t, "eph-mongo-")
	mongoCmd := exec.Command(mongod,
		"--auth",
		"--port", fmt.Sprintf("%d", mp),
		"--dbpath", mdir,
		"--bind_ip", "127.0.0.1",
		"--nounixsocket",
	)
	mproc, err := startProc(mongoCmd, "mongod", mdir)
	if err != nil {
		t.Fatalf("start ephemeral mongod: %v", err)
	}
	es.mongo = mproc
	es.MongoURI = fmt.Sprintf("mongodb://127.0.0.1:%d", mp)

	dp := mustFreePort(t)
	ddir := mustTempDir(t, "eph-dumbo-")
	dumboCmd := exec.Command(dumbodb,
		"--auth",
		"--addr", fmt.Sprintf("127.0.0.1:%d", dp),
		"--data-dir", ddir,
	)
	dproc, err := startProc(dumboCmd, "dumbodb", ddir)
	if err != nil {
		t.Fatalf("start ephemeral dumbodb: %v", err)
	}
	es.dumbo = dproc
	es.DumboDBURI = fmt.Sprintf("mongodb://127.0.0.1:%d", dp)

	if !waitPort(fmt.Sprintf("127.0.0.1:%d", mp), 25*time.Second) {
		t.Fatalf("ephemeral mongod did not become ready on port %d", mp)
	}
	if !waitPort(fmt.Sprintf("127.0.0.1:%d", dp), 25*time.Second) {
		t.Fatalf("ephemeral dumbodb did not become ready on port %d", dp)
	}
	return es
}

// Stop kills both servers and removes their data directories. Safe to call more
// than once.
func (e *EphemeralServers) Stop() {
	e.mongo.stop()
	e.dumbo.stop()
	e.mongo, e.dumbo = nil, nil
}

func mustFreePort(t *testing.T) int {
	t.Helper()
	p, err := freePort()
	if err != nil {
		t.Fatalf("allocate free port: %v", err)
	}
	return p
}

func mustTempDir(t *testing.T, prefix string) string {
	t.Helper()
	d, err := os.MkdirTemp("", prefix)
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	return d
}
