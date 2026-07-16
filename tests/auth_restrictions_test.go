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
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area E: authenticationRestrictions (AUTHR-01..06). Tests run from
// 127.0.0.1 against a server bound to 127.0.0.1.

// restrictionCase creates a user with the given authenticationRestrictions and
// attempts to authenticate, asserting success or an auth failure on MongoDB.
func restrictionCase(t *testing.T, id string, wantSuccess bool, restrictions bson.A) harness.AuthCase {
	return authCase(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "authr_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		if err := runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createUser", Value: u},
			{Key: "pwd", Value: "pw"},
			{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "read"}, {Key: "db", Value: db}}}},
			{Key: "authenticationRestrictions", Value: restrictions},
		}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectAs(ctx, tgt.BaseURI, u, "pw", db)
		if err == nil {
			_ = c.Disconnect(ctx)
		}
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if wantSuccess && err != nil {
				t.Errorf("%s: auth should succeed, got %v", id, err)
			}
			if !wantSuccess && (err == nil || !strings.Contains(strings.ToLower(err.Error()), "authentication failed")) {
				t.Errorf("%s: auth should fail with authentication failed, got %v", id, err)
			}
		}
		return bson.M{"authOK": err == nil}, err
	})
}

func clientSource(ips ...string) bson.D {
	a := bson.A{}
	for _, ip := range ips {
		a = append(a, ip)
	}
	return bson.D{{Key: "clientSource", Value: a}}
}

func TestAuthRestrictions(t *testing.T) {
	// AUTHR-01: clientSource matching the connection (localhost) -> success.
	harness.AuthPairTest(t, restrictionCase(t, "AUTHR-01-clientSource-match", true,
		bson.A{clientSource("127.0.0.1")}))

	// AUTHR-02: clientSource not matching -> auth fails.
	harness.AuthPairTest(t, restrictionCase(t, "AUTHR-02-clientSource-mismatch", false,
		bson.A{clientSource("10.0.0.1")}))

	// AUTHR-03: serverAddress matching the bound address -> success.
	harness.AuthPairTest(t, restrictionCase(t, "AUTHR-03-serverAddress-match", true,
		bson.A{bson.D{{Key: "serverAddress", Value: bson.A{"127.0.0.1"}}}}))

	// AUTHR-04: serverAddress not matching -> auth fails.
	harness.AuthPairTest(t, restrictionCase(t, "AUTHR-04-serverAddress-mismatch", false,
		bson.A{bson.D{{Key: "serverAddress", Value: bson.A{"10.0.0.1"}}}}))

	// AUTHR-05: multiple restriction documents are OR-ed; one satisfied -> success.
	harness.AuthPairTest(t, restrictionCase(t, "AUTHR-05-multiple-or", true,
		bson.A{clientSource("10.0.0.1"), clientSource("127.0.0.1")}))

	// AUTHR-06: usersInfo showAuthenticationRestrictions echoes the restriction.
	harness.AuthPairTest(t, authCase("AUTHR-06-usersInfo-shows-restrictions", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "authr_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		if err := runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createUser", Value: u},
			{Key: "pwd", Value: "pw"},
			{Key: "roles", Value: bson.A{}},
			{Key: "authenticationRestrictions", Value: bson.A{clientSource("127.0.0.1")}},
		}); err != nil {
			return nil, err
		}
		res, err := usersInfoCmd(ctx, tgt.Admin, db, bson.D{{Key: "usersInfo", Value: u}, {Key: "showAuthenticationRestrictions", Value: true}})
		if err != nil {
			return nil, err
		}
		users, _ := res["users"].(bson.A)
		first, _ := users[0].(bson.M)
		_, hasRestrictions := first["authenticationRestrictions"]
		return bson.M{"hasRestrictions": hasRestrictions}, nil
	}))
}
