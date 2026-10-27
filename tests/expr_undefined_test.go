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
		name    string
		expr    interface{}
		support harness.DumboDBSupport
	}{
		{"EqField", bson.D{{Key: "$eq", Value: bson.A{"$f", undef}}}, harness.DumboDBXFail},
		{"EqNull", bson.D{{Key: "$eq", Value: bson.A{nil, undef}}}, harness.DumboDBFull},
		{"Cmp", bson.D{{Key: "$cmp", Value: bson.A{"$f", undef}}}, harness.DumboDBXFail},
		{"LtNull", bson.D{{Key: "$lt", Value: bson.A{undef, nil}}}, harness.DumboDBFull},
		{"Type", bson.D{{Key: "$type", Value: undef}}, harness.DumboDBXFail},
		{"TypeField", bson.D{{Key: "$type", Value: "$f"}}, harness.DumboDBXFail},
		{"IfNull", bson.D{{Key: "$ifNull", Value: bson.A{undef, "x"}}}, harness.DumboDBXFail},
		{"IfNullField", bson.D{{Key: "$ifNull", Value: bson.A{"$f", "x"}}}, harness.DumboDBXFail},
		{"Add", bson.D{{Key: "$add", Value: bson.A{int32(1), undef}}}, harness.DumboDBXFail},
		{"Concat", bson.D{{Key: "$concat", Value: bson.A{"a", undef}}}, harness.DumboDBXFail},
		{"ToBool", bson.D{{Key: "$toBool", Value: undef}}, harness.DumboDBXFail},
		{"In", bson.D{{Key: "$in", Value: bson.A{undef, bson.A{nil}}}}, harness.DumboDBFull},
		{"Literal", undef, harness.DumboDBFull},
		{"Field", "$f", harness.DumboDBFull},
		{"ArrayOf", bson.A{undef, "$f"}, harness.DumboDBXFail},
	}
	for _, e := range exprs {
		t.Run(e.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Expr_Undefined_" + e.name,
				Support: e.support,
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
	t.Run("FindExpr", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Expr_Undefined_FindExpr",
			Support: harness.DumboDBXFail,
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
