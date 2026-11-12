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

// $densify partitions by exact partition values: a missing value differs from
// null and an array is compared whole. Only partitions that hold a value of
// the field are filled. Generating a document whose partition and densified
// paths overlap is an error.

func densifyPartitionsPipeline(spec bson.D) bson.A {
	return bson.A{
		bson.D{{Key: "$densify", Value: spec}},
		bson.D{{Key: "$addFields", Value: bson.D{{Key: "pt", Value: bson.D{{Key: "$type", Value: "$p"}}}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "pt", Value: 1}, {Key: "p", Value: 1}, {Key: "x", Value: 1}, {Key: "a.c", Value: 1}, {Key: "_id", Value: 1}}}},
	}
}

func densifyPartitionsSpec(field string, bounds interface{}, partitions ...string) bson.D {
	spec := bson.D{{Key: "field", Value: field}, {Key: "range", Value: bson.D{{Key: "step", Value: int32(1)}, {Key: "bounds", Value: bounds}}}}
	if len(partitions) > 0 {
		arr := bson.A{}
		for _, p := range partitions {
			arr = append(arr, p)
		}
		spec = append(spec, bson.E{Key: "partitionByFields", Value: arr})
	}
	return spec
}

func TestDensifyPartitions(t *testing.T) {
	nullMissing := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: nil}, {Key: "x", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: int32(5)}},
	}
	arrays := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: bson.A{int32(1), int32(2)}}, {Key: "x", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: int32(1)}, {Key: "x", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: bson.A{int32(1), int32(2)}}, {Key: "x", Value: int32(4)}},
	}
	nullOnly := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "x", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "x", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "b"}, {Key: "x", Value: nil}},
	}
	nested := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: bson.A{int32(1)}}},
	}
	nestedValues := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}, {Key: "c", Value: int32(0)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(1)}, {Key: "c", Value: int32(2)}}}},
	}
	lateStart := []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "p", Value: "a"}, {Key: "x", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "p", Value: "a"}, {Key: "x", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "p", Value: "b"}, {Key: "x", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "p", Value: "b"}, {Key: "x", Value: int32(4)}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "p", Value: "c"}, {Key: "x", Value: 2.5}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "p", Value: "d"}, {Key: "x", Value: int32(1)}},
	}
	explicit := bson.A{int32(0), int32(3)}
	cases := []struct {
		name string
		docs []interface{}
		spec bson.D
		pre  bson.A
	}{
		{"NullMissing_Partition", nullMissing, densifyPartitionsSpec("x", "partition", "p"), nil},
		{"NullMissing_Full", nullMissing, densifyPartitionsSpec("x", "full", "p"), nil},
		{"LateStart_Full", lateStart, densifyPartitionsSpec("x", "full", "p"), nil},
		{"LateStart_Explicit", lateStart, densifyPartitionsSpec("x", bson.A{int32(0), int32(6)}, "p"), nil},
		{"Arrays_Partition", arrays, densifyPartitionsSpec("x", "partition", "p"), nil},
		{"NullOnlyPartition_Full", nullOnly, densifyPartitionsSpec("x", "full", "p"), nil},
		{"NullOnlyPartition_Explicit", nullOnly, densifyPartitionsSpec("x", explicit, "p"), nil},
		{"NullOnlyUnpartitioned_Explicit", nullOnly, densifyPartitionsSpec("x", explicit),
			bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "p", Value: "b"}}}}}},
		{"EmptyInputUnpartitioned_Explicit", nullOnly, densifyPartitionsSpec("x", explicit),
			bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "y", Value: int32(2)}}}}}},
		{"EmptyInputPartitioned_Explicit", nullOnly, densifyPartitionsSpec("x", explicit, "p"),
			bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "y", Value: int32(2)}}}}}},
		{"FieldBelowPartition_Explicit", nested, densifyPartitionsSpec("a.b", explicit, "a"), nil},
		{"FieldBelowPartition_Full", nested, densifyPartitionsSpec("a.b", "full", "a"), nil},
		{"FieldBelowPartition_NoGeneration", nested, densifyPartitionsSpec("a.b", "partition", "a"), nil},
		{"PartitionBelowField", nested, densifyPartitionsSpec("a", explicit, "a.b"), nil},
		{"PartitionBelowMissingField", nested, densifyPartitionsSpec("x", explicit, "x.y"), nil},
		{"FieldBesidePartition", nested, densifyPartitionsSpec("a.c", explicit, "a.b"), nil},
		{"FieldBelowPartitionWithoutValues", nested, densifyPartitionsSpec("a.c", explicit, "a"), nil},
		{"SiblingPaths_Full", nestedValues, densifyPartitionsSpec("a.c", "full", "a.b"), nil},
	}
	for _, c := range cases {
		c := c
		harness.PairTest(t, harness.TestCase{
			Name:    "DensifyPartitions_" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, c.docs)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				pipeline := append(append(bson.A{}, c.pre...), densifyPartitionsPipeline(c.spec)...)
				cursor, err := col.Aggregate(ctx, pipeline)
				if err != nil {
					return nil, err
				}
				var out []bson.D
				err = cursor.All(ctx, &out)
				return out, err
			},
		})
	}
}

