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

// A document literal omits a field whose expression evaluates to missing and
// keeps one that evaluates to null, in every context that builds documents.

func insertDocLiteralDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(0)}, {Key: "i", Value: int32(3)}, {Key: "n", Value: nil}, {Key: "g", Value: "a"}, {Key: "o", Value: bson.D{{Key: "p", Value: int32(1)}}}},
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "i", Value: int32(4)}, {Key: "g", Value: "a"}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "g", Value: "b"}},
		bson.D{{Key: "_id", Value: int32(3)}},
	})
	return err
}

// docLiteralMissingAlreadyMatching are the cases DumboDB matched before
// document literals omitted missing fields (workspace-3xn).
var docLiteralMissingAlreadyMatching = map[string]bool{
	"GroupID_IfNullMissingLast":        true,
	"GroupID_SwitchMissingDefault":     true,
	"AddFields_ArrayWithMissing":       true,
	"AddFields_Literal":                true,
	"AddFields_MissingField":           true,
	"AddFields_MissingSubpath":         true,
	"AddFields_Nested":                 true,
	"AddFields_NestedAllMissing":       true,
	"AddFields_NullField":              true,
	"AddFields_NullLiteral":            true,
	"AddFields_OnlyMissing":            true,
	"AddFields_OperatorOfMissing":      true,
	"AddFields_Remove":                 true,
	"ArrayElemAt_ArrayWithMissing":     true,
	"ArrayElemAt_Literal":              true,
	"ArrayElemAt_NullLiteral":          true,
	"ArrayElemAt_OperatorOfMissing":    true,
	"Cond_ArrayWithMissing":            true,
	"Cond_Literal":                     true,
	"Cond_NullLiteral":                 true,
	"Cond_OperatorOfMissing":           true,
	"FindProjection_ArrayWithMissing":  true,
	"FindProjection_Literal":           true,
	"FindProjection_NullLiteral":       true,
	"FindProjection_OperatorOfMissing": true,
	"GroupIDMultipleKeys":              true,
	"GroupID_Literal":                  true,
	"GroupID_MissingSubpath":           true,
	"GroupID_Nested":                   true,
	"GroupID_NestedAllMissing":         true,
	"GroupID_NullField":                true,
	"GroupID_NullLiteral":              true,
	"GroupID_OnlyMissing":              true,
	"GroupID_OperatorOfMissing":        true,
	"GroupID_Remove":                   true,
	"IfNull_ArrayWithMissing":          true,
	"IfNull_Literal":                   true,
	"IfNull_NullLiteral":               true,
	"IfNull_OperatorOfMissing":         true,
	"Let_ArrayWithMissing":             true,
	"Let_Literal":                      true,
	"Let_NullLiteral":                  true,
	"Let_OperatorOfMissing":            true,
	"MergeObjects_ArrayWithMissing":    true,
	"MergeObjects_Literal":             true,
	"MergeObjects_NullLiteral":         true,
	"MergeObjects_OperatorOfMissing":   true,
	"ProjectTopLevelMissing":           true,
	"Project_ArrayWithMissing":         true,
	"Project_Literal":                  true,
	"Project_MissingField":             true,
	"Project_MissingSubpath":           true,
	"Project_Nested":                   true,
	"Project_NestedAllMissing":         true,
	"Project_NullField":                true,
	"Project_NullLiteral":              true,
	"Project_OnlyMissing":              true,
	"Project_OperatorOfMissing":        true,
	"Project_Remove":                   true,
	"ReplaceRoot_Literal":              true,
	"ReplaceRoot_NullLiteral":          true,
	"ReplaceRoot_OperatorOfMissing":    true}

