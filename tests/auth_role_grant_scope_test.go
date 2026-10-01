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

// Auth parity area: role and privilege grants are authorized against the
// database of each granted role or privilege, not the database the command
// runs on (GRANT-01..09).

type grantScopeNames struct {
	db, actor, other, role, fresh, adminRole string
}

var rootRole = bson.A{bson.D{{Key: "role", Value: "root"}, {Key: "db", Value: "admin"}}}

func TestAuthRoleGrantScope(t *testing.T) {
	dbUserAdmin := func(n grantScopeNames) []harness.RoleRef { return []harness.RoleRef{{Role: "userAdmin", DB: n.db}} }
	adminUserAdmin := func(n grantScopeNames) []harness.RoleRef { return []harness.RoleRef{{Role: "userAdmin", DB: "admin"}} }

	// GRANT-01: a db-scoped user admin cannot grant itself admin.root.
	harness.AuthPairTest(t, authCase("GRANT-01-grantRolesToUser-self-root", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "grantRolesToUser", Value: n.actor}, {Key: "roles", Value: rootRole}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedUserRoles(ctx, tgt, n.db, n.actor) })
	}))

	// GRANT-02: a db-scoped user admin cannot grant an admin-db role to another user.
	harness.AuthPairTest(t, authCase("GRANT-02-grantRolesToUser-other-anyDatabase", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "grantRolesToUser", Value: n.other}, {Key: "roles", Value: bson.A{
					bson.D{{Key: "role", Value: "readWriteAnyDatabase"}, {Key: "db", Value: "admin"}}}}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedUserRoles(ctx, tgt, n.db, n.other) })
	}))

	// GRANT-03: a db-scoped user admin cannot make a role inherit admin.root.
	harness.AuthPairTest(t, authCase("GRANT-03-grantRolesToRole-root", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "grantRolesToRole", Value: n.role}, {Key: "roles", Value: rootRole}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedRole(ctx, tgt, n.db, n.role) })
	}))

	// GRANT-04: a db-scoped user admin cannot revoke admin.root from another user.
	harness.AuthPairTest(t, authCase("GRANT-04-revokeRolesFromUser-root", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13,
			func(n grantScopeNames) error {
				return runCmd(ctx, tgt.Admin, n.db, bson.D{{Key: "grantRolesToUser", Value: n.other}, {Key: "roles", Value: rootRole}})
			},
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "revokeRolesFromUser", Value: n.other}, {Key: "roles", Value: rootRole}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedUserRoles(ctx, tgt, n.db, n.other) })
	}))

	// GRANT-05: a db-scoped user admin cannot create a user holding admin.root.
	harness.AuthPairTest(t, authCase("GRANT-05-createUser-root", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "createUser", Value: n.fresh}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: rootRole}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedUserRoles(ctx, tgt, n.db, n.fresh) })
	}))

	// GRANT-06: a db-scoped user admin cannot create a role inheriting admin.root.
	harness.AuthPairTest(t, authCase("GRANT-06-createRole-root", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "createRole", Value: n.fresh}, {Key: "privileges", Value: bson.A{}}, {Key: "roles", Value: rootRole}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedRole(ctx, tgt, n.db, n.fresh) })
	}))

	// GRANT-07: updateRole requires revokeRole on any normal resource, which a
	// db-scoped user admin lacks.
	harness.AuthPairTest(t, authCase("GRANT-07-updateRole-root", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, dbUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "updateRole", Value: n.role}, {Key: "roles", Value: rootRole}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedRole(ctx, tgt, n.db, n.role) })
	}))

	// GRANT-08: an admin-db user admin cannot grant a privilege on another db.
	harness.AuthPairTest(t, authCase("GRANT-08-grantPrivilegesToRole-other-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, adminUserAdmin, 13, nil,
			func(n grantScopeNames) (string, bson.D) {
				return "admin", bson.D{{Key: "grantPrivilegesToRole", Value: n.adminRole}, {Key: "privileges", Value: bson.A{
					priv(collResource(n.db, ""), "find")}}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedRole(ctx, tgt, "admin", n.adminRole) })
	}))

	// GRANT-09: an admin-db user admin can grant an admin-db role to a user on
	// another db; no privilege on the user's db is required.
	harness.AuthPairTest(t, authCase("GRANT-09-grantRolesToUser-admin-role-from-admin", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return grantScopeCase(ctx, t, tgt, adminUserAdmin, 0, nil,
			func(n grantScopeNames) (string, bson.D) {
				return n.db, bson.D{{Key: "grantRolesToUser", Value: n.other}, {Key: "roles", Value: bson.A{
					bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: "admin"}}}}}
			},
			func(n grantScopeNames) (interface{}, error) { return storedUserRoles(ctx, tgt, n.db, n.other) })
	}))
}

