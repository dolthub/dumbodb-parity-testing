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
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// makeTypedRealisticDoc emits a document with the typed shape that real Mongo
// workloads tend to have: ObjectId _id, two Dates, an int32, a second
// ObjectId, a tag array, and a chain of nested sub-documents that carries
// targets at depths 1, 3, and 5 for the mutation-kind benchmarks. The
// payload field is sized so the whole document is approximately the
// requested docSize.
//
// Mutation paths supported on the returned shape (the four kinds the
// bake-off cares about):
//
//	depth 1:  target_d1_arr, target_d1_removable, parent_d1.<new>
//	depth 3:  n1.n2.target_d3_arr, n1.n2.target_d3_removable, n1.n2.<new>
//	depth 5:  n1.n2.n3.n4.target_d5_arr, n1.n2.n3.n4.target_d5_removable,
//	          n1.n2.n3.n4.<new>
//
// The "removable" fields carry a stable scalar so $unset has something to
// remove. The "arr" fields carry a seed of array entries so $pop has
// values to remove and $push grows a pre-existing array.
func makeTypedRealisticDoc(r *rand.Rand, i int, size docSize) bson.D {
	createdAt := time.Unix(int64(1700000000+i), 0)
	updatedAt := createdAt.Add(time.Hour)
	tags := bson.A{
		fmt.Sprintf("tag-%d", i%100),
		fmt.Sprintf("tag-%d", (i+1)%100),
		fmt.Sprintf("tag-%d", (i+2)%100),
	}
	seedArr := bson.A{int32(0), int32(1), int32(2), int32(3), int32(4)}

	// Build the nested chain bottom-up: depth-5 target first, wrap.
	d5 := bson.D{
		{Key: "target_d5_arr", Value: cloneArr(seedArr)},
		{Key: "target_d5_removable", Value: int32(i)},
		{Key: "filler", Value: fmt.Sprintf("d5-%d", i)},
	}
	d4 := bson.D{{Key: "n4", Value: d5}}
	// n1.n2 also hosts the depth-3 targets so n1.n2.target_d3_* paths
	// resolve. n1.n2.n3.n4 carries the depth-5 chain; depth-3 targets sit
	// beside it.
	d2 := bson.D{
		{Key: "target_d3_arr", Value: cloneArr(seedArr)},
		{Key: "target_d3_removable", Value: int32(i)},
		{Key: "n3", Value: d4},
	}
	d1 := bson.D{{Key: "n2", Value: d2}}

	doc := bson.D{
		{Key: "_id", Value: primitive.NewObjectIDFromTimestamp(createdAt)},
		{Key: "createdAt", Value: createdAt},
		{Key: "updatedAt", Value: updatedAt},
		{Key: "version", Value: int32(1)},
		{Key: "userId", Value: primitive.NewObjectIDFromTimestamp(updatedAt)},
		{Key: "tags", Value: tags},
		{Key: "target_d1_arr", Value: cloneArr(seedArr)},
		{Key: "target_d1_removable", Value: int32(i)},
		{Key: "n1", Value: d1},
		{Key: "payload", Value: typedPayload(r, size)},
	}
	return doc
}

// cloneArr returns a fresh copy of a bson.A so callers don't share backing
// storage across documents.
func cloneArr(a bson.A) bson.A {
	out := make(bson.A, len(a))
	copy(out, a)
	return out
}

// typedPayload returns a payload string sized so the encoded document
// approximates the requested docSize. The typed scaffolding above adds
// roughly 600 bytes of overhead at depth 5; we subtract that from the
// target. Values below the floor (sizeSmall) collapse to a minimal
// payload because the typed structure already dominates.
func typedPayload(r *rand.Rand, size docSize) string {
	const typedOverheadBytes = 600
	target := payloadTargetFor(size)
	if target <= typedOverheadBytes {
		return ""
	}
	p := make([]byte, target-typedOverheadBytes)
	for j := range p {
		p[j] = byte('a' + r.Intn(26))
	}
	return string(p)
}

