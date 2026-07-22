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
	"sort"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area K: direct reads of the admin user store. MongoDB's root can
// run admin.system.users.find(); DumboDB must too, and the returned documents
// must match apart from the random per-user credential material and userId.

func credentialMechanisms(v interface{}) []string {
	var mechs []string
	switch c := v.(type) {
	case bson.M:
		for m := range c {
			mechs = append(mechs, m)
		}
	case bson.D:
		for _, e := range c {
			mechs = append(mechs, e.Key)
		}
	}
	sort.Strings(mechs)
	return mechs
}

// normalizeUserDoc replaces the fields that legitimately differ per user
// (credentials salt/keys, userId) with stable stand-ins so the rest of the
// document -- _id, user, db, roles, authenticationRestrictions -- is compared.
func normalizeUserDoc(d bson.M) bson.M {
	out := bson.M{}
	for k, v := range d {
		switch k {
		case "credentials":
			out["credentialMechanisms"] = credentialMechanisms(v)
		case "userId":
			out["hasUserId"] = v != nil
		default:
			out[k] = v
		}
	}
	return out
}

func TestAuthSystemStoreRead(t *testing.T) {
	// SYS-01: root reads admin.system.users; the documents match, including that
	// an empty authenticationRestrictions is absent and a set one is present.
	harness.AuthPairTest(t, authCaseFull("SYS-01-system-users-read", func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
		db := "sysstore_" + tgt.NS
		defer func() {
			_ = harness.DropUser(ctx, tgt.Admin, db, "plain")
			_ = harness.DropUser(ctx, tgt.Admin, db, "restr")
			_ = tgt.Admin.Database(db).Drop(ctx)
		}()

		tgt.Setup(harness.CreateUser(ctx, tgt.Admin, db, "plain", "pw", []harness.RoleRef{{Role: "read", DB: db}}))
		tgt.Setup(runCmd(ctx, tgt.Admin, db, bson.D{
			{Key: "createUser", Value: "restr"}, {Key: "pwd", Value: "pw"}, {Key: "roles", Value: bson.A{}},
			{Key: "authenticationRestrictions", Value: bson.A{bson.D{{Key: "clientSource", Value: bson.A{"127.0.0.1"}}}}},
		}))

		cur, err := tgt.Admin.Database("admin").Collection("system.users").Find(ctx, bson.D{{Key: "db", Value: db}})
		if err != nil {
			return nil, err
		}
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		sort.Slice(docs, func(i, j int) bool {
			return docs[i]["_id"].(string) < docs[j]["_id"].(string)
		})

		out := make([]bson.M, len(docs))
		for i, d := range docs {
			out[i] = normalizeUserDoc(d)
		}
		return bson.M{"users": out}, nil
	}))
}
