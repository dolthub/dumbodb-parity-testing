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
	"fmt"
	"sort"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

type symbolJavaScriptCase struct {
	name  string
	setup func(context.Context, *mongo.Collection) error
	run   func(context.Context, *mongo.Collection) (interface{}, error)
}

func runSymbolJavaScriptCases(t *testing.T, cases []symbolJavaScriptCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup runs inside Run so a server that rejects the fixture
			// diverges instead of failing setup, which XFail does not cover.
			harness.PairTest(t, harness.TestCase{
				Name:      "SymJS_" + tc.name,
				Support:   harness.DumboDBFull,
				Clustered: true,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					if tc.setup != nil {
						if err := tc.setup(ctx, col); err != nil {
							return nil, err
						}
					}
					return tc.run(ctx, col)
				},
			})
		})
	}
}

var (
	symAbc = primitive.Symbol("abc")
	symAbd = primitive.Symbol("abd")
	jsAbc  = primitive.JavaScript("abc")
	jsFunc = primitive.JavaScript("function () { return 1 }")
)

func symJSDoc(id int32, v interface{}) bson.D {
	return bson.D{{Key: "_id", Value: id}, {Key: "v", Value: v}}
}

func insertSymbolJavaScriptMixed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		symJSDoc(1, "abc"),
		symJSDoc(2, symAbc),
		symJSDoc(3, symAbd),
		symJSDoc(4, "abd"),
		symJSDoc(5, jsAbc),
		symJSDoc(6, jsFunc),
		symJSDoc(7, primitive.Regex{Pattern: "a"}),
		symJSDoc(8, primitive.MaxKey{}),
		symJSDoc(9, int32(5)),
		symJSDoc(10, bson.D{{Key: "s", Value: primitive.Symbol("x")}, {Key: "c", Value: primitive.JavaScript("y")}}),
		symJSDoc(11, bson.A{primitive.Symbol("x"), "x", primitive.JavaScript("x")}),
		symJSDoc(12, true),
		symJSDoc(13, primitive.Timestamp{T: 1, I: 1}),
	})
	return err
}

func insertSymbolJavaScriptStrings(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		symJSDoc(1, "abd"),
		symJSDoc(2, symAbc),
		symJSDoc(3, "abc"),
		symJSDoc(4, symAbd),
	})
	return err
}

func withValueIndex(setup func(context.Context, *mongo.Collection) error) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		if err := setup(ctx, col); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "v", Value: 1}}})
		return err
	}
}

func findDocs(filter interface{}, opts *options.FindOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, filter, opts)
		if err != nil {
			return nil, err
		}
		var docs []bson.D
		if err := cursor.All(ctx, &docs); err != nil {
			return nil, err
		}
		return docs, nil
	}
}

func aggregateDocs(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, err
		}
		var docs []bson.D
		if err := cursor.All(ctx, &docs); err != nil {
			return nil, err
		}
		return docs, nil
	}
}

// aggregateSortedByID sorts the results by _id on the client: mongod's $sort
// after $group returns Symbol and JavaScript group keys as strings.
func aggregateSortedByID(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		res, err := aggregateDocs(pipeline)(ctx, col)
		if err != nil {
			return nil, err
		}
		docs := res.([]bson.D)
		key := func(d bson.D) string { return fmt.Sprintf("%T %v", d[0].Value, d[0].Value) }
		sort.SliceStable(docs, func(i, j int) bool { return key(docs[i]) < key(docs[j]) })
		return docs, nil
	}
}

func insertDocs(docs ...interface{}) func(context.Context, *mongo.Collection) error {
	return func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, docs)
		return err
	}
}

func updateThenFind(filter, update interface{}, opts ...*options.UpdateOptions) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		if _, err := col.UpdateOne(ctx, filter, update, opts...); err != nil {
			return nil, err
		}
		return findDocs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
	}
}

