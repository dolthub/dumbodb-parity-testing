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

// Edge-case parity coverage for find-projection aggregation expressions.
// Each test pins one specific MongoDB behavior; XFails document known gaps
// in DumboDB's implementation so future fixes have a clear signal.
//
// Gap summary (as of 2026-06-12):
//
//   System variables: only $$ROOT and $$CURRENT are resolved. $$NOW,
//   $$REMOVE, $$DESCEND, $$KEEP, $$PRUNE return their literal string.
//   Undefined $$variable names silently return the literal instead of
//   erroring (Location17276 "Use of undefined variable").
//
//   Operators in find projection: only $bsonSize is on the allowlist.
//   MongoDB accepts every standard aggregation operator. Each test below
//   pins one specific operator so the gap can be closed incrementally.
//
//   Bare "$" and "$$" forms: MongoDB returns specific errors
//   (Location16872 / FailedToParse). DumboDB returns InternalError or
//   silently treats "$$" as a literal.

package tests

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

func findEdgeSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertOne(ctx, bson.D{
		{Key: "_id", Value: "a"},
		{Key: "x", Value: int32(1)},
		{Key: "y", Value: "hi"},
		{Key: "n", Value: nil},
		{Key: "arr", Value: bson.A{int32(1), int32(2), int32(3)}},
		{Key: "nested", Value: bson.D{
			{Key: "a", Value: int32(1)},
			{Key: "b", Value: bson.D{
				{Key: "c", Value: int32(5)},
				{Key: "d", Value: nil},
			}},
		}},
	})
	return err
}

func runEdgeProj(ctx context.Context, col *mongo.Collection, projection bson.D) (interface{}, error) {
	cursor, err := col.Find(ctx, bson.D{}, options.Find().SetProjection(projection))
	if err != nil {
		return nil, err
	}
	var results []bson.D
	return results, cursor.All(ctx, &results)
}

// edgeCase is a single projection parity case. Adding a case here makes
// adding a test below near-mechanical.
type edgeCase struct {
	name    string
	support harness.DumboDBSupport
	proj    bson.D
}

func runEdgeCase(t *testing.T, c edgeCase) {
	t.Helper()
	harness.PairTest(t, harness.TestCase{
		Name:    c.name,
		Support: c.support,
		Setup:   findEdgeSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			return runEdgeProj(ctx, col, c.proj)
		},
	})
}

// --- A: deeper $$ROOT/path traversal ---------------------------------------

func TestFindProjEdge_A1_RootDotNestedB(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_A1_RootDotNestedB", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT.nested.b"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_A2_RootDotNestedBC(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_A2_RootDotNestedBC", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT.nested.b.c"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_A3_RootDotNestedBD_Null(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_A3_RootDotNestedBD_Null", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT.nested.b.d"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_A4_RootDotMissingNested(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_A4_RootDotMissingNested", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT.nested.b.missing"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_A5_DottedFieldPath(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_A5_DottedFieldPath", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$nested.b.c"}, {Key: "_id", Value: int32(0)}}})
}

// --- B: other system variables ---------------------------------------------

func TestFindProjEdge_B1_CurrentAlone(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_B1_CurrentAlone", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$CURRENT"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_B2_CurrentDotField(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_B2_CurrentDotField", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$CURRENT.x"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_B3_NowUnresolved(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_B3_NowUnresolved", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$NOW"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_B4_RemoveUnresolved(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_B4_RemoveUnresolved", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$REMOVE"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_B5_UndefinedVariable(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_B5_UndefinedVariable", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT_TYPO"}, {Key: "_id", Value: int32(0)}}})
}

// --- C: array and document field-path values -------------------------------

func TestFindProjEdge_C1_ArrayValue(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_C1_ArrayValue", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$arr"}, {Key: "_id", Value: int32(0)}}})
}

// $arr.0 is a *path* into a positional doc-child; MongoDB does NOT index
// arrays by string-numeric keys in expressions, so the path doesn't resolve.
// Both servers return an empty array under the projected field.
func TestFindProjEdge_C2_ArrayIndexPath(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_C2_ArrayIndexPath", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$arr.0"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_C3_SubdocFieldPath(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_C3_SubdocFieldPath", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$nested"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_C4_NullField(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_C4_NullField", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$n"}, {Key: "_id", Value: int32(0)}}})
}

// --- D: mixing aggregation-expression projection with classic include/exclude

func TestFindProjEdge_D1_ExprWithIncludeID(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_D1_ExprWithIncludeID", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT"}, {Key: "_id", Value: int32(1)}}})
}

func TestFindProjEdge_D2_ExprWithIncludeField(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_D2_ExprWithIncludeField", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT"}, {Key: "x", Value: int32(1)}, {Key: "_id", Value: int32(0)}}})
}

// Mixing an expression (which is inclusion-like) with a non-_id exclusion
// is illegal. Both servers reject with Location31254.
func TestFindProjEdge_D3_ExprWithExcludeFieldRejected(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_D3_ExprWithExcludeFieldRejected", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT"}, {Key: "x", Value: int32(0)}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_D4_MultipleExpressions(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_D4_MultipleExpressions", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT"}, {Key: "n", Value: "$x"}, {Key: "_id", Value: int32(0)}}})
}

// --- E: aggregation operators in find projection -------------------------

func TestFindProjEdge_E1_Add(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_E1_Add", harness.DumboDBFull,
		bson.D{{Key: "m", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(2)}}}}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_E2_ToString(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_E2_ToString", harness.DumboDBFull,
		bson.D{{Key: "m", Value: bson.D{{Key: "$toString", Value: "$x"}}}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_E3_Type(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_E3_Type", harness.DumboDBFull,
		bson.D{{Key: "m", Value: bson.D{{Key: "$type", Value: "$x"}}}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_E4_LiteralPreservesString(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_E4_LiteralPreservesString", harness.DumboDBFull,
		bson.D{{Key: "m", Value: bson.D{{Key: "$literal", Value: "$$ROOT"}}}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_E5_IfNull(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_E5_IfNull", harness.DumboDBFull,
		bson.D{{Key: "m", Value: bson.D{{Key: "$ifNull", Value: bson.A{"$missing", "fallback"}}}}, {Key: "_id", Value: int32(0)}}})
}

// --- F: weird/edge string forms --------------------------------------------

func TestFindProjEdge_F1_EmptyString(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_F1_EmptyString", harness.DumboDBFull,
		bson.D{{Key: "m", Value: ""}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_F2_BareDollarRejected(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_F2_BareDollarRejected", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_F3_BareDoubleDollarRejected(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_F3_BareDoubleDollarRejected", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$"}, {Key: "_id", Value: int32(0)}}})
}

func TestFindProjEdge_F4_RootDotNumericFails(t *testing.T) {
	runEdgeCase(t, edgeCase{"FindProjEdge_F4_RootDotNumericFails", harness.DumboDBFull,
		bson.D{{Key: "m", Value: "$$ROOT.0"}, {Key: "_id", Value: int32(0)}}})
}
