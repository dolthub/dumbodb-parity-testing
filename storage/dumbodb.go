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
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// DumboDBBackend connects to a running DumboDB instance via the MongoDB wire
// protocol.
//
// Configure via environment variables:
//
//	DUMBODB_URI      - MongoDB URI (default: mongodb://localhost:27018)
//	DUMBODB_DATA_DIR - host path to the DumboDB data directory (required)
//
// DumboDB encodes the active branch in the database name using the "@" separator:
// "dbname@branchname". The backend tracks the current branch in memory and
// reencodes the database name on every operation.
type DumboDBBackend struct {
	client        *mongo.Client
	dbName        string
	currentBranch string
	dataDir       string
}

// NewDumboDBBackend connects to the configured DumboDB instance and allocates a
// fresh test database name. Fails immediately if the server is unreachable or
// DUMBODB_DATA_DIR is not set.
func NewDumboDBBackend(ctx context.Context) (*DumboDBBackend, error) {
	uri := os.Getenv("DUMBODB_URI")
	if uri == "" {
		uri = "mongodb://localhost:27018"
	}
	dataDir := os.Getenv("DUMBODB_DATA_DIR")
	if dataDir == "" {
		return nil, fmt.Errorf("DUMBODB_DATA_DIR is required")
	}

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("dumbodb: connect: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("dumbodb: ping %s: %w", uri, err)
	}

	dbName := fmt.Sprintf("storage_%d", time.Now().UnixNano())
	return &DumboDBBackend{
		client:        client,
		dbName:        dbName,
		currentBranch: "main",
		dataDir:       dataDir,
	}, nil
}

func (b *DumboDBBackend) Name() string { return "DumboDB" }

// encodedDB returns the database name with the current branch encoded via "@".
func (b *DumboDBBackend) encodedDB() string {
	return b.dbName + "@" + b.currentBranch
}

func (b *DumboDBBackend) col() *mongo.Collection {
	return b.client.Database(b.encodedDB()).Collection("users")
}

func (b *DumboDBBackend) Setup(_ context.Context) error {
	// Mongo collections are auto-created on first insert; no secondary
	// indexes (index parity is deferred until DumboDB's index storage
	// path is repaired).
	return nil
}

func (b *DumboDBBackend) InsertBatch(ctx context.Context, docs []Doc) error {
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
		ifaces := make([]interface{}, len(batch))
		for j, d := range batch {
			ifaces[j] = bson.D{
				{Key: "_id", Value: d.ID},
				{Key: "email", Value: d.Email},
				{Key: "name", Value: d.Name},
				{Key: "age", Value: d.Age},
			}
		}
		if _, err := b.col().InsertMany(ctx, ifaces); err != nil {
			return err
		}
	}
	return nil
}

func (b *DumboDBBackend) UpdateEmail(ctx context.Context, id, newEmail string) error {
	_, err := b.col().UpdateOne(ctx,
		bson.D{{Key: "_id", Value: id}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "email", Value: newEmail}}}},
	)
	return err
}

func (b *DumboDBBackend) Commit(ctx context.Context, msg string) error {
	return b.client.Database(b.encodedDB()).RunCommand(ctx, bson.D{
		{Key: "dumboCommit", Value: 1},
		{Key: "message", Value: msg},
	}).Err()
}

func (b *DumboDBBackend) CreateBranch(ctx context.Context, branch string) error {
	return b.client.Database(b.encodedDB()).RunCommand(ctx, bson.D{
		{Key: "dumboBranch", Value: 1},
		{Key: "branch", Value: branch},
	}).Err()
}

// Checkout updates the local branch pointer. DumboDB is stateless; the active
// branch is encoded in the database name on every wire request.
func (b *DumboDBBackend) Checkout(_ context.Context, branch string) error {
	b.currentBranch = branch
	return nil
}

func (b *DumboDBBackend) Merge(ctx context.Context, fromBranch string) (time.Duration, error) {
	start := time.Now()
	err := b.client.Database(b.encodedDB()).RunCommand(ctx, bson.D{
		{Key: "dumboMerge", Value: 1},
		{Key: "mergeIn", Value: fromBranch},
	}).Err()
	dur := time.Since(start)
	if err != nil {
		return 0, err
	}
	return dur, nil
}

// StorageBytes runs dumboGC to collect unreferenced chunks, then measures
// the on-disk size of this database's directory. Mirrors DoltBackend's
// CALL DOLT_GC() step so the size measurement compares post-GC stores
// on both sides. Default mode (no full compaction) for parity with
// dolt's CALL DOLT_GC() which also runs default mode.
func (b *DumboDBBackend) StorageBytes(ctx context.Context) (int64, error) {
	if err := b.client.Database(b.encodedDB()).RunCommand(ctx, bson.D{
		{Key: "dumboGC", Value: 1},
	}).Err(); err != nil {
		return 0, fmt.Errorf("dumbodb gc: %w", err)
	}
	return dirBytes(filepath.Join(b.dataDir, b.dbName))
}

func (b *DumboDBBackend) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = b.client.Database(b.dbName).Drop(ctx)
	return b.client.Disconnect(ctx)
}
