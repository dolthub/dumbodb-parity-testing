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

// A compound index refuses a document in which two of its fields pass through
// arrays that do not share a parent array, with CannotIndexParallelArrays, on
// insert, update and index build (dolthub/dumbodb#228).

import (
	"context"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/dolthub/dumbodb-parity-testing/harness"
)

// indexWriteCase checks one write against a collection holding an index, and
// the same document under an index built over it.
type indexWriteCase struct {
	name  string
	key   bson.D
	opts  *options.IndexOptions
	doc   bson.D
	write bson.D // a $set applied to the stored doc with _id 0; nil to skip
}

func runIndexWriteCases(t *testing.T, prefix string, cases []indexWriteCase) {
	createIndex := func(ctx context.Context, col *mongo.Collection, c indexWriteCase) error {
		_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: c.key, Options: c.opts})
		return err
	}
	for _, c := range cases {
		t.Run(c.name+"_insert", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    prefix + "_" + c.name + "_insert",
				Support: harness.DumboDBFull,
				Setup:   func(ctx context.Context, col *mongo.Collection) error { return createIndex(ctx, col, c) },
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.InsertOne(ctx, append(bson.D{{Key: "_id", Value: 1}}, c.doc...))
						return err
					})
				},
			})
		})
		t.Run(c.name+"_build", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    prefix + "_" + c.name + "_build",
				Support: harness.DumboDBFull,
				Setup:   insertDocs(append(bson.D{{Key: "_id", Value: 1}}, c.doc...)),
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					code := errCode(createIndex(ctx, col, c))
					specs, err := col.Indexes().ListSpecifications(ctx)
					if err != nil {
						return nil, err
					}
					return bson.D{{Key: "code", Value: code}, {Key: "indexes", Value: len(specs)}}, nil
				},
			})
		})
		if c.write == nil {
			continue
		}
		t.Run(c.name+"_update", func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    prefix + "_" + c.name + "_update",
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					if err := createIndex(ctx, col, c); err != nil {
						return err
					}
					_, err := col.InsertMany(ctx, []interface{}{bson.D{{Key: "_id", Value: 0}}, bson.D{{Key: "_id", Value: 1}}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					set := bson.D{{Key: "$set", Value: c.write}}
					return writeOutcome(ctx, col, func() error {
						if _, err := col.UpdateOne(ctx, bson.D{{Key: "_id", Value: 0}}, set); err != nil {
							return err
						}
						if err := col.FindOneAndUpdate(ctx, bson.D{{Key: "_id", Value: 1}}, set).Err(); err != nil {
							return err
						}
						return nil
					})
				},
			})
		})
	}
}

func TestIndexParallelArrays(t *testing.T) {
	ab := bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}
	runIndexWriteCases(t, "IndexParallelArrays", []indexWriteCase{
		{"both_arrays", ab, nil, bson.D{{Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: bson.A{3, 4}}},
			bson.D{{Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: bson.A{3, 4}}}},
		{"one_array", ab, nil, bson.D{{Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: 3}},
			bson.D{{Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: 3}}},
		{"three_fields", bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}, {Key: "c", Value: 1}}, nil,
			bson.D{{Key: "a", Value: 1}, {Key: "b", Value: bson.A{1}}, {Key: "c", Value: bson.A{2}}}, nil},
		{"shared_parent", bson.D{{Key: "a.x", Value: 1}, {Key: "a.y", Value: 1}}, nil,
			bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 2}}, bson.D{{Key: "x", Value: 3}, {Key: "y", Value: 4}}}}}, nil},
		{"shared_parent_one_nested", bson.D{{Key: "a.x", Value: 1}, {Key: "a.y", Value: 1}}, nil,
			bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "x", Value: bson.A{1}}, {Key: "y", Value: 2}}}}}, nil},
		{"shared_parent_both_nested", bson.D{{Key: "a.x", Value: 1}, {Key: "a.y", Value: 1}}, nil,
			bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "x", Value: bson.A{1}}, {Key: "y", Value: bson.A{2}}}}}}, nil},
		{"nested_and_top", bson.D{{Key: "a.x", Value: 1}, {Key: "b", Value: 1}}, nil,
			bson.D{{Key: "a", Value: bson.D{{Key: "x", Value: bson.A{1, 2}}}}, {Key: "b", Value: bson.A{3}}}, nil},
		{"positional", bson.D{{Key: "a.0", Value: 1}, {Key: "b", Value: 1}}, nil,
			bson.D{{Key: "a", Value: bson.A{bson.A{1, 2}}}, {Key: "b", Value: bson.A{3}}}, nil},
		{"sparse", ab, options.Index().SetSparse(true),
			bson.D{{Key: "a", Value: bson.A{1}}, {Key: "b", Value: bson.A{2}}}, nil},
		{"partial_excluded", ab, options.Index().SetPartialFilterExpression(bson.D{{Key: "c", Value: 1}}),
			bson.D{{Key: "a", Value: bson.A{1}}, {Key: "b", Value: bson.A{2}}}, nil},
		{"partial_included", ab, options.Index().SetPartialFilterExpression(bson.D{{Key: "c", Value: 1}}),
			bson.D{{Key: "a", Value: bson.A{1}}, {Key: "b", Value: bson.A{2}}, {Key: "c", Value: 1}}, nil},
	})
}

