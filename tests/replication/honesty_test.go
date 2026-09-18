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

// Whether DumboDB tells the truth about its own state.
//
// The convergence gate believes what a member reports. If a member can claim a
// position it has not actually reached, the gate certifies bad data and every
// comparison built on it is meaningless. These are the checks that make the
// rest of the suite worth running.
//go:build replication

package replication

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const honestyDB = "honesty"

func seedPrimary(ctx context.Context, primary *mongo.Client, db string, from, to int) error {
	docs := make([]interface{}, 0, to-from)
	for i := from; i < to; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: int32(i)}, {Key: "seq", Value: int32(i)}})
	}
	if len(docs) == 0 {
		return nil
	}
	_, err := primary.Database(db).Collection("docs").InsertMany(ctx, docs)
	return err
}

func countOn(ctx context.Context, cli *mongo.Client, db string) (int64, error) {
	return cli.Database(db).Collection("docs").CountDocuments(ctx, bson.D{})
}

// The moment a member first claims SECONDARY, everything written before it
// joined must already be present. A member that announces itself ready while
// still cloning would be handed reads it cannot answer.
func TestHonesty_NoSecondaryBeforeInitialSyncCompletes(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := joinWithSeed(t, 500)
	defer cancel()

	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		hello, err := rs.Hello(ctx, subject.Addr)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		secondary, _ := hello["secondary"].(bool)
		if !secondary {
			if writable, _ := hello["isWritablePrimary"].(bool); writable {
				t.Fatalf("subject reports isWritablePrimary; a hidden priority:0 member must never be primary")
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// First claim of SECONDARY. The seeded documents must all be here.
		cli, err := subject.Client(ctx)
		if err != nil {
			t.Fatalf("subject client: %v", err)
		}
		defer func() { _ = cli.Disconnect(context.Background()) }()

		n, err := countOn(ctx, cli, honestyDB)
		if err != nil {
			t.Fatalf("count on subject at first SECONDARY: %v", err)
		}
		if n != 500 {
			t.Errorf("subject claimed SECONDARY holding %d of 500 pre-join documents; it announced readiness before initial sync completed", n)
		}
		t.Logf("first SECONDARY claim held %d/500 pre-join documents", n)
		return
	}
	t.Skip("subject never reached SECONDARY; nothing to assert about premature readiness")
}

// The load-bearing invariant. When the subject reports applied optime T, every
// write at or before T must be present.
//
// Writes go in one at a time, each paired with the primary's applied optime
// immediately afterwards. That gives an exact mapping from a reported position
// to the set of documents that position must cover.
func TestHonesty_ReportedOptimeImpliesDataPresent(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := joinWithSeed(t, 0)
	defer cancel()

	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 120*time.Second); err != nil {
		t.Skipf("subject did not reach SECONDARY: %v", err)
	}

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	primaryClient, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	defer func() { _ = primaryClient.Disconnect(context.Background()) }()

	subjectClient, err := subject.Client(ctx)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	defer func() { _ = subjectClient.Disconnect(context.Background()) }()

	// checkpoint[i] is the primary's applied optime once documents 0..i exist.
	const writes = 60
	checkpoints := make([]harness.OpTime, 0, writes)
	for i := 0; i < writes; i++ {
		if err := seedPrimary(ctx, primaryClient, honestyDB, i, i+1); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		p, err := rs.Progress(ctx, primary.Addr)
		if err != nil {
			t.Fatalf("primary progress after insert %d: %v", i, err)
		}
		checkpoints = append(checkpoints, p.Applied)
	}

	violations := 0
	deadline := time.Now().Add(120 * time.Second)
	for time.Now().Before(deadline) {
		progress, err := rs.Progress(ctx, subject.Addr)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Highest document index whose write is covered by the reported optime.
		implied := -1
		for i, cp := range checkpoints {
			if cp.Compare(progress.Applied) <= 0 {
				implied = i
			}
		}
		if implied >= 0 {
			missing, err := missingUpTo(ctx, subjectClient, implied)
			if err != nil {
				t.Fatalf("reading subject: %v", err)
			}
			if len(missing) > 0 {
				violations++
				t.Errorf("subject reported applied %s, which covers documents 0..%d, but %d of them are absent (first missing: %v)",
					progress.Applied, implied, len(missing), missing[0])
			}
		}
		if implied == writes-1 {
			if violations == 0 {
				t.Logf("reported position never ran ahead of stored data across %d writes", writes)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}

	// A bare skip here proves nothing about the invariant this test exists for,
	// so report exactly how far the subject got.
	progress, _ := rs.Progress(ctx, subject.Addr)
	held, _ := countOn(ctx, subjectClient, honestyDB)
	t.Skipf("subject did not reach the final checkpoint within the budget; "+
		"target %s, subject applied %s (state %s), holding %d of %d documents. "+
		"No violation observed, but the invariant is unproven",
		checkpoints[writes-1], progress.Applied, progress.State, held, writes)
}

func missingUpTo(ctx context.Context, cli *mongo.Client, highest int) ([]int32, error) {
	cur, err := cli.Database(honestyDB).Collection("docs").
		Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$lte", Value: int32(highest)}}}})
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	present := map[int32]bool{}
	for _, d := range docs {
		if id, ok := d["_id"].(int32); ok {
			present[id] = true
		}
	}
	var missing []int32
	for i := 0; i <= highest; i++ {
		if !present[int32(i)] {
			missing = append(missing, int32(i))
		}
	}
	return missing, nil
}

// A durable claim must survive a crash. Reporting an optime as durable means
// the data behind it is on disk, not merely in memory.
func TestHonesty_DurableOptimeSurvivesHardKill(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := joinWithSeed(t, 200)
	defer cancel()

	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 120*time.Second); err != nil {
		t.Skipf("subject did not reach SECONDARY: %v", err)
	}
	if _, err := rs.WaitConverged(ctx, 90*time.Second, subject.Addr); err != nil {
		t.Skipf("subject did not converge: %v", err)
	}

	before, err := rs.Progress(ctx, subject.Addr)
	if err != nil {
		t.Fatalf("progress before kill: %v", err)
	}
	if before.Durable.IsZero() {
		t.Fatal("subject reports a zero durable optime while claiming SECONDARY")
	}

	cli, err := subject.Client(ctx)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	countBefore, err := countOn(ctx, cli, honestyDB)
	_ = cli.Disconnect(context.Background())
	if err != nil {
		t.Fatalf("count before kill: %v", err)
	}

	subject.Kill()
	subject.Start()

	after, err := subject.Client(ctx)
	if err != nil {
		t.Fatalf("subject client after restart: %v", err)
	}
	defer func() { _ = after.Disconnect(context.Background()) }()

	countAfter, err := countOn(ctx, after, honestyDB)
	if err != nil {
		t.Fatalf("count after restart: %v", err)
	}
	if countAfter < countBefore {
		t.Errorf("subject held %d documents when it reported durable %s, but only %d survived a hard kill",
			countBefore, before.Durable, countAfter)
	}
	t.Logf("durable %s: %d documents before kill, %d after", before.Durable, countBefore, countAfter)
}

// hello and replSetGetStatus are two self-descriptions of the same member. A
// member whose two accounts disagree is misreporting to one audience or the
// other, and drivers and operators read different ones.
func TestHonesty_HelloAgreesWithReplSetGetStatus(t *testing.T) {
	t.Parallel()
	rs, subject, ctx, cancel := joinWithSeed(t, 100)
	defer cancel()

	samples, disagreements := 0, 0
	deadline := time.Now().Add(100 * time.Second)
	for time.Now().Before(deadline) {
		hello, herr := rs.Hello(ctx, subject.Addr)
		progress, perr := rs.Progress(ctx, subject.Addr)
		if herr != nil || perr != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		samples++

		helloSecondary, _ := hello["secondary"].(bool)
		statusSecondary := progress.State == harness.StateSecondary
		if helloSecondary != statusSecondary {
			disagreements++
			t.Errorf("hello.secondary=%v but replSetGetStatus reports %s", helloSecondary, progress.State)
		}
		if writable, _ := hello["isWritablePrimary"].(bool); writable {
			t.Fatalf("subject reports isWritablePrimary while configured priority:0 votes:0")
		}
		if statusSecondary && samples > 5 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if samples == 0 {
		t.Skip("subject never answered both commands")
	}
	t.Logf("%d samples across the join, %d disagreements", samples, disagreements)
}

// joinWithSeed provisions a set, seeds the primary with n documents, then joins
// DumboDB so the seed must arrive via initial sync.
func joinWithSeed(t *testing.T, n int) (*harness.ReplicaSet, *harness.DumboMember, context.Context, context.CancelFunc) {
	t.Helper()
	rs := harness.StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)

	primary, err := rs.Primary(ctx)
	if err != nil {
		cancel()
		t.Fatalf("Primary: %v", err)
	}
	cli, err := rs.DirectClient(ctx, primary)
	if err != nil {
		cancel()
		t.Fatalf("primary client: %v", err)
	}
	if err := seedPrimary(ctx, cli, honestyDB, 0, n); err != nil {
		_ = cli.Disconnect(context.Background())
		cancel()
		t.Fatalf("seeding %d documents: %v", n, err)
	}
	_ = cli.Disconnect(context.Background())

	if _, err := rs.WaitConverged(ctx, 60*time.Second); err != nil {
		cancel()
		t.Fatalf("reference members did not converge on the seed: %v", err)
	}

	subject := rs.JoinDumboDB(t)
	if _, err := subject.Commit(ctx); err != nil {
		cancel()
		t.Fatalf("subject unreachable after join: %v", err)
	}
	return rs, subject, ctx, cancel
}
