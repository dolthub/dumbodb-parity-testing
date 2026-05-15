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
	"fmt"
	"os"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	globalClients *Clients
	clientsOnce   sync.Once
	clientsErr    error
)

type Clients struct {
	Mongo *mongo.Client
	DumboDB *mongo.Client
}

func mongoURI() string {
	if v := os.Getenv("MONGO_URI"); v != "" {
		return v
	}
	return "mongodb://localhost:27017"
}

func dumboDBURI() string {
	if v := os.Getenv("DUMBODB_URI"); v != "" {
		return v
	}
	return "mongodb://localhost:27018"
}

// MongoURI is the exported view of the MongoDB connection URI used by the
// harness. Wire-level tests (which open their own TCP connections rather than
// going through the driver) use this to dial the same server the driver-based
// tests do.
func MongoURI() string { return mongoURI() }

// DumboDBURI is the exported view of the DumboDB connection URI. See MongoURI.
func DumboDBURI() string { return dumboDBURI() }

// GetClients returns the shared Mongo+DumboDB client pair, connecting on first call.
func GetClients(ctx context.Context) (*Clients, error) {
	clientsOnce.Do(func() {
		mc, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI()))
		if err != nil {
			clientsErr = fmt.Errorf("connect mongo: %w", err)
			return
		}
		if err := mc.Ping(ctx, nil); err != nil {
			clientsErr = fmt.Errorf("ping mongo: %w", err)
			return
		}

		dc, err := mongo.Connect(ctx, options.Client().ApplyURI(dumboDBURI()))
		if err != nil {
			_ = mc.Disconnect(ctx)
			clientsErr = fmt.Errorf("connect dumbodb: %w", err)
			return
		}
		if err := dc.Ping(ctx, nil); err != nil {
			clientsErr = fmt.Errorf("ping dumbodb: %w", err)
			return
		}

		globalClients = &Clients{Mongo: mc, DumboDB: dc}
	})
	return globalClients, clientsErr
}

// TestDB creates a uniquely-named database for a single test on both servers.
// The returned cleanup function drops both databases; callers should defer it.
// If DumboDB is unreachable (e.g. crashed mid-suite), TestDB returns an error
// immediately rather than blocking for the 30-second server-selection timeout.
func (c *Clients) TestDB(ctx context.Context, testName string) (mongoCol, dumboDBCol *mongo.Collection, cleanup func(), err error) {
	// Fast health check: if DumboDB crashed after the initial connection, detect it
	// quickly (2s) rather than letting every subsequent test hang for 30s.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pingCancel()
	if err := c.DumboDB.Ping(pingCtx, nil); err != nil {
		return nil, nil, func() {}, fmt.Errorf("dumbodb unreachable (crashed?): %w", err)
	}

	dbName := fmt.Sprintf("parity_%s_%d", sanitizeName(testName), time.Now().UnixNano())
	const colName = "col"

	mongoCol = c.Mongo.Database(dbName).Collection(colName)
	dumboDBCol = c.DumboDB.Database(dbName).Collection(colName)

	cleanup = func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = c.Mongo.Database(dbName).Drop(dropCtx)
		_ = c.DumboDB.Database(dbName).Drop(dropCtx)
	}
	return mongoCol, dumboDBCol, cleanup, nil
}

// sanitizeName converts a test name to a safe database name component.
// Budget: "parity_" (7) + name + "_" (1) + UnixNano (19) must be ≤ 63, so name ≤ 36.
func sanitizeName(s string) string {
	const maxLen = 36
	out := make([]byte, 0, maxLen)
	for i := 0; i < len(s) && len(out) < maxLen; i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}
