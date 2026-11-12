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
	"fmt"
	"math"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $bucketAuto fills buckets as mongod does and rounds their boundaries to a
// granularity; $bucket and $sortByCount validate their specifications;
// window $first and $last report a missing value as null; $arrayToObject
// accepts [key, value] pairs.

func docsAgg(docs bson.A, stages ...bson.D) func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Database().Aggregate(ctx, append(mongo.Pipeline{{{Key: "$documents", Value: docs}}}, stages...))
		if err != nil {
			return nil, err
		}
		var out []bson.D
		err = cursor.All(ctx, &out)
		return out, err
	}
}

func xDocs(values ...interface{}) bson.A {
	docs := bson.A{}
	for _, v := range values {
		docs = append(docs, bson.D{{Key: "x", Value: v}})
	}
	return docs
}

func TestBucketAutoGranularity(t *testing.T) {
	auto := func(buckets int32, granularity interface{}) bson.D {
		spec := bson.D{{Key: "groupBy", Value: "$x"}, {Key: "buckets", Value: buckets}}
		if granularity != nil {
			spec = append(spec, bson.E{Key: "granularity", Value: granularity})
		}
		return bson.D{{Key: "$bucketAuto", Value: spec}}
	}
	ints := func(from, to int32) []interface{} {
		var out []interface{}
		for i := from; i < to; i++ {
			out = append(out, i)
		}
		return out
	}
	samples := []interface{}{int32(1), int32(5), int32(0), 0.5, 6.3, 63.0, int32(100), 0.07, 1234.5, int64(1000000),
		dz("5"), dz("6.3"), 2.5, 1e-5, 3.99, dz("1000"), dz("0.001"), 5e-324, 1e300}
	cases := []struct {
		name  string
		docs  bson.A
		stage bson.D
	}{
		{"Distribution10Into4", xDocs(ints(0, 10)...), auto(4, nil)},
		{"EqualValuesTogether", xDocs(int32(1), int32(1), int32(1), int32(1), int32(2), int32(3)), auto(3, nil)},
		{"EqualValuesSpanBucket", xDocs(int32(1), int32(2), int32(2), int32(2), int32(2), int32(3), int32(4)), auto(3, nil)},
		{"Distribution7Into2", xDocs(ints(0, 7)...), auto(2, nil)},
		{"NullValue", xDocs(nil, int32(1), int32(2)), auto(2, nil)},
		{"R5Absorbs", xDocs(ints(1, 21)...), auto(3, "R5")},
		{"PowersOf2Zeros", xDocs(int32(0), int32(0), int32(1), int32(2), int32(3)), auto(2, "POWERSOF2")},
		{"PowersOf2ZeroMax", xDocs(int32(0), int32(0), int32(0), int32(0), int32(3)), auto(2, "POWERSOF2")},
		{"OneTwoFive", xDocs(int32(1), 1.5, int32(2), 2.4, int32(3), int32(9)), auto(3, "1-2-5")},
		{"String", xDocs(int32(1), "a"), auto(2, "R5")},
		{"Null", xDocs(int32(1), nil), auto(2, "R5")},
		{"Negative", xDocs(int32(1), int32(-1)), auto(2, "E6")},
		{"NaN", xDocs(int32(1), math.NaN()), auto(2, "E6")},
		{"UnknownGranularity", xDocs(int32(1)), auto(2, "bogus")},
		{"GranularityNotString", xDocs(int32(1)), auto(2, int32(5))},
	}
	for _, g := range []string{"R5", "R10", "R20", "R40", "R80", "1-2-5", "E6", "E12", "E24", "E48", "E96", "E192", "POWERSOF2"} {
		for i, v := range samples {
			cases = append(cases, struct {
				name  string
				docs  bson.A
				stage bson.D
			}{fmt.Sprintf("Round_%s_%d", g, i), xDocs(v), auto(1, g)})
		}
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "BucketAuto_" + c.name,
			Support: harness.DumboDBFull,
			Run:     docsAgg(c.docs, c.stage),
		})
	}
}

