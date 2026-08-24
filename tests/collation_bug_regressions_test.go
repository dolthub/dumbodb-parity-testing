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

// Regressions surfaced while writing blog examples:
//   - $match+$group must honor an operation collation (workspace-4jf)
//   - a duplicate-key error must name the offending index, not always _id_
//     (workspace-223)

import (
	"context"
	"regexp"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $match then $group under a collation must count the document $match matches
// (find/count/$count all do; $group must too).
func TestCollation_AggregateGroup_HonorsCollation(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Collation_AggregateGroup_HonorsCollation",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			// An index on the matched field triggers the aggregate count
			// shortcut, which is where the collation was being dropped.
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true).
					SetCollation(&options.Collation{Locale: "en", Strength: 2}),
			}); err != nil {
				return err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: "Alice@x.com"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cur, err := col.Aggregate(ctx, mongo.Pipeline{
				{{Key: "$match", Value: bson.D{{Key: "email", Value: "alice@x.com"}}}},
				{{Key: "$group", Value: bson.D{{Key: "_id", Value: 1}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			}, options.Aggregate().SetCollation(&options.Collation{Locale: "en", Strength: 2}))
			if err != nil {
				return nil, err
			}
			var out []bson.M
			if err := cur.All(ctx, &out); err != nil {
				return nil, err
			}
			var n int64
			if len(out) > 0 {
				switch v := out[0]["n"].(type) {
				case int32:
					n = int64(v)
				case int64:
					n = v
				case float64:
					n = int64(v)
				}
			}
			return bson.D{{Key: "n", Value: n}}, nil
		},
	})
}

var dupIndexRE = regexp.MustCompile(`index: (\S+)`)

// A duplicate-key violation on a secondary unique index must name that index in
// the error, not always report _id_.
func TestIndex_DuplicateKeyError_NamesIndex(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "Index_DuplicateKeyError_NamesIndex",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys: bson.D{{Key: "email", Value: 1}}, Options: options.Index().SetUnique(true),
			}); err != nil {
				return err
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: "a@x.com"}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.InsertOne(ctx, bson.D{{Key: "email", Value: "a@x.com"}})
			idx := ""
			if err != nil {
				if m := dupIndexRE.FindStringSubmatch(err.Error()); len(m) > 1 {
					idx = m[1]
				}
			}
			code, _, _ := harness.CommandErrorCode(err)
			return bson.D{{Key: "code", Value: code}, {Key: "index", Value: idx}}, nil
		},
	})
}
