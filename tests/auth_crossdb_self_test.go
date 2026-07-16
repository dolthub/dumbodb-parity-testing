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

// Auth parity area J: cross-database roles and self-service (XDB-01..03,
// SELF-01..03).

func TestAuthCrossDatabase(t *testing.T) {
	// XDB-01 / XDB-02: a user on db X with a role on db Y can read Y but not X.
	harness.AuthPairTest(t, authCase("XDB-01-02-role-on-other-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		x, y := "xdbx_"+tgt.NS, "xdby_"+tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, x, user)
			_ = tgt.Admin.Database(x).Drop(ctx)
			_ = tgt.Admin.Database(y).Drop(ctx)
		}()
		for _, d := range []string{x, y} {
			if _, err := tgt.Admin.Database(d).Collection("c").InsertOne(ctx, bson.D{{Key: "v", Value: 1}}); err != nil {
				return nil, err
			}
		}
		// User belongs to X, but is granted read on Y.
		if err := harness.CreateUser(ctx, tgt.Admin, x, user, pwd, []harness.RoleRef{{Role: "read", DB: y}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, x)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		readY := cmdErr(ctx, c, y, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}}) == nil
		readXErr := cmdErr(ctx, c, x, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}})
		xCode, _, _ := harness.CommandErrorCode(readXErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if !readY {
				t.Errorf("XDB-01: user should read granted db Y")
			}
			if readXErr == nil || xCode != 13 {
				t.Errorf("XDB-02: user should be denied on db X (no role), got code=%d err=%v", xCode, readXErr)
			}
		}
		return bson.M{"readY": readY, "readXCode": xCode}, nil
	}))

	// XDB-03: authenticating against the wrong authSource fails.
	harness.AuthPairTest(t, authCase("XDB-03-wrong-authsource", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		x, y := "xdbx_"+tgt.NS, "xdby_"+tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() { _ = harness.DropUser(ctx, tgt.Admin, x, user); _ = tgt.Admin.Database(x).Drop(ctx) }()
		if err := harness.CreateUser(ctx, tgt.Admin, x, user, pwd, []harness.RoleRef{{Role: "read", DB: x}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, y) // wrong authSource
		if err == nil {
			_ = c.Disconnect(ctx)
		}
		return nil, err
	}))
}

func TestAuthSelfService(t *testing.T) {
	// SELF-01 / SELF-02: changeOwnPassword lets a user change its own password
	// but not another user's.
	harness.AuthPairTest(t, authCase("SELF-01-02-changeOwnPassword", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "self_" + tgt.NS
		role, user, pwd := "role_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		other := "other_" + tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropUser(ctx, tgt.Admin, db, other)
			_ = harness.DropRole(ctx, tgt.Admin, db, role)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err := harness.CreateUser(ctx, tgt.Admin, db, other, "pw", nil); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, db, role,
			[]harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"changeOwnPassword", "changeOwnCustomData"}}}, nil); err != nil {
			return nil, err
		}
		if err := harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: role, DB: db}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		selfErr := cmdErr(ctx, c, db, bson.D{{Key: "updateUser", Value: user}, {Key: "pwd", Value: "newpw"}})
		otherErr := cmdErr(ctx, c, db, bson.D{{Key: "updateUser", Value: other}, {Key: "pwd", Value: "newpw"}})
		otherCode, _, _ := harness.CommandErrorCode(otherErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if selfErr != nil {
				t.Errorf("SELF-01: changeOwnPassword should allow self update, got %v", selfErr)
			}
			if otherErr == nil || otherCode != 13 {
				t.Errorf("SELF-02: changing another user's password should be denied, got code=%d err=%v", otherCode, otherErr)
			}
		}
		return bson.M{"selfOK": selfErr == nil, "otherCode": otherCode}, nil
	}))

	// SELF-03: a user may view itself via usersInfo without viewUser, but not
	// other users.
	harness.AuthPairTest(t, authCase("SELF-03-usersInfo-self-vs-others", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "self_" + tgt.NS
		user, pwd, other := "u_"+tgt.NS, "pw-"+tgt.NS, "other_"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropUser(ctx, tgt.Admin, db, other)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err := harness.CreateUser(ctx, tgt.Admin, db, other, "pw", nil); err != nil {
			return nil, err
		}
		// User has read (no viewUser).
		if err := harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: "read", DB: db}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		selfErr := cmdErr(ctx, c, db, bson.D{{Key: "usersInfo", Value: user}})
		otherErr := cmdErr(ctx, c, db, bson.D{{Key: "usersInfo", Value: other}})
		otherCode, _, _ := harness.CommandErrorCode(otherErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if selfErr != nil {
				t.Errorf("SELF-03: user should view itself via usersInfo, got %v", selfErr)
			}
			if otherErr == nil || otherCode != 13 {
				t.Errorf("SELF-03: viewing another user without viewUser should be denied, got code=%d err=%v", otherCode, otherErr)
			}
		}
		return bson.M{"selfOK": selfErr == nil, "otherCode": otherCode}, nil
	}))
}
