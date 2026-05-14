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

package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// DoltBackend connects to a running dolt sql-server instance.
//
// Configure via environment variables:
//
//	DOLT_URI      - MySQL DSN (default: root:@tcp(localhost:3306)/)
//	DOLT_DATA_DIR - host path to the dolt data directory (required)
type DoltBackend struct {
	db      *sql.DB
	dbName  string
	dataDir string
}

// NewDoltBackend connects to the configured dolt sql-server and creates a
// fresh test database. Fails immediately if either the server is unreachable
// or DOLT_DATA_DIR is not set.
func NewDoltBackend(ctx context.Context) (*DoltBackend, error) {
	uri := os.Getenv("DOLT_URI")
	if uri == "" {
		uri = "root:@tcp(localhost:3306)/"
	}
	dataDir := os.Getenv("DOLT_DATA_DIR")
	if dataDir == "" {
		return nil, fmt.Errorf("DOLT_DATA_DIR is required")
	}

	db, err := sql.Open("mysql", uri)
	if err != nil {
		return nil, fmt.Errorf("dolt: open: %w", err)
	}
	// Force a single connection so CALL DOLT_CHECKOUT keeps its session state.
	db.SetMaxOpenConns(1)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		db.Close()
		return nil, fmt.Errorf("dolt: ping %s: %w", uri, err)
	}

	dbName := fmt.Sprintf("storage_%d", time.Now().UnixNano())
	if _, err := db.ExecContext(ctx, "CREATE DATABASE `"+dbName+"`"); err != nil {
		db.Close()
		return nil, fmt.Errorf("dolt: create database: %w", err)
	}
	if _, err := db.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
		db.Close()
		return nil, fmt.Errorf("dolt: use database: %w", err)
	}

	return &DoltBackend{db: db, dbName: dbName, dataDir: dataDir}, nil
}

func (b *DoltBackend) Name() string { return "Dolt" }

func (b *DoltBackend) Setup(ctx context.Context) error {
	_, err := b.db.ExecContext(ctx, `
		CREATE TABLE users (
			_id   VARCHAR(64)  PRIMARY KEY,
			email VARCHAR(255) NOT NULL,
			name  VARCHAR(255),
			age   INT,
			INDEX idx_email (email)
		)`)
	return err
}

func (b *DoltBackend) InsertBatch(ctx context.Context, docs []Doc) error {
	if len(docs) == 0 {
		return nil
	}
	const batchSize = 500
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[i:end]

		query := "INSERT INTO users (_id, email, name, age) VALUES "
		args := make([]interface{}, 0, len(batch)*4)
		for j, d := range batch {
			if j > 0 {
				query += ","
			}
			query += "(?,?,?,?)"
			args = append(args, d.ID, d.Email, d.Name, d.Age)
		}
		if _, err := b.db.ExecContext(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

func (b *DoltBackend) UpdateEmail(ctx context.Context, id, newEmail string) error {
	_, err := b.db.ExecContext(ctx, "UPDATE users SET email=? WHERE _id=?", newEmail, id)
	return err
}

func (b *DoltBackend) Commit(ctx context.Context, msg string) error {
	if _, err := b.db.ExecContext(ctx, "CALL DOLT_ADD('-A')"); err != nil {
		return err
	}
	_, err := b.db.ExecContext(ctx, "CALL DOLT_COMMIT('-m', ?)", msg)
	return err
}

func (b *DoltBackend) CreateBranch(ctx context.Context, branch string) error {
	_, err := b.db.ExecContext(ctx, "CALL DOLT_BRANCH(?)", branch)
	return err
}

func (b *DoltBackend) Checkout(ctx context.Context, branch string) error {
	_, err := b.db.ExecContext(ctx, "CALL DOLT_CHECKOUT(?)", branch)
	return err
}

func (b *DoltBackend) Merge(ctx context.Context, fromBranch string) (time.Duration, error) {
	start := time.Now()
	_, err := b.db.ExecContext(ctx, "CALL DOLT_MERGE(?)", fromBranch)
	dur := time.Since(start)
	if err != nil {
		return 0, err
	}
	return dur, nil
}

// StorageBytes runs DOLT_GC() to collect unreferenced chunks, then measures
// the on-disk size of this database's directory. Note: CALL DOLT_GC() via SQL
// is equivalent to `dolt gc` without --full; a full reachability sweep is not
// available without stopping the server.
func (b *DoltBackend) StorageBytes(ctx context.Context) (int64, error) {
	if _, err := b.db.ExecContext(ctx, "CALL DOLT_GC()"); err != nil {
		return 0, fmt.Errorf("dolt gc: %w", err)
	}
	return dirBytes(filepath.Join(b.dataDir, b.dbName))
}

func (b *DoltBackend) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, _ = b.db.ExecContext(ctx, "DROP DATABASE IF EXISTS `"+b.dbName+"`")
	return b.db.Close()
}
