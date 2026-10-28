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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Aggregation expressions over the deprecated undefined type, as a literal
// and as a stored value.

func exprUndefinedSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "f", Value: primitive.Undefined{}}},
	})
	return err
}

func TestExpr_Undefined(t *testing.T) {
	undef := bson.D{{Key: "$literal", Value: primitive.Undefined{}}}
	exprs := []struct {
		name string
		expr interface{}
	}{
		{"EqField", bson.D{{Key: "$eq", Value: bson.A{"$f", undef}}}},
		{"EqNull", bson.D{{Key: "$eq", Value: bson.A{nil, undef}}}},
		{"Cmp", bson.D{{Key: "$cmp", Value: bson.A{"$f", undef}}}},
		{"LtNull", bson.D{{Key: "$lt", Value: bson.A{undef, nil}}}},
		{"Type", bson.D{{Key: "$type", Value: undef}}},
		{"TypeField", bson.D{{Key: "$type", Value: "$f"}}},
		{"IfNull", bson.D{{Key: "$ifNull", Value: bson.A{undef, "x"}}}},
		{"IfNullField", bson.D{{Key: "$ifNull", Value: bson.A{"$f", "x"}}}},
		{"Add", bson.D{{Key: "$add", Value: bson.A{int32(1), undef}}}},
		{"Concat", bson.D{{Key: "$concat", Value: bson.A{"a", undef}}}},
		{"ToBool", bson.D{{Key: "$toBool", Value: undef}}},
		{"In", bson.D{{Key: "$in", Value: bson.A{undef, bson.A{nil}}}}},
		{"Literal", undef},
		{"Field", "$f"},
		{"Cond", bson.D{{Key: "$cond", Value: bson.A{undef, "t", "f"}}}},
		{"And", bson.D{{Key: "$and", Value: bson.A{undef}}}},
		{"Not", bson.D{{Key: "$not", Value: bson.A{undef}}}},
		{"IsNumber", bson.D{{Key: "$isNumber", Value: undef}}},
		{"ToString", bson.D{{Key: "$toString", Value: undef}}},
		{"GtNull", bson.D{{Key: "$gt", Value: bson.A{nil, undef}}}},
		{"EqMissing", bson.D{{Key: "$eq", Value: bson.A{"$nope", undef}}}},
		{"MergeObjects", bson.D{{Key: "$mergeObjects", Value: bson.A{bson.D{{Key: "a", Value: undef}}}}}},
		{"ArrayLiteralOnly", bson.A{undef}},
		{"ArrayOfFields", bson.A{"$_id", "$f", bson.A{"$f"}}},
		{"ArrayOf", bson.A{undef, "$f"}},
	}
	for _, e := range exprs {
		t.Run(e.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Expr_Undefined_" + e.name,
				Support: harness.DumboDBFull,
				Setup:   exprUndefinedSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					cursor, err := col.Aggregate(ctx, bson.A{
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
						bson.D{{Key: "$project", Value: bson.D{{Key: "v", Value: e.expr}}}},
					})
					if err != nil {
						return nil, err
					}
					var results []bson.D
					return results, cursor.All(ctx, &results)
				},
			})
		})
	}
	t.Run("FindTypeUndefined", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Expr_Undefined_FindTypeUndefined",
			Support: harness.DumboDBFull,
			Setup:   exprUndefinedSetup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				out := bson.D{}
				for _, alias := range []interface{}{"undefined", int32(6)} {
					cursor, err := col.Find(ctx, bson.D{{Key: "f", Value: bson.D{{Key: "$type", Value: alias}}}},
						options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}))
					if err != nil {
						return nil, err
					}
					var results []bson.D
					if err := cursor.All(ctx, &results); err != nil {
						return nil, err
					}
					out = append(out, bson.E{Key: "q", Value: results})
				}
				return out, nil
			},
		})
	})
	t.Run("FindExpr", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Expr_Undefined_FindExpr",
			Support: harness.DumboDBFull,
			Setup:   exprUndefinedSetup,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$f", undef}}}}},
					options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}))
				if err != nil {
					return nil, err
				}
				var results []bson.D
				return results, cursor.All(ctx, &results)
			},
		})
	})
}
