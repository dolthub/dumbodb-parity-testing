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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// An expression error in $bucket, $bucketAuto or $sortByCount, whether in the
// grouping expression or an output accumulator, fails the aggregate rather
// than grouping the document under null. The $log and date-overflow errors
// used here have the same code whichever engine mongod runs the stage in.

func bucketErrorsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(5)}, {Key: "s", Value: "x"}, {Key: "z", Value: int32(0)},
			{Key: "dt", Value: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}, {Key: "lmin", Value: int64(math.MinInt64)}},
	})
	return err
}

func TestBucketExpressionErrors(t *testing.T) {
	logStr := bson.D{{Key: "$log", Value: bson.A{"$s", int32(10)}}}
	dateOverflow := bson.D{{Key: "$subtract", Value: bson.A{"$dt", "$lmin"}}}
	divideZero := bson.D{{Key: "$divide", Value: bson.A{"$v", "$z"}}}
	missing := bson.D{{Key: "$ifNull", Value: bson.A{"$nope", "$nope2"}}}
	cases := []struct {
		name  string
		stage bson.D
	}{
		{"BucketErr_GroupByLog", bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: logStr}, {Key: "boundaries", Value: bson.A{0, 10}}, {Key: "default", Value: "o"}}}}},
		{"BucketErr_OutputLog", bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{0, 10}},
			{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$push", Value: logStr}}}}}}}}},
		{"BucketErr_OutputDateOverflow", bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{0, 10}},
			{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$max", Value: dateOverflow}}}}}}}}},
		{"BucketErr_DefaultOutputLog", bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{10, 20}}, {Key: "default", Value: "o"},
			{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$sum", Value: logStr}}}}}}}}},
		{"BucketErr_GroupByMissing", bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: missing}, {Key: "boundaries", Value: bson.A{0, 10}}, {Key: "default", Value: "o"}}}}},
		{"BucketAutoErr_GroupByDivide", bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: divideZero}, {Key: "buckets", Value: int32(2)}}}}},
		{"BucketAutoErr_OutputDivide", bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "buckets", Value: int32(2)},
			{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$push", Value: divideZero}}}}}}}}},
		{"BucketAutoErr_GroupByMissing", bson.D{{Key: "$bucketAuto", Value: bson.D{{Key: "groupBy", Value: "$nope"}, {Key: "buckets", Value: int32(2)}}}}},
		{"SortByCountErr_Log", bson.D{{Key: "$sortByCount", Value: logStr}}},
		{"SortByCountErr_Missing", bson.D{{Key: "$sortByCount", Value: missing}}},
	}
	for _, c := range cases {
		stage := c.stage
		harness.PairTest(t, harness.TestCase{
			Name:    c.name,
			Support: harness.DumboDBFull,
			Setup:   bucketErrorsSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stage)
			},
		})
	}
}
