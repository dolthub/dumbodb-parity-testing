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

// Auth parity area I3: role inheritance and effective-privilege closure
// (INH-01..10).

// findPriv is a single find privilege on all collections in db.
func findPriv(db string) []harness.Privilege {
	return []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}
}

// inhCase is a self-contained inheritance test: setup builds roles on db and
// returns the role to grant the user; op is run as that user.
type inhCase struct {
	id          string
	wantAllowed bool
	build       func(ctx context.Context, tgt harness.AuthTarget, db string) (topRole string, cleanup func(), err error)
	op          rbacOp
}

func runInhCase(t *testing.T, tc inhCase) {
	harness.AuthPairTest(t, authCase(tc.id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "inh_" + tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		if _, err := tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}); err != nil {
			return nil, err
		}
		topRole, cleanup, err := tc.build(ctx, tgt, db)
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			if cleanup != nil {
				cleanup()
			}
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err != nil {
			return nil, err
		}
		if err := harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: topRole, DB: db}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		opErr := tc.op.fn(ctx, c, db)
		allowed := opErr == nil
		code, _, _ := harness.CommandErrorCode(opErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() && allowed != tc.wantAllowed {
			t.Errorf("%s: MongoDB allowed=%v (code=%d), want %v", tc.id, allowed, code, tc.wantAllowed)
		}
		return bson.M{"allowed": allowed, "code": code}, nil
	}))
}

func mkRole(ctx context.Context, tgt harness.AuthTarget, db, name string, privs []harness.Privilege, inherits []string) error {
	refs := make([]harness.RoleRef, 0, len(inherits))
	for _, r := range inherits {
		refs = append(refs, harness.RoleRef{Role: r, DB: db})
	}
	return harness.CreateRole(ctx, tgt.Admin, db, name, privs, refs)
}

func TestAuthInheritance(t *testing.T) {
	// INH-01: A inherits B (which has find); A's user can find.
	runInhCase(t, inhCase{id: "INH-01-single-level", wantAllowed: true, op: opFind,
		build: func(ctx context.Context, tgt harness.AuthTarget, db string) (string, func(), error) {
			ns := db
			b, a := "b_"+ns, "a_"+ns
			if err := mkRole(ctx, tgt, db, b, findPriv(db), nil); err != nil {
				return "", nil, err
			}
			err := mkRole(ctx, tgt, db, a, nil, []string{b})
			return a, func() { _ = harness.DropRole(ctx, tgt.Admin, db, a); _ = harness.DropRole(ctx, tgt.Admin, db, b) }, err
		}})

	// INH-02: A -> B -> C transitive closure.
	runInhCase(t, inhCase{id: "INH-02-transitive", wantAllowed: true, op: opFind,
		build: func(ctx context.Context, tgt harness.AuthTarget, db string) (string, func(), error) {
			c, b, a := "c_"+db, "b_"+db, "a_"+db
			if err := mkRole(ctx, tgt, db, c, findPriv(db), nil); err != nil {
				return "", nil, err
			}
			if err := mkRole(ctx, tgt, db, b, nil, []string{c}); err != nil {
				return "", nil, err
			}
			err := mkRole(ctx, tgt, db, a, nil, []string{b})
			return a, func() {
				_ = harness.DropRole(ctx, tgt.Admin, db, a)
				_ = harness.DropRole(ctx, tgt.Admin, db, b)
				_ = harness.DropRole(ctx, tgt.Admin, db, c)
			}, err
		}})

	// INH-03: diamond A -> {B,C} -> D; privileges deduped, still allowed.
	runInhCase(t, inhCase{id: "INH-03-diamond", wantAllowed: true, op: opFind,
		build: func(ctx context.Context, tgt harness.AuthTarget, db string) (string, func(), error) {
			d, b, c, a := "d_"+db, "b_"+db, "c_"+db, "a_"+db
			if err := mkRole(ctx, tgt, db, d, findPriv(db), nil); err != nil {
				return "", nil, err
			}
			if err := mkRole(ctx, tgt, db, b, nil, []string{d}); err != nil {
				return "", nil, err
			}
			if err := mkRole(ctx, tgt, db, c, nil, []string{d}); err != nil {
				return "", nil, err
			}
			err := mkRole(ctx, tgt, db, a, nil, []string{b, c})
			return a, func() {
				for _, r := range []string{a, b, c, d} {
					_ = harness.DropRole(ctx, tgt.Admin, db, r)
				}
			}, err
		}})

	// INH-05: a custom role inheriting the built-in read role gets find.
	runInhCase(t, inhCase{id: "INH-05-inherit-builtin", wantAllowed: true, op: opFind,
		build: func(ctx context.Context, tgt harness.AuthTarget, db string) (string, func(), error) {
			a := "a_" + db
			err := mkRole(ctx, tgt, db, a, nil, []string{"read"})
			return a, func() { _ = harness.DropRole(ctx, tgt.Admin, db, a) }, err
		}})

	// INH-07: two roles on one user union their privileges (find + insert).
	runInhCase(t, inhCase{id: "INH-07-two-roles-union-find", wantAllowed: true, op: opFind,
		build: buildTwoRoleUnion})
	runInhCase(t, inhCase{id: "INH-07-two-roles-union-insert", wantAllowed: true, op: opInsert,
		build: buildTwoRoleUnion})

	// INH-08: overlapping grants on the same resource merge actions.
	runInhCase(t, inhCase{id: "INH-08-overlap-merge-find", wantAllowed: true, op: opFind,
		build: buildOverlap})
	runInhCase(t, inhCase{id: "INH-08-overlap-merge-insert", wantAllowed: true, op: opInsert,
		build: buildOverlap})
}

