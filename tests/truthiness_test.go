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
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Boolean contexts coerce with Value::coerceToBool, where numeric zeros are
// false; DumboDB treated every number as true (workspace-ptx).
func TestTruthiness(t *testing.T) {
	values := map[string]interface{}{
		"IntZero": int32(0), "LongZero": int64(0), "DoubleZero": 0.0, "NegZero": math.Copysign(0, -1),
		"DecimalZero": mustDecimal("0"), "DecimalZeroScaled": mustDecimal("0.000"), "DecimalNegZero": mustDecimal("-0"),
		"NaN": math.NaN(), "DecimalNaN": mustDecimal("NaN"), "One": int32(1), "EmptyString": "",
		"EmptyArray": bson.A{}, "Null": nil, "False": false,
	}
	for name, v := range values {
		lit := bson.D{{Key: "$literal", Value: v}}
		cases := []mathCase{
			{"Cond_" + name, bson.D{{Key: "$cond", Value: bson.A{lit, "t", "f"}}}},
			{"Not_" + name, bson.D{{Key: "$not", Value: bson.A{lit}}}},
			{"And_" + name, bson.D{{Key: "$and", Value: bson.A{true, lit}}}},
			{"Or_" + name, bson.D{{Key: "$or", Value: bson.A{false, lit}}}},
			{"Switch_" + name, bson.D{{Key: "$switch", Value: bson.D{{Key: "branches", Value: bson.A{bson.D{{Key: "case", Value: lit}, {Key: "then", Value: "t"}}}}, {Key: "default", Value: "f"}}}}},
			{"Filter_" + name, bson.D{{Key: "$filter", Value: bson.D{{Key: "input", Value: bson.A{int32(1)}}, {Key: "cond", Value: lit}}}}},
			{"AllElementsTrue_" + name, bson.D{{Key: "$allElementsTrue", Value: bson.A{bson.A{lit}}}}},
			{"AnyElementTrue_" + name, bson.D{{Key: "$anyElementTrue", Value: bson.A{bson.A{lit}}}}},
		}
		runMathCases(t, "Truthiness", harness.DumboDBXFail, cases)
	}
}
