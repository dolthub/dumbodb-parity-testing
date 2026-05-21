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

// All tests that require two independent sessions use two separate mongo.Client
// instances so each session is guaranteed a distinct TCP connection and
// independent server-side conninfo state.

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// errCode mirrors harness/compare.go's unexported errorCode so tests can put
// error codes in returned results.
func errCode(err error) int32 {
	if err == nil {
		return 0
	}
	var cmdErr mongo.CommandError
	if errors.As(err, &cmdErr) {
		return int32(cmdErr.Code)
	}
	var writeExc mongo.WriteException
	if errors.As(err, &writeExc) && len(writeExc.WriteErrors) > 0 {
		return int32(writeExc.WriteErrors[0].Code)
	}
	return 0
}

func sortByID(docs []bson.M) {
	sort.SliceStable(docs, func(i, j int) bool {
		si, _ := docs[i]["_id"].(string)
		sj, _ := docs[j]["_id"].(string)
		return si < sj
	})
}

// secondClient opens a second mongo.Client to the same server as col.
// It uses the server URI injected by the harness into ctx so session B's
// operations land on a separate TCP connection with independent conninfo state.
func secondClient(ctx context.Context) (*mongo.Client, func(), error) {
	uri := harness.ServerURI(ctx)
	if uri == "" {
		return nil, nil, errors.New("secondClient: no server URI in context (test must run via harness.PairTest)")
	}
	c, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, nil, err
	}
	return c, func() { _ = c.Disconnect(ctx) }, nil
}

func TestTransaction_basic_start_commit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "basic_start_commit",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.InsertOne(scA, bson.D{
				{Key: "_id", Value: "p1"},
				{Key: "v", Value: "in-txn"},
			}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			beforeErr := colB.FindOne(ctx, bson.D{{Key: "_id", Value: "p1"}}).Err()
			bSawBefore := beforeErr == nil

			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			var afterDoc bson.M
			afterErr := colB.FindOne(ctx, bson.D{{Key: "_id", Value: "p1"}}).Decode(&afterDoc)
			bSawAfter := afterErr == nil

			return bson.D{
				{Key: "bSawBeforeCommit", Value: bSawBefore},
				{Key: "bSawAfterCommit", Value: bSawAfter},
				{Key: "afterDocId", Value: afterDoc["_id"]},
				{Key: "afterDocV", Value: afterDoc["v"]},
			}, nil
		},
	})
}

func TestTransaction_abort_discards(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "abort_discards",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()

			sessA, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.InsertOne(scA, bson.D{
				{Key: "_id", Value: "p2"},
				{Key: "v", Value: "will-abort"},
			}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			var insideTxn bson.M
			if err := col.FindOne(scA, bson.D{{Key: "_id", Value: "p2"}}).Decode(&insideTxn); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			if err := sessA.AbortTransaction(ctx); err != nil {
				return nil, err
			}

			afterErr := col.FindOne(ctx, bson.D{{Key: "_id", Value: "p2"}}).Err()
			discarded := errors.Is(afterErr, mongo.ErrNoDocuments)

			return bson.D{
				{Key: "insideTxnId", Value: insideTxn["_id"]},
				{Key: "insideTxnV", Value: insideTxn["v"]},
				{Key: "abortedDiscarded", Value: discarded},
			}, nil
		},
	})
}

func TestTransaction_read_your_own_writes(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "read_your_own_writes",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.InsertOne(scA, bson.D{
				{Key: "_id", Value: "p3"},
				{Key: "v", Value: "hello"},
			}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			// A reads its own write inside the txn.
			var aOwn bson.M
			if err := col.FindOne(scA, bson.D{{Key: "_id", Value: "p3"}}).Decode(&aOwn); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			// B reads before commit on a separate connection.
			bBeforeErr := colB.FindOne(ctx, bson.D{{Key: "_id", Value: "p3"}}).Err()
			bSawBefore := bBeforeErr == nil

			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			var bAfter bson.M
			bAfterErr := colB.FindOne(ctx, bson.D{{Key: "_id", Value: "p3"}}).Decode(&bAfter)
			bSawAfter := bAfterErr == nil

			return bson.D{
				{Key: "aReadOwnId", Value: aOwn["_id"]},
				{Key: "aReadOwnV", Value: aOwn["v"]},
				{Key: "bSawBeforeCommit", Value: bSawBefore},
				{Key: "bSawAfterCommit", Value: bSawAfter},
				{Key: "bAfterId", Value: bAfter["_id"]},
			}, nil
		},
	})
}

