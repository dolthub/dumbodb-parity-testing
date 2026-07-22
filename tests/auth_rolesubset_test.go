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
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area L: DumboDB must never grant a built-in role an action that
// MongoDB's role of the same name does not grant. DumboDB may grant fewer (it
// omits actions it does not support), but never more.

var builtinRoleNames = []string{
	"read", "readWrite", "dbAdmin", "userAdmin", "dbOwner",
	"readAnyDatabase", "readWriteAnyDatabase", "dbAdminAnyDatabase", "userAdminAnyDatabase",
	"clusterMonitor", "clusterManager", "hostManager", "clusterAdmin",
	"backup", "restore", "root",
}

func roleActions(ctx context.Context, admin *mongo.Client, role string) (map[string]bool, error) {
	var res bson.M
	err := admin.Database("admin").RunCommand(ctx, bson.D{
		{Key: "rolesInfo", Value: bson.D{{Key: "role", Value: role}, {Key: "db", Value: "admin"}}},
		{Key: "showPrivileges", Value: true},
	}).Decode(&res)
	set := map[string]bool{}
	if err != nil {
		return set, err
	}
	roles, _ := res["roles"].(bson.A)
	if len(roles) == 0 {
		return set, nil
	}
	privs, _ := roles[0].(bson.M)["privileges"].(bson.A)
	for _, p := range privs {
		if a, ok := p.(bson.M)["actions"].(bson.A); ok {
			for _, x := range a {
				set[x.(string)] = true
			}
		}
	}
	return set, nil
}

func TestAuthBuiltinRoleActionSubset(t *testing.T) {
	ctx := context.Background()
	ac, err := harness.GetAuthClients(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range builtinRoleNames {
		mongoActs, err := roleActions(ctx, ac.MongoAdmin, role)
		if err != nil {
			t.Fatalf("mongo rolesInfo %s: %v", role, err)
		}
		dumboActs, err := roleActions(ctx, ac.DumboDBAdmin, role)
		if err != nil {
			t.Fatalf("dumbo rolesInfo %s: %v", role, err)
		}
		var extra []string
		for a := range dumboActs {
			if !mongoActs[a] {
				extra = append(extra, a)
			}
		}
		sort.Strings(extra)
		if len(extra) > 0 {
			t.Errorf("SUBSET-%s: DumboDB grants actions MongoDB does not: %v", role, extra)
		}
	}
}
