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
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Op is one named operation a workload can perform against the primary.
//
// Operations are named because coverage is accounted by name: the question
// "which parts of the oplog surface has this suite actually exercised" needs an
// answer, and counting anonymous closures does not give one.
type Op struct {
	Name string
	// Run performs the operation. r is seeded, so a failing workload replays
	// from its seed.
	Run func(ctx context.Context, db *mongo.Database, r *rand.Rand) error
}

// Workload is a named, seeded, ordered set of operations.
type Workload struct {
	Name string
	Seed int64
	Ops  []Op
	// Repeat runs the whole op list this many times. Zero means once.
	Repeat int
	// Concurrency runs that many goroutines over the op list. Zero means one.
	// Interleaving is where ordering defects surface; a serial workload cannot
	// produce them.
	Concurrency int
}

// Coverage records which named operations actually ran, and how many failed.
type Coverage struct {
	mu       sync.Mutex
	Ran      map[string]int
	Failures map[string]int
}

func newCoverage() *Coverage {
	return &Coverage{Ran: map[string]int{}, Failures: map[string]int{}}
}

func (c *Coverage) record(name string, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Ran[name]++
	if err != nil {
		c.Failures[name]++
	}
}

// Names returns the operations that ran, sorted.
func (c *Coverage) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.Ran))
	for name := range c.Ran {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (c *Coverage) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var b strings.Builder
	total := 0
	for _, n := range c.Ran {
		total += n
	}
	fmt.Fprintf(&b, "%d operations across %d kinds", total, len(c.Ran))
	if len(c.Failures) > 0 {
		fmt.Fprintf(&b, "; failures:")
		names := make([]string, 0, len(c.Failures))
		for name := range c.Failures {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, " %s=%d", name, c.Failures[name])
		}
	}
	return b.String()
}

// Run executes the workload against db.
//
// An operation returning an error does not abort the run. Several operations in
// the standard vocabulary are expected to fail sometimes (a duplicate key, an
// update matching nothing), and those failures still produce oplog activity
// worth replicating. Failures are counted so a workload that fails wholesale is
// still visible.
func (w Workload) Run(ctx context.Context, db *mongo.Database) (*Coverage, error) {
	coverage := newCoverage()
	repeat := w.Repeat
	if repeat == 0 {
		repeat = 1
	}
	concurrency := w.Concurrency
	if concurrency == 0 {
		concurrency = 1
	}

	var wg sync.WaitGroup
	errs := make([]error, concurrency)
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			// Each worker gets its own stream derived from the workload seed, so
			// a run is reproducible regardless of goroutine scheduling.
			r := rand.New(rand.NewSource(w.Seed + int64(worker)*7919))
			for i := 0; i < repeat; i++ {
				for _, op := range w.Ops {
					if ctx.Err() != nil {
						errs[worker] = ctx.Err()
						return
					}
					coverage.record(op.Name, op.Run(ctx, db, r))
				}
			}
		}(worker)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return coverage, err
		}
	}
	return coverage, nil
}

// docID returns a deterministic id from the worker's stream.
func docID(r *rand.Rand) string { return fmt.Sprintf("doc-%09d", r.Intn(1_000_000_000)) }

// existingID picks an id likely to already exist, so updates and deletes match
// something. A workload whose updates never match anything exercises far less
// than its operation list suggests.
func existingID(r *rand.Rand) string { return fmt.Sprintf("doc-%09d", r.Intn(200)) }

var words = []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf", "hotel"}

func pickWord(r *rand.Rand) string { return words[r.Intn(len(words))] }

