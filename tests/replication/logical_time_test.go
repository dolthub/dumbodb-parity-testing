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

//go:build replication

package replication

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// The other half of TestLogicalTime_AbsentOnStandalone in the main suite.
//
// Suppressing $clusterTime and operationTime on a standalone is only correct
// if a member that HAS a replica set configuration still sends them: drivers
// use them for causal consistency, and a gate that is too tight would take
// that away from exactly the topology that needs it. The absence case alone
// would pass against a server that had stopped emitting them entirely.
func TestLogicalTime_PresentOnAReplicaSetMember(t *testing.T) {
	t.Parallel()
	rs := harness.StartReplicaSet(t, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	subject := rs.JoinDumboDB(t)
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 120*time.Second); err != nil {
		t.Skipf("subject did not reach SECONDARY: %v", err)
	}
	cli, err := subject.Client(ctx)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	defer func() { _ = cli.Disconnect(context.Background()) }()

	var raw bson.Raw
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Decode(&raw); err != nil {
		t.Fatalf("ping: %v", err)
	}
	for _, field := range []string{"$clusterTime", "operationTime"} {
		if _, err := raw.LookupErr(field); err != nil {
			t.Errorf("a member of a replica set did not return %s; the condition that "+
				"suppresses it on a standalone is too tight", field)
		}
	}
}
