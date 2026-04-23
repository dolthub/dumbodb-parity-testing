// Package benchmarks measures the per-operation latency of MongoDB wire-protocol
// operations against a single target (MongoDB or DumboDB).
//
// Each benchmark runs against one target at a time, chosen by -bench.target-uri.
// The cmd/compare runner orchestrates dual runs and produces a side-by-side
// comparison table.
package benchmarks

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	targetURI  = flag.String("bench.target-uri", envOr("BENCH_TARGET_URI", "mongodb://localhost:27017"), "URI of the target server (MongoDB or DumboDB)")
	targetName = flag.String("bench.target-name", envOr("BENCH_TARGET_NAME", "target"), "short label for the target, used in database names and reports")
	dataSeed   = flag.Int64("bench.seed", 42, "PRNG seed for deterministic document generation")
)

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// connect dials the configured target URI. It pings once to surface config errors
// before the benchmark timer starts.
func connect(ctx context.Context) (*mongo.Client, error) {
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(*targetURI))
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", *targetURI, err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping %s: %w", *targetURI, err)
	}
	return client, nil
}

// freshCollection returns a collection under a uniquely-named database, plus a
// cleanup that drops the database. Each benchmark gets its own DB so concurrent
// or sequential runs cannot pollute each other's state.
func freshCollection(ctx context.Context, client *mongo.Client, label string) (*mongo.Collection, func()) {
	dbName := fmt.Sprintf("bench_%s_%s_%d", sanitize(*targetName), sanitize(label), time.Now().UnixNano())
	col := client.Database(dbName).Collection("col")
	cleanup := func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = client.Database(dbName).Drop(dropCtx)
	}
	return col, cleanup
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > 24 {
		out = out[:24]
	}
	return out
}

// docSize controls the approximate serialized size of generated documents.
type docSize int

const (
	sizeSmall  docSize = iota // ~100 bytes
	sizeMedium                // ~1 KB
	sizeLarge                 // ~10 KB
	size100KB                 // ~100 KB
	size1MB                   // ~1 MB
)

func (s docSize) String() string {
	switch s {
	case sizeSmall:
		return "small"
	case sizeMedium:
		return "1kb"
	case sizeLarge:
		return "10kb"
	case size100KB:
		return "100kb"
	case size1MB:
		return "1mb"
	}
	return "unknown"
}

// makeDoc generates a deterministic document of roughly the requested size.
// The document shape (fields: _id, i, grp, tag, payload) is the same across
// sizes; only the payload blob grows. This keeps query benchmarks (which filter
// on i/grp/tag) comparable across size tiers.
func makeDoc(r *rand.Rand, i int, size docSize) bson.D {
	var payloadLen int
	switch size {
	case sizeSmall:
		payloadLen = 40
	case sizeMedium:
		payloadLen = 900
	case sizeLarge:
		payloadLen = 9500
	case size100KB:
		payloadLen = 100 * 1024
	case size1MB:
		payloadLen = 1024 * 1024
	}
	payload := make([]byte, payloadLen)
	for j := range payload {
		payload[j] = byte('a' + r.Intn(26))
	}
	return bson.D{
		{Key: "_id", Value: i},
		{Key: "i", Value: i},
		{Key: "grp", Value: i % 10},
		{Key: "tag", Value: fmt.Sprintf("tag-%d", i%100)},
		{Key: "payload", Value: string(payload)},
	}
}

// seedCollection inserts n documents of the given size into col, using a
// deterministic PRNG so runs are reproducible. Uses unordered bulk writes
// for speed — the dataset is setup, not the benchmarked operation.
func seedCollection(ctx context.Context, col *mongo.Collection, n int, size docSize) error {
	r := rand.New(rand.NewSource(*dataSeed))
	const batch = 500
	buf := make([]interface{}, 0, batch)
	for i := 0; i < n; i++ {
		buf = append(buf, makeDoc(r, i, size))
		if len(buf) == batch || i == n-1 {
			if _, err := col.InsertMany(ctx, buf, options.InsertMany().SetOrdered(false)); err != nil {
				return fmt.Errorf("seed insert: %w", err)
			}
			buf = buf[:0]
		}
	}
	return nil
}

// TargetLabel returns the configured target label, suitable for report headers.
func TargetLabel() string { return *targetName }

// TargetURI returns the configured target URI, suitable for report headers.
func TargetURI() string { return *targetURI }
