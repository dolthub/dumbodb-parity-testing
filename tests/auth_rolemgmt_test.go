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

// Auth parity area G: role management command semantics (ROLE-01..25). DumboDB
// does not implement the role commands (createRole et al are not registered ->
// CommandNotFound), so every case diverges from MongoDB and starts XFail.

func decodeCmd(ctx context.Context, admin *mongo.Client, db string, cmd bson.D) (bson.M, error) {
	var res bson.M
	err := admin.Database(db).RunCommand(ctx, cmd).Decode(&res)
	return res, err
}

func priv(resource bson.D, actions ...string) bson.D {
	a := bson.A{}
	for _, act := range actions {
		a = append(a, act)
	}
	return bson.D{{Key: "resource", Value: resource}, {Key: "actions", Value: a}}
}

func collResource(db, coll string) bson.D {
	return bson.D{{Key: "db", Value: db}, {Key: "collection", Value: coll}}
}

func rolesCount(res bson.M) int {
	roles, _ := res["roles"].(bson.A)
	return len(roles)
}

func anyBuiltin(res bson.M) bool {
	roles, _ := res["roles"].(bson.A)
	for _, r := range roles {
		m, _ := r.(bson.M)
		if b, ok := m["isBuiltin"].(bool); ok && b {
			return true
		}
	}
	return false
}

func TestAuthRoleCreate(t *testing.T) {
	// ROLE-01: createRole with privileges and inherited roles succeeds.
	harness.AuthPairTest(t, authCaseFull("ROLE-01-create", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		err := runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{priv(collResource(db, ""), "find")}},
			{Key: "roles", Value: bson.A{}},
		})
		return bson.M{"ok": err == nil}, err
	}))

	// ROLE-02: duplicate createRole is DuplicateKey (11000).
	harness.AuthPairTest(t, authCaseFull("ROLE-02-create-duplicate", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		mk := bson.D{{Key: "createRole", Value: role}, {Key: "privileges", Value: bson.A{}}, {Key: "roles", Value: bson.A{}}}
		tgt.Setup(runCmd(ctx, tgt.Admin, db, mk))
		return nil, runCmd(ctx, tgt.Admin, db, mk)
	}))

	// ROLE-03: inheriting a non-existent role is RoleNotFound (31).
	harness.AuthPairTest(t, authCaseFull("ROLE-03-create-missing-inherited", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{}},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "ghost_" + tgt.NS}, {Key: "db", Value: db}}}},
		})
	}))

	// ROLE-04: a non-admin-db role naming a cluster resource is BadValue (2).
	harness.AuthPairTest(t, authCaseFull("ROLE-04-nonadmin-cluster-resource", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{priv(bson.D{{Key: "cluster", Value: true}}, "shutdown")}},
			{Key: "roles", Value: bson.A{}},
		})
	}))

	// ROLE-05: an admin-db role may use the anyResource resource.
	harness.AuthPairTest(t, authCaseFull("ROLE-05-admin-anyResource", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		role := "r_" + tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, "admin", role) }()
		err := runCmd(ctx, tgt.Admin, "admin", bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{priv(bson.D{{Key: "anyResource", Value: true}}, "find")}},
			{Key: "roles", Value: bson.A{}},
		})
		return bson.M{"ok": err == nil}, err
	}))

	// ROLE-06: createRole stores authenticationRestrictions.
	harness.AuthPairTest(t, authCaseFull("ROLE-06-create-authRestrictions", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		err := runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{}},
			{Key: "roles", Value: bson.A{}},
			{Key: "authenticationRestrictions", Value: bson.A{bson.D{{Key: "clientSource", Value: bson.A{"127.0.0.1"}}}}},
		})
		return bson.M{"ok": err == nil}, err
	}))
}

