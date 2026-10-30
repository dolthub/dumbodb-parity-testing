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

// Math edge cases from the review of #184 (workspace-m8c.10): Decimal128
// arguments in binary128's subnormal and underflow range, and the value
// shown in out-of-domain error messages.

func projectWithMessage(expr interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: 0}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: expr}}}},
		})
		if err != nil {
			return errOutcomeWithMessage(err), nil
		}
		var docs []bson.Raw
		if err = cursor.All(ctx, &docs); err != nil {
			return errOutcomeWithMessage(err), nil
		}
		return len(docs), nil
	}
}

func TestMathEdgesReview(t *testing.T) {
	op := func(name string, v interface{}) bson.D { return bson.D{{Key: name, Value: v}} }

	type testCase struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}
	var cases []testCase
	for _, fn := range []string{"$asinh", "$sinh", "$tanh", "$sin", "$tan", "$atan", "$asin", "$atanh", "$degreesToRadians"} {
		for _, v := range []string{"1E-4970", "-1E-4970", "1E-4966", "6.475175119438025110924438958227647E-4966", "1E-4950", "3E-4935", "1.234567E-4940", "3.362103143112093506262677817321753E-4932"} {
			cases = append(cases, testCase{fn[1:] + "_" + v, projectTyped(op(fn, mustDecimal(v)))})
		}
	}
	// A field-dependent argument fails at execution in both servers; a
	// constant one fails while mongod optimizes the pipeline (workspace-7aj).
	runtime := func(v interface{}) bson.D {
		return bson.D{{Key: "$add", Value: bson.A{v, bson.D{{Key: "$ifNull", Value: bson.A{"$nope", int32(0)}}}}}}
	}
	for _, c := range []struct {
		name string
		expr interface{}
	}{
		{"AcosMessage", op("$acos", runtime(1.23456789))},
		{"AsinMessage", op("$asin", runtime(-2.5))},
		{"AcosMessageLarge", op("$acos", runtime(123456789.0))},
		{"AcosMessageTiny", op("$acos", runtime(-1.0000000001))},
		{"AtanhMessage", op("$atanh", runtime(1.5))},
		{"AcoshMessage", op("$acosh", runtime(0.123456789))},
		{"SinMessageInfinity", op("$sin", runtime(math.Inf(1)))},
		{"CosMessageInfinity", op("$cos", runtime(math.Inf(-1)))},
		{"AcosMessageDecimal", op("$acos", runtime(mustDecimal("1.234567890123")))},
	} {
		cases = append(cases, testCase{c.name, projectWithMessage(c.expr)})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "MathEdgesReview_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   insertDatePartsDocs,
				Run:     tc.run,
			})
		})
	}
}
