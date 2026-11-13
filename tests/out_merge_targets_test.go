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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// TestOutTargetOptions: $out keeps an existing target's validator (121
// unless bypassed or set to warn), collation and time-series options, and
// refuses a view with 166.
func TestOutTargetOptions(t *testing.T) {
	ci := &options.Collation{Locale: "en", Strength: 2}
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: "a"}, {Key: "t", Value: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}, {Key: "m", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "k", Value: "A"}, {Key: "t", Value: time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC)}, {Key: "m", Value: int32(1)}},
		})
		return err
	}
	type target struct {
		opts *options.CreateCollectionOptions
		view bool
	}
	targets := map[string]target{
		"validator":      {opts: options.CreateCollection().SetValidator(bson.D{{Key: "k", Value: bson.D{{Key: "$type", Value: "int"}}}})},
		"validator_warn": {opts: options.CreateCollection().SetValidator(bson.D{{Key: "k", Value: bson.D{{Key: "$type", Value: "int"}}}}).SetValidationAction("warn")},
		"collation":      {opts: options.CreateCollection().SetCollation(ci)},
		"timeseries":     {opts: options.CreateCollection().SetTimeSeriesOptions(options.TimeSeries().SetTimeField("t").SetMetaField("m"))},
		"view":           {view: true},
	}
	for name, tg := range targets {
		for _, bypass := range []bool{false, true} {
			caseName := name
			if bypass {
				caseName += "_bypass"
			}
			harness.PairTest(t, harness.TestCase{
				Name:    "OutTargetOptions_" + caseName,
				Support: harness.DumboDBFull,
				Setup:   seed,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					db := col.Database()
					out := db.Collection(col.Name() + "_out")
					if err := out.Drop(ctx); err != nil {
						return nil, err
					}
					if tg.view {
						if err := db.CreateView(ctx, out.Name(), col.Name(), bson.A{}); err != nil {
							return nil, err
						}
					} else if err := db.CreateCollection(ctx, out.Name(), tg.opts); err != nil {
						return nil, err
					}
					opts := options.Aggregate()
					if bypass {
						opts.SetBypassDocumentValidation(true)
					}
					cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$out", Value: out.Name()}}}, opts)
					if err != nil {
						return errOutcome(err), nil
					}
					_ = cursor.Close(ctx)
					res := bson.M{}
					found, err := out.Find(ctx, bson.D{{Key: "k", Value: "a"}}, options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}))
					if res["k_a"], err = drainCursor(ctx, found, err); err != nil {
						return nil, err
					}
					indexes, err := out.Indexes().List(ctx)
					var specs []bson.M
					if err == nil {
						err = indexes.All(ctx, &specs)
					}
					if err != nil {
						return nil, err
					}
					names := bson.A{}
					for _, s := range specs {
						names = append(names, s["name"])
					}
					res["indexes"] = names
					return res, nil
				},
			})
		}
	}
}

// TestMergeTargetIDAndIndex: $merge's replace compares _id by value, so 1.0 or
// NumberLong(1) replaces a document whose _id is 1 and is stored; merge
// treats them as a change to the immutable _id. A unique index on the "on"
// fields qualifies when its collation is the aggregate's.
func TestMergeTargetIDAndIndex(t *testing.T) {
	ci := &options.Collation{Locale: "en", Strength: 2}
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: 1.0}, {Key: "a", Value: int32(10)}, {Key: "k", Value: "a"}},
			bson.D{{Key: "_id", Value: int64(2)}, {Key: "a", Value: int32(20)}, {Key: "k", Value: "b"}},
		})
		return err
	}
	prepare := func(ctx context.Context, col *mongo.Collection, indexCollation *options.Collation) (*mongo.Collection, error) {
		target := col.Database().Collection(col.Name() + "_t")
		if err := target.Drop(ctx); err != nil {
			return nil, err
		}
		if _, err := target.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}, {Key: "k", Value: "A"}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: int32(2)}, {Key: "k", Value: "y"}},
		}); err != nil {
			return nil, err
		}
		if indexCollation != nil {
			if _, err := target.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "k", Value: 1}},
				Options: options.Index().SetUnique(true).SetCollation(indexCollation)}); err != nil {
				return nil, err
			}
		}
		return target, nil
	}
	types := bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 1}, {Key: "t", Value: bson.D{{Key: "$type", Value: "$_id"}}}}}}
	cases := map[string]struct {
		merge          bson.D
		indexCollation *options.Collation
		aggCollation   *options.Collation
	}{
		"replace":              {merge: bson.D{{Key: "whenMatched", Value: "replace"}}},
		"merge":                {merge: bson.D{{Key: "whenMatched", Value: "merge"}}},
		"index_same":           {merge: bson.D{{Key: "on", Value: "k"}, {Key: "whenNotMatched", Value: "insert"}}, indexCollation: ci, aggCollation: ci},
		"index_differs":        {merge: bson.D{{Key: "on", Value: "k"}}, indexCollation: ci},
		"index_other_strength": {merge: bson.D{{Key: "on", Value: "k"}}, indexCollation: ci, aggCollation: &options.Collation{Locale: "en", Strength: 1}},
	}
	for name, tc := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "MergeTargetIDAndIndex_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				target, err := prepare(ctx, col, tc.indexCollation)
				if err != nil {
					return nil, err
				}
				spec := append(bson.D{{Key: "into", Value: target.Name()}}, tc.merge...)
				opts := options.Aggregate()
				if tc.aggCollation != nil {
					opts.SetCollation(tc.aggCollation)
				}
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$merge", Value: spec}}}, opts)
				if err != nil {
					return errOutcome(err), nil
				}
				_ = cursor.Close(ctx)
				found, err := target.Aggregate(ctx, bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, types})
				return drainCursor(ctx, found, err)
			},
		})
	}
}