// GenerateDocument builds a document with mixed shape and size: nested
// subdocuments, arrays, and a variable payload. Modelled on cmd/soak's makeDoc,
// but built from bson.D and the seeded stream only, so a workload replays
// exactly from its seed.
func GenerateDocument(r *rand.Rand, id string) bson.D {
	d := bson.D{
		{Key: "_id", Value: id},
		{Key: "score", Value: int32(r.Intn(1000))},
		{Key: "total", Value: int64(r.Intn(1_000_000))},
		{Key: "ratio", Value: float64(r.Intn(10000)) / 100.0},
		{Key: "active", Value: r.Intn(2) == 0},
		{Key: "label", Value: pickWord(r)},
	}
	if r.Intn(2) == 0 {
		tags := bson.A{}
		for i := 0; i < r.Intn(5)+1; i++ {
			tags = append(tags, pickWord(r))
		}
		d = append(d, bson.E{Key: "tags", Value: tags})
	}
	if r.Intn(3) == 0 {
		d = append(d, bson.E{Key: "address", Value: bson.D{
			{Key: "street", Value: fmt.Sprintf("%d %s St", r.Intn(9999), pickWord(r))},
			{Key: "city", Value: pickWord(r)},
			{Key: "zip", Value: fmt.Sprintf("%05d", r.Intn(100000))},
		}})
	}
	if r.Intn(3) == 0 {
		d = append(d, bson.E{Key: "counters", Value: bson.D{
			{Key: "views", Value: int32(r.Intn(1_000_000))},
			{Key: "clicks", Value: int32(r.Intn(10_000))},
		}})
	}
	// items is always present: the positional operators ($[], $[<id>], $) error
	// when the path is missing, so making it optional meant they mostly targeted
	// documents without an array and exercised nothing.
	items := bson.A{}
	for i := 0; i < r.Intn(4)+1; i++ {
		items = append(items, bson.D{
			{Key: "sku", Value: pickWord(r)},
			{Key: "qty", Value: int32(r.Intn(50))},
		})
	}
	d = append(d, bson.E{Key: "items", Value: items})
	// Payload sizes mirror soak's distribution: mostly small, occasionally large
	// enough to cross storage and batching boundaries.
	if pad := payloadSize(r); pad > 0 {
		d = append(d, bson.E{Key: "payload", Value: strings.Repeat("x", pad)})
	}
	return d
}

func payloadSize(r *rand.Rand) int {
	switch n := r.Intn(100); {
	case n < 55:
		return 0
	case n < 80:
		return 256
	case n < 93:
		return 4 * 1024
	case n < 99:
		return 64 * 1024
	default:
		return 512 * 1024
	}
}

func coll(db *mongo.Database) *mongo.Collection { return db.Collection("workload") }

