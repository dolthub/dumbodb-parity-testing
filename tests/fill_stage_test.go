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

// $fill (dolthub/dumbodb#186). Input is sorted by _id first, since sortBy
// ties keep input order.

func fillSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(1)}, {Key: "x", Value: int32(10)}, {Key: "s", Value: "q"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(2)}, {Key: "x", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "p", Value: "a"}, {Key: "t", Value: int32(4)}, {Key: "x", Value: int32(40)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "p", Value: "b"}, {Key: "t", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "p", Value: "b"}, {Key: "t", Value: int32(2)}, {Key: "x", Value: int32(5)}, {Key: "s", Value: nil}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "p", Value: "b"}, {Key: "t", Value: int32(3)}},
	})
	return err
}

func TestFillStage(t *testing.T) {
	method := func(m string) bson.D { return bson.D{{Key: "method", Value: m}} }
	value := func(v interface{}) bson.D { return bson.D{{Key: "value", Value: v}} }
	sortT := bson.E{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}}}
	out := func(fields ...bson.E) bson.E { return bson.E{Key: "output", Value: bson.D(fields)} }
	cases := []struct {
		name string
		spec interface{}
	}{
		{"LinearRepeatedSortValues", bson.D{sortT, out(bson.E{Key: "x", Value: method("linear")})}},
		{"LinearPartitionByFields", bson.D{{Key: "partitionByFields", Value: bson.A{"p"}}, sortT, out(bson.E{Key: "x", Value: method("linear")})}},
		{"LinearDescending", bson.D{{Key: "partitionByFields", Value: bson.A{"p"}}, {Key: "sortBy", Value: bson.D{{Key: "t", Value: -1}}}, out(bson.E{Key: "x", Value: method("linear")})}},
		{"Locf", bson.D{sortT, out(bson.E{Key: "x", Value: method("locf")})}},
		{"LocfDescending", bson.D{{Key: "sortBy", Value: bson.D{{Key: "t", Value: -1}}}, out(bson.E{Key: "x", Value: method("locf")})}},
		{"LocfNoSortBy", bson.D{out(bson.E{Key: "x", Value: method("locf")})}},
		{"LocfAndValuePartitionBy", bson.D{sortT, {Key: "partitionBy", Value: "$p"}, out(bson.E{Key: "x", Value: method("locf")}, bson.E{Key: "s", Value: value("z")})}},
		{"LocfObjectPartitionBy", bson.D{sortT, {Key: "partitionBy", Value: bson.D{{Key: "q", Value: "$p"}}}, out(bson.E{Key: "x", Value: method("locf")})}},
		{"ValueReadsFilledField", bson.D{sortT, {Key: "partitionByFields", Value: bson.A{"p"}}, out(bson.E{Key: "x", Value: method("locf")}, bson.E{Key: "s", Value: value("$x")})}},
		{"Values", bson.D{out(bson.E{Key: "x", Value: value(0)}, bson.E{Key: "s", Value: value(bson.D{{Key: "$concat", Value: bson.A{"$p", "!"}}})})}},
		{"DottedValue", bson.D{sortT, out(bson.E{Key: "x.y", Value: value(1)})}},
		{"EmptyFieldSpec", bson.D{sortT, out(bson.E{Key: "x", Value: bson.D{}})}},
		{"ErrLinearNoSortBy", bson.D{out(bson.E{Key: "x", Value: method("linear")})}},
		{"ErrLinearCompoundSortBy", bson.D{{Key: "sortBy", Value: bson.D{{Key: "t", Value: 1}, {Key: "p", Value: 1}}}, out(bson.E{Key: "x", Value: method("linear")})}},
		{"ErrLinearString", bson.D{sortT, out(bson.E{Key: "x", Value: method("linear")}, bson.E{Key: "s", Value: method("linear")})}},
		{"ErrLinearMissingSortValue", bson.D{{Key: "sortBy", Value: bson.D{{Key: "s", Value: 1}}}, out(bson.E{Key: "x", Value: method("linear")})}},
		{"ErrBadMethod", bson.D{sortT, out(bson.E{Key: "x", Value: method("bogus")})}},
		{"ErrMethodAndValue", bson.D{sortT, out(bson.E{Key: "x", Value: bson.D{{Key: "method", Value: "locf"}, {Key: "value", Value: 1}}})}},
		{"ErrFieldSpecNotObject", bson.D{sortT, out(bson.E{Key: "x", Value: 5})}},
		{"ErrEmptyOutput", bson.D{sortT, {Key: "output", Value: bson.D{}}}},
		{"ErrNullOutput", bson.D{sortT, {Key: "output", Value: nil}}},
		{"NullSortBy", bson.D{{Key: "sortBy", Value: nil}, out(bson.E{Key: "x", Value: value(1)})}},
		{"ErrOutputNotObject", bson.D{{Key: "output", Value: "x"}}},
		{"ErrNoOutput", bson.D{sortT}},
		{"ErrNotObject", int32(5)},
		{"ErrUnknownField", bson.D{sortT, out(bson.E{Key: "x", Value: value(1)}), {Key: "extra", Value: 1}}},
		{"ErrBothPartitions", bson.D{sortT, {Key: "partitionBy", Value: "$p"}, {Key: "partitionByFields", Value: bson.A{"p"}}, out(bson.E{Key: "x", Value: value(1)})}},
		{"ErrSortByNotObject", bson.D{{Key: "sortBy", Value: 1}, out(bson.E{Key: "x", Value: value(1)})}},
		{"ErrPartitionByFieldsNotArray", bson.D{sortT, {Key: "partitionByFields", Value: "p"}, out(bson.E{Key: "x", Value: value(1)})}},
		{"ErrPartitionByFieldsDollar", bson.D{sortT, {Key: "partitionByFields", Value: bson.A{"$p"}}, out(bson.E{Key: "x", Value: value(1)})}},
		{"ErrOutputFieldDollar", bson.D{sortT, out(bson.E{Key: "$x", Value: value(1)})}},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "Fill_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   fillSeed,
			Run: pipelineRun(bson.A{
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				bson.D{{Key: "$fill", Value: c.spec}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			}),
		})
	}
}
