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

// Parity matrix for $getField / $setField / $unsetField (MongoDB 5.0+).
//
// These operators exist to reach field names that dot-notation cannot express:
// names containing a literal dot, names beginning with a dollar sign, and names
// computed at runtime. The cases below therefore lean on those shapes rather
// than on ordinary names, and several assert the negative: that the operators
// do NOT perform the path traversal a dot-path would.
//
// Expected values come from MongoDB at run time, so the assertions are whatever
// the oracle does; these cases were recorded against 8.0.28 while the operators
// were still unimplemented, and became the specification DumboDB was built to.

func fieldOpsAgg(ctx context.Context, col *mongo.Collection, stages ...bson.D) (interface{}, error) {
	cursor, err := col.Aggregate(ctx, mongo.Pipeline(stages))
	if err != nil {
		return nil, err
	}
	var results []bson.D
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// fieldOpsProject runs a single $project stage and returns the resulting docs.
func fieldOpsProject(ctx context.Context, col *mongo.Collection, projection bson.D) (interface{}, error) {
	return fieldOpsAgg(ctx, col, bson.D{{Key: "$project", Value: projection}})
}

// getFieldOnRoot projects one computed field named "got" from a $getField
// applied to the whole document.
func getFieldOnRoot(ctx context.Context, col *mongo.Collection, getField interface{}) (interface{}, error) {
	return fieldOpsProject(ctx, col, bson.D{
		{Key: "_id", Value: 0},
		{Key: "got", Value: getField},
	})
}

func fieldOpsSeedPlain(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "f1"},
		{Key: "plain", Value: int32(1)},
		{Key: "nameHolder", Value: "plain"},
		{Key: "prefix", Value: "pl"},
		{Key: "sub", Value: bson.D{{Key: "inner", Value: "v"}}},
		{Key: "arr", Value: bson.A{
			bson.D{{Key: "k", Value: int32(1)}},
			bson.D{{Key: "k", Value: int32(2)}},
		}},
	})
	return err
}

// fieldOpsSeedDotted stores both a literal "price.usd" key and a nested
// price.usd path in one document. This is the sharpest statement of what
// dot-notation cannot express: "$price.usd" can only reach the nested value.
func fieldOpsSeedDotted(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "f1"},
		{Key: "price.usd", Value: int32(10)},
		{Key: "price", Value: bson.D{{Key: "usd", Value: int32(20)}}},
		{Key: "a.b.c", Value: int32(30)},
	})
	return err
}

func fieldOpsSeedDollar(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "f1"},
		{Key: "$weird", Value: int32(1)},
		{Key: "$$weird", Value: int32(2)},
	})
	return err
}

func fieldOpsSeedEdgeNames(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "f1"},
		{Key: ".x", Value: int32(1)},
		{Key: "x.", Value: int32(2)},
		{Key: "", Value: int32(3)},
	})
	return err
}

// ---------------------------------------------------------------------------
// $getField: form
// ---------------------------------------------------------------------------

func TestGetField_ShorthandOrdinaryName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_ShorthandOrdinaryName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: "plain"}})
		},
	})
}

func TestGetField_FullFormOrdinaryName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_FullFormOrdinaryName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "plain"},
				{Key: "input", Value: "$$CURRENT"},
			}}})
		},
	})
}

// Shorthand cannot express a dollar-prefixed name: the string is parsed as a
// field path, not a literal key.
func TestGetField_ShorthandDollarNameInvalid(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_ShorthandDollarNameInvalid",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDollar,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: "$weird"}})
		},
	})
}

// ---------------------------------------------------------------------------
// $getField: field name shapes
// ---------------------------------------------------------------------------

func TestGetField_LiteralDotKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_LiteralDotKey",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: "price.usd"}})
		},
	})
}

func TestGetField_MultipleDotsKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_MultipleDotsKey",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: "a.b.c"}})
		},
	})
}

