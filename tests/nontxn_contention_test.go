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

// These tests probe how a non-transactional client (B) interacts with a
// document held by another client's (A's) open multi-document transaction.
// MongoDB blocks B's writes until A's transaction commits or aborts;
// reads see the committed pre-image without blocking; operations on
// different documents do not block.

import (
	"context"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// scheduleTxnEnd commits or aborts sessA after delay. The returned channel
// closes when the end has run, so the caller can wait before EndSession.
func scheduleTxnEnd(ctx context.Context, sessA mongo.Session, commit bool, delay time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(delay)
		if commit {
			_ = sessA.CommitTransaction(ctx)
		} else {
			_ = sessA.AbortTransaction(ctx)
		}
	}()
	return done
}

// seedDoc inserts a single document {_id: id, x: "orig"} for tests that
// need a doc to update.
func seedDoc(id string) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertOne(ctx, bson.D{
			{Key: "_id", Value: id},
			{Key: "x", Value: "orig"},
		})
		return err
	}
}

// Case 1: A holds, B (non-txn) updates the same doc, A commits.
// Expected MongoDB: B blocks ~2s, then succeeds. final.x = "B".
func TestNonTxnUpdate_BlocksUntilCommit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnUpdate_BlocksUntilCommit",
		Support:  harness.DumboDBXFail,
		Topology: harness.TopologyReplicaSet,
		Setup:    seedDoc("p"),
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
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			done := scheduleTxnEnd(ctx, sessA, true, 2*time.Second)
			start := time.Now()
			_, bErr := colB.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
			)
			elapsed := time.Since(start)
			<-done

			var final bson.M
			_ = col.FindOne(ctx, bson.D{{Key: "_id", Value: "p"}}).Decode(&final)
			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bWaitedAtLeast1s", Value: elapsed >= 1*time.Second},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}

// Case 2: A holds, B (non-txn) updates the same doc, A aborts.
// Expected MongoDB: B blocks ~2s, then succeeds. final.x = "B".
func TestNonTxnUpdate_BlocksUntilAbort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnUpdate_BlocksUntilAbort",
		Support:  harness.DumboDBXFail,
		Topology: harness.TopologyReplicaSet,
		Setup:    seedDoc("p"),
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
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			done := scheduleTxnEnd(ctx, sessA, false, 2*time.Second)
			start := time.Now()
			_, bErr := colB.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
			)
			elapsed := time.Since(start)
			<-done

			var final bson.M
			_ = col.FindOne(ctx, bson.D{{Key: "_id", Value: "p"}}).Decode(&final)
			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bWaitedAtLeast1s", Value: elapsed >= 1*time.Second},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}

// Case 3: A holds (via update), B (non-txn) deletes the doc, A commits.
// Expected MongoDB: B blocks ~2s, then deletes the (now-committed) doc.
func TestNonTxnDelete_BlocksUntilCommit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnDelete_BlocksUntilCommit",
		Support:  harness.DumboDBXFail,
		Topology: harness.TopologyReplicaSet,
		Setup:    seedDoc("p"),
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
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			done := scheduleTxnEnd(ctx, sessA, true, 2*time.Second)
			start := time.Now()
			delRes, bErr := colB.DeleteOne(ctx, bson.D{{Key: "_id", Value: "p"}})
			elapsed := time.Since(start)
			<-done

			var deletedCount int64
			if delRes != nil {
				deletedCount = delRes.DeletedCount
			}
			cnt, _ := col.CountDocuments(ctx, bson.D{{Key: "_id", Value: "p"}})
			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bWaitedAtLeast1s", Value: elapsed >= 1*time.Second},
				{Key: "bDeletedCount", Value: deletedCount},
				{Key: "finalCount", Value: cnt},
			}, nil
		},
	})
}

// Case 4: A inserts new doc inside txn, B (non-txn) upserts with same _id,
// A commits. Empirically MongoDB does NOT block B's upsert when no
// committed doc exists yet: B's local-read sees no match, B inserts its
// own doc, and A's commit then silently fails with a duplicate-key
// conflict. DumboDB observes the same client-visible outcome.
func TestNonTxnUpsert_RacesWithInsertCommit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnUpsert_RacesWithInsertCommit",
		Support:  harness.DumboDBFull,
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
				{Key: "_id", Value: "p"}, {Key: "x", Value: "A"},
			}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			done := scheduleTxnEnd(ctx, sessA, true, 2*time.Second)
			start := time.Now()
			_, bErr := colB.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
				options.Update().SetUpsert(true),
			)
			elapsed := time.Since(start)
			<-done

			var final bson.M
			_ = col.FindOne(ctx, bson.D{{Key: "_id", Value: "p"}}).Decode(&final)
			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bWaitedAtLeast1s", Value: elapsed >= 1*time.Second},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}

