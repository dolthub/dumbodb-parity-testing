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

// Auth parity area K: error-code fidelity (ERR-01..08). Each isolates a failure
// path and asserts the exact MongoDB code (and codeName when available). ERR-07
// (bad SASL mechanism -> 334) needs a mechanism the driver will not negotiate
// client-side and is tracked with the wire-level SCRAM cases.

// errCase runs a failing operation and asserts, on the MongoDB run, that it
// returns wantCode (and wantName when both sides carry a codeName).
func errCase(t *testing.T, id string, wantCode int32, wantName string,
	do func(ctx context.Context, tgt harness.AuthTarget) error) harness.AuthCase {
	return authCase(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		err := do(ctx, tgt)
		code, name, _ := harness.CommandErrorCode(err)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if code != wantCode {
				t.Errorf("%s: MongoDB code=%d, want %d (err=%v)", id, code, wantCode, err)
			}
			if wantName != "" && name != "" && name != wantName {
				t.Errorf("%s: MongoDB codeName=%q, want %q", id, name, wantName)
			}
		}
		return nil, err
	})
}

// errCaseMsg is for failures the driver surfaces as an auth-handshake error
// (not a CommandError with an extractable code): it asserts the MongoDB error
// is non-nil and its message contains msgSubstr (case-insensitive).
func errCaseMsg(t *testing.T, id, msgSubstr string, do func(ctx context.Context, tgt harness.AuthTarget) error) harness.AuthCase {
	return authCase(id, func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		err := do(ctx, tgt)
		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), msgSubstr) {
				t.Errorf("%s: MongoDB error=%v, want message containing %q", id, err, msgSubstr)
			}
		}
		return nil, err
	})
}

func full(c harness.AuthCase) harness.AuthCase {
	c.Support = harness.DumboDBFull
	return c
}

func TestAuthErrorFidelity(t *testing.T) {
	// ERR-01: bad password -> auth failure. The driver surfaces the server's
	// AuthenticationFailed(18) as a handshake error whose code is not a
	// CommandError, so assert on the (deliberately vague) message.
	harness.AuthPairTest(t, full(errCaseMsg(t, "ERR-01-bad-password", "authentication failed",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			db, u := "errk_"+tgt.NS, "u_"+tgt.NS
			defer cleanupUser(ctx, tgt, db, u)
			if err := harness.CreateUser(ctx, tgt.Admin, db, u, "right", []harness.RoleRef{{Role: "read", DB: db}}); err != nil {
				return err
			}
			c, err := harness.ConnectAs(ctx, tgt.BaseURI, u, "wrong", db)
			if err == nil {
				_ = c.Disconnect(ctx)
			}
			return err
		})))

	// ERR-02: a privileged command with no authentication -> Unauthorized (13).
	harness.AuthPairTest(t, full(errCase(t, "ERR-02-unauthenticated", 13, "Unauthorized",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
			if err != nil {
				return err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			return cmdErr(ctx, c, "errk_"+tgt.NS, bson.D{{Key: "insert", Value: "c"}, {Key: "documents", Value: bson.A{bson.D{{Key: "x", Value: 1}}}}})
		})))

	// ERR-03: authenticated but insufficient privilege -> Unauthorized (13).
	harness.AuthPairTest(t, errCase(t, "ERR-03-insufficient-privilege", 13, "Unauthorized",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			db, u := "errk_"+tgt.NS, "u_"+tgt.NS
			defer cleanupUser(ctx, tgt, db, u)
			if err := harness.CreateUser(ctx, tgt.Admin, db, u, "pw", []harness.RoleRef{{Role: "read", DB: db}}); err != nil {
				return err
			}
			c, err := harness.ConnectAs(ctx, tgt.BaseURI, u, "pw", db)
			if err != nil {
				return err
			}
			defer func() { _ = c.Disconnect(ctx) }()
			return cmdErr(ctx, c, db, bson.D{{Key: "insert", Value: "c"}, {Key: "documents", Value: bson.A{bson.D{{Key: "x", Value: 1}}}}})
		}))

	// ERR-04: dropUser on a missing user -> UserNotFound (11).
	harness.AuthPairTest(t, full(errCase(t, "ERR-04-user-not-found", 11, "UserNotFound",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			return harness.DropUser(ctx, tgt.Admin, "errk_"+tgt.NS, "ghost_"+tgt.NS)
		})))

	// ERR-05: dropRole on a missing role -> RoleNotFound (31).
	harness.AuthPairTest(t, errCase(t, "ERR-05-role-not-found", 31, "RoleNotFound",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			return harness.DropRole(ctx, tgt.Admin, "errk_"+tgt.NS, "ghost_"+tgt.NS)
		}))

	// ERR-06: duplicate createUser -> MongoDB 8.0 reports location 51003
	// ("User already exists"), not the underlying DuplicateKey 11000.
	harness.AuthPairTest(t, full(errCase(t, "ERR-06-duplicate-user", 51003, "",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			db, u := "errk_"+tgt.NS, "u_"+tgt.NS
			defer cleanupUser(ctx, tgt, db, u)
			if err := harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil); err != nil {
				return err
			}
			return harness.CreateUser(ctx, tgt.Admin, db, u, "pw", nil)
		})))

	// ERR-08: an unmet authenticationRestriction is reported server-side as
	// AuthenticationRestrictionUnmet(214) but masked to the client as a generic
	// auth failure, so assert on the client-visible message.
	harness.AuthPairTest(t, errCaseMsg(t, "ERR-08-auth-restriction-unmet", "authentication failed",
		func(ctx context.Context, tgt harness.AuthTarget) error {
			db, u := "errk_"+tgt.NS, "u_"+tgt.NS
			defer cleanupUser(ctx, tgt, db, u)
			// Restrict to a client source we are not connecting from.
			if err := runCmd(ctx, tgt.Admin, db, bson.D{
				{Key: "createUser", Value: u},
				{Key: "pwd", Value: "pw"},
				{Key: "roles", Value: bson.A{}},
				{Key: "authenticationRestrictions", Value: bson.A{bson.D{{Key: "clientSource", Value: bson.A{"10.0.0.1"}}}}},
			}); err != nil {
				return err
			}
			c, err := harness.ConnectAs(ctx, tgt.BaseURI, u, "pw", db)
			if err == nil {
				_ = c.Disconnect(ctx)
			}
			return err
		}))
}
