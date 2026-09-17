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

// The replication observability surface, as MongoDB shapes it.
//
// There are no dumbo-prefixed replication commands to interrogate: they were
// removed in favour of serverStatus.repl, serverStatus.metrics.repl and
// opcountersRepl. The thing these tests defend is that an operator running the
// stock commands against DumboDB learns the truth, because the recurring defect
// in this feature is replication stopping while nothing says so.
//
//go:build replication

package tests

import (
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const obsDB = "observability"

// obsFixture is a converged subject with a reference mongod secondary beside
// it, which is what lets a shape assertion be about parity rather than about
// my opinion of what the field names should be.
type obsFixture struct {
	rs        *harness.ReplicaSet
	subject   *harness.DumboMember
	commit    string
	primary   *harness.Member
	reference *harness.Member
	client    *mongo.Client
}

func startObservability(t *testing.T, ctx context.Context) *obsFixture {
	t.Helper()

	rs := harness.StartReplicaSet(t, 2)

	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("AnySecondary: %v", err)
	}
	client, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}

	// Seed before the join so the subject has something to have replicated by
	// the time any counter is read. A counter that is zero because nothing
	// happened proves nothing either way.
	if _, err := client.Database(obsDB).Collection("seed").InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: 1}},
		bson.D{{Key: "_id", Value: 2}, {Key: "v", Value: 2}},
	}); err != nil {
		t.Fatalf("seeding the primary: %v", err)
	}

	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)
	if commit == "" || commit == "unknown" {
		t.Fatalf("subject reports gitVersion %q; a failure that cannot name its build is not reproducible", commit)
	}
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 150*time.Second); err != nil {
		t.Fatalf("dumbodb %s did not reach SECONDARY: %v", commit, err)
	}
	if _, err := rs.WaitConverged(ctx, 90*time.Second, subject.Addr, reference.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", commit, err)
	}

	return &obsFixture{rs: rs, subject: subject, commit: commit, primary: primary, reference: reference, client: client}
}

// serverStatus runs the command against addr, optionally with section filters.
func serverStatus(ctx context.Context, rs *harness.ReplicaSet, addr string, extra ...bson.E) (bson.M, error) {
	cli, err := rs.ClientFor(ctx, addr)
	if err != nil {
		return nil, err
	}
	cmd := bson.D{{Key: "serverStatus", Value: 1}}
	cmd = append(cmd, extra...)
	var res bson.M
	if err := cli.Database("admin").RunCommand(ctx, cmd).Decode(&res); err != nil {
		return nil, err
	}
	return res, nil
}

func subdoc(t *testing.T, parent bson.M, path ...string) bson.M {
	t.Helper()
	cur := parent
	for i, key := range path {
		next, ok := cur[key].(bson.M)
		if !ok {
			t.Fatalf("%v is %T, want a document", path[:i+1], cur[key])
		}
		cur = next
	}
	return cur
}

func number(t *testing.T, doc bson.M, key string) int64 {
	t.Helper()
	switch v := doc[key].(type) {
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	default:
		t.Fatalf("%s is %T (%v), want a number", key, doc[key], doc[key])
		return 0
	}
}

// TestObservability_ReplSectionAgreesWithStatus checks that the two commands an
// operator would reach for do not contradict each other, and that DumboDB's
// repl section invents no fields a real secondary does not have.
func TestObservability_ReplSectionAgreesWithStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	status, err := f.rs.Status(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("replSetGetStatus: %v", err)
	}
	server, err := serverStatus(ctx, f.rs, f.subject.Addr)
	if err != nil {
		t.Fatalf("serverStatus: %v", err)
	}
	repl := subdoc(t, server, "repl")

	if got := number(t, status, "myState"); got != 2 {
		t.Fatalf("replSetGetStatus.myState = %d, want 2 (SECONDARY)", got)
	}
	if repl["secondary"] != true {
		t.Errorf("dumbodb %s: serverStatus.repl.secondary = %v while replSetGetStatus.myState is SECONDARY", f.commit, repl["secondary"])
	}
	if repl["isWritablePrimary"] != false {
		t.Errorf("dumbodb %s: serverStatus.repl.isWritablePrimary = %v, want false", f.commit, repl["isWritablePrimary"])
	}
	if got, _ := repl["setName"].(string); got != f.rs.Name {
		t.Errorf("serverStatus.repl.setName = %q, want %q", got, f.rs.Name)
	}
	if got, _ := repl["me"].(string); got != f.subject.Addr {
		t.Errorf("serverStatus.repl.me = %q, want %q", got, f.subject.Addr)
	}

	// rbid is the rollback identifier initial sync validates against. If the
	// two commands disagree about it, one of them is lying about whether a
	// rollback happened under the sync.
	var rbid bson.M
	cli, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	if err := cli.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetRBID", Value: 1}}).Decode(&rbid); err != nil {
		t.Fatalf("replSetGetRBID: %v", err)
	}
	if a, b := number(t, rbid, "rbid"), number(t, repl, "rbid"); a != b {
		t.Errorf("dumbodb %s: replSetGetRBID.rbid = %d but serverStatus.repl.rbid = %d", f.commit, a, b)
	}

	// The reference secondary is the authority on what fields belong here.
	// DumboDB is allowed to omit, never to invent: an operator's tooling keys
	// off these names, and a dumbo-only name in a mongo-shaped section is the
	// parity break this whole exercise was meant to remove.
	refServer, err := serverStatus(ctx, f.rs, f.reference.Addr)
	if err != nil {
		t.Fatalf("reference serverStatus: %v", err)
	}
	refRepl := subdoc(t, refServer, "repl")
	for key := range repl {
		if _, ok := refRepl[key]; !ok {
			t.Errorf("dumbodb %s: serverStatus.repl.%s does not exist on a real mongod secondary", f.commit, key)
		}
	}
}