// WriteOps covers the insert, update, delete and replace surface, including
// every update operator the design document lists.
func WriteOps() []Op {
	return []Op{
		{"insertOne", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			// A fresh id: targeting the seeded range makes every insert a
			// duplicate-key error, so the operation runs but replicates nothing.
			_, err := coll(db).InsertOne(ctx, GenerateDocument(r, docID(r)))
			return err
		}},
		{"insertMany", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			docs := make([]interface{}, 0, 5)
			for i := 0; i < 5; i++ {
				docs = append(docs, GenerateDocument(r, docID(r)))
			}
			_, err := coll(db).InsertMany(ctx, docs)
			return err
		}},
		{"insertMany-unordered-with-duplicate", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			// A fresh id twice: the first insert succeeds and replicates, the
			// second collides. A partially failed batch is a distinct oplog
			// shape. Using an already-seeded id would fail both halves and
			// exercise nothing.
			id := docID(r)
			docs := []interface{}{GenerateDocument(r, id), GenerateDocument(r, id)}
			_, err := coll(db).InsertMany(ctx, docs, options.InsertMany().SetOrdered(false))
			// An unordered batch reports a bulk error even though the first
			// insert succeeded and replicated. The duplicate is the point of
			// this operation, so it is not a failure of the operation.
			if mongo.IsDuplicateKeyError(err) {
				return nil
			}
			return err
		}},
		{"update-$set", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$set", Value: bson.D{{Key: "label", Value: pickWord(r)}}}}
		})},
		{"update-$unset", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$unset", Value: bson.D{{Key: "label", Value: ""}}}}
		})},
		{"update-$inc", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(r.Intn(10) - 5)}}}}
		})},
		{"update-$mul", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$mul", Value: bson.D{{Key: "score", Value: int32(2)}}}}
		})},
		{"update-$min", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$min", Value: bson.D{{Key: "score", Value: int32(r.Intn(500))}}}}
		})},
		{"update-$max", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$max", Value: bson.D{{Key: "score", Value: int32(r.Intn(500))}}}}
		})},
		{"update-$rename", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$rename", Value: bson.D{{Key: "label", Value: "moved_label"}}}}
		})},
		{"update-$currentDate", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$currentDate", Value: bson.D{{Key: "touched", Value: true}}}}
		})},
		{"update-$bit", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$bit", Value: bson.D{
				{Key: "flags", Value: bson.D{{Key: "or", Value: int32(1 << uint(r.Intn(8)))}}},
			}}}
		})},
		{"update-$setOnInsert-upsert", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			_, err := coll(db).UpdateOne(ctx,
				bson.D{{Key: "_id", Value: docID(r)}},
				bson.D{
					{Key: "$setOnInsert", Value: bson.D{{Key: "created", Value: pickWord(r)}}},
					{Key: "$set", Value: bson.D{{Key: "seen", Value: int32(1)}}},
				},
				options.Update().SetUpsert(true))
			return err
		}},
		{"updateMany", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			// One command, one oplog entry per matched document. A classic
			// place for a replica to diverge from its source.
			_, err := coll(db).UpdateMany(ctx,
				bson.D{{Key: "score", Value: bson.D{{Key: "$lt", Value: int32(r.Intn(1000))}}}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "bulkTouched", Value: int32(1)}}}})
			return err
		}},
		{"replaceOne", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			id := existingID(r)
			_, err := coll(db).ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: id}}, GenerateDocument(r, id))
			return err
		}},
		{"deleteOne", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			_, err := coll(db).DeleteOne(ctx, bson.D{{Key: "_id", Value: existingID(r)}})
			return err
		}},
		{"deleteMany", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			_, err := coll(db).DeleteMany(ctx,
				bson.D{{Key: "label", Value: pickWord(r)}})
			return err
		}},
		{"findOneAndUpdate", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			err := coll(db).FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: existingID(r)}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "score", Value: int32(1)}}}},
				options.FindOneAndUpdate().SetReturnDocument(options.After)).Err()
			return ignoreNoDocuments(err)
		}},
		{"findOneAndDelete", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			err := coll(db).FindOneAndDelete(ctx, bson.D{{Key: "_id", Value: existingID(r)}}).Err()
			return ignoreNoDocuments(err)
		}},
		{"bulkWrite-mixed", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			_, err := coll(db).BulkWrite(ctx, []mongo.WriteModel{
				mongo.NewInsertOneModel().SetDocument(GenerateDocument(r, docID(r))),
				mongo.NewUpdateOneModel().
					SetFilter(bson.D{{Key: "_id", Value: existingID(r)}}).
					SetUpdate(bson.D{{Key: "$set", Value: bson.D{{Key: "bulk", Value: true}}}}),
				mongo.NewDeleteOneModel().SetFilter(bson.D{{Key: "_id", Value: existingID(r)}}),
			}, options.BulkWrite().SetOrdered(false))
			return err
		}},
	}
}

// ArrayOps covers array mutation, which carries the most intricate $v:2 diff
// encoding and is where delta-application defects hide.
func ArrayOps() []Op {
	return []Op{
		{"array-$push", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: pickWord(r)}}}}
		})},
		{"array-$push-$each-$slice-$sort", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$push", Value: bson.D{{Key: "scores", Value: bson.D{
				{Key: "$each", Value: bson.A{int32(r.Intn(100)), int32(r.Intn(100))}},
				{Key: "$sort", Value: int32(1)},
				{Key: "$slice", Value: int32(5)},
			}}}}}
		})},
		{"array-$push-$position", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$push", Value: bson.D{{Key: "tags", Value: bson.D{
				{Key: "$each", Value: bson.A{pickWord(r)}},
				{Key: "$position", Value: int32(0)},
			}}}}}
		})},
		{"array-$addToSet", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$addToSet", Value: bson.D{{Key: "tags", Value: pickWord(r)}}}}
		})},
		{"array-$pull", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$pull", Value: bson.D{{Key: "tags", Value: pickWord(r)}}}}
		})},
		{"array-$pullAll", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$pullAll", Value: bson.D{{Key: "tags", Value: bson.A{pickWord(r), pickWord(r)}}}}}
		})},
		{"array-$pop", updateOp(func(r *rand.Rand) bson.D {
			side := int32(1)
			if r.Intn(2) == 0 {
				side = -1
			}
			return bson.D{{Key: "$pop", Value: bson.D{{Key: "tags", Value: side}}}}
		})},
		{"array-positional-all", updateOp(func(r *rand.Rand) bson.D {
			return bson.D{{Key: "$inc", Value: bson.D{{Key: "items.$[].qty", Value: int32(1)}}}}
		})},
		{"array-positional-filtered", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			_, err := coll(db).UpdateOne(ctx,
				bson.D{{Key: "_id", Value: existingID(r)}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "items.$[low].qty", Value: int32(99)}}}},
				options.Update().SetArrayFilters(options.ArrayFilters{
					Filters: []interface{}{bson.D{{Key: "low.qty", Value: bson.D{{Key: "$lt", Value: int32(10)}}}}},
				}))
			return err
		}},
		{"array-positional-match", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			_, err := coll(db).UpdateOne(ctx,
				bson.D{{Key: "items.qty", Value: bson.D{{Key: "$gte", Value: int32(0)}}}},
				bson.D{{Key: "$inc", Value: bson.D{{Key: "items.$.qty", Value: int32(1)}}}})
			return err
		}},
	}
}

