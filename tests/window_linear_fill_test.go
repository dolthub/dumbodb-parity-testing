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
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// $linearFill interpolates by the sortBy value between the nearest values on
// either side, and leaves a gap at either end of a partition null.

func linearFillSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(1)}, {Key: "x", Value: int32(10)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(2)}, {Key: "x", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(4)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(5)}, {Key: "x", Value: int32(40)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "p", Value: "b"}, {Key: "t", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "p", Value: "b"}, {Key: "t", Value: int32(2)}, {Key: "x", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "p", Value: "b"}, {Key: "t", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(8)}, {Key: "p", Value: "c"}, {Key: "t", Value: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}, {Key: "x", Value: int64(10)}},
		bson.D{{Key: "_id", Value: int32(9)}, {Key: "p", Value: "c"}, {Key: "t", Value: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)}},
		bson.D{{Key: "_id", Value: int32(10)}, {Key: "p", Value: "c"}, {Key: "t", Value: time.Date(2024, 1, 4, 0, 0, 0, 0, time.UTC)}, {Key: "x", Value: 2.5}},
	})
	return err
}

func TestWindowLinearFill(t *testing.T) {
	cases := []struct {
		name string
		spec bson.D
	}{
		{"Partitioned", bson.D{{Key: "partitionBy", Value: "$p"}, {Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}}}},
		{"Descending", bson.D{{Key: "partitionBy", Value: "$p"}, {Key: "sortBy", Value: bson.D{{Key: "t", Value: -1}}}}},
		{"RepeatedSortValues", bson.D{{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}}}},
		{"NullSortValue", bson.D{{Key: "partitionBy", Value: "$p"}, {Key: "sortBy", Value: bson.D{{Key: "x", Value: 1}}}}},
		{"MissingSortValue", bson.D{{Key: "partitionBy", Value: "$p"}, {Key: "sortBy", Value: bson.D{{Key: "nope", Value: 1}}}}},
		{"NoSortBy", bson.D{{Key: "partitionBy", Value: "$p"}}},
		{"CompoundSortBy", bson.D{{Key: "partitionBy", Value: "$p"}, {Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}, {Key: "x", Value: 1}}}}},
	}
	for _, c := range cases {
		spec := append(c.spec, bson.E{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$linearFill", Value: "$x"}}}}})
		harness.PairTest(t, harness.TestCase{
			Name:    "LinearFill_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   linearFillSeed,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$setWindowFields", Value: spec}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "x", Value: 1}, {Key: "r", Value: 1}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}),
		})
	}
	harness.PairTest(t, harness.TestCase{
		Name:    "LinearFill_ValueErrorBeforeRepeatedSortValue",
		Support: harness.DumboDBFull,
		Setup:   linearFillSeed,
		Run: pipelineRun(bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$setWindowFields", Value: bson.D{
				{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
				{Key: "output", Value: bson.D{
					{Key: "r", Value: bson.D{{Key: "$linearFill", Value: "$x"}}},
					{Key: "s", Value: bson.D{{Key: "$linearFill", Value: "$p"}}},
				}},
			}}},
		}),
	})
	for _, e := range []struct {
		name string
		expr interface{}
	}{
		{"Expression", bson.D{{Key: "$multiply", Value: bson.A{"$x", 2}}}},
		{"StringValue", "$p"},
	} {
		harness.PairTest(t, harness.TestCase{
			Name:    "LinearFill_" + e.name,
			Support: harness.DumboDBFull,
			Setup:   linearFillSeed,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$setWindowFields", Value: bson.D{
					{Key: "partitionBy", Value: "$p"},
					{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}},
					{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$linearFill", Value: e.expr}}}}},
				}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}),
		})
	}
}