func TestTransaction_doc_lock_conflict(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "doc_lock_conflict",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "p4"},
				{Key: "x", Value: "original"},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := clientB.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p4"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			// MongoDB's default maxTransactionLockRequestTimeoutMillis is 5ms.
			if err := sessB.StartTransaction(); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}
			scB := mongo.NewSessionContext(ctx, sessB)
			_, bErr := colB.UpdateOne(scB,
				bson.D{{Key: "_id", Value: "p4"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
			)

			_ = sessB.AbortTransaction(ctx)
			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			var final bson.M
			if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "p4"}}).Decode(&final); err != nil {
				return nil, err
			}

			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}

func TestTransaction_non_conflicting_succeed(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "non_conflicting_succeed",
		Support: harness.DumboDBXFail,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := clientB.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

			aStartErr := sessA.StartTransaction()
			scA := mongo.NewSessionContext(ctx, sessA)
			_, aInsertErr := col.InsertOne(scA, bson.D{{Key: "_id", Value: "p5-a"}})

			bStartErr := sessB.StartTransaction()
			scB := mongo.NewSessionContext(ctx, sessB)
			_, bInsertErr := colB.InsertOne(scB, bson.D{{Key: "_id", Value: "p5-b"}})

			aCommitErr := sessA.CommitTransaction(ctx)
			bCommitErr := sessB.CommitTransaction(ctx)

			cur, _ := col.Find(ctx, bson.D{})
			var docs []bson.M
			if cur != nil {
				_ = cur.All(ctx, &docs)
			}
			sortByID(docs)
			ids := make([]interface{}, 0, len(docs))
			for _, d := range docs {
				ids = append(ids, d["_id"])
			}

			return bson.D{
				{Key: "aStartOk", Value: aStartErr == nil},
				{Key: "aInsertOk", Value: aInsertErr == nil},
				{Key: "aInsertCode", Value: errCode(aInsertErr)},
				{Key: "bStartOk", Value: bStartErr == nil},
				{Key: "bInsertOk", Value: bInsertErr == nil},
				{Key: "bInsertCode", Value: errCode(bInsertErr)},
				{Key: "aCommitOk", Value: aCommitErr == nil},
				{Key: "aCommitCode", Value: errCode(aCommitErr)},
				{Key: "bCommitOk", Value: bCommitErr == nil},
				{Key: "bCommitCode", Value: errCode(bCommitErr)},
				{Key: "finalCount", Value: int32(len(docs))},
				{Key: "finalIds", Value: ids},
			}, nil
		},
	})
}

func TestTransaction_concurrent_inserts_preexisting_collection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "concurrent_inserts_preexisting_collection",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := clientB.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

			aStartErr := sessA.StartTransaction()
			scA := mongo.NewSessionContext(ctx, sessA)
			_, aInsertErr := col.InsertOne(scA, bson.D{{Key: "_id", Value: "p5-pre-a"}})

			bStartErr := sessB.StartTransaction()
			scB := mongo.NewSessionContext(ctx, sessB)
			_, bInsertErr := colB.InsertOne(scB, bson.D{{Key: "_id", Value: "p5-pre-b"}})

			aCommitErr := sessA.CommitTransaction(ctx)
			bCommitErr := sessB.CommitTransaction(ctx)

			cur, _ := col.Find(ctx, bson.D{})
			var docs []bson.M
			if cur != nil {
				_ = cur.All(ctx, &docs)
			}
			sortByID(docs)
			ids := make([]interface{}, 0, len(docs))
			for _, d := range docs {
				ids = append(ids, d["_id"])
			}

			return bson.D{
				{Key: "aStartOk", Value: aStartErr == nil},
				{Key: "aInsertOk", Value: aInsertErr == nil},
				{Key: "bStartOk", Value: bStartErr == nil},
				{Key: "bInsertOk", Value: bInsertErr == nil},
				{Key: "aCommitOk", Value: aCommitErr == nil},
				{Key: "bCommitOk", Value: bCommitErr == nil},
				{Key: "finalCount", Value: int32(len(docs))},
				{Key: "finalIds", Value: ids},
			}, nil
		},
	})
}

