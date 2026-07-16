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
	"testing"
)

// Auth parity area H3: built-in-role enforcement for the admin-only
// *AnyDatabase roles (RBAC-rad/rwad/daad/uaad-*). Each grants the base role
// across all databases plus listDatabases on the cluster.

func TestAuthRBACReadAnyDatabase(t *testing.T) {
	runAdminRbacRows(t, "readAnyDatabase", []rbacRow{
		{"RBAC-rad-01-find", true, opFind},
		{"RBAC-rad-03-collStats", true, opCollStats},
		{"RBAC-rad-04-listDatabases", true, opListDatabases},
		{"RBAC-rad-20-insert", false, opInsert},
		{"RBAC-rad-21-createUser", false, opCreateUser},
		{"RBAC-rad-22-dropDatabase", false, opDropDatabase},
	})
}

func TestAuthRBACReadWriteAnyDatabase(t *testing.T) {
	runAdminRbacRows(t, "readWriteAnyDatabase", []rbacRow{
		{"RBAC-rwad-01-insert", true, opInsert},
		{"RBAC-rwad-03-find", true, opFind},
		{"RBAC-rwad-04-createCollection", true, opCreateCollection},
		{"RBAC-rwad-05-listDatabases", true, opListDatabases},
		{"RBAC-rwad-20-dropDatabase", false, opDropDatabase},
		{"RBAC-rwad-21-createUser", false, opCreateUser},
		{"RBAC-rwad-22-serverStatus", false, opServerStatus},
	})
}

func TestAuthRBACDbAdminAnyDatabase(t *testing.T) {
	runAdminRbacRows(t, "dbAdminAnyDatabase", []rbacRow{
		{"RBAC-daad-01-createCollection", true, opCreateCollection},
		{"RBAC-daad-02-dropDatabase", true, opDropDatabase},
		{"RBAC-daad-03-collMod", true, opCollMod},
		{"RBAC-daad-04-validate", true, opValidate},
		{"RBAC-daad-05-listDatabases", true, opListDatabases},
		{"RBAC-daad-20-find", false, opFind},
		{"RBAC-daad-21-insert", false, opInsert},
		{"RBAC-daad-22-createUser", false, opCreateUser},
	})
}

func TestAuthRBACUserAdminAnyDatabase(t *testing.T) {
	runAdminRbacRows(t, "userAdminAnyDatabase", []rbacRow{
		{"RBAC-uaad-01-createUser", true, opCreateUser},
		{"RBAC-uaad-02-createRole", true, opCreateRole},
		{"RBAC-uaad-03-usersInfo", true, opUsersInfo},
		{"RBAC-uaad-04-listDatabases", true, opListDatabases},
		{"RBAC-uaad-20-find", false, opFind},
		{"RBAC-uaad-21-insert", false, opInsert},
	})
}
