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
	"context"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area A: server enablement and the localhost exception
// (ENABLE-01..12). These assert behavior on a fresh, access-control-enabled
// server with zero users, using the ephemeral-server primitive.
//
// DumboDB now implements the localhost exception (loopback createUser bootstraps
// the first user, then latches off), so it walks the same state machine as
// MongoDB. Cases needing a non-localhost client, an --auth-off server, or
// enableLocalhostAuthBypass=0 (ENABLE-01/02/09/11) require setups not available
// from the test host and are left for a dedicated deployment harness.

func mkCreateAdmin(name string) bson.D {
	return bson.D{
		{Key: "createUser", Value: name},
		{Key: "pwd", Value: "rootpw"},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}}}},
	}
}

// walkLocalhostException walks the localhost-exception state machine against one
// fresh server: the exception is narrow (only createUser), consumed by the first
// createUser, after which unauthenticated operations are denied and the
// bootstrapped admin can authenticate and create more users.
func walkLocalhostException(t *testing.T, ctx context.Context, uri, server string) {
	t.Helper()

	noauth, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("%s connect: %v", server, err)
	}
	defer func() { _ = noauth.Disconnect(ctx) }()
	admin := noauth.Database("admin")

	code := func(err error) int32 { c, _, _ := harness.CommandErrorCode(err); return c }

	// ENABLE-03: before any user, the exception does NOT grant arbitrary reads.
	if err := admin.RunCommand(ctx, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}}).Err(); err == nil {
		t.Errorf("%s ENABLE-03: unauthenticated find should be denied under the (narrow) localhost exception", server)
	}
	// ENABLE-06: nor arbitrary admin commands like serverStatus.
	if err := admin.RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Err(); err == nil {
		t.Errorf("%s ENABLE-06: unauthenticated serverStatus should be denied under the localhost exception", server)
	}
	// ENABLE-04: the exception DOES permit creating the first user.
	if err := admin.RunCommand(ctx, mkCreateAdmin("root")).Err(); err != nil {
		t.Fatalf("%s ENABLE-04: first createUser under localhost exception should succeed: %v", server, err)
	}
	// ENABLE-07: the exception is now consumed; unauthenticated ops are denied.
	if c := code(admin.RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err()); c != 13 {
		t.Errorf("%s ENABLE-07: after first user, unauthenticated command should be Unauthorized(13), got %d", server, c)
	}
	// ENABLE-08: a second unauthenticated createUser is denied.
	if c := code(admin.RunCommand(ctx, mkCreateAdmin("root2")).Err()); c != 13 {
		t.Errorf("%s ENABLE-08: second unauthenticated createUser should be Unauthorized(13), got %d", server, c)
	}
	// ENABLE-10: the bootstrapped admin can authenticate and create more users.
	ac, err := harness.ConnectAs(ctx, uri, "root", "rootpw", "admin")
	if err != nil {
		t.Fatalf("%s ENABLE-10: bootstrapped admin should authenticate: %v", server, err)
	}
	defer func() { _ = ac.Disconnect(ctx) }()
	if err := ac.Database("admin").RunCommand(ctx, mkCreateAdmin("root2")).Err(); err != nil {
		t.Errorf("%s ENABLE-10: authenticated admin should create another user: %v", server, err)
	}
}

// TestAuthLocalhostExceptionWalk walks the localhost-exception state machine on a
// fresh MongoDB and a fresh DumboDB: the exception is narrow (only createUser)
// and is consumed by the first createUser. DumboDB matches MongoDB here.
func TestAuthLocalhostExceptionWalk(t *testing.T) {
	srv := harness.StartEphemeralServers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	walkLocalhostException(t, ctx, srv.MongoURI, "MongoDB")
	walkLocalhostException(t, ctx, srv.DumboDBURI, "DumboDB")
}

// TestAuthLocalhostExceptionCreateRole checks ENABLE-05 on its own fresh server.
// Although the manual describes the localhost exception as permitting the first
// "user or role", MongoDB 8.0.4 in practice grants it only for createUser:
// createRole on a fresh server is denied with Unauthorized(13). This test pins
// the observed behavior.
func TestAuthLocalhostExceptionCreateRole(t *testing.T) {
	srv := harness.StartEphemeralServers(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	noauth, err := mongo.Connect(ctx, options.Client().ApplyURI(srv.MongoURI))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = noauth.Disconnect(ctx) }()

	err = noauth.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createRole", Value: "bootrole"},
		{Key: "privileges", Value: bson.A{}},
		{Key: "roles", Value: bson.A{}},
	}).Err()
	c, _, _ := harness.CommandErrorCode(err)
	if c != 13 {
		t.Errorf("ENABLE-05: createRole under the localhost exception is denied on MongoDB 8.0.4; want Unauthorized(13), got code=%d err=%v", c, err)
	}
}
