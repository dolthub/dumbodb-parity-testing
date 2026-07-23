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
	// Mongo is a connection to a standalone MongoDB. Used for tests whose
	// expected behavior matches a non-replica-set deployment (e.g. compact
	// allowed on primary, validate {repair:true} allowed, $changeStream
	// unsupported).
	Mongo *mongo.Client
	// MongoRS is a connection to the single-node MongoDB replica set the
	// harness provisions, used for tests that require multi-document
	// transaction semantics.
	MongoRS *mongo.Client
	DumboDB *mongo.Client
}

func mongoURI() string {
	return provisioned.mongoURI
}

// mongoRSURI returns the optional MongoDB replica-set URI. Empty when the
// env var is not set; the harness then skips ReplicaSet-topology tests.
func mongoRSURI() string {
	return provisioned.rsURI
}

func dumboDBURI() string {
	return provisioned.dumboURI
}

// MongoURI is the exported view of the MongoDB connection URI used by the
// harness. Wire-level tests (which open their own TCP connections rather than
// going through the driver) use this to dial the same server the driver-based
// tests do.
func MongoURI() string { return mongoURI() }

// MongoRSURI is the exported view of the optional MongoDB replica-set URI.
// Empty when not configured.
func MongoRSURI() string { return mongoRSURI() }

// DumboDBURI is the exported view of the DumboDB connection URI. See MongoURI.
func DumboDBURI() string { return dumboDBURI() }

// GetClients returns the shared Mongo+DumboDB client trio, connecting on first
// call to the servers provisioned by ProvisionServers, including the single-node
// replica set used by transaction/replica-set-topology tests.
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

		rsc, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoRSURI()))
		if err != nil {
			_ = mc.Disconnect(ctx)
			clientsErr = fmt.Errorf("connect mongo replica set: %w", err)
			return
		}
		if err := rsc.Ping(ctx, nil); err != nil {
			_ = mc.Disconnect(ctx)
			_ = rsc.Disconnect(ctx)
			clientsErr = fmt.Errorf("ping mongo replica set: %w", err)
			return
		}

		dc, err := mongo.Connect(ctx, options.Client().ApplyURI(dumboDBURI()))
		if err != nil {
			_ = mc.Disconnect(ctx)
			if rsc != nil {
				_ = rsc.Disconnect(ctx)
			}
			clientsErr = fmt.Errorf("connect dumbodb: %w", err)
			return
		}
		if err := dc.Ping(ctx, nil); err != nil {
			clientsErr = fmt.Errorf("ping dumbodb: %w", err)
			return
		}

		globalClients = &Clients{Mongo: mc, MongoRS: rsc, DumboDB: dc}
	})
	return globalClients, clientsErr
}

// TestDB creates a uniquely-named database for a single test on both servers.
// The mongo side uses the standalone client; see TestDBForTopology for
// replica-set-requiring tests.
func (c *Clients) TestDB(ctx context.Context, testName string) (mongoCol, dumboDBCol *mongo.Collection, cleanup func(), err error) {
	return c.TestDBForTopology(ctx, testName, TopologyStandalone)
}

// TestDBForTopology creates the per-test database pair, picking the Mongo
// client matching the requested topology. The returned cleanup function
// drops both databases; callers should defer it.
//
// If DumboDB is unreachable (e.g. crashed mid-suite), TestDBForTopology
// returns an error immediately rather than blocking for the 30-second
// server-selection timeout. If the requested topology's Mongo client is
// not configured (typically MONGO_RS_URI unset), mongoCol is nil and the
// error is ErrTopologyUnavailable so callers can skip cleanly.
func (c *Clients) TestDBForTopology(ctx context.Context, testName string, topo Topology) (mongoCol, dumboDBCol *mongo.Collection, cleanup func(), err error) {
	mongoClient, ok := c.clientForTopology(topo)
	if !ok {
		return nil, nil, func() {}, ErrTopologyUnavailable
	}

	// Fast health check: if DumboDB crashed after the initial connection, detect it
	// quickly (2s) rather than letting every subsequent test hang for 30s.
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer pingCancel()
	if err := c.DumboDB.Ping(pingCtx, nil); err != nil {
		return nil, nil, func() {}, fmt.Errorf("dumbodb unreachable (crashed?): %w", err)
	}

	dbName := fmt.Sprintf("parity_%s_%d", sanitizeName(testName), time.Now().UnixNano())
	const colName = "col"

	mongoCol = mongoClient.Database(dbName).Collection(colName)
	dumboDBCol = c.DumboDB.Database(dbName).Collection(colName)

	cleanup = func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = mongoClient.Database(dbName).Drop(dropCtx)
		_ = c.DumboDB.Database(dbName).Drop(dropCtx)
	}
	return mongoCol, dumboDBCol, cleanup, nil
}

// clientForTopology returns the Mongo client for the requested topology
// and reports whether one is available.
func (c *Clients) clientForTopology(topo Topology) (*mongo.Client, bool) {
	switch topo {
	case TopologyReplicaSet:
		if c.MongoRS == nil {
			return nil, false
		}
		return c.MongoRS, true
	default:
		return c.Mongo, true
	}
}

// mongoURIForTopology returns the Mongo URI for the requested topology.
// Empty when the topology has no configured URI.
func mongoURIForTopology(topo Topology) string {
	switch topo {
	case TopologyReplicaSet:
		return mongoRSURI()
	default:
		return mongoURI()
	}
}

// ErrTopologyUnavailable is returned by TestDBForTopology when the
// requested Mongo topology is not configured in this environment.
// PairTest translates this into a skip.
var ErrTopologyUnavailable = topologyUnavailableError{}

type topologyUnavailableError struct{}

func (topologyUnavailableError) Error() string {
	return "harness: requested Mongo topology not configured (set MONGO_RS_URI for ReplicaSet)"
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
