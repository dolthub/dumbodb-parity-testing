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

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area SYS: direct client access to the auth store,
// admin.system.users. MongoDB-root permits raw insert/update/delete on it while
// denying drop (IllegalOperation) and create of system.* (Unauthorized). DumboDB
// deviates: it forbids all direct client mutation of admin.system.*, so the auth
// store changes only through the user management commands.

func allowedOnMongo(t *testing.T, _ interface{}, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("MongoDB: expected the operation to be permitted, got %v", err)
	}
}

func deniedWith(code int32) func(t *testing.T, res interface{}, err error) {
	return func(t *testing.T, _ interface{}, err error) {
		t.Helper()
		got, _, ok := harness.CommandErrorCode(err)
		if !ok {
			t.Errorf("expected a denial error, got %v", err)
			return
		}
		if got != code {
			t.Errorf("expected denial code %d, got %d (err=%v)", code, got, err)
		}
	}
}

func TestAuthSystemCollectionRowWritesDeviate(t *testing.T) {
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SYS-01-raw-insert-system-users",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			users := tgt.Admin.Database("admin").Collection("system.users")
			id := "sysdev_" + tgt.NS + ".ghost"
			_, err := users.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "user", Value: "ghost_" + tgt.NS}, {Key: "db", Value: "sysdev_" + tgt.NS}})
			_, _ = users.DeleteOne(ctx, bson.D{{Key: "_id", Value: id}})
			return nil, err
		},
		MongoExpect: allowedOnMongo,
		DumboExpect: deniedWith(13),
	})

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SYS-02-raw-update-system-users",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db, u := "sysdev_"+tgt.NS, "v_"+tgt.NS
			_ = harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil)
			defer func() { _ = harness.DropUser(ctx, tgt.Admin, db, u) }()
			_, err := tgt.Admin.Database("admin").Collection("system.users").
				UpdateOne(ctx, bson.D{{Key: "user", Value: u}}, bson.D{{Key: "$set", Value: bson.D{{Key: "customData", Value: bson.D{{Key: "x", Value: 1}}}}}})
			return nil, err
		},
		MongoExpect: allowedOnMongo,
		DumboExpect: deniedWith(13),
	})

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SYS-03-raw-delete-system-users",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db, u := "sysdev_"+tgt.NS, "v_"+tgt.NS
			_ = harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil)
			defer func() { _ = harness.DropUser(ctx, tgt.Admin, db, u) }()
			_, err := tgt.Admin.Database("admin").Collection("system.users").
				DeleteOne(ctx, bson.D{{Key: "user", Value: u}})
			return nil, err
		},
		MongoExpect: allowedOnMongo,
		DumboExpect: deniedWith(13),
	})
}

func TestAuthSystemCollectionStructuralDenied(t *testing.T) {
	// Both servers deny these; DumboDB matches MongoDB's denial codes.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SYS-04-drop-system-users",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			return nil, tgt.Admin.Database("admin").Collection("system.users").Drop(ctx)
		},
		MongoExpect: deniedWith(20),
		DumboExpect: deniedWith(20),
	})

	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SYS-05-create-system-collection",
		Support: harness.DumboDBDeviates,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			return nil, tgt.Admin.Database("admin").RunCommand(ctx, bson.D{{Key: "create", Value: "system.foobar"}}).Err()
		},
		MongoExpect: deniedWith(13),
		DumboExpect: deniedWith(13),
	})
}
