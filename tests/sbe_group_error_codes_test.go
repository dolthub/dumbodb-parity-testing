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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod 8.0 runs $group (and $bucket and $sortByCount, which are built on it)
// in its SBE engine, with the leading stages its find layer can absorb. A
// math expression error raised there has SBE's code and message; the same
// expression elsewhere keeps the classic engine's. dolthub/dumbodb#125.

func sbeErrorsSeed(ctx context.Context, col *mongo.Collection) error {
	d0, err := primitive.ParseDecimal128("0")
	if err != nil {
		return err
	}
	_, err = col.InsertMany(ctx, []interface{}{
		bson.D{
			{Key: "_id", Value: int32(1)}, {Key: "v", Value: int32(5)}, {Key: "z", Value: int32(0)}, {Key: "n", Value: int32(-1)},
			{Key: "s", Value: "x"}, {Key: "f", Value: 1.5}, {Key: "d0", Value: d0}, {Key: "lmin", Value: int64(math.MinInt64)},
			{Key: "inf", Value: math.Inf(1)}, {Key: "a", Value: bson.A{int32(1), int32(2)}},
		},
	})
	return err
}

func sbeOp(name string, arg interface{}) bson.D { return bson.D{{Key: name, Value: arg}} }

var sbeErrorExpressions = []struct {
	name string
	expr bson.D
}{
	{"DivideByZero", sbeOp("$divide", bson.A{"$v", "$z"})},
	{"DivideString", sbeOp("$divide", bson.A{"$v", "$s"})},
	{"ModByZero", sbeOp("$mod", bson.A{"$v", "$z"})},
	{"ModByZeroDecimal", sbeOp("$mod", bson.A{"$v", "$d0"})},
	{"ModString", sbeOp("$mod", bson.A{"$s", "$v"})},
	{"AbsString", sbeOp("$abs", "$s")},
	{"AbsLongMin", sbeOp("$abs", "$lmin")},
	{"CeilString", sbeOp("$ceil", "$s")},
	{"FloorString", sbeOp("$floor", "$s")},
	{"ExpString", sbeOp("$exp", "$s")},
	{"LnString", sbeOp("$ln", "$s")},
	{"LnZero", sbeOp("$ln", "$z")},
	{"Log10String", sbeOp("$log10", "$s")},
	{"Log10Negative", sbeOp("$log10", "$n")},
	{"SqrtString", sbeOp("$sqrt", "$s")},
	{"SqrtNegative", sbeOp("$sqrt", "$n")},
	{"PowZeroNegative", sbeOp("$pow", bson.A{"$z", "$n"})},
	{"PowStringBase", sbeOp("$pow", bson.A{"$s", "$v"})},
	{"PowStringExponent", sbeOp("$pow", bson.A{"$v", "$s"})},
	{"TruncString", sbeOp("$trunc", "$s")},
	{"RoundString", sbeOp("$round", "$s")},
	{"RoundPlaceOutOfRange", sbeOp("$round", bson.A{"$v", 200})},
	{"RoundPlaceFraction", sbeOp("$round", bson.A{"$v", "$f"})},
	{"TruncPlaceString", sbeOp("$trunc", bson.A{"$v", "$s"})},
	{"SubtractString", sbeOp("$subtract", bson.A{"$v", "$s"})},
	{"AddString", sbeOp("$add", bson.A{"$v", "$s"})},
	{"MultiplyString", sbeOp("$multiply", bson.A{"$v", "$s"})},
	{"LogString", sbeOp("$log", bson.A{"$s", 10})},
	{"SinString", sbeOp("$sin", "$s")},
	{"CosInfinity", sbeOp("$cos", "$inf")},
	{"AcoshZero", sbeOp("$acosh", "$z")},
	{"AsinFive", sbeOp("$asin", "$v")},
	{"AtanhString", sbeOp("$atanh", "$s")},
	{"SinhString", sbeOp("$sinh", "$s")},
	{"RadiansToDegreesString", sbeOp("$radiansToDegrees", "$s")},
	{"Atan2String", sbeOp("$atan2", bson.A{"$v", "$s"})},
}