func TestBucketSpecs(t *testing.T) {
	docs := xDocs(int32(1), int32(5), "a")
	bucket := func(spec bson.D) bson.D { return bson.D{{Key: "$bucket", Value: spec}} }
	gb := bson.E{Key: "groupBy", Value: "$x"}
	bounds := func(v ...interface{}) bson.E { return bson.E{Key: "boundaries", Value: bson.A(v)} }
	def := func(v interface{}) bson.E { return bson.E{Key: "default", Value: v} }
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"GroupByObject", bucket(bson.D{{Key: "groupBy", Value: bson.D{{Key: "a", Value: int32(1)}}}, bounds(int32(0), int32(10))})},
		{"GroupByPlainString", bucket(bson.D{{Key: "groupBy", Value: "x"}, bounds(int32(0), int32(10))})},
		{"GroupByNull", bucket(bson.D{{Key: "groupBy", Value: nil}, bounds(int32(0), int32(1))})},
		{"GroupByArray", bucket(bson.D{{Key: "groupBy", Value: bson.A{int32(1)}}, bounds(int32(0), int32(1))})},
		{"GroupByEmptyObject", bucket(bson.D{{Key: "groupBy", Value: bson.D{}}, bounds(int32(0), int32(1))})},
		{"MixedBoundaryTypes", bucket(bson.D{gb, bounds(int32(0), "a")})},
		{"MixedBoundaryTypesWithDefault", bucket(bson.D{gb, bounds(int32(0), "a"), def(int32(5))})},
		{"NullBoundary", bucket(bson.D{gb, bounds(nil, int32(1))})},
		{"MixedNumericBoundaries", bucket(bson.D{gb, bounds(int32(0), int64(5), 7.5), def("z")})},
		{"UnsortedBoundaries", bucket(bson.D{gb, bounds(int32(0), int32(1), int64(3), 2.5)})},
		{"EqualBoundaries", bucket(bson.D{gb, bounds(int32(1), int32(1))})},
		{"DefaultInRange", bucket(bson.D{gb, bounds(int32(0), int32(10)), def(int32(5))})},
		{"DefaultInRangeLong", bucket(bson.D{gb, bounds(int32(0), int32(2)), def(int64(1))})},
		{"DefaultInStringRange", bucket(bson.D{{Key: "groupBy", Value: bson.D{{Key: "$toString", Value: "$x"}}}, bounds("0", "9"), def("5")})},
		{"DefaultAtUpper", bucket(bson.D{gb, bounds(int32(0), int32(10)), def(int32(10))})},
		{"DefaultOtherType", bucket(bson.D{gb, bounds(int32(0), int32(10)), def("z")})},
		{"SortByCountNull", bson.D{{Key: "$sortByCount", Value: nil}}},
		{"SortByCountArray", bson.D{{Key: "$sortByCount", Value: bson.A{int32(1)}}}},
		{"SortByCountNumber", bson.D{{Key: "$sortByCount", Value: int32(5)}}},
		{"SortByCountDollar", bson.D{{Key: "$sortByCount", Value: "$"}}},
		{"SortByCountPlainString", bson.D{{Key: "$sortByCount", Value: "x"}}},
		{"SortByCountEmptyObject", bson.D{{Key: "$sortByCount", Value: bson.D{}}}},
		{"SortByCountObject", bson.D{{Key: "$sortByCount", Value: bson.D{{Key: "a", Value: int32(1)}}}}},
		{"SortByCountPath", bson.D{{Key: "$sortByCount", Value: "$x"}}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "BucketSpecs_" + c.name,
			Support: harness.DumboDBFull,
			Run:     docsAgg(docs, c.stage, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}),
		})
	}
}

func TestWindowFirstLastMissing(t *testing.T) {
	docs := bson.A{
		bson.D{{Key: "_id", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "v", Value: nil}},
		bson.D{{Key: "_id", Value: int32(4)}},
	}
	window := func(output bson.D) bson.D {
		return bson.D{{Key: "$setWindowFields", Value: bson.D{{Key: "sortBy", Value: bson.D{{Key: "_id", Value: 1}}}, {Key: "output", Value: output}}}}
	}
	frame := func(lo, hi int32) bson.E {
		return bson.E{Key: "window", Value: bson.D{{Key: "documents", Value: bson.A{lo, hi}}}}
	}
	cases := map[string]bson.D{
		"Unbounded": window(bson.D{{Key: "f", Value: bson.D{{Key: "$first", Value: "$v"}}}, {Key: "l", Value: bson.D{{Key: "$last", Value: "$v"}}}}),
		"Frames": window(bson.D{
			{Key: "f", Value: bson.D{{Key: "$first", Value: "$v"}, frame(0, 1)}},
			{Key: "l", Value: bson.D{{Key: "$last", Value: "$v"}, frame(-1, 0)}},
		}),
		"EmptyFrame": window(bson.D{{Key: "f", Value: bson.D{{Key: "$first", Value: "$v"}, frame(5, 6)}}}),
	}
	for name, stage := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "WindowFirstLastMissing_" + name,
			Support: harness.DumboDBFull,
			Run:     docsAgg(docs, stage),
		})
	}
}

