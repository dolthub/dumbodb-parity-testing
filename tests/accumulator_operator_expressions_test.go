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

// $group accumulators applied to every kind of expression, not just field
// paths: operators, nested operators, document and array literals, system
// variables and expressions that produce a missing value.

func insertAccumulatorExprDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "g", Value: "a"}, {Key: "a", Value: int32(3)}, {Key: "b", Value: 1.5}, {Key: "y", Value: int32(2020)}, {Key: "o", Value: bson.D{{Key: "p", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "g", Value: "a"}, {Key: "a", Value: int32(1)}, {Key: "b", Value: nil}, {Key: "y", Value: int32(1999)}, {Key: "o", Value: bson.D{{Key: "q", Value: int32(2)}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "g", Value: "a"}, {Key: "a", Value: int32(3)}, {Key: "y", Value: int32(2001)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "g", Value: "b"}, {Key: "a", Value: int64(7)}, {Key: "b", Value: "x"}, {Key: "y", Value: nil}, {Key: "o", Value: bson.D{{Key: "p", Value: int32(9)}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "g", Value: "b"}, {Key: "b", Value: mustDecimal("2.5")}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "g", Value: nil}},
	})
	return err
}

// accumulatorExprKnownDivergences are expressions that hit DumboDB gaps
// outside the accumulators. Those marked numericOnly diverge only where the
// accumulator returns the value itself, since $sum, $avg and the standard
// deviations ignore non-numeric input.
var accumulatorExprKnownDivergences = map[string]struct {
	numericOnly bool
	reason      string
}{
	"Nested":          {false, "$multiply over a nested missing value (workspace-bqk)"},
	"ArrayExpression": {true, "$concatArrays does not evaluate array literals (workspace-r5f)"},
	"Root":            {true, "stored field order is not preserved (field_order_test.go)"},
}

func TestAccumulatorOperatorExpressions(t *testing.T) {
	expressions := []struct {
		name string
		expr interface{}
	}{
		{"FieldPath", "$a"},
		{"Literal", bson.D{{Key: "$literal", Value: "$a"}}},
		{"Constant", int32(5)},
		{"Add", bson.D{{Key: "$add", Value: bson.A{"$a", int32(1)}}}},
		{"Nested", bson.D{{Key: "$multiply", Value: bson.A{bson.D{{Key: "$add", Value: bson.A{"$a", int32(1)}}}, 2.5}}}},
		{"CondNull", bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$gt", Value: bson.A{"$a", int32(2)}}}, "$a", nil}}}},
		{"IfNullDefault", bson.D{{Key: "$ifNull", Value: bson.A{"$b", int32(0)}}}},
		{"Remove", bson.D{{Key: "$cond", Value: bson.A{bson.D{{Key: "$gt", Value: bson.A{"$a", int32(2)}}}, "$a", "$$REMOVE"}}}},
		{"DateFromParts", bson.D{{Key: "$dateFromParts", Value: bson.D{{Key: "year", Value: "$y"}}}}},
		{"ToString", bson.D{{Key: "$toString", Value: "$a"}}},
		{"DocumentLiteral", bson.D{{Key: "x", Value: "$a"}, {Key: "y", Value: bson.D{{Key: "$add", Value: bson.A{"$a", int32(1)}}}}}},
		{"DocumentLiteralMissing", bson.D{{Key: "x", Value: "$missing"}, {Key: "y", Value: int32(1)}}},
		{"DocumentLiteralWithArray", bson.D{{Key: "x", Value: bson.A{"$a", bson.D{{Key: "$add", Value: bson.A{"$a", int32(1)}}}}}}},
		{"ArrayExpression", bson.D{{Key: "$concatArrays", Value: bson.A{bson.A{"$a"}, bson.A{"$g"}}}}},
		{"Root", "$$ROOT"},
		{"Subdocument", "$o"},
		{"MergeableExpression", bson.D{{Key: "$ifNull", Value: bson.A{"$o", bson.D{{Key: "none", Value: true}}}}}},
	}
	accumulators := []string{"$sum", "$avg", "$min", "$max", "$first", "$last", "$push", "$addToSet", "$stdDevPop", "$stdDevSamp", "$mergeObjects"}

	for _, acc := range accumulators {
		for _, e := range expressions {
			if acc == "$mergeObjects" && e.name != "DocumentLiteral" && e.name != "DocumentLiteralMissing" &&
				e.name != "Root" && e.name != "Subdocument" && e.name != "MergeableExpression" && e.name != "CondNull" {
				continue
			}
			pipeline := bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$g"}, {Key: "r", Value: bson.D{{Key: acc, Value: e.expr}}}}}},
			}
			if acc == "$addToSet" {
				pipeline = append(pipeline,
					bson.D{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$r"}, {Key: "preserveNullAndEmptyArrays", Value: true}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}, {Key: "r", Value: 1}}}},
					bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$_id"}, {Key: "r", Value: bson.D{{Key: "$push", Value: "$r"}}}}}},
				)
			}
			pipeline = append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})

			name := acc[1:] + "_" + e.name
			support := harness.DumboDBFull
			if d, ok := accumulatorExprKnownDivergences[e.name]; ok {
				numeric := acc == "$sum" || acc == "$avg" || acc == "$stdDevPop" || acc == "$stdDevSamp"
				if !d.numericOnly || !numeric {
					support = harness.DumboDBXFail
				}
			}
			t.Run(name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "AccumulatorExpr_" + name,
					Support: support,
					Setup:   insertAccumulatorExprDocs,
					Run:     pipelineRunExtJSON(pipeline),
				})
			})
		}
	}
}
