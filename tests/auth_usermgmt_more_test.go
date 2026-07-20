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

// Auth parity area F, remaining cases: cross-db roles, update replace-semantics,
// usersInfo query forms, and the forAllDBs-after-dropDatabase divergence.

// userPresent reports whether a usersInfo result contains a user named name.
func userPresent(res bson.M, name string) bool {
	users, _ := res["users"].(bson.A)
	for _, u := range users {
		m, _ := u.(bson.M)
		if m["user"] == name {
			return true
		}
	}
	return false
}

func TestAuthUserCreateMore(t *testing.T) {
	// USER-03: a {role, db} grant to another database is stored as given.
	harness.AuthPairTest(t, authCaseFull("USER-03-create-role-crossdb", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, other, u := "userf_"+tgt.NS, "userfother_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", []harness.RoleRef{{Role: "read", DB: other}}))
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
}

func TestAuthUserUpdateMore(t *testing.T) {
	// USER-12: updateUser replaces customData entirely.
	harness.AuthPairTest(t, authCase("USER-12-update-customData-replace", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		if err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}, {Key: "customData", Value: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 2}}}}); err != nil {
			return nil, err
		}
		if err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateUser", Value: u}, {Key: "customData", Value: bson.D{{Key: "c", Value: 3}}}}); err != nil {
			return nil, err
		}
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		cd, _ := first["customData"].(bson.M)
		_, hasA := cd["a"]
		_, hasC := cd["c"]
		return bson.M{"keyCount": len(cd), "hasA": hasA, "hasC": hasC}, nil
	}))

	// USER-14: updateUser may narrow mechanisms (to a subset) without a new pwd.
	harness.AuthPairTest(t, authCase("USER-14-update-narrow-mechanisms", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{{Key: "createUser", Value: u}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}, {Key: "mechanisms", Value: bson.A{"SCRAM-SHA-1", "SCRAM-SHA-256"}}}))
		if err := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "updateUser", Value: u}, {Key: "mechanisms", Value: bson.A{"SCRAM-SHA-256"}}}); err != nil {
			return nil, err
		}
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

func TestAuthUsersInfoForms(t *testing.T) {
	// USER-23: usersInfo:"name" returns that single user.
	harness.AuthPairTest(t, authCaseFull("USER-23-usersInfo-string", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": usersCount(res), "present": userPresent(res, u)}, nil
	}))

	// USER-24: usersInfo:{user,db} returns that specific user@db.
	harness.AuthPairTest(t, authCaseFull("USER-24-usersInfo-user-db-doc", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: bson.D{{Key: "user", Value: u}, {Key: "db", Value: db}}}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": usersCount(res), "present": userPresent(res, u)}, nil
	}))

	// USER-25: usersInfo:[...] returns multiple named users.
	harness.AuthPairTest(t, authCaseFull("USER-25-usersInfo-array", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "userf_arr_" + tgt.NS
		a, b := "a_"+tgt.NS, "b_"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, a)
			_ = harness.DropUser(ctx, tgt.Admin, db, b)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		for _, n := range []string{a, b} {
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, n, "pw", nil))
		}
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: bson.A{a, b}}})
		if err != nil {
			return nil, err
		}
		return bson.M{"count": usersCount(res)}, nil
	}))

	// USER-26: usersInfo:{forAllDBs:true} lists users across databases.
	harness.AuthPairTest(t, authCaseFull("USER-26-usersInfo-forAllDBs", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_fad_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil))
		res, err := usersInfoCmd(ctx, tgt.Admin, "admin", bson.D{{Key: "usersInfo", Value: bson.D{{Key: "forAllDBs", Value: true}}}})
		if err != nil {
			return nil, err
		}
		return bson.M{"present": userPresent(res, u)}, nil
	}))

	// USER-31: after dropDatabase, MongoDB still lists that db's users under
	// forAllDBs (users live in admin, independent of the db's data). This is a
	// deliberate DumboDB divergence (per-db credential storage) and is expected
	// to remain XFail.
	harness.AuthPairTest(t, authCaseFull("USER-31-users-survive-dropDatabase", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "userf_drop_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, u, "pw", []harness.RoleRef{{Role: "readWrite", DB: db}}))
		// Materialize the db with a collection, then drop it.
		tgt.Setup1(tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "_id", Value: 1}}))
		tgt.Setup(tgt.Admin.Database(db).Drop(ctx))
		res, err := usersInfoCmd(ctx, tgt.Admin, "admin", bson.D{{Key: "usersInfo", Value: bson.D{{Key: "forAllDBs", Value: true}}}})
		if err != nil {
			return nil, err
		}
		return bson.M{"userStillPresent": userPresent(res, u)}, nil
	}))
}
