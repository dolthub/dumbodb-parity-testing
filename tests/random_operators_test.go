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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $rand and $sampleRate are random, so the cases check types, ranges,
// validation and statistics that hold with overwhelming probability.

const randomDocCount = 200

func insertRandomDocs(ctx context.Context, col *mongo.Collection) error {
	docs := make([]interface{}, randomDocCount)
	for i := range docs {
		docs[i] = bson.D{{Key: "_id", Value: int32(i)}, {Key: "arr", Value: bson.A{bson.D{{Key: "x", Value: int32(i)}}}}}
	}
	_, err := col.InsertMany(ctx, docs)
	return err
}

func countRun(filter interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		n, err := col.CountDocuments(ctx, filter)
		if err != nil {
			return errOutcome(err), nil
		}
		return n, nil
	}
}

// countBetweenRun reports whether the number of matching documents is in
// [lo, hi].
func countBetweenRun(filter interface{}, lo, hi int64) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		n, err := col.CountDocuments(ctx, filter)
		if err != nil {
			return errOutcome(err), nil
		}
		return n >= lo && n <= hi, nil
	}
}

func TestRand(t *testing.T) {
	rand := bson.D{{Key: "$rand", Value: bson.D{}}}
	inUnitInterval := bson.D{{Key: "$let", Value: bson.D{
		{Key: "vars", Value: bson.D{{Key: "r", Value: rand}}},
		{Key: "in", Value: bson.D{{Key: "$and", Value: bson.A{
			bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$type", Value: "$$r"}}, "double"}}},
			bson.D{{Key: "$gte", Value: bson.A{"$$r", int32(0)}}},
			bson.D{{Key: "$lt", Value: bson.A{"$$r", int32(1)}}},
		}}}},
	}}}

	cases := []mathCase{
		{"Type", bson.D{{Key: "$type", Value: rand}}},
		{"InUnitInterval", inUnitInterval},
		{"TwoCallsDiffer", bson.D{{Key: "$ne", Value: bson.A{rand, rand}}}},
		{"EmptyArrayArgument", bson.D{{Key: "$type", Value: bson.D{{Key: "$rand", Value: bson.A{}}}}}},
		{"InArithmetic", bson.D{{Key: "$lt", Value: bson.A{bson.D{{Key: "$multiply", Value: bson.A{rand, int32(10)}}}, int32(10)}}}},
		{"Floor", bson.D{{Key: "$in", Value: bson.A{bson.D{{Key: "$floor", Value: bson.D{{Key: "$multiply", Value: bson.A{rand, int32(2)}}}}}, bson.A{0.0, 1.0}}}}},

		{"DocumentArgument", bson.D{{Key: "$rand", Value: bson.D{{Key: "seed", Value: int32(1)}}}}},
		{"ArrayArgument", bson.D{{Key: "$rand", Value: bson.A{int32(1)}}}},
		{"NumberArgument", bson.D{{Key: "$rand", Value: int32(1)}}},
		{"StringArgument", bson.D{{Key: "$rand", Value: "x"}}},
		{"NullArgument", bson.D{{Key: "$rand", Value: nil}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Rand_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   insertRandomDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"DistinctPerDocument", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "rs", Value: bson.D{{Key: "$addToSet", Value: rand}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "n", Value: bson.D{{Key: "$size", Value: "$rs"}}}}}},
		}},
		{"AllInUnitInterval", bson.A{
			bson.D{{Key: "$project", Value: bson.D{{Key: "ok", Value: inUnitInterval}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$ok"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
		}},
		{"MeanNearHalf", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "m", Value: bson.D{{Key: "$avg", Value: rand}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "ok", Value: bson.D{{Key: "$and", Value: bson.A{
				bson.D{{Key: "$gt", Value: bson.A{"$m", 0.35}}}, bson.D{{Key: "$lt", Value: bson.A{"$m", 0.65}}},
			}}}}}}},
		}},
		{"MatchExprNever", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{rand, int32(0)}}}}}}},
			bson.D{{Key: "$count", Value: "n"}},
		}},
		{"MatchExprAlways", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$gte", Value: bson.A{rand, int32(0)}}}}}}},
			bson.D{{Key: "$count", Value: "n"}},
		}},
		{"AddFieldsThenSortByRandom", bson.A{
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: rand}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "r", Value: 1}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "Rand_" + p.name,
				Support: harness.DumboDBXFail,
				Setup:   insertRandomDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}

	t.Run("FindProjection", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Rand_FindProjection",
			Support: harness.DumboDBXFail,
			Setup:   insertRandomDocs,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(bson.D{{Key: "ok", Value: inUnitInterval}}))
				if err != nil {
					return errOutcome(err), nil
				}
				var docs []bson.M
				if err = cursor.All(ctx, &docs); err != nil {
					return errOutcome(err), nil
				}
				ok := 0
				for _, d := range docs {
					if d["ok"] == true {
						ok++
					}
				}
				return ok, nil
			},
		})
	})

	t.Run("CollectionValidator", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "Rand_CollectionValidator",
			Support: harness.DumboDBXFail,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				err := col.Database().CreateCollection(ctx, col.Name()+"_validated", options.CreateCollection().SetValidator(
					bson.D{{Key: "$expr", Value: bson.D{{Key: "$lt", Value: bson.A{rand, int32(1)}}}}},
				))
				if err != nil {
					return errOutcome(err), nil
				}
				return "created", nil
			},
		})
	})
}

