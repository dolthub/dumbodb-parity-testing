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
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// mongod rejects a malformed field path in an expression when it parses the
// expression, makes a computed _id an inclusion, and refuses to store an
// _id that $out or $merge cannot write.

func projectionPathsSeed(ctx context.Context, col *mongo.Collection) error {
	_, err := col.InsertMany(ctx, []interface{}{
		bson.D{{Key: "_id", Value: int32(1)}, {Key: "a", Value: bson.D{{Key: "b", Value: int32(2)}}}, {Key: "arr", Value: bson.A{int32(1), int32(2)}}},
		bson.D{{Key: "_id", Value: int32(2)}, {Key: "re", Value: primitive.Regex{Pattern: "x"}}, {Key: "d", Value: bson.D{{Key: "q", Value: bson.A{bson.D{{Key: "$z", Value: int32(1)}}}}}}},
	})
	return err
}

func TestProjectionFieldPaths(t *testing.T) {
	project := func(spec bson.D) []bson.D { return []bson.D{{{Key: "$project", Value: spec}}} }
	set := func(spec bson.D) []bson.D { return []bson.D{{{Key: "$set", Value: spec}}} }
	cases := []struct {
		name   string
		stages []bson.D
	}{
		{"IDEmptyComponent", project(bson.D{{Key: "_id", Value: "$a..b"}})},
		{"IDRemove", append([]bson.D{{{Key: "$match", Value: bson.D{{Key: "_id", Value: int32(1)}}}}},
			project(bson.D{{Key: "_id", Value: "$$REMOVE"}, {Key: "a", Value: int32(1)}})...)},
		{"IDArrayExpression", project(bson.D{{Key: "_id", Value: bson.A{"$a.b", int32(1)}}, {Key: "k", Value: "$_id"}})},
		{"IDDollarComponent", project(bson.D{{Key: "_id", Value: "$a.$b"}})},
		{"FieldEmptyComponent", project(bson.D{{Key: "c", Value: "$a..b"}})},
		{"FieldDollarComponent", project(bson.D{{Key: "c", Value: "$a.$b"}})},
		{"OperatorArgument", project(bson.D{{Key: "c", Value: bson.D{{Key: "$add", Value: bson.A{"$a.$b", int32(1)}}}}})},
		{"NestedField", project(bson.D{{Key: "c", Value: bson.D{{Key: "d", Value: "$a..b"}}}})},
		{"DollarAlone", project(bson.D{{Key: "c", Value: "$"}})},
		{"SetEmptyComponent", set(bson.D{{Key: "c", Value: "$a..b"}})},
		{"SetDollarComponent", set(bson.D{{Key: "c", Value: "$a.$b"}})},
		{"SetDollarAlone", set(bson.D{{Key: "c", Value: "$"}})},
		{"SetLiteral", set(bson.D{{Key: "c", Value: bson.D{{Key: "$literal", Value: "$a..b"}}}})},
		{"IDExpressionThenExclusion", project(bson.D{{Key: "_id", Value: "$a"}, {Key: "b", Value: int32(0)}})},
		{"ExclusionThenIDExpression", project(bson.D{{Key: "b", Value: int32(0)}, {Key: "_id", Value: "$a"}})},
		{"IDOperatorThenExclusion", project(bson.D{{Key: "_id", Value: bson.D{{Key: "$add", Value: bson.A{int32(1), int32(1)}}}}, {Key: "b", Value: int32(0)}})},
		{"IDLiteralThenExclusion", project(bson.D{{Key: "_id", Value: "x"}, {Key: "b", Value: int32(0)}})},
		{"IDNumberThenExclusion", project(bson.D{{Key: "_id", Value: int32(5)}, {Key: "b", Value: int32(0)}})},
		{"ExclusionThenDottedIDExpression", project(bson.D{{Key: "b", Value: int32(0)}, {Key: "_id.x", Value: "$a"}})},
		{"ExclusionThenNestedIDExpression", project(bson.D{{Key: "b", Value: int32(0)}, {Key: "_id", Value: bson.D{{Key: "x", Value: "$a"}}}})},
		{"ExclusionThenFieldExpression", project(bson.D{{Key: "b", Value: int32(0)}, {Key: "c.x", Value: "$a"}})},
		{"ConstantConversionThenExclusion", project(bson.D{{Key: "_id", Value: bson.D{{Key: "$toDate", Value: int32(0)}}}, {Key: "b", Value: int32(0)}})},
		{"ConstantConversionAlone", project(bson.D{{Key: "x", Value: bson.D{{Key: "$toDate", Value: int32(0)}}}, {Key: "a", Value: int32(1)}})},
		{"IDExpressionThenInclusion", project(bson.D{{Key: "_id", Value: bson.D{{Key: "$concat", Value: bson.A{"x", bson.D{{Key: "$toString", Value: "$_id"}}}}}}, {Key: "a", Value: int32(1)}})},
	}
	for _, c := range cases {
		stages := c.stages
		harness.PairTest(t, harness.TestCase{
			Name:    "ProjectionFieldPaths_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   projectionPathsSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, append(stages, bson.D{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}})...)
			},
		})
	}

	finds := []struct {
		name string
		proj bson.D
	}{
		{"FindIDExpressionThenExclusion", bson.D{{Key: "_id", Value: "$a"}, {Key: "b", Value: int32(0)}}},
		{"FindExclusionThenIDExpression", bson.D{{Key: "b", Value: int32(0)}, {Key: "_id", Value: "$a"}}},
		{"FindConstantConversionThenExclusion", bson.D{{Key: "_id", Value: bson.D{{Key: "$toDate", Value: int32(0)}}}, {Key: "b", Value: int32(0)}}},
		{"FindExclusionThenDottedIDExpression", bson.D{{Key: "b", Value: int32(0)}, {Key: "_id.x", Value: "$a"}}},
		{"FindEmptyComponent", bson.D{{Key: "c", Value: "$a..b"}}},
		{"FindDollarComponent", bson.D{{Key: "c", Value: "$a.$b"}}},
	}
	for _, c := range finds {
		proj := c.proj
		harness.PairTest(t, harness.TestCase{
			Name:    "ProjectionFieldPaths_" + c.name,
			Support: harness.DumboDBFull,
			Setup:   projectionPathsSeed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return findWithProjection(ctx, col, bson.D{}, proj)
			},
		})
	}

	harness.PairTest(t, harness.TestCase{
		Name:    "ProjectionFieldPaths_UpdatePipelineSet",
		Support: harness.DumboDBFull,
		Setup:   projectionPathsSeed,
		Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
			_, err := col.UpdateOne(ctx, bson.D{}, mongo.Pipeline{{{Key: "$set", Value: bson.D{{Key: "c", Value: "$a..b"}}}}})
			return nil, err
		},
	})
}