func TestIndexParallelArraysBatch(t *testing.T) {
	for _, ordered := range []bool{true, false} {
		name := "ordered"
		if !ordered {
			name = "unordered"
		}
		t.Run(name, func(t *testing.T) {
			harness.PairTest(t, harness.TestCase{
				Name:    "IndexParallelArraysBatch_" + name,
				Support: harness.DumboDBFull,
				Setup: func(ctx context.Context, col *mongo.Collection) error {
					_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}})
					return err
				},
				Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
					return writeOutcome(ctx, col, func() error {
						_, err := col.InsertMany(ctx, []interface{}{
							bson.D{{Key: "_id", Value: 1}, {Key: "a", Value: bson.A{1}}, {Key: "b", Value: 1}},
							bson.D{{Key: "_id", Value: 2}, {Key: "a", Value: bson.A{1}}, {Key: "b", Value: bson.A{2}}},
							bson.D{{Key: "_id", Value: 3}, {Key: "a", Value: 1}, {Key: "b", Value: bson.A{2}}},
						}, options.InsertMany().SetOrdered(ordered))
						return err
					})
				},
			})
		})
	}
}

// mongod does not check parallel arrays in a 2d or 2dsphere compound index,
// and counts an array a numeric path element reaches like any other.
func TestIndexParallelArraysPositionalAndGeo(t *testing.T) {
	ab := bson.D{{Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: bson.A{3, 4}}}
	elems := bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "x", Value: 1}, {Key: "y", Value: 2}}, bson.D{{Key: "x", Value: 3}, {Key: "y", Value: 4}}}}}
	runIndexWriteCases(t, "IndexParallelArraysPositionalAndGeo", []indexWriteCase{
		{"2dsphere_compound", bson.D{{Key: "loc", Value: "2dsphere"}, {Key: "a", Value: 1}, {Key: "b", Value: 1}}, nil,
			bson.D{{Key: "loc", Value: bson.D{{Key: "type", Value: "Point"}, {Key: "coordinates", Value: bson.A{0, 0}}}}, {Key: "a", Value: bson.A{1}}, {Key: "b", Value: bson.A{2}}}, nil},
		{"2d_compound", bson.D{{Key: "loc", Value: "2d"}, {Key: "a", Value: 1}, {Key: "b", Value: 1}}, nil,
			bson.D{{Key: "loc", Value: bson.A{0, 0}}, {Key: "a", Value: bson.A{1}}, {Key: "b", Value: bson.A{2}}}, nil},
		{"index_element_and_field", bson.D{{Key: "a.0", Value: 1}, {Key: "b", Value: 1}}, nil, ab, ab},
		{"second_index_element_and_field", bson.D{{Key: "a.1", Value: 1}, {Key: "b", Value: 1}}, nil, ab, nil},
		{"missing_index_element_and_field", bson.D{{Key: "a.5", Value: 1}, {Key: "b", Value: 1}}, nil, ab, nil},
		{"nested_index_element_and_field", bson.D{{Key: "a.0", Value: 1}, {Key: "b", Value: 1}}, nil,
			bson.D{{Key: "a", Value: bson.A{bson.A{1, 2}}}, {Key: "b", Value: bson.A{3}}}, nil},
		{"two_index_elements", bson.D{{Key: "a.0", Value: 1}, {Key: "a.1", Value: 1}}, nil, ab, nil},
		{"index_element_and_sibling", bson.D{{Key: "a.0", Value: 1}, {Key: "a.x", Value: 1}}, nil, elems, nil},
		{"index_element_child_and_sibling", bson.D{{Key: "a.0.x", Value: 1}, {Key: "a.y", Value: 1}}, nil, elems, nil},
	})
}

