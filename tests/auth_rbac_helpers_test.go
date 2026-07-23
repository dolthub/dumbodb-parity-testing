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

// Shared machinery for the built-in-role enforcement areas (H). Each probe
// creates a user with a built-in role, connects as that user, runs one
// operation, and reports {allowed, code}. Every case starts XFail: DumboDB
// cannot create the user (createUser is stubbed), so the operation-outcome on
// DumboDB diverges from MongoDB, which this suite pins to the correct
// allow/deny.

// rbacXFail lists the built-in-role enforcement cases that still diverge from
// MongoDB because the command they exercise is not yet implemented
// (getClusterParameter, logRotate) or is rejected before authorization runs
// (setParameter). Every other case is DumboDBFull: DumboDB's built-in-role
// privilege model matches MongoDB. These flip as the commands land.
var rbacXFail = map[string]struct{}{
	"RBAC-cmgr-01-getClusterParameter": {},
	"RBAC-hm-02-logRotate":             {},
	"RBAC-ca-02-setParameter":          {},
	"RBAC-hm-01-setParameter":          {},
	"RBAC-root-11-setParameter":        {},
}

func rbacSupport(id string) harness.DumboDBSupport {
	if _, ok := rbacXFail[id]; ok {
		return harness.DumboDBXFail
	}
	return harness.DumboDBFull
}

// rbacOp is a single operation attempted as the role-holder. fn runs a command
// against the given database and returns the driver error (nil = allowed).
type rbacOp struct {
	name      string
	fn        func(ctx context.Context, c *mongo.Client, db string) error
	onOtherDB bool // run against a different database than the role grant
}

func cmdErr(ctx context.Context, c *mongo.Client, db string, cmd bson.D) error {
	return c.Database(db).RunCommand(ctx, cmd).Err()
}

// Operation library (commands, so that "allowed" is distinguishable from an
// empty result and errors carry MongoDB codes).
var (
	opFind             = rbacOp{name: "find", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}}) }}
	opCount            = rbacOp{name: "count", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "count", Value: "c"}}) }}
	opDistinct         = rbacOp{name: "distinct", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "distinct", Value: "c"}, {Key: "key", Value: "x"}}) }}
	opAggregateRead    = rbacOp{name: "aggregate-read", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "aggregate", Value: "c"}, {Key: "pipeline", Value: bson.A{}}, {Key: "cursor", Value: bson.D{}}}) }}
	opInsert           = rbacOp{name: "insert", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "insert", Value: "c"}, {Key: "documents", Value: bson.A{bson.D{{Key: "y", Value: 1}}}}}) }}
	opUpdate           = rbacOp{name: "update", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "update", Value: "c"}, {Key: "updates", Value: bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: 2}}}}}}}}}) }}
	opDelete           = rbacOp{name: "delete", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "delete", Value: "c"}, {Key: "deletes", Value: bson.A{bson.D{{Key: "q", Value: bson.D{}}, {Key: "limit", Value: 1}}}}}) }}
	opCreateCollection = rbacOp{name: "createCollection", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "create", Value: "c2"}}) }}
	opCreateIndexes    = rbacOp{name: "createIndexes", fn: func(ctx context.Context, c *mongo.Client, db string) error {
		return cmdErr(ctx, c, db, bson.D{{Key: "createIndexes", Value: "c"}, {Key: "indexes", Value: bson.A{bson.D{{Key: "key", Value: bson.D{{Key: "x", Value: 1}}}, {Key: "name", Value: "x_1"}}}}})
	}}
	opDropCollection = rbacOp{name: "dropCollection", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "drop", Value: "c"}}) }}
	opDropIndexes    = rbacOp{name: "dropIndexes", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "dropIndexes", Value: "c"}, {Key: "index", Value: "*"}}) }}
	opCollMod        = rbacOp{name: "collMod", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "collMod", Value: "c"}}) }}
	opValidate       = rbacOp{name: "validate", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "validate", Value: "c"}}) }}
	opCollStats      = rbacOp{name: "collStats", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "collStats", Value: "c"}}) }}
	opDbStats        = rbacOp{name: "dbStats", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "dbStats", Value: 1}}) }}
	opListColls      = rbacOp{name: "listCollections", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "listCollections", Value: 1}}) }}
	opListIndexes    = rbacOp{name: "listIndexes", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "listIndexes", Value: "c"}}) }}
	opDropDatabase   = rbacOp{name: "dropDatabase", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "dropDatabase", Value: 1}}) }}
	opCreateUser     = rbacOp{name: "createUser", fn: func(ctx context.Context, c *mongo.Client, db string) error {
		return cmdErr(ctx, c, db, bson.D{{Key: "createUser", Value: "probeuser"}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}})
	}}
	opUsersInfo      = rbacOp{name: "usersInfo", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "usersInfo", Value: 1}}) }}
	opRolesInfo      = rbacOp{name: "rolesInfo", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "rolesInfo", Value: 1}}) }}
	opCreateRole     = rbacOp{name: "createRole", fn: func(ctx context.Context, c *mongo.Client, db string) error {
		return cmdErr(ctx, c, db, bson.D{{Key: "createRole", Value: "proberole"}, {Key: "privileges", Value: bson.A{}}, {Key: "roles", Value: bson.A{}}})
	}}
	opFindOtherDB    = rbacOp{name: "find-other-db", onOtherDB: true, fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "find", Value: "c"}, {Key: "filter", Value: bson.D{}}}) }}
	opInsertOtherDB  = rbacOp{name: "insert-other-db", onOtherDB: true, fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, db, bson.D{{Key: "insert", Value: "c"}, {Key: "documents", Value: bson.A{bson.D{{Key: "y", Value: 1}}}}}) }}
	opServerStatus   = rbacOp{name: "serverStatus", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "serverStatus", Value: 1}}) }}
	opListDatabases  = rbacOp{name: "listDatabases", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "listDatabases", Value: 1}}) }}
)