// The ambiguity case: one document, two different "price.usd" values reachable
// only by different access paths. Pins which value each path resolves to.
func TestGetField_DottedKeyVersusNestedPath(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_DottedKeyVersusNestedPath",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "viaGetField", Value: bson.D{{Key: "$getField", Value: "price.usd"}}},
				{Key: "viaDotPath", Value: "$price.usd"},
			})
		},
	})
}

func TestGetField_DollarPrefixedName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_DollarPrefixedName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDollar,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: bson.D{{Key: "$literal", Value: "$weird"}}},
				{Key: "input", Value: "$$CURRENT"},
			}}})
		},
	})
}

func TestGetField_DoubleDollarPrefixedName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_DoubleDollarPrefixedName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDollar,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: bson.D{{Key: "$literal", Value: "$$weird"}}},
				{Key: "input", Value: "$$CURRENT"},
			}}})
		},
	})
}

func TestGetField_LeadingDotName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_LeadingDotName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedEdgeNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: ".x"}})
		},
	})
}

func TestGetField_TrailingDotName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_TrailingDotName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedEdgeNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: "x."}})
		},
	})
}

func TestGetField_EmptyStringName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_EmptyStringName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedEdgeNames,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: ""}})
		},
	})
}

// ---------------------------------------------------------------------------
// $getField: field argument forms
// ---------------------------------------------------------------------------

func TestGetField_DynamicFieldNameFromDocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_DynamicFieldNameFromDocument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "$nameHolder"},
				{Key: "input", Value: "$$CURRENT"},
			}}})
		},
	})
}

func TestGetField_ComputedFieldName(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_ComputedFieldName",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: bson.D{{Key: "$concat", Value: bson.A{"$prefix", "ain"}}}},
				{Key: "input", Value: "$$CURRENT"},
			}}})
		},
	})
}

func TestGetField_NonStringFieldError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_NonStringFieldError",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: int32(123)},
				{Key: "input", Value: "$$CURRENT"},
			}}})
		},
	})
}

func TestGetField_UnknownArgumentError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_UnknownArgumentError",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "plain"},
				{Key: "input", Value: "$$CURRENT"},
				{Key: "bogus", Value: int32(1)},
			}}})
		},
	})
}

// ---------------------------------------------------------------------------
// $getField: input argument
// ---------------------------------------------------------------------------

func TestGetField_InputExplicitObject(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InputExplicitObject",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "inner"},
				{Key: "input", Value: bson.D{{Key: "inner", Value: "literal"}}},
			}}})
		},
	})
}

func TestGetField_InputExpressionSubdocument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InputExpressionSubdocument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "inner"},
				{Key: "input", Value: "$sub"},
			}}})
		},
	})
}

// An absent field must yield missing, not null: $project omits the key entirely.
func TestGetField_AbsentFieldIsMissing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_AbsentFieldIsMissing",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "keep", Value: "$plain"},
				{Key: "got", Value: bson.D{{Key: "$getField", Value: "nosuch"}}},
			})
		},
	})
}

func TestGetField_InputNull(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InputNull",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "plain"},
				{Key: "input", Value: nil},
			}}})
		},
	})
}

func TestGetField_InputNonObjectError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InputNonObjectError",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "plain"},
				{Key: "input", Value: int32(5)},
			}}})
		},
	})
}

// ---------------------------------------------------------------------------
// $getField: traversal boundaries
// ---------------------------------------------------------------------------

// $getField reads one key. It must not descend a dot path the way "$sub.inner"
// does, so this returns missing rather than "v".
func TestGetField_NoDotPathTraversal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_NoDotPathTraversal",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "viaGetField", Value: bson.D{{Key: "$getField", Value: "sub.inner"}}},
				{Key: "viaDotPath", Value: "$sub.inner"},
			})
		},
	})
}

// Dot-notation maps over arrays ("$arr.k" yields [1,2]); $getField must not.
func TestGetField_NoArrayTraversal(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_NoArrayTraversal",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "viaGetField", Value: bson.D{{Key: "$getField", Value: "arr.k"}}},
				{Key: "viaDotPath", Value: "$arr.k"},
			})
		},
	})
}

