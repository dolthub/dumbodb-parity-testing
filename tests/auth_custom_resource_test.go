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

// Auth parity area I1: custom-role resource-document matching (RES-01..18).

// findOn returns an op that runs find on database.collection.
func findOn(useOther bool, coll string) func(ctx context.Context, c *mongo.Client, db, other string) error {
	return func(ctx context.Context, c *mongo.Client, db, other string) error {
		target := db
		if useOther {
			target = other
		}
		return cmdErr(ctx, c, target, bson.D{{Key: "find", Value: coll}, {Key: "filter", Value: bson.D{}}})
	}
}

type resRow struct {
	id          string
	adminScoped bool
	wantAllowed bool
	privs       func(db, other string) []harness.Privilege
	op          func(ctx context.Context, c *mongo.Client, db, other string) error
}

// customResourceProbe grants a custom role carrying the given privileges (on the
// test database, or on admin when adminScoped) and runs one operation as the
// holder, validating the outcome against MongoDB.
func customResourceProbe(t *testing.T, r resRow) harness.AuthCase {
	return authCase(r.id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "resm_" + tgt.NS
		other := "resmo_" + tgt.NS
		roleDB := db
		if r.adminScoped {
			roleDB = "admin"
		}
		role, user, pwd := "role_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, roleDB, user)
			_ = harness.DropRole(ctx, tgt.Admin, roleDB, role)
			_ = tgt.Admin.Database(db).Drop(ctx)
			_ = tgt.Admin.Database(other).Drop(ctx)
		}()
		// Seed collections the ops read.
		for _, s := range []struct{ d, coll string }{{db, "c1"}, {db, "c2"}, {db, "logs"}, {db, "events"}, {other, "c1"}, {other, "logs"}} {
			if _, err := tgt.Admin.Database(s.d).Collection(s.coll).InsertOne(ctx, bson.D{{Key: "x", Value: 1}}); err != nil {
				return nil, err
			}
		}
		if err := harness.CreateRole(ctx, tgt.Admin, roleDB, role, r.privs(db, other), nil); err != nil {
			return nil, err
		}
		if err := harness.CreateUser(ctx, tgt.Admin, roleDB, user, pwd, []harness.RoleRef{{Role: role, DB: roleDB}}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, roleDB)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()

		opErr := r.op(ctx, c, db, other)
		allowed := opErr == nil
		code, _, _ := harness.CommandErrorCode(opErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if allowed != r.wantAllowed {
				t.Errorf("%s: MongoDB allowed=%v (code=%d), want allowed=%v", r.id, allowed, code, r.wantAllowed)
			}
			if !allowed && code != 13 {
				t.Errorf("%s: MongoDB denial code=%d, want Unauthorized(13)", r.id, code)
			}
		}
		return bson.M{"allowed": allowed, "code": code}, nil
	})
}

func pColl(coll string) func(db, other string) []harness.Privilege {
	return func(db, other string) []harness.Privilege {
		return []harness.Privilege{{Resource: collResource(db, coll), Actions: []string{"find"}}}
	}
}

func TestAuthCustomResourceMatching(t *testing.T) {
	rows := []resRow{
		{"RES-01-exact-coll-allow", false, true, pColl("c1"), findOn(false, "c1")},
		{"RES-02-exact-coll-other-coll-deny", false, false, pColl("c1"), findOn(false, "c2")},
		{"RES-03-exact-coll-other-db-deny", false, false, pColl("c1"), findOn(true, "c1")},
		{"RES-04-db-wide-allow", false, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}
			}, findOn(false, "c2")},
		{"RES-05-db-wide-excludes-system.js", false, false,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}
			}, findOn(false, "system.js")},
		{"RES-07-explicit-system.js-allow", false, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: collResource(db, "system.js"), Actions: []string{"find"}}}
			}, findOn(false, "system.js")},
		{"RES-08-db-wide-other-db-deny", false, false,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{"find"}}}
			}, findOn(true, "c1")},
		{"RES-09-any-db-any-coll-allow", true, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "db", Value: ""}, {Key: "collection", Value: ""}}, Actions: []string{"find"}}}
			}, findOn(true, "c1")},
		{"RES-10-any-db-any-coll-excludes-system", true, false,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "db", Value: ""}, {Key: "collection", Value: ""}}, Actions: []string{"find"}}}
			}, findOn(false, "system.js")},
		{"RES-11-any-db-named-coll-allow", true, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "db", Value: ""}, {Key: "collection", Value: "logs"}}, Actions: []string{"find"}}}
			}, findOn(true, "logs")},
		{"RES-12-any-db-named-coll-other-coll-deny", true, false,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "db", Value: ""}, {Key: "collection", Value: "logs"}}, Actions: []string{"find"}}}
			}, findOn(false, "events")},
		{"RES-15-cluster-serverStatus-allow", true, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "cluster", Value: true}}, Actions: []string{"serverStatus"}}}
			}, func(ctx context.Context, c *mongo.Client, db, other string) error {
				return cmdErr(ctx, c, "admin", bson.D{{Key: "serverStatus", Value: 1}})
			}},
		{"RES-16-cluster-grant-collection-op-deny", true, false,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "cluster", Value: true}}, Actions: []string{"serverStatus"}}}
			}, findOn(false, "c1")},
		{"RES-17-anyResource-reads-system", true, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{{Resource: bson.D{{Key: "anyResource", Value: true}}, Actions: []string{"find"}}}
			}, findOn(false, "system.js")},
		{"RES-18-additive-broader-grant-wins", false, true,
			func(db, other string) []harness.Privilege {
				return []harness.Privilege{
					{Resource: collResource(db, "c1"), Actions: []string{"insert"}},
					{Resource: collResource(db, ""), Actions: []string{"find"}},
				}
			}, findOn(false, "c1")},
	}
	for _, r := range rows {
		harness.AuthPairTest(t, customResourceProbe(t, r))
	}
}

// RES-13 / RES-14: role-creation validation errors (BadValue) for illegal
// resources on a non-admin-database role.
func TestAuthCustomResourceCreateErrors(t *testing.T) {
	// RES-13: a non-admin role naming a cluster resource is rejected.
	harness.AuthPairTest(t, authCase("RES-13-nonadmin-cluster-resource", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, role := "resm_"+tgt.NS, "role_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{priv(bson.D{{Key: "cluster", Value: true}}, "serverStatus")}},
			{Key: "roles", Value: bson.A{}},
		})
	}))

	// RES-14: a non-admin role naming a resource in another database is rejected.
	harness.AuthPairTest(t, authCase("RES-14-nonadmin-crossdb-resource", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, other, role := "resm_"+tgt.NS, "resmo_"+tgt.NS, "role_"+tgt.NS
		defer func() { _ = harness.DropRole(ctx, tgt.Admin, db, role); _ = tgt.Admin.Database(db).Drop(ctx) }()
		return nil, runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createRole", Value: role},
			{Key: "privileges", Value: bson.A{priv(collResource(other, ""), "find")}},
			{Key: "roles", Value: bson.A{}},
		})
	}))
}
