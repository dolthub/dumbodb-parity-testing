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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// count parses limit and skip with its own rules: a negative limit counts by
// its absolute value, a fractional limit is an error, and skip truncates a
// double but rounds a Decimal128.

func countLimitSkipSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(3)}},
	})
	return err
}

func TestCountLimitSkip(t *testing.T) {
	dec := func(s string) primitive.Decimal128 {
		d, err := primitive.ParseDecimal128(s)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	cases := []struct {
		name string
		opts bson.D
	}{
		{"Count_LimitNegativeOne", bson.D{{Key: "limit", Value: int32(-1)}}},
		{"Count_LimitNegativeWithSkip", bson.D{{Key: "limit", Value: int32(-2)}, {Key: "skip", Value: int32(2)}}},
		{"Count_LimitNegativeWithQuery", bson.D{{Key: "query", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$gt", Value: int32(0)}}}}}, {Key: "limit", Value: int64(-2)}}},
		{"Count_LimitNegativeDecimal", bson.D{{Key: "limit", Value: dec("-2")}}},
		{"Count_LimitNegativeDouble", bson.D{{Key: "limit", Value: -2.0}}},
		{"Count_LimitMinLong", bson.D{{Key: "limit", Value: int64(math.MinInt64)}}},
		{"Count_LimitFraction", bson.D{{Key: "limit", Value: 2.5}}},
		{"Count_LimitNegativeFraction", bson.D{{Key: "limit", Value: -0.5}}},
		{"Count_LimitDecimalFraction", bson.D{{Key: "limit", Value: dec("2.5")}}},
		{"Count_LimitDecimalHuge", bson.D{{Key: "limit", Value: dec("1E+30")}}},
		{"Count_LimitDoubleHuge", bson.D{{Key: "limit", Value: 1e300}}},
		{"Count_LimitNaN", bson.D{{Key: "limit", Value: math.NaN()}}},
		{"Count_LimitNull", bson.D{{Key: "limit", Value: nil}}},
		{"Count_LimitString", bson.D{{Key: "limit", Value: "a"}}},
		{"Count_LimitBool", bson.D{{Key: "limit", Value: true}}},
		{"Count_SkipNegative", bson.D{{Key: "skip", Value: int32(-1)}}},
		{"Count_SkipNegativeFraction", bson.D{{Key: "skip", Value: -1.5}}},
		{"Count_SkipSmallNegativeFraction", bson.D{{Key: "skip", Value: -0.5}}},
		{"Count_SkipFraction", bson.D{{Key: "skip", Value: 1.5}}},
		{"Count_SkipDecimalHalfEvenUp", bson.D{{Key: "skip", Value: dec("1.5")}}},
		{"Count_SkipDecimalHalfEvenDown", bson.D{{Key: "skip", Value: dec("2.5")}}},
		{"Count_SkipDecimalNegative", bson.D{{Key: "skip", Value: dec("-1")}}},
		{"Count_SkipDoubleHuge", bson.D{{Key: "skip", Value: 1e300}}},
		{"Count_SkipNaN", bson.D{{Key: "skip", Value: math.NaN()}}},
		{"Count_SkipNull", bson.D{{Key: "skip", Value: nil}}},
		{"Count_SkipString", bson.D{{Key: "skip", Value: "a"}}},
	}
	for _, c := range cases {
		opts := c.opts
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Setup:   countLimitSkipSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cmd := append(bson.D{{Key: "count", Value: col.Name()}}, opts...)
				var res bson.D
				err := col.Database().RunCommand(ctx, cmd).Decode(&res)
				return res, err
			},
		})
	}
}

func TestCountLimitSkip_Explain(t *testing.T) {
	cases := []struct {
		name  string
		limit interface{}
	}{
		{"CountExplain_LimitNegative", int32(-2)},
		{"CountExplain_LimitFraction", 2.5},
	}
	for _, c := range cases {
		limit := c.limit
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Setup:   countLimitSkipSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cmd := bson.D{{Key: "explain", Value: bson.D{{Key: "count", Value: col.Name()}, {Key: "limit", Value: limit}}}}
				var res bson.M
				if err := col.Database().RunCommand(ctx, cmd).Decode(&res); err != nil {
					return nil, err
				}
				_, explained := res["queryPlanner"]
				return bson.D{{Key: "explained", Value: explained}}, nil
			},
		})
	}
}