// CatalogOps covers operations that travel the oplog as commands rather than
// document writes.
func CatalogOps() []Op {
	return []Op{
		{"createCollection", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			return db.CreateCollection(ctx, fmt.Sprintf("made_%s_%d", pickWord(r), r.Intn(1000)))
		}},
		{"createCollection-with-validator", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			name := fmt.Sprintf("validated_%d", r.Intn(1000))
			return db.RunCommand(ctx, bson.D{
				{Key: "create", Value: name},
				{Key: "validator", Value: bson.D{{Key: "score", Value: bson.D{{Key: "$exists", Value: true}}}}},
				{Key: "validationLevel", Value: "strict"},
			}).Err()
		}},
		{"collMod-validator", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			return db.RunCommand(ctx, bson.D{
				{Key: "collMod", Value: "workload"},
				{Key: "validationLevel", Value: []string{"off", "moderate", "strict"}[r.Intn(3)]},
			}).Err()
		}},
		{"createIndex", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			field := []string{"score", "label", "total", "address.city"}[r.Intn(4)]
			_, err := coll(db).Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: field, Value: int32(1)}},
			})
			return err
		}},
		{"dropIndex", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			// Create then drop, so the drop always has a target. Picking a
			// random existing index name meant the operation failed on every
			// attempt in short runs and exercised nothing.
			name := fmt.Sprintf("transient_%d", r.Intn(1_000_000))
			if _, err := coll(db).Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "score", Value: int32(1)}, {Key: "label", Value: int32(-1)}},
				Options: options.Index().SetName(name),
			}); err != nil {
				return err
			}
			_, err := coll(db).Indexes().DropOne(ctx, name)
			return err
		}},
		{"renameCollection", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			src := fmt.Sprintf("rename_src_%d", r.Intn(100))
			dst := fmt.Sprintf("rename_dst_%d", r.Intn(100))
			if err := db.CreateCollection(ctx, src); err != nil {
				return err
			}
			return db.Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "renameCollection", Value: db.Name() + "." + src},
				{Key: "to", Value: db.Name() + "." + dst},
				{Key: "dropTarget", Value: true},
			}).Err()
		}},
		{"dropCollection", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			return db.Collection(fmt.Sprintf("made_%s_%d", pickWord(r), r.Intn(1000))).Drop(ctx)
		}},
	}
}

// TransactionOps covers multi-document transactions, which reach the oplog as
// applyOps entries that may span several records.
func TransactionOps() []Op {
	return []Op{
		{"transaction-commit", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			return runTransaction(ctx, db, r, true)
		}},
		{"transaction-abort", func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
			return runTransaction(ctx, db, r, false)
		}},
	}
}

func runTransaction(ctx context.Context, db *mongo.Database, r *rand.Rand, commit bool) error {
	session, err := db.Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	idA, idB := docID(r), docID(r)
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if _, err := db.Collection("txn_a").InsertOne(sc, GenerateDocument(r, idA)); err != nil {
			return nil, err
		}
		if _, err := db.Collection("txn_b").InsertOne(sc, GenerateDocument(r, idB)); err != nil {
			return nil, err
		}
		if !commit {
			// Aborting must leave no trace on either member.
			return nil, fmt.Errorf("deliberate abort")
		}
		return nil, nil
	})
	if !commit && err != nil && strings.Contains(err.Error(), "deliberate abort") {
		return nil
	}
	return err
}

// StandardVocabulary is every operation in the driver.
func StandardVocabulary() []Op {
	ops := WriteOps()
	ops = append(ops, ArrayOps()...)
	ops = append(ops, CatalogOps()...)
	ops = append(ops, TransactionOps()...)
	return ops
}

