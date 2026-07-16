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

// Auth parity area C: hello / saslSupportedMechs negotiation (SPEC-04, 05, 06,
// 08). The speculativeAuthenticate payload itself (SPEC-01/02/03/07) is
// driver-internal and requires crafting raw SASL frames; those are tracked with
// the wire-level SCRAM cases.

// helloMechs runs hello with saslSupportedMechs for user@db and returns the
// mechanisms the server reports for that user.
func helloMechs(ctx context.Context, c *mongo.Client, user, db string) ([]string, error) {
	var res bson.M
	err := c.Database("admin").RunCommand(ctx, bson.D{
		{Key: "hello", Value: 1},
		{Key: "saslSupportedMechs", Value: db + "." + user},
	}).Decode(&res)
	if err != nil {
		return nil, err
	}
	var mechs []string
	if arr, ok := res["saslSupportedMechs"].(bson.A); ok {
		for _, m := range arr {
			if s, ok := m.(string); ok {
				mechs = append(mechs, s)
			}
		}
	}
	return mechs, nil
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestAuthSpeculativeNegotiation(t *testing.T) {
	// SPEC-04 / SPEC-06: for a user with both mechanisms, saslSupportedMechs
	// lists both, with SCRAM-SHA-256 present (the preferred mechanism).
	harness.AuthPairTest(t, authCase("SPEC-04-saslSupportedMechs-both", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "spec_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		if err := createUserMech(ctx, tgt.Admin, db, u, "pw", rwRole(db), []string{"SCRAM-SHA-1", "SCRAM-SHA-256"}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		mechs, err := helloMechs(ctx, c, u, db)
		if err != nil {
			return nil, err
		}
		return bson.M{"count": len(mechs), "hasSHA256": containsStr(mechs, "SCRAM-SHA-256"), "hasSHA1": containsStr(mechs, "SCRAM-SHA-1")}, nil
	}))

	// SPEC-06b: a SHA-256-only user reports only SCRAM-SHA-256.
	harness.AuthPairTest(t, authCase("SPEC-06-saslSupportedMechs-sha256-only", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db, u := "spec_"+tgt.NS, "u_"+tgt.NS
		defer cleanupUser(ctx, tgt, db, u)
		if err := createUserMech(ctx, tgt.Admin, db, u, "pw", rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
			return nil, err
		}
		c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		mechs, err := helloMechs(ctx, c, u, db)
		if err != nil {
			return nil, err
		}
		return bson.M{"count": len(mechs), "hasSHA256": containsStr(mechs, "SCRAM-SHA-256"), "hasSHA1": containsStr(mechs, "SCRAM-SHA-1")}, nil
	}))

	// SPEC-05: saslSupportedMechs for an unknown user does not error the hello
	// and reports no mechanisms.
	harness.AuthPairTest(t, authCase("SPEC-05-saslSupportedMechs-unknown-user", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		mechs, err := helloMechs(ctx, c, "nobody_"+tgt.NS, "spec_"+tgt.NS)
		if err != nil {
			return nil, err
		}
		return bson.M{"count": len(mechs)}, nil
	}))

	// SPEC-08: hello is allowed pre-authentication (anonymous handshake).
	harness.AuthPairTest(t, authCase("SPEC-08-hello-anonymous", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		c, err := harness.ConnectNoAuth(ctx, tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = c.Disconnect(ctx) }()
		var res bson.M
		err = c.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&res)
		_, hasPrimaryField := res["isWritablePrimary"]
		return bson.M{"ok": err == nil, "hasPrimaryField": hasPrimaryField}, err
	}))
}
