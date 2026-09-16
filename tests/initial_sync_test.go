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

// Tier 1: did everything that existed before the join actually arrive.
//
// Every case here seeds the primary BEFORE the subject joins, so the data must
// travel through initial sync rather than the oplog.
package tests

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const syncDB = "initsync"

// noWorkload satisfies the required Workload hook for cases whose entire point
// is what existed before the join.
func noWorkload(ctx context.Context, primary *mongo.Client) error { return nil }

func TestInitialSync_EmptySource(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:     "InitialSync_EmptySource",
		Support:  harness.DumboDBFull,
		Workload: noWorkload,
	})
}

func TestInitialSync_EveryBSONType(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "InitialSync_EveryBSONType",
		Support: harness.DumboDBFull,
		Timeout: 8 * time.Minute,
		SeedBeforeJoin: func(ctx context.Context, primary *mongo.Client) error {
			_, err := primary.Database(syncDB).Collection("types").
				InsertMany(ctx, harness.BSONTypeCorpus())
			return err
		},
		Workload: noWorkload,
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			coll := collectionOrFail(t, res.Subject, syncDB, "types")
			if want := len(harness.BSONTypeCorpus()); len(coll.Documents) != want {
				t.Errorf("subject holds %d type documents, want %d", len(coll.Documents), want)
			}
		},
	})
}

// Sizes and shapes that cross clone batching and storage boundaries, including
// documents close to the 16MB BSON limit.
func TestInitialSync_LargeAndDeepDocuments(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "InitialSync_LargeAndDeepDocuments",
		Support: harness.DumboDBFull,
		Timeout: 10 * time.Minute,
		SeedBeforeJoin: func(ctx context.Context, primary *mongo.Client) error {
			for _, doc := range harness.LargeDocumentCorpus() {
				if _, err := primary.Database(syncDB).Collection("large").InsertOne(ctx, doc); err != nil {
					return err
				}
			}
			return nil
		},
		Workload: noWorkload,
	})
}

// Many databases and collections, to check the clone enumerates the whole
// catalog rather than the first namespace it finds.
func TestInitialSync_ManyDatabasesAndCollections(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "InitialSync_ManyDatabasesAndCollections",
		Support: harness.DumboDBFull,
		Timeout: 10 * time.Minute,
		SeedBeforeJoin: func(ctx context.Context, primary *mongo.Client) error {
			r := harness.SeedRand(11)
			for d := 0; d < 4; d++ {
				db := primary.Database(fmt.Sprintf("%s_db%d", syncDB, d))
				for c := 0; c < 5; c++ {
					docs := make([]interface{}, 0, 20)
					for i := 0; i < 20; i++ {
						docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
					}
					if _, err := db.Collection(fmt.Sprintf("coll%d", c)).InsertMany(ctx, docs); err != nil {
						return err
					}
				}
			}
			return nil
		},
		Workload: noWorkload,
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			for d := 0; d < 4; d++ {
				name := fmt.Sprintf("%s_db%d", syncDB, d)
				db, ok := res.Subject.Databases[name]
				if !ok {
					t.Errorf("subject is missing database %s", name)
					continue
				}
				if len(db.Collections) != 5 {
					t.Errorf("subject database %s holds %d collections, want 5", name, len(db.Collections))
				}
			}
		},
	})
}

// Indexes, validators and collection options are cloned separately from
// documents and can be lost on their own.
func TestInitialSync_IndexesAndValidators(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "InitialSync_IndexesAndValidators",
		Support: harness.DumboDBFull,
		Timeout: 8 * time.Minute,
		SeedBeforeJoin: func(ctx context.Context, primary *mongo.Client) error {
			db := primary.Database(syncDB)
			err := db.RunCommand(ctx, bson.D{
				{Key: "create", Value: "validated"},
				{Key: "validator", Value: bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: true}}}}},
				{Key: "validationLevel", Value: "moderate"},
				{Key: "validationAction", Value: "error"},
			}).Err()
			if err != nil {
				return err
			}

			r := harness.SeedRand(13)
			docs := make([]interface{}, 0, 50)
			for i := 0; i < 50; i++ {
				docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
			}
			if _, err := db.Collection("validated").InsertMany(ctx, docs); err != nil {
				return err
			}

			_, err = db.Collection("validated").Indexes().CreateMany(ctx, []mongo.IndexModel{
				{Keys: bson.D{{Key: "score", Value: int32(1)}}},
				{Keys: bson.D{{Key: "label", Value: int32(1)}, {Key: "total", Value: int32(-1)}}},
				{Keys: bson.D{{Key: "address.city", Value: int32(1)}}},
			})
			return err
		},
		Workload: noWorkload,
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			coll := collectionOrFail(t, res.Subject, syncDB, "validated")
			if len(coll.Indexes) != 4 {
				t.Errorf("subject holds %d indexes, want 4 (_id plus three)", len(coll.Indexes))
			}
			if len(coll.Options) == 0 {
				t.Error("subject carries no collection options, so the validator did not clone")
			}
		},
	})
}

