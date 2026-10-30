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
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// A NaN or infinite double summed alongside a Decimal128 panicked DumboDB
// (workspace-8tj.9). mongod carries the special value into the decimal total.

var sumAvgSpecialInputs = []struct {
	name   string
	values bson.A
}{
	{"NaNThenDecimal", bson.A{math.NaN(), mustDecimal("1.5")}},
	{"DecimalThenNaN", bson.A{mustDecimal("1.5"), math.NaN()}},
	{"InfThenDecimal", bson.A{math.Inf(1), mustDecimal("1.5")}},
	{"NegInfThenDecimal", bson.A{math.Inf(-1), mustDecimal("1.5")}},
	{"BothInfDecimal", bson.A{math.Inf(1), math.Inf(-1), mustDecimal("1.5")}},
	{"InfAndDecimalNegInf", bson.A{math.Inf(1), mustDecimal("-Infinity")}},
	{"InfAndDecimalInf", bson.A{math.Inf(1), mustDecimal("Infinity")}},
	{"DecimalNaN", bson.A{int32(1), mustDecimal("NaN")}},
	{"DecimalNaNAndDouble", bson.A{2.5, mustDecimal("NaN"), int64(3)}},
	{"DecimalInf", bson.A{int32(1), mustDecimal("Infinity")}},
	{"DecimalNegInf", bson.A{mustDecimal("-Infinity"), 2.5}},
	{"DecimalBothInf", bson.A{mustDecimal("Infinity"), mustDecimal("-Infinity")}},
	{"NaNIntsDecimal", bson.A{int32(1), int64(2), math.NaN(), mustDecimal("4")}},
	{"DoubleNaNOnly", bson.A{math.NaN(), int32(1)}},
	{"DoubleInfOnly", bson.A{math.Inf(1), int64(1)}},
	{"DoubleBothInf", bson.A{math.Inf(1), math.Inf(-1)}},
	{"InfDecimalNaN", bson.A{math.Inf(-1), mustDecimal("NaN")}},
}

func TestSumAvgSpecialValues(t *testing.T) {
	for _, in := range sumAvgSpecialInputs {
		for _, op := range []string{"$sum", "$avg"} {
			values := in.values
			name := "SumAvgSpecial_" + op[1:] + "_" + in.name

			t.Run(op[1:]+"_Expression_"+in.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name + "_Expression",
					Support: harness.DumboDBFull,
					Setup:   insertOneDoc,
					Run:     projectTyped(bson.D{{Key: op, Value: bson.D{{Key: "$literal", Value: values}}}}),
				})
			})

			t.Run(op[1:]+"_Group_"+in.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name + "_Group",
					Support: harness.DumboDBFull,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						docs := make([]interface{}, len(values))
						for i, v := range values {
							docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "v", Value: v}}
						}
						_, err := col.InsertMany(ctx, docs)
						return err
					},
					Run: pipelineRunExtJSON(bson.A{
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
						bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: op, Value: "$v"}}}}}},
					}),
				})
			})
		}
	}
}

func insertOneDoc(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(0)}})
	return err
}

// The $sum and $avg expressions dropped literal and Decimal128 operands and
// swallowed errors (workspace-ec6).
func TestSumAvgExpressionOperands(t *testing.T) {
	lit := func(v interface{}) bson.D { return bson.D{{Key: "$literal", Value: v}} }
	cases := []struct {
		name    string
		operand interface{}
	}{
		{"LiteralArray", lit(bson.A{int32(1), int64(2), 3.5})},
		{"DecimalLiteral", mustDecimal("1.25")},
		{"DecimalOperands", bson.A{mustDecimal("1.25"), int32(2)}},
		{"ArrayLiteralSingle", bson.A{bson.A{int32(1), int32(2)}}},
		{"ArrayWithOtherOperand", bson.A{lit(bson.A{int32(1), int32(2)}), int32(3)}},
		{"OperatorReturningArray", bson.D{{Key: "$concatArrays", Value: bson.A{bson.A{int32(1)}, bson.A{mustDecimal("2")}}}}},
		{"OperatorError", bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}},
		{"OperatorErrorAmongOperands", bson.A{int32(1), bson.D{{Key: "$divide", Value: bson.A{int32(1), int32(0)}}}}},
		{"NoNumbers", bson.A{"a", true, nil}},
		{"Empty", bson.A{}},
		{"MissingField", "$missing"},
		{"LongOverflow", bson.A{int64(math.MaxInt64), int64(1)}},
		{"IntOverflow", bson.A{int32(math.MaxInt32), int32(1)}},
	}
	for _, tc := range cases {
		for _, op := range []string{"$sum", "$avg"} {
			name := "SumAvgOperands_" + op[1:] + "_" + tc.name
			expr := bson.D{{Key: op, Value: tc.operand}}
			t.Run(op[1:]+"_"+tc.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    name,
					Support: harness.DumboDBFull,
					Setup:   insertOneDoc,
					Run:     projectTyped(expr),
				})
			})
		}
	}
}