func TestSBEGroupErrorCodes(t *testing.T) {
	contexts := []struct {
		name  string
		build func(expr bson.D) bson.A
	}{
		{"GroupAccumulator", func(e bson.D) bson.A {
			return bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: e}}}}}}}
		}},
		{"GroupKey", func(e bson.D) bson.A {
			return bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: e}}}}}
		}},
		{"BucketOutput", func(e bson.D) bson.A {
			return bson.A{bson.D{{Key: "$bucket", Value: bson.D{{Key: "groupBy", Value: "$v"}, {Key: "boundaries", Value: bson.A{0, 10}},
				{Key: "output", Value: bson.D{{Key: "r", Value: bson.D{{Key: "$push", Value: e}}}}}}}}}
		}},
		{"SortByCount", func(e bson.D) bson.A {
			return bson.A{bson.D{{Key: "$sortByCount", Value: e}}}
		}},
		{"Project", func(e bson.D) bson.A {
			return bson.A{bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: e}}}}}
		}},
		{"AddFieldsBeforeGroup", func(e bson.D) bson.A {
			return bson.A{bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: e}}}}, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$r"}}}}}
		}},
	}
	for _, c := range contexts {
		for _, e := range sbeErrorExpressions {
			pipeline := c.build(e.expr)
			harness.PairTest(t, harness.TestCase{
				Name:    "SBEErr_" + c.name + "_" + e.name,
				Support: harness.DumboDBFull,
				Setup:   sbeErrorsSeed,
				Run:     pipelineRun(pipeline),
			})
		}
	}
}