func TestAuthRoleUpdateDrop(t *testing.T) {
	// ROLE-07: updateRole replaces privileges wholesale.
	harness.AuthPairTest(t, authCaseFull("ROLE-07-update-privileges-replace", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}, nil))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "insert")}}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		privs, _ := r0["privileges"].(bson.A)
		return bson.M{"privCount": len(privs)}, nil
	}))

	// ROLE-09: updateRole on a missing role is RoleNotFound (31).
	harness.AuthPairTest(t, authCaseFull("ROLE-09-update-missing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateRole", Value: "ghost_" + tgt.NS}, {Key: "roles", Value: bson.A{}}})
	}))

	// ROLE-10: dropRole removes an existing user-defined role.
	harness.AuthPairTest(t, authCaseFull("ROLE-10-drop-existing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, nil, nil))
		err := harness.DropRole(ctx, tgt.Admin, db, role)
		return bson.M{"ok": err == nil}, err
	}))

	// ROLE-11: dropRole on a missing role is RoleNotFound (31).
	harness.AuthPairTest(t, authCaseFull("ROLE-11-drop-missing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		return nil, harness.DropRole(ctx, tgt.Admin, db, "ghost_"+tgt.NS)
	}))

	// ROLE-12: dropping a built-in role fails (built-ins are not user-defined).
	harness.AuthPairTest(t, authCaseFull("ROLE-12-drop-builtin", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		return nil, harness.DropRole(ctx, tgt.Admin, db, "read")
	}))

	// ROLE-13: dropAllRolesFromDatabase returns the count removed.
	harness.AuthPairTest(t, authCaseFull("ROLE-13-drop-all", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_dropall_" + tgt.NS
		defer func() { _ = tgt.Admin.Database(db).Drop(ctx) }()
		for _, r := range []string{"a_" + tgt.NS, "b_" + tgt.NS} {
			tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, r, nil, nil))
		}
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "dropAllRolesFromDatabase", Value: 1}})
		if err != nil {
			return nil, err
		}
		return bson.M{"n": res["n"]}, nil
	}))
}

func TestAuthRoleGrantRevoke(t *testing.T) {
	// ROLE-14: grantPrivilegesToRole appends a new-resource privilege.
	// ROLE-15: granting an existing resource unions the actions.
	harness.AuthPairTest(t, authCaseFull("ROLE-14-15-grant-privileges", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, "c1"), Actions: []string{"find"}}}, nil))
		// New resource -> appended.
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "grantPrivilegesToRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, "c2"), "insert")}}}))
		// Existing resource -> action unioned.
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "grantPrivilegesToRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, "c1"), "insert")}}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		privs, _ := r0["privileges"].(bson.A)
		return bson.M{"privResourceCount": len(privs)}, nil
	}))

	// ROLE-16: revokePrivilegesFromRole removes an exactly-matching action.
	harness.AuthPairTest(t, authCaseFull("ROLE-16-revoke-privilege-exact", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find", "insert"}}}, nil))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokePrivilegesFromRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "insert")}}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		privs, _ := r0["privileges"].(bson.A)
		p0, _ := privs[0].(bson.M)
		acts, _ := p0["actions"].(bson.A)
		return bson.M{"actionCount": len(acts)}, nil
	}))

	// ROLE-18: revoking all actions of a privilege drops the privilege entry.
	harness.AuthPairTest(t, authCaseFull("ROLE-18-revoke-privilege-empties", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}, nil))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokePrivilegesFromRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, ""), "find")}}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		privs, _ := r0["privileges"].(bson.A)
		return bson.M{"privCount": len(privs)}, nil
	}))

	// ROLE-19: grantRolesToRole adds an inherited role.
	// ROLE-20: revokeRolesFromRole removes it.
	harness.AuthPairTest(t, authCaseFull("ROLE-19-20-grant-revoke-roles", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		base, derived := "base_"+tgt.NS, "derived_"+tgt.NS
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, derived)
			_ = harness.DropRole(ctx, tgt.Admin, db, base)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, base, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}, nil))
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, derived, nil, nil))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "grantRolesToRole", Value: derived}, {Key: "roles", Value: bson.A{base}}}))
		afterGrant, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: derived}})
		if err != nil {
			return nil, err
		}
		rg, _ := afterGrant["roles"].(bson.A)
		g0, _ := rg[0].(bson.M)
		inheritedAfterGrant, _ := g0["roles"].(bson.A)
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokeRolesFromRole", Value: derived}, {Key: "roles", Value: bson.A{base}}}))
		afterRevoke, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: derived}})
		if err != nil {
			return nil, err
		}
		rr, _ := afterRevoke["roles"].(bson.A)
		r0, _ := rr[0].(bson.M)
		inheritedAfterRevoke, _ := r0["roles"].(bson.A)
		return bson.M{"inheritedAfterGrant": len(inheritedAfterGrant), "inheritedAfterRevoke": len(inheritedAfterRevoke)}, nil
	}))
}

