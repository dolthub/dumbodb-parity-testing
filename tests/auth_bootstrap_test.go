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

// TestAuthHarnessBootstrap is the R1 smoke test: it proves the harness can
// bootstrap the MongoDB admin via the localhost exception and connect to both
// servers as that admin, and that MongoDB is genuinely enforcing access
// control (an unauthenticated privileged command is rejected).
func TestAuthHarnessBootstrap(t *testing.T) {
	harness.RequireAuth(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ac, err := harness.GetAuthClients(ctx)
	if err != nil {
		t.Fatalf("GetAuthClients: %v", err)
	}

	// Admin can run a privileged command on MongoDB.
	if err := ac.MongoAdmin.Database("admin").
		RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err(); err != nil {
		t.Fatalf("admin listDatabases on MongoDB: %v", err)
	}

	// connectionStatus reports the admin as authenticated.
	var cs bson.M
	if err := ac.MongoAdmin.Database("admin").
		RunCommand(ctx, bson.D{{Key: "connectionStatus", Value: 1}}).Decode(&cs); err != nil {
		t.Fatalf("connectionStatus on MongoDB: %v", err)
	}
	t.Logf("MongoDB connectionStatus.authInfo = %v", cs["authInfo"])

	// DumboDB admin connection is usable (--auth is a no-op today).
	if err := ac.DumboDBAdmin.Database("admin").
		RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
		t.Fatalf("ping DumboDB as admin: %v", err)
	}

	// Confirm MongoDB actually enforces auth: an unauthenticated privileged
	// command must be rejected. If this passes, --auth is not really on and
	// the whole auth suite would be meaningless.
	noAuth, err := mongo.Connect(ctx, options.Client().ApplyURI(harness.AuthMongoBaseURI()))
	if err != nil {
		t.Fatalf("connect no-auth client: %v", err)
	}
	defer func() { _ = noAuth.Disconnect(ctx) }()
	err = noAuth.Database("admin").RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err()
	if err == nil {
		t.Fatal("unauthenticated listDatabases succeeded; MongoDB is not enforcing --auth")
	}
	var ce mongo.CommandError
	if !mongoErrorHasCode(err, &ce, 13) {
		t.Fatalf("unauthenticated listDatabases: want Unauthorized(13), got %v", err)
	}
	t.Logf("unauthenticated listDatabases correctly rejected: %v", err)
}

// mongoErrorHasCode reports whether err is a CommandError with the given code.
func mongoErrorHasCode(err error, ce *mongo.CommandError, code int32) bool {
	if c, ok := err.(mongo.CommandError); ok {
		*ce = c
		return c.Code == code
	}
	return false
}
