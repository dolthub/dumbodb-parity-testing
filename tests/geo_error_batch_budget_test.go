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
	"strings"
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

// TestGeoErrorMessagesLargeEcho: mongod keeps every message up to about 1 MiB
// in full, and of longer ones keeps the first two and empties the rest.
func TestGeoErrorMessagesLargeEcho(t *testing.T) {
	for _, c := range []struct {
		name string
		keys int
	}{
		{"under_one_mib", 33000},
		{"over_one_mib", 35000},
	} {
		t.Run(c.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "GeoErrorMessagesLargeEcho_" + c.name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					var m bson.D
					for i := 0; i < c.keys; i++ {
						m = append(m, bson.E{Key: fmt.Sprintf("k%d", i), Value: primitive.MinKey{}})
					}
					var docs bson.A
					for i := 0; i < 8; i++ {
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
					full, empty := 0, 0
					for _, e := range we.WriteErrors {
						switch {
						case len(e.Message) > 900000:
							full++
						case e.Message == "":
							empty++
						}
					}
					return bson.D{{Key: "errors", Value: len(we.WriteErrors)}, {Key: "full", Value: full}, {Key: "empty", Value: empty}}, nil
				},
			})
		})
	}
}

// TestGeoErrorMessagesEmptiedEcho: once a reply has kept two messages over
// 1 MiB, mongod empties each later one but still reports a short one in full.
func TestGeoErrorMessagesEmptiedEcho(t *testing.T) {
	var pad bson.A
	for i := 0; i < 7000; i++ {
		pad = append(pad, strings.Repeat("x", 150))
	}
	order := []int{1, 1, 1, 2, 1}
	describe := func(errs []mongo.WriteError) bson.A {
		var out bson.A
		for _, e := range errs {
			switch {
			case len(e.Message) > 900000:
				out = append(out, bson.D{{Key: "index", Value: e.Index}, {Key: "code", Value: e.Code}, {Key: "message", Value: "full"}})
			default:
				out = append(out, bson.D{{Key: "index", Value: e.Index}, {Key: "code", Value: e.Code}, {Key: "message", Value: e.Message}})
			}
		}
		return out
	}
	setup := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}})
		return err
	}
	point := bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{0, 0}}}

	harness.PairTest(t, harness.TestCase{
		Name:    "GeoErrorMessagesEmptiedEcho_update",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if err := setup(ctx, col); err != nil {
				return err
			}
			_, err := col.InsertMany(ctx, []interface{}{
				bson.D{{Key: "_id", Value: 1}, {Key: "loc", Value: point}, {Key: "pad", Value: pad}},
				bson.D{{Key: "_id", Value: 2}, {Key: "loc", Value: point}},
			})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var updates bson.A
			for _, id := range order {
				updates = append(updates, bson.D{
					{Key: "q", Value: bson.D{{Key: "_id", Value: id}}},
					{Key: "u", Value: bson.D{{Key: "$set", Value: bson.D{{Key: "loc", Value: "bad"}}}}},
				})
			}
			var res struct {
				WriteErrors []mongo.WriteError `bson:"writeErrors"`
			}
			err := col.Database().RunCommand(ctx, bson.D{
				{Key: "update", Value: col.Name()},
				{Key: "updates", Value: updates},
				{Key: "ordered", Value: false},
			}).Decode(&res)
			if err != nil {
				return nil, err
			}
			return describe(res.WriteErrors), nil
		},
	})

	harness.PairTest(t, harness.TestCase{
		Name:    "GeoErrorMessagesEmptiedEcho_insert",
		Support: harness.DumboDBFull,
		Setup:   setup,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var docs bson.A
			for i, id := range order {
				doc := bson.D{{Key: "_id", Value: i}, {Key: "loc", Value: "bad"}}
				if id == 1 {
					doc = append(doc, bson.E{Key: "pad", Value: pad})
				}
				docs = append(docs, doc)
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
			return describe(we.WriteErrors), nil
		},
	})
}