// buildTwoRoleUnion creates roleF (find) and roleI (insert) and a top role
// inheriting both, so the user has the union. Returns the top role.
func buildTwoRoleUnion(ctx context.Context, tgt harness.AuthTarget, db string) (string, func(), error) {
	rf, ri, a := "rf_"+db, "ri_"+db, "a_"+db
	if err := mkRole(ctx, tgt, db, rf, findPriv(db), nil); err != nil {
		return "", nil, err
	}
	if err := mkRole(ctx, tgt, db, ri, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"insert"}}}, nil); err != nil {
		return "", nil, err
	}
	err := mkRole(ctx, tgt, db, a, nil, []string{rf, ri})
	return a, func() {
		for _, r := range []string{a, rf, ri} {
			_ = harness.DropRole(ctx, tgt.Admin, db, r)
		}
	}, err
}

// buildOverlap creates two roles that both grant on the same resource but with
// different actions, inherited by a top role.
func buildOverlap(ctx context.Context, tgt harness.AuthTarget, db string) (string, func(), error) {
	r1, r2, a := "r1_"+db, "r2_"+db, "a_"+db
	if err := mkRole(ctx, tgt, db, r1, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}, nil); err != nil {
		return "", nil, err
	}
	if err := mkRole(ctx, tgt, db, r2, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"insert"}}}, nil); err != nil {
		return "", nil, err
	}
	err := mkRole(ctx, tgt, db, a, nil, []string{r1, r2})
	return a, func() {
		for _, r := range []string{a, r1, r2} {
			_ = harness.DropRole(ctx, tgt.Admin, db, r)
		}
	}, err
}

