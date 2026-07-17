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
	"github.com/dolthub/dumbodb-parity-testing/wire"
)

func findWithLsid(conn *wire.Conn, db string, lsid bson.D) (bson.M, error) {
	return conn.RunCommand(bson.D{
		{Key: "find", Value: "c"},
		{Key: "filter", Value: bson.D{}},
		{Key: "lsid", Value: lsid},
		{Key: "$db", Value: db},
	})
}

func TestAuthLsidCrossUserScoped(t *testing.T) {
	harness.AuthPairTest(t, authCase("LSID-01-same-id-different-user-is-user-scoped", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "lsid_" + tgt.NS
		userA, userB, pw := "a_"+tgt.NS, "b_"+tgt.NS, "pw"
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, userA)
			_ = harness.DropUser(ctx, tgt.Admin, db, userB)
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()
		if _, err := tgt.Admin.Database(db).Collection("c").InsertOne(ctx, bson.D{{Key: "x", Value: 1}}); err != nil {
			return nil, err
		}
		for _, u := range []string{userA, userB} {
			if err := createUserMech(ctx, tgt.Admin, db, u, pw, rwRole(db), []string{"SCRAM-SHA-256"}); err != nil {
				return nil, err
			}
		}

		lsid := bson.D{{Key: "id", Value: wire.NewLsid()}}

		conn1, err := wire.Dial(tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		hsA, err := fullHandshake(conn1, db, userA, pw, true)
		if err != nil {
			_ = conn1.Close()
			return nil, err
		}
		rA, err := findWithLsid(conn1, db, lsid)
		if err != nil {
			_ = conn1.Close()
			return nil, err
		}
		_ = conn1.Close()

		conn2, err := wire.Dial(tgt.BaseURI)
		if err != nil {
			return nil, err
		}
		defer func() { _ = conn2.Close() }()
		hsB, err := fullHandshake(conn2, db, userB, pw, true)
		if err != nil {
			return nil, err
		}
		rB, err := findWithLsid(conn2, db, lsid)
		if err != nil {
			return nil, err
		}

		res := bson.M{
			"authA":    hsA["authOK"],
			"authB":    hsB["authOK"],
			"asA_ok":   replyOK(rA),
			"asB_ok":   replyOK(rB),
			"asB_code": replyCode(rB),
		}

		if tgt.BaseURI == harness.AuthMongoBaseURI() {
			if res["authA"] != true || res["authB"] != true {
				t.Errorf("LSID-01: both users should authenticate, got %v", res)
			}
			if res["asA_ok"] != true {
				t.Errorf("LSID-01: user A should use its own logical session, got %v", res)
			}
			if res["asB_ok"] != true {
				t.Errorf("LSID-01: user B presenting A's id should be accepted (user-scoped), got %v", res)
			}
		}
		return res, nil
	}))
}