// An update that brings a document with parallel arrays into a partial or
// sparse compound index is refused, as an insert would be.
func TestIndexParallelArraysUpdateIntoIndex(t *testing.T) {
	for _, tc := range []struct {
		name   string
		opts   *options.IndexOptions
		stored bson.D
		update bson.D
	}{
		{"partial", options.Index().SetPartialFilterExpression(bson.D{{Key: "k", Value: 1}}),
			bson.D{{Key: "_id", Value: 1}, {Key: "k", Value: 2}, {Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: bson.A{3, 4}}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "k", Value: 1}}}}},
		{"sparse", options.Index().SetSparse(true),
			bson.D{{Key: "_id", Value: 1}, {Key: "c", Value: 1}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "a", Value: bson.A{1, 2}}, {Key: "b", Value: bson.A{3, 4}}}}}},
	} {
		for _, w := range []string{"update", "findAndModify"} {
			t.Run(tc.name+"_"+w, func(t *testing.T) {
				harness.PairTest(t, harness.TestCase{
					Name:    "IndexParallelArraysUpdateIntoIndex_" + tc.name + "_" + w,
					Support: harness.DumboDBFull,
					Setup: func(ctx context.Context, col *mongo.Collection) error {
						if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}, {Key: "b", Value: 1}}, Options: tc.opts}); err != nil {
							return err
						}
						_, err := col.InsertOne(ctx, tc.stored)
						return err
					},
					Run: func(ctx context.Context, col *mongo.Collection) (interface{}, error) {
						filter := bson.D{{Key: "_id", Value: 1}}
						return writeOutcome(ctx, col, func() error {
							if w == "update" {
								_, err := col.UpdateOne(ctx, filter, tc.update)
								return err
							}
							return col.FindOneAndUpdate(ctx, filter, tc.update).Err()
						})
					},
				})
			})
		}
	}
}

// mongod checks parallel arrays within each element of an array the index
// fields share, so arrays in different elements are not parallel.
func TestIndexParallelArraysPerElement(t *testing.T) {
	bc := bson.D{{Key: "a.b", Value: 1}, {Key: "a.c", Value: 1}}
	apart := bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "b", Value: bson.A{1}}}, bson.D{{Key: "c", Value: bson.A{2}}}}}}
	crossed := bson.D{{Key: "a", Value: bson.A{
		bson.D{{Key: "b", Value: 1}, {Key: "c", Value: bson.A{3, 4}}},
		bson.D{{Key: "b", Value: bson.A{1}}, {Key: "c", Value: 2}},
	}}}
	together := bson.D{{Key: "a", Value: bson.A{bson.D{{Key: "b", Value: bson.A{1}}, {Key: "c", Value: bson.A{2}}}}}}
	runIndexWriteCases(t, "IndexParallelArraysPerElement", []indexWriteCase{
		{"arrays_in_different_elements", bc, nil, apart, apart},
		{"arrays_crossed_across_elements", bc, nil, crossed, crossed},
		{"arrays_in_one_element", bc, nil, together, nil},
		{"partial_arrays_in_different_elements", bc, options.Index().SetPartialFilterExpression(bson.D{{Key: "k", Value: 1}}),
			append(bson.D{{Key: "k", Value: 1}}, apart...), nil},
	})
}