func TestGetField_NestedComposition(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_NestedComposition",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "inner"},
				{Key: "input", Value: bson.D{{Key: "$getField", Value: "sub"}}},
			}}})
		},
	})
}

// ---------------------------------------------------------------------------
// $getField: host stages
// ---------------------------------------------------------------------------

func TestGetField_InAddFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InAddFields",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$addFields", Value: bson.D{
					{Key: "copied", Value: bson.D{{Key: "$getField", Value: "price.usd"}}},
				}}},
			)
		},
	})
}

func TestGetField_InReplaceWith(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InReplaceWith",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$getField", Value: "sub"}}}},
			)
		},
	})
}

func TestGetField_InMatchExpr(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InMatchExpr",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$match", Value: bson.D{
					{Key: "$expr", Value: bson.D{{Key: "$eq", Value: bson.A{
						bson.D{{Key: "$getField", Value: "price.usd"}}, int32(10),
					}}}},
				}}},
			)
		},
	})
}

func TestGetField_InGroupId(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InGroupId",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$group", Value: bson.D{
					{Key: "_id", Value: bson.D{{Key: "$getField", Value: "price.usd"}}},
					{Key: "n", Value: bson.D{{Key: "$sum", Value: int32(1)}}},
				}}},
			)
		},
	})
}

func TestGetField_InSortViaAddFields(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_InSortViaAddFields",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$addFields", Value: bson.D{
					{Key: "sortKey", Value: bson.D{{Key: "$getField", Value: "price.usd"}}},
				}}},
				bson.D{{Key: "$sort", Value: bson.D{{Key: "sortKey", Value: int32(1)}}}},
			)
		},
	})
}

// ---------------------------------------------------------------------------
// $setField
// ---------------------------------------------------------------------------

// setFieldOnRoot replaces the document with $setField applied to $$ROOT.
func setFieldOnRoot(ctx context.Context, col *mongo.Collection, field, value interface{}) (interface{}, error) {
	return fieldOpsAgg(ctx, col,
		bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$setField", Value: bson.D{
			{Key: "field", Value: field},
			{Key: "input", Value: "$$ROOT"},
			{Key: "value", Value: value},
		}}}}},
	)
}

func TestSetField_DottedKeyScalar(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_DottedKeyScalar",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "price.usd", int32(99))
		},
	})
}

func TestSetField_NewDottedKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_NewDottedKey",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "brand.new", int32(7))
		},
	})
}

func TestSetField_DollarPrefixedKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_DollarPrefixedKey",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDollar,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col,
				bson.D{{Key: "$literal", Value: "$weird"}}, int32(42))
		},
	})
}

func TestSetField_DocumentValue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_DocumentValue",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "price.usd",
				bson.D{{Key: "amount", Value: int32(5)}})
		},
	})
}

func TestSetField_ArrayValue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_ArrayValue",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "price.usd", bson.A{int32(1), int32(2)})
		},
	})
}

func TestSetField_NullValue(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_NullValue",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "price.usd", nil)
		},
	})
}

// $$REMOVE deletes the field rather than storing a value.
func TestSetField_RemoveDeletesField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_RemoveDeletesField",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "price.usd", "$$REMOVE")
		},
	})
}

// $setField with $$REMOVE and $unsetField must produce the same document.
func TestSetField_RemoveMatchesUnsetField(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_RemoveMatchesUnsetField",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "viaRemove", Value: bson.D{{Key: "$setField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: "$$ROOT"},
					{Key: "value", Value: "$$REMOVE"},
				}}}},
				{Key: "viaUnset", Value: bson.D{{Key: "$unsetField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: "$$ROOT"},
				}}}},
			})
		},
	})
}

func TestSetField_MissingValueArgumentError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_MissingValueArgumentError",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$setField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: "$$ROOT"},
				}}}}},
			)
		},
	})
}

func TestSetField_NonStringFieldError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_NonStringFieldError",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, int32(123), int32(1))
		},
	})
}