// Which stages run in SBE depends on what mongod's find layer absorbs ahead
// of the $group.
func TestSBEGroupErrorCodes_PipelineShapes(t *testing.T) {
	div := sbeOp("$divide", bson.A{"$v", "$z"})
	pushDiv := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: div}}}}}}
	groupByR := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$r"}}}}
	firsts := bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$v"}, {Key: "z", Value: bson.D{{Key: "$first", Value: "$z"}}}, {Key: "v", Value: bson.D{{Key: "$first", Value: "$v"}}}}}}
	match := bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: int32(5)}}}}
	sortV := bson.D{{Key: "$sort", Value: bson.D{{Key: "v", Value: 1}}}}
	limit := bson.D{{Key: "$limit", Value: int32(4)}}
	skip := bson.D{{Key: "$skip", Value: int32(0)}}
	include := bson.D{{Key: "$project", Value: bson.D{{Key: "v", Value: 1}, {Key: "z", Value: 1}}}}
	exclude := bson.D{{Key: "$project", Value: bson.D{{Key: "a", Value: 0}}}}
	computeR := bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: div}, {Key: "v", Value: 1}}}}
	exprMatch := bson.D{{Key: "$match", Value: bson.D{{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{div, 1}}}}}}}
	lookupEq := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "localField", Value: "v"}, {Key: "foreignField", Value: "v"}, {Key: "as", Value: "j"}}}}
	}
	lookupPipe := func(col *mongo.Collection) bson.D {
		return bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{}}, {Key: "as", Value: "j"}}}}
	}
	cases := []struct {
		name  string
		build func(col *mongo.Collection) bson.A
	}{
		{"Match", func(*mongo.Collection) bson.A { return bson.A{match, pushDiv} }},
		{"MatchSortSkipLimit", func(*mongo.Collection) bson.A { return bson.A{match, sortV, skip, limit, pushDiv} }},
		{"LimitThenMatch", func(*mongo.Collection) bson.A { return bson.A{limit, match, pushDiv} }},
		{"LimitThenSort", func(*mongo.Collection) bson.A { return bson.A{limit, sortV, pushDiv} }},
		{"Inclusion", func(*mongo.Collection) bson.A { return bson.A{include, pushDiv} }},
		{"InclusionTwice", func(*mongo.Collection) bson.A { return bson.A{include, include, pushDiv} }},
		{"InclusionThenSort", func(*mongo.Collection) bson.A { return bson.A{include, sortV, pushDiv} }},
		{"InclusionThenMatch", func(*mongo.Collection) bson.A { return bson.A{include, match, pushDiv} }},
		{"SortThenInclusion", func(*mongo.Collection) bson.A { return bson.A{sortV, include, pushDiv} }},
		{"Exclusion", func(*mongo.Collection) bson.A { return bson.A{exclude, pushDiv} }},
		{"Unset", func(*mongo.Collection) bson.A { return bson.A{bson.D{{Key: "$unset", Value: "a"}}, pushDiv} }},
		{"Unwind", func(*mongo.Collection) bson.A { return bson.A{bson.D{{Key: "$unwind", Value: "$a"}}, pushDiv} }},
		{"ComputedProject", func(*mongo.Collection) bson.A { return bson.A{computeR, groupByR} }},
		{"ComputedProjectThenSkip", func(*mongo.Collection) bson.A { return bson.A{computeR, skip, groupByR} }},
		{"SortThenComputedProject", func(*mongo.Collection) bson.A { return bson.A{sortV, computeR, groupByR} }},
		{"ExprMatch", func(*mongo.Collection) bson.A {
			return bson.A{exprMatch, bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}}}}}
		}},
		{"ExprMatchAlone", func(*mongo.Collection) bson.A { return bson.A{exprMatch} }},
		{"GroupThenGroup", func(*mongo.Collection) bson.A { return bson.A{firsts, pushDiv} }},
		{"GroupSortGroup", func(*mongo.Collection) bson.A { return bson.A{firsts, sortV, pushDiv} }},
		{"GroupMatchGroup", func(*mongo.Collection) bson.A { return bson.A{firsts, match, pushDiv} }},
		{"GroupThenProject", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$v"}, {Key: "x", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: sbeOp("$divide", bson.A{"$x", 0})}}}}}
		}},
		{"LookupThenGroup", func(col *mongo.Collection) bson.A { return bson.A{lookupEq(col), pushDiv} }},
		{"LookupMatchGroup", func(col *mongo.Collection) bson.A { return bson.A{lookupEq(col), match, pushDiv} }},
		{"GroupLookupGroup", func(col *mongo.Collection) bson.A { return bson.A{firsts, lookupEq(col), pushDiv} }},
		{"LookupPipelineThenGroup", func(col *mongo.Collection) bson.A { return bson.A{lookupPipe(col), pushDiv} }},
		{"InLookupSubPipeline", func(col *mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$lookup", Value: bson.D{{Key: "from", Value: col.Name()}, {Key: "pipeline", Value: bson.A{pushDiv}}, {Key: "as", Value: "j"}}}}}
		}},
		{"InFacet", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$facet", Value: bson.D{{Key: "f", Value: bson.A{pushDiv}}}}}}
		}},
		{"InUnionWith", func(col *mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{pushDiv}}}}}}
		}},
		{"InUnionWithAfterAddFields", func(col *mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$unionWith", Value: bson.D{{Key: "coll", Value: col.Name()}, {Key: "pipeline", Value: bson.A{
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "q", Value: 1}}}}, pushDiv,
			}}}}}}
		}},
		{"ConstantFolded", func(*mongo.Collection) bson.A {
			return bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: sbeOp("$divide", bson.A{1, 0})}}}}}}}
		}},
	}
	for _, c := range cases {
		build := c.build
		harness.PairTest(t, harness.TestCase{
			Name:    "SBEShape_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   sbeErrorsSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return pipelineRun(build(col))(ctx, col)
			},
		})
	}
}

func TestSBEGroupErrorCodes_View(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SBEView_GroupOverView",
		Support: harness.DumboDBFull,
		Setup:   sbeErrorsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			db := col.Database()
			for _, v := range []struct {
				name     string
				pipeline bson.A
			}{
				{"vmatch", bson.A{bson.D{{Key: "$match", Value: bson.D{{Key: "v", Value: int32(5)}}}}}},
				{"vset", bson.A{bson.D{{Key: "$set", Value: bson.D{{Key: "q", Value: 1}}}}}},
			} {
				if err := db.CreateView(ctx, v.name, col.Name(), v.pipeline); err != nil {
					return nil, err
				}
			}
			pushDiv := bson.A{bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "r", Value: bson.D{{Key: "$push", Value: sbeOp("$divide", bson.A{"$v", "$z"})}}}}}}}
			var out bson.A
			for _, name := range []string{"vmatch", "vset"} {
				_, err := pipelineRun(pushDiv)(ctx, db.Collection(name))
				out = append(out, errOutcome(err))
			}
			return out, nil
		},
	})
}