func updateOp(build func(r *rand.Rand) bson.D) func(context.Context, *mongo.Database, *rand.Rand) error {
	return func(ctx context.Context, db *mongo.Database, r *rand.Rand) error {
		_, err := coll(db).UpdateOne(ctx, bson.D{{Key: "_id", Value: existingID(r)}}, build(r))
		return err
	}
}

func ignoreNoDocuments(err error) error {
	if err == mongo.ErrNoDocuments {
		return nil
	}
	return err
}

// SeedRand returns a deterministic stream for building fixtures outside a
// workload run.
func SeedRand(seed int64) *rand.Rand { return rand.New(rand.NewSource(seed)) }

// DocIDFor returns the id the workload's update, array and delete operations
// target for index i. Seeding a collection with these ids is what makes those
// operations match something instead of silently doing nothing.
func DocIDFor(i int) string { return fmt.Sprintf("doc-%09d", i) }

// BSONTypeCorpus returns documents covering the BSON types DumboDB can decode,
// one type per document so a divergence names the type.
//
// The types it cannot decode are in UndecodableTypeCorpus, kept separate so a
// single unsupported type does not block verification of the rest. See
// workspace-lhm.
//
// Authored here rather than lifted from tests/bson_types_test.go, which holds
// its values inside ~80 inline closures with no extractable corpus.
func BSONTypeCorpus() []interface{} {
	oid := primitive.NewObjectID()
	dec, _ := primitive.ParseDecimal128("1234.5678901234567890123456789")
	return []interface{}{
		bson.D{{Key: "_id", Value: "t-double"}, {Key: "v", Value: float64(3.14159)}},
		bson.D{{Key: "_id", Value: "t-double-neg-zero"}, {Key: "v", Value: math.Copysign(0, -1)}},
		bson.D{{Key: "_id", Value: "t-double-inf"}, {Key: "v", Value: math.Inf(1)}},
		bson.D{{Key: "_id", Value: "t-string"}, {Key: "v", Value: "hello"}},
		bson.D{{Key: "_id", Value: "t-string-unicode"}, {Key: "v", Value: "é中文\U0001f600"}},
		bson.D{{Key: "_id", Value: "t-string-empty"}, {Key: "v", Value: ""}},
		bson.D{{Key: "_id", Value: "t-object"}, {Key: "v", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: bson.D{{Key: "c", Value: "deep"}}}}}},
		bson.D{{Key: "_id", Value: "t-object-empty"}, {Key: "v", Value: bson.D{}}},
		bson.D{{Key: "_id", Value: "t-array"}, {Key: "v", Value: bson.A{int32(1), "two", 3.0, nil, bson.D{{Key: "k", Value: "v"}}}}},
		bson.D{{Key: "_id", Value: "t-array-empty"}, {Key: "v", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: "t-array-nested"}, {Key: "v", Value: bson.A{bson.A{bson.A{int32(1)}}}}},
		bson.D{{Key: "_id", Value: "t-binary-generic"}, {Key: "v", Value: primitive.Binary{Subtype: 0x00, Data: []byte{0, 1, 2, 255}}}},
		bson.D{{Key: "_id", Value: "t-binary-uuid"}, {Key: "v", Value: primitive.Binary{Subtype: 0x04, Data: make([]byte, 16)}}},
		bson.D{{Key: "_id", Value: "t-binary-md5"}, {Key: "v", Value: primitive.Binary{Subtype: 0x05, Data: make([]byte, 16)}}},
		bson.D{{Key: "_id", Value: "t-objectid"}, {Key: "v", Value: oid}},
		bson.D{{Key: "_id", Value: "t-bool-true"}, {Key: "v", Value: true}},
		bson.D{{Key: "_id", Value: "t-bool-false"}, {Key: "v", Value: false}},
		bson.D{{Key: "_id", Value: "t-date"}, {Key: "v", Value: primitive.NewDateTimeFromTime(time.Unix(1700000000, 0).UTC())}},
		bson.D{{Key: "_id", Value: "t-date-epoch"}, {Key: "v", Value: primitive.NewDateTimeFromTime(time.Unix(0, 0).UTC())}},
		bson.D{{Key: "_id", Value: "t-null"}, {Key: "v", Value: nil}},
		bson.D{{Key: "_id", Value: "t-regex"}, {Key: "v", Value: primitive.Regex{Pattern: "^a.*z$", Options: "im"}}},
		bson.D{{Key: "_id", Value: "t-int32"}, {Key: "v", Value: int32(2147483647)}},
		bson.D{{Key: "_id", Value: "t-int32-min"}, {Key: "v", Value: int32(-2147483648)}},
		bson.D{{Key: "_id", Value: "t-timestamp"}, {Key: "v", Value: primitive.Timestamp{T: 1700000000, I: 7}}},
		bson.D{{Key: "_id", Value: "t-int64"}, {Key: "v", Value: int64(9223372036854775807)}},
		bson.D{{Key: "_id", Value: "t-int64-min"}, {Key: "v", Value: int64(-9223372036854775808)}},
		bson.D{{Key: "_id", Value: "t-decimal128"}, {Key: "v", Value: dec}},
		// MinKey/MaxKey are supported: internal/bson has a dedicated decode path
		// (ToDocumentHandlingMinMaxKey) that bson.ToDocument tries first.
		bson.D{{Key: "_id", Value: "t-minkey"}, {Key: "v", Value: primitive.MinKey{}}},
		bson.D{{Key: "_id", Value: "t-maxkey"}, {Key: "v", Value: primitive.MaxKey{}}},
		// Same numeric value in three widths: a replica that collapses numeric
		// types looks correct until these are compared.
		bson.D{{Key: "_id", Value: "t-num-int32"}, {Key: "v", Value: int32(42)}},
		bson.D{{Key: "_id", Value: "t-num-int64"}, {Key: "v", Value: int64(42)}},
		bson.D{{Key: "_id", Value: "t-num-double"}, {Key: "v", Value: float64(42)}},
		// Field order inside _id is identity in MongoDB, so these are two
		// distinct documents rather than one.
		bson.D{{Key: "_id", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}}, {Key: "v", Value: "ab"}},
		bson.D{{Key: "_id", Value: bson.D{{Key: "b", Value: int32(2)}, {Key: "a", Value: int32(1)}}}, {Key: "v", Value: "ba"}},
	}
}