// oplogOpCounts counts what the primary actually wrote after mark, by the
// operation types opcountersRepl reports.
//
// mongod 8.0 does not write one oplog entry per document: a twenty document
// insertMany arrives as a single applyOps command carrying twenty inserts, so
// the inner operations have to be unwrapped or the count is off by nineteen.
func oplogOpCounts(ctx context.Context, primary *mongo.Client, mark time.Time) (map[string]int64, error) {
	cursor, err := primary.Database("local").Collection("oplog.rs").Find(ctx,
		bson.D{{Key: "wall", Value: bson.D{{Key: "$gte", Value: mark}}}})
	if err != nil {
		return nil, err
	}
	var entries []bson.M
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}
	counts := map[string]int64{"insert": 0, "update": 0, "delete": 0}
	var tally func(op string, o bson.M)
	tally = func(op string, o bson.M) {
		switch op {
		case "i":
			counts["insert"]++
		case "u":
			counts["update"]++
		case "d":
			counts["delete"]++
		case "c":
			inner, ok := o["applyOps"].(bson.A)
			if !ok {
				return
			}
			for _, raw := range inner {
				nested, ok := raw.(bson.M)
				if !ok {
					continue
				}
				nestedObject, _ := nested["o"].(bson.M)
				tally(asStringField(nested["op"]), nestedObject)
			}
		}
	}
	for _, entry := range entries {
		object, _ := entry["o"].(bson.M)
		tally(asStringField(entry["op"]), object)
	}
	return counts, nil
}

func asStringField(v interface{}) string {
	s, _ := v.(string)
	return s
}

// TestObservability_CountersAdvanceWithWorkload holds opcountersRepl against
// what the primary actually put in its oplog.
//
// The oplog is the oracle rather than the reference secondary's own counters.
// A live probe against mongod 8.0.28 showed the reference reporting 18 updates
// for an oplog containing 7 update entries: mongod counts writes beyond the
// replicated stream there, so its update counter cannot be compared exactly.
// Its insert and delete counters did match the oplog, which is what establishes
// that a real secondary unwraps applyOps and counts the inner operations.
func TestObservability_CountersAdvanceWithWorkload(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	before, err := serverStatus(ctx, f.rs, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject serverStatus: %v", err)
	}
	mark := time.Now().Add(-time.Second)

	const inserts, updates, deletes = 20, 7, 3
	coll := f.client.Database(obsDB).Collection("counters")
	docs := make([]interface{}, 0, inserts)
	for i := 0; i < inserts; i++ {
		docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "v", Value: i}})
	}
	if _, err := coll.InsertMany(ctx, docs); err != nil {
		t.Fatalf("insert: %v", err)
	}
	for i := 0; i < updates; i++ {
		// i*100+1 rather than i*100: an update that sets a field to the value
		// it already holds writes no oplog entry at all, so at i == 0 the
		// workload would be one operation shorter than it looks.
		if _, err := coll.UpdateOne(ctx, bson.D{{Key: "_id", Value: i}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "v", Value: i*100 + 1}}}}); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	for i := 0; i < deletes; i++ {
		if _, err := coll.DeleteOne(ctx, bson.D{{Key: "_id", Value: inserts - 1 - i}}); err != nil {
			t.Fatalf("delete: %v", err)
		}
	}

	if _, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr, f.reference.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge after the workload: %v", f.commit, err)
	}

	after, err := serverStatus(ctx, f.rs, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject serverStatus: %v", err)
	}
	want, err := oplogOpCounts(ctx, f.client, mark)
	if err != nil {
		t.Fatalf("reading the primary oplog: %v", err)
	}
	if want["insert"] < inserts || want["update"] < updates || want["delete"] < deletes {
		t.Fatalf("premise failed: the primary oplog holds %v for a workload of %d inserts, %d updates and %d deletes",
			want, inserts, updates, deletes)
	}

	beforeOps := subdoc(t, before, "opcountersRepl")
	afterOps := subdoc(t, after, "opcountersRepl")
	for _, field := range []string{"insert", "update", "delete"} {
		got := number(t, afterOps, field) - number(t, beforeOps, field)
		if got != want[field] {
			t.Errorf("dumbodb %s: opcountersRepl.%s advanced by %d; the primary oplog it replicated holds %d such operations",
				f.commit, field, got, want[field])
		}
	}

	total := want["insert"] + want["update"] + want["delete"]
	for _, c := range []struct {
		name string
		path []string
	}{
		{"metrics.repl.apply.ops", []string{"metrics", "repl", "apply"}},
		{"metrics.repl.network.ops", []string{"metrics", "repl", "network"}},
	} {
		got := number(t, subdoc(t, after, c.path...), "ops") - number(t, subdoc(t, before, c.path...), "ops")
		if got < 1 {
			t.Errorf("dumbodb %s: %s advanced by %d across %d replicated operations", f.commit, c.name, got, total)
		}
	}
	if number(t, subdoc(t, after, "metrics", "repl", "apply", "batches"), "num") <=
		number(t, subdoc(t, before, "metrics", "repl", "apply", "batches"), "num") {
		t.Errorf("dumbodb %s: metrics.repl.apply.batches.num did not advance across %d replicated operations", f.commit, total)
	}
}