func TestDocumentLiteralMissingFields(t *testing.T) {
	removeIf := func(cond interface{}) bson.D {
		return bson.D{{Key: "$cond", Value: bson.A{cond, "$i", "$$REMOVE"}}}
	}
	literals := []struct {
		name string
		doc  bson.D
	}{
		{"MissingField", bson.D{{Key: "x", Value: "$missing"}, {Key: "y", Value: int32(1)}}},
		{"OnlyMissing", bson.D{{Key: "x", Value: "$missing"}}},
		{"NullField", bson.D{{Key: "x", Value: "$n"}, {Key: "y", Value: "$i"}}},
		{"NullLiteral", bson.D{{Key: "x", Value: nil}}},
		{"Remove", bson.D{{Key: "x", Value: "$$REMOVE"}, {Key: "y", Value: "$i"}}},
		{"CondRemove", bson.D{{Key: "x", Value: removeIf(false)}, {Key: "y", Value: removeIf(true)}}},
		{"OperatorOfMissing", bson.D{{Key: "x", Value: bson.D{{Key: "$add", Value: bson.A{"$missing", int32(1)}}}}}},
		{"Nested", bson.D{{Key: "x", Value: bson.D{{Key: "y", Value: "$missing"}, {Key: "z", Value: "$i"}}}}},
		{"NestedAllMissing", bson.D{{Key: "x", Value: bson.D{{Key: "y", Value: "$missing"}}}}},
		{"ArrayWithMissing", bson.D{{Key: "x", Value: bson.A{"$missing", "$i"}}}},
		{"MissingSubpath", bson.D{{Key: "x", Value: "$o.q"}, {Key: "y", Value: "$o.p"}}},
		{"Literal", bson.D{{Key: "x", Value: bson.D{{Key: "$literal", Value: "$missing"}}}}},
		{"CondMissingBranch", bson.D{{Key: "x", Value: bson.D{{Key: "$cond", Value: bson.A{true, "$missing", int32(1)}}}}, {Key: "y", Value: "$i"}}},
		{"SwitchMissingDefault", bson.D{{Key: "x", Value: bson.D{{Key: "$switch", Value: bson.D{
			{Key: "branches", Value: bson.A{bson.D{{Key: "case", Value: false}, {Key: "then", Value: int32(1)}}}},
			{Key: "default", Value: "$missing"},
		}}}}}},
		{"IfNullMissingLast", bson.D{{Key: "x", Value: bson.D{{Key: "$ifNull", Value: bson.A{nil, "$missing"}}}}}},
		{"LetMissing", bson.D{{Key: "x", Value: bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{}}, {Key: "in", Value: "$missing"}}}}}}},
		{"ArrayElemAtOutOfRange", bson.D{{Key: "x", Value: bson.D{{Key: "$arrayElemAt", Value: bson.A{bson.A{int32(1)}, int32(5)}}}}, {Key: "y", Value: int32(2)}}},
	}
	expressions := []struct {
		name string
		wrap func(bson.D) interface{}
	}{
		{"ArrayElemAt", func(d bson.D) interface{} {
			return bson.D{{Key: "$arrayElemAt", Value: bson.A{bson.A{d}, int32(0)}}}
		}},
		{"Cond", func(d bson.D) interface{} {
			return bson.D{{Key: "$cond", Value: bson.A{true, d, int32(0)}}}
		}},
		{"IfNull", func(d bson.D) interface{} {
			return bson.D{{Key: "$ifNull", Value: bson.A{nil, d}}}
		}},
		{"MergeObjects", func(d bson.D) interface{} {
			return bson.D{{Key: "$mergeObjects", Value: bson.A{d, bson.D{{Key: "w", Value: int32(9)}}}}}
		}},
		{"Let", func(d bson.D) interface{} {
			return bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: bson.D{{Key: "v", Value: d}}}, {Key: "in", Value: "$$v"}}}}
		}},
	}

	sorted := func(stages ...bson.D) bson.A {
		p := bson.A{bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}}
		for _, s := range stages {
			p = append(p, s)
		}
		return p
	}
	type pipelineCase struct {
		name     string
		pipeline bson.A
	}
	var cases []pipelineCase
	for _, l := range literals {
		for _, e := range expressions {
			cases = append(cases, pipelineCase{e.name + "_" + l.name, sorted(
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: e.wrap(l.doc)}}}},
			)})
		}
		cases = append(cases,
			pipelineCase{"Project_" + l.name, sorted(bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: l.doc}}}})},
			pipelineCase{"AddFields_" + l.name, sorted(
				bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: l.doc}}}},
				bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: 1}}}},
			)},
			pipelineCase{"ReplaceRoot_" + l.name, sorted(bson.D{{Key: "$replaceRoot", Value: bson.D{{Key: "newRoot", Value: l.doc}}}})},
			pipelineCase{"GroupID_" + l.name, bson.A{
				bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: l.doc}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "n", Value: 1}, {Key: "_id", Value: 1}}}},
			}},
		)
	}
	cases = append(cases,
		pipelineCase{"ProjectTopLevelMissing", sorted(bson.D{{Key: "$project", Value: bson.D{{Key: "r", Value: "$missing"}, {Key: "s", Value: "$i"}}}})},
		pipelineCase{"AddFieldsTopLevelMissing", sorted(
			bson.D{{Key: "$addFields", Value: bson.D{{Key: "r", Value: "$missing"}, {Key: "i", Value: "$missing"}}}},
			bson.D{{Key: "$project", Value: bson.D{{Key: "i", Value: 1}, {Key: "r", Value: 1}, {Key: "g", Value: 1}}}},
		)},
		pipelineCase{"GroupIDMultipleKeys", bson.A{
			bson.D{{Key: "$group", Value: bson.D{{Key: "_id", Value: bson.D{{Key: "g", Value: "$g"}, {Key: "i", Value: "$i"}}}, {Key: "n", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
			bson.D{{Key: "$sort", Value: bson.D{{Key: "_id.g", Value: 1}, {Key: "_id.i", Value: 1}}}},
		}},
	)

	support := func(name string) harness.DumboDBSupport {
		if docLiteralMissingAlreadyMatching[name] {
			return harness.DumboDBFull
		}
		return harness.DumboDBXFail
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DocLiteralMissing_" + tc.name,
				Support: support(tc.name),
				Setup:   insertDocLiteralDocs,
				Run:     pipelineRunExtJSON(tc.pipeline),
			})
		})
	}

	for _, l := range literals {
		t.Run("FindProjection_"+l.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "DocLiteralMissing_FindProjection_" + l.name,
				Support: support("FindProjection_" + l.name),
				Setup:   insertDocLiteralDocs,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					opts := options.Find().SetProjection(bson.D{{Key: "r", Value: bson.D{{Key: "$ifNull", Value: bson.A{nil, l.doc}}}}}).SetSort(bson.D{{Key: "_id", Value: 1}})
					cursor, err := col.Find(ctx, bson.D{}, opts)
					if err != nil {
						return errOutcome(err), nil
					}
					var docs []bson.Raw
					if err = cursor.All(ctx, &docs); err != nil {
						return errOutcome(err), nil
					}
					out := make([]string, len(docs))
					for i, d := range docs {
						out[i] = d.String()
					}
					return out, nil
				},
			})
		})
	}
}
