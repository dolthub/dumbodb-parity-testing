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

package harness

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
)

func TestConverge_ReferenceMembersConverge(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	pc, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	defer func() { _ = pc.Disconnect(context.Background()) }()

	coll := pc.Database("harness_converge").Collection("docs")
	for i := 0; i < 50; i++ {
		if _, err := coll.InsertOne(ctx, bson.D{{Key: "i", Value: i}}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	watermark := rs.MustConverge(t, ctx, 60*time.Second)
	if watermark.IsZero() {
		t.Fatal("watermark is zero")
	}
	t.Logf("converged on %s", watermark)

	for _, m := range rs.Members {
		cli, err := rs.DirectClient(ctx, m)
		if err != nil {
			t.Fatalf("client %s: %v", m.Addr, err)
		}
		n, err := cli.Database("harness_converge").Collection("docs").
			CountDocuments(ctx, bson.D{})
		_ = cli.Disconnect(context.Background())
		if err != nil {
			t.Fatalf("count on %s: %v", m.Addr, err)
		}
		if n != 50 {
			t.Errorf("%s holds %d documents after convergence, want 50", m.Addr, n)
		}
	}
}

func TestConverge_ProgressReportsMemberState(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	progress, err := rs.Progress(ctx, primary.Addr)
	if err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if progress.State != StatePrimary {
		t.Errorf("primary reports state %q, want %s", progress.State, StatePrimary)
	}
	if progress.Applied.IsZero() {
		t.Error("primary reports a zero applied optime")
	}
	if progress.Durable.IsZero() {
		t.Error("primary reports a zero durable optime")
	}
	t.Logf("%s", progress)
}

func TestConverge_UnreachableMemberTimesOutLegibly(t *testing.T) {
	requireMongod(t)
	rs := StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	start := time.Now()
	_, err := rs.WaitConverged(ctx, 3*time.Second, "127.0.0.1:9")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected convergence to fail for an address that is not a member")
	}
	if !contains(err.Error(), "127.0.0.1:9") {
		t.Errorf("the failing address should be named: %v", err)
	}
	if elapsed > 15*time.Second {
		t.Errorf("a 3s convergence timeout took %s; the deadline is not bounding the progress reads", elapsed)
	}
}