func TestWrittenIDs(t *testing.T) {
	id := func(expr interface{}) bson.D {
		return bson.D{{Key: "$project", Value: bson.D{{Key: "_id", Value: expr}}}}
	}
	cases := []struct {
		name  string
		id    bson.D
		match int32
	}{
		{"Array", id("$arr"), 1},
		{"Regex", id("$re"), 2},
		{"DollarField", id(bson.D{{Key: "$literal", Value: bson.D{{Key: "$x", Value: int32(1)}}}}), 1},
		{"NestedDollarField", id("$d"), 2},
	}
	for _, c := range cases {
		for _, target := range []string{"$out", "$merge"} {
			c, target := c, target
			harness.PairTest(t, harness.TestCase{
				Name:    "WrittenIDs_" + target[1:] + "_" + c.name,
				Support: harness.DumboDBFull,
				Setup:   projectionPathsSeed,
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					into := col.Name() + "_out"
					defer col.Database().Collection(into).Drop(ctx)
					_, err := fieldOpsAgg(ctx, col,
						bson.D{{Key: "$match", Value: bson.D{{Key: "_id", Value: c.match}}}},
						c.id,
						bson.D{{Key: target, Value: into}},
					)
					return nil, err
				},
			})
		}
	}
}

// Flags on paths within _id include or exclude those paths of a document
// _id; an inclusion drops an _id that is not a document.
func TestProjectionIDSubpaths(t *testing.T) {
	seed := func(ctx context.Context, col *mongo.Collection) error {
		_, err := col.InsertMany(ctx, []interface{}{
			bson.D{{Key: "_id", Value: bson.D{{Key: "x", Value: int32(1)}, {Key: "y", Value: int32(2)}}}, {Key: "a", Value: int32(1)}, {Key: "b", Value: int32(2)}},
			bson.D{{Key: "_id", Value: int32(5)}, {Key: "a", Value: int32(2)}},
		})
		return err
	}
	projections := map[string]bson.D{
		"ExcludeSubpath":            {{Key: "_id.x", Value: int32(0)}},
		"ExcludeSubpathAndField":    {{Key: "_id.x", Value: int32(0)}, {Key: "a", Value: int32(0)}},
		"ExcludeFieldThenSubpath":   {{Key: "b", Value: int32(0)}, {Key: "_id.x", Value: int32(0)}},
		"IncludeSubpath":            {{Key: "_id.x", Value: int32(1)}},
		"IncludeSubpathAndField":    {{Key: "_id.x", Value: int32(1)}, {Key: "a", Value: int32(1)}},
		"IncludeSubpaths":           {{Key: "_id.x", Value: int32(1)}, {Key: "_id.y", Value: int32(1)}},
		"IncludeMissingSubpath":     {{Key: "_id.z", Value: int32(1)}},
		"IncludeSubpathAndComputed": {{Key: "_id.x", Value: int32(1)}, {Key: "c", Value: "$a"}},
	}
	for name, proj := range projections {
		proj := proj
		harness.PairTest(t, harness.TestCase{
			Name:    "ProjectionIDSubpaths_Find" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				cursor, err := col.Find(ctx, bson.D{}, options.Find().SetSort(bson.D{{Key: "a", Value: 1}}).SetProjection(proj))
				if err != nil {
					return nil, err
				}
				var docs []bson.D
				err = cursor.All(ctx, &docs)
				return docs, err
			},
		})
		harness.PairTest(t, harness.TestCase{
			Name:    "ProjectionIDSubpaths_Project" + name,
			Support: harness.DumboDBFull,
			Setup:   seed,
			Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
				return fieldOpsAgg(ctx, col, bson.D{{Key: "$sort", Value: bson.D{{Key: "a", Value: 1}}}}, bson.D{{Key: "$project", Value: proj}})
			},
		})
	}
}
