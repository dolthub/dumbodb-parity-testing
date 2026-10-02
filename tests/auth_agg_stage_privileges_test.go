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

// Auth parity area: aggregation stages carry their own privileges
// (AGGPRIV-01..09). $out needs insert+remove on its target, $merge needs
// insert (whenNotMatched: insert) and update (whenMatched not fail), and
// $lookup/$unionWith need find on the foreign collection.

type aggPrivNames struct{ db, other, actor string }

func TestAuthAggregationStagePrivileges(t *testing.T) {
	noPrivileges := func(aggPrivNames) []harness.Privilege { return nil }
	readRole := func(n aggPrivNames) []harness.RoleRef { return []harness.RoleRef{{Role: "read", DB: n.db}} }
	readWriteRole := func(n aggPrivNames) []harness.RoleRef { return []harness.RoleRef{{Role: "readWrite", DB: n.db}} }
	noRoles := func(aggPrivNames) []harness.RoleRef { return nil }

	harness.AuthPairTest(t, authCase("AGGPRIV-01-read-cannot-out-same-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, readRole, noPrivileges, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$out", Value: "copy"}}}
		})
	}))

	harness.AuthPairTest(t, authCase("AGGPRIV-02-read-cannot-out-other-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, readRole, noPrivileges, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$out", Value: bson.D{{Key: "db", Value: n.other}, {Key: "coll", Value: "copy"}}}}}
		})
	}))

	harness.AuthPairTest(t, authCase("AGGPRIV-03-read-cannot-merge", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, readRole, noPrivileges, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: "copy"}}}}}
		})
	}))

	harness.AuthPairTest(t, authCase("AGGPRIV-04-readWrite-cannot-out-other-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, readWriteRole, noPrivileges, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$out", Value: bson.D{{Key: "db", Value: n.other}, {Key: "coll", Value: "copy"}}}}}
		})
	}))

	findSourceOnly := func(n aggPrivNames) []harness.Privilege {
		return []harness.Privilege{{Resource: collResource(n.db, "c"), Actions: []string{"find"}}}
	}

	harness.AuthPairTest(t, authCase("AGGPRIV-05-lookup-needs-find-on-foreign", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, noRoles, findSourceOnly, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: "secret"}, {Key: "localField", Value: "a"},
				{Key: "foreignField", Value: "a"}, {Key: "as", Value: "x"},
			}}}}
		})
	}))

	harness.AuthPairTest(t, authCase("AGGPRIV-06-unionWith-needs-find-on-foreign", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, noRoles, findSourceOnly, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$unionWith", Value: "secret"}}}
		})
	}))

	harness.AuthPairTest(t, authCaseFull("AGGPRIV-07-readWrite-can-out-same-db", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, readWriteRole, noPrivileges, 0, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$out", Value: "copy"}}}
		})
	}))

	findAndInsert := func(n aggPrivNames) []harness.Privilege {
		return []harness.Privilege{{Resource: collResource(n.db, ""), Actions: []string{"find", "insert"}}}
	}

	harness.AuthPairTest(t, authCaseFull("AGGPRIV-08-merge-insert-only-needs-insert", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, noRoles, findAndInsert, 0, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$merge", Value: bson.D{
				{Key: "into", Value: "copy"}, {Key: "whenMatched", Value: "fail"}, {Key: "whenNotMatched", Value: "insert"},
			}}}}
		})
	}))

	harness.AuthPairTest(t, authCase("AGGPRIV-09-merge-default-needs-update", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		return aggStageCase(ctx, t, tgt, noRoles, findAndInsert, 13, func(n aggPrivNames) bson.A {
			return bson.A{bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: "copy"}}}}}
		})
	}))
}

// aggStageCase runs aggregate on <db>.c as a user holding |roles| plus a
// custom role with |privileges| (if any), and reports the error code and
// whether the write targets <db>.copy and <other>.copy now exist. MongoDB must
// answer with |wantMongoCode| (0 = success). <db>.secret holds data the user
// may not be allowed to read.
func aggStageCase(
	ctx context.Context,
	t *testing.T,
	tgt harness.AuthTarget,
	roles func(aggPrivNames) []harness.RoleRef,
	privileges func(aggPrivNames) []harness.Privilege,
	wantMongoCode int32,
	pipeline func(aggPrivNames) bson.A,
) (interface{}, error) {
	n := aggPrivNames{db: "aggpriv_" + tgt.NS, other: "aggpriv_other_" + tgt.NS, actor: "actor_" + tgt.NS}
	role := "role_" + tgt.NS
	defer func() {
		_ = harness.DropUser(ctx, tgt.Admin, n.db, n.actor)
		_ = harness.DropRole(ctx, tgt.Admin, n.db, role)
		_ = tgt.Admin.Database(n.db).Drop(ctx)
		_ = tgt.Admin.Database(n.other).Drop(ctx)
	}()

	tgt.Setup1(tgt.Admin.Database(n.db).Collection("c").InsertOne(ctx, bson.D{{Key: "a", Value: 1}}))
	tgt.Setup1(tgt.Admin.Database(n.db).Collection("secret").InsertOne(ctx, bson.D{{Key: "a", Value: 1}, {Key: "pw", Value: "hunter2"}}))
	actorRoles := roles(n)
	if privs := privileges(n); len(privs) > 0 {
		tgt.Setup(harness.CreateRole(ctx, tgt.Admin, n.db, role, privs, nil))
		actorRoles = append(actorRoles, harness.RoleRef{Role: role, DB: n.db})
	}
	tgt.Setup(harness.CreateUser(ctx, tgt.Admin, n.db, n.actor, "pw", actorRoles))

	c, err := harness.ConnectAs(ctx, tgt.BaseURI, n.actor, "pw", n.db)
	if err != nil {
		return nil, err
	}
	defer func() { _ = c.Disconnect(ctx) }()

	aggErr := cmdErr(ctx, c, n.db, bson.D{
		{Key: "aggregate", Value: "c"},
		{Key: "pipeline", Value: pipeline(n)},
		{Key: "cursor", Value: bson.D{}},
	})
	code, _, _ := harness.CommandErrorCode(aggErr)
	if tgt.BaseURI == harness.AuthMongoBaseURI() && code != wantMongoCode {
		t.Errorf("aggregate: MongoDB returned code=%d err=%v, want code %d", code, aggErr, wantMongoCode)
	}

	exists := func(db string) (bool, error) {
		names, err := tgt.Admin.Database(db).ListCollectionNames(ctx, bson.D{{Key: "name", Value: "copy"}})
		return len(names) > 0, err
	}
	sameDB, err := exists(n.db)
	if err != nil {
		return nil, err
	}
	otherDB, err := exists(n.other)
	if err != nil {
		return nil, err
	}
	return bson.M{"code": code, "copyInDB": sameDB, "copyInOtherDB": otherDB}, nil
}
