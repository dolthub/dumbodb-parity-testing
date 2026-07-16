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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Auth parity area H5: built-in-role enforcement for backup, restore, root, and
// a role-less user (RBAC-bak/res/root/none-*).

func TestAuthRBACBackup(t *testing.T) {
	runAdminRbacRows(t, "backup", []rbacRow{
		{"RBAC-bak-01-find", true, opFind},
		{"RBAC-bak-02-listDatabases", true, opListDatabases},
		{"RBAC-bak-03-listCollections", true, opListColls},
		{"RBAC-bak-05-serverStatus", true, opServerStatus},
		{"RBAC-bak-20-insert", false, opInsert},
		{"RBAC-bak-21-createUser", false, opCreateUser},
		{"RBAC-bak-22-dropDatabase", false, opDropDatabase},
	})
}

func TestAuthRBACRestore(t *testing.T) {
	runAdminRbacRows(t, "restore", []rbacRow{
		{"RBAC-res-01-insert", true, opInsert},
		{"RBAC-res-02-createCollection", true, opCreateCollection},
		{"RBAC-res-04-createUser", true, opCreateUser},
		{"RBAC-res-20-serverStatus", false, opServerStatus},
	})
}

func TestAuthRBACRoot(t *testing.T) {
	runAdminRbacRows(t, "root", []rbacRow{
		{"RBAC-root-01-find", true, opFind},
		{"RBAC-root-02-insert", true, opInsert},
		{"RBAC-root-05-createCollection", true, opCreateCollection},
		{"RBAC-root-06-dropDatabase", true, opDropDatabase},
		{"RBAC-root-07-createUser", true, opCreateUser},
		{"RBAC-root-08-createRole", true, opCreateRole},
		{"RBAC-root-10-serverStatus", true, opServerStatus},
		{"RBAC-root-12-listDatabases", true, opListDatabases},
		{"RBAC-root-11-setParameter", true, opSetParameter},
		// Boundary: root is not anyAction/anyResource, so it cannot write a
		// system collection directly.
		{"RBAC-root-13-insert-system.users", false, opInsertSystemUsers},
	})
}

func TestAuthRBACNoRole(t *testing.T) {
	for _, r := range []rbacRow{
		{"RBAC-none-01-find", false, opFind},
		{"RBAC-none-02-insert", false, opInsert},
		{"RBAC-none-03-createUser", false, opCreateUser},
		{"RBAC-none-04-serverStatus", false, opServerStatus},
		{"RBAC-none-05-listDatabases", false, opListDatabases},
		{"RBAC-none-20-ping", true, opPing},
		{"RBAC-none-21-connectionStatus", true, opConnectionStatus},
	} {
		harness.AuthPairTest(t, noRoleProbe(t, r.id, r.allowed, r.op))
	}
}