func TestSymbolJavaScript_RoundTrip(t *testing.T) {
	nested := bson.D{
		{Key: "_id", Value: int32(1)},
		{Key: "s", Value: symAbc},
		{Key: "j", Value: jsAbc},
		{Key: "d", Value: bson.D{{Key: "s", Value: symAbd}, {Key: "j", Value: jsFunc}}},
		{Key: "a", Value: bson.A{primitive.Symbol("x"), "x", primitive.JavaScript("x")}},
		{Key: "n", Value: int32(1)},
	}
	byID := bson.D{{Key: "_id", Value: int32(1)}}
	padding := strings.Repeat("p", 200000)

	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"FindOne", insertDocs(nested), findDocs(byID, nil)},
		{"FindAllNaturalOrder", insertSymbolJavaScriptMixed, findDocs(bson.D{}, nil)},
		{"InclusionProjection", insertDocs(nested),
			findDocs(byID, options.Find().SetProjection(bson.D{{Key: "s", Value: 1}, {Key: "d.j", Value: 1}, {Key: "a", Value: 1}}))},
		{"ExclusionProjection", insertDocs(nested),
			findDocs(byID, options.Find().SetProjection(bson.D{{Key: "j", Value: 0}}))},
		{"RewriteOtherField", insertDocs(nested),
			updateThenFind(byID, bson.D{{Key: "$set", Value: bson.D{{Key: "n", Value: int32(2)}}}})},
		{"ReplaceOne", insertDocs(symJSDoc(1, "abc")),
			func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				if _, err := col.ReplaceOne(ctx, byID, nested); err != nil {
					return nil, err
				}
				return findDocs(byID, nil)(ctx, col)
			}},
		{"LargeDocument",
			insertDocs(bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: padding}, {Key: "s", Value: symAbc}, {Key: "j", Value: jsAbc}}),
			findDocs(byID, options.Find().SetProjection(bson.D{{Key: "a", Value: 0}}))},
		{"LargeDocumentFull",
			insertDocs(bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: padding}, {Key: "s", Value: symAbc}, {Key: "j", Value: jsAbc}}),
			findDocs(byID, nil)},
		{"BSONSize", insertDocs(nested), aggregateDocs(bson.A{
			bson.D{{Key: "$project", Value: bson.D{{Key: "size", Value: bson.D{{Key: "$bsonSize", Value: "$$ROOT"}}}}}},
		})},
		{"AggregatePassThrough", insertSymbolJavaScriptMixed, aggregateDocs(bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		})},
	})
}

func TestSymbolJavaScript_QueryMatching(t *testing.T) {
	filters := []struct {
		name   string
		filter bson.D
	}{
		{"EqString", bson.D{{Key: "v", Value: "abc"}}},
		{"EqSymbol", bson.D{{Key: "v", Value: symAbc}}},
		{"EqJavaScript", bson.D{{Key: "v", Value: jsAbc}}},
		{"GtString", bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: "abb"}}}}},
		{"LtString", bson.D{{Key: "v", Value: bson.D{{Key: "$lt", Value: "abd"}}}}},
		{"GteSymbol", bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: symAbd}}}}},
		{"GtJavaScript", bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: primitive.JavaScript("a")}}}}},
		{"InMixed", bson.D{{Key: "v", Value: bson.D{{Key: "$in", Value: bson.A{"abc", jsFunc}}}}}},
		{"TypeSymbolAlias", bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "symbol"}}}}},
		{"TypeSymbolCode", bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: int32(14)}}}}},
		{"TypeJavaScriptAlias", bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "javascript"}}}}},
		{"TypeJavaScriptCode", bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: int32(13)}}}}},
		{"TypeString", bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: "string"}}}}},
		{"TypeStringOrSymbol", bson.D{{Key: "v", Value: bson.D{{Key: "$type", Value: bson.A{"string", "symbol"}}}}}},
		{"Regex", bson.D{{Key: "v", Value: bson.D{{Key: "$regex", Value: "^ab"}}}}},
		{"Ne", bson.D{{Key: "v", Value: bson.D{{Key: "$ne", Value: "abc"}}}}},
		{"NinSymbol", bson.D{{Key: "v", Value: bson.D{{Key: "$nin", Value: bson.A{symAbc}}}}}},
		{"DottedSymbolByString", bson.D{{Key: "v.s", Value: "x"}}},
		{"DottedJavaScript", bson.D{{Key: "v.c", Value: primitive.JavaScript("y")}}},
		{"ArrayElementString", bson.D{{Key: "v", Value: "x"}}},
		{"ArrayElementJavaScript", bson.D{{Key: "v", Value: primitive.JavaScript("x")}}},
		{"ElemMatchTypeSymbol", bson.D{{Key: "v", Value: bson.D{{Key: "$elemMatch", Value: bson.D{{Key: "$type", Value: "symbol"}}}}}}},
		{"ExprEqString", bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$v", "abc"}}}}}},
		{"ExprEqSymbol", bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$v", symAbc}}}}}},
	}
	idsOnly := func() *options.FindOptions {
		return options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}})
	}
	var cases []symbolJavaScriptCase
	for _, f := range filters {
		cases = append(cases,
			symbolJavaScriptCase{"Scan_" + f.name, insertSymbolJavaScriptMixed, findDocs(f.filter, idsOnly())},
			symbolJavaScriptCase{"Indexed_" + f.name, withValueIndex(insertSymbolJavaScriptMixed),
				findDocs(f.filter, idsOnly().SetHint(bson.D{{Key: "v", Value: 1}}))},
		)
	}
	runSymbolJavaScriptCases(t, cases)
}

