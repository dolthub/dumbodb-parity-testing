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

// A component with a leading zero names a field, not an array index. A null
// local value matches a foreign path that a document lacks, but not one that
// ends at a scalar array element.
func TestLookupLeadingZeroAndNullPaths(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		var local, foreign []interface{}
		for _, js := range []string{
			`{"_id":1,"k":[{"m":5},{"m":6}]}`, `{"_id":2,"k":[7,8]}`, `{"_id":3,"k":{"01":8}}`,
		} {
			var d bson.D
			if err := bson.UnmarshalExtJSON([]byte(js), false, &d); err != nil {
				return err
			}
			local = append(local, d)
		}
		for _, js := range []string{
			`{"_id":10,"a":[]}`, `{"_id":11,"a":[1]}`, `{"_id":12,"a":[[{"c":1}]]}`, `{"_id":13,"a":[{"c":1}]}`,
			`{"_id":14,"a":[5,{"b":2}]}`, `{"_id":15,"a":5}`, `{"_id":16,"a":[[1]]}`, `{"_id":17}`,
			`{"_id":18,"a":[{"b":[]}]}`, `{"_id":19,"a":[7,8]}`, `{"_id":20,"a":{"01":8,"00":7}}`,
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
		{"NullThroughField", "zz", "a.b"},
		{"NullThroughIndex", "zz", "a.1"},
		{"NullThroughLeadingZero", "zz", "a.01"},
		{"NullThroughIndexThenField", "zz", "a.0.b"},
		{"NullThroughTwoIndexes", "zz", "a.0.0"},
		{"LocalLeadingZero", "k.01", "a.01"},
		{"LocalIndexForeignLeadingZero", "k.1", "a.01"},
		{"LocalDoubleZero", "k.00", "a.00"},
	}
	for _, c := range cases {
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLeadingZeroAndNullPaths_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run:     lookupPathsRun(c.local, c.foreign, "j"),
		})
	}
	filters := map[string]string{
		"FindLeadingZero":        `{"a.01":8}`,
		"FindIndex":              `{"a.1":8}`,
		"FindPlusSign":           `{"a.+1":8}`,
		"FindLeadingZeroExists":  `{"a.01":{"$exists":true}}`,
		"FindIndexThenFieldNull": `{"a.0.b":null}`,
		"FindTwoIndexesNull":     `{"a.0.0":null}`,
	}
	for name, js := range filters {
		var filter bson.D
		if err := bson.UnmarshalExtJSON([]byte(js), false, &filter); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupLeadingZeroAndNullPaths_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col.Database().Collection(col.Name()+"_f"),
					bson.D{{Key: "$match", Value: filter}},
					bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
				)
			},
		})
	}
}

