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

// TestAuthFixtures is the R3 smoke test: it uses the typed fixture helpers
// (CreateRole, CreateUser, DropUser, DropRole) to build a custom-role user and
// verifies the enforcement boundary (find allowed, insert denied). MongoDB
// enforces; DumboDB's role/user commands are stubbed, so the two diverge -- XFail.
func TestAuthFixtures(t *testing.T) {
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "R3-fixture-custom-role-boundary",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "authr3_" + tgt.NS
			role := "reader_" + tgt.NS
			user := "u_" + tgt.NS
			pwd := "pw-" + tgt.NS

			defer func() {
				_ = harness.DropUser(ctx, tgt.Admin, db, user)
				_ = harness.DropRole(ctx, tgt.Admin, db, role)
				_ = tgt.Admin.Database(db).Drop(ctx)
			}()

			// Custom role: find only, on all collections in db.
			readOnly := []harness.Privilege{{
				Resource: bson.D{{Key: "db", Value: db}, {Key: "collection", Value: ""}},
				Actions:  []string{"find"},
			}}
			tgt.Setup(harness.CreateRole(ctx, tgt.Admin, db, role, readOnly, nil))
			tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, user, pwd, []harness.RoleRef{{Role: role, DB: db}}))

			uc, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
			if err != nil {
				return nil, err
			}
			defer func() { _ = uc.Disconnect(ctx) }()
			coll := uc.Database(db).Collection("c")

			// find is allowed (empty result is fine).
			if err := coll.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Err(); err != nil && err.Error() != "mongo: no documents in result" {
				return nil, err
			}
			// insert must be denied with Unauthorized (13); return the code so
			// the two servers are compared on the authorization outcome.
			_, insErr := coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}})
			code, _, _ := harness.CommandErrorCode(insErr)
			return bson.M{"insertDeniedCode": code, "insertErrIsNil": insErr == nil}, nil
		},
	})
}