func TestSymbolJavaScript_Ordering(t *testing.T) {
	sortBy := func(dir int32) *options.FindOptions {
		return options.Find().SetSort(bson.D{{Key: "v", Value: dir}, {Key: "_id", Value: 1}})
	}
	cmp := func(a, b interface{}) bson.D { return bson.D{{Key: "$cmp", Value: bson.A{a, b}}} }
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"ScanSortAsc", insertSymbolJavaScriptMixed, findDocs(bson.D{}, sortBy(1))},
		{"ScanSortDesc", insertSymbolJavaScriptMixed, findDocs(bson.D{}, sortBy(-1))},
		{"IndexedSortAsc", withValueIndex(insertSymbolJavaScriptMixed), findDocs(bson.D{}, sortBy(1).SetHint(bson.D{{Key: "v", Value: 1}}))},
		{"IndexedSortDesc", withValueIndex(insertSymbolJavaScriptMixed), findDocs(bson.D{}, sortBy(-1).SetHint(bson.D{{Key: "v", Value: 1}}))},
		{"AggregateSortLimit", insertSymbolJavaScriptMixed, aggregateDocs(bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}}},
			bson.D{{Key: "$limit", Value: int32(6)}},
		})},
		{"GroupMinMax", insertSymbolJavaScriptMixed, aggregateDocs(bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(2), int32(3), int32(4), int32(5), int32(7)}}}}}}},
			bson.D{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: nil},
				{Key: "mn", Value: bson.D{{Key: "$min", Value: "$v"}}},
				{Key: "mx", Value: bson.D{{Key: "$max", Value: "$v"}}},
			}}},
		})},
		{"CmpExpressions", insertDocs(symJSDoc(1, nil)), aggregateDocs(bson.A{
			bson.D{{Key: "$project", Value: bson.D{
				{Key: "_id", Value: 0},
				{Key: "stringSymbol", Value: cmp("abc", symAbc)},
				{Key: "symbolString", Value: cmp(symAbc, "abd")},
				{Key: "javaScriptString", Value: cmp(primitive.JavaScript("a"), "a")},
				{Key: "javaScriptRegex", Value: cmp(primitive.JavaScript("a"), primitive.Regex{Pattern: "a"})},
				{Key: "javaScriptMaxKey", Value: cmp(primitive.JavaScript("a"), primitive.MaxKey{})},
				{Key: "javaScriptJavaScript", Value: cmp(primitive.JavaScript("a"), primitive.JavaScript("b"))},
			}}},
		})},
	})
}

func TestSymbolJavaScript_Indexes(t *testing.T) {
	uniqueValueIndex := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys:    bson.D{{Key: "v", Value: 1}},
			Options: options.Index().SetUnique(true),
		})
		return err
	}
	insertAfter := func(first, second interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if _, err := col.InsertOne(ctx, first); err != nil {
				return nil, err
			}
			if _, err := col.InsertOne(ctx, second); err != nil {
				return nil, err
			}
			return findDocs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		}
	}
	distinct := func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		return col.Distinct(ctx, "v", bson.D{})
	}
	hinted := options.Find().SetHint(bson.D{{Key: "v", Value: 1}})
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"CoveredSymbol", withValueIndex(insertSymbolJavaScriptMixed), findDocs(bson.D{{Key: "v", Value: symAbc}},
			options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "v", Value: 1}}).SetHint(bson.D{{Key: "v", Value: 1}}))},
		{"CoveredStringMatchesSymbol", withValueIndex(insertSymbolJavaScriptStrings), findDocs(bson.D{{Key: "v", Value: "abc"}},
			options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "v", Value: 1}}).SetHint(bson.D{{Key: "v", Value: 1}}))},
		{"CoveredJavaScript", withValueIndex(insertSymbolJavaScriptMixed), findDocs(bson.D{{Key: "v", Value: jsAbc}},
			options.Find().SetProjection(bson.D{{Key: "_id", Value: 0}, {Key: "v", Value: 1}}).SetHint(bson.D{{Key: "v", Value: 1}}))},
		{"UniqueSymbolAfterString", uniqueValueIndex, insertAfter(symJSDoc(1, "abc"), symJSDoc(2, symAbc))},
		{"UniqueStringAfterSymbol", uniqueValueIndex, insertAfter(symJSDoc(1, symAbc), symJSDoc(2, "abc"))},
		{"UniqueJavaScriptAfterString", uniqueValueIndex, insertAfter(symJSDoc(1, "abc"), symJSDoc(2, jsAbc))},
		{"IDSymbol", insertDocs(bson.D{{Key: "_id", Value: symAbc}, {Key: "n", Value: int32(1)}}),
			findDocs(bson.D{{Key: "_id", Value: symAbc}}, nil)},
		{"IDSymbolFoundByString", insertDocs(bson.D{{Key: "_id", Value: symAbc}, {Key: "n", Value: int32(1)}}),
			findDocs(bson.D{{Key: "_id", Value: "abc"}}, nil)},
		{"IDStringAfterSymbol", nil, insertAfter(bson.D{{Key: "_id", Value: symAbc}}, bson.D{{Key: "_id", Value: "abc"}})},
		{"IDJavaScript", insertDocs(bson.D{{Key: "_id", Value: jsAbc}, {Key: "n", Value: int32(1)}}),
			findDocs(bson.D{{Key: "_id", Value: jsAbc}}, nil)},
		{"DistinctScan", insertSymbolJavaScriptStrings, distinct},
		{"DistinctIndexed", withValueIndex(insertSymbolJavaScriptStrings), distinct},
		{"MultikeySymbol", withValueIndex(insertSymbolJavaScriptMixed), findDocs(bson.D{{Key: "v", Value: primitive.Symbol("x")}},
			hinted.SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}))},
	})
}

