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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func bsonSizeSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "bs1"},
		{Key: "x", Value: int32(1)},
		{Key: "y", Value: "hello"},
	})
	return err
}

func TestBsonSize_AggProjectRoot(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BsonSize_AggProjectRoot",
		Support: harness.DumboDBFull,
		Setup:   bsonSizeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: "$$ROOT"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestBsonSize_AggProjectLiteralDoc(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BsonSize_AggProjectLiteralDoc",
		Support: harness.DumboDBFull,
		Setup:   bsonSizeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: bson.D{
						{Key: "a", Value: int32(1)},
						{Key: "b", Value: "hi"},
					}}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestBsonSize_AggProjectNull(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BsonSize_AggProjectNull",
		Support: harness.DumboDBFull,
		Setup:   bsonSizeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: nil}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestBsonSize_AggProjectMissingField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BsonSize_AggProjectMissingField",
		Support: harness.DumboDBFull,
		Setup:   bsonSizeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: "$nope"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

func TestBsonSize_FindProjectionRoot(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BsonSize_FindProjectionRoot",
		Support: harness.DumboDBFull,
		Setup:   bsonSizeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{},
				options.Find().SetProjection(bson.D{
					{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: "$$ROOT"}}},
					{Key: "_id", Value: int32(0)},
				}),
			)
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}

// TestBsonSize_AggProjectScalarErrors: both servers return code 31393, but
// MongoDB wraps the operator error with an executor-layer prefix
// ("PlanExecutor error during aggregation :: caused by :: ...") that DumboDB
// does not yet add. Tracked separately as a broader error-wrapping gap.
func TestBsonSize_AggProjectScalarErrors(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "BsonSize_AggProjectScalarErrors",
		Support: harness.DumboDBXFail,
		Setup:   bsonSizeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: "$y"}}},
				}}},
			})
			if err != nil {
				return nil, err
			}
			var results []bson.D
			return results, cursor.All(ctx, &results)
		},
	})
}