func TestTransaction_drop_in_txn(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "drop_in_txn",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sess, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer sess.EndSession(ctx)

			startErr := sess.StartTransaction()
			sc := mongo.NewSessionContext(ctx, sess)
			dropErr := col.Drop(sc)
			commitErr := sess.CommitTransaction(ctx)

			names, _ := col.Database().ListCollectionNames(ctx, bson.D{})
			present := false
			for _, n := range names {
				if n == col.Name() {
					present = true
					break
				}
			}

			return bson.D{
				{Key: "startOk", Value: startErr == nil},
				{Key: "dropOk", Value: dropErr == nil},
				{Key: "dropCode", Value: errCode(dropErr)},
				{Key: "commitOk", Value: commitErr == nil},
				{Key: "commitCode", Value: errCode(commitErr)},
				{Key: "collectionExistsAfter", Value: present},
			}, nil
		},
	})
}

func TestTransaction_drop_database_in_txn(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "drop_database_in_txn",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sess, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer sess.EndSession(ctx)

			startErr := sess.StartTransaction()
			sc := mongo.NewSessionContext(ctx, sess)
			dropErr := col.Database().Drop(sc)
			commitErr := sess.CommitTransaction(ctx)

			names, _ := col.Database().Client().ListDatabaseNames(ctx, bson.D{})
			present := false
			for _, n := range names {
				if n == col.Database().Name() {
					present = true
					break
				}
			}

			return bson.D{
				{Key: "startOk", Value: startErr == nil},
				{Key: "dropOk", Value: dropErr == nil},
				{Key: "dropCode", Value: errCode(dropErr)},
				{Key: "commitOk", Value: commitErr == nil},
				{Key: "commitCode", Value: errCode(commitErr)},
				{Key: "databaseExistsAfter", Value: present},
			}, nil
		},
	})
}

func TestTransaction_create_index_in_txn(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "create_index_in_txn",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}, {Key: "x", Value: int32(1)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sess, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer sess.EndSession(ctx)

			startErr := sess.StartTransaction()
			sc := mongo.NewSessionContext(ctx, sess)
			_, createErr := col.Indexes().CreateOne(sc, mongo.IndexModel{
				Keys: bson.D{{Key: "x", Value: int32(1)}},
			})
			commitErr := sess.CommitTransaction(ctx)

			cur, _ := col.Indexes().List(ctx)
			var indexes []bson.M
			if cur != nil {
				_ = cur.All(ctx, &indexes)
			}
			names := make([]interface{}, 0, len(indexes))
			for _, idx := range indexes {
				names = append(names, idx["name"])
			}
			sort.SliceStable(names, func(i, j int) bool {
				si, _ := names[i].(string)
				sj, _ := names[j].(string)
				return si < sj
			})

			return bson.D{
				{Key: "startOk", Value: startErr == nil},
				{Key: "createOk", Value: createErr == nil},
				{Key: "createCode", Value: errCode(createErr)},
				{Key: "commitOk", Value: commitErr == nil},
				{Key: "commitCode", Value: errCode(commitErr)},
				{Key: "indexNamesAfter", Value: names},
			}, nil
		},
	})
}