// ---------------------------------------------------------------------------
// $unsetField
// ---------------------------------------------------------------------------

func unsetFieldOnRoot(ctx context.Context, col *mongo.Collection, field interface{}) (interface{}, error) {
	return fieldOpsAgg(ctx, col,
		bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$unsetField", Value: bson.D{
			{Key: "field", Value: field},
			{Key: "input", Value: "$$ROOT"},
		}}}}},
	)
}

func TestUnsetField_DottedKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_DottedKey",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return unsetFieldOnRoot(ctx, col, "price.usd")
		},
	})
}

func TestUnsetField_DollarPrefixedKey(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_DollarPrefixedKey",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDollar,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return unsetFieldOnRoot(ctx, col, bson.D{{Key: "$literal", Value: "$weird"}})
		},
	})
}

// Unsetting a key the document does not carry is a no-op, not an error.
func TestUnsetField_AbsentKeyIsNoOp(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_AbsentKeyIsNoOp",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return unsetFieldOnRoot(ctx, col, "nosuch")
		},
	})
}

// Removing the literal "price.usd" key must leave the nested price.usd intact.
func TestUnsetField_DottedKeyLeavesNestedPath(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_DottedKeyLeavesNestedPath",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsAgg(ctx, col,
				bson.D{{Key: "$replaceWith", Value: bson.D{{Key: "$unsetField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: "$$ROOT"},
				}}}}},
				bson.D{{Key: "$project", Value: bson.D{
					{Key: "_id", Value: 0},
					{Key: "nested", Value: "$price.usd"},
				}}},
			)
		},
	})
}

func TestUnsetField_NonStringFieldError(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_NonStringFieldError",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return unsetFieldOnRoot(ctx, col, int32(123))
		},
	})
}

// ---------------------------------------------------------------------------
// Round trips
// ---------------------------------------------------------------------------

func TestFieldOps_SetThenGetRoundTrip(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FieldOps_SetThenGetRoundTrip",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "round.trip"},
				{Key: "input", Value: bson.D{{Key: "$setField", Value: bson.D{
					{Key: "field", Value: "round.trip"},
					{Key: "input", Value: "$$ROOT"},
					{Key: "value", Value: "stored"},
				}}}},
			}}})
		},
	})
}

func TestFieldOps_UnsetThenGetIsMissing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "FieldOps_UnsetThenGetIsMissing",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "keep", Value: int32(1)},
				{Key: "got", Value: bson.D{{Key: "$getField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: bson.D{{Key: "$unsetField", Value: bson.D{
						{Key: "field", Value: "price.usd"},
						{Key: "input", Value: "$$ROOT"},
					}}}},
				}}}},
			})
		},
	})
}

// ---------------------------------------------------------------------------
// Argument contract
//
// The full form requires both 'field' and 'input' for all three operators;
// only the $getField shorthand defaults to $$CURRENT. The two families report
// the same conditions under different codes ($getField 3041701/02/03 against
// the $setField family 4161101/02/09), and $unsetField names itself here even
// though it reports a non-string 'field' as $setField.
// ---------------------------------------------------------------------------

func TestGetField_MissingFieldArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_MissingFieldArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "input", Value: "$$ROOT"},
			}}})
		},
	})
}

func TestGetField_MissingInputArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_MissingInputArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "plain"},
			}}})
		},
	})
}

func TestSetField_MissingFieldArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_MissingFieldArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "got", Value: bson.D{{Key: "$setField", Value: bson.D{
					{Key: "input", Value: "$$ROOT"},
					{Key: "value", Value: int32(9)},
				}}}},
			})
		},
	})
}

func TestSetField_MissingInputArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_MissingInputArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "got", Value: bson.D{{Key: "$setField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "value", Value: int32(9)},
				}}}},
			})
		},
	})
}

