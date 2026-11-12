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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func miscCodesSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "n", Value: int32(0)}, {Key: "x", Value: int32(1)},
		{Key: "o", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}}})
	return err
}

// $switch without a match fails with 40066 in the classic engine and 7158303
// in SBE; a constant one fails while mongod optimizes it (40069). $bucket and
// $fill check their fields in the order given. $$REMOVE in $percentile's p is
// a constant null. A Decimal128 projection flag includes or excludes.
func TestMiscErrorCodes(t *testing.T) {
	pipelines := map[string]string{
		"SwitchNoMatch":                `[{"$project":{"r":{"$switch":{"branches":[{"case":"$n","then":1}]}}}}]`,
		"SwitchNoMatchInSBE":           `[{"$group":{"_id":null,"r":{"$push":{"$switch":{"branches":[{"case":"$n","then":1}]}}}}}]`,
		"SwitchNoMatchNested":          `[{"$project":{"r":{"$add":[1,{"$switch":{"branches":[{"case":"$n","then":1}]}}]}}}]`,
		"SwitchConstantNoMatch":        `[{"$project":{"r":{"$switch":{"branches":[{"case":false,"then":1}]}}}}]`,
		"SwitchConstantNoMatchInSBE":   `[{"$group":{"_id":null,"r":{"$push":{"$switch":{"branches":[{"case":false,"then":1}]}}}}}]`,
		"BucketNoMatchingBucket":       `[{"$bucket":{"groupBy":"$x","boundaries":[5,6]}}]`,
		"BucketBoundariesBeforeGroup":  `[{"$bucket":{"boundaries":[0,"a"],"groupBy":{"a":1}}}]`,
		"BucketGroupBeforeBoundaries":  `[{"$bucket":{"groupBy":{"a":1},"boundaries":[0,"a"]}}]`,
		"BucketBadBoundariesNoGroup":   `[{"$bucket":{"boundaries":[0,"a"]}}]`,
		"BucketBoundariesNotArray":     `[{"$bucket":{"groupBy":"$x","boundaries":5}}]`,
		"BucketNoBoundaries":           `[{"$bucket":{"groupBy":"$x"}}]`,
		"BucketDollarGroupBy":          `[{"$bucket":{"groupBy":"$","boundaries":[0,5]}}]`,
		"FillNullPartitionThenUnknown": `[{"$fill":{"partitionBy":null,"zz":1}}]`,
		"FillUnknownThenNullPartition": `[{"$fill":{"zz":1,"partitionBy":null}}]`,
		"PercentileRemoveInGroup":      `[{"$group":{"_id":null,"p":{"$percentile":{"input":"$x","p":["$$REMOVE"],"method":"approximate"}}}}]`,
		"PercentileRemoveInProject":    `[{"$project":{"p":{"$percentile":{"input":["$x"],"p":["$$REMOVE"],"method":"approximate"}}}}]`,
		"PercentileRemoveAsP":          `[{"$project":{"p":{"$percentile":{"input":["$x"],"p":"$$REMOVE","method":"approximate"}}}}]`,
		"ProjectDecimalOne":            `[{"$project":{"x":{"$numberDecimal":"1"}}}]`,
		"ProjectDecimalZero":           `[{"$project":{"x":{"$numberDecimal":"0"}}}]`,
		"ProjectDecimalNested":         `[{"$project":{"o":{"a":{"$numberDecimal":"1"}}}}]`,
		"ProjectDecimalWithExclusion":  `[{"$project":{"x":{"$numberDecimal":"1"},"n":0}}]`,
	}
	for name, js := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *[]bson.D }{&stages}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "MiscErrorCodes_" + name,
			Support: harness.DumboDBFull,
			Setup:   miscCodesSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
	for name, js := range map[string]string{
		"FindDecimalOne":     `{"x":{"$numberDecimal":"1"}}`,
		"FindDecimalZero":    `{"x":{"$numberDecimal":"0"}}`,
		"FindDecimalNegZero": `{"x":{"$numberDecimal":"-0"}}`,
		"FindDecimalDotted":  `{"o.a":{"$numberDecimal":"1"},"_id":{"$numberDecimal":"0"}}`,
	} {
		var proj bson.D
		if err := bson.UnmarshalExtJSON([]byte(js), false, &proj); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "MiscErrorCodes_" + name,
			Support: harness.DumboDBFull,
			Setup:   miscCodesSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return findWithProjection(ctx, col, bson.D{}, proj)
			},
		})
	}
}

