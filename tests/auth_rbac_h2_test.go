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

// Auth parity area H2: built-in-role enforcement for the database-administration
// roles dbAdmin, userAdmin, and dbOwner (RBAC-dba-*, RBAC-ua-*, RBAC-do-*).

func TestAuthRBACDbAdmin(t *testing.T) {
	runRbacRows(t, "dbAdmin", []rbacRow{
		{"RBAC-dba-01-createCollection", true, opCreateCollection},
		{"RBAC-dba-02-createIndexes", true, opCreateIndexes},
		{"RBAC-dba-03-dropCollection", true, opDropCollection},
		{"RBAC-dba-04-dropIndexes", true, opDropIndexes},
		{"RBAC-dba-05-collMod", true, opCollMod},
		{"RBAC-dba-06-validate", true, opValidate},
		{"RBAC-dba-07-dropDatabase", true, opDropDatabase},
		{"RBAC-dba-08-collStats", true, opCollStats},
		{"RBAC-dba-09-dbStats", true, opDbStats},
		{"RBAC-dba-10-listCollections", true, opListColls},
		{"RBAC-dba-11-listIndexes", true, opListIndexes},
		{"RBAC-dba-20-find", false, opFind},
		{"RBAC-dba-21-insert", false, opInsert},
		{"RBAC-dba-22-update", false, opUpdate},
		{"RBAC-dba-23-createUser", false, opCreateUser},
	})
}

func TestAuthRBACUserAdmin(t *testing.T) {
	runRbacRows(t, "userAdmin", []rbacRow{
		{"RBAC-ua-01-createUser", true, opCreateUser},
		{"RBAC-ua-06-createRole", true, opCreateRole},
		{"RBAC-ua-09-usersInfo", true, opUsersInfo},
		{"RBAC-ua-20-find", false, opFind},
		{"RBAC-ua-21-insert", false, opInsert},
		{"RBAC-ua-22-createCollection", false, opCreateCollection},
		{"RBAC-ua-23-dropDatabase", false, opDropDatabase},
	})
}

func TestAuthRBACDbOwner(t *testing.T) {
	runRbacRows(t, "dbOwner", []rbacRow{
		{"RBAC-do-01-find", true, opFind},
		{"RBAC-do-02-insert", true, opInsert},
		{"RBAC-do-03-createCollection", true, opCreateCollection},
		{"RBAC-do-04-dropDatabase", true, opDropDatabase},
		{"RBAC-do-05-collMod", true, opCollMod},
		{"RBAC-do-06-validate", true, opValidate},
		{"RBAC-do-07-createUser", true, opCreateUser},
		{"RBAC-do-08-createRole", true, opCreateRole},
		{"RBAC-do-09-usersInfo", true, opUsersInfo},
		{"RBAC-do-20-serverStatus", false, opServerStatus},
		{"RBAC-do-21-find-other-db", false, opFindOtherDB},
	})
}