func TestAuthRolesInfo(t *testing.T) {
	// ROLE-21: rolesInfo on a user-defined role reports isBuiltin:false.
	harness.AuthPairTest(t, authCaseFull("ROLE-21-rolesInfo-single", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, nil, nil))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		return bson.M{"count": len(roles), "isBuiltin": r0["isBuiltin"]}, nil
	}))

	// ROLE-22: rolesInfo:1 lists user-defined roles on the database.
	harness.AuthPairTest(t, authCaseFull("ROLE-22-rolesInfo-all", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_all_" + tgt.NS
		defer func() { _ = tgt.Admin.Database(db).Drop(ctx) }()
		for _, r := range []string{"a_" + tgt.NS, "b_" + tgt.NS} {
			tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, r, nil, nil))
		}
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, "a_"+tgt.NS)
			_ = harness.DropRole(ctx, tgt.Admin, db, "b_"+tgt.NS)
		}()
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: 1}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": rolesCount(res)}, nil
	}))

	// ROLE-23: rolesInfo:1 showBuiltinRoles includes built-ins (isBuiltin:true).
	harness.AuthPairTest(t, authCaseFull("ROLE-23-rolesInfo-showBuiltin", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: 1}, {Key: "showBuiltinRoles", Value: true}})
		if err != nil {
			return nil, err
		}
		return bson.M{"hasBuiltin": anyBuiltin(res)}, nil
	}))

	// ROLE-25: rolesInfo on a missing role returns an empty list (not an error).
	harness.AuthPairTest(t, authCaseFull("ROLE-25-rolesInfo-missing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: "ghost_" + tgt.NS}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": rolesCount(res)}, nil
	}))
}

func TestAuthRoleMore(t *testing.T) {
	// ROLE-08: updateRole replaces the inherited-roles array wholesale.
	harness.AuthPairTest(t, authCaseFull("ROLE-08-update-roles-replace", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		base, other, role := "base_"+tgt.NS, "other_"+tgt.NS, "r_"+tgt.NS
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, role)
			_ = harness.DropRole(ctx, tgt.Admin, db, base)
			_ = harness.DropRole(ctx, tgt.Admin, db, other)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		for _, r := range []string{base, other} {
			tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, r, nil, nil))
		}
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, nil, []harness.RoleRef{{Role: base, DB: db}}))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateRole", Value: role}, {Key: "roles", Value: bson.A{other}}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		inherited, _ := r0["roles"].(bson.A)
		var name interface{}
		if len(inherited) > 0 {
			m, _ := inherited[0].(bson.M)
			name = m["role"]
		}
		return bson.M{"inheritedCount": len(inherited), "inheritedRole": name}, nil
	}))

	// ROLE-17: revokePrivilegesFromRole with a non-matching resource is a no-op.
	harness.AuthPairTest(t, authCaseFull("ROLE-17-revoke-privilege-nonexact-noop", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "roleg_"+tgt.NS, "r_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find", "insert"}}}, nil))
		// Different resource (specific collection) than the stored db-wide grant.
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "revokePrivilegesFromRole", Value: role}, {Key: "privileges", Value: bson.A{priv(collResource(db, "c1"), "find")}}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		privs, _ := r0["privileges"].(bson.A)
		p0, _ := privs[0].(bson.M)
		acts, _ := p0["actions"].(bson.A)
		return bson.M{"privCount": len(privs), "actionCount": len(acts)}, nil
	}))

	// ROLE-24: rolesInfo showPrivileges reports privileges and inheritedPrivileges.
	harness.AuthPairTest(t, authCaseFull("ROLE-24-rolesInfo-showPrivileges", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "roleg_" + tgt.NS
		base, role := "base_"+tgt.NS, "r_"+tgt.NS
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, role)
			_ = harness.DropRole(ctx, tgt.Admin, db, base)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, base, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"insert"}}}, nil))
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}, []harness.RoleRef{{Role: base, DB: db}}))
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: role}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		_, hasPrivs := r0["privileges"]
		_, hasInherited := r0["inheritedPrivileges"]
		return bson.M{"hasPrivileges": hasPrivs, "hasInheritedPrivileges": hasInherited}, nil
	}))
}
