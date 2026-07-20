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

	if err := ac.DumboDBAdmin.Database("admin").
		RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err(); err != nil {
		t.Fatalf("admin listDatabases on DumboDB: %v", err)
	}

	for _, srv := range []struct {
		name string
		uri  string
	}{
		{"MongoDB", harness.AuthMongoBaseURI()},
		{"DumboDB", harness.AuthDumboDBBaseURI()},
	} {
		noAuth, err := mongo.Connect(ctx, options.Client().ApplyURI(srv.uri))
		if err != nil {
			t.Fatalf("connect no-auth client to %s: %v", srv.name, err)
		}
		err = noAuth.Database("admin").RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err()
		_ = noAuth.Disconnect(ctx)
		if err == nil {
			t.Fatalf("unauthenticated listDatabases on %s succeeded; --auth is not enforced", srv.name)
		}
		var ce mongo.CommandError
		if !mongoErrorHasCode(err, &ce, 13) {
			t.Fatalf("unauthenticated listDatabases on %s: want Unauthorized(13), got %v", srv.name, err)
		}
		t.Logf("%s unauthenticated listDatabases correctly rejected: %v", srv.name, err)
	}
}

// mongoErrorHasCode reports whether err is a CommandError with the given code.
func mongoErrorHasCode(err error, ce *mongo.CommandError, code int32) bool {
	if c, ok := err.(mongo.CommandError); ok {
		*ce = c
		return c.Code == code
	}
	return false
}
