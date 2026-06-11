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
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestTop_AdminOK pins error/no-error and ok=1 parity. DumboDB returns a
// degenerate totals table (no per-collection counters), so we don't compare
// the totals content; the shape and ok flag are what Compass renders against.
func TestTop_AdminOK(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Top_AdminOK",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.M
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "top", Value: int32(1)},
			}).Decode(&doc)
			if err != nil {
				return nil, err
			}
			totals, _ := doc["totals"].(bson.M)
			return bson.D{
				{Key: "ok", Value: doc["ok"]},
				{Key: "note", Value: totals["note"]},
			}, nil
		},
	})
}

func TestTop_NonAdminRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Top_NonAdminRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.M
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "top", Value: int32(1)},
			}).Decode(&doc)
			if err != nil {
				return nil, err
			}
			return doc, nil
		},
	})
}