// payloadTargetFor returns the approximate total document size for a
// given docSize. It mirrors the bytes used by makeDoc's payload lengths
// so typed and untyped docs at the same size bucket are comparable.
func payloadTargetFor(size docSize) int {
	switch size {
	case sizeSmall:
		return 100
	case sizeMedium:
		return 1024
	case sizeLarge:
		return 10 * 1024
	case size100KB:
		return 100 * 1024
	case size1MB:
		return 1024 * 1024
	}
	return 1024
}

// makeTypedExtremeDoc emits a document dominated by typed fields: ten
// ObjectIds and ten Dates at the top level, in addition to the same _id
// and payload scaffolding as the realistic shape. Used to stress the
// prefilter substring scan and to make ExtJSON wrapping cost dominate on
// the baseline. Nested chain depths follow the same convention as the
// realistic doc so the same mutation paths apply.
func makeTypedExtremeDoc(r *rand.Rand, i int, size docSize) bson.D {
	doc := makeTypedRealisticDoc(r, i, size)
	insertAt := len(doc) - 1 // before payload
	extras := make(bson.D, 0, 20)
	for j := 0; j < 10; j++ {
		ts := time.Unix(int64(1700000000+i*100+j), 0)
		extras = append(extras,
			bson.E{Key: fmt.Sprintf("oid%d", j), Value: primitive.NewObjectIDFromTimestamp(ts)},
			bson.E{Key: fmt.Sprintf("date%d", j), Value: ts},
		)
	}
	out := make(bson.D, 0, len(doc)+len(extras))
	out = append(out, doc[:insertAt]...)
	out = append(out, extras...)
	out = append(out, doc[insertAt:]...)
	return out
}

// mutationPath returns the dotted mongo path for a mutation at the
// requested depth on the typed-realistic / typed-extreme fixtures.
// suffix names the leaf field. Valid depths: 1, 3, 5.
func mutationPath(depth int, suffix string) string {
	switch depth {
	case 1:
		return suffix
	case 3:
		return fmt.Sprintf("n1.n2.%s", suffix)
	case 5:
		return fmt.Sprintf("n1.n2.n3.n4.%s", suffix)
	default:
		panic(fmt.Sprintf("mutationPath: unsupported depth %d", depth))
	}
}

// seedTypedRealistic inserts n typed-realistic documents into col and
// returns their _id values in insertion order so benchmarks can look them
// up by index. Batched for setup speed; not the timed path.
func seedTypedRealistic(ctx context.Context, col *mongo.Collection, n int, size docSize) ([]primitive.ObjectID, error) {
	r := rand.New(rand.NewSource(*dataSeed))
	ids := make([]primitive.ObjectID, n)
	const batch = 200
	buf := make([]interface{}, 0, batch)
	for i := 0; i < n; i++ {
		doc := makeTypedRealisticDoc(r, i, size)
		// The first element is _id (per makeTypedRealisticDoc).
		ids[i] = doc[0].Value.(primitive.ObjectID)
		buf = append(buf, doc)
		if len(buf) == batch || i == n-1 {
			if _, err := col.InsertMany(ctx, buf, options.InsertMany().SetOrdered(false)); err != nil {
				return nil, fmt.Errorf("seed typed insert: %w", err)
			}
			buf = buf[:0]
		}
	}
	return ids, nil
}

// withSeededTypedRealistic is the typed-realistic counterpart to
// withSeededCollection. Returns the collection plus the slice of inserted
// _id ObjectIDs in insertion order.
func withSeededTypedRealistic(b interface {
	Helper()
	Cleanup(func())
	Fatalf(string, ...interface{})
}, label string, n int, size docSize) (*mongo.Collection, context.Context, []primitive.ObjectID) {
	b.Helper()
	ctx := context.Background()
	client, err := connect(ctx)
	if err != nil {
		b.Fatalf("connect: %v", err)
	}
	col, cleanup := freshCollection(ctx, client, label)
	b.Cleanup(func() {
		cleanup()
		_ = client.Disconnect(context.Background())
	})
	ids, err := seedTypedRealistic(ctx, col, n, size)
	if err != nil {
		b.Fatalf("seed: %v", err)
	}
	return col, ctx, ids
}
