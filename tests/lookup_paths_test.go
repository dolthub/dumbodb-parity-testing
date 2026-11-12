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

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// An equality $lookup reads a dotted localField through arrays, and matches
// foreignField as a query on that path does. Numbers compare exactly, and NaN
// equals NaN.

func lookupPathsRun(local, foreign, as string) func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return fieldOpsAgg(ctx, col,
			bson.D{{Key: "$lookup", Value: bson.D{
				{Key: "from", Value: col.Name() + "_f"},
				{Key: "localField", Value: local},
				{Key: "foreignField", Value: foreign},
				{Key: "as", Value: as},
			}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "j", Value: bson.D{{Key: "$sortArray", Value: bson.D{
				{Key: "input", Value: "$" + as + "._id"}, {Key: "sortBy", Value: int32(1)},
			}}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		)
	}
}

func lookupPathsSeed(ctx context.Context, col *mongo.Collection) error {
	if _, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{bson.D{{Key: "b", Value: int32(1)}}, bson.D{{Key: "b", Value: int32(2)}}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(3)}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "x", Value: nil}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{bson.D{{Key: "c", Value: int32(1)}}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.A{bson.A{bson.D{{Key: "b", Value: int32(1)}}}}}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "a", Value: bson.D{{Key: "b", Value: bson.A{bson.A{int32(1)}, int32(2)}}}}},
		bson.D{{Key: "_id", Value: int32(7)}, {Key: "a", Value: bson.A{bson.D{{Key: "b", Value: bson.A{}}}}}},
	}); err != nil {
		return err
	}
	_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(10)}, {Key: "k", Value: int32(1)}},
		bson.D{{Key: "_id", Value: int32(11)}, {Key: "k", Value: int32(2)}},
		bson.D{{Key: "_id", Value: int32(12)}, {Key: "k", Value: int32(3)}},
		bson.D{{Key: "_id", Value: int32(13)}, {Key: "k", Value: nil}},
		bson.D{{Key: "_id", Value: int32(14)}},
		bson.D{{Key: "_id", Value: int32(15)}, {Key: "k", Value: bson.A{int32(1), int32(5)}}},
		bson.D{{Key: "_id", Value: int32(16)}, {Key: "q", Value: bson.A{bson.D{{Key: "k", Value: int32(2)}}, bson.D{{Key: "k", Value: int32(7)}}}}},
		bson.D{{Key: "_id", Value: int32(17)}, {Key: "k", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: int32(18)}, {Key: "k", Value: bson.A{bson.A{int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(19)}, {Key: "q", Value: bson.A{bson.D{{Key: "k", Value: int32(1)}}, bson.D{{Key: "z", Value: int32(1)}}}}},
		bson.D{{Key: "_id", Value: int32(20)}, {Key: "q", Value: int32(5)}},
	})
	return err
}

func TestLookupPaths(t *testing.T) {
	cases := []struct {
		name, local, foreign, as string
	}{
		{"LocalThroughArrays", "a.b", "k", "j"},
		{"ForeignThroughArrays", "_id", "q.k", "j"},
		{"NullLocalForeignThroughArrays", "x", "q.k", "j"},
		{"BothThroughArrays", "a.b", "q.k", "j"},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupPaths_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   lookupPathsSeed,
			Run:     lookupPathsRun(c.local, c.foreign, c.as),
		})
	}

	harness.PairTest(t, harness.TestCase{
		Name:    "LookupPaths_NumbersCompareExactly",
		Support: harness.DumboDBFull,
		Setup: func(ctx context.Context, col *mongo.Collection) error {
			docs := []interface{}{
				bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: math.NaN()}},
				bson.D{{Key: "_id", Value: int32(2)}, {Key: "v", Value: int64(9007199254740993)}},
				bson.D{{Key: "_id", Value: int32(3)}, {Key: "v", Value: dz("NaN")}},
				bson.D{{Key: "_id", Value: int32(4)}, {Key: "v", Value: 9007199254740992.0}},
				bson.D{{Key: "_id", Value: int32(5)}, {Key: "v", Value: int64(9007199254740992)}},
			}
			if _, err := col.InsertMany(ctx, docs); err != nil {
				return err
			}
			_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, docs)
			return err
		},
		Run: lookupPathsRun("v", "v", "j"),
	})
}

// A numeric path component indexes into an array. In foreignField an array
// reached by an index is compared whole.
func TestLookupNumericPaths(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		var local, foreign []interface{}
		for _, js := range []string{
			`{"_id":1,"a":[5,6]}`, `{"_id":2,"a":[{"b":5},{"b":6}]}`, `{"_id":3,"a":[{"0":6},{"1":5}]}`,
			`{"_id":4,"a":[[5],[6]]}`, `{"_id":5,"a":{"0":5}}`, `{"_id":6,"k":[1]}`, `{"_id":7,"a":[{"0":[5,6]}]}`,
			`{"_id":8,"v":5}`, `{"_id":9,"v":6}`,
		} {
			var d bson.D
			if err := bson.UnmarshalExtJSON([]byte(js), false, &d); err != nil {
				return err
			}
			local = append(local, d)
		}
		for _, js := range []string{
			`{"_id":10,"k":5}`, `{"_id":11}`, `{"_id":12,"k":6}`, `{"_id":13,"k":[5,7]}`, `{"_id":14,"k":[[5]]}`,
			`{"_id":15,"q":[{"k":5},{"k":6}]}`, `{"_id":16,"q":[{"0":{"k":6}}]}`, `{"_id":17,"k":{"0":5}}`,
			`{"_id":18,"k":[{"0":6}]}`, `{"_id":19,"q":{"0":{"k":5}}}`,
		} {
			var d bson.D
			if err := bson.UnmarshalExtJSON([]byte(js), false, &d); err != nil {
				return err
			}
			foreign = append(foreign, d)
		}
		if _, err := col.InsertMany(ctx, local); err != nil {
			return err
		}
		_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, foreign)
		return err
	}
	cases := []struct{ name, local, foreign string }{
		{"LocalIndex", "a.0", "k"},
		{"LocalSecondIndex", "a.1", "k"},
		{"LocalIndexThenField", "a.1.b", "k"},
		{"LocalTwoIndexes", "a.0.0", "k"},
		{"LocalIndexOfOneElement", "k.0", "_id"},
		{"ForeignIndex", "v", "k.0"},
		{"ForeignIndexThenField", "v", "q.0.k"},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupNumericPaths_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run:     lookupPathsRun(c.local, c.foreign, "j"),
		})
	}
}
