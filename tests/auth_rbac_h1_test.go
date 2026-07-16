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

// Auth parity area H1: built-in-role enforcement for the read and readWrite
// database-scoped roles (RBAC-read-*, RBAC-rw-*).

type rbacRow struct {
	id      string
	allowed bool
	op      rbacOp
}

func runRbacRows(t *testing.T, role string, rows []rbacRow) {
	t.Helper()
	for _, r := range rows {
		harness.AuthPairTest(t, dbScopedRoleProbe(t, r.id, role, r.allowed, r.op))
	}
}

func TestAuthRBACRead(t *testing.T) {
	runRbacRows(t, "read", []rbacRow{
		{"RBAC-read-01-find", true, opFind},
		{"RBAC-read-02-count", true, opCount},
		{"RBAC-read-03-distinct", true, opDistinct},
		{"RBAC-read-04-aggregate", true, opAggregateRead},
		{"RBAC-read-05-collStats", true, opCollStats},
		{"RBAC-read-06-dbStats", true, opDbStats},
		{"RBAC-read-07-listCollections", true, opListColls},
		{"RBAC-read-08-listIndexes", true, opListIndexes},
		{"RBAC-read-20-insert", false, opInsert},
		{"RBAC-read-21-update", false, opUpdate},
		{"RBAC-read-22-delete", false, opDelete},
		{"RBAC-read-23-createCollection", false, opCreateCollection},
		{"RBAC-read-24-createIndexes", false, opCreateIndexes},
		{"RBAC-read-25-dropCollection", false, opDropCollection},
		{"RBAC-read-28-createUser", false, opCreateUser},
		{"RBAC-read-29-find-other-db", false, opFindOtherDB},
	})
}

func TestAuthRBACReadWrite(t *testing.T) {
	runRbacRows(t, "readWrite", []rbacRow{
		{"RBAC-rw-01-find", true, opFind},
		{"RBAC-rw-02-insert", true, opInsert},
		{"RBAC-rw-03-update", true, opUpdate},
		{"RBAC-rw-04-delete", true, opDelete},
		{"RBAC-rw-05-createCollection", true, opCreateCollection},
		{"RBAC-rw-06-createIndexes", true, opCreateIndexes},
		{"RBAC-rw-07-dropCollection", true, opDropCollection},
		{"RBAC-rw-08-dropIndexes", true, opDropIndexes},
		{"RBAC-rw-20-collMod", false, opCollMod},
		{"RBAC-rw-21-validate", false, opValidate},
		{"RBAC-rw-22-dropDatabase", false, opDropDatabase},
		{"RBAC-rw-23-createUser", false, opCreateUser},
		{"RBAC-rw-24-createRole", false, opCreateRole},
		{"RBAC-rw-25-find-other-db", false, opFindOtherDB},
		{"RBAC-rw-26-insert-other-db", false, opInsertOtherDB},
	})
}