func TestArrayToObjectForms(t *testing.T) {
	inputs := map[string]interface{}{
		"Pairs":            bson.A{bson.A{"k", int32(1)}, bson.A{"j", int32(2)}},
		"Objects":          bson.A{bson.D{{Key: "k", Value: "x"}, {Key: "v", Value: int32(1)}}},
		"NotArray":         int32(5),
		"PairThenObject":   bson.A{bson.A{"k", int32(1)}, bson.D{{Key: "k", Value: "x"}, {Key: "v", Value: int32(1)}}},
		"ObjectThenPair":   bson.A{bson.D{{Key: "k", Value: "x"}, {Key: "v", Value: int32(1)}}, bson.A{"k", int32(1)}},
		"ShortPair":        bson.A{bson.A{"k"}},
		"NumericKey":       bson.A{bson.A{int32(1), int32(2)}},
		"ObjectMissingV":   bson.A{bson.D{{Key: "k", Value: "x"}}},
		"ObjectNumericKey": bson.A{bson.D{{Key: "k", Value: int32(1)}, {Key: "v", Value: int32(2)}}},
		"ObjectExtraKey":   bson.A{bson.D{{Key: "k", Value: "x"}, {Key: "v", Value: int32(1)}, {Key: "z", Value: int32(1)}}},
		"Scalars":          bson.A{int32(5)},
		"Empty":            bson.A{},
		"DuplicateKeys":    bson.A{bson.A{"a", int32(1)}, bson.A{"a", int32(2)}},
		"Null":             nil,
	}
	for name, input := range inputs {
		harness.PairTest(t, harness.TestCase{
			Name:    "ArrayToObject_" + name,
			Support: harness.DumboDBFull,
			Run: docsAgg(bson.A{bson.D{{Key: "a", Value: input}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "o", Value: bson.D{{Key: "$arrayToObject", Value: "$a"}}}}}}),
		})
	}
}

// Granularity rounding at the ends of the double and Decimal128 ranges: a
// value whose double is zero or infinite stays unrounded, and the powers of
// two rounder refuses an infinity (11785400).
func TestBucketAutoGranularityExtremes(t *testing.T) {
	values := map[string]string{
		"Infinity":        `{"$numberDouble":"Infinity"}`,
		"DecimalInfinity": `{"$numberDecimal":"Infinity"}`,
		"DecimalAboveDbl": `{"$numberDecimal":"1E+400"}`,
		"DecimalMax":      `{"$numberDecimal":"9.999999999999999999999999999999999E+6144"}`,
		"DecimalBelowDbl": `{"$numberDecimal":"3E-400"}`,
		"DecimalSmallest": `{"$numberDecimal":"1E-6176"}`,
		"DecimalSmall":    `{"$numberDecimal":"1E-300"}`,
		"DoubleSubnormal": `{"$numberDouble":"1e-310"}`,
		"DoubleLargest":   `{"$numberDouble":"1.7e308"}`,
	}
	for name, js := range values {
		var v struct{ V interface{} }
		if err := bson.UnmarshalExtJSON([]byte(`{"v":`+js+`}`), false, &v); err != nil {
			t.Fatal(err)
		}
		for _, g := range []string{"R5", "1-2-5", "E192", "POWERSOF2"} {
			value, g := v.V, g
			harness.PairTest(t, harness.TestCase{
				Name:    "BucketAutoGranularityExtremes_" + name + "_" + g,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: value}},
						bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: value}},
					})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return fieldOpsAgg(ctx, col, bson.D{{Key: "$bucketAuto", Value: bson.D{
						{Key: "groupBy", Value: "$v"}, {Key: "buckets", Value: int32(2)}, {Key: "granularity", Value: g},
					}}})
				},
			})
		}
	}
}