// grantScopeCase runs a command as a user holding only |actorRoles| and
// reports its error code together with what |observe| reads back as admin.
// Fixtures: |other| (a user with no roles) and |role| (a role with no
// privileges) on |db|, and |adminRole| (no privileges) on admin. MongoDB must
// answer with |wantMongoCode| (0 = success).
func grantScopeCase(
	ctx context.Context,
	t *testing.T,
	tgt harness.AuthTarget,
	actorRoles func(grantScopeNames) []harness.RoleRef,
	wantMongoCode int32,
	setup func(grantScopeNames) error,
	command func(grantScopeNames) (string, bson.D),
	observe func(grantScopeNames) (interface{}, error),
) (interface{}, error) {
	n := grantScopeNames{
		db:        "gscope_" + tgt.NS,
		actor:     "actor_" + tgt.NS,
		other:     "other_" + tgt.NS,
		role:      "role_" + tgt.NS,
		fresh:     "fresh_" + tgt.NS,
		adminRole: "adm_" + tgt.NS,
	}
	defer func() {
		for _, u := range []string{n.actor, n.other, n.fresh} {
			_ = harness.DropUser(ctx, tgt.Admin, n.db, u)
		}
		_ = harness.DropRole(ctx, tgt.Admin, n.db, n.role)
		_ = harness.DropRole(ctx, tgt.Admin, n.db, n.fresh)
		_ = harness.DropRole(ctx, tgt.Admin, "admin", n.adminRole)
		_ = tgt.Admin.Database(n.db).Drop(ctx)
	}()

	tgt.Setup(harness.CreateUser(ctx, tgt.Admin, n.db, n.other, "pw", nil))
	tgt.Setup(harness.CreateRole(ctx, tgt.Admin, n.db, n.role, nil, nil))
	tgt.Setup(harness.CreateRole(ctx, tgt.Admin, "admin", n.adminRole, nil, nil))
	tgt.Setup(harness.CreateUser(ctx, tgt.Admin, n.db, n.actor, "pw", actorRoles(n)))
	if setup != nil {
		tgt.Setup(setup(n))
	}

	c, err := harness.ConnectAs(ctx, tgt.BaseURI, n.actor, "pw", n.db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Disconnect(ctx) }()

	cmdDB, cmd := command(n)
	cmdErrResult := cmdErr(ctx, c, cmdDB, cmd)
	code, _, _ := harness.CommandErrorCode(cmdErrResult)
	if tgt.BaseURI == harness.AuthMongoBaseURI() && code != wantMongoCode {
		t.Errorf("%s: MongoDB returned code=%d err=%v, want code %d", cmd[0].Key, code, cmdErrResult, wantMongoCode)
	}

	state, err := observe(n)
	if err != nil {
		return nil, err
	}
	return bson.M{"code": code, "state": state}, nil
}

// storedUserRoles returns the user's stored roles, or nil if the user does not exist.
func storedUserRoles(ctx context.Context, tgt harness.AuthTarget, db, user string) (interface{}, error) {
	res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: user}})
	if err != nil {
		return nil, err
	}
	users, _ := res["users"].(bson.A)
	if len(users) == 0 {
		return nil, nil
	}
	stored, _ := users[0].(bson.M)
	return stored["roles"], nil
}

// storedRole returns the role's inherited roles and privilege count, or nil if
// the role does not exist.
func storedRole(ctx context.Context, tgt harness.AuthTarget, db, role string) (interface{}, error) {
	res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
	if err != nil {
		return nil, err
	}
	roles, _ := res["roles"].(bson.A)
	if len(roles) == 0 {
		return nil, nil
	}
	stored, _ := roles[0].(bson.M)
	privileges, _ := stored["privileges"].(bson.A)
	return bson.M{"roles": stored["roles"], "privileges": len(privileges)}, nil
}
