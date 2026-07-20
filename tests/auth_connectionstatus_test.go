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
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area D: connectionStatus, logout, and pre-auth connection state
// (CONN-01..08). CONN-01/04/07 are DumboDBFull (DumboDB already matches the
// anonymous/pre-auth behavior); the rest are XFail pending auth enforcement.

// authInfoCounts runs connectionStatus on c and returns the counts of
// authenticatedUsers and authenticatedUserRoles, plus whether a privileges
// field is present.
func authInfoCounts(ctx context.Context, c *mongo.Client, showPrivileges bool) (bson.M, error) {
	cmd := bson.D{{Key: "connectionStatus", Value: 1}}
	if showPrivileges {
		cmd = append(cmd, bson.E{Key: "showPrivileges", Value: true})
	}
	var res bson.M
	if err := c.Database("admin").RunCommand(ctx, cmd).Decode(&res); err != nil {
		return nil, err
	}
	ai, _ := res["authInfo"].(bson.M)
	users, _ := ai["authenticatedUsers"].(bson.A)
	roles, _ := ai["authenticatedUserRoles"].(bson.A)
	_, hasPrivs := ai["authenticatedUserPrivileges"]
	return bson.M{"users": len(users), "roles": len(roles), "hasPrivileges": hasPrivs}, nil
}

func TestAuthConnectionStatusPreAuth(t *testing.T) {
	// CONN-01: pre-auth connectionStatus reports empty user/role sets.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-01-connectionStatus-preauth-empty",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			return authInfoCounts(ctx, c, false)
		},
	})

	// CONN-04: connectionStatus is allowed pre-auth (anonymous).
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-04-connectionStatus-anonymous-allowed",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			err = c.Database("admin").RunCommand(ctx, bson.D{{Key: "connectionStatus", Value: 1}}).Err()
			return bson.M{"ok": err == nil}, err
		},
	})
}

func TestAuthConnectionStatusAuthenticated(t *testing.T) {
	// CONN-02: after auth, connectionStatus lists the user and its roles.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-02-connectionStatus-after-auth",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "conn02_" + tgt.NS
			user := "u_" + tgt.NS
			pwd := "pw-" + tgt.NS
			defer func() {
				_ = harness.DropUser(ctx, tgt.Admin, db, user)
				_ = tgt.Admin.Database(db).Drop(ctx)
			}()
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: "readWrite", DB: db}}))
			c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			return authInfoCounts(ctx, c, false)
		},
	})

	// CONN-03: showPrivileges adds authenticatedUserPrivileges.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-03-connectionStatus-showPrivileges",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "conn03_" + tgt.NS
			user := "u_" + tgt.NS
			pwd := "pw-" + tgt.NS
			defer func() {
				_ = harness.DropUser(ctx, tgt.Admin, db, user)
				_ = tgt.Admin.Database(db).Drop(ctx)
			}()
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: "readWrite", DB: db}}))
			c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			return authInfoCounts(ctx, c, true)
		},
	})
}

func TestAuthLogout(t *testing.T) {
	// CONN-06: logout when not authenticated is a harmless ok.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-06-logout-unauthenticated",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			err = c.Database("admin").RunCommand(ctx, bson.D{{Key: "logout", Value: 1}}).Err()
			return bson.M{"ok": err == nil}, err
		},
	})

	// CONN-05: logout after authenticating succeeds.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-05-logout-after-auth",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "conn05_" + tgt.NS
			user := "u_" + tgt.NS
			pwd := "pw-" + tgt.NS
			defer func() {
				_ = harness.DropUser(ctx, tgt.Admin, db, user)
				_ = tgt.Admin.Database(db).Drop(ctx)
			}()
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: "readWrite", DB: db}}))
			c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			err = c.Database(db).RunCommand(ctx, bson.D{{Key: "logout", Value: 1}}).Err()
			return bson.M{"ok": err == nil}, err
		},
	})
}

func TestAuthPreAuthState(t *testing.T) {
	// CONN-07: ping is allowed pre-auth (anonymous).
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-07-ping-anonymous",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			err = c.Database("admin").RunCommand(ctx, bson.D{{Key: "ping", Value: 1}}).Err()
			return bson.M{"ok": err == nil}, err
		},
	})

	// CONN-08: a privileged command pre-auth is rejected (Unauthorized 13).
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "CONN-08-privileged-command-preauth-denied",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			// listDatabases requires the listDatabases cluster action; with
			// access control on and no authentication, MongoDB returns 13.
			return nil, c.Database("admin").RunCommand(ctx, bson.D{{Key: "listDatabases", Value: 1}}).Err()
		},
	})
}