// Decimal time-series bucket spans round half to even.
func TestTimeSeriesDecimalBucketSpan(t *testing.T) {
	cases := map[string]bson.D{
		"RoundsUp":       {{Key: "bucketMaxSpanSeconds", Value: dz("100.7")}, {Key: "bucketRoundingSeconds", Value: int32(101)}},
		"NotTruncated":   {{Key: "bucketMaxSpanSeconds", Value: dz("100.7")}, {Key: "bucketRoundingSeconds", Value: int32(100)}},
		"HalfToEvenDown": {{Key: "bucketMaxSpanSeconds", Value: dz("100.5")}, {Key: "bucketRoundingSeconds", Value: int32(100)}},
		"HalfToEvenUp":   {{Key: "bucketMaxSpanSeconds", Value: dz("101.5")}, {Key: "bucketRoundingSeconds", Value: int32(102)}},
		"RoundsToOne":    {{Key: "bucketMaxSpanSeconds", Value: dz("0.6")}, {Key: "bucketRoundingSeconds", Value: int32(1)}},
		"RoundsToZero":   {{Key: "bucketMaxSpanSeconds", Value: dz("0.4")}, {Key: "bucketRoundingSeconds", Value: int32(1)}},
	}
	for name, opts := range cases {
		opts := opts
		harness.PairTest(t, harness.TestCase{
			Name:    "TimeSeriesDecimalBucketSpan_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				db := col.Database()
				name := col.Name() + "_ts"
				defer db.Collection(name).Drop(ctx)
				if err := db.RunCommand(ctx, bson.D{{Key: "create", Value: name}, {Key: "timeseries", Value: append(bson.D{{Key: "timeField", Value: "t"}}, opts...)}}).Err(); err != nil {
					return nil, err
				}
				specs, err := db.ListCollectionSpecifications(ctx, bson.D{{Key: "name", Value: name}})
				if err != nil || len(specs) != 1 {
					return nil, err
				}
				var options bson.D
				err = bson.Unmarshal(specs[0].Options, &options)
				return options, err
			},
		})
	}
}

// $unionWith starts its sub-pipeline once its input is exhausted, so an error
// reading the input comes first.
func TestUnionWithStartsAfterInput(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnionWithStartsAfterInput",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "r", Value: "s"}, {Key: "arr", Value: bson.A{int32(1)}}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$project", Value: bson.D{{Key: "x", Value: bson.D{{Key: "$add", Value: bson.A{"$r", int32(1)}}}}}}},
				bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{
					bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "t", Value: bson.D{{Key: "$sum", Value: bson.D{{Key: "$divide", Value: bson.A{int32(1), "$arr"}}}}}}}}},
				}}}}})
		},
	})
}