// The case the buffered-oplog design exists for. A clone is not a single
// instant; writes landing while it runs must be reconciled through the oplog
// rather than lost or double-applied.
func TestInitialSync_WritesConcurrentWithClone(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "InitialSync_WritesConcurrentWithClone",
		Support: harness.DumboDBFull,
		Timeout: 10 * time.Minute,
		SeedBeforeJoin: func(ctx context.Context, primary *mongo.Client) error {
			r := harness.SeedRand(17)
			docs := make([]interface{}, 0, 400)
			for i := 0; i < 400; i++ {
				docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
			}
			_, err := primary.Database(syncDB).Collection("workload").InsertMany(ctx, docs)
			return err
		},
		DuringClone: func(ctx context.Context, primary *mongo.Client) error {
			w := harness.Workload{
				Name:   "during-clone",
				Seed:   20260916,
				Ops:    harness.StandardVocabulary(),
				Repeat: 4,
			}
			_, err := w.Run(ctx, primary.Database(syncDB))
			return err
		},
		Workload: noWorkload,
	})
}

// A collection dropped while the clone is running. The clone must not fail, and
// must not resurrect the dropped collection.
func TestInitialSync_CollectionDroppedDuringClone(t *testing.T) {
	harness.ReplicaTest(t, harness.ReplicaCase{
		Name:    "InitialSync_CollectionDroppedDuringClone",
		Support: harness.DumboDBFull,
		Timeout: 10 * time.Minute,
		SeedBeforeJoin: func(ctx context.Context, primary *mongo.Client) error {
			r := harness.SeedRand(19)
			db := primary.Database(syncDB)
			for _, name := range []string{"keep", "doomed", "renamed_src"} {
				docs := make([]interface{}, 0, 100)
				for i := 0; i < 100; i++ {
					docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
				}
				if _, err := db.Collection(name).InsertMany(ctx, docs); err != nil {
					return err
				}
			}
			return nil
		},
		DuringClone: func(ctx context.Context, primary *mongo.Client) error {
			db := primary.Database(syncDB)
			if err := db.Collection("doomed").Drop(ctx); err != nil {
				return err
			}
			return primary.Database("admin").RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: syncDB + ".renamed_src"},
				{Key: "to", Value: syncDB + ".renamed_dst"},
			}).Err()
		},
		Workload: noWorkload,
		Assert: func(t *testing.T, res harness.ReplicaResult) {
			db, ok := res.Subject.Databases[syncDB]
			if !ok {
				t.Fatal("subject is missing the sync database")
			}
			if _, present := db.Collections["doomed"]; present {
				t.Error("subject resurrected a collection dropped during the clone")
			}
			if _, present := db.Collections["renamed_dst"]; !present {
				t.Error("subject is missing the collection renamed during the clone")
			}
		},
	})
}

