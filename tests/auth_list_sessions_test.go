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

package tests

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area: $listLocalSessions is scoped to the caller (LSESS-01..05).
// Listing your own sessions needs no privilege; {allUsers: true} or naming
// another user needs listSessions on the cluster. A session's _id.uid is
// SHA256("user@db").

func TestAuthListLocalSessions(t *testing.T) {
	readWrite := func(db string) []harness.RoleRef { return []harness.RoleRef{{Role: "readWrite", DB: db}} }
	readAnyDatabase := func(string) []harness.RoleRef { return []harness.RoleRef{{Role: "readAnyDatabase", DB: "admin"}} }
	clusterMonitor := func(string) []harness.RoleRef { return []harness.RoleRef{{Role: "clusterMonitor", DB: "admin"}} }

	// LSESS-01: an ordinary user may list its own sessions.
	harness.AuthPairTest(t, authCase("LSESS-01-own-sessions-no-privilege", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return listLocalSessionsCase(ctx, t, tgt, readWrite, func(string, string) bson.D { return bson.D{} }, 0)
	}))

	// LSESS-02: a user who can read admin still sees only its own sessions.
	harness.AuthPairTest(t, authCase("LSESS-02-readAnyDatabase-sees-only-own", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return listLocalSessionsCase(ctx, t, tgt, readAnyDatabase, func(string, string) bson.D { return bson.D{} }, 0)
	}))

	// LSESS-03: allUsers requires listSessions.
	harness.AuthPairTest(t, authCase("LSESS-03-allUsers-needs-listSessions", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return listLocalSessionsCase(ctx, t, tgt, readAnyDatabase,
			func(string, string) bson.D { return bson.D{{Key: "allUsers", Value: true}} }, 13)
	}))

	// LSESS-04: naming another user requires listSessions.
	harness.AuthPairTest(t, authCase("LSESS-04-other-user-needs-listSessions", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return listLocalSessionsCase(ctx, t, tgt, readAnyDatabase, func(other, db string) bson.D {
			return bson.D{{Key: "users", Value: bson.A{bson.D{{Key: "user", Value: other}, {Key: "db", Value: db}}}}}
		}, 13)
	}))

	// LSESS-05: clusterMonitor carries listSessions, so allUsers is allowed.
	harness.AuthPairTest(t, authCase("LSESS-05-clusterMonitor-allUsers", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return listLocalSessionsCase(ctx, t, tgt, clusterMonitor,
			func(string, string) bson.D { return bson.D{{Key: "allUsers", Value: true}} }, 0)
	}))
}

// listLocalSessionsCase runs $listLocalSessions with |spec| as a user holding
// |actorRoles| while another user also holds a session, and reports the error
// code and whether every returned session belongs to the caller. MongoDB must
// answer with |wantMongoCode| (0 = success).
func listLocalSessionsCase(
	ctx context.Context,
	t *testing.T,
	tgt harness.AuthTarget,
	actorRoles func(db string) []harness.RoleRef,
	spec func(other, db string) bson.D,
	wantMongoCode int32,
) (interface{}, error) {
	db := "lsess_" + tgt.NS
	actor, other := "actor_"+tgt.NS, "other_"+tgt.NS
	defer func() {
		_ = harness.DropUser(ctx, tgt.Admin, db, actor)
		_ = harness.DropUser(ctx, tgt.Admin, db, other)
		_ = tgt.Admin.Database(db).Drop(ctx)
	}()
	tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, other, "pw", []harness.RoleRef{{Role: "readWrite", DB: db}}))
	tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, actor, "pw", actorRoles(db)))

	otherClient, err := harness.ConnectAs(ctx, tgt.BaseURI, other, "pw", db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = otherClient.Disconnect(ctx) }()
	tgt.Setup(cmdErr(ctx, otherClient, db, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}}))

	c, err := harness.ConnectAs(ctx, tgt.BaseURI, actor, "pw", db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Disconnect(ctx) }()

	var res bson.M
	listErr := c.Database("admin").RunCommand(ctx, bson.D{
		{Key: "aggregate", Value: 1},
		{Key: "pipeline", Value: bson.A{bson.D{{Key: "$listLocalSessions", Value: spec(other, db)}}}},
		{Key: "cursor", Value: bson.D{}},
	}).Decode(&res)
	code, _, _ := harness.CommandErrorCode(listErr)
	if tgt.BaseURI == harness.AuthMongoBaseURI() && code != wantMongoCode {
		t.Errorf("$listLocalSessions: MongoDB returned code=%d err=%v, want code %d", code, listErr, wantMongoCode)
	}
	if listErr != nil {
		return bson.M{"code": code}, nil
	}

	actorUID := sha256.Sum256([]byte(actor + "@" + db))
	cursor, _ := res["cursor"].(bson.M)
	sessions, _ := cursor["firstBatch"].(bson.A)
	onlyOwn := true
	for _, s := range sessions {
		doc, _ := s.(bson.M)
		id, _ := doc["_id"].(bson.M)
		uid, _ := id["uid"].(primitive.Binary)
		if !bytes.Equal(uid.Data, actorUID[:]) {
			onlyOwn = false
		}
	}
	if len(spec(other, db)) > 0 {
		return bson.M{"code": code, "nonEmpty": len(sessions) > 0}, nil
	}
	return bson.M{"code": code, "onlyOwnSessions": onlyOwn}, nil
}