// dbScopedRoleProbe builds an AuthCase that grants a database-scoped built-in
// role and runs one operation as the role-holder. In addition to the standard
// MongoDB-vs-DumboDB comparison, it asserts (on the MongoDB run) that the
// outcome matches wantAllowed and that denials use Unauthorized(13), so a wrong
// expectation in the table fails loudly against real MongoDB.
func dbScopedRoleProbe(t *testing.T, id, role string, wantAllowed bool, op rbacOp, support harness.DumboDBSupport) harness.AuthCase {
	c := authCase(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "rbac_" + tgt.NS
		other := "rbacother_" + tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropUser(ctx, tgt.Admin, db, "probeuser")
			_ = harness.DropRole(ctx, tgt.Admin, db, "proberole")
			_ = tgt.Admin.Database(db).Drop(ctx)
			_ = tgt.Admin.Database(other).Drop(ctx)
		}()
		// Seed both databases with a document so read-style operations have
		// something to act on.
		tgt.Setup1(tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}))
		if op.onOtherDB {
			tgt.Setup1(tgt.Admin.Database(other).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}))
		}
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: role, DB: db}}))
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()

		target := db
		if op.onOtherDB {
			target = other
		}
		opErr := op.fn(ctx, c, target)
		allowed := opErr == nil
		code, _, _ := harness.CommandErrorCode(opErr)

		// Validate the expectation against real MongoDB (the DumboDB run is
		// compared separately by AuthPairTest).
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if allowed != wantAllowed {
				t.Errorf("%s [%s]: MongoDB allowed=%v (code=%d), want allowed=%v", id, op.name, allowed, code, wantAllowed)
			}
			if !allowed && code != 13 {
				t.Errorf("%s [%s]: MongoDB denial code=%d, want Unauthorized(13)", id, op.name, code)
			}
		}
		return bson.M{"allowed": allowed, "code": code}, nil
	})
	c.Support = support
	return c
}