func TestTransaction_rename_collection_in_txn(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "rename_collection_in_txn",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sess, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer sess.EndSession(ctx)

			oldName := col.Name()
			newName := col.Name() + "_renamed"
			dbName := col.Database().Name()

			startErr := sess.StartTransaction()
			sc := mongo.NewSessionContext(ctx, sess)
			renameErr := col.Database().Client().Database("admin").RunCommand(sc, bson.D{
				{Key: "renameCollection", Value: dbName + "." + oldName},
				{Key: "to", Value: dbName + "." + newName},
			}).Err()
			commitErr := sess.CommitTransaction(ctx)

			names, _ := col.Database().ListCollectionNames(ctx, bson.D{})
			oldPresent, newPresent := false, false
			for _, n := range names {
				if n == oldName {
					oldPresent = true
				}
				if n == newName {
					newPresent = true
				}
			}

			return bson.D{
				{Key: "startOk", Value: startErr == nil},
				{Key: "renameOk", Value: renameErr == nil},
				{Key: "renameCode", Value: errCode(renameErr)},
				{Key: "commitOk", Value: commitErr == nil},
				{Key: "commitCode", Value: errCode(commitErr)},
				{Key: "oldNameExistsAfter", Value: oldPresent},
				{Key: "newNameExistsAfter", Value: newPresent},
			}, nil
		},
	})
}

func TestTransaction_create_collection_existing_in_txn(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "create_collection_existing_in_txn",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: "seed"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			sess, err := col.Database().Client().StartSession()
			if err != nil {
				return nil, err
			}
			defer sess.EndSession(ctx)

			startErr := sess.StartTransaction()
			sc := mongo.NewSessionContext(ctx, sess)
			createErr := col.Database().CreateCollection(sc, col.Name())
			commitErr := sess.CommitTransaction(ctx)

			cnt, _ := col.CountDocuments(ctx, bson.D{})

			return bson.D{
				{Key: "startOk", Value: startErr == nil},
				{Key: "createOk", Value: createErr == nil},
				{Key: "createCode", Value: errCode(createErr)},
				{Key: "commitOk", Value: commitErr == nil},
				{Key: "commitCode", Value: errCode(commitErr)},
				{Key: "seedCountAfter", Value: int32(cnt)},
			}, nil
		},
	})
}

func TestTransaction_endSession_discards(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "endSession_discards",
		Support: harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}

			if err := sessA.StartTransaction(); err != nil {
				sessA.EndSession(ctx)
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.InsertOne(scA, bson.D{
				{Key: "_id", Value: "p8"},
				{Key: "v", Value: "ephemeral"},
			}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				sessA.EndSession(ctx)
				return nil, err
			}

			sessA.EndSession(ctx)

			bErr := colB.FindOne(ctx, bson.D{{Key: "_id", Value: "p8"}}).Err()
			discarded := errors.Is(bErr, mongo.ErrNoDocuments)

			return bson.D{
				{Key: "discardedAfterEndSession", Value: discarded},
			}, nil
		},
	})
}

// TestTransaction_doc_conflict_ignores_lock_timeout pins down empirical
// MongoDB 8.0 behaviour: document-level write conflicts inside a multi-
// document transaction return WriteConflict (112) with the
// TransientTransactionError label essentially immediately, even when
// maxTransactionLockRequestTimeoutMillis is set high. WiredTiger uses
// optimistic concurrency control for per-document conflicts; the lock-
// timeout parameter applies to intent/collection-level lock acquisition,
// not to OCC-detected document conflicts. DumboDB's DocLockManager fails
// fast on conflict, which matches.
func TestTransaction_doc_conflict_ignores_lock_timeout(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "doc_conflict_ignores_lock_timeout",
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "p9"},
				{Key: "x", Value: "original"},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			colB := clientB.Database(col.Database().Name()).Collection(col.Name())

			if err := clientA.Database("admin").RunCommand(ctx, bson.D{
				{Key: "setParameter", Value: 1},
				{Key: "maxTransactionLockRequestTimeoutMillis", Value: int32(5000)},
			}).Err(); err != nil {
				return nil, err
			}
			defer resetLockTimeout(ctx, clientA)()

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := clientB.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p9"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			if err := sessB.StartTransaction(); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}
			scB := mongo.NewSessionContext(ctx, sessB)
			start := time.Now()
			_, bErr := colB.UpdateOne(scB,
				bson.D{{Key: "_id", Value: "p9"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
			)
			elapsed := time.Since(start)

			_ = sessB.AbortTransaction(ctx)
			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			var final bson.M
			if err := col.FindOne(ctx, bson.D{{Key: "_id", Value: "p9"}}).Decode(&final); err != nil {
				return nil, err
			}

			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bReturnedFast", Value: elapsed < 500*time.Millisecond},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}
