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

// Package storage measures and compares on-disk storage behaviour between
// DumboDB and Dolt when performing equivalent branch + merge operations with
// a secondary index.
//
// Both backends must be running as Docker instances with their data directories
// mounted and accessible to the test process. Configure them via environment
// variables:
//
//	DUMBODB_URI      - MongoDB URI for DumboDB (default: mongodb://localhost:27018)
//	DUMBODB_DATA_DIR - host path to DumboDB data directory (required)
//	DOLT_URI         - MySQL DSN for dolt sql-server  (default: root:@tcp(localhost:3306)/)
//	DOLT_DATA_DIR    - host path to Dolt data directory (required)
//
// Both variables are required. The tests fail immediately if either backend is
// unreachable or its data directory is not configured.
package storage

import (
	"context"
	"time"
)

// Doc is the canonical document shape shared by all backends.
type Doc struct {
	ID    string
	Email string
	Name  string
	Age   int
}

// Backend is a versioned storage system that supports branching and merging.
type Backend interface {
	Name() string

	// Setup creates the collection/table and secondary index on Email.
	Setup(ctx context.Context) error

	// InsertBatch appends docs to the current working set.
	InsertBatch(ctx context.Context, docs []Doc) error

	// UpdateEmail changes the Email field of the document with the given ID.
	UpdateEmail(ctx context.Context, id, newEmail string) error

	// Commit snapshots the current working set with a message.
	Commit(ctx context.Context, msg string) error

	// CreateBranch creates a new branch from the current HEAD.
	CreateBranch(ctx context.Context, branch string) error

	// Checkout switches the backend to the named branch.
	Checkout(ctx context.Context, branch string) error

	// Merge merges fromBranch into the current branch and returns elapsed time.
	Merge(ctx context.Context, fromBranch string) (time.Duration, error)

	// StorageBytes runs GC (where supported) then returns the on-disk byte count
	// for this backend's database.
	StorageBytes(ctx context.Context) (int64, error)

	// Close releases connections and drops the test database.
	Close() error
}
