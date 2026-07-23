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

func findProjExprSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "p1"},
		{Key: "x", Value: int32(1)},
		{Key: "y", Value: "hello"},
		{Key: "nested", Value: bson.D{{Key: "a", Value: int32(1)}}},
		{Key: "nullField", Value: nil},
	})
	return err
}

func runFindProj(ctx context.Context, col *mongo.Collection, projection bson.D) (interface{}, error) {
	cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	var results []bson.D
	return results, cursor.All(ctx, &results)
}

func TestFindProjExpr_CompassFullProjection(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_CompassFullProjection",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "_id", Value: int32(0)},
				{Key: "__doc", Value: "$$ROOT"},
				{Key: "__size", Value: bson.D{{Key: "$bsonSize", Value: "$$ROOT"}}},
			})
		},
	})
}

func TestFindProjExpr_BareRoot(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_BareRoot",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "__doc", Value: "$$ROOT"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_RootDotField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_RootDotField",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "n", Value: "$$ROOT.nested"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_FieldPath(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_FieldPath",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "copyOfY", Value: "$y"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_NullFieldProjectedAsNull(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_NullFieldProjectedAsNull",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "m", Value: "$nullField"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_NullFieldViaRootDot(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_NullFieldViaRootDot",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "m", Value: "$$ROOT.nullField"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_MissingFieldOmitted(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_MissingFieldOmitted",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "m", Value: "$missing"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_BareLiteralString(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_BareLiteralString",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "foo", Value: "literal"},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}

func TestFindProjExpr_UnsupportedOperatorRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FindProjExpr_UnsupportedOperatorRejected",
		Support: harness.DumboDBFull,
		Setup:   findProjExprSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runFindProj(ctx, col, bson.D{
				{Key: "f", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(2)}}}},
				{Key: "_id", Value: int32(0)},
			})
		},
	})
}
