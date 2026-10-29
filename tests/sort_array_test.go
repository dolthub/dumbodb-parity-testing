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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func insertSortArrayDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(0)},
			{Key: "a", Value: bson.A{int32(3), int32(1), int32(2)}},
			{Key: "people", Value: bson.A{
				bson.D{{Key: "age", Value: int32(30)}, {Key: "name", Value: "bo"}},
				bson.D{{Key: "age", Value: int32(25)}, {Key: "name", Value: "al"}},
				bson.D{{Key: "age", Value: int32(30)}, {Key: "name", Value: "ann"}},
			}},
			{Key: "s", Value: "x"},
		},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{"b", "a"}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: nil}},
		bson.D{{Key: "_id", Value: int32(3)}},
	})
	return err
}

// libstdc++'s std::sort, which $sortArray uses, is an insertion sort for up
// to 16 elements and an introsort beyond.
func manyWithTies(n int) bson.A {
	out := bson.A{}
	for i := 0; i < n; i++ {
		out = append(out, bson.D{{Key: "i", Value: int32(i)}, {Key: "k", Value: int32((i * 7) % 5)}})
	}
	return out
}

func TestSortArray(t *testing.T) {
	sortArray := func(input, sortBy interface{}) bson.D {
		return bson.D{{Key: "$sortArray", Value: bson.D{part("input", input), part("sortBy", sortBy)}}}
	}
	doc := func(kv ...interface{}) bson.D {
		d := bson.D{}
		for i := 0; i < len(kv); i += 2 {
			d = append(d, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return d
	}
	people := bson.A{
		doc("age", int32(30), "name", "bo"),
		doc("age", int32(25), "name", "al"),
		doc("age", int32(30), "name", "ann"),
		doc("age", int32(25), "name", "cy"),
	}
	date := time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	mixed := bson.A{
		"b", int32(2), nil, true, bson.D{{Key: "x", Value: int32(1)}}, bson.A{int32(1)}, 1.5,
		date, primitive.NewObjectIDFromTimestamp(date), primitive.Regex{Pattern: "a"}, false,
		primitive.MinKey{}, primitive.MaxKey{}, primitive.Binary{Data: []byte{1}}, primitive.Timestamp{T: 1, I: 1},
		"a", mustDecimal("1.75"), int64(-1),
	}

	cases := []mathCase{
		{"Ascending", sortArray(bson.A{int32(3), int32(1), int32(2)}, int32(1))},
		{"Descending", sortArray(bson.A{int32(3), int32(1), int32(2)}, int32(-1))},
		{"SortByDouble", sortArray(bson.A{int32(3), int32(1)}, 1.0)},
		{"SortByLong", sortArray(bson.A{int32(3), int32(1)}, int64(-1))},
		{"SortByDecimal", sortArray(bson.A{int32(3), int32(1)}, mustDecimal("-1"))},
		{"Strings", sortArray(bson.A{"b", "B", "a", "", "aa"}, int32(1))},
		{"UnicodeStrings", sortArray(bson.A{"é", "e", "f", "E"}, int32(1))},
		{"MixedNumbers", sortArray(bson.A{int32(2), 1.5, int64(1), mustDecimal("1.25"), math.NaN(), math.Inf(-1)}, int32(1))},
		{"EqualNumbersOfDifferentTypes", sortArray(bson.A{int64(1), 1.0, int32(1), mustDecimal("1.0")}, int32(1))},
		{"EqualNumbersDescending", sortArray(bson.A{int64(1), 1.0, int32(1), mustDecimal("1.0")}, int32(-1))},
		{"MixedTypes", sortArray(mixed, int32(1))},
		{"MixedTypesDescending", sortArray(mixed, int32(-1))},
		{"Arrays", sortArray(bson.A{bson.A{int32(2)}, bson.A{int32(1), int32(5)}, bson.A{}, bson.A{int32(1)}}, int32(1))},
		{"Documents", sortArray(bson.A{doc("a", int32(2)), doc("a", int32(1), "b", int32(1)), doc("a", int32(1)), doc()}, int32(1))},
		{"NullAndMissingElements", sortArray(bson.A{int32(1), nil, "$missing", int32(0)}, int32(1))},

		{"ByField", sortArray(people, doc("age", int32(1)))},
		{"ByFieldDescending", sortArray(people, doc("age", int32(-1)))},
		{"ByTwoFields", sortArray(people, doc("age", int32(-1), "name", int32(1)))},
		{"ByFieldStable", sortArray(people, doc("age", int32(1)))},
		{"ByMissingField", sortArray(people, doc("zip", int32(1)))},
		{"ByFieldSomeMissing", sortArray(bson.A{doc("a", int32(2)), doc("b", int32(1)), doc("a", nil), doc("a", int32(1))}, doc("a", int32(1)))},
		{"ByFieldNonDocuments", sortArray(bson.A{int32(5), doc("a", int32(2)), "x", doc("a", nil), doc("a", int32(-1))}, doc("a", int32(1)))},
		{"ByFieldNonDocumentsDescending", sortArray(bson.A{int32(5), doc("a", int32(2)), "x", doc("a", int32(-1))}, doc("a", int32(-1)))},
		{"ByDottedField", sortArray(bson.A{doc("a", doc("b", int32(2))), doc("a", doc("b", int32(1))), doc("a", int32(0))}, doc("a.b", int32(1)))},
		{"ByDottedThroughArray", sortArray(bson.A{doc("a", bson.A{doc("b", int32(2))}), doc("a", bson.A{doc("b", int32(1))}), doc("a", doc("b", int32(3)))}, doc("a.b", int32(1)))},
		{"ByArrayIndexPath", sortArray(bson.A{doc("a", bson.A{int32(3), int32(1)}), doc("a", bson.A{int32(2), int32(9)})}, doc("a.1", int32(1)))},
		{"ByArrayValuedField", sortArray(bson.A{doc("a", bson.A{int32(3), int32(1)}), doc("a", bson.A{int32(2), int32(9)}), doc("a", int32(2))}, doc("a", int32(1)))},
		{"ByFieldMixedTypes", sortArray(bson.A{doc("a", "x"), doc("a", int32(1)), doc("a", nil), doc("a", bson.D{})}, doc("a", int32(1)))},
		{"ByFieldNumberTypes", sortArray(bson.A{doc("a", int64(2)), doc("a", 1.5), doc("a", mustDecimal("1.75"))}, doc("a", int32(-1)))},
		{"ByFieldDoubleDirection", sortArray(people, doc("age", -1.0))},

		{"Empty", sortArray(bson.A{}, int32(1))},
		{"Single", sortArray(bson.A{"x"}, int32(-1))},
		{"SingleDocumentByField", sortArray(bson.A{int32(5)}, doc("a", int32(1)))},
		{"FieldInput", sortArray("$a", int32(1))},
		{"ExpressionInput", sortArray(bson.D{{Key: "$concatArrays", Value: bson.A{bson.A{int32(2)}, bson.A{int32(1)}}}}, int32(1))},
		{"FieldOfDocuments", sortArray("$people", doc("name", int32(-1)))},
		{"SixteenWithTies", sortArray(manyWithTies(16), doc("k", int32(1)))},
		{"SeventeenWithTies", sortArray(manyWithTies(17), doc("k", int32(1)))},
		{"ManyWithTies", sortArray(manyWithTies(100), doc("k", int32(1)))},
		{"ManyWithTiesDescending", sortArray(manyWithTies(100), doc("k", int32(-1)))},
		{"ManyWithTiesWholeDocuments", sortArray(manyWithTies(40), int32(-1))},

		{"InputNull", sortArray(nil, int32(1))},
		{"InputMissing", sortArray("$missing", int32(1))},
		{"InputNullField", sortArray("$n", int32(1))},
		{"InputString", sortArray("abc", int32(1))},
		{"InputStringField", sortArray("$s", int32(1))},
		{"InputDocument", sortArray(doc("a", int32(1)), int32(1))},
		{"InputNumber", sortArray(int32(1), int32(1))},
		{"InputStringBadSortBy", sortArray("abc", int32(2))},

		{"SortByZero", sortArray(bson.A{int32(1)}, int32(0))},
		{"SortByTwo", sortArray(bson.A{int32(1)}, int32(2))},
		{"SortByHalf", sortArray(bson.A{int32(1)}, 0.5)},
		{"SortByString", sortArray(bson.A{int32(1)}, "1")},
		{"SortByFieldPath", sortArray(bson.A{int32(1)}, "$a")},
		{"SortByBool", sortArray(bson.A{int32(1)}, true)},
		{"SortByNull", sortArray(bson.A{int32(1)}, nil)},
		{"SortByArray", sortArray(bson.A{int32(1)}, bson.A{int32(1)})},
		{"SortByEmptyDocument", sortArray(bson.A{int32(1)}, doc())},
		{"SortByFieldZero", sortArray(bson.A{int32(1)}, doc("a", int32(0)))},
		{"SortByFieldString", sortArray(bson.A{int32(1)}, doc("a", "asc"))},
		{"SortByFieldMeta", sortArray(bson.A{int32(1)}, doc("a", doc("$meta", "textScore")))},
		{"SortByEmptyFieldName", sortArray(bson.A{int32(1)}, doc("", int32(1)))},
		{"SortByEmptyPathPart", sortArray(bson.A{int32(1)}, doc("a..b", int32(1)))},
		{"SortByTrailingDot", sortArray(bson.A{int32(1)}, doc("a.", int32(1)))},
		{"SortByDollarField", sortArray(bson.A{int32(1)}, doc("$a", int32(1)))},
		{"SortByExpression", sortArray(bson.A{int32(1)}, doc("$add", bson.A{int32(1)}))},

		{"NoInput", bson.D{{Key: "$sortArray", Value: bson.D{part("sortBy", int32(1))}}}},
		{"NoSortBy", bson.D{{Key: "$sortArray", Value: bson.D{part("input", bson.A{})}}}},
		{"UnknownField", bson.D{{Key: "$sortArray", Value: bson.D{part("input", bson.A{}), part("sortBy", int32(1)), part("order", int32(1))}}}},
		{"UnknownFieldBeforeBadSortBy", bson.D{{Key: "$sortArray", Value: bson.D{part("order", int32(1)), part("sortBy", int32(5))}}}},
		{"BadSortByBeforeUnknownField", bson.D{{Key: "$sortArray", Value: bson.D{part("sortBy", int32(5)), part("order", int32(1))}}}},
		{"NonObjectArgument", bson.D{{Key: "$sortArray", Value: bson.A{int32(2), int32(1)}}}},
		{"StringArgument", bson.D{{Key: "$sortArray", Value: "$a"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "SortArray_" + tc.name,
				Support: harness.DumboDBXFail,
				Setup:   insertSortArrayDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}

	pipelines := []struct {
		name     string
		pipeline bson.A
	}{
		{"AllDocs", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: sortArray("$a", int32(-1))}}}},
		}},
		{"AllDocsByField", bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: sortArray("$people", doc("age", int32(1), "name", int32(-1)))}}}},
		}},
		{"GroupPush", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(2)}}}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "ids", Value: bson.D{{Key: "$push", Value: "$_id"}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "ids", Value: sortArray("$ids", int32(-1))}}}},
		}},
		{"MatchExpr", bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{
				bson.D{{Key: "$arrayElemAt", Value: bson.A{sortArray("$a", int32(1)), int32(0)}}}, int32(1),
			}}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
		}},
	}
	for _, p := range pipelines {
		t.Run(p.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "SortArray_" + p.name,
				Support: harness.DumboDBXFail,
				Setup:   insertSortArrayDocs,
				Run:     pipelineRunExtJSON(p.pipeline),
			})
		})
	}
}
