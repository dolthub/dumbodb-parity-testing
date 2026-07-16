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

// Auth parity area H4: built-in-role enforcement for the admin-only cluster
// roles (RBAC-cm/hm/ca/cmgr-*). Sharding/replication actions need a
// non-standalone topology and are out of scope; only standalone-valid ops and
// their denials are asserted.

func TestAuthRBACClusterMonitor(t *testing.T) {
	runAdminRbacRows(t, "clusterMonitor", []rbacRow{
		{"RBAC-cm-01-serverStatus", true, opServerStatus},
		{"RBAC-cm-02-getParameter", true, opGetParameter},
		{"RBAC-cm-03-hostInfo", true, opHostInfo},
		{"RBAC-cm-04-listDatabases", true, opListDatabases},
		{"RBAC-cm-08-dbStats", true, opDbStats},
		{"RBAC-cm-20-insert", false, opInsert},
		{"RBAC-cm-21-find-user-collection", false, opFind},
		{"RBAC-cm-22-createUser", false, opCreateUser},
		{"RBAC-cm-23-setParameter", false, opSetParameter},
	})
}

func TestAuthRBACHostManager(t *testing.T) {
	runAdminRbacRows(t, "hostManager", []rbacRow{
		{"RBAC-hm-01-setParameter", true, opSetParameter},
		{"RBAC-hm-02-logRotate", true, opLogRotate},
		{"RBAC-hm-20-find", false, opFind},
		{"RBAC-hm-21-insert", false, opInsert},
		{"RBAC-hm-22-createUser", false, opCreateUser},
		{"RBAC-hm-23-serverStatus", false, opServerStatus},
	})
}

func TestAuthRBACClusterAdmin(t *testing.T) {
	runAdminRbacRows(t, "clusterAdmin", []rbacRow{
		{"RBAC-ca-01-serverStatus", true, opServerStatus},
		{"RBAC-ca-02-setParameter", true, opSetParameter},
		{"RBAC-ca-04-listDatabases", true, opListDatabases},
		{"RBAC-ca-20-find-user-data", false, opFind},
		{"RBAC-ca-21-insert", false, opInsert},
		{"RBAC-ca-22-createUser", false, opCreateUser},
	})
}

func TestAuthRBACClusterManager(t *testing.T) {
	runAdminRbacRows(t, "clusterManager", []rbacRow{
		{"RBAC-cmgr-01-getClusterParameter", true, opGetClusterParameter},
		{"RBAC-cmgr-20-find", false, opFind},
		{"RBAC-cmgr-21-insert", false, opInsert},
		{"RBAC-cmgr-22-createUser", false, opCreateUser},
	})
}
