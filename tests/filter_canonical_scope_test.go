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

// DumboDB stores keys sorted, so it sorts the document values a filter
// compares against stored documents (workspace-2r6). Documents built by a
// pipeline keep the order they were built in, and order-sensitive
// expression arguments are not stored values; neither may be sorted.
// (review of #184: workspace-m8c.1, workspace-m8c.2, workspace-m8c.3)

func insertCanonicalScopeDocs(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "x", Value: bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "x", Value: bson.D{{Key: "a", Value: int32(3)}, {Key: "b", Value: int32(4)}}}},
	})
	return err
}

func insertPositionalArrayDocs(ctx context.Context, col *mongo.Collection) error {
	ab := bson.D{{Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}}
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "arr", Value: bson.A{ab, int32(5)}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "arr", Value: bson.A{bson.A{ab}}}},
		bson.D{{Key: "_id", Value: int32(3)}, {Key: "arr", Value: bson.A{int32(0), ab}}},
		bson.D{{Key: "_id", Value: int32(4)}, {Key: "arr", Value: bson.A{bson.D{{Key: "o", Value: ab}}}}},
	})
	return err
}

func canonicalFindIDs(filter interface{}) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
		if err != nil {
			return errOutcome(err), nil
		}
		return collectIDs(ctx, cursor)
	}
}

func canonicalAggregateIDs(pipeline bson.A) func(context.Context, *mongo.Collection) (interface{}, error) {
	return func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
		cursor, err := col.Aggregate(ctx, append(pipeline, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}}))
		if err != nil {
			return errOutcome(err), nil
		}
		return collectIDs(ctx, cursor)
	}
}

func TestFilterCanonicalizationScope(t *testing.T) {
	d := func(kv ...interface{}) bson.D {
		out := bson.D{}
		for i := 0; i < len(kv); i += 2 {
			out = append(out, bson.E{Key: kv[i].(string), Value: kv[i+1]})
		}
		return out
	}
	ba := d("b", int32(2), "a", int32(1))
	ab := d("a", int32(1), "b", int32(2))
	builtBA := d("$project", d("y", d("b", "$x.b", "a", "$x.a")))

	cases := []struct {
		name  string
		setup func(context.Context, *mongo.Collection) error
		run   func(context.Context, *mongo.Collection) (interface{}, error)
	}{
		// Documents built by a stage keep their built order.
		{"ProjectThenMatchBuiltOrder", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{builtBA, d("$match", d("y", ba))})},
		{"ProjectThenMatchSortedOrder", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{builtBA, d("$match", d("y", ab))})},
		{"ProjectThenMatchEq", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{builtBA, d("$match", d("y", d("$eq", ba)))})},
		{"ProjectThenMatchIn", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{builtBA, d("$match", d("y", d("$in", bson.A{ba})))})},
		{"ProjectThenMatchExpr", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{builtBA, d("$match", d("$expr", d("$eq", bson.A{"$y", d("$literal", ba)})))})},
		{"ReplaceRootThenMatch", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{
			d("$replaceRoot", d("newRoot", d("_id", "$_id", "y", d("b", "$x.b", "a", "$x.a")))), d("$match", d("y", ba)),
		})},
		{"AddFieldsThenMatch", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{
			d("$addFields", d("y", d("b", "$x.b", "a", "$x.a"))), d("$match", d("y", ba)),
		})},
		{"GroupThenMatch", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{
			d("$group", d("_id", "$_id", "y", d("$first", d("b", "$x.b", "a", "$x.a")))), d("$match", d("y", ba)),
		})},
		{"MatchAfterSortSeesStored", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{
			d("$sort", d("_id", int32(-1))), d("$limit", int32(5)), d("$match", d("x", ab)),
		})},
		{"LookupPipelineBuiltOrder", insertCanonicalScopeDocs, func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			cursor, err := col.Aggregate(ctx, bson.A{
				d("$match", d("_id", int32(1))),
				d("$lookup", d("from", col.Name(), "as", "j", "pipeline", bson.A{builtBA, d("$match", d("y", ba))})),
				d("$project", d("n", d("$size", "$j"))),
			})
			if err != nil {
				return errOutcome(err), nil
			}
			var docs []bson.M
			if err = cursor.All(ctx, &docs); err != nil {
				return errOutcome(err), nil
			}
			return docs, nil
		}},

		// Order-sensitive arguments under $expr are not stored values.
		{"ExprObjectToArrayOrder", insertCanonicalScopeDocs, canonicalFindIDs(d("$expr", d("$eq", bson.A{
			d("$objectToArray", d("b", int32(1), "a", int32(1))),
			bson.A{d("k", "b", "v", int32(1)), d("k", "a", "v", int32(1))},
		})))},
		{"ExprSortArraySortByOrder", insertCanonicalScopeDocs, canonicalFindIDs(d("$expr", d("$eq", bson.A{
			d("$arrayElemAt", bson.A{d("$sortArray", d("input", bson.A{d("a", int32(1), "b", int32(2)), d("a", int32(2), "b", int32(1))}, "sortBy", d("b", int32(1), "a", int32(1)))), int32(0)}),
			d("$literal", d("a", int32(2), "b", int32(1))),
		})))},
		{"ExprLiteralsDifferingInOrder", insertCanonicalScopeDocs, canonicalFindIDs(d("$expr", d("$eq", bson.A{d("$literal", ba), d("$literal", ab)})))},
		{"ExprLiteralsSameOrder", insertCanonicalScopeDocs, canonicalFindIDs(d("$expr", d("$eq", bson.A{d("$literal", ba), d("$literal", ba)})))},
		{"ExprObjectLiteralKeys", insertCanonicalScopeDocs, canonicalFindIDs(d("$expr", d("$eq", bson.A{
			d("$arrayElemAt", bson.A{d("$map", d("input", d("$objectToArray", d("z", int32(1), "a", int32(2))), "in", "$$this.k")), int32(0)}), "z",
		})))},
		{"AggregateExprObjectToArrayOrder", insertCanonicalScopeDocs, canonicalAggregateIDs(bson.A{d("$match", d("$expr", d("$eq", bson.A{
			d("$objectToArray", d("b", int32(1), "a", int32(1))),
			bson.A{d("k", "b", "v", int32(1)), d("k", "a", "v", int32(1))},
		})))})},

		// A numeric path component does not expand the array it reaches.
		{"PositionalImplicitEq", insertPositionalArrayDocs, canonicalFindIDs(d("arr.0", ab))},
		{"PositionalEq", insertPositionalArrayDocs, canonicalFindIDs(d("arr.0", d("$eq", ab)))},
		{"PositionalNe", insertPositionalArrayDocs, canonicalFindIDs(d("arr.0", d("$ne", ab)))},
		{"PositionalSecond", insertPositionalArrayDocs, canonicalFindIDs(d("arr.1", ab))},
		{"PositionalWholeNested", insertPositionalArrayDocs, canonicalFindIDs(d("arr.0", bson.A{ab}))},
		{"PositionalThenField", insertPositionalArrayDocs, canonicalFindIDs(d("arr.0.o", ab))},
		{"ArrayElement", insertPositionalArrayDocs, canonicalFindIDs(d("arr", ab))},
		{"ArrayElementNe", insertPositionalArrayDocs, canonicalFindIDs(d("arr", d("$ne", ab)))},
		{"ArrayFieldPath", insertPositionalArrayDocs, canonicalFindIDs(d("arr.o", ab))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "CanonicalScope_" + tc.name,
				Support: harness.DumboDBFull,
				Setup:   tc.setup,
				Run:     tc.run,
			})
		})
	}
}
