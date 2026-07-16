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

package harness

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Typed helpers for user/role management fixtures, run through an admin client
// (see AuthTarget.Admin). They keep auth tests terse and consistent. Each
// returns the raw command error so tests can assert exact codes via
// CommandErrorCode; cleanup helpers (DropUser/DropRole) are safe to call in a
// defer and tolerate a missing target on the caller's side.

// RoleRef is a {role, db} grant reference. A bare role name on the command's
// own database is RoleRef{Role: "read", DB: db}.
type RoleRef struct {
	Role string
	DB   string
}

// Privilege is a resource + actions pair for custom roles.
type Privilege struct {
	Resource bson.D
	Actions  []string
}

func rolesArray(roles []RoleRef) bson.A {
	a := bson.A{}
	for _, r := range roles {
		a = append(a, bson.D{{Key: "role", Value: r.Role}, {Key: "db", Value: r.DB}})
	}
	return a
}

func privilegesArray(privs []Privilege) bson.A {
	a := bson.A{}
	for _, p := range privs {
		a = append(a, bson.D{{Key: "resource", Value: p.Resource}, {Key: "actions", Value: p.Actions}})
	}
	return a
}

// CreateUser creates (user, pwd) on db with the given roles.
func CreateUser(ctx context.Context, admin *mongo.Client, db, user, pwd string, roles []RoleRef) error {
	return admin.Database(db).RunCommand(ctx, bson.D{
		{Key: "createUser", Value: user},
		{Key: "pwd", Value: pwd},
		{Key: "roles", Value: rolesArray(roles)},
	}).Err()
}

// DropUser removes user from db.
func DropUser(ctx context.Context, admin *mongo.Client, db, user string) error {
	return admin.Database(db).RunCommand(ctx, bson.D{{Key: "dropUser", Value: user}}).Err()
}

// GrantRolesToUser adds roles to user on db.
func GrantRolesToUser(ctx context.Context, admin *mongo.Client, db, user string, roles []RoleRef) error {
	return admin.Database(db).RunCommand(ctx, bson.D{
		{Key: "grantRolesToUser", Value: user},
		{Key: "roles", Value: rolesArray(roles)},
	}).Err()
}

// RevokeRolesFromUser removes roles from user on db.
func RevokeRolesFromUser(ctx context.Context, admin *mongo.Client, db, user string, roles []RoleRef) error {
	return admin.Database(db).RunCommand(ctx, bson.D{
		{Key: "revokeRolesFromUser", Value: user},
		{Key: "roles", Value: rolesArray(roles)},
	}).Err()
}

// CreateRole creates a custom role on db with the given privileges and
// inherited roles (either may be empty).
func CreateRole(ctx context.Context, admin *mongo.Client, db, role string, privs []Privilege, roles []RoleRef) error {
	return admin.Database(db).RunCommand(ctx, bson.D{
		{Key: "createRole", Value: role},
		{Key: "privileges", Value: privilegesArray(privs)},
		{Key: "roles", Value: rolesArray(roles)},
	}).Err()
}

// DropRole removes a custom role from db.
func DropRole(ctx context.Context, admin *mongo.Client, db, role string) error {
	return admin.Database(db).RunCommand(ctx, bson.D{{Key: "dropRole", Value: role}}).Err()
}

// UsersInfo returns the usersInfo result document for (user, db).
func UsersInfo(ctx context.Context, admin *mongo.Client, db, user string) (bson.M, error) {
	var res bson.M
	err := admin.Database(db).RunCommand(ctx, bson.D{
		{Key: "usersInfo", Value: bson.D{{Key: "user", Value: user}, {Key: "db", Value: db}}},
	}).Decode(&res)
	return res, err
}
