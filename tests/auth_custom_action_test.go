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

// Auth parity area I2: action granularity (ACT-01..24). Grant exactly one action
// on a resource and verify the command needing it is allowed while a sibling
// command on the same resource is denied.

type actRow struct {
	id      string
	action  string
	cluster bool // grant on {cluster:true} (role on admin) instead of {db,""}
	allowed rbacOp
	denied  rbacOp
}

func customActionProbe(t *testing.T, r actRow) harness.AuthCase {
	return authCaseFull(r.id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "acta_" + tgt.NS
		roleDB := db
		var resource bson.D
		if r.cluster {
			roleDB = "admin"
			resource = bson.D{{Key: "cluster", Value: true}}
		} else {
			resource = collResource(db, "")
		}
		role, user, pwd := "role_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, roleDB, user)
			_ = harness.DropUser(ctx, tgt.Admin, db, "probeuser")
			_ = harness.DropRole(ctx, tgt.Admin, db, "proberole")
			_ = harness.DropRole(ctx, tgt.Admin, roleDB, role)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if _, err := tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, roleDB, role, []harness.Privilege{{Resource: resource, Actions: []string{r.action}}}, nil); err != nil {
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

		allowErr := r.allowed.fn(ctx, c, db)
		denyErr := r.denied.fn(ctx, c, db)
		allowOK := allowErr == nil
		denyCode, _, _ := harness.CommandErrorCode(denyErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if !allowOK {
				ac, _, _ := harness.CommandErrorCode(allowErr)
				t.Errorf("%s: granted action %q but %s was denied (code=%d): %v", r.id, r.action, r.allowed.name, ac, allowErr)
			}
			if denyErr == nil {
				t.Errorf("%s: %s should be denied without its action", r.id, r.denied.name)
			} else if denyCode != 13 {
				t.Errorf("%s: %s denial code=%d, want Unauthorized(13)", r.id, r.denied.name, denyCode)
			}
		}
		return bson.M{"allowOK": allowOK, "denyCode": denyCode}, nil
	})
}

func TestAuthCustomActionGranularity(t *testing.T) {
	rows := []actRow{
		{id: "ACT-01-find", action: "find", allowed: opFind, denied: opInsert},
		{id: "ACT-02-insert", action: "insert", allowed: opInsert, denied: opFind},
		{id: "ACT-03-update", action: "update", allowed: opUpdate, denied: opDelete},
		{id: "ACT-04-remove", action: "remove", allowed: opDelete, denied: opUpdate},
		{id: "ACT-05-createCollection", action: "createCollection", allowed: opCreateCollection, denied: opDropCollection},
		{id: "ACT-06-createIndex", action: "createIndex", allowed: opCreateIndexes, denied: opDropIndexes},
		{id: "ACT-07-dropIndex", action: "dropIndex", allowed: opDropIndexes, denied: opCreateIndexes},
		{id: "ACT-08-dropCollection", action: "dropCollection", allowed: opDropCollection, denied: opCreateCollection},
		{id: "ACT-09-collMod", action: "collMod", allowed: opCollMod, denied: opValidate},
		{id: "ACT-10-validate", action: "validate", allowed: opValidate, denied: opCollMod},
		{id: "ACT-11-listCollections", action: "listCollections", allowed: opListColls, denied: opFind},
		{id: "ACT-12-listIndexes", action: "listIndexes", allowed: opListIndexes, denied: opCreateIndexes},
		{id: "ACT-13-collStats", action: "collStats", allowed: opCollStats, denied: opDbStats},
		{id: "ACT-14-dbStats", action: "dbStats", allowed: opDbStats, denied: opCollStats},
		{id: "ACT-15-createUser", action: "createUser", allowed: opCreateUser, denied: opUsersInfo},
		{id: "ACT-18-viewUser", action: "viewUser", allowed: opUsersInfo, denied: opCreateUser},
		{id: "ACT-19-viewRole", action: "viewRole", allowed: opRolesInfo, denied: opCreateRole},
		{id: "ACT-21-dropDatabase", action: "dropDatabase", allowed: opDropDatabase, denied: opDropCollection},
		{id: "ACT-22-listDatabases", action: "listDatabases", cluster: true, allowed: opListDatabases, denied: opServerStatus},
		// listDatabases is never outright denied (MongoDB filters results), so
		// use find as the denied sibling for the serverStatus grant.
		{id: "ACT-23-serverStatus", action: "serverStatus", cluster: true, allowed: opServerStatus, denied: opFind},
	}
	for _, r := range rows {
		harness.AuthPairTest(t, customActionProbe(t, r))
	}
}

