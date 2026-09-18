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
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const bulkDB = "bulkload"

func TestBulkLoad_LargeInsertManyKeepsReplicating(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	rs := harness.StartReplicaSet(t, 2)
	primary, err := rs.Primary(ctx)
	if err != nil {
		t.Fatalf("Primary: %v", err)
	}
	primaryClient, err := rs.DirectClient(ctx, primary)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	reference, err := rs.AnySecondary(ctx)
	if err != nil {
		t.Fatalf("AnySecondary: %v", err)
	}
	subject := rs.JoinDumboDB(t)
	commit, _ := subject.Commit(ctx)
	if commit == "" || commit == "unknown" {
		t.Fatalf("subject reports gitVersion %q", commit)
	}
	if err := rs.WaitForState(ctx, subject.Member, harness.StateSecondary, 180*time.Second); err != nil {
		t.Fatalf("dumbodb %s did not reach SECONDARY: %v", commit, err)
	}

	const documents = 200
	r := harness.SeedRand(11)
	docs := make([]interface{}, 0, documents)
	for i := 0; i < documents; i++ {
		docs = append(docs, harness.GenerateDocument(r, harness.DocIDFor(i)))
	}
	collection := primaryClient.Database(bulkDB).Collection("workload")
	if _, err := collection.InsertMany(ctx, docs); err != nil {
		t.Fatalf("inserting %d documents: %v", documents, err)
	}

	chained, total, err := chainedApplyOpsCount(ctx, primaryClient, bulkDB)
	if err != nil {
		t.Fatalf("reading the primary oplog: %v", err)
	}
	if chained == 0 {
		t.Fatalf("premise failed: inserting %d documents produced %d applyOps entries and none chained by prevOpTime, so this case is not exercising the batched-write shape. Raise the document count or the payload size",
			documents, total)
	}
	t.Logf("the load produced %d applyOps entries, %d of them chained", total, chained)

	if _, err := rs.WaitConverged(ctx, 150*time.Second, subject.Addr, reference.Addr); err != nil {
		t.Fatalf("dumbodb %s stopped replicating on a bulk load of %d documents: %v.\n"+
			"This is workspace-9xv if the member log says \"first transaction fragment has non-null prevOpTime\": "+
			"MongoDB chains a large batched write across several applyOps entries carrying lsid and txnNumber "+
			"but no partialTxn, and they are not transaction fragments",
			commit, documents, err)
	}

	subjectClient, err := rs.ClientFor(ctx, subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	count, err := subjectClient.Database(bulkDB).Collection("workload").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("counting on the subject: %v", err)
	}
	if count != documents {
		t.Errorf("dumbodb %s holds %d of %d bulk-loaded documents", commit, count, documents)
	}
}

func chainedApplyOpsCount(ctx context.Context, primary *mongo.Client, dbName string) (int, int, error) {
	cursor, err := primary.Database("local").Collection("oplog.rs").Find(ctx, bson.D{})
	if err != nil {
		return 0, 0, err
	}
	var entries []bson.M
	if err := cursor.All(ctx, &entries); err != nil {
		return 0, 0, err
	}
	chained, total := 0, 0
	for _, entry := range entries {
		object, _ := entry["o"].(bson.M)
		inner, isApplyOps := object["applyOps"].(bson.A)
		if !isApplyOps || !mentionsDatabase(inner, dbName) {
			continue
		}
		total++
		previous, _ := entry["prevOpTime"].(bson.M)
		if previous == nil {
			continue
		}
		if timestamp, ok := previous["ts"].(primitive.Timestamp); ok && timestamp.T != 0 {
			chained++
		}
	}
	return chained, total, nil
}

func mentionsDatabase(operations bson.A, dbName string) bool {
	for _, raw := range operations {
		operation, ok := raw.(bson.M)
		if !ok {
			continue
		}
		if namespace, _ := operation["ns"].(string); strings.HasPrefix(namespace, dbName+".") {
			return true
		}
	}
	return false
}
