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

// Wire-level parity tests, used when the Go driver hides the behavior under
// test. The MongoDB Driver Specification forbids drivers from accepting a
// caller-supplied lsid, so anything that needs a chosen lsid across
// connections speaks OP_MSG directly.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"github.com/dolthub/dumbodb-parity-testing/wire"
	"go.mongodb.org/mongo-driver/bson"
)

func TestTransaction_lsid_reconnect_wire(t *testing.T) {
	runWireParity(t, "lsid_reconnect_wire", harness.DumboDBXFail, func(addr, dbName string) (interface{}, error) {
		lsid := wire.NewLsid()
		const txnNum int64 = 1
		lsidDoc := bson.D{{Key: "id", Value: lsid}}

		c1, err := wire.Dial(addr)
		if err != nil {
			return nil, fmt.Errorf("dial 1: %w", err)
		}
		_ = c1.SetDeadline(time.Now().Add(15 * time.Second))

		insertReply, err := c1.RunCommand(bson.D{
			{Key: "insert", Value: "col"},
			{Key: "documents", Value: bson.A{bson.D{
				{Key: "_id", Value: "p6"},
				{Key: "v", Value: "before-disconnect"},
			}}},
			{Key: "lsid", Value: lsidDoc},
			{Key: "txnNumber", Value: txnNum},
			{Key: "startTransaction", Value: true},
			{Key: "autocommit", Value: false},
			{Key: "$db", Value: dbName},
		})
		_ = c1.Close()
		if err != nil {
			return nil, fmt.Errorf("insert: %w", err)
		}

		c2, err := wire.Dial(addr)
		if err != nil {
			return nil, fmt.Errorf("dial 2: %w", err)
		}
		defer c2.Close()
		_ = c2.SetDeadline(time.Now().Add(15 * time.Second))

		findReply, findErr := c2.RunCommand(bson.D{
			{Key: "find", Value: "col"},
			{Key: "filter", Value: bson.D{{Key: "_id", Value: "p6"}}},
			{Key: "lsid", Value: lsidDoc},
			{Key: "txnNumber", Value: txnNum},
			{Key: "autocommit", Value: false},
			{Key: "$db", Value: dbName},
		})

		commitReply, commitErr := c2.RunCommand(bson.D{
			{Key: "commitTransaction", Value: 1},
			{Key: "lsid", Value: lsidDoc},
			{Key: "txnNumber", Value: txnNum},
			{Key: "autocommit", Value: false},
			{Key: "$db", Value: "admin"},
		})

		freshLsid := wire.NewLsid()
		outsideReply, outsideErr := c2.RunCommand(bson.D{
			{Key: "find", Value: "col"},
			{Key: "filter", Value: bson.D{{Key: "_id", Value: "p6"}}},
			{Key: "lsid", Value: bson.D{{Key: "id", Value: freshLsid}}},
			{Key: "$db", Value: dbName},
		})

		return bson.D{
			{Key: "insertOk", Value: replyOk(insertReply, nil)},
			{Key: "findInReconnectedTxnOk", Value: replyOk(findReply, findErr)},
			{Key: "findInReconnectedTxnCount", Value: cursorFirstBatchLen(findReply)},
			{Key: "commitOk", Value: replyOk(commitReply, commitErr)},
			{Key: "outsideFindOk", Value: replyOk(outsideReply, outsideErr)},
			{Key: "outsideFindCount", Value: cursorFirstBatchLen(outsideReply)},
		}, nil
	})
}

// reply["ok"] may be float64(1) or int32(1); harness normalizer treats them equal.
func replyOk(reply bson.M, err error) interface{} {
	if err != nil {
		return false
	}
	return reply["ok"]
}

// Returns -1 if reply has no cursor (e.g. error reply).
func cursorFirstBatchLen(reply bson.M) int32 {
	cursor, ok := reply["cursor"].(bson.M)
	if !ok {
		return -1
	}
	fb, ok := cursor["firstBatch"].(bson.A)
	if !ok {
		return -1
	}
	return int32(len(fb))
}

func runWireParity(t *testing.T, name string, support harness.DumboDBSupport, fn func(addr, dbName string) (interface{}, error)) {
	t.Helper()

	connectCtx, connectCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer connectCancel()
	clients, err := harness.GetClients(connectCtx)
	if err != nil {
		t.Fatalf("wire test %s: get clients: %v", name, err)
	}

	dbName := fmt.Sprintf("parity_wire_%s_%d", name, time.Now().UnixNano())
	defer func() {
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = clients.Mongo.Database(dbName).Drop(dropCtx)
		_ = clients.DumboDB.Database(dbName).Drop(dropCtx)
	}()

	mongoResult, mongoErr := fn(harness.MongoURI(), dbName)

	if support == harness.DumboDBMongoOnly {
		if mongoErr != nil {
			t.Errorf("MONGO_ONLY %s: mongo error: %v", name, mongoErr)
		} else {
			t.Logf("MONGO_ONLY %s: OK (DumboDB skipped)", name)
		}
		return
	}

	dumboResult, dumboErr := fn(harness.DumboDBURI(), dbName)
	cmp := harness.CompareResponses(mongoResult, mongoErr, dumboResult, dumboErr)

	switch support {
	case harness.DumboDBFull:
		if cmp.Result == harness.Match {
			t.Logf("FULL %s: PASS", name)
		} else {
			t.Errorf("FULL %s: DIVERGE\n%s", name, cmp.Diff)
		}
	case harness.DumboDBXFail:
		if cmp.Result == harness.Match {
			t.Logf("XFAIL %s: PASS (DumboDB matched)", name)
		} else {
			t.Logf("XFAIL %s: diverged as expected\n%s", name, cmp.Diff)
		}
	}
}
