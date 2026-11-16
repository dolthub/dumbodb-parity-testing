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

// mongod refuses findAndModify with remove and arrayFilters before reading a
// document, even when the list is empty or nothing matches.
func TestFindAndModifyRemoveArrayFilters(t *testing.T) {
	for _, c := range []struct {
		name    string
		query   bson.D
		filters bson.A
	}{
		{"empty", bson.D{{Key: "_id", Value: 1}}, bson.A{}},
		{"empty_no_match", bson.D{{Key: "_id", Value: 9}}, bson.A{}},
		{"one", bson.D{{Key: "_id", Value: 1}}, bson.A{bson.D{{Key: "x", Value: 1}}}},
		{"one_no_match", bson.D{{Key: "_id", Value: 9}}, bson.A{bson.D{{Key: "x", Value: 1}}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "FindAndModifyRemoveArrayFilters_" + c.name,
				Support: harness.DumboDBFull,
				Setup:   insertDocs(bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{1, 2}}}),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						return col.Database().RunCommand(ctx, bson.D{
							{Key: "findAndModify", Value: col.Name()},
							{Key: "query", Value: c.query},
							{Key: "remove", Value: true},
							{Key: "arrayFilters", Value: c.filters},
						}).Err()
					})
				},
			})
		})
	}
}