// Case 5: A inserts new doc inside txn, B (non-txn) upserts with same _id,
// A aborts. Empirically MongoDB does NOT block B's upsert here either:
// B sees no committed match and inserts its own doc, then A's abort is
// a no-op for B's row. DumboDB observes the same client-visible outcome.
func TestNonTxnUpsert_RacesWithInsertAbort(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnUpsert_RacesWithInsertAbort",
		Support:  harness.DumboDBFull,
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
				{Key: "_id", Value: "p"}, {Key: "x", Value: "A"},
			}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			done := scheduleTxnEnd(ctx, sessA, false, 2*time.Second)
			start := time.Now()
			_, bErr := colB.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
				options.Update().SetUpsert(true),
			)
			elapsed := time.Since(start)
			<-done

			var final bson.M
			_ = col.FindOne(ctx, bson.D{{Key: "_id", Value: "p"}}).Decode(&final)
			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bWaitedAtLeast1s", Value: elapsed >= 1*time.Second},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}

// Case 6: A holds (via update), B (non-txn) reads the doc.
// Expected MongoDB: B does not block; sees the committed pre-image
// ("orig"). Default read concern is local.
func TestNonTxnRead_DoesNotBlock(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnRead_DoesNotBlock",
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup:    seedDoc("p"),
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
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			start := time.Now()
			var read bson.M
			bErr := colB.FindOne(ctx, bson.D{{Key: "_id", Value: "p"}}).Decode(&read)
			elapsed := time.Since(start)
			_ = sessA.AbortTransaction(ctx)

			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bReturnedFast", Value: elapsed < 500*time.Millisecond},
				{Key: "bSawX", Value: read["x"]},
			}, nil
		},
	})
}

// Case 7: A holds and never commits, B (non-txn) attempts update with
// maxTimeMS: 500. Expected MongoDB: B times out with code 50
// MaxTimeMSExpired. Encoded via RunCommand to attach maxTimeMS to the
// update command directly.
func TestNonTxnUpdate_MaxTimeMSExpires(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnUpdate_MaxTimeMSExpires",
		Support:  harness.DumboDBXFail,
		Topology: harness.TopologyReplicaSet,
		Setup:    seedDoc("p"),
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			clientA := col.Database().Client()
			clientB, closeB, err := secondClient(ctx)
			if err != nil {
				return nil, err
			}
			defer closeB()
			dbB := clientB.Database(col.Database().Name())

			sessA, err := clientA.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			start := time.Now()
			bErr := dbB.RunCommand(ctx, bson.D{
				{Key: "update", Value: col.Name()},
				{Key: "updates", Value: bson.A{bson.D{
					{Key: "q", Value: bson.D{{Key: "_id", Value: "p"}}},
					{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}}},
				}}},
				{Key: "maxTimeMS", Value: int32(500)},
			}).Err()
			elapsed := time.Since(start)
			_ = sessA.AbortTransaction(ctx)

			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bElapsedRoughly500ms", Value: elapsed >= 400*time.Millisecond && elapsed < 2*time.Second},
			}, nil
		},
	})
}

// Case 8: A holds doc "p" in txn, B (non-txn) updates a different doc "q"
// in the same collection. Expected MongoDB: B does not block; doc-level
// locking is per _id, not per collection.
func TestNonTxnUpdate_DifferentDoc_DoesNotBlock(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnUpdate_DifferentDoc_DoesNotBlock",
		Support:  harness.DumboDBFull,
		Topology: harness.TopologyReplicaSet,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: "p"}, {Key: "x", Value: "orig"}},
				bson.D{{Key: "_id", Value: "q"}, {Key: "x", Value: "orig"}},
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

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			start := time.Now()
			_, bErr := colB.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "q"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
			)
			elapsed := time.Since(start)
			_ = sessA.AbortTransaction(ctx)

			var finalQ bson.M
			_ = col.FindOne(ctx, bson.D{{Key: "_id", Value: "q"}}).Decode(&finalQ)

			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bReturnedFast", Value: elapsed < 500*time.Millisecond},
				{Key: "finalQX", Value: finalQ["x"]},
			}, nil
		},
	})
}

// Case 9: A holds (via update), B (non-txn) findAndModify on the same doc,
// A commits. Expected MongoDB: B blocks ~2s like a plain update. final.x = "B".
func TestNonTxnFindAndModify_BlocksUntilCommit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:     "NonTxnFindAndModify_BlocksUntilCommit",
		Support:  harness.DumboDBXFail,
		Topology: harness.TopologyReplicaSet,
		Setup:    seedDoc("p"),
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
			if _, err := col.UpdateOne(scA,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "A"}}}},
			); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			done := scheduleTxnEnd(ctx, sessA, true, 2*time.Second)
			start := time.Now()
			bRes := colB.FindOneAndUpdate(ctx,
				bson.D{{Key: "_id", Value: "p"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "x", Value: "B"}}}},
				options.FindOneAndUpdate().SetReturnDocument(options.After),
			)
			bErr := bRes.Err()
			elapsed := time.Since(start)
			<-done

			var final bson.M
			_ = col.FindOne(ctx, bson.D{{Key: "_id", Value: "p"}}).Decode(&final)
			return bson.D{
				{Key: "bGotError", Value: bErr != nil},
				{Key: "bErrCode", Value: errCode(bErr)},
				{Key: "bWaitedAtLeast1s", Value: elapsed >= 1*time.Second},
				{Key: "finalX", Value: final["x"]},
			}, nil
		},
	})
}