func TestSymbolJavaScript_Expressions(t *testing.T) {
	insertValues := insertDocs(bson.D{{Key: "_id", Value: int32(1)}, {Key: "s", Value: symAbc}, {Key: "j", Value: jsAbc}})
	inputs := []struct {
		name  string
		value interface{}
	}{
		{"SymbolField", "$s"},
		{"JavaScriptField", "$j"},
		{"SymbolLiteral", bson.D{{Key: "$literal", Value: symAbc}}},
		{"JavaScriptLiteral", bson.D{{Key: "$literal", Value: jsAbc}}},
	}
	operators := []struct {
		name string
		expr func(interface{}) interface{}
	}{
		{"Type", func(x interface{}) interface{} { return bson.D{{Key: "$type", Value: x}} }},
		{"ToString", func(x interface{}) interface{} { return bson.D{{Key: "$toString", Value: x}} }},
		{"ToBool", func(x interface{}) interface{} { return bson.D{{Key: "$toBool", Value: x}} }},
		{"IsNumber", func(x interface{}) interface{} { return bson.D{{Key: "$isNumber", Value: x}} }},
		{"ConvertStringOnError", convertTo("string", true)},
		{"ConvertString", convertTo("string", false)},
		{"ConvertBool", convertTo("bool", false)},
		{"ConvertIntOnError", convertTo("int", true)},
		{"ConvertInt", convertTo("int", false)},
		{"ConvertDoubleOnError", convertTo("double", true)},
		{"ConvertObjectIDOnError", convertTo("objectId", true)},
		{"StrLenBytes", func(x interface{}) interface{} { return bson.D{{Key: "$strLenBytes", Value: x}} }},
		{"StrLenCP", func(x interface{}) interface{} { return bson.D{{Key: "$strLenCP", Value: x}} }},
		{"Concat", func(x interface{}) interface{} { return bson.D{{Key: "$concat", Value: bson.A{x, "z"}}} }},
		{"ToUpper", func(x interface{}) interface{} { return bson.D{{Key: "$toUpper", Value: x}} }},
		{"SubstrBytes", func(x interface{}) interface{} { return bson.D{{Key: "$substrBytes", Value: bson.A{x, 0, 1}}} }},
		{"ToLower", func(x interface{}) interface{} { return bson.D{{Key: "$toLower", Value: x}} }},
		{"SubstrCP", func(x interface{}) interface{} { return bson.D{{Key: "$substrCP", Value: bson.A{x, 1, 2}}} }},
		{"Substr", func(x interface{}) interface{} { return bson.D{{Key: "$substr", Value: bson.A{x, 0, 2}}} }},
		{"Strcasecmp", func(x interface{}) interface{} { return bson.D{{Key: "$strcasecmp", Value: bson.A{x, "ABC"}}} }},
		{"StrcasecmpSecond", func(x interface{}) interface{} { return bson.D{{Key: "$strcasecmp", Value: bson.A{"abd", x}}} }},
		{"ReplaceAll", func(x interface{}) interface{} {
			return bson.D{{Key: "$replaceAll", Value: bson.D{{Key: "input", Value: x}, {Key: "find", Value: "a"}, {Key: "replacement", Value: "b"}}}}
		}},
		{"RegexFind", func(x interface{}) interface{} {
			return bson.D{{Key: "$regexFind", Value: bson.D{{Key: "input", Value: x}, {Key: "regex", Value: "b"}}}}
		}},
		{"Split", func(x interface{}) interface{} { return bson.D{{Key: "$split", Value: bson.A{x, "b"}}} }},
		{"Trim", func(x interface{}) interface{} {
			return bson.D{{Key: "$trim", Value: bson.D{{Key: "input", Value: x}}}}
		}},
		{"IndexOfBytes", func(x interface{}) interface{} { return bson.D{{Key: "$indexOfBytes", Value: bson.A{x, "b"}}} }},
		{"RegexMatch", func(x interface{}) interface{} {
			return bson.D{{Key: "$regexMatch", Value: bson.D{{Key: "input", Value: x}, {Key: "regex", Value: "b"}}}}
		}},
		{"BinarySize", func(x interface{}) interface{} { return bson.D{{Key: "$binarySize", Value: x}} }},
		{"EqString", func(x interface{}) interface{} { return bson.D{{Key: "$eq", Value: bson.A{x, "abc"}}} }},
		{"InArray", func(x interface{}) interface{} { return bson.D{{Key: "$in", Value: bson.A{x, bson.A{"abc"}}}} }},
		{"IfNull", func(x interface{}) interface{} { return bson.D{{Key: "$ifNull", Value: bson.A{"$missing", x}}} }},
	}
	var cases []symbolJavaScriptCase
	for _, op := range operators {
		for _, in := range inputs {
			cases = append(cases, symbolJavaScriptCase{op.name + "_" + in.name, insertValues, aggregateDocs(bson.A{
				bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: op.expr(in.value)}}}},
			})})
		}
	}
	cases = append(cases,
		symbolJavaScriptCase{"GroupBySymbolAndString", insertSymbolJavaScriptStrings, aggregateSortedByID(bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$v"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
		})},
		symbolJavaScriptCase{"GroupBySymbolAndStringReversed", insertSymbolJavaScriptStrings, aggregateSortedByID(bson.A{
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: -1}}}},
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "k", Value: "$v"}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
		})},
		symbolJavaScriptCase{"AddToSetSize", insertSymbolJavaScriptStrings, aggregateDocs(bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "set", Value: bson.D{{Key: "$addToSet", Value: "$v"}}}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "n", Value: bson.D{{Key: "$size", Value: "$set"}}}}}},
		})},
		symbolJavaScriptCase{"GetField", insertValues, aggregateDocs(bson.A{
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 0}, {Key: "r", Value: bson.D{{Key: "$getField", Value: "s"}}}}}},
		})},
		symbolJavaScriptCase{"SetField", insertValues, aggregateDocs(bson.A{
			bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$setField", Value: bson.D{
				{Key: "field", Value: "q"}, {Key: "input", Value: "$$ROOT"}, {Key: "value", Value: bson.D{{Key: "$literal", Value: jsFunc}}},
			}}}}},
		})},
		symbolJavaScriptCase{"MatchSymbolLiteral", insertSymbolJavaScriptMixed, aggregateDocs(bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: symAbc}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		})},
		symbolJavaScriptCase{"AddFieldsLiterals", insertDocs(symJSDoc(1, nil)), aggregateDocs(bson.A{
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "s", Value: bson.D{{Key: "$literal", Value: symAbc}}}, {Key: "j", Value: bson.D{{Key: "$literal", Value: jsAbc}}}}}},
		})},
	)
	runSymbolJavaScriptCases(t, cases)
}

