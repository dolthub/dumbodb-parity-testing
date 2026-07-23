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

func runDBAggregate(ctx context.Context, col *mongo.Collection, cmd bson.D) (interface{}, error) {
	result := col.Database().RunCommand(ctx, cmd)
	var doc bson.D
	if err := result.Decode(&doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func TestAggDBLevel_EmptyPipeline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_EmptyPipeline",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_MatchRequiresCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_MatchRequiresCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$match", Value: bson.D{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_CollStatsRequiresCollection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_CollStatsRequiresCollection",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$collStats", Value: bson.D{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_AggregateZeroRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_AggregateZeroRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(0)},
				{Key: "pipeline", Value: bson.A{}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_MissingCursor(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_MissingCursor",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$currentOp", Value: bson.D{}}}}},
			})
		},
	})
}

func TestAggDBLevel_CursorNotObject(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_CursorNotObject",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$currentOp", Value: bson.D{}}}}},
				{Key: "cursor", Value: int32(5)},
			})
		},
	})
}

func TestAggDBLevel_NumberLongOneAccepted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_NumberLongOneAccepted",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int64(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_FloatOneAccepted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_FloatOneAccepted",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: float64(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_DocumentsEmptyArray(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_DocumentsEmptyArray",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_DocumentsInline(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_DocumentsInline",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$documents", Value: bson.A{
					bson.D{{Key: "x", Value: int32(1)}},
					bson.D{{Key: "x", Value: int32(2)}},
				}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_CurrentOp_NonAdminRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_CurrentOp_NonAdminRejected",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$currentOp", Value: bson.D{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

func TestAggDBLevel_CurrentOp_AdminOK(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_CurrentOp_AdminOK",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			var doc bson.M
			err := col.Database().Client().Database("admin").RunCommand(ctx, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$currentOp", Value: bson.D{}}}}},
				{Key: "cursor", Value: bson.D{}},
			}).Decode(&doc)
			if err != nil {
				return nil, err
			}
			return bson.D{{Key: "ok", Value: doc["ok"]}}, nil
		},
	})
}

func TestAggDBLevel_ListLocalSessions(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "AggDBLevel_ListLocalSessions",
		Support: harness.DumboDBFull,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runDBAggregate(ctx, col, bson.D{
				{Key: "aggregate", Value: int32(1)},
				{Key: "pipeline", Value: bson.A{bson.D{{Key: "$listLocalSessions", Value: bson.D{}}}}},
				{Key: "cursor", Value: bson.D{}},
			})
		},
	})
}

