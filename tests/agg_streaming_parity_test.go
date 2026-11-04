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
	"math"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

const streamingDocs = 2500

// insertStreamingDocs inserts streamingDocs documents; the last has a scalar
// sub, so a pipeline replacing the root with sub fails only after most of its
// input was processed.
func insertStreamingDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, streamingDocs)
	for i := range docs {
		var sub interface{} = bson.D{{Key: "n", Value: i}}
		if i == streamingDocs-1 {
			sub = 5
		}
		docs[i] = bson.D{{Key: "_id", Value: i}, {Key: "v", Value: i}, {Key: "sub", Value: sub}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func tempOutCollections(ctx context.Context, db *mongo.Database) (int, error) {
	names, err := db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names {
		if strings.HasPrefix(name, "tmp.agg_out.") {
			n++
		}
	}
	return n, nil
}

func runAggregate(ctx context.Context, col *mongo.Collection, pipeline bson.A) error {
	cursor, err := col.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)
	for cursor.Next(ctx) {
	}
	return cursor.Err()
}

func TestAggOut_FailureLeavesTargetUnchanged(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggOut_FailureLeavesTargetUnchanged",
		Support: harness.DumboDBFull,
		Setup:   insertStreamingDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			target := col.Database().Collection("target")
			if _, err := target.InsertOne(ctx, bson.D{{Key: "_id", Value: "old"}}); err != nil {
				return nil, err
			}
			aggErr := runAggregate(ctx, col, bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$sub"}}}},
				bson.D{{Key: "$out", Value: "target"}},
			})
			count, err := target.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			temps, err := tempOutCollections(ctx, col.Database())
			if err != nil {
				return nil, err
			}
			return bson.M{"aggregate": errOutcome(aggErr), "targetCount": count, "tempCollections": temps}, nil
		},
	})
}

func TestAggOut_ReplacesTarget(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggOut_ReplacesTarget",
		Support: harness.DumboDBFull,
		Setup:   insertStreamingDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			target := col.Database().Collection("target")
			if _, err := target.InsertOne(ctx, bson.D{{Key: "_id", Value: "old"}}); err != nil {
				return nil, err
			}
			aggErr := runAggregate(ctx, col, bson.A{bson.D{{Key: "$out", Value: "target"}}})
			count, err := target.CountDocuments(ctx, bson.D{})
			if err != nil {
				return nil, err
			}
			old, err := target.CountDocuments(ctx, bson.D{{Key: "_id", Value: "old"}})
			if err != nil {
				return nil, err
			}
			temps, err := tempOutCollections(ctx, col.Database())
			if err != nil {
				return nil, err
			}
			return bson.M{"aggregate": errOutcome(aggErr), "targetCount": count, "oldLeft": old, "tempCollections": temps}, nil
		},
	})
}

func TestAggMerge_MatchesAcrossBatches(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggMerge_MatchesAcrossBatches",
		Support: harness.DumboDBFull,
		Setup:   insertStreamingDocs,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			target := col.Database().Collection("target")
			if err := runAggregate(ctx, col, bson.A{bson.D{{Key: "$out", Value: "target"}}}); err != nil {
				return nil, err
			}
			aggErr := runAggregate(ctx, col, bson.A{
				bson.D{{Key: "$project", Value: bson.D{{Key: "w", Value: "$v"}}}},
				bson.D{{Key: "$merge", Value: bson.D{{Key: "into", Value: "target"}, {Key: "whenMatched", Value: "merge"}}}},
			})
			merged, err := target.CountDocuments(ctx, bson.D{
				{Key: "w", Value: bson.D{{Key: "$exists", Value: true}}},
				{Key: "v", Value: bson.D{{Key: "$exists", Value: true}}},
			})
			if err != nil {
				return nil, err
			}
			return bson.M{"aggregate": errOutcome(aggErr), "merged": merged}, nil
		},
	})
}

// A streaming stage that fails on a document past the first batch fails the
// getMore, after the first batch was returned.
func TestAggStreaming_ErrorAfterFirstBatch(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggStreaming_ErrorAfterFirstBatch",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := make([]interface{}, 300)
			for i := range docs {
				var sub interface{} = bson.D{{Key: "n", Value: i}}
				if i == 200 {
					sub = 5
				}
				docs[i] = bson.D{{Key: "_id", Value: i}, {Key: "sub", Value: sub}}
			}
			_, err := col.InsertMany(ctx, docs)
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			for name, stage := range map[string]bson.D{
				"replaceRoot": {{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: "$sub"}}}},
			} {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}, stage})
				if err != nil {
					return bson.M{name: bson.M{"aggregate": errOutcome(err)}}, nil
				}
				seen := 0
				for cursor.Next(ctx) {
					seen++
				}
				iterErr := cursor.Err()
				_ = cursor.Close(ctx)
				return bson.M{name: bson.M{"returnedBeforeError": seen, "error": errOutcome(iterErr)}}, nil
			}
			return nil, errors.New("unreachable")
		},
	})
}

func TestAggOperators_BoundsArguments(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggOperators_BoundsArguments",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: 1}, {Key: "s", Value: "hello"}, {Key: "a", Value: bson.A{1, 2, 3}}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			out := bson.M{}
			for name, expr := range map[string]interface{}{
				"slice negative count":    bson.D{{Key: "$slice", Value: bson.A{"$a", 1, -1}}},
				"slice large count":       bson.D{{Key: "$slice", Value: bson.A{"$a", 1, int64(math.MaxInt32)}}},
				"substrBytes huge length": bson.D{{Key: "$substrBytes", Value: bson.A{"$s", 2, int64(math.MaxInt32)}}},
				"substrCP huge length":    bson.D{{Key: "$substrCP", Value: bson.A{"$s", 2, int64(math.MaxInt32)}}},
			} {
				cursor, err := col.Aggregate(ctx, bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}}})
				if err != nil {
					out[name] = errOutcome(err)
					continue
				}
				var docs []bson.M
				err = cursor.All(ctx, &docs)
				if err != nil {
					out[name] = errOutcome(err)
					continue
				}
				out[name] = docs
			}
			return out, nil
		},
	})
}

// $geoNear streams from its geo index, nearest first, so over ~130MB of
// matches it succeeds where an in-memory sort limited to 100MB like $sort
// fails (dolthub/dumbodb#124).
func TestAggGeoNear_LargeResultBeyondSortLimit(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggGeoNear_LargeResultBeyondSortLimit",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			blob := strings.Repeat("x", 100*1024)
			for start := 0; start < 1300; start += 100 {
				docs := make([]interface{}, 100)
				for i := range docs {
					n := start + i
					docs[i] = bson.D{{Key: "_id", Value: n}, {Key: "loc", Value: bson.A{float64(n % 10), 0.0}}, {Key: "blob", Value: blob}}
				}
				if _, err := col.InsertMany(ctx, docs); err != nil {
					return err
				}
			}
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "loc", Value: "2dsphere"}}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$geoNear", Value: bson.D{
					{Key: "near", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{0.0, 0.0}}}},
					{Key: "distanceField", Value: "dist"},
					{Key: "key", Value: "loc"},
					{Key: "spherical", Value: true},
				}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "blob", Value: 0}}}},
			})
			if err != nil {
				return errOutcome(err), nil
			}
			defer cursor.Close(ctx)
			n := 0
			for cursor.Next(ctx) {
				n++
			}
			if err := cursor.Err(); err != nil {
				return errOutcome(err), nil
			}
			return bson.M{"returned": n}, nil
		},
	})
}