func convertTo(to string, onError bool) func(interface{}) interface{} {
	return func(x interface{}) interface{} {
		spec := bson.D{{Key: "input", Value: x}, {Key: "to", Value: to}}
		if onError {
			spec = append(spec, bson.E{Key: "onError", Value: "error"})
		}
		return bson.D{{Key: "$convert", Value: spec}}
	}
}

func TestSymbolJavaScript_Updates(t *testing.T) {
	byID := bson.D{{Key: "_id", Value: int32(1)}}
	set := func(field string, v interface{}) bson.D {
		return bson.D{{Key: "$set", Value: bson.D{{Key: field, Value: v}}}}
	}
	op := func(name, field string, v interface{}) bson.D {
		return bson.D{{Key: name, Value: bson.D{{Key: field, Value: v}}}}
	}
	arrayDoc := func(values ...interface{}) func(context.Context, *mongo.Collection) error {
		return insertDocs(bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A(values)}})
	}
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"SetSymbol", insertDocs(symJSDoc(1, "abc")), updateThenFind(byID, set("v", symAbd))},
		{"SetJavaScript", insertDocs(symJSDoc(1, "abc")), updateThenFind(byID, set("v", jsFunc))},
		{"SetNestedSymbol", insertDocs(symJSDoc(1, "abc")), updateThenFind(byID, set("d.s", symAbc))},
		{"IncSymbol", insertDocs(symJSDoc(1, symAbc)), updateThenFind(byID, op("$inc", "v", int32(1)))},
		{"RenameSymbol", insertDocs(symJSDoc(1, symAbc)), updateThenFind(byID, op("$rename", "v", "w"))},
		{"AddToSetStringIntoSymbols", arrayDoc(primitive.Symbol("x")), updateThenFind(byID, op("$addToSet", "a", "x"))},
		{"AddToSetJavaScriptIntoStrings", arrayDoc("x"), updateThenFind(byID, op("$addToSet", "a", primitive.JavaScript("x")))},
		{"PullString", arrayDoc(primitive.Symbol("x"), primitive.JavaScript("x"), "x", "y"), updateThenFind(byID, op("$pull", "a", "x"))},
		{"PullJavaScript", arrayDoc(primitive.Symbol("x"), primitive.JavaScript("x"), "x"), updateThenFind(byID, op("$pull", "a", primitive.JavaScript("x")))},
		{"MinStringBelowSymbol", insertDocs(symJSDoc(1, primitive.Symbol("b"))), updateThenFind(byID, op("$min", "v", "a"))},
		{"MinStringAboveSymbol", insertDocs(symJSDoc(1, primitive.Symbol("b"))), updateThenFind(byID, op("$min", "v", "c"))},
		{"MaxJavaScriptOverString", insertDocs(symJSDoc(1, "z")), updateThenFind(byID, op("$max", "v", primitive.JavaScript("a")))},
		{"PositionalBySymbol", arrayDoc(primitive.Symbol("x"), "y"),
			updateThenFind(bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: primitive.Symbol("x")}}, set("a.$", "z"))},
		{"UpsertSymbolEquality", nil,
			updateThenFind(bson.D{{Key: "_id", Value: int32(1)}, {Key: "v", Value: symAbc}}, set("n", int32(1)), options.Update().SetUpsert(true))},
		{"FindOneAndUpdateReturnsSymbol", insertDocs(symJSDoc(1, "abc")),
			func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				var doc bson.D
				err := col.FindOneAndUpdate(ctx, byID, set("v", symAbc), options.FindOneAndUpdate().SetReturnDocument(options.After)).Decode(&doc)
				return doc, err
			}},
		{"UpdatePipelineSymbol", insertDocs(symJSDoc(1, "abc")),
			updateThenFind(byID, bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "w", Value: bson.D{{Key: "$literal", Value: symAbc}}}}}}})},
	})
}

