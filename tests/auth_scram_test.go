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

// Auth parity area B: the SCRAM handshake, driver-expressible cases.
// Protocol-level cases that require crafting raw SASL frames (SCRAM-07, 11-17,
// 20) are tracked separately (the driver hides that layer).

// createUserMech creates a user with an explicit mechanisms list (nil = server
// default of both SCRAM mechanisms).
func createUserMech(ctx context.Context, admin *mongo.Client, db, user, pwd string, roles bson.A, mechanisms []string) error {
	cmd := bson.D{
		{Key: "createUser", Value: user},
		{Key: "pwd", Value: pwd},
		{Key: "roles", Value: roles},
	}
	if mechanisms != nil {
		cmd = append(cmd, bson.E{Key: "mechanisms", Value: mechanisms})
	}
	return admin.Database(db).RunCommand(ctx, cmd).Err()
}

func rwRole(db string) bson.A {
	return bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: db}}}
}

func TestAuthScramSuccess(t *testing.T) {
	// SCRAM-01: correct password over SCRAM-SHA-256 authenticates.
	// SCRAM-02: correct password over SCRAM-SHA-1 authenticates.
	// SCRAM-10: a user with both mechanisms authenticates under either.
	for _, mech := range []struct {
		id, name string
	}{
		{"SCRAM-01", "SCRAM-SHA-256"},
		{"SCRAM-02", "SCRAM-SHA-1"},
	} {
		mech := mech
		harness.AuthPairTest(t, harness.AuthCase{
			Name:    mech.id + "-correct-password-" + mech.name,
			Support: harness.DumboDBXFail,
			Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
				db := "scram_" + tgt.NS
				user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
				defer cleanupUser(ctx, tgt, db, user)
				if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), nil); err != nil {
					return nil, err
				}
				c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, user, pwd, db, mech.name)
				if err != nil {
					return nil, err
				}
				_ = c.Disconnect(ctx)
				return bson.M{"authOK": true}, nil
			},
		})
	}

	// SCRAM-09: a user created with only SCRAM-SHA-256 authenticates via SHA-256.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SCRAM-09-sha256-only-authenticates",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "scram_" + tgt.NS
			user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
			defer cleanupUser(ctx, tgt, db, user)
			if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
				return nil, err
			}
			c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, user, pwd, db, "SCRAM-SHA-256")
			if err != nil {
				return nil, err
			}
			_ = c.Disconnect(ctx)
			return bson.M{"authOK": true}, nil
		},
	})

	// SCRAM-18: a password requiring SASLprep normalization authenticates
	// (SHA-256). The unicode is spelled with a \u escape to keep the source
	// 7-bit ASCII.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SCRAM-18-saslprep-unicode-password",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "scram_" + tgt.NS
			user, pwd := "u_"+tgt.NS, "p\u00e9\u00dfw\u00f6rd-"+tgt.NS
			defer cleanupUser(ctx, tgt, db, user)
			if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
				return nil, err
			}
			c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, user, pwd, db, "SCRAM-SHA-256")
			if err != nil {
				return nil, err
			}
			_ = c.Disconnect(ctx)
			return bson.M{"authOK": true}, nil
		},
	})
}

func TestAuthScramFailure(t *testing.T) {
	// SCRAM-03 / SCRAM-04: wrong password fails with AuthenticationFailed (18).
	for _, mech := range []struct{ id, name string }{
		{"SCRAM-03", "SCRAM-SHA-256"},
		{"SCRAM-04", "SCRAM-SHA-1"},
	} {
		mech := mech
		harness.AuthPairTest(t, harness.AuthCase{
			Name:    mech.id + "-wrong-password-" + mech.name,
			Support: harness.DumboDBXFail,
			Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
				db := "scram_" + tgt.NS
				user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
				defer cleanupUser(ctx, tgt, db, user)
				if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), nil); err != nil {
					return nil, err
				}
				c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, user, "wrong-"+pwd, db, mech.name)
				if err == nil {
					_ = c.Disconnect(ctx)
				}
				return nil, err
			},
		})
	}

	// SCRAM-05: authenticating as a non-existent user fails with 18 (and the
	// same vague message, so user existence is not leaked).
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SCRAM-05-unknown-user",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, "nobody_"+tgt.NS, "whatever", "admin", "SCRAM-SHA-256")
			if err == nil {
				_ = c.Disconnect(ctx)
			}
			return nil, err
		},
	})

	// SCRAM-06: authenticating against the wrong authSource fails.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SCRAM-06-wrong-authsource",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "scram_" + tgt.NS
			user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
			defer cleanupUser(ctx, tgt, db, user)
			if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), nil); err != nil {
				return nil, err
			}
			c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, user, pwd, "admin", "SCRAM-SHA-256")
			if err == nil {
				_ = c.Disconnect(ctx)
			}
			return nil, err
		},
	})

	// SCRAM-08: a user created with only SCRAM-SHA-256 cannot authenticate when
	// the client insists on SCRAM-SHA-1.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SCRAM-08-sha256-only-rejects-sha1",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "scram_" + tgt.NS
			user, pwd := "u_"+tgt.NS, "pw-"+tgt.NS
			defer cleanupUser(ctx, tgt, db, user)
			if err := createUserMech(ctx, tgt.Admin, db, user, pwd, rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
				return nil, err
			}
			c, err := harness.ConnectAsMech(ctx, tgt.BaseURI, user, pwd, db, "SCRAM-SHA-1")
			if err == nil {
				_ = c.Disconnect(ctx)
			}
			return nil, err
		},
	})
}

func TestAuthScramCreateValidation(t *testing.T) {
	// SCRAM-19: createUser with an empty username is rejected.
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "SCRAM-19-empty-username-rejected",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "scram_" + tgt.NS
			return nil, createUserMech(ctx, tgt.Admin, db, "", "pw", rwRole(db), nil)
		},
	})
}

// cleanupUser best-effort removes a test user and its database.
func cleanupUser(ctx context.Context, tgt harness.AuthTarget, db, user string) {
	_ = harness.DropUser(ctx, tgt.Admin, db, user)
	_ = tgt.Admin.Database(db).Drop(ctx)
}
