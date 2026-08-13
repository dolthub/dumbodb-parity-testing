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

// Parity matrix for the system variables $$REMOVE, $$ROOT, $$CURRENT and $$NOW
// used in expression position.
//
// $$REMOVE is the target; the siblings are here because all four share one code
// path, so a fix for one resolves all four and a matrix covering only $$REMOVE
// would be half true.
//
// Two failure modes are covered, and the second is the more dangerous: in
// $project DumboDB reports NotImplemented, but in $addFields it stores the
// literal string "$$REMOVE" as the field's value, which is wrong data rather
// than a refusal.

func sysVarSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "s1"},
		{Key: "a", Value: int32(1)},
	})
	return err
}

func sysVarAgg(ctx context.Context, col *mongo.Collection, stage bson.D) (interface{}, error) {
	cursor, err := col.Aggregate(ctx, mongo.Pipeline{stage})
	if err != nil {
		return nil, err
	}

	var results []bson.D
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

func sysVarProject(ctx context.Context, col *mongo.Collection, projection bson.D) (interface{}, error) {
	return sysVarAgg(ctx, col, bson.D{{Key: "$project", Value: projection}})
}

// ---------------------------------------------------------------------------
// $$REMOVE
// ---------------------------------------------------------------------------

// The key must be omitted, not set to null. "keep" survives so that an
// empty result cannot pass this by accident.
func TestSystemVar_RemoveOmitsKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_RemoveOmitsKey",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "keep", Value: "$a"},
				{Key: "x", Value: "$$REMOVE"},
			})
		},
	})
}

// $$REMOVE suppresses its own key only, leaving an inclusion beside it intact.
func TestSystemVar_RemoveKeepsSiblingInclusion(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_RemoveKeepsSiblingInclusion",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "a", Value: 1},
				{Key: "x", Value: "$$REMOVE"},
			})
		},
	})
}

// $addFields is a separate stage path. DumboDB currently stores the literal
// string "$$REMOVE" here rather than dropping the field.
func TestSystemVar_RemoveInAddFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_RemoveInAddFields",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarAgg(ctx, col, bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "x", Value: "$$REMOVE"},
			}}})
		},
	})
}

// The idiomatic use: drop a field conditionally. The taken branch omits.
func TestSystemVar_CondRemoveTaken(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_CondRemoveTaken",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "a", Value: 1},
				{Key: "x", Value: bson.D{{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$gt", Value: bson.A{"$a", int32(0)}}},
					"$$REMOVE",
					"kept",
				}}}},
			})
		},
	})
}

// Control: the untaken branch already matches, so this guards against a fix
// that drops the field regardless of the condition. Full from the start.
func TestSystemVar_CondRemoveNotTaken(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_CondRemoveNotTaken",
		Support: harness.DumboDBFull,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "a", Value: 1},
				{Key: "x", Value: bson.D{{Key: "$cond", Value: bson.A{
					bson.D{{Key: "$lt", Value: bson.A{"$a", int32(0)}}},
					"$$REMOVE",
					"kept",
				}}}},
			})
		},
	})
}

// ---------------------------------------------------------------------------
// $$ROOT, $$CURRENT, $$NOW
// ---------------------------------------------------------------------------

func TestSystemVar_Root(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_Root",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "x", Value: "$$ROOT"},
			})
		},
	})
}

func TestSystemVar_Current(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_Current",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "x", Value: "$$CURRENT"},
			})
		},
	})
}

// The suffix form reaches into the variable rather than returning it whole.
func TestSystemVar_RootSuffixPath(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_RootSuffixPath",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "x", Value: "$$ROOT.a"},
			})
		},
	})
}

func TestSystemVar_RootInAddFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_RootInAddFields",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarAgg(ctx, col, bson.D{{Key: "$addFields", Value: bson.D{
				{Key: "x", Value: "$$ROOT"},
			}}})
		},
	})
}

// $$NOW differs between the two servers by construction, so the type is
// asserted rather than the instant.
func TestSystemVar_NowIsDate(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SystemVar_NowIsDate",
		Support: harness.DumboDBXFail,
		Setup:   sysVarSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return sysVarProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "t", Value: bson.D{{Key: "$type", Value: "$$NOW"}}},
			})
		},
	})
}