func TestSymbolJavaScript_IndexPlanner(t *testing.T) {
	stageAfter := func(prepare func(context.Context, *mongo.Collection) error, filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := prepare(ctx, col); err != nil {
				return nil, err
			}
			return winningStage(explainFind(ctx, col, filter, nil, "queryPlanner")), nil
		}
	}
	deleteSymbolsAndCode := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.DeleteMany(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{int32(2), int32(3), int32(5), int32(6), int32(10), int32(11)}}}}})
		return err
	}
	noop := func(context.Context, *mongo.Collection) error { return nil }
	hinted := func() *options.FindOptions {
		return options.Find().SetHint(bson.D{{Key: "v", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}})
	}
	countHinted := func(filter bson.D) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.CountDocuments(ctx, filter, options.Count().SetHint(bson.D{{Key: "v", Value: 1}}))
		}
	}
	nullsAndStrings := insertDocs(symJSDoc(1, "x"), symJSDoc(2, nil), bson.D{{Key: "_id", Value: int32(3)}},
		symJSDoc(6, "y"), symJSDoc(7, bson.A{nil, "z"}))
	compoundIndex := func(ctx context.Context, col *mongo.Collection) error {
		if err := insertSymbolJavaScriptMixed(ctx, col); err != nil {
			return err
		}
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}})
		return err
	}
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"PlanWithSymbolStored", withValueIndex(insertSymbolJavaScriptMixed), stageAfter(noop, bson.D{{Key: "v", Value: int32(5)}})},
		{"PlanAfterSymbolDeleted", withValueIndex(insertSymbolJavaScriptMixed), stageAfter(deleteSymbolsAndCode, bson.D{{Key: "v", Value: int32(5)}})},
		{"PlanSymbolEquality", withValueIndex(insertSymbolJavaScriptMixed), stageAfter(noop, bson.D{{Key: "v", Value: symAbc}})},
		{"PlanJavaScriptEquality", withValueIndex(insertSymbolJavaScriptMixed), stageAfter(noop, bson.D{{Key: "v", Value: jsAbc}})},
		{"IndexedSymbolNotNull", withValueIndex(nullsAndStrings), findDocs(bson.D{{Key: "v", Value: primitive.Symbol("x")}}, hinted())},
		{"IndexedJavaScriptNotNull", withValueIndex(nullsAndStrings), findDocs(bson.D{{Key: "v", Value: primitive.JavaScript("x")}}, hinted())},
		{"IndexedInSymbolAndJavaScript", withValueIndex(nullsAndStrings),
			findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$in", Value: bson.A{primitive.JavaScript("x"), primitive.Symbol("y")}}}}}, hinted())},
		{"IndexedGteSymbol", withValueIndex(nullsAndStrings),
			findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$gte", Value: primitive.Symbol("x")}}}}, hinted())},
		{"IndexedGtJavaScript", withValueIndex(insertSymbolJavaScriptMixed),
			findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: primitive.JavaScript("abc")}}}}, hinted())},
		{"IndexedCountJavaScript", withValueIndex(nullsAndStrings), countHinted(bson.D{{Key: "v", Value: primitive.JavaScript("x")}})},
		{"IndexedCountSymbol", withValueIndex(nullsAndStrings), countHinted(bson.D{{Key: "v", Value: primitive.Symbol("x")}})},
		{"IndexedUpdateManySymbol", withValueIndex(nullsAndStrings), func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			res, err := col.UpdateMany(ctx, bson.D{{Key: "v", Value: primitive.Symbol("x")}}, bson.D{{Key: "$set", Value: bson.D{{Key: "hit", Value: true}}}})
			if err != nil {
				return nil, err
			}
			return res.ModifiedCount, nil
		}},
		{"IndexedDistinctSymbolFilter", withValueIndex(nullsAndStrings), func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.Distinct(ctx, "_id", bson.D{{Key: "v", Value: primitive.Symbol("x")}})
		}},
		{"CompoundIndexSymbol", compoundIndex, findDocs(bson.D{{Key: "v", Value: symAbc}},
			options.Find().SetHint(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetProjection(bson.D{{Key: "_id", Value: 1}}))},
		{"CompoundIndexSortAsc", compoundIndex, findDocs(bson.D{},
			options.Find().SetHint(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}))},
	})
}

