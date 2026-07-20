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

func runCmd(ctx context.Context, admin *mongo.Client, db string, cmd bson.D) error {
	return admin.Database(db).RunCommand(ctx, cmd).Err()
}

func usersInfoCmd(ctx context.Context, admin *mongo.Client, db string, cmd bson.D) (bson.M, error) {
	var res bson.M
	err := admin.Database(db).RunCommand(ctx, cmd).Decode(&res)
	return res, err
}

// usersCount returns the number of entries in a usersInfo result's users array.
func usersCount(res bson.M) int {
	users, _ := res["users"].(bson.A)
	return len(users)
}

func authCase(name string, run func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error)) harness.AuthCase {
	return harness.AuthCase{Name: name, Support: harness.DumboDBXFail, Run: run}
}

func authCaseFull(name string, run func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error)) harness.AuthCase {
	return harness.AuthCase{Name: name, Support: harness.DumboDBFull, Run: run}
}

func TestAuthUserCreate(t *testing.T) {
	// USER-01: minimal createUser succeeds.
	harness.AuthPairTest(t, authCaseFull("USER-01-create-minimal", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}})
		return bson.M{"ok": err == nil}, err
	}))

	// USER-02: roles shorthand string resolves to {role, currentDb}.
	harness.AuthPairTest(t, authCaseFull("USER-02-create-role-string-form", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{"readWrite"}}}))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		roles, _ := first["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		return bson.M{"role": r0["role"], "db": r0["db"]}, nil
	}))

	// USER-04: duplicate createUser is DuplicateKey (11000).
	harness.AuthPairTest(t, authCaseFull("USER-04-create-duplicate", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		mk := bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}}
		tgt.Setup(runCmd(ctx, tgt.Admin, db, mk))
		return nil, runCmd(ctx, tgt.Admin, db, mk)
	}))

	// USER-05: createUser referencing a non-existent role is RoleNotFound (31).
	harness.AuthPairTest(t, authCase("USER-05-create-missing-role", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{"nosuchrole_" + tgt.NS}}})
	}))

	// USER-06: explicit mechanisms yields only that credential (visible via usersInfo).
	harness.AuthPairTest(t, authCaseFull("USER-06-create-explicit-mechanisms", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}, {Key: "mechanisms", Value: bson.A{"SCRAM-SHA-256"}}}))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		mechs, _ := first["mechanisms"].(bson.A)
		return bson.M{"mechanisms": mechs}, nil
	}))

	// USER-07: customData is stored and returned by usersInfo.
	harness.AuthPairTest(t, authCase("USER-07-create-customData", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		if err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}, {Key: "customData", Value: bson.D{{Key: "team", Value: "eng"}}}}); err != nil {
			return nil, err
		}
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		cd, _ := first["customData"].(bson.M)
		return bson.M{"team": cd["team"]}, nil
	}))

	// USER-08: createUser without pwd (non-$external) is rejected.
	harness.AuthPairTest(t, authCaseFull("USER-08-create-no-pwd", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "roles", Value: bson.A{}}})
	}))

	// USER-09: createUser on the reserved local database is rejected.
	harness.AuthPairTest(t, authCase("USER-09-create-on-local", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		u := "u_" + tgt.NS
		defer func() { _ = harness.DropUser(ctx, tgt.Admin, "local", u) }()
		return nil, runCmd(ctx, tgt.Admin, "local", bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}})
	}))

	// USER-32: createUser on a database with no collections succeeds.
	harness.AuthPairTest(t, authCaseFull("USER-32-create-on-empty-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_empty_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}})
		return bson.M{"ok": err == nil}, err
	}))
}

func TestAuthUserUpdate(t *testing.T) {
	// USER-10: updateUser changes the password (old fails, new works).
	harness.AuthPairTest(t, authCaseFull("USER-10-update-password", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "old", []harness.RoleRef{{Role: "readWrite", DB: db}}))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateUser", Value: u}, {Key: "pwd", Value: "new"}}))
		oldC, oldErr := harness.ConnectAs(ctx, tgt.BaseURI, u, "old", db)
		if oldErr == nil {
			_ = oldC.Disconnect(ctx)
		}
		newC, newErr := harness.ConnectAs(ctx, tgt.BaseURI, u, "new", db)
		if newErr == nil {
			_ = newC.Disconnect(ctx)
		}
		return bson.M{"oldRejected": oldErr != nil, "newAccepted": newErr == nil}, nil
	}))

	// USER-11: updateUser replaces the roles array wholesale.
	harness.AuthPairTest(t, authCaseFull("USER-11-update-roles-replace", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", []harness.RoleRef{{Role: "readWrite", DB: db}}))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateUser", Value: u}, {Key: "roles", Value: bson.A{"read"}}}))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		roles, _ := first["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		return bson.M{"roleCount": len(roles), "role": r0["role"]}, nil
	}))

	// USER-13: updateUser on a missing user is UserNotFound (11).
	harness.AuthPairTest(t, authCaseFull("USER-13-update-missing-user", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_" + tgt.NS
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateUser", Value: "ghost_" + tgt.NS}, {Key: "pwd", Value: "x"}})
	}))
}

