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

// TestAuthEphemeralPrimitive is the R4 smoke test: it starts a fresh
// access-control-enabled server pair with zero users and confirms the primitive
// works by exercising MongoDB's localhost exception -- the first createUser
// succeeds unauthenticated, after which the exception is consumed and further
// unauthenticated privileged commands are denied. It also confirms the
// ephemeral DumboDB is reachable.
func TestAuthEphemeralPrimitive(t *testing.T) {
	srv := harness.StartEphemeralServers(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dial := func(uri string) *mongo.Client {
		c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
		if err != nil {
			t.Fatalf("connect %s: %v", uri, err)
		}
		return c
	}

	// Localhost exception: with zero users, the first createUser succeeds
	// without authentication.
	boot := dial(srv.MongoURI)
	defer func() { _ = boot.Disconnect(ctx) }()
	if err := boot.Database("admin").RunCommand(ctx, bson.D{
		{Key: "createUser", Value: "root"},
		{Key: "pwd", Value: "rootpw"},
		{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}}}},
	}).Err(); err != nil {
		t.Fatalf("localhost-exception createUser: %v", err)
	}

	// Exception is now consumed: a fresh unauthenticated connection is denied a
	// privileged command.
	after := dial(srv.MongoURI)
	defer func() { _ = after.Disconnect(ctx) }()
	err := after.Database("admin").RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err()
	if err == nil {
		t.Fatal("localhost exception not consumed: unauthenticated listDatabases succeeded")
	}
	if code, _, _ := harness.CommandErrorCode(err); code != 13 {
		t.Fatalf("post-bootstrap unauthenticated command: want Unauthorized(13), got %v", err)
	}

	// The created admin can authenticate and act.
	admin, err := harness.ConnectAs(ctx, srv.MongoURI, "root", "rootpw", "admin")
	if err != nil {
		t.Fatalf("authenticate as bootstrapped root: %v", err)
	}
	defer func() { _ = admin.Disconnect(ctx) }()
	if err := admin.Database("admin").RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err(); err != nil {
		t.Fatalf("root listDatabases: %v", err)
	}

	// The ephemeral DumboDB is reachable.
	dc := dial(srv.DumboDBURI)
	defer func() { _ = dc.Disconnect(ctx) }()
	if err := dc.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
		t.Fatalf("ping ephemeral dumbodb: %v", err)
	}
}
