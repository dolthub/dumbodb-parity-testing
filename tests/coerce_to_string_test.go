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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// String operators coerce their input with Value::coerceToString; the substr
// operators returned "" or a generic error instead (workspace-d1q).
func TestCoerceToString(t *testing.T) {
	values := map[string]interface{}{
		"Int": int32(-42), "Long": int64(1) << 40, "Double": 1.5, "DoubleBig": 3e9, "DoubleTenth": 0.1,
		"DoubleSevenDigits": 1234567.0, "DoubleTiny": 1e-7, "DoubleWhole": 5.0, "NaN": math.NaN(),
		"Inf": math.Inf(1), "NegInf": math.Inf(-1), "NegZero": math.Copysign(0, -1),
		"Decimal": mustDecimal("1.50"), "DecimalBig": mustDecimal("1E+40"),
		"Date": time.Date(2021, 3, 4, 5, 6, 7, 8e6, time.UTC), "Timestamp": primitive.Timestamp{T: 1500000000, I: 7},
		"Null": nil, "Bool": true, "ObjectID": primitive.NewObjectIDFromTimestamp(time.Unix(0, 0)),
		"Array": bson.A{"a"}, "Document": bson.D{{Key: "a", Value: "b"}}, "Regex": primitive.Regex{Pattern: "a"},
		"Binary": primitive.Binary{Data: []byte("a")}, "MinKey": primitive.MinKey{},
	}
	var cases []mathCase
	for name, v := range values {
		lit := bson.D{{Key: "$literal", Value: v}}
		cases = append(cases,
			mathCase{"Substr_" + name, bson.D{{Key: "$substr", Value: bson.A{lit, int32(0), int32(-1)}}}},
			mathCase{"SubstrBytes_" + name, bson.D{{Key: "$substrBytes", Value: bson.A{lit, int32(0), int32(-1)}}}},
			mathCase{"SubstrCP_" + name, bson.D{{Key: "$substrCP", Value: bson.A{lit, int32(0), int32(100)}}}},
			mathCase{"ToUpper_" + name, bson.D{{Key: "$toUpper", Value: lit}}},
			mathCase{"ToLower_" + name, bson.D{{Key: "$toLower", Value: lit}}},
			mathCase{"Strcasecmp_" + name, bson.D{{Key: "$strcasecmp", Value: bson.A{lit, "a"}}}},
		)
	}
	cases = append(cases, mathCase{"Substr_Missing", bson.D{{Key: "$substr", Value: bson.A{"$missing", int32(0), int32(1)}}}})
	runMathCases(t, "CoerceToString", harness.DumboDBXFail, cases)
}
