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

// Index and length operands must be numeric and, for most operators,
// representable as 32-bit integers; DumboDB converted any value
// (workspace-ycs).
func TestIndexOperands(t *testing.T) {
	values := map[string]interface{}{
		"DoubleFraction": 1.5, "DecimalFraction": mustDecimal("1.5"), "DecimalWhole": mustDecimal("2"),
		"String": "1", "Bool": true, "HugeDouble": 3e9, "HugeLong": int64(1) << 40, "NaN": math.NaN(),
		"Inf": math.Inf(1), "NegZero": math.Copysign(0, -1), "Negative": int32(-1), "Null": nil, "LongWhole": int64(2),
	}
	arr := bson.D{{Key: "$literal", Value: bson.A{int32(10), int32(20), int32(30), int32(40)}}}
	op := func(name string, args ...interface{}) bson.D { return bson.D{{Key: name, Value: bson.A(args)}} }
	forms := map[string]func(x interface{}) bson.D{
		"ArrayElemAt":      func(x interface{}) bson.D { return op("$arrayElemAt", arr, x) },
		"SliceCount":       func(x interface{}) bson.D { return op("$slice", arr, x) },
		"SliceStart":       func(x interface{}) bson.D { return op("$slice", arr, x, int32(2)) },
		"SliceN":           func(x interface{}) bson.D { return op("$slice", arr, int32(1), x) },
		"SubstrBytesStart": func(x interface{}) bson.D { return op("$substrBytes", "abcdef", x, int32(2)) },
		"SubstrBytesLen":   func(x interface{}) bson.D { return op("$substrBytes", "abcdef", int32(1), x) },
		"SubstrCPStart":    func(x interface{}) bson.D { return op("$substrCP", "abcdef", x, int32(2)) },
		"SubstrCPLen":      func(x interface{}) bson.D { return op("$substrCP", "abcdef", int32(1), x) },
		"IndexOfArrayFrom": func(x interface{}) bson.D { return op("$indexOfArray", arr, int32(30), x) },
		"IndexOfArrayTo":   func(x interface{}) bson.D { return op("$indexOfArray", arr, int32(30), int32(0), x) },
		"IndexOfBytesFrom": func(x interface{}) bson.D { return op("$indexOfBytes", "abcabc", "c", x) },
		"IndexOfBytesTo":   func(x interface{}) bson.D { return op("$indexOfBytes", "abcabc", "c", int32(0), x) },
		"IndexOfCPFrom":    func(x interface{}) bson.D { return op("$indexOfCP", "abcabc", "c", x) },
	}
	var cases []mathCase
	for form, build := range forms {
		for name, v := range values {
			cases = append(cases, mathCase{form + "_" + name, build(v)})
		}
	}
	runMathCases(t, "IndexOperands", harness.DumboDBXFail, cases)
}
