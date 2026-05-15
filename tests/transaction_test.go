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

// All currently DumboDBXFail: DumboDB has not shipped startTransaction.
// Flip Support to DumboDBFull when the feature lands.

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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

func TestTransaction_basic_start_commit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "basic_start_commit",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()

			sessA, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

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

			scB := mongo.NewSessionContext(ctx, sessB)
			beforeErr := col.FindOne(scB, bson.D{{Key: "_id", Value: "p1"}}).Err()
			bSawBefore := beforeErr == nil

			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			var afterDoc bson.M
			afterErr := col.FindOne(scB, bson.D{{Key: "_id", Value: "p1"}}).Decode(&afterDoc)
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
		Support: harness.DumboDBXFail,
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
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()

			sessA, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

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

			var aOwn bson.M
			if err := col.FindOne(scA, bson.D{{Key: "_id", Value: "p3"}}).Decode(&aOwn); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			scB := mongo.NewSessionContext(ctx, sessB)
			bBeforeErr := col.FindOne(scB, bson.D{{Key: "_id", Value: "p3"}}).Err()
			bSawBefore := bBeforeErr == nil

			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			var bAfter bson.M
			bAfterErr := col.FindOne(scB, bson.D{{Key: "_id", Value: "p3"}}).Decode(&bAfter)
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
		Support: harness.DumboDBXFail,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{
				{Key: "_id", Value: "p4"},
				{Key: "x", Value: "original"},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()

			sessA, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := client.StartSession()
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

			// MongoDB's default maxTransactionLockRequestTimeoutMillis is 5ms,
			// so B's conflicting update returns WriteConflict (112) quickly.
			if err := sessB.StartTransaction(); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}
			scB := mongo.NewSessionContext(ctx, sessB)
			_, bErr := col.UpdateOne(scB,
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
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()

			sessA, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessA.EndSession(ctx)
			sessB, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)

			if err := sessA.StartTransaction(); err != nil {
				return nil, err
			}
			scA := mongo.NewSessionContext(ctx, sessA)
			if _, err := col.InsertOne(scA, bson.D{{Key: "_id", Value: "p5-a"}}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}

			if err := sessB.StartTransaction(); err != nil {
				_ = sessA.AbortTransaction(ctx)
				return nil, err
			}
			scB := mongo.NewSessionContext(ctx, sessB)
			if _, err := col.InsertOne(scB, bson.D{{Key: "_id", Value: "p5-b"}}); err != nil {
				_ = sessA.AbortTransaction(ctx)
				_ = sessB.AbortTransaction(ctx)
				return nil, err
			}

			if err := sessA.CommitTransaction(ctx); err != nil {
				return nil, err
			}
			if err := sessB.CommitTransaction(ctx); err != nil {
				return nil, err
			}

			cur, err := col.Find(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			var docs []bson.M
			if err := cur.All(ctx, &docs); err != nil {
				return nil, err
			}
			sortByID(docs)

			ids := make([]interface{}, 0, len(docs))
			for _, d := range docs {
				ids = append(ids, d["_id"])
			}
			return bson.D{
				{Key: "count", Value: int32(len(docs))},
				{Key: "ids", Value: ids},
			}, nil
		},
	})
}

func TestTransaction_endSession_discards(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "endSession_discards",
		Support: harness.DumboDBXFail,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			client := col.Database().Client()

			sessA, err := client.StartSession()
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

			sessB, err := client.StartSession()
			if err != nil {
				return nil, err
			}
			defer sessB.EndSession(ctx)
			scB := mongo.NewSessionContext(ctx, sessB)
			bErr := col.FindOne(scB, bson.D{{Key: "_id", Value: "p8"}}).Err()
			discarded := errors.Is(bErr, mongo.ErrNoDocuments)

			return bson.D{
				{Key: "discardedAfterEndSession", Value: discarded},
			}, nil
		},
	})
}