// $bucketAuto's buckets must be a positive 32-bit integer; an update
// pipeline parses a stage before refusing it; $documents evaluates its
// argument as an expression.
func TestParseChecksQ11(t *testing.T) {
	pipelines := map[string]string{
		"BucketsFraction":      `[{"$bucketAuto":{"groupBy":"$x","buckets":1.5}}]`,
		"BucketsTooLarge":      `[{"$bucketAuto":{"groupBy":"$x","buckets":8589934592}}]`,
		"BucketsLongTooLarge":  `[{"$bucketAuto":{"groupBy":"$x","buckets":{"$numberLong":"8589934592"}}}]`,
		"BucketsString":        `[{"$bucketAuto":{"groupBy":"$x","buckets":"a"}}]`,
		"BucketsZero":          `[{"$bucketAuto":{"groupBy":"$x","buckets":0}}]`,
		"BucketsNegativeFract": `[{"$bucketAuto":{"groupBy":"$x","buckets":-1.5}}]`,
		"BucketsDecimal":       `[{"$bucketAuto":{"groupBy":"$x","buckets":{"$numberDecimal":"2"}}}]`,
		"BucketsDecimalFract":  `[{"$bucketAuto":{"groupBy":"$x","buckets":{"$numberDecimal":"2.5"}}}]`,
		"SortByCountTrailing":  `[{"$sortByCount":"$x."}]`,
		"SortByCountEmptyVar":  `[{"$sortByCount":"$$"}]`,
	}
	for name, js := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *[]bson.D }{&stages}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ParseChecksQ11_" + name,
			Support: harness.DumboDBFull,
			Setup:   miscCodesSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
	updates := map[string]string{
		"UpdateBadDensify": `[{"$densify":{"field":1}}]`,
		"UpdateBadGroup":   `[{"$group":{"_id":1,"zz":1}}]`,
		"UpdateGroup":      `[{"$group":{"_id":1}}]`,
		"UpdateUnknown":    `[{"$bogus":{}}]`,
	}
	for name, js := range updates {
		var stages bson.A
		if err := bson.UnmarshalExtJSON([]byte(js), false, &stages); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ParseChecksQ11_" + name,
			Support: harness.DumboDBFull,
			Setup:   miscCodesSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				_, err := col.UpdateOne(ctx, bson.D{}, stages)
				return nil, err
			},
		})
	}
	documents := map[string]string{
		"DocumentsFieldPaths": `[{"$documents":[{"a":"$b"},{"a":{"$add":[1,2]}}]},{"$project":{"_id":0,"a":1,"t":{"$type":"$a"}}}]`,
		"DocumentsNow":        `[{"$documents":[{"a":"$$NOW"}]},{"$project":{"_id":0,"t":{"$type":"$a"}}}]`,
		"DocumentsMap":        `[{"$documents":{"$map":{"input":[1,2],"in":{"v":"$$this"}}}}]`,
		"DocumentsNotArray":   `[{"$documents":"$x"}]`,
		"DocumentsScalar":     `[{"$documents":[1]}]`,
	}
	for name, js := range documents {
		var stages bson.A
		if err := bson.UnmarshalExtJSON([]byte(js), false, &stages); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ParseChecksQ11_" + name,
			Support: harness.DumboDBFull,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var res bson.M
				if err := col.Database().RunCommand(ctx, bson.D{{Key: "aggregate", Value: 1}, {Key: "pipeline", Value: stages}, {Key: "cursor", Value: bson.D{}}}).Decode(&res); err != nil {
					return nil, err
				}
				return res["cursor"].(bson.M)["firstBatch"], nil
			},
		})
	}
}

