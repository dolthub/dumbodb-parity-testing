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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $exists takes any value and uses its truthiness: false, zero numbers and
// null mean false, everything else true.

func existsValuesSetup(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "f", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "f", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "f", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "g", Value: bson.D{{Key: "f", Value: int32(1)}}}},
	})
	return err
}

func TestFilter_ExistsValues(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   interface{}
		support harness.DumboDBSupport
	}{
		{"True", true, harness.DumboDBFull},
		{"False", false, harness.DumboDBFull},
		{"Int1", int32(1), harness.DumboDBXFail},
		{"Int0", int32(0), harness.DumboDBXFail},
		{"IntNegative", int32(-1), harness.DumboDBXFail},
		{"Long1", int64(1), harness.DumboDBXFail},
		{"Long0", int64(0), harness.DumboDBXFail},
		{"Double", 0.5, harness.DumboDBXFail},
		{"Double0", 0.0, harness.DumboDBXFail},
		{"NegativeZero", math.Copysign(0, -1), harness.DumboDBXFail},
		{"NaN", math.NaN(), harness.DumboDBXFail},
		{"Decimal0", primitive.NewDecimal128(0, 0), harness.DumboDBXFail},
		{"Decimal1", primitive.NewDecimal128(0, 1), harness.DumboDBXFail},
		{"Null", nil, harness.DumboDBXFail},
		{"String", "x", harness.DumboDBXFail},
		{"EmptyString", "", harness.DumboDBXFail},
		{"Document", bson.D{}, harness.DumboDBXFail},
		{"Array", bson.A{}, harness.DumboDBXFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Filter_ExistsValues_" + tc.name,
				Support: tc.support,
				Setup:   existsValuesSetup,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}})
					out := bson.D{}
					for _, path := range []string{"f", "g.f"} {
						cursor, err := col.Find(ctx, bson.D{{Key: path, Value: bson.D{{Key: "$exists", Value: tc.value}}}}, opts)
						if err != nil {
							return nil, err
						}
						var results []bson.D
						if err := cursor.All(ctx, &results); err != nil {
							return nil, err
						}
						cursor, err = col.Find(ctx, bson.D{{Key: path, Value: bson.D{{Key: "$not", Value: bson.D{{Key: "$exists", Value: tc.value}}}}}}, opts)
						if err != nil {
							return nil, err
						}
						var negated []bson.D
						if err := cursor.All(ctx, &negated); err != nil {
							return nil, err
						}
						out = append(out, bson.E{Key: path, Value: bson.D{{Key: "exists", Value: results}, {Key: "not", Value: negated}}})
					}
					return out, nil
				},
			})
		})
	}
}