func TestAuthUserDrop(t *testing.T) {
	// USER-15: dropUser removes an existing user.
	harness.AuthPairTest(t, authCaseFull("USER-15-drop-existing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "dropUser", Value: u}})
		return bson.M{"ok": err == nil}, err
	}))

	// USER-16: dropUser on a missing user is UserNotFound (11).
	harness.AuthPairTest(t, authCaseFull("USER-16-drop-missing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_" + tgt.NS
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{{Key: "dropUser", Value: "ghost_" + tgt.NS}})
	}))

	// USER-17: dropAllUsersFromDatabase returns the count removed.
	harness.AuthPairTest(t, authCaseFull("USER-17-drop-all-nonzero", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_dropall_" + tgt.NS
		defer func() { _ = tgt.Admin.Database(db).Drop(ctx) }()
		for i, name := range []string{"a_" + tgt.NS, "b_" + tgt.NS} {
			_ = i
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, name, "pw", nil))
		}
		var res bson.M
		tgt.Setup(tgt.Admin.Database(db).RunCommand(ctx, bson.D{{Key: "dropAllUsersFromDatabase", Value: 1}}).Decode(&res))
		return bson.M{"n": res["n"]}, nil
	}))

	// USER-18: dropAllUsersFromDatabase with no users returns n:0 (no error).
	harness.AuthPairTest(t, authCaseFull("USER-18-drop-all-zero", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_dropall0_" + tgt.NS
		var res bson.M
		tgt.Setup(tgt.Admin.Database(db).RunCommand(ctx, bson.D{{Key: "dropAllUsersFromDatabase", Value: 1}}).Decode(&res))
		return bson.M{"n": res["n"]}, nil
	}))
}

func TestAuthUserGrantRevoke(t *testing.T) {
	// USER-19: grantRolesToUser unions roles.
	harness.AuthPairTest(t, authCase("USER-19-grant-roles-union", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", []harness.RoleRef{{Role: "read", DB: db}}))
		tgt.Setup(harness.GrantRolesToUser(ctx, tgt.Admin, db, u, []harness.RoleRef{{Role: "dbAdmin", DB: db}}))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		roles, _ := first["roles"].(bson.A)
		return bson.M{"roleCount": len(roles)}, nil
	}))

	// USER-20: grantRolesToUser with a missing role is RoleNotFound (31).
	harness.AuthPairTest(t, authCase("USER-20-grant-missing-role", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		return nil, harness.GrantRolesToUser(ctx, tgt.Admin, db, u, []harness.RoleRef{{Role: "nosuch_" + tgt.NS, DB: db}})
	}))

	// USER-21: revokeRolesFromUser removes a role.
	harness.AuthPairTest(t, authCase("USER-21-revoke-role", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", []harness.RoleRef{{Role: "read", DB: db}, {Role: "dbAdmin", DB: db}}))
		tgt.Setup(harness.RevokeRolesFromUser(ctx, tgt.Admin, db, u, []harness.RoleRef{{Role: "dbAdmin", DB: db}}))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		roles, _ := first["roles"].(bson.A)
		return bson.M{"roleCount": len(roles)}, nil
	}))
}

func TestAuthUsersInfo(t *testing.T) {
	// USER-27: usersInfo for a missing user returns an empty list (not an error).
	harness.AuthPairTest(t, authCaseFull("USER-27-usersInfo-missing", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_" + tgt.NS
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: "ghost_" + tgt.NS}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": usersCount(res)}, nil
	}))

	// USER-22: usersInfo:1 lists all users on the database.
	harness.AuthPairTest(t, authCaseFull("USER-22-usersInfo-all", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_all_" + tgt.NS
		defer func() { _ = tgt.Admin.Database(db).Drop(ctx) }()
		for _, name := range []string{"a_" + tgt.NS, "b_" + tgt.NS} {
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, name, "pw", nil))
		}
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, "a_"+tgt.NS)
			_ = harness.DropUser(ctx, tgt.Admin, db, "b_"+tgt.NS)
		}()
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: 1}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": usersCount(res)}, nil
	}))

	// USER-28 / USER-29: showCredentials toggles the credentials field.
	harness.AuthPairTest(t, authCaseFull("USER-28-usersInfo-showCredentials", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		with, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}, {Key: "showCredentials", Value: true}})
		if err != nil {
			return nil, err
		}
		without, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		hasCred := func(res bson.M) bool {
			users, _ := res["users"].(bson.A)
			first, _ := users[0].(bson.M)
			_, ok := first["credentials"]
			return ok
		}
		return bson.M{"withCred": hasCred(with), "withoutCred": hasCred(without)}, nil
	}))

	// USER-30: usersInfo derives the mechanisms array from stored credentials.
	harness.AuthPairTest(t, authCaseFull("USER-30-usersInfo-mechanisms", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		mechs, _ := first["mechanisms"].(bson.A)
		return bson.M{"mechanismCount": len(mechs)}, nil
	}))
}