// $tsSecond and $tsIncrement fail on a value that is not a timestamp with
// their own codes and give null for null or missing; $const is $literal; a
// filter checks $expr after its other conditions.
func TestExpressionsQ12(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(1), int32(2)}}, {Key: "v", Value: int32(9)},
				{Key: "t", Value: primitive.Timestamp{T: 5, I: 7}}, {Key: "n", Value: nil}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int32(9)}},
		})
		return err
	}
	pipelines := map[string]string{
		"TsSecondConstant":     `[{"$project":{"t":{"$tsSecond":{"$literal":1}}}}]`,
		"TsIncrementString":    `[{"$project":{"t":{"$tsIncrement":"x"}}}]`,
		"TsValues":             `[{"$project":{"s":{"$tsSecond":"$t"},"i":{"$tsIncrement":"$t"},"m":{"$tsSecond":"$n"},"z":{"$tsSecond":"$zz"}}}]`,
		"TsSecondField":        `[{"$project":{"s":{"$tsSecond":"$v"}}}]`,
		"Const":                `[{"$project":{"t":{"$const":5}}}]`,
		"ConstFieldPath":       `[{"$project":{"t":{"$const":"$a"}}}]`,
		"MatchOrExprFirst":     `[{"$match":{"$or":[{"$expr":{"$eq":[{"$size":"$a"},2]}},{"v":9}]}},{"$project":{"_id":1}}]`,
		"MatchAndExprFirst":    `[{"$match":{"$and":[{"$expr":{"$eq":[{"$size":"$a"},2]}},{"v":8}]}}]`,
		"MatchExprBeforeField": `[{"$match":{"$expr":{"$eq":[{"$size":"$a"},2]},"v":8}}]`,
		"MatchNorExprFirst":    `[{"$match":{"$nor":[{"$expr":{"$eq":[{"$size":"$a"},2]}},{"v":9}]}}]`,
		"MatchOrExprFails":     `[{"$match":{"$or":[{"$expr":{"$eq":[{"$size":"$a"},2]}},{"v":8}]}}]`,
	}
	for name, js := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *[]bson.D }{&stages}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "ExpressionsQ12_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, append(stages, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})...)
			},
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "ExpressionsQ12_FindOrExprFirst",
		Support: harness.DumboDBFull,
		Setup:   seed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return findWithProjection(ctx, col, bson.D{{Key: "$or", Value: bson.A{
				bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{bson.D{{Key: "$size", Value: "$a"}}, int32(2)}}}}},
				bson.D{{Key: "v", Value: int32(9)}},
			}}}, bson.D{{Key: "_id", Value: 1}})
		},
	})
}

// A dotted includeArrayIndex names a path; $collStats reports latencyStats in
// mongod's shape and checks its specification.
func TestUnwindIndexPathAndLatencyStats(t *testing.T) {
	shape := bson.D{{Key: "$project", Value: bson.D{{Key: "k", Value: bson.D{{Key: "$map", Value: bson.D{
		{Key: "input", Value: bson.D{{Key: "$objectToArray", Value: "$latencyStats"}}},
		{Key: "in", Value: bson.D{{Key: "k", Value: "$$this.k"}, {Key: "t", Value: bson.D{{Key: "$map", Value: bson.D{
			{Key: "input", Value: bson.D{{Key: "$objectToArray", Value: "$$this.v"}}}, {Key: "as", Value: "f"},
			{Key: "in", Value: bson.A{"$$f.k", bson.D{{Key: "$type", Value: "$$f.v"}}}},
		}}}}}},
	}}}}}}}
	cases := map[string][]bson.D{
		"UnwindIndexDottedScalar": {{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$k"}, {Key: "includeArrayIndex", Value: "a.b"}}}}},
		"UnwindIndexDottedArray": {{{Key: "$set", Value: bson.D{{Key: "arr", Value: bson.A{int32(5), int32(6)}}}}},
			{{Key: "$unwind", Value: bson.D{{Key: "path", Value: "$arr"}, {Key: "includeArrayIndex", Value: "a.b"}}}}},
		"LatencyStats":           {{{Key: "$collStats", Value: bson.D{{Key: "latencyStats", Value: bson.D{}}}}}, shape},
		"LatencyStatsHistograms": {{{Key: "$collStats", Value: bson.D{{Key: "latencyStats", Value: bson.D{{Key: "histograms", Value: true}}}}}}, shape},
		"LatencyStatsUnknown":    {{{Key: "$collStats", Value: bson.D{{Key: "latencyStats", Value: bson.D{{Key: "zz", Value: int32(1)}}}}}}},
		"LatencyStatsNotObject":  {{{Key: "$collStats", Value: bson.D{{Key: "latencyStats", Value: int32(5)}}}}},
	}
	for name, stages := range cases {
		stages := stages
		harness.PairTest(t, harness.TestCase{
			Name:    "UnwindIndexPathAndLatencyStats_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: int32(1)}})
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
}
