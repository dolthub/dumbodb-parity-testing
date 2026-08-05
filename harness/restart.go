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

// restartGrace bounds how long Restart/Stop wait for a server to exit after
// SIGTERM before escalating to SIGKILL.
const restartGrace = 15 * time.Second

// RestartableServers is a private, non-auth MongoDB + DumboDB pair whose data
// directories survive a restart. It is the durability counterpart to
// EphemeralServers, which discards its data directories on Stop.
//
// A restart is a graceful shutdown (SIGTERM), not a kill, so it exercises
// durable-state persistence rather than crash recovery, and relaunches each
// server on a fresh port -- so MongoURI and DumboDBURI change across a Restart
// and callers must reconnect to the current values. Each test owns its own
// pair; Stop (registered with t.Cleanup) removes the data directories.
type RestartableServers struct {
	MongoURI   string
	DumboDBURI string

	mongo *restartTarget
	dumbo *restartTarget
}

type restartTarget struct {
	bin  string
	dir  string
	args serverArgs
	proc *serverProc
}

func (rt *restartTarget) launch(t *testing.T) string {
	t.Helper()
	port := mustFreePort(t)
	cmd := exec.Command(rt.bin, rt.args.flags(port, rt.dir)...)
	// Pass an empty dir to startProc so the serverProc never owns removal of the
	// data directory; RestartableServers.Stop removes it after the final shutdown.
	proc, err := startProc(cmd, rt.args.binName, "")
	if err != nil {
		t.Fatalf("start %s: %v", rt.args.binName, err)
	}
	rt.proc = proc
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	if !waitPort(addr, 25*time.Second) {
		t.Fatalf("%s did not become ready on %s", rt.args.binName, addr)
	}
	return "mongodb://" + addr
}

// StartRestartableServers launches a fresh non-auth mongod and dumbodb on free
// ports with empty data directories, returning once both accept connections. A
// missing server binary or a failed start is fatal -- never a skip -- so an
// environment that cannot run these tests fails loudly.
func StartRestartableServers(t *testing.T) *RestartableServers {
	t.Helper()

	mongod := findMongodBin()
	if mongod == "" {
		t.Fatal("mongod binary not found; set MONGOD_BIN to run durability tests")
	}
	dumbodb := findDumboDBBinary()
	if dumbodb == "" {
		t.Fatal("dumbodb binary not found; set DUMBODB_BIN to run durability tests")
	}

	rs := &RestartableServers{
		mongo: &restartTarget{bin: mongod, dir: mustTempDir(t, "restart-mongo-"), args: mongodArgs(false)},
		dumbo: &restartTarget{bin: dumbodb, dir: mustTempDir(t, "restart-dumbo-"), args: dumbodbArgs(false)},
	}
	t.Cleanup(rs.Stop)

	rs.MongoURI = rs.mongo.launch(t)
	rs.DumboDBURI = rs.dumbo.launch(t)
	return rs
}

// Restart gracefully shuts both servers down and relaunches each on its same
// data directory and a fresh port. MongoURI and DumboDBURI are updated to the
// new ports; any client opened before Restart now points at a dead port and
// must be reconnected to the current URIs.
func (rs *RestartableServers) Restart(t *testing.T) {
	t.Helper()
	rs.mongo.proc.shutdownGraceful(restartGrace)
	rs.dumbo.proc.shutdownGraceful(restartGrace)
	rs.MongoURI = rs.mongo.launch(t)
	rs.DumboDBURI = rs.dumbo.launch(t)
}

// Stop shuts both servers down gracefully and removes their data directories.
// Safe to call more than once.
func (rs *RestartableServers) Stop() {
	if rs.mongo != nil {
		rs.mongo.proc.shutdownGraceful(restartGrace)
		_ = os.RemoveAll(rs.mongo.dir)
		rs.mongo = nil
	}
	if rs.dumbo != nil {
		rs.dumbo.proc.shutdownGraceful(restartGrace)
		_ = os.RemoveAll(rs.dumbo.dir)
		rs.dumbo = nil
	}
}