// TestAuthInheritanceMore covers the cross-db, cycle, and showPrivileges cases.
func TestAuthInheritanceMore(t *testing.T) {
	// INH-06: an admin-defined role inherits a role in another database.
	harness.AuthPairTest(t, authCase("INH-06-cross-db-inheritance", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		xdb := "inhx_" + tgt.NS
		base, top, user, pwd := "base_"+tgt.NS, "top_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, "admin", user)
			_ = harness.DropRole(ctx, tgt.Admin, "admin", top)
			_ = harness.DropRole(ctx, tgt.Admin, xdb, base)
			_ = tgt.Admin.Database(xdb).Drop(ctx)
		}()
		if _, err := tgt.Admin.Database(xdb).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, xdb, base, findPriv(xdb), nil); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, "admin", top, nil, []harness.RoleRef{{Role: base, DB: xdb}}); err != nil {
			return nil, err
		}
		if err := harness.CreateUser(ctx, tgt.Admin, "admin", user, pwd, []harness.RoleRef{{Role: top, DB: "admin"}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, "admin")
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		err = cmdErr(ctx, c, xdb, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}})
		if tgt.BaseURI == harness.AuthMongoBaseURI() && err != nil {
			t.Errorf("INH-06: admin role inheriting cross-db role should allow find, got %v", err)
		}
		return bson.M{"allowed": err == nil}, nil
	}))

	// INH-04: a role inheritance cycle is handled (MongoDB rejects creating it).
	// We capture the outcome; the point is the server must not hang.
	harness.AuthPairTest(t, authCase("INH-04-cycle", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "inhcyc_" + tgt.NS
		a, b := "a_"+tgt.NS, "b_"+tgt.NS
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, a)
			_ = harness.DropRole(ctx, tgt.Admin, db, b)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err := harness.CreateRole(ctx, tgt.Admin, db, a, nil, nil); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, db, b, nil, []harness.RoleRef{{Role: a, DB: db}}); err != nil {
			return nil, err
		}
		// Close the cycle: a -> b (b already -> a).
		cycleErr := runCmd(ctx, tgt.Admin, db, bson.D{{Key: "grantRolesToRole", Value: a}, {Key: "roles", Value: bson.A{b}}})
		return bson.M{"cycleRejected": cycleErr != nil}, cycleErr
	}))

	// INH-09: rolesInfo showPrivileges reports inheritedPrivileges (closure).
	harness.AuthPairTest(t, authCase("INH-09-rolesInfo-inheritedPrivileges", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "inhri_" + tgt.NS
		base, top := "base_"+tgt.NS, "top_"+tgt.NS
		defer func() {
			_ = harness.DropRole(ctx, tgt.Admin, db, top)
			_ = harness.DropRole(ctx, tgt.Admin, db, base)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err := harness.CreateRole(ctx, tgt.Admin, db, base, findPriv(db), nil); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, db, top, nil, []harness.RoleRef{{Role: base, DB: db}}); err != nil {
			return nil, err
		}
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "rolesInfo", Value: top}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		roles, _ := res["roles"].(bson.A)
		r0, _ := roles[0].(bson.M)
		inherited, _ := r0["inheritedPrivileges"].(bson.A)
		return bson.M{"inheritedPrivCount": len(inherited)}, nil
	}))

	// INH-10: usersInfo showPrivileges reports the flattened effective set.
	harness.AuthPairTest(t, authCase("INH-10-usersInfo-inheritedPrivileges", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "inhui_" + tgt.NS
		base, top, user := "base_"+tgt.NS, "top_"+tgt.NS, "u_"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropRole(ctx, tgt.Admin, db, top)
			_ = harness.DropRole(ctx, tgt.Admin, db, base)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err := harness.CreateRole(ctx, tgt.Admin, db, base, findPriv(db), nil); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, db, top, nil, []harness.RoleRef{{Role: base, DB: db}}); err != nil {
			return nil, err
		}
		if err := harness.CreateUser(ctx, tgt.Admin, db, user, "pw", []harness.RoleRef{{Role: top, DB: db}}); err != nil {
			return nil, err
		}
		res, err := decodeCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: user}, {Key: "showPrivileges", Value: true}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		u0, _ := users[0].(bson.M)
		inherited, _ := u0["inheritedPrivileges"].(bson.A)
		return bson.M{"inheritedPrivCount": len(inherited)}, nil
	}))
}