// Additional operations for admin-scoped (AnyDatabase, cluster, backup/restore,
// root) role enforcement.
var (
	opGetParameter        = rbacOp{name: "getParameter", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "getParameter", Value: "*"}}) }}
	opSetParameter        = rbacOp{name: "setParameter", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "setParameter", Value: 1}, {Key: "logLevel", Value: 0}}) }}
	opHostInfo            = rbacOp{name: "hostInfo", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "hostInfo", Value: 1}}) }}
	opLogRotate           = rbacOp{name: "logRotate", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "logRotate", Value: 1}}) }}
	opGetClusterParameter = rbacOp{name: "getClusterParameter", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "getClusterParameter", Value: "*"}}) }}
)

var (
	opPing             = rbacOp{name: "ping", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "ping", Value: 1}}) }}
	opConnectionStatus = rbacOp{name: "connectionStatus", fn: func(ctx context.Context, c *mongo.Client, db string) error { return cmdErr(ctx, c, "admin", bson.D{{Key: "connectionStatus", Value: 1}}) }}
)

// noRoleProbe creates an authenticated user with NO roles and runs one
// operation, validating the outcome against MongoDB. Denials for a role-less
// user are Unauthorized(13); anonymous commands (ping/connectionStatus) are
// still allowed.
func noRoleProbe(t *testing.T, id string, wantAllowed bool, op rbacOp, support harness.DumboDBSupport) harness.AuthCase {
	c := authCase(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "rbacnone_" + tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, "admin", user)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup1(tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}))
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, "admin", user, pwd, nil))
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, "admin")
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		opErr := op.fn(ctx, c, db)
		allowed := opErr == nil
		code, _, _ := harness.CommandErrorCode(opErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if allowed != wantAllowed {
				t.Errorf("%s [%s]: MongoDB allowed=%v (code=%d), want allowed=%v", id, op.name, allowed, code, wantAllowed)
			}
			if !allowed && code != 13 {
				t.Errorf("%s [%s]: MongoDB denial code=%d, want Unauthorized(13)", id, op.name, code)
			}
		}
		return bson.M{"allowed": allowed, "code": code}, nil
	})
	c.Support = support
	return c
}

// adminScopedRoleProbe grants an admin-database role (an *AnyDatabase, cluster,
// backup/restore, or root role) and runs one operation as the holder. Data ops
// target an arbitrary non-admin database; cluster ops target admin internally.
// Like dbScopedRoleProbe it validates the outcome against real MongoDB.
func adminScopedRoleProbe(t *testing.T, id, role string, wantAllowed bool, op rbacOp, support harness.DumboDBSupport) harness.AuthCase {
	c := authCase(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "rbacany_" + tgt.NS
		user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, "admin", user)
			_ = harness.DropUser(ctx, tgt.Admin, db, "probeuser")
			_ = harness.DropRole(ctx, tgt.Admin, db, "proberole")
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		tgt.Setup1(tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}))
		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, "admin", user, pwd, []harness.RoleRef{{Role: role, DB: "admin"}}))
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, "admin")
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()

		opErr := op.fn(ctx, c, db)
		allowed := opErr == nil
		code, _, _ := harness.CommandErrorCode(opErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if allowed != wantAllowed {
				t.Errorf("%s [%s]: MongoDB allowed=%v (code=%d), want allowed=%v", id, op.name, allowed, code, wantAllowed)
			}
			if !allowed && code != 13 {
				t.Errorf("%s [%s]: MongoDB denial code=%d, want Unauthorized(13)", id, op.name, code)
			}
		}
		return bson.M{"allowed": allowed, "code": code}, nil
	})
	c.Support = support
	return c
}

func runAdminRbacRows(t *testing.T, role string, rows []rbacRow) {
	t.Helper()
	for _, r := range rows {
		harness.AuthPairTest(t, adminScopedRoleProbe(t, r.id, role, r.allowed, r.op, rbacSupport(r.id)))
	}
}