// TestObservability_FetchedPositionTracksApplied checks the reported positions
// against each other. A fetch position behind the applied position means the
// server is reporting progress it cannot have made.
func TestObservability_FetchedPositionTracksApplied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	if _, err := f.client.Database(obsDB).Collection("positions").InsertOne(ctx,
		bson.D{{Key: "_id", Value: 1}}); err != nil {
		t.Fatalf("insert: %v", err)
	}
	watermark, err := f.rs.WaitConverged(ctx, 90*time.Second, f.subject.Addr)
	if err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", f.commit, err)
	}

	server, err := serverStatus(ctx, f.rs, f.subject.Addr)
	if err != nil {
		t.Fatalf("serverStatus: %v", err)
	}
	network := subdoc(t, server, "metrics", "repl", "network")
	fetched := harness.ReadOpTime(network["oplogFetcherHighestFetchedOptime"])
	if fetched.IsZero() {
		t.Fatalf("dumbodb %s: metrics.repl.network.oplogFetcherHighestFetchedOptime is zero after converging to %s", f.commit, watermark)
	}

	progress, err := f.rs.Progress(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("Progress: %v", err)
	}
	if fetched.Compare(progress.Applied) < 0 {
		t.Errorf("dumbodb %s: reports applied %s but only fetched %s; it cannot have applied what it did not read",
			f.commit, progress.Applied, fetched)
	}

	// The data has to be there, not merely claimed. A position is a promise
	// about stored state and this is the only assertion that tests the promise.
	subjectClient, err := f.rs.ClientFor(ctx, f.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	count, err := subjectClient.Database(obsDB).Collection("positions").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("counting on the subject: %v", err)
	}
	if count != 1 {
		t.Errorf("dumbodb %s: reports applied %s but holds %d documents in %s.positions, want 1",
			f.commit, progress.Applied, count, obsDB)
	}
}

// TestObservability_SectionFilterParity covers the include/exclude filter,
// because an operator scripting {serverStatus: 1, repl: 0} against a fleet
// should get the same shape from every member of it.
func TestObservability_SectionFilterParity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	f := startObservability(t, ctx)

	for _, section := range []string{"repl", "metrics", "opcountersRepl"} {
		excluded := bson.E{Key: section, Value: 0}
		ref, err := serverStatus(ctx, f.rs, f.reference.Addr, excluded)
		if err != nil {
			t.Fatalf("reference serverStatus with %s excluded: %v", section, err)
		}
		if _, present := ref[section]; present {
			t.Fatalf("mongod kept %s despite the exclusion; the premise of this test is wrong, not dumbodb", section)
		}
		got, err := serverStatus(ctx, f.rs, f.subject.Addr, excluded)
		if err != nil {
			t.Fatalf("subject serverStatus with %s excluded: %v", section, err)
		}
		if _, present := got[section]; present {
			t.Errorf("dumbodb %s: serverStatus kept %s despite {%s: 0}", f.commit, section, section)
		}
	}
}