var symJSCollation = &options.Collation{Locale: "en", Strength: 2}

func insertSymbolJavaScriptCases(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		symJSDoc(1, "abc"),
		symJSDoc(2, symAbc),
		symJSDoc(3, jsAbc),
		symJSDoc(4, "ABC"),
		symJSDoc(5, primitive.Symbol("ABC")),
		symJSDoc(6, int32(5)),
	})
	return err
}

func TestSymbolJavaScript_Collation(t *testing.T) {
	idsOnly := func() *options.FindOptions {
		return options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}}).SetCollation(symJSCollation)
	}
	collatedAggregate := func(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, pipeline, options.Aggregate().SetCollation(symJSCollation))
			if err != nil {
				return nil, err
			}
			var docs []bson.D
			if err := cursor.All(ctx, &docs); err != nil {
				return nil, err
			}
			return docs, nil
		}
	}
	collatedIndex := func(unique bool) func(context.Context, *mongo.Collection) error {
		return func(ctx context.Context, col *mongo.Collection) error {
			_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
				Keys:    bson.D{{Key: "v", Value: 1}},
				Options: options.Index().SetUnique(unique).SetCollation(symJSCollation),
			})
			return err
		}
	}
	insertThenFind := func(docs ...interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			for _, d := range docs {
				if _, err := col.InsertOne(ctx, d); err != nil {
					return nil, err
				}
			}
			return findDocs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		}
	}
	groupPipeline := bson.A{
		bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: bson.A{int32(1), int32(5)}}}}}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$v"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
	}
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"CollatedFindEq", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: "abc"}}, idsOnly())},
		{"CollatedFindEqSymbol", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: symAbc}}, idsOnly())},
		{"CollatedFindIn", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$in", Value: bson.A{"abc"}}}}}, idsOnly())},
		{"CollatedFindGt", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$gt", Value: "abb"}}}}, idsOnly())},
		{"CollatedFindNe", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$ne", Value: "abc"}}}}, idsOnly())},
		{"CollatedExprEq", insertSymbolJavaScriptCases,
			findDocs(bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{"$v", "abc"}}}}}, idsOnly())},
		{"CollatedMatch", insertSymbolJavaScriptCases, collatedAggregate(bson.A{
			bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: "abc"}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: 1}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		})},
		{"CollatedGroup", insertSymbolJavaScriptCases, collatedAggregate(groupPipeline)},
		{"CollatedDistinct", insertSymbolJavaScriptCases, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return col.Distinct(ctx, "v", bson.D{{Key: "_id", Value: bson.D{{Key: "$lt", Value: int32(6)}}}}, options.Distinct().SetCollation(symJSCollation))
		}},
		{"CollatedSort", insertSymbolJavaScriptCases,
			findDocs(bson.D{}, options.Find().SetSort(bson.D{{Key: "v", Value: 1}, {Key: "_id", Value: 1}}).SetCollation(&options.Collation{Locale: "en", Strength: 1}))},
		{"CollatedIndexInsertSymbol", collatedIndex(false), insertThenFind(symJSDoc(1, "abc"), symJSDoc(2, primitive.Symbol("ABC")))},
		{"CollatedUniqueIndexInsertSymbol", collatedIndex(true), insertThenFind(symJSDoc(1, "abc"), symJSDoc(2, primitive.Symbol("ABC")))},
		{"CollatedIndexInsertJavaScript", collatedIndex(false), insertThenFind(symJSDoc(1, "abc"), symJSDoc(2, jsAbc))},
		{"CollatedIndexBuildOverSymbol", insertSymbolJavaScriptCases, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := collatedIndex(false)(ctx, col); err != nil {
				return nil, err
			}
			return findDocs(bson.D{}, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))(ctx, col)
		}},
	})
}

