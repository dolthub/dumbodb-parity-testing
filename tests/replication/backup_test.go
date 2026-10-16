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
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const (
	backupDB               = "backupdb"
	backupDocs             = 50
	codeNotWritablePrimary = 10107
)

func backupCommand(fields ...bson.E) bson.D {
	return append(bson.D{{Key: "dumboBackup", Value: int32(1)}}, fields...)
}

func commandCode(err error) int32 {
	var ce mongo.CommandError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return -1
}

// A hidden DumboDB secondary is the natural place to take backups from: it
// holds every replicated database but serves no application traffic. sync and
// sync-url only read local data, so they run there; add, remove and restore
// write to the server and are refused like any write on a secondary. The
// backup it produces restores on a standalone server with the replicated data.
func TestBackup_HiddenSecondarySyncsButDoesNotMutate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	fx := startMemberProtocol(t, ctx)

	primary, err := fx.rs.Client(ctx)
	if err != nil {
		t.Fatalf("primary client: %v", err)
	}
	defer func() { _ = primary.Disconnect(context.Background()) }()

	docs := make([]interface{}, backupDocs)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: i}, {Key: "v", Value: i * i}}
	}
	if _, err := primary.Database(backupDB).Collection("items").InsertMany(ctx, docs); err != nil {
		t.Fatalf("seed primary: %v", err)
	}
	if _, err := fx.rs.WaitConverged(ctx, 150*time.Second, fx.subject.Addr); err != nil {
		t.Fatalf("dumbodb %s did not converge: %v", fx.commit, err)
	}

	subject, err := fx.rs.ClientFor(ctx, fx.subject.Addr)
	if err != nil {
		t.Fatalf("subject client: %v", err)
	}
	admin := subject.Database("admin")
	url := "file://" + filepath.Join(t.TempDir(), "bk")

	var res bson.M
	if err := admin.RunCommand(ctx, backupCommand(bson.E{Key: "action", Value: "list"}, bson.E{Key: "db", Value: backupDB})).Decode(&res); err != nil {
		t.Fatalf("list on secondary: %v", err)
	}

	sync := backupCommand(bson.E{Key: "action", Value: "sync-url"}, bson.E{Key: "db", Value: backupDB}, bson.E{Key: "url", Value: url})
	res = nil
	if err := admin.RunCommand(ctx, sync).Decode(&res); err != nil {
		t.Fatalf("sync-url on secondary (dumbodb %s): %v", fx.commit, err)
	}
	root, _ := res["root"].(string)
	if root == "" || res["upToDate"] != false {
		t.Fatalf("first sync-url = %v, want a root and upToDate false", res)
	}

	for _, fields := range [][]bson.E{
		{{Key: "action", Value: "add"}, {Key: "db", Value: backupDB}, {Key: "name", Value: "nightly"}, {Key: "url", Value: url}},
		{{Key: "action", Value: "remove"}, {Key: "db", Value: backupDB}, {Key: "name", Value: "nightly"}},
		{{Key: "action", Value: "restore"}, {Key: "url", Value: url}, {Key: "as", Value: "restored"}},
	} {
		err := admin.RunCommand(ctx, backupCommand(fields...)).Err()
		if code := commandCode(err); code != codeNotWritablePrimary {
			t.Errorf("dumboBackup %v on secondary: err = %v (code %d), want NotWritablePrimary (%d)", fields[0].Value, err, code, codeNotWritablePrimary)
		}
	}

	standalone := harness.StartRestartableServers(t)
	restoreClient, err := mongo.Connect(ctx, options.Client().ApplyURI(standalone.DumboDBURI))
	if err != nil {
		t.Fatalf("standalone client: %v", err)
	}
	defer func() { _ = restoreClient.Disconnect(context.Background()) }()

	res = nil
	restore := backupCommand(bson.E{Key: "action", Value: "restore"}, bson.E{Key: "url", Value: url}, bson.E{Key: "as", Value: backupDB})
	if err := restoreClient.Database("admin").RunCommand(ctx, restore).Decode(&res); err != nil {
		t.Fatalf("restore on standalone: %v", err)
	}
	if res["root"] != root {
		t.Errorf("restored root = %v, want the synced root %s", res["root"], root)
	}

	n, err := restoreClient.Database(backupDB).Collection("items").CountDocuments(ctx, bson.D{})
	if err != nil {
		t.Fatalf("count restored: %v", err)
	}
	if n != backupDocs {
		t.Errorf("restored %d documents, want %d", n, backupDocs)
	}
	var doc bson.M
	if err := restoreClient.Database(backupDB).Collection("items").FindOne(ctx, bson.D{{Key: "_id", Value: 7}}).Decode(&doc); err != nil {
		t.Fatalf("find restored doc: %v", err)
	}
	if v, _ := doc["v"].(int32); v != 49 {
		t.Errorf("restored doc = %v, want v 49", doc)
	}
}
