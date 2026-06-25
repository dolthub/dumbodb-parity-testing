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

// Behavior U2 of dumbodb
// docs/design/secondary-index-structural-sharing.md.

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func idxmUniqueSetup(ctx context.Context, col *mongo.Collection) error {
	docs := []interface{}{
		bson.D{{Key: "_id", Value: "u1"}, {Key: "f", Value: "alpha"}},
		bson.D{{Key: "_id", Value: "u2"}, {Key: "f", Value: "bravo"}},
	}
	if _, err := col.InsertMany(ctx, docs); err != nil {
		return err
	}
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "f", Value: int32(1)}},
		Options: options.Index().SetUnique(true),
	})
	return err
}

func idxmDupOutcome(err error) bson.D {
	if err == nil {
		return bson.D{{Key: "failed", Value: false}, {Key: "dup", Value: false}}
	}
	dup := mongo.IsDuplicateKeyError(err) ||
		strings.Contains(strings.ToLower(err.Error()), "duplicate")
	return bson.D{{Key: "failed", Value: true}, {Key: "dup", Value: dup}}
}

func TestIndex_UniqueUpdate_SetCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UniqueUpdate_SetCollision",
		Support: harness.DumboDBFull,
		Setup:   idxmUniqueSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, updErr := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "bravo"}}}})
			out := idxmDupOutcome(updErr)

			probes, err := idxmProbe(ctx, col, "alpha", bson.D{{Key: "f", Value: "alpha"}})
			if err != nil {
				return nil, err
			}
			out = append(out, probes...)
			probes, err = idxmProbe(ctx, col, "bravo", bson.D{{Key: "f", Value: "bravo"}})
			if err != nil {
				return nil, err
			}
			out = append(out, probes...)
			return out, nil
		},
	})
}

func TestIndex_UniqueUpdate_ReplaceCollision(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UniqueUpdate_ReplaceCollision",
		Support: harness.DumboDBFull,
		Setup:   idxmUniqueSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, updErr := col.ReplaceOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "_id", Value: "u1"}, {Key: "f", Value: "bravo"}})
			out := idxmDupOutcome(updErr)
			probes, err := idxmProbe(ctx, col, "bravo", bson.D{{Key: "f", Value: "bravo"}})
			if err != nil {
				return nil, err
			}
			out = append(out, probes...)
			return out, nil
		},
	})
}

func TestIndex_UniqueUpdate_NonCollidingSucceeds(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_UniqueUpdate_NonCollidingSucceeds",
		Support: harness.DumboDBFull,
		Setup:   idxmUniqueSetup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u1"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "charlie"}}}}); err != nil {
				return nil, err
			}
			if _, err := col.UpdateOne(ctx,
				bson.D{{Key: "_id", Value: "u2"}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "f", Value: "bravo"}}}}); err != nil {
				return nil, err
			}
			out := bson.D{}
			for _, probe := range []struct {
				label  string
				filter interface{}
			}{
				{"charlie", bson.D{{Key: "f", Value: "charlie"}}},
				{"bravo", bson.D{{Key: "f", Value: "bravo"}}},
			} {
				d, err := idxmProbe(ctx, col, probe.label, probe.filter)
				if err != nil {
					return nil, err
				}
				out = append(out, d...)
			}
			return out, nil
		},
	})
}