// A source holding a BSON type DumboDB cannot represent must fail honestly:
// stop trying, never claim readiness, and say what and where.
//
// Promoted from XFail after dumbodb 90cbaaa. Written as a direct test rather
// than a ReplicaCase because convergence grading cannot express the
// distinction: the member fails to converge both when it retries forever and
// when it stops cleanly.
//
// The unsupported types themselves are not going to be supported, by owner
// direction. The deliverable is the error, so that a user who hits this can
// come and ask for the type rather than filing "replication does not work".
func TestInitialSync_UndecodableTypesFailHonestly(t *testing.T) {
	rs := harness.StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	cli, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	if _, err := cli.Database(syncDB).Collection("undecodable").
		InsertMany(ctx, harness.UndecodableTypeCorpus()); err != nil {
		_ = cli.Disconnect(context.Background())
		t.Fatalf("seeding undecodable types: %v", err)
	}
	_ = cli.Disconnect(context.Background())

	if _, err := rs.WaitConverged(ctx, 60*time.Second, primary.Addr); err != nil {
		t.Fatalf("primary did not settle: %v", err)
	}

	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)

	// It must stop. Sample until the failure is reported, failing if it is
	// still cycling through initial sync at the deadline.
	deadline := time.Now().Add(120 * time.Second)
	var status bson.M
	for time.Now().Before(deadline) {
		p, err := rs.Progress(ctx, subject.Addr)
		if err == nil && p.State == harness.StateSecondary {
			t.Fatalf("dumbodb %s claims SECONDARY while holding a collection it could not clone", commit)
		}
		if s, err := rs.Status(ctx, subject.Addr); err == nil {
			if _, reported := s["initialSyncStatus"]; reported {
				status = s
				break
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	if status == nil {
		t.Fatalf("dumbodb %s never reported a terminal initial-sync failure; it is still retrying", commit)
	}

	// The message is the deliverable, so assert its content rather than its
	// presence. A user reading this should learn the type and where it is.
	sync, ok := status["initialSyncStatus"].(bson.M)
	if !ok {
		t.Fatalf("initialSyncStatus is %T, want a document", status["initialSyncStatus"])
	}
	for field, want := range map[string]string{
		"bsonType":  "JavaScript",
		"namespace": syncDB + ".undecodable",
	} {
		got, _ := sync[field].(string)
		if got != want {
			t.Errorf("initialSyncStatus.%s = %q, want %q", field, got, want)
		}
	}
	message, _ := sync["initialSyncFailure"].(string)
	for _, fragment := range []string{"does not support", "JavaScript", syncDB + ".undecodable"} {
		if !contains(message, fragment) {
			t.Errorf("initialSyncFailure message %q does not mention %q", message, fragment)
		}
	}
	t.Logf("dumbodb %s stopped with: %s", commit, message)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// memberIdentity returns the set name and this member's configured _id, which
// live in replication control state. If that state is lost and recreated, they
// are what gets lost with it.
func memberIdentity(t *testing.T, ctx context.Context, rs *harness.ReplicaSet, addr string) (string, int64, bool) {
	t.Helper()
	status, err := rs.Status(ctx, addr)
	if err != nil {
		return "", 0, false
	}
	setName, _ := status["set"].(string)
	members, _ := status["members"].(bson.A)
	for _, raw := range members {
		m, ok := raw.(bson.M)
		if !ok {
			continue
		}
		if self, _ := m["self"].(bool); !self {
			continue
		}
		switch id := m["_id"].(type) {
		case int32:
			return setName, int64(id), true
		case int64:
			return setName, id, true
		}
	}
	return setName, 0, false
}

// A hard kill while the clone is running must leave the member able to finish,
// without losing who it is.
//
// This covers two things the suite was missing. The tier 1 requirement that a
// member killed mid-initial-sync either finishes or restarts cleanly and never
// reports SECONDARY holding partial data. And the reset window addressed by
// dumbodb 9be6976, where resetting replicated admin data also removed
// admin.system.dumbodb.replication, so a kill in that window lost member
// identity and replica configuration.
func TestInitialSync_KilledMidCloneKeepsIdentityAndFinishes(t *testing.T) {
	rs := harness.StartReplicaSet(t, 2)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	cli, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	// Large enough that the clone takes long enough to interrupt.
	r := harness.SeedRand(23)
	for batch := 0; batch < 6; batch++ {
		docs := make([]interface{}, 0, 150)
		for i := 0; i < 150; i++ {
			docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(batch*150+i)))
		}
		if _, err := cli.Database(syncDB).Collection("bulk").InsertMany(ctx, docs); err != nil {
			_ = cli.Disconnect(context.Background())
			t.Fatalf("seeding batch %d: %v", batch, err)
		}
	}
	_ = cli.Disconnect(context.Background())

	if _, err := rs.WaitConverged(ctx, 120*time.Second, primary.Addr); err != nil {
		t.Fatalf("primary did not settle: %v", err)
	}

	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)

	setBefore, idBefore, haveBefore := "", int64(0), false
	killedIn := "<never observed>"
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		p, err := rs.Progress(ctx, subject.Addr)
		if err == nil {
			if p.State == harness.StateSecondary {
				// The clone finished before the kill landed. Still worth
				// killing, but say so: this run did not exercise mid-clone.
				killedIn = p.State
				break
			}
			if p.State == harness.StateStartup2 {
				if !haveBefore {
					setBefore, idBefore, haveBefore = memberIdentity(t, ctx, rs, subject.Addr)
				}
				killedIn = p.State
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Logf("killing dumbodb %s while it reported %s", commit, killedIn)
	subject.Kill()
	subject.Start()

	// It must finish, and must never have claimed readiness with partial data.
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 180*time.Second); err != nil {
		t.Fatalf("subject did not reach SECONDARY after being killed during the clone: %v", err)
	}

	setAfter, idAfter, haveAfter := memberIdentity(t, ctx, rs, subject.Addr)
	if !haveAfter {
		t.Fatal("subject reports no self entry after restart; it does not know which member it is")
	}
	if setAfter != rs.Name {
		t.Errorf("subject reports set %q after restart, want %q; replica configuration was lost", setAfter, rs.Name)
	}
	if haveBefore && (setBefore != setAfter || idBefore != idAfter) {
		t.Errorf("member identity changed across the kill: was set=%q id=%d, now set=%q id=%d",
			setBefore, idBefore, setAfter, idAfter)
	}

	// And the data must be right.
	if _, err := rs.WaitConverged(ctx, 180*time.Second, subject.Addr); err != nil {
		t.Fatalf("subject did not converge after the interrupted clone: %v", err)
	}
	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("no reference secondary: %v", err)
	}
	refCli, err := rs.ClientFor(ctx, reference.Addr)
	if err != nil {
		t.Fatalf("reference client: %v", err)
	}
	defer func() { _ = refCli.Disconnect(context.Background()) }()
	subCli, err := subject.Client(ctx)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	defer func() { _ = subCli.Disconnect(context.Background()) }()

	refState, err := harness.CaptureServerState(ctx, refCli, "reference")
	if err != nil {
		t.Fatalf("capturing reference: %v", err)
	}
	subState, err := harness.CaptureServerState(ctx, subCli, "subject")
	if err != nil {
		t.Fatalf("capturing subject: %v", err)
	}
	if d := harness.DiffServerState(refState, subState); len(d) > 0 {
		t.Errorf("subject diverged from the reference after an interrupted clone, %d place(s); first: %s", len(d), d[0])
	}
}