func TestDensifyPartitions_NumericBoundsWithUnit(t *testing.T) {
	for name, docs := range map[string][]interface{}{
		"NumericValue": {bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: int32(1)}}},
		"NullValue":    {bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: nil}}},
	} {
		docs := docs
		harness.PairTest(t, harness.TestCase{
			Name:    "DensifyNumericBoundsWithUnit_" + name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				_, err := col.InsertMany(ctx, docs)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, bson.D{{Key: "$densify", Value: bson.D{
					{Key: "field", Value: "x"},
					{Key: "range", Value: bson.D{{Key: "step", Value: int32(1)}, {Key: "bounds", Value: bson.A{int32(0), int32(3)}}, {Key: "unit", Value: "day"}}},
				}}})
			},
		})
	}
}

// $densify emits documents in the order mongod streams them: partitions
// sorted with null and missing tying, generated documents before the one that
// ends their gap, and partition values copied from that document. Under full
// bounds a partition's first value is generated again only when it is one
// step above the minimum. A memory-heavy partition value is shared.
func TestDensifyStreamOrder(t *testing.T) {
	cases := []struct {
		name, docs, stages string
	}{
		{"FullLateStarts", `[{"p":"a","x":0},{"p":"b","x":2},{"p":"c","x":1}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"full"}}},{"$sort":{"p":1,"x":1}}]`},
		{"FullGap", `[{"p":"a","x":0},{"p":"b","x":3},{"p":"a","x":5}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"full"}}},{"$sort":{"p":1,"x":1}}]`},
		{"FullSameMax", `[{"p":"a","x":0},{"p":"a","x":2},{"p":"b","x":2}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"full"}}},{"$sort":{"p":1,"x":1}}]`},
		{"FullLater", `[{"p":"a","x":0},{"p":"a","x":3},{"p":"b","x":2}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"full"}}},{"$sort":{"p":1,"x":1}}]`},
		{"FullStaircase", `[{"p":"a","x":0},{"p":"b","x":1},{"p":"c","x":2},{"p":"d","x":3}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"full"}}},{"$sort":{"p":1,"x":1}}]`},
		{"PartitionUnsorted", `[{"p":"a","x":0},{"p":"b","x":3},{"p":"a","x":5},{"p":"b","x":5}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"partition"}}}]`},
		{"ExplicitUnsorted", `[{"p":"a","x":0},{"p":"b","x":3},{"p":"a","x":5}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":[1,4]}}}]`},
		{"ExplicitNoPartitions", `[{"p":"a","x":0},{"p":"b","x":3},{"p":"a","x":5}]`, `[{"$densify":{"field":"x","range":{"step":2,"bounds":[-3,9]}}}]`},
		{"FullNoPartitions", `[{"p":null,"x":2},{"x":3},{"p":null,"x":5},{"x":6}]`, `[{"$densify":{"field":"x","range":{"step":1,"bounds":"full"}}}]`},
		{"NullAndMissingInterleave", `[{"p":null,"x":2},{"x":3},{"p":null,"x":5},{"x":6}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"partition"}}}]`},
		{"PartitionValueType", `[{"p":1,"x":1},{"p":1.0,"x":4},{"p":{"$numberLong":"1"},"x":6}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"partition"}}},{"$project":{"x":1,"t":{"$type":"$p"}}}]`},
		{"PartitionValueTypeExplicit", `[{"p":1,"x":1},{"p":1.0,"x":4},{"p":{"$numberLong":"1"},"x":6}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":[0,9]}}},{"$project":{"x":1,"t":{"$type":"$p"}}}]`},
		{"SharedPartitionSet", `[{"p":{"q":1},"x":1},{"p":{"q":1},"x":4}]`, `[{"$densify":{"field":"x","partitionByFields":["p"],"range":{"step":1,"bounds":"partition"}}},{"$set":{"p.r":"$x"}},{"$unset":"p.q"}]`},
	}
	for _, c := range cases {
		var docs []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"d":`+c.docs+`}`), false, &struct{ D *[]bson.D }{&docs}); err != nil {
			t.Fatal(c.name, err)
		}
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"s":`+c.stages+`}`), false, &struct{ S *[]bson.D }{&stages}); err != nil {
			t.Fatal(c.name, err)
		}
		stages = append(stages, bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}}}})
		harness.PairTest(t, harness.TestCase{
			Name:    "DensifyStreamOrder_" + c.name,
			Support: harness.DumboDBFull,
			Setup: func(ctx context.Context, col *mongo.Collection) error {
				var ins []interface{}
				for i, d := range docs {
					ins = append(ins, append(bson.D{{Key: "_id", Value: int32(i + 1)}}, d...))
				}
				_, err := col.InsertMany(ctx, ins)
				return err
			},
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}

	harness.PairTest(t, harness.TestCase{
		Name:    "DensifyStreamOrder_LargePartitionValue",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			part := make(bson.A, 100000)
			for i := range part {
				part[i] = int32(i)
			}
			_, err := col.InsertOne(ctx, bson.D{{Key: "_id", Value: int32(1)}, {Key: "part", Value: part}, {Key: "v", Value: int32(0)}})
			return err
		},
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$densify", Value: bson.D{{Key: "field", Value: "v"}, {Key: "partitionByFields", Value: bson.A{"part"}},
					{Key: "range", Value: bson.D{{Key: "step", Value: int32(1)}, {Key: "bounds", Value: bson.A{int32(0), int32(499999)}}}}}}},
				bson.D{{Key: "$count", Value: "n"}})
		},
	})
}