// A $getField without input reads the current document, so a $match that
// reads the $lookup output through it stays after the $lookup, and the
// expression is not folded to a constant.
func TestLookupGetFieldReadsAs(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		if _, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "k", Value: int32(1)}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "k", Value: int32(2)}},
		}); err != nil {
			return err
		}
		_, err := col.Database().Collection(col.Name()+"_f").InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: int32(10)}, {Key: "k", Value: int32(1)}},
		})
		return err
	}
	pipelines := map[string]string{
		"MatchGetField":         `[{"$match":{"$expr":{"$eq":[{"$size":{"$getField":"j"}},1]}}}]`,
		"MatchGetFieldLiteral":  `[{"$match":{"$expr":{"$eq":[{"$size":{"$getField":{"$literal":"j"}}},0]}}}]`,
		"MatchLetCurrent":       `[{"$match":{"$expr":{"$let":{"vars":{"CURRENT":"$$CURRENT"},"in":{"$eq":[{"$size":"$j"},1]}}}}}]`,
		"ProjectGetField":       `[{"$project":{"n":{"$size":{"$getField":"j"}}}}]`,
		"ProjectGetFieldInput":  `[{"$project":{"n":{"$size":{"$getField":{"field":"j","input":{"j":[1,2]}}}}}}]`,
		"ProjectGetFieldIntErr": `[{"$project":{"n":{"$size":{"$getField":{"field":"j","input":{"$literal":{"j":5}}}}}}}]`,
		"ProjectGetFieldNull":   `[{"$project":{"n":{"$getField":{"field":"j","input":null}}}}]`,
	}
	for name, js := range pipelines {
		var stages []bson.D
		if err := bson.UnmarshalExtJSON([]byte(`{"p":`+js+`}`), false, &struct{ P *[]bson.D }{&stages}); err != nil {
			t.Fatal(name, err)
		}
		harness.PairTest(t, harness.TestCase{
			Name:    "LookupGetFieldReadsAs_" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				lookup := bson.D{{Key: "$lookup", Value: bson.D{
					{Key: "from", Value: col.Name() + "_f"}, {Key: "localField", Value: "k"},
					{Key: "foreignField", Value: "k"}, {Key: "as", Value: "j"},
				}}}
				all := append([]bson.D{lookup}, stages...)
				all = append(all, bson.D{{Key: "$project", Value: bson.D{{Key: "j", Value: 0}}}},
					bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})
				return fieldOpsAgg(ctx, col, all...)
			},
		})
	}
}

// $graphLookup's startWith is an expression: mongod checks its paths,
// variables and operators while parsing, and evaluates it per document
// without folding its constants.
func TestGraphLookupStartWith(t *testing.T) {
	cases := map[string]string{
		"EmptyPathPart":     `"$a..b"`,
		"DollarPathPart":    `"$a.$b"`,
		"UndefinedVariable": `"$$nope"`,
		"VariableInOp":      `{"$add":["$$nope",1]}`,
		"UnknownOperator":   `{"$bogus":1}`,
		"WrongArgCount":     `{"$abs":[1,2]}`,
		"ConstantError":     `{"$divide":[1,0]}`,
		"FieldError":        `{"$divide":[1,"$n"]}`,
		"RootPath":          `"$$ROOT.a"`,
		"Operator":          `{"$add":["$a",1]}`,
		"ArrayOfPaths":      `["$a","$arr"]`,
		"ArrayField":        `"$arr"`,
		"Constant":          `2`,
		"NoMatchSwitch":     `{"$switch":{"branches":[{"case":false,"then":"$a"}]}}`,
	}
	for name, js := range cases {
		var holder struct{ S interface{} }
		if err := bson.UnmarshalExtJSON([]byte(`{"s":`+js+`}`), false, &holder); err != nil {
			t.Fatal(name, err)
		}
		for _, empty := range []bool{false, true} {
			testName := "GraphLookupStartWith_" + name
			if empty {
				testName += "_Empty"
			}
			empty := empty
			harness.PairTest(t, harness.TestCase{
				Name:    testName,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if empty {
						return col.Database().CreateCollection(ctx, col.Name())
					}
					_, err := col.InsertMany(ctx, []interface{}{
						bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: int32(1)}, {Key: "n", Value: int32(0)},
							{Key: "arr", Value: bson.A{int32(1), bson.A{int32(2)}}}},
						bson.D{{Key: "_id", Value: int32(2)}, {Key: "a", Value: int32(2)}, {Key: "n", Value: int32(1)}},
					})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return fieldOpsAgg(ctx, col,
						bson.D{{Key: "$graphLookup", Value: bson.D{
							{Key: "from", Value: col.Name()}, {Key: "startWith", Value: holder.S},
							{Key: "connectFromField", Value: "zz"}, {Key: "connectToField", Value: "_id"}, {Key: "as", Value: "g"},
						}}},
						bson.D{{Key: "$project", Value: bson.D{{Key: "g", Value: bson.D{{Key: "$sortArray", Value: bson.D{
							{Key: "input", Value: "$g._id"}, {Key: "sortBy", Value: int32(1)},
						}}}}}}},
						bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
					)
				},
			})
		}
	}
}