// victimActionProbe covers user/role-management action granularity that needs a
// pre-existing target user (ACT-16 dropUser, ACT-17 grantRole, ACT-20
// changePassword). The acting user is granted exactly one action on {db,""};
// allowedFn exercises it against the victim, deniedFn a sibling that needs a
// different user-admin action.
func victimActionProbe(t *testing.T, id, action string,
	allowedFn func(ctx context.Context, c *mongo.Client, db, victim string) error,
	deniedFn func(ctx context.Context, c *mongo.Client, db string) error) harness.AuthCase {
	return authCaseFull(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "actv_" + tgt.NS
		role, user, pwd := "role_"+tgt.NS, "u_"+tgt.NS, "pw-"+tgt.NS
		victim := "victim_" + tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, user)
			_ = harness.DropUser(ctx, tgt.Admin, db, victim)
			_ = harness.DropRole(ctx, tgt.Admin, db, role)
			_ = harness.DropRole(ctx, tgt.Admin, db, "grantme_"+tgt.NS)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if err := harness.CreateUser(ctx, tgt.Admin, db, victim, "pw", []harness.RoleRef{{Role: "read", DB: db}}); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, db, "grantme_"+tgt.NS, nil, nil); err != nil {
			return nil, err
		}
		if err := harness.CreateRole(ctx, tgt.Admin, db, role, []harness.Privilege{{Resource: collResource(db, ""), Actions: []string{action}}}, nil); err != nil {
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

		allowErr := allowedFn(ctx, c, db, victim)
		denyErr := deniedFn(ctx, c, db)
		denyCode, _, _ := harness.CommandErrorCode(denyErr)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if allowErr != nil {
				t.Errorf("%s: granted %q but allowed op was denied: %v", id, action, allowErr)
			}
			if denyErr == nil {
				t.Errorf("%s: sibling op should be denied without its action", id)
			} else if denyCode != 13 {
				t.Errorf("%s: denial code=%d, want Unauthorized(13)", id, denyCode)
			}
		}
		return bson.M{"allowOK": allowErr == nil, "denyCode": denyCode}, nil
	})
}

func TestAuthCustomActionUserFamily(t *testing.T) {
	// ACT-16: dropUser action allows dropUser, denies createUser.
	harness.AuthPairTest(t, victimActionProbe(t, "ACT-16-dropUser", "dropUser",
		func(ctx context.Context, c *mongo.Client, db, victim string) error {
			return cmdErr(ctx, c, db, bson.D{{Key: "dropUser", Value: victim}})
		},
		func(ctx context.Context, c *mongo.Client, db string) error {
			return cmdErr(ctx, c, db, bson.D{{Key: "createUser", Value: "x"}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}})
		}))

	// ACT-17: grantRole action allows grantRolesToUser, denies revokeRolesFromUser.
	harness.AuthPairTest(t, victimActionProbe(t, "ACT-17-grantRole", "grantRole",
		func(ctx context.Context, c *mongo.Client, db, victim string) error {
			return cmdErr(ctx, c, db, bson.D{{Key: "grantRolesToUser", Value: victim}, {Key: "roles", Value: bson.A{"grantme_" + trimActv(db)}}})
		},
		func(ctx context.Context, c *mongo.Client, db string) error {
			return cmdErr(ctx, c, db, bson.D{{Key: "revokeRolesFromUser", Value: "x"}, {Key: "roles", Value: bson.A{"read"}}})
		}))

	// ACT-20: changePassword action allows updateUser(pwd), denies createUser.
	harness.AuthPairTest(t, victimActionProbe(t, "ACT-20-changePassword", "changePassword",
		func(ctx context.Context, c *mongo.Client, db, victim string) error {
			return cmdErr(ctx, c, db, bson.D{{Key: "updateUser", Value: victim}, {Key: "pwd", Value: "newpw"}})
		},
		func(ctx context.Context, c *mongo.Client, db string) error {
			return cmdErr(ctx, c, db, bson.D{{Key: "createUser", Value: "x"}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}}})
		}))

	// ACT-24 (bypassDocumentValidation) requires a collection validator and is a
	// modifier rather than a standalone command gate; deferred.
}

// trimActv recovers the tgt.NS suffix from the actv_ database name so the
// grantme role name matches.
func trimActv(db string) string { return db[len("actv_"):] }
