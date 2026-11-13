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
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// Aggregation comparison operators compare whole values in BSON order, so an
// array is not matched element by element (dolthub/dumbodb#236).

func arrayComparisonSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "b", Value: bson.A{int32(1), int32(9)}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "b", Value: bson.A{}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "b", Value: int32(5)}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "b", Value: bson.A{bson.A{int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(5)}, {Key: "b", Value: bson.D{{Key: "x", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(6)}, {Key: "b", Value: nil}},
		bson.D{{Key: "_id", Value: int32(7)}},
	})
	return err
}

func TestAggregationArrayComparison(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayComparison_FindExpr",
		Support: harness.DumboDBFull,
		Setup:   arrayComparisonSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Find(ctx, bson.D{{Key: "$expr", Value: bson.D{{Key: "$gt", Value: bson.A{"$b", int32(1)}}}}},
				options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
			if err != nil {
				return nil, err
			}
			var out []bson.D
			err = cursor.All(ctx, &out)
			return out, err
		},
	})
	harness.PairTest(t, harness.TestCase{
		Name:    "ArrayComparison_Operators",
		Support: harness.DumboDBFull,
		Setup:   arrayComparisonSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "cmp", Value: bson.D{{Key: "$cmp", Value: bson.A{"$b", int32(1)}}}},
					{Key: "eq", Value: bson.D{{Key: "$eq", Value: bson.A{"$b", bson.A{int32(1), int32(9)}}}}},
					{Key: "lt", Value: bson.D{{Key: "$lt", Value: bson.A{"$b", bson.A{int32(2)}}}}},
					{Key: "ne", Value: bson.D{{Key: "$ne", Value: bson.A{"$b", int32(5)}}}},
					{Key: "gte", Value: bson.D{{Key: "$gte", Value: bson.A{"$b", bson.A{}}}}},
					{Key: "lte", Value: bson.D{{Key: "$lte", Value: bson.A{"$b", bson.D{{Key: "x", Value: int32(1)}}}}}},
					{Key: "prefix", Value: bson.D{{Key: "$cmp", Value: bson.A{bson.A{int32(1), int32(2)}, bson.A{int32(1), int32(2), int32(0)}}}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			)
		},
	})
}

// More places that compare values as aggregation does, not as a query: $in
// and $indexOfArray match whole values, window ranks tie as $sort orders, a
// range window needs numeric sortBy values, a $sort or window sortBy on _id
// alone compares arrays whole, and the set operators compare whole values
// and report a non-array operand with their own codes.
func TestAggregationComparisonSiblings(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{int32(3), int32(1)}}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: bson.A{int32(2)}}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "a", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(4)}, {Key: "a", Value: bson.A{int32(1), int32(5)}}},
			bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: bson.A{}}},
		})
		return err
	}
	pipelines := map[string]string{
		"InAndIndexOfArray":   `[{"$match":{"_id":1}},{"$project":{"r":{"$in":[1,[[1],2]]},"s":{"$indexOfArray":[[[1,2],1],1]},"t":{"$in":[[1],[[1],2]]},"u":{"$indexOfArray":[[[1],1],[1]]}}}]`,
		"RankAscending":       `[{"$setWindowFields":{"sortBy":{"a":1},"output":{"r":{"$rank":{}},"d":{"$denseRank":{}}}}},{"$sort":{"_id":1}}]`,
		"RankDescending":      `[{"$setWindowFields":{"sortBy":{"a":-1},"output":{"r":{"$rank":{}},"d":{"$denseRank":{}}}}},{"$sort":{"_id":1}}]`,
		"RangeWindowArray":    `[{"$setWindowFields":{"sortBy":{"a":1},"output":{"s":{"$sum":1,"window":{"range":[-1,1]}}}}}]`,
		"RangeWindowMissing":  `[{"$set":{"b":"$zz"}},{"$setWindowFields":{"sortBy":{"b":1},"output":{"s":{"$sum":1,"window":{"range":["unbounded","current"]}}}}}]`,
		"RangeWindowNumbers":  `[{"$setWindowFields":{"sortBy":{"_id":1},"output":{"s":{"$sum":1,"window":{"range":[-1,1]}}}}},{"$sort":{"_id":1}}]`,
		"SortGroupIDAsc":      `[{"$group":{"_id":"$a"}},{"$sort":{"_id":1}}]`,
		"SortGroupIDDesc":     `[{"$group":{"_id":"$a"}},{"$sort":{"_id":-1}}]`,
		"SortProjectedIDDesc": `[{"$project":{"_id":"$a"}},{"$sort":{"_id":-1}}]`,
		"SortGroupIDLimit":    `[{"$group":{"_id":"$a"}},{"$sort":{"_id":1}},{"$limit":2}]`,
		"SetOperators":        `[{"$match":{"_id":1}},{"$project":{"i":{"$setIntersection":[[[1]],[1]]},"s":{"$setIsSubset":[[[1]],[1]]},"e":{"$setEquals":[[[1]],[1]]},"d":{"$setDifference":[[[1]],[1]]},"u":{"$setUnion":[[1,[1]],[[1]]]}}}]`,
		"SetOperatorsFields":  `[{"$project":{"i":{"$setIntersection":[["$a"],[1]]},"d":{"$setDifference":["$a",[[1]]]},"u":{"$setUnion":[[1],["$a"]]}}},{"$sort":{"_id":1}}]`,
		"WindowGroupIDAsc":    `[{"$group":{"_id":"$a"}},{"$setWindowFields":{"sortBy":{"_id":1},"output":{"r":{"$documentNumber":{}}}}}]`,
		"WindowGroupIDDesc":   `[{"$group":{"_id":"$a"}},{"$setWindowFields":{"sortBy":{"_id":-1},"output":{"r":{"$rank":{}}}}}]`,
		"WindowSetIDAsc":      `[{"$set":{"_id":"$a"}},{"$setWindowFields":{"sortBy":{"_id":1},"output":{"r":{"$documentNumber":{}}}}}]`,
		"SetUnionNotArray":    `[{"$project":{"u":{"$setUnion":["$a",[1]]}}},{"$sort":{"_id":1}}]`,
		"SetEqualsNotArray":   `[{"$project":{"u":{"$setEquals":[[1],"$a"]}}},{"$sort":{"_id":1}}]`,
		"SetEqualsOneArg":     `[{"$project":{"u":{"$setEquals":[[1]]}}}]`,
		"SetIsSubsetFirst":    `[{"$project":{"u":{"$setIsSubset":["$a",[1]]}}},{"$sort":{"_id":1}}]`,
		"SetIsSubsetConstant": `[{"$project":{"u":{"$setIsSubset":["$a",5]}}}]`,
		"SetDifferenceNull":   `[{"$project":{"u":{"$setDifference":["$a",null]}}},{"$sort":{"_id":1}}]`,
		"AnyElementTrueNull":  `[{"$project":{"u":{"$anyElementTrue":["$zz"]}}}]`,
		"AllElementsTrueInt":  `[{"$match":{"_id":3}},{"$project":{"u":{"$allElementsTrue":["$a"]}}}]`,
		"SetUnionNotArraySBE": `[{"$group":{"_id":null,"u":{"$push":{"$setUnion":["$a",[1]]}}}}]`,
	}
	for name, js := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *[]bson.D }{&stages}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "AggregationComparisonSiblings_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, stages...)
			},
		})
	}
}