// sampleRateAlreadyMatching are the cases DumboDB matched before $sampleRate
// was implemented (workspace-x6i.14): its BadValue for an unknown operator
// has the same code as MongoDB's validation errors.
var sampleRateAlreadyMatching = map[string]bool{
	"AggregateMatchInvalid": true,
	"Bool":                  true,
	"Document":              true,
	"FieldLevel":            true,
	"GreaterThanOne":        true,
	"Infinity":              true,
	"InsideElemMatch":       true,
	"InsideNot":             true,
	"NaN":                   true,
	"Negative":              true,
	"Null":                  true,
	"String":                true,
}

func TestSampleRate(t *testing.T) {
	sampleRate := func(rate interface{}) bson.D { return bson.D{{Key: "$sampleRate", Value: rate}} }
	n := int64(randomDocCount)

	cases := []struct {
		name string
		run  func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		{"Zero", countRun(sampleRate(0.0))},
		{"One", countRun(sampleRate(1.0))},
		{"IntOne", countRun(sampleRate(int32(1)))},
		{"LongZero", countRun(sampleRate(int64(0)))},
		{"DecimalOne", countRun(sampleRate(mustDecimal("1")))},
		{"Half", countBetweenRun(sampleRate(0.5), n/5, 4*n/5)},
		{"DecimalHalf", countBetweenRun(sampleRate(mustDecimal("0.5")), n/5, 4*n/5)},
		{"Tiny", countBetweenRun(sampleRate(1e-9), 0, 1)},
		{"NearlyOne", countBetweenRun(sampleRate(0.999999999), n-1, n)},
		{"WithPredicate", countRun(bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(10)}}}, {Key: "$sampleRate", Value: 1.0}})},
		{"WithPredicateZero", countRun(bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(10)}}}, {Key: "$sampleRate", Value: 0.0}})},
		{"InsideAnd", countRun(bson.D{{Key: "$and", Value: bson.A{sampleRate(1.0), bson.D{{Key: "_id", Value: int32(3)}}}}})},
		{"InsideOr", countRun(bson.D{{Key: "$or", Value: bson.A{sampleRate(0.0), bson.D{{Key: "_id", Value: int32(3)}}}}})},
		{"InsideNor", countRun(bson.D{{Key: "$nor", Value: bson.A{sampleRate(0.0)}}})},

		{"Negative", countRun(sampleRate(-0.1))},
		{"GreaterThanOne", countRun(sampleRate(1.5))},
		{"NaN", countRun(sampleRate(math.NaN()))},
		{"Infinity", countRun(sampleRate(math.Inf(1)))},
		{"String", countRun(sampleRate("0.5"))},
		{"Null", countRun(sampleRate(nil))},
		{"Bool", countRun(sampleRate(true))},
		{"Document", countRun(sampleRate(bson.D{{Key: "$literal", Value: 0.5}}))},
		{"FieldLevel", countRun(bson.D{{Key: "_id", Value: sampleRate(0.5)}})},
		{"InsideElemMatch", countRun(bson.D{{Key: "arr", Value: bson.D{{Key: "$elemMatch", Value: sampleRate(0.5)}}}})},
		{"InsideNot", countRun(bson.D{{Key: "_id", Value: bson.D{{Key: "$not", Value: sampleRate(0.5)}}}})},

		{"AggregateMatch", pipelineRunExtJSON(bson.A{
			bson.D{{Key: "$match", Value: sampleRate(1.0)}},
			bson.D{{Key: "$count", Value: "n"}},
		})},
		{"AggregateMatchZero", pipelineRunExtJSON(bson.A{
			bson.D{{Key: "$match", Value: sampleRate(0.0)}},
			bson.D{{Key: "$count", Value: "n"}},
		})},
		{"AggregateMatchHalf", pipelineRunExtJSON(bson.A{
			bson.D{{Key: "$match", Value: sampleRate(0.5)}},
			bson.D{{Key: "$count", Value: "n"}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "ok", Value: bson.D{{Key: "$and", Value: bson.A{
				bson.D{{Key: "$gte", Value: bson.A{"$n", n / 5}}}, bson.D{{Key: "$lte", Value: bson.A{"$n", 4 * n / 5}}},
			}}}}}}},
		})},
		{"AggregateMatchInvalid", pipelineRunExtJSON(bson.A{bson.D{{Key: "$match", Value: sampleRate(2)}}})},
		{"FindIDs", func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(5)}}}, {Key: "$sampleRate", Value: 1.0}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return errOutcome(err), nil
			}
			var docs []bson.D
			if err = cursor.All(ctx, &docs); err != nil {
				return errOutcome(err), nil
			}
			return docs, nil
		}},
	}
	for _, tc := range cases {
		support := harness.DumboDBXFail
		if sampleRateAlreadyMatching[tc.name] {
			support = harness.DumboDBFull
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "SampleRate_" + tc.name,
				Support: support,
				Setup:   insertRandomDocs,
				Run:     tc.run,
			})
		})
	}
}
