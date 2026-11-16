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
	"errors"
	"fmt"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod repeats a refused value in full in the first errors of a batch and
// shortens the messages after them, so a batch of large refusals cannot make
// the reply too large to send.
func TestGeoErrorMessagesBatchBudget(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GeoErrorMessagesBatchBudget",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var m bson.D
			for i := 0; i < 60000; i++ {
				m = append(m, bson.E{Key: fmt.Sprintf("k%d", i), Value: primitive.MinKey{}})
			}
			var docs bson.A
			for i := 0; i < 6; i++ {
				docs = append(docs, bson.D{{Key: "_id", Value: i}, {Key: "loc", Value: bson.D{{Key: "type", Value: "Foo"}, {Key: "m", Value: m}}}})
			}
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "insert", Value: col.Name()},
				{Key: "documents", Value: docs},
				{Key: "ordered", Value: false},
			}).Err()
			var we mongo.WriteException
			if !errors.As(err, &we) {
				return nil, err
			}
			full := 0
			for _, e := range we.WriteErrors {
				if len(e.Message) > 1<<20 {
					full++
				}
			}
			errs := we.WriteErrors
			return bson.D{{Key: "errors", Value: len(errs)}, {Key: "full", Value: full}}, nil
		},
	})
}
