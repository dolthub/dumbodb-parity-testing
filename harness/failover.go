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
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

// KillMember SIGKILLs one mongod and leaves its data directory in place.
//
// SIGKILL rather than a graceful stop on purpose. A clean shutdown lets a
// primary step down and hand over, which is the orderly path and a different
// one in the server; the rest of the set learns of a SIGKILL only by missing
// heartbeats, which is the failure a failover test means to cause.
//
// The member stays in the configuration, because that is what the survivors
// see: a configured member that stopped answering, not one that was removed.
func (rs *ReplicaSet) KillMember(addr string) error {
	for _, m := range rs.Members {
		if m.Addr != addr {
			continue
		}
		if m.proc == nil {
			return fmt.Errorf("member %s is not running", addr)
		}
		if m.proc.cmd != nil && m.proc.cmd.Process != nil {
			_ = m.proc.cmd.Process.Kill()
			_, _ = m.proc.cmd.Process.Wait()
		}
		if m.proc.logf != nil {
			_ = m.proc.logf.Close()
		}
		m.proc = nil
		rs.dropPooled(addr)
		return nil
	}
	return fmt.Errorf("member %s is not in the configuration of %s", addr, rs.Name)
}

// dropPooled discards the cached connection to one address, which a killed
// member leaves pointing at nothing.
func (rs *ReplicaSet) dropPooled(addr string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	cli, ok := rs.pool[addr]
	if !ok {
		return
	}
	delete(rs.pool, addr)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cli.Disconnect(ctx)
	}()
}

// SyncSourceOf reports the member that the member at addr is replicating from,
// as that member itself reports it. An empty string means it names none.
//
// A failover test needs this to know what it actually exercised. Killing the
// primary only forces a subject to find a new sync source if the subject was
// reading from the primary in the first place; if it had already chained off
// another secondary, the same kill proves nothing about re-targeting.
func (rs *ReplicaSet) SyncSourceOf(ctx context.Context, addr string) (string, error) {
	cli, err := rs.client(ctx, addr)
	if err != nil {
		return "", err
	}
	status, err := replSetGetStatus(ctx, cli)
	if err != nil {
		return "", err
	}
	return asString(status["syncSourceHost"]), nil
}

// WaitForNewPrimary waits for a writable primary at an address other than the
// one given, which is how a caller waits out an election it caused.
func (rs *ReplicaSet) WaitForNewPrimary(ctx context.Context, notAddr string, timeout time.Duration) (*Member, error) {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		m, err := rs.Primary(ctx)
		if err == nil && m.Addr != notAddr {
			return m, nil
		}
		if err != nil {
			last = err
		}
		time.Sleep(300 * time.Millisecond)
	}
	return nil, fmt.Errorf("no primary other than %s within %s: %w", notAddr, timeout, last)
}

// WaitForSyncSourceChange waits until the member at addr reports a sync source
// other than the one it had, so a test can assert re-targeting happened rather
// than inferring it from the data arriving.
func (rs *ReplicaSet) WaitForSyncSourceChange(ctx context.Context, addr, was string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	got := ""
	for time.Now().Before(deadline) {
		current, err := rs.SyncSourceOf(ctx, addr)
		if err == nil {
			got = current
			if current != "" && current != was {
				return current, nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return got, fmt.Errorf("%s still reports sync source %q after %s, was %q", addr, got, timeout, was)
}

// WritableCount counts documents through a direct connection to one member,
// used to show that a member is still receiving writes.
func (rs *ReplicaSet) WritableCount(ctx context.Context, addr, database, collection string) (int64, error) {
	cli, err := rs.client(ctx, addr)
	if err != nil {
		return 0, err
	}
	return cli.Database(database).Collection(collection).CountDocuments(ctx, bson.D{})
}
