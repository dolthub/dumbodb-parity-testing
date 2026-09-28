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

import (
	"fmt"
	"testing"
	"time"

	"github.com/dolthub/dumbodb-parity-testing/harness"
	"github.com/dolthub/dumbodb-parity-testing/wire"
	"go.mongodb.org/mongo-driver/bson"
)

// A driver only attaches a retryable txnNumber when it is connected to a replica
// set or mongos; against a standalone it never sends one, so the Go driver hides
// this path entirely. Speaking OP_MSG directly lets us force a txnNumber onto a
// standalone and assert both servers refuse it the same way (IllegalOperation,
// 20) instead of silently applying the write -- which would break the
// at-most-once contract retryable writes exist to provide.
func TestRetryableWrite_standalone_rejects_txnNumber_wire(t *testing.T) {
	// Keep the name short: runWireParity derives the database name as
	// parity_wire_<name>_<UnixNano>, and MongoDB rejects a database name over 63
	// bytes with InvalidNamespace(73) before it ever evaluates txnNumber -- which
	// would mask the behavior under test.
	runWireParity(t, "retryable_txn", harness.DumboDBFull, harness.TopologyStandalone,
		func(addr, dbName string) (interface{}, error) {
			c, err := wire.Dial(addr)
			if err != nil {
				return nil, fmt.Errorf("dial: %w", err)
			}
			defer c.Close()
			_ = c.SetDeadline(time.Now().Add(15 * time.Second))

			// Single update carrying lsid + txnNumber with NO autocommit field:
			// the wire shape of a retryable write. A multi-document transaction
			// would carry autocommit:false and is a different path.
			reply, err := c.RunCommand(bson.D{
				{Key: "update", Value: "col"},
				{Key: "updates", Value: bson.A{bson.D{
					{Key: "q", Value: bson.D{{Key: "_id", Value: "r1"}}},
					{Key: "u", Value: bson.D{{Key: "$inc", Value: bson.D{{Key: "v", Value: 1}}}}},
					{Key: "upsert", Value: true},
				}}},
				{Key: "lsid", Value: bson.D{{Key: "id", Value: wire.NewLsid()}}},
				{Key: "txnNumber", Value: int64(1)},
				{Key: "$db", Value: dbName},
			})
			if err != nil {
				return nil, fmt.Errorf("update: %w", err)
			}

			// Compared on ok/code/codeName, not errmsg: MongoDB's message text is
			// not contractual and need not match byte-for-byte.
			return bson.D{
				{Key: "ok", Value: reply["ok"]},
				{Key: "code", Value: reply["code"]},
				{Key: "codeName", Value: reply["codeName"]},
			}, nil
		})
}
