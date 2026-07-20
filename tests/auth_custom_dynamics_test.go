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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area I4: grant/revoke dynamics and replace-vs-merge (DYN-01..10).
// Each exercises the enforcement effect of a privilege/role change, reconnecting
// after the change so the effective privileges are freshly resolved.

// dynEnforce creates a role (with initialPrivs) granted to a fresh user, checks
// op before a change, applies change, then reconnects and checks op after.
func dynEnforce(t *testing.T, id string, wantBefore, wantAfter bool, op rbacOp,
	initialPrivs func(db string) []harness.Privilege,
	change func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error) harness.AuthCase {
	return authCaseFull(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "dyn_" + tgt.NS
		role, user, pwd := "role_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropRole(ctx, tgt.Admin, db, role)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup1(tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}))
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, initialPrivs(db), nil))
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: role, DB: db}}))

		before, err := probeOnce(ctx, tgt, db, user, pwd, op)
		if err != nil {
			return nil, err
		}
		tgt.Setup(change(ctx, tgt, db, role, user))
		after, err := probeOnce(ctx, tgt, db, user, pwd, op)
		if err != nil {
			return nil, err
		}
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if before != wantBefore {
				t.Errorf("%s: before-change allowed=%v, want %v", id, before, wantBefore)
			}
			if after != wantAfter {
				t.Errorf("%s: after-change allowed=%v, want %v", id, after, wantAfter)
			}
		}
		return bson.M{"before": before, "after": after}, nil
	})
}

// probeOnce connects as the user (fresh connection resolves current privileges)
// and reports whether op is allowed.
func probeOnce(ctx context.Context, tgt harness.AuthTarget, db, user, pwd string, op rbacOp) (bool, error) {
	c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
	if err != nil {
		return false, err
	}
	defer func() { _ = c.Disconnect(ctx) }()
	return op.fn(ctx, c, db) == nil, nil
}

func dbWide(actions ...string) func(db string) []harness.Privilege {
	return func(db string) []harness.Privilege {
		return []harness.Privilege{{Resource: collResource(db, ""), Actions: actions}}
	}
}

func TestAuthDynamics(t *testing.T) {
	// DYN-01: grant a privilege (op allowed), then revoke it (op denied).
	harness.AuthPairTest(t, dynEnforce(t, "DYN-01-grant-then-revoke", true, false, opFind,
		dbWide("find"),
		func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error {
			return runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokePrivilegesFromRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "find")}}})
		}))

	// DYN-02: grantPrivilegesToRole unions a new action in (insert becomes allowed).
	harness.AuthPairTest(t, dynEnforce(t, "DYN-02-grant-union", false, true, opInsert,
		dbWide("find"),
		func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error {
			return runCmd(ctx, tgt.Admin, db, bson.D{{Key: "grantPrivilegesToRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "insert")}}})
		}))

	// DYN-03: revokePrivilegesFromRole (exact) removes one action, others intact.
	harness.AuthPairTest(t, dynEnforce(t, "DYN-03-revoke-exact-keeps-others", true, true, opFind,
		dbWide("find", "insert"),
		func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error {
			return runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokePrivilegesFromRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "insert")}}})
		}))

	// DYN-04: revoke with a non-matching resource is a no-op (find still allowed).
	harness.AuthPairTest(t, dynEnforce(t, "DYN-04-revoke-nonexact-noop", true, true, opFind,
		dbWide("find"),
		func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error {
			return runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokePrivilegesFromRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, "specific"), "find")}}})
		}))

	// DYN-05: updateRole replaces privileges wholesale (find replaced by insert).
	harness.AuthPairTest(t, dynEnforce(t, "DYN-05-updateRole-replace", true, false, opFind,
		dbWide("find"),
		func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error {
			return runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "insert")}}})
		}))

	// DYN-08: dropRole removes the privilege from its grantee.
	harness.AuthPairTest(t, dynEnforce(t, "DYN-08-dropRole-cascades-to-user", true, false, opFind,
		dbWide("find"),
		func(ctx context.Context, tgt harness.AuthTarget, db, role, user string) error {
			return harness.DropRole(ctx, tgt.Admin, db, role)
		}))
}

func TestAuthDynamicsUserRoles(t *testing.T) {
	// DYN-06: grantRolesToUser makes the new role's privileges effective.
	// DYN-07: revokeRolesFromUser removes them.
	harness.AuthPairTest(t, authCaseFull("DYN-06-07-grant-revoke-role-to-user", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "dynur_" + tgt.NS
		reader, user, pwd := "reader_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropRole(ctx, tgt.Admin, db, reader)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup1(tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}))
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, reader, findPriv(db), nil))
		// User starts with no roles.
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, nil))
		beforeGrant, err := probeOnce(ctx, tgt, db, user, pwd, opFind)
		if err != nil {
			return nil, err
		}
		tgt.Setup(harness.GrantRolesToUser(ctx, tgt.Admin, db, user, []harness.RoleRef{{Role: reader, DB: db}}))
		afterGrant, err := probeOnce(ctx, tgt, db, user, pwd, opFind)
		if err != nil {
			return nil, err
		}
		tgt.Setup(harness.RevokeRolesFromUser(ctx, tgt.Admin, db, user, []harness.RoleRef{{Role: reader, DB: db}}))
		afterRevoke, err := probeOnce(ctx, tgt, db, user, pwd, opFind)
		if err != nil {
			return nil, err
		}
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if beforeGrant || !afterGrant || afterRevoke {
				t.Errorf("DYN-06/07: before=%v afterGrant=%v afterRevoke=%v, want false/true/false", beforeGrant, afterGrant, afterRevoke)
			}
		}
		return bson.M{"before": beforeGrant, "afterGrant": afterGrant, "afterRevoke": afterRevoke}, nil
	}))

	// DYN-09: dropRole removes it from an inheriting role's roles array.
	harness.AuthPairTest(t, authCaseFull("DYN-09-dropRole-cascades-to-role", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "dynir_" + tgt.NS
		base, top := "base_"+tgt.NS, "top_"+tgt.NS
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, top)
			_ = harness.DropRole(ctx, tgt.Admin, db, base)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, base, findPriv(db), nil))
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, top, nil, []harness.RoleRef{{Role: base, DB: db}}))
		tgt.Setup(harness.DropRole(ctx, tgt.Admin, db, base))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: top}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		inherited, _ := r0["roles"].(bson.A)
		return bson.M{"inheritedCountAfterDrop": len(inherited)}, nil
	}))

	// DYN-10: the unauthorized error names the operation ("not authorized").
	harness.AuthPairTest(t, authCaseFull("DYN-10-unauthorized-message-shape", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "dynmsg_" + tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: "read", DB: db}}))
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		insErr := opInsert.fn(ctx, c, db)
		code, _, _ := harness.CommandErrorCode(insErr)
		hasNotAuthorized := insErr != nil && strings.Contains(strings.ToLower(insErr.Error()), "not authorized")
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if code != 13 || !hasNotAuthorized {
				t.Errorf("DYN-10: want code 13 and 'not authorized' message, got code=%d err=%v", code, insErr)
			}
		}
		return bson.M{"code": code, "hasNotAuthorized": hasNotAuthorized}, nil
	}))
}
