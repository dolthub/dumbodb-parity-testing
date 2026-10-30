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

// Expressions whose result is the missing value: a projected field is
// omitted and $type reports "missing". (review of #184: workspace-m8c.5)

func insertMissingResultDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.A{}}, {Key: "n", Value: nil}},
	})
	return err
}

// missingResultsPending diverged before workspace-m8c.5 was fixed.
var missingResultsPending = map[string]bool{
	"LetMissingVar_Project":                true,
	"LetMissingVar_Type":                   true,
	"LetMissingVar_InDocument":             true,
	"LetNestedMissing_Project":             true,
	"LetNestedMissing_Type":                true,
	"LetNestedMissing_InDocument":          true,
	"FirstOfEmpty_Project":                 true,
	"FirstOfEmpty_Type":                    true,
	"FirstOfEmpty_InDocument":              true,
	"LastOfEmpty_Project":                  true,
	"LastOfEmpty_Type":                     true,
	"LastOfEmpty_InDocument":               true,
	"FirstOfEmptyField_Project":            true,
	"FirstOfEmptyField_Type":               true,
	"FirstOfEmptyField_InDocument":         true,
	"ReduceEmptyMissingInitial_Project":    true,
	"ReduceEmptyMissingInitial_Type":       true,
	"ReduceEmptyMissingInitial_InDocument": true,
	"ReduceToMissing_Project":              true,
	"ReduceToMissing_Type":                 true,
	"ReduceToMissing_InDocument":           true,
	"AddFieldsLetMissing":                  true,
	"GroupKeyLetMissing":                   true,
}

func missingResultsSupport(name string) harness.DumboDBSupport {
	if missingResultsPending[name] {
		return harness.DumboDBXFail
	}
	return harness.DumboDBFull
}

func TestMissingResults(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		out := bson.D{}
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	let := func(v, in interface{}) bson.D { return d("$let", d("vars", d("v", v), "in", in)) }
	reduce := func(input, initial interface{}) bson.D {
		return d("$reduce", d("input", input, "initialValue", initial, "in", d("$add", bson.A{"$$value", "$$this"})))
	}

	exprs := []struct {
		name string
		expr interface{}
	}{
		{"LetMissingVar", let("$nope", "$$v")},
		{"LetRemoveVar", let("$$REMOVE", "$$v")},
		{"LetNullVar", let("$n", "$$v")},
		{"LetMissingIn", let(int32(1), "$nope")},
		{"LetNestedMissing", let("$nope", let("$$v", "$$v"))},
		{"LetVarPathMissing", let(d("$literal", d("a", int32(1))), "$$v.b")},
		{"FirstOfEmpty", d("$first", bson.A{bson.A{}})},
		{"LastOfEmpty", d("$last", bson.A{bson.A{}})},
		{"FirstOfEmptyField", d("$first", "$a")},
		{"FirstOfMissing", d("$first", "$nope")},
		{"FirstOfNullElement", d("$first", bson.A{bson.A{nil}})},
		{"ReduceEmptyMissingInitial", reduce(bson.A{}, "$nope")},
		{"ReduceEmptyNullInitial", reduce(bson.A{}, nil)},
		{"ReduceEmptyRemoveInitial", reduce(bson.A{}, "$$REMOVE")},
		{"ReduceToMissing", d("$reduce", d("input", bson.A{int32(1)}, "initialValue", int32(0), "in", "$nope"))},
		{"CondMissing", d("$cond", bson.A{true, "$nope", int32(1)})},
		{"IfNullMissing", d("$ifNull", bson.A{nil, "$nope"})},
	}

	for _, e := range exprs {
		for _, form := range []struct {
			name string
			expr interface{}
		}{
			{"Project", e.expr},
			{"Type", d("$type", e.expr)},
			{"InDocument", d("x", e.expr, "y", int32(1))},
			{"InArray", bson.A{e.expr}},
		} {
			pipeline := bson.A{d("$project", d("_id", int32(0), "r", form.expr))}
			t.Run(e.name+"_"+form.name, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "MissingResult_" + e.name + "_" + form.name,
					Support: missingResultsSupport(e.name + "_" + form.name),
					Setup:   insertMissingResultDocs,
					Run:     pipelineRunExtJSON(pipeline),
				})
			})
		}
	}

	t.Run("AddFieldsLetMissing", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "MissingResult_AddFieldsLetMissing",
			Support: missingResultsSupport("AddFieldsLetMissing"),
			Setup:   insertMissingResultDocs,
			Run:     pipelineRunExtJSON(bson.A{d("$addFields", d("l", let("$nope", "$$v"))), d("$project", d("_id", int32(0), "l", int32(1)))}),
		})
	})
	t.Run("GroupKeyLetMissing", func(t *testing.T) {
		harness.PairTest(t, harness.TestCase{
			Name:    "MissingResult_GroupKeyLetMissing",
			Support: missingResultsSupport("GroupKeyLetMissing"),
			Setup:   insertMissingResultDocs,
			Run:     pipelineRunExtJSON(bson.A{d("$group", d("_id", d("k", let("$nope", "$$v"))))}),
		})
	})
}