func TestSymbolJavaScript_JSONSchema(t *testing.T) {
	idsOnly := func() *options.FindOptions {
		return options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}})
	}
	schema := func(bsonType interface{}) bson.D {
		return bson.D{{Key: "$jsonSchema", Value: bson.D{
			{Key: "required", Value: bson.A{"v"}},
			{Key: "properties", Value: bson.D{{Key: "v", Value: bson.D{{Key: "bsonType", Value: bsonType}}}}},
		}}}
	}
	validated := func(bsonType string, doc interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
		return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			if err := col.Database().RunCommand(ctx, bson.D{{Key: "collMod", Value: col.Name()}, {Key: "validator", Value: schema(bsonType)}}).Err(); err != nil {
				return nil, err
			}
			if _, err := col.InsertOne(ctx, doc); err != nil {
				return nil, err
			}
			return findDocs(bson.D{}, nil)(ctx, col)
		}
	}
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"SchemaSymbol", insertSymbolJavaScriptCases, findDocs(schema("symbol"), idsOnly())},
		{"SchemaJavaScript", insertSymbolJavaScriptCases, findDocs(schema("javascript"), idsOnly())},
		{"SchemaString", insertSymbolJavaScriptCases, findDocs(schema("string"), idsOnly())},
		{"SchemaJavaScriptOrString", insertSymbolJavaScriptCases, findDocs(schema(bson.A{"javascript", "string"}), idsOnly())},
		{"ValidatorAcceptsSymbol", insertDocs(symJSDoc(1, "abc")), validated("symbol", symJSDoc(2, symAbc))},
		{"ValidatorRejectsStringForSymbol", insertDocs(symJSDoc(1, symAbc)), validated("symbol", symJSDoc(2, "abc"))},
		{"ValidatorAcceptsJavaScript", insertDocs(symJSDoc(1, jsAbc)), validated("javascript", symJSDoc(2, jsFunc))},
	})
}

func TestSymbolJavaScript_RegexAndGroups(t *testing.T) {
	idsOnly := func() *options.FindOptions {
		return options.Find().SetProjection(bson.D{{Key: "_id", Value: 1}}).SetSort(bson.D{{Key: "_id", Value: 1}})
	}
	re := primitive.Regex{Pattern: "^ab"}
	group := aggregateSortedByID(bson.A{
		bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
		bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$v"}, {Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}}}}},
	})
	runSymbolJavaScriptCases(t, []symbolJavaScriptCase{
		{"RegexLiteral", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: re}}, idsOnly())},
		{"RegexInList", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$in", Value: bson.A{re}}}}}, idsOnly())},
		{"RegexNot", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$not", Value: re}}}}, idsOnly())},
		{"RegexNin", insertSymbolJavaScriptCases, findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$nin", Value: bson.A{re}}}}}, idsOnly())},
		{"RegexCaseInsensitive", insertSymbolJavaScriptCases,
			findDocs(bson.D{{Key: "v", Value: bson.D{{Key: "$regex", Value: "^abc$"}, {Key: "$options", Value: "i"}}}}, idsOnly())},
		{"RegexIndexed", withValueIndex(insertSymbolJavaScriptCases),
			findDocs(bson.D{{Key: "v", Value: re}}, idsOnly().SetHint(bson.D{{Key: "v", Value: 1}}))},
		{"GroupDistinctSymbols", insertDocs(symJSDoc(1, primitive.Symbol("a")), symJSDoc(2, primitive.Symbol("b")),
			symJSDoc(3, primitive.JavaScript("x")), symJSDoc(4, primitive.JavaScript("y"))), group},
		{"GroupStringBeforeSymbol", insertDocs(symJSDoc(1, "abc"), symJSDoc(2, symAbc)), group},
		{"GroupJavaScriptAndString", insertDocs(symJSDoc(1, "abc"), symJSDoc(2, jsAbc), symJSDoc(3, jsAbc)), group},
		{"DistinctSymbolsAndCode", insertDocs(symJSDoc(1, primitive.Symbol("a")), symJSDoc(2, primitive.Symbol("b")),
			symJSDoc(3, primitive.JavaScript("x")), symJSDoc(4, primitive.JavaScript("x")), symJSDoc(5, "a")),
			func(ctx context.Context, col *mongo.Collection) (interface{}, error) { return col.Distinct(ctx, "v", bson.D{}) }},
		{"GraphLookupSymbolToJavaScript", insertDocs(
			bson.D{{Key: "_id", Value: int32(1)}, {Key: "from", Value: symAbc}},
			bson.D{{Key: "_id", Value: int32(2)}, {Key: "to", Value: jsAbc}},
			bson.D{{Key: "_id", Value: int32(3)}, {Key: "to", Value: "abc"}}),
			func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return aggregateDocs(bson.A{
					bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}},
					bson.D{{Key: "$graphLookup", Value: bson.D{
						{Key: "from", Value: col.Name()}, {Key: "startWith", Value: "$from"},
						{Key: "connectFromField", Value: "from"}, {Key: "connectToField", Value: "to"}, {Key: "as", Value: "hits"},
					}}},
					bson.D{{Key: "$project", Value: bson.D{{Key: "ids", Value: "$hits._id"}}}},
				})(ctx, col)
			}},
	})
}
