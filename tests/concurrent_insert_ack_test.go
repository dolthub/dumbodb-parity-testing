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
	"sync"
	"sync/atomic"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestConcurrentInsert_AcksMatchDocuments: clients racing to insert the same
// _ids get exactly one acknowledged insert per stored document; every other
// attempt is a duplicate key.
func TestConcurrentInsert_AcksMatchDocuments(t *testing.T) {
	const clients, ids = 8, 50
	harness.PairTest(t, harness.TestCase{
		Name:    "ConcurrentInsert_AcksMatchDocuments",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var acked, duplicates, other atomic.Int64
			var wg sync.WaitGroup
			for c := 0; c < clients; c++ {
				wg.Add(1)
				go func(c int) {
					defer wg.Done()
					for i := 0; i < ids; i++ {
						id := int32((i + c*7) % ids)
						_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: id}, {Key: "client", Value: int32(c)}})
						switch {
						case err == nil:
							acked.Add(1)
						case mongo.IsDuplicateKeyError(err):
							duplicates.Add(1)
						default:
							other.Add(1)
						}
					}
				}(c)
			}
			wg.Wait()
			stored, err := col.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			return bson.D{
				{Key: "acked", Value: acked.Load()},
				{Key: "duplicates", Value: duplicates.Load()},
				{Key: "other", Value: other.Load()},
				{Key: "stored", Value: stored},
			}, nil
		},
	})
}
