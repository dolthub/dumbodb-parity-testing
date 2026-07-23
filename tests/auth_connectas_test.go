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

// TestAuthConnectAs is the R2 smoke test: it creates a user via the admin
// client, then connects AS that user through ConnectAs, exercised via the
// AuthPairTest paired runner. Against MongoDB the created user authenticates
// and reads its own data; against DumboDB createUser is stubbed and the user
// cannot authenticate, so the two diverge -- expected while XFail.
func TestAuthConnectAs(t *testing.T) {
	harness.AuthPairTest(t, harness.AuthCase{
		Name:    "R2-connect-as-created-user",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, tgt harness.AuthTarget) (interface{}, error) {
			db := "authr2_" + tgt.NS
			user := "u_" + tgt.NS
			pwd := "pw-" + tgt.NS
			adminDB := tgt.Admin.Database(db)

			// Best-effort cleanup so reruns start clean.
			defer func() {
				_ = adminDB.RunCommand(ctx, bson.D{{Key: "dropUser", Value: user}}).Err()
				_ = adminDB.Drop(ctx)
			}()

			// Create a readWrite user on db.
			if err := adminDB.RunCommand(ctx, bson.D{
				{Key: "createUser", Value: user},
				{Key: "pwd", Value: pwd},
				{Key: "roles", Value: bson.A{bson.D{{Key: "role", Value: "readWrite"}, {Key: "db", Value: db}}}},
			}).Err(); err != nil {
				return nil, err
			}

			// Connect AS that user and perform a scoped write+read.
			uc, err := harness.ConnectAs(ctx, tgt.BaseURI, user, pwd, db)
			if err != nil {
				return nil, err
			}
			defer func() { _ = uc.Disconnect(ctx) }()

			coll := uc.Database(db).Collection("c")
			tgt.Setup1(coll.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "v", Value: "hi"}}))
			var got bson.M
			tgt.Setup(coll.FindOne(ctx, bson.D{{Key: "_id", Value: 1}}).Decode(&got))
			return got["v"], nil
		},
	})
}
