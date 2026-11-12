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