// LargeDocumentCorpus returns documents that stress size and shape boundaries.
func LargeDocumentCorpus() []interface{} {
	deep := bson.D{{Key: "leaf", Value: int32(1)}}
	for i := 0; i < 40; i++ {
		deep = bson.D{{Key: "n", Value: deep}}
	}
	bigArray := bson.A{}
	for i := 0; i < 5000; i++ {
		bigArray = append(bigArray, int32(i))
	}
	return []interface{}{
		bson.D{{Key: "_id", Value: "big-payload-4mb"}, {Key: "v", Value: strings.Repeat("x", 4*1024*1024)}},
		bson.D{{Key: "_id", Value: "big-payload-12mb"}, {Key: "v", Value: strings.Repeat("y", 12*1024*1024)}},
		bson.D{{Key: "_id", Value: "deep-nesting"}, {Key: "v", Value: deep}},
		bson.D{{Key: "_id", Value: "large-array"}, {Key: "v", Value: bigArray}},
		bson.D{{Key: "_id", Value: "many-fields"}, {Key: "v", Value: manyFields(1000)}},
	}
}

func manyFields(n int) bson.D {
	d := make(bson.D, 0, n)
	for i := 0; i < n; i++ {
		d = append(d, bson.E{Key: fmt.Sprintf("f%04d", i), Value: int32(i)})
	}
	return d
}

// UndecodableTypeCorpus holds values DumboDB's BSON decoder rejects outright
// (github.com/FerretDB/wire wirebson/decode.go). Most are deprecated in
// MongoDB; MinKey, MaxKey and the JavaScript code type are not.
//
// These are kept out of the main corpus deliberately. The question they answer
// is not "does this replicate" but "does the member fail honestly when it
// cannot", which is a different assertion.
func UndecodableTypeCorpus() []interface{} {
	return []interface{}{
		bson.D{{Key: "_id", Value: "u-javascript"}, {Key: "v", Value: primitive.JavaScript("function () { return 1; }")}},
		bson.D{{Key: "_id", Value: "u-symbol"}, {Key: "v", Value: primitive.Symbol("sym")}},
		bson.D{{Key: "_id", Value: "u-undefined"}, {Key: "v", Value: primitive.Undefined{}}},
	}
}