// $unsetField names itself when an argument is missing, unlike the non-string
// 'field' case which it reports as $setField.
func TestUnsetField_MissingFieldArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_MissingFieldArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "got", Value: bson.D{{Key: "$unsetField", Value: bson.D{
					{Key: "input", Value: "$$ROOT"},
				}}}},
			})
		},
	})
}

func TestUnsetField_MissingInputArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_MissingInputArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "got", Value: bson.D{{Key: "$unsetField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
				}}}},
			})
		},
	})
}

// 'value' belongs to $setField only; $unsetField rejects it as unknown.
func TestUnsetField_RejectsValueArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_RejectsValueArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "got", Value: bson.D{{Key: "$unsetField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: "$$ROOT"},
					{Key: "value", Value: int32(1)},
				}}}},
			})
		},
	})
}

// The $setField family uses its own unknown-argument code, distinct from the
// $getField one exercised above.
func TestSetField_UnknownArgument(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_UnknownArgument",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedDotted,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "got", Value: bson.D{{Key: "$setField", Value: bson.D{
					{Key: "field", Value: "price.usd"},
					{Key: "input", Value: "$$ROOT"},
					{Key: "value", Value: int32(1)},
					{Key: "bogus", Value: int32(1)},
				}}}},
			})
		},
	})
}

// ---------------------------------------------------------------------------
// 'field' argument evaluation
//
// $getField resolves a dynamic field name; $setField and $unsetField do not.
// They require a constant and reject anything else while parsing, under two
// codes: 4161108 for a field path or variable reference, 4161106 for a
// non-constant expression. MongoDB renders the offending reference in its own
// normalised form, dropping one dollar from a variable and routing a bare path
// through $CURRENT.
// ---------------------------------------------------------------------------

// A 'field' that evaluates to missing is an error, not a reason to drop the
// field being projected.
func TestGetField_FieldEvaluatesToMissing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_FieldEvaluatesToMissing",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return fieldOpsProject(ctx, col, bson.D{
				{Key: "_id", Value: 0},
				{Key: "keep", Value: "$plain"},
				{Key: "got", Value: bson.D{{Key: "$getField", Value: bson.D{
					{Key: "field", Value: "$$REMOVE"},
					{Key: "input", Value: "$$ROOT"},
				}}}},
			})
		},
	})
}

// An absent field path is the same condition reached a different way.
func TestGetField_FieldPathResolvesToMissing(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_FieldPathResolvesToMissing",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "$nosuch"},
				{Key: "input", Value: "$$ROOT"},
			}}})
		},
	})
}

// A non-string that is present rather than missing already matched, so this
// guards the "but got <type>" branch while the missing branch is fixed.
func TestGetField_FieldEvaluatesToObject(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "GetField_FieldEvaluatesToObject",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return getFieldOnRoot(ctx, col, bson.D{{Key: "$getField", Value: bson.D{
				{Key: "field", Value: "$$ROOT"},
				{Key: "input", Value: "$$ROOT"},
			}}})
		},
	})
}

// $setField rejects a variable reference as 'field'. The message drops one
// dollar, naming '$REMOVE'.
func TestSetField_FieldVariableReferenceRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_FieldVariableReferenceRejected",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "$$REMOVE", int32(1))
		},
	})
}

func TestUnsetField_FieldVariableReferenceRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "UnsetField_FieldVariableReferenceRejected",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return unsetFieldOnRoot(ctx, col, "$$REMOVE")
		},
	})
}

// A bare field path is reported normalised through $CURRENT.
func TestSetField_FieldPathReferenceRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_FieldPathReferenceRejected",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col, "$nameHolder", int32(1))
		},
	})
}

// A computed expression is rejected under a different code from a reference.
func TestSetField_FieldNonConstantRejected(t *testing.T) {
	harness.PairTest(t, harness.TestCase{
		Name:    "SetField_FieldNonConstantRejected",
		Support: harness.DumboDBFull,
		Setup:   fieldOpsSeedPlain,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return setFieldOnRoot(ctx, col,
				bson.D{{Key: "$concat", Value: bson.A{"$prefix", "ain"}}}, int32(1))
		},
	})
}
