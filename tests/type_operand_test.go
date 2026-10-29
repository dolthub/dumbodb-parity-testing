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

func insertTypeOperandDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(0)}, {Key: "s", Value: "$i"}, {Key: "i", Value: int32(1)}, {Key: "n", Value: nil}},
	})
	return err
}

// typeOperandAlreadyMatching are the cases DumboDB matched before $type used
// the shared operand evaluator (workspace-9nx).
var typeOperandAlreadyMatching = map[string]bool{
	"ArrayLiteralOfExpressions":   true,
	"Current":                     true,
	"DocumentLiteral":             true,
	"FieldValueLooksLikePath":     true,
	"Missing":                     true,
	"NestedType":                  true,
	"Null":                        true,
	"OperatorResultLooksLikePath": true,
	"Remove":                      true,
	"Root":                        true,
	"RootField":                   true,
	"UnknownNestedOperator":       true,
}

// TestTypeOperand covers how $type evaluates its operand: variables, nested
// expressions whose results look like field paths, array and document
// literals, and the missing value.
func TestTypeOperand(t *testing.T) {
	typeOf := func(arg interface{}) bson.D { return bson.D{{Key: "$type", Value: arg}} }
	let := func(vars bson.D, in interface{}) bson.D {
		return bson.D{{Key: "$let", Value: bson.D{{Key: "vars", Value: vars}, {Key: "in", Value: in}}}}
	}

	cases := []mathCase{
		{"LetVariable", let(bson.D{{Key: "v", Value: 0.5}}, typeOf("$$v"))},
		{"LetVariableString", let(bson.D{{Key: "v", Value: "x"}}, typeOf("$$v"))},
		{"LetVariableDocumentPath", let(bson.D{{Key: "v", Value: bson.D{{Key: "a", Value: int64(1)}}}}, typeOf("$$v.a"))},
		{"LetVariableMissingPath", let(bson.D{{Key: "v", Value: bson.D{}}}, typeOf("$$v.a"))},
		{"Root", typeOf("$$ROOT")},
		{"RootField", typeOf("$$ROOT.i")},
		{"Current", typeOf("$$CURRENT.n")},
		{"Remove", typeOf("$$REMOVE")},
		{"FieldValueLooksLikePath", typeOf("$s")},
		{"OperatorResultLooksLikePath", typeOf(bson.D{{Key: "$concat", Value: bson.A{"$", "i"}}})},
		{"LiteralLooksLikePath", typeOf(bson.D{{Key: "$literal", Value: "$i"}})},
		{"Missing", typeOf("$missing")},
		{"Null", typeOf("$n")},
		{"OperatorMissing", typeOf(bson.D{{Key: "$ifNull", Value: bson.A{nil, "$missing"}}})},
		{"ArrayLiteralOfExpressions", typeOf(bson.A{bson.A{"$i", "$missing"}})},
		{"DocumentLiteral", typeOf(bson.D{{Key: "a", Value: "$i"}})},
		{"NestedType", typeOf(typeOf("$i"))},
		{"UnknownNestedOperator", typeOf(bson.D{{Key: "$nope", Value: int32(1)}})},
	}
	for _, tc := range cases {
		support := harness.DumboDBXFail
		if typeOperandAlreadyMatching[tc.name] {
			support = harness.DumboDBFull
		}
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "TypeOperand_" + tc.name,
				Support: support,
				Setup:   insertTypeOperandDocs,
				Run:     projectTyped(tc.expr),
			})
		})
	}
}
